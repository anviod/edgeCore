package knxnetip

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"go.uber.org/zap"
)

type packetConn interface {
	ReadFrom(p []byte) (n int, addr net.Addr, err error)
	WriteTo(p []byte, addr net.Addr) (n int, err error)
	Close() error
	LocalAddr() net.Addr
}

// KNXTransport manages KNXnet/IP tunneling over UDP or TCP.
type KNXTransport struct {
	cfg transportConfig

	conn packetConn
	tcp  net.Conn

	remoteUDPAddr *net.UDPAddr

	dialFn func(cfg transportConfig) (packetConn, *net.UDPAddr, error)

	connected          atomic.Bool
	mu                 sync.Mutex
	writeMu            sync.Mutex
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string

	channelID atomic.Uint32
	seq       atomic.Uint32
	srcAddr   uint16

	cacheMu sync.RWMutex
	cache   map[uint16][]byte

	connMgr *driver.ConnectionManager
}

func NewKNXTransport(cfg map[string]any) *KNXTransport {
	tc := parseTransportConfig(cfg)
	t := &KNXTransport{
		cfg:        tc,
		remoteAddr: tc.remoteAddr(),
		cache:      make(map[uint16][]byte),
	}
	t.dialFn = defaultUDPDial
	t.connMgr = driver.NewConnectionManager("knxnetip")
	t.connMgr.SetMaxRetries(tc.maxRetries)
	return t
}

func defaultUDPDial(cfg transportConfig) (packetConn, *net.UDPAddr, error) {
	if cfg.ip == "" {
		return nil, nil, fmt.Errorf("KNXnet/IP: gateway IP not configured")
	}
	remote, err := net.ResolveUDPAddr("udp4", cfg.remoteAddr())
	if err != nil {
		return nil, nil, err
	}

	var lc net.ListenConfig
	var conn packetConn
	var err2 error
	if cfg.localIP != "" {
		laddr, err := net.ResolveUDPAddr("udp4", cfg.localIP+":0")
		if err != nil {
			return nil, nil, err
		}
		conn, err2 = lc.ListenPacket(context.Background(), "udp4", laddr.String())
	} else {
		conn, err2 = lc.ListenPacket(context.Background(), "udp4", ":0")
	}
	if err2 != nil {
		return nil, nil, err2
	}
	return conn, remote, nil
}

func defaultTCPDial(cfg transportConfig) (packetConn, *net.UDPAddr, error) {
	if cfg.ip == "" {
		return nil, nil, fmt.Errorf("KNXnet/IP: gateway IP not configured")
	}
	dialer := net.Dialer{Timeout: cfg.timeout}
	if cfg.localIP != "" {
		localAddr, err := net.ResolveTCPAddr("tcp4", cfg.localIP+":0")
		if err != nil {
			return nil, nil, err
		}
		dialer.LocalAddr = localAddr
	}
	conn, err := dialer.Dial("tcp4", cfg.remoteAddr())
	if err != nil {
		return nil, nil, err
	}
	return &tcpPacketConn{conn: conn}, nil, nil
}

func (t *KNXTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected.Load() {
		return nil
	}

	if t.connMgr != nil {
		t.connMgr.SetState(driver.StateConnecting)
	}

	var lastErr error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			canRetry, wait := t.connMgr.CanRetry()
			if !canRetry {
				return fmt.Errorf("KNXnet/IP: connection retry limit exceeded: %w", lastErr)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		dialFn := t.dialFn
		if t.cfg.isTCP() {
			dialFn = defaultTCPDial
		}

		conn, remote, err := dialFn(t.cfg)
		if err != nil {
			lastErr = err
			if t.connMgr != nil {
				shouldRetry, _ := t.connMgr.RecordFailure()
				if !shouldRetry {
					return fmt.Errorf("KNXnet/IP dial failed: %w", lastErr)
				}
			} else if attempt >= t.cfg.maxRetries {
				return fmt.Errorf("KNXnet/IP dial failed after %d attempts: %w", attempt+1, lastErr)
			}
			continue
		}

		t.conn = conn
		t.remoteUDPAddr = remote
		if conn != nil && conn.LocalAddr() != nil {
			t.localAddr = conn.LocalAddr().String()
		}
		if remote != nil {
			t.remoteAddr = remote.String()
		} else if t.cfg.ip != "" {
			t.remoteAddr = t.cfg.remoteAddr()
		}
		if tc, ok := conn.(*tcpPacketConn); ok {
			t.tcp = tc.conn
		}

		if err := t.performConnect(ctx); err != nil {
			_ = conn.Close()
			t.conn = nil
			t.tcp = nil
			lastErr = err
			if t.connMgr != nil {
				shouldRetry, _ := t.connMgr.RecordFailure()
				if !shouldRetry {
					return fmt.Errorf("KNXnet/IP connect handshake failed: %w", lastErr)
				}
			} else if attempt >= t.cfg.maxRetries {
				return fmt.Errorf("KNXnet/IP connect handshake failed: %w", lastErr)
			}
			continue
		}

		t.connected.Store(true)
		t.connectTime = time.Now()
		t.reconnectCount.Add(1)
		if t.connMgr != nil {
			t.connMgr.RecordSuccess()
		}
		t.startHeartbeat()

		zap.L().Info("[KNXnet/IP] connected",
			zap.String("remote", t.remoteAddr),
			zap.String("mode", t.cfg.mode),
			zap.Uint32("channel", t.channelID.Load()),
		)
		return nil
	}
}

func (t *KNXTransport) performConnect(ctx context.Context) error {
	control := t.localHPAI()
	data := t.localHPAI()
	req := buildConnectRequest(control, data)
	respFrame, err := t.transact(ctx, req)
	if err != nil {
		return err
	}
	svc, body, err := parseHeader(respFrame)
	if err != nil {
		return err
	}
	if svc != svcConnectResponse {
		return fmt.Errorf("unexpected service 0x%04X during connect", svc)
	}
	cr, err := parseConnectResponse(body)
	if err != nil {
		return err
	}
	if cr.Status != 0 {
		return fmt.Errorf("connect rejected status 0x%02X", cr.Status)
	}
	t.channelID.Store(uint32(cr.ChannelID))
	t.srcAddr = cr.KNXAddr
	if t.srcAddr == 0 {
		t.srcAddr = 0x0001
	}
	return nil
}

func (t *KNXTransport) localHPAI() hpai {
	h := hpai{hostProtocol: hostProtocolIPv4UDP}
	if t.cfg.isTCP() {
		h.hostProtocol = hostProtocolIPv4TCP
	}
	if t.conn != nil && t.conn.LocalAddr() != nil {
		host, portStr, err := net.SplitHostPort(t.conn.LocalAddr().String())
		if err == nil {
			if ip := net.ParseIP(host); ip != nil {
				copy(h.ip[:], ip.To4())
			}
			var port int
			fmt.Sscanf(portStr, "%d", &port)
			h.port = uint16(port)
		}
	}
	return h
}

func (t *KNXTransport) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.stopHeartbeat()

	if t.connected.Load() && t.channelID.Load() > 0 {
		_ = t.sendRaw(buildDisconnectRequest(byte(t.channelID.Load())))
	}

	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}
	t.tcp = nil
	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()
	if t.connMgr != nil {
		t.connMgr.SetState(driver.StateDisconnected)
	}
	return nil
}

func (t *KNXTransport) IsConnected() bool {
	return t.connected.Load()
}

func (t *KNXTransport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	reconnectCount = int64(t.reconnectCount.Load())
	lastDisconnectTime = t.lastDisconnectTime
	remoteAddr = t.remoteAddr
	if remoteAddr == "" {
		remoteAddr = t.cfg.remoteAddr()
	}
	if !t.connected.Load() {
		return 0, reconnectCount, t.localAddr, remoteAddr, lastDisconnectTime
	}
	return int64(time.Since(t.connectTime).Seconds()), reconnectCount, t.localAddr, remoteAddr, lastDisconnectTime
}

func (t *KNXTransport) GetCached(group uint16) ([]byte, bool) {
	t.cacheMu.RLock()
	defer t.cacheMu.RUnlock()
	data, ok := t.cache[group]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

func (t *KNXTransport) ReadGroup(ctx context.Context, group uint16) ([]byte, error) {
	cemi := buildGroupValueReadCEMI(group, t.srcAddr)
	data, err := t.requestGroupValue(ctx, group, cemi)
	if err == nil {
		return data, nil
	}
	if cached, ok := t.GetCached(group); ok {
		return cached, nil
	}
	return nil, err
}

func (t *KNXTransport) WriteGroup(ctx context.Context, group uint16, payload []byte) error {
	cemi := buildGroupValueWriteCEMI(group, t.srcAddr, payload)
	seq := byte(t.nextSeq())
	req := buildTunnelingRequest(byte(t.channelID.Load()), seq, cemi)
	respFrame, err := t.transact(ctx, req)
	if err != nil {
		return err
	}
	svc, body, err := parseHeader(respFrame)
	if err != nil {
		return err
	}
	switch svc {
	case svcTunnelingConfirm:
		_, _, status, _, err := parseTunnelingBody(body)
		if err != nil {
			return err
		}
		if status != 0 {
			return fmt.Errorf("tunnel write rejected status 0x%02X", status)
		}
		t.storeCache(group, payload)
		return nil
	case svcTunnelingIndication:
		ch, indSeq, _, cemiResp, err := parseTunnelingBody(body)
		if err != nil {
			return err
		}
		_ = t.sendRaw(buildTunnelingConfirm(ch, indSeq, 0))
		parsed, err := parseCEMI(cemiResp)
		if err != nil {
			return err
		}
		t.storeCache(parsed.Destination, parsed.Data)
		return nil
	default:
		return fmt.Errorf("unexpected service 0x%04X during write", svc)
	}
}

func (t *KNXTransport) requestGroupValue(ctx context.Context, group uint16, cemi []byte) ([]byte, error) {
	seq := byte(t.nextSeq())
	req := buildTunnelingRequest(byte(t.channelID.Load()), seq, cemi)

	deadline := time.Now().Add(t.cfg.timeout)
	for {
		if err := t.sendRaw(req); err != nil {
			return nil, err
		}

		respFrame, err := t.readFrameUntil(ctx, deadline)
		if err != nil {
			if cached, ok := t.GetCached(group); ok {
				return cached, nil
			}
			return nil, err
		}

		svc, body, err := parseHeader(respFrame)
		if err != nil {
			return nil, err
		}

		switch svc {
		case svcTunnelingConfirm:
			continue
		case svcTunnelingIndication:
			ch, indSeq, _, cemiResp, err := parseTunnelingBody(body)
			if err != nil {
				return nil, err
			}
			_ = t.sendRaw(buildTunnelingConfirm(ch, indSeq, 0))
			parsed, err := parseCEMI(cemiResp)
			if err != nil {
				return nil, err
			}
			if parsed.Destination == group && isGroupValueResponse(parsed) {
				t.storeCache(group, parsed.Data)
				return parsed.Data, nil
			}
			t.storeCache(parsed.Destination, parsed.Data)
			continue
		default:
			return nil, fmt.Errorf("unexpected service 0x%04X during read", svc)
		}
	}
}

func (t *KNXTransport) storeCache(group uint16, data []byte) {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	t.cache[group] = append([]byte(nil), data...)
}

func (t *KNXTransport) nextSeq() byte {
	return byte(t.seq.Add(1) & 0xFF)
}

func (t *KNXTransport) transact(ctx context.Context, req []byte) ([]byte, error) {
	if err := t.sendRaw(req); err != nil {
		return nil, err
	}
	return t.readFrameUntil(ctx, time.Now().Add(t.cfg.timeout))
}

func (t *KNXTransport) sendRaw(payload []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.conn == nil {
		return fmt.Errorf("KNXnet/IP not connected")
	}
	if t.remoteUDPAddr != nil {
		_, err := t.conn.WriteTo(payload, t.remoteUDPAddr)
		return err
	}
	_, err := t.conn.WriteTo(payload, nil)
	return err
}

func (t *KNXTransport) readFrameUntil(ctx context.Context, deadline time.Time) ([]byte, error) {
	buf := make([]byte, 2048)
	var acc []byte

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("KNXnet/IP read timeout")
		}

		if len(acc) >= headerLen {
			total := int(binary.BigEndian.Uint16(acc[4:6]))
			if total <= len(acc) {
				return append([]byte(nil), acc[:total]...), nil
			}
		}

		_ = t.setReadDeadline(time.Now().Add(remaining))
		n, _, err := t.conn.ReadFrom(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				return nil, fmt.Errorf("KNXnet/IP read timeout")
			}
			return nil, err
		}
		acc = append(acc, buf[:n]...)
	}
}

func (t *KNXTransport) setReadDeadline(tm time.Time) error {
	if t.conn == nil {
		return nil
	}
	if s, ok := t.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		return s.SetReadDeadline(tm)
	}
	return nil
}

func (t *KNXTransport) startHeartbeat() {
	if t.connMgr == nil || t.cfg.heartbeatInterval <= 0 {
		return
	}
	interval := t.cfg.heartbeatInterval
	t.connMgr.StartBackgroundLoop(func(ctx context.Context) {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				t.sendConnectionState(ctx)
			}
		}
	})
}

func (t *KNXTransport) stopHeartbeat() {
	if t.connMgr != nil {
		t.connMgr.StopBackgroundLoop()
	}
}

func (t *KNXTransport) sendConnectionState(ctx context.Context) {
	if !t.connected.Load() {
		return
	}
	req := buildConnectionStateRequest(byte(t.channelID.Load()))
	respFrame, err := t.transact(ctx, req)
	if err != nil {
		zap.L().Warn("[KNXnet/IP] connection state heartbeat failed", zap.Error(err))
		return
	}
	svc, body, err := parseHeader(respFrame)
	if err != nil || svc != svcConnectionStateResp {
		zap.L().Warn("[KNXnet/IP] unexpected connection state response", zap.Error(err))
		return
	}
	_, status, err := parseConnectionStateResponse(body)
	if err != nil || status != 0 {
		zap.L().Warn("[KNXnet/IP] connection state rejected", zap.Uint8("status", status), zap.Error(err))
	}
}

type tcpPacketConn struct {
	conn net.Conn
}

func (t *tcpPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = t.conn.Read(p)
	return n, t.conn.RemoteAddr(), err
}

func (t *tcpPacketConn) WriteTo(p []byte, _ net.Addr) (n int, err error) {
	return t.conn.Write(p)
}

func (t *tcpPacketConn) Close() error {
	return t.conn.Close()
}

func (t *tcpPacketConn) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

func (t *tcpPacketConn) SetReadDeadline(tm time.Time) error {
	return t.conn.SetReadDeadline(tm)
}

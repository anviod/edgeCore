package dlt645

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/goburrow/serial"
	"go.uber.org/zap"
)

type frameLink interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}

// DLT645Transport manages serial (RTU) or TCP connections and frame I/O.
type DLT645Transport struct {
	cfg transportConfig

	link frameLink

	linkFactory func(cfg transportConfig) (frameLink, error)

	connected          atomic.Bool
	mu                 sync.Mutex
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string
	lastSendTime       time.Time

	collectFailCount atomic.Int32
	maxFailCount     int32
	connMgr          *driver.ConnectionManager
}

func NewDLT645Transport(cfg map[string]any) *DLT645Transport {
	tc := parseTransportConfig(cfg)
	t := &DLT645Transport{
		cfg:          tc,
		maxFailCount: tc.maxFailCount,
		remoteAddr:   tc.remoteAddr(),
	}
	t.linkFactory = defaultLinkFactory
	t.connMgr = driver.NewConnectionManager("dlt645")
	t.connMgr.SetMaxRetries(tc.maxRetries)
	return t
}

func defaultLinkFactory(cfg transportConfig) (frameLink, error) {
	switch cfg.mode {
	case connTCP:
		if cfg.ip == "" {
			return nil, fmt.Errorf("DLT645 TCP: IP address not configured")
		}
		addr := net.JoinHostPort(cfg.ip, fmt.Sprintf("%d", cfg.port))
		conn, err := net.DialTimeout("tcp", addr, cfg.timeout)
		if err != nil {
			return nil, err
		}
		if err := conn.SetDeadline(time.Time{}); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	default:
		if cfg.serialPort == "" {
			return nil, fmt.Errorf("DLT645 serial: port not configured")
		}
		port, err := serial.Open(&serial.Config{
			Address:  cfg.serialPort,
			BaudRate: cfg.baudRate,
			DataBits: cfg.dataBits,
			Parity:   cfg.parity,
			StopBits: cfg.stopBits,
			Timeout:  10 * time.Millisecond,
		})
		if err != nil {
			return nil, err
		}
		return port, nil
	}
}

func (t *DLT645Transport) Connect(ctx context.Context) error {
	if t.connected.Load() {
		return nil
	}
	if t.cfg.mode == connTCP && t.cfg.ip == "" {
		return fmt.Errorf("DLT645 TCP: IP address not configured")
	}

	if t.connMgr != nil {
		return t.connMgr.EnsureConnected(ctx, func(ctx context.Context) error {
			return t.connectOnce(ctx)
		})
	}

	var lastErr error
	for attempt := 0; attempt <= t.cfg.maxRetries; attempt++ {
		if err := t.connectOnce(ctx); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("DLT645 transport: connection failed after %d attempts: %w", t.cfg.maxRetries+1, lastErr)
}

func (t *DLT645Transport) connectOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if t.connected.Load() {
		return nil
	}

	link, err := t.linkFactory(t.cfg)
	if err != nil {
		return err
	}

	t.mu.Lock()
	if t.connected.Load() {
		t.mu.Unlock()
		_ = link.Close()
		return nil
	}
	if t.link != nil {
		_ = t.link.Close()
	}
	t.link = link
	t.connected.Store(true)
	t.connectTime = time.Now()
	t.reconnectCount.Add(1)
	t.collectFailCount.Store(0)
	t.remoteAddr = t.cfg.remoteAddr()
	t.localAddr = t.resolveLocalAddr()
	t.mu.Unlock()

	zap.L().Info("[DLT645] Connection established",
		zap.String("remote", t.remoteAddr),
		zap.String("mode", t.modeLabel()),
		zap.Duration("timeout", t.cfg.timeout),
	)
	return nil
}

func (t *DLT645Transport) scheduleReconnect() {
	if t.connMgr == nil {
		return
	}
	timeout := t.cfg.timeout * time.Duration(t.cfg.maxRetries+1)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	t.connMgr.ScheduleReconnect(context.Background(), timeout, func(ctx context.Context) error {
		return t.connectOnce(ctx)
	})
}

func (t *DLT645Transport) modeLabel() string {
	if t.cfg.mode == connTCP {
		return "tcp"
	}
	return "serial"
}

func (t *DLT645Transport) resolveLocalAddr() string {
	if t.cfg.mode == connSerial {
		return t.cfg.serialPort
	}
	if conn, ok := t.link.(net.Conn); ok && conn.LocalAddr() != nil {
		return conn.LocalAddr().String()
	}
	return ""
}

func (t *DLT645Transport) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	wasConnected := t.connected.Load()
	if t.link != nil {
		_ = t.link.Close()
		t.link = nil
	}
	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()
	if t.connMgr != nil {
		t.connMgr.SetState(driver.StateDisconnected)
	}
	if wasConnected {
		zap.L().Info("[DLT645] Disconnected")
	}
	return nil
}

func (t *DLT645Transport) IsConnected() bool {
	return t.connected.Load()
}

func (t *DLT645Transport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
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

func (t *DLT645Transport) RecordSuccess() {
	t.collectFailCount.Store(0)
	if t.connMgr != nil {
		t.connMgr.RecordSuccess()
	}
}

func (t *DLT645Transport) RecordFailure(err error) {
	failCount := t.collectFailCount.Add(1)
	zap.L().Warn("[DLT645] Request failed",
		zap.Error(err),
		zap.Int32("failCount", failCount),
		zap.Int32("maxFailCount", t.maxFailCount),
	)
	if t.connMgr != nil {
		t.connMgr.RecordFailure()
	}
	if failCount >= t.maxFailCount {
		t.Disconnect()
	}
}

func (t *DLT645Transport) waitSendInterval() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cfg.sendInterval <= 0 {
		return
	}
	if !t.lastSendTime.IsZero() {
		elapsed := time.Since(t.lastSendTime)
		if elapsed < t.cfg.sendInterval {
			time.Sleep(t.cfg.sendInterval - elapsed)
		}
	}
	t.lastSendTime = time.Now()
}

// ReadData sends a read request and returns the response value bytes (without DI prefix).
func (t *DLT645Transport) ReadData(ctx context.Context, meterAddr [AddrLen]byte, dataID [DataIDLen]byte) ([]byte, error) {
	frame := BuildReadFrame(meterAddr, dataID)
	resp, err := t.transact(ctx, frame)
	if err != nil {
		return nil, err
	}
	parsed, err := DecodeFrame(resp)
	if err != nil {
		return nil, err
	}
	_, value, err := ParseReadResponse(parsed)
	return value, err
}

// WriteData sends a write request with encoded payload (password + data per meter spec).
func (t *DLT645Transport) WriteData(ctx context.Context, meterAddr [AddrLen]byte, dataID [DataIDLen]byte, payload []byte) error {
	frame := BuildWriteFrame(meterAddr, dataID, payload)
	resp, err := t.transact(ctx, frame)
	if err != nil {
		return err
	}
	parsed, err := DecodeFrame(resp)
	if err != nil {
		return err
	}
	if parsed.IsError() {
		return fmt.Errorf("write rejected: error code 0x%02X", parsed.ErrorCode())
	}
	if parsed.Control != CtrlWriteResp {
		return fmt.Errorf("unexpected write response control: 0x%02X", parsed.Control)
	}
	return nil
}

func (t *DLT645Transport) transact(ctx context.Context, request []byte) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= t.cfg.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}

		if !t.connected.Load() {
			t.scheduleReconnect()
			return nil, fmt.Errorf("DLT645: not connected")
		}

		t.mu.Lock()
		link := t.link
		t.mu.Unlock()
		if link == nil {
			lastErr = fmt.Errorf("DLT645 link not available")
			t.scheduleReconnect()
			break
		}

		t.waitSendInterval()

		deadline := time.Now().Add(t.cfg.timeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}

		writeBuf := request
		if t.cfg.mode == connSerial && t.cfg.preambleBytes > 0 {
			preamble := make([]byte, t.cfg.preambleBytes)
			for i := range preamble {
				preamble[i] = PreambleByte
			}
			writeBuf = append(preamble, request...)
		}

		setWriteDeadline(link, deadline)
		if _, err := link.Write(writeBuf); err != nil {
			lastErr = err
			t.RecordFailure(err)
			continue
		}

		resp, err := readFrame(link, deadline)
		if err != nil {
			lastErr = err
			t.RecordFailure(err)
			continue
		}

		t.RecordSuccess()
		return resp, nil
	}
	return nil, lastErr
}

type deadlineSetter interface {
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

func setWriteDeadline(link frameLink, deadline time.Time) {
	if ds, ok := link.(deadlineSetter); ok {
		_ = ds.SetWriteDeadline(deadline)
	}
}

func readFrame(r io.Reader, deadline time.Time) ([]byte, error) {
	if ds, ok := r.(deadlineSetter); ok {
		_ = ds.SetReadDeadline(deadline)
	}

	buf := make([]byte, 0, 256)
	chunk := make([]byte, 128)
	startFound := false

	for {
		n, err := r.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			if !startFound {
				for i, b := range buf {
					if b == FrameStart {
						buf = buf[i:]
						startFound = true
						break
					}
				}
				if !startFound {
					trim := 0
					for i := len(buf) - 1; i >= 0; i-- {
						if buf[i] == PreambleByte {
							trim = i
						} else {
							break
						}
					}
					if trim > 0 {
						buf = buf[trim:]
					} else if len(buf) > 8 {
						buf = buf[len(buf)-8:]
					}
				}
			}
			if startFound && len(buf) >= 10 {
				dataLen := int(buf[9])
				total := 12 + dataLen
				if len(buf) >= total {
					frame := buf[:total]
					if frame[total-1] != FrameEnd {
						return nil, fmt.Errorf("invalid frame end")
					}
					return frame, nil
				}
			}
		}
		if err != nil {
			if len(buf) >= 12 && startFound {
				return nil, fmt.Errorf("incomplete frame: %w", err)
			}
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("read timeout")
		}
	}
}

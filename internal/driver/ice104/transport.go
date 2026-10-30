package ice104

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type ICE104Transport struct {
	cfg deviceConfig

	conn               net.Conn
	connected          atomic.Bool
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string

	sendSeq atomic.Uint32
	recvSeq atomic.Uint32

	cacheMu sync.RWMutex
	cache   map[string]cachedPoint

	readCancel context.CancelFunc
	readWG     sync.WaitGroup

	writeMu sync.Mutex
}

func NewICE104Transport(cfg map[string]any) *ICE104Transport {
	return &ICE104Transport{
		cfg:   parseDeviceConfig(cfg),
		cache: make(map[string]cachedPoint),
	}
}

func (t *ICE104Transport) Connect(ctx context.Context) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if t.connected.Load() {
		return nil
	}

	dialer := net.Dialer{Timeout: t.cfg.T0}
	conn, err := dialer.DialContext(ctx, "tcp", t.cfg.remoteAddr())
	if err != nil {
		return fmt.Errorf("ice104 dial %s: %w", t.cfg.remoteAddr(), err)
	}

	t.conn = conn
	t.localAddr = conn.LocalAddr().String()
	t.remoteAddr = conn.RemoteAddr().String()
	t.connectTime = time.Now()
	t.reconnectCount.Add(1)
	t.sendSeq.Store(0)
	t.recvSeq.Store(0)

	if err := t.sendUFrame(uTestFRAct); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ice104 TESTFR: %w", err)
	}
	if err := t.readUFrame(ctx, uTestFRCon); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ice104 TESTFR confirm: %w", err)
	}
	if err := t.sendUFrame(uStartDTAct); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ice104 STARTDT: %w", err)
	}
	if err := t.readUFrame(ctx, uStartDTCon); err != nil {
		_ = conn.Close()
		return fmt.Errorf("ice104 STARTDT confirm: %w", err)
	}

	readCtx, cancel := context.WithCancel(context.Background())
	t.readCancel = cancel
	t.readWG.Add(1)
	go t.readLoop(readCtx)

	t.connected.Store(true)
	zap.L().Info("[ICE104] connected", zap.String("remote", t.remoteAddr))
	return nil
}

func (t *ICE104Transport) Disconnect() error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if t.readCancel != nil {
		t.readCancel()
		t.readWG.Wait()
		t.readCancel = nil
	}

	if t.conn != nil {
		_ = t.sendUFrame(uStopDTAct)
		_ = t.conn.Close()
		t.conn = nil
	}
	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()
	t.cacheMu.Lock()
	t.cache = make(map[string]cachedPoint)
	t.cacheMu.Unlock()
	return nil
}

func (t *ICE104Transport) IsConnected() bool {
	return t.connected.Load()
}

func (t *ICE104Transport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if !t.connectTime.IsZero() && t.connected.Load() {
		connectionSeconds = int64(time.Since(t.connectTime).Seconds())
	}
	return connectionSeconds, int64(t.reconnectCount.Load()), t.localAddr, t.remoteAddr, t.lastDisconnectTime
}

func (t *ICE104Transport) GetCached(key string) (cachedPoint, bool) {
	t.cacheMu.RLock()
	defer t.cacheMu.RUnlock()
	cp, ok := t.cache[key]
	return cp, ok
}

func (t *ICE104Transport) SendGeneralCall(ctx context.Context) error {
	asdu := buildASDU(typeC_IC_NA_1, 1, cotActivation, t.cfg.CommonAddress, encodeGeneralInterrogation(0))
	return t.sendIFrame(ctx, asdu)
}

func (t *ICE104Transport) SendSingleCommand(ctx context.Context, ioa uint32, execute bool) error {
	asdu := buildASDU(typeC_SC_NA_1, 1, cotActivation, t.cfg.CommonAddress, encodeSingleCommand(ioa, execute))
	return t.sendIFrame(ctx, asdu)
}

func (t *ICE104Transport) sendUFrame(ctrl byte) error {
	if t.conn == nil {
		return fmt.Errorf("not connected")
	}
	frame := []byte{startByte, 0x04, ctrl, 0x00, 0x00, 0x00}
	_, err := t.conn.Write(frame)
	return err
}

func (t *ICE104Transport) readUFrame(ctx context.Context, expected byte) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(t.cfg.T1)
	}
	_ = t.conn.SetReadDeadline(deadline)

	buf := make([]byte, 6)
	if _, err := io.ReadFull(t.conn, buf); err != nil {
		return err
	}
	if buf[0] != startByte || buf[1] != 0x04 {
		return fmt.Errorf("invalid U-frame header")
	}
	if buf[2] != expected {
		return fmt.Errorf("unexpected U-frame 0x%02x", buf[2])
	}
	return nil
}

func (t *ICE104Transport) sendIFrame(ctx context.Context, asdu []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if t.conn == nil {
		return fmt.Errorf("not connected")
	}

	send := t.sendSeq.Load()
	t.sendSeq.Add(1)
	recv := t.recvSeq.Load()
	ctrl := make([]byte, 4)
	binary.LittleEndian.PutUint16(ctrl[0:2], uint16(send<<1))
	binary.LittleEndian.PutUint16(ctrl[2:4], uint16(recv<<1))

	apduLen := 4 + len(asdu)
	// IEC 60870-5-104 长度域为单字节（标准上限 253），超长帧无法编码
	if apduLen > 255 {
		return fmt.Errorf("APDU too large: %d bytes (max 255)", apduLen)
	}
	frame := make([]byte, 0, apduLen+2)
	frame = append(frame, startByte, byte(apduLen))
	frame = append(frame, ctrl...)
	frame = append(frame, asdu...)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(t.cfg.T1)
	}
	_ = t.conn.SetWriteDeadline(deadline)
	_, err := t.conn.Write(frame)
	return err
}

func (t *ICE104Transport) readLoop(ctx context.Context) {
	defer t.readWG.Done()
	decoder := NewICE104Decoder()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if t.conn == nil {
			return
		}

		_ = t.conn.SetReadDeadline(time.Now().Add(t.cfg.T3))
		header := make([]byte, 2)
		if _, err := io.ReadFull(t.conn, header); err != nil {
			if ctx.Err() != nil {
				return
			}
			t.connected.Store(false)
			return
		}
		if header[0] != startByte {
			continue
		}
		length := int(header[1])
		if length < 4 {
			continue
		}
		body := make([]byte, length)
		if _, err := io.ReadFull(t.conn, body); err != nil {
			if ctx.Err() != nil {
				return
			}
			t.connected.Store(false)
			return
		}

		if err := t.handleAPDU(decoder, body); err != nil {
			zap.L().Debug("[ICE104] handle APDU", zap.Error(err))
		}
	}
}

func (t *ICE104Transport) handleAPDU(decoder *ICE104Decoder, apdu []byte) error {
	if len(apdu) < 4 {
		return fmt.Errorf("short APDU")
	}

	ctrl0 := apdu[0]
	switch ctrl0 & 0x03 {
	case 0x03:
		// U-frame
		return nil
	case 0x01:
		// S-frame
		recv := binary.LittleEndian.Uint16(apdu[2:4]) >> 1
		t.recvSeq.Store(uint32(recv))
		return nil
	}

	// I-frame (ctrl0&0x01 == 0)
	send := binary.LittleEndian.Uint16(apdu[0:2]) >> 1
	recv := binary.LittleEndian.Uint16(apdu[2:4]) >> 1
	t.recvSeq.Store(uint32(send))
	t.sendSeq.Store(uint32(recv))

	asdu := apdu[4:]
	if len(asdu) < 6 {
		return nil
	}

	typeID := asdu[0]
	vsq := asdu[1]
	count := int(vsq & 0x7F)
	sq := vsq&0x80 != 0
	payload := asdu[6:]

	points, err := decoder.DecodeInformationObject(typeID, payload, sq, count)
	if err != nil {
		return err
	}
	if len(points) == 0 {
		return nil
	}

	t.cacheMu.Lock()
	for _, p := range points {
		key := decoder.PointKey(p.TypeID, p.IOA)
		t.cache[key] = p
	}
	t.cacheMu.Unlock()
	return nil
}

func buildASDU(typeID byte, count int, cot uint16, ca uint16, info []byte) []byte {
	vsq := byte(count & 0x7F)
	out := make([]byte, 0, 6+len(info))
	out = append(out, typeID, vsq)
	out = append(out, byte(cot), byte(cot>>8))
	out = append(out, byte(ca), byte(ca>>8))
	out = append(out, info...)
	return out
}

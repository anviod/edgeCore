package ethernetip

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"

	go_ethernet_ip "github.com/anviod/ethernet-ip"
	"go.uber.org/zap"
)

type ENIPClient = go_ethernet_ip.EIPTCP

const (
	StateDisconnected driver.ConnState = driver.StateDisconnected
	StateConnecting   driver.ConnState = driver.StateConnecting
	StateConnected    driver.ConnState = driver.StateConnected
	StateRetrying     driver.ConnState = driver.StateRetrying
	StateDead         driver.ConnState = driver.StateDead
)

type ENIPTransport struct {
	cfg map[string]any

	tcp *ENIPClient

	tcpFactory func(address string, cfg *go_ethernet_ip.Config) (*go_ethernet_ip.EIPTCP, error)

	ip             string
	port           int
	slot           int
	connectionType string
	timeout        time.Duration
	maxRetries     int

	connected          atomic.Bool
	mu                 sync.Mutex
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string

	connMgr *driver.ConnectionManager

	lastActivityTime atomic.Value
	collectFailCount atomic.Int32
	maxFailCount     int32
	collectCycle     time.Duration
}

func NewENIPTransport(cfg map[string]any) *ENIPTransport {
	t := &ENIPTransport{
		cfg:          cfg,
		port:         44818,
		slot:         0,
		timeout:      2 * time.Second,
		maxRetries:   64,
		maxFailCount: 5,
		collectCycle: 10 * time.Second,
	}

	t.tcpFactory = func(address string, cfg *go_ethernet_ip.Config) (*go_ethernet_ip.EIPTCP, error) {
		return go_ethernet_ip.NewTCP(address, cfg)
	}

	t.parseConfig()
	t.lastActivityTime.Store(time.Time{})

	t.connMgr = driver.NewConnectionManager("ethernetip")
	t.connMgr.SetMaxRetries(t.maxRetries)

	return t
}

func (t *ENIPTransport) parseConfig() {
	if v, ok := t.cfg["ip"].(string); ok {
		t.ip = v
	}

	if v, ok := t.cfg["port"].(float64); ok {
		t.port = int(v)
	} else if v, ok := t.cfg["port"].(int); ok {
		t.port = v
	}

	if v, ok := t.cfg["slot"].(float64); ok {
		t.slot = int(v)
	} else if v, ok := t.cfg["slot"].(int); ok {
		t.slot = v
	}

	if v, ok := t.cfg["timeout"].(float64); ok {
		t.timeout = time.Duration(v) * time.Millisecond
	} else if v, ok := t.cfg["timeout"].(int); ok {
		t.timeout = time.Duration(v) * time.Millisecond
	}

	if v, ok := t.cfg["max_retries"].(float64); ok {
		t.maxRetries = int(v)
	} else if v, ok := t.cfg["max_retries"].(int); ok {
		t.maxRetries = v
	}

	if v, ok := t.cfg["max_fail_count"].(float64); ok {
		t.maxFailCount = int32(v)
	} else if v, ok := t.cfg["max_fail_count"].(int); ok {
		t.maxFailCount = int32(v)
	}

	if v, ok := t.cfg["collect_cycle"].(float64); ok {
		t.collectCycle = time.Duration(v) * time.Millisecond
	} else if v, ok := t.cfg["collect_cycle"].(int); ok {
		t.collectCycle = time.Duration(v) * time.Millisecond
	}

	if v, ok := t.cfg["connection_type"].(string); ok {
		t.connectionType = v
	} else {
		t.connectionType = "cip"
	}
}

func (t *ENIPTransport) Connect(ctx context.Context) error {
	if t.connected.Load() {
		return nil
	}
	if t.ip == "" {
		return fmt.Errorf("ENIP transport: IP address not configured")
	}

	// Dial / backoff must not hold Transport.mu (peer I/O and Disconnect).
	return t.connMgr.EnsureConnected(ctx, func(ctx context.Context) error {
		return t.connectOnce(ctx)
	})
}

func (t *ENIPTransport) connectOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if t.ip == "" {
		return fmt.Errorf("ENIP transport: IP address not configured")
	}

	remoteAddr := fmt.Sprintf("%s:%d", t.ip, t.port)

	tcp, err := t.tcpFactory(t.ip, nil)
	if err != nil {
		zap.L().Warn("[ENIP] Failed to create ENIP client",
			zap.Error(err),
			zap.String("addr", remoteAddr),
		)
		return fmt.Errorf("failed to create ENIP client: %w", err)
	}

	if err := tcp.Connect(); err != nil {
		zap.L().Warn("[ENIP] Connection failed",
			zap.Error(err),
			zap.String("addr", remoteAddr),
		)
		tcp.Close()
		return fmt.Errorf("ENIP connection failed: %w", err)
	}

	t.mu.Lock()
	if t.connected.Load() {
		t.mu.Unlock()
		tcp.Close()
		return nil
	}
	if t.tcp != nil {
		t.tcp.Close()
	}
	t.tcp = tcp
	t.connected.Store(true)
	t.connectTime = time.Now()
	t.reconnectCount.Add(1)
	t.remoteAddr = remoteAddr
	t.localAddr = t.getLocalAddr()
	t.lastActivityTime.Store(time.Now())
	t.mu.Unlock()

	zap.L().Info("[ENIP] TCP connection established",
		zap.String("addr", remoteAddr),
		zap.Int("slot", t.slot),
		zap.Duration("timeout", t.timeout),
	)
	return nil
}

func (t *ENIPTransport) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected.Load() {
		return nil
	}

	if t.tcp != nil {
		t.tcp.Close()
	}

	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()
	t.connMgr.SetState(StateDisconnected)
	t.connMgr.Close()

	zap.L().Info("[ENIP] Disconnected")
	return nil
}

func (t *ENIPTransport) IsConnected() bool {
	return t.connected.Load()
}

func (t *ENIPTransport) GetClient() *ENIPClient {
	if !t.connected.Load() {
		return nil
	}
	return t.tcp
}

func (t *ENIPTransport) RecordSuccess() {
	t.connMgr.RecordSuccess()
	t.collectFailCount.Store(0)
	t.lastActivityTime.Store(time.Now())
}

func (t *ENIPTransport) RecordFailure(err error) {
	t.collectFailCount.Add(1)
	t.lastActivityTime.Store(time.Now())

	if t.collectFailCount.Load() >= t.maxFailCount {
		t.scheduleReconnect()
	}
}

func (t *ENIPTransport) scheduleReconnect() {
	timeout := t.timeout * time.Duration(t.maxRetries)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	t.connMgr.ScheduleReconnect(context.Background(), timeout, func(ctx context.Context) error {
		t.mu.Lock()
		defer t.mu.Unlock()

		if t.connected.Load() && t.tcp != nil {
			t.tcp.Close()
			t.tcp = nil
		}
		t.connected.Store(false)

		return t.connectOnce(ctx)
	})
}

func (t *ENIPTransport) NeedProbeCheck() bool {
	lastSuccess, _ := t.lastActivityTime.Load().(time.Time)
	if lastSuccess.IsZero() {
		return false
	}
	return time.Since(lastSuccess) > t.collectCycle*3
}

func (t *ENIPTransport) ProbeConnection() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.connected.Load() || t.tcp == nil {
		return
	}

	_, err := t.tcp.ListIdentity()
	if err != nil {
		t.RecordFailure(err)
	} else {
		t.RecordSuccess()
	}
}

func (t *ENIPTransport) GetConnectionMetrics() (
	connectionSeconds int64,
	reconnectCount int64,
	localAddr string,
	remoteAddr string,
	lastDisconnectTime time.Time,
) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connectTime.IsZero() {
		remoteAddr := t.remoteAddr
		if remoteAddr == "" && t.ip != "" {
			remoteAddr = fmt.Sprintf("%s:%d", t.ip, t.port)
		}
		return 0, 0, "", remoteAddr, time.Time{}
	}

	connectionSeconds = int64(time.Since(t.connectTime).Seconds())
	reconnectCount = int64(t.reconnectCount.Load())
	localAddr = t.localAddr
	remoteAddr = t.remoteAddr
	lastDisconnectTime = t.lastDisconnectTime

	return
}

// GetRemoteAddr 返回远程地址
func (t *ENIPTransport) GetRemoteAddr() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.remoteAddr
}

func (t *ENIPTransport) getLocalAddr() string {
	if t.tcp == nil {
		return ""
	}

	// Try to get local address through reflection
	// The underlying TCP connection may be stored in an unexported field
	val := reflect.ValueOf(t.tcp).Elem()

	// Look for common connection field names
	fieldNames := []string{"conn", "connection", "tcp", "client", "netConn"}
	for _, name := range fieldNames {
		field := val.FieldByName(name)
		if field.IsValid() && !field.IsNil() {
			// Check if it implements net.Conn
			connIface := field.Interface()
			if conn, ok := connIface.(net.Conn); ok {
				return conn.LocalAddr().String()
			}
			// Try to find net.Conn in nested structure
			nestedVal := reflect.ValueOf(connIface)
			if nestedVal.Kind() == reflect.Ptr {
				nestedVal = nestedVal.Elem()
			}
			if nestedVal.Kind() == reflect.Struct {
				for i := 0; i < nestedVal.NumField(); i++ {
					nestedField := nestedVal.Field(i)
					if nestedField.CanInterface() {
						if nestedConn, ok := nestedField.Interface().(net.Conn); ok {
							return nestedConn.LocalAddr().String()
						}
					}
				}
			}
		}
	}

	// Fallback: return a reasonable default
	return fmt.Sprintf("127.0.0.1:%d", t.port)
}

type MockENIPClient struct {
	connectFunc    func() error
	disconnectFunc func() error
	readFunc       func(tag string) ([]byte, error)
	writeFunc      func(tag string, data []byte) error
	pingFunc       func() error
}

func (m *MockENIPClient) Connect() error {
	if m.connectFunc != nil {
		return m.connectFunc()
	}
	return nil
}

func (m *MockENIPClient) Disconnect() error {
	if m.disconnectFunc != nil {
		return m.disconnectFunc()
	}
	return nil
}

func (m *MockENIPClient) Ping() error {
	if m.pingFunc != nil {
		return m.pingFunc()
	}
	return nil
}

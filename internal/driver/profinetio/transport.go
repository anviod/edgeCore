package profinetio

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"go.uber.org/zap"
)

type ioReader interface {
	ReadIO(slot, subslot, index, length int) ([]byte, error)
	WriteIO(slot, subslot, index int, data []byte) error
	close() error
}

// ProfinetTransport manages TCP connection to IO devices.
type ProfinetTransport struct {
	channelCfg channelConfig
	deviceCfg  deviceConfig

	dialFn func(ctx context.Context, localIF, remote string, timeout time.Duration) (net.Conn, error)

	conn ioReader
	sim  *simulationStore

	connected          atomic.Bool
	mu                 sync.Mutex
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string

	connMgr *driver.ConnectionManager
}

func NewProfinetTransport(channelCfg channelConfig) *ProfinetTransport {
	t := &ProfinetTransport{
		channelCfg: channelCfg,
		dialFn:     defaultDial,
	}
	if channelCfg.simulation {
		t.sim = newSimulationStore()
	}
	t.connMgr = driver.NewConnectionManager("profinet-io")
	t.connMgr.SetMaxRetries(channelCfg.maxRetries)
	return t
}

func defaultDial(ctx context.Context, localIF, remote string, timeout time.Duration) (net.Conn, error) {
	if remote == "" {
		return nil, fmt.Errorf("profinet-io: device IP not configured")
	}
	dialer := net.Dialer{Timeout: timeout}
	if localIF != "" {
		iface, err := net.InterfaceByName(localIF)
		if err != nil {
			return nil, fmt.Errorf("profinet-io: local interface %q: %w", localIF, err)
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("profinet-io: interface addresses: %w", err)
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.To4() == nil || ip.IsLoopback() {
				continue
			}
			dialer.LocalAddr = &net.TCPAddr{IP: ip}
			break
		}
	}
	return dialer.DialContext(ctx, "tcp4", remote)
}

func (t *ProfinetTransport) SetDeviceConfig(cfg deviceConfig) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.deviceCfg = cfg
	if cfg.remoteAddr() != "" {
		t.remoteAddr = cfg.remoteAddr()
	}
}

func (t *ProfinetTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.connected.Load() {
		return nil
	}
	if t.channelCfg.simulation {
		t.connected.Store(true)
		t.connectTime = time.Now()
		t.reconnectCount.Add(1)
		t.remoteAddr = "simulation"
		zap.L().Info("[Profinet IO] simulation mode enabled")
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
				return fmt.Errorf("profinet-io: connection retry limit exceeded: %w", lastErr)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		remote := t.deviceCfg.remoteAddr()
		if remote == "" {
			// Channel-level connect without device IP: defer until SetDeviceConfig.
			t.connected.Store(true)
			t.connectTime = time.Now()
			return nil
		}

		conn, err := t.dialFn(ctx, t.channelCfg.localInterface, remote, t.channelCfg.timeout)
		if err != nil {
			lastErr = err
			if t.connMgr != nil {
				shouldRetry, _ := t.connMgr.RecordFailure()
				if !shouldRetry {
					return fmt.Errorf("profinet-io dial failed: %w", lastErr)
				}
			} else if attempt >= t.channelCfg.maxRetries {
				return fmt.Errorf("profinet-io dial failed after %d attempts: %w", attempt+1, lastErr)
			}
			continue
		}

		if conn.LocalAddr() != nil {
			t.localAddr = conn.LocalAddr().String()
		}
		t.remoteAddr = remote
		t.conn = newRPCClient(conn, t.channelCfg.timeout)
		t.connected.Store(true)
		t.connectTime = time.Now()
		t.reconnectCount.Add(1)
		if t.connMgr != nil {
			t.connMgr.RecordSuccess()
		}

		zap.L().Info("[Profinet IO] connected",
			zap.String("remote", t.remoteAddr),
			zap.String("local", t.localAddr),
			zap.String("device", t.deviceCfg.deviceName),
		)
		return nil
	}
}

func (t *ProfinetTransport) ensureDeviceConnection(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.channelCfg.simulation {
		return nil
	}
	if t.conn != nil {
		return nil
	}
	remote := t.deviceCfg.remoteAddr()
	if remote == "" {
		return fmt.Errorf("profinet-io: device IP not configured")
	}

	conn, err := t.dialFn(ctx, t.channelCfg.localInterface, remote, t.channelCfg.timeout)
	if err != nil {
		return err
	}
	if conn.LocalAddr() != nil {
		t.localAddr = conn.LocalAddr().String()
	}
	t.remoteAddr = remote
	t.conn = newRPCClient(conn, t.channelCfg.timeout)
	return nil
}

func (t *ProfinetTransport) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn != nil {
		err := t.conn.close()
		t.conn = nil
		t.lastDisconnectTime = time.Now()
		t.connected.Store(false)
		return err
	}
	if t.connected.Load() {
		t.lastDisconnectTime = time.Now()
	}
	t.connected.Store(false)
	return nil
}

func (t *ProfinetTransport) IsConnected() bool {
	return t.connected.Load()
}

func (t *ProfinetTransport) ReadIO(ctx context.Context, slot, subslot, index, length int) ([]byte, error) {
	if !t.connected.Load() {
		return nil, fmt.Errorf("profinet-io: not connected")
	}
	if t.channelCfg.simulation && t.sim != nil {
		return t.sim.read(slot, subslot, index, length), nil
	}
	if err := t.ensureDeviceConnection(ctx); err != nil {
		return nil, err
	}
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()
	if conn == nil {
		return nil, fmt.Errorf("profinet-io: rpc client not ready")
	}
	return conn.ReadIO(slot, subslot, index, length)
}

func (t *ProfinetTransport) WriteIO(ctx context.Context, slot, subslot, index int, data []byte) error {
	if !t.connected.Load() {
		return fmt.Errorf("profinet-io: not connected")
	}
	if t.channelCfg.simulation && t.sim != nil {
		t.sim.write(slot, subslot, index, data)
		return nil
	}
	if err := t.ensureDeviceConnection(ctx); err != nil {
		return err
	}
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("profinet-io: rpc client not ready")
	}
	return conn.WriteIO(slot, subslot, index, data)
}

func (t *ProfinetTransport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if t.connectTime.IsZero() {
		return 0, int64(t.reconnectCount.Load()), t.localAddr, t.remoteAddr, t.lastDisconnectTime
	}
	return int64(time.Since(t.connectTime).Seconds()), int64(t.reconnectCount.Load()), t.localAddr, t.remoteAddr, t.lastDisconnectTime
}

func (t *ProfinetTransport) RecordSuccess() {
	if t.connMgr != nil {
		t.connMgr.RecordSuccess()
	}
}

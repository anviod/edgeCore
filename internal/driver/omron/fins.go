package omron

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	finslib "github.com/anviod/fins"

	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("omron-fins", func() driver.Driver {
		return NewOmronFinsDriver()
	})
}

type finsBackend interface {
	Init(cfg finslib.DriverConfig) error
	Connect(ctx context.Context) error
	Disconnect() error
	ReadPoints(ctx context.Context, points []finslib.Point) (map[string]finslib.Value, error)
	WritePoint(ctx context.Context, point finslib.Point, value interface{}) error
	Health() finslib.HealthStatus
	SetSlaveID(slaveID uint8) error
	SetDeviceConfig(config map[string]interface{}) error
	GetConnectionMetrics() finslib.ConnectionMetrics
	GetSchedulerStats() finslib.SchedulerStats
}

type OmronFinsDriver struct {
	config  model.DriverConfig
	backend finsBackend
}

func NewOmronFinsDriver() driver.Driver {
	return &OmronFinsDriver{}
}

func (d *OmronFinsDriver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	finsCfg := toFinsLibConfig(cfg.Config)

	backendCfg := finslib.DriverConfig{
		Protocol: "omron-fins",
		Config:   finsCfg,
	}

	var backend finsBackend
	switch transportMode(cfg.Config) {
	case "UDP":
		backend = newUDPBackend()
	default:
		backend = newTCPBackend()
	}

	if err := backend.Init(backendCfg); err != nil {
		return fmt.Errorf("omron fins init failed: %w", err)
	}

	d.backend = backend
	zap.L().Info("[FINS] Driver initialized",
		zap.String("mode", transportMode(cfg.Config)),
		zap.Any("config", finsCfg),
	)
	return nil
}

func (d *OmronFinsDriver) Connect(ctx context.Context) error {
	if d.backend == nil {
		return fmt.Errorf("omron fins driver not initialized")
	}
	if err := d.backend.Connect(ctx); err != nil {
		return fmt.Errorf("omron fins connection failed: %w", err)
	}
	return nil
}

func (d *OmronFinsDriver) Disconnect() error {
	if d.backend == nil {
		return nil
	}
	return d.backend.Disconnect()
}

func (d *OmronFinsDriver) Health() driver.HealthStatus {
	if d.backend == nil {
		return driver.HealthStatusBad
	}
	return toDriverHealth(d.backend.Health())
}

func (d *OmronFinsDriver) SetSlaveID(slaveID uint8) error {
	if d.backend == nil {
		return nil
	}
	return d.backend.SetSlaveID(slaveID)
}

func (d *OmronFinsDriver) SetDeviceConfig(config map[string]any) error {
	if d.backend == nil {
		return nil
	}
	return d.backend.SetDeviceConfig(config)
}

func (d *OmronFinsDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.backend == nil {
		if d.config.Config != nil {
			return 0, 0, "", remoteAddrFromConfig(d.config.Config), time.Time{}
		}
		return
	}
	metrics := d.backend.GetConnectionMetrics()
	if metrics.RemoteAddr == "" && d.config.Config != nil {
		metrics.RemoteAddr = remoteAddrFromConfig(d.config.Config)
	}
	return connectionMetricsTuple(metrics)
}

func (d *OmronFinsDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.backend == nil {
		return nil, fmt.Errorf("omron fins driver not initialized")
	}
	results, err := d.backend.ReadPoints(ctx, toFinsPoints(points))
	if err != nil {
		return nil, err
	}
	return fromFinsValues(results), nil
}

func (d *OmronFinsDriver) WritePoint(ctx context.Context, p model.Point, value any) error {
	if d.backend == nil {
		return fmt.Errorf("omron fins driver not initialized")
	}
	return d.backend.WritePoint(ctx, toFinsPoint(p), value)
}

func (d *OmronFinsDriver) GetMetrics() model.ChannelMetrics {
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()

	totalRequests, successCount, failureCount := int64(0), int64(0), int64(0)
	if d.backend != nil {
		stats := d.backend.GetSchedulerStats()
		totalRequests = stats.TotalRequests
		successCount = stats.SuccessCount
		failureCount = stats.FailureCount
	}

	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(successCount) / float64(totalRequests)
	}

	return model.ChannelMetrics{
		QualityScore:       d.calculateQualityScore(successRate),
		Protocol:           "Omron FINS",
		SuccessRate:        successRate,
		TimeoutCount:       failureCount,
		TotalRequests:      totalRequests,
		SuccessCount:       successCount,
		FailureCount:       failureCount,
		PacketLoss:         1.0 - successRate,
		ReconnectCount:     reconCount,
		ConnectionSeconds:  connSec,
		LocalAddr:          localAddr,
		RemoteAddr:         remoteAddr,
		LastDisconnectTime: lastDisc,
		Timestamp:          time.Now(),
	}
}

func (d *OmronFinsDriver) calculateQualityScore(successRate float64) int {
	score := 85

	if successRate < 0.5 {
		score -= 30
	} else if successRate < 0.8 {
		score -= 15
	} else if successRate < 0.95 {
		score -= 5
	}

	if d.backend != nil && d.backend.Health() != finslib.HealthStatusUp {
		score -= 40
	}

	_, reconCount, _, _, _ := d.GetConnectionMetrics()
	if reconCount > 10 {
		score -= 20
	} else if reconCount > 5 {
		score -= 10
	} else if reconCount > 0 {
		score -= 5
	}

	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}
	return score
}

// tcpBackend defers finslib TCP driver creation until Connect so channels can be added without plcIP.
type tcpBackend struct {
	cfg finslib.DriverConfig

	mu          sync.RWMutex
	inner       finsBackend
	initialized bool
}

func newTCPBackend() *tcpBackend {
	return &tcpBackend{}
}

func (b *tcpBackend) Init(cfg finslib.DriverConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized {
		return fmt.Errorf("tcp backend already initialized")
	}

	b.cfg = cfg
	b.initialized = true
	return nil
}

func (b *tcpBackend) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return fmt.Errorf("tcp backend not initialized")
	}

	if b.inner == nil {
		inner := finslib.NewFinsTCPDriver()
		if err := inner.Init(b.cfg); err != nil {
			return err
		}
		b.inner = inner
	}
	return b.inner.Connect(ctx)
}

func (b *tcpBackend) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.inner == nil {
		return nil
	}
	return b.inner.Disconnect()
}

func (b *tcpBackend) ReadPoints(ctx context.Context, points []finslib.Point) (map[string]finslib.Value, error) {
	b.mu.RLock()
	inner := b.inner
	b.mu.RUnlock()

	if inner == nil {
		return nil, fmt.Errorf("omron fins tcp not connected")
	}
	return inner.ReadPoints(ctx, points)
}

func (b *tcpBackend) WritePoint(ctx context.Context, point finslib.Point, value interface{}) error {
	b.mu.RLock()
	inner := b.inner
	b.mu.RUnlock()

	if inner == nil {
		return fmt.Errorf("omron fins tcp not connected")
	}
	return inner.WritePoint(ctx, point, value)
}

func (b *tcpBackend) Health() finslib.HealthStatus {
	b.mu.RLock()
	inner := b.inner
	b.mu.RUnlock()

	if inner == nil {
		return finslib.HealthStatusDown
	}
	return inner.Health()
}

func (b *tcpBackend) SetSlaveID(slaveID uint8) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.inner != nil {
		return b.inner.SetSlaveID(slaveID)
	}
	return nil
}

func (b *tcpBackend) SetDeviceConfig(config map[string]interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cfg.Config == nil {
		b.cfg.Config = make(map[string]interface{})
	}
	for k, v := range config {
		b.cfg.Config[k] = v
	}
	b.inner = nil
	return nil
}

func (b *tcpBackend) GetConnectionMetrics() finslib.ConnectionMetrics {
	b.mu.RLock()
	inner := b.inner
	b.mu.RUnlock()

	if inner == nil {
		return finslib.ConnectionMetrics{RemoteAddr: remoteAddrFromConfig(b.cfg.Config)}
	}
	return inner.GetConnectionMetrics()
}

func (b *tcpBackend) GetSchedulerStats() finslib.SchedulerStats {
	b.mu.RLock()
	inner := b.inner
	b.mu.RUnlock()

	if inner == nil {
		return finslib.SchedulerStats{}
	}
	return inner.GetSchedulerStats()
}

// udpBackend implements finsBackend using fins/udp client and fins decoder/scheduler logic.
type udpBackend struct {
	cfg map[string]interface{}

	client    udpClient
	decoder   *finslib.Decoder
	scheduler *udpScheduler

	mu          sync.RWMutex
	initialized bool
	slaveID     uint8
	deviceCfg   map[string]interface{}

	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	connected          atomic.Bool
	remoteAddr         string
}

type udpClient interface {
	Close()
	ReadWords(memoryArea byte, address uint16, readCount uint16) ([]uint16, error)
	ReadBytes(memoryArea byte, address uint16, readCount uint16) ([]byte, error)
	ReadBits(memoryArea byte, address uint16, bitOffset byte, readCount uint16) ([]bool, error)
	WriteWords(memoryArea byte, address uint16, data []uint16) error
	WriteBytes(memoryArea byte, address uint16, b []byte) error
	WriteBits(memoryArea byte, address uint16, bitOffset byte, data []bool) error
	SetTimeoutMs(t uint)
}

func newUDPBackend() *udpBackend {
	return &udpBackend{
		deviceCfg: make(map[string]interface{}),
	}
}

func (b *udpBackend) Init(cfg finslib.DriverConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.initialized {
		return fmt.Errorf("udp backend already initialized")
	}

	b.cfg = toFinsLibConfig(cfg.Config)
	for k, v := range b.cfg {
		b.deviceCfg[k] = v
	}

	decoder := finslib.NewDecoder()
	b.decoder = decoder
	b.scheduler = newUDPScheduler(b, decoder, b.cfg)
	b.remoteAddr = remoteAddrFromConfig(b.cfg)
	b.initialized = true
	return nil
}

func (b *udpBackend) Connect(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.initialized {
		return fmt.Errorf("udp backend not initialized")
	}

	client, err := dialUDPClient(b.cfg)
	if err != nil {
		return err
	}

	if b.client != nil {
		b.client.Close()
	}

	b.client = client
	if timeout := configInt(b.cfg, "timeout"); timeout > 0 {
		b.client.SetTimeoutMs(uint(timeout))
	}

	b.connected.Store(true)
	b.connectTime = time.Now()
	b.reconnectCount.Add(1)
	b.scheduler.setClient(b.client)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return nil
}

func (b *udpBackend) Disconnect() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client != nil {
		b.client.Close()
		b.client = nil
	}
	if b.connected.Load() {
		b.lastDisconnectTime = time.Now()
	}
	b.connected.Store(false)
	b.scheduler.setClient(nil)
	return nil
}

func (b *udpBackend) ReadPoints(ctx context.Context, points []finslib.Point) (map[string]finslib.Value, error) {
	if !b.connected.Load() || b.client == nil {
		return nil, fmt.Errorf("omron fins udp not connected")
	}
	return b.scheduler.ReadPoints(ctx, points)
}

func (b *udpBackend) WritePoint(ctx context.Context, point finslib.Point, value interface{}) error {
	if !b.connected.Load() || b.client == nil {
		return fmt.Errorf("omron fins udp not connected")
	}
	return b.scheduler.WritePoint(ctx, point, value)
}

func (b *udpBackend) Health() finslib.HealthStatus {
	if b.connected.Load() && b.client != nil {
		return finslib.HealthStatusUp
	}
	return finslib.HealthStatusDown
}

func (b *udpBackend) SetSlaveID(slaveID uint8) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.slaveID = slaveID
	return nil
}

func (b *udpBackend) SetDeviceConfig(config map[string]interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, v := range config {
		b.deviceCfg[k] = v
	}
	return nil
}

func (b *udpBackend) GetConnectionMetrics() finslib.ConnectionMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return finslib.ConnectionMetrics{
		Connected:          b.connected.Load(),
		ConnectTime:        b.connectTime,
		LastDisconnectTime: b.lastDisconnectTime,
		ReconnectCount:     b.reconnectCount.Load(),
		RemoteAddr:         b.remoteAddr,
	}
}

func (b *udpBackend) GetSchedulerStats() finslib.SchedulerStats {
	if b.scheduler == nil {
		return finslib.SchedulerStats{}
	}
	return b.scheduler.GetStats()
}

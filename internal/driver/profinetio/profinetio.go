package profinetio

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("profinet-io", func() driver.Driver {
		return NewProfinetIODriver()
	})
}

// ProfinetIODriver implements PROFINET IO acyclic read/write as IO-Controller.
type ProfinetIODriver struct {
	config     model.DriverConfig
	channelCfg channelConfig
	deviceCfg  deviceConfig
	transport  *ProfinetTransport
	decoder    *ProfinetDecoder
	scheduler  *ProfinetScheduler
}

func NewProfinetIODriver() driver.Driver {
	return &ProfinetIODriver{}
}

func (d *ProfinetIODriver) Init(cfg model.DriverConfig) error {
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	d.config = cfg
	d.channelCfg = parseChannelConfig(cfg.Config)
	d.decoder = NewProfinetDecoder()
	d.transport = NewProfinetTransport(d.channelCfg)
	d.scheduler = NewProfinetScheduler(d.transport, d.decoder)

	zap.L().Info("[Profinet IO] driver initialized",
		zap.String("local_interface", d.channelCfg.localInterface),
		zap.Bool("simulation", d.channelCfg.simulation),
	)
	return nil
}

func (d *ProfinetIODriver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("profinet-io driver not initialized")
	}
	return d.transport.Connect(ctx)
}

func (d *ProfinetIODriver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *ProfinetIODriver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *ProfinetIODriver) SetSlaveID(_ uint8) error {
	return nil
}

func (d *ProfinetIODriver) SetDeviceConfig(config map[string]any) error {
	d.deviceCfg = parseDeviceConfig(config)
	if d.transport != nil {
		d.transport.SetDeviceConfig(d.deviceCfg)
	}
	return nil
}

func (d *ProfinetIODriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		return
	}
	return d.transport.GetConnectionMetrics()
}

func (d *ProfinetIODriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("profinet-io driver not connected")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

func (d *ProfinetIODriver) WritePoint(ctx context.Context, p model.Point, value any) error {
	if d.transport == nil || !d.transport.IsConnected() {
		return fmt.Errorf("profinet-io driver not connected")
	}
	return d.scheduler.WritePoint(ctx, p, value)
}

func (d *ProfinetIODriver) GetMetrics() model.ChannelMetrics {
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()

	totalRequests, successCount, failureCount := int64(0), int64(0), int64(0)
	if d.scheduler != nil {
		totalRequests, successCount, failureCount = d.scheduler.GetStats()
	}

	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(successCount) / float64(totalRequests)
	}

	return model.ChannelMetrics{
		QualityScore:       d.calculateQualityScore(successRate),
		Protocol:           "Profinet IO",
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

func (d *ProfinetIODriver) calculateQualityScore(successRate float64) int {
	score := 85
	if successRate < 0.5 {
		score -= 30
	} else if successRate < 0.8 {
		score -= 15
	} else if successRate < 0.95 {
		score -= 5
	}
	if d.transport != nil && !d.transport.IsConnected() {
		score -= 40
	}
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}
	return score
}

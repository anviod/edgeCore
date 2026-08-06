package mitsubishi

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("mitsubishi-slmp", func() driver.Driver {
		return NewMitsubishiDriver()
	})
}

type MitsubishiDriver struct {
	config    model.DriverConfig
	driverCfg driverConfig
	transport *MCTransport
	decoder   *MCDecoder
	scheduler *MCScheduler
}

func NewMitsubishiDriver() driver.Driver {
	return &MitsubishiDriver{}
}

func (d *MitsubishiDriver) Init(cfg model.DriverConfig) error {
	dc, err := parseDriverConfig(cfg.Config)
	if err != nil {
		return fmt.Errorf("mitsubishi init failed: %w", err)
	}

	d.config = cfg
	d.driverCfg = dc
	d.decoder = NewMCDecoder()
	d.transport = NewMCTransport(dc)
	d.scheduler = NewMCScheduler(d.transport, d.decoder, dc.batchReadMax)

	zap.L().Info("[Mitsubishi] Driver initialized",
		zap.String("ip", dc.ip),
		zap.Int("port", dc.port),
		zap.String("frame", dc.frameType),
	)
	return nil
}

func (d *MitsubishiDriver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("mitsubishi driver not initialized")
	}
	if d.driverCfg.ip == "" {
		return fmt.Errorf("mitsubishi ip is required")
	}
	if err := d.transport.Connect(ctx); err != nil {
		return fmt.Errorf("mitsubishi connection failed: %w", err)
	}
	return nil
}

func (d *MitsubishiDriver) Disconnect() error {
	if d.transport == nil {
		return nil
	}
	return d.transport.Disconnect()
}

func (d *MitsubishiDriver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *MitsubishiDriver) SetSlaveID(slaveID uint8) error {
	return nil
}

func (d *MitsubishiDriver) SetDeviceConfig(config map[string]any) error {
	return nil
}

func (d *MitsubishiDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		return 0, 0, "", remoteAddrFromConfig(d.config.Config), time.Time{}
	}
	return d.transport.GetConnectionMetrics()
}

func (d *MitsubishiDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("mitsubishi driver not connected")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

func (d *MitsubishiDriver) WritePoint(ctx context.Context, point model.Point, value interface{}) error {
	if d.transport == nil || !d.transport.IsConnected() {
		return fmt.Errorf("mitsubishi driver not connected")
	}
	return d.scheduler.WritePoint(ctx, point, value)
}

func (d *MitsubishiDriver) GetMetrics() model.ChannelMetrics {
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
		Protocol:           "Mitsubishi MC",
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

func (d *MitsubishiDriver) calculateQualityScore(successRate float64) int {
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

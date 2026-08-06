package ethernetip

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("ethernet-ip", func() driver.Driver {
		return NewEtherNetIPDriver()
	})
}

type EtherNetIPDriver struct {
	config    model.DriverConfig
	transport *ENIPTransport
	decoder   *ENIPDecoder
	scheduler *ENIPScheduler
}

func NewEtherNetIPDriver() driver.Driver {
	return &EtherNetIPDriver{}
}

func (d *EtherNetIPDriver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	d.decoder = NewENIPDecoder()
	d.transport = NewENIPTransport(cfg.Config)
	d.scheduler = NewENIPScheduler(d.transport, d.decoder, cfg.Config)

	zap.L().Info("[ENIP] Driver initialized",
		zap.Any("config", cfg.Config),
	)
	return nil
}

func (d *EtherNetIPDriver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("ENIP driver not initialized")
	}

	if err := d.transport.Connect(ctx); err != nil {
		return fmt.Errorf("ENIP connection failed: %w", err)
	}

	return nil
}

func (d *EtherNetIPDriver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *EtherNetIPDriver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *EtherNetIPDriver) SetSlaveID(slaveID uint8) error {
	return nil
}

func (d *EtherNetIPDriver) SetDeviceConfig(config map[string]any) error {
	return nil
}

func (d *EtherNetIPDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		return
	}
	return d.transport.GetConnectionMetrics()
}

func (d *EtherNetIPDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("ENIP driver not connected")
	}

	return d.scheduler.ReadPoints(ctx, points)
}

func (d *EtherNetIPDriver) WritePoint(ctx context.Context, p model.Point, value interface{}) error {
	if d.transport == nil || !d.transport.IsConnected() {
		return fmt.Errorf("ENIP driver not connected")
	}

	return d.scheduler.WritePoint(ctx, p, value)
}

func (d *EtherNetIPDriver) GetMetrics() model.ChannelMetrics {
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
		Protocol:           "EtherNet/IP",
		SuccessRate:        successRate,
		TimeoutCount:       failureCount,
		CrcError:           0,
		CrcErrorRate:       0.0,
		RetryRate:          0.0,
		ExceptionCode:      0,
		AvgRtt:             0,
		MaxRtt:             0,
		MinRtt:             0,
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

func (d *EtherNetIPDriver) calculateQualityScore(successRate float64) int {
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

	if d.transport != nil {
		_, reconCount, _, _, _ := d.transport.GetConnectionMetrics()
		if reconCount > 10 {
			score -= 20
		} else if reconCount > 5 {
			score -= 10
		} else if reconCount > 0 {
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

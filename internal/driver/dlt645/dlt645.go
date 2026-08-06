package dlt645

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("dlt645", func() driver.Driver {
		return NewDLT645Driver()
	})
}

// DLT645Driver implements DL/T 645-2007 meter communication over serial or TCP.
type DLT645Driver struct {
	config    model.DriverConfig
	transport *DLT645Transport
	decoder   *DLT645Decoder
	scheduler *DLT645Scheduler
}

func NewDLT645Driver() driver.Driver {
	return &DLT645Driver{}
}

func (d *DLT645Driver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	d.decoder = NewDLT645Decoder()
	d.transport = NewDLT645Transport(cfg.Config)
	d.scheduler = NewDLT645Scheduler(d.transport, d.decoder)

	zap.L().Info("[DLT645] Driver initialized",
		zap.Any("config", cfg.Config),
	)
	return nil
}

func (d *DLT645Driver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("DLT645 driver not initialized")
	}
	return d.transport.Connect(ctx)
}

func (d *DLT645Driver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *DLT645Driver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *DLT645Driver) SetSlaveID(slaveID uint8) error {
	return nil
}

func (d *DLT645Driver) SetDeviceConfig(config map[string]any) error {
	if d.decoder == nil || config == nil {
		return nil
	}
	if addr, ok := config["station_address"].(string); ok && addr != "" {
		d.decoder.SetDefaultMeterAddress(addr)
	} else if addr, ok := config["address"].(string); ok && addr != "" {
		d.decoder.SetDefaultMeterAddress(addr)
	}
	return nil
}

func (d *DLT645Driver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		return 0, 0, "", remoteAddrFromConfig(d.config.Config), time.Time{}
	}
	return d.transport.GetConnectionMetrics()
}

func remoteAddrFromConfig(cfg map[string]any) string {
	if cfg == nil {
		return ""
	}
	return parseTransportConfig(cfg).remoteAddr()
}

func (d *DLT645Driver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("DLT645 driver not connected")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

func (d *DLT645Driver) WritePoint(ctx context.Context, p model.Point, value any) error {
	if d.transport == nil || !d.transport.IsConnected() {
		return fmt.Errorf("DLT645 driver not connected")
	}
	return d.scheduler.WritePoint(ctx, p, value)
}

func (d *DLT645Driver) GetMetrics() model.ChannelMetrics {
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
		Protocol:           "DLT645",
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

func (d *DLT645Driver) calculateQualityScore(successRate float64) int {
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

// BindLinkMutex injects channelMu into ConnectionManager for shared-link reconnect.
func (d *DLT645Driver) BindLinkMutex(mu *sync.Mutex) {
	if d.transport != nil && d.transport.connMgr != nil {
		d.transport.connMgr.SetLinkMutex(mu)
	}
}

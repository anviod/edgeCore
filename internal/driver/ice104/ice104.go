package ice104

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("iec60870-5-104", func() driver.Driver {
		return NewICE104Driver()
	})
}

type ICE104Driver struct {
	config    model.DriverConfig
	transport *ICE104Transport
	decoder   *ICE104Decoder
	scheduler *ICE104Scheduler
}

func NewICE104Driver() driver.Driver {
	return &ICE104Driver{}
}

func (d *ICE104Driver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	d.decoder = NewICE104Decoder()
	d.transport = NewICE104Transport(cfg.Config)
	d.scheduler = NewICE104Scheduler(d.transport, d.decoder, cfg.Config)
	zap.L().Info("[ICE104] driver initialized", zap.Any("config", cfg.Config))
	return nil
}

func (d *ICE104Driver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("ice104 driver not initialized")
	}
	if err := d.transport.Connect(ctx); err != nil {
		return fmt.Errorf("ice104 connection failed: %w", err)
	}
	return nil
}

func (d *ICE104Driver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *ICE104Driver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *ICE104Driver) SetSlaveID(_ uint8) error {
	return nil
}

func (d *ICE104Driver) SetDeviceConfig(config map[string]any) error {
	if d.transport != nil {
		d.transport.cfg = parseDeviceConfig(config)
	}
	if d.scheduler != nil {
		d.scheduler.cfg = parseDeviceConfig(config)
	}
	return nil
}

func (d *ICE104Driver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		if d.config.Config != nil {
			return 0, 0, "", parseDeviceConfig(d.config.Config).remoteAddr(), time.Time{}
		}
		return
	}
	connectionSeconds, reconnectCount, localAddr, remoteAddr, lastDisconnectTime = d.transport.GetConnectionMetrics()
	if remoteAddr == "" && d.config.Config != nil {
		remoteAddr = parseDeviceConfig(d.config.Config).remoteAddr()
	}
	return
}

func (d *ICE104Driver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.scheduler == nil {
		return nil, fmt.Errorf("ice104 driver not initialized")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

func (d *ICE104Driver) WritePoint(ctx context.Context, point model.Point, value any) error {
	if d.scheduler == nil {
		return fmt.Errorf("ice104 driver not initialized")
	}
	return d.scheduler.WritePoint(ctx, point, value)
}

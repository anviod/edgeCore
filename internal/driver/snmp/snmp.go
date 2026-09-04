package snmp

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"

	"go.uber.org/zap"
)

func init() {
	driver.RegisterDriver("snmp", func() driver.Driver {
		return NewSNMPDriver()
	})
}

type SNMPDriver struct {
	config    model.DriverConfig
	transport *SNMPTransport
	decoder   *SNMPDecoder
	scheduler *SNMPScheduler
}

func NewSNMPDriver() driver.Driver {
	return &SNMPDriver{}
}

func (d *SNMPDriver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	d.decoder = NewSNMPDecoder()
	d.transport = NewSNMPTransport(cfg.Config)
	d.scheduler = NewSNMPScheduler(d.transport, d.decoder, cfg.Config)
	zap.L().Info("[SNMP] driver initialized", zap.Any("config", cfg.Config))
	return nil
}

func (d *SNMPDriver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("snmp driver not initialized")
	}
	if err := d.transport.Connect(ctx); err != nil {
		return fmt.Errorf("snmp connection failed: %w", err)
	}
	return nil
}

func (d *SNMPDriver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *SNMPDriver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *SNMPDriver) SetSlaveID(_ uint8) error {
	return nil
}

func (d *SNMPDriver) SetDeviceConfig(config map[string]any) error {
	cfg := parseDeviceConfig(config)
	if d.transport != nil {
		d.transport.SetConfig(cfg)
	}
	if d.scheduler != nil {
		d.scheduler.cfg = cfg
	}
	return nil
}

func (d *SNMPDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
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

func (d *SNMPDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.scheduler == nil {
		return nil, fmt.Errorf("snmp driver not initialized")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

func (d *SNMPDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	if d.scheduler == nil {
		return fmt.Errorf("snmp driver not initialized")
	}
	return d.scheduler.WritePoint(ctx, point, value)
}

// ScanObjects walks a MIB subtree for device discovery (ObjectScanner).
func (d *SNMPDriver) ScanObjects(ctx context.Context, config map[string]any) (any, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("snmp not connected")
	}

	merged := make(map[string]any)
	if d.config.Config != nil {
		for k, v := range d.config.Config {
			merged[k] = v
		}
	}
	if config != nil {
		for k, v := range config {
			merged[k] = v
		}
	}
	cfg := parseDeviceConfig(merged)

	rootOID := stringFromAny(merged["rootOID"])
	if rootOID == "" {
		rootOID = "1.3.6.1.2.1"
	}
	community := cfg.Community
	if v := stringFromAny(merged["community"]); v != "" {
		community = v
	}

	type scanEntry struct {
		OID   string `json:"oid"`
		Value any    `json:"value"`
		Type  string `json:"type"`
	}
	entries := make([]scanEntry, 0, 64)

	err := d.transport.Walk(rootOID, community, func(pdu gosnmp.SnmpPDU) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries = append(entries, scanEntry{
			OID:   pdu.Name,
			Value: pdu.Value,
			Type:  fmt.Sprintf("%v", pdu.Type),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

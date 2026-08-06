package ethercat

import (
	"context"
	"fmt"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"go.uber.org/zap"
)

// EtherCATDriver implements the driver.Driver interface for EtherCAT protocol.
// It follows the ScanEngine Driver contract: pure execution function with no
// internal tickers, goroutines (except Transport-owned PDO cycle thread), or
// connection management.
//
// Architecture:
//
//	EtherCATDriver
//	  ├── transport (EtherCATTransport) — master lifecycle, PDO cycle, snapshots
//	  ├── scheduler (EtherCATScheduler) — ReadPoints/WritePoint orchestration
//	  └── decoder   (EtherCATDecoder)   — type/endian codec

type EtherCATDriver struct {
	config     model.DriverConfig
	channelCfg channelConfig
	deviceCfg  deviceConfig
	transport  *EtherCATTransport
	decoder    *EtherCATDecoder
	scheduler  *EtherCATScheduler
}

// init registers the ethercat protocol driver factory.
// This is called automatically at program startup via blank import in cmd/main.go.
func init() {
	driver.RegisterDriver("ethercat", func() driver.Driver {
		return NewEtherCATDriver()
	})
}

// NewEtherCATDriver creates a new EtherCAT driver instance.
func NewEtherCATDriver() *EtherCATDriver {
	return &EtherCATDriver{
		decoder: NewEtherCATDecoder(),
	}
}

// ============================================================================
// driver.Driver interface implementation
// ============================================================================

// Init parses channel configuration and creates transport/scheduler components.
// Does NOT establish network connection (deferred to Connect).
func (d *EtherCATDriver) Init(cfg model.DriverConfig) error {
	d.config = cfg

	chCfg, err := parseChannelConfig(cfg.Config)
	if err != nil {
		return fmt.Errorf("ethercat Init: %w", err)
	}
	d.channelCfg = chCfg

	d.transport = NewEtherCATTransport(d.channelCfg)
	d.scheduler = NewEtherCATScheduler(d.transport, d.decoder)

	zap.L().Info("ethercat: driver initialized",
		zap.String("channel_id", cfg.ChannelID),
		zap.String("interface", d.channelCfg.localInterface),
		zap.Duration("cycle_time", d.channelCfg.cycleTime),
		zap.Bool("simulation", d.channelCfg.simulation),
	)
	return nil
}

// Connect initializes the EtherCAT master and starts the PDO cycle thread.
// Delegates to transport.Connect which uses ConnectionManager for retry/backoff.
func (d *EtherCATDriver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("ethercat Connect: driver not initialized")
	}
	return d.transport.Connect(ctx)
}

// Disconnect stops the PDO cycle thread and closes the master.
// Idempotent — safe to call multiple times.
func (d *EtherCATDriver) Disconnect() error {
	if d.transport == nil {
		return nil
	}
	d.transport.Disconnect()
	return nil
}

// ReadPoints reads values for the given points from PDO snapshots or SDO mailbox.
// PDO reads are zero-wait (atomic snapshot memory reads).
// SDO reads are synchronous with independent timeout.
// Each point is processed independently — failures are per-point, not batch-aborting.
func (d *EtherCATDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.scheduler == nil {
		return nil, fmt.Errorf("ethercat ReadPoints: driver not initialized")
	}
	if !d.transport.IsConnected() {
		return nil, fmt.Errorf("ethercat ReadPoints: master not connected")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

// WritePoint writes a value to a single point.
// PDO writes go to the RxPDO buffer for next-cycle delivery.
// SDO writes are synchronous via CoE mailbox.
func (d *EtherCATDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	if d.scheduler == nil {
		return fmt.Errorf("ethercat WritePoint: driver not initialized")
	}
	if !d.transport.IsConnected() {
		return fmt.Errorf("ethercat WritePoint: master not connected")
	}
	return d.scheduler.WritePoint(ctx, point, value)
}

// Health returns the current health status of the EtherCAT master.
// Returns HealthStatusGood when the master is connected (OP state).
// Returns HealthStatusBad when disconnected or in error state.
func (d *EtherCATDriver) Health() driver.HealthStatus {
	if d.transport == nil {
		return driver.HealthStatusBad
	}
	if d.transport.IsConnected() {
		return driver.HealthStatusGood
	}
	return driver.HealthStatusBad
}

// SetSlaveID is a no-op for EtherCAT.
// EtherCAT uses positional addressing, not slave ID.
func (d *EtherCATDriver) SetSlaveID(slaveID uint8) error {
	return nil
}

// SetDeviceConfig parses and stores per-device (slave) configuration.
// Called by ChannelManager when switching device context.
func (d *EtherCATDriver) SetDeviceConfig(config map[string]any) error {
	devCfg, err := parseDeviceConfig(config)
	if err != nil {
		return fmt.Errorf("ethercat SetDeviceConfig: %w", err)
	}
	d.deviceCfg = devCfg
	if d.transport != nil {
		d.transport.SetDeviceConfig(d.deviceCfg)
	}
	return nil
}

// GetConnectionMetrics returns connection statistics for diagnostics.
func (d *EtherCATDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		return 0, 0, d.channelCfg.localInterface, "ethercat-bus", time.Time{}
	}
	cs, rc, la, ra := d.transport.GetConnectionMetrics()
	return cs, rc, la, ra, time.Time{}
}

// ============================================================================
// Optional interfaces
// ============================================================================

// ScanResult holds the result of a bus scan operation.
type ScanResult struct {
	Position    int    `json:"position"`
	VendorID    string `json:"vendor_id"`
	ProductCode string `json:"product_code"`
	Revision    string `json:"revision"`
	TxPDOSize   int    `json:"tx_pdo_size"`
	RxPDOSize   int    `json:"rx_pdo_size"`
}

// Scan performs a bus scan and returns discovered slaves.
// Implements driver.Scanner interface.
func (d *EtherCATDriver) Scan(ctx context.Context, params map[string]any) (any, error) {
	if d.transport == nil {
		return nil, fmt.Errorf("ethercat Scan: transport not initialized")
	}

	// Ensure the master is initialized for scanning
	if !d.transport.IsConnected() {
		if err := d.transport.Connect(ctx); err != nil {
			return nil, fmt.Errorf("ethercat Scan: connect failed: %w", err)
		}
	}

	slaves, err := d.transport.master.scanSlaves()
	if err != nil {
		return nil, fmt.Errorf("ethercat Scan: %w", err)
	}

	results := make([]ScanResult, 0, len(slaves))
	for _, sl := range slaves {
		results = append(results, ScanResult{
			Position:    sl.Position,
			VendorID:    fmt.Sprintf("0x%08X", sl.VendorID),
			ProductCode: fmt.Sprintf("0x%08X", sl.ProductCode),
			Revision:    fmt.Sprintf("0x%08X", sl.Revision),
			TxPDOSize:   sl.TxPDOSize,
			RxPDOSize:   sl.RxPDOSize,
		})
	}

	zap.L().Info("ethercat: scan completed",
		zap.Int("slave_count", len(results)),
	)
	return results, nil
}

// ResetDeviceCollection clears the device's PDO snapshot cache.
// Called by ScanEngine when points are added or removed for a device.
// Implements driver.DeviceCollectionResetter interface.
func (d *EtherCATDriver) ResetDeviceCollection(deviceID string) {
	if d.transport != nil {
		d.transport.ResetDeviceCollection()
		zap.L().Info("ethercat: device collection reset",
			zap.String("device_id", deviceID),
		)
	}
}

// Ensure EtherCATDriver implements required interfaces.
var (
	_ driver.Driver                   = (*EtherCATDriver)(nil)
	_ driver.Scanner                  = (*EtherCATDriver)(nil)
	_ driver.DeviceCollectionResetter = (*EtherCATDriver)(nil)
)

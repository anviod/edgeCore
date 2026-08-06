package knxnetip

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
	driver.RegisterDriver("knxnet-ip", func() driver.Driver {
		return NewKNXnetIPDriver()
	})
}

// KNXnetIPDriver implements KNXnet/IP tunneling for group address read/write.
type KNXnetIPDriver struct {
	config    model.DriverConfig
	transport *KNXTransport
	decoder   *KNXDecoder
	scheduler *KNXScheduler
}

func NewKNXnetIPDriver() driver.Driver {
	return &KNXnetIPDriver{}
}

func (d *KNXnetIPDriver) Init(cfg model.DriverConfig) error {
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}
	d.config = cfg
	d.decoder = NewKNXDecoder()
	d.transport = NewKNXTransport(cfg.Config)
	d.scheduler = NewKNXScheduler(d.transport, d.decoder)

	tc := parseTransportConfig(cfg.Config)
	zap.L().Info("[KNXnet/IP] driver initialized",
		zap.String("gateway", tc.remoteAddr()),
		zap.String("mode", tc.mode),
		zap.Bool("discovery", tc.discovery),
	)
	return nil
}

func (d *KNXnetIPDriver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("KNXnet/IP driver not initialized")
	}
	if err := d.ensureGatewayConfigured(ctx); err != nil {
		return err
	}
	return d.transport.Connect(ctx)
}

func (d *KNXnetIPDriver) ensureGatewayConfigured(ctx context.Context) error {
	tc := parseTransportConfig(d.config.Config)
	if tc.ip != "" {
		return nil
	}
	if !tc.discovery {
		return fmt.Errorf("KNXnet/IP: gateway IP (ip) is required, or enable discovery")
	}

	discoveryCtx, cancel := context.WithTimeout(ctx, tc.discoveryTimeout)
	defer cancel()

	gateways, err := DiscoverGateways(discoveryCtx, tc)
	if err != nil {
		return fmt.Errorf("KNXnet/IP: gateway discovery failed: %w", err)
	}
	gw := gateways[0]
	if d.config.Config == nil {
		d.config.Config = map[string]any{}
	}
	d.config.Config["ip"] = gw.IP
	if gw.Port > 0 {
		d.config.Config["port"] = gw.Port
	}

	d.transport = NewKNXTransport(d.config.Config)
	d.scheduler = NewKNXScheduler(d.transport, d.decoder)
	return nil
}

func (d *KNXnetIPDriver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *KNXnetIPDriver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *KNXnetIPDriver) SetSlaveID(_ uint8) error {
	return nil
}

func (d *KNXnetIPDriver) SetDeviceConfig(_ map[string]any) error {
	return nil
}

func (d *KNXnetIPDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
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

func (d *KNXnetIPDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("KNXnet/IP driver not connected")
	}
	return d.scheduler.ReadPoints(ctx, points)
}

func (d *KNXnetIPDriver) WritePoint(ctx context.Context, p model.Point, value any) error {
	if d.transport == nil || !d.transport.IsConnected() {
		return fmt.Errorf("KNXnet/IP driver not connected")
	}
	return d.scheduler.WritePoint(ctx, p, value)
}

func (d *KNXnetIPDriver) GetMetrics() model.ChannelMetrics {
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
		Protocol:           "KNXnet/IP",
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

func (d *KNXnetIPDriver) calculateQualityScore(successRate float64) int {
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
func (d *KNXnetIPDriver) BindLinkMutex(mu *sync.Mutex) {
	if d.transport != nil && d.transport.connMgr != nil {
		d.transport.connMgr.SetLinkMutex(mu)
	}
}

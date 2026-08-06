package s7

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/anviod/gos7"
	"go.uber.org/zap"
)

// Client 接口别名，方便mock测试
type Client = gos7.Client

func init() {
	driver.RegisterDriver("s7", func() driver.Driver {
		return NewS7Driver()
	})
}

// S7Driver S7协议驱动（真实实现）
type S7Driver struct {
	config    model.DriverConfig
	transport *S7Transport
	decoder   *S7Decoder
	scheduler *S7Scheduler
}

func NewS7Driver() driver.Driver {
	return &S7Driver{}
}

func (d *S7Driver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	d.decoder = NewS7Decoder()
	d.transport = NewS7Transport(cfg.Config)
	d.scheduler = NewS7Scheduler(d.transport, d.decoder, cfg.Config)

	zap.L().Info("[S7] Driver initialized",
		zap.Any("config", cfg.Config),
	)
	return nil
}

func (d *S7Driver) Connect(ctx context.Context) error {
	if d.transport == nil {
		return fmt.Errorf("S7 driver not initialized")
	}

	if err := d.transport.Connect(ctx); err != nil {
		return fmt.Errorf("S7 connection failed: %w", err)
	}

	return nil
}

func (d *S7Driver) Disconnect() error {
	if d.transport != nil {
		return d.transport.Disconnect()
	}
	return nil
}

func (d *S7Driver) Health() driver.HealthStatus {
	if d.transport == nil || !d.transport.IsConnected() {
		return driver.HealthStatusBad
	}
	return driver.HealthStatusGood
}

func (d *S7Driver) SetSlaveID(slaveID uint8) error {
	// S7协议不使用SlaveID，但可能映射到rack/slot
	// 暂不实现动态修改
	return nil
}

func (d *S7Driver) SetDeviceConfig(config map[string]any) error {
	return nil
}

func (d *S7Driver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	if d.transport == nil {
		return
	}
	return d.transport.GetConnectionMetrics()
}

func (d *S7Driver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if d.transport == nil || !d.transport.IsConnected() {
		return nil, fmt.Errorf("S7 driver not connected")
	}

	return d.scheduler.ReadPoints(ctx, points)
}

func (d *S7Driver) WritePoint(ctx context.Context, p model.Point, value any) error {
	if d.transport == nil || !d.transport.IsConnected() {
		return fmt.Errorf("S7 driver not connected")
	}

	return d.scheduler.WritePoint(ctx, p, value)
}

// GetMetrics 返回S7驱动的详细指标
func (d *S7Driver) GetMetrics() model.ChannelMetrics {
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
		Protocol:           "S7",
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

// calculateQualityScore 计算S7质量评分
func (d *S7Driver) calculateQualityScore(successRate float64) int {
	score := 85

	// 根据成功率调整
	if successRate < 0.5 {
		score -= 30
	} else if successRate < 0.8 {
		score -= 15
	} else if successRate < 0.95 {
		score -= 5
	}

	// 根据连接状态调整
	if d.transport != nil && !d.transport.IsConnected() {
		score -= 40
	}

	// 根据重连次数调整
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

// parseConfigFloat 从配置中解析float64值
func parseConfigFloat(cfg map[string]any, key string, defaultVal float64) float64 {
	if v, ok := cfg[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case int:
			return float64(val)
		case string:
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return f
			}
		}
	}
	return defaultVal
}

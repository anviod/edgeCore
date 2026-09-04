package server

import (
	"context"
	"time"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// 使用 model 包中的全局 collector（在 init 中注入）

// getChannelMetrics 获取通道监控指标
func (s *Server) getChannelMetrics(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel id is required"})
	}

	// 获取通道信息
	ch := s.cm.GetChannel(channelID)
	if ch == nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}

	metrics := &model.ChannelMetrics{
		Timestamp: time.Now(),
		Protocol:  ch.Protocol,
	}

	// 从 driver 获取连接状态与详细指标（优先于 collector 默认值）
	driver := s.cm.GetDriver(channelID)
	if driver != nil {
		connSec, reconCount, localAddr, remoteAddr, lastDisc := driver.GetConnectionMetrics()
		metrics.ConnectionSeconds = connSec
		metrics.ReconnectCount = reconCount
		metrics.LocalAddr = localAddr
		metrics.RemoteAddr = remoteAddr
		metrics.LastDisconnectTime = lastDisc
		metrics.LinkUp = driver.Health() == drv.HealthStatusGood

		if metricsDriver, ok := driver.(interface{ GetMetrics() model.ChannelMetrics }); ok {
			metricsChannel := make(chan model.ChannelMetrics, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						zap.L().Warn("GetMetrics panic", zap.String("channelId", channelID), zap.Any("error", r))
					}
				}()
				metricsChannel <- metricsDriver.GetMetrics()
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()

			select {
			case driverMetrics := <-metricsChannel:
				mergeDriverChannelMetrics(metrics, &driverMetrics)
			case <-ctx.Done():
				zap.L().Warn("GetMetrics timeout", zap.String("channelId", channelID))
			}
		}
	}

	// 合并 collector 中已观测到的请求统计（补充驱动侧不完整的数据）
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		collected := mc.GetChannelMetrics(channelID)
		if collected != nil && (collected.TotalRequests > 0 || collected.SuccessRate > 0) {
			mergeCollectorChannelMetrics(metrics, collected)
		}
	}

	finalizeChannelMetrics(metrics)
	s.enrichChannelScanEngineMetrics(channelID, ch, metrics)

	// 更新时间戳
	metrics.Timestamp = time.Now()

	return c.JSON(metrics)
}

func mergeDriverChannelMetrics(dst, src *model.ChannelMetrics) {
	dst.QualityScore = src.QualityScore
	if src.Protocol != "" {
		dst.Protocol = src.Protocol
	}
	dst.SuccessRate = src.SuccessRate
	dst.TimeoutCount = src.TimeoutCount
	dst.CrcError = src.CrcError
	dst.CrcErrorRate = src.CrcErrorRate
	dst.RetryRate = src.RetryRate
	dst.ExceptionCode = src.ExceptionCode
	dst.AvgRtt = src.AvgRtt
	dst.MaxRtt = src.MaxRtt
	dst.MinRtt = src.MinRtt
	dst.TotalRequests = src.TotalRequests
	dst.SuccessCount = src.SuccessCount
	dst.FailureCount = src.FailureCount
	dst.PacketLoss = src.PacketLoss
	dst.Trend = src.Trend
	dst.RecentErrors = src.RecentErrors
	if src.ConnectionSeconds > 0 {
		dst.ConnectionSeconds = src.ConnectionSeconds
	}
	if src.ReconnectCount > 0 {
		dst.ReconnectCount = src.ReconnectCount
	}
	if src.LocalAddr != "" {
		dst.LocalAddr = src.LocalAddr
	}
	if src.RemoteAddr != "" {
		dst.RemoteAddr = src.RemoteAddr
	}
	if !src.LastDisconnectTime.IsZero() {
		dst.LastDisconnectTime = src.LastDisconnectTime
	}
}

func mergeCollectorChannelMetrics(dst, src *model.ChannelMetrics) {
	if src.TotalRequests > dst.TotalRequests {
		dst.TotalRequests = src.TotalRequests
		dst.SuccessCount = src.SuccessCount
		dst.FailureCount = src.FailureCount
		dst.SuccessRate = src.SuccessRate
		dst.PacketLoss = src.PacketLoss
		dst.AvgRtt = src.AvgRtt
		dst.MaxRtt = src.MaxRtt
		dst.MinRtt = src.MinRtt
		dst.TimeoutCount = src.TimeoutCount
		dst.CrcError = src.CrcError
		dst.CrcErrorRate = src.CrcErrorRate
		dst.RetryRate = src.RetryRate
		dst.ExceptionCode = src.ExceptionCode
		dst.Trend = src.Trend
		dst.RecentErrors = src.RecentErrors
		if src.QualityScore > 0 {
			dst.QualityScore = src.QualityScore
		}
		return
	}

	// Driver request counters may be incomplete; supplement from collector when available.
	if dst.TotalRequests > 0 && dst.SuccessCount == 0 && src.SuccessCount > 0 {
		dst.SuccessCount = src.SuccessCount
		dst.FailureCount = src.FailureCount
	}

	if dst.SuccessRate <= 0 && src.SuccessRate > 0 {
		dst.SuccessRate = src.SuccessRate
		dst.PacketLoss = src.PacketLoss
	}

	if dst.AvgRtt == 0 && src.AvgRtt > 0 {
		dst.AvgRtt = src.AvgRtt
		dst.MinRtt = src.MinRtt
		dst.MaxRtt = src.MaxRtt
	}

	if src.QualityScore > dst.QualityScore {
		dst.QualityScore = src.QualityScore
	}
}

func finalizeChannelMetrics(metrics *model.ChannelMetrics) {
	if !metrics.LinkUp {
		metrics.QualityScore = 0
		metrics.SuccessRate = 0
		metrics.PacketLoss = 0
		metrics.AvgRtt = 0
		return
	}

	if metrics.TotalRequests == 0 {
		metrics.SuccessRate = 0
		metrics.PacketLoss = 0
		metrics.AvgRtt = 0
		metrics.CrcErrorRate = 0
		metrics.RetryRate = 0
		return
	}

	if metrics.SuccessRate <= 0 && metrics.TotalRequests > 0 && metrics.SuccessCount > 0 {
		metrics.SuccessRate = float64(metrics.SuccessCount) / float64(metrics.TotalRequests)
	}
	if metrics.PacketLoss <= 0 && metrics.TotalRequests > 0 {
		metrics.PacketLoss = 1.0 - metrics.SuccessRate
	}
}

// getDeviceMetrics 获取设备监控指标
func (s *Server) getDeviceMetrics(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	deviceID := c.Params("deviceId")

	if channelID == "" || deviceID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel id and device id are required"})
	}

	// 获取通道信息
	ch := s.cm.GetChannel(channelID)
	if ch == nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}

	// 查找设备
	var device *model.Device
	for i := range ch.Devices {
		if ch.Devices[i].ID == deviceID {
			device = &ch.Devices[i]
			break
		}
	}

	if device == nil {
		return c.Status(404).JSON(fiber.Map{"error": "device not found"})
	}

	// 获取设备指标
	metrics := model.GetGlobalMetricsCollector().GetDeviceMetrics(deviceID)

	// 补充设备基本信息（含通道链路有效状态）
	dev := s.cm.GetDevice(channelID, deviceID)
	if dev != nil {
		metrics.State = dev.State
	} else if device.Enable {
		node := s.cm.GetStateManager().GetNode(deviceID)
		if node != nil {
			metrics.State = int(node.Runtime.State)
		}
	} else {
		metrics.State = 2 // 离线
	}

	if shadowCore := s.shadowCoreRef(); shadowCore != nil {
		if opt := shadowCore.GetDeviceOptimization(deviceID); opt != nil {
			metrics.CommunicationProfile = opt
		}
	}

	// 更新时间戳
	metrics.Timestamp = time.Now()

	return c.JSON(metrics)
}

// RecordChannelRequest 记录通道请求指标 (供 driver 调用)
func RecordChannelRequest(channelID string, success bool, duration time.Duration, errorType string) {
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		mc.RecordRequest(channelID, success, duration, errorType)
	}
}

// RecordChannelError 记录通道错误 (供 driver 调用)
func RecordChannelError(channelID string, errType, code, message string) {
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		mc.RecordError(channelID, errType, code, message)
	}
}

// RecordChannelReconnect 记录通道重连 (供 driver 调用)
func RecordChannelReconnect(channelID string) {
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		mc.RecordReconnect(channelID)
	}
}

// RecordChannelConnectionStart 记录通道连接开始 (供 driver 调用)
func RecordChannelConnectionStart(channelID string) {
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		mc.RecordConnectionStart(channelID)
	}
}

// UpdateDeviceMetrics 更新设备指标 (供 driver 调用)
func UpdateDeviceMetrics(deviceID string, update func(*model.DeviceMetrics)) {
	if mc := model.GetGlobalMetricsCollector(); mc != nil {
		mc.UpdateDeviceMetrics(deviceID, update)
	}
}

// getPointDebug 返回点位调试信息（原始字节 + 解析后值）
func (s *Server) getPointDebug(c *fiber.Ctx) error {
	pointID := c.Params("pointId")
	if pointID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "point id is required"})
	}

	mc := model.GetGlobalMetricsCollector()
	if mc != nil {
		pm := mc.GetPointMetrics(pointID)
		if pm != nil && pm.LastUpdateTime.After(time.Time{}) {
			return c.JSON(pm)
		}
	}

	// Fallback: try to get last stored value
	if s.storage != nil {
		if val, err := s.storage.GetLastValue(pointID); err == nil && val != nil {
			resp := model.PointMetrics{
				PointID:        pointID,
				LastUpdateTime: val.TS,
				Quality:        val.Quality,
				ParsedValue:    val.Value,
			}
			return c.JSON(resp)
		}
	}

	return c.Status(404).JSON(fiber.Map{"error": "debug info not found"})
}

func (s *Server) enrichChannelScanEngineMetrics(channelID string, ch *model.Channel, metrics *model.ChannelMetrics) {
	if s.cm == nil || metrics == nil || ch == nil {
		return
	}

	seSnap := s.cm.GetChannelScanEngineMetricsSnapshot(channelID)

	scanSLA := &model.ChannelScanSLA{}
	if p95, ok := seSnap["scan_lag_p95_ms"].(float64); ok {
		metrics.ScanLagP95Ms = p95
		scanSLA.ScanLagP95Ms = p95
	}
	if drift, ok := seSnap["scan_drift_avg_ms"].(float64); ok {
		metrics.ScanDriftAvgMs = drift
		scanSLA.ScanDriftAvgMs = drift
	}
	if driftWin, ok := seSnap["scan_drift_avg_ms_window"].(float64); ok {
		metrics.ScanDriftAvgMsWindow = driftWin
		scanSLA.ScanDriftAvgMsWindow = driftWin
	}
	if missTotal, ok := seSnap["scan_miss_deadline_total"].(uint64); ok {
		metrics.ScanMissDeadlineTotal = missTotal
		scanSLA.ScanMissDeadlineTotal = missTotal
	}
	if missWin, ok := seSnap["scan_miss_deadline_window"].(uint64); ok {
		metrics.ScanMissDeadlineWindow = missWin
		scanSLA.ScanMissDeadlineWindow = missWin
	}
	if openCount, ok := seSnap["circuit_breaker_open"].(int); ok {
		metrics.CircuitBreakerOpen = openCount
		scanSLA.CircuitBreakerOpen = openCount
	}
	if warnings, ok := seSnap["sla_warnings"].([]map[string]any); ok {
		metrics.SLAWarnings = warnings
		scanSLA.SLAWarnings = warnings
	}
	metrics.ScanSLA = scanSLA

	metrics.QualityScore = model.CalculateQualityScore(metrics)
}

// init 初始化
func init() {
	// 初始化全局指标收集器并注入到 model 包
	mc := model.NewMetricsCollector()
	model.SetGlobalMetricsCollector(mc)
	zap.L().Info("Metrics collector initialized")
}

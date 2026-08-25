package model

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	pointDebugSampleRate   = 100 // 热路径 1% 采样
	pointMetricsMaxEntries = 500
	pointDebugMaxRawBytes  = 64
)

var pointDebugCounter atomic.Uint64

// ChannelMetrics 通道级监控指标
type ChannelMetrics struct {
	// 质量评分 (0-100)
	QualityScore int `json:"qualityScore"`

	// 协议信息
	Protocol string `json:"protocol"` // 协议类型

	// 通信质量指标
	SuccessRate   float64 `json:"successRate"`   // 成功率 (0-1)
	TimeoutCount  int64   `json:"timeoutCount"`  // 超时次数
	CrcError      int64   `json:"crcError"`      // CRC错误次数
	CrcErrorRate  float64 `json:"crcErrorRate"`  // CRC错误率
	RetryRate     float64 `json:"retryRate"`     // 重试率
	ExceptionCode int64   `json:"exceptionCode"` // 异常响应次数

	// 响应时间指标 (毫秒)
	AvgRtt float64 `json:"avgRtt"` // 平均响应时间
	MaxRtt float64 `json:"maxRtt"` // 最大响应时间
	MinRtt float64 `json:"minRtt"` // 最小响应时间

	// 连接指标
	LocalAddr          string    `json:"localAddr"`          // 本地地址 (IP:Port)
	RemoteAddr         string    `json:"remoteAddr"`         // 远程地址 (IP:Port)
	LinkUp             bool      `json:"linkUp"`             // 通道链路是否已建立
	ReconnectCount     int64     `json:"reconnectCount"`     // 重连次数
	ConnectionSeconds  int64     `json:"connectionSeconds"`  // 当前连接时长(秒)
	LastDisconnectTime time.Time `json:"lastDisconnectTime"` // 最后断开时间

	// 请求统计
	TotalRequests int64 `json:"totalRequests"` // 总请求数
	SuccessCount  int64 `json:"successCount"`  // 成功次数
	FailureCount  int64 `json:"failureCount"`  // 失败次数

	// 丢包率
	PacketLoss float64 `json:"packetLoss"` // (超时+CRC)/总数

	// 趋势数据 (最近1小时，每5分钟一个点)
	Trend []TrendPoint `json:"trend,omitempty"`

	// 最近异常 (最近10条)
	RecentErrors []ErrorRecord `json:"recentErrors,omitempty"`

	// ScanEngine SLA 联动（Phase C）
	ScanLagP95Ms           float64          `json:"scanLagP95Ms,omitempty"`
	ScanDriftAvgMs         float64          `json:"scanDriftAvgMs,omitempty"`
	ScanDriftAvgMsWindow   float64          `json:"scanDriftAvgMsWindow,omitempty"`
	ScanMissDeadlineTotal  uint64           `json:"scanMissDeadlineTotal,omitempty"`
	ScanMissDeadlineWindow uint64           `json:"scanMissDeadlineWindow,omitempty"`
	CircuitBreakerOpen     int              `json:"circuitBreakerOpen,omitempty"`
	SLAWarnings            []map[string]any `json:"slaWarnings,omitempty"`

	// ScanSLA 调度 SLA 明细（通道级，与通信质量分离）
	ScanSLA *ChannelScanSLA `json:"scanSla,omitempty"`

	// 时间戳
	Timestamp time.Time `json:"timestamp"`
}

// ChannelScanSLA 通道级调度 SLA（与通信质量指标分离）。
type ChannelScanSLA struct {
	ScanLagP95Ms           float64          `json:"scanLagP95Ms,omitempty"`
	ScanDriftAvgMs         float64          `json:"scanDriftAvgMs,omitempty"`
	ScanDriftAvgMsWindow   float64          `json:"scanDriftAvgMsWindow,omitempty"`
	ScanMissDeadlineTotal  uint64           `json:"scanMissDeadlineTotal,omitempty"`
	ScanMissDeadlineWindow uint64           `json:"scanMissDeadlineWindow,omitempty"`
	CircuitBreakerOpen     int              `json:"circuitBreakerOpen,omitempty"`
	SLAWarnings            []map[string]any `json:"slaWarnings,omitempty"`
}

// TrendPoint 趋势数据点
type TrendPoint struct {
	Time time.Time `json:"time"` // 时间点
	Rate float64   `json:"rate"` // 成功率
}

// ErrorRecord 错误记录
type ErrorRecord struct {
	Time    time.Time `json:"time"`    // 发生时间
	Type    string    `json:"type"`    // 错误类型: timeout, crc, exception, network
	Code    string    `json:"code"`    // 错误码
	Message string    `json:"message"` // 错误描述
}

// DeviceMetrics 设备级监控指标
type DeviceMetrics struct {
	// 健康评分 (0-100)
	HealthScore int `json:"healthScore"`

	// 基础信息
	State               int       `json:"state"`               // 0:在线, 1:不稳定, 2:离线, 3:隔离
	LastCollectTime     time.Time `json:"lastCollectTime"`     // 上次采集时间
	ConsecutiveFailures int       `json:"consecutiveFailures"` // 连续失败次数
	Degraded            bool      `json:"degraded"`            // 是否降级
	Recovering          bool      `json:"recovering"`          // 是否恢复中

	// 采集质量
	PointSuccessRate float64 `json:"pointSuccessRate"` // 点位成功率
	AvgCollectTime   float64 `json:"avgCollectTime"`   // 平均采集耗时(ms)
	AbnormalPoints   int     `json:"abnormalPoints"`   // 异常点位数量
	InvalidValues    int     `json:"invalidValues"`    // 无效值数量
	NullValueRate    float64 `json:"nullValueRate"`    // Null值比例

	// 时间戳
	Timestamp time.Time `json:"timestamp"`

	// 通信画像（RTT/MTU/Gap，来自 Shadow 优化器）
	CommunicationProfile map[string]interface{} `json:"communicationProfile,omitempty"`
}

// PointMetrics 点位级监控指标
type PointMetrics struct {
	PointID        string    `json:"pointId"`        // 点位ID
	LastUpdateTime time.Time `json:"lastUpdateTime"` // 最近更新时间
	Quality        string    `json:"quality"`        // 质量码: Good, Bad, Uncertain
	RawValue       []byte    `json:"rawValue"`       // 原始寄存器数据
	ParsedValue    any       `json:"parsedValue"`    // 解析后的值
	DataType       string    `json:"dataType"`       // 数据类型
	ByteOrder      string    `json:"byteOrder"`      // 字节序
	UpdateCount    int64     `json:"updateCount"`    // 更新次数
	ErrorCount     int64     `json:"errorCount"`     // 错误次数
	LastError      string    `json:"lastError"`      // 最后错误信息
	LastErrorTime  time.Time `json:"lastErrorTime"`  // 最后错误时间
}

// MetricsCollector 监控指标收集器
type MetricsCollector struct {
	mu sync.RWMutex

	// 通道级指标
	channelMetrics map[string]*ChannelMetrics // channelId -> metrics

	// 设备级指标
	deviceMetrics map[string]*DeviceMetrics // deviceId -> metrics

	// 点位级指标
	pointMetrics map[string]*PointMetrics // pointId -> metrics

	// 滑动窗口数据 (用于计算实时指标)
	windowSize   time.Duration
	history      map[string][]RequestRecord // channelId -> records
	cycleHistory map[string][]CycleRecord   // channelId -> records
}

// RequestRecord 请求记录 (用于滑动窗口统计)
type RequestRecord struct {
	Timestamp time.Time
	Success   bool
	Duration  time.Duration // RTT
	ErrorType string        // timeout, crc, exception, network
}

// CycleRecord 周期记录 (一轮采集的结果)
type CycleRecord struct {
	Timestamp time.Time
	Success   bool
}

// NewMetricsCollector 创建新的监控指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		channelMetrics: make(map[string]*ChannelMetrics),
		deviceMetrics:  make(map[string]*DeviceMetrics),
		windowSize:     time.Minute * 5, // 5分钟滑动窗口
		history:        make(map[string][]RequestRecord),
		cycleHistory:   make(map[string][]CycleRecord),
		pointMetrics:   make(map[string]*PointMetrics),
	}
}

// Global collector accessible across packages
var globalMetricsCollector atomic.Pointer[MetricsCollector]

// SetGlobalMetricsCollector sets the shared collector instance
func SetGlobalMetricsCollector(mc *MetricsCollector) {
	globalMetricsCollector.Store(mc)
}

// GetGlobalMetricsCollector returns the shared collector instance
func GetGlobalMetricsCollector() *MetricsCollector {
	return globalMetricsCollector.Load()
}

// RecordRequest 记录一次请求
func (mc *MetricsCollector) RecordRequest(channelID string, success bool, duration time.Duration, errorType string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	record := RequestRecord{
		Timestamp: time.Now(),
		Success:   success,
		Duration:  duration,
		ErrorType: errorType,
	}

	// 添加到历史记录
	mc.history[channelID] = append(mc.history[channelID], record)

	// 清理过期数据
	mc.cleanupHistory(channelID)

	// 更新通道指标
	mc.updateChannelMetrics(channelID)
}

// RecordCycle 记录一轮完整采集的结果
func (mc *MetricsCollector) RecordCycle(channelID string, success bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	record := CycleRecord{
		Timestamp: time.Now(),
		Success:   success,
	}

	mc.cycleHistory[channelID] = append(mc.cycleHistory[channelID], record)

	// 清理过期周期数据
	cutoff := time.Now().Add(-mc.windowSize)
	cycles := mc.cycleHistory[channelID]
	var valid []CycleRecord
	for _, c := range cycles {
		if c.Timestamp.After(cutoff) {
			valid = append(valid, c)
		}
	}
	mc.cycleHistory[channelID] = valid

	// 更新通道指标
	mc.updateChannelMetrics(channelID)
}

// RecordReconnect 记录重连事件
func (mc *MetricsCollector) RecordReconnect(channelID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	metrics := mc.getOrCreateChannelMetrics(channelID)
	metrics.ReconnectCount++
	metrics.LastDisconnectTime = time.Now()
}

// RecordConnectionStart 记录连接开始
func (mc *MetricsCollector) RecordConnectionStart(channelID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	metrics := mc.getOrCreateChannelMetrics(channelID)
	metrics.LastDisconnectTime = time.Time{} // 清空断开时间
}

// GetChannelMetrics 获取通道监控指标
func (mc *MetricsCollector) GetChannelMetrics(channelID string) *ChannelMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if metrics, ok := mc.channelMetrics[channelID]; ok {
		// 创建副本
		copy := *metrics
		return &copy
	}

	return &ChannelMetrics{
		Timestamp: time.Now(),
	}
}

// GetDeviceMetrics 获取设备监控指标
func (mc *MetricsCollector) GetDeviceMetrics(deviceID string) *DeviceMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if metrics, ok := mc.deviceMetrics[deviceID]; ok {
		copy := *metrics
		return &copy
	}

	return &DeviceMetrics{
		Timestamp: time.Now(),
	}
}

// UpdateDeviceMetrics 更新设备监控指标
func (mc *MetricsCollector) UpdateDeviceMetrics(deviceID string, update func(*DeviceMetrics)) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	metrics := mc.getOrCreateDeviceMetrics(deviceID)
	update(metrics)

	// Recalculate HealthScore
	metrics.HealthScore = mc.calculateDeviceHealthScore(metrics)

	metrics.Timestamp = time.Now()
}

// calculateDeviceHealthScore 计算设备健康评分
func (mc *MetricsCollector) calculateDeviceHealthScore(m *DeviceMetrics) int {
	score := 100

	// 连续失败扣分 (每次扣10分，最多30分)
	if m.ConsecutiveFailures > 0 {
		deduction := m.ConsecutiveFailures * 10
		if deduction > 30 {
			deduction = 30
		}
		score -= deduction
	}

	// 点位成功率扣分 (权重40%)
	// 如果成功率 < 100%，则扣分
	if m.PointSuccessRate < 1.0 {
		score -= int((1.0 - m.PointSuccessRate) * 40)
	}

	// 异常点位扣分 (每个异常扣5分，最多20分)
	if m.AbnormalPoints > 0 {
		deduction := m.AbnormalPoints * 5
		if deduction > 20 {
			deduction = 20
		}
		score -= deduction
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// cleanupHistory 清理过期的历史记录
func (mc *MetricsCollector) cleanupHistory(channelID string) {
	cutoff := time.Now().Add(-mc.windowSize)
	records := mc.history[channelID]

	var valid []RequestRecord
	for _, r := range records {
		if r.Timestamp.After(cutoff) {
			valid = append(valid, r)
		}
	}

	mc.history[channelID] = valid
}

// updateChannelMetrics 更新通道指标
func (mc *MetricsCollector) updateChannelMetrics(channelID string) {
	metrics := mc.getOrCreateChannelMetrics(channelID)
	records := mc.history[channelID]
	cycles := mc.cycleHistory[channelID]

	if len(records) == 0 && len(cycles) == 0 {
		return
	}

	var successCount, timeoutCount, crcCount, exceptionCount int64
	var totalDuration time.Duration
	var minRtt, maxRtt time.Duration

	for i, r := range records {
		if r.Success {
			successCount++
		}

		switch r.ErrorType {
		case "timeout":
			timeoutCount++
		case "crc":
			crcCount++
		case "exception":
			exceptionCount++
		}

		if r.Duration > 0 {
			totalDuration += r.Duration
			if i == 0 || r.Duration < minRtt {
				minRtt = r.Duration
			}
			if r.Duration > maxRtt {
				maxRtt = r.Duration
			}
		}
	}

	// 计算请求级统计
	totalRequests := int64(len(records))
	metrics.TotalRequests = totalRequests
	metrics.SuccessCount = successCount
	metrics.FailureCount = totalRequests - successCount
	metrics.TimeoutCount = timeoutCount
	metrics.CrcError = crcCount
	metrics.ExceptionCode = exceptionCount

	// 优先使用周期成功率作为展示成功率
	if len(cycles) > 0 {
		var cycleSuccess int64
		for _, c := range cycles {
			if c.Success {
				cycleSuccess++
			}
		}
		metrics.SuccessRate = float64(cycleSuccess) / float64(len(cycles))
	} else if totalRequests > 0 {
		metrics.SuccessRate = float64(successCount) / float64(totalRequests)
	}

	if totalRequests > 0 {
		metrics.CrcErrorRate = float64(crcCount) / float64(totalRequests)
		metrics.PacketLoss = float64(timeoutCount+crcCount) / float64(totalRequests)
	}

	if successCount > 0 {
		metrics.AvgRtt = float64(totalDuration/time.Millisecond) / float64(successCount)
	}
	metrics.MinRtt = float64(minRtt / time.Millisecond)
	metrics.MaxRtt = float64(maxRtt / time.Millisecond)

	// 计算质量评分
	metrics.QualityScore = mc.calculateQualityScore(metrics)

	// 更新趋势数据
	metrics.updateTrend()

	metrics.Timestamp = time.Now()
}

// CalculateQualityScore computes channel quality score including SLA penalties.
func CalculateQualityScore(m *ChannelMetrics) int {
	mc := &MetricsCollector{}
	return mc.calculateQualityScore(m)
}

// calculateQualityScore 计算质量评分
func (mc *MetricsCollector) calculateQualityScore(m *ChannelMetrics) int {
	score := 100

	// 成功率扣分 (权重40%)
	score -= int((1.0 - m.SuccessRate) * 40)

	// CRC错误率扣分 (权重20%)
	score -= int(m.CrcErrorRate * 20)

	// 超时率扣分 (权重20%)
	timeoutRate := float64(m.TimeoutCount) / float64(max(m.TotalRequests, 1))
	score -= int(timeoutRate * 20)

	// RTT扣分 (RTT > 100ms开始扣分)
	if m.AvgRtt > 100 {
		score -= int(min(10, (m.AvgRtt-100)/50))
	}

	// 调度 lag P95 扣分（>100ms 起扣，最多 15 分）
	if m.ScanLagP95Ms > 100 {
		score -= int(min(15, (m.ScanLagP95Ms-100)/20))
	}

	// 断路器 Open 设备扣分（每设备 10 分，最多 20 分）
	if m.CircuitBreakerOpen > 0 {
		penalty := m.CircuitBreakerOpen * 10
		if penalty > 20 {
			penalty = 20
		}
		score -= penalty
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return score
}

// RecordError 记录错误
func (mc *MetricsCollector) RecordError(channelID string, errType, code, message string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	metrics := mc.getOrCreateChannelMetrics(channelID)

	errorRecord := ErrorRecord{
		Time:    time.Now(),
		Type:    errType,
		Code:    code,
		Message: message,
	}

	// 添加到最近错误 (保持最近10条)
	metrics.RecentErrors = append([]ErrorRecord{errorRecord}, metrics.RecentErrors...)
	if len(metrics.RecentErrors) > 10 {
		metrics.RecentErrors = metrics.RecentErrors[:10]
	}
}

// RecordPointDebug 保存点位调试信息（原始字节 + 解析后值）。
// 热路径按 1% 采样写入，RawValue 截断至 pointDebugMaxRawBytes，map 有上限。
func (mc *MetricsCollector) RecordPointDebug(channelID, pointID string, raw []byte, parsed any, quality string) {
	if mc == nil || pointID == "" {
		return
	}
	if pointDebugCounter.Add(1)%pointDebugSampleRate != 0 {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	pm := &PointMetrics{
		PointID:        pointID,
		LastUpdateTime: time.Now(),
		Quality:        quality,
		ParsedValue:    parsed,
		UpdateCount:    1,
	}
	if n := len(raw); n > 0 {
		if n > pointDebugMaxRawBytes {
			n = pointDebugMaxRawBytes
		}
		pm.RawValue = append([]byte(nil), raw[:n]...)
	}

	if existing, ok := mc.pointMetrics[pointID]; ok {
		pm.UpdateCount = existing.UpdateCount + 1
		pm.ErrorCount = existing.ErrorCount
		pm.LastError = existing.LastError
		pm.LastErrorTime = existing.LastErrorTime
	}

	mc.pointMetrics[pointID] = pm
	mc.evictPointMetricsIfNeeded()
}

func (mc *MetricsCollector) evictPointMetricsIfNeeded() {
	for len(mc.pointMetrics) > pointMetricsMaxEntries {
		var oldestID string
		var oldestTime time.Time
		for id, pm := range mc.pointMetrics {
			if oldestID == "" || pm.LastUpdateTime.Before(oldestTime) {
				oldestID = id
				oldestTime = pm.LastUpdateTime
			}
		}
		if oldestID == "" {
			return
		}
		delete(mc.pointMetrics, oldestID)
	}
}

// GetPointMetrics 返回点位调试信息副本
func (mc *MetricsCollector) GetPointMetrics(pointID string) *PointMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if mc == nil {
		return &PointMetrics{PointID: pointID}
	}

	if pm, ok := mc.pointMetrics[pointID]; ok {
		copy := *pm
		return &copy
	}
	return &PointMetrics{PointID: pointID}
}

// updateTrend 更新趋势数据
func (m *ChannelMetrics) updateTrend() {
	// 每小时一个点，保留最近12个点
	now := time.Now()
	point := TrendPoint{
		Time: now,
		Rate: m.SuccessRate,
	}

	m.Trend = append(m.Trend, point)

	// 只保留最近12个点
	if len(m.Trend) > 12 {
		m.Trend = m.Trend[len(m.Trend)-12:]
	}
}

// getOrCreateChannelMetrics 获取或创建通道指标
func (mc *MetricsCollector) getOrCreateChannelMetrics(channelID string) *ChannelMetrics {
	if metrics, ok := mc.channelMetrics[channelID]; ok {
		return metrics
	}

	metrics := &ChannelMetrics{
		Timestamp: time.Now(),
	}
	mc.channelMetrics[channelID] = metrics
	return metrics
}

// getOrCreateDeviceMetrics 获取或创建设备指标
func (mc *MetricsCollector) getOrCreateDeviceMetrics(deviceID string) *DeviceMetrics {
	if metrics, ok := mc.deviceMetrics[deviceID]; ok {
		return metrics
	}

	metrics := &DeviceMetrics{
		Timestamp: time.Now(),
	}
	mc.deviceMetrics[deviceID] = metrics
	return metrics
}

// min 返回最小值
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// max 返回最大值
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

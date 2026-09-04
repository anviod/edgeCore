package opcua

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/pkg/dataformat"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"go.uber.org/zap"
)

const (
	StateDisconnected driver.ConnState = driver.StateDisconnected
	StateConnecting   driver.ConnState = driver.StateConnecting
	StateConnected    driver.ConnState = driver.StateConnected
	StateRetrying     driver.ConnState = driver.StateRetrying
	StateDead         driver.ConnState = driver.StateDead
)

func init() {
	driver.RegisterDriver("opc-ua", NewOpcUaDriver)
}

type OpcUaDriver struct {
	mu                   sync.RWMutex
	config               model.DriverConfig
	clients              map[string]*ClientWrapper // Key: Endpoint URL
	activeClient         *ClientWrapper
	useDataformatDecoder bool

	// Connection metrics
	connectionStartTime time.Time
	reconnectCount      int64
	lastDisconnectTime  time.Time

	// Request metrics
	totalRequests int64
	successCount  int64
	failureCount  int64

	// RTT metrics (direct Read RPC only)
	rttMu    sync.Mutex
	rttSumMs int64
	rttCount int64
	minRttMs int64
	maxRttMs int64

	maxNodesPerRead int // OPC UA 批量读上限，自适应调整
}

type DeviceSubscription struct {
	mu         sync.RWMutex
	Sub        *opcua.Subscription
	Cache      map[string]model.Value
	PointIDs   []string
	Points     map[string]model.Point
	HandleMap  map[uint32]string
	NextHandle uint32
	NotifyCh   chan *opcua.PublishNotificationData
	Ctx        context.Context
	Cancel     context.CancelFunc
	lastUpdate time.Time
}

type ClientWrapper struct {
	Client        *opcua.Client
	Endpoint      string
	Connected     bool
	Config        map[string]any
	mu            sync.Mutex
	Subscriptions map[string]*DeviceSubscription // DeviceID -> Subscription

	connMgr *driver.ConnectionManager

	lastActivityTime atomic.Value
	collectFailCount atomic.Int32
	maxFailCount     int32
	collectCycle     time.Duration

	onReconnectNeeded func()
}

func NewOpcUaDriver() driver.Driver {
	return &OpcUaDriver{
		clients:         make(map[string]*ClientWrapper),
		maxNodesPerRead: 100,
	}
}

func (d *OpcUaDriver) Init(cfg model.DriverConfig) error {
	d.config = cfg
	return nil
}

func (d *OpcUaDriver) resolveEndpointInConfig(config map[string]any) (string, error) {
	if config == nil {
		config = map[string]any{}
	}
	endpoint := model.ResolveOpcUaEndpoint(d.config.Config, config)
	if endpoint == "" {
		return "", fmt.Errorf("endpoint is required (configure channel url or device endpoint)")
	}
	config["endpoint"] = endpoint
	return endpoint, nil
}

func (d *OpcUaDriver) Connect(ctx context.Context) error {
	d.connectionStartTime = time.Now()
	if mc := model.GetGlobalMetricsCollector(); mc != nil && d.config.ChannelID != "" {
		mc.RecordConnectionStart(d.config.ChannelID)
	}
	return nil
}

func (d *OpcUaDriver) Disconnect() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastDisconnectTime = time.Now()

	for _, c := range d.clients {
		if c.Client != nil {
			c.Client.Close(context.Background())
		}
	}
	d.clients = make(map[string]*ClientWrapper)
	return nil
}

func (d *OpcUaDriver) Health() driver.HealthStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.activeClient != nil && d.activeClient.Connected {
		return driver.HealthStatusGood
	}
	return driver.HealthStatusUnknown
}

func (d *OpcUaDriver) SetSlaveID(slaveID uint8) error {
	return nil
}

func (d *OpcUaDriver) SetDeviceConfig(config map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if v, ok := config["use_dataformat_decoder"]; ok {
		switch val := v.(type) {
		case bool:
			d.useDataformatDecoder = val
		case string:
			if val == "true" || val == "1" {
				d.useDataformatDecoder = true
			} else {
				d.useDataformatDecoder = false
			}
		case float64:
			d.useDataformatDecoder = val != 0
		}
	}

	endpoint, err := d.resolveEndpointInConfig(config)
	if err != nil {
		return err
	}

	// Check if client exists
	if wrapper, exists := d.clients[endpoint]; exists {
		d.activeClient = wrapper
		// Check connection state
		if wrapper.Client.State() == opcua.Closed {
			wrapper.Connected = false
			d.scheduleReconnect(wrapper)
		}
		return nil
	}

	wrapper := &ClientWrapper{
		Endpoint:      endpoint,
		Config:        config,
		Subscriptions: make(map[string]*DeviceSubscription),
		connMgr:       driver.NewConnectionManager("opcua"),
		maxFailCount:  5,
		collectCycle:  10 * time.Second,
	}
	wrapper.lastActivityTime.Store(time.Time{})

	if v, ok := config["max_fail_count"].(float64); ok {
		wrapper.maxFailCount = int32(v)
	}
	if v, ok := config["collect_cycle"].(float64); ok {
		wrapper.collectCycle = time.Duration(v) * time.Millisecond
	}

	opts, err := d.buildClientOptions(config)
	if err != nil {
		return err
	}

	c, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return fmt.Errorf("failed to create opcua client: %v", err)
	}
	wrapper.Client = c

	wrapper.connMgr.SetState(StateConnecting)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		zap.L().Warn("[OPC UA] Failed to connect", zap.String("endpoint", endpoint), zap.Error(err))
		wrapper.Connected = false
		wrapper.connMgr.RecordFailure()
	} else {
		wrapper.Connected = true
		wrapper.connMgr.RecordSuccess()
	}

	wrapper.onReconnectNeeded = func() { d.scheduleReconnect(wrapper) }

	d.clients[endpoint] = wrapper
	d.activeClient = wrapper
	return nil
}

func (d *OpcUaDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 计算连接时长
	connSec := int64(0)
	if !d.connectionStartTime.IsZero() && d.activeClient != nil && d.activeClient.Connected {
		connSec = int64(time.Since(d.connectionStartTime).Seconds())
	}

	// 获取本机地址
	local := ""
	if d.activeClient != nil {
		endpoint := d.activeClient.Endpoint
		if endpoint != "" && strings.Contains(endpoint, ":") {
			// 尝试解析endpoint中的主机部分，获取连接到该endpoint的本机地址
			hostPort := endpoint
			if strings.HasPrefix(hostPort, "opc.tcp://") {
				hostPort = strings.TrimPrefix(hostPort, "opc.tcp://")
			}

			// 提取IP部分（去掉路径）
			if slashIdx := strings.Index(hostPort, "/"); slashIdx > 0 {
				hostPort = hostPort[:slashIdx]
			}

			// 尝试通过UDP获取本机地址
			udpConn, err := net.DialTimeout("udp", hostPort, 1*time.Second)
			if err == nil {
				if localIP, _, err := net.SplitHostPort(udpConn.LocalAddr().String()); err == nil {
					local = localIP + ":0" // OPC-UA客户端使用动态端口
				}
				udpConn.Close()
			}
		}
	}

	// 如果仍然没有本机地址，尝试从配置获取endpoint
	if local == "" && d.config.Config != nil {
		if endpoint, ok := d.config.Config["endpoint"].(string); ok && endpoint != "" {
			if strings.Contains(endpoint, ":") {
				hostPort := endpoint
				if strings.HasPrefix(hostPort, "opc.tcp://") {
					hostPort = strings.TrimPrefix(hostPort, "opc.tcp://")
				}

				// 提取IP部分（去掉路径）
				if slashIdx := strings.Index(hostPort, "/"); slashIdx > 0 {
					hostPort = hostPort[:slashIdx]
				}

				udpConn, err := net.DialTimeout("udp", hostPort, 1*time.Second)
				if err == nil {
					if localIP, _, err := net.SplitHostPort(udpConn.LocalAddr().String()); err == nil {
						local = localIP + ":0"
					}
					udpConn.Close()
				}
			}
		}
	}

	// 如果还是获取不到，使用默认值
	if local == "" {
		local = "127.0.0.1:0"
	}

	// 获取远程地址
	remote := ""
	if d.activeClient != nil && d.activeClient.Client != nil {
		// 尝试从endpoint URL中提取地址信息
		endpoint := d.activeClient.Endpoint
		if strings.HasPrefix(endpoint, "opc.tcp://") {
			addr := strings.TrimPrefix(endpoint, "opc.tcp://")
			remote = addr
		}
	} else if d.activeClient != nil {
		// 如果客户端存在但未连接，从endpoint获取地址
		endpoint := d.activeClient.Endpoint
		if strings.HasPrefix(endpoint, "opc.tcp://") {
			addr := strings.TrimPrefix(endpoint, "opc.tcp://")
			remote = addr
		}
	} else if d.config.Config != nil {
		// 从配置中获取endpoint
		if endpoint, ok := d.config.Config["endpoint"].(string); ok && endpoint != "" {
			if strings.HasPrefix(endpoint, "opc.tcp://") {
				addr := strings.TrimPrefix(endpoint, "opc.tcp://")
				remote = addr
			}
		}
	}

	return connSec, d.reconnectCount, local, remote, d.lastDisconnectTime
}

// GetMetrics 返回OPC-UA驱动的详细指标
func (d *OpcUaDriver) GetMetrics() model.ChannelMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 获取基础连接指标
	connSec, reconCount, localAddr, remoteAddr, lastDisc := d.GetConnectionMetrics()

	// 使用真实的请求统计数据
	totalRequests := d.totalRequests
	successCount := d.successCount
	failureCount := d.failureCount

	// 计算成功率
	successRate := 0.0
	if totalRequests > 0 {
		successRate = float64(successCount) / float64(totalRequests)
	}

	avgRtt, minRtt, maxRtt := d.rttSnapshot()

	// 构建指标
	metrics := model.ChannelMetrics{
		QualityScore:       d.calculateQualityScore(),
		Protocol:           "OPC-UA",
		SuccessRate:        successRate,
		TimeoutCount:       0, // OPC-UA有自己的超时处理
		CrcError:           0, // OPC-UA使用TCP，不适用CRC
		CrcErrorRate:       0.0,
		RetryRate:          0.0, // 可以后续添加重试统计
		ExceptionCode:      0,
		AvgRtt:             avgRtt,
		MaxRtt:             maxRtt,
		MinRtt:             minRtt,
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

	return metrics
}

// calculateQualityScore 计算OPC-UA质量评分
func (d *OpcUaDriver) calculateQualityScore() int {
	if d.activeClient == nil || !d.activeClient.Connected {
		return 0 // 未连接
	}

	// 基础分数70分
	score := 70

	// 根据重连次数降低分数
	if d.reconnectCount > 10 {
		score -= 20
	} else if d.reconnectCount > 5 {
		score -= 10
	} else if d.reconnectCount > 0 {
		score -= 5
	}

	// 根据成功率调整分数
	total := d.totalRequests
	success := d.successCount
	if total > 0 {
		rate := float64(success) / float64(total)
		if rate > 0.95 {
			score += 20
		} else if rate > 0.90 {
			score += 10
		} else if rate < 0.80 {
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

func (d *OpcUaDriver) avgRttMs() float64 {
	avg, _, _ := d.rttSnapshot()
	return avg
}

func (d *OpcUaDriver) rttSnapshot() (avg, min, max float64) {
	d.rttMu.Lock()
	defer d.rttMu.Unlock()
	if d.rttCount > 0 {
		avg = float64(d.rttSumMs) / float64(d.rttCount)
	}
	return avg, float64(d.minRttMs), float64(d.maxRttMs)
}

func (d *OpcUaDriver) recordRtt(duration time.Duration) {
	ms := duration.Milliseconds()
	if ms <= 0 && duration > 0 {
		ms = 1
	}
	d.rttMu.Lock()
	defer d.rttMu.Unlock()
	d.rttSumMs += ms
	d.rttCount++
	if d.minRttMs == 0 || ms < d.minRttMs {
		d.minRttMs = ms
	}
	if ms > d.maxRttMs {
		d.maxRttMs = ms
	}
}

func (d *OpcUaDriver) recordReadOutcome(start time.Time, success bool, errorType string) {
	d.totalRequests++
	if success {
		d.successCount++
	} else {
		d.failureCount++
	}
	if mc := model.GetGlobalMetricsCollector(); mc != nil && d.config.ChannelID != "" {
		mc.RecordRequest(d.config.ChannelID, success, time.Since(start), errorType)
	}
}

func (d *OpcUaDriver) recordReconnect() {
	d.reconnectCount++
	if mc := model.GetGlobalMetricsCollector(); mc != nil && d.config.ChannelID != "" {
		mc.RecordReconnect(d.config.ChannelID)
	}
}

// Scan implements Scanner interface for OPC-UA device discovery
func (d *OpcUaDriver) Scan(ctx context.Context, params map[string]any) (any, error) {
	// For OPC-UA, we typically scan a specific endpoint
	// This implementation returns a list of OPC-UA endpoints that can be connected to

	// Check if endpoint is provided
	endpoint, err := d.resolveEndpointInConfig(params)
	if err != nil {
		// 无通道/请求 endpoint 时返回占位列表（兼容旧行为）
		defaultEndpoints := []map[string]any{
			{
				"device_id":   "opcua-default",
				"endpoint":    "opc.tcp://localhost:4840",
				"name":        "Local OPC UA Server",
				"description": "Default OPC UA Server on localhost",
			},
			{
				"device_id":   "opcua-simulation",
				"endpoint":    "opc.tcp://localhost:5050/test",
				"name":        "Simulation OPC UA Server",
				"description": "Simulation OPC UA Server",
			},
		}
		return defaultEndpoints, nil
	}
	_ = endpoint

	// If endpoint is provided, test connection and return device info
	opts, err := d.buildClientOptions(params)
	if err != nil {
		return nil, err
	}

	c, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	defer c.Close(context.Background())

	if err := c.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to endpoint: %v", err)
	}

	// Test connection by getting endpoints
	_, err = c.GetEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get server info: %v", err)
	}

	// Return device info (endpoint is the connection target; device ID is assigned on add)
	device := map[string]any{
		"endpoint":    endpoint,
		"name":        "OPC UA Server",
		"description": "OPC UA Server at " + endpoint,
		"vendor_name": "Unknown",
		"model_name":  "OPC UA Server",
		"version":     "Unknown",
	}

	return []map[string]any{device}, nil
}

func (d *OpcUaDriver) scheduleReconnect(w *ClientWrapper) {
	const connectTimeout = 10 * time.Second
	const maxRetries = 64
	timeout := connectTimeout * time.Duration(maxRetries)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	w.connMgr.ScheduleReconnect(context.Background(), timeout, func(ctx context.Context) error {
		d.mu.Lock()
		defer d.mu.Unlock()

		if w.Connected {
			return nil
		}
		if w.Client == nil {
			return fmt.Errorf("opcua client not initialized")
		}

		zap.L().Info("[OPC UA] Reconnecting", zap.String("endpoint", w.Endpoint))

		if err := w.Client.Connect(ctx); err != nil {
			return err
		}

		w.Connected = true
		d.recordReconnect()
		zap.L().Info("[OPC UA] Reconnected", zap.String("endpoint", w.Endpoint))
		return nil
	})
}

func (d *OpcUaDriver) buildClientOptions(config map[string]any) ([]opcua.Option, error) {
	opts := []opcua.Option{
		opcua.RequestTimeout(10 * time.Second),
		// opcua.SessionTimeout(30 * time.Minute),
	}

	/*
		// Security Policy
		policy, _ := config["security_policy"].(string)
		if policy == "" {
			policy = "None"
		}
		opts = append(opts, opcua.SecurityPolicy(policy))

		// Security Mode
		modeStr, _ := config["security_mode"].(string)
		mode := ua.MessageSecurityModeNone
		switch modeStr {
		case "Sign":
			mode = ua.MessageSecurityModeSign
		case "SignAndEncrypt":
			mode = ua.MessageSecurityModeSignAndEncrypt
		}
		opts = append(opts, opcua.SecurityMode(mode))

		// Auth Method
		authMethod, _ := config["auth_method"].(string)
		switch authMethod {
		case "UserName":
			user, _ := config["username"].(string)
			pass, _ := config["password"].(string)
			opts = append(opts, opcua.AuthUsername(user, pass))
		case "Certificate":
			certFile, _ := config["certificate_file"].(string)
			keyFile, _ := config["private_key_file"].(string)
			certBytes, err := os.ReadFile(certFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read cert file: %v", err)
			}
			opts = append(opts, opcua.AuthCertificate(certBytes))
			opts = append(opts, opcua.PrivateKeyFile(keyFile))
		default:
			opts = append(opts, opcua.AuthAnonymous())
		}
	*/

	return opts, nil
}

func (d *OpcUaDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	startTime := time.Now()

	if len(points) == 0 {
		return nil, nil
	}

	d.mu.Lock()
	client := d.activeClient
	d.mu.Unlock()

	if client == nil {
		d.recordReadOutcome(startTime, false, "network")
		return nil, fmt.Errorf("no active client")
	}
	if !client.Connected {
		err := client.connMgr.EnsureConnected(ctx, func(ctx context.Context) error {
			d.mu.Lock()
			defer d.mu.Unlock()

			if client.Connected {
				return nil
			}
			if client.Client == nil {
				return fmt.Errorf("opcua client not initialized")
			}
			if err := client.Client.Connect(ctx); err != nil {
				return err
			}
			client.Connected = true
			d.recordReconnect()
			return nil
		})
		if err != nil {
			d.recordReadOutcome(startTime, false, "network")
			return nil, fmt.Errorf("client not connected: %v", err)
		}
	}

	deviceID := points[0].DeviceID

	sub := d.ensureSubscription(ctx, client, deviceID, points)

	if sub != nil {
		sub.mu.RLock()
		defer sub.mu.RUnlock()

		result := make(map[string]model.Value)
		missing := false
		linkDown := !client.Connected
		for _, p := range points {
			v, ok := sub.Cache[p.ID]
			if ok && subscriptionCacheReady(v) {
				if linkDown {
					v.Quality = "Bad"
				}
				// Preserve cached TS so stale subscription values are not masked as fresh.
				result[p.ID] = v
				zap.L().Debug("[OPC UA] Read (Cache)", zap.String("point", p.ID), zap.Any("value", v.Value), zap.String("quality", v.Quality))
			} else {
				missing = true
				result[p.ID] = model.Value{
					PointID: p.ID,
					Quality: "Bad",
					Value:   0,
					TS:      time.Now(),
				}
			}
		}

		if !missing {
			d.recordReadOutcome(startTime, true, "")
			client.RecordSuccess()
			return result, nil
		}
	} else {
		zap.L().Debug("[OPC UA] No subscription, using direct read")
	}

	result, err := d.readDirect(ctx, client, points)
	if err != nil {
		d.recordReadOutcome(startTime, false, classifyOpcUaReadError(err))
		return result, err
	}
	stampCollectionTime(result)
	d.recordReadOutcome(startTime, true, "")
	client.RecordSuccess()
	return result, nil
}

// subscriptionCacheReady reports whether a cached subscription value is safe to
// return without a direct read fallback. Good status with a nil value can
// appear on initial monitored-item notifications before the server payload
// is populated (common when points are merged into an existing subscription).
func subscriptionCacheReady(v model.Value) bool {
	if v.Quality == "Bad" {
		return true
	}
	return v.Quality == "Good" && v.Value != nil
}

func classifyOpcUaReadError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "timeout") || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "network"
}

// stampCollectionTime sets TS to the local poll time so UI shows last collection, not last server-side change.
func stampCollectionTime(results map[string]model.Value) {
	now := time.Now()
	for id, v := range results {
		v.TS = now
		results[id] = v
	}
}

func sortedPointIDs(points []model.Point) []string {
	ids := make([]string, len(points))
	for i, p := range points {
		ids[i] = p.ID
	}
	sort.Strings(ids)
	return ids
}

func mergeSubscriptionPoints(existing map[string]model.Point, incoming []model.Point) (map[string]model.Point, []model.Point, bool) {
	merged := make(map[string]model.Point, len(existing)+len(incoming))
	for id, p := range existing {
		merged[id] = p
	}

	addressChanged := false
	var toAdd []model.Point
	for _, p := range incoming {
		if prev, ok := merged[p.ID]; ok {
			if prev.Address != p.Address {
				addressChanged = true
			}
			merged[p.ID] = p
			continue
		}
		merged[p.ID] = p
		toAdd = append(toAdd, p)
	}
	return merged, toAdd, addressChanged
}

func (d *OpcUaDriver) ResetDeviceCollection(deviceID string) {
	if deviceID == "" {
		return
	}

	d.mu.RLock()
	client := d.activeClient
	d.mu.RUnlock()
	if client == nil {
		return
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	d.cancelDeviceSubscription(client, deviceID)
}

func (d *OpcUaDriver) cancelDeviceSubscription(w *ClientWrapper, deviceID string) {
	sub, ok := w.Subscriptions[deviceID]
	if !ok || sub == nil {
		return
	}
	sub.Cancel()
	delete(w.Subscriptions, deviceID)
}

func (d *OpcUaDriver) ensureSubscription(ctx context.Context, w *ClientWrapper, deviceID string, points []model.Point) *DeviceSubscription {
	w.mu.Lock()
	defer w.mu.Unlock()

	sub, exists := w.Subscriptions[deviceID]
	if exists && sub != nil && sub.Ctx.Err() != nil {
		delete(w.Subscriptions, deviceID)
		exists = false
		sub = nil
	}

	if exists {
		merged, toAdd, addressChanged := mergeSubscriptionPoints(sub.Points, points)
		if addressChanged {
			d.cancelDeviceSubscription(w, deviceID)
			return d.createDeviceSubscription(ctx, w, deviceID, pointsFromMap(merged))
		}
		if len(toAdd) > 0 {
			d.addMonitoredItems(ctx, sub, toAdd)
			for _, p := range toAdd {
				sub.Points[p.ID] = p
			}
			sub.PointIDs = sortedPointIDsFromMap(sub.Points)
		}
		return sub
	}

	return d.createDeviceSubscription(ctx, w, deviceID, points)
}

func pointsFromMap(points map[string]model.Point) []model.Point {
	out := make([]model.Point, 0, len(points))
	for _, p := range points {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedPointIDsFromMap(points map[string]model.Point) []string {
	ids := make([]string, 0, len(points))
	for id := range points {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (d *OpcUaDriver) createDeviceSubscription(ctx context.Context, w *ClientWrapper, deviceID string, points []model.Point) *DeviceSubscription {
	if len(points) == 0 {
		return nil
	}

	notifyCh := make(chan *opcua.PublishNotificationData)
	subCtx, cancel := context.WithCancel(context.Background())

	opcuaSub, err := w.Client.Subscribe(ctx, &opcua.SubscriptionParameters{
		Interval: 1000 * time.Millisecond, // 1s interval
	}, notifyCh)
	if err != nil {
		zap.L().Error("[OPC UA] Failed to create subscription", zap.String("device_id", deviceID), zap.Error(err))
		cancel()
		return nil
	}

	newSub := &DeviceSubscription{
		Sub:        opcuaSub,
		Cache:      make(map[string]model.Value),
		PointIDs:   sortedPointIDs(points),
		Points:     make(map[string]model.Point, len(points)),
		HandleMap:  make(map[uint32]string),
		NextHandle: 1,
		NotifyCh:   notifyCh,
		Ctx:        subCtx,
		Cancel:     cancel,
	}
	for _, p := range points {
		newSub.Points[p.ID] = p
	}

	d.addMonitoredItems(ctx, newSub, points)
	go d.subscriptionLoop(newSub)
	w.Subscriptions[deviceID] = newSub
	return newSub
}

func (d *OpcUaDriver) addMonitoredItems(ctx context.Context, sub *DeviceSubscription, points []model.Point) {
	if sub == nil || sub.Sub == nil || len(points) == 0 {
		return
	}

	requests := make([]*ua.MonitoredItemCreateRequest, 0, len(points))
	monitoredPoints := make([]model.Point, 0, len(points))
	for _, p := range points {
		id, err := ua.ParseNodeID(p.Address)
		if err != nil {
			zap.L().Error("[OPC UA] Invalid node id", zap.String("address", p.Address), zap.Error(err))
			continue
		}

		handle := sub.NextHandle
		sub.NextHandle++
		sub.HandleMap[handle] = p.ID
		monitoredPoints = append(monitoredPoints, p)
		requests = append(requests, opcua.NewMonitoredItemCreateRequestWithDefaults(
			id,
			ua.AttributeIDValue,
			handle,
		))
	}

	if len(requests) == 0 {
		return
	}

	resp, err := sub.Sub.Monitor(ctx, ua.TimestampsToReturnBoth, requests...)
	if err != nil {
		zap.L().Error("[OPC UA] Monitor failed", zap.Error(err))
		return
	}

	for i, res := range resp.Results {
		if res.StatusCode != ua.StatusOK {
			zap.L().Error("[OPC UA] Monitor item failed",
				zap.String("address", monitoredPoints[i].Address),
				zap.Any("status", res.StatusCode))
		}
	}
}

func (d *OpcUaDriver) subscriptionLoop(sub *DeviceSubscription) {
	for {
		select {
		case <-sub.Ctx.Done():
			return
		case res, ok := <-sub.NotifyCh:
			if !ok {
				return
			}
			if res.Error != nil {
				zap.L().Error("[OPC UA] Subscription error", zap.Error(res.Error))
				d.mu.RLock()
				client := d.activeClient
				d.mu.RUnlock()
				if client != nil {
					client.RecordFailure(res.Error)
				}
				continue
			}

			switch x := res.Value.(type) {
			case *ua.DataChangeNotification:
				sub.mu.Lock()
				for _, item := range x.MonitoredItems {
					pointID, ok := sub.HandleMap[item.ClientHandle]
					if !ok {
						continue
					}

					val := model.Value{
						PointID: pointID,
						TS:      time.Now(),
					}

					if item.Value != nil {
						if item.Value.Status == ua.StatusOK {
							raw := item.Value.Value.Value()
							if d.useDataformatDecoder {
								if p, ok := sub.Points[pointID]; ok {
									if formatted, err := dataformat.FormatScalar(p, "ABCD", raw); err == nil {
										raw = formatted
									}
								}
							}
							if raw == nil {
								zap.L().Debug("[OPC UA] Subscription update skipped nil value",
									zap.String("point_id", pointID))
								continue
							}
							val.Quality = "Good"
							val.Value = raw
							if !item.Value.SourceTimestamp.IsZero() {
								val.TS = item.Value.SourceTimestamp
							}
						} else {
							val.Quality = "Bad"
							zap.L().Warn("[OPC UA] Subscription update bad status", zap.String("point_id", pointID), zap.Any("status", item.Value.Status))
						}
					}

					sub.Cache[pointID] = val
				}
				sub.lastUpdate = time.Now()
				sub.mu.Unlock()

				d.mu.RLock()
				client := d.activeClient
				d.mu.RUnlock()
				if client != nil {
					client.RecordSuccess()
				}
			}
		}
	}
}

func (d *OpcUaDriver) readDirect(ctx context.Context, client *ClientWrapper, points []model.Point) (map[string]model.Value, error) {
	if len(points) == 0 {
		return make(map[string]model.Value), nil
	}

	maxNodes := d.maxNodesPerRead
	if maxNodes <= 0 {
		maxNodes = 100
	}

	result := make(map[string]model.Value, len(points))
	for i := 0; i < len(points); i += maxNodes {
		end := i + maxNodes
		if end > len(points) {
			end = len(points)
		}
		batch := points[i:end]
		batchResult, err := d.readDirectBatch(ctx, client, batch)
		if err != nil {
			if maxNodes > 16 {
				d.maxNodesPerRead = maxNodes / 2
			}
			return result, err
		}
		for k, v := range batchResult {
			result[k] = v
		}
		if maxNodes < 200 {
			d.maxNodesPerRead = maxNodes + 10
		}
	}

	return result, nil
}

func (d *OpcUaDriver) readDirectBatch(ctx context.Context, client *ClientWrapper, points []model.Point) (map[string]model.Value, error) {
	req := &ua.ReadRequest{
		MaxAge:             2000,
		TimestampsToReturn: ua.TimestampsToReturnBoth,
		NodesToRead:        make([]*ua.ReadValueID, len(points)),
	}

	for i, p := range points {
		id, err := ua.ParseNodeID(p.Address)
		if err != nil {
			return nil, fmt.Errorf("invalid node id %s: %v", p.Address, err)
		}
		req.NodesToRead[i] = &ua.ReadValueID{
			NodeID:      id,
			AttributeID: ua.AttributeIDValue,
		}
	}

	readStart := time.Now()
	resp, err := client.Client.Read(ctx, req)
	readDuration := time.Since(readStart)
	if err != nil {
		return nil, err
	}
	if resp.Results == nil || len(resp.Results) != len(points) {
		return nil, fmt.Errorf("invalid read response")
	}

	d.recordRtt(readDuration)

	result := make(map[string]model.Value)
	now := time.Now()

	for i, res := range resp.Results {
		p := points[i]
		val := model.Value{
			PointID: p.ID,
			TS:      now,
		}

		if res.Status != ua.StatusOK {
			val.Quality = "Bad"
			zap.L().Warn("[OPC UA] Read failed", zap.String("point_id", p.ID), zap.Any("status", res.Status))
		} else {
			val.Quality = "Good"
			raw := res.Value.Value()
			if d.useDataformatDecoder {
				if formatted, err := dataformat.FormatScalar(p, "ABCD", raw); err == nil {
					raw = formatted
				}
			}
			val.Value = raw
		}
		result[p.ID] = val
	}

	return result, nil
}

// WritePoint writes a value to an OPC-UA node with full type conversion and error handling
func (d *OpcUaDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	d.mu.Lock()
	client := d.activeClient
	d.mu.Unlock()

	if client == nil {
		return fmt.Errorf("no active OPC-UA client")
	}

	// Try to reconnect if not connected
	if !client.Connected {
		if err := client.Client.Connect(ctx); err != nil {
			return fmt.Errorf("OPC-UA client not connected: %v", err)
		}
		d.mu.Lock()
		client.Connected = true
		d.mu.Unlock()
	}

	// Parse and validate the node ID
	nodeID, err := ua.ParseNodeID(point.Address)
	if err != nil {
		return fmt.Errorf("invalid node ID %s: %w", point.Address, err)
	}

	// Handle namespace URI format (nsu=...) and string identifier conversion
	nodeID = d.normalizeNodeID(point.Address, nodeID)

	zap.L().Debug("[OPC UA] Write: node=%s address=%s dataType=%s",
		zap.Stringer("nodeID", nodeID), zap.String("address", point.Address), zap.String("dataType", point.DataType))

	// Determine the data type to use
	dataTypeToUse := point.DataType

	// Optionally read the node's actual data type from server
	serverDataType := d.getServerDataType(ctx, client.Client, nodeID)
	if serverDataType != "" {
		zap.L().Debug("[OPC UA] Write: server reports DataType",
			zap.String("dataType", serverDataType),
			zap.String("node", point.Address))
		// If we have a mismatch, log a warning but use the server's type for better compatibility
		if dataTypeToUse != "" && !strings.EqualFold(dataTypeToUse, serverDataType) {
			zap.L().Warn("[OPC UA] Write: model DataType mismatch, using server type",
				zap.String("point", point.ID),
				zap.String("modelType", dataTypeToUse),
				zap.String("serverType", serverDataType))
		}
		dataTypeToUse = serverDataType
	}

	// Parse and convert the value according to data type
	valToWrite, err := d.parseWriteValue(value, dataTypeToUse)
	if err != nil {
		return fmt.Errorf("value conversion failed for node %s: %w", point.Address, err)
	}

	// Create the OPC-UA Variant with the correct type
	variant := d.createWriteVariant(dataTypeToUse, valToWrite)
	if variant == nil {
		return fmt.Errorf("failed to create variant for node %s", point.Address)
	}

	// Create DataValue with timestamp
	dataValue := &ua.DataValue{
		Value:           variant,
		SourceTimestamp: time.Now(),
	}
	dataValue.UpdateMask() // Essential: set EncodingMask

	// Build the write request
	writeValue := &ua.WriteValue{
		NodeID:      nodeID,
		AttributeID: ua.AttributeIDValue,
		Value:       dataValue,
	}

	// Execute the write with retry on transient errors
	var resp *ua.WriteResponse
	for attempt := 0; attempt < 2; attempt++ {
		resp, err = client.Client.Write(ctx, &ua.WriteRequest{
			NodesToWrite: []*ua.WriteValue{writeValue},
		})
		if err == nil {
			break
		}

		// Check if it's a transient error that warrants reconnect
		if !isOPCUAConnError(err) {
			return fmt.Errorf("OPC-UA write request failed: %w", err)
		}

		zap.L().Warn("[OPC UA] Write: connection error, attempting reconnect",
			zap.String("point", point.ID), zap.Error(err))

		if reconnErr := client.Client.Close(context.Background()); reconnErr == nil {
			if reconnErr = client.Client.Connect(ctx); reconnErr != nil {
				return fmt.Errorf("OPC-UA reconnect failed: %w", reconnErr)
			}
		}
	}

	if err != nil {
		return fmt.Errorf("OPC-UA write request failed after retry: %w", err)
	}

	if len(resp.Results) == 0 {
		return fmt.Errorf("OPC-UA write response invalid (no results)")
	}

	if resp.Results[0] != ua.StatusOK {
		statusCode := resp.Results[0]
		errMsg := fmt.Errorf("write failed: %s (0x%X)", statusCode, uint32(statusCode))

		// Try alternative types on failure
		if d.tryAlternativeWriteTypes(ctx, client.Client, nodeID, value, dataTypeToUse) {
			zap.L().Info("[OPC UA] Write: succeeded on retry with alternative type",
				zap.String("point", point.ID), zap.Any("value", value))
			return nil
		}

		return errMsg
	}

	zap.L().Info("[OPC UA] Write success",
		zap.String("point_id", point.ID),
		zap.String("address", point.Address),
		zap.Any("value", valToWrite))

	return nil
}

// getServerDataType queries the OPC-UA server for the actual data type of a node
func (d *OpcUaDriver) getServerDataType(ctx context.Context, client *opcua.Client, nodeID *ua.NodeID) string {
	if client == nil {
		return ""
	}

	// Use a short timeout for metadata read
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	attrs, err := client.Node(nodeID).Attributes(readCtx, ua.AttributeIDDataType)
	if err != nil || len(attrs) == 0 || attrs[0].Status != ua.StatusOK {
		return ""
	}

	dtNodeID, ok := attrs[0].Value.Value().(*ua.NodeID)
	if !ok {
		return ""
	}

	return lookupDataType(dtNodeID)
}

// normalizeNodeID handles various NodeID formats and converts to canonical form
func (d *OpcUaDriver) normalizeNodeID(original string, nodeID *ua.NodeID) *ua.NodeID {
	if !strings.HasPrefix(original, "ns=") {
		return nil
	}
	// Handle ns=...;s=... format with string identifier
	if strings.HasPrefix(original, "ns=") && strings.Contains(original, ";s=") {
		parts := strings.SplitN(original, ";s=", 2)
		if len(parts) == 2 {
			nsPart := strings.TrimPrefix(parts[0], "ns=")
			sIdentifier := parts[1]

			// Try to convert numeric string identifier to numeric NodeID
			if numericID, err := strconv.ParseUint(sIdentifier, 10, 32); err == nil {
				numericNodeIDStr := "ns=" + nsPart + ";i=" + fmt.Sprintf("%d", numericID)
				if converted, err := ua.ParseNodeID(numericNodeIDStr); err == nil {
					zap.L().Debug("[OPC UA] Write: string ID converted to numeric",
						zap.String("original", original), zap.String("converted", numericNodeIDStr))
					return converted
				}
			}
		}
	}

	return nodeID
}

// parseWriteValue converts the input value to the appropriate Go type for OPC-UA
func (d *OpcUaDriver) parseWriteValue(value any, dataType string) (any, error) {
	dt := strings.ToLower(dataType)

	// Handle ByteString specially: decode base64 if input is string
	if dt == "bytestring" || dt == "i=15" || dt == "15" {
		switch v := value.(type) {
		case []byte:
			return v, nil
		case string:
			// Try base64 decode first
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && len(decoded) > 0 {
				return decoded, nil
			}
			// Fallback: treat as raw bytes
			return []byte(v), nil
		case map[string]any:
			encoding := strings.ToLower(fmt.Sprintf("%v", v["encoding"]))
			raw := fmt.Sprintf("%v", v["value"])
			switch encoding {
			case "hex":
				return decodeHexString(raw)
			case "base64":
				return base64.StdEncoding.DecodeString(raw)
			default:
				return parseByteStringValue(value)
			}
		default:
			return parseByteStringValue(value)
		}
	}

	// Handle DateTime
	if dt == "datetime" || dt == "i=13" || dt == "13" {
		switch v := value.(type) {
		case time.Time:
			return v, nil
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t, nil
			}
			// Try other common formats
			formats := []string{
				"2006-01-02T15:04:05Z",
				"2006-01-02 15:04:05",
				"2006-01-02",
			}
			for _, f := range formats {
				if t, err := time.Parse(f, v); err == nil {
					return t, nil
				}
			}
			return nil, fmt.Errorf("invalid datetime format: %s", v)
		case int64:
			return time.Unix(v, 0), nil
		case float64:
			return time.Unix(int64(v), 0), nil
		}
	}

	// Handle Guid
	if dt == "guid" || dt == "i=14" || dt == "14" {
		switch v := value.(type) {
		case string:
			g := parseGuid(v)
			if g != nil {
				return g, nil
			}
			return nil, fmt.Errorf("invalid GUID format: %s", v)
		case [16]byte:
			return ua.NewGUID(string(v[:])), nil
		default:
			// Try to parse from string representation
			if str, ok := value.(string); ok {
				g := parseGuid(str)
				if g != nil {
					return g, nil
				}
				return nil, fmt.Errorf("invalid GUID format: %s", str)
			}
			g := parseGuid(fmt.Sprintf("%v", value))
			if g != nil {
				return g, nil
			}
			return nil, fmt.Errorf("cannot convert %T to GUID", value)
		}
	}

	// Handle StatusCode
	if dt == "statuscode" || dt == "i=19" || dt == "19" {
		switch v := value.(type) {
		case uint32:
			return ua.StatusCode(v), nil
		case int:
			return ua.StatusCode(v), nil
		case int64:
			return ua.StatusCode(v), nil
		case float64:
			return ua.StatusCode(uint32(v)), nil
		case string:
			// Try to look up status code by name
			if code, ok := statusCodeFromName(v); ok {
				return code, nil
			}
			return ua.StatusCode(0), nil
		default:
			return ua.StatusCode(0), nil
		}
	}

	// Handle QualifiedName
	if dt == "qualifiedname" || dt == "i=20" || dt == "20" {
		switch v := value.(type) {
		case string:
			parts := strings.SplitN(v, ":", 2)
			if len(parts) == 2 {
				ns, _ := strconv.ParseUint(parts[0], 10, 16)
				return ua.QualifiedName{NamespaceIndex: uint16(ns), Name: parts[1]}, nil
			}
			return ua.QualifiedName{NamespaceIndex: 0, Name: v}, nil
		case map[string]any:
			ns := uint16(0)
			if n, ok := v["namespace"].(float64); ok {
				ns = uint16(n)
			}
			name := fmt.Sprintf("%v", v["name"])
			return ua.QualifiedName{NamespaceIndex: ns, Name: name}, nil
		default:
			return ua.QualifiedName{NamespaceIndex: 0, Name: fmt.Sprintf("%v", value)}, nil
		}
	}

	// Handle LocalizedText
	if dt == "localizedtext" || dt == "i=21" || dt == "21" {
		switch v := value.(type) {
		case string:
			return ua.LocalizedText{Text: v, Locale: "en"}, nil
		case map[string]any:
			text := fmt.Sprintf("%v", v["text"])
			locale := fmt.Sprintf("%v", v["locale"])
			if locale == "<nil>" || locale == "" {
				locale = "en"
			}
			return ua.LocalizedText{Text: text, Locale: locale}, nil
		default:
			return ua.LocalizedText{Text: fmt.Sprintf("%v", value), Locale: "en"}, nil
		}
	}

	// Handle NodeID (rarely written, but supported)
	if dt == "nodeid" || dt == "i=17" || dt == "17" {
		switch v := value.(type) {
		case string:
			if nodeID, err := ua.ParseNodeID(v); err == nil {
				return nodeID, nil
			}
			return nil, fmt.Errorf("invalid NodeID string: %s", v)
		case *ua.NodeID:
			return v, nil
		default:
			return nil, fmt.Errorf("cannot convert %T to NodeID", value)
		}
	}

	// Handle ExtensionObject - treat as byte array with encoding info
	if dt == "extensionobject" || dt == "i=22" || dt == "22" {
		switch v := value.(type) {
		case []byte:
			return v, nil
		case string:
			decoded, err := base64.StdEncoding.DecodeString(v)
			if err != nil {
				return []byte(v), nil
			}
			return decoded, nil
		default:
			return nil, fmt.Errorf("cannot convert %T to ExtensionObject", value)
		}
	}

	// Handle Byte (unsigned 8-bit)
	if dt == "byte" || dt == "uint8" || dt == "i=3" || dt == "3" {
		switch v := value.(type) {
		case uint8:
			return v, nil
		case int:
			return uint8(v), nil
		case int64:
			return uint8(v), nil
		case float64:
			return uint8(v), nil
		case string:
			if n, err := strconv.ParseUint(v, 10, 8); err == nil {
				return uint8(n), nil
			}
			return uint8(0), nil
		default:
			return uint8(0), nil
		}
	}

	// Use the existing castValue for other types
	return castValue(value, dataType)
}

// parseGuid parses a GUID string in standard format
func parseGuid(s string) *ua.GUID {
	s = strings.TrimSpace(s)
	if s == "" {
		return &ua.GUID{}
	}
	// Standard UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	if len(s) == 36 {
		s = strings.ReplaceAll(s, "-", "")
	}
	if g := ua.NewGUID(s); g != nil {
		return g
	}
	return &ua.GUID{}
}

// statusCodeFromName converts a status code name to its value
func statusCodeFromName(name string) (ua.StatusCode, bool) {
	// Common status codes
	codes := map[string]ua.StatusCode{
		"good":                   ua.StatusGood,
		"uncertain":              ua.StatusUncertain,
		"bad":                    ua.StatusBad,
		"badconnectionclosed":    ua.StatusBadConnectionClosed,
		"badnotconnected":        ua.StatusBadNotConnected,
		"badserverhalted":        ua.StatusBadServerHalted,
		"badnocommunication":     ua.StatusBadNoCommunication,
		"badoutofmemory":         ua.StatusBadOutOfMemory,
		"badresourceunavailable": ua.StatusBadResourceUnavailable,
		"badtimeout":             ua.StatusBadTimeout,
		"goodclampped":           ua.StatusGoodClamped,
		"goodlocaloverride":      ua.StatusGoodLocalOverride,
	}
	if code, ok := codes[strings.ToLower(name)]; ok {
		return code, true
	}
	return 0, false
}

// createWriteVariant creates an OPC-UA Variant with the correct type encoding
func (d *OpcUaDriver) createWriteVariant(dataType string, value any) *ua.Variant {
	if value == nil {
		return ua.MustVariant(nil)
	}

	dt := strings.ToLower(dataType)

	switch {
	// ===== 基础数值类型 =====
	case dt == "int32" || dt == "i=6" || dt == "6":
		return createInt32Variant(value)

	case dt == "int16" || dt == "i=4" || dt == "4":
		return createInt16Variant(value)

	case dt == "uint32" || dt == "i=7" || dt == "7":
		return createUint32Variant(value)

	case dt == "uint16" || dt == "i=5" || dt == "5":
		return createUint16Variant(value)

	case dt == "int64" || dt == "i=8" || dt == "8":
		return createInt64Variant(value)

	case dt == "uint64" || dt == "i=9" || dt == "9":
		return createUint64Variant(value)

	case dt == "float" || dt == "float32" || dt == "i=10" || dt == "10":
		return createFloat32Variant(value)

	case dt == "double" || dt == "i=11" || dt == "11":
		return createFloat64Variant(value)

	// ===== 布尔和字节类型 =====
	case dt == "boolean" || dt == "bool" || dt == "i=1" || dt == "1":
		return createBooleanVariant(value)

	case dt == "sbyte" || dt == "i=2" || dt == "2":
		return createSByteVariant(value)

	case dt == "byte" || dt == "uint8" || dt == "i=3" || dt == "3":
		return createByteVariant(value)

	// ===== 字符串类型 =====
	case dt == "string" || dt == "i=12" || dt == "12":
		return ua.MustVariant(fmt.Sprintf("%v", value))

	case dt == "xmlliteral" || dt == "i=16" || dt == "16":
		return ua.MustVariant(fmt.Sprintf("%v", value))

	// ===== 日期时间类型 =====
	case dt == "datetime" || dt == "i=13" || dt == "13":
		return createDateTimeVariant(value)

	// ===== GUID 类型 =====
	case dt == "guid" || dt == "i=14" || dt == "14":
		return createGuidVariant(value)

	// ===== ByteString 类型 =====
	case dt == "bytestring" || dt == "i=15" || dt == "15":
		return createByteStringVariant(value)

	// ===== NodeID 类型 =====
	case dt == "nodeid" || dt == "i=17" || dt == "17":
		return createNodeIDVariant(value)

	// ===== StatusCode 类型 =====
	case dt == "statuscode" || dt == "i=19" || dt == "19":
		return createStatusCodeVariant(value)

	// ===== QualifiedName 类型 =====
	case dt == "qualifiedname" || dt == "i=20" || dt == "20":
		return createQualifiedNameVariant(value)

	// ===== LocalizedText 类型 =====
	case dt == "localizedtext" || dt == "i=21" || dt == "21":
		return createLocalizedTextVariant(value)

	// ===== ExtensionObject 类型 =====
	case dt == "extensionobject" || dt == "i=22" || dt == "22":
		return createExtensionObjectVariant(value)

	// ===== 数组类型支持 =====
	case strings.HasPrefix(dt, "array") || strings.HasPrefix(dt, "[]"):
		return createArrayVariant(dt, value)

	default:
		// Fallback: let OPC-UA library handle type inference
		zap.L().Warn("[OPC UA] Write: using fallback type inference", zap.String("dataType", dataType))
		return ua.MustVariant(value)
	}
}

// ===== Variant 创建辅助函数 =====

func createInt32Variant(value any) *ua.Variant {
	var v int32
	switch val := value.(type) {
	case int32:
		v = val
	case int64:
		v = int32(val)
	case float64:
		v = int32(val)
	case int:
		v = int32(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 32); err == nil {
			v = int32(n)
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createInt16Variant(value any) *ua.Variant {
	var v int16
	switch val := value.(type) {
	case int16:
		v = val
	case int32:
		v = int16(val)
	case int64:
		v = int16(val)
	case float64:
		v = int16(val)
	case int:
		v = int16(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 16); err == nil {
			v = int16(n)
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createUint32Variant(value any) *ua.Variant {
	var v uint32
	switch val := value.(type) {
	case uint32:
		v = val
	case uint64:
		v = uint32(val)
	case float64:
		v = uint32(val)
	case uint:
		v = uint32(val)
	case int:
		v = uint32(val)
	case string:
		if n, err := strconv.ParseUint(val, 10, 32); err == nil {
			v = uint32(n)
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createUint16Variant(value any) *ua.Variant {
	var v uint16
	switch val := value.(type) {
	case uint16:
		v = val
	case uint32:
		v = uint16(val)
	case uint64:
		v = uint16(val)
	case float64:
		v = uint16(val)
	case uint:
		v = uint16(val)
	case int:
		v = uint16(val)
	case string:
		if n, err := strconv.ParseUint(val, 10, 16); err == nil {
			v = uint16(n)
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createInt64Variant(value any) *ua.Variant {
	var v int64
	switch val := value.(type) {
	case int64:
		v = val
	case int32:
		v = int64(val)
	case float64:
		v = int64(val)
	case int:
		v = int64(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			v = n
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createUint64Variant(value any) *ua.Variant {
	var v uint64
	switch val := value.(type) {
	case uint64:
		v = val
	case uint32:
		v = uint64(val)
	case float64:
		v = uint64(val)
	case uint:
		v = uint64(val)
	case int:
		v = uint64(val)
	case string:
		if n, err := strconv.ParseUint(val, 10, 64); err == nil {
			v = n
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createFloat32Variant(value any) *ua.Variant {
	var v float32
	switch val := value.(type) {
	case float32:
		v = val
	case float64:
		v = float32(val)
	case int:
		v = float32(val)
	case int32:
		v = float32(val)
	case int64:
		v = float32(val)
	case string:
		if n, err := strconv.ParseFloat(val, 32); err == nil {
			v = float32(n)
		} else {
			return nil
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createFloat64Variant(value any) *ua.Variant {
	var v float64
	switch val := value.(type) {
	case float64:
		v = val
	case float32:
		v = float64(val)
	case int:
		v = float64(val)
	case int32:
		v = float64(val)
	case int64:
		v = float64(val)
	case uint:
		v = float64(val)
	case uint32:
		v = float64(val)
	case uint64:
		v = float64(val)
	case string:
		if n, err := strconv.ParseFloat(val, 64); err == nil {
			v = n
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createBooleanVariant(value any) *ua.Variant {
	var v bool
	switch val := value.(type) {
	case bool:
		v = val
	case float64:
		v = val != 0
	case int:
		v = val != 0
	case string:
		s := strings.ToLower(val)
		switch s {
		case "true", "1":
			v = true
		case "false", "0":
			v = false
		default:
			return nil
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createSByteVariant(value any) *ua.Variant {
	var v int8
	switch val := value.(type) {
	case int8:
		v = val
	case int:
		v = int8(val)
	case int32:
		v = int8(val)
	case int64:
		v = int8(val)
	case float64:
		v = int8(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 8); err == nil {
			v = int8(n)
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createByteVariant(value any) *ua.Variant {
	var v uint8
	switch val := value.(type) {
	case uint8:
		v = val
	case uint:
		v = uint8(val)
	case uint32:
		v = uint8(val)
	case int:
		v = uint8(val)
	case int64:
		v = uint8(val)
	case float64:
		v = uint8(val)
	case string:
		if n, err := strconv.ParseUint(val, 10, 8); err == nil {
			v = uint8(n)
		}
	default:
		return nil
	}
	return ua.MustVariant(v)
}

func createDateTimeVariant(value any) *ua.Variant {
	switch val := value.(type) {
	case time.Time:
		return ua.MustVariant(val)
	case int64:
		return ua.MustVariant(time.Unix(val, 0))
	case float64:
		return ua.MustVariant(time.Unix(int64(val), 0))
	case string:
		formats := []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"}
		for _, f := range formats {
			if t, err := time.Parse(f, val); err == nil {
				return ua.MustVariant(t)
			}
		}
	}
	return nil
}

func createGuidVariant(value any) *ua.Variant {
	switch val := value.(type) {
	case *ua.GUID:
		return ua.MustVariant(val)
	case string:
		g := parseGuid(val)
		if g != nil {
			return ua.MustVariant(g)
		}
	case [16]byte:
		return ua.MustVariant(ua.NewGUID(string(val[:])))
	}
	return nil
}

func createByteStringVariant(value any) *ua.Variant {
	switch val := value.(type) {
	case []byte:
		return ua.MustVariant(val)
	case string:
		// Try base64 decode first
		if decoded, err := base64.StdEncoding.DecodeString(val); err == nil && len(decoded) > 0 {
			return ua.MustVariant(decoded)
		}
		// Fallback to raw bytes
		return ua.MustVariant([]byte(val))
	default:
		return nil
	}
}

func createNodeIDVariant(value any) *ua.Variant {
	switch val := value.(type) {
	case *ua.NodeID:
		return ua.MustVariant(val)
	case string:
		if !strings.HasPrefix(val, "ns=") {
			return nil
		}
		if nodeID, err := ua.ParseNodeID(val); err == nil {
			return ua.MustVariant(nodeID)
		}
	}
	return nil
}

func createStatusCodeVariant(value any) *ua.Variant {
	var v ua.StatusCode
	switch val := value.(type) {
	case ua.StatusCode:
		v = val
	case uint32:
		v = ua.StatusCode(val)
	case uint64:
		v = ua.StatusCode(val)
	case int:
		v = ua.StatusCode(val)
	case int64:
		v = ua.StatusCode(val)
	case float64:
		v = ua.StatusCode(uint32(val))
	case string:
		if code, ok := statusCodeFromName(val); ok {
			v = code
		}
	default:
		v = ua.StatusGood
	}
	return ua.MustVariant(v)
}

func createQualifiedNameVariant(value any) *ua.Variant {
	switch val := value.(type) {
	case ua.QualifiedName:
		return ua.MustVariant(&val)
	case *ua.QualifiedName:
		return ua.MustVariant(val)
	case string:
		parts := strings.SplitN(val, ":", 2)
		if len(parts) == 2 {
			if ns, err := strconv.ParseUint(parts[0], 10, 16); err == nil {
				qn := ua.QualifiedName{NamespaceIndex: uint16(ns), Name: parts[1]}
				return ua.MustVariant(&qn)
			}
		}
		qn := ua.QualifiedName{NamespaceIndex: 0, Name: val}
		return ua.MustVariant(&qn)
	case map[string]any:
		ns := uint16(0)
		if n, ok := val["namespace"].(float64); ok {
			ns = uint16(n)
		}
		name := fmt.Sprintf("%v", val["name"])
		qn := ua.QualifiedName{NamespaceIndex: ns, Name: name}
		return ua.MustVariant(&qn)
	}
	return nil
}

func createLocalizedTextVariant(value any) *ua.Variant {
	switch val := value.(type) {
	case ua.LocalizedText:
		return ua.MustVariant(&val)
	case *ua.LocalizedText:
		return ua.MustVariant(val)
	case string:
		lt := ua.LocalizedText{Text: val, Locale: "en"}
		return ua.MustVariant(&lt)
	case map[string]any:
		text := fmt.Sprintf("%v", val["text"])
		locale := fmt.Sprintf("%v", val["locale"])
		if locale == "<nil>" || locale == "" {
			locale = "en"
		}
		lt := ua.LocalizedText{Text: text, Locale: locale}
		return ua.MustVariant(&lt)
	default:
		lt := ua.LocalizedText{Text: fmt.Sprintf("%v", value), Locale: "en"}
		return ua.MustVariant(&lt)
	}
}

func createExtensionObjectVariant(value any) *ua.Variant {
	// ExtensionObject - treat as bytes
	switch val := value.(type) {
	case []byte:
		return ua.MustVariant(val)
	case string:
		if decoded, err := base64.StdEncoding.DecodeString(val); err == nil {
			return ua.MustVariant(decoded)
		}
		return ua.MustVariant([]byte(val))
	default:
		return nil
	}
}

func createArrayVariant(dataType string, value any) *ua.Variant {
	// Extract element type from array type, e.g., "array:int32" -> "int32"
	elemType := strings.TrimPrefix(dataType, "array:")
	elemType = strings.TrimPrefix(elemType, "[]")

	// Helper to create variant for array element
	createElemVariant := func(elemType string, elem any) *ua.Variant {
		dt := strings.ToLower(elemType)
		switch {
		case dt == "int32", dt == "i=6", dt == "6":
			return createInt32Variant(elem)
		case dt == "int16", dt == "i=4", dt == "4":
			return createInt16Variant(elem)
		case dt == "uint32", dt == "i=7", dt == "7":
			return createUint32Variant(elem)
		case dt == "uint16", dt == "i=5", dt == "5":
			return createUint16Variant(elem)
		case dt == "int64", dt == "i=8", dt == "8":
			return createInt64Variant(elem)
		case dt == "uint64", dt == "i=9", dt == "9":
			return createUint64Variant(elem)
		case dt == "float", dt == "float32", dt == "i=10", dt == "10":
			return createFloat32Variant(elem)
		case dt == "double", dt == "i=11", dt == "11":
			return createFloat64Variant(elem)
		case dt == "boolean", dt == "i=1", dt == "1":
			return createBooleanVariant(elem)
		case dt == "sbyte", dt == "i=2", dt == "2":
			return createSByteVariant(elem)
		case dt == "byte", dt == "uint8", dt == "i=3", dt == "3":
			return createByteVariant(elem)
		case dt == "string", dt == "i=12", dt == "12":
			return ua.MustVariant(fmt.Sprintf("%v", elem))
		case dt == "datetime", dt == "i=13", dt == "13":
			return createDateTimeVariant(elem)
		case dt == "guid", dt == "i=14", dt == "14":
			return createGuidVariant(elem)
		case dt == "bytestring", dt == "i=15", dt == "15":
			return createByteStringVariant(elem)
		default:
			return ua.MustVariant(elem)
		}
	}

	// Handle slice types
	switch val := value.(type) {
	case []any:
		variants := make([]*ua.Variant, len(val))
		for i, elem := range val {
			v := createElemVariant(elemType, elem)
			if v == nil {
				return nil
			}
			variants[i] = v
		}
		return ua.MustVariant(variants)
	case []int:
		intVariants := make([]int, len(val))
		copy(intVariants, val)
		return ua.MustVariant(intVariants)
	case []int32:
		return ua.MustVariant(val)
	case []int64:
		return ua.MustVariant(val)
	case []uint:
		uintVariants := make([]uint, len(val))
		copy(uintVariants, val)
		return ua.MustVariant(uintVariants)
	case []uint32:
		return ua.MustVariant(val)
	case []uint64:
		return ua.MustVariant(val)
	case []float32:
		return ua.MustVariant(val)
	case []float64:
		return ua.MustVariant(val)
	case []string:
		return ua.MustVariant(val)
	case []bool:
		return ua.MustVariant(val)
	case []byte:
		return ua.MustVariant(val)
	}
	return nil
}

// tryAlternativeWriteTypes attempts to write using alternative data types when the primary type fails
func (d *OpcUaDriver) tryAlternativeWriteTypes(ctx context.Context, client *opcua.Client, nodeID *ua.NodeID, originalValue any, originalType string) bool {
	alternatives := getAlternativeTypes(originalType)
	if len(alternatives) == 0 {
		return false
	}

	for _, altType := range alternatives {
		// Parse value with alternative type
		altValue, err := castValue(originalValue, altType)
		if err != nil {
			continue
		}

		// Create variant with alternative type
		variant := d.createWriteVariant(altType, altValue)
		if variant == nil {
			continue
		}

		// Build write request
		dataValue := &ua.DataValue{
			Value:           variant,
			SourceTimestamp: time.Now(),
		}
		dataValue.UpdateMask()

		resp, err := client.Write(ctx, &ua.WriteRequest{
			NodesToWrite: []*ua.WriteValue{
				{
					NodeID:      nodeID,
					AttributeID: ua.AttributeIDValue,
					Value:       dataValue,
				},
			},
		})

		if err == nil && len(resp.Results) > 0 && resp.Results[0] == ua.StatusOK {
			zap.L().Info("[OPC UA] Write: alternative type succeeded",
				zap.Stringer("nodeID", nodeID),
				zap.String("originalType", originalType),
				zap.String("altType", altType))
			return true
		}
	}

	return false
}

// getAlternativeTypes returns a list of alternative data types to try on write failure
func getAlternativeTypes(originalType string) []string {
	dt := strings.ToLower(originalType)
	switch {
	case dt == "int32", dt == "i=6", dt == "6":
		return []string{"Int16", "UInt16", "Int64", "UInt32", "Double"}
	case dt == "int16", dt == "i=4", dt == "4":
		return []string{"Int32", "UInt16", "UInt32", "Double"}
	case dt == "uint32", dt == "i=7", dt == "7":
		return []string{"UInt16", "Int32", "Int64", "Double"}
	case dt == "uint16", dt == "i=5", dt == "5":
		return []string{"UInt32", "Int16", "Int32", "Double"}
	case dt == "double", dt == "i=11", dt == "11":
		return []string{"Float", "Int32", "UInt32"}
	case dt == "float", dt == "float32", dt == "i=10", dt == "10":
		return []string{"Double", "Int32"}
	default:
		return nil
	}
}

func (d *OpcUaDriver) ScanObjects(ctx context.Context, config map[string]any) (any, error) {
	endpoint, err := d.resolveEndpointInConfig(config)
	if err != nil {
		return nil, err
	}

	opts, err := d.buildClientOptions(config)
	if err != nil {
		return nil, err
	}

	// Start browsing from Objects folder
	rootID := ua.NewNumericNodeID(0, 85)

	if rootNodeIDStr, ok := config["root_node_id"].(string); ok && rootNodeIDStr != "" {
		id, err := ua.ParseNodeID(rootNodeIDStr)
		if err == nil {
			rootID = id
			zap.L().Info("Starting scan from custom node", zap.String("node_id", rootID.String()))
		}
	}

	// Create client
	c, err := opcua.NewClient(endpoint, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create scan client: %v", err)
	}

	scanCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok {
		scanCtx, cancel = context.WithTimeout(ctx, 180*time.Second) // 增加到3分钟
	}
	defer cancel()

	if err := c.Connect(scanCtx); err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}
	defer c.Close(context.Background())

	// Use recursive helper with concurrency control
	results, err := d.browseNode(scanCtx, c, rootID, 0)
	if err != nil {
		return nil, fmt.Errorf("scan failed: %v", err)
	}

	return results, nil
}

// browseNode recursively browses the OPC UA address space
// It now supports concurrent browsing for children to speed up scanning
func (d *OpcUaDriver) browseNode(ctx context.Context, c *opcua.Client, nodeID *ua.NodeID, depth int) ([]map[string]any, error) {
	if depth > 10 { // Limit depth
		return nil, nil
	}

	// Fetch references
	refs, err := d.fetchReferences(ctx, c, nodeID)
	if err != nil {
		return nil, err
	}

	var results []map[string]any
	var variableNodeIDs []*ua.ReadValueID
	var variableIndices []int
	var childrenToBrowse []*ua.NodeID
	var childrenIndices []int

	for _, ref := range refs {
		// Convert NodeID to string
		nodeIDStr := ref.NodeID.NodeID.String()
		parsedID, parseErr := ua.ParseNodeID(nodeIDStr)
		if parseErr != nil {
			continue
		}

		// Skip standard "Server" object (ns=0;i=2253)
		if parsedID.Namespace() == 0 && (parsedID.IntID() == 2253 || parsedID.IntID() == 23470 || parsedID.IntID() == 31915) {
			continue
		}

		item := map[string]any{
			"node_id": nodeIDStr,
			"name":    ref.DisplayName.Text,
			"class":   ref.NodeClass.String(),
		}

		if ref.NodeClass == ua.NodeClassVariable {
			item["type"] = "Variable"
			item["address"] = nodeIDStr

			// Queue for DataType and AccessLevel reading
			variableNodeIDs = append(variableNodeIDs,
				&ua.ReadValueID{
					NodeID:      parsedID,
					AttributeID: ua.AttributeIDDataType,
				},
				&ua.ReadValueID{
					NodeID:      parsedID,
					AttributeID: ua.AttributeIDAccessLevel,
				},
			)
			variableIndices = append(variableIndices, len(results), len(results))
			results = append(results, item)

		} else if ref.NodeClass == ua.NodeClassObject {
			item["type"] = "Folder"
			// Queue for recursive browsing
			childrenToBrowse = append(childrenToBrowse, parsedID)
			childrenIndices = append(childrenIndices, len(results))
			results = append(results, item)
		}
	}

	// 1. Batch Read DataTypes (Sequential, fast enough)
	if len(variableNodeIDs) > 0 {
		d.batchReadDataTypes(ctx, c, variableNodeIDs, results, variableIndices)
	}

	// 2. Browse Children (concurrent)
	if len(childrenToBrowse) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Limit concurrency to avoid overwhelming the server
		concurrencyLimit := 5
		semaphore := make(chan struct{}, concurrencyLimit)

		for i, childID := range childrenToBrowse {
			semaphore <- struct{}{} // Acquire semaphore
			wg.Add(1)

			go func(idx int, childID *ua.NodeID) {
				defer wg.Done()
				defer func() { <-semaphore }() // Release semaphore

				children, err := d.browseNode(ctx, c, childID, depth+1)
				if err != nil {
					zap.L().Warn("Browse child failed", zap.String("node", childID.String()), zap.Error(err))
					mu.Lock()
					results[idx]["browse_error"] = err.Error()
					mu.Unlock()
					return
				}
				if len(children) > 0 {
					mu.Lock()
					results[idx]["children"] = children
					mu.Unlock()
				}
			}(childrenIndices[i], childID)
		}

		wg.Wait()
	}

	return results, nil
}

// fetchReferences handles the Browse request with retries
func (d *OpcUaDriver) fetchReferences(ctx context.Context, c *opcua.Client, nodeID *ua.NodeID) ([]ua.ReferenceDescription, error) {
	// Initial request
	req := &ua.BrowseRequest{
		RequestedMaxReferencesPerNode: 50,
		NodesToBrowse: []*ua.BrowseDescription{
			{
				NodeID:          nodeID,
				BrowseDirection: ua.BrowseDirectionForward,
				ReferenceTypeID: ua.NewNumericNodeID(0, 33), // HierarchicalReferences
				IncludeSubtypes: true,
				NodeClassMask:   uint32(ua.NodeClassObject | ua.NodeClassVariable),
				ResultMask:      uint32(ua.BrowseResultMaskAll),
			},
		},
	}

	resp, err := c.Browse(ctx, req)
	if err != nil && isOPCUAConnError(err) {
		_ = c.Close(context.Background())
		if err2 := c.Connect(ctx); err2 == nil {
			resp, err = c.Browse(ctx, req)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(resp.Results) == 0 || resp.Results[0].StatusCode != ua.StatusOK {
		return nil, fmt.Errorf("bad status code: %v", resp.Results[0].StatusCode)
	}

	var refs []ua.ReferenceDescription
	for _, r := range resp.Results[0].References {
		refs = append(refs, *r)
	}

	// Handle ContinuationPoint
	continuationPoint := resp.Results[0].ContinuationPoint
	for len(continuationPoint) > 0 {
		reqNext := &ua.BrowseNextRequest{
			ReleaseContinuationPoints: false,
			ContinuationPoints:        [][]byte{continuationPoint},
		}

		respNext, err := c.BrowseNext(ctx, reqNext)
		if err != nil {
			return nil, err
		}
		if len(respNext.Results) == 0 || respNext.Results[0].StatusCode != ua.StatusOK {
			return nil, fmt.Errorf("browse next bad status code: %v", respNext.Results[0].StatusCode)
		}

		for _, r := range respNext.Results[0].References {
			refs = append(refs, *r)
		}

		continuationPoint = respNext.Results[0].ContinuationPoint
	}

	return refs, nil
}

func isOPCUAConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "use of closed network connection") || strings.Contains(msg, "EOF")
}

func isOpcUaSessionError(err error) bool {
	if err == nil {
		return false
	}
	if isOPCUAConnError(err) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	sessionPatterns := []string{
		"badconnectionclosed",
		"badnotconnected",
		"badsessionclosed",
		"badsessionidinvalid",
		"badsecurechannelclosed",
		"badsecurechannelidinvalid",
		"badservernotconnected",
		"badnocommunication",
		"client not connected",
		"no active client",
		"secure channel",
		"session closed",
		"connection closed",
		"connection refused",
		"connection reset",
	}
	for _, p := range sessionPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// batchReadDataTypes reads data types in batches with concurrency
func (d *OpcUaDriver) batchReadDataTypes(ctx context.Context, c *opcua.Client, nodeIDs []*ua.ReadValueID, results []map[string]any, indices []int) {
	// Split into smaller chunks if necessary (e.g., 50 items)
	chunkSize := 50
	chunks := make([][]int, 0)

	for i := 0; i < len(nodeIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(nodeIDs) {
			end = len(nodeIDs)
		}
		chunks = append(chunks, []int{i, end})
	}

	// Process chunks concurrently
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Limit concurrency to avoid overwhelming the server
	concurrencyLimit := 3
	semaphore := make(chan struct{}, concurrencyLimit)

	for _, chunk := range chunks {
		semaphore <- struct{}{} // Acquire semaphore
		wg.Add(1)

		go func(start, end int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			chunkIDs := nodeIDs[start:end]
			chunkIndices := indices[start:end]

			req := &ua.ReadRequest{
				NodesToRead: chunkIDs,
				MaxAge:      2000,
			}

			resp, err := c.Read(ctx, req)
			if err != nil {
				zap.L().Warn("Read DataTypes chunk failed", zap.Error(err))
				return
			}

			for j, res := range resp.Results {
				if res.Status != ua.StatusOK || res.Value == nil {
					continue
				}

				mu.Lock()
				switch value := res.Value.Value().(type) {
				case *ua.NodeID:
					results[chunkIndices[j]]["data_type"] = lookupDataType(value)
				case byte:
					results[chunkIndices[j]]["access_level"] = lookupAccessLevel(value)
				}
				mu.Unlock()
			}
		}(chunk[0], chunk[1])
	}

	wg.Wait()
}

func lookupAccessLevel(level byte) string {
	flags := make([]string, 0, 8)

	if level&1 != 0 {
		flags = append(flags, "CurrentRead")
	}
	if level&2 != 0 {
		flags = append(flags, "CurrentWrite")
	}
	if level&4 != 0 {
		flags = append(flags, "HistoryRead")
	}
	if level&8 != 0 {
		flags = append(flags, "HistoryWrite")
	}
	if level&16 != 0 {
		flags = append(flags, "SemanticChange")
	}
	if level&32 != 0 {
		flags = append(flags, "StatusWrite")
	}
	if level&64 != 0 {
		flags = append(flags, "TimestampWrite")
	}

	return strings.Join(flags, ",")
}

func lookupDataType(id *ua.NodeID) string {
	if id.Namespace() != 0 {
		return id.String()
	}
	switch id.IntID() {
	case 1:
		return "Boolean"
	case 2:
		return "SByte"
	case 3:
		return "Byte"
	case 4:
		return "Int16"
	case 5:
		return "UInt16"
	case 6:
		return "Int32"
	case 7:
		return "UInt32"
	case 8:
		return "Int64"
	case 9:
		return "UInt64"
	case 10:
		return "Float"
	case 11:
		return "Double"
	case 12:
		return "String"
	case 13:
		return "DateTime"
	case 15:
		return "ByteString"
	default:
		return fmt.Sprintf("ns=%d;i=%d", id.Namespace(), id.IntID())
	}
}

func decodeHexString(raw string) ([]byte, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	if len(s)%2 != 0 {
		s = "0" + s
	}
	return hex.DecodeString(s)
}

func decodeBase64String(raw string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
}

func parseByteStringValue(val any) ([]byte, error) {
	switch v := val.(type) {
	case []byte:
		return v, nil
	case string:
		s := strings.TrimSpace(v)
		if strings.HasPrefix(strings.ToLower(s), "0x") {
			return decodeHexString(s)
		}
		if decoded, err := decodeBase64String(s); err == nil {
			return decoded, nil
		}
		return nil, fmt.Errorf("invalid bytestring value: %s", v)
	case map[string]any:
		encoding := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", v["encoding"])))
		raw := fmt.Sprintf("%v", v["value"])
		switch encoding {
		case "hex":
			return decodeHexString(raw)
		case "base64":
			return decodeBase64String(raw)
		default:
			return nil, fmt.Errorf("unsupported bytestring encoding: %s", encoding)
		}
	default:
		return nil, fmt.Errorf("cannot cast %T to bytestring", val)
	}
}

func (w *ClientWrapper) RecordSuccess() {
	w.connMgr.RecordSuccess()
	w.collectFailCount.Store(0)
	w.lastActivityTime.Store(time.Now())
}

func (w *ClientWrapper) RecordFailure(err error) {
	if err != nil && !isOpcUaSessionError(err) {
		return
	}

	w.collectFailCount.Add(1)
	w.lastActivityTime.Store(time.Now())
	w.connMgr.RecordFailure()

	if w.collectFailCount.Load() >= w.maxFailCount {
		zap.L().Warn("[OPC UA] Max fail count reached, triggering reconnect",
			zap.String("endpoint", w.Endpoint),
			zap.Int32("fail_count", w.collectFailCount.Load()),
			zap.Int32("max_fail", w.maxFailCount),
		)
		w.Connected = false
		if w.onReconnectNeeded != nil {
			w.onReconnectNeeded()
		}
	}
}

func (w *ClientWrapper) NeedProbeCheck() bool {
	lastActivity := w.lastActivityTime.Load()
	if lastActivity == nil {
		return false
	}

	lastTime, ok := lastActivity.(time.Time)
	if !ok || lastTime.IsZero() {
		return false
	}

	return time.Since(lastTime) > w.collectCycle*3
}

func (w *ClientWrapper) ProbeConnection() {
	if !w.Connected || w.Client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := w.Client.Read(ctx, &ua.ReadRequest{
		NodesToRead: []*ua.ReadValueID{
			{
				NodeID:      ua.NewNumericNodeID(0, 2253),
				AttributeID: ua.AttributeIDBrowseName,
			},
		},
	})

	if err != nil {
		w.RecordFailure(err)
	} else {
		w.RecordSuccess()
	}
}

func normalizeOpcUaDataType(dataType string) string {
	dt := strings.ToLower(strings.TrimSpace(dataType))
	switch dt {
	case "i=1", "1":
		return "boolean"
	case "i=2", "2":
		return "sbyte"
	case "i=3", "3":
		return "byte"
	case "i=4", "4":
		return "int16"
	case "i=5", "5":
		return "uint16"
	case "i=6", "6":
		return "int32"
	case "i=7", "7":
		return "uint32"
	case "i=8", "8":
		return "int64"
	case "i=9", "9":
		return "uint64"
	case "i=10", "10":
		return "float32"
	case "i=11", "11":
		return "float64"
	case "i=12", "12":
		return "string"
	default:
		return dt
	}
}

func castValue(val any, dataType string) (any, error) {
	dt := normalizeOpcUaDataType(dataType)
	asString := func(v any) string {
		return fmt.Sprintf("%v", v)
	}

	switch {
	case dt == "bool" || dt == "boolean":
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			return strconv.ParseBool(v)
		default:
			s := asString(v)
			if b, err := strconv.ParseBool(s); err == nil {
				return b, nil
			}
			// Numeric fallback: != 0 is true
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				return f != 0, nil
			}
			return nil, fmt.Errorf("cannot cast %v to bool", val)
		}

	case strings.Contains(dt, "uint16") || dt == "unsignedshort":
		switch v := val.(type) {
		case uint16:
			return v, nil
		case float64:
			return uint16(v), nil
		case int:
			return uint16(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return uint16(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to uint16", val)

	case strings.Contains(dt, "int16") || dt == "short":
		switch v := val.(type) {
		case int16:
			return v, nil
		case float64:
			return int16(v), nil
		case int:
			return int16(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int16(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to int16", val)

	case dt == "sbyte" || dt == "int8":
		switch v := val.(type) {
		case int8:
			return v, nil
		case float64:
			return int8(v), nil
		case int:
			return int8(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int8(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to sbyte", val)

	case dt == "byte" || dt == "uint8":
		switch v := val.(type) {
		case uint8:
			return v, nil
		case float64:
			return uint8(v), nil
		case int:
			return uint8(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return uint8(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to byte", val)

	case strings.Contains(dt, "uint32") || dt == "unsignedint":
		switch v := val.(type) {
		case uint32:
			return v, nil
		case float64:
			return uint32(v), nil
		case int:
			return uint32(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return uint32(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to uint32", val)

	case strings.Contains(dt, "int32") || dt == "int":
		switch v := val.(type) {
		case int32:
			return v, nil
		case float64:
			return int32(v), nil
		case int:
			return int32(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int32(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to int32", val)

	case strings.Contains(dt, "uint64") || dt == "unsignedlong":
		switch v := val.(type) {
		case uint64:
			return v, nil
		case float64:
			return uint64(v), nil
		case int:
			return uint64(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return uint64(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to uint64", val)

	case strings.Contains(dt, "int64") || dt == "long":
		switch v := val.(type) {
		case int64:
			return v, nil
		case float64:
			return int64(v), nil
		case int:
			return int64(v), nil
		}
		s := asString(val)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int64(f), nil
		}
		return nil, fmt.Errorf("cannot cast %v to int64", val)

	case strings.Contains(dt, "float32") || dt == "float":
		s := asString(val)
		f, err := strconv.ParseFloat(s, 32)
		if err != nil {
			return nil, err
		}
		return float32(f), nil

	case strings.Contains(dt, "float64") || dt == "double":
		s := asString(val)
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, err
		}
		return f, nil

	case dt == "string":
		return asString(val), nil

	case dt == "bytestring":
		return parseByteStringValue(val)
	}

	return val, nil
}

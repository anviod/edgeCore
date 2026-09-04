package s7

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"

	"github.com/anviod/gos7"
	"go.uber.org/zap"
)

const (
	StateDisconnected driver.ConnState = driver.StateDisconnected
	StateConnecting   driver.ConnState = driver.StateConnecting
	StateConnected    driver.ConnState = driver.StateConnected
	StateRetrying     driver.ConnState = driver.StateRetrying
	StateDead         driver.ConnState = driver.StateDead
)

// S7ClientHandler 扩展gos7.ClientHandler，添加连接管理方法
type S7ClientHandler interface {
	gos7.ClientHandler
	Connect() error
	Close() error
	Timeout() time.Duration
	SetTimeout(timeout time.Duration)
	IdleTimeout() time.Duration
	SetIdleTimeout(timeout time.Duration)
}

// S7 区域常量
const (
	S7AreaDB = 0x84 // 数据块
	S7AreaMK = 0x83 // 标志存储器 (M区)
	S7AreaPE = 0x81 // 输入过程映像 (I区)
	S7AreaPA = 0x80 // 输出过程映像 (Q区)
	S7AreaTM = 0x1D // 定时器
	S7AreaCT = 0x1C // 计数器
)

// S7 字长常量
const (
	S7WLBit     = 0x01
	S7WLByte    = 0x02
	S7WLWord    = 0x04
	S7WLDWord   = 0x06
	S7WLReal    = 0x08
	S7WLCounter = 0x1C
	S7WLTimer   = 0x1D
)

// S7 连接类型
const (
	ConnTypePG      = 1 // 编程设备连接
	ConnTypeOP      = 2 // 操作面板连接
	ConnTypeS7Basic = 3 // 基本S7连接
)

// PLC类型默认参数
var plcDefaults = map[string]struct {
	Rack         int
	Slot         int
	ConnType     int
	MaxFailCount int
	DefaultCycle time.Duration
}{
	"s7-200smart": {Rack: 0, Slot: 1, ConnType: ConnTypeS7Basic, MaxFailCount: 3, DefaultCycle: 60 * time.Second},
	"s7-1200":     {Rack: 0, Slot: 1, ConnType: ConnTypeS7Basic, MaxFailCount: 5, DefaultCycle: 10 * time.Second},
	"s7-1500":     {Rack: 0, Slot: 0, ConnType: ConnTypeS7Basic, MaxFailCount: 5, DefaultCycle: 10 * time.Second},
	"s7-300":      {Rack: 0, Slot: 2, ConnType: ConnTypePG, MaxFailCount: 5, DefaultCycle: 10 * time.Second},
	"s7-400":      {Rack: 0, Slot: 3, ConnType: ConnTypePG, MaxFailCount: 5, DefaultCycle: 10 * time.Second},
}

// S7Transport S7传输层，封装gos7连接管理
type S7Transport struct {
	cfg     map[string]any
	client  gos7.Client
	handler S7ClientHandler

	// 依赖注入（用于测试）
	clientFactory  func(handler S7ClientHandler) gos7.Client
	handlerFactory func(address string, rack, slot, connType int) S7ClientHandler

	// 配置参数
	ip       string
	port     int
	rack     int
	slot     int
	timeout  time.Duration
	connType int
	pduSize  int
	plcType  string

	// 连接状态
	connected          atomic.Bool
	mu                 sync.Mutex
	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string

	// 采集健康检测（替代独立心跳）
	lastSuccessTime  atomic.Value // time.Time
	collectFailCount atomic.Int32
	maxFailCount     int32
	collectCycle     time.Duration

	// 重试
	maxRetries    int
	retryInterval time.Duration
	maxBackoff    time.Duration

	// 指数退避参数
	baseDelay     time.Duration
	backoffFactor float64

	// 连接管理器（状态机）
	connMgr *driver.ConnectionManager
}

// NewS7Transport 创建S7传输层实例
func NewS7Transport(cfg map[string]any) *S7Transport {
	t := &S7Transport{
		cfg:           cfg,
		port:          102,
		timeout:       2 * time.Second,
		connType:      ConnTypeS7Basic,
		pduSize:       4096,
		maxRetries:    1,
		retryInterval: 100 * time.Millisecond,
		maxBackoff:    30 * time.Second,
		baseDelay:     100 * time.Millisecond,
		backoffFactor: 2.0,
		maxFailCount:  5,
		collectCycle:  10 * time.Second,
	}
	t.lastSuccessTime.Store(time.Time{})

	// 设置默认工厂函数
	t.clientFactory = func(handler S7ClientHandler) gos7.Client {
		return gos7.NewClient(handler)
	}
	t.handlerFactory = func(address string, rack, slot, connType int) S7ClientHandler {
		return &defaultS7ClientHandler{
			handler: gos7.NewTCPClientHandlerWithConnectType(address, rack, slot, connType),
		}
	}

	// 解析配置
	t.parseConfig()

	// 创建连接管理器（状态机）
	t.connMgr = driver.NewConnectionManager("s7")
	if t.plcType == "s7-200smart" {
		t.connMgr.SetMaxRetries(8)
		t.connMgr.SetMaxFailCount(3)
	}

	return t
}

// parseConfig 从配置map中解析参数
func (t *S7Transport) parseConfig() {
	// IP
	if v, ok := t.cfg["ip"].(string); ok {
		t.ip = v
	}

	// Port
	t.port = getCfgInt(t.cfg, "port", 102)

	// Rack & Slot (可能从plcType推导)
	t.rack = getCfgInt(t.cfg, "rack", -1)
	t.slot = getCfgInt(t.cfg, "slot", -1)

	// PLC Type
	t.plcType = ""
	if v, ok := t.cfg["plcType"].(string); ok {
		t.plcType = strings.ToLower(v)
	}

	// 如果rack/slot未指定，从plcType推导，并设置默认参数
	if t.rack < 0 || t.slot < 0 {
		if defaults, ok := plcDefaults[t.plcType]; ok {
			if t.rack < 0 {
				t.rack = defaults.Rack
			}
			if t.slot < 0 {
				t.slot = defaults.Slot
			}
			if _, exists := t.cfg["connect_type"]; !exists {
				t.connType = defaults.ConnType
			}
			if _, exists := t.cfg["max_fail_count"]; !exists {
				t.maxFailCount = int32(defaults.MaxFailCount)
			}
			if _, exists := t.cfg["collect_cycle"]; !exists {
				t.collectCycle = defaults.DefaultCycle
			}
		} else {
			// 默认值
			if t.rack < 0 {
				t.rack = 0
			}
			if t.slot < 0 {
				t.slot = 1
			}
		}
	}

	// Timeout
	if v, ok := t.cfg["timeout"]; ok {
		switch val := v.(type) {
		case float64:
			t.timeout = time.Duration(val) * time.Millisecond
		case int:
			t.timeout = time.Duration(val) * time.Millisecond
		case string:
			if d, err := time.ParseDuration(val); err == nil {
				t.timeout = d
			}
		}
	}

	// Connect type
	if v, ok := t.cfg["connect_type"].(string); ok {
		switch strings.ToLower(v) {
		case "pg":
			t.connType = ConnTypePG
		case "op":
			t.connType = ConnTypeOP
		case "s7basic", "s7_basic":
			t.connType = ConnTypeS7Basic
		}
	}

	// PDU size
	t.pduSize = getCfgInt(t.cfg, "pdu_size", 4096)

	// Max fail count (基于采集失败的连接健康检测)
	t.maxFailCount = int32(getCfgInt(t.cfg, "max_fail_count", int(t.maxFailCount)))

	// Collect cycle
	if v, ok := t.cfg["collect_cycle"]; ok {
		switch val := v.(type) {
		case float64:
			t.collectCycle = time.Duration(val) * time.Millisecond
		case int:
			t.collectCycle = time.Duration(val) * time.Millisecond
		case string:
			if d, err := time.ParseDuration(val); err == nil {
				t.collectCycle = d
			}
		}
	}

	// Max retries (重连最大尝试次数)
	maxRetries := getCfgInt(t.cfg, "max_retries", 64)
	// S7-200Smart 最大重试次数为8
	if t.plcType == "s7-200smart" {
		t.maxRetries = 8
	} else {
		t.maxRetries = maxRetries
	}

	// Base delay for backoff
	if v, ok := t.cfg["retry_base_delay"]; ok {
		switch val := v.(type) {
		case float64:
			t.baseDelay = time.Duration(val) * time.Millisecond
		case int:
			t.baseDelay = time.Duration(val) * time.Millisecond
		}
	}
	if t.baseDelay <= 0 {
		t.baseDelay = 100 * time.Millisecond
	}

	// Max backoff delay
	if v, ok := t.cfg["retry_max_delay"]; ok {
		switch val := v.(type) {
		case float64:
			t.maxBackoff = time.Duration(val) * time.Millisecond
		case int:
			t.maxBackoff = time.Duration(val) * time.Millisecond
		case string:
			if d, err := time.ParseDuration(val); err == nil {
				t.maxBackoff = d
			}
		}
	}
	if t.maxBackoff <= 0 {
		t.maxBackoff = 30 * time.Second
	}
}

// Connect 建立S7 TCP连接。拨号与退避在 Transport.mu 外执行，避免拖慢整通道。
func (t *S7Transport) Connect(ctx context.Context) error {
	if t.connected.Load() {
		return nil
	}
	if t.ip == "" {
		return fmt.Errorf("S7 transport: IP address not configured")
	}

	if t.connMgr != nil {
		return t.connMgr.EnsureConnected(ctx, func(ctx context.Context) error {
			return t.connectOnce(ctx)
		})
	}

	var lastErr error
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			wait := t.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		if err := t.connectOnce(ctx); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("S7 transport: connection failed after %d attempts: %w", t.maxRetries+1, lastErr)
}

func (t *S7Transport) connectOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if t.connected.Load() {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", t.ip, t.port)
	handler := t.handlerFactory(addr, t.rack, t.slot, t.connType)
	handler.SetTimeout(t.timeout)
	handler.SetIdleTimeout(t.collectCycle * 2)

	if err := handler.Connect(); err != nil {
		zap.L().Warn("[S7] Connection failed",
			zap.Error(err),
			zap.String("addr", addr),
			zap.String("plcType", t.plcType),
		)
		handler.Close()
		return err
	}

	client := t.clientFactory(handler)

	t.mu.Lock()
	if t.connected.Load() {
		t.mu.Unlock()
		handler.Close()
		return nil
	}
	if t.handler != nil {
		t.handler.Close()
	}
	t.handler = handler
	t.client = client
	t.connected.Store(true)
	t.connectTime = time.Now()
	t.reconnectCount.Add(1)
	t.collectFailCount.Store(0)
	t.remoteAddr = addr
	t.localAddr = t.getLocalAddr()
	t.mu.Unlock()

	zap.L().Info("[S7] TCP connection established",
		zap.String("addr", addr),
		zap.Int("rack", t.rack),
		zap.Int("slot", t.slot),
		zap.Int("connType", t.connType),
		zap.Duration("timeout", t.timeout),
		zap.String("plcType", t.plcType),
	)
	return nil
}

func (t *S7Transport) scheduleReconnect() {
	if t.connMgr == nil {
		return
	}
	timeout := t.timeout * time.Duration(t.maxRetries+1)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	t.connMgr.ScheduleReconnect(context.Background(), timeout, func(ctx context.Context) error {
		return t.connectOnce(ctx)
	})
}

// calculateBackoff 计算指数退避时间
func (t *S7Transport) calculateBackoff(attempt int) time.Duration {
	backoff := t.baseDelay * time.Duration(t.backoffFactor*float64(attempt))
	if backoff > t.maxBackoff {
		backoff = t.maxBackoff
	}
	return backoff
}

// Disconnect 断开连接
func (t *S7Transport) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	wasConnected := t.connected.Load()

	if t.handler != nil {
		_ = t.handler.Close()
		t.handler = nil
	}
	t.client = nil
	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()

	if t.connMgr != nil {
		t.connMgr.SetState(StateDisconnected)
	}

	if wasConnected {
		zap.L().Info("[S7] Disconnected")
	}

	return nil
}

// IsConnected 是否已连接
func (t *S7Transport) IsConnected() bool {
	return t.connected.Load()
}

// GetClient 获取gos7客户端
func (t *S7Transport) GetClient() gos7.Client {
	return t.client
}

// RecordSuccess 记录采集成功，重置失败计数
func (t *S7Transport) RecordSuccess() {
	t.lastSuccessTime.Store(time.Now())
	t.collectFailCount.Store(0)
	if t.connMgr != nil {
		t.connMgr.RecordSuccess()
	}
}

// RecordFailure 记录采集失败，达到阈值时断开连接
func (t *S7Transport) RecordFailure(err error) {
	failCount := t.collectFailCount.Add(1)
	zap.L().Warn("[S7] Collect failed",
		zap.Error(err),
		zap.Int32("failCount", failCount),
		zap.Int32("maxFailCount", t.maxFailCount),
		zap.String("addr", t.remoteAddr),
	)

	if t.connMgr != nil {
		t.connMgr.RecordFailure()
	}

	if failCount >= t.maxFailCount {
		zap.L().Error("[S7] Collect failed max times, disconnecting",
			zap.Int32("failCount", failCount),
			zap.Int32("maxFailCount", t.maxFailCount),
			zap.String("plcType", t.plcType),
		)
		t.Disconnect()
	}
}

// NeedProbeCheck 检查是否需要轻量探测（低频采集场景补偿）
func (t *S7Transport) NeedProbeCheck() bool {
	if !t.connected.Load() {
		return false
	}
	lastSuccess := t.lastSuccessTime.Load().(time.Time)
	if lastSuccess.IsZero() {
		return true
	}
	return time.Since(lastSuccess) > t.collectCycle*3
}

// ProbeConnection 轻量探测连接是否存活
func (t *S7Transport) ProbeConnection() bool {
	if !t.connected.Load() {
		return false
	}

	t.mu.Lock()
	client := t.client
	t.mu.Unlock()

	if client == nil {
		return false
	}

	buf := make([]byte, 1)
	err := client.AGReadMB(0, 1, buf)
	if err != nil {
		zap.L().Debug("[S7] Probe failed", zap.Error(err))
		return false
	}

	t.RecordSuccess()
	return true
}

// GetConnectionMetrics 获取连接指标
func (t *S7Transport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	reconnectCount = int64(t.reconnectCount.Load())
	lastDisconnectTime = t.lastDisconnectTime

	if !t.connected.Load() {
		remoteAddr = t.remoteAddr
		if remoteAddr == "" && t.ip != "" {
			remoteAddr = fmt.Sprintf("%s:%d", t.ip, t.port)
		}
		return 0, reconnectCount, t.localAddr, remoteAddr, lastDisconnectTime
	}

	connectionSeconds = int64(time.Since(t.connectTime).Seconds())
	localAddr = t.localAddr
	remoteAddr = t.remoteAddr

	return
}

// GetHealthStatus 获取健康状态信息
func (t *S7Transport) GetHealthStatus() (connected bool, failCount int32, maxFailCount int32, lastSuccess time.Time) {
	return t.connected.Load(), t.collectFailCount.Load(), t.maxFailCount, t.lastSuccessTime.Load().(time.Time)
}

// getLocalAddr 获取本地地址
func (t *S7Transport) getLocalAddr() string {
	if t.handler == nil {
		return ""
	}
	udpConn, err := net.DialTimeout("udp", t.remoteAddr, 1*time.Second)
	if err == nil {
		addr, _, _ := net.SplitHostPort(udpConn.LocalAddr().String())
		udpConn.Close()
		return addr
	}
	return ""
}

// withRetry 带重试的操作执行。链路级错误只异步重连，禁止在热路径同步 dial。
func (t *S7Transport) withRetry(ctx context.Context, fn func(client gos7.Client) error) error {
	var lastErr error

	if !t.connected.Load() {
		t.scheduleReconnect()
		return fmt.Errorf("S7: not connected")
	}

	for i := 0; i <= t.maxRetries; i++ {
		if i > 0 {
			wait := t.calculateBackoff(i)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			if !t.connected.Load() {
				t.scheduleReconnect()
				return fmt.Errorf("S7: not connected")
			}
		}

		t.mu.Lock()
		client := t.client
		t.mu.Unlock()

		if client == nil {
			lastErr = fmt.Errorf("S7 client is nil")
			t.scheduleReconnect()
			break
		}

		err := fn(client)
		if err == nil {
			t.RecordSuccess()
			return nil
		}

		lastErr = err
		errMsg := err.Error()

		isNetworkError := containsAny(errMsg, "timeout", "connection", "broken pipe", "reset", "eof")
		if isNetworkError {
			zap.L().Warn("[S7] Network error — async reconnect",
				zap.Error(err),
				zap.Int("attempt", i),
				zap.String("remoteAddr", t.remoteAddr),
			)
			t.RecordFailure(err)
			_ = t.Disconnect()
			t.scheduleReconnect()
			break
		}
	}

	return lastErr
}

// getCfgInt 从配置map获取int值
func getCfgInt(cfg map[string]any, key string, defaultVal int) int {
	if v, ok := cfg[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val)
		case int:
			return val
		case string:
			var n int
			if _, err := fmt.Sscanf(val, "%d", &n); err == nil {
				return n
			}
		}
	}
	return defaultVal
}

// containsAny 检查字符串是否包含任意一个子串
func containsAny(s string, substrs ...string) bool {
	lower := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(lower, sub) {
			return true
		}
	}
	return false
}

// defaultS7ClientHandler 默认S7客户端处理器，包装gos7.TCPClientHandler
type defaultS7ClientHandler struct {
	handler *gos7.TCPClientHandler
}

func (d *defaultS7ClientHandler) Connect() error {
	return d.handler.Connect()
}

func (d *defaultS7ClientHandler) Close() error {
	return d.handler.Close()
}

func (d *defaultS7ClientHandler) Timeout() time.Duration {
	return d.handler.Timeout
}

func (d *defaultS7ClientHandler) SetTimeout(timeout time.Duration) {
	d.handler.Timeout = timeout
}

func (d *defaultS7ClientHandler) IdleTimeout() time.Duration {
	return d.handler.IdleTimeout
}

func (d *defaultS7ClientHandler) SetIdleTimeout(timeout time.Duration) {
	d.handler.IdleTimeout = timeout
}

func (d *defaultS7ClientHandler) Verify(request []byte, response []byte) (err error) {
	return d.handler.Verify(request, response)
}

func (d *defaultS7ClientHandler) Send(request []byte) (response []byte, err error) {
	return d.handler.Send(request)
}

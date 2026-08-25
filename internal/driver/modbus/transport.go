package modbus

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/simonvetter/modbus"
	"go.uber.org/zap"
)

// normalizeModbusURL 补全 Modbus 连接 URL 的 scheme（如 127.0.0.1:502 → tcp://127.0.0.1:502）。
func normalizeModbusURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		return raw
	}
	return "tcp://" + raw
}

// parseParity 将校验位字符串（N/E/O 或 none/even/odd）转换为 modbus 库的枚举值。
func parseParity(p string) uint {
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case "E", "EVEN":
		return modbus.PARITY_EVEN
	case "O", "ODD":
		return modbus.PARITY_ODD
	default:
		return modbus.PARITY_NONE
	}
}

const (
	StateDisconnected driver.ConnState = driver.StateDisconnected
	StateConnecting   driver.ConnState = driver.StateConnecting
	StateConnected    driver.ConnState = driver.StateConnected
	StateRetrying     driver.ConnState = driver.StateRetrying
	StateDead         driver.ConnState = driver.StateDead
)

// Transport 接口定义
type Transport interface {
	Connect(ctx context.Context) error
	Disconnect() error
	IsConnected() bool

	ReadRegisters(ctx context.Context, regType string, offset uint16, count uint16) ([]byte, error)
	ReadCoil(ctx context.Context, offset uint16) (bool, error)
	ReadDiscreteInput(ctx context.Context, offset uint16) (bool, error)
	ReadCustom(ctx context.Context, funcCode byte, offset uint16, count uint16) ([]byte, error)

	WriteRegister(ctx context.Context, offset uint16, value uint16) error
	WriteRegisters(ctx context.Context, offset uint16, values []uint16) error
	WriteCoil(ctx context.Context, offset uint16, value bool) error

	SetUnitID(id uint8)
	GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time)
}

// MetricsRecorder 指标记录器接口
type MetricsRecorder interface {
	RecordRequest(channelID string, success bool, duration time.Duration, errorType string)
	RecordReconnect(channelID string)
	RecordConnectionStart(channelID string)
	RecordError(channelID string, errType, code, message string)
	RecordPointDebug(channelID, pointID string, raw []byte, parsed any, quality string)
	RecordCycle(channelID string, success bool)
}

// ModbusTransport 实现 Transport 接口
type ModbusTransport struct {
	cfg        model.DriverConfig
	client     *modbus.ModbusClient
	connected  atomic.Bool
	mu         sync.Mutex
	timeout    time.Duration
	maxRetries int
	maxBackoff time.Duration

	connMgr *driver.ConnectionManager

	lastActivityTime atomic.Value
	collectFailCount atomic.Int32
	maxFailCount     int32
	collectCycle     time.Duration

	metricsRecorder MetricsRecorder
	channelID       string

	connectTime        time.Time
	lastDisconnectTime time.Time
	reconnectCount     atomic.Int32
	localAddr          string
	remoteAddr         string

	readRegistersHook     func(ctx context.Context, regType string, offset uint16, count uint16) ([]byte, error)
	readCoilHook          func(ctx context.Context, offset uint16) (bool, error)
	readDiscreteInputHook func(ctx context.Context, offset uint16) (bool, error)
	writeRegisterHook     func(ctx context.Context, offset uint16, value uint16) error
	writeRegistersHook    func(ctx context.Context, offset uint16, values []uint16) error
	writeCoilHook         func(ctx context.Context, offset uint16, value bool) error
}

// SetMetricsRecorder 设置指标收集器
func (t *ModbusTransport) SetMetricsRecorder(recorder MetricsRecorder, channelID string) {
	t.metricsRecorder = recorder
	t.channelID = channelID
}

func NewModbusTransport(cfg model.DriverConfig) *ModbusTransport {
	// Defaults
	timeout := 2 * time.Second
	maxRetries := 3

	// Parse config
	if tVal, ok := cfg.Config["timeout"]; ok {
		if f, ok := tVal.(float64); ok {
			timeout = time.Duration(f) * time.Millisecond
		} else if i, ok := tVal.(int); ok {
			timeout = time.Duration(i) * time.Millisecond
		} else if s, ok := tVal.(string); ok {
			if d, err := time.ParseDuration(s); err == nil {
				timeout = d
			}
		}
	}

	if v, ok := cfg.Config["max_retries"]; ok {
		if f, ok := v.(float64); ok {
			maxRetries = int(f)
		} else if i, ok := v.(int); ok {
			maxRetries = i
		}
	}

	maxBackoff := 30 * time.Second
	if v, ok := cfg.Config["max_backoff"]; ok {
		if f, ok := v.(float64); ok {
			maxBackoff = time.Duration(f) * time.Millisecond
		} else if i, ok := v.(int); ok {
			maxBackoff = time.Duration(i) * time.Millisecond
		}
	}

	maxFailCount := int32(5)
	if v, ok := cfg.Config["max_fail_count"]; ok {
		if f, ok := v.(float64); ok {
			maxFailCount = int32(f)
		} else if i, ok := v.(int); ok {
			maxFailCount = int32(i)
		}
	}

	collectCycle := 10 * time.Second
	if v, ok := cfg.Config["collect_cycle"]; ok {
		if f, ok := v.(float64); ok {
			collectCycle = time.Duration(f) * time.Millisecond
		} else if i, ok := v.(int); ok {
			collectCycle = time.Duration(i) * time.Millisecond
		}
	}

	mt := &ModbusTransport{
		cfg:          cfg,
		timeout:      timeout,
		maxRetries:   maxRetries,
		maxBackoff:   maxBackoff,
		maxFailCount: maxFailCount,
		collectCycle: collectCycle,
	}
	mt.lastActivityTime.Store(time.Now())
	mt.connMgr = driver.NewConnectionManager("modbus")
	mt.connMgr.SetMaxRetries(maxRetries)
	return mt
}

func (t *ModbusTransport) RecordSuccess() {
	t.connMgr.RecordSuccess()
	t.collectFailCount.Store(0)
	t.lastActivityTime.Store(time.Now())
}

func isDeviceLevelModbusError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	devicePatterns := []string{
		"timeout", "i/o timeout", "request timed out",
		"illegal", "exception", "busy",
		"gateway path unavailable", "gateway target device failed",
		"slave device failure", "server device failure", "server device busy",
		"memory parity error", "bad unit id",
		"bad response", "invalid data", "short frame",
		"illegal data address", "illegal function",
	}
	for _, p := range devicePatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func (t *ModbusTransport) RecordFailure(err error) {
	t.lastActivityTime.Store(time.Now())
	if isDeviceLevelModbusError(err) {
		return
	}

	t.collectFailCount.Add(1)
}

func (t *ModbusTransport) IsReconnectExhausted() bool {
	return t.connMgr.GetState() == StateDead
}

func (t *ModbusTransport) ScheduleReconnect() {
	timeout := t.timeout * time.Duration(t.maxRetries)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	// connectOnce dials outside Transport.mu; only the client pointer swap is locked.
	t.connMgr.ScheduleReconnect(context.Background(), timeout, func(ctx context.Context) error {
		return t.connectOnce(ctx)
	})
}

func (t *ModbusTransport) NeedProbeCheck() bool {
	lastSuccess, _ := t.lastActivityTime.Load().(time.Time)
	if lastSuccess.IsZero() {
		return false
	}
	return time.Since(lastSuccess) > t.collectCycle*3
}

func (t *ModbusTransport) ProbeConnection() {
	if !t.connected.Load() {
		return
	}

	client := t.getClient()
	if client == nil && t.readCoilHook == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()

	var err error
	if t.readCoilHook != nil {
		_, err = t.readCoilHook(ctx, 0)
	} else {
		_, err = client.ReadCoil(0)
	}
	if err != nil {
		t.RecordFailure(err)
		if t.collectFailCount.Load() >= t.maxFailCount {
			t.ScheduleReconnect()
		}
	} else {
		t.RecordSuccess()
	}
}

func (t *ModbusTransport) Connect(ctx context.Context) error {
	if t.connected.Load() {
		zap.L().Debug("[Modbus] Connect skipped: already connected")
		return nil
	}

	// Never hold Transport.mu across dial / backoff — that stalls peers on the
	// shared link and turns offline devices into channel-wide freezes.
	return t.connMgr.EnsureConnected(ctx, func(ctx context.Context) error {
		return t.connectOnce(ctx)
	})
}

func (t *ModbusTransport) connectOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Serial parameters for RTU links. The modbus library does NOT parse URL
	// query parameters — Speed/DataBits/Parity/StopBits must be passed via
	// ClientConfiguration struct fields, and the URL must be a bare
	// rtu://<device> locator.
	var serialSpeed, serialDataBits, serialStopBits, serialParity uint
	useSerialConf := false

	// Build URL
	url, ok := t.cfg.Config["url"].(string)
	if !ok || url == "" {
		if port, okPort := t.cfg.Config["port"].(string); okPort && port != "" {
			baudRate := 9600
			if v, ok := t.cfg.Config["baudRate"]; ok {
				if f, ok := v.(float64); ok {
					baudRate = int(f)
				} else if i, ok := v.(int); ok {
					baudRate = i
				}
			}
			dataBits := 8
			if v, ok := t.cfg.Config["dataBits"]; ok {
				if f, ok := v.(float64); ok {
					dataBits = int(f)
				} else if i, ok := v.(int); ok {
					dataBits = i
				}
			}
			stopBits := 1
			if v, ok := t.cfg.Config["stopBits"]; ok {
				if f, ok := v.(float64); ok {
					stopBits = int(f)
				} else if i, ok := v.(int); ok {
					stopBits = i
				}
			}
			parity := "N"
			if v, ok := t.cfg.Config["parity"].(string); ok {
				parity = v
			}
			url = fmt.Sprintf("rtu://%s", port)
			serialSpeed = uint(baudRate)
			serialDataBits = uint(dataBits)
			serialStopBits = uint(stopBits)
			serialParity = parseParity(parity)
			useSerialConf = true
		} else {
			// Try to get address from config
			addr, _ := t.cfg.Config["address"].(string)
			if addr == "" {
				// Try host and port separately for TCP
				host, hostOk := t.cfg.Config["host"].(string)
				portVal, portOk := t.cfg.Config["port"]
				if hostOk && portOk {
					portStr := ""
					switch v := portVal.(type) {
					case string:
						portStr = v
					case float64:
						portStr = fmt.Sprintf("%d", int(v))
					case int:
						portStr = fmt.Sprintf("%d", v)
					}
					if portStr != "" {
						addr = fmt.Sprintf("%s:%s", host, portStr)
					}
				}
			}
			if addr != "" {
				url = "tcp://" + addr
			} else {
				return fmt.Errorf("modbus url or port not configured")
			}
		}
	}

	url = normalizeModbusURL(url)

	// For rtu:// URLs that carry legacy query parameters (e.g. a user-provided
	// url config like rtu:///dev/ttyS3?baudrate=4800&...), parse and strip
	// them — the library treats everything after "://" as the device path.
	if u, perr := neturl.Parse(url); perr == nil && strings.HasPrefix(url, "rtu://") && u.RawQuery != "" {
		q := u.Query()
		if v := q.Get("baudrate"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				serialSpeed = uint(n)
			}
		}
		if v := q.Get("data_bits"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				serialDataBits = uint(n)
			}
		}
		if v := q.Get("stop_bits"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				serialStopBits = uint(n)
			}
		}
		if v := q.Get("parity"); v != "" {
			serialParity = parseParity(v)
		}
		u.RawQuery = ""
		url = u.String()
		useSerialConf = true
	}

	clientConf := &modbus.ClientConfiguration{
		URL:     url,
		Timeout: t.timeout,
	}
	if useSerialConf {
		if serialSpeed != 0 {
			clientConf.Speed = serialSpeed
		}
		if serialDataBits != 0 {
			clientConf.DataBits = serialDataBits
		}
		clientConf.Parity = serialParity
		if serialStopBits != 0 {
			clientConf.StopBits = serialStopBits
		}
		zap.L().Info("[Modbus] RTU serial config",
			zap.Uint("baudRate", clientConf.Speed),
			zap.Uint("dataBits", clientConf.DataBits),
			zap.Uint("parity", clientConf.Parity),
			zap.Uint("stopBits", clientConf.StopBits),
		)
	}

	client, err := modbus.NewClient(clientConf)
	if err != nil {
		zap.L().Warn("[Modbus] Create client failed", zap.Error(err))
		return err
	}

	if err := client.Open(); err != nil {
		zap.L().Warn("[Modbus] Open TCP connection failed", zap.Error(err))
		_ = client.Close()
		return err
	}

	// Resolve local address before taking mu (may dial UDP briefly).
	remoteAddr := url
	if strings.HasPrefix(remoteAddr, "tcp://") {
		remoteAddr = strings.TrimPrefix(remoteAddr, "tcp://")
	} else if strings.HasPrefix(remoteAddr, "rtuovertcp://") {
		remoteAddr = strings.TrimPrefix(remoteAddr, "rtuovertcp://")
	}
	localAddr := getLocalAddr(client)
	if localAddr == "" {
		if strings.Contains(url, "://") && !strings.HasPrefix(url, "rtu://") {
			hostPort := remoteAddr
			udpConn, err := net.DialTimeout("udp", hostPort, 1*time.Second)
			if err == nil {
				localAddr, _, _ = net.SplitHostPort(udpConn.LocalAddr().String())
				udpConn.Close()
			} else {
				localAddr = "Local IP: (Auto)"
			}
		} else {
			localAddr = "Serial Port"
		}
	}

	var sid uint8 = 1
	if slaveID, ok := t.cfg.Config["slave_id"]; ok {
		switch v := slaveID.(type) {
		case int:
			sid = uint8(v)
		case float64:
			sid = uint8(v)
		case uint8:
			sid = v
		}
	}

	t.mu.Lock()
	if t.connected.Load() {
		t.mu.Unlock()
		_ = client.Close()
		return nil
	}
	if t.client != nil {
		_ = t.client.Close()
	}
	t.client = client
	t.client.SetUnitId(sid)
	t.connected.Store(true)
	t.connectTime = time.Now()
	t.remoteAddr = remoteAddr
	t.localAddr = localAddr
	t.mu.Unlock()

	if t.metricsRecorder != nil && t.channelID != "" {
		t.metricsRecorder.RecordConnectionStart(t.channelID)
	}

	if strings.HasPrefix(url, "rtu://") {
		zap.L().Info("[Modbus] Serial (RTU) connection established", zap.String("url", url))
	} else {
		zap.L().Info("[Modbus] TCP connection established", zap.String("url", url))
	}
	return nil
}

// DetectMTU performs a simple binary-search-like probe to determine a safely readable register count
func (t *ModbusTransport) DetectMTU(ctx context.Context) (uint16, error) {
	// Try to find max count between 32 and 125 registers
	min := 32
	max := 125
	best := 0

	lo := min
	hi := max
	for lo <= hi {
		mid := (lo + hi) / 2
		// use ReadRegisters with offset 0; caller should ensure this is safe or server responds
		_, err := t.ReadRegisters(ctx, "HOLDING_REGISTER", 0, uint16(mid))
		if err == nil {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	if best == 0 {
		// fallback to a conservative default
		return 32, nil
	}
	return uint16(best), nil
}

func (t *ModbusTransport) Disconnect() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	wasConnected := t.connected.Load()

	if t.client != nil {
		zap.L().Info("[Modbus] Closing TCP connection")
		_ = t.client.Close()
		t.client = nil
	}

	t.connected.Store(false)
	t.lastDisconnectTime = time.Now()
	t.connMgr.SetState(StateDisconnected)

	if wasConnected && t.metricsRecorder != nil && t.channelID != "" {
		t.reconnectCount.Add(1)
		t.metricsRecorder.RecordReconnect(t.channelID)
	}

	zap.L().Info("[Modbus] Disconnected")
	return nil
}

func (t *ModbusTransport) IsConnected() bool {
	return t.connected.Load()
}

func (t *ModbusTransport) SetUnitID(id uint8) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.client != nil {
		t.client.SetUnitId(id)
	}
}

// GetConnectionMetrics 获取连接指标
func (t *ModbusTransport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	reconnectCount = int64(t.reconnectCount.Load())

	t.mu.Lock()
	wasConnected := t.connected.Load()
	connectTime := t.connectTime
	lastDisconnectTime = t.lastDisconnectTime
	localAddr = t.localAddr
	remoteAddr = t.remoteAddr
	t.mu.Unlock()

	if !wasConnected {
		return 0, reconnectCount, "", "", lastDisconnectTime
	}

	connectionSeconds = int64(time.Since(connectTime).Seconds())

	return
}

func (t *ModbusTransport) withRetry(ctx context.Context, fn func() (any, error)) (any, error) {
	var lastErr error
	startTime := time.Now()

	// Hot path must not dial: offline / reconnect is owned by ScheduleReconnect
	// so a single dead slave cannot stall Scan/API on EnsureConnected backoff.
	if !t.connected.Load() {
		t.ScheduleReconnect()
		return nil, fmt.Errorf("modbus: not connected")
	}

	for i := 0; i <= t.maxRetries; i++ {
		res, err := fn()
		duration := time.Since(startTime)

		if err == nil {
			t.RecordSuccess()

			if t.metricsRecorder != nil && t.channelID != "" {
				t.metricsRecorder.RecordRequest(t.channelID, true, duration, "")
			}

			return res, nil
		}

		lastErr = err
		zap.L().Warn("[Modbus] Operation failed",
			zap.Int("attempt", i+1),
			zap.Int("max_attempts", t.maxRetries+1),
			zap.Error(err),
		)

		errMsg := err.Error()
		errorType := "network"

		if len(errMsg) > 0 {
			if contains(errMsg, "illegal") || contains(errMsg, "exception") || contains(errMsg, "busy") {
				errorType = "exception"
			} else if contains(errMsg, "crc") || contains(errMsg, "CRC") {
				errorType = "crc"
			} else if contains(errMsg, "timeout") {
				errorType = "timeout"
			}
		}

		if isDeviceLevelModbusError(err) {
			zap.L().Debug("[Modbus] Device-level error, keeping TCP connection alive for other slaves on shared link",
				zap.String("error", errMsg),
			)
			if t.metricsRecorder != nil && t.channelID != "" {
				t.metricsRecorder.RecordRequest(t.channelID, false, duration, errorType)
				t.metricsRecorder.RecordError(t.channelID, errorType, "", errMsg)
			}
			break
		}

		isProtocolError := errorType == "exception" || errorType == "crc"

		if t.metricsRecorder != nil && t.channelID != "" && i == t.maxRetries {
			t.metricsRecorder.RecordRequest(t.channelID, false, duration, errorType)
			t.metricsRecorder.RecordError(t.channelID, errorType, "", errMsg)
		}

		if !isProtocolError {
			zap.L().Warn("[Modbus] Network/link error: disconnect and async reconnect (no sync dial on hot path)",
				zap.String("error", errMsg),
			)
			t.RecordFailure(err)
			_ = t.Disconnect()
			t.ScheduleReconnect()
			break
		}
		zap.L().Debug("[Modbus] Protocol error detected, keeping TCP connection alive for other devices on same bus",
			zap.String("error", errMsg),
		)
	}
	return nil, lastErr
}

func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), substr)
}

// getClient snapshots the client pointer under mu; I/O runs without holding mu
// (channelMu or SerialQueue provides serialization — v5.2).
func (t *ModbusTransport) getClient() *modbus.ModbusClient {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.client
}

func (t *ModbusTransport) ReadRegisters(ctx context.Context, regType string, offset uint16, count uint16) ([]byte, error) {
	if t.readRegistersHook != nil {
		return t.readRegistersHook(ctx, regType, offset, count)
	}
	res, err := t.withRetry(ctx, func() (any, error) {
		client := t.getClient()
		if client == nil {
			return nil, fmt.Errorf("client is nil")
		}

		switch regType {
		case "HOLDING_REGISTER", "holding", "holding_register", "HOLDING", "Holding Registers":
			return client.ReadBytes(offset, count*2, modbus.HOLDING_REGISTER)
		case "INPUT_REGISTER", "input", "input_register", "INPUT", "Input Registers":
			return client.ReadBytes(offset, count*2, modbus.INPUT_REGISTER)
		default:
			return nil, fmt.Errorf("unsupported regType for ReadRegisters: %s", regType)
		}
	})
	if err != nil {
		return nil, err
	}
	return res.([]byte), nil
}

func (t *ModbusTransport) ReadCoil(ctx context.Context, offset uint16) (bool, error) {
	if t.readCoilHook != nil {
		return t.readCoilHook(ctx, offset)
	}
	res, err := t.withRetry(ctx, func() (any, error) {
		client := t.getClient()
		if client == nil {
			return nil, fmt.Errorf("client is nil")
		}
		return client.ReadCoil(offset)
	})
	if err != nil {
		return false, err
	}
	return res.(bool), nil
}

func (t *ModbusTransport) ReadDiscreteInput(ctx context.Context, offset uint16) (bool, error) {
	if t.readDiscreteInputHook != nil {
		return t.readDiscreteInputHook(ctx, offset)
	}
	res, err := t.withRetry(ctx, func() (any, error) {
		client := t.getClient()
		if client == nil {
			return nil, fmt.Errorf("client is nil")
		}
		return client.ReadDiscreteInput(offset)
	})
	if err != nil {
		return false, nil
	}
	return res.(bool), nil
}

// ReadCustom 使用自定义功能码读取数据（暂不支持）
func (t *ModbusTransport) ReadCustom(ctx context.Context, funcCode byte, offset uint16, count uint16) ([]byte, error) {
	return nil, fmt.Errorf("custom function code not supported, please use standard register types (holding/input)")
}

func (t *ModbusTransport) WriteRegister(ctx context.Context, offset uint16, value uint16) error {
	if t.writeRegisterHook != nil {
		return t.writeRegisterHook(ctx, offset, value)
	}
	_, err := t.withRetry(ctx, func() (any, error) {
		client := t.getClient()
		if client == nil {
			return nil, fmt.Errorf("client is nil")
		}
		return nil, client.WriteRegister(offset, value)
	})
	return err
}

// getLocalAddr 使用反射从 modbus.ModbusClient 中提取真实的本地连接地址
func getLocalAddr(client *modbus.ModbusClient) string {
	if client == nil {
		return ""
	}

	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("[Modbus] Recovered from reflection error in getLocalAddr", zap.Any("error", r))
		}
	}()

	// 1. 获取 ModbusClient 结构体中的 transport 字段
	vClient := reflect.ValueOf(client).Elem()
	fTransport := vClient.FieldByName("transport")
	if !fTransport.IsValid() {
		return ""
	}

	// 2. 使用 unsafe 获取私有字段 transport 的值
	ptrTransport := unsafe.Pointer(fTransport.UnsafeAddr())
	vTransport := reflect.NewAt(fTransport.Type(), ptrTransport).Elem()
	transport := vTransport.Interface()

	if transport == nil {
		return ""
	}

	// 3. transport 是 internal.transport 接口，其实际类型通常为 *modbus.tcpTransport 或 *modbus.rtuTransport
	vTransportConcrete := reflect.ValueOf(transport)
	if vTransportConcrete.Kind() == reflect.Ptr {
		vTransportConcrete = vTransportConcrete.Elem()
	}

	if vTransportConcrete.Kind() != reflect.Struct {
		return ""
	}

	// 4. 情况 A: tcpTransport (对应 modbusTCP)
	// tcpTransport 结构体中有 socket net.Conn 字段
	fSocket := vTransportConcrete.FieldByName("socket")
	if fSocket.IsValid() {
		ptrSocket := unsafe.Pointer(fSocket.UnsafeAddr())
		vSocket := reflect.NewAt(fSocket.Type(), ptrSocket).Elem()
		if conn, ok := vSocket.Interface().(net.Conn); ok && conn != nil {
			return conn.LocalAddr().String()
		}
	}

	// 5. 情况 B: rtuTransport (对应 modbusRTUOverTCP 等)
	// rtuTransport 结构体中有 link rtuLink 字段
	fLink := vTransportConcrete.FieldByName("link")
	if fLink.IsValid() {
		ptrLink := unsafe.Pointer(fLink.UnsafeAddr())
		vLink := reflect.NewAt(fLink.Type(), ptrLink).Elem()
		link := vLink.Interface()
		if link != nil {
			// 尝试调用 LocalAddr() 方法（如果实现类（如 tcpSockWrapper）提供了该方法）
			mLocalAddr := reflect.ValueOf(link).MethodByName("LocalAddr")
			if mLocalAddr.IsValid() {
				results := mLocalAddr.Call(nil)
				if len(results) > 0 {
					if addr, ok := results[0].Interface().(net.Addr); ok && addr != nil {
						return addr.String()
					}
				}
			}

			// 如果方法不存在，尝试从其包装类中提取私有 socket
			vLinkConcrete := reflect.ValueOf(link)
			if vLinkConcrete.Kind() == reflect.Ptr {
				vLinkConcrete = vLinkConcrete.Elem()
			}
			if vLinkConcrete.Kind() == reflect.Struct {
				// 尝试提取 'sock' (tlsSockWrapper/udpSockWrapper 等使用)
				fSock := vLinkConcrete.FieldByName("sock")
				if fSock.IsValid() {
					ptrSock := unsafe.Pointer(fSock.UnsafeAddr())
					vSock := reflect.NewAt(fSock.Type(), ptrSock).Elem()
					if conn, ok := vSock.Interface().(net.Conn); ok && conn != nil {
						return conn.LocalAddr().String()
					}
				}
			}
		}
	}

	return ""
}

func (t *ModbusTransport) WriteRegisters(ctx context.Context, offset uint16, values []uint16) error {
	if t.writeRegistersHook != nil {
		return t.writeRegistersHook(ctx, offset, values)
	}
	_, err := t.withRetry(ctx, func() (any, error) {
		client := t.getClient()
		if client == nil {
			return nil, fmt.Errorf("client is nil")
		}
		return nil, client.WriteRegisters(offset, values)
	})
	return err
}

func (t *ModbusTransport) WriteCoil(ctx context.Context, offset uint16, value bool) error {
	if t.writeCoilHook != nil {
		return t.writeCoilHook(ctx, offset, value)
	}
	_, err := t.withRetry(ctx, func() (any, error) {
		client := t.getClient()
		if client == nil {
			return nil, fmt.Errorf("client is nil")
		}
		return nil, client.WriteCoil(offset, value)
	})
	return err
}

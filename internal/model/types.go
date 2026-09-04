package model

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
)

type Duration time.Duration

// RegisterType 定义寄存器类型
type RegisterType int

const (
	RegHolding       RegisterType = iota // 0: 默认Holding寄存器 (03)
	RegCoil                              // 1: Coil (01)
	RegDiscreteInput                     // 2: Discrete Input (02)
	RegInput                             // 3: Input Register (04)
	RegCustom                            // 4: 非标准
)

func (r RegisterType) FunctionCode() byte {
	switch r {
	case RegCoil:
		return 1
	case RegDiscreteInput:
		return 2
	case RegHolding:
		return 3
	case RegInput:
		return 4
	case RegCustom:
		return 0 // 由配置指定
	}
	return 0
}

func (r RegisterType) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r RegisterType) MarshalYAML() (interface{}, error) {
	return r.String(), nil
}

func (r RegisterType) String() string {
	switch r {
	case RegCoil:
		return "Coils (outputs)"
	case RegDiscreteInput:
		return "Discrete Inputs"
	case RegHolding:
		return "Holding Registers"
	case RegInput:
		return "Input Registers"
	case RegCustom:
		return "Custom"
	}
	return "Unknown"
}

// Code 返回Modbus功能码
func (r RegisterType) Code() byte {
	switch r {
	case RegCoil:
		return 1
	case RegDiscreteInput:
		return 2
	case RegHolding:
		return 3
	case RegInput:
		return 4
	case RegCustom:
		return 0
	}
	return 0
}

// ShortString 返回短名称
func (r RegisterType) ShortString() string {
	switch r {
	case RegCoil:
		return "coil"
	case RegDiscreteInput:
		return "discrete_input"
	case RegHolding:
		return "holding"
	case RegInput:
		return "input"
	case RegCustom:
		return "custom"
	}
	return "unknown"
}

func (r *RegisterType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		var n int
		if err := json.Unmarshal(data, &n); err != nil {
			return err
		}
		*r = RegisterType(n)
		return nil
	}
	*r = ParseRegisterType(s)
	return nil
}

func (r *RegisterType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		var n int
		if err := unmarshal(&n); err != nil {
			return err
		}
		*r = RegisterType(n)
		return nil
	}
	*r = ParseRegisterType(s)
	return nil
}

func ParseRegisterType(s string) RegisterType {
	switch s {
	case "coil", "1", "COIL", "Coil", "Coils (outputs)", "Coils":
		return RegCoil
	case "discrete_input", "2", "DISCRETE_INPUT", "DiscreteInput", "discrete", "Discrete Inputs":
		return RegDiscreteInput
	case "holding", "3", "HOLDING", "Holding", "holding_register", "4x", "Holding Registers", "HoldingRegister":
		return RegHolding
	case "input", "4", "INPUT", "Input", "input_register", "3x", "Input Registers", "InputRegister":
		return RegInput
	case "custom", "0", "Custom":
		return RegCustom
	}
	return RegHolding
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	val, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(val)
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
	case string:
		tmp, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(tmp)
	default:
		return errors.New("invalid duration")
	}
	return nil
}

// Point represents a data point configuration (Tag/Variable)
type Point struct {
	ID           string           `json:"id" yaml:"id"`
	Name         string           `json:"name" yaml:"name"`
	RegisterType RegisterType     `json:"register_type" yaml:"register_type"`
	FunctionCode byte             `json:"function_code" yaml:"function_code"`               // 允许非标准功能码 (当前会将配置初始化值设置为0)
	Address      string           `json:"address" yaml:"address"`                           // 地址字符串，支持不同协议格式
	DataType     string           `json:"datatype" yaml:"datatype"`                         // int16, float32, bool, bit.0
	ParseType    string           `json:"parse_type,omitempty" yaml:"parse_type,omitempty"` // 原始字节解析类型，如 INT16、FLOAT32
	Scale        float64          `json:"scale" yaml:"scale"`
	Offset       float64          `json:"offset" yaml:"offset"`
	Format       string           `json:"format,omitempty" yaml:"format,omitempty"`
	WordOrder    string           `json:"word_order,omitempty" yaml:"word_order,omitempty"`
	ReadFormula  string           `json:"read_formula,omitempty" yaml:"read_formula,omitempty"`
	WriteFormula string           `json:"write_formula,omitempty" yaml:"write_formula,omitempty"`
	Unit         string           `json:"unit" yaml:"unit"`
	ReadWrite    string           `json:"readwrite" yaml:"readwrite"` // R / RW
	Group        string           `json:"group" yaml:"group"`
	ScanClass    string           `json:"scan_class,omitempty" yaml:"scan_class,omitempty"` // fast / normal / slow
	ReportMode   string           `json:"report_mode" yaml:"report_mode"`                   // cycle / cov / event
	Threshold    *ThresholdConfig `json:"threshold" yaml:"threshold"`
	DeviceID     string           `json:"-" yaml:"-"` // Runtime field, not persisted
}

// RawDataType returns the protocol payload type. ParseType is authoritative for
// byte-oriented protocols; DataType remains the converted output type.
func RawDataType(point Point) string {
	if point.ParseType == "" {
		return strings.ToLower(point.DataType)
	}
	switch strings.ToUpper(point.ParseType) {
	case "INT8":
		return "int8"
	case "UINT8", "BCD8":
		return "uint8"
	case "INT16", "INT16_SWAP":
		return "int16"
	case "UINT16", "UINT16_SWAP", "BCD16":
		return "uint16"
	case "INT32", "INT32_SWAP":
		return "int32"
	case "UINT32", "UINT32_SWAP", "BCD32":
		return "uint32"
	case "FLOAT32", "FLOAT32_SWAP":
		return "float32"
	case "INT64":
		return "int64"
	case "UINT64":
		return "uint64"
	case "FLOAT64", "FLOAT64_SWAP":
		return "float64"
	}
	return strings.ToLower(point.DataType)
}

// ThresholdConfig defines alarm thresholds for a point
type ThresholdConfig struct {
	High float64 `json:"high" yaml:"high"`
	Low  float64 `json:"low" yaml:"low"`
}

// Value represents the standardized output of a collected point
type Value struct {
	ChannelID string         `json:"channel_id"`
	DeviceID  string         `json:"device_id"`
	PointID   string         `json:"point_id"`
	Value     any            `json:"value"`
	Quality   string         `json:"quality"`
	TS        time.Time      `json:"timestamp"`
	CachedAt  time.Time      `json:"cachedAt,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
}

// PointData represents point configuration and current value for frontend display
type PointData struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SlaveID      uint8     `json:"slave_id"`
	RegisterType string    `json:"register_type"`
	FunctionCode byte      `json:"function_code"`
	Address      string    `json:"address"`
	DataType     string    `json:"datatype"`
	ParseType    string    `json:"parse_type,omitempty"`
	Value        any       `json:"value"`
	Quality      string    `json:"quality"`
	Timestamp    time.Time `json:"timestamp"`    // 采集时间（兼容旧字段）
	CollectedAt  time.Time `json:"collected_at"` // 采集时间
	UpdatedAt    time.Time `json:"updated_at"`   // 影子更新时间
	Unit         string    `json:"unit,omitempty"`
	ReadWrite    string    `json:"readwrite"` // R / RW
	// 点位配置字段（与 Point 对齐），供点位列表接口透传给前端，
	// 避免"设备协议类型(format)"、编辑回显等依赖配置的展示丢失
	Format       string  `json:"format,omitempty"`
	WordOrder    string  `json:"word_order,omitempty"`
	Scale        float64 `json:"scale,omitempty"`
	Offset       float64 `json:"offset,omitempty"`
	ReadFormula  string  `json:"read_formula,omitempty"`
	WriteFormula string  `json:"write_formula,omitempty"`
	Group        string  `json:"group,omitempty"`
	ScanClass    string  `json:"scan_class,omitempty"`
	ReportMode   string  `json:"report_mode,omitempty"`
}

// DeviceStorage defines data storage strategy for a device
type DeviceStorage struct {
	Enable     bool   `json:"enable" yaml:"enable"`
	Strategy   string `json:"strategy" yaml:"strategy"`       // "realtime" (every record), "interval" (fixed time)
	Interval   int    `json:"interval" yaml:"interval"`       // Storage interval in minutes (1, 10, 60)
	MaxRecords int    `json:"max_records" yaml:"max_records"` // Max history records (default 1000)
}

// Device represents a device configuration (within a channel)
type Device struct {
	ID       string   `json:"id" yaml:"id"`
	Name     string   `json:"name" yaml:"name"`
	Enable   bool     `json:"enable" yaml:"enable"`
	Interval Duration `json:"interval" yaml:"interval"`
	// Spatial attributes / 空间属性 - 物理位置信息，用于资产管理和设备定位
	StationName      string         `json:"station_name,omitempty" yaml:"station_name,omitempty"`             // 局站名称 / Station name
	StationCode      string         `json:"station_code,omitempty" yaml:"station_code,omitempty"`             // 局站编码 / Station code
	RoomName         string         `json:"room_name,omitempty" yaml:"room_name,omitempty"`                   // 机房名称 / Equipment room name
	RoomCode         string         `json:"room_code,omitempty" yaml:"room_code,omitempty"`                   // 机房编码 / Equipment room code
	DegradeOnFailure *bool          `json:"degrade_on_failure,omitempty" yaml:"degrade_on_failure,omitempty"` // 默认 true；设为 false 关闭失败退避
	DeviceFile       string         `json:"device_file,omitempty" yaml:"device_file,omitempty"`               // 设备配置文件路径
	Config           map[string]any `json:"config" yaml:"config"`                                             // 设备特定配置（如 slave_id）
	Storage          DeviceStorage  `json:"storage,omitempty" yaml:"storage,omitempty"`                       // Data storage strategy
	Points           []Point        `json:"points,omitempty" yaml:"points,omitempty"`                         // 该设备的点位列表
	PointsCount      int            `json:"points_count,omitempty" yaml:"-"`                                  // 列表 API 省略 points 时返回点位数量
	State            int            `json:"state" yaml:"-"`                                                   // 运行时状态：0=Online, 1=Unstable, 2=Offline, 3=Quarantine
	QualityScore     int            `json:"quality_score" yaml:"-"`                                           // 质量评分 (0-100)
	// Runtime state fields
	NodeRuntime *NodeRuntime `json:"runtime,omitempty" yaml:"-"`
}

// WithPointsSummary returns a copy with Points stripped and PointsCount populated.
// Used by list APIs to avoid shipping large point payloads to the UI.
func (d Device) WithPointsSummary() Device {
	out := d
	out.PointsCount = len(d.Points)
	out.Points = nil
	return out
}

// DeepCopy returns a deep copy of a Device so callers may mutate the result
// (Points, Config, DegradeOnFailure, NodeRuntime) without corrupting the
// channel manager's internal state. Without this, the Points slice and Config
// map share backing storage with the stored device, so MCP/API update handlers
// mutate the live configuration before the change-diff / restart decision runs.
func (d Device) DeepCopy() Device {
	out := d
	if d.Config != nil {
		out.Config = cloneAnyMap(d.Config)
	}
	if d.DegradeOnFailure != nil {
		v := *d.DegradeOnFailure
		out.DegradeOnFailure = &v
	}
	if d.NodeRuntime != nil {
		nr := *d.NodeRuntime
		out.NodeRuntime = &nr
	}
	if d.Points != nil {
		pts := make([]Point, len(d.Points))
		for i, p := range d.Points {
			pts[i] = p
			if p.Threshold != nil {
				th := *p.Threshold
				pts[i].Threshold = &th
			}
		}
		out.Points = pts
	}
	return out
}

// DeepCopy returns an independent copy of a Channel configuration.
func (c Channel) DeepCopy() Channel {
	out := c
	out.Config = cloneAnyMap(c.Config)
	if c.Devices != nil {
		out.Devices = make([]Device, len(c.Devices))
		for i, device := range c.Devices {
			out.Devices[i] = device.DeepCopy()
		}
	}
	if c.NodeRuntime != nil {
		runtime := *c.NodeRuntime
		out.NodeRuntime = &runtime
	}
	return out
}

// ClonePoint returns a deep copy of a Point (duplicates the Threshold pointer).
func (p Point) Clone() Point {
	out := p
	if p.Threshold != nil {
		th := *p.Threshold
		out.Threshold = &th
	}
	return out
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneAny(item)
		}
		return out
	}
	valueRef := reflect.ValueOf(value)
	if !valueRef.IsValid() {
		return nil
	}
	if valueRef.Kind() == reflect.Slice {
		out := reflect.MakeSlice(valueRef.Type(), valueRef.Len(), valueRef.Len())
		reflect.Copy(out, valueRef)
		for i := 0; i < out.Len(); i++ {
			item := cloneAny(out.Index(i).Interface())
			if item != nil {
				out.Index(i).Set(reflect.ValueOf(item))
			}
		}
		return out.Interface()
	}
	if valueRef.Kind() == reflect.Map && valueRef.Type().Key().Kind() == reflect.String {
		out := reflect.MakeMapWithSize(valueRef.Type(), valueRef.Len())
		iter := valueRef.MapRange()
		for iter.Next() {
			item := cloneAny(iter.Value().Interface())
			if item == nil {
				out.SetMapIndex(iter.Key(), reflect.Zero(valueRef.Type().Elem()))
			} else {
				out.SetMapIndex(iter.Key(), reflect.ValueOf(item))
			}
		}
		return out.Interface()
	}
	return value
}

// NodeRuntime defines runtime statistics for a node (device or channel)
type NodeRuntime struct {
	FailCount     int       `json:"fail_count"`
	SuccessCount  int       `json:"success_count"`
	LastFailTime  time.Time `json:"last_fail_time"`
	NextRetryTime time.Time `json:"next_retry_time"`
	State         int       `json:"state"` // NodeState enum
}

// Channel represents a collection channel (采集通道)
// 一个通道对应一个采集驱动 (如 Modbus TCP, S7, Modbus RTU 等)
type Channel struct {
	ID       string         `json:"id" yaml:"id"`
	Name     string         `json:"name" yaml:"name"`
	Protocol string         `json:"protocol" yaml:"protocol"` // modbus-tcp, modbus-rtu, s7, opc-ua, etc.
	Enable   bool           `json:"enable" yaml:"enable"`
	Config   map[string]any `json:"config" yaml:"config"`   // 协议特定配置 (IP, Port, etc.)
	Devices  []Device       `json:"devices" yaml:"devices"` // 该通道下的设备列表
	// Runtime fields
	NodeRuntime *NodeRuntime `json:"runtime,omitempty" yaml:"-"`
}

// DriverConfig is the configuration passed to a driver
type DriverConfig struct {
	ChannelID string         `json:"channel_id"`
	Protocol  string         `json:"protocol"` // Protocol name (e.g. modbus-tcp, modbus-rtu)
	Config    map[string]any `json:"config"`
}

// NorthboundConfig defines configuration for northbound data reporting
type NorthboundConfig struct {
	MQTT         []MQTTConfig         `json:"mqtt" yaml:"mqtt"`
	HTTP         []HTTPConfig         `json:"http" yaml:"http"`
	OPCUA        []OPCUAConfig        `json:"opcua" yaml:"opcua"`
	SparkplugB   []SparkplugBConfig   `json:"sparkplug_b" yaml:"sparkplug_b"`
	EdgeOSMQTT   []EdgeOSMQTTConfig   `json:"edgeos_mqtt" yaml:"edgeos_mqtt"`
	EdgeOSNATS   []EdgeOSNATSConfig   `json:"edgeos_nats" yaml:"edgeos_nats"`
	BACnetServer []BACnetServerConfig `json:"bacnet_server" yaml:"bacnet_server"`
	Status       map[string]int       `json:"status,omitempty" yaml:"-"`
}

type DataCacheConfig struct {
	Enable        bool   `json:"enable" yaml:"enable"`
	MaxCount      int    `json:"max_count" yaml:"max_count"`           // Default 1000
	FlushInterval string `json:"flush_interval" yaml:"flush_interval"` // e.g. "1m"
}

type MQTTConfig struct {
	ID             string `json:"id" yaml:"id"`
	Name           string `json:"name" yaml:"name"`
	Enable         bool   `json:"enable" yaml:"enable"`
	Broker         string `json:"broker" yaml:"broker"`
	ClientID       string `json:"client_id" yaml:"client_id"`
	Topic          string `json:"topic" yaml:"topic"`
	SubscribeTopic string `json:"subscribe_topic" yaml:"subscribe_topic"` // New: Subscribe topic for write requests

	StatusTopic          string `json:"status_topic" yaml:"status_topic"`                     // Online/Offline status topic
	LwtTopic             string `json:"lwt_topic" yaml:"lwt_topic"`                           // LWT topic (if different from StatusTopic)
	DeviceStatusTopic    string `json:"device_status_topic" yaml:"device_status_topic"`       // Sub-device Online/Offline topic
	DeviceLifecycleTopic string `json:"device_lifecycle_topic" yaml:"device_lifecycle_topic"` // Sub-device Add/Remove topic
	OnlinePayload        string `json:"online_payload" yaml:"online_payload"`                 // Payload for online status
	OfflinePayload       string `json:"offline_payload" yaml:"offline_payload"`               // Payload for offline status (graceful disconnect)
	LwtPayload           string `json:"lwt_payload" yaml:"lwt_payload"`                       // Payload for LWT (ungraceful disconnect)
	IgnoreOfflineData    bool   `json:"ignore_offline_data" yaml:"ignore_offline_data"`       // If true, do not report data when device is offline

	WriteResponseTopic string `json:"write_response_topic" yaml:"write_response_topic"` // Topic for write responses

	Username       string                         `json:"username" yaml:"username"`
	Password       string                         `json:"password" yaml:"password"`
	Cache          DataCacheConfig                `json:"cache" yaml:"cache"`
	Devices        map[string]DevicePublishConfig `json:"devices" yaml:"devices"`
	VirtualDevices OpcUaDeviceMap                 `json:"virtual_devices" yaml:"virtual_devices"`
}

type HTTPConfig struct {
	ID                  string            `json:"id" yaml:"id"`
	Name                string            `json:"name" yaml:"name"`
	Enable              bool              `json:"enable" yaml:"enable"`
	URL                 string            `json:"url" yaml:"url"`       // Base URL
	Method              string            `json:"method" yaml:"method"` // POST/PUT
	Headers             map[string]string `json:"headers" yaml:"headers"`
	AuthType            string            `json:"auth_type" yaml:"auth_type"` // None, Basic, Bearer, APIKey
	Username            string            `json:"username" yaml:"username"`
	Password            string            `json:"password" yaml:"password"`
	Token               string            `json:"token" yaml:"token"`
	APIKeyName          string            `json:"api_key_name" yaml:"api_key_name"`
	APIKeyValue         string            `json:"api_key_value" yaml:"api_key_value"`
	DataEndpoint        string            `json:"data_endpoint" yaml:"data_endpoint"`                 // Relative path for data
	DeviceEventEndpoint string            `json:"device_event_endpoint" yaml:"device_event_endpoint"` // Relative path for events
	Cache               DataCacheConfig   `json:"cache" yaml:"cache"`
	Devices             OpcUaDeviceMap    `json:"devices" yaml:"devices"` // Key: DeviceID; legacy bool or DevicePublishConfig
	VirtualDevices      OpcUaDeviceMap    `json:"virtual_devices" yaml:"virtual_devices"`
}

type DevicePublishConfig struct {
	Enable   bool     `json:"enable" yaml:"enable"`
	Strategy string   `json:"strategy" yaml:"strategy"` // "realtime" or "periodic"
	Interval Duration `json:"interval" yaml:"interval"` // Push interval for periodic mode (e.g., "5s", "1m")
}

type OPCUAConfig struct {
	ID              string            `json:"id" yaml:"id"`
	Name            string            `json:"name" yaml:"name"`
	Enable          bool              `json:"enable" yaml:"enable"`
	Port            int               `json:"port" yaml:"port"`
	Endpoint        string            `json:"endpoint" yaml:"endpoint"`
	SecurityPolicy  string            `json:"security_policy" yaml:"security_policy"` // Auto, None, Basic256Sha256, Basic256, Basic128Rsa15, Aes128_Sha256_RsaOaep, Aes256Sha256RsaPss
	SecurityMode    string            `json:"security_mode" yaml:"security_mode"`     // Auto, None, Sign, SignAndEncrypt
	TrustedCertPath string            `json:"trusted_cert_path" yaml:"trusted_cert_path"`
	AuthMethods     []string          `json:"auth_methods" yaml:"auth_methods"` // "Anonymous", "UserName", "Certificate"
	Users           map[string]string `json:"users" yaml:"users"`               // Username -> Password
	CertFile        string            `json:"cert_file" yaml:"cert_file"`       // legacy path fallback
	KeyFile         string            `json:"key_file" yaml:"key_file"`         // legacy path fallback
	// ServerCertPEM / ServerKeyPEM 持久化在北向配置（edge.db），启动时物化到 data/certs/opcua/{id}/
	ServerCertPEM   string         `json:"server_cert_pem,omitempty" yaml:"server_cert_pem,omitempty"`
	ServerKeyPEM    string         `json:"server_key_pem,omitempty" yaml:"server_key_pem,omitempty"`
	TrustedCertsPEM []string       `json:"trusted_certs_pem,omitempty" yaml:"trusted_certs_pem,omitempty"`
	HasServerCert   bool           `json:"has_server_cert,omitempty" yaml:"-"`
	HasServerKey    bool           `json:"has_server_key,omitempty" yaml:"-"`
	Devices         OpcUaDeviceMap `json:"devices" yaml:"devices"`                 // Key: DeviceID, Value: enable/strategy/interval
	VirtualDevices  OpcUaDeviceMap `json:"virtual_devices" yaml:"virtual_devices"` // Key: VirtualShadowDeviceID, Value: enable
}

type SparkplugBConfig struct {
	ID             string         `json:"id" yaml:"id"`
	Name           string         `json:"name" yaml:"name"`
	Enable         bool           `json:"enable" yaml:"enable"`
	ClientID       string         `json:"client_id" yaml:"client_id"`
	GroupID        string         `json:"group_id" yaml:"group_id"`
	NodeID         string         `json:"node_id" yaml:"node_id"`
	EnableAlias    bool           `json:"enable_alias" yaml:"enable_alias"`
	GroupPath      bool           `json:"group_path" yaml:"group_path"`
	OfflineCache   bool           `json:"offline_cache" yaml:"offline_cache"`
	CacheMemSize   int            `json:"cache_mem_size" yaml:"cache_mem_size"`
	CacheDiskSize  int            `json:"cache_disk_size" yaml:"cache_disk_size"`
	CacheResendInt int            `json:"cache_resend_int" yaml:"cache_resend_int"`
	Broker         string         `json:"broker" yaml:"broker"`
	Port           int            `json:"port" yaml:"port"`
	Username       string         `json:"username" yaml:"username"`
	Password       string         `json:"password" yaml:"password"`
	SSL            bool           `json:"ssl" yaml:"ssl"`
	CACert         string         `json:"ca_cert" yaml:"ca_cert"`
	ClientCert     string         `json:"client_cert" yaml:"client_cert"`
	ClientKey      string         `json:"client_key" yaml:"client_key"`
	KeyPassword    string         `json:"key_password" yaml:"key_password"`
	Devices        OpcUaDeviceMap `json:"devices" yaml:"devices"` // Key: DeviceID; legacy bool or DevicePublishConfig
	VirtualDevices OpcUaDeviceMap `json:"virtual_devices" yaml:"virtual_devices"`
}

// EdgeOSMQTTConfig defines configuration for edgeOS(MQTT) northbound channel
type EdgeOSMQTTConfig struct {
	ID                   string                         `json:"id" yaml:"id"`
	Name                 string                         `json:"name" yaml:"name"`
	Enable               bool                           `json:"enable" yaml:"enable"`
	Broker               string                         `json:"broker" yaml:"broker"`
	ClientID             string                         `json:"client_id" yaml:"client_id"`
	NodeID               string                         `json:"node_id" yaml:"node_id"`
	Username             string                         `json:"username" yaml:"username"`
	Password             string                         `json:"password" yaml:"password"`
	QoS                  byte                           `json:"qos" yaml:"qos"`
	Retain               bool                           `json:"retain" yaml:"retain"`
	CleanSession         bool                           `json:"clean_session" yaml:"clean_session"`
	KeepAlive            int                            `json:"keep_alive" yaml:"keep_alive"`
	ConnectTimeout       int                            `json:"connect_timeout" yaml:"connect_timeout"`
	AutoReconnect        bool                           `json:"auto_reconnect" yaml:"auto_reconnect"`
	MaxReconnectInterval int                            `json:"max_reconnect_interval" yaml:"max_reconnect_interval"`
	HeartbeatInterval    string                         `json:"heartbeat_interval" yaml:"heartbeat_interval"` // e.g. "30s" V1 心跳
	Devices              map[string]DevicePublishConfig `json:"devices" yaml:"devices"`                       // Key: DeviceID, Value: DevicePublishConfig
	VirtualDevices       OpcUaDeviceMap                 `json:"virtual_devices" yaml:"virtual_devices"`

	// EAN 2.0 能力层配置 | EAN 2.0 capability layer config
	// EANEnabled 控制北向 EAN Runtime 启停；MCP Runtime 不受此字段影响
	EANEnabled bool `json:"ean_enabled" yaml:"ean_enabled"`
	// EANHeartbeatSec EAN 心跳周期（秒），0 时使用默认值 60
	EANHeartbeatSec int `json:"ean_heartbeat_sec" yaml:"ean_heartbeat_sec"`
	// EANEventAutoPublish 设备数据变化时是否自动发布 EAN Event
	EANEventAutoPublish bool `json:"ean_event_auto_publish" yaml:"ean_event_auto_publish"`

	// Phase 4 (EX-P4): V1 命令面开关——已全面下线，默认 false（不订阅 edgeCore/cmd/*）；
	// 命令统一 EAN Invoke（$edgeos/invoke/*）。V1 数据面/告警不受影响。
	// V1 command plane switch (Phase 4): default false - V1 command plane fully retired; commands use EAN Invoke.
	V1CommandEnabled bool `json:"v1_command_enabled" yaml:"v1_command_enabled"`
}

// EdgeOSNATSConfig defines configuration for edgeOS(NATS) northbound channel
type EdgeOSNATSConfig struct {
	ID                  string                         `json:"id" yaml:"id"`
	Name                string                         `json:"name" yaml:"name"`
	Enable              bool                           `json:"enable" yaml:"enable"`
	URL                 string                         `json:"url" yaml:"url"`
	ClientID            string                         `json:"client_id" yaml:"client_id"`
	NodeID              string                         `json:"node_id" yaml:"node_id"`
	Username            string                         `json:"username" yaml:"username"`
	Password            string                         `json:"password" yaml:"password"`
	Token               string                         `json:"token" yaml:"token"`
	ConnectTimeout      int                            `json:"connect_timeout" yaml:"connect_timeout"`
	ReconnectWait       int                            `json:"reconnect_wait" yaml:"reconnect_wait"`
	MaxReconnects       int                            `json:"max_reconnects" yaml:"max_reconnects"`
	PingInterval        int                            `json:"ping_interval" yaml:"ping_interval"`
	MaxPingsOutstanding int                            `json:"max_pings_outstanding" yaml:"max_pings_outstanding"`
	JetStreamEnabled    bool                           `json:"jetstream_enabled" yaml:"jetstream_enabled"`
	HeartbeatInterval   string                         `json:"heartbeat_interval" yaml:"heartbeat_interval"` // e.g. "30s" V1 心跳
	Devices             map[string]DevicePublishConfig `json:"devices" yaml:"devices"`                       // Key: DeviceID, Value: DevicePublishConfig
	VirtualDevices      OpcUaDeviceMap                 `json:"virtual_devices" yaml:"virtual_devices"`

	// EAN 2.0 能力层配置 | EAN 2.0 capability layer config
	// EANEnabled 控制北向 EAN Runtime 启停；MCP Runtime 不受此字段影响
	EANEnabled bool `json:"ean_enabled" yaml:"ean_enabled"`
	// EANHeartbeatSec EAN 心跳周期（秒），0 时使用默认值 60
	EANHeartbeatSec int `json:"ean_heartbeat_sec" yaml:"ean_heartbeat_sec"`
	// EANEventAutoPublish 设备数据变化时是否自动发布 EAN Event
	EANEventAutoPublish bool `json:"ean_event_auto_publish" yaml:"ean_event_auto_publish"`

	// Phase 4 (EX-P4): V1 命令面开关——已全面下线，默认 false（不订阅 edgeCore.cmd.*）；
	// 命令统一 EAN Invoke（$edgeos/invoke/*）。V1 数据面/告警不受影响。
	// V1 command plane switch (Phase 4): default false - V1 command plane fully retired; commands use EAN Invoke.
	V1CommandEnabled bool `json:"v1_command_enabled" yaml:"v1_command_enabled"`
}

// BACnetServerConfig 北向 BACnet Server 配置，以从机模式运行，对外暴露点位数据
type BACnetServerConfig struct {
	ID             string         `json:"id" yaml:"id"`
	Name           string         `json:"name" yaml:"name"`
	Enable         bool           `json:"enable" yaml:"enable"`
	Interface      string         `json:"interface" yaml:"interface"`             // 网络接口名 (如 eth0)，空则自动选择
	IP             string         `json:"ip" yaml:"ip"`                           // 绑定的 IP 地址，空则自动选择
	Port           int            `json:"port" yaml:"port"`                       // BACnet 端口，默认 47808 (0xBAC0)
	SubnetCIDR     int            `json:"subnet_cidr" yaml:"subnet_cidr"`         // 子网 CIDR，默认 24
	DeviceID       int            `json:"device_id" yaml:"device_id"`             // BACnet 设备实例 ID，默认自动生成
	DeviceName     string         `json:"device_name" yaml:"device_name"`         // BACnet 设备名称
	VendorID       uint32         `json:"vendor_id" yaml:"vendor_id"`             // 厂商 ID，默认 999
	VendorName     string         `json:"vendor_name" yaml:"vendor_name"`         // 厂商名称
	MaxPDU         uint16         `json:"max_pdu" yaml:"max_pdu"`                 // 最大 PDU 大小，默认 1476
	Devices        OpcUaDeviceMap `json:"devices" yaml:"devices"`                 // 暴露的设备列表，空则全部暴露
	VirtualDevices OpcUaDeviceMap `json:"virtual_devices" yaml:"virtual_devices"` // 虚拟设备
}

// EdgeRule represents an edge computing rule
type EdgeRule struct {
	ID            string        `json:"id" yaml:"id"`
	Name          string        `json:"name" yaml:"name"`
	Type          string        `json:"type" yaml:"type"` // threshold, calculation, state, window
	Enable        bool          `json:"enable" yaml:"enable"`
	Priority      int           `json:"priority" yaml:"priority"`
	CheckInterval string        `json:"check_interval" yaml:"check_interval"` // e.g. "5s", "1m"
	TriggerMode   string        `json:"trigger_mode" yaml:"trigger_mode"`     // always, on_change
	Source        RuleSource    `json:"source" yaml:"source"`                 // Deprecated: use Sources
	Sources       []RuleSource  `json:"sources" yaml:"sources"`               // New: Multiple sources
	TriggerLogic  string        `json:"trigger_logic" yaml:"trigger_logic"`   // "AND", "OR", "EXPR"
	Condition     string        `json:"condition" yaml:"condition"`           // Boolean Expression
	Expression    string        `json:"expression" yaml:"expression"`         // Calculation Expression
	Actions       []RuleAction  `json:"actions" yaml:"actions"`
	Window        *WindowConfig `json:"window,omitempty" yaml:"window,omitempty"`
	State         *StateConfig  `json:"state,omitempty" yaml:"state,omitempty"`
}

type RuleSource struct {
	Alias     string `json:"alias" yaml:"alias"` // Variable name in expression (e.g. "t1")
	ChannelID string `json:"channel_id" yaml:"channel_id"`
	DeviceID  string `json:"device_id" yaml:"device_id"`
	PointID   string `json:"point_id" yaml:"point_id"`
	PointName string `json:"point_name" yaml:"point_name"`
}

type RuleAction struct {
	Type   string         `json:"type" yaml:"type"` // mqtt, http, log, command
	Config map[string]any `json:"config" yaml:"config"`
}

type WindowConfig struct {
	Type     string `json:"type" yaml:"type"`           // sliding, tumbling
	Size     string `json:"size" yaml:"size"`           // e.g. "10s", "100" (count)
	Interval string `json:"interval" yaml:"interval"`   // Step size for sliding
	AggrFunc string `json:"aggr_func" yaml:"aggr_func"` // avg, min, max, sum, count
}

type StateConfig struct {
	Duration string `json:"duration" yaml:"duration"` // e.g. "10s" (Hold time)
	Count    int    `json:"count" yaml:"count"`       // Consecutive count
}

// RuleRuntimeState represents the runtime status of a rule
type RuleRuntimeState struct {
	RuleID               string            `json:"rule_id"`
	RuleName             string            `json:"rule_name"`
	Enable               bool              `json:"enable"`
	LastCheckTime        time.Time         `json:"last_check_time,omitempty"`  // For CheckInterval
	LastWindowEval       time.Time         `json:"last_window_eval,omitempty"` // Window aggregation step tick
	LastTrigger          time.Time         `json:"last_trigger"`
	LastValue            any               `json:"last_value"`
	TriggerCount         int64             `json:"trigger_count"`
	SuccessCount         int64             `json:"success_count"`        // completed trigger cycles without error
	FailureCount         int64             `json:"failure_count"`        // evaluate/dispatch/action failures
	ActionSuccessCount   int64             `json:"action_success_count"` // individual action successes
	ActionFailureCount   int64             `json:"action_failure_count"` // individual action failures
	CurrentStatus        string            `json:"current_status"`       // NORMAL, WARNING, ALARM
	ConditionStart       time.Time         `json:"condition_start,omitempty"`
	ConditionCount       int               `json:"condition_count,omitempty"`
	ErrorMessage         string            `json:"error_message,omitempty"`
	ActionLastRuns       map[int]time.Time `json:"action_last_runs,omitempty"`
	ExecutionPhase       string            `json:"execution_phase,omitempty"`        // idle, window, evaluate, state_hold, trigger, action, completed, error
	ExecutionActionIndex int               `json:"execution_action_index,omitempty"` // 0-based when execution_phase=action
}

type FailedAction struct {
	ID         string         `json:"id"`
	RuleID     string         `json:"rule_id"`
	Action     RuleAction     `json:"action"`
	Value      Value          `json:"value"`
	Timestamp  time.Time      `json:"timestamp"`
	RetryCount int            `json:"retry_count"`
	LastError  string         `json:"last_error"`
	Env        map[string]any `json:"env"`
}

// SouthboundManager interface defines methods required by Northbound components
// to interact with southbound devices (e.g. for building address space or writing values)
type SouthboundManager interface {
	GetChannels() []Channel
	GetChannelDevices(channelID string) []Device
	GetDevice(channelID, deviceID string) *Device
	WritePoint(channelID, deviceID, pointID string, value any) error
	GetDevicePoints(channelID, deviceID string) ([]PointData, error)
	// GetShadowPoint 从影子设备实时快照中读取单个点位数据。
	// 供 OPC UA Server 的 ReadHandler 调用，使第三方客户端能按需获取实时值。
	GetShadowPoint(channelID, deviceID, pointID string) (*ShadowPoint, error)
}

// ProtocolConfig 协议配置
type ProtocolConfig struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

// ========== Shadow Device Structures ==========

// ShadowPoint represents a single point in a shadow device
type ShadowPoint struct {
	Value          any       `json:"value"`
	PreviousValue  any       `json:"previous_value,omitempty"` // notify-only: prior value before this delta
	Unit           string    `json:"unit"`
	RW             string    `json:"rw"` // "r" or "rw"
	SamplePeriodMs int       `json:"sample_period_ms"`
	Quality        string    `json:"quality"`
	Degraded       bool      `json:"degraded,omitempty"` // 点位降级标记
	Timestamp      time.Time `json:"timestamp"`          // 采集时间（兼容旧字段）
	CollectedAt    time.Time `json:"collected_at"`       // 采集时间
	UpdatedAt      time.Time `json:"updated_at"`         // 影子更新时间
	Version        uint64    `json:"version"`
}

// ShadowDevice represents a real shadow device (physical device shadow)
type ShadowDevice struct {
	ShadowDeviceID   string                 `json:"shadow_device_id"`
	PhysicalDeviceID string                 `json:"physical_device_id"`
	ChannelID        string                 `json:"channel_id"`
	Version          uint64                 `json:"version"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Points           map[string]ShadowPoint `json:"points"`
	// 通信画像相关字段
	CommunicationProfile *DeviceCommunicationProfile `json:"communication_profile,omitempty"`
}

// VirtualDevice represents a virtual shadow device with formula-based points
type VirtualDevice struct {
	VirtualDeviceID string                 `json:"virtual_device_id"`
	ChannelID       string                 `json:"channel_id,omitempty"`
	Version         uint64                 `json:"version"`
	UpdatedAt       time.Time              `json:"updated_at"`
	FormulaPoints   map[string]string      `json:"formula_points"` // pointID -> formula
	Dependencies    []string               `json:"dependencies"`   // list of dependent point keys
	Points          map[string]ShadowPoint `json:"points"`         // computed values
}

// VirtualShadowPointDef 虚拟影子点位定义（UI 配置）
type VirtualShadowPointDef struct {
	PointID   string `json:"point_id"`
	Name      string `json:"name,omitempty"`
	Unit      string `json:"unit,omitempty"`
	Mode      string `json:"mode"`                 // "map" 直接映射 | "formula" 公式计算
	SourceRef string `json:"source_ref,omitempty"` // map 模式：ch1.dev1.temp
	Formula   string `json:"formula,omitempty"`    // formula 模式
}

// VirtualShadowDeviceConfig 虚拟影子设备配置（持久化）
type VirtualShadowDeviceConfig struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Enable      bool                    `json:"enable"`
	Points      []VirtualShadowPointDef `json:"points"`
}

// PointSourceRef 可选点位引用（供 UI 拼积木选择）
type PointSourceRef struct {
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	PointID     string `json:"point_id"`
	PointName   string `json:"point_name"`
	Ref         string `json:"ref"`
}

// SourceDeviceSummary 源设备摘要（供 UI 检索列表，不含全量点位）
type SourceDeviceSummary struct {
	Key         string `json:"key"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	PointCount  int    `json:"point_count"`
	State       int    `json:"state,omitempty"` // 0=Online, 1=Unstable, 2=Offline, 3=Quarantine
	Online      bool   `json:"online"`
}

// ShadowIngressMessage represents the standard message format from Points layer to ShadowIngress
type ShadowIngressMessage struct {
	MessageID string               `json:"message_id"`
	QoS       int                  `json:"qos"` // 0, 1, 2
	DeviceID  string               `json:"device_id"`
	ChannelID string               `json:"channel_id"`
	Timestamp time.Time            `json:"timestamp"`
	Points    []ShadowIngressPoint `json:"points"`
	Meta      ShadowIngressMeta    `json:"meta"`
}

// ShadowIngressPoint represents a single point in the ingress message
type ShadowIngressPoint struct {
	PointID        string    `json:"point_id"`
	Value          any       `json:"value"`
	Unit           string    `json:"unit"`
	Quality        string    `json:"quality"`
	SamplePeriodMs int       `json:"sample_period_ms"`
	CollectedAt    time.Time `json:"collected_at"` // 设备侧采集时间
	Degraded       bool      `json:"degraded,omitempty"`
}

// ShadowIngressMeta represents metadata in the ingress message
type ShadowIngressMeta struct {
	Source   string `json:"source"`
	Sequence uint64 `json:"sequence"`
}

// ConsistencyCheckResult represents the result of a consistency check
type ConsistencyCheckResult struct {
	Pass          bool              `json:"pass"`
	DiffPoints    []ShadowDiffPoint `json:"diff_points"`
	DiffSource    string            `json:"diff_source"`
	RepairSuggest string            `json:"repair_suggest"`
}

// ShadowDiffPoint represents a difference found during consistency check
type ShadowDiffPoint struct {
	PointID  string `json:"point_id"`
	Field    string `json:"field"` // "value", "version", "timestamp", "quality"
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
}

// ShadowWriteRequest represents a write request to shadow device
type ShadowWriteRequest struct {
	ShadowDeviceID string    `json:"shadow_device_id"`
	PointID        string    `json:"point_id"`
	Value          any       `json:"value"`
	QoS            int       `json:"qos"`
	Timestamp      time.Time `json:"timestamp"`
}

// ShadowWriteResponse represents a write response from shadow device
type ShadowWriteResponse struct {
	Success   bool      `json:"success"`
	Version   uint64    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
}

// RTTNode RTT统计节点
type RTTNode struct {
	SeqNo     uint16    `json:"seq_no"`
	SendTS    time.Time `json:"send_ts"`
	RecvTS    time.Time `json:"recv_ts"`
	AckStatus bool      `json:"ack_status"`
	RTT       int64     `json:"rtt"`
}

// MTUNegotiationRecord MTU协商记录
type MTUNegotiationRecord struct {
	AttemptValue int       `json:"attempt_value"`
	ResponseTime int64     `json:"response_time"`
	RetryCount   int       `json:"retry_count"`
	Success      bool      `json:"success"`
	Timestamp    time.Time `json:"timestamp"`
}

// BatchReadSnapshot Batch Read快照
type BatchReadSnapshot struct {
	CurrentGap     int     `json:"current_gap"`
	MaxGap         int     `json:"max_gap"`
	MergedRequests uint64  `json:"merged_requests"`
	SavedRequests  uint64  `json:"saved_requests"`
	FillEfficiency float64 `json:"fill_efficiency"`
}

// DeviceCommunicationProfile 设备通信画像结构
type DeviceCommunicationProfile struct {
	DeviceID              string                 `json:"device_id"`
	ChannelID             string                 `json:"channel_id"`
	ProtocolType          string                 `json:"protocol_type"`
	SlaveID               interface{}            `json:"slave_id"`
	AvgResponseTime       time.Duration          `json:"avg_response_time"`
	MaxResponseTime       time.Duration          `json:"max_response_time"`
	ErrorRate             float64                `json:"error_rate"`
	StabilityScore        float64                `json:"stability_score"`
	OptimalTimeout        time.Duration          `json:"optimal_timeout"`
	OptimalInterval       time.Duration          `json:"optimal_interval"`
	RetryCount            int                    `json:"retry_count"`
	BatchSize             int                    `json:"batch_size"`
	ProtocolParams        map[string]interface{} `json:"protocol_params"`
	LastUpdated           time.Time              `json:"last_updated"`
	CollectionSuccessRate float64                `json:"collection_success_rate"`
	AbnormalPointCount    int                    `json:"abnormal_point_count"`
	ConsecutiveFailures   int                    `json:"consecutive_failures"`
	// RTT相关字段
	RTTSamples      []int64 `json:"rtt_samples"`
	RTTSampleWindow int     `json:"rtt_sample_window"`
	EWMARTT         int64   `json:"ewma_rtt"`
	// MTU相关字段
	CurrentMTU int `json:"current_mtu"`
	MaxMTU     int `json:"max_mtu"`
	MinMTU     int `json:"min_mtu"`
	// Gap合并相关字段
	CurrentGap      int `json:"current_gap"`
	MaxGap          int `json:"max_gap"`
	GapFillStrategy int `json:"gap_fill_strategy"`
	// 心跳相关字段
	HeartbeatInterval int       `json:"heartbeat_interval"`
	LastActivity      time.Time `json:"last_activity"`
}

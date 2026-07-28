package capability

import "strings"

// KnownDriverProtocols lists EdgeX driver protocol ids used for auto-generation.
// Values use EdgeX driver registration names (hyphenated); they are normalized
// to Capability ID prefixes with underscores.
var KnownDriverProtocols = []string{
	"modbus-tcp",
	"modbus-rtu",
	"modbus-rtu-over-tcp",
	"bacnet-ip",
	"s7",
	"opc-ua",
	"ethernet-ip",
	"omron-fins",
	"iec60870-5-104",
	"knxnet-ip",
	"snmp",
	"dlt645",
	"mitsubishi-slmp",
	"profinet-io",
	"ethercat",
}

// NormalizeProtocolID converts driver protocol names to Capability ID prefixes.
// Example: "modbus-tcp" → "modbus_tcp".
func NormalizeProtocolID(protocol string) string {
	p := strings.TrimSpace(strings.ToLower(protocol))
	p = strings.ReplaceAll(p, "-", "_")
	p = strings.ReplaceAll(p, ".", "_")
	return p
}

// CapabilityID builds "{protocol}.{command}".
func CapabilityID(protocol, command string) string {
	return NormalizeProtocolID(protocol) + "." + command
}

// GenerateProtocolCapabilities auto-maps Driver commands to Capabilities
// for a single protocol, per EAN 2.0 auto-generation rules.
func GenerateProtocolCapabilities(agentID, protocol string) []Capability {
	prefix := NormalizeProtocolID(protocol)
	caps := []Capability{
		{
			ID:      prefix + ".read_holding_register",
			AgentID: agentID,
			Description: "读取设备保持寄存器/点位值 | Read device holding registers/point values. " +
				"参数 Params: device_id(string, required 必需-设备ID或通道标识), " +
				"address(string, optional 可选-单个寄存器地址), " +
				"addresses(array[string], optional 可选-多个寄存器地址数组), " +
				"quantity(integer, default=1 默认读取数量). " +
				"返回 Returns: values(array 点位值数组), timestamp(integer 时间戳)",
			Category: CategoryDevice,
			InputSchema: objectSchema(map[string]any{
				"device_id": map[string]any{"type": "string", "description": "设备ID或通道标识 | Device ID or channel identifier"},
				"address":   map[string]any{"type": "string", "description": "单个寄存器地址 | Single register address"},
				"addresses": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "多个寄存器地址数组 | Array of register addresses"},
				"quantity":  map[string]any{"type": "integer", "default": 1, "description": "读取数量，默认1 | Read quantity, default 1"},
			}, []string{"device_id"}),
			OutputSchema: objectSchema(map[string]any{
				"values":    map[string]any{"type": "array", "description": "点位值数组 | Array of point values"},
				"timestamp": map[string]any{"type": "integer", "description": "读取时间戳 | Read timestamp"},
			}, nil),
			TimeoutSec: 10,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "ReadPoints",
				"protocol":       protocol,
			},
		},
		{
			ID:      prefix + ".write_register",
			AgentID: agentID,
			Description: "写入单个寄存器/点位值 | Write a single register/point value. " +
				"参数 Params: device_id(string, required 必需-设备ID), " +
				"address(string, required 必需-寄存器地址), " +
				"value(number, required 必需-写入值). " +
				"返回 Returns: success(boolean 是否成功), timestamp(integer 时间戳)",
			Category: CategoryDevice,
			InputSchema: objectSchema(map[string]any{
				"device_id": map[string]any{"type": "string", "description": "设备ID | Device ID"},
				"address":   map[string]any{"type": "string", "description": "寄存器地址 | Register address"},
				"value":     map[string]any{"type": "number", "description": "写入值 | Value to write"},
			}, []string{"device_id", "address", "value"}),
			OutputSchema: objectSchema(map[string]any{
				"success":   map[string]any{"type": "boolean", "description": "是否写入成功 | Whether write succeeded"},
				"timestamp": map[string]any{"type": "integer", "description": "写入时间戳 | Write timestamp"},
			}, nil),
			TimeoutSec: 10,
			Permission: PermissionWrite,
			Metadata: map[string]any{
				"driver_command": "WritePoint",
				"protocol":       protocol,
			},
		},
		{
			ID:      prefix + ".scan_devices",
			AgentID: agentID,
			Description: "扫描/发现指定通道下的设备 | Scan/discover devices under a channel. " +
				"参数 Params: channel_id(string, required 必需-通道ID), " +
				"network(string, optional 可选-网络范围/网段，如 192.168.1.0/24). " +
				"返回 Returns: devices(array 发现的设备列表)",
			Category: CategoryDevice,
			InputSchema: objectSchema(map[string]any{
				"channel_id": map[string]any{"type": "string", "description": "通道ID，必需 | Channel ID, required"},
				"network":    map[string]any{"type": "string", "description": "网络范围/网段，可选 | Network range, optional"},
			}, []string{"channel_id"}),
			OutputSchema: objectSchema(map[string]any{
				"devices": map[string]any{"type": "array", "description": "发现的设备列表 | Discovered device list"},
			}, nil),
			TimeoutSec: 30,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "ScanDevices",
				"protocol":       protocol,
			},
		},
		{
			ID:      prefix + ".list_points",
			AgentID: agentID,
			Description: "列出指定设备的所有点位 | List all points of a device. " +
				"参数 Params: device_id(string, required 必需-设备ID). " +
				"返回 Returns: points(array 点位列表，包含地址、类型、当前值等)",
			Category: CategoryDevice,
			InputSchema: objectSchema(map[string]any{
				"device_id": map[string]any{"type": "string", "description": "设备ID，必需 | Device ID, required"},
			}, []string{"device_id"}),
			OutputSchema: objectSchema(map[string]any{
				"points": map[string]any{"type": "array", "description": "点位列表 | List of points"},
			}, nil),
			TimeoutSec: 10,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "GetDevicePoints",
				"protocol":       protocol,
			},
		},
	}
	return caps
}

// GenerateSystemCapabilities returns built-in system/AI capabilities.
func GenerateSystemCapabilities(agentID string) []Capability {
	return []Capability{
		{
			ID:      "system.diagnostics",
			AgentID: agentID,
			Description: "收集 EdgeX 系统诊断信息 | Collect EdgeX system diagnostics. " +
				"参数 Params: 无 none. " +
				"返回 Returns: diagnostics(object 诊断数据对象，包含通道状态、设备统计、资源使用等)",
			Category:    CategorySystem,
			InputSchema: objectSchema(nil, nil),
			OutputSchema: objectSchema(map[string]any{
				"diagnostics": map[string]any{"type": "object", "description": "诊断数据对象 | Diagnostics data object"},
			}, nil),
			TimeoutSec: 15,
			Permission: PermissionAdmin,
			Metadata: map[string]any{
				"driver_command": "Diagnostics",
			},
		},
		{
			ID:      "ai.protocol_reverse",
			AgentID: agentID,
			Description: "AI 辅助协议逆向工程 | AI-assisted protocol reverse engineering. " +
				"参数 Params: payload(object, required 必需-输入数据，包含抓包数据或观测值). " +
				"返回 Returns: candidates(array 候选协议结构列表)",
			Category: CategoryAI,
			InputSchema: objectSchema(map[string]any{
				"payload": map[string]any{"type": "object", "description": "输入数据对象，包含抓包数据或观测值 | Input data object with packet captures or observations"},
			}, []string{"payload"}),
			OutputSchema: objectSchema(map[string]any{
				"candidates": map[string]any{"type": "array", "description": "候选协议结构列表 | List of candidate protocol structures"},
			}, nil),
			TimeoutSec: 120,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "AI.protocol_reverse",
			},
		},
		{
			ID:      "ai.doc_parse",
			AgentID: agentID,
			Description: "AI 辅助协议文档解析 | AI-assisted protocol document parsing. " +
				"参数 Params: payload(object, required 必需-输入数据，包含协议文档内容或截图). " +
				"返回 Returns: points(array 解析出的点位配置列表)",
			Category: CategoryAI,
			InputSchema: objectSchema(map[string]any{
				"payload": map[string]any{"type": "object", "description": "输入数据对象，包含协议文档内容或截图 | Input data object with protocol document content or screenshots"},
			}, []string{"payload"}),
			OutputSchema: objectSchema(map[string]any{
				"points": map[string]any{"type": "array", "description": "解析出的点位配置列表 | List of parsed point configurations"},
			}, nil),
			TimeoutSec: 120,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "AI.doc_parse",
			},
		},
	}
}

// GenerateDefaultCapabilities builds the full default Capability set for EdgeX.
func GenerateDefaultCapabilities(agentID string, protocols []string) []Capability {
	if len(protocols) == 0 {
		protocols = KnownDriverProtocols
	}
	out := make([]Capability, 0, len(protocols)*4+3)
	for _, p := range protocols {
		out = append(out, GenerateProtocolCapabilities(agentID, p)...)
	}
	out = append(out, GenerateSystemCapabilities(agentID)...)
	return out
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

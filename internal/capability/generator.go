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
			ID:      prefix + ".read_point",
			AgentID: agentID,
			Description: "读取单个/多个点位数据（中性命名，跨协议统一） | Read single/multiple point data (neutral, unified across protocols). " +
				"参数 Params: device_id(string, required 必需-设备ID或通道标识), " +
				"address(string, optional 可选-单个点位标识/地址), " +
				"addresses(array[string], optional 可选-多个点位标识/地址数组), " +
				"quantity(integer, default=1 默认读取数量). " +
				"返回 Returns: values(array 点位值数组), timestamp(integer 时间戳)",
			Category: CategoryDevice,
			InputSchema: objectSchema(map[string]any{
				"device_id": map[string]any{"type": "string", "description": "设备ID或通道标识 | Device ID or channel identifier"},
				"address":   map[string]any{"type": "string", "description": "单个点位标识/地址 | Single point id/address"},
				"addresses": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "多个点位标识/地址数组 | Array of point ids/addresses"},
				"quantity":  map[string]any{"type": "integer", "default": 1, "description": "读取数量，默认1 | Read quantity, default 1"},
			}, []string{"device_id"}),
			OutputSchema: objectSchema(map[string]any{
				"values":    map[string]any{"type": "array", "description": "点位值数组 | Array of point values"},
				"timestamp": map[string]any{"type": "integer", "description": "读取时间戳 | Read timestamp"},
			}, nil),
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "ReadPoints",
				"protocol":       protocol,
			},
		},
		{
			ID:      prefix + ".write_point",
			AgentID: agentID,
			Description: "写入单个/多个点位数据（中性命名，跨协议统一） | Write single/multiple point data (neutral, unified across protocols). " +
				"参数 Params: device_id(string, required 必需-设备ID), " +
				"address(string, required 必需-点位标识/地址), " +
				"value(number, required 必需-写入值), " +
				"writes(array[object], optional 可选-批量写入：[{address,value}]). " +
				"返回 Returns: success(boolean 是否成功), timestamp(integer 时间戳)",
			Category: CategoryDevice,
			InputSchema: objectSchema(map[string]any{
				"device_id": map[string]any{"type": "string", "description": "设备ID | Device ID"},
				"address":   map[string]any{"type": "string", "description": "点位标识/地址 | Point id/address"},
				"value":     map[string]any{"type": "number", "description": "写入值 | Value to write"},
				"writes":    map[string]any{"type": "array", "description": "批量写入：[{address,value}] | Batch write entries"},
			}, []string{"device_id", "address", "value"}),
			OutputSchema: objectSchema(map[string]any{
				"success":   map[string]any{"type": "boolean", "description": "是否写入成功 | Whether write succeeded"},
				"timestamp": map[string]any{"type": "integer", "description": "写入时间戳 | Write timestamp"},
			}, nil),
			TimeoutSec: 5,
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
			TimeoutSec: 5,
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
			TimeoutSec: 5,
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
			TimeoutSec: 5,
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
			TimeoutSec: 5,
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
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "AI.doc_parse",
			},
		},
	}
}

// GenerateDefaultCapabilities builds the full default Capability set for EdgeX.
// Generates 63 capabilities (15 protocols x 4 ops + 3 system/AI).
// Used by northbound EAN Runtime for EdgeOS cross-system discovery.
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

// GenerateUnifiedCapabilities builds a consolidated MCP tool set (7 tools).
// Replaces the 63 protocol-specific capabilities with 5 unified device operations
// plus 2 AI capabilities, reducing MCP tool count from 63 to 7.
// Used by MCP Runtime for LLM tool calling.
func GenerateUnifiedCapabilities(agentID string) []Capability {
	return []Capability{
		{
		ID:      "read_points",
		AgentID: agentID,
		Description: "统一点位读取 | Unified point read. " +
			"Supports single point, batch (point_ids[]/addresses[]), and quantity-based reads. " +
			"Protocol is auto-detected from device_id via ShadowCore channel lookup. " +
			"⚠ 地址语义统一说明 | Address semantics (IMPORTANT): " +
			"address/addresses 参数接受三种形式，系统自动解析为内部 point_id: " +
			"(1) point_id — 推荐，list_points 返回的 id 字段，如 'pt_0723121000'; " +
			"(2) address — 寄存器地址，如 Modbus PDU 偏移 '0' 或 PLC 地址 '40001'; " +
			"(3) name — 点位名称（不区分大小写）. " +
			"list_points 的输出可直接作为 read_points 的输入. " +
			"The address/addresses parameter accepts three forms, auto-resolved to internal point_id: " +
			"(1) point_id — preferred, the id field from list_points output; " +
			"(2) address — register address (PDU offset or PLC address); " +
			"(3) name — point name (case-insensitive). " +
			"list_points output can be passed directly to read_points. " +
			"参数 Params: device_id(string, required), " +
			"point_id(string, optional 点位ID-推荐), " +
			"address(string, optional 地址或point_id), " +
			"point_ids(array[string], optional 点位ID数组-推荐), " +
			"addresses(array[string], optional 地址数组), " +
			"quantity(integer, default=1), " +
			"protocol(string, optional), " +
			"live(bool, default=true 是否实时读取), " +
			"prefer_shadow(bool, optional 优先读影子缓存). " +
			"返回 Returns: values(array 含 point_id/address/value/quality/timestamp/source), timestamp(integer)",
		Category: CategoryDevice,
		InputSchema: objectSchema(map[string]any{
			"device_id":      map[string]any{"type": "string", "description": "设备ID | Device ID"},
			"point_id":       map[string]any{"type": "string", "description": "点位ID（推荐，list_points 返回的 id 字段）| Point ID (preferred, from list_points output id field)"},
			"address":        map[string]any{"type": "string", "description": "单个地址或 point_id，系统自动解析 | Single address or point_id, auto-resolved"},
			"point_ids":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "点位ID数组（推荐）| Array of point IDs (preferred)"},
			"addresses":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "地址数组，系统自动解析为 point_id | Array of addresses, auto-resolved to point_id"},
			"quantity":       map[string]any{"type": "integer", "default": 1, "description": "读取数量，默认1 | Read quantity, default 1"},
			"protocol":       map[string]any{"type": "string", "description": "显式协议类型（可选），不传则自动路由 | Optional explicit protocol, auto-routed if omitted"},
			"live":           map[string]any{"type": "boolean", "default": true, "description": "是否实时读取（true=驱动直读, false=影子缓存）| Live read from driver (true) or shadow cache (false)"},
			"prefer_shadow":  map[string]any{"type": "boolean", "default": false, "description": "优先读影子缓存（低延迟）| Prefer shadow cache (low latency)"},
		}, []string{"device_id"}),
			OutputSchema: objectSchema(map[string]any{
				"values":    map[string]any{"type": "array", "description": "点位值数组 | Array of point values"},
				"timestamp": map[string]any{"type": "integer", "description": "读取时间戳 | Read timestamp"},
			}, nil),
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "ReadPoints",
				"unified":        true,
			},
		},
		{
		ID:      "write_points",
		AgentID: agentID,
		Description: "统一点位写入 | Unified point write. " +
			"Supports single point and batch writes. Pre-validates device enabled state. " +
			"⚠ 地址语义统一说明 | Address semantics (IMPORTANT): " +
			"address 参数接受三种形式，系统自动解析为内部 point_id: " +
			"(1) point_id — 推荐，list_points 返回的 id 字段; " +
			"(2) address — 寄存器地址（PDU 偏移或 PLC 地址）; " +
			"(3) name — 点位名称（不区分大小写）. " +
			"The address parameter accepts three forms, auto-resolved to internal point_id: " +
			"(1) point_id — preferred; (2) address — register address; (3) name — point name. " +
			"参数 Params: device_id(string, required), " +
			"address(string, required 单点-地址或point_id), " +
			"value(number, required 单点-写入值), " +
			"point_id(string, optional 点位ID-推荐替代address), " +
			"writes(array[object], optional 批量写入，每项含 address+value 或 point_id+value), " +
			"protocol(string, optional). " +
			"返回 Returns: success(boolean), timestamp(integer), " +
			"results(array 批量时每条含 point_id/address/value/success/error)",
		Category: CategoryDevice,
		InputSchema: objectSchema(map[string]any{
			"device_id": map[string]any{"type": "string", "description": "设备ID | Device ID"},
			"point_id":  map[string]any{"type": "string", "description": "点位ID（推荐，替代 address）| Point ID (preferred, replaces address)"},
			"address":   map[string]any{"type": "string", "description": "地址或 point_id，系统自动解析 | Address or point_id, auto-resolved"},
			"value":     map[string]any{"type": "number", "description": "写入值 | Value to write"},
			"writes": map[string]any{
				"type":        "array",
				"description": "批量写入列表 | Batch write list",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"point_id": map[string]any{"type": "string", "description": "点位ID（推荐）| Point ID (preferred)"},
						"address":  map[string]any{"type": "string", "description": "地址或 point_id，自动解析 | Address or point_id, auto-resolved"},
						"value":    map[string]any{"type": "number", "description": "写入值 | Value to write"},
					},
				},
			},
			"protocol": map[string]any{"type": "string", "description": "显式协议类型（可选） | Optional explicit protocol"},
		}, []string{"device_id"}),
			OutputSchema: objectSchema(map[string]any{
				"success":   map[string]any{"type": "boolean", "description": "是否写入成功 | Whether write succeeded"},
				"timestamp": map[string]any{"type": "integer", "description": "写入时间戳 | Write timestamp"},
				"results":   map[string]any{"type": "array", "description": "批量写入时每条结果 | Per-item results for batch writes"},
			}, nil),
			TimeoutSec: 5,
			Permission: PermissionWrite,
			Metadata: map[string]any{
				"driver_command": "WritePoint",
				"unified":        true,
			},
		},
		{
			ID:      "scan_devices",
			AgentID: agentID,
			Description: "统一设备扫描 | Unified device scan. " +
				"Scans/discovers devices under a channel. Protocol auto-detected from channel_id. " +
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
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "ScanDevices",
				"unified":        true,
			},
		},
		{
		ID:      "list_points",
		AgentID: agentID,
		Description: "统一点位列表 | Unified point list. " +
			"Lists all points of a device. Protocol auto-detected from device_id. " +
			"⚠ 输出字段说明 | Output fields (IMPORTANT): " +
			"返回的每个点位包含 id(point_id, 用于 read/write 的推荐标识), " +
			"address(协议地址，如 Modbus PDU 偏移 '0' 或 PLC 地址 '40001'), " +
			"name(点位名称), datatype(数据类型), value(当前值), quality(质量). " +
			"调用 read_points/write_points 时，可直接传入此处的 id 字段作为 point_id 参数. " +
			"Each returned point contains: id (point_id, the preferred identifier for read/write), " +
			"address (protocol-specific address), name, datatype, value, quality. " +
			"Pass the id field directly to read_points/write_points as point_id. " +
			"参数 Params: device_id(string, required). " +
			"返回 Returns: points(array 含 id/address/name/datatype/value/quality), count(integer)",
		Category: CategoryDevice,
		InputSchema: objectSchema(map[string]any{
			"device_id": map[string]any{"type": "string", "description": "设备ID，必需 | Device ID, required"},
		}, []string{"device_id"}),
		OutputSchema: objectSchema(map[string]any{
			"points": map[string]any{"type": "array", "description": "点位列表，每项含 id(point_id)/address/name/datatype/value/quality | Point list, each with id(point_id)/address/name/datatype/value/quality"},
			"count":  map[string]any{"type": "integer", "description": "点位数量 | Point count"},
		}, nil),
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "GetDevicePoints",
				"unified":        true,
			},
		},
		{
			ID:      "get_diagnostics",
			AgentID: agentID,
			Description: "系统诊断 | System diagnostics. " +
				"Collects EdgeX system diagnostics including channel status, device statistics, " +
				"resource usage, and invoke metrics. " +
				"参数 Params: channel_id(string, optional 可选-通道ID，不填则返回全部通道摘要), " +
				"device_id(string, optional 可选-设备ID，需配合 channel_id). " +
				"返回 Returns: diagnostics(object 诊断数据对象)",
			Category: CategorySystem,
			InputSchema: objectSchema(map[string]any{
				"channel_id": map[string]any{"type": "string", "description": "通道ID（可选） | Channel ID (optional)"},
				"device_id":  map[string]any{"type": "string", "description": "设备ID（可选，需配合 channel_id） | Device ID (optional)"},
			}, nil),
			OutputSchema: objectSchema(map[string]any{
				"diagnostics": map[string]any{"type": "object", "description": "诊断数据对象 | Diagnostics data object"},
			}, nil),
			TimeoutSec: 5,
			Permission: PermissionAdmin,
			Metadata: map[string]any{
				"driver_command": "Diagnostics",
				"unified":        true,
			},
		},
		{
			ID:      "ai_protocol_reverse",
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
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "AI.protocol_reverse",
			},
		},
		{
			ID:      "ai_doc_parse",
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
			TimeoutSec: 5,
			Permission: PermissionRead,
			Metadata: map[string]any{
				"driver_command": "AI.doc_parse",
			},
		},
	}
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

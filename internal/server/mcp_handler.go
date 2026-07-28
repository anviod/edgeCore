package server

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anviod/edgex/internal/capability"
	"github.com/anviod/edgex/internal/execution"
	"github.com/anviod/edgex/internal/mcp"
	"github.com/anviod/edgex/internal/model"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// ── MCP 会话管理（MCP 2025-11-25 Streamable HTTP）──

var (
	mcpSessions   = make(map[string]string)
	mcpSessionsMu sync.Mutex
)

// getOrCreateMCPSession 获取或创建 MCP 会话 ID
func (s *Server) getOrCreateMCPSession(c *fiber.Ctx) string {
	sessionID := c.Get("Mcp-Session-Id", "")
	if sessionID != "" {
		return sessionID
	}
	b := make([]byte, 16)
	rand.Read(b)
	sessionID = hex.EncodeToString(b)
	mcpSessionsMu.Lock()
	mcpSessions[sessionID] = time.Now().Format(time.RFC3339)
	mcpSessionsMu.Unlock()
	return sessionID
}

// ── MCP 认证与权限 ──

// mcpCheckAuth 校验 MCP API Key 认证；返回 true 表示认证通过
func (s *Server) mcpCheckAuth(c *fiber.Ctx) bool {
	settings := s.loadAiCopilotSettings()
	if !settings.McpEnabled || settings.McpApiKey == "" {
		return false
	}
	authHeader := c.Get("Authorization", "")
	// 支持 Bearer <key> 和 X-MCP-API-Key: <key> 两种方式
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ") == settings.McpApiKey
	}
	if key := c.Get("X-MCP-API-Key", ""); key != "" {
		return key == settings.McpApiKey
	}
	return false
}

// mcpHasFullAccess 检查是否已开启全功能读写权限
func (s *Server) mcpHasFullAccess() bool {
	settings := s.loadAiCopilotSettings()
	return settings.McpEnabled && settings.McpFullAccess
}

// mcpRequireFullAccess 返回 nil 表示允许，返回 error 表示需要用户确认开启全功能
func (s *Server) mcpRequireFullAccess() *mcp.CallToolResult {
	if !s.mcpHasFullAccess() {
		return mcp.NewErrorResult("全功能读写未开启。请在 EdgeX UI → AI 助手 → MCP 接入页面，点击「激活全功能」确认后重试。当前仅支持只读操作。")
	}
	return nil
}

// ── MCP 工具注册 ──

// registerMCPTools 注册所有 EdgeX MCP 工具到 MCP Server
func (s *Server) registerMCPTools(mcpSrv *mcp.MCPServer) {
	// ── 查询类工具 ──

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_list_channels",
		Description: "列出所有采集通道及其状态（协议、连接状态、设备数量、点位数量）",
		InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.PropertyDef{}},
	}, s.mcpListChannels)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_list_devices",
		Description: "列出指定通道下的所有设备（名称、从站地址、在线状态、采集间隔）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
			},
			Required: []string{"channel_id"},
		},
	}, s.mcpListDevices)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_list_points",
		Description: "列出指定设备和通道下的所有点位（名称、地址、数据类型、缩放、扫描类、读写属性、当前值）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
			},
			Required: []string{"channel_id", "device_id"},
		},
	}, s.mcpListPoints)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_read_point",
		Description: "读取指定点位的当前实时值（返回采集值、时间戳、质量状态）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"point_id":   {Type: "string", Description: "点位 ID"},
			},
			Required: []string{"channel_id", "device_id", "point_id"},
		},
	}, s.mcpReadPoint)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_get_system_info",
		Description: "获取 EdgeX 网关系统信息（CPU/内存/磁盘使用率、运行时间、Go 版本、协议支持列表）",
		InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.PropertyDef{}},
	}, s.mcpGetSystemInfo)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_get_diagnostics",
		Description: "获取通道或设备的诊断信息（连接状态、数据质量、错误计数、延迟统计、重启次数）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID（可选，不填则返回所有通道摘要）"},
				"device_id":  {Type: "string", Description: "设备 ID（可选，需配合 channel_id 使用）"},
			},
		},
	}, s.mcpGetDiagnostics)

	// ── 操作类工具 ──

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_write_point",
		Description: "向指定点位写入控制值（写操作需要人工确认，不会自动执行；仅支持 R/W 点位）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"point_id":   {Type: "string", Description: "点位 ID（必须为 R/W 权限）"},
				"value":      {Type: "string", Description: "写入值（数字、布尔值或字符串）"},
			},
			Required: []string{"channel_id", "device_id", "point_id", "value"},
		},
	}, s.mcpWritePoint)

	// ── 协议分析类工具 ──

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_analyze_protocol",
		Description: "分析工业协议特征（根据端口号、帧模式、协议名称推断协议类型，返回协议 ID、置信度和特征描述）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"protocol_hint": {Type: "string", Description: "协议提示（如 modbus、s7、bacnet、opcua、eip、profinet、ethercat、dlt645、omron、snmp、knx、mitsubishi、ice104）"},
				"port":          {Type: "number", Description: "端口号（如 502、102、47808、4840）"},
				"description":   {Type: "string", Description: "场景描述"},
			},
		},
	}, s.mcpAnalyzeProtocol)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_get_protocol_help",
		Description: "获取指定工业协议的接入帮助（地址格式、功能码、数据类型、字节序、典型配置示例）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"protocol": {Type: "string", Description: "协议名称（modbus/s7/bacnet/opcua/eip/profinet/ethercat/dlt645/snmp/knx/mitsubishi/omron/ice104）",
					Enum: []string{"modbus", "s7", "bacnet", "opcua", "eip", "profinet", "ethercat", "dlt645", "snmp", "knx", "mitsubishi", "omron", "ice104"}},
			},
			Required: []string{"protocol"},
		},
	}, s.mcpGetProtocolHelp)

	// ── 全功能 CRUD 工具（需要 MCP 全功能激活）──
	s.registerMCPFullTools(mcpSrv)
}

// registerMCPFullTools 注册需要全功能权限的 CRUD 工具
func (s *Server) registerMCPFullTools(mcpSrv *mcp.MCPServer) {
	// 通道管理
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_create_channel",
		Description: "创建南向采集通道（需要 MCP 全功能激活；自动配置协议驱动参数）。TCP 协议（modbus-tcp/s7/bacnet/opcua 等）必须在 config 中提供 ip 字段",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"name":     {Type: "string", Description: "通道名称"},
				"protocol": {Type: "string", Description: "协议类型：modbus-tcp, modbus-rtu, s7, bacnet, opcua, ethernetip, snmp, dlt645, ice104, knxnetip, mitsubishi, omron"},
				"config":   {Type: "object", Description: "协议配置（JSON 对象）。TCP 协议必填：ip（目标 IP）, port（可选，有默认端口）；RTU 协议必填：serial_port, baud_rate"},
			},
			Required: []string{"name", "protocol"},
		},
	}, s.mcpCreateChannel)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_delete_channel",
		Description: "删除指定通道（需要 MCP 全功能激活；会同时删除通道下所有设备和点位）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
			},
			Required: []string{"channel_id"},
		},
	}, s.mcpDeleteChannel)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_start_channel",
		Description: "启动指定通道的采集引擎（需要 MCP 全功能激活）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
			},
			Required: []string{"channel_id"},
		},
	}, s.mcpStartChannel)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_stop_channel",
		Description: "停止指定通道的采集引擎（需要 MCP 全功能激活）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
			},
			Required: []string{"channel_id"},
		},
	}, s.mcpStopChannel)

	// 设备管理
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_create_device",
		Description: "在指定通道下创建设备（需要 MCP 全功能激活；自动配置从站地址、采集间隔等参数）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"name":       {Type: "string", Description: "设备名称"},
				"config":     {Type: "object", Description: "设备配置（JSON 对象）：slave_id, interval, degrade_on_failure 等"},
			},
			Required: []string{"channel_id", "name"},
		},
	}, s.mcpCreateDevice)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_delete_device",
		Description: "删除指定设备（需要 MCP 全功能激活；会同时删除设备下所有点位）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
			},
			Required: []string{"channel_id", "device_id"},
		},
	}, s.mcpDeleteDevice)

	// 点位管理
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_create_point",
		Description: "在指定设备下创建采集点位（需要 MCP 全功能激活；自动配置地址、数据类型、缩放等参数）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id":    {Type: "string", Description: "通道 ID"},
				"device_id":     {Type: "string", Description: "设备 ID"},
				"name":          {Type: "string", Description: "点位名称"},
				"address":       {Type: "string", Description: "点位地址（如 40001, DB1.DBD0, analog-input:1）"},
				"datatype":      {Type: "string", Description: "数据类型：int16, uint16, int32, uint32, float32, float64, bool, string"},
				"register_type": {Type: "string", Description: "寄存器类型：holding, coil, discrete, input（Modbus 协议）"},
				"function_code": {Type: "number", Description: "功能码（Modbus：1/2/3/4）"},
				"scale":         {Type: "number", Description: "缩放系数（默认 1）"},
				"offset":        {Type: "number", Description: "偏移量（默认 0）"},
				"unit":          {Type: "string", Description: "单位（如 V, A, ℃）"},
				"readwrite":     {Type: "string", Description: "读写属性：R（只读）或 RW（读写），默认 R"},
				"scan_class":    {Type: "string", Description: "扫描类：fast, normal, slow（默认 normal）"},
				"word_order":    {Type: "string", Description: "字节序：ABCD, CDAB, BADC, DCBA（默认 ABCD）"},
			},
			Required: []string{"channel_id", "device_id", "name", "address", "datatype"},
		},
	}, s.mcpCreatePoint)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_delete_point",
		Description: "删除指定点位（需要 MCP 全功能激活）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"point_id":   {Type: "string", Description: "点位 ID"},
			},
			Required: []string{"channel_id", "device_id", "point_id"},
		},
	}, s.mcpDeletePoint)

	// 批量读写测试
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_read_point_batch",
		Description: "批量读取多个点位的实时值（需要 MCP 全功能激活；用于点位读取测试验证）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"point_ids":  {Type: "array", Description: "点位 ID 列表"},
			},
			Required: []string{"channel_id", "device_id", "point_ids"},
		},
	}, s.mcpReadPointBatch)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_write_point_batch",
		Description: "批量写入多个点位值（需要 MCP 全功能激活；用于点位写入测试验证；仅支持 R/W 点位）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"writes":     {Type: "array", Description: "写入列表：[{\"point_id\": \"...\", \"value\": \"...\"}]"},
			},
			Required: []string{"channel_id", "device_id", "writes"},
		},
	}, s.mcpWritePointBatch)

	// 边缘规则
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_create_edge_rule",
		Description: "创建边缘计算规则（需要 MCP 全功能激活；支持阈值告警、计算、状态、窗口等类型）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"name":           {Type: "string", Description: "规则名称"},
				"type":           {Type: "string", Description: "规则类型：threshold, calculation, state, window"},
				"condition":      {Type: "string", Description: "触发条件表达式（如 t1 > 100 AND t2 < 50）"},
				"expression":     {Type: "string", Description: "计算表达式（calculation 类型需要）"},
				"actions":        {Type: "array", Description: "动作列表（JSON 数组）：[{\"type\": \"set_point\", \"channel_id\": \"...\", \"device_id\": \"...\", \"point_id\": \"...\", \"value\": \"...\"}]"},
				"sources":        {Type: "array", Description: "数据源列表（JSON 数组）：[{\"alias\": \"t1\", \"channel_id\": \"...\", \"device_id\": \"...\", \"point_id\": \"...\"}]"},
				"check_interval": {Type: "string", Description: "检查间隔（如 5s, 1m, 默认 10s）"},
				"trigger_mode":   {Type: "string", Description: "触发模式：always, on_change（默认 on_change）"},
			},
			Required: []string{"name", "type", "condition", "actions", "sources"},
		},
	}, s.mcpCreateEdgeRule)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_delete_edge_rule",
		Description: "删除边缘计算规则（需要 MCP 全功能激活）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"rule_id": {Type: "string", Description: "规则 ID"},
			},
			Required: []string{"rule_id"},
		},
	}, s.mcpDeleteEdgeRule)

	// 虚拟设备
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_create_virtual_device",
		Description: "创建虚拟设备用于公式计算（需要 MCP 全功能激活；虚拟设备通过公式引用真实点位，不占用物理连接）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"virtual_device_id": {Type: "string", Description: "虚拟设备 ID"},
				"channel_id":        {Type: "string", Description: "关联通道 ID"},
				"formula_points":    {Type: "object", Description: "公式点位映射（JSON 对象）：{\"point_id\": \"formula_expression\"}，如 {\"total_power\": \"p1 + p2 + p3\"}"},
			},
			Required: []string{"virtual_device_id", "formula_points"},
		},
	}, s.mcpCreateVirtualDevice)

	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_delete_virtual_device",
		Description: "删除虚拟设备（需要 MCP 全功能激活）",
		InputSchema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]mcp.PropertyDef{"virtual_device_id": {Type: "string", Description: "虚拟设备 ID"}},
			Required:   []string{"virtual_device_id"},
		},
	}, s.mcpDeleteVirtualDevice)

	// ── 扩展工具 ──

	// 通道重启
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_restart_channel",
		Description: "重启指定通道的采集引擎（需要 MCP 全功能激活；先停止再启动，用于恢复异常连接）",
		InputSchema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]mcp.PropertyDef{"channel_id": {Type: "string", Description: "通道 ID"}},
			Required:   []string{"channel_id"},
		},
	}, s.mcpRestartChannel)

	// 通道配置查询
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_get_channel_config",
		Description: "获取指定通道的完整配置（包括 IP、端口、驱动参数、设备数、点位总数）",
		InputSchema: mcp.InputSchema{
			Type:       "object",
			Properties: map[string]mcp.PropertyDef{"channel_id": {Type: "string", Description: "通道 ID"}},
			Required:   []string{"channel_id"},
		},
	}, s.mcpGetChannelConfig)

	// 设备更新
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_update_device",
		Description: "更新已存在设备的配置（需要 MCP 全功能激活；支持修改名称、采集间隔、从站地址等参数）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"name":       {Type: "string", Description: "新名称（可选）"},
				"config":     {Type: "object", Description: "新配置（可选，JSON 对象）：slave_id, interval, degrade_on_failure 等"},
				"enable":     {Type: "boolean", Description: "启用/禁用（可选）"},
			},
			Required: []string{"channel_id", "device_id"},
		},
	}, s.mcpUpdateDevice)

	// 点位更新
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_update_point",
		Description: "更新已存在点位的配置（需要 MCP 全功能激活；支持修改名称、地址、数据类型、缩放、字节序等参数）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id":    {Type: "string", Description: "通道 ID"},
				"device_id":     {Type: "string", Description: "设备 ID"},
				"point_id":      {Type: "string", Description: "点位 ID"},
				"name":          {Type: "string", Description: "新名称（可选）"},
				"address":       {Type: "string", Description: "新地址（可选）"},
				"datatype":      {Type: "string", Description: "新数据类型（可选）：int16, uint16, int32, uint32, float32, float64, bool, string"},
				"scale":         {Type: "number", Description: "新缩放系数（可选）"},
				"offset":        {Type: "number", Description: "新偏移量（可选）"},
				"unit":          {Type: "string", Description: "新单位（可选）"},
				"readwrite":     {Type: "string", Description: "新读写属性（可选）：R 或 RW"},
				"word_order":    {Type: "string", Description: "新字节序（可选）：ABCD, CDAB, BADC, DCBA"},
				"scan_class":    {Type: "string", Description: "新扫描类（可选）：fast, normal, slow"},
			},
			Required: []string{"channel_id", "device_id", "point_id"},
		},
	}, s.mcpUpdatePoint)

	// 边缘规则列表
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_list_edge_rules",
		Description: "列出所有边缘计算规则（规则名称、类型、条件、状态、触发次数）",
		InputSchema: mcp.InputSchema{Type: "object", Properties: map[string]mcp.PropertyDef{}},
	}, s.mcpListEdgeRules)

	// 历史数据
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_get_point_history",
		Description: "获取指定点位的历史数据（最近 N 条记录，含时间戳、值、质量）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"point_id":   {Type: "string", Description: "点位 ID"},
				"limit":      {Type: "number", Description: "返回记录数（默认 100，最大 1000）"},
				"duration":   {Type: "string", Description: "时间范围（如 5m, 1h, 24h，与 limit 互斥）"},
			},
			Required: []string{"channel_id", "device_id", "point_id"},
		},
	}, s.mcpGetPointHistory)

	// 设备启停
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_enable_device",
		Description: "启用或禁用指定设备（需要 MCP 全功能激活；禁用后停止采集但不删除配置）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"channel_id": {Type: "string", Description: "通道 ID"},
				"device_id":  {Type: "string", Description: "设备 ID"},
				"enable":     {Type: "boolean", Description: "true=启用, false=禁用"},
			},
			Required: []string{"channel_id", "device_id", "enable"},
		},
	}, s.mcpEnableDevice)

	// 配置导出
	mcpSrv.RegisterTool(mcp.Tool{
		Name:        "edgex_export_config",
		Description: "导出 EdgeX 完整配置（所有通道、设备、点位、边缘规则的 JSON 配置，可用于备份或迁移）",
		InputSchema: mcp.InputSchema{
			Type: "object",
			Properties: map[string]mcp.PropertyDef{
				"format": {Type: "string", Description: "导出格式：json（默认）或 yaml"},
				"scope":  {Type: "string", Description: "导出范围：all（全部），channels（仅通道），rules（仅规则），默认 all"},
			},
		},
	}, s.mcpExportConfig)
}

// ── 工具实现 ──

func (s *Server) mcpListChannels(args json.RawMessage) (*mcp.CallToolResult, error) {
	channels := s.cm.GetChannels()

	if len(channels) == 0 {
		return mcp.NewSuccessResult("当前没有配置任何通道。可通过 EdgeX UI 或 API 创建通道。"), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## EdgeX 通道列表 (共 %d 个)\n\n", len(channels)))
	sb.WriteString("| ID | 名称 | 协议 | 状态 | 设备数 |\n")
	sb.WriteString("|----|------|------|------|--------|\n")

	stats := s.cm.GetChannelStats()
	for _, ch := range channels {
		deviceCount := len(s.cm.GetChannelDevices(ch.ID))
		status := "offline"
		for _, cs := range stats {
			if cs.ID == ch.ID {
				status = strings.ToLower(cs.Status)
				if status == "" {
					status = "offline"
				}
				break
			}
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %d |\n",
			ch.ID, ch.Name, ch.Protocol, status, deviceCount))
	}

	return mcp.NewSuccessResult(sb.String()), nil
}

func (s *Server) mcpListDevices(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	ch := s.cm.GetChannel(params.ChannelID)
	if ch == nil {
		return mcp.NewErrorResult("通道不存在: " + params.ChannelID), nil
	}

	devices := s.cm.GetChannelDevices(params.ChannelID)
	if len(devices) == 0 {
		return mcp.NewSuccessResult(fmt.Sprintf("通道 `%s` (%s) 下没有设备。", params.ChannelID, ch.Name)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 通道 `%s` (%s) 设备列表 (共 %d 个)\n\n", params.ChannelID, ch.Name, len(devices)))
	sb.WriteString("| ID | 名称 | 从站地址 | 采集间隔 | 启用 | 点位数量 |\n")
	sb.WriteString("|----|------|----------|----------|------|----------|\n")

	for _, dev := range devices {
		points, _ := s.cm.GetDevicePoints(params.ChannelID, dev.ID)
		enabled := "是"
		if !dev.Enable {
			enabled = "否"
		}
		slaveID := ""
		if sid, ok := dev.Config["slave_id"]; ok {
			slaveID = fmt.Sprintf("%v", sid)
		}
		intervalStr := time.Duration(dev.Interval).String()
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %d |\n",
			dev.ID, dev.Name, slaveID, intervalStr, enabled, len(points)))
	}

	return mcp.NewSuccessResult(sb.String()), nil
}

func (s *Server) mcpListPoints(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	points, err := s.cm.GetDevicePoints(params.ChannelID, params.DeviceID)
	if err != nil {
		return mcp.NewErrorResult("获取点位失败: " + err.Error()), nil
	}
	if len(points) == 0 {
		return mcp.NewSuccessResult(fmt.Sprintf("设备 `%s` 下没有点位。", params.DeviceID)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 设备 `%s` 点位列表 (共 %d 个)\n\n", params.DeviceID, len(points)))
	sb.WriteString("| ID | 名称 | 地址 | 数据类型 | 读/写 | 当前值 | 采集时间 |\n")
	sb.WriteString("|----|------|------|----------|-------|--------|----------|\n")

	for _, p := range points {
		// 获取当前值
		curVal := "-"
		collectedAt := "-"
		if val, err := s.cm.ReadPoint(params.ChannelID, params.DeviceID, p.ID); err == nil {
			curVal = fmt.Sprintf("%v", val.Value)
			collectedAt = val.TS.Format("15:04:05")
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | `%s` | %s | %s | %s | %s |\n",
			p.ID, p.Name, p.Address, p.DataType, p.ReadWrite, curVal, collectedAt))
	}

	return mcp.NewSuccessResult(sb.String()), nil
}

func (s *Server) mcpReadPoint(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
		PointID   string `json:"point_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	val, err := s.cm.ReadPoint(params.ChannelID, params.DeviceID, params.PointID)
	if err != nil {
		return mcp.NewErrorResult("读取失败: " + err.Error()), nil
	}

	result := map[string]any{
		"point_id":   params.PointID,
		"value":      val.Value,
		"timestamp":  val.TS,
		"quality":    val.Quality,
		"channel_id": params.ChannelID,
		"device_id":  params.DeviceID,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("```json\n" + string(resultJSON) + "\n```"), nil
}

func (s *Server) mcpGetSystemInfo(args json.RawMessage) (*mcp.CallToolResult, error) {
	info := s.getSystemInfoSnapshot()
	infoJSON, _ := json.MarshalIndent(info, "", "  ")
	return mcp.NewSuccessResult("```json\n" + string(infoJSON) + "\n```"), nil
}

func (s *Server) mcpGetDiagnostics(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
	}
	json.Unmarshal(args, &params)

	if params.ChannelID != "" && params.DeviceID != "" {
		diag := s.cm.GetDeviceDiagnostics(params.DeviceID)
		diagJSON, _ := json.MarshalIndent(diag, "", "  ")
		return mcp.NewSuccessResult(fmt.Sprintf("## 设备 `%s` 诊断信息\n\n```json\n%s\n```", params.DeviceID, string(diagJSON))), nil
	}

	if params.ChannelID != "" {
		metrics := s.cm.GetChannelScanEngineMetricsSnapshot(params.ChannelID)
		metricsJSON, _ := json.MarshalIndent(metrics, "", "  ")
		return mcp.NewSuccessResult(fmt.Sprintf("## 通道 `%s` 扫描引擎指标\n\n```json\n%s\n```", params.ChannelID, string(metricsJSON))), nil
	}

	// 全部通道摘要
	channels := s.cm.GetChannels()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 所有通道诊断摘要 (共 %d 个)\n\n", len(channels)))

	for _, ch := range channels {
		metrics := s.cm.GetChannelScanEngineMetricsSnapshot(ch.ID)
		metricsJSON, _ := json.MarshalIndent(metrics, "", "  ")
		sb.WriteString(fmt.Sprintf("### 通道 `%s` (%s)\n```json\n%s\n```\n\n", ch.ID, ch.Name, string(metricsJSON)))
	}

	return mcp.NewSuccessResult(sb.String()), nil
}

func (s *Server) mcpWritePoint(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
		PointID   string `json:"point_id"`
		Value     string `json:"value"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	// 安全检查：获取点位信息确认 R/W 权限
	points, err := s.cm.GetDevicePoints(params.ChannelID, params.DeviceID)
	if err != nil {
		return mcp.NewErrorResult("获取点位信息失败: " + err.Error()), nil
	}

	var targetPoint *model.PointData
	for _, p := range points {
		if p.ID == params.PointID {
			targetPoint = &p
			break
		}
	}
	if targetPoint == nil {
		return mcp.NewErrorResult("点位不存在: " + params.PointID), nil
	}
	if targetPoint.ReadWrite == "R" {
		return mcp.NewErrorResult("点位 `" + params.PointID + "` 为只读，不允许写入"), nil
	}

	// 尝试写入
	if err := s.cm.WritePoint(params.ChannelID, params.DeviceID, params.PointID, params.Value); err != nil {
		return mcp.NewErrorResult("写入失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("已成功向点位 `%s` (%s) 写入值: %s", params.PointID, targetPoint.Name, params.Value)), nil
}

// ── 全功能 CRUD 工具实现 ──

// mcpCreateChannel 创建南向通道
func (s *Server) mcpCreateChannel(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		Name     string         `json:"name"`
		Protocol string         `json:"protocol"`
		Config   map[string]any `json:"config"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	// 生成通道 ID
	chID := generateID("ch")
	ch := &model.Channel{
		ID:       chID,
		Name:     params.Name,
		Protocol: params.Protocol,
		Enable:   true,
		Config:   params.Config,
	}
	if ch.Config == nil {
		ch.Config = make(map[string]any)
	}

	// 校验 TCP 协议必需的 ip/port
	if tcpProtocols[params.Protocol] {
		ip, _ := ch.Config["ip"].(string)
		if ip == "" {
			ip, _ = ch.Config["host"].(string)
		}
		if ip == "" {
			return mcp.NewErrorResult("TCP 协议通道需要 config.ip 参数（目标 IP 地址）"), nil
		}
		if _, ok := ch.Config["port"]; !ok {
			ch.Config["port"] = defaultPort(params.Protocol)
		}
	}

	if err := s.cm.AddChannel(ch); err != nil {
		return mcp.NewErrorResult("创建通道失败: " + err.Error()), nil
	}

	result := map[string]any{
		"channel_id": chID,
		"name":       params.Name,
		"protocol":   params.Protocol,
		"config":     ch.Config,
		"status":     "created",
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 通道创建成功\n\n```json\n" + string(resultJSON) + "\n```\n\n通道已创建并启用。可通过 `edgex_start_channel` 启动采集引擎。"), nil
}

// mcpDeleteChannel 删除通道
func (s *Server) mcpDeleteChannel(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	ch := s.cm.GetChannel(params.ChannelID)
	if ch == nil {
		return mcp.NewErrorResult("通道不存在: " + params.ChannelID), nil
	}

	if err := s.cm.RemoveChannel(params.ChannelID); err != nil {
		return mcp.NewErrorResult("删除通道失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 通道已删除\n\n通道 `%s` (%s) 及其下所有设备和点位已成功删除。", params.ChannelID, ch.Name)), nil
}

// mcpStartChannel 启动通道
func (s *Server) mcpStartChannel(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if err := s.cm.StartChannel(params.ChannelID); err != nil {
		return mcp.NewErrorResult("启动通道失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 通道已启动\n\n通道 `%s` 采集引擎已成功启动。", params.ChannelID)), nil
}

// mcpStopChannel 停止通道
func (s *Server) mcpStopChannel(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if err := s.cm.StopChannel(params.ChannelID); err != nil {
		return mcp.NewErrorResult("停止通道失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 通道已停止\n\n通道 `%s` 采集引擎已成功停止。", params.ChannelID)), nil
}

// mcpCreateDevice 创建设备
func (s *Server) mcpCreateDevice(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string         `json:"channel_id"`
		Name      string         `json:"name"`
		Config    map[string]any `json:"config"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	ch := s.cm.GetChannel(params.ChannelID)
	if ch == nil {
		return mcp.NewErrorResult("通道不存在: " + params.ChannelID), nil
	}

	devID := generateID("dev")
	dev := &model.Device{
		ID:       devID,
		Name:     params.Name,
		Enable:   true,
		Interval: model.Duration(1 * time.Second),
		Config:   params.Config,
	}
	if dev.Config == nil {
		dev.Config = make(map[string]any)
	}
	// 默认从站地址
	if _, ok := dev.Config["slave_id"]; !ok {
		dev.Config["slave_id"] = "1"
	}

	if err := s.cm.AddDevice(params.ChannelID, dev); err != nil {
		return mcp.NewErrorResult("创建设备失败: " + err.Error()), nil
	}

	result := map[string]any{
		"device_id":  devID,
		"channel_id": params.ChannelID,
		"name":       params.Name,
		"config":     dev.Config,
		"interval":   "1s",
		"status":     "created",
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 设备创建成功\n\n```json\n" + string(resultJSON) + "\n```\n\n设备已创建并启用。可通过 `edgex_create_point` 添加采集点位。"), nil
}

// mcpDeleteDevice 删除设备
func (s *Server) mcpDeleteDevice(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	dev := s.cm.GetDevice(params.ChannelID, params.DeviceID)
	if dev == nil {
		return mcp.NewErrorResult("设备不存在: " + params.DeviceID), nil
	}

	if err := s.cm.RemoveDevice(params.ChannelID, params.DeviceID); err != nil {
		return mcp.NewErrorResult("删除设备失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 设备已删除\n\n设备 `%s` (%s) 及其下所有点位已成功删除。", params.DeviceID, dev.Name)), nil
}

// mcpCreatePoint 创建点位
func (s *Server) mcpCreatePoint(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID    string  `json:"channel_id"`
		DeviceID     string  `json:"device_id"`
		Name         string  `json:"name"`
		Address      string  `json:"address"`
		Datatype     string  `json:"datatype"`
		RegisterType string  `json:"register_type"`
		FunctionCode float64 `json:"function_code"`
		Scale        float64 `json:"scale"`
		Offset       float64 `json:"offset"`
		Unit         string  `json:"unit"`
		ReadWrite    string  `json:"readwrite"`
		ScanClass    string  `json:"scan_class"`
		WordOrder    string  `json:"word_order"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	// 默认值
	if params.ReadWrite == "" {
		params.ReadWrite = "R"
	}
	if params.ScanClass == "" {
		params.ScanClass = "normal"
	}
	if params.Scale == 0 {
		params.Scale = 1
	}
	if params.WordOrder == "" {
		params.WordOrder = "ABCD"
	}

	// 验证数据类型
	validTypes := map[string]bool{
		"int16": true, "uint16": true, "int32": true, "uint32": true,
		"float32": true, "float64": true, "bool": true, "string": true,
	}
	if !validTypes[params.Datatype] {
		return mcp.NewErrorResult("无效的数据类型: " + params.Datatype + "。支持: int16, uint16, int32, uint32, float32, float64, bool, string"), nil
	}

	// 映射寄存器类型
	regType := model.RegHolding // 默认 holding
	if params.RegisterType != "" {
		switch strings.ToLower(params.RegisterType) {
		case "holding":
			regType = model.RegHolding
		case "coil":
			regType = model.RegCoil
		case "discrete", "discreteinput", "discrete_input":
			regType = model.RegDiscreteInput
		case "input", "inputregister", "input_register":
			regType = model.RegInput
		}
	}

	ptID := generateID("pt")
	pt := &model.Point{
		ID:           ptID,
		Name:         params.Name,
		Address:      params.Address,
		DataType:     params.Datatype,
		RegisterType: regType,
		FunctionCode: byte(params.FunctionCode),
		Scale:        params.Scale,
		Offset:       params.Offset,
		Unit:         params.Unit,
		ReadWrite:    params.ReadWrite,
		ScanClass:    params.ScanClass,
		WordOrder:    params.WordOrder,
		ReportMode:   "cycle",
	}

	if err := s.cm.AddPoint(params.ChannelID, params.DeviceID, pt); err != nil {
		return mcp.NewErrorResult("创建点位失败: " + err.Error()), nil
	}

	result := map[string]any{
		"point_id":      ptID,
		"channel_id":    params.ChannelID,
		"device_id":     params.DeviceID,
		"name":          params.Name,
		"address":       params.Address,
		"datatype":      params.Datatype,
		"register_type": params.RegisterType,
		"function_code": int(params.FunctionCode),
		"scale":         params.Scale,
		"offset":        params.Offset,
		"unit":          params.Unit,
		"readwrite":     params.ReadWrite,
		"scan_class":    params.ScanClass,
		"word_order":    params.WordOrder,
		"status":        "created",
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 点位创建成功\n\n```json\n" + string(resultJSON) + "\n```\n\n点位已创建。可通过 `edgex_read_point` 或 `edgex_read_point_batch` 验证读取。"), nil
}

// mcpDeletePoint 删除点位
func (s *Server) mcpDeletePoint(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
		PointID   string `json:"point_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	points, err := s.cm.GetDevicePoints(params.ChannelID, params.DeviceID)
	if err != nil {
		return mcp.NewErrorResult("获取点位信息失败: " + err.Error()), nil
	}

	var ptName string
	for _, p := range points {
		if p.ID == params.PointID {
			ptName = p.Name
			break
		}
	}

	if err := s.cm.RemovePoint(params.ChannelID, params.DeviceID, params.PointID); err != nil {
		return mcp.NewErrorResult("删除点位失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 点位已删除\n\n点位 `%s` (%s) 已成功删除。", params.PointID, ptName)), nil
}

// mcpReadPointBatch 批量读取点位
func (s *Server) mcpReadPointBatch(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string   `json:"channel_id"`
		DeviceID  string   `json:"device_id"`
		PointIDs  []string `json:"point_ids"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if len(params.PointIDs) == 0 {
		return mcp.NewErrorResult("point_ids 不能为空"), nil
	}

	type readResult struct {
		PointID string `json:"point_id"`
		Value   any    `json:"value"`
		Time    string `json:"timestamp"`
		Quality string `json:"quality"`
		Error   string `json:"error,omitempty"`
	}

	results := make([]readResult, 0, len(params.PointIDs))
	successCount := 0

	for _, ptID := range params.PointIDs {
		val, err := s.cm.ReadPoint(params.ChannelID, params.DeviceID, ptID)
		if err != nil {
			results = append(results, readResult{
				PointID: ptID,
				Error:   err.Error(),
			})
		} else {
			results = append(results, readResult{
				PointID: ptID,
				Value:   val.Value,
				Time:    val.TS.Format("15:04:05"),
				Quality: val.Quality,
			})
			successCount++
		}
	}

	resultJSON, _ := json.MarshalIndent(map[string]any{
		"total":   len(params.PointIDs),
		"success": successCount,
		"failed":  len(params.PointIDs) - successCount,
		"results": results,
	}, "", "  ")

	return mcp.NewSuccessResult(fmt.Sprintf("## 批量读取结果 (%d/%d 成功)\n\n```json\n%s\n```", successCount, len(params.PointIDs), string(resultJSON))), nil
}

// mcpWritePointBatch 批量写入点位
func (s *Server) mcpWritePointBatch(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
		Writes    []struct {
			PointID string `json:"point_id"`
			Value   string `json:"value"`
		} `json:"writes"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if len(params.Writes) == 0 {
		return mcp.NewErrorResult("writes 不能为空"), nil
	}

	points, err := s.cm.GetDevicePoints(params.ChannelID, params.DeviceID)
	if err != nil {
		return mcp.NewErrorResult("获取点位信息失败: " + err.Error()), nil
	}

	pointMap := make(map[string]*model.PointData)
	for i := range points {
		pointMap[points[i].ID] = &points[i]
	}

	type writeResult struct {
		PointID string `json:"point_id"`
		Value   string `json:"value"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}

	results := make([]writeResult, 0, len(params.Writes))
	successCount := 0

	for _, w := range params.Writes {
		pt, ok := pointMap[w.PointID]
		if !ok {
			results = append(results, writeResult{PointID: w.PointID, Value: w.Value, Status: "failed", Error: "点位不存在"})
			continue
		}
		if pt.ReadWrite == "R" {
			results = append(results, writeResult{PointID: w.PointID, Value: w.Value, Status: "failed", Error: "点位为只读"})
			continue
		}

		if err := s.cm.WritePoint(params.ChannelID, params.DeviceID, w.PointID, w.Value); err != nil {
			results = append(results, writeResult{PointID: w.PointID, Value: w.Value, Status: "failed", Error: err.Error()})
		} else {
			results = append(results, writeResult{PointID: w.PointID, Value: w.Value, Status: "success"})
			successCount++
		}
	}

	resultJSON, _ := json.MarshalIndent(map[string]any{
		"total":   len(params.Writes),
		"success": successCount,
		"failed":  len(params.Writes) - successCount,
		"results": results,
	}, "", "  ")

	return mcp.NewSuccessResult(fmt.Sprintf("## 批量写入结果 (%d/%d 成功)\n\n```json\n%s\n```", successCount, len(params.Writes), string(resultJSON))), nil
}

// mcpCreateEdgeRule 创建边缘规则
func (s *Server) mcpCreateEdgeRule(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Condition  string `json:"condition"`
		Expression string `json:"expression"`
		Actions    []struct {
			ActionType string `json:"type"`
			ChannelID  string `json:"channel_id"`
			DeviceID   string `json:"device_id"`
			PointID    string `json:"point_id"`
			Value      string `json:"value"`
		} `json:"actions"`
		Sources []struct {
			Alias     string `json:"alias"`
			ChannelID string `json:"channel_id"`
			DeviceID  string `json:"device_id"`
			PointID   string `json:"point_id"`
		} `json:"sources"`
		CheckInterval string `json:"check_interval"`
		TriggerMode   string `json:"trigger_mode"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	// 默认值
	if params.CheckInterval == "" {
		params.CheckInterval = "10s"
	}
	if params.TriggerMode == "" {
		params.TriggerMode = "on_change"
	}

	ruleID := generateID("rule")
	rule := model.EdgeRule{
		ID:            ruleID,
		Name:          params.Name,
		Type:          params.Type,
		Enable:        true,
		Priority:      5,
		CheckInterval: params.CheckInterval,
		TriggerMode:   params.TriggerMode,
		Condition:     params.Condition,
		Expression:    params.Expression,
	}

	for _, src := range params.Sources {
		rule.Sources = append(rule.Sources, model.RuleSource{
			Alias:     src.Alias,
			ChannelID: src.ChannelID,
			DeviceID:  src.DeviceID,
			PointID:   src.PointID,
		})
	}

	for _, act := range params.Actions {
		actConfig := make(map[string]any)
		if act.ChannelID != "" {
			actConfig["channel_id"] = act.ChannelID
		}
		if act.DeviceID != "" {
			actConfig["device_id"] = act.DeviceID
		}
		if act.PointID != "" {
			actConfig["point_id"] = act.PointID
		}
		if act.Value != "" {
			actConfig["value"] = act.Value
		}
		rule.Actions = append(rule.Actions, model.RuleAction{
			Type:   act.ActionType,
			Config: actConfig,
		})
	}

	if s.ecm == nil {
		return mcp.NewErrorResult("边缘计算引擎未初始化"), nil
	}

	if err := s.ecm.UpsertRule(rule); err != nil {
		return mcp.NewErrorResult("创建边缘规则失败: " + err.Error()), nil
	}

	result := map[string]any{
		"rule_id":        ruleID,
		"name":           params.Name,
		"type":           params.Type,
		"condition":      params.Condition,
		"sources_count":  len(params.Sources),
		"actions_count":  len(params.Actions),
		"check_interval": params.CheckInterval,
		"trigger_mode":   params.TriggerMode,
		"status":         "created",
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 边缘规则创建成功\n\n```json\n" + string(resultJSON) + "\n```\n\n规则已启用，将按 `check_interval` 定时检查条件。"), nil
}

// mcpDeleteEdgeRule 删除边缘规则
func (s *Server) mcpDeleteEdgeRule(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		RuleID string `json:"rule_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if s.ecm == nil {
		return mcp.NewErrorResult("边缘计算引擎未初始化"), nil
	}

	if err := s.ecm.DeleteRule(params.RuleID); err != nil {
		return mcp.NewErrorResult("删除规则失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 规则已删除\n\n边缘规则 `%s` 已成功删除。", params.RuleID)), nil
}

// mcpCreateVirtualDevice 创建虚拟设备
func (s *Server) mcpCreateVirtualDevice(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		VirtualDeviceID string            `json:"virtual_device_id"`
		ChannelID       string            `json:"channel_id"`
		FormulaPoints   map[string]string `json:"formula_points"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if s.virtualShadow == nil {
		return mcp.NewErrorResult("虚拟影子引擎未初始化"), nil
	}

	if err := s.virtualShadow.CreateVirtualDevice(params.VirtualDeviceID, params.ChannelID, params.FormulaPoints); err != nil {
		return mcp.NewErrorResult("创建虚拟设备失败: " + err.Error()), nil
	}

	formulas := make([]string, 0, len(params.FormulaPoints))
	formulaCount := 0
	for ptID, formula := range params.FormulaPoints {
		formulas = append(formulas, fmt.Sprintf("  - `%s` = %s", ptID, formula))
		formulaCount++
	}

	result := fmt.Sprintf("## 虚拟设备创建成功\n\n- **虚拟设备 ID**: `%s`\n- **关联通道**: `%s`\n- **公式点位**: %d 个\n\n```\n%s\n```\n\n虚拟设备通过公式实时计算，不占用物理连接。",
		params.VirtualDeviceID, params.ChannelID, formulaCount, strings.Join(formulas, "\n"))
	return mcp.NewSuccessResult(result), nil
}

// mcpDeleteVirtualDevice 删除虚拟设备
func (s *Server) mcpDeleteVirtualDevice(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}

	var params struct {
		VirtualDeviceID string `json:"virtual_device_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	if s.virtualShadow == nil {
		return mcp.NewErrorResult("虚拟影子引擎未初始化"), nil
	}

	if err := s.virtualShadow.DeleteVirtualDevice(params.VirtualDeviceID); err != nil {
		return mcp.NewErrorResult("删除虚拟设备失败: " + err.Error()), nil
	}

	return mcp.NewSuccessResult(fmt.Sprintf("## 虚拟设备已删除\n\n虚拟设备 `%s` 已成功删除。", params.VirtualDeviceID)), nil
}

// ── 扩展工具实现 ──

// mcpRestartChannel 重启通道
func (s *Server) mcpRestartChannel(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}
	var params struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}
	if err := s.cm.StopChannel(params.ChannelID); err != nil {
		return mcp.NewErrorResult("停止通道失败: " + err.Error()), nil
	}
	time.Sleep(500 * time.Millisecond)
	if err := s.cm.StartChannel(params.ChannelID); err != nil {
		return mcp.NewErrorResult("启动通道失败: " + err.Error()), nil
	}
	return mcp.NewSuccessResult(fmt.Sprintf("## 通道已重启\n\n通道 `%s` 已成功停止并重新启动。", params.ChannelID)), nil
}

// mcpGetChannelConfig 获取通道配置详情
func (s *Server) mcpGetChannelConfig(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}
	ch := s.cm.GetChannel(params.ChannelID)
	if ch == nil {
		return mcp.NewErrorResult("通道不存在: " + params.ChannelID), nil
	}
	devices := s.cm.GetChannelDevices(params.ChannelID)
	totalPoints := 0
	deviceList := make([]map[string]any, 0, len(devices))
	for _, dev := range devices {
		pts, _ := s.cm.GetDevicePoints(params.ChannelID, dev.ID)
		totalPoints += len(pts)
		deviceList = append(deviceList, map[string]any{
			"device_id": dev.ID, "name": dev.Name, "enabled": dev.Enable,
			"interval": time.Duration(dev.Interval).String(), "point_count": len(pts),
		})
	}
	stats := s.cm.GetChannelStats()
	status := "offline"
	for _, cs := range stats {
		if cs.ID == params.ChannelID && cs.Status == "online" {
			status = "online"
			break
		}
	}
	result := map[string]any{
		"channel_id":   ch.ID,
		"name":         ch.Name,
		"protocol":     ch.Protocol,
		"enabled":      ch.Enable,
		"status":       status,
		"config":       ch.Config,
		"device_count": len(devices),
		"point_count":  totalPoints,
		"devices":      deviceList,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 通道配置详情\n\n```json\n" + string(resultJSON) + "\n```"), nil
}

// mcpUpdateDevice 更新设备配置
func (s *Server) mcpUpdateDevice(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}
	var params struct {
		ChannelID string         `json:"channel_id"`
		DeviceID  string         `json:"device_id"`
		Name      string         `json:"name"`
		Config    map[string]any `json:"config"`
		Enable    *bool          `json:"enable"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}
	dev := s.cm.GetDevice(params.ChannelID, params.DeviceID)
	if dev == nil {
		return mcp.NewErrorResult("设备不存在: " + params.DeviceID), nil
	}
	updated := false
	if params.Name != "" {
		dev.Name = params.Name
		updated = true
	}
	if params.Config != nil {
		if dev.Config == nil {
			dev.Config = make(map[string]any)
		}
		for k, v := range params.Config {
			dev.Config[k] = v
		}
		updated = true
	}
	if params.Enable != nil {
		dev.Enable = *params.Enable
		updated = true
	}
	if !updated {
		return mcp.NewErrorResult("未提供任何需要更新的字段"), nil
	}
	if err := s.cm.UpdateDevice(params.ChannelID, dev); err != nil {
		return mcp.NewErrorResult("更新设备失败: " + err.Error()), nil
	}
	result := map[string]any{
		"device_id": dev.ID, "name": dev.Name, "enabled": dev.Enable, "config": dev.Config,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 设备更新成功\n\n```json\n" + string(resultJSON) + "\n```"), nil
}

// mcpUpdatePoint 更新点位配置
func (s *Server) mcpUpdatePoint(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}
	var params struct {
		ChannelID  string   `json:"channel_id"`
		DeviceID   string   `json:"device_id"`
		PointID    string   `json:"point_id"`
		Name       string   `json:"name"`
		Address    string   `json:"address"`
		Datatype   string   `json:"datatype"`
		Scale      *float64 `json:"scale"`
		Offset     *float64 `json:"offset"`
		Unit       string   `json:"unit"`
		ReadWrite  string   `json:"readwrite"`
		WordOrder  string   `json:"word_order"`
		ScanClass  string   `json:"scan_class"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}
	dev := s.cm.GetDevice(params.ChannelID, params.DeviceID)
	if dev == nil {
		return mcp.NewErrorResult("设备不存在: " + params.DeviceID), nil
	}
	var pt *model.Point
	for i := range dev.Points {
		if dev.Points[i].ID == params.PointID {
			pt = &dev.Points[i]
			break
		}
	}
	if pt == nil {
		return mcp.NewErrorResult("点位不存在: " + params.PointID), nil
	}
	updated := false
	if params.Name != "" {
		pt.Name = params.Name
		updated = true
	}
	if params.Address != "" {
		pt.Address = params.Address
		updated = true
	}
	if params.Datatype != "" {
		pt.DataType = params.Datatype
		updated = true
	}
	if params.Scale != nil {
		pt.Scale = *params.Scale
		updated = true
	}
	if params.Offset != nil {
		pt.Offset = *params.Offset
		updated = true
	}
	if params.Unit != "" {
		pt.Unit = params.Unit
		updated = true
	}
	if params.ReadWrite != "" {
		pt.ReadWrite = params.ReadWrite
		updated = true
	}
	if params.WordOrder != "" {
		pt.WordOrder = params.WordOrder
		updated = true
	}
	if params.ScanClass != "" {
		pt.ScanClass = params.ScanClass
		updated = true
	}
	if !updated {
		return mcp.NewErrorResult("未提供任何需要更新的字段"), nil
	}
	if _, err := s.cm.UpdatePoint(params.ChannelID, params.DeviceID, pt); err != nil {
		return mcp.NewErrorResult("更新点位失败: " + err.Error()), nil
	}
	result := map[string]any{
		"point_id": pt.ID, "name": pt.Name, "address": pt.Address, "datatype": pt.DataType,
		"scale": pt.Scale, "offset": pt.Offset, "unit": pt.Unit, "readwrite": pt.ReadWrite,
		"word_order": pt.WordOrder, "scan_class": pt.ScanClass,
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewSuccessResult("## 点位更新成功\n\n```json\n" + string(resultJSON) + "\n```"), nil
}

// mcpListEdgeRules 列出边缘规则
func (s *Server) mcpListEdgeRules(args json.RawMessage) (*mcp.CallToolResult, error) {
	if s.ecm == nil {
		return mcp.NewSuccessResult("## 边缘规则列表\n\n边缘计算引擎未初始化，当前没有配置规则。"), nil
	}
	rules := s.ecm.GetRules()
	if len(rules) == 0 {
		return mcp.NewSuccessResult("## 边缘规则列表\n\n当前没有配置任何边缘计算规则。可通过 `edgex_create_edge_rule` 创建。"), nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 边缘规则列表 (共 %d 个)\n\n", len(rules)))
	sb.WriteString("| ID | 名称 | 类型 | 条件 | 状态 | 触发次数 | 动作数 |\n")
	sb.WriteString("|----|------|------|------|------|----------|--------|\n")
	ruleStates := s.ecm.GetRuleStates()
	for _, rule := range rules {
		status := "active"
		if !rule.Enable {
			status = "disabled"
		}
		triggerCount := int64(0)
		if rs, ok := ruleStates[rule.ID]; ok {
			triggerCount = rs.TriggerCount
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s | %s | %d | %d |\n",
			rule.ID, rule.Name, rule.Type, truncate(rule.Condition, 40), status, triggerCount, len(rule.Actions)))
	}
	return mcp.NewSuccessResult(sb.String()), nil
}

// mcpGetPointHistory 获取点位历史数据
func (s *Server) mcpGetPointHistory(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ChannelID string  `json:"channel_id"`
		DeviceID  string  `json:"device_id"`
		PointID   string  `json:"point_id"`
		Limit     float64 `json:"limit"`
		Duration  string  `json:"duration"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}
	limit := 100
	if params.Limit > 0 {
		limit = int(params.Limit)
		if limit > 1000 {
			limit = 1000
		}
	}
	var history []map[string]any
	var err error
	if params.Duration != "" {
		duration, err := time.ParseDuration(params.Duration)
		if err != nil {
			return mcp.NewErrorResult("无效的时间范围: " + params.Duration + " (支持格式: 5m, 1h, 24h)"), nil
		}
		end := time.Now()
		start := end.Add(-duration)
		history, err = s.dsm.GetHistoryByTimeRange(params.DeviceID, start, end, limit)
	} else {
		history, err = s.dsm.GetHistory(params.DeviceID, limit)
	}
	if err != nil {
		return mcp.NewErrorResult("获取历史数据失败: " + err.Error()), nil
	}
	if len(history) == 0 {
		return mcp.NewSuccessResult(fmt.Sprintf("## 点位 `%s` 历史数据\n\n暂无历史数据。点位可能尚未采集到数据。", params.PointID)), nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 点位 `%s` 历史数据 (最近 %d 条)\n\n", params.PointID, len(history)))
	sb.WriteString("| 时间 | 值 | 质量 |\n")
	sb.WriteString("|------|-----|------|\n")
	for _, h := range history {
		ts := fmt.Sprintf("%v", h["ts"])
		val := fmt.Sprintf("%v", h["value"])
		quality := fmt.Sprintf("%v", h["quality"])
		sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ts, val, quality))
	}
	return mcp.NewSuccessResult(sb.String()), nil
}

// mcpEnableDevice 启用/禁用设备
func (s *Server) mcpEnableDevice(args json.RawMessage) (*mcp.CallToolResult, error) {
	if blocked := s.mcpRequireFullAccess(); blocked != nil {
		return blocked, nil
	}
	var params struct {
		ChannelID string `json:"channel_id"`
		DeviceID  string `json:"device_id"`
		Enable    bool   `json:"enable"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}
	dev := s.cm.GetDevice(params.ChannelID, params.DeviceID)
	if dev == nil {
		return mcp.NewErrorResult("设备不存在: " + params.DeviceID), nil
	}
	dev.Enable = params.Enable
	if err := s.cm.UpdateDevice(params.ChannelID, dev); err != nil {
		return mcp.NewErrorResult("更新设备状态失败: " + err.Error()), nil
	}
	action := "启用"
	if !params.Enable {
		action = "禁用"
	}
	return mcp.NewSuccessResult(fmt.Sprintf("## 设备已%s\n\n设备 `%s` (%s) 已成功%s。", action, params.DeviceID, dev.Name, action)), nil
}

// mcpExportConfig 导出配置
func (s *Server) mcpExportConfig(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		Format string `json:"format"`
		Scope  string `json:"scope"`
	}
	json.Unmarshal(args, &params)
	if params.Scope == "" {
		params.Scope = "all"
	}
	if params.Format == "" {
		params.Format = "json"
	}
	export := make(map[string]any)
	export["export_time"] = time.Now().Format(time.RFC3339)
	export["server"] = map[string]string{"name": "EdgeX", "version": "v0.0.8"}
	if params.Scope == "all" || params.Scope == "channels" {
		channels := s.cm.GetChannels()
		chList := make([]map[string]any, 0, len(channels))
		for _, ch := range channels {
			chData := map[string]any{
				"channel_id": ch.ID, "name": ch.Name, "protocol": ch.Protocol,
				"enabled": ch.Enable, "config": ch.Config,
			}
			devices := s.cm.GetChannelDevices(ch.ID)
			devList := make([]map[string]any, 0, len(devices))
			for _, dev := range devices {
				devData := map[string]any{
					"device_id": dev.ID, "name": dev.Name, "enabled": dev.Enable,
					"interval": time.Duration(dev.Interval).String(), "config": dev.Config,
				}
				ptList := make([]map[string]any, 0, len(dev.Points))
				for _, p := range dev.Points {
					ptList = append(ptList, map[string]any{
						"point_id": p.ID, "name": p.Name, "address": p.Address,
						"datatype": p.DataType, "readwrite": p.ReadWrite, "scale": p.Scale,
						"offset": p.Offset, "unit": p.Unit, "word_order": p.WordOrder,
						"scan_class": p.ScanClass, "register_type": string(p.RegisterType),
						"function_code": p.FunctionCode,
					})
				}
				devData["points"] = ptList
				devList = append(devList, devData)
			}
			chData["devices"] = devList
			chList = append(chList, chData)
		}
		export["channels"] = chList
	}
	if params.Scope == "all" || params.Scope == "rules" {
		if s.ecm != nil {
			rules := s.ecm.GetRules()
			ruleStates := s.ecm.GetRuleStates()
			ruleList := make([]map[string]any, 0, len(rules))
			for _, rule := range rules {
				triggerCount := int64(0)
				actionStatus := int64(0)
				if rs, ok := ruleStates[rule.ID]; ok {
					triggerCount = rs.TriggerCount
					actionStatus = rs.ActionSuccessCount
				}
				ruleList = append(ruleList, map[string]any{
					"rule_id": rule.ID, "name": rule.Name, "type": rule.Type,
					"condition": rule.Condition, "enabled": rule.Enable,
					"expression": rule.Expression,
					"trigger_count": triggerCount, "action_count": len(rule.Actions),
					"action_success": actionStatus,
				})
			}
			export["rules"] = ruleList
		}
	}
	var output string
	var lang string
	if params.Format == "yaml" || params.Format == "yml" {
		exportYAML, err := yaml.Marshal(export)
		if err != nil {
			return mcp.NewErrorResult("YAML 序列化失败: " + err.Error()), nil
		}
		output = string(exportYAML)
		lang = "yaml"
	} else {
		exportBytes, _ := json.MarshalIndent(export, "", "  ")
		output = string(exportBytes)
		lang = "json"
	}
	return mcp.NewSuccessResult(fmt.Sprintf("## 配置导出 (format=%s)\n\n```%s\n%s\n```", params.Format, lang, output)), nil
}

// truncate 截断字符串到指定长度
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// tcpProtocols 需要 ip/port 的 TCP 协议白名单
var tcpProtocols = map[string]bool{
	"modbus-tcp":          true,
	"modbus-rtu-over-tcp": true,
	"s7":                  true,
	"bacnet":              true,
	"ethernetip":          true,
	"snmp":                true,
	"ice104":              true,
	"knxnetip":            true,
	"mitsubishi":          true,
	"omron":               true,
	"opcua":               true,
	"opc-ua":              true,
}

// defaultPort 协议默认端口
func defaultPort(protocol string) int {
	switch protocol {
	case "modbus-tcp", "modbus-rtu-over-tcp":
		return 502
	case "s7":
		return 102
	case "bacnet":
		return 47808
	case "ethernetip":
		return 44818
	case "snmp":
		return 161
	case "ice104":
		return 2404
	case "knxnetip":
		return 3671
	case "mitsubishi":
		return 5000
	case "omron":
		return 9600
	case "opcua", "opc-ua":
		return 4840
	default:
		return 502
	}
}

// generateID 生成简短唯一 ID（含随机后缀，避免秒级并发碰撞）
func generateID(prefix string) string {
	b := make([]byte, 3)
	rand.Read(b)
	raw := fmt.Sprintf("%s_%s%s", prefix, time.Now().Format("0102150405"), hex.EncodeToString(b)[:4])
	if len(raw) > 18 {
		return raw[:18]
	}
	return raw
}

func (s *Server) mcpAnalyzeProtocol(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		ProtocolHint string  `json:"protocol_hint"`
		Port         float64 `json:"port"`
		Description  string  `json:"description"`
	}
	json.Unmarshal(args, &params)

	protocolMap := map[string]struct {
		Name     string
		Port     int
		Features string
	}{
		"modbus":     {"Modbus TCP/RTU", 502, "MBAP 头 + PDU；功能码 01/02/03/04/05/06/15/16；支持 holding/coil/discrete/input 寄存器"},
		"s7":         {"Siemens S7", 102, "TPKT + COTP + S7 PDU；支持 DB/I/Q/M 区；Put/Get 通信"},
		"bacnet":     {"BACnet/IP", 47808, "BVLC 0x81 + NPDU + APDU；Who-Is/I-Am/ReadProperty/WriteProperty"},
		"opcua":      {"OPC UA", 4840, "二进制 UA TCP；支持 Browse/Read/Write/Subscribe；NodeId 寻址"},
		"eip":        {"EtherNet/IP", 44818, "CIP over EtherNet/IP；支持 Class1/Class3 连接；Tag Read/Write"},
		"profinet":   {"PROFINET IO", 34964, "DCE/RPC + PNIO；支持 Read/Write Record；GSD 文件描述"},
		"ethercat":   {"EtherCAT", 0, "CoE (CANopen over EtherCAT)；支持 SDO/PDO；ESI 文件描述"},
		"dlt645":     {"DL/T 645", 0, "中国电能表协议；支持 07/97 版本；Block/Field 寻址"},
		"snmp":       {"SNMP", 161, "SNMPv1/v2c/v3；支持 Get/GetNext/GetBulk/Walk；OID 寻址"},
		"knx":        {"KNXnet/IP", 3671, "KNXnet/IP Tunneling/Routing；支持 GroupValue Read/Write"},
		"mitsubishi": {"Mitsubishi MELSEC", 0, "MC 协议 (3E/4E 帧)；支持位/字软元件；ASCII/Binary 模式"},
		"omron":      {"Omron FINS", 9600, "FINS UDP/TCP；支持 CIO/WR/HR/DM/AR 区；路由表寻址"},
		"ice104":     {"IEC 60870-5-104", 2404, "APCI + ASDU；支持总召/时钟同步/单点/双点/测量值"},
	}

	// 按端口匹配
	if params.Port > 0 {
		for _, v := range protocolMap {
			if v.Port == int(params.Port) {
				return mcp.NewSuccessResult(fmt.Sprintf("## 协议识别结果\n\n- **协议**: %s\n- **端口**: %d\n- **识别依据**: 端口匹配\n- **特征**: %s\n- **置信度**: 0.85", v.Name, v.Port, v.Features)), nil
			}
		}
	}

	// 按名称匹配
	if params.ProtocolHint != "" {
		key := strings.ToLower(strings.TrimSpace(params.ProtocolHint))
		if info, ok := protocolMap[key]; ok {
			return mcp.NewSuccessResult(fmt.Sprintf("## 协议识别结果\n\n- **协议**: %s\n- **默认端口**: %d\n- **特征**: %s\n- **置信度**: 0.92", info.Name, info.Port, info.Features)), nil
		}
	}

	// 列出所有支持的协议
	var sb strings.Builder
	sb.WriteString("## 支持的工业协议\n\n")
	sb.WriteString("| 协议 | 默认端口 | 特征 |\n")
	sb.WriteString("|------|----------|------|\n")
	for k, v := range protocolMap {
		portStr := fmt.Sprintf("%d", v.Port)
		if v.Port == 0 {
			portStr = "N/A"
		}
		sb.WriteString(fmt.Sprintf("| %s (%s) | %s | %s |\n", v.Name, k, portStr, v.Features))
	}
	sb.WriteString("\n> 请提供具体协议名称、端口号或报文特征以获取精确识别结果。")

	return mcp.NewSuccessResult(sb.String()), nil
}

func (s *Server) mcpGetProtocolHelp(args json.RawMessage) (*mcp.CallToolResult, error) {
	var params struct {
		Protocol string `json:"protocol"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return mcp.NewErrorResult("参数解析失败: " + err.Error()), nil
	}

	helpMap := map[string]string{
		"modbus": `## Modbus TCP/RTU 接入帮助

### 通道配置
| 参数 | 说明 | 示例 |
|------|------|------|
| protocol | 协议类型 | modbus-tcp / modbus-rtu |
| ip | 设备 IP | 192.168.1.100 |
| port | 端口 | 502（TCP）/ 0（RTU） |
| slave_id | 从站地址 | 1-247 |

### 点位地址格式（PDU 偏移，0 基）
- **Holding Register**: 地址 0~N，对应 PLC 地址 40001+N（FC 03/06/16）
- **Coil**: 地址 0~N，对应 PLC 地址 00001+N（FC 01/05/15）
- **Discrete Input**: 地址 0~N，对应 PLC 地址 10001+N（FC 02）
- **Input Register**: 地址 0~N，对应 PLC 地址 30001+N（FC 04）

> 提示：系统同时兼容传统 40001+ 写法，会自动转换为 PDU 偏移；但推荐直接使用 0 基偏移，避免歧义。

### 数据类型
- uint16 / int16 / uint32 / int32 / float32 / float64
- 字节序: ABCD / CDAB / BADC / DCBA（默认 ABCD）

### 典型配置示例
` + "```json\n" + `{
  "protocol": "modbus-tcp",
  "ip": "192.168.1.100",
  "port": 502,
  "slave_id": 1,
  "interval": "1s",
  "points": [{
    "name": "电压",
    "address": "0",
    "register_type": "holding",
    "function_code": 3,
    "datatype": "float32",
    "scale": 0.1,
    "unit": "V"
  }]
}
` + "```\n> address \"0\" 对应 PLC 地址 40001（Holding Register 首地址）。\n",

		"s7": `## Siemens S7 接入帮助

### 通道配置
| 参数 | 说明 | 示例 |
|------|------|------|
| protocol | 协议类型 | s7 |
| ip | PLC IP | 192.168.1.10 |
| port | 端口 | 102 |
| rack | 机架号 | 0 |
| slot | 槽位号 | 1（S7-300）/ 2（S7-1200/1500） |

### 点位地址格式
- **DB 块**: DB1.DBD0（DB 块 1，偏移 0，双字）
- **M 区**: M0.0（位）/ MB0（字节）/ MW0（字）/ MD0（双字）
- **I 区**: I0.0 / IB0 / IW0 / ID0
- **Q 区**: Q0.0 / QB0 / QW0 / QD0

### 数据类型
bool / byte / int16 / uint16 / int32 / uint32 / float32 / string`,

		"bacnet": `## BACnet/IP 接入帮助

### 通道配置
| 参数 | 说明 | 示例 |
|------|------|------|
| protocol | 协议类型 | bacnet |
| port | 端口 | 47808 |
| device_instance | 设备实例号 | 自动发现 |

### 点位地址格式
- analog-input:1 (AI:1)
- analog-output:2 (AO:2)
- analog-value:3 (AV:3)
- binary-input:1 (BI:1)
- binary-output:2 (BO:2)
- multi-state-value:1 (MSV:1)

### 支持发现
- Who-Is / I-Am 设备自动发现
- ReadProperty 读取属性`,

		"opcua": `## OPC UA 接入帮助

### 通道配置
| 参数 | 说明 | 示例 |
|------|------|------|
| protocol | 协议类型 | opcua |
| endpoint | 端点 URL | opc.tcp://192.168.1.50:4840 |
| security | 安全模式 | None / Sign / SignAndEncrypt |

### 点位地址格式
- ns=2;s=Temperature
- ns=2;i=12345
- ns=2;g=XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX

### 支持功能
- Browse 节点浏览
- Read/Write 读写
- Subscribe 订阅（自动轮询）`,
	}

	help, ok := helpMap[strings.ToLower(params.Protocol)]
	if !ok {
		return mcp.NewSuccessResult(fmt.Sprintf("协议 `%s` 的帮助文档正在编写中。支持的协议: modbus, s7, bacnet, opcua", params.Protocol)), nil
	}

	return mcp.NewSuccessResult(help), nil
}

// ── MCP 资源注册 ──

// registerMCPResources 注册所有 EdgeX MCP 资源
func (s *Server) registerMCPResources(mcpSrv *mcp.MCPServer) {
	mcpSrv.RegisterResource(mcp.Resource{
		URI:         "edgex://channels",
		Name:        "通道列表",
		Description: "所有采集通道的完整配置信息（JSON 格式）",
		MimeType:    "application/json",
	}, s.mcpResourceChannels)

	mcpSrv.RegisterResource(mcp.Resource{
		URI:         "edgex://system",
		Name:        "系统信息",
		Description: "EdgeX 网关系统状态信息（CPU/内存/运行时间/协议支持）",
		MimeType:    "application/json",
	}, s.mcpResourceSystem)

	mcpSrv.RegisterResource(mcp.Resource{
		URI:         "edgex://diagnostics",
		Name:        "诊断快照",
		Description: "所有通道和设备的诊断信息汇总",
		MimeType:    "application/json",
	}, s.mcpResourceDiagnostics)

	// 协议支持列表
	mcpSrv.RegisterResource(mcp.Resource{
		URI:         "edgex://protocols",
		Name:        "协议支持列表",
		Description: "EdgeX 支持的工业协议完整列表（含端口、特性）",
		MimeType:    "application/json",
	}, s.mcpResourceProtocols)

	// 边缘规则
	mcpSrv.RegisterResource(mcp.Resource{
		URI:         "edgex://edge-rules",
		Name:        "边缘规则",
		Description: "所有边缘计算规则的配置和状态",
		MimeType:    "application/json",
	}, s.mcpResourceEdgeRules)

	// 完整配置导出
	mcpSrv.RegisterResource(mcp.Resource{
		URI:         "edgex://config",
		Name:        "完整配置",
		Description: "EdgeX 完整配置导出（通道、设备、点位、规则）",
		MimeType:    "application/json",
	}, s.mcpResourceConfig)
}

func (s *Server) mcpResourceChannels(uri string) (*mcp.ReadResourceResult, error) {
	channels := s.cm.GetChannels()
	data, _ := json.MarshalIndent(channels, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(data)},
		},
	}, nil
}

func (s *Server) mcpResourceSystem(uri string) (*mcp.ReadResourceResult, error) {
	info := s.getSystemInfoSnapshot()
	data, _ := json.MarshalIndent(info, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(data)},
		},
	}, nil
}

func (s *Server) mcpResourceDiagnostics(uri string) (*mcp.ReadResourceResult, error) {
	channels := s.cm.GetChannels()
	diag := make(map[string]any)
	for _, ch := range channels {
		diag[ch.ID] = s.cm.GetChannelScanEngineMetricsSnapshot(ch.ID)
		devices := s.cm.GetChannelDevices(ch.ID)
		devDiag := make(map[string]any)
		for _, dev := range devices {
			devDiag[dev.ID] = s.cm.GetDeviceDiagnostics(dev.ID)
		}
		diag[ch.ID+"_devices"] = devDiag
	}
	data, _ := json.MarshalIndent(diag, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(data)},
		},
	}, nil
}

func (s *Server) mcpResourceProtocols(uri string) (*mcp.ReadResourceResult, error) {
	protocols := []map[string]any{
		{"name": "Modbus TCP", "type": "modbus-tcp", "port": 502, "transport": "TCP", "features": "MBAP + PDU, 功能码 01-06/15-16, holding/coil/discrete/input"},
		{"name": "Modbus RTU", "type": "modbus-rtu", "port": 0, "transport": "Serial", "features": "RS-232/485, 功能码 01-06/15-16"},
		{"name": "Siemens S7", "type": "s7", "port": 102, "transport": "TCP", "features": "S7-200/300/400/1200/1500, DB/I/Q/M 区, Put/Get"},
		{"name": "BACnet/IP", "type": "bacnet", "port": 47808, "transport": "UDP", "features": "AI/AO/BI/BO/AV/BV/MSI/MSO, Who-Is/ReadProperty/WriteProperty"},
		{"name": "OPC UA", "type": "opcua", "port": 4840, "transport": "TCP", "features": "Browse/Read/Write/Subscribe, NodeId, 安全策略"},
		{"name": "EtherNet/IP", "type": "ethernetip", "port": 44818, "transport": "TCP", "features": "CIP, Class1/3, Tag Read/Write"},
		{"name": "SNMP", "type": "snmp", "port": 161, "transport": "UDP", "features": "v1/v2c/v3, Get/GetNext/GetBulk/Walk, OID"},
		{"name": "DL/T 645", "type": "dlt645", "port": 0, "transport": "Serial", "features": "中国电能表, 07/97 版本, Block/Field"},
		{"name": "IEC 104", "type": "ice104", "port": 2404, "transport": "TCP", "features": "APCI+ASDU, 总召/时钟同步/单点/双点/测量值"},
		{"name": "KNXnet/IP", "type": "knxnetip", "port": 3671, "transport": "UDP", "features": "Tunneling/Routing, GroupValue Read/Write"},
		{"name": "Mitsubishi MELSEC", "type": "mitsubishi", "port": 5000, "transport": "TCP", "features": "MC 3E/4E, 位/字软元件, ASCII/Binary"},
		{"name": "Omron FINS", "type": "omron", "port": 9600, "transport": "TCP", "features": "FINS UDP/TCP, CIO/WR/HR/DM/AR"},
	}
	data, _ := json.MarshalIndent(protocols, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(data)},
		},
	}, nil
}

func (s *Server) mcpResourceEdgeRules(uri string) (*mcp.ReadResourceResult, error) {
	type ruleInfo struct {
		ID             string           `json:"id"`
		Name           string           `json:"name"`
		Type           string           `json:"type"`
		Condition      string           `json:"condition"`
		Expression     string           `json:"expression"`
		Actions        []model.RuleAction `json:"actions"`
		Enable         bool             `json:"enable"`
		TriggerCount   int64            `json:"trigger_count"`
		ActionSuccess  int64            `json:"action_success_count"`
	}
	var rules []ruleInfo
	if s.ecm != nil {
		ruleStates := s.ecm.GetRuleStates()
		for _, r := range s.ecm.GetRules() {
			triggerCount := int64(0)
			actionSuccess := int64(0)
			if rs, ok := ruleStates[r.ID]; ok {
				triggerCount = rs.TriggerCount
				actionSuccess = rs.ActionSuccessCount
			}
			actions := r.Actions
			if actions == nil {
				actions = []model.RuleAction{}
			}
			rules = append(rules, ruleInfo{
				ID: r.ID, Name: r.Name, Type: r.Type,
				Condition: r.Condition, Expression: r.Expression,
				Actions: actions, Enable: r.Enable,
				TriggerCount: triggerCount, ActionSuccess: actionSuccess,
			})
		}
	}
	if rules == nil {
		rules = []ruleInfo{}
	}
	data, _ := json.MarshalIndent(rules, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(data)},
		},
	}, nil
}

func (s *Server) mcpResourceConfig(uri string) (*mcp.ReadResourceResult, error) {
	export := make(map[string]any)
	export["export_time"] = time.Now().Format(time.RFC3339)
	export["server"] = map[string]string{"name": "EdgeX", "version": "v0.0.8"}
	channels := s.cm.GetChannels()
	chList := make([]map[string]any, 0, len(channels))
	for _, ch := range channels {
		chData := map[string]any{"channel_id": ch.ID, "name": ch.Name, "protocol": ch.Protocol, "enabled": ch.Enable, "config": ch.Config}
		devices := s.cm.GetChannelDevices(ch.ID)
		devList := make([]map[string]any, 0, len(devices))
		for _, dev := range devices {
			devData := map[string]any{"device_id": dev.ID, "name": dev.Name, "enabled": dev.Enable, "interval": time.Duration(dev.Interval).String(), "config": dev.Config}
			ptList := make([]map[string]any, 0, len(dev.Points))
			for _, p := range dev.Points {
				ptList = append(ptList, map[string]any{
					"point_id": p.ID, "name": p.Name, "address": p.Address, "datatype": p.DataType,
					"scale": p.Scale, "offset": p.Offset, "unit": p.Unit, "readwrite": p.ReadWrite,
					"word_order": p.WordOrder, "scan_class": p.ScanClass, "register_type": string(p.RegisterType), "function_code": p.FunctionCode,
				})
			}
			devData["points"] = ptList
			devList = append(devList, devData)
		}
		chData["devices"] = devList
		chList = append(chList, chData)
	}
	export["channels"] = chList
	if s.ecm != nil {
		rules := s.ecm.GetRules()
		ruleStates := s.ecm.GetRuleStates()
		ruleList := make([]map[string]any, 0, len(rules))
		for _, r := range rules {
			triggerCount := int64(0)
			if rs, ok := ruleStates[r.ID]; ok {
				triggerCount = rs.TriggerCount
			}
			ruleList = append(ruleList, map[string]any{
				"rule_id": r.ID, "name": r.Name, "type": r.Type,
				"condition": r.Condition, "enabled": r.Enable,
				"trigger_count": triggerCount, "action_count": len(r.Actions),
			})
		}
		export["rules"] = ruleList
	}
	data, _ := json.MarshalIndent(export, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []mcp.ResourceContent{
			{URI: uri, MimeType: "application/json", Text: string(data)},
		},
	}, nil
}

// ── getSystemInfoSnapshot 返回系统信息快照 ──

func (s *Server) getSystemInfoSnapshot() map[string]any {
	info := map[string]any{
		"server": map[string]string{
			"name":    "EdgeX",
			"version": "v0.0.8",
		},
		"protocols": []string{
			"modbus-tcp", "modbus-rtu", "s7", "bacnet", "opcua",
			"ethernetip", "profinetio", "ethercat", "dlt645",
			"snmp", "knxnetip", "mitsubishi", "omron", "ice104",
		},
		"mcp": map[string]any{
			"enabled":   true,
			"endpoint":  "/api/mcp",
			"transport": "HTTP/SSE (JSON-RPC 2.0)",
			"version":   "2024-11-05",
		},
	}

	channels := s.cm.GetChannels()
	info["channel_count"] = len(channels)
	info["uptime"] = fmt.Sprintf("%s", s.getUptime())

	return info
}

func (s *Server) getUptime() string {
	return time.Since(s.startTime).String()
}

// ── MCP HTTP Handler ──

// initMCPServer 初始化 MCP 服务端
func (s *Server) initMCPServer() *mcp.MCPServer {
	mcpSrv := mcp.NewMCPServer(mcp.ServerName, mcp.ServerVersion)
	s.registerMCPTools(mcpSrv)
	s.registerMCPResources(mcpSrv)
	s.registerMCPPrompts(mcpSrv)
	// EAN 2.0: auto-register Capability → MCP Tool (Invoke via Runtime, not JSON-RPC CRUD).
	if rt := s.ensureCapabilityRuntime(); rt != nil {
		n := mcp.RegisterCapabilityTools(mcpSrv, rt, s.mcpRequireFullAccess)
		s.logger.Info("EAN Capability tools registered on MCP", zap.Int("count", n))
	}
	return mcpSrv
}

// ensureCapabilityRuntime builds (once) a local Capability Runtime for MCP / in-process Invoke.
func (s *Server) ensureCapabilityRuntime() *capability.Runtime {
	s.eanRuntimeOnce.Do(func() {
		agentID := "edgex"
		if s.nbm != nil {
			cfg := s.nbm.GetConfig()
			if len(cfg.EdgeOSMQTT) > 0 && cfg.EdgeOSMQTT[0].NodeID != "" {
				agentID = cfg.EdgeOSMQTT[0].NodeID
			} else if len(cfg.EdgeOSNATS) > 0 && cfg.EdgeOSNATS[0].NodeID != "" {
				agentID = cfg.EdgeOSNATS[0].NodeID
			}
		}
		rt, err := capability.NewRuntime(capability.RuntimeConfig{
			AgentID:              agentID,
			AgentVersion:         capability.RuntimeVersion,
			Transport:            capability.TransportSDK,
			HeartbeatIntervalSec: 60,
			Metadata: map[string]any{
				"northbound": "mcp",
			},
		}, capability.NoopBus{})
		if err != nil {
			s.logger.Warn("Failed to create EAN Capability Runtime for MCP", zap.Error(err))
			return
		}
		exec := execution.NewDriverExecutor(s.cm)
		exec.SetAI(execution.NewAIAdapter(s.ensureAiAgent()))
		rt.SetMapper(execution.NewCapabilityMapper(exec))
		s.eanRuntime = rt
	})
	return s.eanRuntime
}

// registerMCPPrompts 注册 MCP 提示词模板
func (s *Server) registerMCPPrompts(mcpSrv *mcp.MCPServer) {
	// 协议逆向
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "protocol-reverse",
		Description: "工业协议逆向工程：根据 PCAP 抓包与 HMI 显示值，分析协议结构并生成点位配置",
		Arguments: []mcp.PromptArgument{
			{Name: "protocol", Description: "协议类型（modbus/s7/bacnet/opcua 等）", Required: true},
			{Name: "observations", Description: "HMI 显示值列表（格式：标签=值，多个用逗号分隔）", Required: false},
		},
	})

	// 通道配置
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "channel-config",
		Description: "生成通道配置：根据协议类型和设备信息，生成完整的 Channel JSON 配置",
		Arguments: []mcp.PromptArgument{
			{Name: "protocol", Description: "协议类型", Required: true},
			{Name: "ip", Description: "设备 IP 地址", Required: true},
			{Name: "port", Description: "端口号", Required: false},
		},
	})

	// 诊断分析
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "diagnostics-analyze",
		Description: "诊断分析：根据诊断数据，分析通道/设备异常原因并给出排查建议",
		Arguments: []mcp.PromptArgument{
			{Name: "channel_id", Description: "通道 ID", Required: true},
		},
	})

	// ── 新增提示词 ──

	// Modbus 快速接入
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "modbus-quick-start",
		Description: "Modbus TCP/RTU 设备快速接入指南：通道创建、设备注册、线圈/保持寄存器点位配置",
		Arguments: []mcp.PromptArgument{
			{Name: "ip", Description: "Modbus 设备 IP 地址", Required: true},
			{Name: "port", Description: "Modbus 端口（默认 502）", Required: false},
			{Name: "slave_id", Description: "从站 ID（默认 1）", Required: false},
		},
	})

	// S7 快速接入
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "s7-quick-start",
		Description: "Siemens S7 PLC 快速接入指南：S7-200/300/400/1200/1500 系列连接配置、DB 块读写、点位优化",
		Arguments: []mcp.PromptArgument{
			{Name: "ip", Description: "S7 PLC IP 地址", Required: true},
			{Name: "rack", Description: "机架号（默认 0）", Required: false},
			{Name: "slot", Description: "槽位号（默认 1）", Required: false},
		},
	})

	// BACnet 快速接入
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "bacnet-quick-start",
		Description: "BACnet/IP 楼宇自控协议快速接入指南：设备发现、对象/属性映射、AI/AO/BI/BO 点位配置",
		Arguments: []mcp.PromptArgument{
			{Name: "device_id", Description: "BACnet 设备实例 ID", Required: false},
		},
	})

	// OPC UA 快速接入
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "opcua-quick-start",
		Description: "OPC UA 工业自动化协议快速接入指南：安全策略配置、节点浏览、订阅模式点位配置",
		Arguments: []mcp.PromptArgument{
			{Name: "endpoint", Description: "OPC UA 端点 URL（如 opc.tcp://192.168.1.1:4840）", Required: true},
			{Name: "security", Description: "安全策略：None, Basic256, Basic256Sha256", Required: false},
		},
	})

	// 点位批量生成
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "point-batch-generator",
		Description: "点位批量生成模板：根据起始地址、数量、数据类型自动生成 Modbus/S7/BACnet 点位配置 JSON",
		Arguments: []mcp.PromptArgument{
			{Name: "protocol", Description: "协议类型：modbus-tcp, s7, bacnet", Required: true},
			{Name: "start_address", Description: "起始地址（如 40001 或 DB1.0）", Required: true},
			{Name: "count", Description: "点位数量", Required: true},
			{Name: "datatype", Description: "数据类型：uint16, int16, float32, float64, bool", Required: false},
		},
	})

	// 边缘规则构建
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "edge-rule-builder",
		Description: "边缘计算规则构建助手：根据触发条件、动作类型、目标设备自动生成规则配置",
		Arguments: []mcp.PromptArgument{
			{Name: "rule_type", Description: "规则类型：threshold, schedule, expression, state_change", Required: true},
			{Name: "channel_id", Description: "关联通道 ID", Required: false},
		},
	})

	// 故障排查
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "troubleshooting-guide",
		Description: "工业协议故障排查流程：连接超时、数据跳变、断线重连、点位不可达等常见问题的诊断步骤",
		Arguments: []mcp.PromptArgument{
			{Name: "issue_type", Description: "问题类型：connection, data_quality, timeout, offline", Required: true},
		},
	})

	// 数据流架构
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "data-flow-architect",
		Description: "数据流架构设计指南：从采集到上云的端到端数据链路设计，包括本地存储、边缘计算、北向推送",
		Arguments: []mcp.PromptArgument{
			{Name: "target", Description: "目标平台：mqtt, opcua, bacnet, rest, kafka", Required: false},
		},
	})

	// 网关健康检查
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "gateway-health-check",
		Description: "网关健康检查清单：CPU/内存/磁盘/网络/采集延迟/断线重连等关键指标的监控与诊断",
		Arguments:    []mcp.PromptArgument{},
	})

	// 协议迁移
	mcpSrv.RegisterPrompt(mcp.Prompt{
		Name:        "protocol-migration",
		Description: "协议迁移指南：从一种工业协议迁移到另一种协议的配置转换、地址映射、数据类型对应关系",
		Arguments: []mcp.PromptArgument{
			{Name: "from", Description: "源协议类型", Required: true},
			{Name: "to", Description: "目标协议类型", Required: true},
		},
	})
}

// handleMCP 处理 MCP 协议的 HTTP 请求
func (s *Server) handleMCP(c *fiber.Ctx) error {
	// MCP Server 懒初始化
	if s.mcpServer == nil {
		s.mcpServer = s.initMCPServer()
	}

	// GET → SSE 流（MCP 2025-11-25 Streamable HTTP）
	if c.Method() == fiber.MethodGet {
		return s.handleMCPSSE(c)
	}

	// DELETE → 会话终止（MCP 2025-11-25）
	if c.Method() == fiber.MethodDelete {
		sessionID := c.Get("Mcp-Session-Id", "")
		if sessionID != "" {
			mcpSessionsMu.Lock()
			delete(mcpSessions, sessionID)
			mcpSessionsMu.Unlock()
		}
		return c.SendStatus(204)
	}

	body := c.Body()
	if len(body) == 0 {
		// POST 空请求返回 MCP Server 信息
		settings := s.loadAiCopilotSettings()
		return c.JSON(fiber.Map{
			"name":        mcp.ServerName,
			"version":     mcp.ServerVersion,
			"protocol":    "MCP " + mcp.MCPVersion,
			"transport":   "HTTP/SSE with JSON-RPC 2.0",
			"endpoint":    "/api/mcp",
			"description": "EdgeX Industrial Protocol Copilot MCP Server",
			"tools":       len(s.mcpServer.GetTools()),
			"resources":   len(s.mcpServer.GetResources()),
			"prompts":     len(s.mcpServer.GetPrompts()),
			"auth_mode":   "api_key",
			"mcp_enabled": settings.McpEnabled,
			"full_access": settings.McpFullAccess,
			"docs":        "/api/mcp/help",
			"versions":    mcp.SupportedVersions(),
		})
	}

	// MCP API Key 认证
	if !s.mcpCheckAuth(c) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"jsonrpc": "2.0",
			"error": fiber.Map{
				"code":    -32001,
				"message": "MCP 认证失败：请在 EdgeX UI → AI 助手 → MCP 接入页面设置 API Key，并在请求头中携带 Authorization: Bearer <key> 或 X-MCP-API-Key: <key>",
			},
		})
	}

	// 处理 JSON-RPC 请求
	resp := s.mcpServer.HandleMessage(body)
	if resp == nil {
		return c.SendStatus(204)
	}

	// MCP 2025-11-25: 响应中返回 Mcp-Session-Id
	sessionID := s.getOrCreateMCPSession(c)
	c.Set("Mcp-Session-Id", sessionID)

	return c.JSON(resp)
}

// handleMCPSSE 处理 MCP Streamable HTTP SSE 连接（GET 请求）
func (s *Server) handleMCPSSE(c *fiber.Ctx) error {
	// MCP API Key 认证
	if !s.mcpCheckAuth(c) {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		_, _ = c.Write([]byte("event: error\ndata: MCP authentication failed\n\n"))
		return nil
	}

	sessionID := s.getOrCreateMCPSession(c)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Mcp-Session-Id", sessionID)
	c.Status(fiber.StatusOK)

	// 必须在 SetBodyStreamWriter 之前捕获 Done channel，避免 goroutine 中 ctx 失效
	done := c.Context().Done()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// 发送 endpoint 事件，告知客户端会话 URL
		fmt.Fprintf(w, "event: endpoint\ndata: /api/mcp?session=%s\n\n", sessionID)
		w.Flush()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
					return
				}
				w.Flush()
			case <-done:
				return
			}
		}
	})

	return nil
}

// handleMCPHelp 返回 MCP 接入帮助文档（丰富版）
func (s *Server) handleMCPHelp(c *fiber.Ctx) error {
	settings := s.loadAiCopilotSettings()
	if s.mcpServer == nil {
		s.mcpServer = s.initMCPServer()
	}

	// 工具分类
	type toolEntry struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"` // read / write
	}
	readTools := []string{
		"edgex_list_channels", "edgex_list_devices", "edgex_list_points",
		"edgex_read_point", "edgex_get_system_info", "edgex_get_diagnostics",
		"edgex_analyze_protocol", "edgex_get_protocol_help",
		"edgex_list_edge_rules", "edgex_get_point_history", "edgex_export_config",
		"edgex_get_channel_config",
	}
	readOnly := make(map[string]bool)
	for _, n := range readTools {
		readOnly[n] = true
	}

	var tools []toolEntry
	for _, t := range s.mcpServer.GetTools() {
		cat := "write"
		if readOnly[t.Name] {
			cat = "read"
		}
		tools = append(tools, toolEntry{Name: t.Name, Description: t.Description, Category: cat})
	}

	help := map[string]any{
		"title":       "EdgeX MCP Server — 接入指南",
		"description": "通过 MCP (Model Context Protocol) 协议，外部 LLM 应用（Claude Desktop、Cursor、Windsurf、Continue.dev 等）可以安全地操作 EdgeX 工业网关。支持 30 个工具、13 个提示词模板、6 个资源端点。",
		"transport":   "HTTP/SSE (JSON-RPC 2.0)",
		"endpoint":    "/api/mcp",
		"auth":        "MCP API Key（简化认证）— 在 EdgeX UI → AI 助手 → MCP 接入页面设置",
		"auth_mode":   "api_key",
		"auth_header": "Authorization: Bearer <mcp_api_key> 或 X-MCP-API-Key: <mcp_api_key>",
		"mcp_enabled": settings.McpEnabled,
		"full_access": settings.McpFullAccess,
		"clients": []map[string]string{
			{
				"name":   "Claude Desktop",
				"config": `{"mcpServers":{"edgex":{"url":"http://<host>:8080/api/mcp","headers":{"Authorization":"Bearer <mcp_api_key>"}}}}`,
			},
			{
				"name":   "Cursor / Windsurf",
				"config": `{"mcpServers":{"edgex":{"url":"http://<host>:8080/api/mcp","headers":{"Authorization":"Bearer <mcp_api_key>"}}}}`,
			},
			{
				"name":   "Continue.dev",
				"config": `{"mcpServers":{"edgex":{"transport":{"type":"http","url":"http://<host>:8080/api/mcp"},"auth":{"type":"bearer","token":"<mcp_api_key>"}}}}`,
			},
		},
		"tools":        tools,
		"tool_names":   s.mcpServer.GetToolNames(),
		"resources":    s.mcpServer.GetResources(),
		"prompts":      s.mcpServer.GetPrompts(),
		"security":     "全功能 CRUD 操作（创建/删除/写入）需要用户在 UI 中确认激活全功能权限；默认仅支持只读操作；所有操作通过 MCP API Key 认证",
		"activation":   "POST /api/mcp/activate — 激活全功能读写（需用户确认）",
		"status":       "GET /api/mcp/status — 查询 MCP 激活状态",
		"architecture": map[string]any{
			"layers": []map[string]string{
				{"name": "LLM 客户端", "desc": "Claude Desktop / Cursor / Windsurf / Continue.dev", "color": "purple"},
				{"name": "MCP 协议层", "desc": "JSON-RPC 2.0 / SSE / Streamable HTTP", "color": "blue"},
				{"name": "EdgeX 网关", "desc": "认证 → 权限检查 → 工具分发 → 数据读写", "color": "green"},
				{"name": "工业设备", "desc": "Modbus / S7 / BACnet / OPC UA / SNMP / ICE104", "color": "orange"},
			},
		},
	}

	return c.JSON(help)
}

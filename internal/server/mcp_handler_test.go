package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/mcp"
	"github.com/anviod/edgeCore/internal/model"

	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	_ "github.com/anviod/edgeCore/internal/driver/dlt645"
	_ "github.com/anviod/edgeCore/internal/driver/ethercat"
	_ "github.com/anviod/edgeCore/internal/driver/modbus"
	_ "github.com/anviod/edgeCore/internal/driver/profinetio"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMcpFullAccessServer(t *testing.T) *Server {
	t.Helper()
	pipeline := core.NewDataPipeline(100)
	cm := core.NewChannelManager(pipeline, nil)
	srv := NewServer(cm, nil, pipeline, nil, nil, nil, nil, nil, nil)
	srv.aiSettingsMem = &model.AICopilotSettings{McpEnabled: true, McpFullAccess: true}
	return srv
}

func callTool(t *testing.T, fn func(json.RawMessage) (*mcp.CallToolResult, error), args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	return fn(raw)
}

func TestNormalizeProtocolName(t *testing.T) {
	cases := map[string]string{
		"modbus":              "modbus-tcp",
		"modbus-tcp":          "modbus-tcp",
		"modbus-rtu":          "modbus-rtu",
		"bacnet":              "bacnet-ip",
		"bacnet-ip":           "bacnet-ip",
		"opcua":               "opc-ua",
		"opc-ua":              "opc-ua",
		"ethernetip":          "ethernet-ip",
		"eip":                 "ethernet-ip",
		"ice104":              "iec60870-5-104",
		"knxnetip":            "knxnet-ip",
		"knx":                 "knxnet-ip",
		"mitsubishi":          "mitsubishi-slmp",
		"omron":               "omron-fins",
		"profinet":            "profinet-io",
		"profinetio":          "profinet-io",
		"s7":                  "s7",
		"snmp":                "snmp",
		"dlt645":              "dlt645",
		"ethercat":            "ethercat",
		" BACPAC?":            "bacpac?",
		"Unknown":             "unknown",
	}
	for in, want := range cases {
		assert.Equalf(t, want, normalizeProtocolName(in), "normalizeProtocolName(%q)", in)
	}
}

func TestDefaultPortCoverage(t *testing.T) {
	// profinet-io 默认端口 34964
	assert.Equal(t, 34964, defaultPort("profinet-io"))
	// ethercat 无 TCP 端口概念，返回 0
	assert.Equal(t, 0, defaultPort("ethercat"))
	// 现有 TCP 协议默认端口回归
	assert.Equal(t, 502, defaultPort("modbus-tcp"))
	assert.Equal(t, 102, defaultPort("s7"))
	assert.Equal(t, 47808, defaultPort("bacnet-ip"))
	assert.Equal(t, 4840, defaultPort("opc-ua"))
	assert.Equal(t, 2404, defaultPort("iec60870-5-104"))
}

func TestTCPProtocolsIncludesProfinetIO(t *testing.T) {
	assert.True(t, tcpProtocols["profinet-io"])
	// 全部 15 个驱动注册名：12 个 TCP 协议应全部在白名单
	tcp := []string{
		"modbus-tcp", "modbus-rtu-over-tcp", "s7", "bacnet-ip",
		"opc-ua", "ethernet-ip", "snmp", "iec60870-5-104",
		"knxnet-ip", "mitsubishi-slmp", "omron-fins", "profinet-io",
	}
	for _, p := range tcp {
		assert.Truef(t, tcpProtocols[p], "tcpProtocols should contain %s", p)
	}
}

func TestMcpCreateChannel_NormalizesAlias(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// 别名 bacnet → 驱动注册名 bacnet-ip
	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "BACnet Ch",
		"protocol": "bacnet",
		"config":   map[string]any{"ip": "192.168.1.10"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "create_channel(bacnet) should succeed: %s", res.Content)
	require.Contains(t, res.Content[0].Text, "bacnet-ip")
	require.False(t, strings.Contains(res.Content[0].Text, "bacnet\""), "protocol should be normalized")

	// 别名 opcua → opc-ua
	res, err = callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "OPC Ch",
		"protocol": "opcua",
		"config":   map[string]any{"ip": "192.168.1.20"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "create_channel(opcua) should succeed: %s", res.Content)

	// 未知协议应返回 driver not found 错误
	res, err = callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "Bad Ch",
		"protocol": "not-a-real-protocol",
		"config":   map[string]any{"ip": "1.2.3.4"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "driver for protocol")
}

func TestMcpCreateChannel_ProfinetIO(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "PNIO Ch",
		"protocol": "profinet-io",
		"config":   map[string]any{"ip": "192.168.1.30"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "create_channel(profinet-io) should succeed: %s", res.Content)
	// 默认端口应自动填充为 34964
	require.Contains(t, res.Content[0].Text, `"port": 34964`)
}

func TestMcpCreateChannel_TCPRequiresIP(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// TCP 协议缺少 ip 应报错
	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "No IP",
		"protocol": "modbus-tcp",
		"config":   map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "config.ip")
}

func TestMcpCreateChannel_SerialFieldNormalization(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// 串口协议缺少 port 应报错
	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "RTU No Port",
		"protocol": "modbus-rtu",
		"config":   map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "config.port")

	// 传入 serial_port/baud_rate 别名应归一化为 port/baudRate 且创建成功
	res, err = callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "RTU Alias",
		"protocol": "modbus-rtu",
		"config":   map[string]any{"serial_port": "COM3", "baud_rate": 19200},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "create_channel(modbus-rtu) with serial_port should succeed: %s", res.Content)
	require.Contains(t, res.Content[0].Text, `"port": "COM3"`)
	require.Contains(t, res.Content[0].Text, `"baudRate": 19200`)

	// dlt645 也应在串口校验集合内
	res, err = callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "DLT645 No Port",
		"protocol": "dlt645",
		"config":   map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "config.port")
}

func TestMcpCreateChannel_EtherCATRequiresInterface(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// EtherCAT 缺 local_interface 应报错
	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "ECAT No Iface",
		"protocol": "ethercat",
		"config":   map[string]any{},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "local_interface")

	// simulation 模拟模式放行
	res, err = callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "ECAT Sim",
		"protocol": "ethercat",
		"config":   map[string]any{"simulation": true},
	})
	require.NoError(t, err)
	require.False(t, res.IsError, "create_channel(ethercat) simulation mode should succeed: %s", res.Content)
}

func TestMcpBatchCreateDevices(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// 先创建 modbus-tcp 通道
	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "Batch Ch",
		"protocol": "modbus-tcp",
		"config":   map[string]any{"ip": "192.168.1.40"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)

	var created struct {
		ChannelID string `json:"channel_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractJSON(t, res.Content[0].Text)), &created))

	// 批量创建设备
	res, err = callTool(t, srv.mcpBatchCreateDevices, map[string]any{
		"channel_id": created.ChannelID,
		"devices": []map[string]any{
			{"name": "DevA", "config": map[string]any{"slave_id": "1"}},
			{"name": "DevB", "config": map[string]any{"slave_id": "2"}},
			{"name": "", "config": map[string]any{}}, // 应失败
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError, "batch_create_devices partial failure should be flagged as error: %s", res.Content)
	require.Contains(t, res.Content[0].Text, `"created_count": 2`)
	require.Contains(t, res.Content[0].Text, `"failed_count": 1`)

	devices := srv.cm.GetChannelDevices(created.ChannelID)
	require.Len(t, devices, 2)
}

func TestMcpBatchCreatePoints(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// 创建通道
	res, err := callTool(t, srv.mcpCreateChannel, map[string]any{
		"name":     "Batch Pt Ch",
		"protocol": "modbus-tcp",
		"config":   map[string]any{"ip": "192.168.1.50"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	var ch struct {
		ChannelID string `json:"channel_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractJSON(t, res.Content[0].Text)), &ch))

	// 创建单个设备
	res, err = callTool(t, srv.mcpCreateDevice, map[string]any{
		"channel_id": ch.ChannelID,
		"name":       "PtDev",
		"config":     map[string]any{"slave_id": "1"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	var dev struct {
		DeviceID string `json:"device_id"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractJSON(t, res.Content[0].Text)), &dev))

	// 批量创建点位
	res, err = callTool(t, srv.mcpBatchCreatePoints, map[string]any{
		"channel_id": ch.ChannelID,
		"device_id":  dev.DeviceID,
		"points": []map[string]any{
			{"name": "Temp", "address": "40001", "datatype": "float32"},
			{"name": "Pressure", "address": "40003", "datatype": "float32"},
			{"name": "", "address": "40005", "datatype": "float32"}, // 应失败
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError, "batch_create_points partial failure should be flagged as error: %s", res.Content)
	require.Contains(t, res.Content[0].Text, `"created_count": 2`)
	require.Contains(t, res.Content[0].Text, `"failed_count": 1`)

	pts, err := srv.cm.GetDevicePoints(ch.ChannelID, dev.DeviceID)
	require.NoError(t, err)
	require.Len(t, pts, 2)
}

func TestProtocolShortKey(t *testing.T) {
	cases := map[string]string{
		"modbus-tcp":       "modbus",
		"modbus-rtu":       "modbus",
		"bacnet-ip":        "bacnet",
		"opc-ua":           "opcua",
		"ethernet-ip":      "eip",
		"iec60870-5-104":   "ice104",
		"knxnet-ip":        "knx",
		"mitsubishi-slmp":  "mitsubishi",
		"omron-fins":       "omron",
		"profinet-io":      "profinet",
		"MODBUS-TCP":       "modbus",
		"unknown":          "unknown",
	}
	for in, want := range cases {
		assert.Equalf(t, want, protocolShortKey(in), "protocolShortKey(%q)", in)
	}
}

func TestMcpGetProtocolHelp_CanonicalNames(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	// 规范驱动名应命中帮助文档
	res, err := callTool(t, srv.mcpGetProtocolHelp, map[string]any{"protocol": "modbus-tcp"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "Modbus TCP/RTU 接入帮助")

	res, err = callTool(t, srv.mcpGetProtocolHelp, map[string]any{"protocol": "bacnet-ip"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "BACnet/IP 接入帮助")

	res, err = callTool(t, srv.mcpGetProtocolHelp, map[string]any{"protocol": "opc-ua"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "OPC UA 接入帮助")
}

func TestMcpAnalyzeProtocol_CanonicalNames(t *testing.T) {
	srv := newMcpFullAccessServer(t)

	res, err := callTool(t, srv.mcpAnalyzeProtocol, map[string]any{"protocol_hint": "profinet-io"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "PROFINET IO")

	res, err = callTool(t, srv.mcpAnalyzeProtocol, map[string]any{"protocol_hint": "opc-ua"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, res.Content[0].Text, "OPC UA")
}

// extractJSON 提取 markdown 代码块中的 JSON
func extractJSON(t *testing.T, text string) string {
	t.Helper()
	start := strings.Index(text, "{")
	require.GreaterOrEqual(t, start, 0, "no JSON object found in: %s", text)
	end := strings.LastIndex(text, "}")
	require.Greater(t, end, start, "no closing brace in: %s", text)
	return text[start : end+1]
}

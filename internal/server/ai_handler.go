package server

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

type aiChatRequest struct {
	Message string         `json:"message"`
	Context map[string]any `json:"context"`
}

type aiAction struct {
	Type    string `json:"type"`
	Label   string `json:"label"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
}

type aiChatResponse struct {
	Reply           string         `json:"reply"`
	Suggestions     []string       `json:"suggestions"`
	Mode            string         `json:"mode"`
	Actions         []aiAction     `json:"actions,omitempty"`
	ContextSnapshot map[string]any `json:"context_snapshot,omitempty"`
}

type aiStatusResponse struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Capabilities []string `json:"capabilities"`
	Message      string   `json:"message"`
	Scenarios    []string `json:"scenarios,omitempty"`
}

func (s *Server) getAiStatus(c *fiber.Ctx) error {
	settings := s.loadAiCopilotSettings()
	mode := settings.RuntimeMode()
	providerLabel := settings.ProviderLabel()
	modelName := settings.Model
	if modelName == "" {
		modelName = "copilot-service"
	}

	message := "AI助手已连接 AI Model Center（" + settings.GrpcEndpoint + "）。工作台流水线将使用远端 Rule Engine 与 LLM 推理。"
	if settings.DeploymentMode == "cloud" {
		message = "AI助手已配置云端 API（" + providerLabel + "）。协议逆向与文档解析将路由至 " + settings.BaseURL + "。"
	}

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": aiStatusResponse{
			Enabled:  true,
			Mode:     mode,
			Provider: providerLabel,
			Model:    modelName,
			Capabilities: []string{
				"system-overview",
				"channel-status",
				"channel-diagnosis",
				"edge-rules",
				"navigation-help",
				"troubleshooting",
				"protocol-guide",
				"point-config",
				"northbound-config",
				"virtual-shadow",
				"protocol-reverse",
				"doc-parse",
				"schema-validation",
				"validation-cases",
				"edge-rule-draft",
				"diagnostics-assist",
				"token-quota",
				"human-confirm",
			},
			Scenarios: []string{"qa", "troubleshoot", "config", "protocol", "workbench"},
			Message:   message,
		},
	})
}

func (s *Server) postAiChat(c *fiber.Ctx) error {
	var req aiChatRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "1",
			"message": "请求格式无效",
			"data":    nil,
		})
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code":    "1",
			"message": "消息内容不能为空",
			"data":    nil,
		})
	}

	snapshot := s.buildAiContextSnapshot()
	reply, suggestions, actions := s.generateAiReply(message, req.Context, snapshot)

	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data": aiChatResponse{
			Reply:           reply,
			Suggestions:     suggestions,
			Mode:            "local",
			Actions:         actions,
			ContextSnapshot: snapshot,
		},
	})
}

func (s *Server) buildAiContextSnapshot() map[string]any {
	snapshot := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime":    time.Since(s.startTime).Round(time.Second).String(),
	}

	cpuUsage := 0.0
	if cpuInfos, err := cpu.Percent(0, false); err == nil && len(cpuInfos) > 0 {
		cpuUsage = cpuInfos[0]
	}
	memoryUsage := 0.0
	if memStats, err := mem.VirtualMemory(); err == nil {
		memoryUsage = memStats.UsedPercent
	}

	snapshot["system"] = map[string]any{
		"cpu_usage":    fmt.Sprintf("%.1f%%", cpuUsage),
		"memory_usage": fmt.Sprintf("%.1f%%", memoryUsage),
		"goroutines":   runtime.NumGoroutine(),
	}

	channelStats := s.cm.GetChannelStats()
	totalDevices := 0
	onlineDevices := 0
	offlineChannels := 0
	channelBriefs := make([]map[string]any, 0, len(channelStats))

	for _, ch := range channelStats {
		totalDevices += ch.DeviceCount
		onlineDevices += ch.OnlineCount
		if ch.Status != "online" && ch.Enable {
			offlineChannels++
		}
		channelBriefs = append(channelBriefs, map[string]any{
			"name":         ch.Name,
			"protocol":     ch.Protocol,
			"status":       ch.Status,
			"device_count": ch.DeviceCount,
			"online_count": ch.OnlineCount,
		})
	}

	snapshot["channels"] = map[string]any{
		"total":           len(channelStats),
		"offline_enabled": offlineChannels,
		"device_total":    totalDevices,
		"device_online":   onlineDevices,
		"items":           channelBriefs,
	}

	if s.ecm != nil {
		metrics := s.ecm.GetMetrics()
		snapshot["edge_rules"] = map[string]any{
			"rule_count":      metrics.RuleCount,
			"rules_triggered": metrics.RulesTriggered,
			"rules_executed":  metrics.RulesExecuted,
			"cache_size":      metrics.CacheSize,
		}
	}

	if s.nbm != nil {
		nbStats := s.nbm.GetNorthboundStats()
		snapshot["northbound"] = map[string]any{
			"total": len(nbStats),
		}
	}

	return snapshot
}

func (s *Server) generateAiReply(message string, pageContext map[string]any, snapshot map[string]any) (string, []string, []aiAction) {
	lower := strings.ToLower(message)
	route, _ := pageContext["route"].(string)
	scenario, _ := pageContext["scenario"].(string)
	workspace, _ := pageContext["workspace"].(string)
	taskID, _ := pageContext["task_id"].(string)

	if workspace != "" && containsAny(lower, "帮助", "怎么", "当前", "工作台") {
		return s.replyWorkbenchContext(workspace, taskID, snapshot)
	}

	// Scenario-mode defaults when message is generic
	if containsAny(lower, "概况", "状态", "运行") && scenario == "troubleshoot" {
		return s.replyTroubleshooting(snapshot)
	}

	switch {
	case containsAny(lower, "pcap", "抓包", "逆向", "报文", "hex", "无文档", "协议逆向"):
		return s.replyProtocolReverse(snapshot)
	case containsAny(lower, "modbus", "功能码", "fc03", "fc04", "寄存器", "从站", "slave", "字节序"):
		return s.replyModbusGuide(snapshot)
	case containsAny(lower, "s7", "plc", "db块", "rack", "slot", "西门子"):
		return s.replyS7Guide(snapshot)
	case containsAny(lower, "bacnet", "object", "instance", "who-is"):
		return s.replyBACnetGuide(snapshot)
	case containsAny(lower, "opc ua", "opcua", "opc-ua"):
		return s.replyOPCUAGuide(snapshot)
	case containsAny(lower, "协议识别", "协议分析", "协议选择", "什么协议"):
		return s.replyProtocolIdentification(snapshot)
	case containsAny(lower, "点位", "点表", "导入", "scale", "缩放", "数据类型", "address"):
		return s.replyPointConfig(snapshot)
	case containsAny(lower, "虚拟影子", "公式", "shadow", "计算点"):
		return s.replyVirtualShadow(snapshot)
	case containsAny(lower, "诊断", "soak", "scanengine", "成功率", "采集率", "质量"):
		return s.replyChannelDiagnosis(snapshot)
	case containsAny(lower, "异常通道", "离线通道", "离线排查", "离线"):
		return s.replyOfflineChannels(snapshot)
	case containsAny(lower, "通道", "采集", "channel", "设备在线"):
		return s.replyChannelStatus(snapshot)
	case containsAny(lower, "边缘", "规则", "edge", "报警", "联动", "阈值"):
		return s.replyEdgeRules(snapshot)
	case containsAny(lower, "北向", "mqtt", "上报", "northbound", "sparkplug"):
		return s.replyNorthbound(snapshot)
	case containsAny(lower, "日志", "错误", "故障", "排查", "debug", "联调"):
		return s.replyTroubleshooting(snapshot)
	case containsAny(lower, "系统", "概况", "状态", "监控", "cpu", "内存"):
		return s.replySystemOverview(snapshot)
	case containsAny(lower, "帮助", "怎么", "如何", "教程", "配置", "页面"):
		return s.replyNavigationHelp(route)
	default:
		return s.replyDefault(snapshot, route, scenario)
	}
}

func (s *Server) replyChannelStatus(snapshot map[string]any) (string, []string, []aiAction) {
	ch, _ := snapshot["channels"].(map[string]any)
	total, _ := ch["total"].(int)
	deviceTotal, _ := ch["device_total"].(int)
	deviceOnline, _ := ch["device_online"].(int)
	offlineEnabled, _ := ch["offline_enabled"].(int)

	var lines []string
	lines = append(lines, fmt.Sprintf("当前共有 %d 个采集通道，%d 台设备（在线 %d 台）。", total, deviceTotal, deviceOnline))
	if offlineEnabled > 0 {
		lines = append(lines, fmt.Sprintf("注意：有 %d 个已启用通道处于非在线状态，建议检查通讯参数与网络连通性。", offlineEnabled))
	} else if total > 0 {
		lines = append(lines, "所有已启用通道通讯正常。")
	} else {
		lines = append(lines, "尚未配置采集通道，可在「采集通道」页面新建。")
	}

	if items, ok := ch["items"].([]map[string]any); ok && len(items) > 0 {
		lines = append(lines, "通道摘要：")
		limit := len(items)
		if limit > 4 {
			limit = 4
		}
		for i := 0; i < limit; i++ {
			item := items[i]
			lines = append(lines, fmt.Sprintf("• %s（%s）— %s，设备 %v/%v 在线",
				item["name"], item["protocol"], item["status"], item["online_count"], item["device_count"]))
		}
		if len(items) > limit {
			lines = append(lines, fmt.Sprintf("… 另有 %d 个通道未列出", len(items)-limit))
		}
	}

	actions := []aiAction{{Type: "navigate", Label: "打开采集通道", Path: "/channels"}}
	suggestions := []string{"查看异常通道", "通道诊断指标", "如何添加 Modbus 通道？"}
	if offlineEnabled > 0 {
		suggestions = append([]string{"通道离线排查"}, suggestions...)
	}
	return strings.Join(lines, "\n"), suggestions, actions
}

func (s *Server) replyOfflineChannels(snapshot map[string]any) (string, []string, []aiAction) {
	ch, _ := snapshot["channels"].(map[string]any)
	offlineEnabled, _ := ch["offline_enabled"].(int)

	var lines []string
	if offlineEnabled == 0 {
		lines = append(lines, "当前没有检测到已启用但离线的通道。")
	} else {
		lines = append(lines, fmt.Sprintf("检测到 %d 个已启用通道处于离线状态：", offlineEnabled))
		if items, ok := ch["items"].([]map[string]any); ok {
			for _, item := range items {
				if status, _ := item["status"].(string); status != "online" {
					lines = append(lines, fmt.Sprintf("• %s（%s）— %s", item["name"], item["protocol"], status))
				}
			}
		}
		lines = append(lines, "\n排查步骤：")
		lines = append(lines, "1. 确认 IP/端口/串口参数与设备一致")
		lines = append(lines, "2. ping / telnet 测试网络连通")
		lines = append(lines, "3. 检查从站地址、机架槽位等协议专属参数")
		lines = append(lines, "4. 查看系统日志中该通道的 ERROR 记录")
	}

	actions := []aiAction{
		{Type: "navigate", Label: "采集通道", Path: "/channels"},
		{Type: "navigate", Label: "系统日志", Path: "/logs"},
	}
	return strings.Join(lines, "\n"), []string{"通道诊断指标", "Modbus 通讯排查", "查看系统日志"}, actions
}

func (s *Server) replyChannelDiagnosis(snapshot map[string]any) (string, []string, []aiAction) {
	ch, _ := snapshot["channels"].(map[string]any)
	total, _ := ch["total"].(int)

	lines := []string{
		"通道诊断指引（结合 /api/diagnostics/* 接口）：",
		"1. 进入异常通道 → 设备详情，查看点位采集成功率与最近错误；",
		"2. 关注 ScanEngine soak 指标：轮询周期、超时率、重试次数；",
		"3. 对比在线/离线设备数，定位是单设备还是整通道问题；",
		"4. 检查 ExecutionLayer 背压与协议 Token 限流是否触发。",
		fmt.Sprintf("\n当前网关共 %d 个通道，可在设备详情页查看实时诊断数据。", total),
	}

	actions := []aiAction{{Type: "navigate", Label: "采集通道", Path: "/channels"}}
	return strings.Join(lines, "\n"), []string{"查看异常通道", "通道离线排查", "系统运行概况"}, actions
}

func (s *Server) replyEdgeRules(snapshot map[string]any) (string, []string, []aiAction) {
	edge, ok := snapshot["edge_rules"].(map[string]any)
	actions := []aiAction{{Type: "navigate", Label: "边缘计算", Path: "/edge-compute"}}
	if !ok {
		return "边缘计算模块未就绪。可在「边缘计算」页面创建阈值、定时或表达式规则，实现本地报警与联动。",
			[]string{"规则类型说明", "阈值报警示例", "边缘场景模版"},
			actions
	}
	ruleCount, _ := edge["rule_count"].(int)
	triggered, _ := edge["rules_triggered"].(int64)
	executed, _ := edge["rules_executed"].(int64)

	reply := fmt.Sprintf(
		"边缘计算当前配置 %d 条规则，累计触发 %d 次、执行 %d 次。\n\n"+
			"支持能力：\n"+
			"• 阈值报警 — 点位超限时触发动作链\n"+
			"• 定时任务 — Cron 表达式调度\n"+
			"• 表达式计算 — 多点位逻辑组合\n"+
			"• 动作链 — MQTT/HTTP/写点/通知等\n\n"+
			"完整 AI 场景模版生成（EdgeRule JSON 草案）需对接 AI Model Center。",
		ruleCount, triggered, executed,
	)
	return reply, []string{"阈值报警示例", "规则执行记录", "边缘场景模版"}, actions
}

func (s *Server) replyNorthbound(snapshot map[string]any) (string, []string, []aiAction) {
	nb, ok := snapshot["northbound"].(map[string]any)
	actions := []aiAction{{Type: "navigate", Label: "北向接口", Path: "/northbound"}}
	if !ok {
		return "北向接口模块未就绪。可在「北向接口」配置 MQTT、HTTP、OPC UA 或 Sparkplug B 上报。",
			[]string{"MQTT 配置步骤", "OPC UA 证书", "上报策略说明"},
			actions
	}
	total, _ := nb["total"].(int)
	reply := fmt.Sprintf(
		"当前已配置 %d 个北向上报通道。\n\n"+
			"常见场景：\n"+
			"• MQTT — 实时数据推送与告警上报\n"+
			"• HTTP — Webhook 回调\n"+
			"• OPC UA — 对外提供点位服务\n"+
			"• Sparkplug B — 工业物联网标准上报",
		total,
	)
	return reply, []string{"MQTT 配置步骤", "上报策略说明", "北向连接状态"}, actions
}

func (s *Server) replySystemOverview(snapshot map[string]any) (string, []string, []aiAction) {
	sys, _ := snapshot["system"].(map[string]any)
	uptime, _ := snapshot["uptime"].(string)
	cpu, _ := sys["cpu_usage"].(string)
	memUsage, _ := sys["memory_usage"].(string)
	goroutines, _ := sys["goroutines"].(int)

	ch, _ := snapshot["channels"].(map[string]any)
	channelTotal, _ := ch["total"].(int)
	deviceOnline, _ := ch["device_online"].(int)

	reply := fmt.Sprintf(
		"网关运行概况：\n"+
			"• 运行时长：%s\n"+
			"• CPU：%s，内存：%s，协程：%d\n"+
			"• 采集通道：%d 个，设备在线 %d 台\n"+
			"可在首页监控查看实时图表与通道质量评分。",
		uptime, cpu, memUsage, goroutines, channelTotal, deviceOnline,
	)
	actions := []aiAction{{Type: "navigate", Label: "首页监控", Path: "/"}}
	return reply, []string{"通道运行状态", "边缘规则概况", "打开系统设置"}, actions
}

func (s *Server) replyTroubleshooting(snapshot map[string]any) (string, []string, []aiAction) {
	ch, _ := snapshot["channels"].(map[string]any)
	offlineEnabled, _ := ch["offline_enabled"].(int)

	lines := []string{
		"联调排障建议（G5 诊断辅助）：",
		"1. 「系统日志」筛选 ERROR 级别，定位最近异常；",
		"2. 进入异常通道 → 设备详情，查看点位采集成功率；",
		"3. 调用 /api/diagnostics/* 查看 ScanEngine 与 soak 指标；",
		"4. 确认串口/网口参数、从站地址、防火墙与路由；",
		"5. 对比 HMI 显示值与网关读数，排查 scale/字节序问题。",
	}
	if offlineEnabled > 0 {
		lines = append(lines, fmt.Sprintf("\n⚠ 当前 %d 个已启用通道离线，建议优先排查通讯链路。", offlineEnabled))
	}

	actions := []aiAction{
		{Type: "navigate", Label: "系统日志", Path: "/logs"},
		{Type: "navigate", Label: "采集通道", Path: "/channels"},
	}
	return strings.Join(lines, "\n"), []string{"查看异常通道", "通道诊断指标", "Modbus 通讯排查"}, actions
}

func (s *Server) replyProtocolReverse(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `工业协议逆向工程（Scenario B · 核心能力）流程：

阶段一 · 协议识别
  PCAP/HEX → Rule Engine 匹配端口与帧特征（Modbus 502、S7 102、BACnet 47808）

阶段二 · 报文结构解析
  网关本地 Decoder 提取 slave_id、FC、地址、raw[]（确定性，不经 LLM）

阶段三 · 物理量推理
  AI Model Center LLM 关联 HMI 显示值与候选字段（如 220.5/221.1/219.8 → 三相电压）

阶段四 · 生产配置生成
  输出 Channel JSON + Point JSON + Validation Case → 人工确认 → import

当前为本地助手模式，完整 PCAP 分析需对接 AI Model Center（gRPC CopilotService）。
可先上传抓包至「采集通道」诊断页或使用 Wireshark 预检协议类型。`

	actions := []aiAction{
		{Type: "navigate", Label: "采集通道", Path: "/channels"},
		{Type: "prompt", Label: "协议识别方法", Message: "如何识别设备协议？"},
	}
	return reply, []string{"Modbus 功能码说明", "协议识别方法", "厂家点表导入指引"}, actions
}

func (s *Server) replyProtocolIdentification(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `协议识别特征（Rule Engine 优先）：

• Modbus TCP — TCP 502 + MBAP 头
• Modbus RTU — 从站地址 + FC + CRC16
• S7 — TCP 102 + TPKT/COTP + Setup Communication
• BACnet/IP — UDP 47808 + BVLC type 0x81
• OPC UA — TCP 4840 + Hello/Acknowledge 握手

置信度 < 0.7 时建议人工选择协议驱动。
识别后进入对应 Decoder 做字段提取，再由 AI 推理物理量含义。`

	return reply, []string{"PCAP 逆向分析流程", "Modbus 功能码说明", "如何添加 Modbus 通道？"}, nil
}

func (s *Server) replyModbusGuide(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `Modbus 配置要点：

通道参数
  • IP + 端口（默认 502）或串口（RTU）
  • 从站号 slave_id（1～247）
  • 扫描类 scan_class：fast / normal / slow

点位参数
  • 功能码 FC03（保持寄存器）/ FC04（输入寄存器）
  • 地址 start_address（0-based 或 1-based 取决于设备）
  • 数据类型：UINT16 / INT32 / FLOAT32 等
  • 字节序：ABCD / CDAB / BADC / DCBA
  • scale / offset 换算物理量

常见排障
  • 超时 → 检查 IP/端口/防火墙
  • 读数为 0 或乱码 → 检查字节序与数据类型
  • Exception 02 → 非法地址，核对寄存器表`

	actions := []aiAction{
		{Type: "navigate", Label: "新建通道", Path: "/channels"},
		{Type: "prompt", Label: "离线排查", Message: "Modbus 通道离线怎么排查"},
	}
	return reply, []string{"点位配置帮助", "字节序如何选择", "通道离线排查"}, actions
}

func (s *Server) replyS7Guide(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `S7 协议配置要点：

通道参数
  • IP 地址 + 端口 102
  • rack（机架号，通常 0）/ slot（槽位号，CPU 通常 2）
  • 连接类型：PG / OP / Basic

点位参数
  • area：DB / M / I / Q
  • DB 块号 + 字节偏移 offset
  • 数据类型：BOOL / BYTE / WORD / DWORD / REAL
  • 字符串需注意 Siemens 专有格式

提示：可从 TIA Portal 导出 PLC 变量表（.xml）作为 Scenario A 文档输入，由 AI Model Center 批量生成点位。`

	actions := []aiAction{{Type: "navigate", Label: "采集通道", Path: "/channels"}}
	return reply, []string{"厂家点表导入指引", "协议识别方法", "点位配置帮助"}, actions
}

func (s *Server) replyBACnetGuide(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `BACnet/IP 配置要点：

通道参数
  • UDP 端口 47808（0xBAC0）
  • device_instance（本机 BACnet 设备实例号）
  • 广播/单播模式

点位参数
  • object_type：AI / AO / AV / BI / BO / BV / MSI 等
  • instance（对象实例号）
  • property_id：通常 present-value (85)

识别特征：Who-Is / I-Am / ReadProperty 报文，BVLC type 0x81。`

	return reply, []string{"协议识别方法", "点位配置帮助", "如何添加通道"}, nil
}

func (s *Server) replyOPCUAGuide(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `OPC UA 配置要点：

通道参数
  • endpoint URL（opc.tcp://host:4840）
  • 安全策略：None / Basic256Sha256 等
  • 证书与用户名（按服务器要求）

点位参数
  • NodeId 字符串（如 ns=2;s=Temperature）
  • 订阅或轮询模式

南向 OPC UA 用于采集；北向 OPC UA 用于对外提供点位服务，两者配置入口不同。`

	actions := []aiAction{
		{Type: "navigate", Label: "采集通道", Path: "/channels"},
		{Type: "navigate", Label: "北向接口", Path: "/northbound"},
	}
	return reply, []string{"北向 OPC UA 配置", "点位配置帮助", "通道诊断指标"}, actions
}

func (s *Server) replyPointConfig(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `点位配置指引（对齐 model.Point）：

必填字段
  • id / name — 点位标识与显示名
  • address — 协议地址（寄存器/NodeId/DB偏移等）
  • datatype — UINT16 / FLOAT32 / BOOL 等
  • scan_class — 采集频率分级

可选字段
  • scale / offset — 工程单位换算
  • function_code — Modbus 专用
  • byte_order — 多字节类型字节序
  • writable — 读写属性

批量导入
  • Excel/CSV 点表 → AI Model Center 文档解析（Scenario A）
  • 确认后 POST .../points/import 落库
  • 须经 Human-in-the-loop 确认，AI 不自动写入 config.db`

	actions := []aiAction{
		{Type: "navigate", Label: "采集通道", Path: "/channels"},
		{Type: "prompt", Label: "Modbus 配置", Message: "Modbus 点位怎么配置"},
	}
	return reply, []string{"Modbus 点位配置", "厂家点表导入指引", "字节序如何选择"}, actions
}

func (s *Server) replyVirtualShadow(snapshot map[string]any) (string, []string, []aiAction) {
	reply := `虚拟影子（Virtual Shadow）用于跨设备公式计算：

适用场景
  • 多点位算术运算（如总功率 = U × I）
  • 单位换算与聚合（平均值、最大值）
  • 不直接对应物理 IO 的逻辑点位

配置入口
  「虚拟影子」页面 → 新建影子 → 编写表达式

表达式支持引用其他点位 ID，由 ShadowCore 统一计算与缓存。
AI 可生成场景模版草案（EdgeRule + Shadow 组合），完整生成需 AI Model Center。`

	actions := []aiAction{{Type: "navigate", Label: "虚拟影子", Path: "/virtual-shadows"}}
	return reply, []string{"虚拟影子公式示例", "边缘规则阈值示例", "点位配置帮助"}, actions
}

func (s *Server) replyNavigationHelp(route string) (string, []string, []aiAction) {
	pages := map[string]string{
		"/":                "首页监控 — 查看通道、北向与系统资源概览",
		"/channels":        "采集通道 — 管理协议驱动、设备与点位",
		"/edge-compute":    "边缘计算 — 配置规则、动作链与执行记录",
		"/virtual-shadows": "虚拟影子 — 公式点位与跨设备计算",
		"/northbound":      "北向接口 — MQTT/HTTP/OPC UA 上报",
		"/logs":            "系统日志 — 实时日志流与检索",
		"/system":          "系统设置 — 网络、HA、同步与维护",
	}

	var reply string
	if route != "" {
		if desc, ok := pages[route]; ok {
			reply = fmt.Sprintf("您当前在「%s」。\n\n常用页面：\n%s", desc, formatPageList(pages))
		}
	}
	if reply == "" {
		reply = "edgeCore 边缘计算网关主要模块：\n" + formatPageList(pages) + "\n\n可直接问我「通道状态」「边缘规则」「Modbus 配置」「PCAP 逆向」等。"
	}

	actions := []aiAction{{Type: "navigate", Label: "首页监控", Path: "/"}}
	return reply, []string{"系统运行概况", "通道在线状态", "边缘计算帮助"}, actions
}

func (s *Server) replyWorkbenchContext(workspace, taskID string, snapshot map[string]any) (string, []string, []aiAction) {
	labels := map[string]string{
		"protocol":    "协议工作台 (G0/G1)",
		"validation":  "Schema 校验 (G2)",
		"cases":       "验证用例 (G3)",
		"edge":        "边缘场景 (G4)",
		"diagnostics": "联调诊断 (G5)",
	}
	label := labels[workspace]
	if label == "" {
		label = workspace
	}

	taskHint := ""
	if taskID != "" {
		taskHint = fmt.Sprintf("\n\n当前任务 `%s`：可在左侧查看流水线进度与产出。", taskID[len(taskID)-10:])
	}

	var hints string
	switch workspace {
	case "protocol":
		hints = "上传 PCAP 或厂家文档 → 等待四阶段流水线 → Human Confirm 导出配置。"
	case "validation":
		hints = "选择任务后运行 Schema 校验，目标通过率 ≥ 95%。"
	case "cases":
		hints = "查看 Validation Case 证据链，支持单条导出 JSON。"
	case "edge":
		hints = "描述场景关键词（温度/MQTT/持续时间），生成 EdgeRule 草案。"
	case "diagnostics":
		hints = "刷新获取通道健康、ScanEngine 与 Soak 诊断步骤。"
	default:
		hints = "切换顶部 Tab 使用不同工作台能力。"
	}

	reply := fmt.Sprintf("**当前工作台**：%s\n\n%s%s", label, hints, taskHint)
	suggestions := []string{"PCAP 逆向流程", "Schema 校验规则", "通道离线排查"}
	return reply, suggestions, nil
}

func (s *Server) replyDefault(snapshot map[string]any, route, scenario string) (string, []string, []aiAction) {
	sys, _ := snapshot["system"].(map[string]any)
	ch, _ := snapshot["channels"].(map[string]any)
	cpu, _ := sys["cpu_usage"].(string)
	channelTotal, _ := ch["total"].(int)
	deviceOnline, _ := ch["device_online"].(int)

	hint := ""
	if route != "" {
		hint = fmt.Sprintf("（当前页面：%s）", route)
	}

	modeHint := ""
	switch scenario {
	case "troubleshoot":
		modeHint = "\n\n排障模式：可问「通道离线排查」「诊断指标」「日志错误」。"
	case "config":
		modeHint = "\n\n配置模式：可问「Modbus 通道」「点位配置」「边缘规则」「北向 MQTT」。"
	case "protocol":
		modeHint = "\n\n协议模式：可问「PCAP 逆向」「协议识别」「功能码」「点表导入」。"
	}

	reply := fmt.Sprintf(
		"我是 AI助手（本地模式）%s。\n\n"+
			"当前快照：CPU %s，%d 个通道、%d 台设备在线。%s\n\n"+
			"试试：「通道在线情况」「Modbus 点位配置」「PCAP 逆向流程」",
		hint, cpu, channelTotal, deviceOnline, modeHint,
	)

	suggestions := []string{"系统运行概况", "通道在线状态", "边缘计算帮助"}
	switch scenario {
	case "troubleshoot":
		suggestions = []string{"通道离线排查", "通道诊断指标", "查看系统日志"}
	case "config":
		suggestions = []string{"如何添加 Modbus 通道？", "Modbus 点位配置", "边缘规则阈值示例"}
	case "protocol":
		suggestions = []string{"PCAP 逆向分析流程", "协议识别方法", "厂家点表导入指引"}
	}

	return reply, suggestions, nil
}

func formatPageList(pages map[string]string) string {
	order := []string{"/", "/channels", "/edge-compute", "/virtual-shadows", "/northbound", "/logs", "/system"}
	var lines []string
	for _, path := range order {
		if desc, ok := pages[path]; ok {
			lines = append(lines, fmt.Sprintf("• %s", desc))
		}
	}
	return strings.Join(lines, "\n")
}

func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

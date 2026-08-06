package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
)

type MockRunner struct {
	Mode string
}

func NewMockRunner(mode string) *MockRunner {
	if mode == "" {
		mode = "local"
	}
	return &MockRunner{Mode: mode}
}

func (r *MockRunner) InitialStages() []aitypes.StageProgress {
	return []aitypes.StageProgress{
		{Stage: aitypes.StageCapture, Label: "Capture · 抓包解帧", Status: "pending"},
		{Stage: aitypes.StageDecode, Label: "Decode · 报文结构", Status: "pending"},
		{Stage: aitypes.StageSemantic, Label: "Semantic · 物理量推理", Status: "pending"},
		{Stage: aitypes.StageOutput, Label: "Output · 生产配置", Status: "pending"},
	}
}

func (r *MockRunner) AdvanceStage(stages []aitypes.StageProgress, idx int, message string) []aitypes.StageProgress {
	now := time.Now()
	out := make([]aitypes.StageProgress, len(stages))
	copy(out, stages)
	if idx >= 0 && idx < len(out) {
		if out[idx].StartedAt == nil {
			t := now
			out[idx].StartedAt = &t
		}
		out[idx].Status = "done"
		out[idx].FinishedAt = &now
		out[idx].Message = message
	}
	if idx+1 < len(out) && out[idx+1].Status == "pending" {
		t := now
		out[idx+1].Status = "running"
		out[idx+1].StartedAt = &t
	}
	return out
}

func (r *MockRunner) GenerateDeliverables(skill aitypes.Skill, protocolID string, filename string, observations []aitypes.Observation) *aitypes.Deliverables {
	if protocolID == "" {
		protocolID = "modbus-tcp"
	}

	isDocParse := skill == aitypes.SkillDocParse
	labels := defaultLabels(observations, isDocParse)
	conn := connectionForProtocol(protocolID, filename)
	framePattern := framePatternForProtocol(protocolID)

	points := make([]aitypes.PointCandidate, 0, len(labels))
	cases := make([]aitypes.ValidationCaseEntry, 0, len(labels))
	rawHexSamples := rawHexForSkill(skill, len(labels))

	for i, l := range labels {
		addr := addressForProtocol(protocolID, i)
		conf := 0.92 - float64(i)*0.04
		if isDocParse {
			conf += 0.03
		}
		points = append(points, aitypes.PointCandidate{
			ID: l.id, Name: l.name, Address: addr,
			RegisterType: registerTypeForProtocol(protocolID),
			FunctionCode: functionCodeForProtocol(protocolID),
			Datatype:     l.datatype, ByteOrder: byteOrderForProtocol(protocolID), Scale: l.scale,
			Unit: l.unit, ReadWrite: "R", ScanClass: "normal", SlaveID: 1,
			Confidence: conf,
			Evidence:   evidenceLine(skill, protocolID, i, rawHexSamples[i], l),
		})
		cases = append(cases, aitypes.ValidationCaseEntry{
			PointID: l.id, ExpectedValue: l.value, TolerancePct: 0.5,
			ObservationTime: time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
			FrameEvidence: aitypes.FrameEvidence{
				FC: functionCodeForProtocol(protocolID), StartAddr: i * 2,
				RawHex: rawHexSamples[i], Decoded: l.value - 0.1,
			},
			Confidence: conf,
		})
	}

	channelName := channelNameFromFile(filename)
	warnings := []string{"本地模式：确定性 Mock 产出，对接 AI Model Center 后可获得真实推理结果"}
	if isDocParse {
		warnings = append(warnings, "Scenario A：已从文档表格提取点位映射（Mock）")
	} else {
		warnings = append(warnings, "Scenario B：已从 PCAP 帧序列推断寄存器布局（Mock）")
	}

	return &aitypes.Deliverables{
		ProtocolModel: &aitypes.ProtocolModel{
			ProtocolID:    protocolID,
			Confidence:    protocolConfidence(isDocParse),
			FramePattern:  framePattern,
			AddressModel:  addressModelForProtocol(protocolID),
			DatatypeRules: datatypeRulesForProtocol(protocolID),
		},
		PointDefinition: &aitypes.PointDefinition{
			Skill: string(skill), ProtocolID: protocolID,
			Points: points, Warnings: warnings,
		},
		DriverParameter: &aitypes.DriverParameter{
			ProtocolID: protocolID, Name: channelName,
			Connection:   conn,
			ScanDefaults: map[string]any{"scan_class": "normal", "report_mode": "on_change"},
		},
		ValidationCase: &aitypes.ValidationCase{Cases: cases},
	}
}

type labelDef struct {
	id, name, unit, datatype string
	value, scale             float64
}

func defaultLabels(observations []aitypes.Observation, docParse bool) []labelDef {
	base := []labelDef{
		{"uab", "Uab线电压", "V", "float32", 220.5, 0.1},
		{"ubc", "Ubc线电压", "V", "float32", 221.1, 0.1},
		{"uca", "Uca线电压", "V", "float32", 219.8, 0.1},
	}
	if docParse {
		base = []labelDef{
			{"supply_temp", "供水温度", "°C", "float32", 7.2, 0.1},
			{"return_temp", "回水温度", "°C", "float32", 12.4, 0.1},
			{"flow_rate", "瞬时流量", "m³/h", "float32", 45.6, 1.0},
			{"power_kw", "实时功率", "kW", "float32", 128.5, 0.01},
		}
	}
	if len(observations) > 0 {
		for i, obs := range observations {
			if i >= len(base) {
				break
			}
			base[i].name = obs.Label
			if obs.Label != "" {
				base[i].id = slugify(obs.Label)
			}
			base[i].value = obs.Value
			if obs.Unit != "" {
				base[i].unit = obs.Unit
			}
		}
	}
	return base
}

func (r *MockRunner) GenerateEdgeRuleDraft(description string) map[string]any {
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = "冷机出水温度超限报警"
	}

	lower := strings.ToLower(desc)
	pointID := "chiller_outlet_temp"
	threshold := 12.0
	durationSec := 30
	notifyType := "notify"
	topic := "edge/alarm/chiller"

	if strings.Contains(lower, "mqtt") {
		notifyType = "mqtt_publish"
		topic = "edge/alarm/" + slugify(extractKeyword(desc, []string{"冷机", "配电", "ups", "空调", "锅炉"}))
	}
	if strings.Contains(lower, "邮件") || strings.Contains(lower, "email") {
		notifyType = "email"
	}
	if v := extractNumber(desc); v > 0 {
		threshold = v
	}
	if d := extractDurationSec(desc); d > 0 {
		durationSec = d
	}
	if strings.Contains(lower, "功率") || strings.Contains(lower, "power") {
		pointID = "active_power"
	}
	if strings.Contains(lower, "电压") || strings.Contains(lower, "battery") {
		pointID = "battery_voltage"
	}
	if strings.Contains(lower, "温度") {
		pointID = "outlet_temperature"
	}

	condition := ">"
	if strings.Contains(lower, "低于") || strings.Contains(lower, "小于") {
		condition = "<"
	}

	return map[string]any{
		"name":        "copilot-" + slugify(pointID) + "-rule",
		"description": desc,
		"enabled":     false,
		"trigger": map[string]any{
			"type": "threshold", "point_id": pointID,
			"condition": condition, "value": threshold,
			"duration_sec": durationSec,
		},
		"actions": []map[string]any{
			{"type": notifyType, "topic": topic, "message": desc + " — 请检查设备运行状态"},
		},
		"metadata": map[string]any{
			"source": "ai-copilot-local", "mode": r.Mode,
			"keywords_parsed": map[string]any{
				"point_id": pointID, "threshold": threshold, "duration_sec": durationSec,
			},
			"note": "草案需人工确认后启用；完整 expr 校验需 AI Model Center",
		},
	}
}

func connectionForProtocol(protocolID, filename string) map[string]any {
	switch protocolID {
	case "bacnet-ip":
		return map[string]any{
			"ip": "192.168.1.50", "port": 47808, "device_id": 1001,
			"timeout_ms": 5000, "retries": 2,
		}
	case "s7":
		return map[string]any{
			"ip": "192.168.1.10", "rack": 0, "slot": 1,
			"timeout_ms": 4000, "retries": 2,
		}
	case "modbus-rtu":
		return map[string]any{
			"port": "/dev/ttyUSB0", "baud_rate": 9600, "parity": "N",
			"data_bits": 8, "stop_bits": 1, "slave_id": 1,
			"timeout_ms": 3000, "retries": 2,
		}
	default:
		return map[string]any{
			"ip": "192.168.1.100", "port": 502, "slave_id": 1,
			"timeout_ms": 3000, "retries": 2,
			"source_file": filename,
		}
	}
}

func framePatternForProtocol(protocolID string) map[string]any {
	switch protocolID {
	case "bacnet-ip":
		return map[string]any{"transport": "udp", "port": 47808, "service": "read-property", "default_endian": "big"}
	case "s7":
		return map[string]any{"transport": "tcp", "port": 102, "pdu_type": "read", "area": "DB"}
	default:
		return map[string]any{"transport": "tcp", "port": 502, "header": "mbap", "default_byte_order": "ABCD"}
	}
}

func addressForProtocol(protocolID string, idx int) string {
	switch protocolID {
	case "bacnet-ip":
		return fmt.Sprintf("analog-input,%d", idx+1)
	case "s7":
		return fmt.Sprintf("DB1.DBD%d", idx*4)
	default:
		return fmt.Sprintf("%d", 40001+idx*2)
	}
}

func registerTypeForProtocol(protocolID string) string {
	if protocolID == "bacnet-ip" || protocolID == "s7" {
		return ""
	}
	return "holding"
}

func functionCodeForProtocol(protocolID string) int {
	if protocolID == "bacnet-ip" || protocolID == "s7" {
		return 0
	}
	return 3
}

func byteOrderForProtocol(protocolID string) string {
	if protocolID == "s7" {
		return "DCBA"
	}
	return "ABCD"
}

func addressModelForProtocol(protocolID string) string {
	switch protocolID {
	case "bacnet-ip":
		return "bacnet_object_instance"
	case "s7":
		return "s7_db_block"
	default:
		return "holding_register_4xxxx"
	}
}

func datatypeRulesForProtocol(protocolID string) []string {
	if protocolID == "bacnet-ip" {
		return []string{"real@4bytes", "enumerated@1"}
	}
	if protocolID == "s7" {
		return []string{"real@4bytes", "int@2bytes"}
	}
	return []string{"float32@2regs", "uint16@1reg"}
}

func protocolConfidence(docParse bool) float64 {
	if docParse {
		return 0.97
	}
	return 0.95
}

func rawHexForSkill(skill aitypes.Skill, n int) []string {
	if skill == aitypes.SkillDocParse {
		samples := []string{"40E66666", "41466666", "42366666", "43000000"}
		if n > len(samples) {
			n = len(samples)
		}
		return samples[:n]
	}
	samples := []string{"43DC6666", "43DD8F5C", "43DBCCCD", "43480000"}
	if n > len(samples) {
		n = len(samples)
	}
	return samples[:n]
}

func evidenceLine(skill aitypes.Skill, protocolID string, i int, rawHex string, l labelDef) string {
	fc := functionCodeForProtocol(protocolID)
	if skill == aitypes.SkillDocParse {
		return fmt.Sprintf("文档行 %d → %s=%.1f%s; 表格映射 confidence=high", i+1, l.name, l.value, l.unit)
	}
	return fmt.Sprintf("FC%02d rsp offset=%d raw=0x%s → %.1f%s; polling 5s", fc, i*4, rawHex, l.value, l.unit)
}

func channelNameFromFile(filename string) string {
	channelName := "copilot-chiller-01"
	if filename != "" {
		base := strings.TrimSuffix(filename, filepathExt(filename))
		if base != "" {
			channelName = "copilot-" + slugify(base)
		}
	}
	return channelName
}

func filepathExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[idx:]
	}
	return ""
}

var numRe = regexp.MustCompile(`(\d+(?:\.\d+)?)`)

func extractNumber(s string) float64 {
	m := numRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v
}

func extractDurationSec(s string) int {
	if strings.Contains(s, "分钟") {
		if v := int(extractNumber(s)); v > 0 {
			return v * 60
		}
	}
	if strings.Contains(s, "秒") || strings.Contains(s, "s") {
		if v := int(extractNumber(s)); v > 0 {
			return v
		}
	}
	return 0
}

func extractKeyword(s string, keywords []string) string {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return kw
		}
	}
	return "device"
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '-' {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "point"
	}
	return out
}

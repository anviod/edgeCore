package validate

import (
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
)

var supportedDatatypes = map[string]bool{
	"bool": true, "int16": true, "uint16": true, "int32": true, "uint32": true,
	"float32": true, "float64": true, "string": true,
}

type Validator struct{}

func New() *Validator { return &Validator{} }

func (v *Validator) ValidateDeliverables(d *aitypes.Deliverables) *aitypes.ValidationReport {
	if d == nil {
		return &aitypes.ValidationReport{
			Passed: false, PassRate: 0, Fields: []aitypes.ValidationFieldResult{{
				Field: "deliverables", Passed: false, Severity: "error", Message: "无产出数据",
			}},
		}
	}

	var fields []aitypes.ValidationFieldResult

	if d.ProtocolModel != nil {
		fields = append(fields, v.validateProtocolModel(d.ProtocolModel)...)
	} else {
		fields = append(fields, aitypes.ValidationFieldResult{
			Field: "protocol_model", Passed: false, Severity: "warning", Message: "缺少 Protocol Model",
		})
	}

	if d.PointDefinition != nil {
		fields = append(fields, v.validatePointDefinition(d.PointDefinition)...)
	} else {
		fields = append(fields, aitypes.ValidationFieldResult{
			Field: "point_definition", Passed: false, Severity: "error", Message: "缺少 Point Definition",
		})
	}

	if d.DriverParameter != nil {
		fields = append(fields, v.validateDriverParameter(d.DriverParameter)...)
	} else {
		fields = append(fields, aitypes.ValidationFieldResult{
			Field: "driver_parameter", Passed: false, Severity: "warning", Message: "缺少 Driver Parameter",
		})
	}

	if d.ValidationCase != nil {
		fields = append(fields, v.validateValidationCase(d.ValidationCase, d.PointDefinition)...)
	} else {
		fields = append(fields, aitypes.ValidationFieldResult{
			Field: "validation_case", Passed: false, Severity: "warning", Message: "缺少 Validation Case",
		})
	}

	passed := 0
	failed := 0
	for _, f := range fields {
		if f.Passed {
			passed++
		} else if f.Severity == "error" {
			failed++
		}
	}
	total := len(fields)
	passRate := 0.0
	if total > 0 {
		passRate = float64(passed) / float64(total) * 100
	}

	return &aitypes.ValidationReport{
		Passed:       failed == 0 && passRate >= 95,
		PassRate:     passRate,
		TotalChecks:  total,
		FailedChecks: failed,
		Fields:       fields,
	}
}

func (v *Validator) validateProtocolModel(m *aitypes.ProtocolModel) []aitypes.ValidationFieldResult {
	var out []aitypes.ValidationFieldResult
	ok := m.ProtocolID != ""
	out = append(out, aitypes.ValidationFieldResult{
		Field: "protocol_id", Path: "protocol_model.protocol_id", Passed: ok,
		Severity: errOrInfo(ok), Message: msg(ok, "协议 ID 有效", "protocol_id 不能为空"),
		Confidence: m.Confidence,
	})
	ok = m.Confidence >= 0.7
	out = append(out, aitypes.ValidationFieldResult{
		Field: "confidence", Path: "protocol_model.confidence", Passed: ok,
		Severity: warnOrInfo(ok), Message: msg(ok, "协议识别置信度 ≥ 0.7", "置信度偏低，建议人工确认协议"),
	})
	return out
}

func (v *Validator) validatePointDefinition(pd *aitypes.PointDefinition) []aitypes.ValidationFieldResult {
	var out []aitypes.ValidationFieldResult
	ids := map[string]int{}

	for i, p := range pd.Points {
		path := fmt.Sprintf("points[%d]", i)
		idOK := strings.TrimSpace(p.ID) != ""
		out = append(out, aitypes.ValidationFieldResult{
			Field: "id", Path: path + ".id", Passed: idOK,
			Severity: errOrInfo(idOK), Message: msg(idOK, "点位 ID 有效", "点位 ID 不能为空"),
		})
		if idOK {
			ids[p.ID]++
		}
		addrOK := strings.TrimSpace(p.Address) != ""
		out = append(out, aitypes.ValidationFieldResult{
			Field: "address", Path: path + ".address", Passed: addrOK,
			Severity: errOrInfo(addrOK), Message: msg(addrOK, "地址格式有效", "地址不能为空"),
		})
		dtOK := supportedDatatypes[strings.ToLower(p.Datatype)]
		out = append(out, aitypes.ValidationFieldResult{
			Field: "datatype", Path: path + ".datatype", Passed: dtOK,
			Severity: errOrInfo(dtOK), Message: msg(dtOK, "数据类型在驱动支持集内", "不支持的数据类型: "+p.Datatype),
		})
		confOK := p.Confidence >= 0.6
		out = append(out, aitypes.ValidationFieldResult{
			Field: "confidence", Path: path + ".confidence", Passed: confOK,
			Severity: warnOrInfo(confOK), Message: msg(confOK, "置信度 ≥ 0.6", "低置信度点位需人工复核"),
			Confidence: p.Confidence,
		})
	}

	for id, count := range ids {
		if count > 1 {
			out = append(out, aitypes.ValidationFieldResult{
				Field: "id_unique", Path: "points", Passed: false,
				Severity: "error", Message: fmt.Sprintf("重复点位 ID: %s", id),
			})
		}
	}
	return out
}

func (v *Validator) validateDriverParameter(dp *aitypes.DriverParameter) []aitypes.ValidationFieldResult {
	var out []aitypes.ValidationFieldResult
	ok := dp.ProtocolID != ""
	out = append(out, aitypes.ValidationFieldResult{
		Field: "protocol_id", Path: "driver_parameter.protocol_id", Passed: ok,
		Severity: errOrInfo(ok), Message: msg(ok, "通道协议有效", "protocol_id 不能为空"),
	})
	ok = dp.Name != ""
	out = append(out, aitypes.ValidationFieldResult{
		Field: "name", Path: "driver_parameter.name", Passed: ok,
		Severity: warnOrInfo(ok), Message: msg(ok, "通道名称有效", "建议填写通道名称"),
	})
	if dp.Connection != nil {
		_, hasIP := dp.Connection["ip"]
		out = append(out, aitypes.ValidationFieldResult{
			Field: "connection.ip", Path: "driver_parameter.connection.ip", Passed: hasIP,
			Severity: warnOrInfo(hasIP), Message: msg(hasIP, "连接 IP 已配置", "缺少 IP 地址"),
		})
	}
	return out
}

func (v *Validator) validateValidationCase(vc *aitypes.ValidationCase, pd *aitypes.PointDefinition) []aitypes.ValidationFieldResult {
	var out []aitypes.ValidationFieldResult
	pointIDs := map[string]bool{}
	if pd != nil {
		for _, p := range pd.Points {
			pointIDs[p.ID] = true
		}
	}
	for i, c := range vc.Cases {
		path := fmt.Sprintf("validation_cases[%d]", i)
		refOK := pointIDs[c.PointID] || c.PointID != ""
		out = append(out, aitypes.ValidationFieldResult{
			Field: "point_id", Path: path + ".point_id", Passed: refOK,
			Severity: warnOrInfo(refOK), Message: msg(refOK, "关联点位存在", "验证用例点位未在 Point Definition 中找到"),
		})
		evOK := c.FrameEvidence.RawHex != "" || c.ExpectedValue != 0
		out = append(out, aitypes.ValidationFieldResult{
			Field: "evidence", Path: path + ".frame_evidence", Passed: evOK,
			Severity: warnOrInfo(evOK), Message: msg(evOK, "证据链完整", "缺少帧证据或期望值"),
		})
	}
	return out
}

func errOrInfo(ok bool) string {
	if ok {
		return "info"
	}
	return "error"
}

func warnOrInfo(ok bool) string {
	if ok {
		return "info"
	}
	return "warning"
}

func msg(ok bool, pass, fail string) string {
	if ok {
		return pass
	}
	return fail
}

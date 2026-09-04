package core

import (
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// DLT645PointTemplate defines a standard DL/T 645-2007 data identifier point.
type DLT645PointTemplate struct {
	ID       string
	Name     string
	DataID   string
	DataType string
	Scale    float64
	Unit     string
}

// DLT645StandardPointTemplates lists common standard DIs from DL/T 645-2007 section 4.2.3
// and appendix A.2–A.6 (commonly deployed subset).
var DLT645StandardPointTemplates = []DLT645PointTemplate{
	// DI3 = 00 — 电能量
	{ID: "combined_active_energy", Name: "当前组合有功总电能", DataID: "00-00-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "last_settlement_combined_active_energy", Name: "上1结算日组合有功总电能", DataID: "00-00-00-01", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "associated_total_energy", Name: "当前关联总电能", DataID: "00-80-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "last_settlement_a_forward_active_energy", Name: "上1结算日A相正向有功电能", DataID: "00-15-00-01", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "forward_active_energy", Name: "当前正向有功总电能", DataID: "00-01-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "last_settlement_forward_active_energy", Name: "上1结算日正向有功总电能", DataID: "00-01-00-01", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "reverse_active_energy", Name: "当前反向有功总电能", DataID: "00-02-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "combined_reactive1_energy", Name: "当前组合无功1总电能", DataID: "00-03-00-00", DataType: "uint64", Scale: 0.01, Unit: "kvarh"},
	{ID: "combined_reactive2_energy", Name: "当前组合无功2总电能", DataID: "00-04-00-00", DataType: "uint64", Scale: 0.01, Unit: "kvarh"},
	{ID: "forward_reactive_energy", Name: "当前正向无功总电能", DataID: "00-05-00-00", DataType: "uint64", Scale: 0.01, Unit: "kvarh"},
	{ID: "reverse_reactive_energy", Name: "当前反向无功总电能", DataID: "00-06-00-00", DataType: "uint64", Scale: 0.01, Unit: "kvarh"},
	{ID: "a_phase_forward_active_energy", Name: "当前A相正向有功电能", DataID: "00-15-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "a_phase_reverse_active_energy", Name: "当前A相反向有功电能", DataID: "00-16-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "b_phase_forward_active_energy", Name: "当前B相正向有功电能", DataID: "00-29-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "b_phase_reverse_active_energy", Name: "当前B相反向有功电能", DataID: "00-2A-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "c_phase_forward_active_energy", Name: "当前C相正向有功电能", DataID: "00-3D-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	{ID: "c_phase_reverse_active_energy", Name: "当前C相反向有功电能", DataID: "00-3E-00-00", DataType: "uint64", Scale: 0.01, Unit: "kWh"},
	// DI3 = 01 — 最大需量
	{ID: "forward_active_max_demand", Name: "当前正向有功最大需量", DataID: "01-01-00-00", DataType: "uint64", Scale: 0.0001, Unit: "kW"},
	{ID: "forward_active_max_demand_time", Name: "当前正向有功最大需量发生时间", DataID: "01-01-00-00#T", DataType: "string", Scale: 0, Unit: ""},
	{ID: "reverse_active_max_demand", Name: "当前反向有功最大需量", DataID: "01-02-00-00", DataType: "uint64", Scale: 0.0001, Unit: "kW"},
	{ID: "reverse_active_max_demand_time", Name: "当前反向有功最大需量发生时间", DataID: "01-02-00-00#T", DataType: "string", Scale: 0, Unit: ""},
	// DI3 = 02 — 变量
	{ID: "a_phase_voltage", Name: "A相电压", DataID: "02-01-01-00", DataType: "uint16", Scale: 0.1, Unit: "V"},
	{ID: "b_phase_voltage", Name: "B相电压", DataID: "02-01-02-00", DataType: "uint16", Scale: 0.1, Unit: "V"},
	{ID: "c_phase_voltage", Name: "C相电压", DataID: "02-01-03-00", DataType: "uint16", Scale: 0.1, Unit: "V"},
	{ID: "a_phase_current", Name: "A相电流", DataID: "02-02-01-00", DataType: "uint32", Scale: 0.001, Unit: "A"},
	{ID: "b_phase_current", Name: "B相电流", DataID: "02-02-02-00", DataType: "uint32", Scale: 0.001, Unit: "A"},
	{ID: "c_phase_current", Name: "C相电流", DataID: "02-02-03-00", DataType: "uint32", Scale: 0.001, Unit: "A"},
	{ID: "total_active_power", Name: "瞬时总有功功率", DataID: "02-03-00-00", DataType: "int32", Scale: 0.0001, Unit: "kW"},
	{ID: "a_phase_active_power", Name: "A相有功功率", DataID: "02-03-01-00", DataType: "int32", Scale: 0.1, Unit: "kW"},
	{ID: "b_phase_active_power", Name: "B相有功功率", DataID: "02-03-02-00", DataType: "int32", Scale: 0.0001, Unit: "kW"},
	{ID: "c_phase_active_power", Name: "C相有功功率", DataID: "02-03-03-00", DataType: "int32", Scale: 0.0001, Unit: "kW"},
	{ID: "total_reactive_power", Name: "瞬时总无功功率", DataID: "02-04-00-00", DataType: "int32", Scale: 0.0001, Unit: "kvar"},
	{ID: "total_apparent_power", Name: "瞬时总视在功率", DataID: "02-05-00-00", DataType: "int32", Scale: 0.0001, Unit: "kVA"},
	{ID: "total_power_factor", Name: "总功率因数", DataID: "02-06-00-00", DataType: "uint16", Scale: 0.001, Unit: ""},
	{ID: "a_phase_power_factor", Name: "A相功率因数", DataID: "02-06-01-00", DataType: "uint16", Scale: 0.001, Unit: ""},
	{ID: "b_phase_power_factor", Name: "B相功率因数", DataID: "02-06-02-00", DataType: "uint16", Scale: 0.001, Unit: ""},
	{ID: "c_phase_power_factor", Name: "C相功率因数", DataID: "02-06-03-00", DataType: "uint16", Scale: 0.001, Unit: ""},
	{ID: "a_phase_voltage_harmonic_1", Name: "A相电压1次谐波含量", DataID: "02-0A-01-01", DataType: "uint16", Scale: 0.01, Unit: "%"},
	{ID: "b_phase_voltage_harmonic_1", Name: "B相电压1次谐波含量", DataID: "02-0A-02-01", DataType: "uint16", Scale: 0.01, Unit: "%"},
	{ID: "c_phase_voltage_harmonic_1", Name: "C相电压1次谐波含量", DataID: "02-0A-03-01", DataType: "uint16", Scale: 0.01, Unit: "%"},
	{ID: "a_phase_current_harmonic_1", Name: "A相电流1次谐波含量", DataID: "02-0B-01-01", DataType: "uint16", Scale: 0.01, Unit: "%"},
	{ID: "neutral_current", Name: "零线电流", DataID: "02-80-00-01", DataType: "uint16", Scale: 0.01, Unit: "A"},
	{ID: "grid_frequency", Name: "电网频率", DataID: "02-80-00-02", DataType: "uint16", Scale: 0.01, Unit: "Hz"},
	{ID: "current_active_demand", Name: "当前有功需量", DataID: "02-80-00-04", DataType: "int32", Scale: 0.0001, Unit: "kW"},
	// DI3 = 04 — 参变量
	{ID: "date_time", Name: "日期及时间", DataID: "04-00-01-01", DataType: "string", Scale: 0, Unit: ""},
	{ID: "max_demand_period", Name: "最大需量周期", DataID: "04-00-01-03", DataType: "uint16", Scale: 0, Unit: "min"},
	{ID: "slip_time", Name: "滑差时间", DataID: "04-00-01-04", DataType: "uint16", Scale: 0, Unit: "min"},
	{ID: "tariff_count", Name: "费率数", DataID: "04-00-02-04", DataType: "uint8", Scale: 0, Unit: ""},
	{ID: "communication_address", Name: "通信地址", DataID: "04-00-04-01", DataType: "string", Scale: 0, Unit: ""},
	{ID: "meter_number", Name: "表号", DataID: "04-00-04-02", DataType: "string", Scale: 0, Unit: ""},
	{ID: "meter_status_word_1", Name: "电表运行状态字1", DataID: "04-00-05-01", DataType: "uint32", Scale: 0, Unit: ""},
	{ID: "meter_status_word_2", Name: "电表运行状态字2", DataID: "04-00-05-02", DataType: "uint32", Scale: 0, Unit: ""},
	// DI3 = 05 — 冻结数据
	{ID: "last_timed_freeze_time", Name: "上1次定时冻结时间", DataID: "05-00-00-01", DataType: "string", Scale: 0, Unit: ""},
	{ID: "last_timed_freeze_forward_active_energy", Name: "上1次定时冻结正向有功总电能", DataID: "05-00-01-01", DataType: "uint32", Scale: 0.01, Unit: "kWh"},
	{ID: "last_timed_freeze_reverse_active_energy", Name: "上1次定时冻结反向有功总电能", DataID: "05-00-02-01", DataType: "uint32", Scale: 0.01, Unit: "kWh"},
	{ID: "last_timed_freeze_forward_active_max_demand", Name: "上1次定时冻结正向有功最大需量", DataID: "05-00-09-01", DataType: "uint32", Scale: 0.0001, Unit: "kW"},
	{ID: "last_timed_freeze_forward_active_max_demand_time", Name: "上1次定时冻结正向有功最大需量发生时间", DataID: "05-00-09-01#T", DataType: "string", Scale: 0, Unit: ""},
	{ID: "last_instant_freeze_time", Name: "上1次瞬时冻结时间", DataID: "05-01-00-01", DataType: "string", Scale: 0, Unit: ""},
	{ID: "last_instant_freeze_forward_active_energy", Name: "上1次瞬时冻结正向有功总电能", DataID: "05-01-01-01", DataType: "uint32", Scale: 0.01, Unit: "kWh"},
	{ID: "last_daily_freeze_time", Name: "上1次日冻结时间", DataID: "05-06-00-01", DataType: "string", Scale: 0, Unit: ""},
	{ID: "last_daily_freeze_forward_active_energy", Name: "上1次日冻结正向有功总电能", DataID: "05-06-01-01", DataType: "uint32", Scale: 0.01, Unit: "kWh"},
}

// IsDLT645AutoPointsEnabled reports whether standard DI points should be auto-created.
// Defaults to true when the key is absent (new devices and legacy configs).
func IsDLT645AutoPointsEnabled(config map[string]any) bool {
	if config == nil {
		return true
	}
	v, ok := config["auto_points_enabled"]
	if !ok {
		return true
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true") || b == "1"
	default:
		return fmt.Sprintf("%v", v) == "true" || fmt.Sprintf("%v", v) == "1"
	}
}

// DLT645StationAddress returns the 12-digit meter address from device config.
func DLT645StationAddress(config map[string]any) string {
	if config == nil {
		return ""
	}
	if addr, ok := config["station_address"]; ok {
		if s := strings.TrimSpace(fmt.Sprintf("%v", addr)); s != "" {
			return s
		}
	}
	if addr, ok := config["address"]; ok {
		return strings.TrimSpace(fmt.Sprintf("%v", addr))
	}
	return ""
}

// GenerateDLT645StandardPoints builds standard DI points for a meter address.
func GenerateDLT645StandardPoints(stationAddress, deviceID string) []model.Point {
	addr := strings.TrimSpace(stationAddress)
	if addr == "" {
		return nil
	}
	points := make([]model.Point, 0, len(DLT645StandardPointTemplates))
	for _, tpl := range DLT645StandardPointTemplates {
		points = append(points, model.Point{
			Name:      tpl.Name,
			ID:        tpl.ID,
			DeviceID:  deviceID,
			Address:   fmt.Sprintf("%s#%s", addr, tpl.DataID),
			DataType:  tpl.DataType,
			ReadWrite: "R",
			Scale:     tpl.Scale,
			Unit:      tpl.Unit,
		})
	}
	return points
}

func (cm *ChannelManager) autoGenerateDLT645PointsFromConfig(dev *model.Device) {
	if dev == nil || !IsDLT645AutoPointsEnabled(dev.Config) {
		return
	}
	addr := DLT645StationAddress(dev.Config)
	if addr == "" {
		return
	}
	for _, p := range GenerateDLT645StandardPoints(addr, dev.ID) {
		if err := cm.validateDLT645Point(&p); err != nil {
			continue
		}
		dev.Points = append(dev.Points, p)
	}
}

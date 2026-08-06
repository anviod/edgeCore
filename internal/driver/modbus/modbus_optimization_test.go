package modbus

import (
	"context"
	"fmt"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

// TestGroupPoints 测试点位分组功能
func TestGroupPoints(t *testing.T) {
	// Initialize components
	decoder := NewPointDecoder("ABCD", 0, 0)
	// mock transport can be nil for grouping test
	// maxPacketSize=125 registers, groupThreshold=50
	scheduler := NewPointScheduler(nil, decoder, 125, 50, 0)

	// 测试场景1：连续的点位应该分组
	points := []model.Point{
		{ID: "point1", Address: "0", DataType: "int16"},
		{ID: "point2", Address: "1", DataType: "int16"},
		{ID: "point3", Address: "2", DataType: "int16"},
	}

	groups, err := scheduler.groupPoints(points)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(groups) != 1 {
		t.Errorf("Expected 1 group, got: %d", len(groups))
	}

	if len(groups) > 0 && groups[0].Count != 3 {
		t.Errorf("Expected group count 3, got: %d", groups[0].Count)
	}

	// 测试场景2：地址间隔大的点位应该分组
	points = []model.Point{
		{ID: "point1", Address: "0", DataType: "int16"},
		{ID: "point2", Address: "99", DataType: "int16"}, // 间隔99，超过阈值50
	}

	groups, err = scheduler.groupPoints(points)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(groups) != 2 {
		t.Errorf("Expected 2 groups due to large gap, got: %d", len(groups))
	}

	// 测试场景3：不同寄存器类型应该分组
	points = []model.Point{
		{ID: "point1", Address: "0", DataType: "int16", RegisterType: model.RegHolding}, // HOLDING_REGISTER
		{ID: "point2", Address: "0", DataType: "int16", RegisterType: model.RegInput},   // INPUT_REGISTER
	}

	groups, err = scheduler.groupPoints(points)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(groups) != 2 {
		t.Errorf("Expected 2 groups for different types, got: %d", len(groups))
	}

	// 测试场景4：32位数据类型占用2个寄存器
	points = []model.Point{
		{ID: "point1", Address: "0", DataType: "float32"}, // 占用2个寄存器
		{ID: "point2", Address: "2", DataType: "int16"},   // 占用1个寄存器
	}

	groups, err = scheduler.groupPoints(points)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(groups) != 1 {
		t.Errorf("Expected 1 group, got: %d", len(groups))
	}

	// 总计应该是 3 个寄存器（2 + 1）
	if len(groups) > 0 && groups[0].Count != 3 {
		t.Errorf("Expected total count 3, got: %d", groups[0].Count)
	}
}

// TestRegisterCount 测试寄存器数量计算
func TestRegisterCount(t *testing.T) {
	decoder := NewPointDecoder("ABCD", 0, 0)

	tests := []struct {
		dataType string
		expected uint16
	}{
		{"int16", 1},
		{"uint16", 1},
		{"int32", 2},
		{"uint32", 2},
		{"float32", 2},
		{"int64", 4},
		{"uint64", 4},
		{"float64", 4},
		{"boolean", 1},
	}

	for _, test := range tests {
		count := decoder.GetRegisterCount(test.dataType)
		if count != test.expected {
			t.Errorf("DataType %s: expected %d, got %d", test.dataType, test.expected, count)
		}
	}
}

// TestModbus100DiscreteGroupingReduction 验证 Gap 块读相对逐点读取减少请求数 ≥30%。
func TestModbus100DiscreteGroupingReduction(t *testing.T) {
	decoder := NewPointDecoder("ABCD", 0, 0)
	scheduler := NewPointScheduler(nil, decoder, 125, 64, 0)

	points := make([]model.Point, 100)
	for i := range points {
		points[i] = model.Point{
			ID:       fmt.Sprintf("p%d", i),
			Address:  fmt.Sprintf("%d", i*5),
			DataType: "int16",
		}
	}

	groups, err := scheduler.groupPoints(points)
	if err != nil {
		t.Fatalf("groupPoints: %v", err)
	}

	perPointBaseline := len(points)
	reduction := 1.0 - float64(len(groups))/float64(perPointBaseline)
	if reduction < 0.30 {
		t.Fatalf("request reduction %.1f%% < 30%% (baseline=%d groups=%d)",
			reduction*100, perPointBaseline, len(groups))
	}
}

// TestAddressBase tests the address base functionality
func TestAddressBase(t *testing.T) {
	// Test 0-based (default) with raw address 0
	decoder0 := NewPointDecoder("ABCD", 0, 0)
	regType, offset, err := decoder0.ParseAddress("0")
	if err != nil {
		t.Fatalf("ParseAddress failed: %v", err)
	}
	if regType != model.RegHolding {
		t.Errorf("Expected RegHolding, got %v", regType)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0, got %d", offset)
	}

	// Test 1-based with raw address 20001 (above discrete input range 10001-20000)
	decoder1 := NewPointDecoder("ABCD", 0, 1)
	regType, offset, err = decoder1.ParseAddress("20001")
	if err != nil {
		t.Fatalf("ParseAddress failed: %v", err)
	}
	if regType != model.RegHolding {
		t.Errorf("Expected RegHolding, got %v", regType)
	}
	if offset != 20000 {
		t.Errorf("Expected offset 20000, got %d", offset)
	}

	// Test 1-based with address 20002 (above discrete input range)
	regType, offset, err = decoder1.ParseAddress("20002")
	if err != nil {
		t.Fatalf("ParseAddress failed: %v", err)
	}
	if regType != model.RegHolding {
		t.Errorf("Expected RegHolding, got %v", regType)
	}
	if offset != 20001 {
		t.Errorf("Expected offset 20001, got %d", offset)
	}

	// Test that standard Modbus addresses (40001+) still work
	regType, offset, err = decoder1.ParseAddress("40001")
	if err != nil {
		t.Fatalf("ParseAddress failed: %v", err)
	}
	if regType != model.RegHolding {
		t.Errorf("Expected RegHolding, got %v", regType)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0, got %d", offset)
	}

	fmt.Println("TestAddressBase passed successfully")
}

// TestParseAddress_UnderflowFix 验证 addressBase > addrInt 时 uint16 下溢问题已修复
// 对应 bug：当 addressBase=1 且地址为 "0" 时，
// 原代码 uint16(0)-1=65535，int(65535)>0 导致下溢检测失效，
// 最终 hr_0 读取失败（Bad），后续点位整体错位 1 个寄存器。
func TestParseAddress_UnderflowFix(t *testing.T) {
	decoder := NewPointDecoder("ABCD", 0, 1) // addressBase = 1（1-based 寻址）

	// 地址 0 < addressBase → 应 clamp 到 0，而非下溢到 65535
	_, offset, err := decoder.ParseAddress("0")
	if err != nil {
		t.Fatalf("ParseAddress(\"0\") failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("addressBase=1, addr=0: expected offset 0, got %d (underflow bug!)", offset)
	}

	// 地址 1 == addressBase → 偏移 0
	_, offset, err = decoder.ParseAddress("1")
	if err != nil {
		t.Fatalf("ParseAddress(\"1\") failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("addressBase=1, addr=1: expected offset 0, got %d", offset)
	}

	// 地址 2 > addressBase → 偏移 1
	_, offset, err = decoder.ParseAddress("2")
	if err != nil {
		t.Fatalf("ParseAddress(\"2\") failed: %v", err)
	}
	if offset != 1 {
		t.Errorf("addressBase=1, addr=2: expected offset 1, got %d", offset)
	}

	// 验证标准地址不受 addressBase 影响（40001 始终对应偏移 0）
	_, offset, err = decoder.ParseAddress("40001")
	if err != nil {
		t.Fatalf("ParseAddress(\"40001\") failed: %v", err)
	}
	if offset != 0 {
		t.Errorf("standard addr 40001: expected offset 0, got %d (addressBase should not apply)", offset)
	}
}

// TestSchedulerRead_WithAddressBase 验证 addressBase>0 时批量读取不会发生错位
// 复现场景：3 个点位，addressBase=1，点位地址 0/1/2
// 预期：点位 0 被 clamp 到偏移 0（与点位 1 同位置）或被合理处理
func TestSchedulerRead_WithAddressBase(t *testing.T) {
	mock := newMockModbusTransport()
	mock.connected = true
	// 设备寄存器：偏移 0 = 100, 偏移 1 = 200, 偏移 2 = 300
	mock.registers[0] = 100
	mock.registers[1] = 200
	mock.registers[2] = 300

	dec := NewPointDecoder("ABCD", 0, 1) // addressBase = 1
	s := NewPointScheduler(mock, dec, 125, 50, 0)

	// 模拟用户创建的点位（地址为 "1"/"2"/"3"，即 1-based 寻址）
	points := []model.Point{
		{ID: "hr_1", Address: "1", DataType: "uint16", RegisterType: model.RegHolding},
		{ID: "hr_2", Address: "2", DataType: "uint16", RegisterType: model.RegHolding},
		{ID: "hr_3", Address: "3", DataType: "uint16", RegisterType: model.RegHolding},
	}
	results, err := s.Read(context.Background(), points)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// hr_1 (addr=1, base=1 → offset=0) 应读到 register 0 = 100
	if results["hr_1"].Value != uint16(100) {
		t.Errorf("hr_1: expected 100, got %v", results["hr_1"].Value)
	}
	if results["hr_1"].Quality != "Good" {
		t.Errorf("hr_1: expected Good, got %s", results["hr_1"].Quality)
	}

	// hr_2 (addr=2, base=1 → offset=1) 应读到 register 1 = 200
	if results["hr_2"].Value != uint16(200) {
		t.Errorf("hr_2: expected 200, got %v", results["hr_2"].Value)
	}

	// hr_3 (addr=3, base=1 → offset=2) 应读到 register 2 = 300
	if results["hr_3"].Value != uint16(300) {
		t.Errorf("hr_3: expected 300, got %v", results["hr_3"].Value)
	}
}

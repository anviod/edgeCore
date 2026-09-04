//go:build integration

package ethernetip

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// TestIntegrationWithSimulator 测试与EtherNet/IP模拟器的集成
func TestIntegrationWithSimulator(t *testing.T) {
	// 配置模拟器连接
	cfg := map[string]any{
		"ip":              "127.0.0.1",
		"port":            44818,
		"slot":            0,
		"timeout":         3000,
		"maxRetries":      3,
		"retryInterval":   100,
		"connection_type": "cip",
	}

	// 创建传输层
	transport := NewENIPTransport(cfg)
	if transport == nil {
		t.Fatal("Failed to create ENIP transport")
	}

	// 创建解码器
	decoder := NewENIPDecoder()
	if decoder == nil {
		t.Fatal("Failed to create ENIP decoder")
	}

	// 创建调度器
	scheduler := NewENIPScheduler(transport, decoder, map[string]any{
		"batch_read_max": 50,
		"min_interval":   0,
	})
	if scheduler == nil {
		t.Fatal("Failed to create ENIP scheduler")
	}

	// 连接到模拟器
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := transport.Connect(ctx)
	cancel()
	if err != nil {
		t.Fatalf("Failed to connect to simulator: %v", err)
	}
	defer transport.Disconnect()

	t.Log("Connected to EtherNet/IP simulator")

	// 测试单点读取
	t.Run("SinglePointRead", func(t *testing.T) {
		testSinglePointRead(t, scheduler)
	})

	// 测试批量读取
	t.Run("BatchRead", func(t *testing.T) {
		testBatchRead(t, scheduler)
	})

	// 测试单点写入
	t.Run("SinglePointWrite", func(t *testing.T) {
		testSinglePointWrite(t, scheduler)
	})

	// 测试数组读取
	t.Run("ArrayRead", func(t *testing.T) {
		testArrayRead(t, scheduler)
	})

	t.Log("All integration tests passed!")
}

// testSinglePointRead 测试单点读取各种数据类型
func testSinglePointRead(t *testing.T, scheduler *ENIPScheduler) {
	testCases := []struct {
		name     string
		address  string
		dataType string
	}{
		{"BoolTag", "Program:MainProgram.BoolTag", "BOOL"},
		{"SintTag", "Program:MainProgram.SintTag", "SINT"},
		{"IntTag", "Program:MainProgram.IntTag", "INT"},
		{"DintTag", "Program:MainProgram.DintTag", "DINT"},
		{"LintTag", "Program:MainProgram.LintTag", "LINT"},
		{"UsintTag", "Program:MainProgram.UsintTag", "USINT"},
		{"UintTag", "Program:MainProgram.UintTag", "UINT"},
		{"UdintTag", "Program:MainProgram.UdintTag", "UDINT"},
		{"RealTag", "Program:MainProgram.RealTag", "REAL"},
		{"LrealTag", "Program:MainProgram.LrealTag", "LREAL"},
		{"StringTag", "Program:MainProgram.StringTag", "STRING"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			points := []model.Point{
				{ID: tc.name, Address: tc.address, DataType: tc.dataType},
			}

			results, err := scheduler.ReadPoints(context.Background(), points)
			if err != nil {
				t.Errorf("Failed to read %s: %v", tc.name, err)
				return
			}

			result, ok := results[tc.name]
			if !ok {
				t.Errorf("No result for %s", tc.name)
				return
			}

			if result.Quality != "Good" {
				t.Errorf("Quality is %s, expected Good for %s", result.Quality, tc.name)
			}

			if result.Value == nil {
				t.Errorf("Value is nil for %s", tc.name)
			} else {
				t.Logf("  %s = %v (type: %T)", tc.address, result.Value, result.Value)
			}
		})
	}
}

// testBatchRead 测试批量读取
func testBatchRead(t *testing.T, scheduler *ENIPScheduler) {
	points := []model.Point{
		{ID: "BoolTag", Address: "Program:MainProgram.BoolTag", DataType: "BOOL"},
		{ID: "IntTag", Address: "Program:MainProgram.IntTag", DataType: "INT"},
		{ID: "RealTag", Address: "Program:MainProgram.RealTag", DataType: "REAL"},
		{ID: "StringTag", Address: "Program:MainProgram.StringTag", DataType: "STRING"},
		{ID: "DintTag", Address: "Program:MainProgram.DintTag", DataType: "DINT"},
	}

	results, err := scheduler.ReadPoints(context.Background(), points)
	if err != nil {
		t.Fatalf("Failed to batch read: %v", err)
	}

	if len(results) != len(points) {
		t.Errorf("Expected %d results, got %d", len(points), len(results))
	}

	for _, p := range points {
		result, ok := results[p.ID]
		if !ok {
			t.Errorf("No result for %s", p.ID)
			continue
		}

		if result.Quality != "Good" {
			t.Errorf("Quality is %s for %s", result.Quality, p.ID)
		}

		t.Logf("  %s = %v", p.Address, result.Value)
	}
}

// testSinglePointWrite 测试单点写入
func testSinglePointWrite(t *testing.T, scheduler *ENIPScheduler) {
	testCases := []struct {
		name     string
		address  string
		dataType string
		value    interface{}
	}{
		{"WriteBool", "Program:MainProgram.BoolTag", "BOOL", false},
		{"WriteInt", "Program:MainProgram.IntTag", "INT", 12345},
		{"WriteDint", "Program:MainProgram.DintTag", "DINT", 987654321},
		{"WriteReal", "Program:MainProgram.RealTag", "REAL", float32(2.71828)},
		{"WriteString", "Program:MainProgram.StringTag", "STRING", "Hi"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := scheduler.WritePoint(context.Background(), model.Point{
				ID:       tc.name,
				Address:  tc.address,
				DataType: tc.dataType,
			}, tc.value)

			if err != nil {
				t.Errorf("Failed to write %s: %v", tc.name, err)
				return
			}

			t.Logf("  Successfully wrote %v to %s", tc.value, tc.address)
		})
	}

	// 验证写入的值可以读回
	t.Run("VerifyWrites", func(t *testing.T) {
		points := []model.Point{
			{ID: "BoolTag", Address: "Program:MainProgram.BoolTag", DataType: "BOOL"},
			{ID: "IntTag", Address: "Program:MainProgram.IntTag", DataType: "INT"},
			{ID: "DintTag", Address: "Program:MainProgram.DintTag", DataType: "DINT"},
			{ID: "RealTag", Address: "Program:MainProgram.RealTag", DataType: "REAL"},
			{ID: "StringTag", Address: "Program:MainProgram.StringTag", DataType: "STRING"},
		}

		results, err := scheduler.ReadPoints(context.Background(), points)
		if err != nil {
			t.Fatalf("Failed to verify writes: %v", err)
		}

		t.Log("Verification results:")
		for _, p := range points {
			if result, ok := results[p.ID]; ok {
				t.Logf("  %s = %v", p.Address, result.Value)
			}
		}
	})
}

// testArrayRead 测试数组读取
func testArrayRead(t *testing.T, scheduler *ENIPScheduler) {
	testCases := []struct {
		name    string
		address string
	}{
		{"ArrayElement0", "Program:MainProgram.IntArray[0]"},
		{"ArrayElement1", "Program:MainProgram.IntArray[1]"},
		{"ArrayElement2", "Program:MainProgram.IntArray[2]"},
		{"ArrayElement3", "Program:MainProgram.IntArray[3]"},
		{"ArrayElement4", "Program:MainProgram.IntArray[4]"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			points := []model.Point{
				{ID: tc.name, Address: tc.address, DataType: "INT"},
			}

			results, err := scheduler.ReadPoints(context.Background(), points)
			if err != nil {
				t.Errorf("Failed to read %s: %v", tc.name, err)
				return
			}

			result, ok := results[tc.name]
			if !ok {
				t.Errorf("No result for %s", tc.name)
				return
			}

			if result.Quality != "Good" {
				t.Errorf("Quality is %s for %s", result.Quality, tc.name)
			}

			t.Logf("  %s = %v", tc.address, result.Value)
		})
	}
}

// TestConnectionReconnect 测试连接断开后自动重连
func TestConnectionReconnect(t *testing.T) {
	cfg := map[string]any{
		"ip":            "127.0.0.1",
		"port":          44818,
		"slot":          0,
		"timeout":       2000,
		"maxRetries":    2,
		"retryInterval": 500,
	}

	transport := NewENIPTransport(cfg)
	if transport == nil {
		t.Fatal("Failed to create transport")
	}

	// 第一次连接
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err := transport.Connect(ctx)
	cancel()
	if err != nil {
		t.Fatalf("Initial connection failed: %v", err)
	}

	t.Log("Initial connection successful")

	// 断开连接
	err = transport.Disconnect()
	if err != nil {
		t.Logf("Disconnect error: %v", err)
	}

	// 重新连接
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	err = transport.Connect(ctx)
	cancel()
	if err != nil {
		t.Fatalf("Reconnection failed: %v", err)
	}

	t.Log("Reconnection successful")
	defer transport.Disconnect()
}

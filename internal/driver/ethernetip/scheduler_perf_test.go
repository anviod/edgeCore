package ethernetip

import (
	"fmt"
	"sync"
	"testing"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

// BenchmarkGroupTags 测试分组逻辑性能
func BenchmarkGroupTags(b *testing.B) {
	scheduler := NewENIPScheduler(nil, nil, map[string]any{
		"batch_read_max": 50,
	})

	for _, totalPoints := range []int{50, 100, 200, 500, 1000} {
		b.Run(fmt.Sprintf("GroupTags_%d_Points", totalPoints), func(b *testing.B) {
			b.StopTimer()

			// 准备测试数据
			var points []pointWithTag
			for i := 0; i < totalPoints; i++ {
				points = append(points, pointWithTag{
					Point: model.Point{ID: fmt.Sprintf("point%d", i)},
					Tag:   &ENIPTag{Name: fmt.Sprintf("Tag%d", i)},
				})
			}

			b.StartTimer()
			for i := 0; i < b.N; i++ {
				_ = scheduler.groupTags(points)
			}
		})
	}
}

// BenchmarkPointParsing 测试地址解析性能
func BenchmarkPointParsing(b *testing.B) {
	decoder := NewENIPDecoder()

	testAddresses := []string{
		"Program:Main.MyTag",
		"Program:Main.ArrayTag[10]",
		"Controller.TagName",
		"Program:Main.StructTag.Field",
	}

	b.StopTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		for _, addr := range testAddresses {
			_, _ = decoder.ParseAddress(addr)
		}
	}
}

// BenchmarkStatsIncrement 测试统计计数器性能
func BenchmarkStatsIncrement(b *testing.B) {
	scheduler := NewENIPScheduler(nil, nil, map[string]any{})

	b.StopTimer()
	b.StartTimer()

	for i := 0; i < b.N; i++ {
		scheduler.incTotal()
		scheduler.incSuccess()
	}
}

// BenchmarkTagGroupOperations 测试TagGroup基本操作（模拟）
func BenchmarkTagGroupOperations(b *testing.B) {
	for _, tagCount := range []int{10, 50, 100, 200} {
		b.Run(fmt.Sprintf("TagGroup_Add_%d", tagCount), func(b *testing.B) {
			b.StopTimer()

			tagGroup := newMockTagGroup()

			b.StartTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < tagCount; j++ {
					tagGroup.Add(&mockTag{name: fmt.Sprintf("Tag%d", j)})
				}
			}
		})
	}
}

// Mock structures for testing
type mockTag struct {
	name  string
	value interface{}
}

type mockTagGroup struct {
	tags []*mockTag
	mu   sync.Mutex
}

func newMockTagGroup() *mockTagGroup {
	return &mockTagGroup{
		tags: make([]*mockTag, 0),
	}
}

func (tg *mockTagGroup) Add(tag *mockTag) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	tg.tags = append(tg.tags, tag)
}

func (tg *mockTagGroup) Read() error {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	for _, tag := range tg.tags {
		tag.value = int32(1)
	}
	return nil
}

// TestSchedulerBatchOptimization 测试调度器批量分组逻辑
func TestSchedulerBatchOptimization(t *testing.T) {
	testCases := []struct {
		name           string
		totalPoints    int
		batchMax       int
		expectedGroups int
	}{
		{"30_points_batch_50", 30, 50, 1},
		{"70_points_batch_50", 70, 50, 2},
		{"120_points_batch_50", 120, 50, 3},
		{"50_points_batch_50", 50, 50, 1},
		{"100_points_batch_50", 100, 50, 2},
		{"100_points_batch_30", 100, 30, 4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := map[string]any{
				"batch_read_max": tc.batchMax,
				"min_interval":   0,
			}

			scheduler := NewENIPScheduler(nil, nil, cfg)

			var points []pointWithTag
			for i := 0; i < tc.totalPoints; i++ {
				points = append(points, pointWithTag{
					Point: model.Point{ID: fmt.Sprintf("point%d", i)},
					Tag:   &ENIPTag{Name: fmt.Sprintf("Tag%d", i)},
				})
			}

			groups := scheduler.groupTags(points)

			t.Logf("总点数: %d, 批量大小: %d, 分组数: %d", tc.totalPoints, tc.batchMax, len(groups))
			for i, g := range groups {
				t.Logf("  组%d: %d个Tag", i+1, len(g))
			}

			if len(groups) != tc.expectedGroups {
				t.Errorf("期望分组数 %d, 实际 %d", tc.expectedGroups, len(groups))
			}
		})
	}
}

// TestDecoderAddressParsing 测试地址解析功能
func TestDecoderAddressParsing(t *testing.T) {
	decoder := NewENIPDecoder()

	testCases := []struct {
		name     string
		address  string
		wantName string
		wantPath []string
		wantIdx  int
	}{
		{"simple_tag", "MyTag", "MyTag", []string{"MyTag"}, -1},
		{"program_tag", "Program:Main.MyTag", "Program:Main", []string{"Program:Main", "MyTag"}, -1},
		{"array_tag", "MyArray[10]", "MyArray", []string{"MyArray"}, 10},
		{"array_tag_zero", "MyArray[0]", "MyArray", []string{"MyArray"}, 0},
		{"program_array_tag", "Program:Main.MyArray[5]", "Program:Main", []string{"Program:Main", "MyArray[5]"}, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tag, err := decoder.ParseAddress(tc.address)
			if err != nil {
				t.Fatalf("解析失败: %v", err)
			}

			if tag.Name != tc.wantName {
				t.Errorf("期望名称 %q, 实际 %q", tc.wantName, tag.Name)
			}

			if len(tag.Path) != len(tc.wantPath) {
				t.Errorf("期望路径长度 %d, 实际 %d", len(tc.wantPath), len(tag.Path))
			} else {
				for i := range tag.Path {
					if tag.Path[i] != tc.wantPath[i] {
						t.Errorf("路径[%d]: 期望 %q, 实际 %q", i, tc.wantPath[i], tag.Path[i])
					}
				}
			}

			if tag.ArrayIndex != tc.wantIdx {
				t.Errorf("期望数组索引 %d, 实际 %d", tc.wantIdx, tag.ArrayIndex)
			}
		})
	}
}

// TestTransportConfiguration 测试传输层配置
func TestTransportConfiguration(t *testing.T) {
	testCases := []struct {
		name     string
		cfg      map[string]any
		wantPort int
		wantSlot int
	}{
		{"default_config", map[string]any{}, 44818, 0},
		{"custom_port", map[string]any{"port": 1234}, 1234, 0},
		{"custom_slot", map[string]any{"slot": 2}, 44818, 2},
		{"full_config", map[string]any{"port": 5000, "slot": 3}, 5000, 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transport := NewENIPTransport(tc.cfg)

			// 通过反射或导出方法检查配置
			if transport.port != tc.wantPort {
				t.Errorf("期望端口 %d, 实际 %d", tc.wantPort, transport.port)
			}
			if transport.slot != tc.wantSlot {
				t.Errorf("期望槽位 %d, 实际 %d", tc.wantSlot, transport.slot)
			}
		})
	}
}

// TestMetricsCollection 测试指标收集
func TestMetricsCollection(t *testing.T) {
	scheduler := NewENIPScheduler(nil, nil, map[string]any{})

	// 初始状态
	total, success, failure := scheduler.GetStats()
	if total != 0 || success != 0 || failure != 0 {
		t.Errorf("初始状态应为0, 实际: total=%d, success=%d, failure=%d", total, success, failure)
	}

	// 模拟操作
	scheduler.incTotal()
	scheduler.incSuccess()
	scheduler.incTotal()
	scheduler.incFailure()
	scheduler.incTotal()
	scheduler.incSuccess()

	total, success, failure = scheduler.GetStats()
	if total != 3 || success != 2 || failure != 1 {
		t.Errorf("期望: total=3, success=2, failure=1, 实际: total=%d, success=%d, failure=%d", total, success, failure)
	}
}

// TestConnectionManagerConfiguration 测试连接管理器配置
func TestConnectionManagerConfiguration(t *testing.T) {
	testCases := []struct {
		name      string
		cfg       map[string]any
		wantState driver.ConnState
	}{
		{"default", map[string]any{}, StateDisconnected},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transport := NewENIPTransport(tc.cfg)

			if transport.connMgr.GetState() != tc.wantState {
				t.Errorf("期望状态 %v, 实际 %v", tc.wantState, transport.connMgr.GetState())
			}
		})
	}
}

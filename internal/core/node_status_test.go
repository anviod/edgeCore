package core

import (
	"testing"
	"time"
)

// TestStateTransitions 测试状态机的转换逻辑
func TestStateTransitions(t *testing.T) {
	// 创建通信管理器
	manager := NewCommunicationManageTemplate()

	// 注册一个设备节点
	node := manager.RegisterNode("device1", "Test Device")

	// 验证初始状态为在线
	if node.Runtime.State != NodeStateOnline {
		t.Errorf("Expected initial state to be Online, got %v", node.Runtime.State)
	}

	// 测试ShouldCollect - 在线状态应该允许采集
	if !manager.ShouldCollect(node) {
		t.Error("Online device should be allowed to collect")
	}

	// 模拟3次失败 -> 应该进入Unstable状态
	for i := 0; i < 3; i++ {
		manager.onCollectFail(node)
	}

	if node.Runtime.State != NodeStateUnstable {
		t.Errorf("After 3 failures, expected Unstable state, got %v", node.Runtime.State)
	}

	if node.Runtime.FailCount != 3 {
		t.Errorf("Expected fail count 3, got %d", node.Runtime.FailCount)
	}

	// 模拟更多失败 -> 应该进入Quarantine状态
	for i := 0; i < 7; i++ {
		manager.onCollectFail(node)
	}

	if node.Runtime.State != NodeStateQuarantine {
		t.Errorf("After 10 failures, expected Quarantine state, got %v", node.Runtime.State)
	}

	// 此时ShouldCollect应该返回false（因为退避时间未过）
	if manager.ShouldCollect(node) {
		t.Error("Quarantine device should be skipped during backoff period")
	}

	// 模拟一次成功 -> 应该恢复到Online状态
	manager.onCollectSuccess(node)

	if node.Runtime.State != NodeStateOnline {
		t.Errorf("After 1 success, expected Online state, got %v", node.Runtime.State)
	}

	if node.Runtime.FailCount != 0 {
		t.Errorf("After success, expected fail count 0, got %d", node.Runtime.FailCount)
	}
}

// TestFinalizeCollect 测试最终裁决函数
func TestFinalizeCollect(t *testing.T) {
	manager := NewCommunicationManageTemplate()
	node := manager.RegisterNode("device1", "Test Device")

	// 测试场景1: panic发生 -> 应判定为失败
	ctx := &CollectContext{
		TotalCmd:   10,
		SuccessCmd: 8,
		FailCmd:    2,
		PanicOccur: true,
	}
	manager.finalizeCollect(node, ctx)
	if node.Runtime.FailCount != 1 {
		t.Errorf("After panic, expected fail count 1, got %d", node.Runtime.FailCount)
	}

	// 重置状态
	node.Runtime.State = NodeStateOnline
	node.Runtime.FailCount = 0

	// 测试场景2: 无交互 -> 应判定为失败
	ctx = &CollectContext{
		TotalCmd:   0,
		SuccessCmd: 0,
		FailCmd:    0,
		PanicOccur: false,
	}
	manager.finalizeCollect(node, ctx)
	if node.Runtime.FailCount != 1 {
		t.Errorf("After no interaction, expected fail count 1, got %d", node.Runtime.FailCount)
	}

	// 重置状态
	node.Runtime.State = NodeStateOnline
	node.Runtime.FailCount = 0

	// 测试场景3: 成功率 < 30% -> 应判定为失败
	ctx = &CollectContext{
		TotalCmd:   10,
		SuccessCmd: 2, // 20%成功率
		FailCmd:    8,
		PanicOccur: false,
	}
	manager.finalizeCollect(node, ctx)
	if node.Runtime.FailCount != 1 {
		t.Errorf("After low success rate, expected fail count 1, got %d", node.Runtime.FailCount)
	}

	// 重置状态
	node.Runtime.State = NodeStateOnline
	node.Runtime.FailCount = 0

	// 测试场景4: 成功率 >= 30% -> 应判定为成功
	ctx = &CollectContext{
		TotalCmd:   10,
		SuccessCmd: 5, // 50%成功率
		FailCmd:    5,
		PanicOccur: false,
	}
	manager.finalizeCollect(node, ctx)
	if node.Runtime.State != NodeStateOnline || node.Runtime.FailCount != 0 {
		t.Errorf("After sufficient success rate, expected Online state and fail count 0, got state %v and fail count %d", node.Runtime.State, node.Runtime.FailCount)
	}
}

// TestBackoffMechanism 测试退避机制
func TestBackoffMechanism(t *testing.T) {
	manager := NewCommunicationManageTemplate()
	node := manager.RegisterNode("device1", "Test Device")

	// 模拟10次失败，进入Quarantine状态
	for i := 0; i < 10; i++ {
		manager.onCollectFail(node)
	}

	if node.Runtime.State != NodeStateQuarantine {
		t.Errorf("Expected Quarantine state, got %v", node.Runtime.State)
	}

	// 记录下次重试时间
	nextRetryTime := node.Runtime.NextRetryTime

	// 应该有一个合理的退避时间（大于当前时间）
	if !nextRetryTime.After(time.Now()) {
		t.Error("Next retry time should be in the future")
	}

	// 验证退避时间不超过5分钟
	maxBackoff := 5 * time.Minute
	expectedMaxTime := time.Now().Add(maxBackoff)
	if nextRetryTime.After(expectedMaxTime.Add(100 * time.Millisecond)) {
		t.Errorf("Backoff time should not exceed 5 minutes")
	}
}

// TestUnregisterNode 测试节点注销
func TestUnregisterNode(t *testing.T) {
	manager := NewCommunicationManageTemplate()
	manager.RegisterNode("ch1", "Channel 1")
	manager.RegisterNode("dev1", "Device 1")
	manager.RegisterNode("dev2", "Device 2")

	if manager.GetNode("dev1") == nil {
		t.Fatal("expected dev1 node to be registered")
	}

	// 注销单个设备节点
	manager.UnregisterNode("dev1")
	if manager.GetNode("dev1") != nil {
		t.Error("expected dev1 node to be removed")
	}
	if manager.GetNode("dev2") == nil {
		t.Error("expected dev2 node to remain")
	}
	if manager.GetNode("ch1") == nil {
		t.Error("expected ch1 node to remain")
	}

	// 注销不存在的节点不应 panic
	manager.UnregisterNode("nonexistent")
}

// TestConcurrentAccess 测试并发访问的安全性
func TestConcurrentAccess(t *testing.T) {
	manager := NewCommunicationManageTemplate()
	node := manager.RegisterNode("device1", "Test Device")

	// 并发操作
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			manager.ShouldCollect(node)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			manager.onCollectSuccess(node)
		}
		done <- true
	}()

	<-done
	<-done
	// 如果没有死锁或数据竞争，测试通过
	t.Log("Concurrent access test passed")
}

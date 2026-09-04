package core

import (
	"sync"
	"time"
)

// ==================== 设备节点模板 ====================
// DeviceNodeTemplate 代表一个设备节点，包含设备信息和运行时状态
type DeviceNodeTemplate struct {
	mu       sync.RWMutex
	DeviceID string            // 设备ID
	Name     string            // 设备名称
	Runtime  *NodeRuntimeState // 运行时状态
}

// ==================== 通信管理模板 ====================
// CommunicationManageTemplate 管理设备采集的状态机和重试策略
type StateChangeCallback func(deviceID string, oldState, newState NodeState)

type CommunicationManageTemplate struct {
	nodes         map[string]*DeviceNodeTemplate // 管理的设备节点
	mu            sync.RWMutex                   // 读写锁，保护nodes访问
	OnStateChange StateChangeCallback            // 状态变更回调
}

func (c *CommunicationManageTemplate) finalizeCollect(node *DeviceNodeTemplate, ctx *CollectContext) {
	c.FinalizeCollect(node, ctx)
}

// NewCommunicationManageTemplate 创建新的通信管理器
func NewCommunicationManageTemplate() *CommunicationManageTemplate {
	return &CommunicationManageTemplate{
		nodes: make(map[string]*DeviceNodeTemplate),
	}
}

// RegisterNode 注册一个新的设备节点
func (c *CommunicationManageTemplate) RegisterNode(deviceID, name string) *DeviceNodeTemplate {
	c.mu.Lock()
	defer c.mu.Unlock()

	node := &DeviceNodeTemplate{
		DeviceID: deviceID,
		Name:     name,
		Runtime: &NodeRuntimeState{
			State: NodeStateOnline,
		},
	}
	c.nodes[deviceID] = node
	return node
}

// GetNode 获取指定的设备节点
func (c *CommunicationManageTemplate) GetNode(deviceID string) *DeviceNodeTemplate {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nodes[deviceID]
}

// UnregisterNode 注销指定设备/通道节点，释放内存中的运行时状态。
// 在删除设备或通道时调用，避免残留节点长期积累。
func (c *CommunicationManageTemplate) UnregisterNode(nodeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.nodes, nodeID)
}

// ==================== 节点运行状态定义 ====================
// NodeState 定义设备节点的运行状态
type NodeState int

// 设备节点状态枚举
const (
	NodeStateOnline     NodeState = iota // 0 在线状态：设备正常通信
	NodeStateUnstable                    // 1 不稳定状态：设备通信时好时坏
	NodeStateOffline                     // 2 离线状态：设备暂时无法连接
	NodeStateQuarantine                  // 3 隔离状态：设备持续故障，需要延长退避时间
)

// ==================== 节点运行时状态结构 ====================
// NodeRuntimeState 存储设备节点的运行时状态信息
type NodeRuntimeState struct {
	FailCount     int       // 连续失败次数
	SuccessCount  int       // 连续成功次数
	LastSuccess   time.Time // 最后一次成功时间
	LastFailTime  time.Time // 最后一次失败时间
	NextRetryTime time.Time // 下一次重试时间（用于退避机制）
	State         NodeState // 当前节点状态
}

// ==================== 采集上下文结构 ====================
// CollectContext 用于记录单次采集过程中的统计信息
type CollectContext struct {
	TotalCmd   int  // 总命令数：本次采集执行的总命令数量
	SuccessCmd int  // 成功命令数：成功执行的命令数量
	FailCmd    int  // 失败命令数：执行失败的命令数量
	PanicOccur bool // 是否发生panic：标记采集过程中是否出现异常
}

// MarkFail 记录一次失败命令
// 调用此方法会增加失败计数，用于统计采集成功率
func (ctx *CollectContext) MarkFail() {
	ctx.FailCmd++
}

// MarkSuccess 记录一次成功命令
// 调用此方法会增加成功计数，用于统计采集成功率
func (ctx *CollectContext) MarkSuccess() {
	ctx.SuccessCmd++
}

// ==================== 状态机核心方法 ====================

// ShouldCollect 判断是否允许对指定节点进行本轮采集
// 根据节点当前状态和退避时间决定是否执行采集
//
// 参数:
//   - node: 目标设备节点
//
// 返回值:
//   - bool: true表示允许采集，false表示跳过采集
//
// 策略说明:
//   - Online/Unstable状态: 始终允许采集
//   - Offline/Quarantine状态: 只有在退避时间过后才允许采集
func (c *CommunicationManageTemplate) ShouldCollect(node *DeviceNodeTemplate) bool {
	node.mu.RLock()
	defer node.mu.RUnlock()
	now := time.Now()

	switch node.Runtime.State {
	case NodeStateQuarantine, NodeStateOffline:
		// 对于隔离和离线状态，检查是否已过退避时间
		return now.After(node.Runtime.NextRetryTime)
	default:
		// 其他状态（在线/不稳定）允许采集
		return true
	}
}

// onCollectFail 处理采集失败的情况
// 更新节点失败统计，根据失败次数调整节点状态和退避策略
//
// 参数:
//   - node: 采集失败的设备节点
//
// 退避策略:
//   - 3-9次失败: 进入不稳定状态，5秒后重试
//   - 10次以上失败: 进入隔离状态，指数退避（最长5分钟）
//
// 设计原则:
//  1. 失败次数越多，退避时间越长
//  2. 隔离状态避免频繁重试浪费资源
//  3. 失败后重置成功计数
func (c *CommunicationManageTemplate) onCollectFail(node *DeviceNodeTemplate) {
	node.mu.Lock()
	defer node.mu.Unlock()
	// 记录旧状态
	oldState := node.Runtime.State

	// 更新失败统计
	node.Runtime.FailCount++               // 增加连续失败次数
	node.Runtime.SuccessCount = 0          // 重置连续成功次数
	node.Runtime.LastFailTime = time.Now() // 记录失败时间

	// 根据失败次数调整节点状态
	switch {
	case node.Runtime.FailCount >= 1 && node.Runtime.FailCount < 10:
		node.Runtime.State = NodeStateUnstable

	case node.Runtime.FailCount >= 10:
		node.Runtime.State = NodeStateQuarantine
		backoff := time.Duration(node.Runtime.FailCount) * time.Second
		if backoff > 5*time.Minute {
			backoff = 5 * time.Minute
		}
		node.Runtime.NextRetryTime = time.Now().Add(backoff)
	}

	// 触发状态变更回调
	if oldState != node.Runtime.State && c.OnStateChange != nil {
		c.OnStateChange(node.DeviceID, oldState, node.Runtime.State)
	}
}

// onCollectSuccess 处理采集成功的情况
// 更新节点成功统计，根据成功次数恢复节点状态
//
// 参数:
//   - node: 采集成功的设备节点
//
// 恢复策略:
//   - 1次成功即可恢复为在线状态
//   - 成功时重置失败计数
//
// 设计原则:
//  1. 成功后立即重置失败计数，给予设备重新证明自己的机会
//  2. 降低恢复门槛（只需1次成功），避免设备长期处于不良状态
//  3. 累计成功次数用于监控设备稳定性
func (c *CommunicationManageTemplate) onCollectSuccess(node *DeviceNodeTemplate) {
	node.mu.Lock()
	defer node.mu.Unlock()
	// 记录旧状态
	oldState := node.Runtime.State

	// 更新成功统计
	node.Runtime.SuccessCount++ // 增加连续成功次数
	node.Runtime.FailCount = 0  // 重置连续失败次数

	// 只需1次成功即可恢复在线状态
	if node.Runtime.SuccessCount >= 1 {
		node.Runtime.State = NodeStateOnline
	}

	// 触发状态变更回调
	if oldState != node.Runtime.State && c.OnStateChange != nil {
		c.OnStateChange(node.DeviceID, oldState, node.Runtime.State)
	}
}

// MarkOffline 将设备标记为离线（例如通道链路断开时批量更新）。
func (c *CommunicationManageTemplate) MarkOffline(deviceID string) {
	c.mu.RLock()
	node, ok := c.nodes[deviceID]
	c.mu.RUnlock()
	if !ok || node == nil {
		return
	}

	oldState := node.Runtime.State
	node.Runtime.State = NodeStateOffline
	node.Runtime.SuccessCount = 0
	node.Runtime.LastFailTime = time.Now()

	if oldState != node.Runtime.State && c.OnStateChange != nil {
		c.OnStateChange(deviceID, oldState, node.Runtime.State)
	}
}

// FinalizeCollect 最终裁决函数
// 根据采集上下文统计信息，决定本次采集的整体结果并更新节点状态
//
// 参数:
//   - node: 设备节点
//   - ctx: 采集上下文，包含本次采集的统计信息
//
// 裁决逻辑:
//  1. 发生panic -> 直接判定为失败
//  2. 无任何命令交互 -> 判定为失败
//  3. 成功率 ≥ 30% -> 判定为成功
//  4. 成功率 < 30% -> 判定为失败
//
// 设计原则:
//  1. panic具有最高优先级，直接否决
//  2. 允许部分失败（30%成功率即可），适应工业现场不稳定性
//  3. 无交互视为最严重失败
func (c *CommunicationManageTemplate) FinalizeCollect(node *DeviceNodeTemplate, ctx *CollectContext) {
	// 规则1: panic一票否决
	if ctx.PanicOccur {
		c.onCollectFail(node)
		return
	}

	// 计算总命令数
	total := ctx.SuccessCmd + ctx.FailCmd

	// 规则2: 无任何有效交互视为失败
	if total == 0 {
		c.onCollectFail(node)
		return
	}

	// 计算本次采集的成功率
	successRatio := float64(ctx.SuccessCmd) / float64(total)
	const MinSuccessRatio = 0.3 // 最低成功率要求：30%

	// 规则3/4: 根据成功率决定最终结果
	if successRatio >= MinSuccessRatio {
		c.onCollectSuccess(node) // 成功率达标，判定为成功
	} else if ctx.SuccessCmd == 0 && ctx.FailCmd > 0 {
		c.onCollectUnreachable(node) // 全部失败：设备不可达，立即离线
	} else {
		c.onCollectFail(node) // 部分失败：降级为不稳定
	}
}

// onCollectUnreachable handles a collect where no points succeeded (transport error
// or all Bad quality). Marks the device offline immediately instead of waiting for
// repeated unstable cycles.
func (c *CommunicationManageTemplate) onCollectUnreachable(node *DeviceNodeTemplate) {
	oldState := node.Runtime.State

	node.Runtime.FailCount++
	node.Runtime.SuccessCount = 0
	node.Runtime.LastFailTime = time.Now()
	node.Runtime.State = NodeStateOffline

	if oldState != node.Runtime.State && c.OnStateChange != nil {
		c.OnStateChange(node.DeviceID, oldState, node.Runtime.State)
	}
}

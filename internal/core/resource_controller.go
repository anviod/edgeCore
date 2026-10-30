package core

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type ResourceLimits struct {
	GoroutineLimit  int
	FDLimit         int
	ConnectionLimit int
	QueueLimit      int
}

type ResourceController struct {
	limits          ResourceLimits
	goroutineCount  int32
	connectionCount int32
	stopCh          chan struct{}
}

// int32Limit 将配置的 int 上限饱和钳制到 int32 域，
// 避免与 int32 原子计数器比较时溢出回绕（gosec G115）。
func int32Limit(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v)
}

func NewResourceController(limits ResourceLimits) *ResourceController {
	return &ResourceController{
		limits: limits,
		stopCh: make(chan struct{}),
	}
}

func (rc *ResourceController) CanExecute() bool {
	if atomic.LoadInt32(&rc.goroutineCount) >= int32Limit(rc.limits.GoroutineLimit) {
		return false
	}

	if atomic.LoadInt32(&rc.connectionCount) >= int32Limit(rc.limits.ConnectionLimit) {
		return false
	}

	return true
}

func (rc *ResourceController) Acquire() {
	atomic.AddInt32(&rc.goroutineCount, 1)
}

func (rc *ResourceController) Release() {
	atomic.AddInt32(&rc.goroutineCount, -1)
}

func (rc *ResourceController) AcquireConnection() bool {
	count := atomic.AddInt32(&rc.connectionCount, 1)
	if count > int32Limit(rc.limits.ConnectionLimit) {
		atomic.AddInt32(&rc.connectionCount, -1)
		return false
	}
	return true
}

func (rc *ResourceController) ReleaseConnection() {
	atomic.AddInt32(&rc.connectionCount, -1)
}

func (rc *ResourceController) Monitor(wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			goroutines := atomic.LoadInt32(&rc.goroutineCount)
			connections := atomic.LoadInt32(&rc.connectionCount)

			if goroutines > int32Limit(rc.limits.GoroutineLimit)*9/10 {
				zap.L().Warn("[Resource] goroutine接近上限",
					zap.Int32("current", goroutines),
					zap.Int("limit", rc.limits.GoroutineLimit),
				)
			}

			if connections > int32Limit(rc.limits.ConnectionLimit)*9/10 {
				zap.L().Warn("[Resource] 连接数接近上限",
					zap.Int32("current", connections),
					zap.Int("limit", rc.limits.ConnectionLimit),
				)
			}
		case <-rc.stopCh:
			return
		}
	}
}

func (rc *ResourceController) Stop() {
	close(rc.stopCh)
}

func (rc *ResourceController) GetGoroutineCount() int32 {
	return atomic.LoadInt32(&rc.goroutineCount)
}

func (rc *ResourceController) GetConnectionCount() int32 {
	return atomic.LoadInt32(&rc.connectionCount)
}

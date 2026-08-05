package sync

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
)

func TestTakeoverManager_New(t *testing.T) {
	tm := NewTakeoverManager()
	assert.NotNil(t, tm)
}

func TestTakeoverManager_TryLock_Acquire(t *testing.T) {
	tm := NewTakeoverManager()
	owner := peer.ID("owner-1")

	ok := tm.TryLock("device-1", owner, 5*time.Minute)
	assert.True(t, ok)

	lock, exists := tm.GetLockStatus("device-1")
	assert.True(t, exists)
	assert.Equal(t, owner, lock.Owner)
	assert.Equal(t, "device-1", lock.DeviceKey)
}

func TestTakeoverManager_TryLock_DifferentOwnerBlocked(t *testing.T) {
	tm := NewTakeoverManager()
	owner1 := peer.ID("owner-1")
	owner2 := peer.ID("owner-2")

	assert.True(t, tm.TryLock("device-1", owner1, 5*time.Minute))
	assert.False(t, tm.TryLock("device-1", owner2, 5*time.Minute))

	lock, exists := tm.GetLockStatus("device-1")
	assert.True(t, exists)
	assert.Equal(t, owner1, lock.Owner)
}

func TestTakeoverManager_TryLock_SameOwnerCanReacquire(t *testing.T) {
	tm := NewTakeoverManager()
	owner := peer.ID("owner-1")

	assert.True(t, tm.TryLock("device-1", owner, 5*time.Minute))
	assert.True(t, tm.TryLock("device-1", owner, 10*time.Minute))

	lock, _ := tm.GetLockStatus("device-1")
	assert.Equal(t, 10*time.Minute, lock.TTL)
}

func TestTakeoverManager_TryLock_ExpiredLock(t *testing.T) {
	tm := NewTakeoverManager()
	owner1 := peer.ID("owner-1")
	owner2 := peer.ID("owner-2")

	assert.True(t, tm.TryLock("device-1", owner1, 1*time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	assert.True(t, tm.TryLock("device-1", owner2, 5*time.Minute))
	lock, _ := tm.GetLockStatus("device-1")
	assert.Equal(t, owner2, lock.Owner)
}

func TestTakeoverManager_GetLockStatus_NotFound(t *testing.T) {
	tm := NewTakeoverManager()
	lock, exists := tm.GetLockStatus("missing")
	assert.False(t, exists)
	assert.Nil(t, lock)
}

func TestTakeoverManager_GetLockStatus_Expired(t *testing.T) {
	tm := NewTakeoverManager()
	owner := peer.ID("owner-1")

	assert.True(t, tm.TryLock("device-1", owner, 1*time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	lock, exists := tm.GetLockStatus("device-1")
	assert.False(t, exists)
	assert.Nil(t, lock)
}

func TestTakeoverManager_ReleaseLock(t *testing.T) {
	tm := NewTakeoverManager()
	owner := peer.ID("owner-1")

	assert.True(t, tm.TryLock("device-1", owner, 5*time.Minute))
	tm.ReleaseLock("device-1")

	_, exists := tm.GetLockStatus("device-1")
	assert.False(t, exists)
}

func TestTakeoverManager_CleanupExpiredLocks(t *testing.T) {
	tm := NewTakeoverManager()
	owner := peer.ID("owner-1")

	assert.True(t, tm.TryLock("short", owner, 1*time.Millisecond))
	assert.True(t, tm.TryLock("long", owner, 5*time.Minute))

	time.Sleep(5 * time.Millisecond)
	tm.CleanupExpiredLocks()

	_, exists := tm.GetLockStatus("short")
	assert.False(t, exists)

	_, exists = tm.GetLockStatus("long")
	assert.True(t, exists)
}

func TestTakeoverManager_RecordEvent(t *testing.T) {
	tm := NewTakeoverManager()
	event := &TakeoverEvent{
		ID:        "evt-1",
		DeviceKey: "device-1",
		Stage:     TakeoverStageHello,
		Status:    "ok",
	}

	tm.RecordEvent(event)
	events := tm.GetEvents("device-1")
	assert.Len(t, events, 1)
	assert.Equal(t, "evt-1", events[0].ID)
	assert.False(t, events[0].Timestamp.IsZero())
}

func TestTakeoverManager_RecordEvent_Nil(t *testing.T) {
	tm := NewTakeoverManager()
	tm.RecordEvent(nil)
	assert.Empty(t, tm.GetEvents(""))
}

func TestTakeoverManager_GetEvents_All(t *testing.T) {
	tm := NewTakeoverManager()
	tm.RecordEvent(&TakeoverEvent{DeviceKey: "device-1", Stage: TakeoverStageHello})
	tm.RecordEvent(&TakeoverEvent{DeviceKey: "device-2", Stage: TakeoverStageTakeover})

	all := tm.GetEvents("")
	assert.Len(t, all, 2)

	device1 := tm.GetEvents("device-1")
	assert.Len(t, device1, 1)
	assert.Equal(t, TakeoverStageHello, device1[0].Stage)
}

func TestTakeoverManager_GetEvents_Ordering(t *testing.T) {
	tm := NewTakeoverManager()
	tm.RecordEvent(&TakeoverEvent{DeviceKey: "device-1", ID: "first"})
	tm.RecordEvent(&TakeoverEvent{DeviceKey: "device-1", ID: "second"})

	events := tm.GetEvents("device-1")
	assert.Len(t, events, 2)
	assert.Equal(t, "second", events[0].ID)
	assert.Equal(t, "first", events[1].ID)
}

package sync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSyncManager_Create(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-sync", 4002)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)

	if mgr != nil {
		mgr.Stop()
	}
}

func TestSyncManager_StartStop(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-sync-startstop", 4003)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)

	err = mgr.Start()
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	mgr.Stop()
}

func TestSyncManager_PutGetConfig(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-config", 4004)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Stop()

	err = mgr.Start()
	assert.NoError(t, err)

	testKey := "test.channel.modbus.device.plc-01"
	testValue := []byte(`{"name": "PLC-01", "protocol": "modbus-tcp"}`)

	err = mgr.PutConfig(testKey, testValue, "device.plc-01")
	assert.NoError(t, err)

	rec, ok := mgr.GetConfig(testKey)
	assert.True(t, ok)
	assert.Equal(t, testKey, rec.Key)
	assert.Equal(t, testValue, rec.Value)
	assert.GreaterOrEqual(t, rec.Version, uint64(1))
}

func TestSyncManager_CompareSnapshots(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-snapshot", 4005)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Stop()

	err = mgr.Start()
	assert.NoError(t, err)

	nodeID := mgr.GetPeerIDString()

	snapshot := &NodeSnapshot{
		NodeID:     nodeID,
		NodeName:   "test-node",
		CapturedAt: time.Now(),
	}

	mgr.mu.Lock()
	mgr.snapshots[nodeID] = snapshot
	mgr.mu.Unlock()

	found, ok := mgr.GetSnapshot(nodeID)
	assert.True(t, ok)
	assert.Equal(t, nodeID, found.NodeID)
}

func TestSyncManager_GetStatus(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-status", 4006)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Stop()

	err = mgr.Start()
	assert.NoError(t, err)

	status := mgr.GetStatus()
	assert.Equal(t, "running", status["status"])
	assert.NotEmpty(t, status["node_id"])
	assert.NotNil(t, status["role"])
}

func TestSyncManager_TriggerSync(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-trigger", 4007)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Stop()

	err = mgr.Start()
	assert.NoError(t, err)

	err = mgr.TriggerSync("full")
	assert.NoError(t, err)

	err = mgr.TriggerSync("delta")
	assert.NoError(t, err)

	err = mgr.TriggerSync("incremental")
	assert.NoError(t, err)

	err = mgr.TriggerSync("invalid")
	assert.Error(t, err)
}

func TestSyncManager_CheckConsistency(t *testing.T) {
	ctx := context.Background()
	mgr, err := NewSyncManager(ctx, "/tmp/edgeCore-test-consistency", 4008)
	assert.NoError(t, err)
	assert.NotNil(t, mgr)
	defer mgr.Stop()

	err = mgr.Start()
	assert.NoError(t, err)

	report, err := mgr.CheckConsistency()
	assert.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, "consistent", report.OverallStatus)
}

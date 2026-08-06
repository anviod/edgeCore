package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestStoreForwardManager_HandleBatch(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	m := NewStoreForwardManager(store, StoreForwardPolicy{
		MaxSouthboundRecords: 10,
		MaxNorthboundPerID:   5,
	})

	batch := []model.Value{
		{ChannelID: "ch1", DeviceID: "dev1", PointID: "temp", Value: 25.5, Quality: "Good"},
		{ChannelID: "ch1", DeviceID: "dev1", PointID: "hum", Value: 60, Quality: "Good"},
	}
	m.HandleBatch(batch)

	count := 0
	_ = store.LoadAll(storage.BucketDataCache, func(k, _ []byte) error {
		if len(k) > len(storeForwardSouthKey) && string(k[:len(storeForwardSouthKey)]) == storeForwardSouthKey {
			count++
		}
		return nil
	})
	if count != 2 {
		t.Fatalf("HandleBatch stored %d records, want 2", count)
	}
}

func TestStoreForwardManager_CacheNorthbound(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	m := NewStoreForwardManager(store, StoreForwardPolicy{MaxNorthboundPerID: 2})
	payload := []byte(`{"topic":"test"}`)
	if err := m.CacheNorthbound("mqtt-1", payload); err != nil {
		t.Fatalf("CacheNorthbound: %v", err)
	}
}

func TestStoreForwardManager_ReplayEmpty(t *testing.T) {
	tmpDir := testOutputDir(t)
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("storage: %v", err)
	}
	defer store.Close()

	m := NewStoreForwardManager(store, StoreForwardPolicy{})
	if got := m.ReplaySouthbound(0); got != nil {
		t.Fatalf("limit 0 = %v, want nil", got)
	}
	if got := m.ReplaySouthbound(5); len(got) != 0 {
		t.Fatalf("empty store replay = %d, want 0", len(got))
	}
}

func TestStoreForwardManager_DefaultPolicy(t *testing.T) {
	m := NewStoreForwardManager(nil, StoreForwardPolicy{})
	if m.policy.MaxSouthboundRecords != 10000 {
		t.Fatalf("default MaxSouthboundRecords = %d, want 10000", m.policy.MaxSouthboundRecords)
	}
	if m.policy.MaxNorthboundPerID != 1000 {
		t.Fatalf("default MaxNorthboundPerID = %d, want 1000", m.policy.MaxNorthboundPerID)
	}
}

func TestStoreForwardManager_NilSafe(t *testing.T) {
	var m *StoreForwardManager
	m.HandleBatch([]model.Value{{ChannelID: "c", DeviceID: "d", PointID: "p", Value: 1}})
	if got := m.ReplaySouthbound(5); got != nil {
		t.Fatalf("nil manager ReplaySouthbound = %v, want nil", got)
	}
	if err := m.CacheNorthbound("id", []byte("x")); err != nil {
		t.Fatalf("nil manager CacheNorthbound: %v", err)
	}

	m = NewStoreForwardManager(nil, StoreForwardPolicy{})
	m.SetStorage(nil)
	m.HandleBatch([]model.Value{{ChannelID: "c", DeviceID: "d", PointID: "p", Value: 1}})
}

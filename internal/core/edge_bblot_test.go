package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"
)

func TestBblotPersistence_ErrorOnly(t *testing.T) {
	tmpDir := testOutputDir(t)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	pipeline := NewDataPipeline(10)
	ecm := NewEdgeComputeManager(pipeline, store, func(rules []model.EdgeRule) error {
		return nil
	})

	pipeline.Start()
	ecm.Start()
	defer ecm.Stop()

	ruleID := "rule-bblot-err"
	rule := model.EdgeRule{
		ID:     ruleID,
		Name:   "TestBblotError",
		Type:   "threshold",
		Enable: true,
		Sources: []model.RuleSource{
			{Alias: "t1", ChannelID: "ch1", DeviceID: "dev1", PointID: "p1"},
		},
		Condition: "t1 >>> 0",
	}

	ecm.LoadRules([]model.EdgeRule{rule})

	minuteKey := time.Now().Format("2006-01-02 15:04")
	expectedKey := fmt.Sprintf("%s_%s", ruleID, minuteKey)

	pipeline.Push(model.Value{
		ChannelID: "ch1", DeviceID: "dev1", PointID: "p1", Value: 15, TS: time.Now(),
	})

	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		store.LoadAll("bblot", func(k, v []byte) error {
			if string(k) == expectedKey {
				found = true
			}
			return nil
		})
		if found {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !found {
		t.Errorf("Expected error bblot record for key %s not found", expectedKey)
	}
}

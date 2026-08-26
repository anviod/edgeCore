package core

import (
	"strings"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestNorthboundManager_UpsertSparkplugB_SavesWhenBrokerUnreachable(t *testing.T) {
	var saved model.NorthboundConfig
	nm := NewNorthboundManager(model.NorthboundConfig{}, nil, nil, nil, func(cfg model.NorthboundConfig) error {
		saved = cfg
		return nil
	})

	cfg := model.SparkplugBConfig{
		ID:       "nb-spb-1",
		Name:     "Test Sparkplug B",
		Enable:   true,
		Broker:   "127.0.0.1",
		Port:     18884,
		ClientID: "spb-test-client",
		GroupID:  "group1",
		NodeID:   "node1",
	}

	warning, err := nm.UpsertSparkplugBConfig(cfg)
	if err != nil {
		t.Fatalf("UpsertSparkplugBConfig should not fail when broker is unreachable: %v", err)
	}
	if warning == "" {
		t.Fatal("expected connector start warning when broker is unreachable")
	}
	if !strings.Contains(warning, "配置已保存") {
		t.Fatalf("unexpected warning: %q", warning)
	}

	if len(saved.SparkplugB) != 1 {
		t.Fatalf("expected 1 Sparkplug B config saved, got %d", len(saved.SparkplugB))
	}
	if saved.SparkplugB[0].ID != "nb-spb-1" {
		t.Errorf("expected id nb-spb-1, got %s", saved.SparkplugB[0].ID)
	}
}

package core

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestPopulateEdgeFailureScope(t *testing.T) {
	rec := model.EdgeFailureRecord{
		Phase: "dispatch",
		Error: "pool full",
		Context: map[string]any{
			"channel_id": "ch1",
			"device_id":  "dev1",
		},
	}
	populateEdgeFailureScope(&rec, model.Value{})
	if rec.Category != model.LogCategoryEdgeCompute {
		t.Fatalf("category = %q, want %q", rec.Category, model.LogCategoryEdgeCompute)
	}
	if rec.ChannelID != "ch1" || rec.DeviceID != "dev1" {
		t.Fatalf("scope = (%q, %q), want (ch1, dev1)", rec.ChannelID, rec.DeviceID)
	}

	rec = model.EdgeFailureRecord{Phase: "evaluate", Error: "bad expr"}
	populateEdgeFailureScope(&rec, model.Value{ChannelID: "ch2", DeviceID: "dev2"})
	if rec.ChannelID != "ch2" || rec.DeviceID != "dev2" {
		t.Fatalf("scope from value = (%q, %q), want (ch2, dev2)", rec.ChannelID, rec.DeviceID)
	}
}

func TestMatchesEdgeLogFilter(t *testing.T) {
	snap := model.RuleMinuteSnapshot{
		RuleID:    "rule-1",
		Category:  model.LogCategoryEdgeCompute,
		ChannelID: "ch1",
		DeviceID:  "dev1",
	}
	filter := EdgeLogFilter{Category: model.LogCategoryEdgeCompute, ChannelID: "ch1", DeviceID: "dev1"}
	if !MatchesEdgeLogFilter(snap, filter) {
		t.Fatal("expected snapshot to match filter")
	}
	if MatchesEdgeLogFilter(snap, EdgeLogFilter{ChannelID: "ch2"}) {
		t.Fatal("expected channel mismatch to fail")
	}
	if MatchesEdgeLogFilter(model.RuleMinuteSnapshot{RuleID: "rule-1", ErrorMessage: "x"}, EdgeLogFilter{Category: model.LogCategorySystem}) {
		t.Fatal("expected default edge category to reject system filter")
	}
}

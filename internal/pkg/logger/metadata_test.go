package logger

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap/zapcore"
)

func TestInferCategoryFromCaller(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"internal/core/scan_engine.go", model.LogCategorySouthbound},
		{"internal/core/channel_manager.go", model.LogCategorySouthbound},
		{"internal/driver/modbus/modbus.go", model.LogCategorySouthbound},
		{"internal/core/edge_compute_manager.go", model.LogCategoryEdgeCompute},
		{"internal/core/edge_event_recorder.go", model.LogCategoryEdgeCompute},
		{"internal/northbound/mqtt/client.go", model.LogCategoryNorthbound},
		{"internal/server/server.go", model.LogCategorySystem},
	}
	for _, tc := range tests {
		if got := InferCategoryFromCaller(tc.file); got != tc.want {
			t.Fatalf("InferCategoryFromCaller(%q) = %q, want %q", tc.file, got, tc.want)
		}
	}
}

func TestEnrichLogFieldsAddsCategoryAndNormalizedIDs(t *testing.T) {
	ent := zapcore.Entry{Caller: zapcore.NewEntryCaller(0, "internal/core/scan_engine.go", 100, true)}
	fields := []zapcore.Field{
		{Key: "deviceKey", Type: zapcore.StringType, String: "dev-1"},
		{Key: "channelID", Type: zapcore.StringType, String: "ch-1"},
	}
	enriched := enrichLogFields(ent, fields)

	var category, channelID, deviceID string
	for _, field := range enriched {
		switch field.Key {
		case "category":
			category = field.String
		case "channel_id":
			channelID = field.String
		case "device_id":
			deviceID = field.String
		}
	}
	if category != model.LogCategorySouthbound {
		t.Fatalf("category = %q, want %q", category, model.LogCategorySouthbound)
	}
	if channelID != "ch-1" {
		t.Fatalf("channel_id = %q, want ch-1", channelID)
	}
	if deviceID != "dev-1" {
		t.Fatalf("device_id = %q, want dev-1", deviceID)
	}
}

func TestEnrichLogFieldsPreservesExplicitCategory(t *testing.T) {
	ent := zapcore.Entry{Caller: zapcore.NewEntryCaller(0, "internal/server/server.go", 1, true)}
	fields := []zapcore.Field{
		{Key: "category", Type: zapcore.StringType, String: model.LogCategoryNorthbound},
	}
	enriched := enrichLogFields(ent, fields)
	categories := 0
	for _, field := range enriched {
		if field.Key == "category" {
			categories++
			if field.String != model.LogCategoryNorthbound {
				t.Fatalf("category = %q, want %q", field.String, model.LogCategoryNorthbound)
			}
		}
	}
	if categories != 1 {
		t.Fatalf("expected one category field, got %d fields total=%d", categories, len(enriched))
	}
}

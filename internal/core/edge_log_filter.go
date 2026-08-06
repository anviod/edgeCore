package core

import (
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// EdgeLogFilter selects persisted edge error logs.
type EdgeLogFilter struct {
	RuleID    string
	Category  string
	ChannelID string
	DeviceID  string
}

func populateEdgeFailureScope(rec *model.EdgeFailureRecord, val model.Value) {
	if rec == nil {
		return
	}
	if rec.Category == "" {
		rec.Category = model.LogCategoryEdgeCompute
	}
	if rec.ChannelID == "" {
		rec.ChannelID = strings.TrimSpace(val.ChannelID)
	}
	if rec.DeviceID == "" {
		rec.DeviceID = strings.TrimSpace(val.DeviceID)
	}
	if rec.Context != nil {
		if rec.ChannelID == "" {
			rec.ChannelID = contextString(rec.Context, "channel_id")
		}
		if rec.DeviceID == "" {
			rec.DeviceID = contextString(rec.Context, "device_id")
		}
	}
}

func contextString(ctx map[string]any, key string) string {
	if ctx == nil {
		return ""
	}
	value, ok := ctx[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// MatchesEdgeLogFilter reports whether a snapshot satisfies optional edge log filters.
func MatchesEdgeLogFilter(snap model.RuleMinuteSnapshot, filter EdgeLogFilter) bool {
	if filter.RuleID != "" && snap.RuleID != filter.RuleID {
		return false
	}
	if filter.Category != "" && snapCategory(snap) != filter.Category {
		return false
	}
	if filter.ChannelID != "" && snap.ChannelID != filter.ChannelID {
		return false
	}
	if filter.DeviceID != "" && snap.DeviceID != filter.DeviceID {
		return false
	}
	return true
}

func snapCategory(snap model.RuleMinuteSnapshot) string {
	if snap.Category != "" {
		return snap.Category
	}
	return model.LogCategoryEdgeCompute
}

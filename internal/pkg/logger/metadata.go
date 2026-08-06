package logger

import (
	"strings"

	"github.com/anviod/edgeCore/internal/model"
	"go.uber.org/zap/zapcore"
)

type metadataCore struct {
	zapcore.Core
}

func wrapMetadataCore(c zapcore.Core) zapcore.Core {
	return &metadataCore{Core: c}
}

func (c *metadataCore) With(fields []zapcore.Field) zapcore.Core {
	return &metadataCore{Core: c.Core.With(enrichFieldSlice(fields))}
}

func (c *metadataCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	return c.Core.Write(ent, enrichLogFields(ent, fields))
}

func enrichFieldSlice(fields []zapcore.Field) []zapcore.Field {
	if len(fields) == 0 {
		return fields
	}
	ent := zapcore.Entry{}
	return enrichLogFields(ent, fields)
}

func enrichLogFields(ent zapcore.Entry, fields []zapcore.Field) []zapcore.Field {
	state := scanLogFieldState(fields)
	extra := make([]zapcore.Field, 0, 3)

	if !state.hasCategory {
		category := InferCategoryFromCaller(ent.Caller.File)
		if category != "" {
			extra = append(extra, zapcore.Field{Key: "category", Type: zapcore.StringType, String: category})
		}
	}
	if !state.hasChannelID && state.channelID != "" {
		extra = append(extra, zapcore.Field{Key: "channel_id", Type: zapcore.StringType, String: state.channelID})
	}
	if !state.hasDeviceID && state.deviceID != "" {
		extra = append(extra, zapcore.Field{Key: "device_id", Type: zapcore.StringType, String: state.deviceID})
	}

	if len(extra) == 0 {
		return fields
	}
	out := make([]zapcore.Field, len(fields), len(fields)+len(extra))
	copy(out, fields)
	return append(out, extra...)
}

type logFieldState struct {
	hasCategory  bool
	hasChannelID bool
	hasDeviceID  bool
	channelID    string
	deviceID     string
}

func scanLogFieldState(fields []zapcore.Field) logFieldState {
	var state logFieldState
	for _, field := range fields {
		switch field.Key {
		case "category":
			state.hasCategory = fieldString(field) != ""
		case "channel_id":
			if value := fieldString(field); value != "" {
				state.hasChannelID = true
				state.channelID = value
			}
		case "channelID", "channelId":
			if value := fieldString(field); value != "" && state.channelID == "" {
				state.channelID = value
			}
		case "device_id":
			if value := fieldString(field); value != "" {
				state.hasDeviceID = true
				state.deviceID = value
			}
		case "deviceID", "deviceId", "deviceKey":
			if value := fieldString(field); value != "" && state.deviceID == "" {
				state.deviceID = value
			}
		}
	}
	return state
}

func fieldString(field zapcore.Field) string {
	if field.Type == zapcore.StringType {
		return strings.TrimSpace(field.String)
	}
	if field.Interface != nil {
		return strings.TrimSpace(fieldStringFromAny(field.Interface))
	}
	return ""
}

func fieldStringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return ""
	}
}

// InferCategoryFromCaller maps source file paths to stable log categories.
func InferCategoryFromCaller(file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	switch {
	case strings.Contains(file, "internal/driver/"),
		strings.Contains(file, "internal/core/channel"),
		strings.Contains(file, "internal/core/scan_engine"):
		return model.LogCategorySouthbound
	case strings.Contains(file, "internal/core/edge_"),
		strings.Contains(file, "internal/core/edge_compute"):
		return model.LogCategoryEdgeCompute
	case strings.Contains(file, "internal/northbound/"):
		return model.LogCategoryNorthbound
	default:
		return model.LogCategorySystem
	}
}

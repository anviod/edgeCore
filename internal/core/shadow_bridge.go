package core

import (
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

// ShadowBridge 将 ShadowCore 快照变更扇出到 DataPipeline，
// 使边缘规则、北向推送与历史落库与 UI 影子数据源一致。
type ShadowBridge struct {
	pipeline *DataPipeline
}

func NewShadowBridge(pipeline *DataPipeline) *ShadowBridge {
	return &ShadowBridge{pipeline: pipeline}
}

// Attach 订阅 ShadowCore 变更并推送到 DataPipeline。
func (sb *ShadowBridge) Attach(sc *ShadowCore) {
	if sb == nil || sb.pipeline == nil || sc == nil {
		return
	}
	sc.Subscribe(func(shadowDeviceID string, points map[string]model.ShadowPoint) {
		sb.pushFromShadow(sc, shadowDeviceID, points)
	})
}

func (sb *ShadowBridge) pushFromShadow(sc *ShadowCore, shadowDeviceID string, points map[string]model.ShadowPoint) {
	channelID, deviceID, err := sc.ResolvePublishTarget(shadowDeviceID)
	if err != nil {
		return
	}

	batch := make([]model.Value, 0, len(points))
	for pointID, pt := range points {
		collectedAt := pt.CollectedAt
		if collectedAt.IsZero() {
			collectedAt = pt.Timestamp
		}
		batch = append(batch, model.Value{
			ChannelID: channelID,
			DeviceID:  deviceID,
			PointID:   pointID,
			Value:     pt.Value,
			Quality:   normalizeQuality(pt.Quality),
			TS:        collectedAt,
		})
	}
	sb.pipeline.PushBatch(batch)
}

func normalizeQuality(q string) string {
	if strings.EqualFold(q, "good") {
		return "Good"
	}
	if strings.EqualFold(q, "bad") {
		return "Bad"
	}
	return q
}

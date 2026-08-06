package core

import (
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

// cowShadowSnapshot is an immutable device snapshot published via atomic store.
// Readers may share the Points map reference; writers never mutate a published snapshot.
type cowShadowSnapshot struct {
	ShadowDeviceID       string
	PhysicalDeviceID     string
	ChannelID            string
	Version              uint64
	UpdatedAt            time.Time
	Points               map[string]model.ShadowPoint
	CommunicationProfile *model.DeviceCommunicationProfile
}

type shadowDeviceEntry struct {
	snapshot atomic.Pointer[cowShadowSnapshot]
}

func newShadowDeviceEntry() *shadowDeviceEntry {
	return &shadowDeviceEntry{}
}

func (e *shadowDeviceEntry) load() *cowShadowSnapshot {
	if e == nil {
		return nil
	}
	return e.snapshot.Load()
}

func (e *shadowDeviceEntry) publish(snap *cowShadowSnapshot) {
	e.snapshot.Store(snap)
}

func cowMergePoints(old map[string]model.ShadowPoint, updates map[string]model.ShadowPoint) map[string]model.ShadowPoint {
	if len(updates) == 0 {
		return old
	}
	if old == nil {
		out := make(map[string]model.ShadowPoint, len(updates))
		for k, v := range updates {
			out[k] = v
		}
		return out
	}
	out := make(map[string]model.ShadowPoint, len(old)+len(updates))
	for k, v := range old {
		out[k] = v
	}
	for k, v := range updates {
		out[k] = v
	}
	return out
}

func viewFromSnapshot(snap *cowShadowSnapshot) *model.ShadowDevice {
	if snap == nil {
		return nil
	}
	return &model.ShadowDevice{
		ShadowDeviceID:       snap.ShadowDeviceID,
		PhysicalDeviceID:     snap.PhysicalDeviceID,
		ChannelID:            snap.ChannelID,
		Version:              snap.Version,
		UpdatedAt:            snap.UpdatedAt,
		Points:               snap.Points,
		CommunicationProfile: snap.CommunicationProfile,
	}
}

func buildSnapshotFromEntry(
	prev *cowShadowSnapshot,
	shadowDeviceID, physicalDeviceID, channelID string,
	version uint64,
	updatedAt time.Time,
	changed map[string]model.ShadowPoint,
	profile *model.DeviceCommunicationProfile,
) *cowShadowSnapshot {
	var oldPoints map[string]model.ShadowPoint
	if prev != nil {
		oldPoints = prev.Points
	}
	return &cowShadowSnapshot{
		ShadowDeviceID:       shadowDeviceID,
		PhysicalDeviceID:     physicalDeviceID,
		ChannelID:            channelID,
		Version:              version,
		UpdatedAt:            updatedAt,
		Points:               cowMergePoints(oldPoints, changed),
		CommunicationProfile: profile,
	}
}

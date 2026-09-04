package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

// TagEntry 表示 Tag 数据库中的标准化点位元数据。
type TagEntry struct {
	ChannelID string
	DeviceID  string
	PointID   string
	Name      string
	Alias     string
	EU        string
	Scale     float64
	Offset    float64
	ScanClass string
}

// TagRegistry 统一点位 ID、别名、工程单位与缩放因子。
type TagRegistry struct {
	mu   sync.RWMutex
	tags map[string]TagEntry
}

func NewTagRegistry() *TagRegistry {
	return &TagRegistry{
		tags: make(map[string]TagEntry),
	}
}

func tagKey(channelID, deviceID, pointID string) string {
	return channelID + "/" + deviceID + "/" + pointID
}

func (tr *TagRegistry) RegisterFromDevice(channelID string, dev *model.Device) {
	if tr == nil || dev == nil {
		return
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for _, p := range dev.Points {
		key := tagKey(channelID, dev.ID, p.ID)
		alias := p.Name
		if p.Group != "" {
			alias = p.Group + "." + p.Name
		}
		tr.tags[key] = TagEntry{
			ChannelID: channelID,
			DeviceID:  dev.ID,
			PointID:   p.ID,
			Name:      p.Name,
			Alias:     alias,
			EU:        p.Unit,
			Scale:     p.Scale,
			Offset:    p.Offset,
			ScanClass: model.NormalizeScanClass(p.ScanClass),
		}
	}
}

func (tr *TagRegistry) UnregisterDevice(channelID, deviceID string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	prefix := channelID + "/" + deviceID + "/"
	for k := range tr.tags {
		if strings.HasPrefix(k, prefix) {
			delete(tr.tags, k)
		}
	}
}

func (tr *TagRegistry) Get(channelID, deviceID, pointID string) (TagEntry, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	e, ok := tr.tags[tagKey(channelID, deviceID, pointID)]
	return e, ok
}

func (tr *TagRegistry) Resolve(alias string) (TagEntry, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	for _, e := range tr.tags {
		if e.Alias == alias || e.Name == alias || e.PointID == alias {
			return e, nil
		}
	}
	return TagEntry{}, fmt.Errorf("tag not found: %s", alias)
}

func (tr *TagRegistry) ApplyScaling(v model.Value) model.Value {
	entry, ok := tr.Get(v.ChannelID, v.DeviceID, v.PointID)
	if !ok {
		return v
	}
	if entry.Scale == 0 && entry.Offset == 0 {
		return v
	}
	if num, ok := toFloat64(v.Value); ok {
		scale := entry.Scale
		if scale == 0 {
			scale = 1
		}
		v.Value = num*scale + entry.Offset
	}
	return v
}

func (tr *TagRegistry) Count() int {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return len(tr.tags)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	default:
		return 0, false
	}
}

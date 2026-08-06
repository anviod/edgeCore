package core

import (
	"fmt"
	"strings"
	"sync"

	"github.com/anviod/edgeCore/internal/model"
)

// VirtualShadowManager 管理虚拟影子设备配置与运行时引擎。
type VirtualShadowManager struct {
	mu      sync.RWMutex
	vse     *VirtualShadowEngine
	cm      *ChannelManager
	shadow  *ShadowCore
	configs []model.VirtualShadowDeviceConfig
	saveFn  func([]model.VirtualShadowDeviceConfig) error
}

func NewVirtualShadowManager(vse *VirtualShadowEngine, cm *ChannelManager, shadow *ShadowCore, saveFn func([]model.VirtualShadowDeviceConfig) error) *VirtualShadowManager {
	return &VirtualShadowManager{
		vse:    vse,
		cm:     cm,
		shadow: shadow,
		saveFn: saveFn,
	}
}

func (m *VirtualShadowManager) Load(configs []model.VirtualShadowDeviceConfig) {
	m.mu.Lock()
	m.configs = append([]model.VirtualShadowDeviceConfig(nil), configs...)
	m.mu.Unlock()

	for _, cfg := range configs {
		if !cfg.Enable {
			continue
		}
		_ = m.applyOne(cfg)
	}
}

// ReloadAll 在通道/设备/点位拓扑变更后，重新应用所有已启用的虚拟影子设备。
func (m *VirtualShadowManager) ReloadAll() {
	m.mu.RLock()
	configs := append([]model.VirtualShadowDeviceConfig(nil), m.configs...)
	m.mu.RUnlock()

	for _, cfg := range configs {
		if !cfg.Enable {
			_ = m.vse.DeleteVirtualDevice(cfg.ID)
			continue
		}
		_ = m.applyOne(cfg)
	}
}

func (m *VirtualShadowManager) List() []model.VirtualShadowDeviceConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]model.VirtualShadowDeviceConfig, len(m.configs))
	copy(out, m.configs)
	return out
}

func (m *VirtualShadowManager) Get(id string) (*model.VirtualShadowDeviceConfig, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.configs {
		if m.configs[i].ID == id {
			copy := m.configs[i]
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("virtual shadow device not found: %s", id)
}

func (m *VirtualShadowManager) Create(cfg model.VirtualShadowDeviceConfig) error {
	copy := cfg
	if err := model.NormalizeVirtualShadowDevice(&copy); err != nil {
		return err
	}
	if copy.Enable {
		if err := m.applyOne(copy); err != nil {
			return err
		}
	}

	m.mu.Lock()
	for _, existing := range m.configs {
		if existing.ID == copy.ID {
			m.mu.Unlock()
			return fmt.Errorf("virtual shadow device already exists: %s", copy.ID)
		}
	}
	m.configs = append(m.configs, copy)
	configs := append([]model.VirtualShadowDeviceConfig(nil), m.configs...)
	m.mu.Unlock()

	return m.persist(configs)
}

func (m *VirtualShadowManager) Update(id string, cfg model.VirtualShadowDeviceConfig) error {
	cfg.ID = id
	copy := cfg
	if err := model.NormalizeVirtualShadowDevice(&copy); err != nil {
		return err
	}

	m.mu.Lock()
	found := false
	for i := range m.configs {
		if m.configs[i].ID == id {
			m.configs[i] = copy
			found = true
			break
		}
	}
	if !found {
		m.mu.Unlock()
		return fmt.Errorf("virtual shadow device not found: %s", id)
	}
	configs := append([]model.VirtualShadowDeviceConfig(nil), m.configs...)
	m.mu.Unlock()

	if copy.Enable {
		if err := m.applyOne(copy); err != nil {
			return err
		}
	} else {
		_ = m.vse.DeleteVirtualDevice(id)
	}

	return m.persist(configs)
}

func (m *VirtualShadowManager) Delete(id string) error {
	m.mu.Lock()
	idx := -1
	for i := range m.configs {
		if m.configs[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return fmt.Errorf("virtual shadow device not found: %s", id)
	}
	m.configs = append(m.configs[:idx], m.configs[idx+1:]...)
	configs := append([]model.VirtualShadowDeviceConfig(nil), m.configs...)
	m.mu.Unlock()

	_ = m.vse.DeleteVirtualDevice(id)
	return m.persist(configs)
}

func (m *VirtualShadowManager) ListPointSources() []model.PointSourceRef {
	if m.cm == nil {
		return nil
	}
	channels := m.cm.GetChannels()
	sources := make([]model.PointSourceRef, 0)
	for _, ch := range channels {
		for _, dev := range ch.Devices {
			for _, pt := range dev.Points {
				sources = append(sources, model.PointSourceRef{
					ChannelID:   ch.ID,
					ChannelName: ch.Name,
					DeviceID:    dev.ID,
					DeviceName:  dev.Name,
					PointID:     pt.ID,
					PointName:   pt.Name,
					Ref:         model.MakePointRef(ch.ID, dev.ID, pt.ID),
				})
			}
		}
	}
	return sources
}

// SearchSourceDevices 按设备名称/ID/通道模糊检索源设备（不含点位明细）。
func (m *VirtualShadowManager) SearchSourceDevices(query, channelID string, limit int) []model.SourceDeviceSummary {
	if m.cm == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query = strings.TrimSpace(query)
	channelID = strings.TrimSpace(channelID)
	if query == "" && channelID == "" {
		return nil
	}

	results := make([]model.SourceDeviceSummary, 0, limit)
	for _, ch := range m.cm.GetChannels() {
		if channelID != "" && ch.ID != channelID {
			continue
		}
		for _, dev := range ch.Devices {
			if query != "" {
				hay := ch.Name + " " + ch.ID + " " + dev.Name + " " + dev.ID
				if !model.MatchSearchQuery(hay, query) {
					continue
				}
			}
			state := 2
			if d := m.cm.GetDevice(ch.ID, dev.ID); d != nil {
				state = d.State
			}
			results = append(results, model.SourceDeviceSummary{
				Key:         ch.ID + "::" + dev.ID,
				ChannelID:   ch.ID,
				ChannelName: ch.Name,
				DeviceID:    dev.ID,
				DeviceName:  dev.Name,
				PointCount:  len(dev.Points),
				State:       state,
				Online:      state == 0 || state == 1,
			})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}

// ListDevicePointSources 返回指定设备的可选点位（支持点位级过滤）。
func (m *VirtualShadowManager) ListDevicePointSources(channelID, deviceID, query string) ([]model.PointSourceRef, error) {
	if m.cm == nil {
		return nil, fmt.Errorf("channel manager not available")
	}
	ch := m.cm.GetChannel(channelID)
	if ch == nil {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}
	dev := m.cm.GetDevice(channelID, deviceID)
	if dev == nil {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}

	sources := make([]model.PointSourceRef, 0, len(dev.Points))
	for _, pt := range dev.Points {
		hay := pt.ID + " " + pt.Name
		if query != "" && !model.MatchSearchQuery(hay, query) {
			continue
		}
		sources = append(sources, model.PointSourceRef{
			ChannelID:   ch.ID,
			ChannelName: ch.Name,
			DeviceID:    dev.ID,
			DeviceName:  dev.Name,
			PointID:     pt.ID,
			PointName:   pt.Name,
			Ref:         model.MakePointRef(ch.ID, dev.ID, pt.ID),
		})
	}
	return sources, nil
}

func (m *VirtualShadowManager) GetRuntime(id string) (*model.VirtualDevice, map[string]model.ShadowPoint, error) {
	if m.shadow != nil {
		if vd, err := m.shadow.GetVirtualShadowDevice(id); err == nil {
			return vd, vd.Points, nil
		}
	}
	if m.vse != nil {
		if vd, err := m.vse.GetVirtualDevice(id); err == nil {
			return vd, vd.Points, nil
		}
	}
	return nil, nil, fmt.Errorf("virtual shadow device not found: %s", id)
}

// RefreshRuntime 触发重算后返回最新运行时快照。
func (m *VirtualShadowManager) RefreshRuntime(id string) (*model.VirtualDevice, map[string]model.ShadowPoint, error) {
	if m.vse != nil {
		m.vse.RecomputeDevice(id)
	}
	return m.GetRuntime(id)
}

func (m *VirtualShadowManager) applyOne(cfg model.VirtualShadowDeviceConfig) error {
	formulas, err := model.BuildVirtualShadowFormulas(cfg.Points)
	if err != nil {
		return err
	}
	return m.vse.ReplaceVirtualDevice(cfg.ID, "", formulas)
}

func (m *VirtualShadowManager) persist(configs []model.VirtualShadowDeviceConfig) error {
	if m.saveFn == nil {
		return nil
	}
	return m.saveFn(configs)
}

func (m *VirtualShadowManager) Engine() *VirtualShadowEngine {
	return m.vse
}

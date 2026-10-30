package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"

	"go.uber.org/zap"
)

var saveMu sync.Mutex

// ConfigManager 管理配置的加载与持久化（运行时以数据库为唯一数据源）。
type ConfigManager struct {
	Config *Config
	db     storage.KVDB
	useDB  bool
	// onReload：Reload() 成功后同步回调，通知持有旧 *Config 引用/快照的
	// 组件重新挂载（P0 数据丢失防护：否则导入/恢复后旧内存态会随下一次
	// 全量保存把刚导入的配置静默覆盖回去）。
	onReload []func(*Config)
}

type Config struct {
	Server struct {
		Port     int
		LogLevel string
	}
	Storage struct {
		Path string
	}
	Northbound model.NorthboundConfig
	Channels   []model.Channel
	EdgeRules  []model.EdgeRule
	System     model.SystemConfig
	Users      []model.UserConfig
}

// NewConfigManagerWithEmptyConfig 创建一个使用默认空配置的配置管理器（用于安装前）
func NewConfigManagerWithEmptyConfig() *ConfigManager {
	cfg := DefaultConfig()

	return &ConfigManager{
		Config: cfg,
		db:     nil,
		useDB:  false,
	}
}

// NewConfigManagerWithDB 创建一个使用数据库存储配置的配置管理器
func NewConfigManagerWithDB(db storage.KVDB) (*ConfigManager, error) {
	configStore, err := storage.NewConfigStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create config store: %w", err)
	}

	hasData, err := configStore.HasConfigData()
	if err != nil {
		return nil, fmt.Errorf("failed to check config data: %w", err)
	}

	var cfg *Config
	if hasData {
		cfg, err = LoadConfigFromDB(db)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from DB: %w", err)
		}
	} else {
		cfg = DefaultConfig()
	}

	return &ConfigManager{
		Config: cfg,
		db:     db,
		useDB:  true,
	}, nil
}

func DefaultConfig() *Config {
	cfg := &Config{}
	cfg.Server.Port = 8080
	cfg.Server.LogLevel = "info"
	cfg.Northbound = model.NorthboundConfig{
		MQTT:       []model.MQTTConfig{},
		HTTP:       []model.HTTPConfig{},
		OPCUA:      []model.OPCUAConfig{},
		SparkplugB: []model.SparkplugBConfig{},
		EdgeOSMQTT: []model.EdgeOSMQTTConfig{},
		EdgeOSNATS: []model.EdgeOSNATSConfig{},
	}
	cfg.Channels = []model.Channel{}
	cfg.EdgeRules = []model.EdgeRule{}
	cfg.System = model.SystemConfig{
		Hostname: model.HostnameConfig{
			Name:       "edgeCore",
			EnableMDNS: true,
			EnableBare: true,
			HTTPPort:   cfg.Server.Port,
			HTTPSPort:  443,
		},
	}
	cfg.Users = []model.UserConfig{}
	return cfg
}

// GetConfig 获取当前配置
func (cm *ConfigManager) GetConfig() *Config {
	return cm.Config
}

// AttachDB 将运行时数据库绑定到配置管理器（安装完成后使用）
func (cm *ConfigManager) AttachDB(db storage.KVDB) {
	cm.db = db
	cm.useDB = true
}

// OnReload 注册 Reload() 完成后的回调（导入/恢复场景下组件重新挂载新配置）。
func (cm *ConfigManager) OnReload(fn func(*Config)) {
	cm.onReload = append(cm.onReload, fn)
}

// Reload 从数据库重新加载配置。
func (cm *ConfigManager) Reload() error {
	if !cm.useDB || cm.db == nil {
		return fmt.Errorf("configuration reload requires database attachment")
	}
	newCfg, err := LoadConfigFromDB(cm.db)
	if err != nil {
		return err
	}
	cm.Config = newCfg
	for _, fn := range cm.onReload {
		fn(newCfg)
	}
	return nil
}

// SaveConfig 保存配置。
// 迁移到数据库后，数据库为唯一数据源：useDB 时仅写入数据库，
// 配置体系以数据库为唯一数据源（legacy YAML 配置已于 2026-10 移除），
// 不再写回任何文件系统配置目录，避免重建配置目录或写入失败。
func (cm *ConfigManager) SaveConfig(cfg *Config) error {
	saveMu.Lock()
	defer saveMu.Unlock()

	if cm.useDB && cm.db != nil {
		return SaveConfigToDB(cm.db, cfg)
	}

	return fmt.Errorf("configuration persistence requires database attachment")
}

func LoadConfigFromDB(db storage.KVDB) (*Config, error) {
	configStore, err := storage.NewConfigStore(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create config store: %w", err)
	}

	cfg := &Config{}

	serverConfig, err := configStore.LoadServerConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load server config: %w", err)
	}
	if serverConfig != nil {
		cfg.Server.Port = serverConfig.Port
		cfg.Server.LogLevel = serverConfig.LogLevel
	}

	channels, err := configStore.LoadChannels()
	if err != nil {
		return nil, fmt.Errorf("failed to load channels: %w", err)
	}

	// 启动加载兜底：历史版本缺陷可能在持久化数据中留下通道内重复设备 ID
	// （例如设备改名时未同步 ID、复制设备时 ID 冲突）。AddChannel 对此 Fail-Fast
	// 会拒收整条通道，导致该通道采集在重启后全部消失。
	// 这里在加载层按"保留首个"去重并告警，不改变 API 层（AddChannel/UpdateChannel）
	// 的严格校验；后续任一次通道保存都会把去重结果写回 DB，实现数据自愈。
	// P1 防护（2026-10-13 审查）：被丢弃的重复设备副本先落 dropped 文件，
	// 副本中的点位表不静默销毁。
	for i := range channels {
		seen := make(map[string]struct{}, len(channels[i].Devices))
		kept := channels[i].Devices[:0]
		var droppedDups []model.Device
		for _, dev := range channels[i].Devices {
			if _, dup := seen[dev.ID]; dup {
				zap.L().Warn("dropping duplicate device in persisted channel",
					zap.String("channel_id", channels[i].ID),
					zap.String("channel_name", channels[i].Name),
					zap.String("device_id", dev.ID),
					zap.String("device_name", dev.Name))
				droppedDups = append(droppedDups, dev)
				continue
			}
			seen[dev.ID] = struct{}{}
			kept = append(kept, dev)
		}
		if len(droppedDups) > 0 {
			dumpDroppedEntries("duplicate-devices-"+channels[i].ID, droppedDups)
		}
		channels[i].Devices = kept
	}

	cfg.Channels = channels

	devices, err := configStore.LoadAllDevices()
	if err != nil {
		return nil, fmt.Errorf("failed to load devices: %w", err)
	}

	for i := range cfg.Channels {
		for j := range cfg.Channels[i].Devices {
			if device, ok := devices[cfg.Channels[i].Devices[j].ID]; ok {
				cfg.Channels[i].Devices[j] = device
			}
		}
	}

	northbound, err := configStore.LoadNorthbound()
	if err != nil {
		return nil, fmt.Errorf("failed to load northbound config: %w", err)
	}
	if northbound != nil {
		cfg.Northbound = *northbound
	}

	edgeRules, err := configStore.LoadEdgeRules()
	if err != nil {
		return nil, fmt.Errorf("failed to load edge rules: %w", err)
	}
	cfg.EdgeRules = edgeRules

	system, err := configStore.LoadSystem()
	if err != nil {
		return nil, fmt.Errorf("failed to load system config: %w", err)
	}
	if system != nil {
		cfg.System = *system
	}

	users, err := configStore.LoadUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to load users: %w", err)
	}
	cfg.Users = users

	for i := range cfg.Channels {
		cfg.Channels[i].NodeRuntime = &model.NodeRuntime{State: 0}

		for j := range cfg.Channels[i].Devices {
			cfg.Channels[i].Devices[j].NodeRuntime = &model.NodeRuntime{State: 0}
		}
	}

	return cfg, nil
}

// dumpDroppedEntries 将持久化层剔除的脏数据落盘带时间戳副本，供人工恢复/排查。
// 隔离策略（P1 防护，2026-10-13 审查）：脏数据不允许劫持持久化管道，
// 但也绝不静默消失——先留副本再剔除。
func dumpDroppedEntries(kind string, entries any) {
	if entries == nil {
		return
	}
	data, err := json.Marshal(entries)
	if err != nil {
		zap.L().Error("failed to marshal dropped entries for dump", zap.String("kind", kind), zap.Error(err))
		return
	}
	if err := os.MkdirAll("data", 0750); err != nil {
		zap.L().Error("failed to create data dir for dropped entries dump", zap.Error(err))
		return
	}
	path := filepath.Join("data", fmt.Sprintf("dropped-%s-%s.json", kind, time.Now().Format("20060102-150405")))
	if err := os.WriteFile(path, data, 0644); err != nil {
		zap.L().Error("failed to dump dropped entries", zap.String("kind", kind), zap.String("path", path), zap.Error(err))
		return
	}
	zap.L().Error("Dropped invalid entries during save (dump saved for recovery)",
		zap.String("kind", kind),
		zap.String("dump", path))
}

func SaveConfigToDB(db storage.KVDB, cfg *Config) error {
	configStore, err := storage.NewConfigStore(db)
	if err != nil {
		return fmt.Errorf("failed to create config store: %w", err)
	}

	serverConfig := model.ServerConfig{
		Port:     cfg.Server.Port,
		LogLevel: cfg.Server.LogLevel,
	}

	var allDevices []model.Device
	for _, channel := range cfg.Channels {
		allDevices = append(allDevices, channel.Devices...)
	}
	// P1 防护（2026-10-13 审查）：容错校验——单条脏数据不得让全系统配置
	// 无法落盘。剔除的条目先落 dropped 副本。通道 blob 内的数据原样保留，
	// 仅 Devices bucket 跳过非法条目。
	allDevices, droppedDevices := model.SanitizeDevicesForSave(allDevices)
	if len(droppedDevices) > 0 {
		dumpDroppedEntries("devices", droppedDevices)
	}

	northbound, droppedNorthbound := model.SanitizeNorthboundForSave(cfg.Northbound)
	if len(droppedNorthbound) > 0 {
		dumpDroppedEntries("northbound", droppedNorthbound)
	}

	edgeRules, droppedRules := model.SanitizeEdgeRulesForSave(cfg.EdgeRules)
	if len(droppedRules) > 0 {
		dumpDroppedEntries("edge-rules", droppedRules)
	}

	if err := configStore.SaveAllConfig(serverConfig, cfg.Channels, allDevices,
		northbound, edgeRules, cfg.System, cfg.Users); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

func (cm *ConfigManager) LoadVirtualShadows() ([]model.VirtualShadowDeviceConfig, error) {
	if !cm.useDB || cm.db == nil {
		return []model.VirtualShadowDeviceConfig{}, nil
	}
	configStore, err := storage.NewConfigStore(cm.db)
	if err != nil {
		return nil, err
	}
	return configStore.LoadVirtualShadows()
}

func (cm *ConfigManager) SaveVirtualShadows(devices []model.VirtualShadowDeviceConfig) error {
	if !cm.useDB || cm.db == nil {
		return nil
	}
	configStore, err := storage.NewConfigStore(cm.db)
	if err != nil {
		return err
	}
	return configStore.SaveVirtualShadows(devices)
}

package config

import (
	"fmt"
	"sync"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/anviod/edgeCore/internal/storage"

	"go.etcd.io/bbolt"
)

var saveMu sync.Mutex

// ConfigManager 管理配置的加载与持久化（运行时以数据库为唯一数据源）。
type ConfigManager struct {
	Config  *Config
	ConfDir string
	db      *bbolt.DB
	useDB   bool
}

type Config struct {
	Server struct {
		Port     int    `yaml:"port"`
		LogLevel string `yaml:"logLevel"`
	} `yaml:"server"`
	Storage struct {
		Path string `yaml:"path"`
	} `yaml:"storage"`
	Northbound model.NorthboundConfig `yaml:"northbound"`
	Channels   []model.Channel        `yaml:"channels"`
	EdgeRules  []model.EdgeRule       `yaml:"edge_rules"`
	System     model.SystemConfig     `yaml:"system"`
	Users      []model.UserConfig     `yaml:"users"`
}

// NewConfigManagerWithEmptyConfig 创建一个使用默认空配置的配置管理器（用于安装前）
func NewConfigManagerWithEmptyConfig(confDir string) *ConfigManager {
	cfg := DefaultConfig()

	return &ConfigManager{
		Config:  cfg,
		ConfDir: confDir,
		db:      nil,
		useDB:   false,
	}
}

// NewConfigManagerWithDB 创建一个使用数据库存储配置的配置管理器
func NewConfigManagerWithDB(confDir string, db *bbolt.DB) (*ConfigManager, error) {
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
		Config:  cfg,
		ConfDir: confDir,
		db:      db,
		useDB:   true,
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
func (cm *ConfigManager) AttachDB(db *bbolt.DB) {
	cm.db = db
	cm.useDB = true
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
	return nil
}

// SaveConfig 保存配置。
// 迁移到数据库后，数据库为唯一数据源：useDB 时仅写入数据库，
// 不再写回已废弃（可能已删除）的 conf/*.yaml，避免重建配置目录或写入失败。
func (cm *ConfigManager) SaveConfig(cfg *Config) error {
	saveMu.Lock()
	defer saveMu.Unlock()

	if cm.useDB && cm.db != nil {
		return SaveConfigToDB(cm.db, cfg)
	}

	return fmt.Errorf("configuration persistence requires database attachment")
}

func LoadConfigFromDB(db *bbolt.DB) (*Config, error) {
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

func SaveConfigToDB(db *bbolt.DB, cfg *Config) error {
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
	allDevices, err = model.NormalizeDevicesForSave(allDevices)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	northbound, err := model.NormalizeNorthboundForSave(cfg.Northbound)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	edgeRules, err := model.NormalizeEdgeRulesForSave(cfg.EdgeRules)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
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

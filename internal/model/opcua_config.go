package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OpcUaDeviceMap 北向 OPC UA 服务端的设备映射，兼容历史 bool 与 DevicePublishConfig 两种格式。
type OpcUaDeviceMap map[string]DevicePublishConfig

// AllowsDevice 仅允许显式启用（Enable=true）的设备暴露到北向地址空间；
// 未在映射中列出或显式禁用的设备默认不暴露，需在树形映射中启用后才可见。
func (m OpcUaDeviceMap) AllowsDevice(deviceID string) bool {
	cfg, ok := m[deviceID]
	if !ok {
		return false
	}
	return cfg.Enable
}

func (m *OpcUaDeviceMap) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*m = nil
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out := make(OpcUaDeviceMap, len(raw))
	for k, v := range raw {
		var enabled bool
		if err := json.Unmarshal(v, &enabled); err == nil {
			out[k] = DevicePublishConfig{Enable: enabled}
			continue
		}
		var cfg DevicePublishConfig
		if err := json.Unmarshal(v, &cfg); err != nil {
			return fmt.Errorf("devices[%q]: %w", k, err)
		}
		out[k] = cfg
	}
	*m = out
	return nil
}

func (m *OpcUaDeviceMap) UnmarshalYAML(unmarshal func(any) error) error {
	var raw map[string]any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if raw == nil {
		*m = nil
		return nil
	}
	out := make(OpcUaDeviceMap, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case bool:
			out[k] = DevicePublishConfig{Enable: val}
		case map[string]any, map[any]any:
			data, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("devices[%q]: %w", k, err)
			}
			var cfg DevicePublishConfig
			if err := json.Unmarshal(data, &cfg); err != nil {
				return fmt.Errorf("devices[%q]: %w", k, err)
			}
			out[k] = cfg
		default:
			return fmt.Errorf("devices[%q]: unsupported type %T", k, v)
		}
	}
	*m = out
	return nil
}

// ResolveOpcUaEndpoint 解析 OPC UA 连接地址：设备 endpoint 优先，其次通道 url / endpoint。
func ResolveOpcUaEndpoint(channelConfig, deviceConfig map[string]any) string {
	if deviceConfig != nil {
		if ep := configString(deviceConfig["endpoint"]); ep != "" {
			return ep
		}
	}
	if channelConfig != nil {
		if url := configString(channelConfig["url"]); url != "" {
			return url
		}
		if ep := configString(channelConfig["endpoint"]); ep != "" {
			return ep
		}
	}
	return ""
}

// MergeOpcUaDeviceConfig 合并通道与设备 OPC UA 配置（设备字段覆盖通道同名字段），并规范化 endpoint。
func MergeOpcUaDeviceConfig(channelConfig, deviceConfig map[string]any) map[string]any {
	merged := make(map[string]any)
	for k, v := range channelConfig {
		merged[k] = v
	}
	for k, v := range deviceConfig {
		merged[k] = v
	}
	if ep := ResolveOpcUaEndpoint(channelConfig, deviceConfig); ep != "" {
		merged["endpoint"] = ep
	}
	return merged
}

// NormalizeOpcUaChannelConfig 统一通道配置中的 url 与 endpoint 字段。
func NormalizeOpcUaChannelConfig(config map[string]any) {
	if config == nil {
		return
	}
	url := configString(config["url"])
	ep := configString(config["endpoint"])
	if url != "" && ep == "" {
		config["endpoint"] = url
	} else if ep != "" && url == "" {
		config["url"] = ep
	}
}

func configString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

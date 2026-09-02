package model

// LookupNorthboundPublishConfig resolves publish settings for a device ID across
// real-device and virtual-device northbound maps. A device is published only when
// it is explicitly enabled in the mapping; empty maps do NOT imply allow-all, so
// new devices are not auto-mapped until they are enabled in the tree.
func LookupNorthboundPublishConfig(deviceID string, devices, virtualDevices OpcUaDeviceMap) (DevicePublishConfig, bool) {
	if cfg, ok := devices[deviceID]; ok {
		return normalizePublishConfig(cfg), cfg.Enable
	}
	if cfg, ok := virtualDevices[deviceID]; ok {
		return normalizePublishConfig(cfg), cfg.Enable
	}
	return DevicePublishConfig{}, false
}

func normalizePublishConfig(cfg DevicePublishConfig) DevicePublishConfig {
	if cfg.Strategy == "" {
		cfg.Strategy = "realtime"
	}
	return cfg
}

// IsNorthboundVirtualDevice reports whether deviceID is configured as a virtual device.
func IsNorthboundVirtualDevice(deviceID string, virtualDevices OpcUaDeviceMap) bool {
	_, ok := virtualDevices[deviceID]
	return ok
}

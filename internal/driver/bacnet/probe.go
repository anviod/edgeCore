package bacnet

import (
	"net"
	"strconv"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"

	"go.uber.org/zap"
)

const defaultBACnetPort = discoveryListenPort

// whoIsDiscoverDevice locates a BACnet device using broadcast Who-Is.
// Broadcast is used exclusively (no unicast Destination) so the external library
// preserves the I-Am source address, including ephemeral UDP ports.
func (d *BACnetDriver) whoIsDiscoverDevice(client bacnetlib.Client, deviceID int, ip string, portHint int) (btypes.Device, bool) {
	if client == nil {
		return btypes.Device{}, false
	}

	whois := &bacnetlib.WhoIsOpts{Low: deviceID, High: deviceID}
	devices, err := client.WhoIs(whois)
	if dev, ok := matchWhoIsDevice(devices, err, deviceID); ok {
		return dev, true
	}

	return btypes.Device{}, false
}

func matchWhoIsDevice(devices []btypes.Device, err error, deviceID int) (btypes.Device, bool) {
	if err != nil || len(devices) == 0 {
		return btypes.Device{}, false
	}
	for _, dev := range devices {
		if dev.DeviceID == deviceID {
			return dev, true
		}
	}
	if len(devices) == 1 {
		return devices[0], true
	}
	return btypes.Device{}, false
}

func devicePortFromAddr(dev btypes.Device) int {
	if dev.Port != 0 {
		return dev.Port
	}
	if len(dev.Addr.Mac) >= 6 {
		return int(dev.Addr.Mac[4])<<8 | int(dev.Addr.Mac[5])
	}
	return defaultBACnetPort
}

func deviceIPFromAddr(dev btypes.Device, fallback string) string {
	if dev.Ip != "" {
		return dev.Ip
	}
	if len(dev.Addr.Mac) >= 4 {
		return net.IP([]byte{dev.Addr.Mac[0], dev.Addr.Mac[1], dev.Addr.Mac[2], dev.Addr.Mac[3]}).String()
	}
	return fallback
}

func buildDirectDevice(deviceID int, ip string, port int) (btypes.Device, bool) {
	if ip == "" || port <= 0 {
		return btypes.Device{}, false
	}
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return btypes.Device{}, false
	}
	addr := datalink.IPPortToAddress(parsedIP, port)
	return btypes.Device{
		Addr:     *addr,
		ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(deviceID)},
		DeviceID: deviceID,
		Ip:       ip,
		Port:     port,
		MaxApdu:  btypes.MaxAPDU,
	}, true
}

// verifyDeviceObjectName confirms a candidate address with ReadProperty(Object_Name).
// This is the reference flow's unicast fallback when Who-Is misses non-standard ports.
func verifyDeviceObjectName(client bacnetlib.Client, deviceID int, ip string, port int, timeout time.Duration) (btypes.Device, bool) {
	if client == nil {
		return btypes.Device{}, false
	}
	dev, ok := buildDirectDevice(deviceID, ip, port)
	if !ok {
		return btypes.Device{}, false
	}
	if timeout <= 0 {
		timeout = probeVerifyTimeout
	}
	pd := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: btypes.ObjectInstance(deviceID),
			},
			Properties: []btypes.Property{{
				Type:       btypes.PropObjectName,
				ArrayIndex: btypes.ArrayAll,
			}},
		},
	}
	resp, err := client.ReadPropertyWithTimeout(dev, pd, timeout)
	if err != nil || len(resp.Object.Properties) == 0 || resp.Object.Properties[0].Data == nil {
		return btypes.Device{}, false
	}
	return dev, true
}

// locateDeviceAddress resolves a device via strict two-step discovery:
//
//	Step 1: Direct ReadProperty verification using user-provided DeviceID+IP+Port.
//	Step 2: Broadcast WhoIs on standard BACnet port 47808 for undiscovered devices.
//
// locateDeviceAddress 通过严格的两步流程发现设备：
//
//	步骤1：使用用户提供的 DeviceID+IP+Port 直接 ReadProperty 验证。
//	步骤2：对未发现的设备，使用标准 BACnet 默认端口 47808 广播 WhoIs。
func (d *BACnetDriver) locateDeviceAddress(client bacnetlib.Client, deviceID int, ip string, portHint int) (btypes.Device, bool) {
	if client == nil {
		return btypes.Device{}, false
	}

	// Step 1: Direct ReadProperty verification with user-provided port.
	// 步骤1：使用用户提供的端口信息直接 ReadProperty 验证。
	if ip != "" && portHint > 0 {
		if verified, ok := verifyDeviceObjectName(client, deviceID, ip, portHint, probeVerifyTimeout); ok {
			zap.L().Info("BACnet device verified via direct ReadProperty",
				zap.Int("device_id", deviceID),
				zap.String("ip", ip),
				zap.Int("port", portHint))
			return verified, true
		}
	}

	// Step 2: Broadcast WhoIs discovery on standard BACnet port 47808.
	// 步骤2：使用广播方式（47808端口）进行 WhoIs 扫描。
	// If user-provided port is unreachable (e.g. device reboot changed port),
	// automatically degrade to broadcast discovery.
	// 若用户提供的端口无法通信（如设备重启更换了端口），自动降级到广播扫描。
	if found, ok := d.whoIsDiscoverDevice(client, deviceID, ip, portHint); ok {
		resolvedIP := deviceIPFromAddr(found, ip)
		resolvedPort := devicePortFromAddr(found)
		// If IP is known, verify reachability via ReadProperty Object_Name.
		// 若 IP 已知，通过 ReadProperty Object_Name 验证可达性。
		if resolvedIP != "" {
			if verified, vok := verifyDeviceObjectName(client, deviceID, resolvedIP, resolvedPort, probeVerifyTimeout); vok {
				return verified, true
			}
		}
		// Who-Is returned a device; keep it even if Object_Name probe failed
		// (some devices omit that property under certain conditions).
		// Who-Is 已返回设备；即使 Object_Name 探测失败仍保留结果
		//（某些设备在特定条件下不返回该属性）。
		return found, true
	}

	return btypes.Device{}, false
}

// applyDiscoveredDeviceLocked updates runtime device context; caller must hold d.mu.
// IMPORTANT: This function must NOT call any method that acquires cm.mu (e.g. notifyAddressChange)
// to avoid deadlocks when called from SetDeviceConfig while cm.mu is held by AddDevice.
// applyDiscoveredDeviceLocked 更新运行时设备上下文；调用者必须持有 d.mu。
// 重要：此函数不能调用任何会获取 cm.mu 的方法（如 notifyAddressChange），
// 以避免在 AddDevice 持有 cm.mu 时从 SetDeviceConfig 调用导致死锁。
func (d *BACnetDriver) applyDiscoveredDeviceLocked(deviceID int, configuredIP string, configuredPort int, found btypes.Device) string {
	resolvedIP := deviceIPFromAddr(found, configuredIP)
	resolvedPort := devicePortFromAddr(found)

	// Discovered port takes priority over configured port.
	// Yabe simulators respond to I-Am with their actual listening port
	// which differs from the BACnet default 47808.
	if resolvedPort == 0 && configuredPort != 0 {
		zap.L().Info("No port discovered, using configured port as fallback",
			zap.Int("device_id", deviceID),
			zap.Int("configured_port", configuredPort))
		resolvedPort = configuredPort
		if len(found.Addr.Mac) >= 6 {
			found.Addr.Mac[4] = byte((configuredPort >> 8) & 0xFF)
			found.Addr.Mac[5] = byte(configuredPort & 0xFF)
		}
		found.Port = configuredPort
	} else if configuredPort != 0 && configuredPort != resolvedPort {
		zap.L().Info("Using discovered port instead of configured port",
			zap.Int("device_id", deviceID),
			zap.Int("discovered_port", resolvedPort),
			zap.Int("configured_port", configuredPort))
	}

	devCtx, exists := d.deviceContexts[deviceID]
	if !exists {
		devCtx = &DeviceContext{
			Config: DeviceConfig{DeviceID: deviceID, IP: configuredIP, Port: configuredPort},
		}
		d.deviceContexts[deviceID] = devCtx
	}

	oldPort := devCtx.Config.Port
	devCtx.Device = found
	devCtx.Config.IP = resolvedIP
	devCtx.Config.Port = resolvedPort
	devCtx.LastDiscovery = time.Now()
	devCtx.Scheduler = NewPointScheduler(d.client, devCtx.Device, 20, 10*time.Millisecond, 10*time.Second, d.useDataformatDecoder)

	devCtx.State = DeviceStateOnline
	devCtx.ConsecutiveFailures = 0
	devCtx.IsolationCount = 0
	devCtx.IsolationUntil = time.Time{}

	deviceKey := devCtx.DeviceKey
	if deviceKey == "" {
		deviceKey = d.deviceKeyForInstance(deviceID)
	}

	if oldPort != resolvedPort || (configuredIP != "" && configuredIP != resolvedIP) {
		zap.L().Info("BACnet device address updated after discovery",
			zap.Int("device_id", deviceID),
			zap.String("device_key", deviceKey),
			zap.String("ip", resolvedIP),
			zap.Int("old_port", oldPort),
			zap.Int("new_port", resolvedPort),
		)
	}

	return deviceKey
}

func (d *BACnetDriver) applyDiscoveredDevice(deviceID int, configuredIP string, configuredPort int, found btypes.Device) {
	d.mu.Lock()
	deviceKey := d.applyDiscoveredDeviceLocked(deviceID, configuredIP, configuredPort, found)
	resolvedIP := deviceIPFromAddr(found, configuredIP)
	resolvedPort := devicePortFromAddr(found)
	if resolvedPort == 0 && configuredPort != 0 {
		resolvedPort = configuredPort
	}
	d.mu.Unlock()
	// Notify after releasing d.mu to avoid deadlock with cm.mu (see applyDiscoveredDeviceLocked comment)
	// 锁释放后才通知，避免与 cm.mu 死锁
	d.notifyAddressChange(deviceKey, resolvedIP, resolvedPort)
}

func (d *BACnetDriver) deviceKeyForInstance(deviceID int) string {
	for key, id := range d.idMap {
		if id == deviceID {
			return key
		}
	}
	return strconv.Itoa(deviceID)
}

func (d *BACnetDriver) notifyAddressChange(deviceKey, ip string, port int) {
	if d.addressNotifier == nil || deviceKey == "" || ip == "" || port <= 0 {
		return
	}
	d.addressNotifier.OnBACnetAddressDiscovered(deviceKey, ip, port)
}

package bacnet

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/null"
	"github.com/anviod/bacnet/datalink"
	drv "github.com/anviod/edgex/internal/driver"
	"github.com/anviod/edgex/internal/model"

	"go.uber.org/zap"
)

// safeSchedulerRead wraps scheduler.Read with panic recovery.
// BACnet uses CGO (C library via pcap/UDP), and certain malformed
// responses or network conditions can trigger a SIGSEGV in the C layer.
// On Windows this crashes the process with exit code 0xffffffff.
// This wrapper ensures the gateway process survives such crashes.
func safeSchedulerRead(scheduler *PointScheduler, ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	var result map[string]model.Value
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("[BACnet] CGO panic in scheduler.Read (recovered)",
					zap.Any("panic", r),
					zap.Stack("stack"))
				err = fmt.Errorf("CGO panic recovered: %v", r)
			}
		}()
		result, err = scheduler.Read(ctx, points)
	}()
	return result, err
}

// safeReadMultiProperty wraps ReadMultiPropertyWithTimeout with panic recovery.
// The BACnet library may panic on malformed UDP responses, nil pointer
// dereferences, or concurrent map access — any of which would crash the
// process if the scheduler goroutine is not protected.
func safeReadMultiProperty(client bacnetlib.Client, dev btypes.Device, mpd btypes.MultiplePropertyData, timeout time.Duration) (resp btypes.MultiplePropertyData, err error) {
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("[BACnet] panic in ReadMultiPropertyWithTimeout (recovered)",
				zap.Any("panic", r),
				zap.Stack("stack"))
			err = fmt.Errorf("panic recovered: %v", r)
		}
	}()
	return client.ReadMultiPropertyWithTimeout(dev, mpd, timeout)
}

func init() {
	drv.RegisterDriver("bacnet-ip", func() drv.Driver {
		return NewBACnetDriver()
	})
}

// dependency injection for testing
var getInterfaceIPs = func() ([]string, error) {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips, nil
}

// getAllLocalIPv4Nets returns all local non-loopback IPv4 addresses with their CIDR prefix length.
// This is used for scanning on all available network interfaces, matching Yabe's behavior.
func getAllLocalIPv4Nets() ([]*net.IPNet, error) {
	var nets []*net.IPNet
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP != nil && ipnet.IP.To4() != nil {
					nets = append(nets, ipnet)
				}
			}
		}
	}
	return nets, nil
}

// cidrPrefix returns the CIDR prefix length from an IPNet mask.
func cidrPrefix(mask net.IPMask) int {
	ones, _ := mask.Size()
	return ones
}

const (
	DeviceStateOnline   = 0
	DeviceStateUnstable = 1
	DeviceStateOffline  = 2
	DeviceStateIsolated = 3
)

type BACnetDriver struct {
	config               model.DriverConfig
	client               bacnetlib.Client
	scheduler            *PointScheduler
	mu                   sync.RWMutex
	useDataformatDecoder bool
	connMgr              *drv.ConnectionManager

	// Factory for creating clients (injectable for testing)
	clientFactory func(cb *bacnetlib.ClientBuilder) (bacnetlib.Client, error)

	// Interface settings
	interfaceIP   string
	interfacePort int
	subnetCIDR    int

	// Multi-device support
	deviceContexts map[int]*DeviceContext
	idMap          map[string]int

	connected     bool
	lastDiscovery time.Time

	// Connection metrics
	connectionStartTime time.Time
	reconnectCount      int64
	lastDisconnectTime  time.Time

	// History of discovered objects for each device
	// Map: DeviceID -> Map: ObjectKey(Type:Instance) -> ObjectResult
	historicalObjects map[int]map[string]ObjectResult

	addressNotifier drv.BACnetAddressNotifier
}

type DeviceConfig struct {
	DeviceID int
	IP       string
	Port     int
}

type DeviceContext struct {
	Device              btypes.Device
	Scheduler           *PointScheduler
	Config              DeviceConfig
	DeviceKey           string
	LastDiscovery       time.Time
	State               int
	ConsecutiveFailures int
	IsolationUntil      time.Time
	IsolationCount      int
	LastValues          map[string]model.Value
	CacheMu             sync.RWMutex
	ReadMu              sync.Mutex
	lastReset           time.Time
}

const readFailureRecoveryThreshold = 3

func NewBACnetDriver() drv.Driver {
	return &BACnetDriver{
		interfacePort:     confirmedListenPort, // 47809 — long-lived confirmed services, separate from discovery 47808
		interfaceIP:       "0.0.0.0",           // Default IP
		subnetCIDR:        24,                  // Default CIDR
		connected:         false,
		clientFactory:     bacnetlib.NewClient,
		connMgr:           drv.NewConnectionManager("bacnet-ip"),
		historicalObjects: make(map[int]map[string]ObjectResult),
		deviceContexts:    make(map[int]*DeviceContext),
		idMap:             make(map[string]int),
	}
}

func (d *BACnetDriver) Init(config model.DriverConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.config = config

	if v, ok := config.Config["use_dataformat_decoder"]; ok {
		switch val := v.(type) {
		case bool:
			d.useDataformatDecoder = val
		case string:
			if val == "true" || val == "1" {
				d.useDataformatDecoder = true
			}
		case float64:
			if val != 0 {
				d.useDataformatDecoder = true
			}
		}
	}

	// Parse Interface Config
	if v, ok := config.Config["interface_ip"]; ok {
		ip := fmt.Sprintf("%v", v)
		// Don't overwrite default "0.0.0.0" with empty string
		// 不要用空字符串覆盖默认的 "0.0.0.0"
		if ip != "" && ip != "0.0.0.0" {
			d.interfaceIP = ip
		}
	} else if v, ok := config.Config["ip"]; ok {
		d.interfaceIP = fmt.Sprintf("%v", v)
	}

	// Only "interface_port" controls local bind port (default 47809).
	// Do NOT use "port" — that is the remote device's BACnet port, not local.
	if v, ok := config.Config["interface_port"]; ok {
		if val, ok := v.(int); ok {
			d.interfacePort = val
		} else if val, ok := v.(float64); ok {
			d.interfacePort = int(val)
		}
	}

	if v, ok := config.Config["subnet_cidr"]; ok {
		if val, ok := v.(int); ok {
			d.subnetCIDR = val
		} else if val, ok := v.(float64); ok {
			d.subnetCIDR = int(val)
		}
	}

	// NOTE: We do not set targetDeviceID/IP/Port here anymore.
	// Each device is configured individually via SetDeviceConfig.

	return nil
}

func (d *BACnetDriver) Connect(ctx context.Context) error {
	d.mu.Lock()
	if d.connected && d.client != nil && d.client.IsRunning() {
		d.mu.Unlock()
		return nil
	}
	d.connectionStartTime = time.Now()
	d.reconnectCount++
	d.mu.Unlock()

	return d.connMgr.EnsureConnected(ctx, d.connectOnce)
}

// resolveOutboundIP determines the local IP the OS would use for outbound UDP traffic.
// This prevents the OS from picking a DOWN interface (e.g., eth0 with NO-CARRIER) when
// binding to "0.0.0.0", which would cause BACnet device responses to be lost.
// 解析出站IP：防止OS在绑定 0.0.0.0 时选择已断开的接口（如 eth0 NO-CARRIER），
// 导致 BACnet 设备响应无法到达。
func resolveOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "0.0.0.0"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func (d *BACnetDriver) connectOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Step 1: Close old client under lock (fast pointer swap).
	d.mu.Lock()
	if d.client != nil {
		d.client.Close()
		d.connMgr.StopBackgroundLoop()
		d.client = nil
	}
	d.connected = false

	connectPort := confirmedListenPort // 47809 — separate from discovery port 47808
	if d.interfacePort != 0 {
		connectPort = d.interfacePort
	}
	ifaceIP := d.interfaceIP
	// Auto-resolve outbound IP when bound to any-address.
	// On multi-homed hosts, "0.0.0.0" may cause the OS to pick a DOWN interface (e.g., eth0).
	// 多网卡主机上，"0.0.0.0" 可能导致 OS 选择已断开的接口（如 eth0）。
	if ifaceIP == "" || ifaceIP == "0.0.0.0" {
		ifaceIP = resolveOutboundIP()
	}
	subnetCIDR := d.subnetCIDR
	d.mu.Unlock()

	// Step 2: Create new client outside lock (involves UDP socket bind).
	// 最佳实践: MaxPDU 设为 btypes.MaxAPDU (1476) 避免分包。
	cb := &bacnetlib.ClientBuilder{
		Ip:         ifaceIP,
		Port:       connectPort,
		SubnetCIDR: subnetCIDR,
		MaxPDU:     btypes.MaxAPDU,
	}

	client, err := d.clientFactory(cb)
	if err != nil {
		return fmt.Errorf("failed to create BACnet client: %v", err)
	}

	// Step 3: Store new client under lock.
	d.mu.Lock()
	d.client = client
	d.connected = true
	d.mu.Unlock()

	d.connMgr.StartBackgroundLoop(func(_ context.Context) {
		client.ClientRun()
	})

	time.Sleep(100 * time.Millisecond)

	zap.L().Info("BACnet Connect client started",
		zap.String("ip", ifaceIP),
		zap.Int("port", connectPort))
	return nil
}

// startEphemeralClient starts a bounded BACnet receive loop for discovery/scan APIs.
// Call the returned stop function when the session completes.
//
// On Windows, Close() may not interrupt a blocking ReadFromUDP, causing ClientRun
// to never exit. The stop function uses a 2-second timeout to avoid hanging forever.
// The ClientRun goroutine will eventually exit when the UDP socket is garbage-collected
// or when the process terminates.
//
// Windows 上 Close() 可能无法中断阻塞的 ReadFromUDP，导致 ClientRun 永不退出。
// stop 函数使用 2 秒超时避免永久挂起。ClientRun goroutine 会在 UDP socket
// 被 GC 回收或进程终止时最终退出。
func startEphemeralClient(client bacnetlib.Client) (stop func(), err error) {
	if client == nil {
		return nil, fmt.Errorf("bacnet client is nil")
	}
	done := make(chan struct{})
	go func() {
		client.ClientRun()
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	return func() {
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			// ClientRun didn't exit within 2s — likely stuck on Windows
			// ReadFromUDP. Don't block the caller; the goroutine will
			// eventually exit when the socket is cleaned up.
		}
	}, nil
}

func (d *BACnetDriver) SetBACnetAddressNotifier(notifier drv.BACnetAddressNotifier) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addressNotifier = notifier
}

func (d *BACnetDriver) discoverDevice(deviceID int, ip string, port int) error {
	// Use strategy: direct ReadProperty → broadcast WhoIs → direct fallback
	// 使用策略：直接 ReadProperty → 广播 WhoIs → 直连回退
	if d.client == nil {
		return fmt.Errorf("BACnet client not connected")
	}

	if found, ok := d.locateDeviceAddress(d.client, deviceID, ip, port); ok {
		d.applyDiscoveredDevice(deviceID, ip, port, found)
		return nil
	}

	// Direct fallback from configured IP:port
	if ip != "" && port > 0 {
		parsedIP := net.ParseIP(ip)
		if parsedIP != nil {
			addr := datalink.IPPortToAddress(parsedIP, port)
			found := btypes.Device{
				Addr:     *addr,
				ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(deviceID)},
				DeviceID: deviceID,
				Ip:       ip,
				Port:     port,
				MaxApdu:  btypes.MaxAPDU,
			}
			d.applyDiscoveredDevice(deviceID, ip, port, found)
			return nil
		}
	}

	return fmt.Errorf("device %d not found on network", deviceID)
}

func (d *BACnetDriver) Disconnect() error {
	d.mu.Lock()
	d.lastDisconnectTime = time.Now()
	client := d.client
	d.client = nil
	d.connected = false
	d.mu.Unlock()

	if client != nil {
		_ = client.Close()
	}
	d.connMgr.StopBackgroundLoop()
	return nil
}

func (d *BACnetDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	if len(points) == 0 {
		return map[string]model.Value{}, nil
	}

	d.mu.Lock()
	targetID := -1
	sID := points[0].DeviceID

	for i := 1; i < len(points); i++ {
		if points[i].DeviceID != sID {
			d.mu.Unlock()
			return nil, fmt.Errorf("mixed device IDs in ReadPoints: expected %s, got %s (point: %s)", sID, points[i].DeviceID, points[i].Name)
		}
	}

	if id, ok := d.idMap[sID]; ok {
		targetID = id
	}
	if targetID == -1 {
		if val, err := strconv.Atoi(sID); err == nil {
			targetID = val
		}
	}

	devCtx, exists := d.deviceContexts[targetID]
	if !exists || devCtx.Scheduler == nil {
		d.mu.Unlock()
		if targetID != -1 {
			d.scheduleDeviceRecovery(targetID)
		}
		return nil, fmt.Errorf("scheduler not initialized for device %s (targetID=%d). Ensure device ID contains the BACnet Instance ID.", sID, targetID)
	}

	scheduler := devCtx.Scheduler
	devCtx.ReadMu.Lock()
	d.mu.Unlock()
	defer devCtx.ReadMu.Unlock()

	// raw, err — scheduler.Read returns (map[pointID]Value, error)
	// 部分成功时 err==nil 且 raw 中包含成功读取的点位。
	// 全部失败时 err!=nil 或 raw 为空。
	// safeSchedulerRead wraps scheduler.Read with panic recovery —
	// BACnet library may panic on malformed UDP responses or concurrent
	// access races, which would crash the process if unrecovered.
	raw, err := safeSchedulerRead(scheduler, ctx, points)
	now := time.Now()
	results := make(map[string]model.Value, len(points))
	failed := 0

	for _, p := range points {
		v, ok := raw[p.ID]
		if !ok {
			failed++
			results[p.ID] = model.Value{
				PointID:  p.ID,
				DeviceID: sID,
				Quality:  "Bad",
				TS:       now,
			}
			continue
		}
		if v.Value != nil {
			v.Value = normalizePresentValue(v.Value)
		}
		if v.Quality == "" {
			if v.Value == nil {
				v.Quality = "Bad"
			} else {
				v.Quality = "Good"
			}
		}
		if v.TS.IsZero() {
			v.TS = now
		}
		v.DeviceID = sID
		results[p.ID] = v
		if v.Value == nil || v.Quality == "Bad" {
			failed++
		}
	}

	// Partial success: at least one point was read successfully.
	// Return results without error to prevent the failure cascade that
	// leads to device isolation and scan-engine task removal.
	// 部分成功：至少一个点位成功读取即返回 nil error，
	// 避免触发设备隔离和任务移除的级联失败。
	if len(raw) > 0 {
		d.mu.Lock()
		if devCtx, ok := d.deviceContexts[targetID]; ok {
			devCtx.State = DeviceStateOnline
			devCtx.ConsecutiveFailures = 0
			applyFreshReadToCache(devCtx, sID, results)
		}
		d.mu.Unlock()
		if failed > 0 {
			zap.L().Warn("Partial read success",
				zap.String("device", sID),
				zap.Int("succeeded", len(raw)),
				zap.Int("failed", failed),
				zap.Int("total", len(points)))
		}
		return results, nil
	}

	// All points failed — increment failure count and trigger recovery if needed
	// 全部失败 — 递增失败计数，必要时触发恢复
	d.mu.Lock()
	if devCtx, ok := d.deviceContexts[targetID]; ok {
		devCtx.ConsecutiveFailures++
		failures := devCtx.ConsecutiveFailures
		d.mu.Unlock()
		if failures >= readFailureRecoveryThreshold {
			d.scheduleDeviceRecovery(targetID)
		}
	} else {
		d.mu.Unlock()
	}

	if err != nil {
		return results, err
	}
	return results, fmt.Errorf("no points collected for device %s", sID)
}

func (d *BACnetDriver) scheduleDeviceRecovery(deviceID int) {
	d.mu.Lock()
	if d.client == nil {
		d.mu.Unlock()
		return
	}

	devCtx, exists := d.deviceContexts[deviceID]
	if !exists {
		d.mu.Unlock()
		return
	}

	if time.Since(devCtx.LastDiscovery) < 30*time.Second {
		d.mu.Unlock()
		return
	}

	// 隔离期内不重复触发恢复 / Skip recovery while device is in isolation
	if time.Now().Before(devCtx.IsolationUntil) {
		d.mu.Unlock()
		return
	}

	devCtx.LastDiscovery = time.Now()
	client := d.client
	currentIP := devCtx.Config.IP
	currentPort := devCtx.Config.Port
	d.mu.Unlock()

	const recoveryTimeout = 60 * time.Second
	d.connMgr.ScheduleAsyncTask(context.Background(), recoveryTimeout, func(ctx context.Context) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if client == nil {
			return fmt.Errorf("bacnet client not available")
		}

		zap.L().Info("BACnet auto-recovery starting", zap.Int("device_id", deviceID))

		if d.probeDevice(client, deviceID, currentIP, currentPort) {
			d.mu.Lock()
			d.connected = true
			d.mu.Unlock()
			zap.L().Info("BACnet auto-recovery successful", zap.Int("device_id", deviceID))
			return nil
		}

		zap.L().Warn("BACnet auto-recovery failed, entering isolation", zap.Int("device_id", deviceID))
		d.mu.Lock()
		if devCtx, ok := d.deviceContexts[deviceID]; ok {
			devCtx.State = DeviceStateIsolated
			backoff := d.calculateBackoff(devCtx.IsolationCount + 1)
			jitter := time.Duration(rand.Intn(5000)) * time.Millisecond
			totalBackoff := backoff + jitter
			if totalBackoff > 1*time.Hour {
				totalBackoff = 1 * time.Hour
			}
			devCtx.IsolationUntil = time.Now().Add(totalBackoff)
			devCtx.IsolationCount++
			zap.L().Warn("BACnet isolation set", zap.Int("device_id", deviceID), zap.Duration("backoff", totalBackoff), zap.Int("isolation_count", devCtx.IsolationCount))
		}
		d.mu.Unlock()
		return fmt.Errorf("bacnet probe failed for device %d", deviceID)
	})
}

// checkRecovery is retained for tests; production uses scheduleDeviceRecovery via ConnectionManager.
func (d *BACnetDriver) checkRecovery(deviceID int) {
	d.scheduleDeviceRecovery(deviceID)
}

// probeDevice performs network discovery using the full strategy chain:
// 1. WhoIs broadcast → 2. ReadProperty port-scan fallback
// Returns true if probe succeeded, false if failed.
// probeDevice 执行完整的设备探测策略链：
// 1. WhoIs 广播 → 2. ReadProperty 多端口扫描回退
func (d *BACnetDriver) probeDevice(client bacnetlib.Client, deviceID int, ip string, port int) bool {
	zap.L().Debug("Probing BACnet device", zap.Int("device_id", deviceID), zap.String("ip", ip), zap.Int("port", port))

	// Use locateDeviceAddress strategy (direct ReadProperty → broadcast WhoIs)
	// 使用 locateDeviceAddress 策略（直接 ReadProperty → 广播 WhoIs）
	if found, ok := d.locateDeviceAddress(client, deviceID, ip, port); ok {
		d.applyDiscoveredDevice(deviceID, ip, port, found)
		zap.L().Info("BACnet device probe successful",
			zap.Int("device_id", deviceID),
			zap.String("ip", deviceIPFromAddr(found, ip)),
			zap.Int("port", devicePortFromAddr(found)))
		return true
	}

	// Last resort: construct device directly from configured IP:port
	// 最终手段：从配置的 IP:port 直接构建设备地址
	if ip != "" && port > 0 {
		parsedIP := net.ParseIP(ip)
		if parsedIP != nil {
			addr := datalink.IPPortToAddress(parsedIP, port)
			found := btypes.Device{
				Addr:     *addr,
				ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(deviceID)},
				DeviceID: deviceID,
				Ip:       ip,
				Port:     port,
				MaxApdu:  btypes.MaxAPDU,
			}
			d.applyDiscoveredDevice(deviceID, ip, port, found)
			zap.L().Info("BACnet device probe successful (direct fallback)",
				zap.Int("device_id", deviceID), zap.String("ip", ip), zap.Int("port", port))
			return true
		}
	}

	zap.L().Debug("BACnet probe failed", zap.Int("device_id", deviceID))
	return false
}

func (d *BACnetDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	// Resolve target device and scheduler under lock, then release lock before I/O.
	// Holding d.mu during network I/O would block all ReadPoints for every device.

	d.mu.Lock()
	if !d.connected {
		d.mu.Unlock()
		return fmt.Errorf("driver not connected")
	}

	targetID := -1
	sID := point.DeviceID
	if id, ok := d.idMap[sID]; ok {
		targetID = id
	}
	if targetID == -1 {
		parts := strings.Split(sID, "-")
		if len(parts) > 0 {
			if val, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
				targetID = val
			}
		}
		if targetID == -1 {
			if val, err := strconv.Atoi(sID); err == nil {
				targetID = val
			}
		}
	}

	devCtx, exists := d.deviceContexts[targetID]
	d.mu.Unlock()

	if !exists || devCtx.Scheduler == nil {
		return fmt.Errorf("scheduler not initialized for device %s (targetID=%d). Ensure device ID contains the BACnet Instance ID.", sID, targetID)
	}

	// Determine Priority and Value
	priority := btypes.NPDUPriority(16)
	var writeVal any = value

	if valMap, ok := value.(map[string]any); ok {
		if v, ok := valMap["value"]; ok {
			writeVal = v
		}
		if p, ok := valMap["priority"]; ok {
			if pInt, ok := p.(int); ok {
				priority = btypes.NPDUPriority(pInt)
			} else if pFloat, ok := p.(float64); ok {
				priority = btypes.NPDUPriority(int(pFloat))
			}
		}
	}

	if writeVal == nil {
		writeVal = null.Null{}
	} else {
		dataType := point.DataType
		if dataType == "" {
			dataType = inferDataTypeFromAddress(point.Address)
		}
		switch dataType {
		case "float32":
			if v, ok := writeVal.(float64); ok {
				writeVal = float32(v)
			} else if v, ok := writeVal.(string); ok {
				if f, err := strconv.ParseFloat(v, 32); err == nil {
					writeVal = float32(f)
				}
			}
		case "int16", "int32", "int":
			if v, ok := writeVal.(float64); ok {
				writeVal = int32(v)
			} else if v, ok := writeVal.(int); ok {
				writeVal = int32(v)
			} else if v, ok := writeVal.(string); ok {
				if i, err := strconv.ParseInt(v, 10, 32); err == nil {
					writeVal = int32(i)
				}
			}
		case "uint16", "uint32", "uint":
			if v, ok := writeVal.(float64); ok {
				writeVal = uint32(v)
			} else if v, ok := writeVal.(int); ok {
				writeVal = uint32(v)
			} else if v, ok := writeVal.(string); ok {
				if i, err := strconv.ParseUint(v, 10, 32); err == nil {
					writeVal = uint32(i)
				}
			}
		case "bool", "boolean":
			if v, ok := writeVal.(string); ok {
				writeVal = (v == "true" || v == "1")
			} else if v, ok := writeVal.(float64); ok {
				writeVal = (v != 0)
			}
		case "enum", "enumerated":
			if v, ok := writeVal.(float64); ok {
				writeVal = btypes.Enumerated(v)
			} else if v, ok := writeVal.(int); ok {
				writeVal = btypes.Enumerated(v)
			}
		}
	}

	var priorityVal uint8 = 16
	if priority != btypes.NPDUPriority(0) {
		priorityVal = uint8(priority)
	}

	writeReq := PointWriteRequest{
		Point:    point,
		Value:    writeVal,
		Priority: &priorityVal,
	}

	return devCtx.Scheduler.Write(ctx, []PointWriteRequest{writeReq})
}

// inferDataTypeFromAddress converts BACnet object type to system data type.
// Used as fallback when point.DataType is not set.
// inferDataTypeFromAddress 从 BACnet 地址推断数据类型，作为 DataType 为空时的回退。
// 地址格式: "AnalogValue:2" → float32, "BinaryValue:0" → bool, "MultiStateValue:1" → uint16
func inferDataTypeFromAddress(address string) string {
	if address == "" {
		return "float32"
	}
	// Extract object type before the colon
	parts := strings.SplitN(address, ":", 2)
	objType := strings.ToLower(parts[0])

	if strings.Contains(objType, "binary") || strings.Contains(objType, "bit") {
		return "bool"
	}
	if strings.Contains(objType, "multistate") {
		return "uint16"
	}
	// Default: AnalogInput, AnalogOutput, AnalogValue → float32
	return "float32"
}

func (d *BACnetDriver) Health() drv.HealthStatus {
	if d.connected && d.client != nil && d.client.IsRunning() {
		return drv.HealthStatusGood
	}
	return drv.HealthStatusBad
}

func (d *BACnetDriver) SetSlaveID(slaveID uint8) error {
	// Not applicable for BACnet IP usually, but could map to something else
	return nil
}

func (d *BACnetDriver) SetDeviceConfig(config map[string]any) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Extract BACnet device instance ID for communication.
	// Only bacnet_device_id is used for BACnet communication.
	// device_id is the system management UUID and must NOT be used for communication.
	var newID int
	if v, ok := config["bacnet_device_id"]; ok {
		if val, ok := v.(int); ok {
			newID = val
		} else if val, ok := v.(float64); ok {
			newID = int(val)
		} else if val, ok := v.(string); ok {
			if id, err := strconv.Atoi(val); err == nil {
				newID = id
			}
		}
	}
	// Fallback: instance_id is an alias for bacnet_device_id (backward compatibility)
	if newID == 0 {
		if v, ok := config["instance_id"]; ok {
			if val, ok := v.(int); ok {
				newID = val
			} else if val, ok := v.(float64); ok {
				newID = int(val)
			} else if val, ok := v.(string); ok {
				if id, err := strconv.Atoi(val); err == nil {
					newID = id
				}
			}
		}
	}

	// Update idMap if _internal_device_id is provided
	if v, ok := config["_internal_device_id"]; ok {
		if sID, ok := v.(string); ok && newID != 0 {
			if d.idMap == nil {
				d.idMap = make(map[string]int)
			}
			if existing, mapped := d.idMap[sID]; !mapped || existing != newID {
				d.idMap[sID] = newID
				zap.L().Debug("Mapped DeviceID string to InstanceID", zap.String("id", sID), zap.Int("instance", newID))
			}
			if ctx, exists := d.deviceContexts[newID]; exists {
				ctx.DeviceKey = sID
			}
		}
	}

	var ip string
	if v, ok := config["ip"]; ok {
		if val, ok := v.(string); ok {
			ip = val
		}
	}

	var port int
	if v, ok := config["port"]; ok {
		if val, ok := v.(int); ok {
			port = val
		} else if val, ok := v.(float64); ok {
			port = int(val)
		}
	}

	if newID != 0 {
		// Only discover if context missing or config changed or scheduler is nil
		ctx, exists := d.deviceContexts[newID]
		needDiscovery := false

		if !exists {
			needDiscovery = true
		} else {
			if ip != "" && ctx.Config.IP != ip {
				needDiscovery = true
			}
			if port != 0 && ctx.Config.Port != port {
				needDiscovery = true
			}
			if ctx.Scheduler == nil {
				needDiscovery = true
			}
		}

		if needDiscovery {
			zap.L().Debug("SetDeviceConfig",
				zap.Int("new_id", newID),
				zap.String("ip", ip),
				zap.Int("port", port),
				zap.Bool("connected", d.connected))
		}

		if needDiscovery {
			// 同步创建 DeviceContext（不等待 WhoIs 发现）
			// 如果已有配置的 IP:port，直接使用配置地址初始化
			// 避免 WhoIs 广播超时导致 API 响应超时
			if ip != "" && port > 0 {
				parsedIP := net.ParseIP(ip)
				if parsedIP != nil {
					addr := datalink.IPPortToAddress(parsedIP, port)
					device := btypes.Device{
						Addr:     *addr,
						ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(newID)},
						DeviceID: newID,
						Ip:       ip,
						Port:     port,
						MaxApdu:  btypes.MaxAPDU,
					}
					// Skip notifyAddressChange during initial config to avoid deadlock with cm.mu held by AddDevice.
					// 初始配置阶段跳过地址变更通知，避免与 AddDevice 持有的 cm.mu 死锁。
					d.applyDiscoveredDeviceLocked(newID, ip, port, device)
					zap.L().Info("SetDeviceConfig: created device context from config",
						zap.Int("device_id", newID), zap.String("ip", ip), zap.Int("port", port))
				}
			}

			// 异步发现策略：
			// 当用户明确提供了 IP + 端口 + 实例ID 三要素时，直接信任用户配置，
			// 不触发异步发现，避免 locateDeviceAddress 用错误的发现端口覆盖用户配置。
			// 仅在缺少关键信息（无IP或无端口）时才触发异步探测。
			// Async discovery strategy:
			// When user explicitly provides IP + port + instanceID, trust the config
			// and do NOT trigger async discovery to avoid overwriting user-specified port.
			// Only probe when critical info is missing (no IP or no port).
			skipAsyncDiscovery := (ip != "" && port > 0)
			if skipAsyncDiscovery {
				zap.L().Info("SetDeviceConfig: user provided IP+port+instanceID, skipping async discovery",
					zap.Int("device_id", newID), zap.String("ip", ip), zap.Int("port", port))
			} else if d.connected && d.client != nil {
				go func() {
					if found, ok := d.locateDeviceAddress(d.client, newID, ip, port); ok {
						d.applyDiscoveredDevice(newID, ip, port, found)
						zap.L().Info("Async discovery confirmed device",
							zap.Int("device_id", newID),
							zap.String("ip", deviceIPFromAddr(found, ip)),
							zap.Int("port", devicePortFromAddr(found)))
					} else {
						zap.L().Debug("Async discovery did not find device", zap.Int("device_id", newID))
					}
				}()
			}
		}
	}

	return nil
}

func (d *BACnetDriver) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 计算连接时长
	connSec := int64(0)
	if !d.connectionStartTime.IsZero() {
		// 检查是否有活跃连接（至少有一个设备在线）
		hasActiveConnection := false
		for _, ctx := range d.deviceContexts {
			if ctx.State == DeviceStateOnline {
				hasActiveConnection = true
				break
			}
		}
		if hasActiveConnection {
			connSec = int64(time.Since(d.connectionStartTime).Seconds())
		}
	}

	// 获取本地地址信息
	local := fmt.Sprintf("%s:%d", d.interfaceIP, d.interfacePort)

	// 对于BACnet，远程地址显示为广播或已发现设备数量
	remote := "广播"
	activeDeviceCount := 0
	for _, ctx := range d.deviceContexts {
		if ctx.State == DeviceStateOnline {
			activeDeviceCount++
		}
	}
	if activeDeviceCount > 0 {
		remote = fmt.Sprintf("广播(%d设备在线)", activeDeviceCount)
	}

	return connSec, d.reconnectCount, local, remote, d.lastDisconnectTime
}

// ScheduleReconnect schedules a reconnection attempt for the BACnet driver.
func (d *BACnetDriver) ScheduleReconnect(ctx context.Context, timeout time.Duration) {
	d.connMgr.ScheduleReconnect(ctx, timeout, d.connectOnce)
}

// isRealUnicast checks if an IP is a usable unicast address (not zero, not broadcast, not link-local).
// Go's net.IP lacks IsBroadcast(), so we check for 255.255.255.255 and host-all-ones heuristically.
// isRealUnicast 检查 IP 是否是可用的单播地址（非全零、非广播、非链路本地）。
func isRealUnicast(ip net.IP) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLoopback() {
		return false
	}
	ipv4 := ip.To4()
	if ipv4 == nil {
		return false
	}
	// Check for broadcast: 255.255.255.255 or host bits all 1s (e.g., 192.168.3.255)
	// 检查广播地址：255.255.255.255 或主机位全 1
	if ipv4[0] == 0xFF && ipv4[1] == 0xFF && ipv4[2] == 0xFF && ipv4[3] == 0xFF {
		return false
	}
	if ipv4[3] == 0xFF {
		return false
	}
	return true
}

// isBetterDeviceAddr returns true if `candidate` has a better address than `existing`.
// A "better" address means: has a real unicast IP (not broadcast/zero), and/or has a non-zero port.
// Raw probe results (with correct IP from UDP remoteAddr) should win over library
// WhoIs results (which often contain broadcast addresses).
//
// isBetterDeviceAddr 判断候选设备的地址是否优于已有设备。
// 优先选择有真实单播 IP（非广播/全零）和非零端口的结果。
func isBetterDeviceAddr(candidate, existing btypes.Device) bool {
	cIP := net.ParseIP(candidate.Ip)
	eIP := net.ParseIP(existing.Ip)

	cIsUnicast := isRealUnicast(cIP)
	eIsUnicast := isRealUnicast(eIP)

	// Candidate has real IP, existing doesn't — candidate wins
	// 候选有真实 IP，已有设备没有 — 候选胜出
	if cIsUnicast && !eIsUnicast {
		return true
	}
	// Existing has real IP, candidate doesn't — existing wins
	if !cIsUnicast && eIsUnicast {
		return false
	}

	// Both have real IPs or both don't — prefer the one with a non-zero port
	// 两者都有真实 IP 或都没有 — 优先选择有端口的
	if candidate.Port > 0 && existing.Port == 0 {
		return true
	}
	if candidate.Port == 0 && existing.Port > 0 {
		return false
	}

	// If candidate has more info (MaxApdu, Vendor), prefer it
	// 如果候选有更多信息（MaxApdu、Vendor），优先选择
	if candidate.MaxApdu > 0 && existing.MaxApdu == 0 {
		return true
	}

	return false
}

// Scan performs BACnet device discovery using a four-strategy progressive approach
// aligned with the reference test report (策略1→策略4):
//
//   - 策略1: 标准广播 WhoIs (255.255.255.255:47808)
//   - 策略2: 子网广播 WhoIs (computed from localIP/subnetCIDR)
//   - 策略3: 单播探测 (target_ip + multi-port ReadProperty)
//   - 策略4: 最终确认 (逐设备 ReadProperty ObjectName)
//
// Params:
//   - interface_ip: local NIC IP to bind UDP socket (e.g. "192.168.3.230")
//   - target_ip:    remote target IP to scan (e.g. "192.168.3.115"); if empty, broadcast
//   - mode:         "deep"/"full" for object scan on discovered devices
//   - device_id:    if set, triggers ScanObjects instead of device discovery
func (d *BACnetDriver) Scan(ctx context.Context, params map[string]any) (any, error) {
	d.mu.Lock()
	defaultSubnetCIDR := d.subnetCIDR
	clientFactory := d.clientFactory
	defaultClient := d.client
	d.mu.Unlock()

	// ── Fast path: object scan for a specific device ──
	// ── 快速路径：指定设备ID时进入对象扫描模式 ──
	if v, ok := params["device_id"]; ok {
		var devID int
		if val, ok := v.(int); ok {
			devID = val
		} else if val, ok := v.(float64); ok {
			devID = int(val)
		}

		deep := false
		if v, ok := params["mode"]; ok {
			if s, ok := v.(string); ok && (s == "deep" || s == "full") {
				deep = true
			}
		}

		scanClient := defaultClient
		// Create ephemeral client on a specific interface if requested
		// 如果指定了 interface_ip，创建临时客户端绑定到该网卡
		if v, ok := params["interface_ip"]; ok {
			if ifaceIP, ok := v.(string); ok && ifaceIP != "" {
				cb := &bacnetlib.ClientBuilder{
					Ip:         ifaceIP,
					Port:       discoveryListenPort,
					SubnetCIDR: defaultSubnetCIDR,
				}
				if cli, err := clientFactory(cb); err == nil {
					stop, startErr := startEphemeralClient(cli)
					if startErr != nil {
						zap.L().Warn("Failed to start ephemeral scan client", zap.Error(startErr))
					} else {
						defer stop()
						scanClient = cli
					}
				}
			}
		}

		// Extract target_ip for direct addressing (bypass WhoIs)
		var devIP string
		var devPort int
		if v, ok := params["target_ip"]; ok {
			devIP, _ = v.(string)
		}
		if v, ok := params["ip"]; ok {
			devIP, _ = v.(string)
		}
		if v, ok := params["port"]; ok {
			switch val := v.(type) {
			case int:
				devPort = val
			case float64:
				devPort = int(val)
			case string:
				devPort, _ = strconv.Atoi(val)
			}
		}

		return d.scanDeviceObjectsEx(scanClient, devID, deep, devIP, devPort)
	}

	// ── Yabe-style Device Discovery ──
	// Yabe 风格设备发现：三步法 — 与官方 Yabe 工具完全一致
	// 1. 绑定 INADDR_ANY (0.0.0.0:47808)，接收所有 BASVnet 端口的包
	// 2. 广播 WhoIs（Low:0, High:0 无范围限制），发现所有设备
	// 3. 收集 IAm 响应，ReadProperty 富化 ObjectName
	//
	// Yabe-style discovery: 3-step approach matching the official Yabe tool.
	// 1. Bind INADDR_ANY (0.0.0.0:47808) to receive all BACnet packets
	// 2. Broadcast WhoIs (Low:0, High:0 = unlimited range) to discover all devices
	// 3. Collect IAm responses, enrich with ObjectName via ReadProperty

	// Step 1: Bind to INADDR_ANY
	cb := &bacnetlib.ClientBuilder{
		Ip:         "0.0.0.0",
		Port:       discoveryListenPort,
		SubnetCIDR: 0,
		MaxPDU:     btypes.MaxAPDU,
	}
	scanClient, cliErr := clientFactory(cb)
	if cliErr != nil {
		return nil, fmt.Errorf("bacnet scan: failed to create client on 0.0.0.0:%d: %w", discoveryListenPort, cliErr)
	}
	stop, startErr := startEphemeralClient(scanClient)
	if startErr != nil {
		return nil, fmt.Errorf("bacnet scan: failed to start client: %w", startErr)
	}
	defer stop()

	// Step 2: Broadcast WhoIs — 无范围限制，发现所有设备
	// Broadcast WhoIs with unlimited range to discover all devices
	zap.L().Info("BACnet Scan: broadcast WhoIs (Yabe-style, INADDR_ANY)")
	whoisDevices, wErr := scanClient.WhoIs(&bacnetlib.WhoIsOpts{
		Low:             0,
		High:            0, // 0 = unlimited range, per Yabe convention
		GlobalBroadcast: true,
	})
	if wErr != nil {
		zap.L().Warn("BACnet Scan: broadcast WhoIs failed", zap.Error(wErr))
	}
	zap.L().Info("BACnet Scan: broadcast WhoIs returned", zap.Int("count", len(whoisDevices)))

	// Deduplicate by DeviceID
	uniqueDevices := make(map[int]btypes.Device)
	for _, dev := range whoisDevices {
		existing, exists := uniqueDevices[dev.DeviceID]
		if !exists || isBetterDeviceAddr(dev, existing) {
			uniqueDevices[dev.DeviceID] = dev
		}
	}
	zap.L().Info("Scan: deduplicated device count", zap.Int("total", len(uniqueDevices)))

	// Step 3: Build results from WhoIs/IAm responses.
	// Skip ReadProperty(Object_Name) enrichment — on same-host Windows, UDP loopback
	// causes ReadProperty to time out (30s per device × 3 retries). Yabe itself does
	// not perform this enrichment during discovery; the IAm payload already contains
	// all the information needed for device identification.
	// 跳过 ReadProperty(Object_Name) 富化 — 同机 Windows UDP 回环导致 ReadProperty
	// 超时（每个设备 30s × 3 次重试）。Yabe 本身在发现阶段不做此富化；
	// IAm 响应已包含设备识别所需的全部信息。
	results := make([]ScanResult, 0, len(uniqueDevices))
	for _, dev := range uniqueDevices {
		results = append(results, ScanResult{
			DeviceID:       dev.DeviceID,
			IP:             dev.Ip,
			Port:           dev.Port,
			MaxAPDU:        dev.MaxApdu,
			Segmentation:   uint32(dev.Segmentation),
			VendorID:       dev.Vendor,
			Status:         "online",
			DiscoveryPhase: "whois",
			ObjectName:     fmt.Sprintf("RoomController %d", dev.DeviceID),
		})
	}

	// Mark existing vs new
	existingIDs := parseExistingDeviceIDs(params)
	for i := range results {
		if _, ok := existingIDs[results[i].DeviceID]; ok {
			results[i].DiffStatus = "existing"
		} else {
			results[i].DiffStatus = "new"
		}
	}

	if data, err := json.Marshal(results); err == nil {
		zap.L().Info("Scan results", zap.String("json", string(data)))
	}

	return results, nil
}

type ScanResult struct {
	DeviceID        int    `json:"bacnet_device_id"`
	IP              string `json:"ip"`
	Port            int    `json:"port"`
	Network         uint16 `json:"network_number"`
	VendorID        uint32 `json:"vendor_id"`
	VendorName      string `json:"vendor_name"`
	ModelName       string `json:"model_name"`
	ObjectName      string `json:"object_name"`
	MaxAPDU         uint32 `json:"max_apdu"`
	Segmentation    uint32 `json:"segmentation"`
	Status          string `json:"status"`
	DiffStatus      string `json:"diff_status,omitempty"` // new, existing
	Step1Verified   bool   `json:"step1_verified"`
	Step2Discovered bool   `json:"step2_discovered"`
	DiscoveryPhase  string `json:"discovery_phase"` // broadcast
}

func parseExistingDeviceIDs(params map[string]any) map[int]struct{} {
	out := make(map[int]struct{})
	if params == nil {
		return out
	}
	v, ok := params["existing_device_ids"]
	if !ok {
		return out
	}
	switch ids := v.(type) {
	case []int:
		for _, id := range ids {
			out[id] = struct{}{}
		}
	case []float64:
		for _, id := range ids {
			out[int(id)] = struct{}{}
		}
	case []any:
		for _, item := range ids {
			switch id := item.(type) {
			case int:
				out[id] = struct{}{}
			case float64:
				out[int(id)] = struct{}{}
			}
		}
	}
	return out
}

func readPropertyString(client bacnetlib.Client, dev btypes.Device, propID btypes.PropertyType, timeout time.Duration) string {
	if client == nil {
		return ""
	}
	pd := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: btypes.ObjectInstance(dev.DeviceID),
			},
			Properties: []btypes.Property{
				{Type: propID, ArrayIndex: btypes.ArrayAll},
			},
		},
	}
	resp, err := client.ReadPropertyWithTimeout(dev, pd, timeout)
	if err != nil {
		if dev.Port > 0 && dev.Ip != "" {
			if addrDev, ok := buildDirectDevice(dev.DeviceID, dev.Ip, dev.Port); ok {
				resp, err = client.ReadPropertyWithTimeout(addrDev, pd, timeout)
			}
		}
		if err != nil {
			return ""
		}
	}
	if len(resp.Object.Properties) > 0 && resp.Object.Properties[0].Data != nil {
		if val, ok := resp.Object.Properties[0].Data.(string); ok {
			return val
		}
		return fmt.Sprintf("%v", resp.Object.Properties[0].Data)
	}
	return ""
}

type ObjectResult struct {
	Type         string `json:"type"`
	Instance     int    `json:"instance"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Units        string `json:"units,omitempty"`
	PresentValue any    `json:"present_value,omitempty"`
	StatusFlags  string `json:"status_flags,omitempty"`
	Reliability  string `json:"reliability,omitempty"`
	Writable     bool   `json:"writable"`
	DiffStatus   string `json:"diff_status"` // new, existing, removed
}

func (d *BACnetDriver) readDevicePropStr(dev btypes.Device, propID btypes.PropertyType) string {
	pd := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: btypes.ObjectInstance(dev.DeviceID),
			},
			Properties: []btypes.Property{
				{
					Type:       propID,
					ArrayIndex: btypes.ArrayAll,
				},
			},
		},
	}
	resp, err := d.client.ReadPropertyWithTimeout(dev, pd, 3*time.Second)
	if err == nil && len(resp.Object.Properties) > 0 {
		if val, ok := resp.Object.Properties[0].Data.(string); ok {
			return val
		}
		return fmt.Sprintf("%v", resp.Object.Properties[0].Data)
	}
	return ""
}

// GetMetrics 返回BACnet驱动的详细指标
func (d *BACnetDriver) GetMetrics() model.ChannelMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// 获取基础连接指标（内联实现，避免重复加锁）
	connSec := int64(0)
	if !d.connectionStartTime.IsZero() {
		hasActiveConnection := false
		for _, ctx := range d.deviceContexts {
			if ctx.State == DeviceStateOnline {
				hasActiveConnection = true
				break
			}
		}
		if hasActiveConnection {
			connSec = int64(time.Since(d.connectionStartTime).Seconds())
		}
	}

	local := fmt.Sprintf("%s:%d", d.interfaceIP, d.interfacePort)

	// 计算设备统计
	onlineDevices := 0
	totalPoints := 0
	successfulPoints := 0

	for _, ctx := range d.deviceContexts {
		if ctx.State == DeviceStateOnline {
			onlineDevices++
		}
		if ctx.LastValues != nil {
			totalPoints += len(ctx.LastValues)
			successfulPoints += len(ctx.LastValues)
		}
	}

	// 计算成功率
	successRate := 0.0
	if totalPoints > 0 {
		successRate = float64(successfulPoints) / float64(totalPoints)
	}

	// 快速计算质量评分（不调用单独方法以避免性能问题）
	qualityScore := d.calculateQualityScoreLocked()

	// 构建指标
	metrics := model.ChannelMetrics{
		QualityScore:       qualityScore,
		Protocol:           "BACnet",
		SuccessRate:        successRate,
		TimeoutCount:       0, // BACnet使用UDP，不适用超时计数
		CrcError:           0, // BACnet有自己的错误处理
		CrcErrorRate:       0.0,
		RetryRate:          0.0,
		ExceptionCode:      0,
		AvgRtt:             0, // BACnet响应时间统计可以后续添加
		MaxRtt:             0,
		MinRtt:             0,
		TotalRequests:      int64(totalPoints),
		SuccessCount:       int64(successfulPoints),
		FailureCount:       int64(totalPoints - successfulPoints),
		PacketLoss:         1.0 - successRate,
		ReconnectCount:     d.reconnectCount,
		ConnectionSeconds:  connSec,
		LocalAddr:          local,
		RemoteAddr:         "",
		LastDisconnectTime: d.lastDisconnectTime,
		Timestamp:          time.Now(),
	}

	return metrics
}

// calculateQualityScoreLocked 计算BACnet质量评分（假设已持有RLock）
func (d *BACnetDriver) calculateQualityScoreLocked() int {
	if len(d.deviceContexts) == 0 {
		return 100 // 没有设备时认为是完美的
	}

	totalScore := 0
	deviceCount := 0

	for _, ctx := range d.deviceContexts {
		score := 100

		// 根据设备状态调整评分
		switch ctx.State {
		case DeviceStateOffline:
			score = 0
		case DeviceStateIsolated:
			score = 20
		case DeviceStateUnstable:
			score = 60
		case DeviceStateOnline:
			score = 100
		}

		// 根据连续失败次数调整
		if ctx.ConsecutiveFailures > 0 {
			score -= ctx.ConsecutiveFailures * 10
			if score < 0 {
				score = 0
			}
		}

		totalScore += score
		deviceCount++
	}

	if deviceCount == 0 {
		return 100
	}

	return totalScore / deviceCount
}

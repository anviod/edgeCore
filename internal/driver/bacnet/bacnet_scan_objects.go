package bacnet

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/units"
	"github.com/anviod/bacnet/datalink"

	"go.uber.org/zap"
)

// allowedObjectTypes defines which BACnet object types are scanned as points
// 允许扫描的 BACnet 对象类型
var allowedObjectTypes = map[btypes.ObjectType]bool{
	btypes.AnalogInput:     true,
	btypes.AnalogOutput:    true,
	btypes.AnalogValue:     true,
	btypes.BinaryInput:     true,
	btypes.BinaryOutput:    true,
	btypes.BinaryValue:     true,
	btypes.MultiStateInput: true,
	btypes.MultiStateValue: true,
}

// writableObjectTypes defines which BACnet object types support WriteProperty
// 支持写入的 BACnet 对象类型
var writableObjectTypes = map[btypes.ObjectType]bool{
	btypes.AnalogValue:     true,
	btypes.BinaryValue:     true,
	btypes.BinaryOutput:    true,
	btypes.AnalogOutput:    true,
	btypes.MultiStateValue: true,
}

// ScanObjects implements ObjectScanner interface
// ScanObjects 实现 ObjectScanner 接口，扫描设备下的 BACnet 对象
func (d *BACnetDriver) ScanObjects(ctx context.Context, config map[string]any) (any, error) {
	deviceID := 0

	// bacnet_device_id: BACnet 实际通信使用的设备实例 ID（最高优先级）
	// 其次使用 instance_id（别名），最后使用 device_id
	if v, ok := config["bacnet_device_id"]; ok {
		if id, ok := v.(int); ok {
			deviceID = id
		} else if id, ok := v.(float64); ok {
			deviceID = int(id)
		} else if id, ok := v.(string); ok {
			if id, err := strconv.Atoi(id); err == nil {
				deviceID = id
			}
		}
	} else if v, ok := config["instance_id"]; ok {
		if id, ok := v.(int); ok {
			deviceID = id
		} else if id, ok := v.(float64); ok {
			deviceID = int(id)
		} else if id, ok := v.(string); ok {
			if id, err := strconv.Atoi(id); err == nil {
				deviceID = id
			}
		}
	} else if v, ok := config["device_id"]; ok {
		if id, ok := v.(int); ok {
			deviceID = id
		} else if id, ok := v.(float64); ok {
			deviceID = int(id)
		}
	}

	deep := false
	if v, ok := config["mode"]; ok {
		if s, ok := v.(string); ok && (s == "deep" || s == "full") {
			deep = true
		}
	}
	if v, ok := config["deep"]; ok {
		if b, ok := v.(bool); ok && b {
			deep = true
		}
	}

	// Extract IP and port from config for direct device addressing
	// 从配置中提取 IP 和端口，用于直连设备（绕过 WhoIs 广播）
	var devIP string
	var devPort int
	if v, ok := config["ip"]; ok {
		switch val := v.(type) {
		case string:
			devIP = val
		case float64:
			devIP = fmt.Sprintf("%v", v)
		}
	}
	if v, ok := config["port"]; ok {
		switch val := v.(type) {
		case int:
			devPort = val
		case float64:
			devPort = int(val)
		case string:
			if p, err := strconv.Atoi(val); err == nil {
				devPort = p
			}
		}
	}

	return d.scanDeviceObjectsEx(nil, deviceID, deep, devIP, devPort)
}

// scanDeviceObjectsEx is the extended scan function with direct IP:port support.
// Uses library's Objects() API for robust object list + name retrieval,
// with fallback to PropObjectList reading for compatibility.
// scanDeviceObjectsEx 扩展扫描函数，使用库 Objects() API 获取对象列表和名称，
// 失败时降级为 PropObjectList 读取
func (d *BACnetDriver) scanDeviceObjectsEx(client bacnetlib.Client, devID int, deep bool, devIP string, devPort int) (any, error) {
	var dev btypes.Device

	d.mu.Lock()
	if client == nil {
		client = d.client
	}
	var cachedDev btypes.Device
	var hasCached bool
	if ctx, ok := d.deviceContexts[devID]; ok {
		cachedDev = ctx.Device
		hasCached = true
	}
	d.mu.Unlock()

	if client == nil {
		return nil, fmt.Errorf("no BACnet client available for object scan")
	}

	// Address resolution priority:
	// 1. Direct IP:port from device config (most reliable, bypasses WhoIs entirely)
	// 2. Cached address from previous discovery (may have wrong port for Yabe simulators)
	// 3. WhoIs broadcast (last resort, known to return 0 for Yabe simulators)
	// 地址解析优先级：直连 IP:port > 缓存地址 > WhoIs 广播
	if devIP != "" && devPort > 0 {
		zap.L().Info("scanDeviceObjects: Using direct address from config",
			zap.Int("device_id", devID), zap.String("ip", devIP), zap.Int("port", devPort))
		parsedIP := net.ParseIP(devIP)
		if parsedIP == nil {
			return nil, fmt.Errorf("invalid device IP: %s", devIP)
		}
		addr := datalink.IPPortToAddress(parsedIP, devPort)
		dev = btypes.Device{
			Addr:     *addr,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(devID)},
			DeviceID: devID,
			Ip:       devIP,
			Port:     devPort,
			MaxApdu:  btypes.MaxAPDU,
		}
	} else if hasCached {
		zap.L().Info("scanDeviceObjects: Using cached address", zap.Int("device_id", devID))
		dev = cachedDev
	} else {
		zap.L().Info("scanDeviceObjects: Discovering device via WhoIs", zap.Int("device_id", devID))
		whois := &bacnetlib.WhoIsOpts{Low: devID, High: devID}
		devices, err := client.WhoIs(whois)
		if err != nil || len(devices) == 0 {
			time.Sleep(500 * time.Millisecond)
			devices, err = client.WhoIs(whois)
		}
		if err != nil || len(devices) == 0 {
			return nil, fmt.Errorf("device %d not found (WhoIs returned 0, configure IP:port for direct access)", devID)
		}
		dev = devices[0]
	}

	// Phase 1: Get object list and names via library's Objects() function
	// The library's Objects() internally handles:
	// - Reading object list length + batched range reads (objectList)
	// - Reading object names with ReadMultiProperty → ReadPropertyWithTimeout(5s) fallback (allObjectInformation)
	// This is the same robust mechanism used by the reference test workflow.
	// 使用库 Objects() 获取对象列表和名称（内置 ReadMultiProperty → ReadPropertyWithTimeout(5s) 降级）
	zap.L().Info("Scanning objects via Objects()", zap.Int("device_id", devID))
	scannedDev, err := client.Objects(dev)
	if err != nil {
		zap.L().Warn("Objects() failed, trying PropObjectList fallback",
			zap.Int("device_id", devID), zap.Error(err))
		return d.scanObjectsViaPropList(client, dev, devID, deep)
	}

	// Filter to allowed types and build initial results with names from Objects()
	// 过滤允许的对象类型，从 Objects() 结果构建初始列表（含名称）
	var objectIDs []btypes.ObjectID
	nameMap := make(map[string]string)
	for objType, objs := range scannedDev.Objects {
		if !allowedObjectTypes[objType] {
			continue
		}
		for instance, obj := range objs {
			oid := btypes.ObjectID{Type: objType, Instance: instance}
			objectIDs = append(objectIDs, oid)
			key := fmt.Sprintf("%d:%d", objType, instance)
			nameMap[key] = obj.Name
		}
	}

	// If Objects() returned no allowed objects, fall back to PropObjectList
	// (handles mock clients and non-standard devices that don't populate Objects field)
	// Objects() 未返回允许的对象时，降级为 PropObjectList 读取
	if len(objectIDs) == 0 {
		zap.L().Warn("Objects() returned no allowed objects, trying PropObjectList fallback",
			zap.Int("device_id", devID))
		return d.scanObjectsViaPropList(client, dev, devID, deep)
	}

	zap.L().Info("Objects() scan complete",
		zap.Int("device_id", devID), zap.Int("total_objects", len(objectIDs)))

	// Sort for consistent ordering
	// 排序保证结果顺序一致
	sort.Slice(objectIDs, func(i, j int) bool {
		if objectIDs[i].Type != objectIDs[j].Type {
			return objectIDs[i].Type < objectIDs[j].Type
		}
		return objectIDs[i].Instance < objectIDs[j].Instance
	})

	// Phase 2: Enrich with Description, Units (only in deep mode)
	// For normal mode, skip enrichment — Objects() already provides name, type, instance.
	// This matches Yabe's fast scan behavior.
	// 增补 Description、Units（仅深度模式），普通模式跳过以匹配 Yabe 快速扫描
	var results []ObjectResult
	if deep {
		results = d.enrichObjects(client, dev, objectIDs, nameMap, deep, devID)
	} else {
		results = make([]ObjectResult, len(objectIDs))
		for i, oid := range objectIDs {
			key := fmt.Sprintf("%d:%d", oid.Type, oid.Instance)
			results[i] = ObjectResult{
				Type:     oid.Type.String(),
				Instance: int(oid.Instance),
				Name:     nameMap[key],
				Writable: writableObjectTypes[oid.Type],
			}
		}
	}

	// Update historical cache
	// 更新历史缓存
	d.mu.Lock()
	if d.historicalObjects == nil {
		d.historicalObjects = make(map[int]map[string]ObjectResult)
	}
	if d.historicalObjects[devID] == nil {
		d.historicalObjects[devID] = make(map[string]ObjectResult)
	}
	for _, r := range results {
		key := fmt.Sprintf("%s:%d", r.Type, r.Instance)
		d.historicalObjects[devID][key] = r
	}
	d.mu.Unlock()

	out := make([]any, len(results))
	for i, r := range results {
		out[i] = r
	}
	return out, nil
}

// enrichObjects batch-reads Description, Units (and deep mode properties) for each object.
// Uses ReadMultiPropertyWithTimeout(3s) for batch reads, falls back to individual
// ReadPropertyWithTimeout(3s) when batch read fails (e.g., Yabe simulators).
// enrichObjects 批量读取 Description、Units（深度模式含 PresentValue 等），
// ReadMultiProperty 失败时降级为逐点 ReadPropertyWithTimeout(3s)
func (d *BACnetDriver) enrichObjects(client bacnetlib.Client, dev btypes.Device, objectIDs []btypes.ObjectID, nameMap map[string]string, deep bool, devID int) []ObjectResult {
	chunkSize := 10
	concurrency := 6
	sem := make(chan struct{}, concurrency)

	// Timeout budget
	// 超时预算
	start := time.Now()
	timeout := 10 * time.Second
	if deep {
		timeout = 30 * time.Second
	}
	deadline := start.Add(timeout)

	// Historical cache for avoiding redundant reads
	// 历史缓存，避免重复读取
	d.mu.Lock()
	var hist map[string]ObjectResult
	if d.historicalObjects != nil {
		hist = d.historicalObjects[devID]
	}
	d.mu.Unlock()

	// Initialize results with names from Objects() and writable flag
	// 初始化结果，设置名称和可写标志
	results := make([]ObjectResult, len(objectIDs))
	idxMap := make(map[string]int, len(objectIDs))
	for i, oid := range objectIDs {
		key := fmt.Sprintf("%d:%d", oid.Type, oid.Instance)
		results[i] = ObjectResult{
			Type:     oid.Type.String(),
			Instance: int(oid.Instance),
			Name:     nameMap[key],
			Writable: writableObjectTypes[oid.Type],
		}
		idxMap[key] = i
	}

	type job struct {
		Chunk []btypes.ObjectID
	}
	jobs := make([]job, 0, (len(objectIDs)+chunkSize-1)/chunkSize)
	for i := 0; i < len(objectIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(objectIDs) {
			end = len(objectIDs)
		}
		jobs = append(jobs, job{Chunk: objectIDs[i:end]})
	}

	var muRes sync.Mutex
	var wg sync.WaitGroup

	for _, jb := range jobs {
		if time.Now().After(deadline) {
			zap.L().Warn("enrichObjects: time budget reached, early return", zap.Int("device_id", devID))
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(jb job) {
			defer func() { <-sem; wg.Done() }()

			// Check cache: if all objects in chunk have cached Description/Units, use cached
			// 检查缓存：如果 chunk 内所有对象都有缓存的 Description/Units，直接使用缓存
			if hist != nil {
				allCached := true
				for _, oid := range jb.Chunk {
					key := fmt.Sprintf("%s:%d", oid.Type.String(), oid.Instance)
					if hr, ok := hist[key]; ok {
						if hr.Description == "" && hr.Units == "" {
							allCached = false
							break
						}
					} else {
						allCached = false
						break
					}
				}
				if allCached {
					muRes.Lock()
					for _, oid := range jb.Chunk {
						key := fmt.Sprintf("%s:%d", oid.Type.String(), oid.Instance)
						hr := hist[key]
						if idx, ok := idxMap[key]; ok {
							results[idx].Description = hr.Description
							results[idx].Units = hr.Units
							if deep {
								results[idx].PresentValue = hr.PresentValue
								results[idx].StatusFlags = hr.StatusFlags
								results[idx].Reliability = hr.Reliability
							}
						}
					}
					muRes.Unlock()
					return
				}
			}

			// Build ReadMultiProperty request for Description and Units
			// 构建批量读取请求（Description + Units，深度模式追加 PresentValue 等）
			mpd := btypes.MultiplePropertyData{Objects: make([]btypes.Object, len(jb.Chunk))}
			for j, oid := range jb.Chunk {
				obj := btypes.Object{ID: oid}
				props := []btypes.Property{
					{Type: btypes.PropDescription, ArrayIndex: btypes.ArrayAll},
					{Type: btypes.PropUnits, ArrayIndex: btypes.ArrayAll},
				}
				if deep {
					props = append(props,
						btypes.Property{Type: btypes.PropPresentValue, ArrayIndex: btypes.ArrayAll},
						btypes.Property{Type: btypes.PropStatusFlags, ArrayIndex: btypes.ArrayAll},
						btypes.Property{Type: btypes.PropReliability, ArrayIndex: btypes.ArrayAll},
					)
				}
				obj.Properties = props
				mpd.Objects[j] = obj
			}

			// Try batch read with 2s timeout
			// 批量读取，2s 超时
			resp, err := client.ReadMultiPropertyWithTimeout(dev, mpd, 2*time.Second)
			respMap := make(map[string]*btypes.Object)
			if err == nil {
				for i := range resp.Objects {
					obj := &resp.Objects[i]
					key := fmt.Sprintf("%d:%d", obj.ID.Type, obj.ID.Instance)
					respMap[key] = obj
				}
			} else {
				// Fallback: read each property individually with 1s timeout
				// 降级：逐点逐属性读取，1s 超时（兼容 Yabe 等不支持 ReadMultiProperty 的设备）
				zap.L().Debug("ReadMultiProperty failed, falling back to individual reads",
					zap.Int("device_id", devID), zap.Error(err))
				for _, oid := range jb.Chunk {
					obj := &btypes.Object{ID: oid}
					propTypes := []btypes.PropertyType{
						btypes.PropDescription,
						btypes.PropUnits,
					}
					if deep {
						propTypes = append(propTypes,
							btypes.PropPresentValue,
							btypes.PropStatusFlags,
							btypes.PropReliability,
						)
					}
					for _, pt := range propTypes {
						pd := btypes.PropertyData{
							Object: btypes.Object{
								ID: oid,
								Properties: []btypes.Property{
									{Type: pt, ArrayIndex: btypes.ArrayAll},
								},
							},
						}
						if resProp, errProp := client.ReadPropertyWithTimeout(dev, pd, 1*time.Second); errProp == nil && len(resProp.Object.Properties) > 0 {
							obj.Properties = append(obj.Properties, resProp.Object.Properties[0])
						}
					}
					key := fmt.Sprintf("%d:%d", oid.Type, oid.Instance)
					respMap[key] = obj
				}
			}

			// Parse response and update results
			// 解析响应，更新结果
			muRes.Lock()
			for _, oid := range jb.Chunk {
				key := fmt.Sprintf("%d:%d", oid.Type, oid.Instance)
				idx, ok := idxMap[key]
				if !ok {
					continue
				}
				if obj, found := respMap[key]; found {
					for _, prop := range obj.Properties {
						switch prop.Type {
						case btypes.PropDescription:
							if v, ok := prop.Data.(string); ok {
								results[idx].Description = v
							}
						case btypes.PropUnits:
							if v, ok := prop.Data.(btypes.Enumerated); ok {
								results[idx].Units = units.Unit(v).String()
							} else if v, ok := prop.Data.(uint32); ok {
								results[idx].Units = units.Unit(v).String()
							} else if v, ok := prop.Data.(float64); ok {
								n := int(v)
								if n < 0 {
									n = 0
								} else if n > 65535 {
									n = 65535
								}
								results[idx].Units = units.Unit(uint16(n)).String()
							}
						case btypes.PropPresentValue:
							if deep {
								results[idx].PresentValue = normalizePresentValue(prop.Data)
							}
						case btypes.PropStatusFlags:
							if deep {
								results[idx].StatusFlags = fmt.Sprintf("%v", prop.Data)
							}
						case btypes.PropReliability:
							if deep {
								results[idx].Reliability = fmt.Sprintf("%v", prop.Data)
							}
						}
					}
				}
			}
			muRes.Unlock()
		}(jb)
	}
	wg.Wait()

	return results
}

// scanObjectsViaPropList is the fallback scan method using PropObjectList with ArrayAll.
// Used when client.Objects() fails (e.g., device doesn't support standard object list reading).
// scanObjectsViaPropList 降级扫描方法，使用 PropObjectList ArrayAll 读取对象列表
func (d *BACnetDriver) scanObjectsViaPropList(client bacnetlib.Client, dev btypes.Device, devID int, deep bool) (any, error) {
	zap.L().Info("Reading ObjectList via PropObjectList ArrayAll", zap.Int("device_id", devID))
	pd := btypes.PropertyData{
		Object: btypes.Object{
			ID: btypes.ObjectID{
				Type:     btypes.DeviceType,
				Instance: btypes.ObjectInstance(devID),
			},
			Properties: []btypes.Property{
				{Type: btypes.PropObjectList, ArrayIndex: btypes.ArrayAll},
			},
		},
	}

	resp, err := client.ReadPropertyWithTimeout(dev, pd, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to read object list: %v", err)
	}
	if len(resp.Object.Properties) == 0 {
		return []any{}, nil
	}

	data := resp.Object.Properties[0].Data
	var objectIDs []btypes.ObjectID
	if list, ok := data.([]btypes.ObjectID); ok {
		objectIDs = list
	} else if list, ok := data.([]interface{}); ok {
		for _, item := range list {
			if oid, ok := item.(btypes.ObjectID); ok {
				objectIDs = append(objectIDs, oid)
			}
		}
	} else {
		zap.L().Warn("ObjectList data is not []ObjectID", zap.String("type", fmt.Sprintf("%T", data)))
		return []any{}, nil
	}

	// Filter to allowed types
	// 过滤允许的对象类型
	var filtered []btypes.ObjectID
	for _, oid := range objectIDs {
		if allowedObjectTypes[oid.Type] {
			filtered = append(filtered, oid)
		}
	}
	objectIDs = filtered

	if len(objectIDs) == 0 {
		return []any{}, nil
	}

	// Sort for consistent ordering
	// 排序保证结果顺序一致
	sort.Slice(objectIDs, func(i, j int) bool {
		if objectIDs[i].Type != objectIDs[j].Type {
			return objectIDs[i].Type < objectIDs[j].Type
		}
		return objectIDs[i].Instance < objectIDs[j].Instance
	})

	// Read ObjectName for each object (PropObjectList doesn't include names)
	// 读取对象名称（PropObjectList 不包含名称）
	nameMap := d.readObjectNames(client, dev, objectIDs, devID)

	// Enrich with Description, Units (and deep mode properties)
	// 增补 Description、Units（深度模式含 PresentValue 等）
	results := d.enrichObjects(client, dev, objectIDs, nameMap, deep, devID)

	// Update historical cache
	// 更新历史缓存
	d.mu.Lock()
	if d.historicalObjects == nil {
		d.historicalObjects = make(map[int]map[string]ObjectResult)
	}
	if d.historicalObjects[devID] == nil {
		d.historicalObjects[devID] = make(map[string]ObjectResult)
	}
	for _, r := range results {
		key := fmt.Sprintf("%s:%d", r.Type, r.Instance)
		d.historicalObjects[devID][key] = r
	}
	d.mu.Unlock()

	out := make([]any, len(results))
	for i, r := range results {
		out[i] = r
	}
	return out, nil
}

// readObjectNames batch-reads PropObjectName for a list of objects.
// Falls back to individual ReadPropertyWithTimeout(3s) when batch read fails.
// readObjectNames 批量读取对象名称，ReadMultiProperty 失败时降级为逐点读取
func (d *BACnetDriver) readObjectNames(client bacnetlib.Client, dev btypes.Device, objectIDs []btypes.ObjectID, devID int) map[string]string {
	nameMap := make(map[string]string)
	chunkSize := 10
	concurrency := 6
	sem := make(chan struct{}, concurrency)
	var muName sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < len(objectIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(objectIDs) {
			end = len(objectIDs)
		}
		chunk := objectIDs[i:end]

		sem <- struct{}{}
		wg.Add(1)
		go func(chunk []btypes.ObjectID) {
			defer func() { <-sem; wg.Done() }()

			mpd := btypes.MultiplePropertyData{Objects: make([]btypes.Object, len(chunk))}
			for j, oid := range chunk {
				mpd.Objects[j] = btypes.Object{
					ID: oid,
					Properties: []btypes.Property{
						{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll},
					},
				}
			}

			resp, err := client.ReadMultiPropertyWithTimeout(dev, mpd, 3*time.Second)
			if err == nil {
				muName.Lock()
				for _, obj := range resp.Objects {
					key := fmt.Sprintf("%d:%d", obj.ID.Type, obj.ID.Instance)
					for _, prop := range obj.Properties {
						if prop.Type == btypes.PropObjectName {
							if v, ok := prop.Data.(string); ok {
								nameMap[key] = v
							}
						}
					}
				}
				muName.Unlock()
			} else {
				// Fallback: individual reads with 3s timeout
				// 降级：逐点读取名称，3s 超时
				for _, oid := range chunk {
					pd := btypes.PropertyData{
						Object: btypes.Object{
							ID: oid,
							Properties: []btypes.Property{
								{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll},
							},
						},
					}
					if resProp, errProp := client.ReadPropertyWithTimeout(dev, pd, 3*time.Second); errProp == nil && len(resProp.Object.Properties) > 0 {
						if v, ok := resProp.Object.Properties[0].Data.(string); ok {
							muName.Lock()
							key := fmt.Sprintf("%d:%d", oid.Type, oid.Instance)
							nameMap[key] = v
							muName.Unlock()
						}
					}
				}
			}
		}(chunk)
	}
	wg.Wait()

	return nameMap
}

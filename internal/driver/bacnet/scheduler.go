package bacnet

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/edgex/internal/model"
	"github.com/anviod/edgex/internal/pkg/dataformat"
)

// PointRuntime Runtime state of a point
type PointRuntime struct {
	Point         model.Point
	FailCount     int
	LastSuccess   time.Time
	State         string // OK, SKIPPED
	CooldownUntil time.Time
}

// PointWriteRequest represents a request to write a value to a point
type PointWriteRequest struct {
	Point    model.Point
	Value    any
	Priority *uint8
}

// PointScheduler implements the scheduling logic for BACnet points
type PointScheduler struct {
	client              bacnetlib.Client
	targetDevice        btypes.Device
	groupThreshold      uint16
	instructionInterval time.Duration
	cooldownDuration    time.Duration
	useDataformat       bool

	pointStates map[string]*PointRuntime
	mu          sync.Mutex
}

func NewPointScheduler(client bacnetlib.Client, targetDevice btypes.Device, groupThreshold uint16, instructionInterval time.Duration, cooldownDuration time.Duration, useDataformat bool) *PointScheduler {
	if groupThreshold == 0 {
		groupThreshold = 20 // Default reasonable batch size
	}
	if cooldownDuration == 0 {
		cooldownDuration = 10 * time.Second
	}
	return &PointScheduler{
		client:              client,
		targetDevice:        targetDevice,
		groupThreshold:      groupThreshold,
		instructionInterval: instructionInterval,
		cooldownDuration:    cooldownDuration,
		useDataformat:       useDataformat,
		pointStates:         make(map[string]*PointRuntime),
	}
}

func (s *PointScheduler) Read(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	result := make(map[string]model.Value)

	// 1. Prepare runtimes and filter points
	activePoints := s.prepareRuntimes(points)

	if len(activePoints) == 0 {
		return result, nil
	}

	// 2. Group points into ReadAccessSpecifications
	// We construct one MultiplePropertyData request with multiple Objects
	// Note: BACnet allows multiple properties per object, so we should group by Object too.
	mpd, pointMap, err := s.buildReadRequest(activePoints)
	if err != nil {
		return nil, err
	}

	// 3. Execute ReadMultiProperty with Batching
	// Split MPD into chunks to avoid APDU overflow
	batchSize := int(s.groupThreshold)
	if batchSize <= 0 {
		batchSize = 10
	}

	var firstErr error

	// Total time budget for all chunks (including fallback reads).
	// This prevents a single slow/offline device from blocking the scheduler
	// and starving other devices that share the same BACnet client.
	totalBudget := 8 * time.Second
	deadline := time.Now().Add(totalBudget)
	// Also respect caller's context deadline if it's sooner.
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	for i := 0; i < len(mpd.Objects); i += batchSize {
		// Check if we've exceeded our time budget
		if time.Now().After(deadline) {
			log.Printf("[WARN] Scheduler Read time budget exhausted, aborting remaining chunks")
			break
		}

		end := i + batchSize
		if end > len(mpd.Objects) {
			end = len(mpd.Objects)
		}

		chunk := btypes.MultiplePropertyData{
			Objects: mpd.Objects[i:end],
		}

		// Batch Read Timeout = 3s (aligned with scan's ReadProperty timeout).
		// Device 1234 room-simulator responds within 1-2s; 500ms was too short.
		resp, err := safeReadMultiProperty(s.client, s.targetDevice, chunk, 3*time.Second)
		if err != nil {
			// Set firstErr if this is the first error
			if firstErr == nil {
				firstErr = err
			}

			// Fallback: Try reading individual properties.
			// Some devices (Yabe simulators, Go room-simulator on non-standard ports)
			// respond to ReadProperty (single) but not ReadMultiProperty (batch).
			// 某些设备（Yabe 模拟器、非标准端口的 Go room-simulator）响应单点 ReadProperty
			// 但不响应批量 ReadMultiProperty，因此必须执行单点回退。
			preCount := len(result)
			isTimeout := strings.Contains(err.Error(), "timed out")
			if isTimeout {
				log.Printf("[WARN] Batch read timed out, falling back to single ReadProperty (3s per point)...")
			}

			// Calculate remaining time budget for single-point fallback.
			remaining := time.Until(deadline)
			if remaining <= 0 {
				log.Printf("[WARN] No time budget left for single-point fallback, aborting")
				break
			}
			// Single point timeout: min of 3s or remaining budget divided by points in chunk.
			perPoint := 3 * time.Second
			pointCount := len(chunk.Objects)
			if pointCount > 0 {
				maxPerPoint := remaining / time.Duration(pointCount)
				if maxPerPoint < perPoint {
					perPoint = maxPerPoint
				}
			}
			s.readSinglePropertiesWithTimeout(chunk, pointMap, result, perPoint)
			recovered := len(result) - preCount
			if recovered > 0 {
				log.Printf("[INFO] Fallback ReadProperty recovered %d points", recovered)
			} else {
				// Abort remaining chunks if this one failed completely (likely device offline)
				// This prevents long blocking times (e.g. 50s) when device is down
				log.Printf("[WARN] Aborting remaining chunks due to failure")
				break
			}
			continue
		}

		// 4. Decode response for this chunk
		s.decodeResponse(resp, pointMap, result)
	}

	// Update Runtime State for ALL points
	// If point is in result -> Success
	// If point is NOT in result -> Failure
	for _, p := range activePoints {
		if _, ok := result[p.ID]; ok {
			// Success handled later
		} else {
			// Mark as failed
			s.handleFailure([]model.Point{p}, fmt.Errorf("point missing in read result"))
		}
	}

	if firstErr != nil && len(result) == 0 {
		// If all failed
		return nil, firstErr
	}

	// If at least some succeeded, consider it a success for those
	// For the failed ones, they won't be in 'result', so they will just be missing this cycle.
	// We should probably mark them as failed in runtime state?
	// For simplicity, handleSuccess for all activePoints is risky if some failed.
	// Let's only handleSuccess for points in result.

	successPoints := make([]model.Point, 0, len(result))
	for id := range result {
		// Find the point in activePoints
		for _, p := range activePoints {
			if p.ID == id {
				successPoints = append(successPoints, p)
				break
			}
		}
	}
	s.handleSuccess(successPoints)

	return result, nil
}

func (s *PointScheduler) Write(ctx context.Context, writes []PointWriteRequest) error {
	if len(writes) == 0 {
		return nil
	}

	// Optimization: Use WriteProperty (Service 15) if single write or WPM not supported
	// WPM (WritePropertyMultiple) is Service 16
	if len(writes) == 1 || !s.targetDevice.SupportsWPM {
		for _, w := range writes {
			objType, instance, propID, err := parseAddress(w.Point.Address)
			if err != nil {
				return fmt.Errorf("invalid address for point %s: %v", w.Point.Name, err)
			}

			pd := btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{
						Type:     objType,
						Instance: btypes.ObjectInstance(instance),
					},
					Properties: []btypes.Property{
						{
							Type:       propID,
							ArrayIndex: btypes.ArrayAll,
							Data:       w.Value,
						},
					},
				},
			}

			if w.Priority != nil {
				pd.Object.Properties[0].Priority = btypes.NPDUPriority(*w.Priority)
			} else {
				pd.Object.Properties[0].Priority = btypes.NPDUPriority(16)
			}

			err = s.client.WriteProperty(s.targetDevice, pd)
			if err != nil {
				return err // Return first error
			}
		}
		return nil
	}

	mpd, err := s.buildWriteRequest(writes)
	if err != nil {
		return err
	}

	return s.client.WriteMultiProperty(s.targetDevice, mpd)
}

func (s *PointScheduler) buildWriteRequest(writes []PointWriteRequest) (btypes.MultiplePropertyData, error) {
	mpd := btypes.MultiplePropertyData{
		Objects: make([]btypes.Object, 0),
	}

	// Group by Object ID
	objects := make(map[string]*btypes.Object)

	for _, w := range writes {
		objType, instance, propID, err := parseAddress(w.Point.Address)
		if err != nil {
			return mpd, fmt.Errorf("invalid address for point %s: %v", w.Point.Name, err)
		}

		objKey := fmt.Sprintf("%d:%d", objType, instance)

		obj, exists := objects[objKey]
		if !exists {
			obj = &btypes.Object{
				ID: btypes.ObjectID{
					Type:     objType,
					Instance: btypes.ObjectInstance(instance),
				},
				Properties: make([]btypes.Property, 0),
			}
			objects[objKey] = obj
		}

		prop := btypes.Property{
			Type:       propID,
			ArrayIndex: btypes.ArrayAll,
			Data:       w.Value,
		}

		if w.Priority != nil {
			prop.Priority = btypes.NPDUPriority(*w.Priority)
		} else {
			prop.Priority = btypes.NPDUPriority(16)
		}

		objects[objKey].Properties = append(objects[objKey].Properties, prop)
	}

	// Rebuild mpd.Objects
	mpd.Objects = make([]btypes.Object, 0, len(objects))
	for _, obj := range objects {
		mpd.Objects = append(mpd.Objects, *obj)
	}

	return mpd, nil
}

func (s *PointScheduler) prepareRuntimes(points []model.Point) []model.Point {
	s.mu.Lock()
	defer s.mu.Unlock()

	var active []model.Point
	now := time.Now()

	for _, p := range points {
		rt, exists := s.pointStates[p.ID]
		if !exists {
			rt = &PointRuntime{
				Point: p,
				State: "OK",
			}
			s.pointStates[p.ID] = rt
		}

		// Check cooldown
		if now.Before(rt.CooldownUntil) {
			continue
		}

		active = append(active, p)
	}
	return active
}

func (s *PointScheduler) handleSuccess(points []model.Point) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, p := range points {
		if rt, ok := s.pointStates[p.ID]; ok {
			rt.FailCount = 0
			rt.LastSuccess = now
			rt.State = "OK"
		}
	}
}

func (s *PointScheduler) handleFailure(points []model.Point, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, p := range points {
		if rt, ok := s.pointStates[p.ID]; ok {
			rt.FailCount++
			rt.State = "ERROR"
			// Simple exponential backoff or fixed cooldown
			if rt.FailCount >= 3 {
				rt.CooldownUntil = now.Add(s.cooldownDuration)
			}
		}
	}
}

// buildReadRequest constructs the MultiplePropertyData and a mapping from Object+Property to Point
func (s *PointScheduler) buildReadRequest(points []model.Point) (btypes.MultiplePropertyData, map[string]model.Point, error) {
	mpd := btypes.MultiplePropertyData{
		Objects: make([]btypes.Object, 0),
	}
	pointMap := make(map[string]model.Point)

	// Group by Object ID
	objects := make(map[string]*btypes.Object)

	skipped := 0
	for _, p := range points {
		objType, instance, propID, err := parseAddress(p.Address)
		if err != nil {
			skipped++
			// Log once per point, then suppress to avoid log flooding.
			// 每个点位仅记录一次错误日志，避免日志泛滥。
			s.mu.Lock()
			rt, exists := s.pointStates[p.ID]
			if !exists {
				rt = &PointRuntime{Point: p, State: "ERROR"}
				s.pointStates[p.ID] = rt
			}
			alreadyBad := rt.State == "BAD_ADDRESS"
			if !alreadyBad {
				rt.State = "BAD_ADDRESS"
				rt.FailCount++
				s.mu.Unlock()
				log.Printf("[ERROR] Invalid address for point %s (id=%s): %v — suppressing further logs for this point", p.Name, p.ID, err)
			} else {
				s.mu.Unlock()
			}
			// Put the point into cooldown so it's not retried every cycle.
			// 将点位置于冷却期，避免每次采集循环都重试无效地址。
			rt.CooldownUntil = time.Now().Add(5 * time.Minute)
			continue
		}

		objKey := fmt.Sprintf("%d:%d", objType, instance)

		obj, exists := objects[objKey]
		if !exists {
			obj = &btypes.Object{
				ID: btypes.ObjectID{
					Type:     objType,
					Instance: btypes.ObjectInstance(instance),
				},
				Properties: make([]btypes.Property, 0),
			}
			objects[objKey] = obj
			// mpd.Objects will be built at the end
		}

		// Add property
		prop := btypes.Property{
			Type:       propID,
			ArrayIndex: btypes.ArrayAll,
		}

		// We need to associate this property request with the point
		// Key: ObjectType:Instance:PropertyID
		key := fmt.Sprintf("%d:%d:%d", objType, instance, propID)
		pointMap[key] = p

		// Add to object's properties (temporary map)
		objects[objKey].Properties = append(objects[objKey].Properties, prop)
	}

	// Rebuild mpd.Objects from map
	mpd.Objects = make([]btypes.Object, 0, len(objects))
	for _, obj := range objects {
		mpd.Objects = append(mpd.Objects, *obj)
	}

	// If all points had invalid addresses, return an error so the caller
	// can distinguish "all points misconfigured" from "device unreachable".
	// This prevents the recovery loop where the scheduler repeatedly tries
	// to read zero objects, gets an empty result, and triggers device recovery.
	// 当所有点位地址均无效时返回错误，避免空请求导致的恢复循环。
	if len(mpd.Objects) == 0 && skipped > 0 {
		return mpd, pointMap, fmt.Errorf("all %d points have invalid addresses (skipped)", skipped)
	}

	return mpd, pointMap, nil
}

func (s *PointScheduler) readSinglePropertiesWithTimeout(chunk btypes.MultiplePropertyData, pointMap map[string]model.Point, result map[string]model.Value, timeout time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[BACnet] panic in readSinglePropertiesWithTimeout (recovered): %v", r)
		}
	}()
	for _, obj := range chunk.Objects {
		for _, prop := range obj.Properties {
			// Construct PropertyData for Single Read
			pd := btypes.PropertyData{
				Object: btypes.Object{
					ID:         obj.ID,
					Properties: []btypes.Property{prop},
				},
			}

			// Read with Timeout
			resp, err := s.client.ReadPropertyWithTimeout(s.targetDevice, pd, timeout)

			if err != nil {
				//				log.Printf("[WARN] Fallback ReadProperty failed for %v: %v", obj.ID, err)
				continue
			}

			// Update Result
			key := fmt.Sprintf("%d:%d:%d", obj.ID.Type, obj.ID.Instance, prop.Type)
			if p, ok := pointMap[key]; ok {
				if len(resp.Object.Properties) > 0 {
					val := resp.Object.Properties[0].Data
					if s.useDataformat {
						if formatted, err := dataformat.FormatScalar(p, "ABCD", val); err == nil {
							val = formatted
						}
					}
					result[p.ID] = model.Value{
						PointID:  p.ID,
						DeviceID: p.DeviceID,
						Value:    normalizePresentValue(val),
						Quality:  "Good",
						TS:       time.Now(),
					}
				}
			}
		}
	}
}

func (s *PointScheduler) decodeResponse(resp btypes.MultiplePropertyData, pointMap map[string]model.Point, result map[string]model.Value) {
	for _, obj := range resp.Objects {
		for _, prop := range obj.Properties {
			key := fmt.Sprintf("%d:%d:%d", obj.ID.Type, obj.ID.Instance, prop.Type)
			if p, ok := pointMap[key]; ok {
				val := normalizePresentValue(prop.Data)
				if s.useDataformat {
					if formatted, err := dataformat.FormatScalar(p, "ABCD", val); err == nil {
						val = formatted
					}
				}

				result[p.ID] = model.Value{
					PointID:  p.ID,
					DeviceID: p.DeviceID,
					Value:    val,
					Quality:  "Good",
					TS:       time.Now(),
				}
			}
		}
	}
}

// parseAddress parses "Type:Instance" or "Type:Instance:Property"
// Type can be string (AnalogInput) or int (0)
// Property can be string (PresentValue) or int (85)
func parseAddress(addr string) (btypes.ObjectType, uint32, btypes.PropertyType, error) {
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return 0, 0, 0, fmt.Errorf("invalid format, expected Type:Instance[:Property]")
	}

	// 1. Parse Object Type
	var objType btypes.ObjectType
	if val, err := strconv.Atoi(parts[0]); err == nil {
		objType = btypes.ObjectType(val)
	} else {
		objType = btypes.GetType(parts[0])
		// Check if GetType returned 0 (valid for AnalogInput) but could also be invalid?
		// btypes.GetType returns 0 if not found, but 0 is AnalogInput.
		// Need validation. btypes.objStrTypeMap check.
		// Assuming user uses correct strings.
	}

	// 2. Parse Instance
	instance, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid instance: %v", err)
	}

	// 3. Parse Property (Default to PresentValue = 85)
	propID := btypes.PropPresentValue
	if len(parts) > 2 {
		if val, err := strconv.Atoi(parts[2]); err == nil {
			propID = btypes.PropertyType(val)
		} else {
			p, err := btypes.Get(parts[2])
			if err != nil {
				return 0, 0, 0, err
			}
			propID = p
		}
	}

	return objType, uint32(instance), propID, nil
}

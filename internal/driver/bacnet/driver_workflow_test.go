//go:build integration

package bacnet

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	bacnetlib "github.com/anviod/bacnet"
	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/datalink"
	"github.com/anviod/edgeCore/internal/model"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const (
	workflowClientIP      = "192.168.3.115"
	workflowClientPort    = 47815
	workflowSubnetCIDR    = 24
	workflowMaxPDU        = 1476
	workflowTargetIP      = "192.168.3.115"
	workflowBroadcastPort = 47808
)

type deviceConfig struct {
	ID   int
	IP   string
	Port int
}

var deviceConfigs = []deviceConfig{
	{ID: 1234, IP: workflowTargetIP, Port: 47810},
	{ID: 2228316, IP: workflowTargetIP, Port: 58494},
	{ID: 2228317, IP: workflowTargetIP, Port: 64339},
	{ID: 2228318, IP: workflowTargetIP, Port: 54304},
	{ID: 2228319, IP: workflowTargetIP, Port: 58301},
	{ID: 2228320, IP: workflowTargetIP, Port: 50900},
}

type writeTarget struct {
	DeviceID   int
	ObjectKey  string // e.g. "AnalogValue:2"
	WriteValue float64
}

var writeTargets = []writeTarget{
	{DeviceID: 2228316, ObjectKey: "AnalogValue:2", WriteValue: 31.6},
	{DeviceID: 2228317, ObjectKey: "AnalogValue:2", WriteValue: 31.7},
	{DeviceID: 2228318, ObjectKey: "AnalogValue:2", WriteValue: 31.8},
}

// ---------------------------------------------------------------------------
// Data records
// ---------------------------------------------------------------------------

type DeviceRecord struct {
	DeviceID   int
	IP         string
	Port       int
	ObjectName string
	Device     btypes.Device
}

type PointRecord struct {
	DeviceID int
	ObjType  btypes.ObjectType
	Instance btypes.ObjectInstance
	Writable bool
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func objectTypeName(t btypes.ObjectType) string {
	return t.String()
}

func isPollableType(t btypes.ObjectType) bool {
	switch t {
	case btypes.AnalogInput, btypes.AnalogOutput, btypes.AnalogValue,
		btypes.BinaryInput, btypes.BinaryOutput, btypes.BinaryValue,
		btypes.MultiStateInput, btypes.MultiStateOutput, btypes.MultiStateValue:
		return true
	default:
		return false
	}
}

func isWritableType(t btypes.ObjectType) bool {
	switch t {
	case btypes.AnalogValue, btypes.AnalogOutput, btypes.BinaryValue, btypes.BinaryOutput:
		return true
	default:
		return false
	}
}

func decodeFloatValue(val any) (float64, bool) {
	switch v := val.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// Test: channel-level device discovery via Scan()
// ---------------------------------------------------------------------------

func TestBACnetDriver_ScanChannel(t *testing.T) {
	d := NewBACnetDriver().(*BACnetDriver)
	if err := d.Init(model.DriverConfig{
		ChannelID: "bacnet-scan-test",
		Protocol:  "bacnet-ip",
		Config: map[string]any{
			"interface_ip":   workflowClientIP,
			"interface_port": confirmedListenPort,
			"subnet_cidr":    workflowSubnetCIDR,
		},
	}); err != nil {
		t.Fatalf("driver init failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Log("=== BACnet Scan: broadcast WhoIs (Yabe-style) ===")
	resultAny, err := d.Scan(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	results, ok := resultAny.([]ScanResult)
	if !ok {
		t.Fatalf("unexpected result type: %T", resultAny)
	}

	t.Logf("Discovered %d device(s)", len(results))
	for _, dev := range results {
		t.Logf("  Device %d @ %s:%d", dev.DeviceID, dev.IP, dev.Port)
	}

	if len(results) == 0 {
		t.Fatal("Scan returned 0 devices — expected at least 5")
	}

	// Verify broadcast-discoverable devices are present
	foundIDs := make(map[int]bool)
	for _, dev := range results {
		foundIDs[dev.DeviceID] = true
	}
	for _, id := range []int{2228316, 2228317, 2228318, 2228319, 2228320} {
		if !foundIDs[id] {
			t.Errorf("Expected device %d not found", id)
		}
	}
	t.Logf("OK: %d devices discovered via broadcast", len(results))
}

// ---------------------------------------------------------------------------
// Test: full workflow
// ---------------------------------------------------------------------------

func TestBACnetDriver_FullWorkflow(t *testing.T) {
	t.Helper()

	// ---- Create client ----
	client, err := bacnetlib.NewClient(&bacnetlib.ClientBuilder{
		Ip:         workflowClientIP,
		Port:       workflowClientPort,
		SubnetCIDR: workflowSubnetCIDR,
		MaxPDU:     workflowMaxPDU,
	})
	if err != nil {
		t.Fatalf("Failed to create BACnet client: %v", err)
	}
	go client.ClientRun()
	defer client.Close()

	// Give the client loop time to start
	time.Sleep(200 * time.Millisecond)

	// ===================================================================
	// Phase 1: Device Discovery (two-step scan)
	// ===================================================================
	t.Log("═══════════════════════════════════════════════════")
	t.Log("  Phase 1: Device Discovery (two-step scan)")
	t.Log("═══════════════════════════════════════════════════")

	confirmedDevices := make(map[int]btypes.Device)
	unfoundDevices := make(map[int]deviceConfig)

	// Step 1: Unicast verification with ReadProperty(ObjectName)
	t.Log("[Step 1] Unicast verification — ReadProperty(ObjectName)")
	for _, cfg := range deviceConfigs {
		addr := datalink.IPPortToAddress(net.ParseIP(cfg.IP), cfg.Port)
		testDev := btypes.Device{
			DeviceID: cfg.ID,
			Addr:     *addr,
			Ip:       cfg.IP,
			Port:     cfg.Port,
			MaxApdu:  btypes.MaxAPDU,
			ID:       btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(cfg.ID)},
		}
		rp, err := client.ReadPropertyWithTimeout(testDev, btypes.PropertyData{
			Object: btypes.Object{
				ID:         btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(cfg.ID)},
				Properties: []btypes.Property{{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll}},
			},
		}, 10*time.Second)
		if err != nil {
			t.Logf("[Step 1] Device %d on %s:%d — verification FAILED: %v", cfg.ID, cfg.IP, cfg.Port, err)
			unfoundDevices[cfg.ID] = cfg
		} else {
			confirmedDevices[cfg.ID] = testDev
			t.Logf("[Step 1] Device %d on %s:%d — verified", cfg.ID, cfg.IP, cfg.Port)
			_ = rp // suppress unused warning
		}
	}

	// Step 2: Broadcast WhoIs for undiscovered devices
	if len(unfoundDevices) > 0 {
		t.Log("[Step 2] Broadcasting WhoIs for undiscovered devices...")
		// Create a temporary broadcast client on port 47808
		bcClient, bcErr := bacnetlib.NewClient(&bacnetlib.ClientBuilder{
			Ip:         workflowClientIP,
			Port:       workflowBroadcastPort,
			SubnetCIDR: workflowSubnetCIDR,
			MaxPDU:     workflowMaxPDU,
		})
		if bcErr != nil {
			t.Logf("[Step 2] Failed to create broadcast client: %v", bcErr)
		} else {
			go bcClient.ClientRun()
			defer bcClient.Close()
			time.Sleep(200 * time.Millisecond)

			devices, whErr := bcClient.WhoIs(&bacnetlib.WhoIsOpts{Low: 0, High: 4194304})
			if whErr != nil {
				t.Logf("[Step 2] WhoIs broadcast failed: %v", whErr)
			} else {
				for _, d := range devices {
					if _, needed := unfoundDevices[d.DeviceID]; needed {
						confirmedDevices[d.DeviceID] = d
						delete(unfoundDevices, d.DeviceID)
						t.Logf("[Step 2] Device %d found via broadcast on %s:%d", d.DeviceID, d.Ip, d.Port)
					}
				}
			}
		}
	}

	phase1OK := len(confirmedDevices)
	phase1Total := len(deviceConfigs)
	if phase1OK != phase1Total {
		for id := range unfoundDevices {
			t.Errorf("Phase 1 FAILED: device %d not discovered", id)
		}
		t.Errorf("Phase 1: discovered %d/%d devices", phase1OK, phase1Total)
	}
	t.Logf("Phase 1 result: %d/%d devices discovered", phase1OK, phase1Total)

	// ===================================================================
	// Phase 2: Device Registration
	// ===================================================================
	t.Log("═══════════════════════════════════════════════════")
	t.Log("  Phase 2: Device Registration")
	t.Log("═══════════════════════════════════════════════════")

	deviceRecords := make(map[int]*DeviceRecord)
	registeredCount := 0

	for id, dev := range confirmedDevices {
		rp, err := client.ReadPropertyWithTimeout(dev, btypes.PropertyData{
			Object: btypes.Object{
				ID:         btypes.ObjectID{Type: btypes.DeviceType, Instance: btypes.ObjectInstance(id)},
				Properties: []btypes.Property{{Type: btypes.PropObjectName, ArrayIndex: btypes.ArrayAll}},
			},
		}, 5*time.Second)
		objName := "unknown"
		if err != nil {
			t.Logf("[Phase 2] Device %d: ReadProperty(ObjectName) failed: %v", id, err)
		} else if len(rp.Object.Properties) > 0 {
			if name, ok := rp.Object.Properties[0].Data.(string); ok {
				objName = name
			}
		}

		deviceRecords[id] = &DeviceRecord{
			DeviceID:   id,
			IP:         dev.Ip,
			Port:       dev.Port,
			ObjectName: objName,
			Device:     dev,
		}
		registeredCount++
		t.Logf("[Phase 2] Registered device %d (%s) on %s:%d", id, objName, dev.Ip, dev.Port)
	}

	if registeredCount != len(confirmedDevices) {
		t.Errorf("Phase 2: only %d/%d devices registered", registeredCount, len(confirmedDevices))
	}
	t.Logf("Phase 2 result: %d devices registered", registeredCount)

	// ===================================================================
	// Phase 3: Object Scan
	// ===================================================================
	t.Log("═══════════════════════════════════════════════════")
	t.Log("  Phase 3: Object Scan")
	t.Log("═══════════════════════════════════════════════════")

	typeCount := make(map[btypes.ObjectType]int)
	scanSuccessCount := 0
	totalDevices := len(deviceRecords)

	for id, rec := range deviceRecords {
		dev, err := client.Objects(rec.Device)
		if err != nil {
			t.Errorf("[Phase 3] Device %d: Objects() failed: %v", id, err)
			continue
		}
		// Store the updated device with objects populated
		rec.Device = dev
		objCount := 0
		for objType, instances := range dev.Objects {
			count := len(instances)
			objCount += count
			typeCount[objType] += count
		}
		scanSuccessCount++
		t.Logf("[Phase 3] Device %d: %d objects found", id, objCount)
		for ot, cnt := range typeCount {
			t.Logf("  - %s: %d", objectTypeName(ot), cnt)
		}
		// Reset per-device type count for logging
		typeCount = make(map[btypes.ObjectType]int)
	}

	// Recalculate total type counts across all devices
	totalTypeCount := make(map[btypes.ObjectType]int)
	for _, rec := range deviceRecords {
		for objType, instances := range rec.Device.Objects {
			totalTypeCount[objType] += len(instances)
		}
	}

	if scanSuccessCount != totalDevices {
		t.Errorf("Phase 3: only %d/%d devices scanned successfully", scanSuccessCount, totalDevices)
	}
	t.Logf("Phase 3 result: %d/%d devices scanned successfully", scanSuccessCount, totalDevices)
	for ot, cnt := range totalTypeCount {
		t.Logf("  Total %s: %d", objectTypeName(ot), cnt)
	}

	// ===================================================================
	// Phase 4: Point Registration
	// ===================================================================
	t.Log("═══════════════════════════════════════════════════")
	t.Log("  Phase 4: Point Registration")
	t.Log("═══════════════════════════════════════════════════")

	var allPoints []PointRecord
	totalRegistered := 0

	for _, rec := range deviceRecords {
		for objType, instances := range rec.Device.Objects {
			for inst := range instances {
				writable := isWritableType(objType)
				allPoints = append(allPoints, PointRecord{
					DeviceID: rec.DeviceID,
					ObjType:  objType,
					Instance: inst,
					Writable: writable,
				})
				totalRegistered++
			}
		}
	}

	t.Logf("Phase 4 result: %d points registered", totalRegistered)
	writableCount := 0
	for _, p := range allPoints {
		if p.Writable {
			writableCount++
		}
	}
	t.Logf("  Writable points: %d, Read-only points: %d", writableCount, totalRegistered-writableCount)

	// ===================================================================
	// Phase 5: Real-time Polling (3 rounds)
	// ===================================================================
	t.Log("═══════════════════════════════════════════════════")
	t.Log("  Phase 5: Real-time Polling (3 rounds)")
	t.Log("═══════════════════════════════════════════════════")

	// Build pollable point list (Device/CharacterString don't support PresentValue)
	var pollablePoints []PointRecord
	for _, pt := range allPoints {
		if isPollableType(pt.ObjType) {
			pollablePoints = append(pollablePoints, pt)
		}
	}

	const pollRounds = 3
	pollSuccess := 0
	pollChanges := 0

	// Store values per round: roundIndex -> "deviceID:objType:instance" -> float64
	roundValues := make([]map[string]float64, pollRounds)

	for round := 0; round < pollRounds; round++ {
		t.Logf("[Phase 5] Round %d/%d — reading %d pollable points...", round+1, pollRounds, len(pollablePoints))
		roundValues[round] = make(map[string]float64)
		roundOK := 0

		for _, pt := range pollablePoints {
			rec, ok := deviceRecords[pt.DeviceID]
			if !ok {
				continue
			}
			rp, err := client.ReadPropertyWithTimeout(rec.Device, btypes.PropertyData{
				Object: btypes.Object{
					ID: btypes.ObjectID{Type: pt.ObjType, Instance: pt.Instance},
					Properties: []btypes.Property{{
						Type:       btypes.PropPresentValue,
						ArrayIndex: btypes.ArrayAll,
					}},
				},
			}, 5*time.Second)
			if err != nil {
				t.Logf("[Phase 5] Round %d: Device %d %s:%d read FAILED: %v",
					round+1, pt.DeviceID, objectTypeName(pt.ObjType), pt.Instance, err)
				continue
			}
			if len(rp.Object.Properties) > 0 {
				key := fmt.Sprintf("%d:%s:%d", pt.DeviceID, objectTypeName(pt.ObjType), pt.Instance)
				if val, ok := decodeFloatValue(rp.Object.Properties[0].Data); ok {
					roundValues[round][key] = val
					roundOK++
				}
			}
		}
		pollSuccess += roundOK
		t.Logf("[Phase 5] Round %d: %d points read successfully", round+1, roundOK)
		time.Sleep(100 * time.Millisecond)
	}

	// Detect changes between rounds
	for _, pt := range pollablePoints {
		key := fmt.Sprintf("%d:%s:%d", pt.DeviceID, objectTypeName(pt.ObjType), pt.Instance)
		for round := 1; round < pollRounds; round++ {
			v1, ok1 := roundValues[round-1][key]
			v2, ok2 := roundValues[round][key]
			if ok1 && ok2 && v1 != v2 {
				pollChanges++
				t.Logf("[Phase 5] Value change detected: %s %.4f -> %.4f", key, v1, v2)
				break // count each point only once
			}
		}
	}

	expected := len(pollablePoints) * pollRounds
	minRequired := expected * 4 / 5 // allow 20% failure tolerance for intermittent timeouts
	if pollSuccess < minRequired {
		t.Errorf("Phase 5: too many poll failures (got %d, expected at least %d of %d)",
			pollSuccess, minRequired, expected)
	}
	if pollSuccess < expected {
		t.Logf("Phase 5: %d/%d reads succeeded (%.1f%%), %d intermittent failures tolerated",
			pollSuccess, expected, float64(pollSuccess)/float64(expected)*100, expected-pollSuccess)
	}
	t.Logf("Phase 5 result: %d points read across %d rounds, %d points with value changes",
		pollSuccess, pollRounds, pollChanges)

	// ===================================================================
	// Phase 6: Write Verification
	// ===================================================================
	t.Log("═══════════════════════════════════════════════════")
	t.Log("  Phase 6: Write Verification")
	t.Log("═══════════════════════════════════════════════════")

	writeSuccess := 0
	writeTotal := 0

	for _, wt := range writeTargets {
		rec, ok := deviceRecords[wt.DeviceID]
		if !ok {
			t.Logf("[Phase 6] Device %d not registered, skipping write", wt.DeviceID)
			continue
		}

		// Parse the object key (e.g. "AnalogValue:2")
		parts := strings.SplitN(wt.ObjectKey, ":", 2)
		if len(parts) != 2 {
			t.Errorf("[Phase 6] Invalid object key format: %s", wt.ObjectKey)
			continue
		}
		var objType btypes.ObjectType
		objInst := btypes.ObjectInstance(0)
		if _, err := fmt.Sscanf(parts[1], "%d", &objInst); err != nil {
			t.Errorf("[Phase 6] Invalid instance in key: %s", wt.ObjectKey)
			continue
		}
		// Determine type from string prefix
		switch parts[0] {
		case "AnalogValue":
			objType = btypes.AnalogValue
		case "BinaryValue":
			objType = btypes.BinaryValue
		case "AnalogOutput":
			objType = btypes.AnalogOutput
		case "BinaryOutput":
			objType = btypes.BinaryOutput
		case "MultiStateValue":
			objType = btypes.MultiStateValue
		default:
			t.Errorf("[Phase 6] Unknown object type in key: %s", wt.ObjectKey)
			continue
		}

		// Skip BinaryValue writes (read-only)
		if objType == btypes.BinaryValue {
			t.Logf("[Phase 6] Skipping BinaryValue write (read-only): Device %d %s", wt.DeviceID, wt.ObjectKey)
			continue
		}

		writeTotal++

		objID := btypes.ObjectID{Type: objType, Instance: objInst}
		wp := btypes.PropertyData{
			Object: btypes.Object{
				ID: objID,
				Properties: []btypes.Property{{
					Type:       btypes.PropPresentValue,
					ArrayIndex: btypes.ArrayAll,
					Data:       float32(wt.WriteValue),
				}},
			},
		}

		// Write
		if err := client.WriteProperty(rec.Device, wp); err != nil {
			t.Errorf("[Phase 6] Write FAILED: Device %d %s = %.1f: %v", wt.DeviceID, wt.ObjectKey, wt.WriteValue, err)
			continue
		}
		t.Logf("[Phase 6] Write sent: Device %d %s = %.1f", wt.DeviceID, wt.ObjectKey, wt.WriteValue)

		// Verify 1: immediate read
		rp1, err := client.ReadPropertyWithTimeout(rec.Device, btypes.PropertyData{
			Object: btypes.Object{
				ID: objID,
				Properties: []btypes.Property{{
					Type:       btypes.PropPresentValue,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Errorf("[Phase 6] Verify 1 FAILED: Device %d %s read error: %v", wt.DeviceID, wt.ObjectKey, err)
			continue
		}
		val1, ok1 := decodeFloatValue(rp1.Object.Properties[0].Data)
		if !ok1 {
			t.Errorf("[Phase 6] Verify 1 FAILED: Device %d %s value decode error", wt.DeviceID, wt.ObjectKey)
			continue
		}
		t.Logf("[Phase 6] Verify 1: Device %d %s = %.4f", wt.DeviceID, wt.ObjectKey, val1)

		// Wait 500ms then verify again
		time.Sleep(500 * time.Millisecond)

		rp2, err := client.ReadPropertyWithTimeout(rec.Device, btypes.PropertyData{
			Object: btypes.Object{
				ID: objID,
				Properties: []btypes.Property{{
					Type:       btypes.PropPresentValue,
					ArrayIndex: btypes.ArrayAll,
				}},
			},
		}, 5*time.Second)
		if err != nil {
			t.Errorf("[Phase 6] Verify 2 FAILED: Device %d %s read error: %v", wt.DeviceID, wt.ObjectKey, err)
			continue
		}
		val2, ok2 := decodeFloatValue(rp2.Object.Properties[0].Data)
		if !ok2 {
			t.Errorf("[Phase 6] Verify 2 FAILED: Device %d %s value decode error", wt.DeviceID, wt.ObjectKey)
			continue
		}
		t.Logf("[Phase 6] Verify 2: Device %d %s = %.4f", wt.DeviceID, wt.ObjectKey, val2)

		// Both verifications must match and equal the written value (within float tolerance)
		tolerance := 0.01
		if math.Abs(val1-val2) > tolerance {
			t.Errorf("[Phase 6] FAILED: Device %d %s — verify 1 (%.4f) != verify 2 (%.4f)",
				wt.DeviceID, wt.ObjectKey, val1, val2)
			continue
		}
		if math.Abs(val1-wt.WriteValue) > tolerance {
			t.Errorf("[Phase 6] FAILED: Device %d %s — read value (%.4f) != written value (%.1f)",
				wt.DeviceID, wt.ObjectKey, val1, wt.WriteValue)
			continue
		}

		writeSuccess++
		t.Logf("[Phase 6] PASSED: Device %d %s = %.1f (both verifications consistent)", wt.DeviceID, wt.ObjectKey, wt.WriteValue)
	}

	if writeSuccess != writeTotal {
		t.Errorf("Phase 6: only %d/%d writes passed verification", writeSuccess, writeTotal)
	}
	t.Logf("Phase 6 result: %d/%d writes passed verification", writeSuccess, writeTotal)

	// ===================================================================
	// Summary
	// ===================================================================
	t.Log("")
	t.Log("═══════════════════════════════════════════════════════════════")
	t.Log("              BACnet 驱动完整链路测试 — 汇总")
	t.Log("═══════════════════════════════════════════════════════════════")

	p1Status := "PASS"
	if phase1OK != phase1Total {
		p1Status = "FAIL"
	}
	t.Logf("  Phase 1 设备发现: %s %d/%d 成功", statusIcon(p1Status == "PASS"), phase1OK, phase1Total)

	p2Status := "PASS"
	if registeredCount != len(confirmedDevices) {
		p2Status = "FAIL"
	}
	t.Logf("  Phase 2 设备注册: %s %d 台设备", statusIcon(p2Status == "PASS"), registeredCount)

	p3Status := "PASS"
	if scanSuccessCount != totalDevices {
		p3Status = "FAIL"
	}
	t.Logf("  Phase 3 点位扫描: %s %d/%d 成功", statusIcon(p3Status == "PASS"), scanSuccessCount, totalDevices)

	t.Logf("  Phase 4 点位注册: PASS %d 个点位", totalRegistered)

	p5Expected := len(pollablePoints) * pollRounds
	p5MinRequired := p5Expected * 4 / 5
	p5Status := "PASS"
	if pollSuccess < p5MinRequired {
		p5Status = "FAIL"
	}
	t.Logf("  Phase 5 实时轮询: %s %d 个点位中有 %d 个变化", statusIcon(p5Status == "PASS"), totalRegistered, pollChanges)

	p6Status := "PASS"
	if writeSuccess != writeTotal {
		p6Status = "FAIL"
	}
	t.Logf("  Phase 6 可写点写入: %s %d/%d 成功", statusIcon(p6Status == "PASS"), writeSuccess, writeTotal)

	allPass := p1Status == "PASS" && p2Status == "PASS" && p3Status == "PASS" &&
		p5Status == "PASS" && p6Status == "PASS"
	t.Logf("%s 所有测试阶段通过!", statusIcon(allPass))
	t.Log("═══════════════════════════════════════════════════════════════")

	if !allPass {
		t.FailNow()
	}
}

func statusIcon(pass bool) string {
	if pass {
		return "OK"
	}
	return "FAIL"
}

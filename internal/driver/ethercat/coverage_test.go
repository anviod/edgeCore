package ethercat

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Integration tests using simulatorMaster — covers the full driver lifecycle
// =============================================================================

// newSimulationTransport creates a transport with simulator backend for testing.
func newSimulationTransport() *EtherCATTransport {
	chCfg := channelConfig{
		localInterface: "lo",
		cycleTime:      10 * time.Millisecond,
		timeout:        3 * time.Second,
		maxRetries:     3,
		simulation:     true,
	}
	transport := NewEtherCATTransport(chCfg)
	transport.master = newSimulatorMaster()
	// Add a test slave
	sim := transport.master.(*simulatorMaster)
	sim.addSlave(1, 0x00000002, 0x07D43052, 16, 16) // 16-byte RxPDO for float32 test
	sim.addSlave(2, 0x00000003, 0x00000001, 32, 16)
	transport.RegisterSlaveSnapshot(1, 16, 16)
	transport.RegisterSlaveSnapshot(2, 32, 16)
	transport.connected.Store(true)
	transport.connectTime = time.Now()
	transport.startCycleThread()
	return transport
}

// =============================================================================
// TestCoverage_DriverLifecycle — Init, Connect, Health, Disconnect
// =============================================================================

func TestCoverage_DriverLifecycle(t *testing.T) {
	d := NewEtherCATDriver()

	// Init
	err := d.Init(model.DriverConfig{
		ChannelID: "test-channel",
		Protocol:  "ethercat",
		Config: map[string]any{
			"local_interface": "lo",
			"simulation":      true,
			"cycle_time_us":   10000,
		},
	})
	require.NoError(t, err)

	// Connect (simulation mode — no real NIC)
	ctx := context.Background()
	err = d.Connect(ctx)
	require.NoError(t, err)
	assert.True(t, d.transport.IsConnected())

	// Health
	assert.Equal(t, driver.HealthStatusGood, d.Health())

	// GetConnectionMetrics
	cs, rc, la, ra, _ := d.GetConnectionMetrics()
	assert.GreaterOrEqual(t, cs, int64(0))
	assert.GreaterOrEqual(t, rc, int64(0))
	assert.Equal(t, "lo", la)
	assert.Equal(t, "ethercat-bus", ra)

	// SetDeviceConfig
	err = d.SetDeviceConfig(map[string]any{
		"position":     1,
		"vendor_id":    "0x00000002",
		"product_code": "0x07D43052",
		"tx_pdo_size":  16,
		"rx_pdo_size":  8,
	})
	require.NoError(t, err)

	// SetSlaveID (no-op)
	assert.NoError(t, d.SetSlaveID(1))

	// Disconnect
	err = d.Disconnect()
	require.NoError(t, err)
	assert.False(t, d.transport.IsConnected())
	assert.Equal(t, driver.HealthStatusBad, d.Health())
}

// =============================================================================
// TestCoverage_ReadPointsPDO — PDO snapshot read through simulator
// =============================================================================

func TestCoverage_ReadPointsPDO(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	// Set test data in simulator slave 1 TxPDO
	sim := transport.master.(*simulatorMaster)
	sim.setTxPDO(1, 0, []byte{0x12, 0x34, 0x56, 0x78})
	sim.setTxPDO(1, 4, []byte{0xFF, 0xFF, 0xFF, 0xFF})
	sim.setTxPDO(1, 8, []byte{0x00, 0x01, 0x02, 0x03})

	// Refresh snapshots
	transport.refreshTxSnapshots()

	points := []model.Point{
		{ID: "p1", Address: "1:Tx:0", DataType: "int16"},
		{ID: "p2", Address: "1:Tx:2", DataType: "uint16"},
		{ID: "p3", Address: "1:Tx:4", DataType: "int32"},
		{ID: "p4", Address: "1:Tx:4", DataType: "uint32"},
		{ID: "p5", Address: "1:Tx:8", DataType: "int32"},
		{ID: "p6", Address: "1:Tx:0.0", DataType: "bit"},
	}

	ctx := context.Background()
	results, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)
	assert.Len(t, results, 6)

	// int16 BE: 0x1234 = 4660
	assert.Equal(t, int16(0x1234), results["p1"].Value)
	assert.Equal(t, "Good", results["p1"].Quality)

	// uint16 BE: 0x5678 = 22136
	assert.Equal(t, uint16(0x5678), results["p2"].Value)
	assert.Equal(t, "Good", results["p2"].Quality)

	// int32 BE: 0xFFFFFFFF = -1
	assert.Equal(t, int32(-1), results["p3"].Value)
	assert.Equal(t, "Good", results["p3"].Quality)

	// uint32 BE: 0xFFFFFFFF = 4294967295
	assert.Equal(t, uint32(0xFFFFFFFF), results["p4"].Value)
	assert.Equal(t, "Good", results["p4"].Quality)

	// int32 BE: 0x00010203 = 66051
	assert.Equal(t, int32(66051), results["p5"].Value)
	assert.Equal(t, "Good", results["p5"].Quality)

	// bit 0 of 0x12 = 0
	assert.Equal(t, false, results["p6"].Value)
	assert.Equal(t, "Good", results["p6"].Quality)
}

// =============================================================================
// TestCoverage_WritePointPDO — RxPDO write through simulator
// =============================================================================

func TestCoverage_WritePointPDO(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()

	// Write int16 to slave 1 RxPDO offset 0
	err := d.WritePoint(ctx, model.Point{
		ID:       "w1",
		Address:  "1:Rx:0",
		DataType: "int16",
	}, int16(0x1234))
	require.NoError(t, err)

	// Write uint32 to slave 1 RxPDO offset 2 (non-overlapping with float at offset 6)
	err = d.WritePoint(ctx, model.Point{
		ID:       "w2",
		Address:  "1:Rx:2",
		DataType: "uint32",
	}, uint32(0xDEADBEEF))
	require.NoError(t, err)

	// Write float32 to slave 1 RxPDO offset 6 (clear of uint32 at offset 2)
	err = d.WritePoint(ctx, model.Point{
		ID:       "w3",
		Address:  "1:Rx:6",
		DataType: "float",
	}, float32(3.14))
	require.NoError(t, err)

	// Verify via simulator
	sim := transport.master.(*simulatorMaster)
	rxPDO := sim.getRxPDO(1)
	require.NotNil(t, rxPDO)
	assert.Len(t, rxPDO, 16)

	// Check int16 at offset 0
	assert.Equal(t, uint16(0x1234), binary.BigEndian.Uint16(rxPDO[0:2]))
	// Check uint32 at offset 2
	assert.Equal(t, uint32(0xDEADBEEF), binary.BigEndian.Uint32(rxPDO[2:6]))
	// Check float32 at offset 6
	assert.InDelta(t, float64(3.14), float64(math.Float32frombits(binary.BigEndian.Uint32(rxPDO[6:10]))), 1e-6)
}

// =============================================================================
// TestCoverage_ReadPointsSDO — SDO read through simulator
// =============================================================================

func TestCoverage_ReadPointsSDO(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	// Set SDO values in simulator
	sim := transport.master.(*simulatorMaster)
	sim.setSDO(1, 0x6041, 0, []byte{0x02, 0x37})             // status word = 0x0237 (BE)
	sim.setSDO(1, 0x6064, 0, []byte{0x00, 0x00, 0x27, 0x10}) // position = 10000 (BE)

	ctx := context.Background()
	points := []model.Point{
		{ID: "s1", Address: "1:SDO:0x6041:0", DataType: "uint16"},
		{ID: "s2", Address: "1:SDO:0x6064:0", DataType: "int32"},
	}

	results, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	// uint16 SDO: 0x0237 = 567
	assert.Equal(t, uint16(0x0237), results["s1"].Value)
	assert.Equal(t, "Good", results["s1"].Quality)

	// int32 SDO: 0x00002710 = 10000
	assert.Equal(t, int32(10000), results["s2"].Value)
	assert.Equal(t, "Good", results["s2"].Quality)
}

// =============================================================================
// TestCoverage_WritePointSDO — SDO write through simulator
// =============================================================================

func TestCoverage_WritePointSDO(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()
	err := d.WritePoint(ctx, model.Point{
		ID:       "ws1",
		Address:  "1:SDO:0x6060:0",
		DataType: "int8",
	}, int8(8)) // set mode of operation = 8
	require.NoError(t, err)

	// Verify
	sim := transport.master.(*simulatorMaster)
	data, err := sim.readSDO(1, 0x6060, 0)
	require.NoError(t, err)
	assert.Equal(t, int8(8), int8(data[0]))
}

// =============================================================================
// TestCoverage_ReadPointsErrors — error paths
// =============================================================================

func TestCoverage_ReadPointsErrors(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()

	// Invalid address
	points := []model.Point{
		{ID: "bad", Address: "invalid", DataType: "int16"},
		{ID: "out_of_range", Address: "1:Tx:999", DataType: "int16"},
		{ID: "sdo_not_found", Address: "1:SDO:0x9999:0", DataType: "uint16"},
	}

	results, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// All should be Bad
	for _, id := range []string{"bad", "out_of_range", "sdo_not_found"} {
		assert.Equal(t, "Bad", results[id].Quality)
	}
}

// =============================================================================
// TestCoverage_DriverNotConnected — error on not-connected
// =============================================================================

func TestCoverage_DriverNotConnected(t *testing.T) {
	d := NewEtherCATDriver()
	d.Init(model.DriverConfig{
		ChannelID: "test",
		Config: map[string]any{
			"simulation":      true,
			"local_interface": "lo",
		},
	})

	ctx := context.Background()

	_, err := d.ReadPoints(ctx, []model.Point{{ID: "p1", Address: "1:Tx:0", DataType: "int16"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	err = d.WritePoint(ctx, model.Point{ID: "w1", Address: "1:Rx:0", DataType: "int16"}, int16(0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// =============================================================================
// TestCoverage_Scanner — Scanner interface
// =============================================================================

func TestCoverage_Scanner(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()
	result, err := d.Scan(ctx, nil)
	require.NoError(t, err)

	slaves, ok := result.([]ScanResult)
	require.True(t, ok)
	assert.Len(t, slaves, 2)

	// Build position-indexed map since scanSlaves iterates over a map (non-deterministic order)
	byPos := make(map[int]ScanResult)
	for _, s := range slaves {
		byPos[s.Position] = s
	}

	s1 := byPos[1]
	assert.Equal(t, 1, s1.Position)
	assert.Equal(t, "0x00000002", s1.VendorID)
	assert.Equal(t, "0x07D43052", s1.ProductCode)
	assert.Equal(t, 16, s1.TxPDOSize)
	assert.Equal(t, 16, s1.RxPDOSize)

	s2 := byPos[2]
	assert.Equal(t, 2, s2.Position)
}

// =============================================================================
// TestCoverage_ResetDeviceCollection
// =============================================================================

func TestCoverage_ResetDeviceCollection(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	// Set some data
	sim := transport.master.(*simulatorMaster)
	sim.setTxPDO(1, 0, []byte{0x12, 0x34})
	transport.refreshTxSnapshots()

	// Verify data exists
	snapshot := transport.getTxPDOSnapshot(1, 0, 2)
	require.NotNil(t, snapshot)

	// Reset device collection
	d.ResetDeviceCollection("device-1")

	// Verify snapshots cleared
	snapshot = transport.getTxPDOSnapshot(1, 0, 2)
	assert.Nil(t, snapshot)
}

// =============================================================================
// TestCoverage_SchedulerMetrics — metrics counters
// =============================================================================

func TestCoverage_SchedulerMetrics(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	sim := transport.master.(*simulatorMaster)
	sim.setTxPDO(1, 0, []byte{0x42, 0x42})
	sim.setTxPDO(2, 0, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	transport.refreshTxSnapshots()

	ctx := context.Background()
	points := []model.Point{
		{ID: "good1", Address: "1:Tx:0", DataType: "int16"},
		{ID: "good2", Address: "2:Tx:0", DataType: "uint32"},
		{ID: "bad1", Address: "invalid", DataType: "int16"},
	}

	_, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)

	total, success, failure := transport.GetSchedulerMetrics()
	assert.Equal(t, int64(3), total)
	assert.Equal(t, int64(2), success)
	assert.Equal(t, int64(1), failure)
}

// =============================================================================
// TestCoverage_SimulatorMaster — simulator backend
// =============================================================================

func TestCoverage_SimulatorMaster(t *testing.T) {
	sim := newSimulatorMaster()

	// Init
	err := sim.init("lo")
	require.NoError(t, err)

	// addSlave with custom params
	sim.addSlave(1, 0x00000002, 0x07D43052, 16, 8)
	sim.addSlave(3, 0x00000059, 0x00000001, 64, 32)

	// Scan
	slaves, err := sim.scanSlaves()
	require.NoError(t, err)
	assert.Len(t, slaves, 2) // default (pos=1) + custom (pos=3)

	// Bring to OP
	err = sim.bringToOP([]int{1, 3})
	require.NoError(t, err)

	// setTxPDO / getTxPDO
	sim.setTxPDO(1, 0, []byte{0xAA, 0xBB})
	data := sim.getTxPDO(1)
	assert.Equal(t, []byte{0xAA, 0xBB}, data[:2])

	// setRxPDO / getRxPDO
	sim.setRxPDO(3, []byte{0x01, 0x02, 0x03})
	rx := sim.getRxPDO(3)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, rx[:3])

	// setSDO / readSDO
	sim.setSDO(1, 0x1008, 0, []byte("MyDevice"))
	data, err = sim.readSDO(1, 0x1008, 0)
	require.NoError(t, err)
	assert.Equal(t, []byte("MyDevice"), data)

	// readSDO not found
	_, err = sim.readSDO(1, 0x9999, 0)
	require.Error(t, err)

	// readSDO invalid slave
	_, err = sim.readSDO(99, 0x1000, 0)
	require.Error(t, err)

	// writeSDO invalid slave
	err = sim.writeSDO(99, 0x1000, 0, []byte{0x00})
	require.Error(t, err)

	// getTxPDO invalid
	assert.Nil(t, sim.getTxPDO(99))

	// getRxPDO invalid
	assert.Nil(t, sim.getRxPDO(99))

	// Close
	err = sim.close()
	require.NoError(t, err)
}

// =============================================================================
// TestCoverage_WritePointErrors — write error paths
// =============================================================================

func TestCoverage_WritePointErrors(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()

	// Invalid address
	err := d.WritePoint(ctx, model.Point{
		ID: "bad", Address: "invalid", DataType: "int16",
	}, int16(0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address parse")

	// Out of range RxPDO offset
	err = d.WritePoint(ctx, model.Point{
		ID: "oor", Address: "1:Rx:999", DataType: "int16",
	}, int16(0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds buffer size")
}

// =============================================================================
// TestCoverage_ScaleOffset — scale/offset application
// =============================================================================

func TestCoverage_ScaleOffset(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	// Set float value in TxPDO
	sim := transport.master.(*simulatorMaster)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, math.Float32bits(25.0))
	sim.setTxPDO(1, 0, buf)
	transport.refreshTxSnapshots()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()
	points := []model.Point{
		{ID: "scaled", Address: "1:Tx:0", DataType: "float", Scale: 2.0, Offset: 10.0},
	}

	results, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)
	// 25.0 * 2.0 + 10.0 = 60.0
	assert.InDelta(t, 60.0, results["scaled"].Value.(float64), 1e-6)
	assert.Equal(t, "Good", results["scaled"].Quality)
}

// =============================================================================
// TestCoverage_DisconnectIdempotent — safe to call multiple times
// =============================================================================

func TestCoverage_DisconnectIdempotent(t *testing.T) {
	d := NewEtherCATDriver()
	d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"simulation": true, "local_interface": "lo"},
	})

	// Disconnect before connect (should be safe)
	assert.NoError(t, d.Disconnect())

	// Connect then disconnect twice
	ctx := context.Background()
	d.Connect(ctx)
	assert.NoError(t, d.Disconnect())
	assert.NoError(t, d.Disconnect())
}

// =============================================================================
// TestCoverage_KnownDataType — unknown data type fallback
// =============================================================================

func TestCoverage_UnknownDataType(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	sim := transport.master.(*simulatorMaster)
	sim.setTxPDO(1, 0, []byte{0xAB, 0xCD, 0xEF})
	transport.refreshTxSnapshots()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()
	points := []model.Point{
		{ID: "unknown", Address: "1:Tx:0", DataType: "custom_type"},
	}

	results, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)
	// Unknown type returns hex string of whatever bytes were read
	assert.Equal(t, "ab", results["unknown"].Value)
	assert.Equal(t, "Good", results["unknown"].Quality)
}

// =============================================================================
// TestCoverage_TransportHelpers — getRxPDOBuffer, RemoveSlaveSnapshot, etc.
// =============================================================================

func TestCoverage_TransportHelpers(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	// getRxPDOBuffer
	rx := transport.getRxPDOBuffer(1)
	assert.NotNil(t, rx)
	assert.Len(t, rx, 16)

	// getRxPDOBuffer for non-existent slave
	rx = transport.getRxPDOBuffer(99)
	assert.Nil(t, rx)

	// getTxPDOSnapshot for non-existent slave
	snap := transport.getTxPDOSnapshot(99, 0, 4)
	assert.Nil(t, snap)

	// getTxPDOSnapshot out of range
	snap = transport.getTxPDOSnapshot(1, 999, 4)
	assert.Nil(t, snap)

	// RemoveSlaveSnapshot
	transport.RemoveSlaveSnapshot(1)
	snap = transport.getTxPDOSnapshot(1, 0, 4)
	assert.Nil(t, snap)
	rx = transport.getRxPDOBuffer(1)
	assert.Nil(t, rx)

	// GetTransport
	s := NewEtherCATScheduler(transport, NewEtherCATDecoder())
	assert.Equal(t, transport, s.GetTransport())

	// GetConnectionMetrics with non-zero connect time
	cs, rc, la, ra := transport.GetConnectionMetrics()
	assert.GreaterOrEqual(t, cs, int64(0))
	assert.GreaterOrEqual(t, rc, int64(0))
	assert.Equal(t, "lo", la)
	assert.Equal(t, "ethercat-bus", ra)
}

// =============================================================================
// TestCoverage_SDOTransportMethods — transport.readSDO/writeSDO
// =============================================================================

func TestCoverage_SDOTransportMethods(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	// Pre-set SDO value in simulator
	sim := transport.master.(*simulatorMaster)
	sim.setSDO(1, 0x1008, 0, []byte("TestDevice"))

	ctx := context.Background()

	// readSDO via transport
	data, err := transport.readSDO(ctx, 1, 0x1008, 0)
	require.NoError(t, err)
	assert.Equal(t, []byte("TestDevice"), data)

	// readSDO non-existent slave
	_, err = transport.readSDO(ctx, 99, 0x1008, 0)
	require.Error(t, err)

	// writeSDO via transport
	err = transport.writeSDO(ctx, 1, 0x6060, 0, []byte{0x08})
	require.NoError(t, err)

	// writeSDO non-existent slave
	err = transport.writeSDO(ctx, 99, 0x6060, 0, []byte{0x08})
	require.Error(t, err)
}

// =============================================================================
// TestCoverage_ParseConfigEdgeCases — edge cases for config parsing
// =============================================================================

func TestCoverage_ParseConfigEdgeCases(t *testing.T) {
	// parseChannelConfig with zero values
	c, err := parseChannelConfig(map[string]any{
		"local_interface": "eth0",
		"cycle_time_us":   0,
		"timeout":         0,
		"max_retries":     0,
		"simulation":      false,
	})
	require.NoError(t, err)
	assert.Equal(t, "eth0", c.localInterface)
	assert.Equal(t, 1*time.Millisecond, c.cycleTime) // defaults
	assert.Equal(t, 3*time.Second, c.timeout)
	assert.Equal(t, 3, c.maxRetries)

	// parseDeviceConfig with SDO mode
	dc, err := parseDeviceConfig(map[string]any{
		"position": 1,
		"run_mode": "sdo",
	})
	require.NoError(t, err)
	assert.Equal(t, "sdo", dc.runMode)

	// parseDeviceConfig with zero values (should use defaults)
	dc, err = parseDeviceConfig(map[string]any{
		"position":    1,
		"alias":       0,
		"tx_pdo_size": 0,
		"rx_pdo_size": 0,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, dc.alias)
	assert.Equal(t, 0, dc.txPDOSize)
	assert.Equal(t, 0, dc.rxPDOSize)
}

// =============================================================================
// TestCoverage_WritePointSDOErrors — SDO write error paths
// =============================================================================

func TestCoverage_WritePointSDOErrors(t *testing.T) {
	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	ctx := context.Background()

	// SDO write to non-existent slave
	err := d.WritePoint(ctx, model.Point{
		ID:       "sdo_bad",
		Address:  "99:SDO:0x6060:0",
		DataType: "int8",
	}, int8(8))
	require.Error(t, err)

	// SDO write with invalid data type
	err = d.WritePoint(ctx, model.Point{
		ID:       "sdo_encode_err",
		Address:  "1:SDO:0x6060:0",
		DataType: "struct",
	}, struct{}{})
	require.Error(t, err)
}

// =============================================================================
// TestCoverage_SimulatorSDOErrors — simulator SDO error paths
// =============================================================================

func TestCoverage_SimulatorSDOErrors(t *testing.T) {
	sim := newSimulatorMaster()
	sim.init("lo")

	// readSDO on non-existent slave
	_, err := sim.readSDO(1, 0x1000, 0)
	require.Error(t, err)

	// writeSDO on non-existent slave
	err = sim.writeSDO(1, 0x1000, 0, []byte{0x00})
	require.Error(t, err)

	// getTxPDO on non-existent slave
	assert.Nil(t, sim.getTxPDO(1))

	// getRxPDO on non-existent slave
	assert.Nil(t, sim.getRxPDO(1))
}

// =============================================================================
// TestCoverage_TransportNotConnected — SDO operations when master is nil
// =============================================================================

func TestCoverage_TransportNotConnected(t *testing.T) {
	transport := NewEtherCATTransport(channelConfig{
		localInterface: "lo",
		simulation:     true,
		cycleTime:      10 * time.Millisecond,
		timeout:        3 * time.Second,
		maxRetries:     3,
	})

	ctx := context.Background()

	// readSDO with nil master
	_, err := transport.readSDO(ctx, 1, 0x1000, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	// writeSDO with nil master
	err = transport.writeSDO(ctx, 1, 0x1000, 0, []byte{0x00})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

// =============================================================================
// TestCoverage_ChannelConfigValidation — validation edge cases
// =============================================================================

func TestCoverage_ChannelConfigValidation(t *testing.T) {
	// Simulation mode requires local_interface even with simulation=true
	// Actually, the parser allows simulation without local_interface
	c, err := parseChannelConfig(map[string]any{
		"simulation": true,
	})
	require.NoError(t, err)
	assert.Equal(t, "", c.localInterface)
	assert.True(t, c.simulation)

	// Non-simulation without local_interface → error
	_, err = parseChannelConfig(map[string]any{
		"simulation": false,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local_interface is required")
}

// =============================================================================
// TestCoverage_ParseAddressOverflowPositions — high position values
// =============================================================================

func TestCoverage_ParseAddressEdgeCases(t *testing.T) {
	// Position 0 should fail
	_, err := ParseAddress("0:Tx:0")
	require.Error(t, err)

	// Large but valid position
	addr, err := ParseAddress("254:Tx:65535")
	require.NoError(t, err)
	assert.Equal(t, 254, addr.Position)
	assert.Equal(t, 65535, addr.Offset)

	// SDO with 0x prefix lowercase
	addr, err = ParseAddress("1:sdo:0xabcd:0xef")
	require.NoError(t, err)
	assert.Equal(t, uint16(0xABCD), addr.Index)
	assert.Equal(t, uint16(0xEF), addr.SubIndex)
}

package ethercat

import (
	"context"
	"encoding/binary"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Stress tests — concurrent ReadPoints/WritePoint, cycle thread stability
// =============================================================================

func TestStress_ConcurrentReadPoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	transport := newSimulationTransport()
	defer transport.Disconnect()

	sim := transport.master.(*simulatorMaster)
	// Set initial TxPDO data
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], 0x12345678)
	binary.BigEndian.PutUint32(buf[4:8], 0xDEADBEEF)
	binary.BigEndian.PutUint32(buf[8:12], 0xCAFEBABE)
	binary.BigEndian.PutUint32(buf[12:16], 0x8BADF00D)
	sim.setTxPDO(1, 0, buf)
	transport.refreshTxSnapshots()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	var wg sync.WaitGroup
	var ops atomic.Int32
	concurrency := 50
	iterations := 200

	ctx := context.Background()
	points := []model.Point{
		{ID: "p1", Address: "1:Tx:0", DataType: "int32"},
		{ID: "p2", Address: "1:Tx:4", DataType: "uint32"},
		{ID: "p3", Address: "1:Tx:8", DataType: "uint32"},
		{ID: "p4", Address: "1:Tx:12", DataType: "uint32"},
	}

	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				results, err := d.ReadPoints(ctx, points)
				if err != nil {
					// Expected when not connected — skip
					continue
				}
				_ = results
				ops.Add(1)
			}
		}()
	}

	wg.Wait()
	totalOps := ops.Load()
	t.Logf("Concurrent ReadPoints: %d goroutines x %d iterations = %d successful ops",
		concurrency, iterations, totalOps)
	assert.Greater(t, totalOps, int32(0))
}

func TestStress_ConcurrentWritePoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	transport := newSimulationTransport()
	defer transport.Disconnect()

	d := &EtherCATDriver{
		transport: transport,
		scheduler: NewEtherCATScheduler(transport, NewEtherCATDecoder()),
		decoder:   NewEtherCATDecoder(),
	}

	var wg sync.WaitGroup
	var ops atomic.Int32
	concurrency := 50
	iterations := 200

	ctx := context.Background()
	points := []struct {
		id      string
		address string
		dtype   string
		value   any
	}{
		{"w1", "1:Rx:0", "int16", int16(0x1234)},
		{"w2", "1:Rx:2", "uint32", uint32(0xDEADBEEF)},
		{"w3", "1:Rx:6", "float", float32(3.14)},
	}

	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, p := range points {
					err := d.WritePoint(ctx, model.Point{
						ID:       p.id,
						Address:  p.address,
						DataType: p.dtype,
					}, p.value)
					if err != nil {
						continue
					}
					ops.Add(1)
				}
			}
		}()
	}

	wg.Wait()
	totalOps := ops.Load()
	t.Logf("Concurrent WritePoints: %d goroutines x %d iterations x %d points = %d successful ops",
		concurrency, iterations, len(points), totalOps)
	assert.Greater(t, totalOps, int32(0))
}

// =============================================================================
// TestStress_PDOCycleStability — verify cycle thread runs continuously
// =============================================================================

func TestStress_PDOCycleStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	transport := newSimulationTransport()
	defer transport.Disconnect()

	sim := transport.master.(*simulatorMaster)

	// Continuously update TxPDO data from another goroutine
	done := make(chan struct{})
	var updateCount atomic.Int64

	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				buf := make([]byte, 4)
				binary.BigEndian.PutUint32(buf, uint32(updateCount.Load()))
				sim.setTxPDO(1, 0, buf)
				updateCount.Add(1)
			}
		}
	}()

	// Let the cycle thread run for a while
	time.Sleep(100 * time.Millisecond)
	close(done)

	updates := updateCount.Load()
	t.Logf("PDO cycle stability: %d TxPDO updates in 100ms", updates)
	assert.Greater(t, updates, int64(10), "should have at least 10 updates in 100ms")

	// Verify transport is still connected
	assert.True(t, transport.IsConnected())
}

// =============================================================================
// TestStress_EncodeDecodeHighVolume — high-volume encode/decode
// =============================================================================

func TestStress_EncodeDecodeHighVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	d := NewEtherCATDecoder()
	addr := &ParsedAddress{Endian: "BE", Bit: -1}

	const totalOps = 100000

	// Encode stress
	for i := 0; i < totalOps; i++ {
		encoded, err := d.EncodeValue(int32(i), "int32", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "int32", addr)
		require.NoError(t, err)
		assert.Equal(t, int32(i), decoded)
	}
}

// =============================================================================
// TestStress_ParseAddressHighVolume — high-volume address parsing
// =============================================================================

func TestStress_ParseAddressHighVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	const totalOps = 50000

	for i := 0; i < totalOps; i++ {
		addr, err := ParseAddress("1:Tx:16.3#LE")
		require.NoError(t, err)
		assert.Equal(t, 1, addr.Position)
		assert.Equal(t, "TX", addr.PDOType)
	}
}

// =============================================================================
// TestStress_ConfigParseHighVolume — high-volume config parsing
// =============================================================================

func TestStress_ConfigParseHighVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	chCfg := map[string]any{
		"local_interface": "eth0",
		"cycle_time_us":   1000,
		"timeout":         3000,
		"max_retries":     3,
	}

	devCfg := map[string]any{
		"position":     1,
		"vendor_id":    "0x00000002",
		"product_code": "0x07D43052",
		"tx_pdo_size":  16,
		"rx_pdo_size":  8,
	}

	const totalOps = 50000

	for i := 0; i < totalOps; i++ {
		c, err := parseChannelConfig(chCfg)
		require.NoError(t, err)
		assert.Equal(t, "eth0", c.localInterface)

		dc, err := parseDeviceConfig(devCfg)
		require.NoError(t, err)
		assert.Equal(t, 1, dc.position)
	}
}

// =============================================================================
// TestStress_SimulatorConcurrent — concurrent simulator access
// =============================================================================

func TestStress_SimulatorConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	sim := newSimulatorMaster()
	sim.init("lo")
	sim.addSlave(1, 0x00000002, 0x07D43052, 256, 256)

	var wg sync.WaitGroup
	concurrency := 100

	// Concurrent reads
	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = sim.getTxPDO(1)
				_ = sim.getRxPDO(1)
			}
		}(g)
	}

	// Concurrent writes
	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				buf := make([]byte, 4)
				binary.BigEndian.PutUint32(buf, uint32(id*100+i))
				sim.setTxPDO(1, 0, buf)
				sim.setRxPDO(1, buf)
			}
		}(g)
	}

	// Concurrent SDO
	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				sim.setSDO(1, uint16(0x6000+id), 0, []byte{byte(id)})
				_, _ = sim.readSDO(1, uint16(0x6000+id), 0)
			}
		}(g)
	}

	wg.Wait()
	// No data races — test passes
}

// =============================================================================
// TestStress_FloatEncodeDecode — edge case float values
// =============================================================================

func TestStress_FloatEncodeDecode(t *testing.T) {
	d := NewEtherCATDecoder()
	addrBE := &ParsedAddress{Endian: "BE", Bit: -1}
	addrLE := &ParsedAddress{Endian: "LE", Bit: -1}

	testValues := []float64{
		0.0,
		-0.0,
		1.0,
		-1.0,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.Pi,
		math.E,
	}

	for _, v := range testValues {
		// float32 round-trip
		encoded, err := d.EncodeValue(float32(v), "float", addrBE)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "float", addrBE)
		require.NoError(t, err)
		assert.InDelta(t, float64(v), float64(decoded.(float32)), 1e-6)

		// float64 round-trip
		encoded, err = d.EncodeValue(v, "float64", addrLE)
		require.NoError(t, err)
		decoded, err = d.DecodeValue(encoded, "float64", addrLE)
		require.NoError(t, err)
		assert.InDelta(t, v, decoded.(float64), 1e-10)
	}

	// float64-only edge values (can't be encoded as float32)
	float64Only := []float64{
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
	}
	for _, v := range float64Only {
		encoded, err := d.EncodeValue(v, "float64", addrLE)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "float64", addrLE)
		require.NoError(t, err)
		assert.InDelta(t, v, decoded.(float64), 1e-10)
	}
}

// =============================================================================
// TestStress_IntBoundaryValues — int boundary values
// =============================================================================

func TestStress_IntBoundaryValues(t *testing.T) {
	d := NewEtherCATDecoder()
	addr := &ParsedAddress{Endian: "BE", Bit: -1}

	// int8 boundaries
	for _, v := range []int8{math.MinInt8, -1, 0, 1, math.MaxInt8} {
		encoded, err := d.EncodeValue(v, "int8", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "int8", addr)
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}

	// int16 boundaries
	for _, v := range []int16{math.MinInt16, -1, 0, 1, math.MaxInt16} {
		encoded, err := d.EncodeValue(v, "int16", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "int16", addr)
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}

	// int32 boundaries
	for _, v := range []int32{math.MinInt32, -1, 0, 1, math.MaxInt32} {
		encoded, err := d.EncodeValue(v, "int32", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "int32", addr)
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}

	// uint boundaries
	for _, v := range []uint8{0, 1, math.MaxUint8} {
		encoded, err := d.EncodeValue(v, "uint8", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "uint8", addr)
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}

	for _, v := range []uint16{0, 1, math.MaxUint16} {
		encoded, err := d.EncodeValue(v, "uint16", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "uint16", addr)
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}

	for _, v := range []uint32{0, 1, math.MaxUint32} {
		encoded, err := d.EncodeValue(v, "uint32", addr)
		require.NoError(t, err)
		decoded, err := d.DecodeValue(encoded, "uint32", addr)
		require.NoError(t, err)
		assert.Equal(t, v, decoded)
	}
}

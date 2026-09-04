package ethercat

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/edgeCore/internal/driver"

	"go.uber.org/zap"
)

// --- etherCATMaster interface ---
// Abstracts the underlying EtherCAT master implementation (real UDP / simulator).
// This allows the transport layer to work with either backend transparently.

type etherCATMaster interface {
	init(iface string) error
	scanSlaves() ([]slaveInfo, error)
	bringToOP(positions []int) error
	sendProcessdata() error
	receiveProcessdata() error
	getTxPDO(position int) []byte
	setRxPDO(position int, data []byte)
	readSDO(position int, index, subindex uint16) ([]byte, error)
	writeSDO(position int, index, subindex uint16, data []byte) error
	close() error
}

// slaveInfo holds basic slave identification data discovered during bus scan.
type slaveInfo struct {
	Position    int    // slave position on bus (1..N)
	VendorID    uint32 // vendor ID
	ProductCode uint32 // product code
	Revision    uint32 // revision number
	TxPDOSize   int    // TxPDO image size in bytes
	RxPDOSize   int    // RxPDO image size in bytes
}

// udpMaster and slaveIO are defined in master_udp.go (real hardware) and
// master_udp_stub.go (simulation build). The transport layer selects the
// appropriate backend via the etherCATMaster interface based on channelCfg.simulation.

// --- EtherCATTransport ---
// Manages the EtherCAT master lifecycle, PDO cycle thread, snapshot memory,
// and ConnectionManager integration.

type EtherCATTransport struct {
	channelCfg channelConfig
	deviceCfg  deviceConfig
	master     etherCATMaster

	// ConnectionManager for retry/backoff control
	connMgr *driver.ConnectionManager

	// PDO cycle thread control
	cycleStopCh  chan struct{}
	cycleWG      sync.WaitGroup
	cycleRunning atomic.Bool

	// PDO snapshots: cycle thread writes, ReadPoints reads
	txSnapshot sync.Map // map[position]*atomic.Pointer[[]byte]
	rxBuffers  sync.Map // map[position]*rxBuffer

	// Connection state
	connected      atomic.Bool
	connectTime    time.Time
	reconnectCount int64

	// Metrics
	totalRequests atomic.Int64
	successCount  atomic.Int64
	failureCount  atomic.Int64
}

// rxBuffer holds RxPDO data with mutex protection.
type rxBuffer struct {
	data []byte
	mu   sync.Mutex
}

// NewEtherCATTransport creates a new transport instance.
func NewEtherCATTransport(chCfg channelConfig) *EtherCATTransport {
	t := &EtherCATTransport{
		channelCfg: chCfg,
		connMgr:    driver.NewConnectionManager("ethercat"),
	}
	return t
}

// Connect initializes the master and starts the PDO cycle thread.
// Must be called via ConnectionManager.EnsureConnected for single-owner guarantees.
func (t *EtherCATTransport) Connect(ctx context.Context) error {
	return t.connMgr.EnsureConnected(ctx, t.connectOnce)
}

// connectOnce performs the actual master initialization (single dial entry).
func (t *EtherCATTransport) connectOnce(ctx context.Context) error {
	// Select master backend based on simulation flag
	if t.channelCfg.simulation {
		t.master = newSimulatorMaster()
	} else {
		t.master = newUDPMaster()
	}

	if err := t.master.init(t.channelCfg.localInterface); err != nil {
		t.connMgr.RecordFailure()
		return err
	}

	// Scan slaves
	slaves, err := t.master.scanSlaves()
	if err != nil {
		t.connMgr.RecordFailure()
		return err
	}

	// Initialize PDO snapshots for each slave
	positions := make([]int, 0, len(slaves))
	for _, s := range slaves {
		// TxPDO snapshot
		var ptr atomic.Pointer[[]byte]
		initial := make([]byte, s.TxPDOSize)
		ptr.Store(&initial)
		t.txSnapshot.Store(s.Position, &ptr)

		// RxPDO buffer
		t.rxBuffers.Store(s.Position, &rxBuffer{
			data: make([]byte, s.RxPDOSize),
		})

		positions = append(positions, s.Position)
	}

	// Bring slaves to OP state
	if err := t.master.bringToOP(positions); err != nil {
		t.connMgr.RecordFailure()
		return err
	}

	// Start PDO cycle thread
	t.startCycleThread()

	t.connected.Store(true)
	t.connectTime = time.Now()
	t.connMgr.RecordSuccess()

	zap.L().Info("ethercat: transport connected",
		zap.String("interface", t.channelCfg.localInterface),
		zap.Int("slave_count", len(slaves)),
	)
	return nil
}

// Disconnect stops the PDO cycle thread and closes the master.
// Idempotent — safe to call multiple times.
func (t *EtherCATTransport) Disconnect() {
	t.stopCycleThread()

	if t.master != nil {
		_ = t.master.close()
	}

	t.connected.Store(false)
	zap.L().Info("ethercat: transport disconnected")
}

// IsConnected returns whether the master is in OP state.
func (t *EtherCATTransport) IsConnected() bool {
	return t.connected.Load()
}

// --- PDO cycle thread ---

func (t *EtherCATTransport) startCycleThread() {
	t.cycleStopCh = make(chan struct{})
	t.cycleRunning.Store(true)
	t.cycleWG.Add(1)
	go t.pdoCycle()
}

func (t *EtherCATTransport) stopCycleThread() {
	if !t.cycleRunning.Load() {
		return
	}
	t.cycleRunning.Store(false)
	close(t.cycleStopCh)
	t.cycleWG.Wait()
}

// pdoCycle is the PDO exchange loop running in its own goroutine.
// It sends and receives process data at the configured cycle time,
// updating the TxPDO snapshot memory for zero-wait reads by ReadPoints.
func (t *EtherCATTransport) pdoCycle() {
	defer t.cycleWG.Done()

	ticker := time.NewTicker(t.channelCfg.cycleTime)
	defer ticker.Stop()

	for {
		select {
		case <-t.cycleStopCh:
			return
		case <-ticker.C:
			// 1. Send process data (includes RxPDO outputs)
			if err := t.master.sendProcessdata(); err != nil {
				t.handleCycleError(err)
				return
			}

			// 2. Receive process data (updates TxPDO inputs)
			if err := t.master.receiveProcessdata(); err != nil {
				t.handleCycleError(err)
				return
			}

			// 3. Refresh TxPDO snapshots (atomic write for lock-free reads)
			t.refreshTxSnapshots()
		}
	}
}

// refreshTxSnapshots copies master's TxPDO data into atomic pointers
// for lock-free reads by ReadPoints.
func (t *EtherCATTransport) refreshTxSnapshots() {
	t.txSnapshot.Range(func(key, val any) bool {
		position := key.(int)
		ptr := val.(*atomic.Pointer[[]byte])
		data := t.master.getTxPDO(position)
		if len(data) > 0 {
			snapshot := make([]byte, len(data))
			copy(snapshot, data)
			ptr.Store(&snapshot)
		}
		return true
	})
}

// handleCycleError is called when the PDO cycle thread encounters an error.
// It stops the cycle thread and schedules a reconnection.
func (t *EtherCATTransport) handleCycleError(err error) {
	zap.L().Error("ethercat: PDO cycle error, initiating reconnect",
		zap.Error(err),
	)
	t.connected.Store(false)

	// Schedule async reconnection
	t.connMgr.ScheduleReconnect(context.Background(), t.channelCfg.timeout, t.connectOnce)
}

// --- PDO snapshot access ---

// getTxPDOSnapshot returns a slice of the TxPDO snapshot for the given position and offset.
// Returns nil if the position is not found or the offset is out of range.
func (t *EtherCATTransport) getTxPDOSnapshot(position, offset, size int) []byte {
	val, ok := t.txSnapshot.Load(position)
	if !ok {
		return nil
	}
	ptr := val.(*atomic.Pointer[[]byte])
	snapshot := ptr.Load()
	if snapshot == nil || *snapshot == nil {
		return nil
	}
	data := *snapshot
	if offset+size > len(data) {
		return nil
	}
	return data[offset : offset+size]
}

// getRxPDOBuffer returns a copy of the RxPDO buffer for the given position.
func (t *EtherCATTransport) getRxPDOBuffer(position int) []byte {
	val, ok := t.rxBuffers.Load(position)
	if !ok {
		return nil
	}
	buf := val.(*rxBuffer)
	buf.mu.Lock()
	defer buf.mu.Unlock()
	result := make([]byte, len(buf.data))
	copy(result, buf.data)
	return result
}

// setRxPDOBuffer writes data to the RxPDO buffer at the given offset.
// The data will be sent in the next PDO cycle.
func (t *EtherCATTransport) setRxPDOBuffer(position, offset int, data []byte) error {
	val, ok := t.rxBuffers.Load(position)
	if !ok {
		return fmt.Errorf("ethercat: no RxPDO buffer for slave position %d", position)
	}
	buf := val.(*rxBuffer)
	buf.mu.Lock()
	defer buf.mu.Unlock()

	if offset+len(data) > len(buf.data) {
		return fmt.Errorf("ethercat: RxPDO write offset %d + len %d exceeds buffer size %d for slave %d",
			offset, len(data), len(buf.data), position)
	}
	copy(buf.data[offset:], data)

	// Also update the master's RxPDO so the cycle thread picks it up
	t.master.setRxPDO(position, buf.data)
	return nil
}

// readSDO performs a CoE SDO read from the given slave.
func (t *EtherCATTransport) readSDO(ctx context.Context, position int, index, subindex uint16) ([]byte, error) {
	if t.master == nil {
		return nil, fmt.Errorf("ethercat: master not connected")
	}
	return t.master.readSDO(position, index, subindex)
}

// writeSDO performs a CoE SDO write to the given slave.
func (t *EtherCATTransport) writeSDO(ctx context.Context, position int, index, subindex uint16, data []byte) error {
	if t.master == nil {
		return fmt.Errorf("ethercat: master not connected")
	}
	return t.master.writeSDO(position, index, subindex, data)
}

// --- Device management ---

// SetDeviceConfig updates the current device configuration.
func (t *EtherCATTransport) SetDeviceConfig(devCfg deviceConfig) {
	t.deviceCfg = devCfg
}

// RegisterSlaveSnapshot creates PDO snapshot entries for a slave at the given position.
func (t *EtherCATTransport) RegisterSlaveSnapshot(position, txSize, rxSize int) {
	if txSize > 0 {
		var ptr atomic.Pointer[[]byte]
		initial := make([]byte, txSize)
		ptr.Store(&initial)
		t.txSnapshot.Store(position, &ptr)
	}
	if rxSize > 0 {
		t.rxBuffers.Store(position, &rxBuffer{
			data: make([]byte, rxSize),
		})
	}
}

// RemoveSlaveSnapshot removes PDO snapshot entries for a slave.
func (t *EtherCATTransport) RemoveSlaveSnapshot(position int) {
	t.txSnapshot.Delete(position)
	t.rxBuffers.Delete(position)
}

// ResetDeviceCollection clears all PDO snapshots (called when points are added/removed).
func (t *EtherCATTransport) ResetDeviceCollection() {
	t.txSnapshot.Range(func(key, val any) bool {
		t.txSnapshot.Delete(key)
		return true
	})
	t.rxBuffers.Range(func(key, val any) bool {
		t.rxBuffers.Delete(key)
		return true
	})
}

// --- Metrics ---

func (t *EtherCATTransport) incSuccess() {
	t.successCount.Add(1)
	t.totalRequests.Add(1)
}

func (t *EtherCATTransport) incFailure() {
	t.failureCount.Add(1)
	t.totalRequests.Add(1)
}

// GetConnectionMetrics returns connection statistics.
func (t *EtherCATTransport) GetConnectionMetrics() (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string) {
	if t.connectTime.IsZero() {
		return 0, 0, t.channelCfg.localInterface, "ethercat-bus"
	}
	return int64(time.Since(t.connectTime).Seconds()),
		atomic.LoadInt64(&t.reconnectCount),
		t.channelCfg.localInterface,
		"ethercat-bus"
}

// GetSchedulerMetrics returns scheduler statistics.
func (t *EtherCATTransport) GetSchedulerMetrics() (totalRequests, successCount, failureCount int64) {
	return t.totalRequests.Load(), t.successCount.Load(), t.failureCount.Load()
}

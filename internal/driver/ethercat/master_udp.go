//go:build !sim

package ethercat

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anviod/EtherCAT/ecad"
	"github.com/anviod/EtherCAT/ecfr"
	"github.com/anviod/EtherCAT/ecmd"
	"github.com/anviod/EtherCAT/etransport"

	"go.uber.org/zap"
)

// --- udpMaster ---
// Real EtherCAT master implementation using anviod/EtherCAT UDP transport.
// Manages the command framer lifecycle and provides PDO/SDO access.
// Excluded from test coverage via //go:build !sim (requires real hardware).

type udpMaster struct {
	mu        sync.Mutex
	framer    *etransport.UDPFramer
	cmdFramer *ecmd.CommandFramer
	iface     *net.Interface
	// slave I/O images indexed by position
	slaves  map[int]*slaveIO
	opState atomic.Int32 // 0=INIT, 1=PREOP, 2=SAFEOP, 3=OP
}

type slaveIO struct {
	position int
	txPDO    []byte // input image (master reads from slave)
	rxPDO    []byte // output image (master writes to slave)
	mu       sync.Mutex
}

func newUDPMaster() *udpMaster {
	return &udpMaster{
		slaves: make(map[int]*slaveIO),
	}
}

func (m *udpMaster) init(iface string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var err error
	m.iface, err = net.InterfaceByName(iface)
	if err != nil {
		return fmt.Errorf("ethercat udp master: interface %q not found: %w", iface, err)
	}

	// Use standard EtherCAT UDP multicast group
	group := net.IPv4(239, 0, 0, 1)

	m.framer, err = etransport.NewUDPFramer(m.iface, group, 1*time.Millisecond)
	if err != nil {
		return fmt.Errorf("ethercat udp master: failed to create UDP framer: %w", err)
	}

	m.cmdFramer = ecmd.NewCommandFramer(m.framer)
	m.opState.Store(0) // INIT
	return nil
}

func (m *udpMaster) scanSlaves() ([]slaveInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdFramer == nil {
		return nil, fmt.Errorf("ethercat: master not initialized")
	}

	var slaves []slaveInfo
	// Scan for slaves using positional addressing.
	// Position 0 addresses the first slave in the chain.
	// Each slave that responds increments the working counter.
	// We scan sequentially until no slave responds.
	for pos := 0; pos < 256; pos++ {
		// Read ESC Type register (0x0000) to detect slave presence
		addr := ecfr.PositionalAddr(int16(pos), ecad.Type)
		data, err := ecmd.ExecuteRead(m.cmdFramer, addr, 2, 1)
		if err != nil {
			// No more slaves or error — stop scanning
			break
		}
		if len(data) < 2 || (data[0] == 0 && data[1] == 0) {
			break
		}

		slavePos := pos + 1 // 1-based position

		// SII identity data (Vendor ID, Product Code, Revision) is stored in EEPROM.
		// Reading via ecee.EEPROM interface requires the ecmd.Commander,
		// which is available as m.cmdFramer. For initial scan, we skip SII reads
		// and rely on later SDO access (0x1018 Identity Object) for detailed info.
		vendorID := uint32(0)
		productCode := uint32(0)
		revision := uint32(0)

		// Read SM2 (TxPDO) and SM3 (RxPDO) lengths from ESC registers
		// SM2 base: 0x0810, SM3 base: 0x0818
		txPDOSize := 0
		rxPDOSize := 0
		addr = ecfr.PositionalAddr(int16(pos), ecad.SyncMangerBase+0x10+7) // SM2 length
		if dl, err := ecmd.ExecuteRead16(m.cmdFramer, addr, 1); err == nil {
			txPDOSize = int(dl)
		}
		addr = ecfr.PositionalAddr(int16(pos), ecad.SyncMangerBase+0x18+7) // SM3 length
		if dl, err := ecmd.ExecuteRead16(m.cmdFramer, addr, 1); err == nil {
			rxPDOSize = int(dl)
		}

		info := slaveInfo{
			Position:    slavePos,
			VendorID:    vendorID,
			ProductCode: productCode,
			Revision:    revision,
			TxPDOSize:   txPDOSize,
			RxPDOSize:   rxPDOSize,
		}
		slaves = append(slaves, info)

		// Initialize slave I/O buffers
		m.slaves[slavePos] = &slaveIO{
			position: slavePos,
			txPDO:    make([]byte, txPDOSize),
			rxPDO:    make([]byte, rxPDOSize),
		}

		zap.L().Info("ethercat: discovered slave",
			zap.Int("position", slavePos),
			zap.String("vendor_id", fmt.Sprintf("0x%08X", vendorID)),
			zap.String("product_code", fmt.Sprintf("0x%08X", productCode)),
			zap.Int("tx_pdo_size", txPDOSize),
			zap.Int("rx_pdo_size", rxPDOSize),
		)
	}

	return slaves, nil
}

func (m *udpMaster) bringToOP(positions []int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdFramer == nil {
		return fmt.Errorf("ethercat: master not initialized")
	}

	// State machine: INIT -> PREOP -> SAFEOP -> OP
	// Each transition writes to AL Control register (0x0120) and waits for AL Status (0x0130)

	for _, pos := range positions {
		// #nosec G115 -- 从站位置以 uint16 位模式承载，再映射到库的 int16 位置地址
		slavePos := int16((pos - 1) & 0xFFFF) // 0-based for positional addressing

		// Step 1: INIT -> PREOP (state=2)
		if err := m.requestState(slavePos, ecad.ALControl, 0x02); err != nil {
			return fmt.Errorf("ethercat: slave %d INIT->PREOP failed: %w", pos, err)
		}
		time.Sleep(10 * time.Millisecond)
		if err := m.checkState(slavePos, ecad.ALStatus, 0x02); err != nil {
			return fmt.Errorf("ethercat: slave %d not in PREOP: %w", pos, err)
		}

		// Step 2: PREOP -> SAFEOP (state=4)
		if err := m.requestState(slavePos, ecad.ALControl, 0x04); err != nil {
			return fmt.Errorf("ethercat: slave %d PREOP->SAFEOP failed: %w", pos, err)
		}
		time.Sleep(10 * time.Millisecond)
		if err := m.checkState(slavePos, ecad.ALStatus, 0x04); err != nil {
			return fmt.Errorf("ethercat: slave %d not in SAFEOP: %w", pos, err)
		}

		// Step 3: SAFEOP -> OP (state=8)
		if err := m.requestState(slavePos, ecad.ALControl, 0x08); err != nil {
			return fmt.Errorf("ethercat: slave %d SAFEOP->OP failed: %w", pos, err)
		}
		time.Sleep(10 * time.Millisecond)
		if err := m.checkState(slavePos, ecad.ALStatus, 0x08); err != nil {
			return fmt.Errorf("ethercat: slave %d not in OP: %w", pos, err)
		}

		zap.L().Info("ethercat: slave transitioned to OP", zap.Int("position", pos))
	}

	m.opState.Store(3) // OP
	return nil
}

// requestState writes the requested state to the AL Control register.
func (m *udpMaster) requestState(slavePos int16, register uint16, state uint8) error {
	addr := ecfr.PositionalAddr(slavePos, register)
	// AL Control: write state + ack bit
	return ecmd.ExecuteWrite8(m.cmdFramer, addr, state|0x10, 1) // ack bit (bit 4) set
}

// checkState reads the AL Status register and verifies the expected state.
func (m *udpMaster) checkState(slavePos int16, register uint16, expected uint8) error {
	addr := ecfr.PositionalAddr(slavePos, register)
	state, err := ecmd.ExecuteRead8(m.cmdFramer, addr, 1)
	if err != nil {
		return err
	}
	if state&0x0F != expected {
		return fmt.Errorf("expected state 0x%02X, got 0x%02X", expected, state&0x0F)
	}
	return nil
}

func (m *udpMaster) sendProcessdata() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdFramer == nil {
		return fmt.Errorf("ethercat: master not initialized")
	}

	// Send LRW (Logical Read/Write) commands for each slave with RxPDO data
	for _, sio := range m.slaves {
		sio.mu.Lock()
		if len(sio.rxPDO) > 0 {
			// Write RxPDO data to slave
			// #nosec G115 -- 从站位置以 uint16 位模式承载，再映射到库的 int16 位置地址
			addr := ecfr.PositionalAddr(int16((sio.position-1)&0xFFFF), 0x0F00) // SM3 start address
			if err := ecmd.ExecuteWrite(m.cmdFramer, addr, sio.rxPDO, 1); err != nil {
				sio.mu.Unlock()
				return fmt.Errorf("ethercat: send processdata to slave %d: %w", sio.position, err)
			}
		}
		sio.mu.Unlock()
	}

	return m.cmdFramer.Cycle()
}

func (m *udpMaster) receiveProcessdata() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdFramer == nil {
		return fmt.Errorf("ethercat: master not initialized")
	}

	// Read TxPDO data from each slave
	for _, sio := range m.slaves {
		if len(sio.txPDO) == 0 {
			continue
		}
		// #nosec G115 -- 从站位置以 uint16 位模式承载，再映射到库的 int16 位置地址
		addr := ecfr.PositionalAddr(int16((sio.position-1)&0xFFFF), 0x0E00) // SM2 start address
		data, err := ecmd.ExecuteRead(m.cmdFramer, addr, len(sio.txPDO), 1)
		if err != nil {
			return fmt.Errorf("ethercat: receive processdata from slave %d: %w", sio.position, err)
		}
		sio.mu.Lock()
		copy(sio.txPDO, data)
		sio.mu.Unlock()
	}

	return nil
}

func (m *udpMaster) getTxPDO(position int) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sio, ok := m.slaves[position]; ok {
		sio.mu.Lock()
		defer sio.mu.Unlock()
		result := make([]byte, len(sio.txPDO))
		copy(result, sio.txPDO)
		return result
	}
	return nil
}

func (m *udpMaster) setRxPDO(position int, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sio, ok := m.slaves[position]; ok {
		sio.mu.Lock()
		defer sio.mu.Unlock()
		if len(data) <= len(sio.rxPDO) {
			copy(sio.rxPDO, data)
		}
	}
}

func (m *udpMaster) readSDO(position int, index, subindex uint16) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdFramer == nil {
		return nil, fmt.Errorf("ethercat: master not initialized")
	}

	// CoE SDO Upload Request via mailbox
	// Mailbox protocol header (6 bytes) + CoE header (10 bytes) + optional data
	mbxHeader := make([]byte, 6)
	mbxHeader[0] = byte((len(mbxHeader) + 10) & 0xFF) // length low byte (header + CoE header)
	mbxHeader[2] = 2 << 4                             // type: CoE (2)
	mbxHeader[4] = 0x03                               // channel: SDO request (3)

	// CoE SDO Upload Request
	coeHeader := make([]byte, 10)
	coeHeader[0] = byte(0x40)                // SDO Upload Request (expedited, no size indicator)
	coeHeader[1] = byte(index & 0xFF)        // index low byte
	coeHeader[2] = byte((index >> 8) & 0xFF) // index high byte
	coeHeader[3] = byte(subindex & 0xFF)

	mbxData := append(mbxHeader, coeHeader...)

	// Write mailbox to SM0 (output mailbox) at 0x1000
	// #nosec G115 -- 从站位置以 uint16 位模式承载，再映射到库的 int16 位置地址
	slavePos := int16((position - 1) & 0xFFFF)
	addr := ecfr.PositionalAddr(slavePos, 0x1000)
	if err := ecmd.ExecuteWrite(m.cmdFramer, addr, mbxData, 1); err != nil {
		return nil, fmt.Errorf("ethercat: SDO read request to slave %d: %w", position, err)
	}

	// Wait for response (poll SM1 — input mailbox)
	// In a real implementation, this would be more sophisticated with timeouts
	time.Sleep(5 * time.Millisecond)

	addr = ecfr.PositionalAddr(slavePos, 0x1080) // SM1 start address
	resp, err := ecmd.ExecuteRead(m.cmdFramer, addr, 64, 1)
	if err != nil {
		return nil, fmt.Errorf("ethercat: SDO read response from slave %d: %w", position, err)
	}

	if len(resp) < 16 {
		return nil, fmt.Errorf("ethercat: SDO response too short from slave %d: %d bytes", position, len(resp))
	}

	// Parse CoE SDO response
	// Mailbox header (6 bytes) + CoE header (10 bytes) + data
	coeResp := resp[6:]
	sdoCmd := coeResp[0]
	if sdoCmd == 0x80 {
		// SDO Abort
		abortCode := uint32(coeResp[4]) | uint32(coeResp[5])<<8 | uint32(coeResp[6])<<16 | uint32(coeResp[7])<<24
		return nil, fmt.Errorf("ethercat: SDO abort from slave %d: code 0x%08X", position, abortCode)
	}

	// SDO Upload Response (0x4B for expedited, 0x41 for segmented)
	dataLen := int(coeResp[4]) | int(coeResp[5])<<8 | int(coeResp[6])<<16 | int(coeResp[7])<<24
	if dataLen > 0 && len(coeResp) >= 10+dataLen {
		result := make([]byte, dataLen)
		copy(result, coeResp[10:10+dataLen])
		return result, nil
	}

	// For expedited transfer (up to 4 bytes), data is in the header itself
	if sdoCmd&0x02 != 0 { // expedited flag
		result := make([]byte, 4)
		result[0] = coeResp[4]
		result[1] = coeResp[5]
		result[2] = coeResp[6]
		result[3] = coeResp[7]
		return result, nil
	}

	return nil, fmt.Errorf("ethercat: unrecognized SDO response from slave %d: cmd=0x%02X", position, sdoCmd)
}

func (m *udpMaster) writeSDO(position int, index, subindex uint16, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdFramer == nil {
		return fmt.Errorf("ethercat: master not initialized")
	}

	// CoE SDO Download Request
	mbxHeader := make([]byte, 6)
	coelen := 10 + len(data)
	mbxHeader[0] = byte(coelen & 0xFF) // length low byte
	mbxHeader[1] = byte((coelen >> 8) & 0xFF)
	mbxHeader[2] = 2 << 4 // type: CoE (2)
	mbxHeader[4] = 0x03   // channel: SDO request (3)

	coeHeader := make([]byte, 10)
	if len(data) <= 4 {
		coeHeader[0] = 0x23 // SDO Download Request (expedited)
	} else {
		coeHeader[0] = 0x21 // SDO Download Request (segmented)
	}
	coeHeader[1] = byte(index & 0xFF)
	coeHeader[2] = byte((index >> 8) & 0xFF)
	coeHeader[3] = byte(subindex & 0xFF)
	coeHeader[4] = byte(len(data) & 0xFF)
	coeHeader[5] = byte((len(data) >> 8) & 0xFF)
	coeHeader[6] = byte((len(data) >> 16) & 0xFF)
	coeHeader[7] = byte((len(data) >> 24) & 0xFF)
	copy(coeHeader[8:], data)

	mbxData := append(mbxHeader, coeHeader...)

	// #nosec G115 -- 从站位置以 uint16 位模式承载，再映射到库的 int16 位置地址
	slavePos := int16((position - 1) & 0xFFFF)
	addr := ecfr.PositionalAddr(slavePos, 0x1000)
	if err := ecmd.ExecuteWrite(m.cmdFramer, addr, mbxData, 1); err != nil {
		return fmt.Errorf("ethercat: SDO write to slave %d: %w", position, err)
	}

	return nil
}

func (m *udpMaster) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.framer != nil {
		m.framer.Close()
	}
	m.opState.Store(0)
	return nil
}

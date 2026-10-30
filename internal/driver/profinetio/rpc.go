package profinetio

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// rpcClient performs acyclic PNIO record read/write over TCP port 34964.
type rpcClient struct {
	conn    net.Conn
	timeout time.Duration
	callID  uint32
	mu      sync.Mutex
}

func newRPCClient(conn net.Conn, timeout time.Duration) *rpcClient {
	return &rpcClient{
		conn:    conn,
		timeout: timeout,
		callID:  1,
	}
}

func (c *rpcClient) close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// ReadIO reads raw IO bytes at slot/subslot starting from index for length bytes.
func (c *rpcClient) ReadIO(slot, subslot, index, length int) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil, fmt.Errorf("profinet-io rpc: not connected")
	}
	if length <= 0 {
		length = 1
	}

	req := buildReadRequest(c.callID, slot, subslot, index, length)
	c.callID++

	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(req); err != nil {
		return nil, fmt.Errorf("profinet-io rpc write: %w", err)
	}

	resp, err := readRPCResponse(c.conn)
	if err != nil {
		return nil, err
	}
	if len(resp) < length {
		return nil, fmt.Errorf("profinet-io rpc: short response (%d < %d)", len(resp), length)
	}
	return resp[:length], nil
}

// WriteIO writes raw IO bytes at slot/subslot starting from index.
func (c *rpcClient) WriteIO(slot, subslot, index int, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("profinet-io rpc: not connected")
	}
	if len(data) == 0 {
		return fmt.Errorf("profinet-io rpc: empty write payload")
	}

	req := buildWriteRequest(c.callID, slot, subslot, index, data)
	c.callID++

	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	if _, err := c.conn.Write(req); err != nil {
		return fmt.Errorf("profinet-io rpc write: %w", err)
	}

	_, err := readRPCResponse(c.conn)
	return err
}

func buildReadRequest(callID uint32, slot, subslot, index, length int) []byte {
	// Simplified PNIO acyclic read frame: header + slot/subslot/index/length.
	payload := make([]byte, 16)
	binary.BigEndian.PutUint16(payload[0:2], uint16(slot))
	binary.BigEndian.PutUint16(payload[2:4], uint16(subslot))
	binary.BigEndian.PutUint32(payload[4:8], uint32(index))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	payload[12] = 0x01 // read op

	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], callID)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	return append(header, payload...)
}

func buildWriteRequest(callID uint32, slot, subslot, index int, data []byte) []byte {
	payload := make([]byte, 16+len(data))
	binary.BigEndian.PutUint16(payload[0:2], uint16(slot))
	binary.BigEndian.PutUint16(payload[2:4], uint16(subslot))
	binary.BigEndian.PutUint32(payload[4:8], uint32(index))
	binary.BigEndian.PutUint32(payload[8:12], uint32(len(data)))
	payload[12] = 0x02 // write op
	copy(payload[16:], data)

	header := make([]byte, 8)
	binary.BigEndian.PutUint32(header[0:4], callID)
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	return append(header, payload...)
}

func readRPCResponse(conn net.Conn) ([]byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("profinet-io rpc read header: %w", err)
	}
	status := binary.BigEndian.Uint32(header[0:4])
	length := binary.BigEndian.Uint32(header[4:8])
	if status != 0 {
		return nil, fmt.Errorf("profinet-io rpc error status %d", status)
	}
	if length == 0 {
		return []byte{}, nil
	}
	if length > 65536 {
		return nil, fmt.Errorf("profinet-io rpc response too large: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("profinet-io rpc read payload: %w", err)
	}
	return buf, nil
}

// simulationStore holds in-memory IO image for simulation mode.
type simulationStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newSimulationStore() *simulationStore {
	return &simulationStore{data: make(map[string][]byte)}
}

func simKey(slot, subslot int) string {
	return fmt.Sprintf("%d:%d", slot, subslot)
}

func (s *simulationStore) read(slot, subslot, index, length int) []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := simKey(slot, subslot)
	buf, ok := s.data[key]
	if !ok || len(buf) <= index {
		out := make([]byte, length)
		for i := range out {
			out[i] = byte((index + i) & 0xFF)
		}
		return out
	}
	end := index + length
	if end > len(buf) {
		end = len(buf)
	}
	out := make([]byte, length)
	copy(out, buf[index:end])
	for i := end - index; i < length; i++ {
		out[i] = byte((index + i) & 0xFF)
	}
	return out
}

func (s *simulationStore) write(slot, subslot, index int, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := simKey(slot, subslot)
	buf := s.data[key]
	need := index + len(data)
	if len(buf) < need {
		nb := make([]byte, need)
		copy(nb, buf)
		buf = nb
	}
	copy(buf[index:], data)
	s.data[key] = buf
}

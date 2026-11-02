package mitsubishi

import (
	"encoding/binary"
	"net"
	"sync"
)

// MockPLC is a minimal MC Protocol 3E TCP server for unit tests.
type MockPLC struct {
	mu      sync.Mutex
	ln      net.Listener
	wordMem map[string]map[int]uint16
	bitMem  map[string]map[int]byte
}

func NewMockPLC() *MockPLC {
	return &MockPLC{
		wordMem: make(map[string]map[int]uint16),
		bitMem:  make(map[string]map[int]byte),
	}
}

func (m *MockPLC) SetWord(device string, offset int, value uint16) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.wordMem[device] == nil {
		m.wordMem[device] = make(map[int]uint16)
	}
	m.wordMem[device][offset] = value
}

func (m *MockPLC) SetBit(device string, offset int, on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bitMem[device] == nil {
		m.bitMem[device] = make(map[int]byte)
	}
	if on {
		m.bitMem[device][offset] = 0x10
	} else {
		m.bitMem[device][offset] = 0x00
	}
}

func (m *MockPLC) Start() (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	m.ln = ln
	go m.acceptLoop()
	return ln.Addr().String(), nil
}

func (m *MockPLC) Close() error {
	if m.ln != nil {
		return m.ln.Close()
	}
	return nil
}

func (m *MockPLC) acceptLoop() {
	for {
		conn, err := m.ln.Accept()
		if err != nil {
			return
		}
		go m.handleConn(conn)
	}
}

func (m *MockPLC) handleConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil || n < 21 {
			return
		}
		resp := m.buildResponse(buf[:n])
		if resp == nil {
			return
		}
		_, _ = conn.Write(resp)
	}
}

func (m *MockPLC) buildResponse(req []byte) []byte {
	if len(req) < 21 || req[0] != 0x50 {
		return nil
	}

	cmd := binary.LittleEndian.Uint16(req[11:13])
	sub := binary.LittleEndian.Uint16(req[13:15])
	offset := int(req[15]) | int(req[16])<<8 | int(req[17])<<16
	deviceCode := req[18]
	count := int(binary.LittleEndian.Uint16(req[19:21]))
	isBit := sub == subCmdBit
	deviceName := deviceNameFromCode(deviceCode)

	switch cmd {
	case cmdBatchReadWord:
		return m.buildReadResponse(req, deviceName, offset, count, isBit)
	case cmdBatchWriteWord:
		if len(req) >= 21+count*2 {
			m.applyWrite(deviceName, offset, req[21:], isBit)
		} else if isBit && len(req) > 21 {
			m.applyWrite(deviceName, offset, req[21:21+count], isBit)
		}
		return m.buildWriteAck(req)
	default:
		return m.buildErrorResponse(req, 0xC059)
	}
}

func (m *MockPLC) buildReadResponse(req []byte, device string, offset, count int, isBit bool) []byte {
	var payload []byte
	m.mu.Lock()
	if isBit {
		for i := 0; i < count; i++ {
			v := byte(0x00)
			if m.bitMem[device] != nil {
				v = m.bitMem[device][offset+i]
			}
			payload = append(payload, v)
		}
	} else {
		for i := 0; i < count; i++ {
			w := uint16(0)
			if m.wordMem[device] != nil {
				w = m.wordMem[device][offset+i]
			}
			payload = append(payload, byte(w&0xFF), byte(w>>8))
		}
	}
	m.mu.Unlock()

	respDataLen := 2 + len(payload)
	resp := make([]byte, 11+len(payload))
	resp[0] = 0xD0
	resp[1] = 0x00
	copy(resp[2:7], req[2:7])
	binary.LittleEndian.PutUint16(resp[7:9], uint16(respDataLen&0xFFFF))
	binary.LittleEndian.PutUint16(resp[9:11], endCodeOK)
	copy(resp[11:], payload)
	return resp
}

func (m *MockPLC) buildWriteAck(req []byte) []byte {
	resp := make([]byte, 11)
	resp[0] = 0xD0
	resp[1] = 0x00
	copy(resp[2:7], req[2:7])
	binary.LittleEndian.PutUint16(resp[7:9], 2)
	binary.LittleEndian.PutUint16(resp[9:11], endCodeOK)
	return resp
}

func (m *MockPLC) buildErrorResponse(req []byte, code uint16) []byte {
	resp := make([]byte, 11)
	resp[0] = 0xD4
	resp[1] = 0x00
	copy(resp[2:7], req[2:7])
	binary.LittleEndian.PutUint16(resp[7:9], 2)
	binary.LittleEndian.PutUint16(resp[9:11], code)
	return resp
}

func (m *MockPLC) applyWrite(device string, offset int, data []byte, isBit bool) {
	if isBit {
		if m.bitMem[device] == nil {
			m.bitMem[device] = make(map[int]byte)
		}
		for i := 0; i < len(data); i++ {
			m.bitMem[device][offset+i] = data[i]
		}
		return
	}
	if m.wordMem[device] == nil {
		m.wordMem[device] = make(map[int]uint16)
	}
	for i := 0; i+1 < len(data); i += 2 {
		w := binary.LittleEndian.Uint16(data[i : i+2])
		m.wordMem[device][offset+i/2] = w
	}
}

func deviceNameFromCode(code byte) string {
	switch code {
	case 0x9C:
		return "X"
	case 0x9D:
		return "Y"
	case 0x90:
		return "M"
	case 0xA8:
		return "D"
	case 0xB4:
		return "W"
	default:
		return fmtDeviceCode(code)
	}
}

func fmtDeviceCode(code byte) string {
	return string([]byte{code})
}

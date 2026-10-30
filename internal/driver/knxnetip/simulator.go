package knxnetip

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

// Simulator is a minimal KNXnet/IP tunneling gateway for unit tests.
type Simulator struct {
	mu        sync.Mutex
	conn      *net.UDPConn
	tcpLn     net.Listener
	channelID byte
	seq       byte
	values    map[uint16][]byte
	knxAddr   uint16
}

func NewSimulator() *Simulator {
	return &Simulator{
		channelID: 1,
		knxAddr:   0x1101,
		values:    make(map[uint16][]byte),
	}
}

func (s *Simulator) SetGroupValue(group uint16, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[group] = append([]byte(nil), data...)
}

func (s *Simulator) Start() (string, error) {
	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return "", err
	}
	s.conn = conn
	go s.serveUDP()
	return conn.LocalAddr().String(), nil
}

func (s *Simulator) StartTCP() (string, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	s.tcpLn = ln
	go s.serveTCPAccept()
	return ln.Addr().String(), nil
}

func (s *Simulator) Close() error {
	if s.tcpLn != nil {
		_ = s.tcpLn.Close()
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *Simulator) serveUDP() {
	buf := make([]byte, 2048)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		resp, dest := s.handle(buf[:n], remote)
		if resp != nil {
			udpDest, ok := dest.(*net.UDPAddr)
			if !ok {
				udpDest = remote
			}
			_, _ = s.conn.WriteToUDP(resp, udpDest)
		}
	}
}

func (s *Simulator) serveTCPAccept() {
	for {
		conn, err := s.tcpLn.Accept()
		if err != nil {
			return
		}
		go s.serveTCPConn(conn)
	}
}

func (s *Simulator) serveTCPConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 2048)
	var acc []byte
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				return
			}
			return
		}
		acc = append(acc, buf[:n]...)
		for len(acc) >= headerLen {
			total := int(binary.BigEndian.Uint16(acc[4:6]))
			if len(acc) < total {
				break
			}
			frame := acc[:total]
			acc = acc[total:]
			resp, _ := s.handle(frame, conn.RemoteAddr())
			if resp != nil {
				if _, err := conn.Write(resp); err != nil {
					return
				}
			}
		}
	}
}

func (s *Simulator) handle(frame []byte, remote net.Addr) ([]byte, net.Addr) {
	svc, body, err := parseHeader(frame)
	if err != nil {
		return nil, remote
	}

	switch svc {
	case svcSearchRequest:
		return s.handleSearch(body), remote
	case svcConnectRequest:
		return s.handleConnect(body), remote
	case svcConnectionStateRequest:
		return s.handleConnectionState(body), remote
	case svcDisconnectRequest:
		return s.handleDisconnect(body), remote
	case svcTunnelingRequest:
		return s.handleTunneling(body), remote
	default:
		return nil, remote
	}
}

func (s *Simulator) handleSearch(body []byte) []byte {
	control := s.controlHPAI()
	respBody := encodeHPAI(control)
	frame := buildHeader(svcSearchResponse, len(respBody))
	return append(frame, respBody...)
}

func (s *Simulator) controlHPAI() hpai {
	h := hpai{hostProtocol: hostProtocolIPv4UDP}
	if s.conn != nil && s.conn.LocalAddr() != nil {
		host, portStr, err := net.SplitHostPort(s.conn.LocalAddr().String())
		if err == nil {
			if ip := net.ParseIP(host); ip != nil {
				copy(h.ip[:], ip.To4())
			}
			var port int
			fmt.Sscanf(portStr, "%d", &port)
			h.port = uint16(port)
		}
		return h
	}
	if s.tcpLn != nil && s.tcpLn.Addr() != nil {
		host, portStr, err := net.SplitHostPort(s.tcpLn.Addr().String())
		if err == nil {
			if ip := net.ParseIP(host); ip != nil {
				copy(h.ip[:], ip.To4())
			}
			var port int
			fmt.Sscanf(portStr, "%d", &port)
			h.port = uint16(port)
			h.hostProtocol = hostProtocolIPv4TCP
		}
	}
	return h
}

func (s *Simulator) handleConnect(body []byte) []byte {
	if len(body) < 16 {
		return nil
	}
	control, _ := decodeHPAI(body[0:8])
	respBody := append(encodeHPAI(control), body[8:16]...)
	ccr := []byte{
		8, s.channelID, 0,
		byte(s.knxAddr >> 8), byte(s.knxAddr & 0xFF),
		0, 0, 0,
	}
	respBody = append(respBody, ccr...)
	frame := buildHeader(svcConnectResponse, len(respBody))
	return append(frame, respBody...)
}

func (s *Simulator) handleConnectionState(body []byte) []byte {
	if len(body) < 2 {
		return nil
	}
	respBody := []byte{4, body[1], 0, 0}
	frame := buildHeader(svcConnectionStateResp, len(respBody))
	return append(frame, respBody...)
}

func (s *Simulator) handleDisconnect(body []byte) []byte {
	if len(body) < 2 {
		return nil
	}
	respBody := []byte{4, body[1], 0, 0}
	frame := buildHeader(svcDisconnectResponse, len(respBody))
	return append(frame, respBody...)
}

func (s *Simulator) handleTunneling(body []byte) []byte {
	ch, seq, _, cemi, err := parseTunnelingBody(body)
	if err != nil {
		return nil
	}

	confirm := buildTunnelingConfirm(ch, seq, 0)

	parsed, err := parseCEMI(cemi)
	if err != nil {
		return confirm
	}

	if parsed.APCI == apciGroupValueRead {
		s.mu.Lock()
		data := append([]byte(nil), s.values[parsed.Destination]...)
		s.mu.Unlock()

		if len(data) == 0 {
			data = []byte{0}
		}

		respCEMI, err := buildGroupValueResponseCEMI(parsed.Destination, s.knxAddr, data)
		if err != nil {
			return confirm
		}
		ind := buildTunnelingIndication(ch, s.nextSeq(), respCEMI)
		return ind
	}

	if parsed.APCI == apciGroupValueWrite {
		s.mu.Lock()
		s.values[parsed.Destination] = append([]byte(nil), parsed.Data...)
		s.mu.Unlock()
		return confirm
	}

	return confirm
}

func (s *Simulator) nextSeq() byte {
	s.seq++
	return s.seq
}

func buildGroupValueResponseCEMI(destGroup, srcAddr uint16, data []byte) ([]byte, error) {
	payloadLen := 1 + len(data)
	// 同 protocol.go：CEMI 长度字节为 6 位域，超长拒绝编码
	if payloadLen > 15 {
		return nil, fmt.Errorf("group value response too large: %d bytes (max 14 data)", len(data))
	}
	cemi := make([]byte, 9+payloadLen)
	cemi[0] = cemiLDataInd
	cemi[1] = 0
	cemi[2] = ctrl1Default
	cemi[3] = ctrl2Group
	binaryBigEndianPutUint16(cemi[4:6], srcAddr)
	binaryBigEndianPutUint16(cemi[6:8], destGroup)
	cemi[8] = byte(payloadLen)
	cemi[9] = apciGroupValueResp
	copy(cemi[10:], data)
	return cemi, nil
}

func binaryBigEndianPutUint16(b []byte, v uint16) {
	b[0] = byte(v >> 8)
	b[1] = byte(v)
}

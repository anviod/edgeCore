package knxnetip

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	defaultPort = 3671

	headerLen = 6

	svcSearchRequest          = 0x0201
	svcSearchResponse         = 0x0202
	svcConnectRequest         = 0x0205
	svcConnectResponse        = 0x0206
	svcConnectionStateRequest = 0x0207
	svcConnectionStateResp    = 0x0208
	svcDisconnectRequest      = 0x0209
	svcDisconnectResponse     = 0x020A
	svcTunnelingRequest       = 0x0420
	svcTunnelingConfirm       = 0x0421
	svcTunnelingIndication    = 0x0422

	hostProtocolIPv4UDP = 0x01
	hostProtocolIPv4TCP = 0x02

	cemiLDataReq = 0x11
	cemiLDataInd = 0x29
	cemiLDataCon = 0x2E

	apciGroupValueRead  = 0x00
	apciGroupValueResp  = 0x01
	apciGroupValueWrite = 0x02

	ctrl1Default = 0xBC
	ctrl2Group   = 0xE0
)

type hpai struct {
	hostProtocol byte
	ip           [4]byte
	port         uint16
}

func buildHeader(serviceType uint16, bodyLen int) []byte {
	total := headerLen + bodyLen
	buf := make([]byte, headerLen)
	buf[0] = headerLen
	buf[1] = 0x10
	binary.BigEndian.PutUint16(buf[2:4], serviceType)
	binary.BigEndian.PutUint16(buf[4:6], uint16(total))
	return buf
}

func parseHeader(frame []byte) (serviceType uint16, body []byte, err error) {
	if len(frame) < headerLen {
		return 0, nil, fmt.Errorf("frame too short")
	}
	if frame[0] != headerLen {
		return 0, nil, fmt.Errorf("invalid header length: %d", frame[0])
	}
	total := int(binary.BigEndian.Uint16(frame[4:6]))
	if total > len(frame) {
		return 0, nil, fmt.Errorf("incomplete frame: want %d got %d", total, len(frame))
	}
	serviceType = binary.BigEndian.Uint16(frame[2:4])
	return serviceType, frame[headerLen:total], nil
}

func encodeHPAI(h hpai) []byte {
	buf := make([]byte, 8)
	buf[0] = 8
	buf[1] = h.hostProtocol
	copy(buf[2:6], h.ip[:])
	binary.BigEndian.PutUint16(buf[6:8], h.port)
	return buf
}

func decodeHPAI(data []byte) (hpai, error) {
	var h hpai
	if len(data) < 8 || data[0] != 8 {
		return h, fmt.Errorf("invalid HPAI")
	}
	h.hostProtocol = data[1]
	copy(h.ip[:], data[2:6])
	h.port = binary.BigEndian.Uint16(data[6:8])
	return h, nil
}

func buildSearchRequest(discovery hpai) []byte {
	body := encodeHPAI(discovery)
	frame := buildHeader(svcSearchRequest, len(body))
	return append(frame, body...)
}

type searchResponse struct {
	Control hpai
}

func parseSearchResponse(body []byte) (searchResponse, error) {
	var resp searchResponse
	if len(body) < 8 {
		return resp, fmt.Errorf("search response too short")
	}
	control, err := decodeHPAI(body[0:8])
	if err != nil {
		return resp, err
	}
	resp.Control = control
	return resp, nil
}

func hpaiUDPAddr(h hpai) (*net.UDPAddr, error) {
	ip := net.IP(h.ip[:])
	if ip == nil || ip.IsUnspecified() {
		return nil, fmt.Errorf("invalid HPAI address")
	}
	return &net.UDPAddr{IP: ip, Port: int(h.port)}, nil
}

func buildConnectRequest(control, data hpai) []byte {
	cri := []byte{0x04, 0x04, 0x00, 0x02} // tunnel, TP1 medium
	body := append(encodeHPAI(control), encodeHPAI(data)...)
	body = append(body, cri...)
	frame := buildHeader(svcConnectRequest, len(body))
	return append(frame, body...)
}

type connectResponse struct {
	ChannelID byte
	Status    byte
	KNXAddr   uint16
}

func parseConnectResponse(body []byte) (connectResponse, error) {
	var resp connectResponse
	if len(body) < 8+8 {
		return resp, fmt.Errorf("connect response too short")
	}
	// skip control + data HPAI
	ccr := body[16:]
	if len(ccr) < 8 {
		return resp, fmt.Errorf("missing CCR")
	}
	if ccr[0] != 8 {
		return resp, fmt.Errorf("invalid CCR length")
	}
	resp.ChannelID = ccr[1]
	resp.Status = ccr[2]
	resp.KNXAddr = binary.BigEndian.Uint16(ccr[3:5])
	return resp, nil
}

func buildConnectionStateRequest(channelID byte) []byte {
	body := []byte{4, channelID, 0, 0}
	frame := buildHeader(svcConnectionStateRequest, len(body))
	return append(frame, body...)
}

func parseConnectionStateResponse(body []byte) (channelID byte, status byte, err error) {
	if len(body) < 4 {
		return 0, 0, fmt.Errorf("connection state response too short")
	}
	return body[1], body[2], nil
}

func buildDisconnectRequest(channelID byte) []byte {
	body := []byte{4, channelID, 0, 0}
	frame := buildHeader(svcDisconnectRequest, len(body))
	return append(frame, body...)
}

func buildTunnelingRequest(channelID, seq byte, cemi []byte) []byte {
	body := make([]byte, 4+len(cemi))
	body[0] = 4
	body[1] = channelID
	body[2] = seq
	body[3] = 0
	copy(body[4:], cemi)
	frame := buildHeader(svcTunnelingRequest, len(body))
	return append(frame, body...)
}

func buildTunnelingConfirm(channelID, seq, status byte) []byte {
	body := []byte{4, channelID, seq, status}
	frame := buildHeader(svcTunnelingConfirm, len(body))
	return append(frame, body...)
}

func buildTunnelingIndication(channelID, seq byte, cemi []byte) []byte {
	body := make([]byte, 4+len(cemi))
	body[0] = 4
	body[1] = channelID
	body[2] = seq
	body[3] = 0
	copy(body[4:], cemi)
	frame := buildHeader(svcTunnelingIndication, len(body))
	return append(frame, body...)
}

func parseTunnelingBody(body []byte) (channelID, seq, status byte, cemi []byte, err error) {
	if len(body) < 4 {
		return 0, 0, 0, nil, fmt.Errorf("tunneling body too short")
	}
	if body[0] != 4 {
		return 0, 0, 0, nil, fmt.Errorf("invalid tunnel structure length")
	}
	channelID = body[1]
	seq = body[2]
	status = body[3]
	if len(body) > 4 {
		cemi = append([]byte(nil), body[4:]...)
	}
	return channelID, seq, status, cemi, nil
}

func buildGroupValueReadCEMI(destGroup uint16, srcAddr uint16) []byte {
	cemi := make([]byte, 10)
	cemi[0] = cemiLDataReq
	cemi[1] = 0
	cemi[2] = ctrl1Default
	cemi[3] = ctrl2Group
	binary.BigEndian.PutUint16(cemi[4:6], srcAddr)
	binary.BigEndian.PutUint16(cemi[6:8], destGroup)
	cemi[8] = 1
	cemi[9] = apciGroupValueRead
	return cemi
}

func buildGroupValueWriteCEMI(destGroup uint16, srcAddr uint16, data []byte) ([]byte, error) {
	// CEMI 数据长度字节为 6 位域（cemi[8] = 1+len(data)，KNX 单帧数据 ≤14 字节），
	// 超长会导致长度字节回绕、帧被对端错误解析
	if len(data) > 14 {
		return nil, fmt.Errorf("group value payload too large: %d bytes (max 14)", len(data))
	}
	cemi := make([]byte, 10+len(data))
	cemi[0] = cemiLDataReq
	cemi[1] = 0
	cemi[2] = ctrl1Default
	cemi[3] = ctrl2Group
	binary.BigEndian.PutUint16(cemi[4:6], srcAddr)
	binary.BigEndian.PutUint16(cemi[6:8], destGroup)
	cemi[8] = byte(1 + len(data))
	cemi[9] = apciGroupValueWrite
	copy(cemi[10:], data)
	return cemi, nil
}

type cemiData struct {
	MessageCode byte
	Source      uint16
	Destination uint16
	APCI        byte
	Data        []byte
}

func parseCEMI(cemi []byte) (cemiData, error) {
	var out cemiData
	if len(cemi) < 10 {
		return out, fmt.Errorf("cEMI too short")
	}
	out.MessageCode = cemi[0]
	out.Source = binary.BigEndian.Uint16(cemi[4:6])
	out.Destination = binary.BigEndian.Uint16(cemi[6:8])
	dataLen := int(cemi[8])
	if len(cemi) < 9+dataLen {
		return out, fmt.Errorf("cEMI data truncated")
	}
	payload := cemi[9 : 9+dataLen]
	if len(payload) > 0 {
		out.APCI = payload[0] & 0x3F
		if len(payload) > 1 {
			out.Data = append([]byte(nil), payload[1:]...)
		}
	}
	return out, nil
}

func isGroupValueResponse(cemi cemiData) bool {
	return cemi.MessageCode == cemiLDataInd || cemi.MessageCode == cemiLDataCon
}

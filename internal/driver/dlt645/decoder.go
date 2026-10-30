package dlt645

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsedAddress holds meter address and data identifier from a point address string.
type ParsedAddress struct {
	MeterAddr [AddrLen]byte
	DataID    [DataIDLen]byte
	Extension string
}

// DLT645Decoder handles DL/T 645-2007 frame and value encoding/decoding.
type DLT645Decoder struct {
	defaultMeterAddr string
}

func NewDLT645Decoder() *DLT645Decoder {
	return &DLT645Decoder{}
}

func (d *DLT645Decoder) SetDefaultMeterAddress(addr string) {
	d.defaultMeterAddr = strings.TrimSpace(addr)
}

// ParseAddress parses "meterAddr#DI3-DI2-DI1-DI0[#extension]" or "DI3-DI2-DI1-DI0".
func (d *DLT645Decoder) ParseAddress(addr string) (*ParsedAddress, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("empty address")
	}

	meterPart := d.defaultMeterAddr
	diPart := addr
	extension := ""

	if idx := strings.Index(addr, "#"); idx >= 0 {
		meterPart = strings.TrimSpace(addr[:idx])
		rest := strings.TrimSpace(addr[idx+1:])
		if extIdx := strings.Index(rest, "#"); extIdx >= 0 {
			diPart = strings.TrimSpace(rest[:extIdx])
			extension = strings.TrimSpace(rest[extIdx+1:])
		} else {
			diPart = rest
		}
	}

	if meterPart == "" {
		return nil, fmt.Errorf("meter address required")
	}

	meterBytes, err := EncodeMeterAddress(meterPart)
	if err != nil {
		return nil, err
	}

	diBytes, err := ParseDataID(diPart)
	if err != nil {
		return nil, err
	}

	return &ParsedAddress{
		MeterAddr: meterBytes,
		DataID:    diBytes,
		Extension: extension,
	}, nil
}

// EncodeMeterAddress converts a 12-digit decimal meter address to 6-byte BCD (low byte first).
func EncodeMeterAddress(addr string) ([AddrLen]byte, error) {
	var out [AddrLen]byte
	addr = strings.TrimSpace(addr)
	addr = strings.ReplaceAll(addr, " ", "")

	if len(addr) > 12 {
		return out, fmt.Errorf("meter address too long: %s", addr)
	}
	if len(addr) < 12 {
		addr = strings.Repeat("0", 12-len(addr)) + addr
	}
	for _, c := range addr {
		if c < '0' || c > '9' {
			return out, fmt.Errorf("invalid meter address digit: %c", c)
		}
	}

	for i := 0; i < AddrLen; i++ {
		pos := 10 - i*2
		hi := addr[pos] - '0'
		lo := addr[pos+1] - '0'
		out[i] = byte(hi<<4 | lo)
	}
	return out, nil
}

// MeterAddressString converts 6-byte BCD address back to 12-digit string.
func MeterAddressString(addr [AddrLen]byte) string {
	var digits [12]byte
	for i := 0; i < AddrLen; i++ {
		pos := 10 - i*2
		b := addr[i]
		digits[pos] = '0' + (b>>4)&0x0F
		digits[pos+1] = '0' + b&0x0F
	}
	return strings.TrimLeft(string(digits[:]), "0")
}

// ParseDataID parses "DI3-DI2-DI1-DI0" or "DI3DI2DI1DI0" into 4 bytes (DI0 lowest index).
func ParseDataID(di string) ([DataIDLen]byte, error) {
	var out [DataIDLen]byte
	di = strings.TrimSpace(di)
	di = strings.ReplaceAll(di, "-", "")
	di = strings.ReplaceAll(di, " ", "")
	if len(di) != 8 {
		return out, fmt.Errorf("invalid data identifier: %s", di)
	}

	parts := make([]byte, DataIDLen)
	for i := 0; i < DataIDLen; i++ {
		hexPair := di[(3-i)*2 : (3-i)*2+2]
		v, err := strconv.ParseUint(hexPair, 16, 8)
		if err != nil {
			return out, fmt.Errorf("invalid data identifier byte: %s", hexPair)
		}
		parts[i] = byte(v)
	}
	copy(out[:], parts)
	return out, nil
}

// DataIDString formats data identifier bytes as "DI3-DI2-DI1-DI0".
func DataIDString(di [DataIDLen]byte) string {
	return fmt.Sprintf("%02X-%02X-%02X-%02X", di[3], di[2], di[1], di[0])
}

// BuildReadFrame builds a read-data request frame (control 0x11).
func BuildReadFrame(meterAddr [AddrLen]byte, dataID [DataIDLen]byte) []byte {
	data := encode033(dataID[:])
	return buildFrame(meterAddr, CtrlRead, data)
}

// BuildWriteFrame builds a write-data request frame (control 0x14).
func BuildWriteFrame(meterAddr [AddrLen]byte, dataID [DataIDLen]byte, payload []byte) []byte {
	body := make([]byte, 0, DataIDLen+len(payload))
	body = append(body, dataID[:]...)
	body = append(body, payload...)
	data := encode033(body)
	return buildFrame(meterAddr, CtrlWrite, data)
}

func buildFrame(meterAddr [AddrLen]byte, control byte, data []byte) []byte {
	// DLT645 长度域为单字节：帧体超过 255 字节无法编码，返回 nil 由调用方拒绝发送
	if len(data) > 255 {
		return nil
	}
	length := byte(len(data))
	frame := make([]byte, 0, 12+len(data))
	frame = append(frame, FrameStart)
	frame = append(frame, meterAddr[:]...)
	frame = append(frame, FrameStart)
	frame = append(frame, control, length)
	frame = append(frame, data...)
	frame = append(frame, checksum(frame), FrameEnd)
	return frame
}

// Frame represents a parsed DL/T 645-2007 response/request frame.
type Frame struct {
	MeterAddr [AddrLen]byte
	Control   byte
	Data      []byte
}

func (f Frame) IsError() bool {
	return f.Control&CtrlErrorMask != 0
}

func (f Frame) ErrorCode() byte {
	if len(f.Data) > 0 {
		return f.Data[0]
	}
	return 0
}

// DecodeFrame parses a complete frame (without leading preamble).
func DecodeFrame(raw []byte) (*Frame, error) {
	if len(raw) < 12 {
		return nil, fmt.Errorf("frame too short: %d bytes", len(raw))
	}
	if raw[0] != FrameStart {
		return nil, fmt.Errorf("invalid start byte: 0x%02X", raw[0])
	}
	var addr [AddrLen]byte
	copy(addr[:], raw[1:7])
	if raw[7] != FrameStart {
		return nil, fmt.Errorf("invalid second start byte: 0x%02X", raw[7])
	}
	control := raw[8]
	dataLen := int(raw[9])
	expected := 12 + dataLen
	if len(raw) < expected {
		return nil, fmt.Errorf("frame length mismatch: have %d, need %d", len(raw), expected)
	}
	if raw[expected-1] != FrameEnd {
		return nil, fmt.Errorf("invalid end byte: 0x%02X", raw[expected-1])
	}
	cs := checksum(raw[:expected-2])
	if cs != raw[expected-2] {
		return nil, fmt.Errorf("checksum mismatch: got 0x%02X, want 0x%02X", raw[expected-2], cs)
	}

	data := decode033(raw[10 : 10+dataLen])
	return &Frame{
		MeterAddr: addr,
		Control:   control,
		Data:      data,
	}, nil
}

// ParseReadResponse extracts data identifier and value bytes from a read response.
func ParseReadResponse(frame *Frame) (dataID [DataIDLen]byte, value []byte, err error) {
	if frame.Control != CtrlReadResp && frame.Control != (CtrlRead|CtrlErrorMask) {
		return dataID, nil, fmt.Errorf("unexpected control code: 0x%02X", frame.Control)
	}
	if frame.IsError() {
		return dataID, nil, fmt.Errorf("meter error response: 0x%02X", frame.ErrorCode())
	}
	if len(frame.Data) < DataIDLen {
		return dataID, nil, fmt.Errorf("response data too short")
	}
	copy(dataID[:], frame.Data[:DataIDLen])
	value = append([]byte(nil), frame.Data[DataIDLen:]...)
	return dataID, value, nil
}

// DecodeValue converts raw meter bytes to a typed value per DL/T 645 BCD encoding.
func DecodeValue(raw []byte, dataType string, scale float64, offset float64) (any, error) {
	dataType = strings.ToUpper(strings.TrimSpace(dataType))
	if dataType == "STRING" || dataType == "STR" {
		return decodeStringValue(raw), nil
	}

	num, err := decodeBCD(raw)
	if err != nil {
		return nil, err
	}

	factor := scale
	if factor == 0 {
		factor = 1
	}
	result := float64(num)*factor + offset

	switch dataType {
	case "UINT8", "INT8", "UINT16", "INT16", "UINT32", "INT32", "UINT64", "INT64":
		return int64(result), nil
	default:
		return result, nil
	}
}

func decodeBCD(raw []byte) (uint64, error) {
	var val uint64
	for i := len(raw) - 1; i >= 0; i-- {
		hi := (raw[i] >> 4) & 0x0F
		lo := raw[i] & 0x0F
		if hi > 9 || lo > 9 {
			return 0, fmt.Errorf("invalid BCD digit at byte %d", i)
		}
		val = val*100 + uint64(hi*10+lo)
	}
	return val, nil
}

func decodeStringValue(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if len(raw) >= 6 {
		parts := make([]string, 0, len(raw))
		for i := len(raw) - 1; i >= 0; i-- {
			hi := (raw[i] >> 4) & 0x0F
			lo := raw[i] & 0x0F
			if hi <= 9 && lo <= 9 {
				parts = append(parts, fmt.Sprintf("%02d", hi*10+lo))
			}
		}
		if len(parts) >= 6 {
			return fmt.Sprintf("20%s-%s-%s %s:%s:%s", parts[5], parts[4], parts[3], parts[2], parts[1], parts[0])
		}
	}
	return fmt.Sprintf("%X", raw)
}

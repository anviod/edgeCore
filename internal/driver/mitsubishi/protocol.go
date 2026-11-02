package mitsubishi

import (
	"encoding/binary"
	"fmt"
)

const (
	cmdBatchReadWord  = 0x0401
	cmdBatchWriteWord = 0x1401
	subCmdWord        = 0x0000
	subCmdBit         = 0x0001

	endCodeOK = 0x0000
)

type frameConfig struct {
	networkNo int
	pcNo      int
	stationNo int
}

func buildReadFrame(cfg frameConfig, addr *MCAddress, byteLen int, isBit bool) []byte {
	pointCount := byteLen
	if !isBit {
		pointCount = byteLen / 2
		if byteLen%2 != 0 {
			pointCount++
		}
	}

	const dataLen = 12
	frame := make([]byte, 21)
	frame[0] = 0x50
	frame[1] = 0x00
	frame[2] = byte(cfg.networkNo & 0xFF)
	frame[3] = byte(cfg.pcNo & 0xFF)
	frame[4] = 0xFF
	frame[5] = 0x03
	frame[6] = byte(cfg.stationNo & 0xFF)
	binary.LittleEndian.PutUint16(frame[7:9], uint16(dataLen&0xFFFF))
	binary.LittleEndian.PutUint16(frame[9:11], 0x000A) // 2.5s monitoring timer
	binary.LittleEndian.PutUint16(frame[11:13], cmdBatchReadWord)
	if isBit {
		binary.LittleEndian.PutUint16(frame[13:15], subCmdBit)
	} else {
		binary.LittleEndian.PutUint16(frame[13:15], subCmdWord)
	}

	offset := addr.readOffset()
	putDeviceAddress(frame[15:19], offset, addr.DeviceCode)
	binary.LittleEndian.PutUint16(frame[19:21], uint16(pointCount&0xFFFF))
	return frame
}

func buildWriteFrame(cfg frameConfig, addr *MCAddress, data []byte, isBit bool) []byte {
	pointCount := len(data)
	if !isBit {
		pointCount = len(data) / 2
	}

	dataLen := 12 + len(data)
	frame := make([]byte, 21+len(data))
	frame[0] = 0x50
	frame[1] = 0x00
	frame[2] = byte(cfg.networkNo & 0xFF)
	frame[3] = byte(cfg.pcNo & 0xFF)
	frame[4] = 0xFF
	frame[5] = 0x03
	frame[6] = byte(cfg.stationNo & 0xFF)
	binary.LittleEndian.PutUint16(frame[7:9], uint16(dataLen&0xFFFF))
	binary.LittleEndian.PutUint16(frame[9:11], 0x000A)
	binary.LittleEndian.PutUint16(frame[11:13], cmdBatchWriteWord)
	if isBit {
		binary.LittleEndian.PutUint16(frame[13:15], subCmdBit)
	} else {
		binary.LittleEndian.PutUint16(frame[13:15], subCmdWord)
	}

	offset := addr.readOffset()
	putDeviceAddress(frame[15:19], offset, addr.DeviceCode)
	binary.LittleEndian.PutUint16(frame[19:21], uint16(pointCount&0xFFFF))
	copy(frame[21:], data)
	return frame
}

func putDeviceAddress(dst []byte, offset int, deviceCode byte) {
	dst[0] = byte(offset & 0xFF)
	dst[1] = byte((offset >> 8) & 0xFF)
	dst[2] = byte((offset >> 16) & 0xFF)
	dst[3] = deviceCode
}

func parseResponse(resp []byte) (endCode uint16, data []byte, err error) {
	if len(resp) < 11 {
		return 0, nil, fmt.Errorf("response too short: %d bytes", len(resp))
	}

	if resp[0] == 0xD4 {
		if len(resp) < 11 {
			return 0, nil, fmt.Errorf("error response too short")
		}
		endCode = binary.LittleEndian.Uint16(resp[9:11])
		return endCode, nil, fmt.Errorf("mc protocol error end code 0x%04X", endCode)
	}

	if resp[0] != 0xD0 || resp[1] != 0x00 {
		return 0, nil, fmt.Errorf("unexpected response header: %02X %02X", resp[0], resp[1])
	}

	endCode = binary.LittleEndian.Uint16(resp[9:11])
	if endCode != endCodeOK {
		return endCode, nil, fmt.Errorf("mc protocol end code 0x%04X", endCode)
	}

	if len(resp) > 11 {
		data = resp[11:]
	}
	return endCode, data, nil
}

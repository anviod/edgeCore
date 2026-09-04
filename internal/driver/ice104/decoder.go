package ice104

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

type cachedPoint struct {
	TypeID  byte
	IOA     uint32
	Value   any
	Quality string
	TS      time.Time
}

type ICE104Decoder struct{}

func NewICE104Decoder() *ICE104Decoder {
	return &ICE104Decoder{}
}

func (d *ICE104Decoder) ParseAddress(addr string) (uint32, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0, fmt.Errorf("empty IOA address")
	}
	v, err := strconv.ParseUint(addr, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid IOA %q: %w", addr, err)
	}
	if v > 0xFFFFFF {
		return 0, fmt.Errorf("IOA out of range: %d", v)
	}
	return uint32(v), nil
}

func (d *ICE104Decoder) PointKey(typeID byte, ioa uint32) string {
	return fmt.Sprintf("%d:%d", typeID, ioa)
}

func (d *ICE104Decoder) PointMeta(p model.Point) (typeID byte, ioa uint32, err error) {
	ioa, err = d.ParseAddress(p.Address)
	if err != nil {
		return 0, 0, err
	}
	typeID = resolveTypeID(strings.TrimSpace(p.Group), strings.ToUpper(p.DataType))
	return typeID, ioa, nil
}

func (d *ICE104Decoder) DecodeInformationObject(typeID byte, payload []byte, sq bool, count int) ([]cachedPoint, error) {
	if count <= 0 {
		return nil, nil
	}
	offset := 0
	var firstIOA uint32
	if sq {
		if len(payload) < 3 {
			return nil, fmt.Errorf("short SQ payload")
		}
		firstIOA = decodeIOA(payload[:3])
		offset = 3
	}

	out := make([]cachedPoint, 0, count)
	for i := 0; i < count; i++ {
		ioa := firstIOA
		if !sq {
			if len(payload) < offset+3 {
				break
			}
			ioa = decodeIOA(payload[offset : offset+3])
			offset += 3
		} else if i > 0 {
			ioa = firstIOA + uint32(i)
		}

		value, quality, size, err := decodeObjectBody(typeID, payload[offset:])
		if err != nil {
			return out, err
		}
		offset += size
		out = append(out, cachedPoint{
			TypeID:  typeID,
			IOA:     ioa,
			Value:   value,
			Quality: quality,
			TS:      time.Now(),
		})
	}
	return out, nil
}

func decodeObjectBody(typeID byte, payload []byte) (value any, quality string, size int, err error) {
	quality = "Good"
	switch typeID {
	case typeM_SP_NA_1:
		if len(payload) < 1 {
			return nil, quality, 0, fmt.Errorf("short M_SP payload")
		}
		value = payload[0]&0x01 == 1
		if payload[0]&0x80 != 0 {
			quality = "Bad"
		}
		return value, quality, 1, nil
	case typeM_ME_NA_1:
		if len(payload) < 3 {
			return nil, quality, 0, fmt.Errorf("short M_ME_NA payload")
		}
		raw := int16(binary.LittleEndian.Uint16(payload[:2]))
		value = float64(raw) / 32768.0
		if payload[2]&0x80 != 0 {
			quality = "Bad"
		}
		return value, quality, 3, nil
	case typeM_ME_NB_1:
		if len(payload) < 3 {
			return nil, quality, 0, fmt.Errorf("short M_ME_NB payload")
		}
		value = int16(binary.LittleEndian.Uint16(payload[:2]))
		if payload[2]&0x80 != 0 {
			quality = "Bad"
		}
		return value, quality, 3, nil
	case typeM_ME_NC_1:
		if len(payload) < 5 {
			return nil, quality, 0, fmt.Errorf("short M_ME_NC payload")
		}
		bits := binary.LittleEndian.Uint32(payload[:4])
		value = math.Float32frombits(bits)
		if payload[4]&0x80 != 0 {
			quality = "Bad"
		}
		return value, quality, 5, nil
	case typeM_IT_NA_1:
		if len(payload) < 5 {
			return nil, quality, 0, fmt.Errorf("short M_IT payload")
		}
		value = binary.LittleEndian.Uint32(payload[:4])
		if payload[4]&0x80 != 0 {
			quality = "Bad"
		}
		return value, quality, 5, nil
	default:
		return nil, quality, 0, fmt.Errorf("unsupported typeID %d", typeID)
	}
}

func (d *ICE104Decoder) ToModelValue(p model.Point, cp cachedPoint) model.Value {
	return model.Value{
		PointID: p.ID,
		Value:   cp.Value,
		Quality: cp.Quality,
		TS:      cp.TS,
	}
}

func encodeSingleCommand(ioa uint32, execute bool) []byte {
	body := encodeIOA(ioa)
	sco := byte(0)
	if execute {
		sco = 1
	}
	body = append(body, sco)
	return body
}

func encodeGeneralInterrogation(ioa uint32) []byte {
	body := encodeIOA(ioa)
	body = append(body, 20) // QOI station interrogation
	return body
}

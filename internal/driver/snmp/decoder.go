package snmp

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
)

type Address struct {
	Community    string
	SecurityName string
	OID          string
}

type SNMPDecoder struct{}

func NewSNMPDecoder() *SNMPDecoder {
	return &SNMPDecoder{}
}

func (d *SNMPDecoder) ParseAddress(addr string, cfg deviceConfig) (*Address, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("empty SNMP address")
	}

	parts := strings.SplitN(addr, "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid SNMP address %q: expected community|oid or securityName|oid", addr)
	}

	prefix := strings.TrimSpace(parts[0])
	oid := strings.TrimSpace(parts[1])
	if prefix == "" || oid == "" {
		return nil, fmt.Errorf("invalid SNMP address %q: prefix and OID required", addr)
	}
	if err := validateOID(oid); err != nil {
		return nil, err
	}

	out := &Address{OID: oid}
	if cfg.isV3() {
		out.SecurityName = prefix
	} else {
		out.Community = prefix
	}
	return out, nil
}

func validateOID(oid string) error {
	segments := strings.Split(oid, ".")
	if len(segments) == 0 {
		return fmt.Errorf("invalid OID %q", oid)
	}
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("invalid OID %q", oid)
		}
		if _, err := strconv.Atoi(seg); err != nil {
			return fmt.Errorf("invalid OID segment %q in %q", seg, oid)
		}
	}
	return nil
}

func (d *SNMPDecoder) ToModelValue(point model.Point, pdu gosnmp.SnmpPDU) model.Value {
	value, quality := d.DecodePDU(pdu, point.DataType)
	return model.Value{
		PointID: point.ID,
		Value:   value,
		Quality: quality,
		TS:      time.Now(),
		Meta: map[string]any{
			"oid": pdu.Name,
		},
	}
}

func (d *SNMPDecoder) DecodePDU(pdu gosnmp.SnmpPDU, dataType string) (any, string) {
	if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance || pdu.Type == gosnmp.EndOfMibView {
		return nil, "Bad"
	}

	dt := strings.ToUpper(strings.TrimSpace(dataType))
	switch dt {
	case "BIT", "BOOL":
		switch v := pdu.Value.(type) {
		case int:
			return v != 0, "Good"
		case uint:
			return v != 0, "Good"
		case int64:
			return v != 0, "Good"
		case uint64:
			return v != 0, "Good"
		case []byte:
			if len(v) > 0 {
				return v[0]&0x01 == 1, "Good"
			}
		}
	case "UINT8":
		if v, ok := toUint64(pdu); ok {
			return uint8(v), "Good"
		}
	case "INT8":
		if v, ok := toInt64(pdu); ok {
			return int8(v), "Good"
		}
	case "UINT16":
		if v, ok := toUint64(pdu); ok {
			return uint16(v), "Good"
		}
	case "INT16":
		if v, ok := toInt64(pdu); ok {
			return int16(v), "Good"
		}
	case "UINT32":
		if v, ok := toUint64(pdu); ok {
			return uint32(v), "Good"
		}
	case "INT32":
		if v, ok := toInt64(pdu); ok {
			return int32(v), "Good"
		}
	case "UINT64":
		if v, ok := toUint64(pdu); ok {
			return v, "Good"
		}
	case "INT64":
		if v, ok := toInt64(pdu); ok {
			return v, "Good"
		}
	case "FLOAT":
		if v, ok := decodeFloat(pdu, 32); ok {
			return v, "Good"
		}
	case "DOUBLE":
		if v, ok := decodeFloat(pdu, 64); ok {
			return v, "Good"
		}
	case "STRING":
		return decodeString(pdu), "Good"
	case "BYTES":
		if b, ok := pdu.Value.([]byte); ok {
			return b, "Good"
		}
	}

	if v, ok := toInt64(pdu); ok {
		return v, "Good"
	}
	if v, ok := toUint64(pdu); ok {
		return v, "Good"
	}
	if s := decodeString(pdu); s != "" {
		return s, "Good"
	}
	return pdu.Value, "Uncertain"
}

func (d *SNMPDecoder) EncodeValue(value any, dataType string) (interface{}, gosnmp.Asn1BER, error) {
	dt := strings.ToUpper(strings.TrimSpace(dataType))
	switch dt {
	case "BIT", "BOOL":
		switch v := value.(type) {
		case bool:
			if v {
				return 1, gosnmp.Integer, nil
			}
			return 0, gosnmp.Integer, nil
		case float64:
			return int(v), gosnmp.Integer, nil
		case int:
			return v, gosnmp.Integer, nil
		case int64:
			return int(v), gosnmp.Integer, nil
		case string:
			if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on") {
				return 1, gosnmp.Integer, nil
			}
			return 0, gosnmp.Integer, nil
		}
	case "UINT8", "INT8", "UINT16", "INT16", "UINT32", "INT32", "INT64":
		switch v := value.(type) {
		case float64:
			return int(v), gosnmp.Integer, nil
		case int:
			return v, gosnmp.Integer, nil
		case int64:
			return int(v), gosnmp.Integer, nil
		case string:
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return nil, gosnmp.Null, err
			}
			return parsed, gosnmp.Integer, nil
		}
	case "UINT64":
		switch v := value.(type) {
		case float64:
			return uint64(v), gosnmp.Counter64, nil
		case int:
			return uint64(v), gosnmp.Counter64, nil
		case int64:
			return uint64(v), gosnmp.Counter64, nil
		case uint64:
			return v, gosnmp.Counter64, nil
		case string:
			parsed, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, gosnmp.Null, err
			}
			return parsed, gosnmp.Counter64, nil
		}
	case "FLOAT", "DOUBLE":
		switch v := value.(type) {
		case float64:
			return fmt.Sprintf("%g", v), gosnmp.OctetString, nil
		case float32:
			return fmt.Sprintf("%g", v), gosnmp.OctetString, nil
		case string:
			return v, gosnmp.OctetString, nil
		}
	case "STRING":
		return fmt.Sprint(value), gosnmp.OctetString, nil
	case "BYTES":
		switch v := value.(type) {
		case []byte:
			return v, gosnmp.OctetString, nil
		case string:
			return []byte(v), gosnmp.OctetString, nil
		}
	}
	return value, gosnmp.OctetString, nil
}

func toInt64(pdu gosnmp.SnmpPDU) (int64, bool) {
	switch v := pdu.Value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func toUint64(pdu gosnmp.SnmpPDU) (uint64, bool) {
	switch v := pdu.Value.(type) {
	case int:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int32:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case uint:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint64:
		return v, true
	case string:
		parsed, err := strconv.ParseUint(v, 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func decodeString(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(pdu.Value)
	}
}

func decodeFloat(pdu gosnmp.SnmpPDU, bits int) (float64, bool) {
	switch v := pdu.Value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case string:
		parsed, err := strconv.ParseFloat(v, bits)
		return parsed, err == nil
	case []byte:
		if bits == 32 && len(v) >= 4 {
			u := binary.BigEndian.Uint32(v)
			return float64(math.Float32frombits(u)), true
		}
		if bits == 64 && len(v) >= 8 {
			u := binary.BigEndian.Uint64(v)
			return math.Float64frombits(u), true
		}
	}
	return 0, false
}

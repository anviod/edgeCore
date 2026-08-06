package snmp

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
)

func TestParseAddressV2c(t *testing.T) {
	dec := NewSNMPDecoder()
	cfg := deviceConfig{SNMPVersion: "v2c"}

	addr, err := dec.ParseAddress("public|1.3.6.1.2.1.1.1.0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if addr.Community != "public" {
		t.Fatalf("community=%q", addr.Community)
	}
	if addr.OID != "1.3.6.1.2.1.1.1.0" {
		t.Fatalf("oid=%q", addr.OID)
	}
}

func TestParseAddressV3(t *testing.T) {
	dec := NewSNMPDecoder()
	cfg := deviceConfig{SNMPVersion: "v3"}

	addr, err := dec.ParseAddress("admin|1.3.6.1.2.1.1.5.0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if addr.SecurityName != "admin" {
		t.Fatalf("securityName=%q", addr.SecurityName)
	}
}

func TestParseAddressInvalid(t *testing.T) {
	dec := NewSNMPDecoder()
	cfg := deviceConfig{SNMPVersion: "v2c"}

	_, err := dec.ParseAddress("invalid", cfg)
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
}

func TestDecodePDUString(t *testing.T) {
	dec := NewSNMPDecoder()
	val, quality := dec.DecodePDU(gosnmp.SnmpPDU{
		Type:  gosnmp.OctetString,
		Value: []byte("Cisco IOS"),
	}, "STRING")
	if quality != "Good" {
		t.Fatalf("quality=%q", quality)
	}
	if val != "Cisco IOS" {
		t.Fatalf("value=%v", val)
	}
}

func TestDecodePDUUint32(t *testing.T) {
	dec := NewSNMPDecoder()
	val, quality := dec.DecodePDU(gosnmp.SnmpPDU{
		Type:  gosnmp.TimeTicks,
		Value: int(123456),
	}, "UINT32")
	if quality != "Good" {
		t.Fatalf("quality=%q", quality)
	}
	if val != uint32(123456) {
		t.Fatalf("value=%v", val)
	}
}

func TestDecodePDUCounter64(t *testing.T) {
	dec := NewSNMPDecoder()
	val, quality := dec.DecodePDU(gosnmp.SnmpPDU{
		Type:  gosnmp.Counter64,
		Value: uint64(9999999999),
	}, "UINT64")
	if quality != "Good" {
		t.Fatalf("quality=%q", quality)
	}
	if val != uint64(9999999999) {
		t.Fatalf("value=%v", val)
	}
}

func TestEncodeValueBool(t *testing.T) {
	dec := NewSNMPDecoder()
	v, asn, err := dec.EncodeValue(true, "BOOL")
	if err != nil {
		t.Fatal(err)
	}
	if v != 1 || asn != gosnmp.Integer {
		t.Fatalf("got %v %v", v, asn)
	}
}

func TestParseDeviceConfigAliases(t *testing.T) {
	cfg := parseDeviceConfig(map[string]any{
		"targetIP":   "10.0.0.1",
		"targetPort": 1161,
		"timeout":    5000,
		"retries":    5,
	})
	if cfg.TargetIP != "10.0.0.1" {
		t.Fatalf("ip=%q", cfg.TargetIP)
	}
	if cfg.TargetPort != 1161 {
		t.Fatalf("port=%d", cfg.TargetPort)
	}
	if cfg.Retries != 5 {
		t.Fatalf("retries=%d", cfg.Retries)
	}
}

func TestGroupPointsByCommunity(t *testing.T) {
	sched := NewSNMPScheduler(nil, NewSNMPDecoder(), map[string]any{
		"snmpVersion": "v2c",
	})
	points := []model.Point{
		{ID: "p1", Address: "public|1.3.6.1.2.1.1.1.0"},
		{ID: "p2", Address: "public|1.3.6.1.2.1.1.5.0"},
		{ID: "p3", Address: "private|1.3.6.1.2.1.1.3.0"},
	}
	groups := sched.groupPoints(points)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}

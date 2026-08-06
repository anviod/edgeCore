package ice104

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestParseAddress(t *testing.T) {
	dec := NewICE104Decoder()
	ioa, err := dec.ParseAddress("400")
	if err != nil {
		t.Fatal(err)
	}
	if ioa != 400 {
		t.Fatalf("expected 400, got %d", ioa)
	}
}

func TestResolveTypeID(t *testing.T) {
	if got := resolveTypeID("M_ME_NA_1", "FLOAT"); got != typeM_ME_NA_1 {
		t.Fatalf("expected %d, got %d", typeM_ME_NA_1, got)
	}
	if got := resolveTypeID("", "BOOL"); got != typeM_SP_NA_1 {
		t.Fatalf("expected %d, got %d", typeM_SP_NA_1, got)
	}
}

func TestDecodeInformationObject_M_ME_NC_1(t *testing.T) {
	dec := NewICE104Decoder()
	payload := append(encodeIOA(1), 0x00, 0x00, 0x80, 0x3F, 0x00)
	points, err := dec.DecodeInformationObject(typeM_ME_NC_1, payload, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].IOA != 1 {
		t.Fatalf("expected IOA 1, got %d", points[0].IOA)
	}
	val, ok := points[0].Value.(float32)
	if !ok || val < 0.99 || val > 1.01 {
		t.Fatalf("expected float ~1.0, got %v", points[0].Value)
	}
}

func TestPointMetaUsesGroupTypeID(t *testing.T) {
	dec := NewICE104Decoder()
	typeID, ioa, err := dec.PointMeta(model.Point{
		Address:  "100",
		Group:    "M_SP_NA_1",
		DataType: "BOOL",
	})
	if err != nil {
		t.Fatal(err)
	}
	if typeID != typeM_SP_NA_1 || ioa != 100 {
		t.Fatalf("unexpected meta type=%d ioa=%d", typeID, ioa)
	}
}

func TestBuildASDU(t *testing.T) {
	asdu := buildASDU(typeC_IC_NA_1, 1, cotActivation, 1, encodeGeneralInterrogation(0))
	if len(asdu) < 8 {
		t.Fatalf("short ASDU: %d bytes", len(asdu))
	}
	if asdu[0] != typeC_IC_NA_1 {
		t.Fatalf("unexpected typeID %d", asdu[0])
	}
}

func TestParseDeviceConfigDefaults(t *testing.T) {
	cfg := parseDeviceConfig(nil)
	if cfg.Port != 2404 || cfg.CommonAddress != 1 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

package omron

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	finslib "github.com/anviod/fins"
)

func TestParseOmronAddress(t *testing.T) {
	valid := []string{"D100", "CIO1.2", "W3.4", "EM10.100", "H4.15L", "A0", "F0"}
	for _, addr := range valid {
		if err := ParseOmronAddress(addr); err != nil {
			t.Errorf("expected valid address %s, got %v", addr, err)
		}
	}

	invalid := []string{"", "X100", "DB1.DBD0", "D100.1.2.3"}
	for _, addr := range invalid {
		if err := ParseOmronAddress(addr); err == nil {
			t.Errorf("expected invalid address %s", addr)
		}
	}
}

func TestToFinsLibConfig(t *testing.T) {
	cfg := map[string]any{
		"ip":                 "192.168.1.10",
		"port":               9600,
		"timeout":            2000,
		"max_retries":        3,
		"heartbeat_interval": 30000,
		"src_node_addr":      2,
		"dst_node_addr":      1,
	}

	out := toFinsLibConfig(cfg)
	if out["plcIP"] != "192.168.1.10" {
		t.Fatalf("expected plcIP mapping, got %v", out["plcIP"])
	}
	if out["plcPort"] != 9600 {
		t.Fatalf("expected plcPort mapping, got %v", out["plcPort"])
	}
	if out["maxRetries"] != 3 {
		t.Fatalf("expected maxRetries mapping, got %v", out["maxRetries"])
	}
	if out["heartbeatInterval"] != 30000 {
		t.Fatalf("expected heartbeatInterval mapping, got %v", out["heartbeatInterval"])
	}
	if out["srcNodeAddr"] != 2 {
		t.Fatalf("expected srcNodeAddr mapping, got %v", out["srcNodeAddr"])
	}
}

func TestTransportMode(t *testing.T) {
	if transportMode(map[string]any{}) != "TCP" {
		t.Fatal("default mode should be TCP")
	}
	if transportMode(map[string]any{"mode": "udp"}) != "UDP" {
		t.Fatal("expected UDP mode")
	}
}

func TestOmronFinsDriverInitAllowsEmptyIP(t *testing.T) {
	d := NewOmronFinsDriver()
	err := d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"port": 9600},
	})
	if err != nil {
		t.Fatalf("expected init to succeed without plcIP: %v", err)
	}
}

func TestOmronFinsDriverConnectRequiresIP(t *testing.T) {
	d := NewOmronFinsDriver()
	if err := d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"port": 9600},
	}); err != nil {
		t.Fatalf("init: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Connect(ctx); err == nil {
		t.Fatal("expected connect error without plcIP")
	}
}

func TestOmronFinsDriverTCPWithMockPLC(t *testing.T) {
	mock := finslib.NewMockPLC()
	addr, err := mock.Start()
	if err != nil {
		t.Fatalf("mock plc start: %v", err)
	}
	defer mock.Close()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	d := NewOmronFinsDriver()
	cfg := model.DriverConfig{
		ChannelID: "test",
		Config: map[string]any{
			"ip":      host,
			"port":    port,
			"timeout": 2000,
			"mode":    "TCP",
		},
	}
	if err := d.Init(cfg); err != nil {
		t.Fatalf("init: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer d.Disconnect()

	if d.Health() != driver.HealthStatusGood {
		t.Fatalf("expected good health after connect")
	}

	points := []model.Point{
		{ID: "p1", Name: "temp", Address: "D100", DataType: "UINT16"},
	}
	results, err := d.ReadPoints(ctx, points)
	if err != nil {
		t.Fatalf("read points: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results["p1"].Quality != "Good" {
		t.Fatalf("expected good quality, got %s", results["p1"].Quality)
	}

	metrics := d.(interface{ GetMetrics() model.ChannelMetrics }).GetMetrics()
	if metrics.Protocol != "Omron FINS" {
		t.Fatalf("expected Omron FINS protocol, got %s", metrics.Protocol)
	}
}

func TestOmronFinsDriverWritePointWithMockPLC(t *testing.T) {
	mock := finslib.NewMockPLC()
	mock.SetWord(finslib.MemoryAreaDMWord, 100, 1234)

	addr, err := mock.Start()
	if err != nil {
		t.Fatalf("mock plc start: %v", err)
	}
	defer mock.Close()

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	d := NewOmronFinsDriver()
	cfg := model.DriverConfig{
		ChannelID: "test",
		Config: map[string]any{
			"ip":      host,
			"port":    port,
			"timeout": 2000,
			"mode":    "TCP",
		},
	}
	if err := d.Init(cfg); err != nil {
		t.Fatalf("init: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer d.Disconnect()

	point := model.Point{ID: "p1", Name: "d100", Address: "D100", DataType: "UINT16"}
	if err := d.WritePoint(ctx, point, uint16(5678)); err != nil {
		t.Fatalf("write point: %v", err)
	}

	results, err := d.ReadPoints(ctx, []model.Point{point})
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if results["p1"].Quality != "Good" {
		t.Fatalf("expected good quality, got %s", results["p1"].Quality)
	}
	if results["p1"].Value.(uint16) != 5678 {
		t.Fatalf("expected 5678, got %v", results["p1"].Value)
	}
}

func TestToFinsDataType(t *testing.T) {
	cases := map[string]finslib.DataType{
		"bool":   finslib.DataTypeBIT,
		"INT16":  finslib.DataTypeINT16,
		"float":  finslib.DataTypeFLOAT,
		"STRING": finslib.DataTypeSTRING,
	}
	for in, want := range cases {
		if got := toFinsDataType(in); got != want {
			t.Fatalf("toFinsDataType(%s) = %s, want %s", in, got, want)
		}
	}
}

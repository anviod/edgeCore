package mitsubishi

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

func TestMitsubishiDriverInitAllowsEmptyIP(t *testing.T) {
	d := NewMitsubishiDriver()
	err := d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"port": 5000},
	})
	if err != nil {
		t.Fatalf("expected init to succeed without ip: %v", err)
	}
}

func TestMitsubishiDriverConnectRequiresIP(t *testing.T) {
	d := NewMitsubishiDriver()
	if err := d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"port": 5000},
	}); err != nil {
		t.Fatalf("init: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Connect(ctx); err == nil {
		t.Fatal("expected connect error without ip")
	}
}

func TestMitsubishiDriverWithMockPLC(t *testing.T) {
	mock := NewMockPLC()
	mock.SetWord("D", 100, 1234)
	mock.SetBit("M", 0, true)

	addr, err := mock.Start()
	if err != nil {
		t.Fatalf("mock start: %v", err)
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

	d := NewMitsubishiDriver()
	cfg := model.DriverConfig{
		ChannelID: "test",
		Config: map[string]any{
			"ip":      host,
			"port":    port,
			"timeout": 2000,
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
		t.Fatal("expected good health")
	}

	points := []model.Point{
		{ID: "p1", Name: "d100", Address: "D100", DataType: "INT16"},
		{ID: "p2", Name: "m0", Address: "M0", DataType: "BOOL"},
	}

	results, err := d.ReadPoints(ctx, points)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if results["p1"].Quality != "Good" {
		t.Fatalf("D100 quality = %s", results["p1"].Quality)
	}
	if results["p1"].Value.(int16) != 1234 {
		t.Fatalf("D100 value = %v", results["p1"].Value)
	}
	if results["p2"].Value.(bool) != true {
		t.Fatalf("M0 value = %v", results["p2"].Value)
	}

	if err := d.WritePoint(ctx, model.Point{ID: "p1", Address: "D100", DataType: "INT16"}, int16(5678)); err != nil {
		t.Fatalf("write: %v", err)
	}

	results, err = d.ReadPoints(ctx, []model.Point{{ID: "p1", Address: "D100", DataType: "INT16"}})
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if results["p1"].Value.(int16) != 5678 {
		t.Fatalf("D100 after write = %v", results["p1"].Value)
	}
}

func TestRemoteAddrFromConfig(t *testing.T) {
	addr := remoteAddrFromConfig(map[string]any{"ip": "10.0.0.5", "port": 5007})
	if addr != "10.0.0.5:5007" {
		t.Fatalf("unexpected addr: %s", addr)
	}
}

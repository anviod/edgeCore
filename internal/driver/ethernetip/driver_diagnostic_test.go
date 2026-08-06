package ethernetip

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"

	go_ethernet_ip "github.com/anviod/ethernet-ip"
)

func TestDriverTagRead(t *testing.T) {
	tcp, err := go_ethernet_ip.NewTCP("127.0.0.1", nil)
	if err != nil {
		t.Skipf("无法创建TCP连接: %v", err)
	}
	if err := tcp.Connect(); err != nil {
		t.Skipf("无法连接到模拟器: %v", err)
	}
	defer tcp.Close()

	tags := []string{
		"Program:MainProgram.BoolTag",
		"Program:MainProgram.IntTag",
		"Program:MainProgram.SintTag",
	}

	for _, tagName := range tags {
		t.Run(tagName, func(t *testing.T) {
			tag := new(go_ethernet_ip.Tag)
			err := tcp.InitializeTag(tagName, tag)
			if err != nil {
				t.Errorf("InitializeTag %s failed: %v", tagName, err)
				return
			}

			err = tag.Read()
			if err != nil {
				t.Errorf("Read %s failed: %v", tagName, err)
				return
			}

			t.Logf("Successfully read %s", tagName)
		})
	}
}

func TestDriverFullReadPoints(t *testing.T) {
	driver := NewEtherNetIPDriver()

	config := model.DriverConfig{
		Config: map[string]any{
			"ip":      "127.0.0.1",
			"port":    44818,
			"timeout": 500,
		},
	}

	err := driver.Init(config)
	if err != nil {
		t.Fatalf("Driver init failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = driver.Connect(ctx)
	if err != nil {
		t.Skipf("无法连接到模拟器: %v", err)
	}
	defer driver.Disconnect()

	points := []model.Point{
		{
			ID:       "TestBool",
			Name:     "TestBool",
			Address:  "Program:MainProgram.BoolTag",
			DataType: "BOOL",
		},
		{
			ID:       "TestInt",
			Name:     "TestInt",
			Address:  "Program:MainProgram.IntTag",
			DataType: "INT",
		},
	}

	results, err := driver.ReadPoints(context.Background(), points)
	if err != nil {
		t.Fatalf("ReadPoints failed: %v", err)
	}

	for id, value := range results {
		t.Logf("Point %s: Quality=%s", id, value.Quality)
		if value.Quality != "Good" {
			t.Errorf("Point %s has bad quality", id)
		}
	}
}

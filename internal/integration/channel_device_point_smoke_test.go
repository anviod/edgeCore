package integration_test

import (
	"testing"

	"github.com/anviod/edgeCore/internal/core"
	_ "github.com/anviod/edgeCore/internal/driver/bacnet"
	_ "github.com/anviod/edgeCore/internal/driver/dlt645"
	_ "github.com/anviod/edgeCore/internal/driver/ethernetip"
	_ "github.com/anviod/edgeCore/internal/driver/ice104"
	_ "github.com/anviod/edgeCore/internal/driver/knxnetip"
	_ "github.com/anviod/edgeCore/internal/driver/mitsubishi"
	_ "github.com/anviod/edgeCore/internal/driver/modbus"
	_ "github.com/anviod/edgeCore/internal/driver/omron"
	_ "github.com/anviod/edgeCore/internal/driver/opcua"
	_ "github.com/anviod/edgeCore/internal/driver/s7"
	_ "github.com/anviod/edgeCore/internal/driver/snmp"
	"github.com/anviod/edgeCore/internal/model"
)

type smokeFixture struct {
	protocol string
	config   map[string]any
	device   model.Device
	point    model.Point
}

func smokeFixtures() []smokeFixture {
	return []smokeFixture{
		{
			protocol: "modbus-tcp",
			config:   map[string]any{"url": "tcp://127.0.0.1:502"},
			device:   model.Device{ID: "dev-modbus-tcp", Name: "Modbus TCP", Config: map[string]any{"slave_id": 1}},
			point:    model.Point{ID: "pt-modbus-tcp", Name: "HR0", Address: "0", DataType: "int16"},
		},
		{
			protocol: "modbus-rtu",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-modbus-rtu", Name: "Modbus RTU", Config: map[string]any{"slave_id": 1}},
			point:    model.Point{ID: "pt-modbus-rtu", Name: "HR0", Address: "0", DataType: "int16"},
		},
		{
			protocol: "modbus-rtu-over-tcp",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-modbus-rtu-tcp", Name: "Modbus RTU/TCP", Config: map[string]any{"slave_id": 1}},
			point:    model.Point{ID: "pt-modbus-rtu-tcp", Name: "HR0", Address: "0", DataType: "int16"},
		},
		{
			protocol: "bacnet-ip",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-bacnet", Name: "BACnet Device", Config: map[string]any{"device_id": 1001}},
			point:    model.Point{ID: "pt-bacnet", Name: "AV1", Address: "AnalogValue:1", DataType: "float32"},
		},
		{
			protocol: "opc-ua",
			config:   map[string]any{"url": "opc.tcp://127.0.0.1:4840"},
			device:   model.Device{ID: "dev-opcua", Name: "OPC UA Device", Config: map[string]any{"endpoint": "opc.tcp://127.0.0.1:4840"}},
			point:    model.Point{ID: "pt-opcua", Name: "Tag", Address: "ns=2;s=tag", DataType: "float32"},
		},
		{
			protocol: "s7",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-s7", Name: "S7 Device", Config: map[string]any{"rack": 0, "slot": 1}},
			point:    model.Point{ID: "pt-s7", Name: "DB0", Address: "DB1.DBD0", DataType: "float32"},
		},
		{
			protocol: "ethernet-ip",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-enip", Name: "ENIP Device"},
			point:    model.Point{ID: "pt-enip", Name: "Tag1", Address: "Tag1", DataType: "float32"},
		},
		{
			protocol: "omron-fins",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-fins", Name: "FINS Device"},
			point:    model.Point{ID: "pt-fins", Name: "D100", Address: "D100", DataType: "INT16"},
		},
		{
			protocol: "knxnet-ip",
			config:   map[string]any{"ip": "127.0.0.1", "port": 3671, "discovery": false},
			device:   model.Device{ID: "dev-knx", Name: "KNX Device"},
			point:    model.Point{ID: "pt-knx", Name: "GA", Address: "1/2/3", DataType: "BOOL"},
		},
		{
			protocol: "mitsubishi-slmp",
			config:   map[string]any{"port": 5000},
			device:   model.Device{ID: "dev-mc", Name: "MC Device"},
			point:    model.Point{ID: "pt-mc", Name: "D100", Address: "D100", DataType: "INT16"},
		},
		{
			protocol: "iec60870-5-104",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-ice104", Name: "ICE104 Device"},
			point:    model.Point{ID: "pt-ice104", Name: "IOA1", Address: "1", DataType: "float32"},
		},
		{
			protocol: "snmp",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-snmp", Name: "SNMP Device", Config: map[string]any{"community": "public"}},
			point:    model.Point{ID: "pt-snmp", Name: "SysDescr", Address: "1.3.6.1.2.1.1.1.0", DataType: "string"},
		},
		{
			protocol: "dlt645",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-dlt645", Name: "DLT645 Device", Config: map[string]any{"station_address": "123456789012"}},
			point:    model.Point{ID: "pt-dlt645", Name: "Voltage", Address: "123456789012#02-01-01-00", DataType: "uint16"},
		},
	}
}

// TestSmoke_ChannelDevicePoint exercises AddChannel → AddDevice → AddPoint for every
// southbound protocol with minimal valid config (enable=false avoids Connect).
func TestSmoke_ChannelDevicePoint_AllProtocols(t *testing.T) {
	for _, fx := range smokeFixtures() {
		t.Run(fx.protocol, func(t *testing.T) {
			cm := core.NewChannelManager(nil, nil)

			channelID := "smoke-ch-" + fx.protocol
			ch := &model.Channel{
				ID:       channelID,
				Name:     fx.protocol,
				Protocol: fx.protocol,
				Enable:   false,
				Config:   fx.config,
			}
			if err := cm.AddChannel(ch); err != nil {
				t.Fatalf("AddChannel: %v", err)
			}

			dev := fx.device
			if err := cm.AddDevice(channelID, &dev); err != nil {
				t.Fatalf("AddDevice: %v", err)
			}

			pt := fx.point
			if err := cm.AddPoint(channelID, dev.ID, &pt); err != nil {
				t.Fatalf("AddPoint: %v", err)
			}
		})
	}
}

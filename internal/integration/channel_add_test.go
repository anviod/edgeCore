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
	_ "github.com/anviod/edgeCore/internal/driver/profinetio"
	_ "github.com/anviod/edgeCore/internal/driver/s7"
	_ "github.com/anviod/edgeCore/internal/driver/snmp"
	"github.com/anviod/edgeCore/internal/model"
)

func TestAddChannel_AllProtocols_MinimalConfig(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		config   map[string]any
		wantErr  bool
	}{
		{name: "modbus-tcp", protocol: "modbus-tcp", config: map[string]any{}},
		{name: "modbus-tcp-url", protocol: "modbus-tcp", config: map[string]any{"url": "tcp://127.0.0.1:502"}},
		{name: "modbus-rtu-over-tcp", protocol: "modbus-rtu-over-tcp", config: map[string]any{}},
		{name: "modbus-rtu", protocol: "modbus-rtu", config: map[string]any{}},
		{name: "bacnet-ip", protocol: "bacnet-ip", config: map[string]any{}},
		{name: "opc-ua", protocol: "opc-ua", config: map[string]any{}},
		{name: "s7", protocol: "s7", config: map[string]any{}},
		{name: "profinet-io", protocol: "profinet-io", config: map[string]any{}},
		{name: "profinet-io-simulation", protocol: "profinet-io", config: map[string]any{"simulation": true}},
		{name: "ethernet-ip", protocol: "ethernet-ip", config: map[string]any{}},
		{name: "omron-fins", protocol: "omron-fins", config: map[string]any{}},
		{name: "knxnet-ip-no-ip", protocol: "knxnet-ip", config: map[string]any{"port": 3671}},
		{name: "knxnet-ip-with-ip", protocol: "knxnet-ip", config: map[string]any{"ip": "192.168.1.50"}},
		{name: "knxnet-ip-discovery", protocol: "knxnet-ip", config: map[string]any{"discovery": true}},
		{name: "knxnet-ip-nil-config", protocol: "knxnet-ip", config: nil},
		{name: "snmp", protocol: "snmp", config: map[string]any{}},
		{name: "iec60870-5-104", protocol: "iec60870-5-104", config: map[string]any{}},
		{name: "dlt645", protocol: "dlt645", config: map[string]any{}},
		{name: "mitsubishi-no-ip", protocol: "mitsubishi-slmp", config: map[string]any{"port": 5000}},
		{name: "unknown-protocol", protocol: "not-real", config: map[string]any{}, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := core.NewChannelManager(nil, nil)

			ch := &model.Channel{
				ID:       "ch-" + tc.name,
				Name:     tc.name,
				Protocol: tc.protocol,
				Enable:   false,
				Config:   tc.config,
			}
			err := cm.AddChannel(ch)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("AddChannel(%s): %v", tc.name, err)
			}
		})
	}
}

func TestAddChannel_ModbusAutoPoints(t *testing.T) {
	cm := core.NewChannelManager(nil, nil)

	ch := &model.Channel{
		ID:       "ch-modbus-auto",
		Name:     "Modbus Auto",
		Protocol: "modbus-tcp",
		Enable:   false,
		Config:   map[string]any{"url": "tcp://127.0.0.1:502"},
		Devices: []model.Device{
			{
				ID:     "dev-auto",
				Name:   "Auto Device",
				Config: map[string]any{"slave_id": 1, "auto_points_range": "0-5"},
				Points: []model.Point{},
			},
		},
	}
	if err := cm.AddChannel(ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}
	if len(ch.Devices[0].Points) == 0 {
		t.Fatal("expected auto-generated modbus points")
	}
}

type protocolFixture struct {
	protocol string
	config   map[string]any
	device   model.Device
	point    model.Point
}

func protocolFixtures() []protocolFixture {
	return []protocolFixture{
		{
			protocol: "modbus-tcp",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-modbus", Name: "Modbus Device", Config: map[string]any{"slave_id": 1}},
			point:    model.Point{ID: "pt-modbus", Name: "HR0", Address: "0", DataType: "int16"},
		},
		{
			protocol: "modbus-rtu",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-modbus-rtu-serial", Name: "Modbus RTU Serial", Config: map[string]any{"slave_id": 1}},
			point:    model.Point{ID: "pt-modbus-rtu-serial", Name: "HR0", Address: "0", DataType: "int16"},
		},
		{
			protocol: "modbus-rtu-over-tcp",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-modbus-rtu", Name: "Modbus RTU", Config: map[string]any{"slave_id": 1}},
			point:    model.Point{ID: "pt-modbus-rtu", Name: "HR0", Address: "0", DataType: "int16"},
		},
		{
			protocol: "bacnet-ip",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-bacnet", Name: "BACnet Device", Config: map[string]any{"device_id": 1001}},
			point:    model.Point{ID: "pt-bacnet", Name: "AV1", Address: "AnalogValue:1", DataType: "float32"},
		},
		{
			protocol: "opc-ua",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-opcua", Name: "OPC UA Device"},
			point:    model.Point{ID: "pt-opcua", Name: "Node", Address: "ns=2;i=3", DataType: "float32"},
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
			config:   map[string]any{"port": 3671},
			device:   model.Device{ID: "dev-knx", Name: "KNX Device"},
			point:    model.Point{ID: "pt-knx", Name: "GA", Address: "1/2/3", DataType: "int16"},
		},
		{
			protocol: "mitsubishi-slmp",
			config:   map[string]any{"port": 5000},
			device:   model.Device{ID: "dev-mc", Name: "MC Device"},
			point:    model.Point{ID: "pt-mc", Name: "D100", Address: "D100", DataType: "INT16"},
		},
		{
			protocol: "snmp",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-snmp", Name: "SNMP Device", Config: map[string]any{"community": "public"}},
			point:    model.Point{ID: "pt-snmp", Name: "SysDescr", Address: "1.3.6.1.2.1.1.1.0", DataType: "string"},
		},
		{
			protocol: "iec60870-5-104",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-ice104", Name: "ICE104 Device"},
			point:    model.Point{ID: "pt-ice104", Name: "IOA1", Address: "1", DataType: "float32"},
		},
		{
			protocol: "dlt645",
			config:   map[string]any{},
			device:   model.Device{ID: "dev-dlt645", Name: "DLT645 Device", Config: map[string]any{"station_address": "123456789012"}},
			point:    model.Point{ID: "pt-dlt645", Name: "Voltage", Address: "123456789012#02-01-01-00", DataType: "uint16"},
		},
		{
			protocol: "profinet-io",
			config:   map[string]any{"simulation": true},
			device: model.Device{
				ID:   "dev-pnio",
				Name: "Profinet IO Device",
				Config: map[string]any{
					"device_name": "io-device-1",
					"ip":          "192.168.1.20",
					"port":        34964,
					"slot":        3,
					"subslot":     1,
				},
			},
			point: model.Point{ID: "pt-pnio", Name: "Input0", Address: "3:1:0", DataType: "int16"},
		},
	}
}

func TestAddDevice_AllProtocols_MinimalConfig(t *testing.T) {
	for _, fx := range protocolFixtures() {
		t.Run(fx.protocol, func(t *testing.T) {
			cm := core.NewChannelManager(nil, nil)

			channelID := "ch-" + fx.protocol
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
		})
	}
}

func TestAddPoint_AllProtocols_MinimalConfig(t *testing.T) {
	for _, fx := range protocolFixtures() {
		t.Run(fx.protocol, func(t *testing.T) {
			cm := core.NewChannelManager(nil, nil)

			channelID := "ch-" + fx.protocol
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

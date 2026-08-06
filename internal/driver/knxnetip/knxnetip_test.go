package knxnetip

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGroupAddress(t *testing.T) {
	tests := []struct {
		addr string
		want uint16
	}{
		{"1/2/3", 0x0A03},
		{"0/0/1", 0x0001},
		{"15/7/255", 0x7FFF},
		{"1/34", 0x0822},
	}
	for _, tt := range tests {
		got, err := parseGroupAddress(tt.addr)
		require.NoError(t, err, tt.addr)
		assert.Equal(t, tt.want, got, tt.addr)
	}
}

func TestParseAddressWithIndividual(t *testing.T) {
	parsed, err := ParseAddress("1/2/3,1.1.1")
	require.NoError(t, err)
	assert.Equal(t, uint16(0x0A03), parsed.GroupAddr)
	assert.Equal(t, uint16(0x1101), parsed.IndividualAddr)

	parsed, err = ParseAddress("0/0/1,1.1.1,2")
	require.NoError(t, err)
	assert.Equal(t, 2, parsed.BitWidth)
}

func TestDecodeValue(t *testing.T) {
	v, err := DecodeValue([]byte{0x01}, "BOOL", nil, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, true, v)

	v, err = DecodeValue([]byte{0x00, 0x64}, "UINT16", nil, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, float64(100), v)

	addr := &ParsedAddress{BitWidth: 2}
	v, err = DecodeValue([]byte{0xC0}, "BIT", addr, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, true, v)
}

func TestDriverWithSimulator(t *testing.T) {
	sim := NewSimulator()
	group, _ := parseGroupAddress("1/2/3")
	sim.SetGroupValue(group, []byte{0x00, 0x2A})

	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	d, ok := driver.GetDriver("knxnet-ip")
	require.True(t, ok)

	err = d.Init(model.DriverConfig{
		ChannelID: "test-knx",
		Config: map[string]any{
			"ip":                 host,
			"port":               mustAtoi(portStr),
			"timeout":            2000,
			"heartbeat_interval": 0,
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = d.Connect(ctx)
	require.NoError(t, err)
	defer d.Disconnect()

	assert.Equal(t, driver.HealthStatusGood, d.Health())

	points := []model.Point{
		{ID: "p1", Name: "temp", Address: "1/2/3", DataType: "UINT16"},
		{ID: "p2", Name: "sw", Address: "0/0/1", DataType: "BOOL"},
	}

	results, err := d.ReadPoints(ctx, points)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "Good", results["p1"].Quality)
	assert.Equal(t, float64(42), results["p1"].Value)

	// Unconfigured group returns default payload from simulator (0 -> false)
	assert.Equal(t, "Good", results["p2"].Quality)
	assert.Equal(t, false, results["p2"].Value)
}

func TestWritePointWithSimulator(t *testing.T) {
	sim := NewSimulator()
	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	group, _ := parseGroupAddress("5/1/1")
	sim.SetGroupValue(group, []byte{0x00})

	d := NewKNXnetIPDriver()
	err = d.Init(model.DriverConfig{
		ChannelID: "test-knx-write",
		Config: map[string]any{
			"ip":                 host,
			"port":               mustAtoi(portStr),
			"timeout":            2000,
			"heartbeat_interval": 0,
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = d.Connect(ctx)
	require.NoError(t, err)
	defer d.Disconnect()

	err = d.WritePoint(ctx, model.Point{
		ID: "w1", Name: "switch", Address: "5/1/1", DataType: "BOOL",
	}, true)
	require.NoError(t, err)

	sim.mu.Lock()
	written := sim.values[group]
	sim.mu.Unlock()
	assert.Equal(t, byte(0x01), written[0])
}

func TestInitAllowsMissingIP(t *testing.T) {
	d := NewKNXnetIPDriver()
	err := d.Init(model.DriverConfig{
		ChannelID: "test",
		Config:    map[string]any{"port": 3671},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = d.Connect(ctx)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "ip") || strings.Contains(err.Error(), "discovery"))
}

func TestDriverTCPWithSimulator(t *testing.T) {
	sim := NewSimulator()
	group, _ := parseGroupAddress("1/2/3")
	sim.SetGroupValue(group, []byte{0x00, 0x2A})

	addr, err := sim.StartTCP()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	d := NewKNXnetIPDriver()
	err = d.Init(model.DriverConfig{
		ChannelID: "test-knx-tcp",
		Config: map[string]any{
			"ip":                 host,
			"port":               mustAtoi(portStr),
			"mode":               "TCP",
			"timeout":            2000,
			"heartbeat_interval": 500,
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = d.Connect(ctx)
	require.NoError(t, err)
	defer d.Disconnect()

	assert.Equal(t, driver.HealthStatusGood, d.Health())

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Name: "temp", Address: "1/2/3", DataType: "UINT16"},
	})
	require.NoError(t, err)
	assert.Equal(t, float64(42), results["p1"].Value)

	time.Sleep(600 * time.Millisecond)
	assert.Equal(t, driver.HealthStatusGood, d.Health())
}

func TestDiscoverGatewaysWithSimulator(t *testing.T) {
	sim := NewSimulator()
	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	gateways, err := DiscoverGateways(ctx, transportConfig{
		discoveryMulticast: net.JoinHostPort(host, portStr),
		discoveryTimeout:   2 * time.Second,
	})
	require.NoError(t, err)
	require.NotEmpty(t, gateways)
	assert.Equal(t, host, gateways[0].IP)
}

func TestConnectWithDiscovery(t *testing.T) {
	sim := NewSimulator()
	group, _ := parseGroupAddress("2/1/5")
	sim.SetGroupValue(group, []byte{0x01})

	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	d := NewKNXnetIPDriver()
	err = d.Init(model.DriverConfig{
		ChannelID: "test-knx-discover",
		Config: map[string]any{
			"discovery":           true,
			"discovery_multicast": net.JoinHostPort(host, portStr),
			"discovery_timeout":   2000,
			"timeout":             2000,
			"heartbeat_interval":  0,
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = d.Connect(ctx)
	require.NoError(t, err)
	defer d.Disconnect()

	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Name: "sw", Address: "2/1/5", DataType: "BOOL"},
	})
	require.NoError(t, err)
	assert.Equal(t, true, results["p1"].Value)
}

func TestConcurrentReadsWithSimulator(t *testing.T) {
	sim := NewSimulator()
	group, _ := parseGroupAddress("3/0/1")
	sim.SetGroupValue(group, []byte{0x00, 0x0A})

	addr, err := sim.Start()
	require.NoError(t, err)
	defer sim.Close()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	d := NewKNXnetIPDriver()
	err = d.Init(model.DriverConfig{
		ChannelID: "test-knx-load",
		Config: map[string]any{
			"ip":                 host,
			"port":               mustAtoi(portStr),
			"timeout":            3000,
			"heartbeat_interval": 0,
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = d.Connect(ctx)
	require.NoError(t, err)
	defer d.Disconnect()

	point := model.Point{ID: "p1", Name: "val", Address: "3/0/1", DataType: "UINT16"}
	const workers = 10
	const readsPerWorker = 5

	var wg sync.WaitGroup
	errCh := make(chan error, workers*readsPerWorker)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < readsPerWorker; j++ {
				results, err := d.ReadPoints(ctx, []model.Point{point})
				if err != nil {
					errCh <- err
					return
				}
				if results["p1"].Quality != "Good" {
					errCh <- fmt.Errorf("bad quality")
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func mustAtoi(s string) int {
	var n int
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

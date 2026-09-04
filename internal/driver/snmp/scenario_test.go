package snmp

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_InvalidV3Config(t *testing.T) {
	transport := NewSNMPTransport(map[string]any{
		"snmpVersion":   "v3",
		"securityLevel": "authPriv",
		"securityName":  "admin",
		"ip":            "127.0.0.1",
	})
	_, err := transport.buildClient()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authPassword")
}

func TestScenario_TimeoutConfig(t *testing.T) {
	cfg := parseDeviceConfig(map[string]any{
		"ip":      "127.0.0.1",
		"timeout": 500,
		"retries": 3,
	})
	assert.Equal(t, 500*time.Millisecond, cfg.Timeout)
	assert.Equal(t, 3, cfg.Retries)
}

func TestScenario_DisconnectRecordsLastDisconnect(t *testing.T) {
	transport := NewSNMPTransport(map[string]any{"ip": "127.0.0.1"})
	transport.connected.Store(true)
	transport.connectTime = time.Now().Add(-time.Second)

	before := time.Now()
	require.NoError(t, transport.Disconnect())
	_, _, _, _, lastDisc := transport.GetConnectionMetrics()
	assert.False(t, lastDisc.IsZero())
	assert.True(t, !lastDisc.Before(before))
	assert.False(t, transport.IsConnected())
}

func TestScenario_InvalidAddressReadPoints(t *testing.T) {
	dec := NewSNMPDecoder()
	cfg := parseDeviceConfig(map[string]any{"community": "public"})
	_, err := dec.ParseAddress("not-valid", cfg)
	require.Error(t, err)

	transport := NewSNMPTransport(map[string]any{"ip": "127.0.0.1", "community": "public"})
	transport.connected.Store(true)
	transport.client = &gosnmp.GoSNMP{}

	scheduler := NewSNMPScheduler(transport, NewSNMPDecoder(), map[string]any{"community": "public"})
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Address: "not-valid", DataType: "STRING"},
	})
	require.NoError(t, err)
	assert.NotContains(t, results, "bad")
}

func TestScenario_ConcurrentSchedulerStats(t *testing.T) {
	transport := NewSNMPTransport(map[string]any{"ip": "127.0.0.1", "community": "public"})
	transport.getHook = func(oids []string, community string) ([]gosnmp.SnmpPDU, error) {
		return []gosnmp.SnmpPDU{{
			Name:  oids[0],
			Type:  gosnmp.Integer,
			Value: 1,
		}}, nil
	}
	transport.connected.Store(true)
	transport.client = &gosnmp.GoSNMP{}

	scheduler := NewSNMPScheduler(transport, NewSNMPDecoder(), map[string]any{"community": "public"})
	point := model.Point{ID: "p1", Address: "public|1.3.6.1.2.1.1.1.0", DataType: "INT32"}

	var wg sync.WaitGroup
	var ops int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = scheduler.ReadPoints(context.Background(), []model.Point{point})
			atomic.AddInt32(&ops, 1)
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(20), atomic.LoadInt32(&ops))

	total, success, _ := scheduler.GetStats()
	assert.Equal(t, int64(20), total)
	assert.Equal(t, int64(20), success)
}

func TestScenario_ConnectProbeFailureLeavesDisconnected(t *testing.T) {
	transport := NewSNMPTransport(map[string]any{
		"ip":      "127.0.0.1",
		"port":    59996,
		"timeout": 100,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := transport.Connect(ctx)
	require.Error(t, err)
	assert.False(t, transport.IsConnected())
}

func TestScenario_DeviceFaultIsolation(t *testing.T) {
	transport := NewSNMPTransport(map[string]any{"ip": "127.0.0.1", "community": "public"})
	transport.getHook = func(oids []string, community string) ([]gosnmp.SnmpPDU, error) {
		if oids[0] == "1.3.6.1.2.1.1.1.0" {
			return []gosnmp.SnmpPDU{{Name: oids[0], Type: gosnmp.Integer, Value: 1}}, nil
		}
		return nil, fmt.Errorf("no such oid")
	}
	transport.connected.Store(true)
	transport.client = &gosnmp.GoSNMP{}

	scheduler := NewSNMPScheduler(transport, NewSNMPDecoder(), map[string]any{"community": "public"})
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "good", Address: "public|1.3.6.1.2.1.1.1.0", DataType: "INT32"},
		{ID: "bad", Address: "public|9.9.9.9.0", DataType: "INT32"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["good"].Quality)
	assert.Equal(t, "Bad", results["bad"].Quality)
	assert.True(t, transport.IsConnected())
}

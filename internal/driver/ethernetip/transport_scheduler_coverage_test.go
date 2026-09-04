package ethernetip

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	go_ethernet_ip "github.com/anviod/ethernet-ip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoverage_TransportConnectErrors(t *testing.T) {
	tr := NewENIPTransport(map[string]any{"max_retries": 0})
	defer tr.connMgr.Close()
	tr.connMgr.SetMaxRetries(0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := tr.Connect(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IP address not configured")

	tr2 := NewENIPTransport(map[string]any{"ip": "10.0.0.1", "max_retries": 0})
	defer tr2.connMgr.Close()
	tr2.connMgr.SetMaxRetries(0)
	tr2.tcpFactory = func(string, *go_ethernet_ip.Config) (*go_ethernet_ip.EIPTCP, error) {
		return nil, fmt.Errorf("injected factory error")
	}
	err = tr2.Connect(ctx)
	require.Error(t, err)
}

func TestCoverage_TransportLifecycleAndMetrics(t *testing.T) {
	tr := NewENIPTransport(map[string]any{
		"ip": "192.168.1.5", "port": float64(44818), "slot": 2,
		"timeout": float64(500), "max_retries": 1, "max_fail_count": float64(2),
		"collect_cycle": float64(1000), "connection_type": "logix",
	})
	defer tr.connMgr.Close()

	assert.False(t, tr.NeedProbeCheck())
	tr.lastActivityTime.Store(time.Now().Add(-5 * time.Second))
	assert.True(t, tr.NeedProbeCheck())

	tr.connectTime = time.Now().Add(-3 * time.Second)
	tr.connected.Store(true)
	tr.reconnectCount.Store(2)
	tr.localAddr = "127.0.0.1:50100"
	tr.remoteAddr = "192.168.1.5:44818"

	sec, recon, local, remote, _ := tr.GetConnectionMetrics()
	assert.GreaterOrEqual(t, sec, int64(2))
	assert.Equal(t, int64(2), recon)
	assert.Equal(t, "127.0.0.1:50100", local)
	assert.Contains(t, remote, "192.168.1.5")

	require.NoError(t, tr.Disconnect())
	assert.False(t, tr.IsConnected())
}

func TestCoverage_SchedulerReadPointsReconnectFail(t *testing.T) {
	tr := NewENIPTransport(map[string]any{"ip": "10.0.0.99", "max_retries": 0})
	defer tr.connMgr.Close()
	tr.connMgr.SetMaxRetries(0)
	tr.tcpFactory = func(string, *go_ethernet_ip.Config) (*go_ethernet_ip.EIPTCP, error) {
		return nil, fmt.Errorf("mock connect refused")
	}

	s := NewENIPScheduler(tr, NewENIPDecoder(), map[string]any{"batch_read_max": 2})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := s.ReadPoints(ctx, []model.Point{
		{ID: "p1", Name: "Tag1", Address: "MyTag", DataType: "DINT"},
	})
	require.Error(t, err)
}

func TestCoverage_SchedulerReadBadAddressAndBatchGroup(t *testing.T) {
	tr := NewENIPTransport(map[string]any{"ip": "127.0.0.1"})
	defer tr.connMgr.Close()

	tcp, err := go_ethernet_ip.NewTCP("127.0.0.1", nil)
	require.NoError(t, err)
	tr.tcp = tcp
	tr.connected.Store(true)
	tr.connectTime = time.Now()

	s := NewENIPScheduler(tr, NewENIPDecoder(), map[string]any{"batch_read_max": 2})

	results, err := s.ReadPoints(context.Background(), []model.Point{
		{ID: "bad", Name: "Bad", Address: "", DataType: "DINT"},
		{ID: "p1", Name: "T1", Address: "TagA", DataType: "DINT"},
		{ID: "p2", Name: "T2", Address: "TagB", DataType: "DINT"},
		{ID: "p3", Name: "T3", Address: "TagC", DataType: "REAL"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["bad"].Quality)
	assert.Equal(t, "Bad", results["p1"].Quality)

	total, _, failures := s.GetStats()
	assert.Greater(t, total+failures, int64(0))
}

func TestCoverage_SchedulerLogixClass2ReadFail(t *testing.T) {
	tr := NewENIPTransport(map[string]any{
		"ip": "127.0.0.1", "connection_type": "logix",
	})
	defer tr.connMgr.Close()

	tcp, err := go_ethernet_ip.NewTCP("127.0.0.1", nil)
	require.NoError(t, err)
	tr.tcp = tcp
	tr.connected.Store(true)

	s := NewENIPScheduler(tr, NewENIPDecoder(), nil)
	results, err := s.ReadPoints(context.Background(), []model.Point{
		{ID: "dint", Name: "DintTag", Address: "Program:Main.DintTag", DataType: "DINT"},
		{ID: "reg", Name: "Regular", Address: "SomeOtherTag", DataType: "REAL"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["dint"].Quality)
	assert.Equal(t, "Bad", results["reg"].Quality)
}

func TestCoverage_SchedulerToInt64Uint64(t *testing.T) {
	assert.Equal(t, int64(42), toInt64(float64(42)))
	assert.Equal(t, int64(-1), toInt64(int8(-1)))
	assert.Equal(t, int64(0), toInt64("bad"))

	assert.Equal(t, uint64(100), toUint64(float64(100)))
	assert.Equal(t, uint64(5), toUint64(uint8(5)))
	assert.Equal(t, uint64(0), toUint64("bad"))
}

func TestCoverage_GroupTagsSplitting(t *testing.T) {
	s := &ENIPScheduler{batchReadMax: 2}
	points := []pointWithTag{
		{Point: model.Point{ID: "1"}},
		{Point: model.Point{ID: "2"}},
		{Point: model.Point{ID: "3"}},
		{Point: model.Point{ID: "4"}},
		{Point: model.Point{ID: "5"}},
	}
	groups := s.groupTags(points)
	require.Len(t, groups, 3)
	assert.Len(t, groups[0], 2)
	assert.Len(t, groups[2], 1)
}

func TestCoverage_TransportRecordFailureReconnect(t *testing.T) {
	tr := NewENIPTransport(map[string]any{
		"ip": "10.0.0.1", "max_fail_count": float64(1), "timeout": float64(100),
	})
	defer tr.connMgr.Close()

	for i := 0; i < 3; i++ {
		tr.RecordFailure(fmt.Errorf("read timeout"))
	}
	time.Sleep(50 * time.Millisecond)
}

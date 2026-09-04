package dlt645

import (
	"bytes"
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerReadPointsMock(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("02-01-01-00")
	value := []byte{0x20, 0x02}
	respFrame := buildFrame(addr, CtrlReadResp, encode033(append(di[:], value...)))

	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"preambleBytes":  0,
		"timeout":        float64(1000),
	})
	mock := &mockLink{readBuf: bytes.NewBuffer(respFrame)}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return mock, nil
	}
	require.NoError(t, transport.Connect(context.Background()))

	scheduler := NewDLT645Scheduler(transport, NewDLT645Decoder())
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{
			ID:       "p1",
			Name:     "VoltageA",
			Address:  "210220003011#02-01-01-00",
			DataType: "UINT16",
		},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Good", results["p1"].Quality)
	assert.Equal(t, int64(220), results["p1"].Value)

	total, success, failure := scheduler.GetStats()
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(1), success)
	assert.Equal(t, int64(0), failure)
}

func TestSchedulerWritePointMock(t *testing.T) {
	addr, _ := EncodeMeterAddress("210220003011")
	di, _ := ParseDataID("04-00-01-01")
	respFrame := buildFrame(addr, CtrlWriteResp, encode033(di[:]))

	transport := NewDLT645Transport(map[string]any{
		"connectionType": "tcp",
		"ip":             "127.0.0.1",
		"preambleBytes":  0,
		"timeout":        float64(1000),
	})
	mock := &mockLink{readBuf: bytes.NewBuffer(respFrame)}
	transport.linkFactory = func(cfg transportConfig) (frameLink, error) {
		return mock, nil
	}
	require.NoError(t, transport.Connect(context.Background()))

	scheduler := NewDLT645Scheduler(transport, NewDLT645Decoder())
	err := scheduler.WritePoint(context.Background(), model.Point{
		Address:  "210220003011#04-00-01-01",
		DataType: "UINT32",
	}, float64(100))
	require.NoError(t, err)
}

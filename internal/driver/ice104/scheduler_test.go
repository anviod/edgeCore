package ice104

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerReadPointsFromCache(t *testing.T) {
	transport := NewICE104Transport(map[string]any{
		"ip":   "127.0.0.1",
		"port": 2404,
	})
	transport.connected.Store(true)

	decoder := NewICE104Decoder()
	key := decoder.PointKey(typeM_SP_NA_1, 100)
	transport.cacheMu.Lock()
	transport.cache[key] = cachedPoint{
		TypeID:  typeM_SP_NA_1,
		IOA:     100,
		Value:   true,
		Quality: "Good",
		TS:      time.Now(),
	}
	transport.cacheMu.Unlock()

	scheduler := NewICE104Scheduler(transport, decoder, nil)
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{
			ID:         "p1",
			Name:       "switch",
			Address:    "100",
			Group:      "M_SP_NA_1",
			DataType:   "BOOL",
			ReportMode: "event",
		},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Good", results["p1"].Quality)
	assert.Equal(t, true, results["p1"].Value)
}

func TestSchedulerWritePointMock(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		buf := make([]byte, 512)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	transport := NewICE104Transport(map[string]any{
		"ip":            "127.0.0.1",
		"port":          2404,
		"commonAddress": 1,
	})
	transport.conn = client
	transport.connected.Store(true)

	scheduler := NewICE104Scheduler(transport, NewICE104Decoder(), map[string]any{
		"t1": 2,
	})
	err := scheduler.WritePoint(context.Background(), model.Point{
		Address:  "200",
		Group:    "C_SC_NA_1",
		DataType: "BOOL",
	}, true)
	require.NoError(t, err)
}

func TestSchedulerWritePointNotConnected(t *testing.T) {
	scheduler := NewICE104Scheduler(NewICE104Transport(nil), NewICE104Decoder(), nil)
	err := scheduler.WritePoint(context.Background(), model.Point{Address: "1"}, true)
	require.Error(t, err)
}

func TestSchedulerReadPointsMissingCache(t *testing.T) {
	transport := NewICE104Transport(map[string]any{"t1": 0})
	transport.connected.Store(true)

	scheduler := NewICE104Scheduler(transport, NewICE104Decoder(), map[string]any{"t1": 0})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	results, err := scheduler.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "999", Group: "M_SP_NA_1", DataType: "BOOL", ReportMode: "event"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bad", results["p1"].Quality)
}

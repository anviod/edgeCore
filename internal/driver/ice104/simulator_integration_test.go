//go:build integration

package ice104

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func simulatorAvailable(t *testing.T) bool {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:2404", 2*time.Second)
	if err != nil {
		t.Skipf("IEC 60870-5-104 simulator not available at 127.0.0.1:2404: %v", err)
		return false
	}
	_ = conn.Close()
	time.Sleep(100 * time.Millisecond)
	return true
}

func TestIntegrationWithSimulator(t *testing.T) {
	if !simulatorAvailable(t) {
		return
	}

	cfg := map[string]any{
		"ip":            "127.0.0.1",
		"port":          2404,
		"commonAddress": 1,
		"t0":            10,
		"t1":            15,
	}

	transport := NewICE104Transport(cfg)
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer connectCancel()
	require.NoError(t, transport.Connect(connectCtx))
	defer transport.Disconnect()

	assert.True(t, transport.IsConnected())
	_, reconCount, localAddr, remoteAddr, _ := transport.GetConnectionMetrics()
	t.Logf("connected local=%s remote=%s reconnectCount=%d", localAddr, remoteAddr, reconCount)

	scheduler := NewICE104Scheduler(transport, NewICE104Decoder(), cfg)
	readCtx := context.Background()

	t.Run("DriverHealthGood", func(t *testing.T) {
		d := NewICE104Driver().(*ICE104Driver)
		require.NoError(t, d.Init(model.DriverConfig{Config: cfg}))
		d.transport = transport
		d.decoder = NewICE104Decoder()
		d.scheduler = scheduler
		assert.Equal(t, driver.HealthStatusGood, d.Health())
	})

	t.Run("ReadM_ME_NA_1_IOA1", func(t *testing.T) {
		results, err := scheduler.ReadPoints(readCtx, []model.Point{
			{
				ID:         "ai-1",
				Address:    "1",
				Group:      "M_ME_NA_1",
				DataType:   "FLOAT",
				ReportMode: "poll",
			},
		})
		require.NoError(t, err)
		require.Contains(t, results, "ai-1")
		if results["ai-1"].Quality != "Good" {
			t.Skipf("simulator did not return M_ME_NA_1 IOA=1 after general call (quality=%s connected=%v); "+
				"verify Freyr simulator Load Configuration + Start Communication and point IOA=1 CA=1",
				results["ai-1"].Quality, transport.IsConnected())
		}
		assert.NotNil(t, results["ai-1"].Value)
		t.Logf("M_ME_NA_1 IOA=1 value=%v", results["ai-1"].Value)
	})

	t.Run("ReadINT16WithoutGroupFailsCacheLookup", func(t *testing.T) {
		if !transport.IsConnected() {
			t.Skip("transport disconnected after general call; see ReadM_ME_NA_1_IOA1")
		}
		results, err := scheduler.ReadPoints(readCtx, []model.Point{
			{
				ID:         "int16-no-group",
				Address:    "1",
				DataType:   "INT16",
				ReportMode: "poll",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "Bad", results["int16-no-group"].Quality,
			"INT16 without Group resolves to M_ME_NB_1 (11), simulator publishes M_ME_NA_1 (9)")
	})

	t.Run("ReadINT16WithGroupSucceeds", func(t *testing.T) {
		if !transport.IsConnected() {
			t.Skip("transport disconnected after general call; see ReadM_ME_NA_1_IOA1")
		}
		results, err := scheduler.ReadPoints(readCtx, []model.Point{
			{
				ID:         "int16-group",
				Address:    "1",
				Group:      "M_ME_NA_1",
				DataType:   "INT16",
				ReportMode: "poll",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "Good", results["int16-group"].Quality,
			"explicit Group M_ME_NA_1 overrides INT16 default type mapping")
	})
}

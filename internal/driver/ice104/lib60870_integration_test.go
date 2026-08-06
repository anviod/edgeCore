//go:build integration

package ice104

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lib60870ServerAddr() string {
	if v := os.Getenv("ICE104_LIB60870_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:2404"
}

func lib60870ServerAvailable(t *testing.T) (host string, port int, ok bool) {
	t.Helper()
	addr := lib60870ServerAddr()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("lib60870 edgeCore_server not available at %s: %v (see test/lib60870/README.md)", addr, err)
		return "", 0, false
	}
	_ = conn.Close()
	time.Sleep(100 * time.Millisecond)

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Skipf("invalid ICE104_LIB60870_ADDR %q: %v", addr, err)
		return "", 0, false
	}
	port, err = strconv.Atoi(portStr)
	if err != nil {
		t.Skipf("invalid port in ICE104_LIB60870_ADDR %q: %v", addr, err)
		return "", 0, false
	}
	return host, port, true
}

// TestIntegrationWithLib60870Server connects to a running lib60870 CS104 slave
// (test/lib60870/edgeCore_server.c). Start the server manually or via build-windows.bat.
func TestIntegrationWithLib60870Server(t *testing.T) {
	host, port, ok := lib60870ServerAvailable(t)
	if !ok {
		return
	}

	cfg := map[string]any{
		"ip":            host,
		"port":          port,
		"commonAddress": 1,
		"t0":            10,
		"t1":            15,
	}
	transport := NewICE104Transport(cfg)
	ctx := context.Background()
	require.NoError(t, transport.Connect(ctx))
	defer transport.Disconnect()

	assert.True(t, transport.IsConnected())

	scheduler := NewICE104Scheduler(transport, NewICE104Decoder(), cfg)
	results, err := scheduler.ReadPoints(ctx, []model.Point{
		{ID: "ai-1", Address: "1", Group: "M_ME_NA_1", DataType: "FLOAT", ReportMode: "poll"},
	})
	require.NoError(t, err)
	require.Contains(t, results, "ai-1")
	if results["ai-1"].Quality != "Good" {
		t.Fatalf("lib60870 server did not return M_ME_NA_1 IOA=1 (quality=%s connected=%v)",
			results["ai-1"].Quality, transport.IsConnected())
	}
	assert.InDelta(t, 0.5, results["ai-1"].Value.(float64), 0.01)
	assert.True(t, transport.IsConnected())
}

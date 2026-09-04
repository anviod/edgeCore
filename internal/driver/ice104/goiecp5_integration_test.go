//go:build integration

package ice104

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thinkgos/go-iecp5/asdu"
	"github.com/thinkgos/go-iecp5/cs104"
)

const goIecp5CommonAddr asdu.CommonAddr = 1

type goIecp5edgeCoreHandler struct{}

func (goIecp5edgeCoreHandler) InterrogationHandler(c asdu.Connect, _ *asdu.ASDU, qoi asdu.QualifierOfInterrogation) error {
	if qoi != asdu.QOIStation {
		return nil
	}
	// Single I-frame response (matches in-repo mock server). Avoid ACT_CON/ACT_TERM
	// extra I-frames — the driver does not emit S-format acks yet.
	return asdu.MeasuredValueNormal(c, false, asdu.CauseOfTransmission{Cause: asdu.InterrogatedByStation}, goIecp5CommonAddr,
		asdu.MeasuredValueNormalInfo{
			Ioa:   1,
			Value: asdu.Normalize(16384),
			Qds:   asdu.QDSGood,
		})
}

func (goIecp5edgeCoreHandler) CounterInterrogationHandler(asdu.Connect, *asdu.ASDU, asdu.QualifierCountCall) error {
	return nil
}
func (goIecp5edgeCoreHandler) ReadHandler(asdu.Connect, *asdu.ASDU, asdu.InfoObjAddr) error {
	return nil
}
func (goIecp5edgeCoreHandler) ClockSyncHandler(asdu.Connect, *asdu.ASDU, time.Time) error {
	return nil
}
func (goIecp5edgeCoreHandler) ResetProcessHandler(asdu.Connect, *asdu.ASDU, asdu.QualifierOfResetProcessCmd) error {
	return nil
}
func (goIecp5edgeCoreHandler) DelayAcquisitionHandler(asdu.Connect, *asdu.ASDU, uint16) error {
	return nil
}
func (goIecp5edgeCoreHandler) ASDUHandler(asdu.Connect, *asdu.ASDU) error { return nil }

func startGoIecp5Server(t *testing.T) (host string, port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	port, err = strconv.Atoi(portStr)
	require.NoError(t, err)
	require.NoError(t, ln.Close())

	conf := cs104.Config{}
	srv, err := cs104.NewServer(&conf, asdu.ParamsWide, goIecp5edgeCoreHandler{})
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.ListenAndServer(fmt.Sprintf("%s:%d", host, port))
	}()

	addr := fmt.Sprintf("%s:%d", host, port)
	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 50*time.Millisecond)

	return host, port, func() {
		_ = srv.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
}

// TestIntegrationWithGoIecp5Server exercises the driver against thinkgos/go-iecp5
// (open-source CS104 slave) over real TCP. This is the recommended Windows-friendly
// alternative to Freyr / Python-IEC104 when GI + M_ME_NA_1 read must pass.
func TestIntegrationWithGoIecp5Server(t *testing.T) {
	host, port, stop := startGoIecp5Server(t)
	defer stop()

	cfg := map[string]any{
		"ip":            host,
		"port":          port,
		"commonAddress": 1,
		"t0":            5,
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
	require.Equal(t, "Good", results["ai-1"].Quality, "go-iecp5 server should return M_ME_NA_1 IOA=1 after GI")
	require.NotNil(t, results["ai-1"].Value)
	assert.InDelta(t, 0.5, results["ai-1"].Value.(float64), 0.01)
	assert.True(t, transport.IsConnected(), "link should stay up after GI")
}

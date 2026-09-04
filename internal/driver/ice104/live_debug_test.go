//go:build integration

package ice104

import (
	"context"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
)

func TestLiveDebugSimulator(t *testing.T) {
	if !simulatorAvailable(t) {
		return
	}
	cfg := map[string]any{"ip": "127.0.0.1", "port": 2404, "commonAddress": 1, "t0": 10, "t1": 15}
	tr := NewICE104Transport(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer tr.Disconnect()
	t.Log("connected", tr.IsConnected())
	time.Sleep(2 * time.Second)
	t.Log("after spontaneous wait connected", tr.IsConnected())
	if cp, ok := tr.GetCached("9:1"); ok {
		t.Logf("spontaneous cache: %+v", cp)
	}
	gctx, gc := context.WithTimeout(context.Background(), 5*time.Second)
	err := tr.SendGeneralCall(gctx)
	gc()
	t.Log("GI send err", err, "connected", tr.IsConnected())
	for i := 0; i < 50; i++ {
		if cp, ok := tr.GetCached("9:1"); ok {
			t.Logf("cache hit i=%d %+v", i, cp)
			return
		}
		if !tr.IsConnected() {
			t.Fatalf("disconnected at i=%d before cache", i)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("no cache; trying subscribe read without GI")
	sched := NewICE104Scheduler(tr, NewICE104Decoder(), cfg)
	res, err := sched.ReadPoints(context.Background(), []model.Point{
		{ID: "x", Address: "1", Group: "M_ME_NA_1", DataType: "FLOAT", ReportMode: "event"},
	})
	t.Log("event read", res, err, "connected", tr.IsConnected())
}

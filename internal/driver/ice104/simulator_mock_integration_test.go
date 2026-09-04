//go:build integration

package ice104

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startMock104Server(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		expectU := func(send, resp byte) {
			buf := make([]byte, 6)
			if _, err := io.ReadFull(conn, buf); err != nil {
				t.Errorf("read u-frame: %v", err)
				return
			}
			if buf[2] != send {
				t.Errorf("expected u-frame 0x%02x got 0x%02x", send, buf[2])
			}
			_, _ = conn.Write([]byte{startByte, 0x04, resp, 0x00, 0x00, 0x00})
		}
		expectU(uTestFRAct, uTestFRCon)
		expectU(uStartDTAct, uStartDTCon)

		hdr := make([]byte, 2)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			t.Errorf("read gi header: %v", err)
			return
		}
		body := make([]byte, int(hdr[1]))
		if _, err := io.ReadFull(conn, body); err != nil {
			t.Errorf("read gi body: %v", err)
			return
		}
		if len(body) < 5 || body[4] != typeC_IC_NA_1 {
			t.Errorf("unexpected GI frame")
			return
		}

		info := encodeIOA(1)
		raw := make([]byte, 2)
		binary.LittleEndian.PutUint16(raw, 16384)
		info = append(info, raw...)
		info = append(info, 0x00)
		respASDU := buildASDU(typeM_ME_NA_1, 1, cotInterrogated, 1, info)

		recv := binary.LittleEndian.Uint16(body[0:2]) >> 1
		ctrl := make([]byte, 4)
		binary.LittleEndian.PutUint16(ctrl[0:2], uint16(1<<1))
		binary.LittleEndian.PutUint16(ctrl[2:4], recv<<1)
		apduLen := 4 + len(respASDU)
		frame := append([]byte{startByte, byte(apduLen)}, append(ctrl, respASDU...)...)
		if _, err := conn.Write(frame); err != nil {
			t.Errorf("write gi response: %v", err)
		}

		time.Sleep(2 * time.Second)
	}()

	return ln.Addr().String(), func() {
		_ = ln.Close()
		<-done
	}
}

func TestIntegrationWithMock104Server(t *testing.T) {
	addr, stop := startMock104Server(t)
	defer stop()

	host, portStr, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	cfg := map[string]any{
		"ip":            host,
		"port":          intFromAny(portStr, 0),
		"commonAddress": 1,
		"t0":            5,
		"t1":            5,
	}
	transport := NewICE104Transport(cfg)
	ctx := context.Background()
	require.NoError(t, transport.Connect(ctx))
	defer transport.Disconnect()

	scheduler := NewICE104Scheduler(transport, NewICE104Decoder(), cfg)
	results, err := scheduler.ReadPoints(ctx, []model.Point{
		{ID: "ai-1", Address: "1", Group: "M_ME_NA_1", DataType: "FLOAT", ReportMode: "poll"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["ai-1"].Quality)
	assert.InDelta(t, 0.5, results["ai-1"].Value.(float64), 0.01)
}

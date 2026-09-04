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

func TestCoverage_TransportConnectHandshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()
		buf := make([]byte, 6)
		for i := 0; i < 2; i++ {
			if _, err := server.Read(buf); err != nil {
				return
			}
			resp := []byte{startByte, 0x04, buf[2] + 1, 0x00, 0x00, 0x00}
			if buf[2] == uTestFRAct {
				resp[2] = uTestFRCon
			} else if buf[2] == uStartDTAct {
				resp[2] = uStartDTCon
			}
			_, _ = server.Write(resp)
		}
		time.Sleep(50 * time.Millisecond)
	}()

	tr := NewICE104Transport(map[string]any{
		"ip": "127.0.0.1", "port": 2404, "t0": 1000, "t1": 1000,
	})
	tr.conn = client

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Exercise connectOnce handshake paths without full dial
	require.NoError(t, tr.sendUFrame(uTestFRAct))
	require.NoError(t, tr.readUFrame(ctx, uTestFRCon))
	require.NoError(t, tr.sendUFrame(uStartDTAct))
	require.NoError(t, tr.readUFrame(ctx, uStartDTCon))
	tr.connected.Store(true)

	assert.True(t, tr.IsConnected())
	require.NoError(t, tr.Disconnect())
}

func TestCoverage_DecoderAllGroups(t *testing.T) {
	dec := NewICE104Decoder()

	cases := []struct {
		group    string
		addr     string
		dataType string
		typeID   byte
	}{
		{"M_SP_NA_1", "1", "BOOL", typeM_SP_NA_1},
		{"M_ME_NA_1", "2", "FLOAT", typeM_ME_NA_1},
		{"M_ME_NB_1", "3", "INT16", typeM_ME_NB_1},
		{"M_ME_NC_1", "4", "FLOAT", typeM_ME_NC_1},
		{"M_IT_NA_1", "5", "INT32", typeM_IT_NA_1},
	}
	for _, tc := range cases {
		id, ioa, err := dec.PointMeta(model.Point{Address: tc.addr, Group: tc.group, DataType: tc.dataType})
		require.NoError(t, err, tc.group)
		assert.Equal(t, tc.typeID, id)
		assert.Equal(t, uint32(parseAddr(tc.addr)), ioa)
	}
}

func parseAddr(s string) uint32 {
	var v uint32
	for _, c := range s {
		v = v*10 + uint32(c-'0')
	}
	return v
}

func TestCoverage_BuildASDUVariants(t *testing.T) {
	asdu := buildASDU(typeM_SP_NA_1, 1, cotInterrogated, 2, append(encodeIOA(5), 0x01))
	require.NotEmpty(t, asdu)

	_, quality, _, err := decodeObjectBody(typeM_SP_NA_1, []byte{0x01})
	require.NoError(t, err)
	assert.Equal(t, "Good", quality)
}

func TestCoverage_TransportSendIFrameViaPipe(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		defer server.Close()
		buf := make([]byte, 256)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	tr := NewICE104Transport(map[string]any{
		"ip": "127.0.0.1", "port": 2404, "t0": 1000, "t1": 1000,
		"common_address": 1,
	})
	tr.conn = client
	tr.connected.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, tr.SendGeneralCall(ctx))
	require.NoError(t, tr.SendSingleCommand(ctx, 10, true))
}

func TestCoverage_TransportCacheAndMetrics(t *testing.T) {
	tr := NewICE104Transport(map[string]any{"ip": "127.0.0.1", "port": 2404})
	tr.connectTime = time.Now().Add(-2 * time.Second)
	tr.connected.Store(true)
	tr.localAddr = "127.0.0.1:50100"
	tr.remoteAddr = "127.0.0.1:2404"
	tr.reconnectCount.Store(1)

	key := "1:5"
	tr.cacheMu.Lock()
	tr.cache[key] = cachedPoint{TypeID: typeM_SP_NA_1, IOA: 5, Value: true, Quality: "Good", TS: time.Now()}
	tr.cacheMu.Unlock()

	cp, ok := tr.GetCached(key)
	require.True(t, ok)
	assert.Equal(t, "Good", cp.Quality)

	sec, recon, local, remote, _ := tr.GetConnectionMetrics()
	assert.GreaterOrEqual(t, sec, int64(1))
	assert.Equal(t, int64(1), recon)
	assert.Equal(t, "127.0.0.1:50100", local)
	assert.Contains(t, remote, "2404")
}

func TestCoverage_SchedulerReadFromPreseededCache(t *testing.T) {
	tr := NewICE104Transport(map[string]any{"ip": "127.0.0.1", "t1": 200})
	tr.connected.Store(true)
	dec := NewICE104Decoder()
	typeID, ioa, err := dec.PointMeta(model.Point{Address: "5", Group: "M_SP_NA_1", DataType: "BOOL"})
	require.NoError(t, err)
	key := dec.PointKey(typeID, ioa)
	tr.cacheMu.Lock()
	tr.cache[key] = cachedPoint{TypeID: typeM_SP_NA_1, IOA: 5, Value: true, Quality: "Good", TS: time.Now()}
	tr.cacheMu.Unlock()

	s := NewICE104Scheduler(tr, dec, map[string]any{"t1": 500})
	results, err := s.ReadPoints(context.Background(), []model.Point{
		{ID: "p1", Address: "5", Group: "M_SP_NA_1", DataType: "BOOL", ReportMode: "event"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["p1"].Quality)
}

func TestCoverage_DecodeObjectBodyAllTypes(t *testing.T) {
	cases := []struct {
		typeID byte
		raw    []byte
	}{
		{typeM_ME_NA_1, []byte{0x00, 0x64, 0x00}},
		{typeM_ME_NB_1, []byte{0x03, 0xE8, 0x00}},
		{typeM_ME_NC_1, []byte{0x00, 0x00, 0x80, 0x3F, 0x00}},
		{typeM_IT_NA_1, []byte{0x00, 0x00, 0x00, 0x0A, 0x00}},
	}
	for _, tc := range cases {
		val, quality, _, err := decodeObjectBody(tc.typeID, tc.raw)
		require.NoError(t, err, tc.typeID)
		assert.Equal(t, "Good", quality)
		assert.NotNil(t, val)
	}
}

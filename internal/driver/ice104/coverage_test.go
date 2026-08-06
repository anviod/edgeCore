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

func TestDriverLifecycleCoverage(t *testing.T) {
	d := NewICE104Driver().(*ICE104Driver)
	require.NoError(t, d.SetSlaveID(1))
	require.NoError(t, d.SetDeviceConfig(map[string]any{"ip": "127.0.0.1", "port": 2404}))

	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "cov",
		Config:    map[string]any{"ip": "127.0.0.1", "port": 2404},
	}))

	_, _, _, remote, _ := d.GetConnectionMetrics()
	assert.Contains(t, remote, "127.0.0.1")

	assert.Equal(t, driver.HealthStatusBad, d.Health())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := d.Connect(ctx)
	require.Error(t, err)
}

func TestTransportGetCached(t *testing.T) {
	tr := NewICE104Transport(map[string]any{"ip": "127.0.0.1"})
	dec := NewICE104Decoder()
	key := dec.PointKey(typeM_ME_NA_1, 1)
	tr.cacheMu.Lock()
	tr.cache[key] = cachedPoint{TypeID: typeM_ME_NA_1, IOA: 1, Value: 0.5, Quality: "Good", TS: time.Now()}
	tr.cacheMu.Unlock()

	cp, ok := tr.GetCached(key)
	require.True(t, ok)
	assert.Equal(t, 0.5, cp.Value)
}

func TestProtocolEncodeDecodeCoverage(t *testing.T) {
	asdu := buildASDU(typeC_IC_NA_1, 1, cotActivation, 1, nil)
	require.NotEmpty(t, asdu)

	ioa := encodeIOA(100)
	assert.Len(t, ioa, 3)

	parsed := decodeIOA(encodeIOA(65535))
	assert.Equal(t, uint32(65535), parsed)

	value, quality, size, err := decodeObjectBody(typeM_ME_NC_1, []byte{0, 0, 0x80, 0x3F, 0x00})
	require.NoError(t, err)
	assert.Equal(t, "Good", quality)
	assert.Equal(t, 5, size)
	assert.InDelta(t, 1.0, value.(float32), 0.01)

	gi := encodeGeneralInterrogation(0)
	require.NotEmpty(t, gi)
}

func TestDecoderAllTypeIDs(t *testing.T) {
	dec := NewICE104Decoder()

	pts, err := dec.DecodeInformationObject(typeM_SP_NA_1, append(encodeIOA(1), 0x01), false, 1)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	assert.Equal(t, true, pts[0].Value)

	pts, err = dec.DecodeInformationObject(typeM_ME_NA_1, append(encodeIOA(1), 0x00, 0x80, 0x00), false, 1)
	require.NoError(t, err)
	require.Len(t, pts, 1)

	pts, err = dec.DecodeInformationObject(typeM_ME_NC_1, append(encodeIOA(1), 0, 0, 0x80, 0x3F, 0), false, 1)
	require.NoError(t, err)
	require.Len(t, pts, 1)

	pts, err = dec.DecodeInformationObject(typeM_IT_NA_1, append(encodeIOA(1), 0, 0, 0, 0x64, 0), false, 1)
	require.NoError(t, err)
	require.Len(t, pts, 1)

	_, err = dec.DecodeInformationObject(byte(255), append(encodeIOA(1), 0x00), false, 1)
	require.Error(t, err)
}

func TestTransportPipeConnect(t *testing.T) {
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
		time.Sleep(100 * time.Millisecond)
	}()

	tr := NewICE104Transport(map[string]any{
		"ip":   "pipe",
		"port": 2404,
		"t0":   1000,
	})
	tr.conn = client
	tr.localAddr = "local"
	tr.remoteAddr = "remote"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	require.NoError(t, tr.sendUFrame(uTestFRAct))
	require.NoError(t, tr.readUFrame(ctx, uTestFRCon))
	require.NoError(t, tr.sendUFrame(uStartDTAct))
	require.NoError(t, tr.readUFrame(ctx, uStartDTCon))

	tr.connected.Store(true)
	assert.True(t, tr.IsConnected())

	sec, recon, local, remote, _ := tr.GetConnectionMetrics()
	assert.GreaterOrEqual(t, sec, int64(0))
	assert.Equal(t, "local", local)
	assert.Equal(t, "remote", remote)
	_ = recon
}

func TestSendGeneralCallAndSingleCommand(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	var written []byte
	go func() {
		defer server.Close()
		buf := make([]byte, 256)
		for i := 0; i < 2; i++ {
			n, err := server.Read(buf)
			if err != nil {
				return
			}
			written = append(written, buf[:n]...)
		}
	}()

	tr := NewICE104Transport(map[string]any{"commonAddress": 1, "t1": 2})
	tr.conn = client
	tr.connected.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, tr.SendGeneralCall(ctx))
	require.NoError(t, tr.SendSingleCommand(ctx, 10, true))

	require.NotEmpty(t, written)
	assert.Equal(t, byte(startByte), written[0])
}

func TestBuildASDUAndPointMeta(t *testing.T) {
	dec := NewICE104Decoder()
	typeID, ioa, err := dec.PointMeta(model.Point{Address: "1", Group: "M_ME_NA_1", DataType: "FLOAT"})
	require.NoError(t, err)
	assert.Equal(t, byte(typeM_ME_NA_1), typeID)
	assert.Equal(t, uint32(1), ioa)

	typeID, _, err = dec.PointMeta(model.Point{Address: "2", DataType: "INT16"})
	require.NoError(t, err)
	assert.Equal(t, byte(typeM_ME_NB_1), typeID)
}

func TestDriverReadWriteWithCache(t *testing.T) {
	d := NewICE104Driver().(*ICE104Driver)
	require.NoError(t, d.Init(model.DriverConfig{
		ChannelID: "rw",
		Config:    map[string]any{"ip": "127.0.0.1", "port": 2404, "t1": 500},
	}))

	client, server := net.Pipe()
	defer client.Close()
	go func() {
		defer server.Close()
		buf := make([]byte, 256)
		for {
			n, err := server.Read(buf)
			if err != nil {
				return
			}
			if n >= 6 && buf[0] == startByte {
				resp := []byte{startByte, 0x04, buf[2] + 1, 0x00, 0x00, 0x00}
				if buf[2] == uTestFRAct {
					resp[2] = uTestFRCon
				} else if buf[2] == uStartDTAct {
					resp[2] = uStartDTCon
				}
				_, _ = server.Write(resp)
			}
		}
	}()

	d.transport.conn = client
	d.transport.connected.Store(true)
	d.transport.localAddr = "local"
	d.transport.remoteAddr = "remote"

	dec := NewICE104Decoder()
	key := dec.PointKey(typeM_ME_NA_1, 1)
	d.transport.cacheMu.Lock()
	d.transport.cache[key] = cachedPoint{TypeID: typeM_ME_NA_1, IOA: 1, Value: 1.5, Quality: "Good", TS: time.Now()}
	d.transport.cacheMu.Unlock()

	ctx := context.Background()
	results, err := d.ReadPoints(ctx, []model.Point{
		{ID: "p1", Address: "1", Group: "M_ME_NA_1", DataType: "FLOAT", ReportMode: "event"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Good", results["p1"].Quality)

	assert.Equal(t, driver.HealthStatusGood, d.Health())
	require.NoError(t, d.Disconnect())
}

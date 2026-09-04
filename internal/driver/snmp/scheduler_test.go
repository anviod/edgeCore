package snmp

import (
	"context"
	"testing"

	"github.com/anviod/edgeCore/internal/model"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerReadPointsMock(t *testing.T) {
	transport := NewSNMPTransport(map[string]any{
		"ip":        "127.0.0.1",
		"port":      161,
		"community": "public",
	})
	transport.getHook = func(oids []string, community string) ([]gosnmp.SnmpPDU, error) {
		require.Equal(t, []string{"1.3.6.1.2.1.1.1.0"}, oids)
		require.Equal(t, "public", community)
		return []gosnmp.SnmpPDU{{
			Name:  oids[0],
			Type:  gosnmp.OctetString,
			Value: []byte("edgeCore Gateway"),
		}}, nil
	}
	transport.connected.Store(true)
	transport.client = &gosnmp.GoSNMP{}

	scheduler := NewSNMPScheduler(transport, NewSNMPDecoder(), map[string]any{
		"community": "public",
	})
	results, err := scheduler.ReadPoints(context.Background(), []model.Point{
		{ID: "p1", Name: "sysDescr", Address: "public|1.3.6.1.2.1.1.1.0", DataType: "STRING"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "Good", results["p1"].Quality)
	assert.Equal(t, "edgeCore Gateway", results["p1"].Value)

	total, success, failure := scheduler.GetStats()
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(1), success)
	assert.Equal(t, int64(0), failure)
}

func TestSchedulerWritePointMock(t *testing.T) {
	var writtenOID string
	var writtenValue any

	transport := NewSNMPTransport(map[string]any{
		"ip":        "127.0.0.1",
		"port":      161,
		"community": "public",
	})
	transport.setHook = func(oid string, value interface{}, asnType gosnmp.Asn1BER, community string) error {
		writtenOID = oid
		writtenValue = value
		require.Equal(t, "private", community)
		require.Equal(t, gosnmp.Integer, asnType)
		return nil
	}
	transport.connected.Store(true)
	transport.client = &gosnmp.GoSNMP{}

	scheduler := NewSNMPScheduler(transport, NewSNMPDecoder(), map[string]any{
		"community": "public",
	})
	err := scheduler.WritePoint(context.Background(), model.Point{
		Address:  "private|1.3.6.1.4.1.99999.1.0",
		DataType: "INT32",
	}, int(42))
	require.NoError(t, err)
	assert.Equal(t, "1.3.6.1.4.1.99999.1.0", writtenOID)
	assert.Equal(t, 42, writtenValue)
}

package omron

import (
	"strings"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
	finslib "github.com/anviod/fins"
)

func toFinsPoints(points []model.Point) []finslib.Point {
	out := make([]finslib.Point, len(points))
	for i, p := range points {
		out[i] = finslib.Point{
			ID:       p.ID,
			Address:  p.Address,
			DataType: toFinsDataType(p.DataType),
		}
	}
	return out
}

func toFinsPoint(p model.Point) finslib.Point {
	return finslib.Point{
		ID:       p.ID,
		Address:  p.Address,
		DataType: toFinsDataType(p.DataType),
	}
}

func toFinsDataType(dt string) finslib.DataType {
	switch strings.ToUpper(strings.TrimSpace(dt)) {
	case "BOOL", "BIT":
		return finslib.DataTypeBIT
	case "INT8":
		return finslib.DataTypeINT8
	case "UINT8":
		return finslib.DataTypeUINT8
	case "INT16":
		return finslib.DataTypeINT16
	case "UINT16":
		return finslib.DataTypeUINT16
	case "INT32":
		return finslib.DataTypeINT32
	case "UINT32":
		return finslib.DataTypeUINT32
	case "INT64":
		return finslib.DataTypeINT64
	case "UINT64":
		return finslib.DataTypeUINT64
	case "FLOAT", "FLOAT32":
		return finslib.DataTypeFLOAT
	case "DOUBLE", "FLOAT64":
		return finslib.DataTypeDOUBLE
	case "STRING":
		return finslib.DataTypeSTRING
	default:
		return finslib.DataType(dt)
	}
}

func fromFinsValues(values map[string]finslib.Value) map[string]model.Value {
	out := make(map[string]model.Value, len(values))
	for id, v := range values {
		out[id] = model.Value{
			PointID: id,
			Value:   v.Value,
			Quality: string(v.Quality),
			TS:      v.TS,
		}
	}
	return out
}

func toDriverHealth(status finslib.HealthStatus) driver.HealthStatus {
	switch status {
	case finslib.HealthStatusUp:
		return driver.HealthStatusGood
	case finslib.HealthStatusDown:
		return driver.HealthStatusBad
	default:
		return driver.HealthStatusUnknown
	}
}

func connectionMetricsTuple(metrics finslib.ConnectionMetrics) (connectionSeconds int64, reconnectCount int64, localAddr string, remoteAddr string, lastDisconnectTime time.Time) {
	reconnectCount = int64(metrics.ReconnectCount)
	localAddr = metrics.LocalAddr
	remoteAddr = metrics.RemoteAddr
	lastDisconnectTime = metrics.LastDisconnectTime
	if metrics.Connected && !metrics.ConnectTime.IsZero() {
		connectionSeconds = int64(time.Since(metrics.ConnectTime).Seconds())
	}
	return
}

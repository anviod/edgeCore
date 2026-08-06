package core

import (
	"context"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

type mockStressDriver struct{}

func (d *mockStressDriver) Init(cfg model.DriverConfig) error           { return nil }
func (d *mockStressDriver) Connect(ctx context.Context) error           { return nil }
func (d *mockStressDriver) Disconnect() error                           { return nil }
func (d *mockStressDriver) Health() driver.HealthStatus                 { return driver.HealthStatusGood }
func (d *mockStressDriver) SetSlaveID(slaveID uint8) error              { return nil }
func (d *mockStressDriver) SetDeviceConfig(config map[string]any) error { return nil }
func (d *mockStressDriver) WritePoint(ctx context.Context, point model.Point, value any) error {
	return nil
}
func (d *mockStressDriver) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	return 0, 0, "", "", time.Time{}
}
func (d *mockStressDriver) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	results := make(map[string]model.Value, len(points))
	for _, p := range points {
		results[p.ID] = model.Value{PointID: p.ID, Value: 1.0, Quality: "Good", TS: time.Now()}
	}
	return results, nil
}

package fault

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

var ErrInjectedDrop = errors.New("injected request drop")

// Injector wraps a driver.Driver and applies configurable faults for tests.
type Injector struct {
	mu sync.Mutex

	Underlying driver.Driver

	Latency             time.Duration
	DropNextN           int
	CorruptNextResponse bool
	HalfOpenDuration    time.Duration
	RotateFaultModes    bool
	halfOpenUntil       time.Time
	rotateSeq           int
}

func Wrap(d driver.Driver) *Injector {
	return &Injector{Underlying: d}
}

func (f *Injector) Init(cfg model.DriverConfig) error {
	if f.Underlying == nil {
		return nil
	}
	return f.Underlying.Init(cfg)
}

func (f *Injector) Connect(ctx context.Context) error {
	if f.Underlying == nil {
		return nil
	}
	return f.Underlying.Connect(ctx)
}

func (f *Injector) Disconnect() error {
	if f.Underlying == nil {
		return nil
	}
	return f.Underlying.Disconnect()
}

func (f *Injector) Health() driver.HealthStatus {
	if f.Underlying == nil {
		return driver.HealthStatusUnknown
	}
	return f.Underlying.Health()
}

func (f *Injector) SetSlaveID(slaveID uint8) error {
	if f.Underlying == nil {
		return nil
	}
	return f.Underlying.SetSlaveID(slaveID)
}

func (f *Injector) SetDeviceConfig(config map[string]any) error {
	if f.Underlying == nil {
		return nil
	}
	return f.Underlying.SetDeviceConfig(config)
}

func (f *Injector) WritePoint(ctx context.Context, point model.Point, value any) error {
	if f.Underlying == nil {
		return nil
	}
	return f.Underlying.WritePoint(ctx, point, value)
}

func (f *Injector) GetConnectionMetrics() (int64, int64, string, string, time.Time) {
	if f.Underlying == nil {
		return 0, 0, "", "", time.Time{}
	}
	return f.Underlying.GetConnectionMetrics()
}

func (f *Injector) ReadPoints(ctx context.Context, points []model.Point) (map[string]model.Value, error) {
	f.mu.Lock()
	if f.RotateFaultModes {
		switch f.rotateSeq % 3 {
		case 0:
			f.Latency = 120 * time.Millisecond
			f.DropNextN = 0
			f.CorruptNextResponse = false
		case 1:
			f.Latency = 0
			f.DropNextN = 1
			f.CorruptNextResponse = false
		default:
			f.Latency = 0
			f.DropNextN = 0
			f.CorruptNextResponse = true
		}
		f.rotateSeq++
	}
	if !f.halfOpenUntil.IsZero() && time.Now().Before(f.halfOpenUntil) {
		f.mu.Unlock()
		if f.Underlying == nil {
			return map[string]model.Value{}, nil
		}
		return f.Underlying.ReadPoints(ctx, points)
	}
	if f.DropNextN > 0 {
		f.DropNextN--
		if f.DropNextN == 0 && f.HalfOpenDuration > 0 {
			f.halfOpenUntil = time.Now().Add(f.HalfOpenDuration)
		}
		f.mu.Unlock()
		return nil, ErrInjectedDrop
	}
	latency := f.Latency
	corrupt := f.CorruptNextResponse
	if corrupt {
		f.CorruptNextResponse = false
	}
	f.mu.Unlock()

	if latency > 0 {
		timer := time.NewTimer(latency)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}

	if f.Underlying == nil {
		return map[string]model.Value{}, nil
	}

	values, err := f.Underlying.ReadPoints(ctx, points)
	if err != nil {
		return values, err
	}
	if !corrupt || values == nil {
		return values, err
	}

	corrupted := make(map[string]model.Value, len(values))
	now := time.Now()
	for id, v := range values {
		v.Quality = "Bad"
		v.Value = nil
		v.TS = now
		corrupted[id] = v
	}
	return corrupted, errors.New("injected corrupt response")
}

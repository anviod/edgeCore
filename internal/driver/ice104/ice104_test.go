package ice104

import (
	"testing"

	"github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

func TestICE104DriverRegistration(t *testing.T) {
	d, ok := driver.GetDriver("iec60870-5-104")
	if !ok {
		t.Fatal("iec60870-5-104 driver not registered")
	}
	if d == nil {
		t.Fatal("nil driver factory result")
	}
}

func TestICE104DriverInitHealth(t *testing.T) {
	d := NewICE104Driver()
	if err := d.Init(modelDriverConfig()); err != nil {
		t.Fatal(err)
	}
	if d.Health() != driver.HealthStatusBad {
		t.Fatalf("expected bad health before connect")
	}
}

func modelDriverConfig() model.DriverConfig { //nolint:unparam
	return model.DriverConfig{
		Protocol: "iec60870-5-104",
		Config: map[string]any{
			"ip":            "127.0.0.1",
			"port":          2404,
			"commonAddress": 1,
		},
	}
}

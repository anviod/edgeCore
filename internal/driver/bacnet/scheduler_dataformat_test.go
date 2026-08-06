package bacnet

import (
	"fmt"
	"testing"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/edgeCore/internal/model"
)

func TestPointSchedulerDecodeResponseWithDataformat(t *testing.T) {
	s := &PointScheduler{
		useDataformat: true,
		pointStates:   make(map[string]*PointRuntime),
	}

	objType := btypes.AnalogValue
	instance := uint32(1)
	propType := btypes.PropPresentValue

	mpd := btypes.MultiplePropertyData{
		Objects: []btypes.Object{
			{
				ID: btypes.ObjectID{
					Type:     objType,
					Instance: btypes.ObjectInstance(instance),
				},
				Properties: []btypes.Property{
					{
						Type: propType,
						Data: uint16(5),
					},
				},
			},
		},
	}

	point := model.Point{
		ID:          "p1",
		DeviceID:    "dev1",
		Name:        "test",
		Address:     "2:1:85",
		DataType:    "uint16",
		ReadFormula: "v*10",
	}

	key := fmt.Sprintf("%d:%d:%d", objType, instance, propType)
	pointMap := map[string]model.Point{
		key: point,
	}

	result := make(map[string]model.Value)

	s.decodeResponse(mpd, pointMap, result)

	v, ok := result["p1"]
	if !ok {
		t.Fatalf("expected value for point p1")
	}

	if v.Quality != "Good" {
		t.Fatalf("expected Quality Good, got %s", v.Quality)
	}

	got, ok := v.Value.(uint16)
	if !ok {
		t.Fatalf("expected uint16 value, got %T", v.Value)
	}

	if got != 50 {
		t.Fatalf("expected formatted value 50, got %d", got)
	}

	if v.PointID != point.ID || v.DeviceID != point.DeviceID {
		t.Fatalf("unexpected point or device id in result")
	}

	if v.TS.IsZero() || v.TS.After(time.Now().Add(1*time.Second)) {
		t.Fatalf("unexpected timestamp in result")
	}
}

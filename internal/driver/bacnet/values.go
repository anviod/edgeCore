package bacnet

import (
	"fmt"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/null"
	"github.com/anviod/edgeCore/internal/model"
)

func applyFreshReadToCache(devCtx *DeviceContext, deviceID string, fresh map[string]model.Value) {
	if devCtx == nil || len(fresh) == 0 {
		return
	}
	devCtx.CacheMu.Lock()
	if devCtx.LastValues == nil {
		devCtx.LastValues = make(map[string]model.Value)
	}
	now := time.Now()
	for k, v := range fresh {
		v.DeviceID = deviceID
		v.CachedAt = now
		devCtx.LastValues[k] = v
	}
	devCtx.CacheMu.Unlock()
}

// normalizePresentValue converts BACnet decoded values into JSON-safe scalars
// suitable for ShadowCore and history storage.
// Special handling:
//   - BACnet Priority Array returns as []float32 (16 entries)
//     Extract the highest priority (lowest index) non-null/non-zero value
func normalizePresentValue(v any) any {
	if v == nil {
		return nil
	}

	// Special handling for BACnet Priority Array ([]float32)
	switch arr := v.(type) {
	case []float32:
		if len(arr) == 0 {
			return nil
		}
		// Priority Array in BACnet has 16 entries from highest (index 0, priority 1)
		// to lowest (index 15, priority 16). Return first non-zero value.
		for i := 0; i < len(arr); i++ {
			if arr[i] != 0 {
				return arr[i]
			}
		}
		return arr[len(arr)-1] // fall back to last entry
	case []float64:
		if len(arr) == 0 {
			return nil
		}
		for i := 0; i < len(arr); i++ {
			if arr[i] != 0 {
				return arr[i]
			}
		}
		return arr[len(arr)-1]
	case []interface{}:
		if len(arr) == 0 {
			return nil
		}
		// For mixed-type priority arrays, find first non-zero numeric value
		for i := 0; i < len(arr); i++ {
			switch val := arr[i].(type) {
			case float32:
				if val != 0 {
					return val
				}
			case float64:
				if val != 0 {
					return val
				}
			case int:
				if val != 0 {
					return val
				}
			}
		}
		// Return last entry if all zero
		return arr[len(arr)-1]
	}

	switch t := v.(type) {
	case null.Null:
		return nil
	case btypes.Enumerated:
		return uint32(t)
	case btypes.BitString:
		return t.String()
	case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, bool, string:
		return t
	default:
		return fmt.Sprintf("%v", v)
	}
}

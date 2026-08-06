package core

import (
	"errors"
	"strings"

	drv "github.com/anviod/edgeCore/internal/driver"
	"github.com/anviod/edgeCore/internal/model"
)

func isChannelLinkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrConnectionUnavailable) {
		return true
	}
	msg := strings.ToLower(err.Error())
	linkPatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"connection closed",
		"dial tcp",
		"network unreachable",
		"no route to host",
		"connection unavailable",
		"entering cooldown",
		"tls handshake",
		"cannot assign requested address",
	}
	for _, p := range linkPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func isChannelLinkUp(d drv.Driver) bool {
	if d == nil {
		return false
	}
	// Only explicit Bad means the transport link is dead. Unknown covers
	// reconnecting/degraded states and must not take the whole channel offline.
	return d.Health() != drv.HealthStatusBad
}

func isCollectQualityGood(v model.Value) bool {
	return strings.EqualFold(resolveCollectQuality(v), "Good")
}

// collectContextFromExecuteResult builds device communication state from a driver
// read outcome. Transport success is necessary but not sufficient: Good points count
// as success and Bad/Uncertain (or missing) points count as failure. Modbus drivers
// often return err == nil with all Bad points after a slave timeout.
func collectContextFromExecuteResult(result *ExecuteResult, pointCount int) *CollectContext {
	ctx := &CollectContext{}
	if result == nil || !result.Success {
		if pointCount > 0 {
			ctx.FailCmd = pointCount
		} else {
			ctx.FailCmd = 1
		}
		return ctx
	}

	if len(result.Values) == 0 {
		if pointCount > 0 {
			ctx.FailCmd = pointCount
		} else {
			ctx.FailCmd = 1
		}
		return ctx
	}

	for _, v := range result.Values {
		if isCollectQualityGood(v) {
			ctx.SuccessCmd++
		} else {
			ctx.FailCmd++
		}
	}
	if pointCount > len(result.Values) {
		ctx.FailCmd += pointCount - len(result.Values)
	}
	return ctx
}

func collectSucceededFromResult(result *ExecuteResult, pointCount int) bool {
	if result == nil || !result.Success {
		return false
	}
	ctx := collectContextFromExecuteResult(result, pointCount)
	total := ctx.SuccessCmd + ctx.FailCmd
	if total == 0 {
		return false
	}
	const minSuccessRatio = 0.3
	return float64(ctx.SuccessCmd)/float64(total) >= minSuccessRatio
}

func resolveEffectiveDeviceState(ch *model.Channel, d drv.Driver, dev *model.Device, rawState int) int {
	if ch != nil && !ch.Enable {
		return int(NodeStateOffline)
	}
	if dev != nil && !dev.Enable {
		return int(NodeStateOffline)
	}
	if !isChannelLinkUp(d) {
		return int(NodeStateOffline)
	}
	return rawState
}

func (cm *ChannelManager) channelIDForDevice(deviceID string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for id, ch := range cm.channels {
		for _, dev := range ch.Devices {
			if dev.ID == deviceID {
				return id
			}
		}
	}
	return ""
}

func (cm *ChannelManager) markChannelDevicesOffline(channelID string) {
	cm.mu.RLock()
	ch, ok := cm.channels[channelID]
	if !ok {
		cm.mu.RUnlock()
		return
	}
	deviceIDs := make([]string, 0, len(ch.Devices))
	for _, dev := range ch.Devices {
		if dev.Enable {
			deviceIDs = append(deviceIDs, dev.ID)
		}
	}
	cm.mu.RUnlock()

	se := cm.scanEngineAdapter.scanEngine
	for _, id := range deviceIDs {
		cm.stateManager.MarkOffline(id)
		if se != nil {
			se.markDeviceShadowBad(id, channelID)
		}
	}
}

// resolveDeviceQualityScore returns the device quality score for API responses.
// The global metrics collector only has HealthScore after UpdateDeviceMetrics runs;
// when no collect metrics exist yet, derive score from the effective device state
// (aligned with BACnet driver quality scoring).
func resolveDeviceQualityScore(dev *model.Device, metrics *model.DeviceMetrics) int {
	if metrics != nil && !metrics.LastCollectTime.IsZero() {
		return metrics.HealthScore
	}
	if dev == nil {
		return 0
	}
	switch dev.State {
	case int(NodeStateOnline):
		return 100
	case int(NodeStateUnstable):
		return 60
	case int(NodeStateQuarantine):
		return 20
	default:
		return 0
	}
}

func (cm *ChannelManager) applyDeviceRuntimeState(ch *model.Channel, d drv.Driver, dev *model.Device) {
	rawState := int(NodeStateOnline)
	if node := cm.stateManager.GetNode(dev.ID); node != nil {
		rawState = int(node.Runtime.State)
		dev.NodeRuntime = &model.NodeRuntime{
			FailCount:     node.Runtime.FailCount,
			SuccessCount:  node.Runtime.SuccessCount,
			LastFailTime:  node.Runtime.LastFailTime,
			NextRetryTime: node.Runtime.NextRetryTime,
			State:         rawState,
		}
	}

	effective := resolveEffectiveDeviceState(ch, d, dev, rawState)
	dev.State = effective
	if dev.NodeRuntime != nil {
		dev.NodeRuntime.State = effective
	}
}

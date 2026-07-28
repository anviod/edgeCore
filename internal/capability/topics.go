package capability

import "fmt"

// EAN 2.0 Topic / Subject templates ($edgeos/*).
const (
	TopicDiscoveryAgent      = "$edgeos/discovery/agent"
	TopicDiscoveryAgentOffline = "$edgeos/discovery/agent/offline"
	TopicDiscoveryCapability = "$edgeos/discovery/capability"
	TopicDiscoveryService    = "$edgeos/discovery/service"
	TopicDiscoveryQuery      = "$edgeos/discovery/query"
	TopicDiscoveryResponse   = "$edgeos/discovery/response"
	TopicEventBroadcast      = "$edgeos/event/broadcast"
	TopicEventSubscribe      = "$edgeos/event/subscribe"
)

// TopicInvoke returns $edgeos/invoke/{agentID}.
func TopicInvoke(agentID string) string {
	return fmt.Sprintf("$edgeos/invoke/%s", agentID)
}

// TopicInvokeStatus returns $edgeos/invoke/{agentID}/status.
func TopicInvokeStatus(agentID string) string {
	return fmt.Sprintf("$edgeos/invoke/%s/status", agentID)
}

// TopicReply returns $edgeos/reply/{agentID}.
func TopicReply(agentID string) string {
	return fmt.Sprintf("$edgeos/reply/%s", agentID)
}

// TopicEvent returns $edgeos/event/{agentID}.
func TopicEvent(agentID string) string {
	return fmt.Sprintf("$edgeos/event/%s", agentID)
}

// TopicEventDevice returns $edgeos/event/{agentID}/{deviceID}.
func TopicEventDevice(agentID, deviceID string) string {
	return fmt.Sprintf("$edgeos/event/%s/%s", agentID, deviceID)
}

// TopicState returns $edgeos/state/{agentID}.
func TopicState(agentID string) string {
	return fmt.Sprintf("$edgeos/state/%s", agentID)
}

// TopicStateDelta returns $edgeos/state/{agentID}/delta.
func TopicStateDelta(agentID string) string {
	return fmt.Sprintf("$edgeos/state/%s/delta", agentID)
}

// TopicStateGet returns $edgeos/state/{agentID}/get.
func TopicStateGet(agentID string) string {
	return fmt.Sprintf("$edgeos/state/%s/get", agentID)
}

// TopicHeartbeat returns $edgeos/heartbeat/{agentID}.
func TopicHeartbeat(agentID string) string {
	return fmt.Sprintf("$edgeos/heartbeat/%s", agentID)
}

// V1Compat holds V1.0 edgex/* topic helpers retained as a compatibility layer.
// New development should prefer EAN 2.0 $edgeos/* topics above.
type V1Compat struct{}

// V1 is the V1.0 compatibility topic namespace.
var V1 = V1Compat{}

func (V1Compat) NodesRegister() string   { return "edgex/nodes/register" }
func (V1Compat) NodesUnregister() string { return "edgex/nodes/unregister" }

func (V1Compat) NodeStatus(nodeID string) string {
	return fmt.Sprintf("edgex/nodes/%s/status", nodeID)
}
func (V1Compat) NodeOnline(nodeID string) string {
	return fmt.Sprintf("edgex/nodes/%s/online", nodeID)
}
func (V1Compat) NodeOffline(nodeID string) string {
	return fmt.Sprintf("edgex/nodes/%s/offline", nodeID)
}
func (V1Compat) Heartbeat(nodeID string) string {
	return fmt.Sprintf("edgex/heartbeat/%s", nodeID)
}

func (V1Compat) DevicesReport() string { return "edgex/devices/report" }
func (V1Compat) DevicesList(nodeID string) string {
	return fmt.Sprintf("edgex/devices/%s/list", nodeID)
}
func (V1Compat) DeviceInfo(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/devices/%s/%s/info", nodeID, deviceID)
}
func (V1Compat) DeviceOnline(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/devices/%s/%s/online", nodeID, deviceID)
}
func (V1Compat) DeviceOffline(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/devices/%s/%s/offline", nodeID, deviceID)
}

func (V1Compat) PointsReport() string { return "edgex/points/report" }
func (V1Compat) PointsDevice(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/points/%s/%s", nodeID, deviceID)
}

func (V1Compat) DataDevice(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/data/%s/%s", nodeID, deviceID)
}
func (V1Compat) DataPoint(nodeID, deviceID, pointID string) string {
	return fmt.Sprintf("edgex/data/%s/%s/%s", nodeID, deviceID, pointID)
}

func (V1Compat) CmdNodesRegister() string { return "edgex/cmd/nodes/register" }
func (V1Compat) CmdDiscover(nodeID string) string {
	return fmt.Sprintf("edgex/cmd/%s/discover", nodeID)
}
func (V1Compat) CmdWrite(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/cmd/%s/%s/write", nodeID, deviceID)
}
func (V1Compat) CmdResponse(nodeID, deviceID string) string {
	return fmt.Sprintf("edgex/cmd/responses/%s/%s", nodeID, deviceID)
}

func (V1Compat) EventsAlert() string { return "edgex/events/alert" }
func (V1Compat) EventsError() string { return "edgex/events/error" }
func (V1Compat) EventsInfo() string  { return "edgex/events/info" }

// QoS defaults for EAN 2.0 topics.
const (
	QoSDiscovery = byte(1)
	QoSInvoke    = byte(1)
	QoSEvent     = byte(1)
	QoSHeartbeat = byte(0)
	QoSQuery     = byte(0)
)

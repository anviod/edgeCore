package capability

import "fmt"

// EAN 2.0 Topic / Subject templates ($edgeos/*).
// These use MQTT-style slash separators. The NATS bus adapter
// (edgos_nats.MqttTopicToNatsSubject) converts them to NATS-native
// dot-separated subjects before every Publish / Subscribe call.
// | EAN 2.0 Topic 模板使用 MQTT 斜杠形式。
// | NATS 适配器会自动将其转换为点分隔的 NATS Subject。
const (
	TopicDiscoveryAgent        = "$edgeos/discovery/agent"
	TopicDiscoveryAgentOffline = "$edgeos/discovery/agent/offline"
	TopicDiscoveryCapability   = "$edgeos/discovery/capability"
	TopicDiscoveryService      = "$edgeos/discovery/service"
	TopicDiscoveryQuery        = "$edgeos/discovery/query"
	TopicDiscoveryResponse     = "$edgeos/discovery/response"
	TopicEventBroadcast        = "$edgeos/event/broadcast"
	TopicEventSubscribe        = "$edgeos/event/subscribe"
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

// V1Compat holds V1.0 edgeCore/* topic helpers retained as a compatibility layer.
// New development should prefer EAN 2.0 $edgeos/* topics above.
type V1Compat struct{}

// V1 is the V1.0 compatibility topic namespace.
var V1 = V1Compat{}

func (V1Compat) NodesRegister() string   { return "edgeCore/nodes/register" }
func (V1Compat) NodesUnregister() string { return "edgeCore/nodes/unregister" }

func (V1Compat) NodeStatus(nodeID string) string {
	return fmt.Sprintf("edgeCore/nodes/%s/status", nodeID)
}
func (V1Compat) NodeOnline(nodeID string) string {
	return fmt.Sprintf("edgeCore/nodes/%s/online", nodeID)
}
func (V1Compat) NodeOffline(nodeID string) string {
	return fmt.Sprintf("edgeCore/nodes/%s/offline", nodeID)
}
func (V1Compat) Heartbeat(nodeID string) string {
	return fmt.Sprintf("edgeCore/heartbeat/%s", nodeID)
}

func (V1Compat) DevicesReport() string { return "edgeCore/devices/report" }
func (V1Compat) DevicesList(nodeID string) string {
	return fmt.Sprintf("edgeCore/devices/%s/list", nodeID)
}
func (V1Compat) DeviceInfo(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/devices/%s/%s/info", nodeID, deviceID)
}
func (V1Compat) DeviceOnline(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/devices/%s/%s/online", nodeID, deviceID)
}
func (V1Compat) DeviceOffline(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/devices/%s/%s/offline", nodeID, deviceID)
}

func (V1Compat) PointsReport() string { return "edgeCore/points/report" }
func (V1Compat) PointsDevice(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/points/%s/%s", nodeID, deviceID)
}

func (V1Compat) DataDevice(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/data/%s/%s", nodeID, deviceID)
}
func (V1Compat) DataPoint(nodeID, deviceID, pointID string) string {
	return fmt.Sprintf("edgeCore/data/%s/%s/%s", nodeID, deviceID, pointID)
}

func (V1Compat) CmdNodesRegister() string { return "edgeCore/cmd/nodes/register" }
func (V1Compat) CmdDiscover(nodeID string) string {
	return fmt.Sprintf("edgeCore/cmd/%s/discover", nodeID)
}
func (V1Compat) CmdWrite(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/cmd/%s/%s/write", nodeID, deviceID)
}
func (V1Compat) CmdResponse(nodeID, deviceID string) string {
	return fmt.Sprintf("edgeCore/cmd/responses/%s/%s", nodeID, deviceID)
}

func (V1Compat) EventsAlert() string { return "edgeCore/events/alert" }
func (V1Compat) EventsError() string { return "edgeCore/events/error" }
func (V1Compat) EventsInfo() string  { return "edgeCore/events/info" }

// QoS defaults for EAN 2.0 topics.
const (
	QoSDiscovery = byte(1)
	QoSInvoke    = byte(1)
	QoSEvent     = byte(1)
	QoSHeartbeat = byte(0)
	QoSQuery     = byte(0)
)

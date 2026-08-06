package server

import (
	"strings"

	syncpkg "github.com/anviod/edgeCore/internal/sync"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func (s *Server) resolveNodeSnapshot(nodeID string) (*syncpkg.NodeSnapshot, error) {
	if s.cfgManager == nil {
		return nil, fiber.ErrServiceUnavailable
	}

	cfg := s.cfgManager.GetConfig()
	localID := ""
	if s.syncManager != nil {
		localID = s.syncManager.GetPeerIDString()
	}

	if nodeID == "" || nodeID == localID || nodeID == "local" {
		return syncpkg.BuildNodeSnapshot(localID, cfg), nil
	}

	if s.syncManager != nil {
		if snapshot, ok := s.syncManager.GetSnapshot(nodeID); ok {
			return snapshot, nil
		}
	}

	fallback := syncpkg.BuildNodeSnapshot(nodeID, cfg)
	if fallback.NodeName == "" {
		fallback.NodeName = nodeID
	}
	return fallback, nil
}

func (s *Server) getSyncNodeTree(c *fiber.Ctx) error {
	nodeID := c.Params("id")
	snapshot, err := s.resolveNodeSnapshot(nodeID)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(snapshot)
}

func (s *Server) getSyncNodeDevices(c *fiber.Ctx) error {
	nodeID := c.Params("id")
	channelID := c.Query("channel_id")
	snapshot, err := s.resolveNodeSnapshot(nodeID)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"error": err.Error()})
	}

	if channelID == "" {
		return c.JSON(fiber.Map{
			"node_id":  snapshot.NodeID,
			"channels": snapshot.Channels,
		})
	}

	for _, ch := range snapshot.Channels {
		if ch.ID == channelID {
			return c.JSON(fiber.Map{
				"node_id": nodeID,
				"channel": ch,
				"devices": ch.Devices,
			})
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "channel not found"})
}

func (s *Server) getSyncNodePoints(c *fiber.Ctx) error {
	nodeID := c.Params("id")
	channelID := c.Query("channel_id")
	deviceID := c.Params("deviceId")
	snapshot, err := s.resolveNodeSnapshot(nodeID)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"error": err.Error()})
	}

	for _, ch := range snapshot.Channels {
		if ch.ID != channelID {
			continue
		}
		for _, dev := range ch.Devices {
			if dev.ID == deviceID {
				return c.JSON(fiber.Map{
					"node_id":     snapshot.NodeID,
					"channel":     ch,
					"device":      dev,
					"points":      dev.Points,
					"point_count": len(dev.Points),
				})
			}
		}
	}

	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "device not found"})
}

func (s *Server) getSyncNodeDiff(c *fiber.Ctx) error {
	sourceID := c.Params("id")
	targetID := c.Query("target_node_id")
	if targetID == "" && s.syncManager != nil {
		targetID = s.syncManager.GetPeerIDString()
	}

	sourceSnapshot, err := s.resolveNodeSnapshot(sourceID)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"error": err.Error()})
	}
	targetSnapshot, err := s.resolveNodeSnapshot(targetID)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(fiber.Map{"error": err.Error()})
	}

	diff := syncpkg.CompareSnapshots(sourceSnapshot, targetSnapshot)
	return c.JSON(diff)
}

func (s *Server) startDeviceTakeover(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}

	nodeID := c.Params("id")
	var req struct {
		DeviceKey  string `json:"device_key"`
		TargetPeer string `json:"target_peer"`
	}
	if err := c.BodyParser(&req); err != nil && len(c.Body()) > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if req.DeviceKey == "" {
		req.DeviceKey = c.Query("device_key")
	}
	if req.TargetPeer == "" {
		req.TargetPeer = c.Query("target_peer")
	}
	if req.TargetPeer == "" {
		req.TargetPeer = nodeID
	}
	if req.DeviceKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "device_key is required"})
	}

	if err := s.syncManager.StartDeviceTakeover(req.DeviceKey, req.TargetPeer); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"message":     "takeover initiated",
		"device_key":  req.DeviceKey,
		"target_peer": req.TargetPeer,
		"flow":        []string{"HELLO", "TAKEOVER", "FULL_CONFIG"},
	})
}

func (s *Server) getTakeoverEvents(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "Sync manager not initialized"})
	}
	deviceKey := c.Query("device_key")
	return c.JSON(fiber.Map{
		"device_key": deviceKey,
		"events":     s.syncManager.GetTakeoverEvents(deviceKey),
	})
}

// helper used by UI detail panels
func snapshotDeviceToDetail(device syncpkg.TreeDevice) fiber.Map {
	return fiber.Map{
		"type":        device.Type,
		"id":          device.ID,
		"name":        device.Name,
		"label":       device.Label,
		"status":      device.Status,
		"enabled":     device.Enabled,
		"point_count": device.PointCount,
		"source_file": device.SourceFile,
		"config":      device.Config,
	}
}

func snapshotPointToDetail(point syncpkg.TreePoint) fiber.Map {
	return fiber.Map{
		"type":        point.Type,
		"id":          point.ID,
		"name":        point.Name,
		"label":       point.Label,
		"status":      point.Status,
		"source_file": point.SourceFile,
		"config":      point.Config,
	}
}

func snapshotSectionToDetail(section syncpkg.TreeSection) fiber.Map {
	return fiber.Map{
		"type":        section.Type,
		"id":          section.ID,
		"name":        section.Name,
		"label":       section.Label,
		"status":      section.Status,
		"enabled":     section.Enabled,
		"source_file": section.SourceFile,
		"config":      section.Config,
	}
}

func treeSelectionFromSnapshot(snapshot *syncpkg.NodeSnapshot, path string) fiber.Map {
	if snapshot == nil {
		return fiber.Map{}
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return fiber.Map{}
	}

	switch parts[0] {
	case "channels":
		if len(parts) == 2 {
			for _, ch := range snapshot.Channels {
				if ch.ID == parts[1] {
					return fiber.Map{
						"type":        ch.Type,
						"id":          ch.ID,
						"name":        ch.Name,
						"label":       ch.Label,
						"status":      ch.Status,
						"enabled":     ch.Enabled,
						"source_file": ch.SourceFile,
						"config":      ch.Config,
					}
				}
			}
		}
	}
	return fiber.Map{}
}

func newUUID() string {
	return uuid.New().String()
}

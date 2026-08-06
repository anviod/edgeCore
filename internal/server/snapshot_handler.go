package server

import (
	syncpkg "github.com/anviod/edgeCore/internal/sync"
	"github.com/gofiber/fiber/v2"
)

// SnapshotResponse represents a snapshot API response
type SnapshotResponse struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	NodeID      string                `json:"node_id"`
	NodeName    string                `json:"node_name"`
	CapturedAt  string                `json:"captured_at"`
	Size        int64                 `json:"size"`
	Description string                `json:"description"`
	Tags        []string              `json:"tags"`
	Data        *syncpkg.NodeSnapshot `json:"data,omitempty"`
}

// getSnapshots returns all snapshots
func (s *Server) getSnapshots(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	nodeID := c.Params("id")
	if nodeID == "" {
		nodeID = s.syncManager.GetPeerIDString()
	}

	// Get snapshots for this node
	snapshots := s.syncManager.GetSnapshotsByNode(nodeID)

	response := make([]SnapshotResponse, 0, len(snapshots))
	for _, snap := range snapshots {
		response = append(response, SnapshotResponse{
			ID:          snap.ID,
			Name:        snap.Name,
			NodeID:      snap.NodeID,
			NodeName:    snap.NodeName,
			CapturedAt:  snap.CapturedAt.Format("2006-01-02T15:04:05Z07:00"),
			Size:        snap.Size,
			Description: snap.Description,
			Tags:        snap.Tags,
		})
	}

	return c.JSON(fiber.Map{
		"snapshots": response,
		"count":     len(response),
	})
}

// createSnapshot creates a new snapshot
func (s *Server) createSnapshot(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	nodeID := c.Params("id")
	if nodeID == "" {
		nodeID = s.syncManager.GetPeerIDString()
	}

	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if req.Name == "" {
		req.Name = "Manual Snapshot"
	}

	snapshot, err := s.syncManager.CreateSnapshot(nodeID, req.Name, req.Description, req.Tags)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message":     "snapshot created",
		"snapshot":    snapshot,
		"snapshot_id": snapshot.ID,
	})
}

// getSnapshot returns a specific snapshot
func (s *Server) getSnapshot(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	snapshotID := c.Params("snapshotId")
	snapshot, ok := s.syncManager.GetSnapshotByID(snapshotID)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "snapshot not found",
		})
	}

	return c.JSON(snapshot)
}

// deleteSnapshot deletes a snapshot
func (s *Server) deleteSnapshot(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	snapshotID := c.Params("snapshotId")
	if err := s.syncManager.DeleteSnapshot(snapshotID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":     "snapshot deleted",
		"snapshot_id": snapshotID,
	})
}

// restoreSnapshot restores a snapshot
func (s *Server) restoreSnapshot(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	snapshotID := c.Params("snapshotId")
	var req struct {
		SnapshotID string `json:"snapshot_id"`
	}

	if len(c.Body()) > 0 {
		c.BodyParser(&req)
	}
	if req.SnapshotID == "" {
		req.SnapshotID = snapshotID
	}

	if err := s.syncManager.RestoreSnapshot(req.SnapshotID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":     "snapshot restored",
		"snapshot_id": req.SnapshotID,
	})
}

// clearNodeConfig clears a node's configuration
func (s *Server) clearNodeConfig(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	nodeID := c.Params("id")
	if err := s.syncManager.ClearNodeConfig(nodeID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "node configuration cleared",
		"node_id": nodeID,
	})
}

// pullFromRemote pulls configuration from a remote node
func (s *Server) pullFromRemote(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	peerID := c.Params("id")

	snapshot, err := s.syncManager.PullFromRemote(peerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":  "configuration pulled from remote",
		"peer_id":  peerID,
		"snapshot": snapshot,
	})
}

// restoreToRemote restores configuration to a remote node
func (s *Server) restoreToRemote(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	peerID := c.Params("id")
	var req struct {
		SnapshotID string `json:"snapshot_id"`
	}

	if len(c.Body()) > 0 {
		c.BodyParser(&req)
	}

	if err := s.syncManager.RestoreToRemote(peerID, req.SnapshotID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message":     "restore initiated to remote node",
		"peer_id":     peerID,
		"snapshot_id": req.SnapshotID,
	})
}

// getSnapshotStats returns snapshot statistics
func (s *Server) getSnapshotStats(c *fiber.Ctx) error {
	if s.syncManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "Sync manager not initialized",
		})
	}

	stats := s.syncManager.GetSnapshotStats()
	return c.JSON(stats)
}

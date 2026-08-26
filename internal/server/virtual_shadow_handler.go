package server

import (
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/model"
	"github.com/gofiber/fiber/v2"
)

func (s *Server) listVirtualShadows(c *fiber.Ctx) error {
	vsm := s.virtualShadowManagerRef()
	if vsm == nil {
		return c.JSON([]model.VirtualShadowDeviceConfig{})
	}
	return c.JSON(vsm.List())
}

func (s *Server) getVirtualShadow(c *fiber.Ctx) error {
	vsm := s.virtualShadowManagerRef()
	if vsm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "virtual shadow manager not available"})
	}
	cfg, err := vsm.Get(c.Params("id"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	resp := fiber.Map{"config": cfg}
	refresh := strings.EqualFold(c.Query("refresh"), "true") || c.Query("refresh") == "1"
	var vd *model.VirtualDevice
	var points map[string]model.ShadowPoint
	var errRuntime error
	if refresh {
		vd, points, errRuntime = vsm.RefreshRuntime(cfg.ID)
	} else {
		vd, points, errRuntime = vsm.GetRuntime(cfg.ID)
	}
	if errRuntime == nil {
		resp["runtime"] = fiber.Map{
			"version":    vd.Version,
			"updated_at": vd.UpdatedAt,
			"points":     points,
		}
	}
	return c.JSON(resp)
}

func (s *Server) createVirtualShadow(c *fiber.Ctx) error {
	vsm := s.virtualShadowManagerRef()
	if vsm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "virtual shadow manager not available"})
	}
	var cfg model.VirtualShadowDeviceConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := vsm.Create(cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"status": "ok", "id": cfg.ID})
}

func (s *Server) updateVirtualShadow(c *fiber.Ctx) error {
	vsm := s.virtualShadowManagerRef()
	if vsm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "virtual shadow manager not available"})
	}
	var cfg model.VirtualShadowDeviceConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := vsm.Update(c.Params("id"), cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (s *Server) deleteVirtualShadow(c *fiber.Ctx) error {
	vsm := s.virtualShadowManagerRef()
	if vsm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "virtual shadow manager not available"})
	}
	if err := vsm.Delete(c.Params("id")); err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (s *Server) listVirtualShadowSources(c *fiber.Ctx) error {
	vsm := s.virtualShadowManagerRef()
	if vsm == nil {
		return c.JSON([]model.PointSourceRef{})
	}
	return c.JSON(vsm.ListPointSources())
}

func (s *Server) searchVirtualShadowDevices(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("q"))
	channelID := strings.TrimSpace(c.Query("channel_id"))
	if query == "" && channelID == "" {
		return c.JSON([]model.SourceDeviceSummary{})
	}
	limit := c.QueryInt("limit", 50)
	if vsm := s.virtualShadowManagerRef(); vsm != nil {
		return c.JSON(vsm.SearchSourceDevices(query, channelID, limit))
	}
	return c.JSON(searchSourceDevicesFromCM(s.cm, query, channelID, limit))
}

func searchSourceDevicesFromCM(cm *core.ChannelManager, query, channelID string, limit int) []model.SourceDeviceSummary {
	if cm == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	results := make([]model.SourceDeviceSummary, 0, limit)
	for _, ch := range cm.GetChannels() {
		if channelID != "" && ch.ID != channelID {
			continue
		}
		for _, dev := range ch.Devices {
			if query != "" {
				hay := ch.Name + " " + ch.ID + " " + dev.Name + " " + dev.ID
				if !model.MatchSearchQuery(hay, query) {
					continue
				}
			}
			state := 2
			if d := cm.GetDevice(ch.ID, dev.ID); d != nil {
				state = d.State
			}
			results = append(results, model.SourceDeviceSummary{
				Key:         ch.ID + "::" + dev.ID,
				ChannelID:   ch.ID,
				ChannelName: ch.Name,
				DeviceID:    dev.ID,
				DeviceName:  dev.Name,
				PointCount:  len(dev.Points),
				State:       state,
				Online:      state == 0 || state == 1,
			})
			if len(results) >= limit {
				return results
			}
		}
	}
	return results
}

func (s *Server) listVirtualShadowDevicePoints(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	deviceID := c.Params("deviceId")
	query := c.Query("q")
	if vsm := s.virtualShadowManagerRef(); vsm != nil {
		points, err := vsm.ListDevicePointSources(channelID, deviceID, query)
		if err == nil {
			return c.JSON(points)
		}
	}
	points, err := listDevicePointSourcesFromCM(s.cm, channelID, deviceID, query)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(points)
}

func listDevicePointSourcesFromCM(cm *core.ChannelManager, channelID, deviceID, query string) ([]model.PointSourceRef, error) {
	if cm == nil {
		return nil, fmt.Errorf("channel manager not available")
	}
	ch := cm.GetChannel(channelID)
	if ch == nil {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}
	dev := cm.GetDevice(channelID, deviceID)
	if dev == nil {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}
	sources := make([]model.PointSourceRef, 0, len(dev.Points))
	for _, pt := range dev.Points {
		hay := pt.ID + " " + pt.Name
		if query != "" && !model.MatchSearchQuery(hay, query) {
			continue
		}
		sources = append(sources, model.PointSourceRef{
			ChannelID:   ch.ID,
			ChannelName: ch.Name,
			DeviceID:    dev.ID,
			DeviceName:  dev.Name,
			PointID:     pt.ID,
			PointName:   pt.Name,
			Ref:         model.MakePointRef(ch.ID, dev.ID, pt.ID),
		})
	}
	return sources, nil
}

func (s *Server) SetVirtualShadowManager(vsm *core.VirtualShadowManager) {
	s.serverMu.Lock()
	defer s.serverMu.Unlock()
	s.vsm = vsm
}

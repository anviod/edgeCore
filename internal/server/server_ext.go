package server

import (
	"encoding/json"
	"strings"

	"github.com/anviod/edgeCore/internal/model"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func northboundUpsertErrorStatus(err error) int {
	if err == nil {
		return fiber.StatusInternalServerError
	}
	msg := err.Error()
	if strings.Contains(msg, "已存在") {
		return fiber.StatusConflict
	}
	if strings.Contains(msg, "不能为空") {
		return fiber.StatusBadRequest
	}
	return fiber.StatusInternalServerError
}

func northboundConfigJSON(cfg any, warning string) fiber.Map {
	out := fiber.Map{}
	data, err := json.Marshal(cfg)
	if err != nil {
		return fiber.Map{"error": err.Error()}
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return fiber.Map{"error": err.Error()}
	}
	if warning != "" {
		out["warning"] = warning
	}
	return out
}

// updateHTTPConfig updates HTTP configuration
func (s *Server) updateHTTPConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}

	var cfg model.HTTPConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}

	if err := s.nbm.UpsertHTTPConfig(cfg); err != nil {
		return c.Status(northboundUpsertErrorStatus(err)).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(cfg)
}

func (s *Server) deleteHTTPConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}
	id := c.Params("id")
	if err := s.nbm.DeleteHTTPConfig(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

func (s *Server) deleteMQTTConfig(c *fiber.Ctx) error {
	if s.nbm == nil {
		return c.Status(503).JSON(fiber.Map{"error": "Northbound manager not initialized"})
	}
	id := c.Params("id")
	if err := s.nbm.DeleteMQTTConfig(id); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(200)
}

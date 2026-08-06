package server

import (
	"encoding/json"
	"time"

	"github.com/anviod/edgeCore/internal/core"
	"github.com/anviod/edgeCore/internal/model"

	"github.com/gofiber/fiber/v2"
)

type generateRegistersRequest struct {
	Start        int    `json:"start"`
	End          int    `json:"end"`
	Datatype     string `json:"datatype"`
	ReadWrite    string `json:"readwrite"`
	RegisterType string `json:"register_type"`
	FunctionCode byte   `json:"function_code"`
	Mode         string `json:"mode"` // merge | replace
}

// generateDeviceRegisters 批量生成 Modbus 寄存器点位区块。
func (s *Server) generateDeviceRegisters(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	deviceID := c.Params("deviceId")
	if channelID == "" || deviceID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel id and device id are required"})
	}

	req, err := parseGenerateRegistersRequest(c.Body())
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.End < req.Start {
		req.Start, req.End = req.End, req.Start
	}
	if req.End-req.Start > 10000 {
		return c.Status(400).JSON(fiber.Map{"error": "register range too large (max 10001 points)"})
	}

	regType := model.RegHolding
	if req.RegisterType != "" {
		regType = model.ParseRegisterType(req.RegisterType)
	}
	fc := req.FunctionCode
	if fc == 0 {
		fc = regType.FunctionCode()
	}

	dev, err := s.cm.GenerateDeviceRegisterPoints(channelID, deviceID, core.ModbusRegisterGenOptions{
		Start:        req.Start,
		End:          req.End,
		DataType:     req.Datatype,
		ReadWrite:    req.ReadWrite,
		RegisterType: regType,
		FunctionCode: fc,
	}, req.Mode)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"device_id":     dev.ID,
		"points_count":  len(dev.Points),
		"register_type": regType.ShortString(),
		"function_code": fc,
	})
}

func parseGenerateRegistersRequest(body []byte) (generateRegistersRequest, error) {
	var req generateRegistersRequest
	if len(body) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return req, nil
	}

	readInt := func(keys ...string) (int, bool) {
		for _, key := range keys {
			v, ok := raw[key]
			if !ok {
				continue
			}
			var n int
			if err := json.Unmarshal(v, &n); err == nil {
				return n, true
			}
			var f float64
			if err := json.Unmarshal(v, &f); err == nil {
				return int(f), true
			}
		}
		return 0, false
	}

	if _, ok := raw["start"]; ok {
		if n, ok := readInt("start"); ok {
			req.Start = n
		}
	}
	if _, ok := raw["end"]; ok {
		if n, ok := readInt("end"); ok {
			req.End = n
		}
	}
	if req.Datatype == "" {
		if v, ok := raw["datatype"]; ok {
			_ = json.Unmarshal(v, &req.Datatype)
		}
	}
	if req.ReadWrite == "" {
		if v, ok := raw["readwrite"]; ok {
			_ = json.Unmarshal(v, &req.ReadWrite)
		}
	}
	if req.Mode == "" {
		if v, ok := raw["mode"]; ok {
			_ = json.Unmarshal(v, &req.Mode)
		}
	}
	if req.RegisterType == "" {
		if v, ok := raw["register_type"]; ok {
			_ = json.Unmarshal(v, &req.RegisterType)
		} else if v, ok := raw["registerType"]; ok {
			_ = json.Unmarshal(v, &req.RegisterType)
		}
	}
	if req.FunctionCode == 0 {
		if n, ok := readInt("function_code", "functionCode"); ok {
			req.FunctionCode = byte(n)
		}
	}

	return req, nil
}

type batchModbusSlavesRequest struct {
	SlaveStart   int    `json:"slave_start"`
	SlaveEnd     int    `json:"slave_end"`
	RegStart     int    `json:"reg_start"`
	RegEnd       int    `json:"reg_end"`
	Interval     string `json:"interval"`
	Datatype     string `json:"datatype"`
	ReadWrite    string `json:"readwrite"`
	RegisterType string `json:"register_type"`
	FunctionCode byte   `json:"function_code"`
	Enable       *bool  `json:"enable"`
}

func parseBatchModbusSlavesRequest(body []byte) (batchModbusSlavesRequest, error) {
	var req batchModbusSlavesRequest
	if len(body) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return req, nil
	}

	readInt := func(keys ...string) (int, bool) {
		for _, key := range keys {
			v, ok := raw[key]
			if !ok {
				continue
			}
			var n int
			if err := json.Unmarshal(v, &n); err == nil {
				return n, true
			}
			var f float64
			if err := json.Unmarshal(v, &f); err == nil {
				return int(f), true
			}
		}
		return 0, false
	}

	if req.SlaveStart == 0 {
		if n, ok := readInt("slave_start", "slaveStart"); ok && n > 0 {
			req.SlaveStart = n
		}
	}
	if req.SlaveEnd == 0 {
		if n, ok := readInt("slave_end", "slaveEnd"); ok && n > 0 {
			req.SlaveEnd = n
		}
	}
	if _, ok := raw["reg_start"]; ok {
		if n, ok := readInt("reg_start"); ok {
			req.RegStart = n
		}
	} else if _, ok := raw["regStart"]; ok {
		if n, ok := readInt("regStart"); ok {
			req.RegStart = n
		}
	}
	if _, ok := raw["reg_end"]; ok {
		if n, ok := readInt("reg_end"); ok {
			req.RegEnd = n
		}
	} else if _, ok := raw["regEnd"]; ok {
		if n, ok := readInt("regEnd"); ok {
			req.RegEnd = n
		}
	}

	if req.Datatype == "" {
		if v, ok := raw["datatype"]; ok {
			_ = json.Unmarshal(v, &req.Datatype)
		}
	}
	if req.ReadWrite == "" {
		if v, ok := raw["readwrite"]; ok {
			_ = json.Unmarshal(v, &req.ReadWrite)
		}
	}
	if req.Interval == "" {
		if v, ok := raw["interval"]; ok {
			_ = json.Unmarshal(v, &req.Interval)
		}
	}
	if req.RegisterType == "" {
		if v, ok := raw["register_type"]; ok {
			_ = json.Unmarshal(v, &req.RegisterType)
		} else if v, ok := raw["registerType"]; ok {
			_ = json.Unmarshal(v, &req.RegisterType)
		}
	}
	if req.FunctionCode == 0 {
		if n, ok := readInt("function_code", "functionCode"); ok {
			req.FunctionCode = byte(n)
		}
	}

	return req, nil
}

// batchCreateModbusSlaves 批量创建 Modbus 从站设备（每台含寄存器区块）。
func (s *Server) batchCreateModbusSlaves(c *fiber.Ctx) error {
	channelID := c.Params("channelId")
	if channelID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "channel id is required"})
	}

	ch := s.cm.GetChannel(channelID)
	if ch == nil {
		return c.Status(404).JSON(fiber.Map{"error": "channel not found"})
	}
	if ch.Protocol != "modbus-tcp" && ch.Protocol != "modbus-rtu" && ch.Protocol != "modbus-rtu-over-tcp" {
		return c.Status(400).JSON(fiber.Map{"error": "channel is not modbus"})
	}

	req, err := parseBatchModbusSlavesRequest(c.Body())
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.SlaveEnd < req.SlaveStart {
		req.SlaveStart, req.SlaveEnd = req.SlaveEnd, req.SlaveStart
	}
	if req.RegEnd < req.RegStart {
		req.RegStart, req.RegEnd = req.RegEnd, req.RegStart
	}
	if req.SlaveEnd-req.SlaveStart > 100 {
		return c.Status(400).JSON(fiber.Map{"error": "too many slaves (max 101)"})
	}
	if req.RegEnd-req.RegStart > 10000 {
		return c.Status(400).JSON(fiber.Map{"error": "register range too large"})
	}
	if req.SlaveStart <= 0 || req.SlaveEnd <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "slave_start and slave_end are required (1-247)"})
	}

	interval := model.Duration(time.Second)
	if req.Interval != "" {
		if d, err := time.ParseDuration(req.Interval); err == nil {
			interval = model.Duration(d)
		}
	}
	enable := true
	if req.Enable != nil {
		enable = *req.Enable
	}
	regType := model.RegHolding
	if req.RegisterType != "" {
		regType = model.ParseRegisterType(req.RegisterType)
	}
	fc := req.FunctionCode
	if fc == 0 {
		fc = regType.FunctionCode()
	}

	result, err := s.cm.BatchAddModbusSlaves(
		channelID,
		req.SlaveStart, req.SlaveEnd,
		req.RegStart, req.RegEnd,
		interval, enable,
		req.Datatype, req.ReadWrite,
		regType, fc,
	)
	if err != nil && (result == nil || len(result.Created) == 0) {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	for i := range result.Created {
		if s.dsm != nil {
			s.dsm.UpdateDeviceConfig(result.Created[i].ID, result.Created[i].Storage)
		}
	}
	if s.nbm != nil && len(result.Created) > 0 {
		s.nbm.PublishPointsMetadata()
	}

	resp := fiber.Map{
		"created": len(result.Created),
		"skipped": result.Skipped,
	}
	if len(result.Created) > 0 {
		summaries := make([]model.Device, len(result.Created))
		for i, d := range result.Created {
			summaries[i] = d.WithPointsSummary()
		}
		resp["devices"] = summaries
	}
	if len(result.Errors) > 0 {
		resp["warnings"] = result.Errors
	}
	if err != nil {
		resp["error"] = err.Error()
	}

	return c.JSON(resp)
}

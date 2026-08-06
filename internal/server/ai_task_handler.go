package server

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"encoding/json"

	"github.com/anviod/edgeCore/internal/ai_agent"
	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
	"github.com/anviod/edgeCore/internal/ai_agent/pipeline"
	"github.com/gofiber/fiber/v2"
)

func (s *Server) ensureAiAgent() *ai_agent.Agent {
	if s.aiAgent == nil {
		settings := s.loadAiCopilotSettings()
		s.aiAgent = ai_agent.NewAgent(settings.RuntimeMode())
		s.aiAgent.ApplySettings(settings)
	}
	return s.aiAgent
}

func (s *Server) getAiQuota(c *fiber.Ctx) error {
	agent := s.ensureAiAgent()
	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data":    agent.Quota().Snapshot(),
	})
}

func (s *Server) listAiTasks(c *fiber.Ctx) error {
	agent := s.ensureAiAgent()
	tasks := agent.List()
	return c.JSON(fiber.Map{
		"code":    "0",
		"message": "success",
		"data":    tasks,
	})
}

func (s *Server) createAiTask(c *fiber.Ctx) error {
	var req ai_agent.CreateRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": "请求格式无效", "data": nil,
		})
	}
	agent := s.ensureAiAgent()
	rec, err := agent.Create(req)
	if err != nil {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"code": "1", "message": err.Error(), "data": nil,
		})
	}
	return c.JSON(fiber.Map{
		"code": "0", "message": "success",
		"data": rec,
	})
}

func (s *Server) getAiTask(c *fiber.Ctx) error {
	id := c.Params("id")
	agent := s.ensureAiAgent()
	rec, ok := agent.Get(id)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code": "1", "message": "任务不存在", "data": nil,
		})
	}
	return c.JSON(fiber.Map{
		"code": "0", "message": "success", "data": rec,
	})
}

func (s *Server) confirmAiTask(c *fiber.Ctx) error {
	id := c.Params("id")
	var req ai_agent.ConfirmRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": "请求格式无效", "data": nil,
		})
	}
	if req.ApplyMode == "" {
		req.ApplyMode = "preview"
	}
	agent := s.ensureAiAgent()
	rec, err := agent.Confirm(id, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": err.Error(), "data": nil,
		})
	}
	return c.JSON(fiber.Map{
		"code": "0", "message": "success", "data": rec,
	})
}

func (s *Server) uploadAiTaskFile(c *fiber.Ctx) error {
	id := c.Params("id")
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": "缺少上传文件", "data": nil,
		})
	}

	uploadDir := filepath.Join(os.TempDir(), "edgeCore-ai-uploads", id)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code": "1", "message": "创建上传目录失败", "data": nil,
		})
	}

	safeName := filepath.Base(file.Filename)
	dest := filepath.Join(uploadDir, safeName)
	if err := c.SaveFile(file, dest); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code": "1", "message": "保存文件失败", "data": nil,
		})
	}

	agent := s.ensureAiAgent()
	if err := agent.AttachFile(id, safeName); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"code": "1", "message": err.Error(), "data": nil,
		})
	}

	return c.JSON(fiber.Map{
		"code": "0", "message": "success",
		"data": fiber.Map{
			"task_id":  id,
			"filename": safeName,
			"path":     dest,
			"size":     file.Size,
		},
	})
}

func (s *Server) postAiValidate(c *fiber.Ctx) error {
	var body struct {
		Deliverables *ai_agent.Deliverables `json:"deliverables"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": "请求格式无效", "data": nil,
		})
	}
	agent := s.ensureAiAgent()
	report := agent.ValidateDeliverables(body.Deliverables)
	return c.JSON(fiber.Map{
		"code": "0", "message": "success", "data": report,
	})
}

func (s *Server) postAiEdgeRuleDraft(c *fiber.Ctx) error {
	var body struct {
		Description string `json:"description"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": "请求格式无效", "data": nil,
		})
	}
	runner := pipeline.NewMockRunner(s.ensureAiAgent().Mode())
	draft := runner.GenerateEdgeRuleDraft(body.Description)
	return c.JSON(fiber.Map{
		"code": "0", "message": "success",
		"data": fiber.Map{
			"draft": draft,
			"mode":  s.ensureAiAgent().Mode(),
		},
	})
}

func (s *Server) getAiDiagnosticsSummary(c *fiber.Ctx) error {
	snapshot := s.buildAiContextSnapshot()
	steps := s.buildDiagnosticsSteps(snapshot)

	scanEngine := fiber.Map{}
	if s.cm != nil {
		scanEngine = s.cm.GetScanEngineMetricsSnapshot()
	}
	soak := fiber.Map{}
	if s.cm != nil {
		if snap := s.cm.GetSoakMonitorSnapshot(); snap != nil {
			soak = snap
		}
	}

	return c.JSON(fiber.Map{
		"code": "0", "message": "success",
		"data": fiber.Map{
			"snapshot":     snapshot,
			"steps":        steps,
			"scan_engine":  scanEngine,
			"soak":         soak,
			"generated_at": time.Now().Format(time.RFC3339),
			"mode":         s.ensureAiAgent().Mode(),
		},
	})
}

func (s *Server) buildDiagnosticsSteps(snapshot map[string]any) []map[string]any {
	ch, _ := snapshot["channels"].(map[string]any)
	offlineEnabled, _ := ch["offline_enabled"].(int)
	total, _ := ch["total"].(int)

	steps := []map[string]any{
		{
			"order": 1, "title": "检查系统日志", "status": "pending",
			"detail": "筛选 ERROR 级别，定位最近通讯异常与驱动报错",
			"action": fiber.Map{"type": "navigate", "path": "/logs"},
		},
		{
			"order": 2, "title": "查看 ScanEngine 指标", "status": "pending",
			"detail": "关注轮询周期、超时率、ExecutionLayer 背压",
			"action": fiber.Map{"type": "api", "path": "/api/diagnostics/scan-engine"},
		},
		{
			"order": 3, "title": "Soak 会话监控", "status": "pending",
			"detail": "对比 Release Gate 验收项与 soak 成功率",
			"action": fiber.Map{"type": "api", "path": "/api/diagnostics/soak"},
		},
		{
			"order": 4, "title": "通道与设备诊断",
			"status": diagStepStatus(total, offlineEnabled),
			"detail": "进入异常通道查看设备详情与点位采集成功率",
			"action": fiber.Map{"type": "navigate", "path": "/channels"},
		},
	}

	if offlineEnabled > 0 {
		steps = append([]map[string]any{{
			"order": 0, "title": "优先排查离线通道",
			"detail":   fmt.Sprintf("检测到 %d 个已启用通道离线：确认 IP/端口/从站参数与网络连通", offlineEnabled),
			"severity": "warning", "status": "running",
			"action": fiber.Map{"type": "navigate", "path": "/channels"},
		}}, steps...)
	} else if total > 0 {
		steps[0]["status"] = "done"
	}
	return steps
}

func diagStepStatus(total, offline int) string {
	if total == 0 {
		return "pending"
	}
	if offline > 0 {
		return "running"
	}
	return "done"
}

// createAiTaskQuick starts a task from uploaded file metadata (used by workbench).
func (s *Server) inferSkillFromFilename(name string) ai_agent.Skill {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".pcap"), strings.HasSuffix(lower, ".pcapng"), strings.HasSuffix(lower, ".hex"):
		return ai_agent.SkillProtocolReverse
	case strings.HasSuffix(lower, ".pdf"), strings.HasSuffix(lower, ".doc"), strings.HasSuffix(lower, ".docx"),
		strings.HasSuffix(lower, ".xls"), strings.HasSuffix(lower, ".xlsx"), strings.HasSuffix(lower, ".csv"):
		return ai_agent.SkillDocParse
	default:
		return ai_agent.SkillProtocolReverse
	}
}

const aiMaxUploadBytes = 50 * 1024 * 1024

var aiAllowedExtensions = map[string]bool{
	".pcap": true, ".pcapng": true, ".hex": true,
	".xlsx": true, ".xls": true, ".csv": true,
	".pdf": true, ".doc": true, ".docx": true,
}

func (s *Server) validateAiUploadFile(file *multipart.FileHeader) error {
	if file.Size <= 0 {
		return fmt.Errorf("文件为空")
	}
	if file.Size > aiMaxUploadBytes {
		return fmt.Errorf("文件过大，最大 50 MB")
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !aiAllowedExtensions[ext] {
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}
	return nil
}

func (s *Server) postAiTaskFromUpload(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": "缺少上传文件", "data": nil,
		})
	}
	if err := s.validateAiUploadFile(file); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "1", "message": err.Error(), "data": nil,
		})
	}

	skill := ai_agent.Skill(c.FormValue("skill"))
	if skill == "" {
		skill = s.inferSkillFromFilename(file.Filename)
	}
	protocolID := c.FormValue("protocol_id")
	observationsJSON := c.FormValue("observations")

	var observations []aitypes.Observation
	if observationsJSON != "" {
		_ = json.Unmarshal([]byte(observationsJSON), &observations)
	}

	safeName := filepath.Base(file.Filename)

	agent := s.ensureAiAgent()
	rec, err := agent.Create(aitypes.CreateRequest{
		Skill:        skill,
		ProtocolID:   protocolID,
		Filename:     safeName,
		Observations: observations,
	})
	if err != nil {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"code": "1", "message": err.Error(), "data": nil,
		})
	}

	uploadDir := filepath.Join(os.TempDir(), "edgeCore-ai-uploads", rec.ID)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code": "1", "message": "创建上传目录失败", "data": nil,
		})
	}
	dest := filepath.Join(uploadDir, safeName)
	if err := c.SaveFile(file, dest); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code": "1", "message": "保存文件失败", "data": nil,
		})
	}

	return c.JSON(fiber.Map{
		"code": "0", "message": "success", "data": rec,
	})
}

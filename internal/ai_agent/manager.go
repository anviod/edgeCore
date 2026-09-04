package ai_agent

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
	"github.com/anviod/edgeCore/internal/ai_agent/pipeline"
	"github.com/anviod/edgeCore/internal/ai_agent/quota"
	"github.com/anviod/edgeCore/internal/ai_agent/validate"
	"github.com/anviod/edgeCore/internal/model"
)

type Agent struct {
	mu        sync.RWMutex
	tasks     map[string]*aitypes.TaskRecord
	quota     *quota.Tracker
	validator *validate.Validator
	runner    *pipeline.MockRunner
	mode      string
}

func NewAgent(mode string) *Agent {
	if mode == "" {
		mode = "local"
	}
	return &Agent{
		tasks:     make(map[string]*aitypes.TaskRecord),
		quota:     quota.NewTracker(mode),
		validator: validate.New(),
		runner:    pipeline.NewMockRunner(mode),
		mode:      mode,
	}
}

func (a *Agent) Mode() string { return a.mode }

func (a *Agent) ApplySettings(s model.AICopilotSettings) {
	mode := s.RuntimeMode()
	a.mu.Lock()
	a.mode = mode
	a.runner = pipeline.NewMockRunner(mode)
	a.mu.Unlock()
	a.quota.SetMode(mode)
	a.quota.SetLimits(s.TokensLimit, s.TasksLimit)
}

func mapModeToDeployment(mode string) string {
	if mode == "remote" {
		return "remote"
	}
	return "local"
}

func (a *Agent) Quota() *quota.Tracker { return a.quota }

func (a *Agent) List() []*aitypes.TaskRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*aitypes.TaskRecord, 0, len(a.tasks))
	for _, t := range a.tasks {
		out = append(out, cloneTask(t))
	}
	return out
}

func (a *Agent) Get(id string) (*aitypes.TaskRecord, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	t, ok := a.tasks[id]
	if !ok {
		return nil, false
	}
	return cloneTask(t), true
}

func (a *Agent) Create(req aitypes.CreateRequest) (*aitypes.TaskRecord, error) {
	if req.Skill == "" {
		req.Skill = aitypes.SkillProtocolReverse
	}
	estTokens := 1200
	if req.Skill == aitypes.SkillDocParse {
		estTokens = 800
	}
	if req.Skill == aitypes.SkillEdgeRuleDraft {
		estTokens = 400
	}
	if a.quota.WouldExceed(estTokens) {
		return nil, fmt.Errorf("token 配额或每日任务数已达上限")
	}

	id := "task_" + time.Now().Format("20060102") + "_" + fmt.Sprintf("%08x", time.Now().UnixNano()&0xffffffff)
	now := time.Now()
	scenario := req.Scenario
	if scenario == "" {
		if req.Skill == aitypes.SkillDocParse {
			scenario = "A"
		} else if req.Skill == aitypes.SkillProtocolReverse {
			scenario = "B"
		}
	}

	rec := &aitypes.TaskRecord{
		ID: id, Skill: req.Skill, Scenario: scenario,
		Status: aitypes.StatusQueued, Mode: a.mode,
		ProtocolID: req.ProtocolID, Meta: req.Meta,
		Stages: a.runner.InitialStages(), CreatedAt: now, UpdatedAt: now,
	}
	if req.Filename != "" {
		rec.InputFiles = []string{req.Filename}
	}

	a.mu.Lock()
	a.tasks[id] = rec
	result := cloneTask(rec)
	a.mu.Unlock()

	go a.runPipeline(id, req, estTokens)
	return result, nil
}

func (a *Agent) AttachFile(taskID, filename string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found")
	}
	t.InputFiles = append(t.InputFiles, filename)
	t.UpdatedAt = time.Now()
	return nil
}

func (a *Agent) Confirm(taskID string, req aitypes.ConfirmRequest) (*aitypes.TaskRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task not found")
	}
	if t.Status != aitypes.StatusWaitingConfirm {
		return nil, fmt.Errorf("task status %s 不可确认", t.Status)
	}
	if t.Validation != nil && !t.Validation.Passed && t.Validation.PassRate < 80 {
		return nil, fmt.Errorf("Schema 校验未通过（通过率 %.0f%%），请先修正", t.Validation.PassRate)
	}

	now := time.Now()
	t.ConfirmedAt = &now
	t.Status = aitypes.StatusApplied
	t.UpdatedAt = now
	t.AppliedResult = map[string]any{
		"mode": req.ApplyMode, "channel_id": req.ChannelID, "device_id": req.DeviceID,
		"message":  "Human Confirm 完成（本地模式：未写入 config.db，对接 import API 后生效）",
		"exported": true,
	}
	return cloneTask(t), nil
}

func (a *Agent) ValidateDeliverables(d *aitypes.Deliverables) *aitypes.ValidationReport {
	return a.validator.ValidateDeliverables(d)
}

func (a *Agent) runPipeline(id string, req aitypes.CreateRequest, tokens int) {
	steps := []struct {
		idx     int
		status  aitypes.TaskStatus
		delay   time.Duration
		message string
	}{
		{0, aitypes.StatusProcessing, 400 * time.Millisecond, "gopacket 解帧完成（本地 Capture）"},
		{1, aitypes.StatusWaitingModel, 500 * time.Millisecond, "Decoder 提取 FC03/04 字段与 raw[]"},
		{2, aitypes.StatusValidating, 600 * time.Millisecond, a.mode + " 模式语义关联（Mock LLM）"},
		{3, aitypes.StatusWaitingConfirm, 300 * time.Millisecond, "四类产出 JSON 已生成"},
	}

	for _, step := range steps {
		time.Sleep(step.delay)
		a.mu.Lock()
		t := a.tasks[id]
		if t == nil {
			a.mu.Unlock()
			return
		}
		t.Status = step.status
		t.Stages = a.runner.AdvanceStage(t.Stages, step.idx, step.message)
		t.UpdatedAt = time.Now()
		a.mu.Unlock()
	}

	a.mu.Lock()
	t := a.tasks[id]
	if t == nil {
		a.mu.Unlock()
		return
	}

	switch req.Skill {
	case aitypes.SkillEdgeRuleDraft:
		draft := a.runner.GenerateEdgeRuleDraft(req.Description)
		t.Deliverables = &aitypes.Deliverables{}
		t.Meta = map[string]string{"edge_rule_draft": "generated"}
		t.AppliedResult = map[string]any{"edge_rule_draft": draft}
		tokens = 400
	case aitypes.SkillDiagnostics:
		t.Meta = map[string]string{"diagnostics": "aggregated"}
		tokens = 200
	default:
		t.Deliverables = a.runner.GenerateDeliverables(req.Skill, req.ProtocolID, req.Filename, req.Observations)
		t.ProtocolID = t.Deliverables.ProtocolModel.ProtocolID
	}

	if t.Deliverables != nil {
		t.Validation = a.validator.ValidateDeliverables(t.Deliverables)
	}
	t.TokensUsed = tokens
	t.Status = aitypes.StatusWaitingConfirm
	t.UpdatedAt = time.Now()
	a.mu.Unlock()

	a.quota.RecordTask(id, string(req.Skill), tokens)
}

func cloneTask(t *aitypes.TaskRecord) *aitypes.TaskRecord {
	if t == nil {
		return nil
	}
	data, err := json.Marshal(t)
	if err == nil {
		var cp aitypes.TaskRecord
		if json.Unmarshal(data, &cp) == nil {
			return &cp
		}
	}

	cp := *t
	if t.Stages != nil {
		cp.Stages = append([]aitypes.StageProgress(nil), t.Stages...)
	}
	if t.InputFiles != nil {
		cp.InputFiles = append([]string(nil), t.InputFiles...)
	}
	return &cp
}

package ai_agent

import (
	"testing"
	"time"

	"github.com/anviod/edgeCore/internal/ai_agent/aitypes"
	"github.com/anviod/edgeCore/internal/model"
)

func waitForTaskStatus(t *testing.T, a *Agent, id string, want aitypes.TaskStatus, timeout time.Duration) *aitypes.TaskRecord {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		rec, ok := a.Get(id)
		if ok && rec.Status == want {
			return rec
		}
		time.Sleep(50 * time.Millisecond)
	}
	rec, _ := a.Get(id)
	if rec == nil {
		t.Fatalf("task %s not found", id)
	}
	t.Fatalf("task %s status = %s, want %s", id, rec.Status, want)
	return nil
}

func TestNewAgent_DefaultMode(t *testing.T) {
	a := NewAgent("")
	if a.Mode() != "local" {
		t.Fatalf("mode = %q, want local", a.Mode())
	}
	if a.Quota() == nil {
		t.Fatal("quota tracker should be initialized")
	}
}

func TestAgent_ApplySettings(t *testing.T) {
	a := NewAgent("local")
	a.ApplySettings(model.AICopilotSettings{
		DeploymentMode: "remote",
		TokensLimit:    12000,
		TasksLimit:     5,
	})
	if a.Mode() != "remote" {
		t.Fatalf("mode = %q, want remote", a.Mode())
	}
	if a.Quota().TokensLimit() != 12000 || a.Quota().TasksLimit() != 5 {
		t.Fatalf("limits not applied: %+v", a.Quota().Snapshot())
	}
}

func TestAgent_CreateProtocolReversePipeline(t *testing.T) {
	a := NewAgent("local")
	rec, err := a.Create(aitypes.CreateRequest{
		Skill:      aitypes.SkillProtocolReverse,
		ProtocolID: "modbus-tcp",
		Filename:   "capture.pcap",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if rec.Scenario != "B" {
		t.Fatalf("scenario = %q, want B", rec.Scenario)
	}
	if len(rec.InputFiles) != 1 || rec.InputFiles[0] != "capture.pcap" {
		t.Fatalf("input files = %+v", rec.InputFiles)
	}

	done := waitForTaskStatus(t, a, rec.ID, aitypes.StatusWaitingConfirm, 3*time.Second)
	if done.Deliverables == nil {
		t.Fatal("expected deliverables after pipeline")
	}
	if done.Validation == nil {
		t.Fatal("expected validation report")
	}
	if done.TokensUsed <= 0 {
		t.Fatalf("tokens used = %d", done.TokensUsed)
	}
}

func TestAgent_CreateEdgeRuleDraftSkill(t *testing.T) {
	a := NewAgent("local")
	rec, err := a.Create(aitypes.CreateRequest{
		Skill:       aitypes.SkillEdgeRuleDraft,
		Description: "温度超过 80 触发告警",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	done := waitForTaskStatus(t, a, rec.ID, aitypes.StatusWaitingConfirm, 3*time.Second)
	if done.AppliedResult == nil {
		t.Fatal("expected edge rule draft in applied result")
	}
	if done.Meta["edge_rule_draft"] != "generated" {
		t.Fatalf("meta = %+v", done.Meta)
	}
}

func TestAgent_CreateDiagnosticsSkill(t *testing.T) {
	a := NewAgent("local")
	rec, err := a.Create(aitypes.CreateRequest{Skill: aitypes.SkillDiagnostics})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	done := waitForTaskStatus(t, a, rec.ID, aitypes.StatusWaitingConfirm, 3*time.Second)
	if done.Meta["diagnostics"] != "aggregated" {
		t.Fatalf("meta = %+v", done.Meta)
	}
}

func TestAgent_CreateQuotaExceeded(t *testing.T) {
	a := NewAgent("local")
	a.Quota().SetLimits(500, 100)
	_, err := a.Create(aitypes.CreateRequest{Skill: aitypes.SkillProtocolReverse})
	if err == nil {
		t.Fatal("expected quota error")
	}
}

func TestAgent_AttachFileAndConfirm(t *testing.T) {
	a := NewAgent("local")
	rec, err := a.Create(aitypes.CreateRequest{Skill: aitypes.SkillDocParse, Filename: "doc.pdf"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := a.AttachFile(rec.ID, "extra.pcap"); err != nil {
		t.Fatalf("AttachFile: %v", err)
	}
	if err := a.AttachFile("missing", "x"); err == nil {
		t.Fatal("expected attach error for missing task")
	}

	done := waitForTaskStatus(t, a, rec.ID, aitypes.StatusWaitingConfirm, 3*time.Second)
	if len(done.InputFiles) != 2 {
		t.Fatalf("input files = %+v", done.InputFiles)
	}

	confirmed, err := a.Confirm(rec.ID, aitypes.ConfirmRequest{
		ApplyMode: "dry-run", ChannelID: "ch-1", DeviceID: "dev-1",
	})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status != aitypes.StatusApplied {
		t.Fatalf("status = %s", confirmed.Status)
	}
	if confirmed.AppliedResult["channel_id"] != "ch-1" {
		t.Fatalf("applied result = %+v", confirmed.AppliedResult)
	}
}

func TestAgent_ConfirmGuards(t *testing.T) {
	a := NewAgent("local")
	rec, err := a.Create(aitypes.CreateRequest{Skill: aitypes.SkillProtocolReverse})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := a.Confirm(rec.ID, aitypes.ConfirmRequest{}); err == nil {
		t.Fatal("expected confirm error before pipeline completes")
	}
	if _, err := a.Confirm("missing", aitypes.ConfirmRequest{}); err == nil {
		t.Fatal("expected confirm error for missing task")
	}
}

func TestAgent_ListAndValidateDeliverables(t *testing.T) {
	a := NewAgent("local")
	if len(a.List()) != 0 {
		t.Fatalf("expected empty list, got %d", len(a.List()))
	}
	rec, err := a.Create(aitypes.CreateRequest{Skill: aitypes.SkillProtocolReverse})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(a.List()) != 1 {
		t.Fatal("expected one task in list")
	}
	if _, ok := a.Get(rec.ID); !ok {
		t.Fatal("Get should find created task")
	}
	done := waitForTaskStatus(t, a, rec.ID, aitypes.StatusWaitingConfirm, 3*time.Second)
	report := a.ValidateDeliverables(done.Deliverables)
	if report == nil {
		t.Fatal("expected validation report")
	}
}

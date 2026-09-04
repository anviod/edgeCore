package ai_agent

import "github.com/anviod/edgeCore/internal/ai_agent/aitypes"

type (
	TaskStatus            = aitypes.TaskStatus
	PipelineStage         = aitypes.PipelineStage
	Skill                 = aitypes.Skill
	ProtocolModel         = aitypes.ProtocolModel
	PointCandidate        = aitypes.PointCandidate
	PointDefinition       = aitypes.PointDefinition
	DriverParameter       = aitypes.DriverParameter
	FrameEvidence         = aitypes.FrameEvidence
	ValidationCaseEntry   = aitypes.ValidationCaseEntry
	ValidationCase        = aitypes.ValidationCase
	Deliverables          = aitypes.Deliverables
	StageProgress         = aitypes.StageProgress
	ValidationFieldResult = aitypes.ValidationFieldResult
	ValidationReport      = aitypes.ValidationReport
	TaskRecord            = aitypes.TaskRecord
	CreateRequest         = aitypes.CreateRequest
	ConfirmRequest        = aitypes.ConfirmRequest
	Observation           = aitypes.Observation
)

const (
	StatusPending        = aitypes.StatusPending
	StatusQueued         = aitypes.StatusQueued
	StatusProcessing     = aitypes.StatusProcessing
	StatusWaitingModel   = aitypes.StatusWaitingModel
	StatusValidating     = aitypes.StatusValidating
	StatusWaitingConfirm = aitypes.StatusWaitingConfirm
	StatusApplied        = aitypes.StatusApplied
	StatusFailed         = aitypes.StatusFailed
	StatusCancelled      = aitypes.StatusCancelled

	StageCapture  = aitypes.StageCapture
	StageDecode   = aitypes.StageDecode
	StageSemantic = aitypes.StageSemantic
	StageOutput   = aitypes.StageOutput

	SkillProtocolReverse = aitypes.SkillProtocolReverse
	SkillDocParse        = aitypes.SkillDocParse
	SkillConfigGen       = aitypes.SkillConfigGen
	SkillEdgeRuleDraft   = aitypes.SkillEdgeRuleDraft
	SkillDiagnostics     = aitypes.SkillDiagnostics
)

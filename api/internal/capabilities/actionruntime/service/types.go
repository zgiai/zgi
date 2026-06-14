package service

import (
	"context"

	"github.com/google/uuid"
	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
)

const (
	AuthModeActorContext    = "actor_context"
	AuthModeInvocationToken = "invocation_token"

	RuntimeInternal   = "internal"
	RuntimeSkill      = "skill"
	RuntimeAgent      = "agent_runtime"
	RuntimeWorkflow   = "workflow_runtime"
	RuntimeAutomation = "automation_runtime"
	RuntimeFile       = "file_service"
)

type Scope struct {
	OrganizationID  uuid.UUID
	AccountID       uuid.UUID
	WorkspaceID     *uuid.UUID
	SkipAccessCheck bool
}

type CapabilityManifest struct {
	ID                   string
	Domain               string
	Action               string
	Name                 string
	Description          string
	Runtime              string
	AuthMode             string
	RiskLevel            string
	RequiresConfirmation bool
	IdempotencyRequired  bool
	TokenTTLSeconds      int
	AllowedResources     []string
	Scopes               []string
}

type ActionRunView struct {
	Run        *actionmodel.ActionRun
	Steps      []*actionmodel.ActionStep
	Capability *CapabilityManifest
}

type Executor interface {
	Execute(ctx context.Context, scope Scope, view ActionRunView, req actiondto.ExecuteActionRequest) (*ExecutionResult, error)
}

type ExecutionResult struct {
	Output   map[string]interface{}
	Ledger   map[string]interface{}
	Metadata map[string]interface{}
}

type Option func(*service)

type InvocationTokenClaims struct {
	ActionRunID    uuid.UUID  `json:"action_run_id"`
	StepID         uuid.UUID  `json:"step_id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	WorkspaceID    *uuid.UUID `json:"workspace_id,omitempty"`
	AccountID      uuid.UUID  `json:"account_id"`
	CapabilityID   string     `json:"capability_id"`
	ResourceIDs    []string   `json:"resource_ids,omitempty"`
	Scopes         []string   `json:"scopes,omitempty"`
	RiskLevel      string     `json:"risk_level"`
	Confirmed      bool       `json:"confirmed"`
	ExpiresAtUnix  int64      `json:"exp"`
	JTI            string     `json:"jti"`
	Audience       string     `json:"aud"`
}

type Service interface {
	ListCapabilities(ctx context.Context, scope Scope) ([]CapabilityManifest, error)
	PlanAction(ctx context.Context, scope Scope, req actiondto.ActionPlanRequest) (*ActionRunView, error)
	GetActionRun(ctx context.Context, scope Scope, id uuid.UUID) (*ActionRunView, error)
	ConfirmAction(ctx context.Context, scope Scope, id uuid.UUID, req actiondto.ConfirmActionRequest) (*ActionRunView, error)
	ExecuteAction(ctx context.Context, scope Scope, id uuid.UUID, req actiondto.ExecuteActionRequest) (*ActionRunView, error)
}

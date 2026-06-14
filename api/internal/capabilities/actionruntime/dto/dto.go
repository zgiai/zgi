package dto

type ResourceRef struct {
	Type     string                 `json:"type,omitempty"`
	ID       string                 `json:"id,omitempty"`
	Name     string                 `json:"name,omitempty"`
	Source   string                 `json:"source,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type ActionPlanRequest struct {
	ConversationID       string                 `json:"conversation_id,omitempty"`
	MessageID            string                 `json:"message_id,omitempty"`
	IdempotencyKey       string                 `json:"idempotency_key,omitempty"`
	Intent               string                 `json:"intent,omitempty"`
	CapabilityID         string                 `json:"capability_id" binding:"required"`
	Title                string                 `json:"title,omitempty"`
	Summary              string                 `json:"summary,omitempty"`
	Resources            []ResourceRef          `json:"resources,omitempty"`
	Arguments            map[string]interface{} `json:"arguments,omitempty"`
	OperationContext     map[string]interface{} `json:"operation_context,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	RequiresConfirmation *bool                  `json:"requires_confirmation,omitempty"`
	RiskLevel            string                 `json:"risk_level,omitempty"`
}

type ConfirmActionRequest struct {
	Confirmed bool                   `json:"confirmed"`
	Reason    string                 `json:"reason,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type ExecuteActionRequest struct {
	DryRun   bool                   `json:"dry_run,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

type ActionCapabilityResponse struct {
	ID                   string   `json:"id"`
	Domain               string   `json:"domain"`
	Action               string   `json:"action"`
	Name                 string   `json:"name"`
	Description          string   `json:"description"`
	Runtime              string   `json:"runtime"`
	AuthMode             string   `json:"auth_mode"`
	RiskLevel            string   `json:"risk_level"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
	IdempotencyRequired  bool     `json:"idempotency_required"`
	TokenTTLSeconds      int      `json:"token_ttl_seconds,omitempty"`
	AllowedResources     []string `json:"allowed_resources,omitempty"`
	Scopes               []string `json:"scopes,omitempty"`
}

type ActionRunResponse struct {
	ID                   string                    `json:"id"`
	OrganizationID       string                    `json:"organization_id"`
	WorkspaceID          *string                   `json:"workspace_id,omitempty"`
	AccountID            string                    `json:"account_id"`
	ConversationID       *string                   `json:"conversation_id,omitempty"`
	MessageID            *string                   `json:"message_id,omitempty"`
	IdempotencyKey       *string                   `json:"idempotency_key,omitempty"`
	Intent               string                    `json:"intent"`
	CapabilityID         string                    `json:"capability_id"`
	Title                string                    `json:"title"`
	Summary              string                    `json:"summary"`
	Status               string                    `json:"status"`
	RiskLevel            string                    `json:"risk_level"`
	RequiresConfirmation bool                      `json:"requires_confirmation"`
	ConfirmationStatus   string                    `json:"confirmation_status"`
	ConfirmedBy          *string                   `json:"confirmed_by,omitempty"`
	ConfirmedAt          *int64                    `json:"confirmed_at,omitempty"`
	CanceledAt           *int64                    `json:"canceled_at,omitempty"`
	Error                *string                   `json:"error,omitempty"`
	Resources            map[string]interface{}    `json:"resources,omitempty"`
	Arguments            map[string]interface{}    `json:"arguments,omitempty"`
	Ledger               map[string]interface{}    `json:"ledger,omitempty"`
	Metadata             map[string]interface{}    `json:"metadata,omitempty"`
	Capability           *ActionCapabilityResponse `json:"capability,omitempty"`
	Steps                []ActionStepResponse      `json:"steps"`
	CreatedAt            int64                     `json:"created_at"`
	UpdatedAt            int64                     `json:"updated_at"`
}

type ActionStepResponse struct {
	ID                   string                 `json:"id"`
	RunID                string                 `json:"run_id"`
	StepIndex            int                    `json:"step_index"`
	StepKey              string                 `json:"step_key"`
	CapabilityID         string                 `json:"capability_id"`
	Title                string                 `json:"title"`
	Status               string                 `json:"status"`
	RiskLevel            string                 `json:"risk_level"`
	RequiresConfirmation bool                   `json:"requires_confirmation"`
	StartedAt            *int64                 `json:"started_at,omitempty"`
	CompletedAt          *int64                 `json:"completed_at,omitempty"`
	Error                *string                `json:"error,omitempty"`
	Input                map[string]interface{} `json:"input,omitempty"`
	Output               map[string]interface{} `json:"output,omitempty"`
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt            int64                  `json:"created_at"`
	UpdatedAt            int64                  `json:"updated_at"`
}

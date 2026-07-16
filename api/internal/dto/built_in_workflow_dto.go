package dto

import "github.com/google/uuid"

// BuiltInWorkflowDTO represents a built-in workflow with all required information
// for API responses. Built-in workflows are system-provided workflows that are
// automatically seeded during deployment.
type BuiltInWorkflowDTO struct {
	Scenario    string    `json:"scenario"`    // Business scenario identifier (e.g., "global_chat", "bi_chat")
	AgentID     uuid.UUID `json:"agent_id"`    // Agent UUID (deterministically generated)
	AgentName   string    `json:"agent_name"`  // Display name of the agent
	WorkflowID  uuid.UUID `json:"workflow_id"` // Workflow UUID (deterministically generated)
	WebAppID    uuid.UUID `json:"web_app_id"`  // Web app UUID (deterministically generated)
	Description string    `json:"description"` // Description of the workflow
	AgentType   string    `json:"agent_type"`  // Agent type (e.g., "CONVERSATIONAL_WORKFLOW")
	Icon        *string   `json:"icon"`        // Icon configuration (JSON string)
	IconType    *string   `json:"icon_type"`   // Icon type (e.g., "text", "image")
}

type BuiltInWorkflowRuntimeSurfaceAuthorizationResponse struct {
	Scenario       string                             `json:"scenario"`
	AgentID        string                             `json:"agent_id"`
	OrganizationID string                             `json:"organization_id"`
	Surfaces       []AgentRuntimeSurfaceAuthorization `json:"surfaces"`
}

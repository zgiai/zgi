package entities

import (
	"sync"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type VariablePool struct {
	mu sync.RWMutex `json:"-"`

	// Variable dictionary, first level key is node_id, second level key is variable name
	VariableDictionary    map[string]map[string]Variable `json:"variable_dictionary"`
	UserInputs            map[string]any                 `json:"user_inputs"`
	SystemVariables       *SystemVariable                `json:"system_variables"`
	EnvironmentVariables  []Variable                     `json:"environment_variables"`
	ConversationVariables []Variable                     `json:"conversation_variables"`
}

const (
	SelectorsLength            = 2
	SystemVariableNodeId       = "sys"
	EnvironmentVariableNodeId  = "environment"
	ConversationVariableNodeId = "conversation"
)

const (
	// SecretHiddenValue is the placeholder value for secret variables in API responses
	// When a secret variable has this value, it means the actual value should be retrieved from persisted variables
	SecretHiddenValue = "[__HIDDEN__]"

	// SecretValueType is the value type for secret variables
	// This matches shared.SegmentTypeSecret but is used for string comparisons
	SecretValueType = "secret"
)

type Segment interface {
	ToObject() any
	GetValue() any
	GetType() shared.SegmentType
	Text() string
	Log() string
	Markdown() string
	Size() int
}

// Variable interface definition
type Variable interface {
	Segment
	GetName() string
	GetSelector() []string
}

type SystemVariable struct {
	TenantID           string `json:"tenant_id"`
	WorkspaceID        string `json:"workspace_id"`
	OrganizationID     string `json:"organization_id"`
	BillingSubjectType string `json:"billing_subject_type"`
	UserID             string `json:"user_id"`
	AppID              string `json:"app_id"`
	WorkflowID         string `json:"workflow_id"`
	WorkflowRunID      string `json:"workflow_run_id"`
	WorkflowType       string `json:"workflow_type"`
	Query              string `json:"query"`
	ConversationID     string `json:"conversation_id"`
	DialogueCount      int    `json:"dialogue_count"`
}

func (sv *SystemVariable) ToDict() map[string]any {
	return map[string]any{
		"tenant_id":            sv.TenantID,
		"workspace_id":         sv.WorkspaceID,
		"organization_id":      sv.OrganizationID,
		"billing_subject_type": sv.BillingSubjectType,
		"user_id":              sv.UserID,
		"app_id":               sv.AppID,
		"agent_id":             sv.AppID,
		"workflow_id":          sv.WorkflowID,
		"workflow_run_id":      sv.WorkflowRunID,
		"workflow_type":        sv.WorkflowType,
		"query":                sv.Query,
		"conversation_id":      sv.ConversationID,
		"dialogue_count":       sv.DialogueCount,
	}
}

func SystemVariableEmpty() *SystemVariable {
	return &SystemVariable{}
}

// VariablePool variable pool implementation

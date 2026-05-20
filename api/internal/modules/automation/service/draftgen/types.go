package draftgen

import (
	"errors"

	automationdto "github.com/zgiai/ginext/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
)

var (
	ErrModelNotConfigured = errors.New("automation task draft generation requires a configured default LLM model")
	ErrModelOutputInvalid = errors.New("automation task draft model output is not usable")
)

type GenerateRequest struct {
	Prompt         string
	Locale         string
	Timezone       string
	Provider       string
	Model          string
	WorkspaceID    string
	OrganizationID string
	AccountID      string
}

type AutomationTaskDraft struct {
	Name           string                                  `json:"name"`
	Description    *string                                 `json:"description,omitempty"`
	ScheduleType   automationmodel.AutomationScheduleType  `json:"schedule_type"`
	Timezone       string                                  `json:"timezone,omitempty"`
	ScheduleConfig map[string]interface{}                  `json:"schedule_config"`
	Actions        []automationdto.CreateTaskActionRequest `json:"actions"`
}

type GenerateResult struct {
	Draft         AutomationTaskDraft `json:"draft"`
	MissingFields []string            `json:"missing_fields,omitempty"`
	Warnings      []string            `json:"warnings,omitempty"`
	Summary       string              `json:"summary,omitempty"`
	Provider      string              `json:"provider,omitempty"`
	Model         string              `json:"model,omitempty"`
}

package dto

import (
	"encoding/json"
	"time"

	jsondto "github.com/zgiai/zgi/api/internal/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
)

type PromptListRequest struct {
	Page        int    `form:"page"`
	Limit       int    `form:"limit"`
	Keyword     string `form:"keyword"`
	WorkspaceID string `form:"workspace_id"`
	Locale      string `form:"locale"`
	Source      string `form:"source"`
	Category    string `form:"category"`
}

type PromptVersionInput struct {
	PromptType    string          `json:"prompt_type" binding:"required,oneof=text chat"`
	Content       json.RawMessage `json:"content" binding:"required"`
	Config        jsondto.JSONMap `json:"config"`
	Labels        []string        `json:"labels"`
	CommitMessage *string         `json:"commit_message,omitempty"`
}

type CreatePromptRequest struct {
	WorkspaceID    string             `json:"workspace_id" binding:"required,uuid"`
	Source         string             `json:"source" binding:"required,oneof=personal workspace"`
	Name           string             `json:"name" binding:"required,max=255"`
	Slug           string             `json:"slug" binding:"omitempty,max=255"`
	Description    *string            `json:"description,omitempty"`
	Locale         string             `json:"locale" binding:"omitempty,max=32"`
	Category       *string            `json:"category,omitempty"`
	Tags           []string           `json:"tags"`
	InitialVersion PromptVersionInput `json:"initial_version" binding:"required"`
}

type UpdatePromptRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Locale      *string  `json:"locale,omitempty"`
	Category    *string  `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Source      *string  `json:"source,omitempty"`
}

type SetPromptLabelsRequest struct {
	Version int      `json:"version" binding:"required,min=1"`
	Labels  []string `json:"labels"`
}

type PromptOptimizeRequest struct {
	RawPrompt         string `json:"raw_prompt" binding:"required"`
	Goal              string `json:"goal" binding:"omitempty,oneof=general reliable structured professional json deep"`
	PreserveVariables *bool  `json:"preserve_variables,omitempty"`
	Provider          string `json:"provider" binding:"omitempty,max=128"`
	Model             string `json:"model" binding:"omitempty,max=255"`
	PromptID          string `json:"prompt_id" binding:"omitempty,uuid"`
}

type PromptOptimizationRunListRequest struct {
	Page  int `form:"page"`
	Limit int `form:"limit"`
}

type PromptOptimizationAdoptRequest struct {
	Variant       string  `json:"variant" binding:"required,oneof=safe balanced advanced"`
	CommitMessage *string `json:"commit_message,omitempty"`
}

type PromptPlaygroundRequest struct {
	Prompt    string                    `json:"prompt,omitempty"`
	Messages  []PromptPlaygroundMessage `json:"messages,omitempty"`
	Input     string                    `json:"input,omitempty"`
	Variables map[string]string         `json:"variables,omitempty"`
	Provider  string                    `json:"provider" binding:"omitempty,max=128"`
	Model     string                    `json:"model" binding:"omitempty,max=255"`
}

type PromptPlaygroundMessage struct {
	Role    string `json:"role" binding:"required,oneof=system user assistant"`
	Content string `json:"content" binding:"required"`
}

type PromptSummaryResponse struct {
	ID               string    `json:"id"`
	WorkspaceID      *string   `json:"workspace_id,omitempty"`
	OwnerAccountID   *string   `json:"owner_account_id,omitempty"`
	Source           string    `json:"source"`
	Name             string    `json:"name"`
	Slug             string    `json:"slug"`
	Description      *string   `json:"description,omitempty"`
	Locale           string    `json:"locale"`
	Category         *string   `json:"category,omitempty"`
	Tags             []string  `json:"tags"`
	LatestVersion    int       `json:"latest_version"`
	LatestLabels     []string  `json:"latest_labels"`
	LatestPromptType string    `json:"latest_prompt_type"`
	IsOwned          bool      `json:"is_owned"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type PromptVersionResponse struct {
	ID            string          `json:"id"`
	Version       int             `json:"version"`
	PromptType    string          `json:"prompt_type"`
	Content       json.RawMessage `json:"content"`
	Config        json.RawMessage `json:"config"`
	Labels        []string        `json:"labels"`
	CommitMessage *string         `json:"commit_message,omitempty"`
	CreatedBy     *string         `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type PromptDetailResponse struct {
	PromptSummaryResponse
	Versions []PromptVersionResponse `json:"versions"`
}

type PromptListResponse struct {
	Data    []PromptSummaryResponse `json:"data"`
	HasMore bool                    `json:"has_more"`
	Limit   int                     `json:"limit"`
	Page    int                     `json:"page"`
	Total   int64                   `json:"total"`
}

type PromptOptimizeVariantsResponse struct {
	Safe     string `json:"safe"`
	Balanced string `json:"balanced"`
	Advanced string `json:"advanced"`
}

type PromptOptimizeResponse struct {
	Goal              string                         `json:"goal"`
	PreserveVariables bool                           `json:"preserve_variables"`
	DetectedVariables []string                       `json:"detected_variables"`
	RunID             string                         `json:"run_id"`
	Output            string                         `json:"output"`
	Variants          PromptOptimizeVariantsResponse `json:"variants"`
}

type PromptOptimizationRunResponse struct {
	ID                     string                         `json:"id"`
	PromptID               *string                        `json:"prompt_id,omitempty"`
	Goal                   string                         `json:"goal"`
	Provider               *string                        `json:"provider,omitempty"`
	Model                  *string                        `json:"model,omitempty"`
	PreserveVariables      bool                           `json:"preserve_variables"`
	DetectedVariables      []string                       `json:"detected_variables"`
	RawPrompt              string                         `json:"raw_prompt"`
	Output                 string                         `json:"output"`
	Variants               PromptOptimizeVariantsResponse `json:"variants"`
	AdoptedVariant         *string                        `json:"adopted_variant,omitempty"`
	AdoptedPromptVersionID *string                        `json:"adopted_prompt_version_id,omitempty"`
	AdoptedAt              *time.Time                     `json:"adopted_at,omitempty"`
	CreatedAt              time.Time                      `json:"created_at"`
	UpdatedAt              time.Time                      `json:"updated_at"`
}

type PromptOptimizationRunListResponse struct {
	Data    []PromptOptimizationRunResponse `json:"data"`
	HasMore bool                            `json:"has_more"`
	Limit   int                             `json:"limit"`
	Page    int                             `json:"page"`
	Total   int64                           `json:"total"`
}

type PromptUsageReferenceResponse struct {
	AgentID       string    `json:"agent_id"`
	AgentName     string    `json:"agent_name"`
	WorkflowID    string    `json:"workflow_id"`
	NodeID        string    `json:"node_id"`
	NodeTitle     string    `json:"node_title"`
	ReferenceMode string    `json:"reference_mode,omitempty"`
	Label         *string   `json:"label,omitempty"`
	Version       *int      `json:"version,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PromptUsageRecentRunResponse struct {
	WorkflowRunID    *string    `json:"workflow_run_id,omitempty"`
	AgentID          string     `json:"agent_id"`
	AgentName        string     `json:"agent_name"`
	NodeID           string     `json:"node_id"`
	NodeTitle        string     `json:"node_title"`
	Status           string     `json:"status"`
	PromptVersion    *int       `json:"prompt_version,omitempty"`
	RequestedLabel   *string    `json:"requested_label,omitempty"`
	RequestedVersion *int       `json:"requested_version,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	ElapsedTime      float64    `json:"elapsed_time"`
}

type PromptUsageSummaryResponse struct {
	LinkedAgentsCount int64                          `json:"linked_agents_count"`
	LinkedNodesCount  int64                          `json:"linked_nodes_count"`
	TotalRunCount     int64                          `json:"total_run_count"`
	LastRunAt         *time.Time                     `json:"last_run_at,omitempty"`
	VersionMetrics    []PromptUsageVersionMetric     `json:"version_metrics"`
	LabelMetrics      []PromptUsageLabelMetric       `json:"label_metrics"`
	References        []PromptUsageReferenceResponse `json:"references"`
	RecentRuns        []PromptUsageRecentRunResponse `json:"recent_runs"`
}

type PromptUsageVersionMetric struct {
	Version   int        `json:"version"`
	RunCount  int64      `json:"run_count"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
}

type PromptUsageLabelMetric struct {
	Label     string     `json:"label"`
	RunCount  int64      `json:"run_count"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
}

func BuildPromptSummary(prompt *promptmodel.Prompt, latest *promptmodel.PromptVersion, accountID string) PromptSummaryResponse {
	resp := PromptSummaryResponse{
		ID:             prompt.ID,
		WorkspaceID:    prompt.WorkspaceID,
		OwnerAccountID: prompt.OwnerAccountID,
		Source:         string(prompt.Source),
		Name:           prompt.Name,
		Slug:           prompt.Slug,
		Description:    prompt.Description,
		Locale:         prompt.Locale,
		Category:       prompt.Category,
		Tags:           append([]string{}, prompt.Tags...),
		LatestVersion:  prompt.LatestVersion,
		IsOwned:        prompt.OwnerAccountID != nil && *prompt.OwnerAccountID == accountID,
		CreatedAt:      prompt.CreatedAt,
		UpdatedAt:      prompt.UpdatedAt,
	}
	if latest != nil {
		resp.LatestLabels = append([]string{}, latest.Labels...)
		resp.LatestPromptType = string(latest.PromptType)
	}
	return resp
}

func BuildPromptVersionResponse(version *promptmodel.PromptVersion) PromptVersionResponse {
	return PromptVersionResponse{
		ID:            version.ID,
		Version:       version.Version,
		PromptType:    string(version.PromptType),
		Content:       json.RawMessage(version.Content),
		Config:        json.RawMessage(version.Config),
		Labels:        append([]string{}, version.Labels...),
		CommitMessage: version.CommitMessage,
		CreatedBy:     version.CreatedBy,
		CreatedAt:     version.CreatedAt,
		UpdatedAt:     version.UpdatedAt,
	}
}

func BuildPromptOptimizationRunResponse(run *promptmodel.PromptOptimizationRun) PromptOptimizationRunResponse {
	return PromptOptimizationRunResponse{
		ID:                run.ID,
		PromptID:          run.PromptID,
		Goal:              run.Goal,
		Provider:          run.Provider,
		Model:             run.Model,
		PreserveVariables: run.PreserveVariables,
		DetectedVariables: append([]string{}, run.DetectedVariables...),
		RawPrompt:         run.RawPrompt,
		Output:            preferredOptimizationOutput(run),
		Variants: PromptOptimizeVariantsResponse{
			Safe:     run.SafeOutput,
			Balanced: run.BalancedOutput,
			Advanced: run.AdvancedOutput,
		},
		AdoptedVariant:         run.AdoptedVariant,
		AdoptedPromptVersionID: run.AdoptedPromptVersionID,
		AdoptedAt:              run.AdoptedAt,
		CreatedAt:              run.CreatedAt,
		UpdatedAt:              run.UpdatedAt,
	}
}

func preferredOptimizationOutput(run *promptmodel.PromptOptimizationRun) string {
	if run == nil {
		return ""
	}
	if run.BalancedOutput != "" {
		return run.BalancedOutput
	}
	if run.SafeOutput != "" {
		return run.SafeOutput
	}
	return run.AdvancedOutput
}

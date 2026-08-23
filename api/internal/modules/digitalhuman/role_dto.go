package digitalhuman

import "github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"

type CreatePersonaTemplateRequest struct {
	Name    string   `json:"name"`
	Persona string   `json:"persona"`
	Goals   []string `json:"goals"`
	Rules   []string `json:"rules"`
}

type UpdatePersonaTemplateRequest struct {
	ExpectedRevision int64     `json:"expected_revision"`
	Name             *string   `json:"name,omitempty"`
	Persona          *string   `json:"persona,omitempty"`
	Goals            *[]string `json:"goals,omitempty"`
	Rules            *[]string `json:"rules,omitempty"`
}

type PersonaTemplateListResult struct {
	Items    []*PersonaTemplate `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	HasMore  bool               `json:"has_more"`
}

type ResourceBindingInput struct {
	Type             resourcebindings.BindingType `json:"type"`
	ResourceID       string                       `json:"resource_id"`
	ParentResourceID string                       `json:"parent_resource_id,omitempty"`
	DisplayName      string                       `json:"display_name,omitempty"`
	AccessMode       resourcebindings.AccessMode  `json:"access_mode"`
	ResourceRevision string                       `json:"resource_revision"`
	CredentialRef    string                       `json:"credential_ref,omitempty"`
	Metadata         map[string]any               `json:"metadata"`
}

type ReplaceResourceBindingsRequest struct {
	ExpectedDraftLockVersion int64                  `json:"expected_draft_lock_version"`
	Items                    []ResourceBindingInput `json:"items"`
}

type DraftResourceBindings struct {
	DraftLockVersion int64              `json:"draft_lock_version"`
	Items            []*ResourceBinding `json:"items"`
}

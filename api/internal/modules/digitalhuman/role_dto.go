package digitalhuman

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
)

type CreatePersonaTemplateRequest struct {
	Name string `json:"name"`
	// IdentityName defaults to Name, as before templates had a title of
	// their own.
	IdentityName       *string  `json:"identity_name,omitempty"`
	Description        string   `json:"description"`
	Persona            string   `json:"persona"`
	Goals              []string `json:"goals"`
	Rules              []string `json:"rules"`
	MemoryInstructions string   `json:"memory_instructions"`
	SkillIDs           []string `json:"skill_ids"`
}

type UpdatePersonaTemplateRequest struct {
	ExpectedRevision   int64     `json:"expected_revision"`
	Name               *string   `json:"name,omitempty"`
	IdentityName       *string   `json:"identity_name,omitempty"`
	Description        *string   `json:"description,omitempty"`
	Persona            *string   `json:"persona,omitempty"`
	Goals              *[]string `json:"goals,omitempty"`
	Rules              *[]string `json:"rules,omitempty"`
	MemoryInstructions *string   `json:"memory_instructions,omitempty"`
	SkillIDs           *[]string `json:"skill_ids,omitempty"`
}

type PersonaTemplateListResult struct {
	Items    []*PersonaTemplate `json:"items"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	HasMore  bool               `json:"has_more"`
}

type ResourceBindingInput struct {
	ID               *uuid.UUID                   `json:"id,omitempty"`
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
	ExpectedDraftVersionID   *uuid.UUID             `json:"expected_draft_version_id,omitempty"`
	ExpectedDraftLockVersion int64                  `json:"expected_draft_lock_version"`
	Items                    []ResourceBindingInput `json:"items"`
}

type DraftResourceBindings struct {
	DraftVersionID   uuid.UUID          `json:"draft_version_id"`
	DraftLockVersion int64              `json:"draft_lock_version"`
	Items            []*ResourceBinding `json:"items"`
}

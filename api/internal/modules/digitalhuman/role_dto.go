package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
)

type CreateRoleTemplateRequest struct {
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Goals            []string             `json:"goals"`
	Responsibilities []string             `json:"responsibilities"`
	Capabilities     []runspec.VersionRef `json:"capabilities"`
	SOPRefs          []runspec.VersionRef `json:"sop_refs"`
	KnowledgeRefs    []runspec.VersionRef `json:"knowledge_refs"`
	Policy           map[string]any       `json:"policy"`
	EvaluationRefs   []runspec.VersionRef `json:"evaluation_refs"`
}

type UpdateRoleTemplateRequest struct {
	ExpectedRevision int64   `json:"expected_revision"`
	Name             *string `json:"name,omitempty"`
	Description      *string `json:"description,omitempty"`
}

type UpdateRoleTemplateDraftRequest struct {
	ExpectedLockVersion int64                 `json:"expected_lock_version"`
	Goals               *[]string             `json:"goals,omitempty"`
	Responsibilities    *[]string             `json:"responsibilities,omitempty"`
	Capabilities        *[]runspec.VersionRef `json:"capabilities,omitempty"`
	SOPRefs             *[]runspec.VersionRef `json:"sop_refs,omitempty"`
	KnowledgeRefs       *[]runspec.VersionRef `json:"knowledge_refs,omitempty"`
	Policy              *map[string]any       `json:"policy,omitempty"`
	EvaluationRefs      *[]runspec.VersionRef `json:"evaluation_refs,omitempty"`
}

type PublishRoleTemplateRequest struct {
	ExpectedRevision         int64 `json:"expected_revision"`
	ExpectedDraftLockVersion int64 `json:"expected_draft_lock_version"`
}

type RoleTemplateDetail struct {
	Template  *RoleTemplate        `json:"template"`
	Draft     *RoleTemplateVersion `json:"draft"`
	Published *RoleTemplateVersion `json:"published,omitempty"`
}

type RoleTemplateListResult struct {
	Items    []*RoleTemplate `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	HasMore  bool            `json:"has_more"`
}

type RoleAssignmentInput struct {
	RoleTemplateVersionID uuid.UUID      `json:"role_template_version_id"`
	Parameters            map[string]any `json:"parameters"`
	OverridePolicy        map[string]any `json:"override_policy"`
	EffectiveFrom         *time.Time     `json:"effective_from,omitempty"`
	EffectiveTo           *time.Time     `json:"effective_to,omitempty"`
}

type ReplaceRoleAssignmentsRequest struct {
	ExpectedDraftLockVersion int64                 `json:"expected_draft_lock_version"`
	Items                    []RoleAssignmentInput `json:"items"`
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

type DraftRoleAssignments struct {
	DraftLockVersion int64             `json:"draft_lock_version"`
	Items            []*RoleAssignment `json:"items"`
}

type DraftResourceBindings struct {
	DraftLockVersion int64              `json:"draft_lock_version"`
	Items            []*ResourceBinding `json:"items"`
}

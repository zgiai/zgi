package digitalhuman

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
)

const (
	SurfaceConsole = "digital_human_console"
	SurfaceWebApp  = "digital_human_webapp"
	SurfaceAPI     = "digital_human_api"
	SurfaceTask    = "digital_human_task"
	SurfaceChannel = "channel"
	// SurfacePreview is the configuration page's preview: a person who may
	// edit the DigitalHuman chats with its unpublished draft. It never
	// changes tasks, artifacts, memory, or connected systems.
	SurfacePreview = "digital_human_preview"
)

// Scope is the authenticated organization, workspace, and actor boundary.
type Scope struct {
	OrganizationID  uuid.UUID
	WorkspaceID     uuid.UUID
	ActorID         uuid.UUID
	PrincipalType   string
	PrincipalID     uuid.UUID
	UserSubjectKind string
	UserSubjectID   *uuid.UUID
	WebAppID        *uuid.UUID
	Surface         string
}

// PresentationPolicy is the versioned public appearance of a DigitalHuman.
type PresentationPolicy struct {
	HomeTitle          string   `json:"home_title"`
	OpeningStatement   string   `json:"opening_statement"`
	InputPlaceholder   string   `json:"input_placeholder"`
	SuggestedQuestions []string `json:"suggested_questions"`
}

// CreateRequest creates a DigitalHuman and its initial mutable draft.
type CreateRequest struct {
	FileUploadPolicy   *chatfile.Policy        `json:"file_upload_policy,omitempty"`
	Name               string                  `json:"name"`
	Avatar             string                  `json:"avatar"`
	IconType           string                  `json:"icon_type"`
	Icon               string                  `json:"icon"`
	Description        string                  `json:"description"`
	AutonomyLevel      AutonomyLevel           `json:"autonomy_level"`
	Visibility         Visibility              `json:"visibility"`
	IdentityName       string                  `json:"identity_name"`
	Persona            string                  `json:"persona"`
	Goals              []string                `json:"goals"`
	Rules              []string                `json:"rules"`
	DefaultModel       string                  `json:"default_model"`
	MemoryPolicy       map[string]any          `json:"memory_policy"`
	TriggerPolicy      map[string]any          `json:"trigger_policy"`
	PresentationPolicy PresentationPolicy      `json:"presentation_policy"`
	ExecutionRuntime   *ExecutionRuntimeConfig `json:"execution_runtime,omitempty"`
}

// UpdateRequest updates aggregate metadata with optimistic concurrency control.
type UpdateRequest struct {
	ExpectedRevision int64          `json:"expected_revision"`
	Name             *string        `json:"name,omitempty"`
	Avatar           *string        `json:"avatar,omitempty"`
	IconType         *string        `json:"icon_type,omitempty"`
	Icon             *string        `json:"icon,omitempty"`
	Description      *string        `json:"description,omitempty"`
	Status           *Status        `json:"status,omitempty"`
	AutonomyLevel    *AutonomyLevel `json:"autonomy_level,omitempty"`
	Visibility       *Visibility    `json:"visibility,omitempty"`
}

// UpdateDraftRequest updates the mutable identity draft.
type UpdateDraftRequest struct {
	FileUploadPolicy    *chatfile.Policy        `json:"file_upload_policy,omitempty"`
	ExpectedLockVersion int64                   `json:"expected_lock_version"`
	IdentityName        *string                 `json:"identity_name,omitempty"`
	Persona             *string                 `json:"persona,omitempty"`
	Goals               *[]string               `json:"goals,omitempty"`
	Rules               *[]string               `json:"rules,omitempty"`
	DefaultModel        *string                 `json:"default_model,omitempty"`
	MemoryPolicy        *map[string]any         `json:"memory_policy,omitempty"`
	TriggerPolicy       *map[string]any         `json:"trigger_policy,omitempty"`
	PresentationPolicy  *PresentationPolicy     `json:"presentation_policy,omitempty"`
	ExecutionRuntime    *ExecutionRuntimeConfig `json:"execution_runtime,omitempty"`
}

// PublishRequest promotes the current draft and creates the next mutable draft.
type PublishRequest struct {
	ExpectedRevision         int64  `json:"expected_revision"`
	ExpectedDraftLockVersion int64  `json:"expected_draft_lock_version"`
	Name                     string `json:"name,omitempty"`
	Description              string `json:"description,omitempty"`
}

// ListQuery controls workspace-scoped DigitalHuman pagination.
type ListQuery struct {
	Page     int
	PageSize int
	Search   string
}

// ListResult is one page of DigitalHuman definitions.
type ListResult struct {
	Items    []*Definition `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	HasMore  bool          `json:"has_more"`
}

// Detail combines aggregate metadata with its draft and latest publication.
type Detail struct {
	Definition *Definition `json:"definition"`
	Draft      *Version    `json:"draft"`
	Published  *Version    `json:"published,omitempty"`
	// HasUnpublishedChanges compares the draft with the current publication,
	// including resource bindings, which the configuration revision omits.
	HasUnpublishedChanges      bool                        `json:"has_unpublished_changes"`
	ExecutionRuntimeCapability *ExecutionRuntimeCapability `json:"execution_runtime_capability,omitempty"`
}

// ExecutionRuntimeCapability is the safe, editor-facing projection of the
// deployment-owned Linux runtime admission decision. It intentionally omits
// profile, policy, backend, snapshot, and organization identifiers.
type ExecutionRuntimeCapability struct {
	Available         bool                                       `json:"available"`
	CanEnable         bool                                       `json:"can_enable"`
	Reason            string                                     `json:"reason,omitempty"`
	EntryCapabilities map[string]ExecutionRuntimeEntryCapability `json:"entry_capabilities,omitempty"`
}

// ExecutionRuntimeEntryCapability describes admission for one DigitalHuman
// entry. It contains no deployment-owned profile or organization details.
type ExecutionRuntimeEntryCapability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// PublishedVersionsResult is a newest-first publication history page.
type PublishedVersionsResult struct {
	Items    []*Version `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	HasMore  bool       `json:"has_more"`
}

// VersionRestoreBindingImpact describes one resource binding changed by restoring a publication.
type VersionRestoreBindingImpact struct {
	Change           string                       `json:"change"`
	Type             resourcebindings.BindingType `json:"type"`
	ResourceID       string                       `json:"resource_id"`
	ParentResourceID string                       `json:"parent_resource_id,omitempty"`
	DisplayName      string                       `json:"display_name,omitempty"`
}

// VersionRestoreBindingSnapshot is the credential-free resource state rendered while previewing a publication.
type VersionRestoreBindingSnapshot struct {
	ID               uuid.UUID                    `json:"id"`
	Type             resourcebindings.BindingType `json:"binding_type"`
	ResourceID       string                       `json:"resource_id"`
	ParentResourceID string                       `json:"parent_resource_id,omitempty"`
	DisplayName      string                       `json:"display_name,omitempty"`
	AccessMode       resourcebindings.AccessMode  `json:"access_mode"`
	ResourceRevision string                       `json:"resource_revision"`
	BindingRevision  string                       `json:"binding_revision"`
	Metadata         map[string]any               `json:"metadata"`
}

// VersionRestorePreview is a point-in-time summary of restoring a publication to the draft.
type VersionRestorePreview struct {
	VersionID                     uuid.UUID                       `json:"version_id"`
	VersionNumber                 int                             `json:"version_number"`
	IsCurrent                     bool                            `json:"is_current"`
	WillDiscardUnpublishedChanges bool                            `json:"will_discard_unpublished_changes"`
	ChangedFields                 []string                        `json:"changed_fields"`
	BindingChanges                []VersionRestoreBindingImpact   `json:"binding_changes"`
	Bindings                      []VersionRestoreBindingSnapshot `json:"bindings"`
	ImpactToken                   string                          `json:"impact_token"`
}

// RestoreVersionRequest restores an immutable publication into the current mutable draft.
type RestoreVersionRequest struct {
	VersionID                uuid.UUID `json:"version_id"`
	ExpectedRevision         int64     `json:"expected_revision"`
	ExpectedDraftLockVersion int64     `json:"expected_draft_lock_version"`
	ImpactToken              string    `json:"impact_token"`
}

// DeleteImpact explains whether deleting a DigitalHuman is currently safe.
type DeleteImpact struct {
	CanDelete             bool     `json:"can_delete"`
	PublishedVersionCount int64    `json:"published_version_count"`
	Blockers              []string `json:"blockers"`
}

// DraftConfig is the normalized, JSON-compatible version content used for hashing.
type DraftConfig struct {
	FileUploadPolicy   *chatfile.Policy        `json:"file_upload_policy,omitempty"`
	IdentityName       string                  `json:"identity_name"`
	Persona            string                  `json:"persona"`
	Goals              []string                `json:"goals"`
	Rules              []string                `json:"rules"`
	DefaultModel       string                  `json:"default_model"`
	MemoryPolicy       json.RawMessage         `json:"memory_policy"`
	TriggerPolicy      json.RawMessage         `json:"trigger_policy"`
	PresentationPolicy json.RawMessage         `json:"presentation_policy"`
	ExecutionRuntime   *ExecutionRuntimeConfig `json:"execution_runtime,omitempty"`
}

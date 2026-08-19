package digitalhuman

import (
	"encoding/json"

	"github.com/google/uuid"
)

// Scope is the authenticated organization, workspace, and actor boundary.
type Scope struct {
	OrganizationID uuid.UUID
	WorkspaceID    uuid.UUID
	ActorID        uuid.UUID
}

// CreateRequest creates a DigitalHuman and its initial mutable draft.
type CreateRequest struct {
	Name          string         `json:"name"`
	Avatar        string         `json:"avatar"`
	Description   string         `json:"description"`
	AutonomyLevel AutonomyLevel  `json:"autonomy_level"`
	Visibility    Visibility     `json:"visibility"`
	Persona       string         `json:"persona"`
	Rules         []string       `json:"rules"`
	DefaultModel  string         `json:"default_model"`
	MemoryPolicy  map[string]any `json:"memory_policy"`
	TriggerPolicy map[string]any `json:"trigger_policy"`
}

// UpdateRequest updates aggregate metadata with optimistic concurrency control.
type UpdateRequest struct {
	ExpectedRevision int64          `json:"expected_revision"`
	Name             *string        `json:"name,omitempty"`
	Avatar           *string        `json:"avatar,omitempty"`
	Description      *string        `json:"description,omitempty"`
	Status           *Status        `json:"status,omitempty"`
	AutonomyLevel    *AutonomyLevel `json:"autonomy_level,omitempty"`
	Visibility       *Visibility    `json:"visibility,omitempty"`
}

// UpdateDraftRequest updates the mutable identity draft.
type UpdateDraftRequest struct {
	ExpectedLockVersion int64           `json:"expected_lock_version"`
	Persona             *string         `json:"persona,omitempty"`
	Rules               *[]string       `json:"rules,omitempty"`
	DefaultModel        *string         `json:"default_model,omitempty"`
	MemoryPolicy        *map[string]any `json:"memory_policy,omitempty"`
	TriggerPolicy       *map[string]any `json:"trigger_policy,omitempty"`
}

// PublishRequest promotes the current draft and creates the next mutable draft.
type PublishRequest struct {
	ExpectedRevision         int64 `json:"expected_revision"`
	ExpectedDraftLockVersion int64 `json:"expected_draft_lock_version"`
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
}

// PublishedVersionsResult is a newest-first publication history page.
type PublishedVersionsResult struct {
	Items    []*Version `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	HasMore  bool       `json:"has_more"`
}

// DeleteImpact explains whether deleting a DigitalHuman is currently safe.
type DeleteImpact struct {
	CanDelete             bool     `json:"can_delete"`
	PublishedVersionCount int64    `json:"published_version_count"`
	Blockers              []string `json:"blockers"`
}

// DraftConfig is the normalized, JSON-compatible version content used for hashing.
type DraftConfig struct {
	Persona       string          `json:"persona"`
	Rules         []string        `json:"rules"`
	DefaultModel  string          `json:"default_model"`
	MemoryPolicy  json.RawMessage `json:"memory_policy"`
	TriggerPolicy json.RawMessage `json:"trigger_policy"`
}

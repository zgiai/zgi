// Package resourcebindings defines subject-neutral resource authorization snapshots.
package resourcebindings

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

// ErrInvalidReference indicates an incomplete subject, snapshot, or resource match.
var ErrInvalidReference = errors.New("invalid resource binding reference")

type Phase string

const (
	PhaseDraft     Phase = "draft"
	PhasePublished Phase = "published"
)

type BindingType string

const (
	BindingTypeSkill                 BindingType = "skill"
	BindingTypeKnowledgeDataset      BindingType = "knowledge_dataset"
	BindingTypeDatabase              BindingType = "database"
	BindingTypeDatabaseTable         BindingType = "database_table"
	BindingTypeWorkflow              BindingType = "workflow"
	BindingTypeIntegrationConnection BindingType = "integration_connection"
)

type AccessMode string

const (
	AccessModeRead    AccessMode = "read"
	AccessModeWrite   AccessMode = "write"
	AccessModeExecute AccessMode = "execute"
)

const IntegrationAllowedActionIDsMetadataKey = "allowed_action_ids"

// SnapshotRef selects one exact subject configuration snapshot.
type SnapshotRef struct {
	Subject   runtimeidentity.SubjectRef `json:"subject"`
	Phase     Phase                      `json:"phase"`
	VersionID *uuid.UUID                 `json:"version_id,omitempty"`
}

// AuthorizationEvidence records who approved access without containing credentials.
type AuthorizationEvidence struct {
	AuthorizedBy *uuid.UUID `json:"authorized_by,omitempty"`
	AuthorizedAt *time.Time `json:"authorized_at,omitempty"`
}

// Binding is a credential-safe, immutable resource authorization projection.
type Binding struct {
	ID               uuid.UUID              `json:"id"`
	Snapshot         SnapshotRef            `json:"snapshot"`
	Type             BindingType            `json:"type"`
	ResourceID       string                 `json:"resource_id"`
	ParentResourceID string                 `json:"parent_resource_id,omitempty"`
	DisplayName      string                 `json:"display_name,omitempty"`
	AccessMode       AccessMode             `json:"access_mode"`
	Revision         string                 `json:"revision"`
	CredentialRef    string                 `json:"credential_ref,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Authorization    AuthorizationEvidence  `json:"authorization"`
}

// Match describes the concrete resource a runtime step is about to use.
type Match struct {
	Type             BindingType `json:"type"`
	ResourceID       string      `json:"resource_id"`
	ParentResourceID string      `json:"parent_resource_id,omitempty"`
	AccessMode       AccessMode  `json:"access_mode"`
	ActionID         string      `json:"action_id,omitempty"`
}

// Decision is the result of the provider-adjacent authorization recheck.
type Decision struct {
	Allowed bool     `json:"allowed"`
	Binding *Binding `json:"binding,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

// Reader loads and verifies bindings against the same immutable snapshot used by a run.
type Reader interface {
	ListSnapshot(context.Context, SnapshotRef) ([]Binding, error)
	Verify(context.Context, SnapshotRef, Match) (Decision, error)
}

func ValidateSnapshotRef(ref SnapshotRef) (SnapshotRef, error) {
	subject, err := runtimeidentity.ValidateSubjectRef(ref.Subject)
	if err != nil {
		return SnapshotRef{}, fmt.Errorf("%w: subject: %v", ErrInvalidReference, err)
	}
	ref.Subject = subject
	switch ref.Phase {
	case PhaseDraft:
		if ref.VersionID != nil && *ref.VersionID == uuid.Nil {
			return SnapshotRef{}, fmt.Errorf("%w: version_id cannot be nil UUID", ErrInvalidReference)
		}
	case PhasePublished:
		if ref.VersionID == nil || *ref.VersionID == uuid.Nil {
			return SnapshotRef{}, fmt.Errorf("%w: published snapshot requires version_id", ErrInvalidReference)
		}
	default:
		return SnapshotRef{}, fmt.Errorf("%w: unsupported phase %q", ErrInvalidReference, ref.Phase)
	}
	return ref, nil
}

func NormalizeMatch(match Match) (Match, error) {
	match.Type = BindingType(strings.ToLower(strings.TrimSpace(string(match.Type))))
	match.ResourceID = strings.TrimSpace(match.ResourceID)
	match.ParentResourceID = strings.TrimSpace(match.ParentResourceID)
	match.AccessMode = AccessMode(strings.ToLower(strings.TrimSpace(string(match.AccessMode))))
	match.ActionID = strings.ToLower(strings.TrimSpace(match.ActionID))
	if match.Type == "" || match.ResourceID == "" {
		return Match{}, fmt.Errorf("%w: type and resource_id are required", ErrInvalidReference)
	}
	switch match.AccessMode {
	case AccessModeRead, AccessModeWrite, AccessModeExecute:
	default:
		return Match{}, fmt.Errorf("%w: unsupported access_mode %q", ErrInvalidReference, match.AccessMode)
	}
	if match.ActionID != "" && match.Type != BindingTypeIntegrationConnection {
		return Match{}, fmt.Errorf("%w: action_id is only valid for integration connections", ErrInvalidReference)
	}
	return match, nil
}

func AllowsAccess(granted, requested AccessMode) bool {
	if requested == AccessModeRead {
		return granted == AccessModeRead || granted == AccessModeWrite
	}
	return granted == requested
}

func AllowedIntegrationActions(metadata map[string]interface{}) []string {
	raw, ok := metadata[IntegrationAllowedActionIDsMetadataKey]
	if !ok {
		return nil
	}
	values := make([]string, 0)
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []interface{}:
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func AllowsIntegrationAction(metadata map[string]interface{}, actionID string) bool {
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	if actionID == "" {
		return false
	}
	for _, allowed := range AllowedIntegrationActions(metadata) {
		if allowed == actionID {
			return true
		}
	}
	return false
}

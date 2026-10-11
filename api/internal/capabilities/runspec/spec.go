// Package runspec defines immutable, replayable runtime configuration snapshots.
package runspec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

const (
	// SchemaVersionV1 is the first immutable runtime snapshot contract.
	SchemaVersionV1 = "run_spec.v1"
	revisionPrefix  = "sha256:"
)

// ErrInvalidSpec indicates that a runtime snapshot is incomplete or inconsistent.
var ErrInvalidSpec = errors.New("invalid run spec")

// AccessMode describes the operation a subject may perform through a binding.
type AccessMode string

const (
	// AccessModeRead permits read-only resource access.
	AccessModeRead AccessMode = "read"
	// AccessModeWrite permits resource mutation.
	AccessModeWrite AccessMode = "write"
	// AccessModeExecute permits invoking an executable resource.
	AccessModeExecute AccessMode = "execute"
)

// VersionRef pins a role, capability, SOP, policy, or evaluation definition.
type VersionRef struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Version  string `json:"version,omitempty"`
	Revision string `json:"revision,omitempty"`
}

// BindingRef pins a resource binding. CredentialRef is an opaque reference;
// credentials and secret material must never be embedded in a RunSpec.
type BindingRef struct {
	Type             string     `json:"type"`
	ResourceID       string     `json:"resource_id"`
	ParentResourceID string     `json:"parent_resource_id,omitempty"`
	Revision         string     `json:"revision"`
	AccessMode       AccessMode `json:"access_mode"`
	CredentialRef    string     `json:"credential_ref,omitempty"`
}

// Budget limits the amount of work one runtime execution may perform.
type Budget struct {
	MaxDurationSeconds int `json:"max_duration_seconds,omitempty"`
	MaxModelTokens     int `json:"max_model_tokens,omitempty"`
	MaxToolCalls       int `json:"max_tool_calls,omitempty"`
	MaxExternalActions int `json:"max_external_actions,omitempty"`
}

// Spec is the complete configuration snapshot used for one execution.
// Once sealed, Revision changes whenever any execution-relevant field changes.
type Spec struct {
	SchemaVersion string                     `json:"schema_version"`
	Revision      string                     `json:"revision"`
	InvocationID  string                     `json:"invocation_id"`
	Subject       runtimeidentity.SubjectRef `json:"subject"`
	Identity      VersionRef                 `json:"identity"`
	Model         VersionRef                 `json:"model"`
	Roles         []VersionRef               `json:"roles,omitempty"`
	Capabilities  []VersionRef               `json:"capabilities,omitempty"`
	SOPs          []VersionRef               `json:"sops,omitempty"`
	Bindings      []BindingRef               `json:"bindings,omitempty"`
	Policies      []VersionRef               `json:"policies,omitempty"`
	Evaluations   []VersionRef               `json:"evaluations,omitempty"`
	Budget        Budget                     `json:"budget"`
	ResolvedAt    time.Time                  `json:"resolved_at"`
}

// Seal normalizes, validates, orders, and hashes a snapshot. If a revision is
// already present, Seal verifies that the snapshot still matches it.
func Seal(spec Spec) (Spec, error) {
	providedRevision := strings.TrimSpace(spec.Revision)
	spec.Revision = ""
	spec.SchemaVersion = strings.TrimSpace(spec.SchemaVersion)
	if spec.SchemaVersion == "" {
		spec.SchemaVersion = SchemaVersionV1
	}
	if spec.SchemaVersion != SchemaVersionV1 {
		return Spec{}, fmt.Errorf("%w: unsupported schema_version %q", ErrInvalidSpec, spec.SchemaVersion)
	}
	spec.InvocationID = strings.TrimSpace(spec.InvocationID)
	if spec.InvocationID == "" {
		return Spec{}, fmt.Errorf("%w: invocation_id is required", ErrInvalidSpec)
	}

	var err error
	spec.Subject, err = runtimeidentity.ValidateSubjectRef(spec.Subject)
	if err != nil {
		return Spec{}, fmt.Errorf("%w: subject: %v", ErrInvalidSpec, err)
	}
	spec.Identity, err = normalizeVersionRef(spec.Identity)
	if err != nil {
		return Spec{}, fmt.Errorf("%w: identity: %v", ErrInvalidSpec, err)
	}
	if spec.Identity.Kind != "identity" {
		return Spec{}, fmt.Errorf("%w: identity.kind must be identity", ErrInvalidSpec)
	}
	spec.Model, err = normalizeVersionRef(spec.Model)
	if err != nil {
		return Spec{}, fmt.Errorf("%w: model: %v", ErrInvalidSpec, err)
	}
	if spec.Model.Kind != "model" {
		return Spec{}, fmt.Errorf("%w: model.kind must be model", ErrInvalidSpec)
	}

	if spec.Roles, err = normalizeVersionRefs("roles", spec.Roles); err != nil {
		return Spec{}, err
	}
	if spec.Capabilities, err = normalizeVersionRefs("capabilities", spec.Capabilities); err != nil {
		return Spec{}, err
	}
	if spec.SOPs, err = normalizeVersionRefs("sops", spec.SOPs); err != nil {
		return Spec{}, err
	}
	if spec.Policies, err = normalizeVersionRefs("policies", spec.Policies); err != nil {
		return Spec{}, err
	}
	if spec.Evaluations, err = normalizeVersionRefs("evaluations", spec.Evaluations); err != nil {
		return Spec{}, err
	}
	if spec.Bindings, err = normalizeBindings(spec.Bindings); err != nil {
		return Spec{}, err
	}
	if err := validateBudget(spec.Budget); err != nil {
		return Spec{}, err
	}
	if spec.ResolvedAt.IsZero() {
		return Spec{}, fmt.Errorf("%w: resolved_at is required", ErrInvalidSpec)
	}
	spec.ResolvedAt = spec.ResolvedAt.UTC()

	encoded, err := json.Marshal(spec)
	if err != nil {
		return Spec{}, fmt.Errorf("%w: encode snapshot: %v", ErrInvalidSpec, err)
	}
	digest := sha256.Sum256(encoded)
	spec.Revision = revisionPrefix + hex.EncodeToString(digest[:])
	if providedRevision != "" && providedRevision != spec.Revision {
		return Spec{}, fmt.Errorf("%w: revision does not match snapshot", ErrInvalidSpec)
	}
	return spec, nil
}

func normalizeVersionRefs(field string, refs []VersionRef) ([]VersionRef, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	normalized := make([]VersionRef, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for i, ref := range refs {
		var err error
		normalized[i], err = normalizeVersionRef(ref)
		if err != nil {
			return nil, fmt.Errorf("%w: %s[%d]: %v", ErrInvalidSpec, field, i, err)
		}
		key := versionRefKey(normalized[i])
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: duplicate %s reference %q", ErrInvalidSpec, field, key)
		}
		seen[key] = struct{}{}
	}
	slices.SortFunc(normalized, func(a, b VersionRef) int {
		return strings.Compare(versionRefKey(a), versionRefKey(b))
	})
	return normalized, nil
}

func normalizeVersionRef(ref VersionRef) (VersionRef, error) {
	ref.Kind = strings.ToLower(strings.TrimSpace(ref.Kind))
	ref.ID = strings.TrimSpace(ref.ID)
	ref.Version = strings.TrimSpace(ref.Version)
	ref.Revision = strings.TrimSpace(ref.Revision)
	if ref.Kind == "" {
		return VersionRef{}, errors.New("kind is required")
	}
	if ref.ID == "" {
		return VersionRef{}, errors.New("id is required")
	}
	if ref.Version == "" && ref.Revision == "" {
		return VersionRef{}, errors.New("version or revision is required")
	}
	return ref, nil
}

func versionRefKey(ref VersionRef) string {
	return strings.Join([]string{ref.Kind, ref.ID, ref.Version, ref.Revision}, "\x00")
}

func normalizeBindings(bindings []BindingRef) ([]BindingRef, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	normalized := make([]BindingRef, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for i, binding := range bindings {
		binding.Type = strings.ToLower(strings.TrimSpace(binding.Type))
		binding.ResourceID = strings.TrimSpace(binding.ResourceID)
		binding.ParentResourceID = strings.TrimSpace(binding.ParentResourceID)
		binding.Revision = strings.TrimSpace(binding.Revision)
		binding.AccessMode = AccessMode(strings.ToLower(strings.TrimSpace(string(binding.AccessMode))))
		binding.CredentialRef = strings.TrimSpace(binding.CredentialRef)
		if binding.Type == "" || binding.ResourceID == "" || binding.Revision == "" {
			return nil, fmt.Errorf("%w: bindings[%d] requires type, resource_id, and revision", ErrInvalidSpec, i)
		}
		switch binding.AccessMode {
		case AccessModeRead, AccessModeWrite, AccessModeExecute:
		default:
			return nil, fmt.Errorf("%w: bindings[%d] has unsupported access_mode %q", ErrInvalidSpec, i, binding.AccessMode)
		}
		key := bindingKey(binding)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: duplicate binding reference %q", ErrInvalidSpec, key)
		}
		seen[key] = struct{}{}
		normalized[i] = binding
	}
	slices.SortFunc(normalized, func(a, b BindingRef) int {
		return strings.Compare(bindingKey(a), bindingKey(b))
	})
	return normalized, nil
}

func bindingKey(binding BindingRef) string {
	return strings.Join([]string{
		binding.Type,
		binding.ResourceID,
		binding.ParentResourceID,
		binding.Revision,
		string(binding.AccessMode),
		binding.CredentialRef,
	}, "\x00")
}

func validateBudget(budget Budget) error {
	if budget.MaxDurationSeconds < 0 || budget.MaxModelTokens < 0 || budget.MaxToolCalls < 0 || budget.MaxExternalActions < 0 {
		return fmt.Errorf("%w: budget values cannot be negative", ErrInvalidSpec)
	}
	return nil
}

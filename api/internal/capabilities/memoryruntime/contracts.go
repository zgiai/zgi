// Package memoryruntime defines the subject-owned memory adapter contract.
package memoryruntime

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

// ErrInvalidContract indicates that a memory adapter contract violates its
// ownership, scope, fencing, or evidence invariants.
var ErrInvalidContract = errors.New("invalid memory runtime contract")

// MemoryType describes the semantic purpose of one memory item.
type MemoryType string

const (
	// MemoryTypeIdentity describes the DigitalHuman's stable self-understanding.
	MemoryTypeIdentity MemoryType = "identity"
	// MemoryTypeExperience records durable lessons from prior work.
	MemoryTypeExperience MemoryType = "experience"
	// MemoryTypeDecision records durable decisions and their context.
	MemoryTypeDecision MemoryType = "decision"
	// MemoryTypeFact records evidence-backed durable facts.
	MemoryTypeFact MemoryType = "fact"
	// MemoryTypeOpenLoop records unfinished commitments or follow-up work.
	MemoryTypeOpenLoop MemoryType = "open_loop"
	// MemoryTypeReflection records synthesized observations about prior work.
	MemoryTypeReflection MemoryType = "reflection"
)

// MutationAction describes the requested state transition.
type MutationAction string

const (
	// MutationActionUpsert creates or replaces a memory value.
	MutationActionUpsert MutationAction = "upsert"
	// MutationActionClear removes a memory value through the adapter.
	MutationActionClear MutationAction = "clear"
)

// EvidenceType identifies the durable record supporting a memory mutation.
type EvidenceType string

const (
	// EvidenceTypeRun references a runtime execution.
	EvidenceTypeRun EvidenceType = "run"
	// EvidenceTypeAction references an action ledger entry.
	EvidenceTypeAction EvidenceType = "action"
	// EvidenceTypeTool references a recorded tool invocation.
	EvidenceTypeTool EvidenceType = "tool"
	// EvidenceTypeArtifact references a durable produced artifact.
	EvidenceTypeArtifact EvidenceType = "artifact"
	// EvidenceTypeMessage references a durable inbound or outbound message.
	EvidenceTypeMessage EvidenceType = "message"
	// EvidenceTypeManual references a human-authored memory change.
	EvidenceTypeManual EvidenceType = "manual"
)

// MemorySubjectRef binds memory to exactly one DigitalHuman. It deliberately
// contains no user identifier or user-specific partition.
type MemorySubjectRef struct {
	DigitalHumanID uuid.UUID `json:"digital_human_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	WorkspaceID    uuid.UUID `json:"workspace_id"`
}

// MemorySubjectFromRuntime converts a generic runtime owner into the narrower
// memory owner contract. Agent subjects are rejected.
func MemorySubjectFromRuntime(subject runtimeidentity.SubjectRef) (MemorySubjectRef, error) {
	subject, err := runtimeidentity.ValidateSubjectRef(subject)
	if err != nil {
		return MemorySubjectRef{}, fmt.Errorf("%w: subject: %v", ErrInvalidContract, err)
	}
	if subject.Type != runtimeidentity.SubjectTypeDigitalHuman {
		return MemorySubjectRef{}, fmt.Errorf("%w: memory subject must be digital_human", ErrInvalidContract)
	}
	return MemorySubjectRef{
		DigitalHumanID: subject.ID,
		OrganizationID: subject.OrganizationID,
		WorkspaceID:    subject.WorkspaceID,
	}, nil
}

// MemoryFence prevents a stale run from writing after configuration or memory
// ownership has advanced.
type MemoryFence struct {
	ConfigRevision string `json:"config_revision"`
	MemoryEpoch    int64  `json:"memory_epoch"`
}

// MemoryEvidence points to a durable source that supports a mutation.
type MemoryEvidence struct {
	Type   EvidenceType `json:"type"`
	ID     string       `json:"id"`
	Digest string       `json:"digest,omitempty"`
}

// MemoryMutation is an idempotent, fenced request to update subject-owned
// memory. SourcePrincipalID is provenance only and must never form a storage or
// retrieval partition.
type MemoryMutation struct {
	OperationID uuid.UUID        `json:"operation_id"`
	Subject     MemorySubjectRef `json:"subject"`
	// Key is stable within the owning DigitalHuman (ADR 0015: it has one
	// memory). It is never derived from the initiating user.
	Key               string           `json:"key"`
	Action            MutationAction   `json:"action"`
	Type              MemoryType       `json:"memory_type"`
	Content           string           `json:"content,omitempty"`
	ExpectedRevision  int64            `json:"expected_revision"`
	Fence             MemoryFence      `json:"fence"`
	Evidence          []MemoryEvidence `json:"evidence,omitempty"`
	SourcePrincipalID string           `json:"source_principal_id,omitempty"`
	OccurredAt        time.Time        `json:"occurred_at"`
}

// ValidateMemorySubjectRef validates the fixed DigitalHuman memory owner.
func ValidateMemorySubjectRef(subject MemorySubjectRef) (MemorySubjectRef, error) {
	if subject.DigitalHumanID == uuid.Nil {
		return MemorySubjectRef{}, fmt.Errorf("%w: subject.digital_human_id is required", ErrInvalidContract)
	}
	if subject.OrganizationID == uuid.Nil {
		return MemorySubjectRef{}, fmt.Errorf("%w: subject.organization_id is required", ErrInvalidContract)
	}
	if subject.WorkspaceID == uuid.Nil {
		return MemorySubjectRef{}, fmt.Errorf("%w: subject.workspace_id is required", ErrInvalidContract)
	}
	return subject, nil
}

// ValidateMemoryFence validates stale-write protection values.
func ValidateMemoryFence(fence MemoryFence) (MemoryFence, error) {
	fence.ConfigRevision = strings.TrimSpace(fence.ConfigRevision)
	if fence.ConfigRevision == "" {
		return MemoryFence{}, fmt.Errorf("%w: fence.config_revision is required", ErrInvalidContract)
	}
	if fence.MemoryEpoch < 0 {
		return MemoryFence{}, fmt.Errorf("%w: fence.memory_epoch cannot be negative", ErrInvalidContract)
	}
	return fence, nil
}

// ValidateMemoryMutation normalizes and validates a mutation and returns copies
// of pointer and slice fields owned by the returned value.
func ValidateMemoryMutation(mutation MemoryMutation) (MemoryMutation, error) {
	if mutation.OperationID == uuid.Nil {
		return MemoryMutation{}, fmt.Errorf("%w: operation_id is required", ErrInvalidContract)
	}
	var err error
	mutation.Subject, err = ValidateMemorySubjectRef(mutation.Subject)
	if err != nil {
		return MemoryMutation{}, err
	}
	mutation.Action = MutationAction(strings.ToLower(strings.TrimSpace(string(mutation.Action))))
	switch mutation.Action {
	case MutationActionUpsert, MutationActionClear:
	default:
		return MemoryMutation{}, fmt.Errorf("%w: unsupported action %q", ErrInvalidContract, mutation.Action)
	}
	mutation.Key = strings.ToLower(strings.TrimSpace(mutation.Key))
	if mutation.Key == "" || len(mutation.Key) > 160 {
		return MemoryMutation{}, fmt.Errorf("%w: key is required and must not exceed 160 bytes", ErrInvalidContract)
	}
	mutation.Type = MemoryType(strings.ToLower(strings.TrimSpace(string(mutation.Type))))
	if !validMemoryType(mutation.Type) {
		return MemoryMutation{}, fmt.Errorf("%w: unsupported memory_type %q", ErrInvalidContract, mutation.Type)
	}
	mutation.Content = strings.TrimSpace(mutation.Content)
	if mutation.Action == MutationActionUpsert && mutation.Content == "" {
		return MemoryMutation{}, fmt.Errorf("%w: upsert content is required", ErrInvalidContract)
	}
	if mutation.Action == MutationActionClear && mutation.Content != "" {
		return MemoryMutation{}, fmt.Errorf("%w: clear content must be empty", ErrInvalidContract)
	}
	if mutation.ExpectedRevision < 0 {
		return MemoryMutation{}, fmt.Errorf("%w: expected_revision cannot be negative", ErrInvalidContract)
	}
	mutation.Fence, err = ValidateMemoryFence(mutation.Fence)
	if err != nil {
		return MemoryMutation{}, err
	}
	mutation.Evidence, err = normalizeEvidence(mutation.Evidence)
	if err != nil {
		return MemoryMutation{}, err
	}
	if mutation.Action == MutationActionUpsert && len(mutation.Evidence) == 0 {
		return MemoryMutation{}, fmt.Errorf("%w: upsert requires evidence", ErrInvalidContract)
	}
	mutation.SourcePrincipalID = strings.TrimSpace(mutation.SourcePrincipalID)
	if mutation.OccurredAt.IsZero() {
		return MemoryMutation{}, fmt.Errorf("%w: occurred_at is required", ErrInvalidContract)
	}
	mutation.OccurredAt = mutation.OccurredAt.UTC()
	return mutation, nil
}

func validMemoryType(memoryType MemoryType) bool {
	switch memoryType {
	case MemoryTypeIdentity, MemoryTypeExperience, MemoryTypeDecision, MemoryTypeFact, MemoryTypeOpenLoop, MemoryTypeReflection:
		return true
	default:
		return false
	}
}

func normalizeEvidence(evidence []MemoryEvidence) ([]MemoryEvidence, error) {
	if len(evidence) == 0 {
		return nil, nil
	}
	normalized := make([]MemoryEvidence, len(evidence))
	seen := make(map[string]struct{}, len(evidence))
	for i, item := range evidence {
		item.Type = EvidenceType(strings.ToLower(strings.TrimSpace(string(item.Type))))
		item.ID = strings.TrimSpace(item.ID)
		item.Digest = strings.TrimSpace(item.Digest)
		switch item.Type {
		case EvidenceTypeRun, EvidenceTypeAction, EvidenceTypeTool, EvidenceTypeArtifact, EvidenceTypeMessage, EvidenceTypeManual:
		default:
			return nil, fmt.Errorf("%w: evidence[%d] has unsupported type %q", ErrInvalidContract, i, item.Type)
		}
		if item.ID == "" {
			return nil, fmt.Errorf("%w: evidence[%d].id is required", ErrInvalidContract, i)
		}
		key := strings.Join([]string{string(item.Type), item.ID, item.Digest}, "\x00")
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("%w: duplicate evidence %q", ErrInvalidContract, item.ID)
		}
		seen[key] = struct{}{}
		normalized[i] = item
	}
	slices.SortFunc(normalized, func(a, b MemoryEvidence) int {
		return strings.Compare(string(a.Type)+"\x00"+a.ID+"\x00"+a.Digest, string(b.Type)+"\x00"+b.ID+"\x00"+b.Digest)
	})
	return normalized, nil
}

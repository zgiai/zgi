package memoryruntime

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

func TestMemorySubjectFromRuntimeOnlyAcceptsDigitalHuman(t *testing.T) {
	t.Parallel()

	subject := validRuntimeSubject()
	got, err := MemorySubjectFromRuntime(subject)
	if err != nil {
		t.Fatalf("MemorySubjectFromRuntime() error = %v", err)
	}
	if got.DigitalHumanID != subject.ID || got.OrganizationID != subject.OrganizationID || got.WorkspaceID != subject.WorkspaceID {
		t.Fatalf("MemorySubjectFromRuntime() = %#v", got)
	}

	subject.Type = runtimeidentity.SubjectTypeAgent
	if _, err := MemorySubjectFromRuntime(subject); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("MemorySubjectFromRuntime(agent) error = %v, want ErrInvalidContract", err)
	}
}

func TestValidateMemoryMutationJSONRoundTrip(t *testing.T) {
	t.Parallel()

	want, err := ValidateMemoryMutation(validMutation())
	if err != nil {
		t.Fatalf("ValidateMemoryMutation() error = %v", err)
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	serialized := string(encoded)
	if strings.Contains(serialized, "user_id") || strings.Contains(serialized, "user_scope") {
		t.Fatalf("memory contract contains a user partition: %s", serialized)
	}
	var decoded MemoryMutation
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	got, err := ValidateMemoryMutation(decoded)
	if err != nil {
		t.Fatalf("ValidateMemoryMutation(decoded) error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestValidateMemoryMutationEnforcesOwnershipAndFences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*MemoryMutation)
	}{
		{name: "operation", mutate: func(mutation *MemoryMutation) { mutation.OperationID = uuid.Nil }},
		{name: "key", mutate: func(mutation *MemoryMutation) { mutation.Key = " " }},
		{name: "subject", mutate: func(mutation *MemoryMutation) { mutation.Subject.DigitalHumanID = uuid.Nil }},
		{name: "action", mutate: func(mutation *MemoryMutation) { mutation.Action = "append" }},
		{name: "content", mutate: func(mutation *MemoryMutation) { mutation.Content = "" }},
		{name: "revision", mutate: func(mutation *MemoryMutation) { mutation.ExpectedRevision = -1 }},
		{name: "config fence", mutate: func(mutation *MemoryMutation) { mutation.Fence.ConfigRevision = "" }},
		{name: "memory epoch", mutate: func(mutation *MemoryMutation) { mutation.Fence.MemoryEpoch = -1 }},
		{name: "evidence", mutate: func(mutation *MemoryMutation) { mutation.Evidence = nil }},
		{name: "occurred time", mutate: func(mutation *MemoryMutation) { mutation.OccurredAt = time.Time{} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutation := validMutation()
			tt.mutate(&mutation)
			if _, err := ValidateMemoryMutation(mutation); !errors.Is(err, ErrInvalidContract) {
				t.Fatalf("ValidateMemoryMutation() error = %v, want ErrInvalidContract", err)
			}
		})
	}
}

func TestValidateMemoryMutationClearRequiresEmptyContent(t *testing.T) {
	t.Parallel()

	mutation := validMutation()
	mutation.Action = MutationActionClear
	if _, err := ValidateMemoryMutation(mutation); !errors.Is(err, ErrInvalidContract) {
		t.Fatalf("ValidateMemoryMutation(clear with content) error = %v, want ErrInvalidContract", err)
	}
	mutation.Content = ""
	mutation.Evidence = nil
	if _, err := ValidateMemoryMutation(mutation); err != nil {
		t.Fatalf("ValidateMemoryMutation(clear) error = %v", err)
	}
}

func validMutation() MemoryMutation {
	return MemoryMutation{
		OperationID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		Subject: MemorySubjectRef{
			DigitalHumanID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			OrganizationID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			WorkspaceID:    uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		},
		Key:              "project.launch-date",
		Action:           MutationActionUpsert,
		Type:             MemoryTypeFact,
		Content:          "The launch date is September 8.",
		ExpectedRevision: 7,
		Fence:            MemoryFence{ConfigRevision: "sha256:run-spec", MemoryEpoch: 3},
		Evidence: []MemoryEvidence{
			{Type: EvidenceTypeMessage, ID: "message-9", Digest: "sha256:evidence"},
		},
		SourcePrincipalID: "external-user-5",
		OccurredAt:        time.Date(2026, time.August, 18, 3, 4, 5, 0, time.FixedZone("CST", 8*60*60)),
	}
}

func validRuntimeSubject() runtimeidentity.SubjectRef {
	return runtimeidentity.SubjectRef{
		Type:           runtimeidentity.SubjectTypeDigitalHuman,
		ID:             uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		OrganizationID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		WorkspaceID:    uuid.MustParse("33333333-3333-3333-3333-333333333333"),
	}
}

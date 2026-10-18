package runtimeidentity

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateSubjectRefNormalizesAndRoundTrips(t *testing.T) {
	want := SubjectRef{
		Type:           SubjectTypeDigitalHuman,
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		WorkspaceID:    uuid.New(),
	}
	got, err := ValidateSubjectRef(SubjectRef{
		Type:           " DIGITAL_HUMAN ",
		ID:             want.ID,
		OrganizationID: want.OrganizationID,
		WorkspaceID:    want.WorkspaceID,
	})
	if err != nil {
		t.Fatalf("ValidateSubjectRef() error = %v", err)
	}
	if got != want {
		t.Fatalf("ValidateSubjectRef() = %#v, want %#v", got, want)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded SubjectRef
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded != want {
		t.Fatalf("JSON round trip = %#v, want %#v", decoded, want)
	}
}

func TestValidateSubjectRefRejectsInvalidReferences(t *testing.T) {
	valid := SubjectRef{
		Type:           SubjectTypeDigitalHuman,
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		WorkspaceID:    uuid.New(),
	}
	tests := []struct {
		name string
		ref  SubjectRef
	}{
		{name: "unsupported type", ref: withSubjectType(valid, "workflow")},
		{name: "missing id", ref: withSubjectID(valid, uuid.Nil)},
		{name: "missing organization", ref: withSubjectOrganizationID(valid, uuid.Nil)},
		{name: "missing workspace", ref: withSubjectWorkspaceID(valid, uuid.Nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateSubjectRef(tt.ref)
			if !errors.Is(err, ErrInvalidSubject) {
				t.Fatalf("ValidateSubjectRef() error = %v, want ErrInvalidSubject", err)
			}
		})
	}
}

func withSubjectType(ref SubjectRef, subjectType SubjectType) SubjectRef {
	ref.Type = subjectType
	return ref
}

func withSubjectID(ref SubjectRef, id uuid.UUID) SubjectRef {
	ref.ID = id
	return ref
}

func withSubjectOrganizationID(ref SubjectRef, id uuid.UUID) SubjectRef {
	ref.OrganizationID = id
	return ref
}

func withSubjectWorkspaceID(ref SubjectRef, id uuid.UUID) SubjectRef {
	ref.WorkspaceID = id
	return ref
}

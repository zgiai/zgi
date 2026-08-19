// Package runtimeidentity defines the stable identity of a runtime-owned subject.
package runtimeidentity

import (
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrInvalidSubject indicates that a runtime subject reference is incomplete
// or uses an unsupported subject type.
var ErrInvalidSubject = errors.New("invalid runtime subject")

// SubjectType identifies the domain object that owns a runtime execution.
type SubjectType string

const (
	// SubjectTypeAgent identifies an existing Agent runtime owner.
	SubjectTypeAgent SubjectType = "agent"
	// SubjectTypeDigitalHuman identifies an independent DigitalHuman runtime owner.
	SubjectTypeDigitalHuman SubjectType = "digital_human"
)

// SubjectRef identifies a runtime subject inside one organization and workspace.
// All identifiers are required so downstream authorization never has to infer scope.
type SubjectRef struct {
	Type           SubjectType `json:"type"`
	ID             uuid.UUID   `json:"id"`
	OrganizationID uuid.UUID   `json:"organization_id"`
	WorkspaceID    uuid.UUID   `json:"workspace_id"`
}

// ValidateSubjectRef normalizes and validates a runtime subject reference.
func ValidateSubjectRef(ref SubjectRef) (SubjectRef, error) {
	ref.Type = SubjectType(strings.ToLower(strings.TrimSpace(string(ref.Type))))
	switch ref.Type {
	case SubjectTypeAgent, SubjectTypeDigitalHuman:
	default:
		return SubjectRef{}, fmt.Errorf("%w: unsupported type %q", ErrInvalidSubject, ref.Type)
	}
	if ref.ID == uuid.Nil {
		return SubjectRef{}, fmt.Errorf("%w: id is required", ErrInvalidSubject)
	}
	if ref.OrganizationID == uuid.Nil {
		return SubjectRef{}, fmt.Errorf("%w: organization_id is required", ErrInvalidSubject)
	}
	if ref.WorkspaceID == uuid.Nil {
		return SubjectRef{}, fmt.Errorf("%w: workspace_id is required", ErrInvalidSubject)
	}
	return ref, nil
}

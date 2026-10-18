package runtimeworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	AccessEventResultSuccess = "success"
	AccessEventResultFailure = "failure"
)

// AccessEventInput is the request-bound portion of an audit event. Keeping
// this separate from the persisted model makes it harder for handlers to
// accidentally pass secrets or request content into the audit table.
type AccessEventInput struct {
	RuntimeWorkspaceID uuid.UUID
	OrganizationID     uuid.UUID
	WorkspaceID        uuid.UUID
	CredentialID       uuid.UUID
	SubjectID          uuid.UUID
	AgentID            uuid.UUID
	Method             string
	Route              string
	StatusCode         int
	Result             string
	Details            map[string]any
}

// RecordAccessEvent persists an authenticated Runtime Workspace request. The
// audit path is deliberately best-effort at the HTTP boundary: a database
// outage must not turn an already completed chat/file/memory operation into a
// second failure, while the error is still returned to the caller for logging
// and metrics.
func accessEventResult(input AccessEventInput) (string, error) {
	result := strings.TrimSpace(input.Result)
	if result == "" {
		result = AccessEventResultSuccess
		if input.StatusCode >= 400 {
			result = AccessEventResultFailure
		}
	}
	if result != AccessEventResultSuccess && result != AccessEventResultFailure {
		return "", errors.New("invalid runtime workspace access event result")
	}
	return result, nil
}

func (s *Service) RecordAccessEvent(ctx context.Context, input AccessEventInput) error {
	if s == nil || s.db == nil || input.RuntimeWorkspaceID == uuid.Nil || input.OrganizationID == uuid.Nil ||
		input.WorkspaceID == uuid.Nil || strings.TrimSpace(input.Method) == "" || strings.TrimSpace(input.Route) == "" || input.StatusCode <= 0 {
		return errors.New("invalid runtime workspace access event")
	}

	details := datatypes.JSON([]byte("{}"))
	if input.Details != nil {
		encoded, err := json.Marshal(input.Details)
		if err != nil {
			return err
		}
		details = datatypes.JSON(encoded)
	}
	result, err := accessEventResult(input)
	if err != nil {
		return err
	}
	event := &AccessEvent{
		ID: uuid.New(), RuntimeWorkspaceID: input.RuntimeWorkspaceID, OrganizationID: input.OrganizationID,
		WorkspaceID: input.WorkspaceID, Method: strings.ToUpper(strings.TrimSpace(input.Method)),
		Route: strings.TrimSpace(input.Route), StatusCode: input.StatusCode, Result: result,
		Details: details, CreatedAt: time.Now().UTC(),
	}
	if input.CredentialID != uuid.Nil {
		event.CredentialID = &input.CredentialID
	}
	if input.SubjectID != uuid.Nil {
		event.SubjectID = &input.SubjectID
	}
	if input.AgentID != uuid.Nil {
		event.AgentID = &input.AgentID
	}
	return s.db.WithContext(ctx).Create(event).Error
}

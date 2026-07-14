package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteSkillRejectsUnknownAgentBindingAction(t *testing.T) {
	svc := &service{}
	err := svc.DeleteSkill(context.Background(), Scope{
		OrganizationID:  uuid.New(),
		AccountID:       uuid.New(),
		SkipAccessCheck: true,
	}, "custom-skill", "approve", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("DeleteSkill() error = %v, want ErrInvalidInput", err)
	}
}

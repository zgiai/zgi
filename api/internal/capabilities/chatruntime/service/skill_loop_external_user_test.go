package service

import (
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestSkillExecutionContextCarriesExternalUserWithoutReplacingAccount(t *testing.T) {
	accountID := uuid.New()
	externalUserID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		Caller:       Caller{Type: runtimemodel.ConversationCallerAgent},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      accountID,
			ExternalUserID: &externalUserID,
		},
		RunConfig: RunConfig{},
	}

	execution := (&service{}).skillExecutionContext(prepared)
	if execution.UserID != accountID.String() {
		t.Fatalf("execution UserID = %q, want internal account %s", execution.UserID, accountID)
	}
	if execution.ExternalUserID != externalUserID.String() {
		t.Fatalf("execution ExternalUserID = %q, want %s", execution.ExternalUserID, externalUserID)
	}
	if got := execution.RuntimeParameters[tools.ExternalUserIDRuntimeParameter]; got != externalUserID.String() {
		t.Fatalf("runtime external user = %#v, want %s", got, externalUserID)
	}
}

func TestSkillExecutionContextOmitsExternalUserForInternalCaller(t *testing.T) {
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New()},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		RunConfig: RunConfig{},
	}

	execution := (&service{}).skillExecutionContext(prepared)
	if execution.ExternalUserID != "" {
		t.Fatalf("execution ExternalUserID = %q, want empty", execution.ExternalUserID)
	}
	if _, exists := execution.RuntimeParameters[tools.ExternalUserIDRuntimeParameter]; exists {
		t.Fatalf("runtime external user = %#v, want omitted", execution.RuntimeParameters[tools.ExternalUserIDRuntimeParameter])
	}
}

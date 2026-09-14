package agents

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestResolveExternalUserID(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		legacy     string
		want       string
		wantLegacy bool
		wantErr    error
	}{
		{name: "header", header: " user-1 ", want: "user-1"},
		{name: "legacy", legacy: " user-1 ", want: "user-1", wantLegacy: true},
		{name: "matching values", header: "user-1", legacy: "user-1", want: "user-1"},
		{name: "missing", wantErr: errExternalUserRequired},
		{name: "mismatch", header: "user-1", legacy: "user-2", wantErr: errExternalUserMismatch},
		{name: "control character", header: "user\t1", wantErr: errExternalUserInvalid},
		{name: "too long", header: strings.Repeat("u", maxExternalUserIDRunes+1), wantErr: errExternalUserInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, usedLegacy, err := ResolveExternalUserID(test.header, test.legacy)
			if err != test.wantErr {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if got != test.want || usedLegacy != test.wantLegacy {
				t.Fatalf("result = %q/%v, want %q/%v", got, usedLegacy, test.want, test.wantLegacy)
			}
		})
	}
}

func TestDeriveExternalUserIDDoesNotPersistRawIdentity(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()
	userID := "customer@example.com"

	derived := DeriveExternalUserID(workspaceID, agentID, userID)

	if derived == uuid.Nil || derived.String() == userID {
		t.Fatalf("derived id = %q, want a non-zero opaque UUID", derived)
	}
	if derived != DeriveExternalUserID(workspaceID, agentID, userID) {
		t.Fatal("derived external user id is not stable")
	}
}

func TestAPIKeyConversationManagementRequiresExternalUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{}
	handler := NewAgentsHandler(nil, nil, nil, nil, nil, runtimeSvc)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/conversations/"+ids.conversationID.String()+"/stop", nil)
	c.Params = gin.Params{{Key: "conversation_id", Value: ids.conversationID.String()}}
	c.Set("organization_id", ids.organizationID.String())
	c.Set("workspace_id", ids.workspaceID.String())
	c.Set("account_id", ids.accountID.String())
	c.Set("agent_id", ids.agentID.String())

	handler.StopAPIKeyAgentRuntimeConversation(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if runtimeSvc.getConversationCalled || runtimeSvc.stopConversationCalled {
		t.Fatal("conversation service must not run without an external user identity")
	}
}

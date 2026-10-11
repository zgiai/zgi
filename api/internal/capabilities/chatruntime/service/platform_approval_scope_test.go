package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestPlatformApprovalScopeFailsClosedAndRejectSurvivesAdmissionWithdrawal(t *testing.T) {
	for _, appType := range []string{"agent", "digital_human"} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			t.Run(appType+"/"+entry, func(t *testing.T) {
				org, ws, app, owner, cid, mid := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
				o := toolgovernance.ExecutionOrigin{OrganizationID: org.String(), WorkspaceID: ws.String(), ApplicationType: appType, ApplicationID: app.String(),
					Entry: entry, SubjectKind: "account", SubjectID: owner.String(), ConversationID: cid.String(), MessageID: mid.String(), PublicationRef: uuid.NewString()}
				p := &PreparedChat{Scope: Scope{OrganizationID: org, WorkspaceID: &ws, AccountID: owner}, Caller: Caller{Type: appType, ID: &app, Source: entry},
					Conversation: &runtimemodel.Conversation{ID: cid}, Message: &runtimemodel.Message{ID: mid, ConversationID: cid}}
				if entry == "external-api" {
					o.SubjectKind = "api_end_user"
					if appType == "agent" {
						p.Scope.ExternalUserID = &owner
						p.Scope.AccountID = uuid.New()
					}
				}
				f := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{ExecutionOrigin: &o, CorrelationID: "pending", SkillID: "test", ToolName: "write", Arguments: map[string]interface{}{"value": 1}})
				event := map[string]interface{}{"correlation_id": "pending", "frozen_invocation": f}
				live, calls := true, 0
				p.RunConfig.RuntimeToolFactory = func(*PreparedChat) []skillloop.RuntimeTool {
					return []skillloop.RuntimeTool{{
						Definition: adapter.Tool{Function: adapter.Function{Name: "shell_exec"}}, PlatformOrigin: &o,
						PlatformInteractiveApproval: true, BindPlatform: func(skillloop.RuntimePlatformServices) {},
						AuthorizePlatform: func(context.Context) error {
							calls++
							if !live {
								return ErrPermissionDenied
							}
							return nil
						},
					}}
				}
				if err := checkPlatformApprovalScope(t.Context(), p, event, true); err != nil || calls != 1 {
					t.Fatalf("approved origin: %v", err)
				}
				live = false
				if checkPlatformApprovalScope(t.Context(), p, event, true) == nil {
					t.Fatal("revoked access approved")
				}
				p.RunConfig.RuntimeToolFactory = nil
				if err := checkPlatformApprovalScope(t.Context(), p, event, false); err != nil {
					t.Fatalf("cannot reject after withdrawal: %v", err)
				}
				if checkPlatformApprovalScope(t.Context(), p, event, true) == nil {
					t.Fatal("missing runtime approved")
				}
				p.Scope.AccountID, p.Scope.ExternalUserID = uuid.New(), nil
				if checkPlatformApprovalScope(t.Context(), p, event, false) == nil {
					t.Fatal("another subject rejected approval")
				}
			})
		}
	}
}

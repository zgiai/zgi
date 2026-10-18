package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type approvalRecordingGateway struct {
	*skills.PolicyToolGovernanceGateway
	decision toolgovernance.Decision
}

func (g *approvalRecordingGateway) DecideSkillTool(ctx context.Context, req skills.ToolGovernanceRequest) (toolgovernance.Decision, error) {
	d, err := g.PolicyToolGovernanceGateway.DecideSkillTool(ctx, req)
	g.decision = d
	return d, err
}

func TestPlatformApprovalStreamsExecuteFrozenCallOnlyAndRejectWithoutCompute(t *testing.T) {
	for _, appType := range []string{"agent", "digital_human"} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			for _, action := range []string{"approve", "reject"} {
				t.Run(appType+"/"+entry+"/"+action, func(t *testing.T) {
					org, ws, account, app, conversationID, messageID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
					scope := Scope{OrganizationID: org, AccountID: account, WorkspaceID: &ws}
					caller := Caller{Type: appType, ID: &app, Source: entry}
					origin := toolgovernance.ExecutionOrigin{OrganizationID: org.String(), WorkspaceID: ws.String(), ApplicationType: appType,
						ApplicationID: app.String(), Entry: entry, SubjectKind: "account", SubjectID: account.String(), ConversationID: conversationID.String(),
						MessageID: messageID.String(), PublicationRef: uuid.NewString()}
					if entry == "external-api" {
						origin.SubjectKind = "api_end_user"
						if appType == "agent" {
							user := uuid.New()
							scope.ExternalUserID = &user
							origin.SubjectID = user.String()
						}
					}
					conversation := &runtimemodel.Conversation{ID: conversationID, OrganizationID: org, AccountID: account, WorkspaceID: &ws,
						CallerType: appType, CallerID: &app, Source: entry, ExternalUserID: scope.ExternalUserID, Status: runtimemodel.ConversationStatusNormal,
						RuntimeStatus: runtimemodel.ConversationRuntimeStatusIdle, CurrentLeafMessageID: &messageID}
					if entry == "webapp" {
						id := uuid.New()
						caller.SourceWebAppID, conversation.SourceWebAppID = &id, &id
					}
					meta := pendingToolGovernanceDecisionMetadata("scoped")
					meta["configured_skill_ids"] = []interface{}{skills.SkillFileManager}
					event, _ := toolGovernanceDecisionEventFromMetadata(meta, "scoped")
					event["assets"] = []interface{}{map[string]interface{}{"id": "file-1", "type": "file", "workspace_id": ws.String()}}
					event["grant"] = map[string]interface{}{"conversation_id": conversationID.String(), "organization_id": org.String(), "user_id": account.String(),
						"skill_id": skills.SkillFileManager, "provider_type": "builtin", "provider_id": "files", "tool_id": "file.delete", "effect": "delete", "asset_type": "file", "risk_level": "high",
						"assets": []interface{}{map[string]interface{}{"id": "file-1", "type": "file"}}}
					approval := toolGovernanceApprovalEventFromEvent(event)
					approval["grant"], approval["assets"] = event["grant"], event["assets"]
					event["approval_event"] = approval
					if g := governanceMapFromAny(event["governance"]); g != nil {
						g["approval_event"] = approval
						event["governance"] = g
					}
					frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{ExecutionOrigin: &origin, CorrelationID: "scoped",
						SkillID: skills.SkillFileManager, ToolName: "delete_file", ProviderType: "builtin", ProviderID: "files",
						Manifest:  toolgovernance.Manifest{ToolID: "file.delete", SkillID: skills.SkillFileManager, Effect: toolgovernance.EffectDelete, AssetType: "file", RiskLevel: toolgovernance.RiskLevelHigh},
						Arguments: map[string]interface{}{"file_id": "file-1"}, Assets: []toolgovernance.AssetRef{{ID: "file-1", Type: "file", WorkspaceID: ws.String()}}})
					if action == "reject" {
						frozen = toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{ExecutionOrigin: &origin, CorrelationID: "scoped", SkillID: skills.SkillFileManager, ToolName: "delete_file", Arguments: map[string]interface{}{"file_id": "file-1"}, Now: time.Now().Add(-2 * time.Hour), TTL: time.Minute})
					}
					event["frozen_invocation"] = frozen
					meta = mergeToolGovernanceDecisionMetadata(meta, event)
					provider := "deepseek"
					message := &runtimemodel.Message{ID: messageID, ConversationID: conversationID, Query: "Delete report", ModelProvider: &provider,
						ModelName: "deepseek-chat", Status: runtimemodel.MessageStatusWaitingApproval, Metadata: meta}
					workspace := ws.String()
					files := &toolGovernanceStreamFileService{files: map[string]*dto.UploadFile{"file-1": {ID: "file-1", OrganizationID: org.String(), WorkspaceID: &workspace, CreatedBy: account.String(), Name: "report.pdf", MimeType: "application/pdf"}}}
					llm := &toolGovernanceStreamLLM{streamChunks: []string{"Approved result."}}
					runtime := newToolGovernanceStreamSkillRuntime(t, files, &toolGovernanceStreamWorkspacePermissionService{allowed: true})
					governance := &approvalRecordingGateway{PolicyToolGovernanceGateway: skills.NewPolicyToolGovernanceGateway(toolgovernance.DefaultPolicy())}
					runtime.WithToolGovernanceGateway(governance)
					svc := NewServiceWithSkillRuntime(&repository.Repositories{Access: toolGovernanceStreamAccessRepo{}, Conversation: &toolGovernanceStreamConversationRepo{conversation: conversation},
						Message: &toolGovernanceStreamMessageRepo{message: message}, SkillConfig: toolGovernanceStreamSkillConfigRepo{skillID: skills.SkillFileManager}}, llm, nil, toolGovernanceStreamModelSpecResolver{}, nil, nil, nil, runtime, nil).(*service)
					svc.events = newStreamEventStore(nil)
					shellCalls := 0
					config := RunConfig{BillingAppID: app.String(), BillingAppType: appType, ModelProvider: provider, Model: message.ModelName, EnabledSkillIDs: []string{skills.SkillFileManager}}
					if action == "approve" {
						config.RuntimeToolFactory = func(*PreparedChat) []skillloop.RuntimeTool {
							return []skillloop.RuntimeTool{{Definition: adapter.Tool{Function: adapter.Function{Name: "shell_exec"}}, PlatformOrigin: &origin, PlatformInteractiveApproval: true,
								BindPlatform: func(skillloop.RuntimePlatformServices) {}, AuthorizePlatform: func(context.Context) error { return nil }, Handler: func(context.Context, adapter.ToolCall) skillloop.RuntimeToolResult {
									shellCalls++
									return skillloop.RuntimeToolResult{}
								}}}
						}
					}
					result, err := svc.RunConfiguredToolGovernanceDecisionStream(t.Context(), scope, caller, config, conversationID, messageID, "scoped", runtimedto.ToolGovernanceDecisionRequest{Action: action, RememberForSession: true}, func(StreamEvent) error { return nil })
					if err != nil {
						t.Fatalf("%v; governance=%+v", err, governance.decision)
					}
					if result.Status != runtimemodel.MessageStatusCompleted || shellCalls != 0 {
						t.Fatalf("result=%+v shell calls=%d", result, shellCalls)
					}
					want := 0
					if action == "approve" {
						want = 1
					}
					if len(files.deleted) != want {
						t.Fatalf("business calls=%d want %d", len(files.deleted), want)
					}
					decision, _ := toolGovernanceDecisionEventFromMetadata(message.Metadata, "scoped")
					stored := governanceMapFromAny(governanceMapFromAny(decision["governance"])["approval_result"])
					if boolMetadataValue(stored["remember_for_session"]) || len(governanceMapFromAny(stored["session_grant"])) != 0 {
						t.Fatal("CLI approval created a reusable session grant")
					}
					// A duplicate result replay must not need compute or execute again.
					config.RuntimeToolFactory = nil
					if _, err := svc.RunConfiguredToolGovernanceDecisionStream(t.Context(), scope, caller, config, conversationID, messageID, "scoped", runtimedto.ToolGovernanceDecisionRequest{Action: action}, func(StreamEvent) error { return nil }); err != nil || len(files.deleted) != want {
						t.Fatalf("duplicate replay: %v calls=%d", err, len(files.deleted))
					}
					if _, err := svc.beginToolGovernanceContinuationForCaller(t.Context(), scope, caller, conversationID, messageID, "scoped"); !IsContinuationAlreadyRunningError(err) {
						t.Fatalf("resolved approval was executable again: %v", err)
					}
					if action == "reject" && (len(llm.appChatRequests) > 0 || len(llm.streamRequests) > 0) {
						t.Fatal("reject called model")
					}
				})
			}
		}
	}
}

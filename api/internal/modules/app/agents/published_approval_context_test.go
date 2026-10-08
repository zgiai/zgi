package agents

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func publishedApprovalFixture(t *testing.T, entry string) (*runtimeservice.PreparedChat, *dto.AgentWebAppRuntimeConfigResponse, toolgovernance.FrozenInvocation) {
	t.Helper()
	p, published := publishedExecutionFixture(entry)
	owner, ok := agentExecutionFileAccess(p.Scope, p.Caller, p.Conversation)
	if !ok {
		t.Fatal("invalid test owner")
	}
	origin := &toolgovernance.ExecutionOrigin{OrganizationID: owner.OrganizationID, WorkspaceID: owner.WorkspaceID,
		ApplicationType: "agent", ApplicationID: owner.ApplicationID, Entry: entry, SubjectKind: owner.SubjectKind, SubjectID: owner.SubjectID,
		ConversationID: p.Conversation.ID.String(), MessageID: p.Message.ID.String(), PublicationRef: published.VersionUUID}
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{ExecutionOrigin: origin, CorrelationID: "pending", SkillID: "chart-generator", ToolName: "generate_chart", Arguments: map[string]interface{}{"title": "original"}})
	setPublishedApprovalMetadata(p.Message, frozen)
	return p, published, frozen
}

func setPublishedApprovalMetadata(message *runtimemodel.Message, frozen toolgovernance.FrozenInvocation) {
	message.Metadata = map[string]interface{}{"tool_governance_decisions": []interface{}{map[string]interface{}{
		"correlation_id": "pending", "approval_event": map[string]interface{}{"frozen_invocation": frozen}}}}
}

func TestPublishedApprovalOriginRejectsUntrustedCoordinates(t *testing.T) {
	for _, entry := range []string{"webapp", "external-api"} {
		for _, kind := range []string{"valid", "tampered", "other user", "other entry", "other application", "other publication owner", "other message", "other conversation", "deleted", "duplicate", "missing"} {
			t.Run(entry+"/"+kind, func(t *testing.T) {
				p, _, frozen := publishedApprovalFixture(t, entry)
				cid, mid := p.Conversation.ID, p.Message.ID
				switch kind {
				case "tampered":
					frozen.Arguments["title"] = "tampered"
				case "other user":
					frozen.ExecutionOrigin.SubjectID = uuid.NewString()
					frozen = toolgovernance.SealFrozenInvocation(frozen)
				case "other entry":
					frozen.ExecutionOrigin.Entry = "console"
					frozen = toolgovernance.SealFrozenInvocation(frozen)
				case "other application":
					frozen.ExecutionOrigin.ApplicationID = uuid.NewString()
					frozen = toolgovernance.SealFrozenInvocation(frozen)
				case "other publication owner":
					frozen.ExecutionOrigin.WorkspaceID = uuid.NewString()
					frozen = toolgovernance.SealFrozenInvocation(frozen)
				case "other message":
					mid = uuid.New()
				case "other conversation":
					cid = uuid.New()
				case "deleted":
					now := time.Now()
					p.Message.DeletedAt = &now
				}
				setPublishedApprovalMetadata(p.Message, frozen)
				if kind == "duplicate" {
					events := p.Message.Metadata["tool_governance_decisions"].([]interface{})
					p.Message.Metadata["tool_governance_decisions"] = append(events, events[0])
				}
				if kind == "missing" {
					p.Message.Metadata = nil
				}
				got, err := publishedApprovalOrigin(agentRuntimeContext{Scope: p.Scope, Caller: p.Caller}, p.Conversation, p.Message, cid, mid, "pending")
				if kind == "valid" {
					if err != nil || got == nil || *got != *frozen.ExecutionOrigin {
						t.Fatalf("origin=%+v err=%v", got, err)
					}
				} else if !errors.Is(err, execution.ErrInvalidIdentity) {
					t.Fatalf("invalid coordinates accepted: origin=%+v err=%v", got, err)
				}
			})
		}
	}
}

type publishedApprovalMessageService struct {
	runtimeservice.Service
	prepared *runtimeservice.PreparedChat
}

func (s *publishedApprovalMessageService) GetMessageByCaller(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, messageID uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	if scope.AccountID != s.prepared.Scope.AccountID || caller.Source != s.prepared.Caller.Source || messageID != s.prepared.Message.ID {
		return nil, nil, runtimeservice.ErrNotFound
	}
	return s.prepared.Message, s.prepared.Conversation, nil
}

type publishedApprovalService struct {
	*publishedExecutionTestService
	original *dto.AgentWebAppRuntimeConfigResponse
}

func (s *publishedApprovalService) getPublishedAgentRuntimeConfigAtVersion(_ context.Context, app, version uuid.UUID) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	if s.original == nil || app.String() != s.original.AgentID || version.String() != s.original.VersionUUID {
		return nil, execution.ErrInvalidIdentity
	}
	return s.original, nil
}

func TestPublishedApprovalRunConfigRestoresOriginalAndKeepsLiveAuthorization(t *testing.T) {
	previous := tool_file.GlobalToolFileManager
	tool_file.GlobalToolFileManager = &tool_file.ToolFileManager{}
	t.Cleanup(func() { tool_file.GlobalToolFileManager = previous })
	for _, entry := range []string{"webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			p, original, frozen := publishedApprovalFixture(t, entry)
			original.Version = "v1"
			original.Config.SystemPrompt, original.Config.Model = "original prompt", "original model"
			original.Config.EnabledSkillIDs = []string{"chart-generator"}
			current := *original
			current.VersionUUID, current.Version = uuid.NewString(), "v2"
			current.Config = dto.AgentConfigResponse{ExecutionRuntime: &dto.AgentExecutionRuntime{Enabled: true}, SystemPrompt: "new prompt", Model: "new model"}
			service := &publishedApprovalService{publishedExecutionTestService: &publishedExecutionTestService{
				stubWebAppStatusHandlerService: &stubWebAppStatusHandlerService{publishedConfigResp: &current, runtimeCapabilityResp: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: true}},
				keyID:                          uuid.New(), application: *p.Caller.ID, credentialWorkspace: *p.Scope.WorkspaceID}, original: original}
			t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", `{"agent:`+entry+`":["`+p.Scope.OrganizationID.String()+`"]}`)
			t.Setenv("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", "true")
			t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "true")
			t.Setenv("ZGI_AGENT_RUNTIME_URL", "http://127.0.0.1:1")
			t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "unit-test-only")
			h := &AgentsHandler{appService: service, chatRuntimeService: &publishedApprovalMessageService{prepared: p}, fileService: &executionUploadTestFiles{}, executionGateway: executionruntime.GatewayFromEnv(), executionEntryReady: func(string, string) bool { return true },
				executionApprovals: agentExecutionApprovals{Enabled: true, GatewayReady: true, Profiles: map[string]agentExecutionApproval{"standard": {Backend: "local-gvisor", OrganizationIDs: []string{p.Scope.OrganizationID.String()}, PolicyIDs: []string{"offline"}}}}}
			restore := func() (runtimeservice.RunConfig, error) {
				return h.publishedApprovalRunConfig(t.Context(), agentRuntimeContext{Scope: p.Scope, Caller: p.Caller}, p.Conversation.ID, p.Message.ID, "pending", service.keyID)
			}
			config, err := restore()
			if err != nil {
				t.Fatal(err)
			}
			if config.SystemPrompt != "original prompt" || config.Model != "original model" || config.SystemPromptVersion != "agent.published.v1" || len(config.EnabledSkillIDs) != 1 || config.EnabledSkillIDs[0] != "chart-generator" {
				t.Fatalf("continuation substituted latest configuration: %+v", config)
			}
			tools := config.RuntimeToolFactory(p)
			if len(tools) < 1 || tools[0].PlatformOrigin == nil || *tools[0].PlatformOrigin != *frozen.ExecutionOrigin || tools[0].AuthorizePlatform == nil {
				t.Fatalf("original platform origin missing: %+v", tools)
			}
			if err := tools[0].AuthorizePlatform(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := h.authorizePublishedExecution(t.Context(), publishedAccessForTest(t, p, original), service.keyID); err == nil {
				t.Fatal("ordinary execution accepted historical version")
			}
			current.Config.ExecutionRuntime.Enabled = false
			if tools[0].AuthorizePlatform(t.Context()) == nil {
				t.Fatal("frozen configuration bypassed current Linux disablement")
			}
			current.Config.ExecutionRuntime.Enabled = true
			if entry == "webapp" {
				service.runtimeCapabilityResp.Allowed = false
			} else {
				service.credentialRevoked = true
			}
			if tools[0].AuthorizePlatform(t.Context()) == nil {
				t.Fatal("frozen configuration bypassed live access revocation")
			}
			service.runtimeCapabilityResp.Allowed, service.credentialRevoked = true, false
			original.Config.ExecutionRuntime.Enabled = false
			if _, err := restore(); err == nil {
				t.Fatal("historical disabled version acquired execution")
			}
		})
	}
}

type frozenPublishedRuntimeRepo struct {
	*publishedRuntimeRepo
	requested uuid.UUID
}

func (r *frozenPublishedRuntimeRepo) getAgentPublishedVersionByUUID(_ context.Context, app, version uuid.UUID) (*AgentPublishedVersion, error) {
	r.requested = version
	return r.version, nil
}

func TestFrozenPublishedVersionLoaderRejectsForeignAndDeletedVersions(t *testing.T) {
	for _, kind := range []string{"valid", "foreign application", "foreign workspace", "wrong version", "deleted", "missing", "non-agent"} {
		t.Run(kind, func(t *testing.T) {
			app, ws, version := uuid.New(), uuid.New(), uuid.New()
			repo := &frozenPublishedRuntimeRepo{publishedRuntimeRepo: &publishedRuntimeRepo{agent: &Agent{ID: app, TenantID: ws, AgentsType: "AGENT"}, version: &AgentPublishedVersion{AgentID: app, WorkspaceID: ws, VersionUUID: version, ConfigSnapshot: map[string]interface{}{"model": "original"}}}}
			switch kind {
			case "foreign application":
				repo.version.AgentID = uuid.New()
			case "foreign workspace":
				repo.version.WorkspaceID = uuid.New()
			case "wrong version":
				repo.version.VersionUUID = uuid.New()
			case "deleted":
				now := time.Now()
				repo.version.DeletedAt = &now
			case "missing":
				repo.version = nil
			case "non-agent":
				repo.agent.AgentsType = "WORKFLOW"
			}
			service := &agentsService{agentsRepo: repo}
			got, err := service.getPublishedAgentRuntimeConfigAtVersion(t.Context(), app, version)
			if kind == "valid" {
				if err != nil || got == nil || got.VersionUUID != version.String() || got.Config.Model != "original" || repo.requested != version {
					t.Fatalf("historical publication=%+v err=%v", got, err)
				}
			} else if err == nil {
				t.Fatal("invalid historical publication accepted")
			}
		})
	}
}

func TestFrozenPublishedVersionRepositoryPinsApplicationUUIDAndLiveRow(t *testing.T) {
	db, mock := newAgentSystemPromptPatchTestDB(t)
	app, version := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT \* FROM "agent_published_versions" WHERE agent_id = \$1 AND version_uuid = \$2 AND deleted_at IS NULL LIMIT \$3`).WithArgs(app, version, 1).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "version_uuid"}).AddRow(app, version))
	repo := &agentsRepository{db: db}
	got, err := repo.getAgentPublishedVersionByUUID(t.Context(), app, version)
	if err != nil || got == nil || got.AgentID != app || got.VersionUUID != version {
		t.Fatalf("publication=%+v err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

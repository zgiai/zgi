package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type mockCurrentWorkspaceGetter struct {
	workspace *workspace_model.Workspace
	err       error
}

func (m mockCurrentWorkspaceGetter) GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.Workspace, error) {
	return m.workspace, m.err
}

type mockWorkspaceOrganizationResolver struct {
	organization *workspace_model.Organization
	err          error
}

func (m mockWorkspaceOrganizationResolver) GetOrganizationByWorkspaceID(ctx context.Context, workspaceID string) (*workspace_model.Organization, error) {
	return m.organization, m.err
}

type mockCurrentOrganizationEnsurer struct {
	organizationID string
	err            error
	called         bool
}

func (m *mockCurrentOrganizationEnsurer) EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error) {
	m.called = true
	if m.err != nil {
		return "", m.err
	}
	return m.organizationID, nil
}

type mockWebAppRunScopeAccountService struct {
	workspace       *workspace_model.Workspace
	workspaceErr    error
	organizationID  string
	organizationErr error
}

func (m mockWebAppRunScopeAccountService) GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.Workspace, error) {
	return m.workspace, m.workspaceErr
}

func (m mockWebAppRunScopeAccountService) EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error) {
	if m.organizationErr != nil {
		return "", m.organizationErr
	}
	return m.organizationID, nil
}

type mockShadowWorkspaceEnsurer struct {
	workspace *workspace_model.Workspace
	err       error
	called    bool
}

func (m *mockShadowWorkspaceEnsurer) GetShadowWorkspaceByID(ctx context.Context, organizationID string) (*workspace_model.Workspace, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return m.workspace, nil
}

func TestRejectInactiveWebAppReturnsOfflineError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/workflows/web-app-id/run", nil)

	rejected := rejectInactiveWebApp(ctx, &agents.Agent{
		ID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		WebAppStatus: agents.AgentWebAppStatusInactive,
	}, "web-app-id")
	if !rejected {
		t.Fatalf("rejectInactiveWebApp returned false for inactive web app")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if body.Code != "204008" {
		t.Fatalf("code = %q, want %q", body.Code, "204008")
	}
	if body.Message != response.ErrWebAppOffline.Message {
		t.Fatalf("message = %q, want %q", body.Message, response.ErrWebAppOffline.Message)
	}
}

func TestRejectInactiveWebAppAllowsPersistedEnabledWebAppSurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, mock := setWorkflowWebAppRuntimeMockDB(t)

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	expectWorkflowWebAppRuntimeSurfaceRows(mock, agentID, workspaceID, "webapp", true, nil)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/workflows/web-app-id/run", nil)

	rejected := rejectInactiveWebApp(ctx, &agents.Agent{
		ID:           agentID,
		TenantID:     workspaceID,
		WebAppStatus: agents.AgentWebAppStatusInactive,
	}, "web-app-id")
	if rejected {
		t.Fatalf("rejectInactiveWebApp returned true, want persisted enabled webapp surface to stay public-compatible")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestRejectInactiveWebAppRejectsPersistedNonPublicWebAppGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, mock := setWorkflowWebAppRuntimeMockDB(t)

	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountGrantID := uuid.MustParse("99999999-9999-9999-9999-999999999998")
	expectWorkflowWebAppRuntimeSurfaceRows(mock, agentID, workspaceID, "webapp", true, &workflowWebAppRuntimeGrantExpectation{
		subjectType: "account",
		subjectID:   accountGrantID,
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/workflows/web-app-id/run", nil)

	rejected := rejectInactiveWebApp(ctx, &agents.Agent{
		ID:           agentID,
		TenantID:     workspaceID,
		WebAppStatus: agents.AgentWebAppStatusActive,
	}, "web-app-id")
	if !rejected {
		t.Fatalf("rejectInactiveWebApp returned false, want stale non-public webapp grant to fail closed")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestResolveWebAppRunWorkspaceID_PrefersCurrentWorkspace(t *testing.T) {
	workspaceID, err := resolveWebAppRunWorkspaceID(
		context.Background(),
		mockCurrentWorkspaceGetter{
			workspace: &workspace_model.Workspace{ID: "ws-current"},
		},
		"acc-1",
		"ws-agent",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunWorkspaceID returned error: %v", err)
	}
	if workspaceID != "ws-current" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-current")
	}
}

type workflowWebAppRuntimeGrantExpectation struct {
	subjectType string
	subjectID   uuid.UUID
}

func setWorkflowWebAppRuntimeMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(oldDB)
		_ = sqlDB.Close()
	})
	return db, mock
}

func expectWorkflowWebAppRuntimeSurfaceRows(mock sqlmock.Sqlmock, agentID, workspaceID uuid.UUID, surfaceName string, enabled bool, grant *workflowWebAppRuntimeGrantExpectation) {
	surfaceID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(
			surfaceID.String(),
			string(runtimeauth.PublishedRuntimeResourceAgent),
			agentID.String(),
			uuid.New().String(),
			workspaceID.String(),
			surfaceName,
			enabled,
			runtimeauth.PublishedRuntimeSourceGrant,
			now,
			now,
			nil,
		))

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	if grant != nil {
		grantRows.AddRow(
			uuid.NewString(),
			surfaceID.String(),
			grant.subjectType,
			grant.subjectID.String(),
			true,
			now,
			now,
			nil,
		)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(grantRows)
}

func TestResolveWebAppRunWorkspaceID_FallsBackToAgentWorkspace(t *testing.T) {
	workspaceID, err := resolveWebAppRunWorkspaceID(
		context.Background(),
		mockCurrentWorkspaceGetter{},
		"acc-1",
		"ws-agent",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunWorkspaceID returned error: %v", err)
	}
	if workspaceID != "ws-agent" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-agent")
	}
}

func TestResolveWebAppRunWorkspaceID_ReturnsEmptyWithoutWorkspaceOrFallback(t *testing.T) {
	workspaceID, err := resolveWebAppRunWorkspaceID(
		context.Background(),
		mockCurrentWorkspaceGetter{},
		"acc-1",
		"",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunWorkspaceID returned error: %v", err)
	}
	if workspaceID != "" {
		t.Fatalf("workspaceID = %q, want empty", workspaceID)
	}
}

func TestResolveWebAppRunScope_SystemUsesShadowWorkspaceFallback(t *testing.T) {
	shadowWorkspace := &mockShadowWorkspaceEnsurer{
		workspace: &workspace_model.Workspace{ID: "org-1"},
	}

	scope, err := resolveWebAppRunScope(
		context.Background(),
		mockWebAppRunScopeAccountService{organizationID: "org-1"},
		nil,
		shadowWorkspace,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.Nil,
		},
		"",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunScope returned error: %v", err)
	}
	if scope.WorkspaceID != "org-1" {
		t.Fatalf("WorkspaceID = %q, want %q", scope.WorkspaceID, "org-1")
	}
	if scope.OrganizationID != "org-1" {
		t.Fatalf("OrganizationID = %q, want %q", scope.OrganizationID, "org-1")
	}
	if scope.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("BillingSubjectType = %q, want %q", scope.BillingSubjectType, llmclient.BillingSubjectTypeOrganization)
	}
	if !shadowWorkspace.called {
		t.Fatalf("GetShadowWorkspaceByID was not called")
	}
}

func TestResolveWebAppRunScope_UnsetWorkspaceAgentUsesShadowWorkspaceFallback(t *testing.T) {
	shadowWorkspace := &mockShadowWorkspaceEnsurer{
		workspace: &workspace_model.Workspace{ID: "org-1"},
	}

	scope, err := resolveWebAppRunScope(
		context.Background(),
		mockWebAppRunScopeAccountService{organizationID: "org-1"},
		nil,
		shadowWorkspace,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.Nil,
			Source:   agents.AgentSourceUser,
		},
		"",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunScope returned error: %v", err)
	}
	if scope.WorkspaceID != "org-1" {
		t.Fatalf("WorkspaceID = %q, want %q", scope.WorkspaceID, "org-1")
	}
	if scope.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("BillingSubjectType = %q, want %q", scope.BillingSubjectType, llmclient.BillingSubjectTypeOrganization)
	}
	if !shadowWorkspace.called {
		t.Fatalf("GetShadowWorkspaceByID was not called")
	}
}

func TestResolveWebAppRunScope_SystemUsesShadowWorkspaceWhenCurrentWorkspaceLookupFails(t *testing.T) {
	shadowWorkspace := &mockShadowWorkspaceEnsurer{
		workspace: &workspace_model.Workspace{ID: "org-1"},
	}

	scope, err := resolveWebAppRunScope(
		context.Background(),
		mockWebAppRunScopeAccountService{
			workspaceErr:   errors.New("workspace not found"),
			organizationID: "org-1",
		},
		nil,
		shadowWorkspace,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.Nil,
		},
		"",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunScope returned error: %v", err)
	}
	if scope.WorkspaceID != "org-1" {
		t.Fatalf("WorkspaceID = %q, want %q", scope.WorkspaceID, "org-1")
	}
	if scope.OrganizationID != "org-1" {
		t.Fatalf("OrganizationID = %q, want %q", scope.OrganizationID, "org-1")
	}
	if !shadowWorkspace.called {
		t.Fatalf("GetShadowWorkspaceByID was not called")
	}
}

func TestResolveWebAppRunScope_SystemKeepsCurrentWorkspace(t *testing.T) {
	shadowWorkspace := &mockShadowWorkspaceEnsurer{
		workspace: &workspace_model.Workspace{ID: "org-1"},
	}

	scope, err := resolveWebAppRunScope(
		context.Background(),
		mockWebAppRunScopeAccountService{
			workspace:      &workspace_model.Workspace{ID: "ws-current"},
			organizationID: "org-1",
		},
		nil,
		shadowWorkspace,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.Nil,
		},
		"",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunScope returned error: %v", err)
	}
	if scope.WorkspaceID != "ws-current" {
		t.Fatalf("WorkspaceID = %q, want %q", scope.WorkspaceID, "ws-current")
	}
	if scope.OrganizationID != "org-1" {
		t.Fatalf("OrganizationID = %q, want %q", scope.OrganizationID, "org-1")
	}
	if shadowWorkspace.called {
		t.Fatalf("GetShadowWorkspaceByID was called despite current workspace")
	}
}

func TestResolveWebAppRunScope_UserAgentDoesNotUseShadowWorkspace(t *testing.T) {
	shadowWorkspace := &mockShadowWorkspaceEnsurer{
		workspace: &workspace_model.Workspace{ID: "org-1"},
	}

	scope, err := resolveWebAppRunScope(
		context.Background(),
		mockWebAppRunScopeAccountService{organizationID: "org-1"},
		nil,
		shadowWorkspace,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			TenantID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			Source:   agents.AgentSourceUser,
		},
		"33333333-3333-3333-3333-333333333333",
	)
	if err != nil {
		t.Fatalf("resolveWebAppRunScope returned error: %v", err)
	}
	if scope.WorkspaceID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("WorkspaceID = %q, want fallback workspace", scope.WorkspaceID)
	}
	if scope.OrganizationID != "" {
		t.Fatalf("OrganizationID = %q, want empty", scope.OrganizationID)
	}
	if shadowWorkspace.called {
		t.Fatalf("GetShadowWorkspaceByID was called for user agent")
	}
}

func TestWebAppPrecheckRequiresWorkspace_SkipsSystemAgent(t *testing.T) {
	requiresWorkspace := webAppPrecheckRequiresWorkspace(&agents.Agent{
		ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID: uuid.Nil,
	})

	if requiresWorkspace {
		t.Fatalf("webAppPrecheckRequiresWorkspace = true, want false")
	}
}

func TestWebAppPrecheckRequiresWorkspace_SkipsUnsetWorkspaceAgent(t *testing.T) {
	requiresWorkspace := webAppPrecheckRequiresWorkspace(&agents.Agent{
		ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID: uuid.Nil,
		Source:   agents.AgentSourceUser,
	})

	if requiresWorkspace {
		t.Fatalf("webAppPrecheckRequiresWorkspace = true, want false")
	}
}

func TestWebAppPrecheckRequiresWorkspace_RequiresUserAgentWorkspace(t *testing.T) {
	requiresWorkspace := webAppPrecheckRequiresWorkspace(&agents.Agent{
		ID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TenantID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Source:   agents.AgentSourceUser,
	})

	if !requiresWorkspace {
		t.Fatalf("webAppPrecheckRequiresWorkspace = false, want true")
	}
}

func TestResolveCallerOrganizationForWebAppPrecheck_SystemAgent(t *testing.T) {
	ensurer := &mockCurrentOrganizationEnsurer{organizationID: "org-current"}
	organizationID := resolveCallerOrganizationForWebAppPrecheck(
		context.Background(),
		ensurer,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			TenantID: uuid.Nil,
		},
	)

	if organizationID != "org-current" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-current")
	}
	if !ensurer.called {
		t.Fatalf("EnsureCurrentOrganizationID was not called")
	}
}

func TestResolveCallerOrganizationForWebAppPrecheck_UnsetWorkspaceAgent(t *testing.T) {
	ensurer := &mockCurrentOrganizationEnsurer{organizationID: "org-current"}
	organizationID := resolveCallerOrganizationForWebAppPrecheck(
		context.Background(),
		ensurer,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			TenantID: uuid.Nil,
			Source:   agents.AgentSourceUser,
		},
	)

	if organizationID != "org-current" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-current")
	}
	if !ensurer.called {
		t.Fatalf("EnsureCurrentOrganizationID was not called")
	}
}

func TestResolveCallerOrganizationForWebAppPrecheck_UserAgentSkipsLookup(t *testing.T) {
	ensurer := &mockCurrentOrganizationEnsurer{organizationID: "org-current"}
	organizationID := resolveCallerOrganizationForWebAppPrecheck(
		context.Background(),
		ensurer,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("55555555-5555-5555-5555-555555555555"),
			TenantID: uuid.MustParse("66666666-6666-6666-6666-666666666666"),
			Source:   agents.AgentSourceUser,
		},
	)

	if organizationID != "" {
		t.Fatalf("organizationID = %q, want empty", organizationID)
	}
	if ensurer.called {
		t.Fatalf("EnsureCurrentOrganizationID was called for user agent")
	}
}

func TestResolveCallerOrganizationForWebAppPrecheck_ReturnsEmptyOnLookupError(t *testing.T) {
	ensurer := &mockCurrentOrganizationEnsurer{err: errors.New("no organization")}
	organizationID := resolveCallerOrganizationForWebAppPrecheck(
		context.Background(),
		ensurer,
		"acc-1",
		&agents.Agent{
			ID:       uuid.MustParse("77777777-7777-7777-7777-777777777777"),
			TenantID: uuid.Nil,
		},
	)

	if organizationID != "" {
		t.Fatalf("organizationID = %q, want empty", organizationID)
	}
	if !ensurer.called {
		t.Fatalf("EnsureCurrentOrganizationID was not called")
	}
}

func TestResolveRunOrganizationID_PrefersInputValue(t *testing.T) {
	organizationID := resolveRunOrganizationID(
		context.Background(),
		mockWorkspaceOrganizationResolver{
			organization: &workspace_model.Organization{ID: "org-from-resolver"},
		},
		"ws-1",
		map[string]interface{}{"sys.organization_id": "org-from-input"},
	)

	if organizationID != "org-from-input" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-from-input")
	}
}

func TestResolveRunOrganizationID_FallsBackToWorkspaceLookup(t *testing.T) {
	organizationID := resolveRunOrganizationID(
		context.Background(),
		mockWorkspaceOrganizationResolver{
			organization: &workspace_model.Organization{ID: "org-from-workspace"},
		},
		"ws-1",
		nil,
	)

	if organizationID != "org-from-workspace" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-from-workspace")
	}
}

func TestResolveRunStreamWorkspaceID_UsesCallerWorkspaceForSystemAgent(t *testing.T) {
	workspaceID := resolveRunStreamWorkspaceID(&agents.Agent{
		ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TenantID: uuid.Nil,
	}, "ws-caller", "00000000-0000-0000-0000-000000000000")

	if workspaceID != "ws-caller" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-caller")
	}
}

func TestResolveRunStreamWorkspaceID_UsesAppWorkspaceForUserAgent(t *testing.T) {
	workspaceID := resolveRunStreamWorkspaceID(&agents.Agent{
		ID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TenantID: uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Source:   agents.AgentSourceUser,
	}, "ws-caller", "ws-app")

	if workspaceID != "ws-app" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-app")
	}
}

func TestBuildWorkflowStartedEventPayload_ConversationWorkflowIncludesTopLevelIDs(t *testing.T) {
	systemInputs := map[string]interface{}{
		"sys.conversation_id": "conv-new",
		"sys.query":           "hello",
	}

	payload := buildWorkflowStartedEventPayload(
		"CONVERSATION_WORKFLOW",
		"run-123",
		"workflow-456",
		1,
		systemInputs,
		1700000000,
	)

	if got := payload["id"]; got != "run-123" {
		t.Fatalf("id = %v, want %q", got, "run-123")
	}
	if got := payload["workflow_id"]; got != "workflow-456" {
		t.Fatalf("workflow_id = %v, want %q", got, "workflow-456")
	}
	if got := payload["message_id"]; got != "run-123" {
		t.Fatalf("message_id = %v, want %q", got, "run-123")
	}
	if got := payload["conversation_id"]; got != "conv-new" {
		t.Fatalf("conversation_id = %v, want %q", got, "conv-new")
	}
	if got := payload["sequence_number"]; got != 1 {
		t.Fatalf("sequence_number = %v, want %d", got, 1)
	}
	if got := payload["created_at"]; got != int64(1700000000) {
		t.Fatalf("created_at = %v, want %d", got, int64(1700000000))
	}
	inputs, ok := payload["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputs type = %T, want map[string]interface{}", payload["inputs"])
	}
	if got := inputs["sys.conversation_id"]; got != "conv-new" {
		t.Fatalf("inputs.sys.conversation_id = %v, want %q", got, "conv-new")
	}
}

func TestBuildWorkflowStartedEventPayload_ConversationWorkflowPreservesExistingConversationID(t *testing.T) {
	systemInputs := map[string]interface{}{
		"sys.conversation_id": "conv-existing",
	}

	payload := buildWorkflowStartedEventPayload(
		"CONVERSATION_WORKFLOW",
		"run-789",
		"workflow-456",
		2,
		systemInputs,
		1700000001,
	)

	if got := payload["conversation_id"]; got != "conv-existing" {
		t.Fatalf("conversation_id = %v, want %q", got, "conv-existing")
	}
	if got := payload["message_id"]; got != "run-789" {
		t.Fatalf("message_id = %v, want %q", got, "run-789")
	}
}

func TestBuildWorkflowStartedEventPayload_WorkflowDoesNotIncludeConversationFields(t *testing.T) {
	systemInputs := map[string]interface{}{
		"sys.conversation_id": "conv-should-stay-nested-only",
	}

	payload := buildWorkflowStartedEventPayload(
		"WORKFLOW",
		"run-321",
		"workflow-654",
		3,
		systemInputs,
		1700000002,
	)

	if got := payload["id"]; got != "run-321" {
		t.Fatalf("id = %v, want %q", got, "run-321")
	}
	if got := payload["workflow_id"]; got != "workflow-654" {
		t.Fatalf("workflow_id = %v, want %q", got, "workflow-654")
	}
	if _, exists := payload["conversation_id"]; exists {
		t.Fatalf("conversation_id should be absent, got %v", payload["conversation_id"])
	}
	if _, exists := payload["message_id"]; exists {
		t.Fatalf("message_id should be absent, got %v", payload["message_id"])
	}
	inputs, ok := payload["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputs type = %T, want map[string]interface{}", payload["inputs"])
	}
	if got := inputs["sys.conversation_id"]; got != "conv-should-stay-nested-only" {
		t.Fatalf("inputs.sys.conversation_id = %v, want %q", got, "conv-should-stay-nested-only")
	}
}

func TestResolveWorkflowPrecheckSubjects_UsesCallerOrganizationForSystemAgent(t *testing.T) {
	organizationID, workspaceID, billingSubjectType := resolveWorkflowPrecheckSubjects(
		context.Background(),
		mockWorkspaceOrganizationResolver{
			organization: &workspace_model.Organization{ID: "org-from-workspace"},
		},
		&agents.Agent{
			ID:       uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			TenantID: uuid.Nil,
		},
		"org-caller",
		"ws-caller",
		"ws-app",
		nil,
	)

	if organizationID != "org-caller" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-caller")
	}
	if workspaceID != "" {
		t.Fatalf("workspaceID = %q, want empty", workspaceID)
	}
	if billingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("billingSubjectType = %q, want %q", billingSubjectType, llmclient.BillingSubjectTypeOrganization)
	}
}

func TestResolveWorkflowPrecheckSubjects_FallsBackToCallerWorkspaceForSystemWebApp(t *testing.T) {
	organizationID, workspaceID, billingSubjectType := resolveWorkflowPrecheckSubjects(
		context.Background(),
		mockWorkspaceOrganizationResolver{
			organization: &workspace_model.Organization{ID: "org-from-workspace"},
		},
		&agents.Agent{
			ID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			TenantID: uuid.Nil,
		},
		"",
		"ws-caller",
		"",
		nil,
	)

	if organizationID != "org-from-workspace" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-from-workspace")
	}
	if workspaceID != "" {
		t.Fatalf("workspaceID = %q, want empty", workspaceID)
	}
	if billingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("billingSubjectType = %q, want %q", billingSubjectType, llmclient.BillingSubjectTypeOrganization)
	}
}

func TestResolveWorkflowPrecheckSubjects_UsesAppWorkspaceForUserAgent(t *testing.T) {
	organizationID, workspaceID, billingSubjectType := resolveWorkflowPrecheckSubjects(
		context.Background(),
		mockWorkspaceOrganizationResolver{
			organization: &workspace_model.Organization{ID: "org-from-app-workspace"},
		},
		&agents.Agent{
			ID:       uuid.MustParse("33333333-3333-3333-3333-333333333333"),
			TenantID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
			Source:   agents.AgentSourceUser,
		},
		"org-caller",
		"ws-caller",
		"ws-app",
		nil,
	)

	if organizationID != "org-from-app-workspace" {
		t.Fatalf("organizationID = %q, want %q", organizationID, "org-from-app-workspace")
	}
	if workspaceID != "ws-app" {
		t.Fatalf("workspaceID = %q, want %q", workspaceID, "ws-app")
	}
	if billingSubjectType != "" {
		t.Fatalf("billingSubjectType = %q, want empty", billingSubjectType)
	}
}

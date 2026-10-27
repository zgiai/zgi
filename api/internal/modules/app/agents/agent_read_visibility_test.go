package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

type stubAgentReadRepository struct {
	AgentsRepository

	published bool
}

func (s *stubAgentReadRepository) HasPublishedAgentVersion(context.Context, string) (bool, error) {
	return s.published, nil
}

func (s *stubAgentReadRepository) HasPublishedWorkflow(context.Context, string) (bool, error) {
	return s.published, nil
}

func TestAuthorizeAgentReadAppliesDraftVisibilityRule(t *testing.T) {
	const (
		organizationID = "88888888-8888-8888-8888-888888888888"
		callerID       = "99999999-9999-9999-9999-999999999999"
	)
	otherCreator := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	caller := uuid.MustParse(callerID)
	view := map[workspace_model.WorkspacePermissionCode]bool{workspace_model.WorkspacePermissionAgentView: true}

	tests := []struct {
		name      string
		agentType string
		createdBy *uuid.UUID
		published bool
		grants    map[workspace_model.WorkspacePermissionCode]bool
		allowed   bool
	}{
		{name: "viewer reads published agent", agentType: "AGENT", createdBy: &otherCreator, published: true, grants: view, allowed: true},
		{name: "viewer cannot read another member's draft", agentType: "AGENT", createdBy: &otherCreator, grants: view},
		{name: "viewer reads own draft", agentType: "AGENT", createdBy: &caller, grants: view, allowed: true},
		{
			name: "editor reads another member's draft", agentType: "AGENT", createdBy: &otherCreator,
			grants:  map[workspace_model.WorkspacePermissionCode]bool{workspace_model.WorkspacePermissionAgentUpdate: true},
			allowed: true,
		},
		{
			name: "agent code does not expose workflow rows", agentType: "WORKFLOW", createdBy: &otherCreator, published: true,
			grants: view,
		},
		{
			name: "workflow code does not expose agent rows", agentType: "AGENT", createdBy: &otherCreator, published: true,
			grants: map[workspace_model.WorkspacePermissionCode]bool{workspace_model.WorkspacePermissionWorkflowView: true},
		},
		{
			name: "workflow viewer cannot read another member's workflow draft", agentType: "WORKFLOW", createdBy: &otherCreator,
			grants: map[workspace_model.WorkspacePermissionCode]bool{workspace_model.WorkspacePermissionWorkflowView: true},
		},
		{
			name: "workflow draft runner reads another member's workflow draft", agentType: "WORKFLOW", createdBy: &otherCreator,
			grants: map[workspace_model.WorkspacePermissionCode]bool{
				workspace_model.WorkspacePermissionWorkflowView:     true,
				workspace_model.WorkspacePermissionWorkflowRunDraft: true,
			},
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &agentsService{
				agentsRepo:        &stubAgentReadRepository{published: tt.published},
				enterpriseService: &stubWebAppStatusOrganizationService{allowedPermissions: tt.grants},
			}
			ag := &Agent{
				ID:         uuid.New(),
				TenantID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
				AgentsType: tt.agentType,
				CreatedBy:  tt.createdBy,
			}

			err := service.authorizeAgentRead(context.Background(), organizationID, callerID, ag)
			if tt.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errAgentReadPermissionDenied)
		})
	}
}

func TestGetAgentFailsClosedWithoutCallerContext(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	orgService := &stubWebAppStatusOrganizationService{allowed: true}
	service := &agentsService{
		agentsRepo: &stubWebAppStatusRepository{
			agent: &Agent{ID: agentID, TenantID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), AgentsType: "AGENT"},
		},
		enterpriseService: orgService,
	}

	ctx := context.WithValue(context.Background(), "account_id", "99999999-9999-9999-9999-999999999999")
	_, err := service.GetAgent(ctx, agentID.String())
	require.EqualError(t, err, "permission denied")
	require.False(t, orgService.checkCalled)
}

func TestApplyAgentVisibilityScopesBuildsPerFamilyPredicate(t *testing.T) {
	db, _, cleanup := openAgentRuntimeSurfacesMockDBWithMock(t)
	defer cleanup()

	scopes := []AgentVisibilityScope{
		{Workflow: false, WorkspaceIDs: []string{"ws-a"}},
		{Workflow: true, WorkspaceIDs: []string{"ws-b"}, DraftWorkspaceIDs: []string{"ws-b"}},
	}
	stmt := applyAgentVisibilityScopes(db.Session(&gorm.Session{DryRun: true}).Model(&Agent{}), scopes, "account-1").
		Find(&[]Agent{}).Statement
	sql := stmt.SQL.String()

	require.Contains(t, sql, "agents.agent_type NOT IN")
	require.Contains(t, sql, "agent_published_versions")
	require.Contains(t, sql, "agents.agent_type IN")
	require.Contains(t, sql, "workflows.version != 'draft'")
	require.Equal(t, 2, strings.Count(sql, "agents.created_by ="))
	require.Contains(t, stmt.Vars, "account-1")

	empty := applyAgentVisibilityScopes(db.Session(&gorm.Session{DryRun: true}).Model(&Agent{}), []AgentVisibilityScope{}, "account-1").
		Find(&[]Agent{}).Statement
	require.Contains(t, empty.SQL.String(), "1 = 0")
}

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	datasource_model "github.com/zgiai/zgi/api/internal/modules/datasource/model"
	datasource_service "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestListAgentSkillCandidatesFiltersByAgentCallerAndSelection(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	agentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountID := "99999999-9999-9999-9999-999999999999"
	creatorID := uuid.MustParse(accountID)

	cfg := &AgentsConfig{AgentsID: agentID}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		EnabledSkillIDs: []string{"chart-generator"},
	}, accountID); err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   agentWorkspaceID,
			AgentsType: "AGENT",
			CreatedBy:  &creatorID,
		},
		config: cfg,
	}
	service := &agentsService{
		agentsRepo:     repo,
		accountService: &stubWebAppStatusAccountService{isEditor: true},
		chatRuntimeService: &stubAgentBindingChatRuntimeService{skills: []skills.SkillDiscoveryMetadata{
			{
				ID:               "chart-generator",
				Name:             "Chart generator",
				Description:      "Generate charts",
				Display:          skills.SkillDisplayMetadata{Label: map[string]string{"en_US": "Chart Generator", "zh_Hans": "Chart Generator Localized"}},
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               "page-only",
				Name:             "Page only",
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAIChat},
			},
			{
				ID:               "broken",
				Name:             "Broken",
				Status:           skills.SkillStatusInvalid,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               skills.SkillAgentDatabase,
				Name:             "Agent database",
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
		}},
	}

	resp, err := service.ListAgentSkillCandidates(context.Background(), agentID.String(), accountID, dto.AgentSkillCandidatesRequest{
		Query:           "chart",
		Limit:           10,
		IncludeSelected: true,
	})
	if err != nil {
		t.Fatalf("ListAgentSkillCandidates() error = %v", err)
	}
	if resp.WorkspaceID != agentWorkspaceID.String() {
		t.Fatalf("WorkspaceID = %q, want agent workspace %q", resp.WorkspaceID, agentWorkspaceID.String())
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].SkillID != "chart-generator" || !resp.Data[0].Selected {
		t.Fatalf("candidate = %#v, want selected chart-generator", resp.Data[0])
	}
	if resp.Data[0].Name != "Chart Generator Localized" {
		t.Fatalf("candidate name = %q, want localized display label", resp.Data[0].Name)
	}

	resp, err = service.ListAgentSkillCandidates(context.Background(), agentID.String(), accountID, dto.AgentSkillCandidatesRequest{
		Limit:           10,
		IncludeSelected: false,
	})
	if err != nil {
		t.Fatalf("ListAgentSkillCandidates(include_selected=false) error = %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("len(Data) = %d, want hidden/invalid/non-agent/selected skills excluded", len(resp.Data))
	}
}

func TestListAgentWorkflowBindingCandidatesUsesAgentWorkspace(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	agentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountID := "99999999-9999-9999-9999-999999999999"
	workflowAgentID := "33333333-3333-3333-3333-333333333333"
	workflowID := "44444444-4444-4444-4444-444444444444"

	cfg := &AgentsConfig{AgentsID: agentID}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		WorkflowBindings: []dto.AgentWorkflowBinding{{
			BindingID:       workflowAgentID,
			Label:           "Approval",
			AgentID:         workflowAgentID,
			WorkflowID:      workflowID,
			VersionStrategy: "latest_published",
		}},
	}, accountID); err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   agentWorkspaceID,
			AgentsType: "AGENT",
		},
		config: cfg,
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: &stubWebAppStatusOrganizationService{allowed: true},
		db:                db,
	}

	columns := []string{"agent_id", "workflow_id", "agent_type", "version_uuid", "version", "graph", "label", "description", "icon", "icon_type", "updated_at"}
	mock.ExpectQuery(`(?s)SELECT .* FROM "workflows" JOIN agents ON agents.id = workflows.agent_id WHERE .*workflows\.tenant_id.*workflows\.version !=.*agents\.deleted_at IS NULL.*ORDER BY workflows\.agent_id ASC, workflows\.created_at DESC`).
		WithArgs(agentWorkspaceID.String(), "draft", AgentWebAppStatusActive, "WORKFLOW", "CONVERSATIONAL_WORKFLOW").
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			workflowAgentID,
			workflowID,
			"WORKFLOW",
			"55555555-5555-5555-5555-555555555555",
			"v1",
			workflowGraphWithStartInput("query"),
			"Approval",
			"Approve requests",
			nil,
			nil,
			time.Now(),
		))

	ctx := context.WithValue(context.Background(), "tenant_id", "runtime-organization")
	resp, err := service.ListAgentWorkflowBindingCandidates(ctx, agentID.String(), accountID, dto.AgentWorkflowBindingCandidatesRequest{
		IncludeSelected:    true,
		IncludeStartInputs: false,
		Limit:              10,
	})
	if err != nil {
		t.Fatalf("ListAgentWorkflowBindingCandidates() error = %v", err)
	}
	if resp.WorkspaceID != agentWorkspaceID.String() {
		t.Fatalf("WorkspaceID = %q, want agent workspace %q", resp.WorkspaceID, agentWorkspaceID.String())
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(resp.Data))
	}
	if !resp.Data[0].Selected {
		t.Fatalf("candidate.Selected = false, want true")
	}
	if len(resp.Data[0].StartInputs) != 0 {
		encoded, _ := json.Marshal(resp.Data[0].StartInputs)
		t.Fatalf("StartInputs = %s, want omitted when IncludeStartInputs=false", encoded)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListAgentDatabaseCandidatesUsesAgentWorkspaceAndFiltersStrays(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	agentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	otherWorkspaceID := "33333333-3333-3333-3333-333333333333"
	accountID := "99999999-9999-9999-9999-999999999999"
	creatorID := uuid.MustParse(accountID)
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   agentWorkspaceID,
			AgentsType: "AGENT",
			CreatedBy:  &creatorID,
		},
		config: &AgentsConfig{AgentsID: agentID},
	}
	dataSources := &stubAgentBindingDataSourceService{
		dataSources: []*dto.DataSourceResponse{
			{
				ID:             "db-in-scope",
				OrganizationID: agentWorkspaceID.String(),
				WorkspaceID:    stringPtr(agentWorkspaceID.String()),
				Name:           "In scope",
				Status:         "active",
				UpdatedAt:      time.Now(),
			},
			{
				ID:             "db-stray",
				OrganizationID: agentWorkspaceID.String(),
				WorkspaceID:    stringPtr(otherWorkspaceID),
				Name:           "Stray",
				Status:         "active",
				UpdatedAt:      time.Now(),
			},
		},
	}
	service := &agentsService{
		agentsRepo:        repo,
		accountService:    &stubWebAppStatusAccountService{isEditor: true},
		enterpriseService: nil,
		dataSourceService: dataSources,
	}

	resp, err := service.ListAgentDatabaseCandidates(context.Background(), agentID.String(), accountID, dto.AgentDatabaseCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListAgentDatabaseCandidates() error = %v", err)
	}
	if len(dataSources.listWorkspaceIDs) != 1 || dataSources.listWorkspaceIDs[0] != agentWorkspaceID.String() {
		t.Fatalf("ListDataSources workspace filter = %#v, want only agent workspace", dataSources.listWorkspaceIDs)
	}
	if resp.WorkspaceID != agentWorkspaceID.String() {
		t.Fatalf("WorkspaceID = %q, want agent workspace %q", resp.WorkspaceID, agentWorkspaceID.String())
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].DataSourceID != "db-in-scope" {
		t.Fatalf("DataSourceID = %q, want db-in-scope", resp.Data[0].DataSourceID)
	}
}

func TestListAgentDatabaseTablesRejectsDatabaseOutsideAgentWorkspace(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	agentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	otherWorkspaceID := "33333333-3333-3333-3333-333333333333"
	accountID := "99999999-9999-9999-9999-999999999999"
	creatorID := uuid.MustParse(accountID)
	repo := &stubWebAppStatusRepository{
		agent: &Agent{
			ID:         agentID,
			TenantID:   agentWorkspaceID,
			AgentsType: "AGENT",
			CreatedBy:  &creatorID,
		},
		config: &AgentsConfig{AgentsID: agentID},
	}
	service := &agentsService{
		agentsRepo:     repo,
		accountService: &stubWebAppStatusAccountService{isEditor: true},
		dataSourceService: &stubAgentBindingDataSourceService{
			dataSourceByID: map[string]*dto.DataSourceResponse{
				"db-stray": {
					ID:             "db-stray",
					OrganizationID: agentWorkspaceID.String(),
					WorkspaceID:    stringPtr(otherWorkspaceID),
					Name:           "Stray",
					Status:         "active",
					UpdatedAt:      time.Now(),
				},
			},
		},
	}

	_, err := service.ListAgentDatabaseTables(context.Background(), agentID.String(), accountID, dto.AgentDatabaseTablesRequest{
		DataSourceID: "db-stray",
		Limit:        10,
	})
	if err == nil {
		t.Fatal("ListAgentDatabaseTables() error = nil, want workspace rejection")
	}
	if !strings.Contains(err.Error(), "not found in agent workspace") {
		t.Fatalf("error = %q, want workspace rejection", err.Error())
	}
}

type stubAgentBindingDataSourceService struct {
	datasource_service.DataSourceService
	dataSources      []*dto.DataSourceResponse
	dataSourceByID   map[string]*dto.DataSourceResponse
	tablesBySourceID map[string][]*datasource_model.Table
	listWorkspaceIDs []string
}

func (s *stubAgentBindingDataSourceService) ListDataSources(_ context.Context, _, _ string, filterWorkspaceIDs []string) ([]*dto.DataSourceResponse, error) {
	s.listWorkspaceIDs = append([]string(nil), filterWorkspaceIDs...)
	return s.dataSources, nil
}

func (s *stubAgentBindingDataSourceService) GetDataSourceByID(_ context.Context, _, id, _ string) (*dto.DataSourceResponse, error) {
	return s.dataSourceByID[id], nil
}

func (s *stubAgentBindingDataSourceService) ListTables(_ context.Context, _, dataSourceID string, _ string) ([]*datasource_model.Table, error) {
	return s.tablesBySourceID[dataSourceID], nil
}

type stubAgentBindingChatRuntimeService struct {
	runtimeservice.Service
	skills []skills.SkillDiscoveryMetadata
}

func (s *stubAgentBindingChatRuntimeService) ListSkills(context.Context, runtimeservice.Scope) ([]skills.SkillDiscoveryMetadata, error) {
	return s.skills, nil
}

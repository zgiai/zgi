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
				ID:               skills.SkillTime,
				Name:             "Time",
				Enabled:          true,
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               "chart-generator",
				Name:             "Chart generator",
				Description:      "Generate charts",
				Display:          skills.SkillDisplayMetadata{Label: map[string]string{"en_US": "Chart Generator", "zh_Hans": "Chart Generator Localized"}},
				Enabled:          true,
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               "page-only",
				Name:             "Page only",
				Enabled:          true,
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAIChat},
			},
			{
				ID:               "broken",
				Name:             "Broken",
				Enabled:          true,
				Status:           skills.SkillStatusInvalid,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               skills.SkillAgentDatabase,
				Name:             "Agent database",
				Enabled:          true,
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
	if len(resp.Data) != 1 || resp.Data[0].SkillID != skills.SkillTime {
		t.Fatalf("Data = %#v, want only the unselected time skill", resp.Data)
	}

	resp, err = service.ListAgentSkillCandidates(context.Background(), agentID.String(), accountID, dto.AgentSkillCandidatesRequest{
		Limit:           10,
		IncludeSelected: true,
	})
	if err != nil {
		t.Fatalf("ListAgentSkillCandidates(include_selected=true) error = %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].SkillID != "chart-generator" || resp.Data[1].SkillID != skills.SkillTime {
		t.Fatalf("Data = %#v, want saved selection first and remaining candidates after it", resp.Data)
	}
}

func TestListAgentSkillCandidatesExcludesRuntimeManagedRouterAndMatchesFileGeneration(t *testing.T) {
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	agentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
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
		chatRuntimeService: &stubAgentBindingChatRuntimeService{skills: []skills.SkillDiscoveryMetadata{
			{
				ID:               skills.SkillIntentRouter,
				Name:             "Intent Router",
				Description:      "Use for file generation routing",
				Enabled:          true,
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               skills.SkillFileGenerator,
				Name:             "File Generator",
				Description:      "Generate downloadable files from provided content",
				WhenToUse:        "Use when the user asks to create or produce a file",
				Enabled:          true,
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
			{
				ID:               "disabled-file-tool",
				Name:             "Disabled file tool",
				Description:      "Generate files",
				Enabled:          false,
				Status:           skills.SkillStatusActive,
				SupportedCallers: []string{runtimemodel.ConversationCallerAgent},
			},
		}},
	}

	resp, err := service.ListAgentSkillCandidates(context.Background(), agentID.String(), accountID, dto.AgentSkillCandidatesRequest{
		Query: "file generation",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListAgentSkillCandidates() error = %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want only file-generator, data = %#v", len(resp.Data), resp.Data)
	}
	if resp.Data[0].SkillID != skills.SkillFileGenerator {
		t.Fatalf("SkillID = %q, want %q", resp.Data[0].SkillID, skills.SkillFileGenerator)
	}
}

func TestFilterAgentWorkflowBindingCandidatesAppliesQueryAndLimit(t *testing.T) {
	items := []dto.AgentWorkflowBindingCandidate{
		{BindingID: "workflow-1", Label: "订单审批", Description: "处理采购订单"},
		{BindingID: "workflow-2", Label: "客户回访", Description: "整理回访结果"},
		{BindingID: "workflow-3", Label: "订单归档", Description: "保存已完成订单"},
	}
	mode := dto.AgentRuntimeModeConfig{
		WorkflowBindings: []dto.AgentWorkflowBinding{{
			BindingID:  "workflow-1",
			AgentID:    "agent-1",
			WorkflowID: "published-workflow-1",
		}},
	}

	got := filterAgentWorkflowBindingCandidates(items, mode, dto.AgentWorkflowBindingCandidatesRequest{
		Query:           "订单",
		Limit:           1,
		IncludeSelected: true,
	})
	if len(got) != 1 {
		t.Fatalf("filterAgentWorkflowBindingCandidates() length = %d, want 1", len(got))
	}
	if got[0].BindingID != "workflow-1" {
		t.Fatalf("filterAgentWorkflowBindingCandidates()[0].BindingID = %q, want %q", got[0].BindingID, "workflow-1")
	}
	if !got[0].Selected {
		t.Fatal("filterAgentWorkflowBindingCandidates()[0].Selected = false, want true")
	}
}

func TestPageAgentWorkflowBindingCandidatesReturnsStablePages(t *testing.T) {
	items := []dto.AgentWorkflowBindingCandidate{
		{BindingID: "workflow-1", Label: "A"},
		{BindingID: "workflow-2", Label: "B"},
		{BindingID: "workflow-3", Label: "C"},
		{BindingID: "workflow-4", Label: "D"},
		{BindingID: "workflow-5", Label: "E"},
	}

	got, total := pageAgentWorkflowBindingCandidates(items, dto.AgentRuntimeModeConfig{}, dto.AgentWorkflowBindingCandidatesRequest{
		Page:            2,
		Limit:           2,
		IncludeSelected: true,
	})
	if total != 5 {
		t.Fatalf("pageAgentWorkflowBindingCandidates() total = %d, want 5", total)
	}
	if len(got) != 2 || got[0].BindingID != "workflow-3" || got[1].BindingID != "workflow-4" {
		t.Fatalf("pageAgentWorkflowBindingCandidates() page 2 = %#v, want workflow-3 and workflow-4", got)
	}

	got, total = pageAgentWorkflowBindingCandidates(items, dto.AgentRuntimeModeConfig{}, dto.AgentWorkflowBindingCandidatesRequest{
		Page:            99999,
		Limit:           2,
		IncludeSelected: true,
	})
	if total != 5 || len(got) != 0 {
		t.Fatalf("pageAgentWorkflowBindingCandidates() out-of-range = %#v, total %d; want empty page and total 5", got, total)
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
	mock.ExpectQuery(`(?s)WITH ranked_workflows AS .*SELECT COUNT\(\*\) FROM ranked_workflows WHERE row_number = 1`).
		WithArgs(agentWorkspaceID.String(), "draft", AgentWebAppStatusActive, "WORKFLOW", "CONVERSATIONAL_WORKFLOW").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)WITH ranked_workflows AS .*SELECT agent_id, workflow_id.*FROM ranked_workflows WHERE row_number = 1.*ORDER BY CASE WHEN LOWER\(agent_id::text\) IN.*LIMIT.*OFFSET`).
		WithArgs(agentWorkspaceID.String(), "draft", AgentWebAppStatusActive, "WORKFLOW", "CONVERSATIONAL_WORKFLOW", workflowAgentID, 10, 0).
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
	if resp.Page != 1 || resp.Total != 1 || resp.HasMore {
		t.Fatalf("pagination = page %d total %d has_more %v, want page 1 total 1 has_more false", resp.Page, resp.Total, resp.HasMore)
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
	db, mock := newRunnableWebAppsMockDB(t)
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	agentWorkspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountID := "99999999-9999-9999-9999-999999999999"
	creatorID := uuid.MustParse(accountID)
	selectedDatabaseID := "44444444-4444-4444-4444-444444444444"
	cfg := &AgentsConfig{AgentsID: agentID}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		DatabaseBindings: []dto.AgentDatabaseBinding{{
			DataSourceID: selectedDatabaseID,
			TableIDs:     []string{"55555555-5555-5555-5555-555555555555"},
		}}}, accountID); err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	if mode := agentRuntimeModeFromConfig(cfg); len(mode.DatabaseBindings) != 1 {
		t.Fatalf("persisted database bindings = %#v, want one binding", mode.DatabaseBindings)
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
		db:             db,
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM data_sources AS ds WHERE \(ds.organization_id = \$1 AND ds.workspace_id = \$2\) AND \(\(ds.id IN \(\$3\) OR EXISTS`).
		WithArgs(agentWorkspaceID.String(), agentWorkspaceID.String(), selectedDatabaseID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT ds.id AS data_source_id.*FROM data_sources AS ds.*ORDER BY CASE WHEN ds.id IN.*LOWER\(ds.name\) ASC, ds.id ASC LIMIT`).
		WithArgs(
			agentWorkspaceID.String(),
			agentWorkspaceID.String(),
			selectedDatabaseID,
			selectedDatabaseID,
			10,
		).
		WillReturnRows(sqlmock.NewRows([]string{
			"data_source_id", "name", "description", "status", "workspace_id", "icon", "icon_type", "icon_background", "updated_at", "table_count",
		}).AddRow(
			selectedDatabaseID, "In scope", "", "active", agentWorkspaceID.String(), nil, nil, nil, time.Now(), 2,
		))

	resp, err := service.ListAgentDatabaseCandidates(context.Background(), agentID.String(), accountID, dto.AgentDatabaseCandidatesRequest{
		Limit:           10,
		AvailableOnly:   true,
		IncludeSelected: true,
	})
	if err != nil {
		t.Fatalf("ListAgentDatabaseCandidates() error = %v", err)
	}
	if resp.WorkspaceID != agentWorkspaceID.String() {
		t.Fatalf("WorkspaceID = %q, want agent workspace %q", resp.WorkspaceID, agentWorkspaceID.String())
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].DataSourceID != selectedDatabaseID || !resp.Data[0].Selected || resp.Data[0].TableCount != 2 {
		t.Fatalf("candidate = %#v, want selected database with two tables", resp.Data[0])
	}
	if resp.Page != 1 || resp.Total != 1 || resp.HasMore {
		t.Fatalf("pagination = page %d total %d has_more %v, want page 1 total 1 has_more false", resp.Page, resp.Total, resp.HasMore)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
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

func TestListAgentDatabaseTablesPaginatesWithPersistedSelectionFirst(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	agentID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	workspaceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	accountID := "99999999-9999-9999-9999-999999999999"
	creatorID := uuid.MustParse(accountID)
	dataSourceID := "44444444-4444-4444-4444-444444444444"
	selectedTableID := "55555555-5555-5555-5555-555555555555"
	cfg := &AgentsConfig{AgentsID: agentID}
	if _, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		DatabaseBindings: []dto.AgentDatabaseBinding{{
			DataSourceID: dataSourceID,
			TableIDs:     []string{selectedTableID},
		}},
	}, accountID); err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	service := &agentsService{
		agentsRepo: &stubWebAppStatusRepository{
			agent: &Agent{
				ID:         agentID,
				TenantID:   workspaceID,
				AgentsType: "AGENT",
				CreatedBy:  &creatorID,
			},
			config: cfg,
		},
		accountService: &stubWebAppStatusAccountService{isEditor: true},
		dataSourceService: &stubAgentBindingDataSourceService{
			dataSourceByID: map[string]*dto.DataSourceResponse{
				dataSourceID: {
					ID:             dataSourceID,
					OrganizationID: workspaceID.String(),
					WorkspaceID:    stringPtr(workspaceID.String()),
					Name:           "In scope",
					Status:         "active",
					UpdatedAt:      time.Now(),
				},
			},
		},
		db: db,
	}
	mock.ExpectQuery(`SELECT count\(\*\) FROM data_source_tables AS dst WHERE dst.data_source_id = \$1`).
		WithArgs(dataSourceID).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`(?s)SELECT dst.id AS table_id.*FROM data_source_tables AS dst.*ORDER BY CASE WHEN dst.id IN.*LOWER\(dst.name\) ASC, dst.id ASC LIMIT`).
		WithArgs(dataSourceID, selectedTableID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"table_id", "data_source_id", "name", "description", "physical_table_name", "updated_at",
		}).AddRow(selectedTableID, dataSourceID, "Selected", "", "selected_table", time.Now()))

	resp, err := service.ListAgentDatabaseTables(context.Background(), agentID.String(), accountID, dto.AgentDatabaseTablesRequest{
		DataSourceID:    dataSourceID,
		Page:            1,
		Limit:           1,
		IncludeSelected: true,
	})
	if err != nil {
		t.Fatalf("ListAgentDatabaseTables() error = %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].TableID != selectedTableID || !resp.Data[0].Selected {
		t.Fatalf("Data = %#v, want persisted selection first", resp.Data)
	}
	if resp.Total != 2 || !resp.HasMore {
		t.Fatalf("pagination = total %d has_more %v, want total 2 has_more true", resp.Total, resp.HasMore)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
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

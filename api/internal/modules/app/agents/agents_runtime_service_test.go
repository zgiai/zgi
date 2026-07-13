package agents

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizeAgentEnabledSkillIDsRemovesRuntimeManagedSkills(t *testing.T) {
	got := normalizeAgentEnabledSkillIDs([]string{
		skills.SkillAgentKnowledge,
		skills.SkillAgentWorkflow,
		skills.SkillUserMemory,
		skills.SkillIntentRouter,
		skills.SkillCalculator,
		skills.SkillCalculator,
		"  time  ",
		"",
	})
	want := []string{skills.SkillCalculator, skills.SkillTime}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeAgentEnabledSkillIDs() = %#v, want %#v", got, want)
	}
}

func TestGetPublishedAgentRuntimeConfigByAgentID(t *testing.T) {
	agentID := uuid.New()
	workspaceID := uuid.New()
	versionUUID := uuid.New()
	repo := &publishedRuntimeRepo{
		agent: &Agent{
			ID:           agentID,
			TenantID:     workspaceID,
			Name:         "Published Agent",
			AgentsType:   "AGENT",
			WebAppID:     uuid.New(),
			WebAppStatus: AgentWebAppStatusInactive,
		},
		version: &AgentPublishedVersion{
			ID:          uuid.New(),
			AgentID:     agentID,
			WorkspaceID: workspaceID,
			Version:     "v1",
			VersionUUID: versionUUID,
			ConfigSnapshot: map[string]interface{}{
				"system_prompt":     "answer carefully",
				"model_provider":    "openai",
				"model":             "gpt-test",
				"enabled_skill_ids": []string{"time"},
			},
			CreatedAt: time.Now(),
		},
	}
	service := &agentsService{agentsRepo: repo}

	got, err := service.GetPublishedAgentRuntimeConfig(context.Background(), agentID.String())
	if err != nil {
		t.Fatalf("GetPublishedAgentRuntimeConfig() error = %v", err)
	}
	if got.AgentID != agentID.String() || got.WorkspaceID != workspaceID.String() || got.OrganizationID != workspaceID.String() {
		t.Fatalf("scope fields = agent:%q workspace:%q organization:%q", got.AgentID, got.WorkspaceID, got.OrganizationID)
	}
	if got.Version != "v1" || got.VersionUUID != versionUUID.String() {
		t.Fatalf("version fields = %q/%q, want v1/%s", got.Version, got.VersionUUID, versionUUID)
	}
	if got.Config.Model != "gpt-test" || got.Config.SystemPrompt != "answer carefully" {
		t.Fatalf("config = %#v", got.Config)
	}
}

func TestGetPublishedAgentRuntimeConfigRejectsNonAgentRuntime(t *testing.T) {
	agentID := uuid.New()
	service := &agentsService{agentsRepo: &publishedRuntimeRepo{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.New(),
			Name:       "Workflow App",
			AgentsType: "workflow",
			WebAppID:   uuid.New(),
		},
	}}

	_, err := service.GetPublishedAgentRuntimeConfig(context.Background(), agentID.String())
	if err == nil || !strings.Contains(err.Error(), "not an AGENT runtime") {
		t.Fatalf("error = %v, want non-agent runtime error", err)
	}
}

func TestGetPublishedAgentRuntimeConfigRejectsUnpublishedAgent(t *testing.T) {
	agentID := uuid.New()
	service := &agentsService{agentsRepo: &publishedRuntimeRepo{
		agent: &Agent{
			ID:         agentID,
			TenantID:   uuid.New(),
			Name:       "Draft Only Agent",
			AgentsType: "AGENT",
			WebAppID:   uuid.New(),
		},
	}}

	_, err := service.GetPublishedAgentRuntimeConfig(context.Background(), agentID.String())
	if !errors.Is(err, errAgentWebAppNotPublished) {
		t.Fatalf("error = %v, want errAgentWebAppNotPublished", err)
	}
}

func TestAgentOpeningStatementPersistsThroughDraftAndSnapshot(t *testing.T) {
	cfg := &AgentsConfig{}
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		HomeTitle:        "  Welcome  ",
		OpeningStatement: "  ## Start here\r\n\r\n- Ask a question  ",
	})
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	const wantStatement = "## Start here\n\n- Ask a question"
	if applied.OpeningStatement != wantStatement {
		t.Fatalf("applied OpeningStatement = %q, want %q", applied.OpeningStatement, wantStatement)
	}

	response := agentConfigResponse("agent-1", cfg)
	if response.OpeningStatement != wantStatement {
		t.Fatalf("response OpeningStatement = %q, want %q", response.OpeningStatement, wantStatement)
	}

	snapshot := agentConfigSnapshot("agent-1", cfg)
	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if fromSnapshot.OpeningStatement != wantStatement {
		t.Fatalf("snapshot OpeningStatement = %q, want %q", fromSnapshot.OpeningStatement, wantStatement)
	}
}

type publishedRuntimeRepo struct {
	AgentsRepository
	agent   *Agent
	version *AgentPublishedVersion
}

func (r *publishedRuntimeRepo) GetByID(_ context.Context, id string) (*Agent, error) {
	if r.agent == nil || r.agent.ID.String() != id {
		return nil, errors.New("agent not found")
	}
	return r.agent, nil
}

func (r *publishedRuntimeRepo) GetLatestAgentPublishedVersion(context.Context, string) (*AgentPublishedVersion, error) {
	return r.version, nil
}

func TestApplyAgentConfigRequestPersistsWorkflowBindings(t *testing.T) {
	cfg := &AgentsConfig{}
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		WorkflowBindings: []dto.AgentWorkflowBinding{
			{BindingID: " Approval-Flow ", Label: " Approval ", Description: " Needs approval ", AgentID: " Agent-1 ", WorkflowID: " Workflow-1 ", VersionStrategy: "latest_published", TimeoutSeconds: 5},
			{BindingID: "approval-flow", Label: "Approval v2", AgentID: "agent-1", WorkflowID: "workflow-1", VersionStrategy: "pinned", VersionUUID: " Version-1 "},
			{BindingID: "missing-agent", WorkflowID: "workflow-2"},
		},
	}, "binder-1")
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	want := []dto.AgentWorkflowBinding{{
		BindingID:       "approval-flow",
		Label:           "Approval v2",
		AgentID:         "agent-1",
		WorkflowID:      "workflow-1",
		VersionStrategy: "pinned",
		VersionUUID:     "version-1",
	}}
	if !reflect.DeepEqual(applied.WorkflowBindings, want) {
		t.Fatalf("applied WorkflowBindings = %#v, want %#v", applied.WorkflowBindings, want)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if !reflect.DeepEqual(mode.WorkflowBindings, want) {
		t.Fatalf("mode WorkflowBindings = %#v, want %#v", mode.WorkflowBindings, want)
	}
	if mode.WorkflowBoundByAccountID != "binder-1" || mode.WorkflowBoundAtUnix <= 0 {
		t.Fatalf("workflow grant = %q/%d, want binder-1 with timestamp", mode.WorkflowBoundByAccountID, mode.WorkflowBoundAtUnix)
	}
	resp := agentConfigResponse("agent-1", cfg)
	if !reflect.DeepEqual(resp.WorkflowBindings, want) {
		t.Fatalf("response WorkflowBindings = %#v, want %#v", resp.WorkflowBindings, want)
	}
	snapshot := agentConfigSnapshot("agent-1", cfg)
	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if !reflect.DeepEqual(fromSnapshot.WorkflowBindings, want) {
		t.Fatalf("snapshot WorkflowBindings = %#v, want %#v", fromSnapshot.WorkflowBindings, want)
	}
	if fromSnapshot.WorkflowBoundByAccountID != "binder-1" || fromSnapshot.WorkflowBoundAtUnix <= 0 {
		t.Fatalf("snapshot workflow grant = %q/%d, want binder-1 with timestamp", fromSnapshot.WorkflowBoundByAccountID, fromSnapshot.WorkflowBoundAtUnix)
	}
}

func TestHydrateAgentWorkflowBindingRuntimeInputsUsesPinnedVersionSchema(t *testing.T) {
	db, mock := newRunnableWebAppsMockDB(t)
	service := &agentsService{db: db}
	workspaceID := "11111111-1111-1111-1111-111111111111"
	agentID := "22222222-2222-2222-2222-222222222222"
	latestWorkflowID := "33333333-3333-3333-3333-333333333333"
	pinnedWorkflowID := "44444444-4444-4444-4444-444444444444"
	pinnedVersionUUID := "55555555-5555-5555-5555-555555555555"

	columns := []string{"agent_id", "workflow_id", "agent_type", "version_uuid", "version", "graph", "label", "description", "icon", "icon_type", "updated_at"}
	mock.ExpectQuery(`(?s)SELECT .* FROM "workflows" JOIN agents ON agents.id = workflows.agent_id WHERE .*workflows\.tenant_id.*workflows\.version !=.*agents\.deleted_at IS NULL.*ORDER BY workflows\.agent_id ASC, workflows\.created_at DESC`).
		WithArgs(workspaceID, "draft", AgentWebAppStatusActive, "WORKFLOW", "CONVERSATIONAL_WORKFLOW").
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			agentID,
			latestWorkflowID,
			"WORKFLOW",
			"latest-version",
			"v2",
			workflowGraphWithStartInput("new_input"),
			"Task workflow",
			"description",
			nil,
			nil,
			time.Now(),
		))
	mock.ExpectQuery(`(?s)SELECT .* FROM "workflows" JOIN agents ON agents.id = workflows.agent_id WHERE .*workflows\.tenant_id.*workflows\.agent_id.*workflows\.id.*workflows\.version_uuid.*LIMIT`).
		WithArgs(workspaceID, agentID, pinnedWorkflowID, pinnedVersionUUID, "draft", AgentWebAppStatusActive, "WORKFLOW", "CONVERSATIONAL_WORKFLOW", 1).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(
			agentID,
			pinnedWorkflowID,
			"WORKFLOW",
			pinnedVersionUUID,
			"v1",
			workflowGraphWithStartInput("old_input"),
			"Task workflow",
			"description",
			nil,
			nil,
			time.Now().Add(-time.Hour),
		))

	hydrated := service.hydrateAgentWorkflowBindingRuntimeInputs(t.Context(), workspaceID, []dto.AgentWorkflowBinding{{
		BindingID:       agentID,
		Label:           "Task workflow",
		AgentID:         agentID,
		WorkflowID:      pinnedWorkflowID,
		VersionStrategy: "pinned",
		VersionUUID:     pinnedVersionUUID,
	}})

	if len(hydrated) != 1 || len(hydrated[0].StartInputs) != 1 {
		t.Fatalf("hydrated bindings = %#v, want one start input", hydrated)
	}
	if got := hydrated[0].StartInputs[0].Variable; got != "old_input" {
		t.Fatalf("hydrated start input = %q, want pinned old_input", got)
	}
	if got := hydrated[0].DefaultInputKey; got != "old_input" {
		t.Fatalf("default input key = %q, want old_input", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func workflowGraphWithStartInput(variable string) string {
	return `{"nodes":[{"data":{"type":"start","variables":[{"variable":"` + variable + `","label":"` + variable + `","type":"string","required":true}]}}]}`
}

func TestAgentMemoryReplaceRequestPreservesInvalidRowsForValidation(t *testing.T) {
	req := agentMemoryReplaceRequestFromConfig([]dto.AgentMemorySlotConfig{
		{Key: "profile", Enabled: true},
		{Key: "", Enabled: true},
		{Key: "profile", Enabled: true},
	}, true)
	if len(req.Slots) != 3 {
		t.Fatalf("len(req.Slots) = %d, want 3", len(req.Slots))
	}
	if req.Slots[1].Key != "" {
		t.Fatalf("empty key row was changed to %q", req.Slots[1].Key)
	}
	if req.Slots[2].Key != "profile" {
		t.Fatalf("duplicate key row was changed to %q", req.Slots[2].Key)
	}
}

func TestAgentMemoryReplaceRequestCanDropHistoricalIDsForRollback(t *testing.T) {
	req := agentMemoryReplaceRequestFromConfig([]dto.AgentMemorySlotConfig{
		{ID: "stale-slot-id", Key: "profile", Enabled: true},
	}, false)
	if len(req.Slots) != 1 {
		t.Fatalf("len(req.Slots) = %d, want 1", len(req.Slots))
	}
	if req.Slots[0].ID != "" {
		t.Fatalf("rollback request preserved historical id %q", req.Slots[0].ID)
	}
	if req.Slots[0].Key != "profile" {
		t.Fatalf("rollback request key = %q, want profile", req.Slots[0].Key)
	}
}

func TestAgentMemorySnapshotSlotsDoNotPersistVolatileIDs(t *testing.T) {
	slots := agentMemorySnapshotSlots([]dto.AgentMemorySlotConfig{
		{
			ID:          "draft-slot-id",
			Key:         "profile",
			Description: "User profile",
			MaxChars:    4000,
			Enabled:     true,
			SortOrder:   3,
			CreatedAt:   123,
			UpdatedAt:   456,
		},
	})
	if len(slots) != 1 {
		t.Fatalf("len(slots) = %d, want 1", len(slots))
	}
	if slots[0].ID != "" {
		t.Fatalf("snapshot preserved slot id %q", slots[0].ID)
	}
	if slots[0].CreatedAt != 0 || slots[0].UpdatedAt != 0 {
		t.Fatalf("snapshot preserved timestamps created=%d updated=%d", slots[0].CreatedAt, slots[0].UpdatedAt)
	}
	if slots[0].MaxChars != 2000 {
		t.Fatalf("snapshot max chars = %d, want 2000", slots[0].MaxChars)
	}
}

func TestApplyAgentConfigRequestToDraftDoesNotPersistDraftMemorySlots(t *testing.T) {
	cfg := &AgentsConfig{}
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		AgentMemoryEnabled: true,
	})
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	if !applied.AgentMemoryEnabled {
		t.Fatal("applied AgentMemoryEnabled = false, want true")
	}
	if cfg.AgentMode == nil {
		t.Fatal("AgentMode = nil, want serialized runtime mode")
	}
	if strings.Contains(*cfg.AgentMode, "agent_memory_slots") {
		t.Fatalf("AgentMode = %s, should not persist draft memory slots", *cfg.AgentMode)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if !mode.AgentMemoryEnabled {
		t.Fatal("mode AgentMemoryEnabled = false, want true")
	}
}

func TestApplyAgentConfigRequestPersistsDatabaseBindings(t *testing.T) {
	cfg := &AgentsConfig{}
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: " db-1 ", TableIDs: []string{"table-2", "table-1", "table-1"}, WritableTableIDs: []string{"table-2", "table-missing", "table-2"}},
			{DataSourceID: "db-empty", TableIDs: []string{}},
		},
	})
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	want := []dto.AgentDatabaseBinding{{DataSourceID: "db-1", TableIDs: []string{"table-1", "table-2"}, WritableTableIDs: []string{"table-2"}}}
	if !reflect.DeepEqual(applied.DatabaseBindings, want) {
		t.Fatalf("applied DatabaseBindings = %#v, want %#v", applied.DatabaseBindings, want)
	}
	if cfg.AgentMode == nil {
		t.Fatal("AgentMode = nil, want serialized runtime mode")
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if !reflect.DeepEqual(mode.DatabaseBindings, want) {
		t.Fatalf("mode DatabaseBindings = %#v, want %#v", mode.DatabaseBindings, want)
	}
	resp := agentConfigResponse("agent-1", cfg)
	if !reflect.DeepEqual(resp.DatabaseBindings, want) {
		t.Fatalf("response DatabaseBindings = %#v, want %#v", resp.DatabaseBindings, want)
	}
	snapshot := agentConfigSnapshot("agent-1", cfg)
	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if !reflect.DeepEqual(fromSnapshot.DatabaseBindings, want) {
		t.Fatalf("snapshot DatabaseBindings = %#v, want %#v", fromSnapshot.DatabaseBindings, want)
	}
}

func TestAgentConfigSnapshotPreservesSupportsVision(t *testing.T) {
	model := "qwen-vl-plus"
	cfg := &AgentsConfig{
		ModelVersionID: &model,
	}

	snapshot := agentConfigSnapshot("agent-1", cfg)
	if snapshot["supports_vision"] != true {
		t.Fatalf("snapshot supports_vision = %#v, want true", snapshot["supports_vision"])
	}

	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if !fromSnapshot.SupportsVision {
		t.Fatal("fromSnapshot SupportsVision = false, want true")
	}
}

func TestApplyAgentConfigRequestRecordsBindingActor(t *testing.T) {
	cfg := &AgentsConfig{}
	_, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs: []string{"dataset-1"},
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1"}},
		},
	}, "binder-1")
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "binder-1" {
		t.Fatalf("KnowledgeBoundByAccountID = %q, want binder-1", mode.KnowledgeBoundByAccountID)
	}
	if mode.KnowledgeBoundAtUnix <= 0 {
		t.Fatalf("KnowledgeBoundAtUnix = %d, want positive", mode.KnowledgeBoundAtUnix)
	}
	if mode.DatabaseBoundByAccountID != "binder-1" {
		t.Fatalf("DatabaseBoundByAccountID = %q, want binder-1", mode.DatabaseBoundByAccountID)
	}
	if mode.DatabaseBoundAtUnix <= 0 {
		t.Fatalf("DatabaseBoundAtUnix = %d, want positive", mode.DatabaseBoundAtUnix)
	}
	snapshot := agentConfigSnapshot("agent-1", cfg)
	fromSnapshot := agentConfigResponseFromSnapshot("agent-1", snapshot)
	if fromSnapshot.KnowledgeBoundByAccountID != "binder-1" || fromSnapshot.DatabaseBoundByAccountID != "binder-1" || fromSnapshot.KnowledgeBoundAtUnix <= 0 || fromSnapshot.DatabaseBoundAtUnix <= 0 {
		t.Fatalf("snapshot binding actors: knowledge=%q/%d database=%q/%d, want binder-1 with timestamps", fromSnapshot.KnowledgeBoundByAccountID, fromSnapshot.KnowledgeBoundAtUnix, fromSnapshot.DatabaseBoundByAccountID, fromSnapshot.DatabaseBoundAtUnix)
	}
	firstKnowledgeBoundAt := mode.KnowledgeBoundAtUnix
	firstDatabaseBoundAt := mode.DatabaseBoundAtUnix

	_, err = applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs: []string{"dataset-1"},
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1"}},
		},
		SystemPrompt: "updated prompt only",
	}, "editor-2")
	if err != nil {
		t.Fatalf("second applyAgentConfigRequestToDraft() error = %v", err)
	}
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal updated AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "binder-1" || mode.DatabaseBoundByAccountID != "binder-1" || mode.KnowledgeBoundAtUnix != firstKnowledgeBoundAt || mode.DatabaseBoundAtUnix != firstDatabaseBoundAt {
		t.Fatalf("binding grants changed on unrelated save: knowledge=%q/%d database=%q/%d", mode.KnowledgeBoundByAccountID, mode.KnowledgeBoundAtUnix, mode.DatabaseBoundByAccountID, mode.DatabaseBoundAtUnix)
	}

	_, err = applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs: []string{"dataset-1", "dataset-2"},
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1", "table-2"}},
		},
	}, "editor-2")
	if err != nil {
		t.Fatalf("third applyAgentConfigRequestToDraft() error = %v", err)
	}
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal changed AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "editor-2" || mode.DatabaseBoundByAccountID != "editor-2" {
		t.Fatalf("binding actors after binding change: knowledge=%q database=%q, want editor-2", mode.KnowledgeBoundByAccountID, mode.DatabaseBoundByAccountID)
	}
}

func TestApplyAgentConfigRequestCanRestoreSnapshotBindingGrant(t *testing.T) {
	cfg := &AgentsConfig{}
	_, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		KnowledgeDatasetIDs:       []string{"dataset-1"},
		KnowledgeBoundByAccountID: "snapshot-binder",
		KnowledgeBoundAtUnix:      123,
		DatabaseBindings: []dto.AgentDatabaseBinding{
			{DataSourceID: "db-1", TableIDs: []string{"table-1"}, WritableTableIDs: []string{"table-1"}},
		},
		DatabaseBoundByAccountID: "snapshot-binder",
		DatabaseBoundAtUnix:      456,
	}, "rollback-user")
	if err != nil {
		t.Fatalf("applyAgentConfigRequestToDraft() error = %v", err)
	}
	var mode dto.AgentRuntimeModeConfig
	if err := json.Unmarshal([]byte(*cfg.AgentMode), &mode); err != nil {
		t.Fatalf("unmarshal AgentMode error = %v", err)
	}
	if mode.KnowledgeBoundByAccountID != "snapshot-binder" || mode.KnowledgeBoundAtUnix != 123 {
		t.Fatalf("knowledge grant = %q/%d, want snapshot-binder/123", mode.KnowledgeBoundByAccountID, mode.KnowledgeBoundAtUnix)
	}
	if mode.DatabaseBoundByAccountID != "snapshot-binder" || mode.DatabaseBoundAtUnix != 456 {
		t.Fatalf("database grant = %q/%d, want snapshot-binder/456", mode.DatabaseBoundByAccountID, mode.DatabaseBoundAtUnix)
	}
	if len(mode.DatabaseBindings) != 1 || !reflect.DeepEqual(mode.DatabaseBindings[0].WritableTableIDs, []string{"table-1"}) {
		t.Fatalf("database bindings = %#v, want writable table-1", mode.DatabaseBindings)
	}
}

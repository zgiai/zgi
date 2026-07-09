package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"gorm.io/gorm"
)

type mockWorkflowNodeRuntimeLogRepo struct {
	statusID               string
	statusValue            string
	statusFinishedAt       *time.Time
	updateOutputsID        string
	updateOutputsValue     *string
	updateProcessDataValue *string
	updateMetadataValue    *string
	updateElapsedTime      float64
	logsByWorkflowRunID    []WorkflowNodeRuntimeLog
	logByNodeExecutionID   *WorkflowNodeRuntimeLog
	createdLogs            []WorkflowNodeRuntimeLog
}

func (m *mockWorkflowNodeRuntimeLogRepo) Create(ctx context.Context, log *WorkflowNodeRuntimeLog) error {
	if log.ID == "" {
		log.ID = "node-log-1"
	}
	m.createdLogs = append(m.createdLogs, *log)
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetByID(ctx context.Context, id string) (*WorkflowNodeRuntimeLog, error) {
	return &WorkflowNodeRuntimeLog{ID: id}, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) Update(ctx context.Context, log *WorkflowNodeRuntimeLog) error {
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetByWorkflowRunID(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error) {
	return m.logsByWorkflowRunID, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetByNodeExecutionID(ctx context.Context, nodeExecutionID string) (*WorkflowNodeRuntimeLog, error) {
	if m.logByNodeExecutionID == nil {
		return nil, nil
	}
	return m.logByNodeExecutionID, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetPaginatedLogs(ctx context.Context, filter WorkflowNodeRuntimeLogFilter, page, limit int) ([]WorkflowNodeRuntimeLog, int64, error) {
	return nil, 0, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) UpdateStatus(ctx context.Context, id string, status string, finishedAt *time.Time) error {
	m.statusID = id
	m.statusValue = status
	m.statusFinishedAt = finishedAt
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) UpdateOutputsAndMetadata(ctx context.Context, id string, outputs, processData, executionMetadata *string, elapsedTime float64) error {
	m.updateOutputsID = id
	m.updateOutputsValue = outputs
	m.updateProcessDataValue = processData
	m.updateMetadataValue = executionMetadata
	m.updateElapsedTime = elapsedTime
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) UpdateDiagnosisResult(ctx context.Context, id string, result, model string, tokens, latencyMs int, isLLMDiagnosed bool) error {
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) UpdateDiagnosisYaml(ctx context.Context, id string, errorType, errorStack, result, model string, tokens, latencyMs int, isLLMDiagnosed bool, nodeYAML, upstreamYAML, inputSnapshot, upstreamOutputs string) error {
	return nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetByAgentAndWorkflow(ctx context.Context, agentID, workflowID string, page, limit int) ([]WorkflowNodeRuntimeLog, int64, error) {
	return nil, 0, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetExecutionPath(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error) {
	return nil, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) GetNextIndex(ctx context.Context, tenantID, agentID, workflowID, triggeredFrom string, workflowRunID *string) (int, error) {
	return 0, nil
}

func (m *mockWorkflowNodeRuntimeLogRepo) MigrateNodeRuntimeLogsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	return 0, nil
}

type mockWorkflowRepository struct {
	draft *Workflow
}

func (m *mockWorkflowRepository) Create(ctx context.Context, workflow *Workflow) error {
	return nil
}

func (m *mockWorkflowRepository) GetByID(ctx context.Context, id string) (*Workflow, error) {
	return m.draft, nil
}

func (m *mockWorkflowRepository) Update(ctx context.Context, workflow *Workflow) error {
	return nil
}

func (m *mockWorkflowRepository) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockWorkflowRepository) GetByAppID(ctx context.Context, appID string) (*Workflow, error) {
	return m.draft, nil
}

func (m *mockWorkflowRepository) GetByAgentID(ctx context.Context, tenantID, agentID string) (*Workflow, error) {
	return m.draft, nil
}

func (m *mockWorkflowRepository) GetByTenantID(ctx context.Context, tenantID string) ([]Workflow, error) {
	if m.draft == nil {
		return nil, nil
	}
	return []Workflow{*m.draft}, nil
}

func (m *mockWorkflowRepository) GetPaginatedWorkflows(ctx context.Context, filter WorkflowFilter, page, limit int) ([]Workflow, int64, error) {
	if m.draft == nil {
		return nil, 0, nil
	}
	return []Workflow{*m.draft}, 1, nil
}

func (m *mockWorkflowRepository) GetDraftWorkflow(ctx context.Context, appID string) (*Workflow, error) {
	return m.draft, nil
}

func (m *mockWorkflowRepository) GetPublishedWorkflows(ctx context.Context, appID string) ([]Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepository) GetLatestPublishedWorkflow(ctx context.Context, agentID string) (*Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepository) GetByVersionUUID(ctx context.Context, versionUUID string) (*Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepository) GetLatestPublishedVersion(ctx context.Context, agentID string) (*Workflow, error) {
	return nil, nil
}

func (m *mockWorkflowRepository) CreateWorkflow(ctx context.Context, workflow *Workflow) error {
	return nil
}

func (m *mockWorkflowRepository) GetPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*Workflow, int64, error) {
	return nil, 0, nil
}

type mockWorkflowRunLogRepo struct {
	createErr         error
	createdLogs       []WorkflowRunLog
	statusID          string
	statusValue       string
	outputsID         string
	outputsValue      string
	outputsElapsed    float64
	outputsTotalToken int64
}

func (m *mockWorkflowRunLogRepo) Create(ctx context.Context, log *WorkflowRunLog) error {
	if m.createErr != nil {
		return m.createErr
	}
	if log.ID == "" {
		log.ID = "run-log-1"
	}
	m.createdLogs = append(m.createdLogs, *log)
	return nil
}

func (m *mockWorkflowRunLogRepo) GetByID(ctx context.Context, id string) (*WorkflowRunLog, error) {
	for _, log := range m.createdLogs {
		if log.ID == id {
			return &log, nil
		}
	}
	return &WorkflowRunLog{ID: id}, nil
}

func (m *mockWorkflowRunLogRepo) Update(ctx context.Context, log *WorkflowRunLog) error {
	return nil
}

func (m *mockWorkflowRunLogRepo) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockWorkflowRunLogRepo) GetByAgentID(ctx context.Context, agentID string, page, limit int, triggeredFrom string, appWorkspaceID string, accountID string) ([]WorkflowRunLog, int64, error) {
	return m.createdLogs, int64(len(m.createdLogs)), nil
}

func (m *mockWorkflowRunLogRepo) GetByWorkflowRunID(ctx context.Context, workflowRunID string) (*WorkflowRunLog, error) {
	return m.GetByID(ctx, workflowRunID)
}

func (m *mockWorkflowRunLogRepo) GetPaginatedLogs(ctx context.Context, filter WorkflowRunLogFilter, page, limit int) ([]WorkflowRunLog, int64, error) {
	return m.createdLogs, int64(len(m.createdLogs)), nil
}

func (m *mockWorkflowRunLogRepo) UpdateStatus(ctx context.Context, id string, status string, finishedAt *time.Time) error {
	m.statusID = id
	m.statusValue = status
	return nil
}

func (m *mockWorkflowRunLogRepo) UpdateOutputsAndTokens(ctx context.Context, id string, outputs string, totalTokens int64, elapsedTime float64) error {
	m.outputsID = id
	m.outputsValue = outputs
	m.outputsTotalToken = totalTokens
	m.outputsElapsed = elapsedTime
	return nil
}

func (m *mockWorkflowRunLogRepo) GetNextSequenceNumber(ctx context.Context, tenantID, agentID string) (int, error) {
	return len(m.createdLogs) + 1, nil
}

func (m *mockWorkflowRunLogRepo) GetByAgentAndWorkflowID(ctx context.Context, agentID, workflowID string, page, limit int) ([]WorkflowRunLog, int64, error) {
	return m.createdLogs, int64(len(m.createdLogs)), nil
}

func (m *mockWorkflowRunLogRepo) GetRuntimeLogs(ctx context.Context, filter WorkflowRunLogFilter, page, limit int) ([]WorkflowRunLog, int64, error) {
	return m.createdLogs, int64(len(m.createdLogs)), nil
}

func (m *mockWorkflowRunLogRepo) MigrateWorkflowRunLogsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	return 0, nil
}

func TestRunDraftWorkflowNode_LinksNodeRuntimeLogToSingleStepRun(t *testing.T) {
	graph := `{
		"nodes": [
			{
				"id": "start-node",
				"type": "start",
				"data": {
					"title": "Start",
					"variables": []
				}
			}
		],
		"edges": []
	}`
	nodeLogRepo := &mockWorkflowNodeRuntimeLogRepo{}
	runLogRepo := &mockWorkflowRunLogRepo{}
	service := &WorkflowService{
		executor: NewWorkflowExecutor(),
		repo: &mockWorkflowRepository{
			draft: &Workflow{
				ID:      "workflow-1",
				AgentID: "agent-1",
				Type:    dto.WorkflowTypeWorkflow,
				Version: "draft",
				Graph:   graph,
			},
		},
		workflowRunLogRepo:         runLogRepo,
		workflowNodeRuntimeLogRepo: nodeLogRepo,
	}

	resp, err := service.RunDraftWorkflowNode(
		context.Background(),
		"workspace-1",
		"agent-1",
		"start-node",
		&dto.DraftWorkflowNodeRunRequest{Inputs: map[string]interface{}{"query": "hello"}},
		"account-1",
	)
	if err != nil {
		t.Fatalf("RunDraftWorkflowNode returned error: %v", err)
	}

	response, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response type = %T, want map[string]interface{}", resp)
	}
	if got := response["workflow_run_id"]; got != "run-log-1" {
		t.Fatalf("workflow_run_id = %v, want run-log-1", got)
	}
	if len(nodeLogRepo.createdLogs) != 1 {
		t.Fatalf("created node logs = %d, want 1", len(nodeLogRepo.createdLogs))
	}

	nodeLog := nodeLogRepo.createdLogs[0]
	if nodeLog.WorkflowID != "workflow-1" {
		t.Fatalf("node log workflow id = %q, want workflow-1", nodeLog.WorkflowID)
	}
	if nodeLog.TriggeredFrom != string(WorkflowNodeExecutionTriggeredFromSingleStep) {
		t.Fatalf("node log triggered_from = %q, want %q", nodeLog.TriggeredFrom, WorkflowNodeExecutionTriggeredFromSingleStep)
	}
	if nodeLog.WorkflowRunID == nil || *nodeLog.WorkflowRunID != "run-log-1" {
		t.Fatalf("node log workflow_run_id = %v, want run-log-1", nodeLog.WorkflowRunID)
	}
}

func TestRunDraftWorkflowNode_OmitsWorkflowRunIDWhenRunLogCreateFails(t *testing.T) {
	graph := `{
		"nodes": [
			{
				"id": "start-node",
				"type": "start",
				"data": {
					"title": "Start",
					"variables": []
				}
			}
		],
		"edges": []
	}`
	nodeLogRepo := &mockWorkflowNodeRuntimeLogRepo{}
	runLogRepo := &mockWorkflowRunLogRepo{createErr: errors.New("database unavailable")}
	service := &WorkflowService{
		executor: NewWorkflowExecutor(),
		repo: &mockWorkflowRepository{
			draft: &Workflow{
				ID:      "workflow-1",
				AgentID: "agent-1",
				Type:    dto.WorkflowTypeWorkflow,
				Version: "draft",
				Graph:   graph,
			},
		},
		workflowRunLogRepo:         runLogRepo,
		workflowNodeRuntimeLogRepo: nodeLogRepo,
	}

	resp, err := service.RunDraftWorkflowNode(
		context.Background(),
		"workspace-1",
		"agent-1",
		"start-node",
		&dto.DraftWorkflowNodeRunRequest{Inputs: map[string]interface{}{"query": "hello"}},
		"account-1",
	)
	if err != nil {
		t.Fatalf("RunDraftWorkflowNode returned error: %v", err)
	}

	response, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response type = %T, want map[string]interface{}", resp)
	}
	if _, exists := response["workflow_run_id"]; exists {
		t.Fatalf("workflow_run_id should be omitted when run log is not persisted, got %v", response["workflow_run_id"])
	}
	if len(nodeLogRepo.createdLogs) != 1 {
		t.Fatalf("created node logs = %d, want 1", len(nodeLogRepo.createdLogs))
	}
	if nodeLogRepo.createdLogs[0].WorkflowRunID != nil {
		t.Fatalf("node log workflow_run_id = %v, want nil", *nodeLogRepo.createdLogs[0].WorkflowRunID)
	}
}

func TestSingleNodeExecutionStatus_MarksNilResultAsFailed(t *testing.T) {
	status, errorMsg, err := singleNodeExecutionStatus(nil, nil)
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
	if err == nil {
		t.Fatalf("error = nil, want non-nil")
	}
	if !strings.Contains(errorMsg, "empty node result") {
		t.Fatalf("error message = %q, want empty node result", errorMsg)
	}
}

func TestGetNodeConfigFromDatabase_UsesDataTypeForCustomNode(t *testing.T) {
	graph := `{
		"nodes": [
			{
				"id": "llm-node",
				"type": "custom",
				"data": {
					"title": "Generate",
					"type": "llm"
				}
			}
		],
		"edges": []
	}`
	service := &WorkflowService{
		repo: &mockWorkflowRepository{
			draft: &Workflow{
				ID:      "workflow-1",
				AgentID: "agent-1",
				Type:    dto.WorkflowTypeWorkflow,
				Version: "draft",
				Graph:   graph,
			},
		},
	}

	_, nodeType, err := service.getNodeConfigFromDatabase(
		context.Background(),
		"agent-1",
		"llm-node",
	)
	if err != nil {
		t.Fatalf("getNodeConfigFromDatabase returned error: %v", err)
	}
	if nodeType != shared.LLM {
		t.Fatalf("node type = %q, want %q", nodeType, shared.LLM)
	}
}

func TestWorkflowStoredElapsedMilliseconds_ConvertsLegacySeconds(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(68759625 * time.Nanosecond)

	got := workflowStoredElapsedMilliseconds(0.068759625, createdAt, &finishedAt)
	if math.Abs(got-68.8) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 68.8)
	}
}

func TestWorkflowStoredElapsedMilliseconds_KeepsMilliseconds(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(69 * time.Millisecond)

	got := workflowStoredElapsedMilliseconds(68.7, createdAt, &finishedAt)
	if math.Abs(got-68.7) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 68.7)
	}
}

func TestWorkflowStoredElapsedMilliseconds_KeepsMillisecondsAcrossApprovalWait(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(12 * time.Minute)

	got := workflowStoredElapsedMilliseconds(5.9, createdAt, &finishedAt)
	if math.Abs(got-5.9) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 5.9)
	}
}

func TestWorkflowStoredElapsedMilliseconds_KeepsSubMillisecondWithoutFinishedAt(t *testing.T) {
	got := workflowStoredElapsedMilliseconds(0.7, time.Time{}, nil)
	if math.Abs(got-0.7) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 0.7)
	}
}

func TestWorkflowStoredElapsedMilliseconds_ConvertsMultiSecondLegacyWhenCloseToWallClock(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(2500 * time.Millisecond)

	got := workflowStoredElapsedMilliseconds(2.5, createdAt, &finishedAt)
	if math.Abs(got-2500) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 2500.0)
	}
}

func TestWorkflowRunElapsedMilliseconds_ConvertsSubSecondLegacyRunLog(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(68759625 * time.Nanosecond)

	got := workflowRunElapsedMilliseconds(WorkflowRunLog{
		ElapsedTime: 0.068759625,
		CreatedAt:   createdAt,
		FinishedAt:  &finishedAt,
	})
	if math.Abs(got-68.8) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 68.8)
	}
}

func TestEnsureWorkflowSystemInputsAddsDraftExecutionContext(t *testing.T) {
	inputs := map[string]interface{}{
		"input1":    "hello",
		"sys.query": "hello",
	}

	ensureWorkflowSystemInputs(inputs, "workspace-1", "agent-1", "workflow-1", "run-1", "account-1", "org-1")

	want := map[string]interface{}{
		"input1":              "hello",
		"sys.query":           "hello",
		"sys.user_id":         "account-1",
		"sys.agent_id":        "agent-1",
		"sys.tenant_id":       "workspace-1",
		"sys.workspace_id":    "workspace-1",
		"sys.organization_id": "org-1",
		"sys.workflow_id":     "workflow-1",
		"sys.workflow_run_id": "run-1",
	}
	for key, expected := range want {
		if got := inputs[key]; got != expected {
			t.Fatalf("%s = %#v, want %#v", key, got, expected)
		}
	}
}

func TestWorkflowRunElapsedMilliseconds_KeepsMillisecondsAcrossApprovalWait(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(13 * time.Second)

	got := workflowRunElapsedMilliseconds(WorkflowRunLog{
		ElapsedTime: 11.1,
		CreatedAt:   createdAt,
		FinishedAt:  &finishedAt,
	})
	if math.Abs(got-11.1) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 11.1)
	}
}

func TestWorkflowRunElapsedMillisecondsForEvent_SumsNodeRuntimeLogs(t *testing.T) {
	repo := &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{ElapsedTime: 12.3},
			{ElapsedTime: 17.3},
			{ElapsedTime: 6.6},
		},
	}
	service := &WorkflowService{workflowNodeRuntimeLogRepo: repo}

	got := service.workflowRunElapsedMillisecondsForEvent(context.Background(), "run-1", 999)
	if math.Abs(got-36.2) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 36.2)
	}
}

func TestWorkflowRunElapsedMillisecondsForEvent_IncludesPausedRuntimeLog(t *testing.T) {
	repo := &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{ID: "start-log", Status: "succeeded", ElapsedTime: 1.1},
			{ID: "answer-log", Status: "succeeded", ElapsedTime: 2.2},
			{ID: "branch-log", Status: "succeeded", ElapsedTime: 2.3},
			{ID: "approval-log", Status: string(dto.NodeStatusPaused), ElapsedTime: 20.7},
		},
	}
	service := &WorkflowService{workflowNodeRuntimeLogRepo: repo}

	got := service.workflowRunElapsedMillisecondsForEvent(context.Background(), "run-1", 20.7)
	if math.Abs(got-26.3) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 26.3)
	}
}

func TestWorkflowRunElapsedMillisecondsForEvent_FallsBackWhenNoNodeRuntimeLogs(t *testing.T) {
	service := &WorkflowService{workflowNodeRuntimeLogRepo: &mockWorkflowNodeRuntimeLogRepo{}}

	got := service.workflowRunElapsedMillisecondsForEvent(context.Background(), "run-1", 63.69)
	if math.Abs(got-63.7) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 63.7)
	}
}

func TestWorkflowStoredEventData_UsesNodeRuntimeLogForNodeFinished(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	finishedAt := createdAt.Add(700 * time.Microsecond)
	service := &WorkflowService{workflowNodeRuntimeLogRepo: &mockWorkflowNodeRuntimeLogRepo{
		logByNodeExecutionID: &WorkflowNodeRuntimeLog{
			ElapsedTime: 0.7,
			CreatedAt:   createdAt,
			FinishedAt:  &finishedAt,
		},
	}}
	handler := &WorkflowHandler{workflowService: service}

	got := handler.workflowStoredEventData(context.Background(), &WorkflowRunLog{ID: "run-1"}, workflowpause.RunEventPayload{
		Event: workflowpause.EventNodeFinished,
		Data: map[string]interface{}{
			"node_execution_id": "node-exec-1",
			"elapsed_time":      700.0,
		},
	})

	elapsed, ok := got["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time type = %T, want float64", got["elapsed_time"])
	}
	if math.Abs(elapsed-0.7) > 0.000001 {
		t.Fatalf("elapsed_time = %.9f, want %.9f", elapsed, 0.7)
	}
}

func TestWorkflowStoredEventData_FiltersStoredNodeInputs(t *testing.T) {
	handler := &WorkflowHandler{}

	got := handler.workflowStoredEventData(context.Background(), &WorkflowRunLog{ID: "run-1"}, workflowpause.RunEventPayload{
		Event: workflowpause.EventNodeStarted,
		Data: map[string]interface{}{
			"node_type": "sql-generator",
			"inputs": map[string]interface{}{
				"prompt_variables": map[string]interface{}{"start.query": "show users"},
				"prompt":           map[string]interface{}{"user": "show users"},
				"system_prompt":    "hidden",
				"model":            map[string]interface{}{"name": "gpt"},
				"data_source":      map[string]interface{}{"id": "ds-1"},
				"schema_tables":    []interface{}{"ds-1.users"},
				"table_schema":     []interface{}{map[string]interface{}{"id": "users", "name": "users"}},
			},
		},
	})

	inputs, ok := got["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputs type = %T, want map", got["inputs"])
	}
	if _, exists := inputs["system_prompt"]; exists {
		t.Fatalf("system_prompt should be removed from stored event inputs: %#v", inputs)
	}
	if _, exists := inputs["model"]; exists {
		t.Fatalf("model should be removed from stored event inputs: %#v", inputs)
	}
	if inputs["prompt"] == nil {
		t.Fatalf("prompt should be kept in stored event inputs: %#v", inputs)
	}
	if _, exists := inputs["prompt_variables"]; exists {
		t.Fatalf("prompt_variables should be removed from stored event inputs: %#v", inputs)
	}
	for _, key := range []string{"data_source", "schema_tables", "table_schema"} {
		if _, exists := inputs[key]; exists {
			t.Fatalf("%s should be removed from stored event inputs: %#v", key, inputs)
		}
	}
}

func TestGetWorkflowRunNodeExecutions_FiltersFrontendInputs(t *testing.T) {
	inputsJSON := `{"prompt":{"user":"show users"},"prompt_variables":{"start.query":"show users"},"system_prompt":"hidden","model":{"name":"gpt"},"data_source":{"id":"ds-1"},"schema_tables":["ds-1.users"],"table_schema":[{"id":"users","name":"users"}]}`
	outputsJSON := `{"sql":"SELECT 1"}`
	processDataJSON := `{"metadata_context":"kept-for-diagnostics"}`
	metadataJSON := `{"total_tokens":12}`
	createdAt := time.Unix(1700000000, 0)

	service := &WorkflowService{workflowNodeRuntimeLogRepo: &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{
				ID:                "node-log-1",
				AgentID:           "agent-1",
				NodeID:            "sql-node",
				NodeType:          "sql-generator",
				Title:             "SQL Generator",
				Index:             1,
				Status:            "succeeded",
				Inputs:            &inputsJSON,
				Outputs:           &outputsJSON,
				ProcessData:       &processDataJSON,
				ExecutionMetadata: &metadataJSON,
				CreatedAt:         createdAt,
			},
		},
	}}

	raw, err := service.GetWorkflowRunNodeExecutions(context.Background(), "tenant-1", "agent-1", "run-1")
	if err != nil {
		t.Fatalf("GetWorkflowRunNodeExecutions returned error: %v", err)
	}
	resp, ok := raw.(*dto.WorkflowRunNodeExecutionListResponse)
	if !ok {
		t.Fatalf("response type = %T, want *WorkflowRunNodeExecutionListResponse", raw)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(response.Data) = %d, want 1", len(resp.Data))
	}

	var inputs map[string]interface{}
	if err := json.Unmarshal(resp.Data[0].Inputs, &inputs); err != nil {
		t.Fatalf("unmarshal inputs: %v", err)
	}
	if _, exists := inputs["system_prompt"]; exists {
		t.Fatalf("system_prompt should be removed from node execution inputs: %#v", inputs)
	}
	if _, exists := inputs["model"]; exists {
		t.Fatalf("model should be removed from node execution inputs: %#v", inputs)
	}
	if inputs["prompt"] == nil {
		t.Fatalf("prompt should be kept in sql-generator inputs: %#v", inputs)
	}
	if _, exists := inputs["prompt_variables"]; exists {
		t.Fatalf("prompt_variables should be removed from sql-generator inputs: %#v", inputs)
	}
	for _, key := range []string{"data_source", "schema_tables", "table_schema"} {
		if _, exists := inputs[key]; exists {
			t.Fatalf("%s should be removed from sql-generator inputs: %#v", key, inputs)
		}
	}
}

func TestGetWorkflowRunNodeExecutions_KeepsScalarOutputs(t *testing.T) {
	outputsJSON := `"final answer"`
	service := &WorkflowService{workflowNodeRuntimeLogRepo: &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{
				ID:       "node-log-1",
				AgentID:  "agent-1",
				NodeID:   "answer-node",
				NodeType: "answer",
				Title:    "Answer",
				Index:    1,
				Status:   "succeeded",
				Outputs:  &outputsJSON,
			},
		},
	}}

	raw, err := service.GetWorkflowRunNodeExecutions(context.Background(), "tenant-1", "agent-1", "run-1")
	if err != nil {
		t.Fatalf("GetWorkflowRunNodeExecutions returned error: %v", err)
	}
	resp, ok := raw.(*dto.WorkflowRunNodeExecutionListResponse)
	if !ok {
		t.Fatalf("response type = %T, want *WorkflowRunNodeExecutionListResponse", raw)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(response.Data) = %d, want 1", len(resp.Data))
	}

	var outputs map[string]interface{}
	if err := json.Unmarshal(resp.Data[0].Outputs, &outputs); err != nil {
		t.Fatalf("unmarshal outputs: %v", err)
	}
	if got := outputs["value"]; got != "final answer" {
		t.Fatalf("outputs[value] = %#v, want final answer", got)
	}
}

func TestWorkflowStoredEventData_UsesNodeRuntimeLogSumForWorkflowFinished(t *testing.T) {
	createdAt := time.Unix(1700000000, 0)
	startFinishedAt := createdAt.Add(2 * time.Millisecond)
	answerFinishedAt := createdAt.Add(3200 * time.Microsecond)
	branchFinishedAt := createdAt.Add(700 * time.Microsecond)
	service := &WorkflowService{workflowNodeRuntimeLogRepo: &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{ID: "start-log", Status: "succeeded", ElapsedTime: 2, CreatedAt: createdAt, FinishedAt: &startFinishedAt},
			{ID: "answer-log", Status: "succeeded", ElapsedTime: 3.2, CreatedAt: createdAt, FinishedAt: &answerFinishedAt},
			{ID: "branch-log", Status: "succeeded", ElapsedTime: 0.7, CreatedAt: createdAt, FinishedAt: &branchFinishedAt},
			{ID: "approval-log", Status: "paused", ElapsedTime: 13.2},
		},
	}}
	handler := &WorkflowHandler{workflowService: service}

	got := handler.workflowStoredEventData(context.Background(), &WorkflowRunLog{ID: "run-1"}, workflowpause.RunEventPayload{
		Event: workflowpause.EventWorkflowFinished,
		Data: map[string]interface{}{
			"elapsed_time": 700000.0,
		},
	})

	elapsed, ok := got["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time type = %T, want float64", got["elapsed_time"])
	}
	if math.Abs(elapsed-19.1) > 0.000001 {
		t.Fatalf("elapsed_time = %.9f, want %.9f", elapsed, 19.1)
	}
}

func TestDurationMilliseconds_RoundsToOneDecimal(t *testing.T) {
	got := durationMilliseconds(123456789 * time.Nanosecond)
	if math.Abs(got-123.5) > 0.000001 {
		t.Fatalf("elapsed milliseconds = %.9f, want %.9f", got, 123.5)
	}
}

func TestWorkflowElapsedTracker_ExcludesPausedSeedLog(t *testing.T) {
	tracker := newWorkflowElapsedTrackerFromNodeLogs([]WorkflowNodeRuntimeLog{
		{ID: "start-log", Status: "succeeded", ElapsedTime: 8.6},
		{ID: "approval-log", Status: "paused", ElapsedTime: 10},
	})

	got := tracker.elapsedOrFallback(0)
	if math.Abs(got-8.6) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 8.6)
	}
}

func TestWorkflowElapsedTracker_ReplacesSameNodeLogElapsed(t *testing.T) {
	tracker := newWorkflowElapsedTrackerFromNodeLogs([]WorkflowNodeRuntimeLog{
		{ID: "start-log", Status: "succeeded", ElapsedTime: 8.6},
	})

	tracker.recordNodeElapsed("approval-log", 10)
	got := tracker.recordNodeElapsed("answer-log", 7.9)
	if math.Abs(got-26.5) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 26.5)
	}

	got = tracker.recordNodeElapsed("answer-log", 8.1)
	if math.Abs(got-26.7) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds after replace = %.9f, want %.9f", got, 26.7)
	}
}

func TestWorkflowElapsedMillisecondsFromOutputs_ReturnsAndRemovesInternalElapsed(t *testing.T) {
	outputs := map[string]interface{}{
		workflowInternalElapsedTimeKey: 26.5,
		"answer":                       "ok",
	}

	got := workflowElapsedMillisecondsFromOutputs(outputs, 99)
	if math.Abs(got-26.5) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 26.5)
	}
	if _, exists := outputs[workflowInternalElapsedTimeKey]; exists {
		t.Fatalf("expected internal elapsed key to be removed")
	}
}

func TestWorkflowElapsedMillisecondsFromOutputs_RoundsFallback(t *testing.T) {
	got := workflowElapsedMillisecondsFromOutputs(nil, 63.694334)
	if math.Abs(got-63.7) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 63.7)
	}
}

func TestWorkflowExecutionResultElapsedMilliseconds_SumsNodeResults(t *testing.T) {
	start := time.Unix(1700000000, 0)
	result := &WorkflowExecutionResult{
		ExecutionTime: time.Second,
		NodeResults: map[string]interface{}{
			"start": map[string]interface{}{
				"startTime": start,
				"endTime":   start.Add(2 * time.Millisecond),
			},
			"answer": map[string]interface{}{
				"startTime": start.Add(3 * time.Millisecond),
				"endTime":   start.Add(6200 * time.Microsecond),
			},
			"branch": map[string]interface{}{
				"startTime": start.Add(7 * time.Millisecond),
				"endTime":   start.Add(7700 * time.Microsecond),
			},
		},
	}

	got := workflowExecutionResultElapsedMilliseconds(result, 999)
	if math.Abs(got-5.9) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 5.9)
	}
}

func TestWorkflowElapsedMillisecondsFromResult_UsesNodeResultsMap(t *testing.T) {
	start := time.Unix(1700000000, 0)
	result := map[string]interface{}{
		"start": map[string]interface{}{
			"startTime": start,
			"endTime":   start.Add(2 * time.Millisecond),
		},
		"answer": map[string]interface{}{
			"startTime": start,
			"endTime":   start.Add(3200 * time.Microsecond),
		},
	}

	got := WorkflowElapsedMillisecondsFromResult(result, 999)
	if math.Abs(got-5.2) > 0.000001 {
		t.Fatalf("workflow elapsed milliseconds = %.9f, want %.9f", got, 5.2)
	}
}

func TestBuildBlockingWorkflowRunResponseIncludesReadableOutputs(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(outputs map[string]any) map[string]any {
		outputs["text"] = "你好，我可以帮你查询订单状态。"
		return outputs
	})
	result := &WorkflowExecutionResult{
		NodeResults: map[string]interface{}{
			"answer-node": map[string]interface{}{
				"answer": "你好，我可以帮你查询订单状态。",
			},
		},
		RuntimeState: runtimeState,
	}

	response := buildBlockingWorkflowRunResponse("agent-1", "run-1", result, 1234.0)

	if response["workflow_run_id"] != "run-1" {
		t.Fatalf("workflow_run_id = %#v, want run-1", response["workflow_run_id"])
	}
	if response["answer"] != "你好，我可以帮你查询订单状态。" {
		t.Fatalf("answer = %#v", response["answer"])
	}
	outputs, ok := response["outputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("outputs type = %T, want map[string]interface{}", response["outputs"])
	}
	if outputs["text"] != "你好，我可以帮你查询订单状态。" {
		t.Fatalf("outputs[text] = %#v", outputs["text"])
	}
	if response["elapsed_time"] != 1234.0 {
		t.Fatalf("elapsed_time = %#v, want 1234", response["elapsed_time"])
	}
}

func TestBuildBlockingWorkflowRunResponseIncludesReadableNodeErrors(t *testing.T) {
	result := &WorkflowExecutionResult{
		Status: "failed",
		NodeResults: map[string]interface{}{
			"llm-node": map[string]interface{}{
				"status": "failed",
				"error":  "failed to invoke LLM: billing operation failed",
			},
		},
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID: "llm-node",
				Status: shared.FAILED,
				Error:  "failed to invoke LLM: billing operation failed",
			},
		},
	}

	response := buildBlockingWorkflowRunResponse("agent-1", "run-1", result, 1234.0)

	if response["status"] != "failed" {
		t.Fatalf("status = %#v, want failed", response["status"])
	}
	if response["error"] != "node llm-node failed: failed to invoke LLM: billing operation failed" {
		t.Fatalf("error = %#v", response["error"])
	}
	nodeErrors, ok := response["node_errors"].(map[string]string)
	if !ok {
		t.Fatalf("node_errors type = %T, want map[string]string", response["node_errors"])
	}
	if nodeErrors["llm-node"] != "failed to invoke LLM: billing operation failed" {
		t.Fatalf("node_errors[llm-node] = %#v", nodeErrors["llm-node"])
	}
}

func TestWorkflowExecutionLogStatusAndErrorUsesExecutionStatus(t *testing.T) {
	result := &WorkflowExecutionResult{
		Status: "failed",
		NodeResults: map[string]interface{}{
			"llm-node": map[string]interface{}{
				"status": "failed",
				"error":  "failed to invoke LLM: provider unavailable",
			},
		},
	}

	status, errorMessage := workflowExecutionLogStatusAndError(result, nil)

	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}
	if errorMessage != "node llm-node failed: failed to invoke LLM: provider unavailable" {
		t.Fatalf("errorMessage = %q", errorMessage)
	}
}

func TestUpdateWorkflowNodeRuntimeLog_PersistsProcessDataAndExecutionMetadata(t *testing.T) {
	repo := &mockWorkflowNodeRuntimeLogRepo{}
	service := &WorkflowService{
		workflowNodeRuntimeLogRepo: repo,
	}

	outputs := map[string]interface{}{
		"text": "vision ok",
	}
	processData := map[string]interface{}{
		"vision_enabled":             true,
		"resolved_file_count":        1,
		"auto_injected_user_prompt":  true,
		"final_prompt_content_types": []string{"image", "text"},
		"llm_gateway_request": map[string]interface{}{
			"model": "gpt-4o",
			"messages": []map[string]interface{}{
				{"role": "system", "content": "You are a diagnosis assistant."},
				{"role": "user", "content": "hello"},
			},
			"params": map[string]interface{}{
				"temperature": 0.3,
			},
		},
	}
	executionMetadata := map[string]interface{}{
		"total_tokens": 42,
	}

	err := service.UpdateWorkflowNodeRuntimeLog(context.Background(), "node-log-1", "succeeded", outputs, processData, executionMetadata, 1.25, "")
	if err != nil {
		t.Fatalf("UpdateWorkflowNodeRuntimeLog returned error: %v", err)
	}

	if repo.statusID != "node-log-1" || repo.statusValue != "succeeded" {
		t.Fatalf("expected status update for node-log-1/succeeded, got id=%q status=%q", repo.statusID, repo.statusValue)
	}
	if repo.statusFinishedAt == nil {
		t.Fatalf("expected finishedAt to be set")
	}
	if repo.updateOutputsID != "node-log-1" {
		t.Fatalf("expected outputs update for node-log-1, got %q", repo.updateOutputsID)
	}
	if repo.updateProcessDataValue == nil {
		t.Fatalf("expected process_data payload to be persisted")
	}
	if repo.updateMetadataValue == nil {
		t.Fatalf("expected execution_metadata payload to be persisted")
	}

	var gotProcessData map[string]interface{}
	if err := json.Unmarshal([]byte(*repo.updateProcessDataValue), &gotProcessData); err != nil {
		t.Fatalf("failed to unmarshal process_data payload: %v", err)
	}
	if got := gotProcessData["resolved_file_count"]; got != float64(1) {
		t.Fatalf("expected resolved_file_count=1, got %v", got)
	}
	if got := gotProcessData["auto_injected_user_prompt"]; got != true {
		t.Fatalf("expected auto_injected_user_prompt=true, got %v", got)
	}
	gatewayRequest, ok := gotProcessData["llm_gateway_request"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected llm_gateway_request to be persisted, got %T", gotProcessData["llm_gateway_request"])
	}
	if got := gatewayRequest["model"]; got != "gpt-4o" {
		t.Fatalf("expected llm_gateway_request.model=gpt-4o, got %v", got)
	}
	params, ok := gatewayRequest["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected llm_gateway_request.params to be persisted, got %T", gatewayRequest["params"])
	}
	if got := params["temperature"]; got != 0.3 {
		t.Fatalf("expected llm_gateway_request.params.temperature=0.3, got %v", got)
	}

	var gotMetadata map[string]interface{}
	if err := json.Unmarshal([]byte(*repo.updateMetadataValue), &gotMetadata); err != nil {
		t.Fatalf("failed to unmarshal execution_metadata payload: %v", err)
	}
	if got := gotMetadata["total_tokens"]; got != float64(42) {
		t.Fatalf("expected total_tokens=42, got %v", got)
	}
}

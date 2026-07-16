package workflowtest

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func testNow() time.Time {
	return time.Date(2026, 5, 25, 20, 0, 0, 0, time.UTC)
}

type captureScenarioRecognizer struct {
	input ScenarioRecognitionInput
}

func (r *captureScenarioRecognizer) RecognizeScenarios(ctx context.Context, req ScenarioRecognitionInput) (*ScenarioRecognitionResult, error) {
	r.input = req
	return &ScenarioRecognitionResult{
		Scenarios:   []RecognizedScenario{{Name: "售后退款", Description: "处理退款诉求"}},
		Assignments: []RecognizedCaseAssignment{{CaseID: "case-1", ScenarioName: "售后退款"}},
	}, nil
}

type staticWorkflowContextProvider struct {
	value string
}

func (p staticWorkflowContextProvider) WorkflowRecognitionContext(ctx context.Context, agentID string) string {
	return p.value
}

func TestParseScenarioRecognitionResponseAcceptsJSONCodeFence(t *testing.T) {
	result, err := parseScenarioRecognitionResponse("```json\n{\"scenarios\":[{\"name\":\"售前咨询\",\"description\":\"产品能力咨询\"}],\"assignments\":[{\"case_id\":\"case-1\",\"scenario_name\":\"售前咨询\"}]}\n```")

	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	require.Equal(t, "售前咨询", result.Scenarios[0].Name)
	require.Len(t, result.Assignments, 1)
	require.Equal(t, "case-1", result.Assignments[0].CaseID)
}

func TestParseScenarioRecognitionResponseExtractsJSONFromText(t *testing.T) {
	result, err := parseScenarioRecognitionResponse("以下是识别结果：\n{\"scenarios\":[{\"name\":\"制度问答\",\"description\":\"内部制度咨询\"}],\"assignments\":[]}\n请查收。")

	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	require.Equal(t, "制度问答", result.Scenarios[0].Name)
}

func TestParseScenarioRecognitionResponseRejectsEmptyResult(t *testing.T) {
	_, err := parseScenarioRecognitionResponse(`{"scenarios":[{"name":"","description":"空名称"}]}`)

	require.Error(t, err)
	require.Contains(t, err.Error(), "scenario recognition result is empty")
}

func TestBuildScenarioRecognitionPromptTruncatesLongCases(t *testing.T) {
	longContent := strings.Repeat("很长的问题内容", 80)

	prompt := buildScenarioRecognitionPrompt(ScenarioRecognitionInput{
		Cases: []Case{
			{ID: "case-1", Content: longContent, QuestionType: CaseTypeCore},
			{ID: "case-2", Content: "短问题", QuestionType: CaseTypeExtension},
		},
	})

	require.Contains(t, prompt, "case-1")
	require.Contains(t, prompt, "短问题")
	require.Less(t, strings.Count(prompt, "很长的问题内容"), 80)
	require.Contains(t, prompt, "...")
}

func TestBuildScenarioRecognitionPromptTreatsPromptAsUserSupplement(t *testing.T) {
	prompt := buildScenarioRecognitionPrompt(ScenarioRecognitionInput{
		Prompt: "Focus on reimbursement and approval scenarios.",
	})

	require.Contains(t, prompt, defaultScenarioRecognitionPrompt())
	require.Contains(t, prompt, "User supplementary requirements")
	require.Contains(t, prompt, "system rules must take priority")
	require.Contains(t, prompt, "Focus on reimbursement and approval scenarios.")
}

func TestBuildScenarioRecognitionPromptUsesTaskModeRules(t *testing.T) {
	prompt := buildScenarioRecognitionPrompt(ScenarioRecognitionInput{
		CaseMode: "task",
	})

	require.Contains(t, prompt, defaultTaskScenarioRecognitionPrompt())
	require.Contains(t, prompt, "Task workflows are function-like executions")
	require.Contains(t, prompt, "business object/document type + processing goal")
	require.Contains(t, prompt, "company contract")
	require.Contains(t, prompt, "school exam paper")
	require.Contains(t, prompt, "meeting notes")
	require.Contains(t, prompt, "Do not create scenarios for unsupported format handling")
	require.Contains(t, prompt, "field completeness checks")
	require.Contains(t, prompt, "输入内容")
	require.Contains(t, prompt, "目标")
	require.Contains(t, prompt, "测试重点")
	require.NotContains(t, prompt, "Input: ...")
	require.NotContains(t, prompt, "Goal: ...")
	require.NotContains(t, prompt, "Test focus: ...")
}

func TestLLMScenarioRecognizerDoesNotForceResponseFormat(t *testing.T) {
	client := &fakeLLMClient{
		responseContent: `{"scenarios":[{"name":"售前咨询","description":"产品能力咨询"}]}`,
	}
	recognizer := &LLMScenarioRecognizer{
		Client:      client,
		WorkspaceID: "00000000-0000-0000-0000-000000000001",
		AccountID:   "00000000-0000-0000-0000-000000000002",
		AgentID:     "00000000-0000-0000-0000-000000000003",
	}

	result, err := recognizer.RecognizeScenarios(context.Background(), ScenarioRecognitionInput{
		Prompt: "只返回 JSON",
		Model:  &Model{Provider: "zgi-cloud", Name: "test-model"},
	})

	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	require.Len(t, client.requests, 1)
	require.Nil(t, client.requests[0].ResponseFormat)
	require.Equal(t, "zgi-cloud", client.requests[0].Provider)
	require.Equal(t, "test-model", client.requests[0].Model)
}

func TestCreateScenarioRecognitionTaskSnapshotsWorkflowContext(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	service.SetWorkflowContextProvider(staticWorkflowContextProvider{value: "Workflow structure summary:\nLLM prompt: 售后退款"})
	ctx := context.Background()
	now := testNow()

	expectActiveScenarioRecognitionTask(mock, "agent-1", scenarioRecognitionTaskRows())
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workflow_test_scenario_recognition_tasks" ("id","agent_id","workspace_id","account_id","status","prompt","context","workflow_context_snapshot","model_provider","model_name","recognized_count","assigned_case_count","error","started_at","cancel_requested_at","completed_at","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) RETURNING "created_at","updated_at"`)).
		WithArgs(
			sqlmock.AnyArg(),
			"agent-1",
			"workspace-1",
			"account-1",
			GenerationTaskStatusQueued,
			"prompt",
			"context",
			"Workflow structure summary:\nLLM prompt: 售后退款",
			"openai",
			"gpt-4.1",
			0,
			0,
			"",
			nil,
			nil,
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	mock.ExpectCommit()

	task, err := service.CreateScenarioRecognitionTask(ctx, "agent-1", "workspace-1", "account-1", RecognizeScenariosRequest{
		Context: " context ",
		Prompt:  " prompt ",
		Model:   &Model{Provider: " openai ", Name: " gpt-4.1 "},
	})

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, "Workflow structure summary:\nLLM prompt: 售后退款", task.WorkflowContextSnapshot)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunScenarioRecognitionTaskUsesSnapshotAndFinishesCounts(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	service.SetWorkflowContextProvider(staticWorkflowContextProvider{value: "latest workflow should not be used"})
	recognizer := &captureScenarioRecognizer{}
	ctx := context.Background()
	now := testNow()

	expectScenarioRecognitionTaskByID(mock, "task-1", scenarioRecognitionTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued,
		"prompt", "context", "snapshot workflow", "openai", "gpt-4.1", 0, 0, "", nil, nil, nil, now, now,
	))
	expectMarkScenarioRecognitionTaskRunning(mock, "task-1", true)
	expectScenarioRecognitionTaskByID(mock, "task-1", scenarioRecognitionTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning,
		"prompt", "context", "snapshot workflow", "openai", "gpt-4.1", 0, 0, "", &now, nil, nil, now, now,
	))
	expectScenarioRecognitionTaskByID(mock, "task-1", scenarioRecognitionTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning,
		"prompt", "context", "snapshot workflow", "openai", "gpt-4.1", 0, 0, "", &now, nil, nil, now, now,
	))
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_cases" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(caseRows(now).AddRow("case-1", "agent-1", nil, "我要退款", "处理退款", CaseTypeCore, CaseStatusEnabled, `[]`, now, now))
	expectListScenarios(mock, "agent-1", scenarioRows(now))
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_scenarios" WHERE agent_id = \$1 AND name = \$2 ORDER BY "workflow_test_scenarios"\."id" LIMIT \$3`).
		WithArgs("agent-1", "售后退款", 1).
		WillReturnRows(scenarioRows(now))
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "workflow_test_scenarios"`).
		WillReturnRows(scenarioRows(now).AddRow("scenario-new", "agent-1", "售后退款", "处理退款诉求", "ai", 0, now, now))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_cases" SET "scenario_id"=$1,"updated_at"=$2 WHERE agent_id = $3 AND id = $4`)).
		WithArgs("scenario-new", sqlmock.AnyArg(), "agent-1", "case-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "workflow_test_scenarios" SET "case_count"=\$1,"updated_at"=\$2 WHERE agent_id = \$3`).
		WithArgs(0, sqlmock.AnyArg(), "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_cases" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(caseRows(now).AddRow("case-1", "agent-1", "scenario-new", "我要退款", "处理退款", CaseTypeCore, CaseStatusEnabled, `[]`, now, now))
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "workflow_test_scenarios" SET "case_count"=\$1,"updated_at"=\$2 WHERE agent_id = \$3 AND id = \$4`).
		WithArgs(1, sqlmock.AnyArg(), "agent-1", "scenario-new").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_scenarios" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(scenarioRows(now).AddRow("scenario-new", "agent-1", "售后退款", "处理退款诉求", "ai", 1, now, now))
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_cases" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(caseRows(now).AddRow("case-1", "agent-1", "scenario-new", "我要退款", "处理退款", CaseTypeCore, CaseStatusEnabled, `[]`, now, now))
	expectScenarioRecognitionTaskByID(mock, "task-1", scenarioRecognitionTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning,
		"prompt", "context", "snapshot workflow", "openai", "gpt-4.1", 0, 0, "", &now, nil, nil, now, now,
	))
	expectFinishScenarioRecognitionTask(mock, "task-1", GenerationTaskStatusCompleted, "", 1, 1)

	err := service.RunScenarioRecognitionTask(ctx, "task-1", recognizer)

	require.NoError(t, err)
	require.Equal(t, "snapshot workflow", recognizer.input.WorkflowContext)
	require.Equal(t, "context", recognizer.input.Context)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRecognizeScenariosPassesWorkflowContextToRecognizer(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	service.SetWorkflowContextProvider(staticWorkflowContextProvider{value: "Workflow structure summary:\nLLM prompt: 售后退款"})
	recognizer := &captureScenarioRecognizer{}
	ctx := context.Background()

	mock.ExpectQuery(`SELECT \* FROM "workflow_test_cases" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(caseRows(testNow()))
	expectListScenarios(mock, "agent-1", scenarioRows(testNow()).AddRow("scenario-1", "agent-1", "旧场景", "", "manual", 0, testNow(), testNow()))
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_scenarios" WHERE agent_id = \$1 AND name = \$2 ORDER BY "workflow_test_scenarios"\."id" LIMIT \$3`).
		WithArgs("agent-1", "售后退款", 1).
		WillReturnRows(scenarioRows(testNow()))
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO "workflow_test_scenarios"`).
		WillReturnRows(scenarioRows(testNow()).AddRow("scenario-new", "agent-1", "售后退款", "处理退款诉求", "ai", 0, testNow(), testNow()))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "workflow_test_scenarios" SET "case_count"=\$1,"updated_at"=\$2 WHERE agent_id = \$3`).
		WithArgs(0, sqlmock.AnyArg(), "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_cases" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(caseRows(testNow()))
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_scenarios" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(scenarioRows(testNow()).AddRow("scenario-new", "agent-1", "售后退款", "处理退款诉求", "ai", 0, testNow(), testNow()))
	mock.ExpectQuery(`SELECT \* FROM "workflow_test_cases" WHERE agent_id = \$1 ORDER BY created_at DESC`).
		WithArgs("agent-1").
		WillReturnRows(caseRows(testNow()))

	_, err := service.RecognizeScenarios(ctx, "agent-1", RecognizeScenariosRequest{Context: "用户补充上下文"}, recognizer)

	require.NoError(t, err)
	require.Contains(t, recognizer.input.WorkflowContext, "LLM prompt: 售后退款")
	require.Equal(t, "用户补充上下文", recognizer.input.Context)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectActiveScenarioRecognitionTask(mock sqlmock.Sqlmock, agentID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_scenario_recognition_tasks" WHERE agent_id = $1 AND status IN ($2,$3,$4) ORDER BY created_at DESC,"workflow_test_scenario_recognition_tasks"."id" LIMIT $5`)).
		WithArgs(
			agentID,
			GenerationTaskStatusQueued,
			GenerationTaskStatusRunning,
			GenerationTaskStatusCanceling,
			1,
		).
		WillReturnRows(rows)
}

func expectScenarioRecognitionTaskByID(mock sqlmock.Sqlmock, taskID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_scenario_recognition_tasks" WHERE id = $1 ORDER BY "workflow_test_scenario_recognition_tasks"."id" LIMIT $2`)).
		WithArgs(taskID, 1).
		WillReturnRows(rows)
}

func expectMarkScenarioRecognitionTaskRunning(mock sqlmock.Sqlmock, taskID string, changed bool) {
	rowsAffected := int64(0)
	if changed {
		rowsAffected = 1
	}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4 AND status = $5`)).
		WithArgs(sqlmock.AnyArg(), GenerationTaskStatusRunning, sqlmock.AnyArg(), taskID, GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
	mock.ExpectCommit()
}

func expectFinishScenarioRecognitionTask(mock sqlmock.Sqlmock, taskID string, status string, reason string, recognizedCount int, assignedCaseCount int) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenario_recognition_tasks" SET "assigned_case_count"=$1,"completed_at"=$2,"error"=$3,"recognized_count"=$4,"status"=$5,"updated_at"=$6 WHERE id = $7`)).
		WithArgs(assignedCaseCount, sqlmock.AnyArg(), reason, recognizedCount, status, sqlmock.AnyArg(), taskID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func scenarioRecognitionTaskRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"workspace_id",
		"account_id",
		"status",
		"prompt",
		"context",
		"workflow_context_snapshot",
		"model_provider",
		"model_name",
		"recognized_count",
		"assigned_case_count",
		"error",
		"started_at",
		"cancel_requested_at",
		"completed_at",
		"created_at",
		"updated_at",
	})
}

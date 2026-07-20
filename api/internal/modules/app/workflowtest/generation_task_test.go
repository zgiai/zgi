package workflowtest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/queue"
)

type fakeCaseGenerator struct {
	mu       sync.Mutex
	requests []GenerateCasesRequest
	result   *GenerateCasesResult
	results  []*GenerateCasesResult
}

type fakeTaskCanceler struct {
	taskIDs []string
}

func (c *fakeTaskCanceler) Cancel(taskID string) {
	c.taskIDs = append(c.taskIDs, taskID)
}

func (g *fakeCaseGenerator) GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests = append(g.requests, req)
	if len(g.results) > 0 {
		result := g.results[0]
		g.results = g.results[1:]
		return result, nil
	}
	if g.result != nil {
		return g.result, nil
	}
	scenarioID := req.ScenarioID
	if scenarioID == "" && len(req.ScenarioIDs) > 0 {
		scenarioID = req.ScenarioIDs[0]
	}
	content := "case for " + scenarioID
	if len(g.requests) > 1 {
		content = fmt.Sprintf("%s variant %c", content, rune('a'+len(g.requests)))
	}
	return &GenerateCasesResult{Cases: []GeneratedCase{{
		Content:        content,
		ExpectedResult: "expected",
		QuestionType:   CaseTypeCore,
	}}}, nil
}

type fakeLLMClient struct {
	err              error
	responseContent  string
	responseContents []string
	requests         []*adapter.ChatRequest
}

func (c *fakeLLMClient) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	c.requests = append(c.requests, req)
	if c.err != nil {
		return nil, c.err
	}
	content := c.responseContent
	if len(c.responseContents) > 0 {
		content = c.responseContents[0]
		c.responseContents = c.responseContents[1:]
	}
	if content == "" {
		content = `{"cases":[{"content":"generated case","expected_result":"expected","question_type":"core"}]}`
	}
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Content: content},
		}},
	}, nil
}

func (c *fakeLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *fakeLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestNewGenerationTaskAsynqTaskPayloadAndType(t *testing.T) {
	task, err := NewGenerationTaskAsynqTask("task-1", nil)

	require.NoError(t, err)
	require.Equal(t, WorkflowTestGenerationTaskType, task.Type())
	require.JSONEq(t, `{"task_id":"task-1"}`, string(task.Payload()))
}

func TestNewGenerationTaskAsynqTaskOptionsAndPrefix(t *testing.T) {
	opts := generationTaskAsynqOptions()
	require.Len(t, opts, 3)
	requireTaskOption(t, opts, asynq.QueueOpt, "default")
	requireTaskOption(t, opts, asynq.MaxRetryOpt, 0)
	requireTaskOption(t, opts, asynq.TimeoutOpt, 10*time.Minute)

	taskManager, err := queue.NewTaskManager(&config.Config{
		Redis: config.RedisConfig{
			Host: "127.0.0.1",
			Port: 6379,
		},
		TaskQueue: config.TaskQueueConfig{
			Concurrency: 1,
			EnvPrefix:   "test",
		},
	})
	require.NoError(t, err)
	defer taskManager.Close()

	task, err := NewGenerationTaskAsynqTask("task-1", taskManager)
	require.NoError(t, err)
	require.Equal(t, "test:"+WorkflowTestGenerationTaskType, task.Type())
}

func TestNewGenerationTaskHandlerReturnsSkipRetryForBadPayload(t *testing.T) {
	handler := NewGenerationTaskHandler(NewService(nil), &fakeLLMClient{})

	err := handler(context.Background(), asynq.NewTask(WorkflowTestGenerationTaskType, []byte(`{`)))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry))

	err = handler(context.Background(), asynq.NewTask(WorkflowTestGenerationTaskType, []byte(`{}`)))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry))
}

func TestCreateGenerationTaskRejectsActiveTask(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 14, 0, 0, 0, time.UTC)
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectActiveGenerationTask(mock, "agent-1", generationTaskRows().AddRow(
		"task-active", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
	))

	task, err := service.CreateGenerationTask(ctx, "agent-1", "workspace-1", "account-1", CreateGenerationTaskRequest{
		Count:       1,
		ScenarioIDs: []string{"scenario-1"},
	})

	require.Nil(t, task)
	require.EqualError(t, err, "generation task is already running")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateGenerationTaskSnapshotsNormalizedRequest(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 14, 30, 0, 0, time.UTC)
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "Support", "", "manual", 0, now, now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "Support", "", "manual", 0, now, now))
	expectActiveGenerationTask(mock, "agent-1", generationTaskRows())
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workflow_test_generation_tasks" ("id","agent_id","workspace_id","account_id","status","requested_count","created_count","scenario_ids","question_types","turn_strategy","prompt","context","model_provider","model_name","error","started_at","cancel_requested_at","completed_at","created_at","updated_at") VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20) RETURNING "created_at","updated_at"`)).
		WithArgs(
			sqlmock.AnyArg(),
			"agent-1",
			"workspace-1",
			"account-1",
			GenerationTaskStatusQueued,
			2,
			0,
			`["scenario-1","scenario-2"]`,
			`["core","extension","fuzzy","manual"]`,
			"mixed",
			"custom prompt",
			"business context",
			"openai",
			"gpt-4.1",
			"",
			nil,
			nil,
			nil,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	mock.ExpectCommit()

	task, err := service.CreateGenerationTask(ctx, "agent-1", "workspace-1", "account-1", CreateGenerationTaskRequest{
		Count:         2,
		ScenarioID:    " scenario-2 ",
		ScenarioIDs:   []string{" scenario-1 ", "scenario-1", "scenario-2"},
		QuestionTypes: []string{" core ", "invalid", "extension", "core", " fuzzy ", "manual"},
		TurnStrategy:  "   ",
		Prompt:        " custom prompt ",
		Context:       " business context ",
		Model:         &Model{Provider: " openai ", Name: " gpt-4.1 "},
	})

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, JSONList{"scenario-1", "scenario-2"}, task.ScenarioIDs)
	require.Equal(t, JSONList{"core", "extension", "fuzzy", "manual"}, task.QuestionTypes)
	require.Equal(t, "mixed", task.TurnStrategy)
	require.Equal(t, "custom prompt", task.Prompt)
	require.Equal(t, "business context", task.Context)
	require.Equal(t, "openai", task.ModelProvider)
	require.Equal(t, "gpt-4.1", task.ModelName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateGenerationTaskDefaultsToCoreQuestionType(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 14, 40, 0, 0, time.UTC)
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectActiveGenerationTask(mock, "agent-1", generationTaskRows())
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workflow_test_generation_tasks"`)).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))
	mock.ExpectCommit()

	task, err := service.CreateGenerationTask(ctx, "agent-1", "workspace-1", "account-1", CreateGenerationTaskRequest{
		Count:       1,
		ScenarioIDs: []string{"scenario-1"},
		CaseMode:    "task",
	})

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, JSONList{"core"}, task.QuestionTypes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateGenerationTaskMapsActiveUniqueConflict(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 14, 45, 0, 0, time.UTC)
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectActiveGenerationTask(mock, "agent-1", generationTaskRows())
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workflow_test_generation_tasks"`)).
		WillReturnError(fmt.Errorf(`ERROR: duplicate key value violates unique constraint "idx_workflow_test_generation_tasks_active_agent"`))
	mock.ExpectRollback()

	task, err := service.CreateGenerationTask(ctx, "agent-1", "workspace-1", "account-1", CreateGenerationTaskRequest{
		Count:       1,
		ScenarioIDs: []string{"scenario-1"},
	})

	require.Nil(t, task)
	require.EqualError(t, err, "generation task is already running")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetActiveGenerationTaskReturnsNilOnRecordNotFound(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	expectActiveGenerationTask(mock, "agent-1", generationTaskRows())

	task, err := service.GetActiveGenerationTask(context.Background(), "agent-1")

	require.NoError(t, err)
	require.Nil(t, task)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCancelGenerationTaskReturnsCurrentTask(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	canceler := &fakeTaskCanceler{}
	service.SetTaskCanceler(canceler)
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 15, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "cancel_requested_at"=$1,"completed_at"=$2,"error"=$3,"status"=$4,"updated_at"=$5 WHERE agent_id = $6 AND id = $7 AND status = $8`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "", GenerationTaskStatusCanceled, sqlmock.AnyArg(), "agent-1", "task-1", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_generation_tasks" WHERE agent_id = $1 AND id = $2 ORDER BY "workflow_test_generation_tasks"."id" LIMIT $3`)).
		WithArgs("agent-1", "task-1", 1).
		WillReturnRows(generationTaskRows().AddRow(
			"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCanceled, 1, 0,
			`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, now, now, now, now,
		))

	task, err := service.CancelGenerationTask(ctx, "agent-1", "task-1")

	require.NoError(t, err)
	require.NotNil(t, task)
	require.Equal(t, "task-1", task.ID)
	require.Equal(t, GenerationTaskStatusCanceled, task.Status)
	require.NotNil(t, task.CancelRequestedAt)
	require.Equal(t, []string{"task-1"}, canceler.taskIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryCancelGenerationTaskCompletesQueuedTaskImmediately(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "cancel_requested_at"=$1,"completed_at"=$2,"error"=$3,"status"=$4,"updated_at"=$5 WHERE agent_id = $6 AND id = $7 AND status = $8`)).
		WithArgs(now, now, "", GenerationTaskStatusCanceled, now, "agent-1", "task-queued", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	changed, err := repo.CancelGenerationTask(ctx, "agent-1", "task-queued", now)

	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateCasesUsesSharedHelper(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 16, 0, 0, 0, time.UTC)
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectCreateCase(mock, "case-1")
	expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-1", 1)
	generator := &fakeCaseGenerator{}

	result, err := service.GenerateCases(ctx, "agent-1", GenerateCasesRequest{
		Count:       1,
		ScenarioIDs: []string{"scenario-1"},
		Context:     " context ",
		Prompt:      " prompt ",
	}, generator)

	require.NoError(t, err)
	require.Len(t, generator.requests, 1)
	require.Equal(t, "scenario-1", generator.requests[0].ScenarioID)
	require.Equal(t, []string{"scenario-1"}, generator.requests[0].ScenarioIDs)
	require.Len(t, result.Items, 1)
	require.Equal(t, "case for scenario-1", result.Items[0].Content)
	require.Len(t, result.Cases, 1)
	require.NotEmpty(t, generator.requests[0].Scenarios)
	require.Empty(t, generator.requests[0].ExistingCases)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateCasesIgnoresGeneratedScenarioIDAndUsesPlannedScenario(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 16, 10, 0, 0, time.UTC)
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "订单查询", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "售后退款", "", "manual", 0, now, now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "订单查询", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "售后退款", "", "manual", 0, now, now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "订单查询", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "售后退款", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-1", "agent-1", "订单查询", "", "manual", 0, now, now).
		AddRow("scenario-2", "agent-1", "售后退款", "", "manual", 0, now, now))
	expectCreateCase(mock, "case-1")
	expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-1", 1)
	generator := &fakeCaseGenerator{result: &GenerateCasesResult{Cases: []GeneratedCase{{
		ScenarioID:     "scenario-2",
		Content:        "我要退货",
		ExpectedResult: "应识别退款诉求",
		QuestionType:   CaseTypeCore,
	}}}}

	result, err := service.GenerateCases(ctx, "agent-1", GenerateCasesRequest{
		Count:       1,
		ScenarioIDs: []string{"scenario-1", "scenario-2"},
	}, generator)

	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.NotNil(t, result.Items[0].ScenarioID)
	require.Equal(t, "scenario-1", *result.Items[0].ScenarioID)
	require.Len(t, generator.requests, 1)
	require.Equal(t, "scenario-1", generator.requests[0].ScenarioID)
	require.Equal(t, []string{"scenario-1"}, generator.requests[0].ScenarioIDs)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScopedGenerateCaseRequestDistributesAcrossSelectedScenarios(t *testing.T) {
	scenarioByID := map[string]Scenario{}
	scenarioIDs := make([]string, 0, 5)
	scenarios := make([]Scenario, 0, 5)
	existingCases := make([]Case, 0, 5)
	for i := 1; i <= 5; i++ {
		scenarioID := fmt.Sprintf("scenario-%d", i)
		scenarioIDs = append(scenarioIDs, scenarioID)
		scenario := Scenario{ID: scenarioID, Name: fmt.Sprintf("Scene %d", i)}
		scenarioByID[scenarioID] = scenario
		scenarios = append(scenarios, scenario)
		existingCases = append(existingCases, Case{
			ScenarioID: &scenarioID,
			Content:    fmt.Sprintf("existing case %d", i),
		})
	}
	baseReq := GenerateCasesRequest{
		ScenarioIDs:   scenarioIDs,
		Scenarios:     scenarios,
		ExistingCases: existingCases,
	}

	for index := 0; index < 10; index++ {
		req := scopedGenerateCaseRequest(baseReq, index, scenarioByID)
		expectedScenarioID := fmt.Sprintf("scenario-%d", index%5+1)
		require.Equal(t, expectedScenarioID, req.ScenarioID)
		require.Equal(t, []string{expectedScenarioID}, req.ScenarioIDs)
		require.Len(t, req.Scenarios, 1)
		require.Equal(t, expectedScenarioID, req.Scenarios[0].ID)
		require.Len(t, req.ExistingCases, 1)
		require.NotNil(t, req.ExistingCases[0].ScenarioID)
		require.Equal(t, expectedScenarioID, *req.ExistingCases[0].ScenarioID)
	}
}

func TestEffectiveTurnStrategySchedulesMixedConversation(t *testing.T) {
	req := GenerateCasesRequest{CaseMode: "conversation", TurnStrategy: "mixed"}

	require.Equal(t, "single", effectiveTurnStrategy(req, 0))
	require.Equal(t, "multi", effectiveTurnStrategy(req, 1))
	require.Equal(t, "single", effectiveTurnStrategy(req, 2))
	require.Equal(t, "multi", effectiveTurnStrategy(req, 3))
	require.Equal(t, "multi", effectiveTurnStrategy(GenerateCasesRequest{CaseMode: "conversation", TurnStrategy: "multi"}, 0))
	require.Equal(t, "single", effectiveTurnStrategy(GenerateCasesRequest{CaseMode: "task", TurnStrategy: "multi"}, 0))
}

func TestValidateGeneratedFileTurnStrategyRejectsUnexecutableMultiTurnCase(t *testing.T) {
	err := validateGeneratedFileTurnStrategy(GenerateCasesRequest{
		CaseMode:                 "conversation",
		TurnStrategy:             "multi",
		RequiresCurrentTurnFiles: true,
		FileGeneration:           &FileGenerationConfig{Enabled: true, Formats: []string{"docx"}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "每次运行都必须读取本轮附件")
	require.NoError(t, validateGeneratedFileTurnStrategy(GenerateCasesRequest{
		CaseMode:                 "conversation",
		TurnStrategy:             "single",
		RequiresCurrentTurnFiles: true,
		FileGeneration:           &FileGenerationConfig{Enabled: true, Formats: []string{"docx"}},
	}))
}

func TestGenerateOneCaseForTurnStrategyRetriesSingleTurnMultiCase(t *testing.T) {
	service := &Service{}
	generator := &fakeCaseGenerator{results: []*GenerateCasesResult{
		{Cases: []GeneratedCase{{
			Content:      "single",
			QuestionType: CaseTypeCore,
			Turns: []CaseTurn{{
				Role:    "user",
				Content: "single turn",
			}},
		}}},
		{Cases: []GeneratedCase{{
			Content:      "two-turn",
			QuestionType: CaseTypeCore,
			Turns: []CaseTurn{
				{Role: "user", Content: "first turn"},
				{Role: "user", Content: "follow up"},
			},
		}}},
		{Cases: []GeneratedCase{{
			Content:      "multi",
			QuestionType: CaseTypeCore,
			Turns: []CaseTurn{
				{Role: "user", Content: "first turn"},
				{Role: "user", Content: "follow up"},
				{Role: "user", Content: "final clarification"},
			},
		}}},
	}}

	result, err := service.generateOneCaseForTurnStrategy(context.Background(), generator, GenerateCasesRequest{
		CaseMode:     "conversation",
		TurnStrategy: "multi",
		Prompt:       "base prompt",
	})

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Equal(t, "multi", result.Cases[0].Content)
	require.Len(t, generator.requests, 3)
	require.Equal(t, "base prompt", generator.requests[0].Prompt)
	require.Contains(t, generator.requests[1].Prompt, "至少包含 3 个")
	require.Contains(t, generator.requests[2].Prompt, "至少包含 3 个")
}

func TestGenerateOneCaseForTurnStrategySelectsMultiCaseFromResponse(t *testing.T) {
	service := &Service{}
	generator := &fakeCaseGenerator{result: &GenerateCasesResult{Cases: []GeneratedCase{
		{
			Content:      "single",
			QuestionType: CaseTypeCore,
			Turns: []CaseTurn{{
				Role:    "user",
				Content: "single turn",
			}},
		},
		{
			Content:      "multi",
			QuestionType: CaseTypeCore,
			Turns: []CaseTurn{
				{Role: "user", Content: "first turn"},
				{Role: "user", Content: "follow up"},
				{Role: "user", Content: "final clarification"},
			},
		},
	}}}

	result, err := service.generateOneCaseForTurnStrategy(context.Background(), generator, GenerateCasesRequest{
		CaseMode:     "conversation",
		TurnStrategy: "multi",
	})

	require.NoError(t, err)
	require.Len(t, result.Cases, 1)
	require.Equal(t, "multi", result.Cases[0].Content)
	require.Len(t, generator.requests, 1)
}

func TestCreateCaseIncrementsCurrentScenarioCountWithoutFullCaseRefresh(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 16, 30, 0, 0, time.UTC)
	longContent := "添加一个很长的输入测试" + strings.Repeat("1", 200)

	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectCreateCase(mock, "case-1")
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenarios" SET "case_count"=case_count + $1,"updated_at"=$2 WHERE agent_id = $3 AND id = $4`)).
		WithArgs(1, sqlmock.AnyArg(), "agent-1", "scenario-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	created, err := service.CreateCase(ctx, "agent-1", CreateCaseRequest{
		Content:        longContent,
		ExpectedResult: "返回结果即可",
		ScenarioID:     "scenario-1",
		QuestionType:   CaseTypeCore,
		Status:         CaseStatusEnabled,
		Turns:          []CaseTurn{{Role: "user", Content: longContent}},
	})

	require.NoError(t, err)
	require.Equal(t, longContent, created.Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCaseWithSameScenarioDoesNotRefreshAllScenarioCounts(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 16, 40, 0, 0, time.UTC)
	longContent := "豫章故郡，洪都新府。" + strings.Repeat("星分翼轸，地接衡庐。", 120)
	turns := `[{"role":"user","content":"` + longContent + `"}]`

	expectGetCase(mock, "agent-1", "case-1", caseRows(now).AddRow(
		"case-1", "agent-1", "scenario-1", "old content", "old expected", CaseTypeCore, CaseStatusEnabled,
		`[{"role":"user","content":"old content"}]`, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 1, now, now))
	expectUpdateCase(mock)
	expectGetCase(mock, "agent-1", "case-1", caseRows(now).AddRow(
		"case-1", "agent-1", "scenario-1", longContent, "返回内容即可", CaseTypeCore, CaseStatusEnabled,
		turns, now, now,
	))

	updated, err := service.UpdateCase(ctx, "agent-1", "case-1", UpdateCaseRequest{
		Content:        longContent,
		ExpectedResult: "返回内容即可",
		ScenarioID:     "scenario-1",
		QuestionType:   CaseTypeCore,
		Status:         CaseStatusEnabled,
		Turns:          []CaseTurn{{Role: "user", Content: longContent}},
	})

	require.NoError(t, err)
	require.Equal(t, longContent, updated.Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateCaseMovingScenarioAdjustsBothScenarioCounts(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 16, 45, 0, 0, time.UTC)

	expectGetCase(mock, "agent-1", "case-1", caseRows(now).AddRow(
		"case-1", "agent-1", "scenario-old", "old content", "old expected", CaseTypeCore, CaseStatusEnabled,
		`[{"role":"user","content":"old content"}]`, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).
		AddRow("scenario-new", "agent-1", "New", "", "manual", 0, now, now).
		AddRow("scenario-old", "agent-1", "Old", "", "manual", 1, now, now))
	expectUpdateCase(mock)
	expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-old", -1)
	expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-new", 1)
	expectGetCase(mock, "agent-1", "case-1", caseRows(now).AddRow(
		"case-1", "agent-1", "scenario-new", "new content", "new expected", CaseTypeCore, CaseStatusEnabled,
		`[{"role":"user","content":"new content"}]`, now, now,
	))

	updated, err := service.UpdateCase(ctx, "agent-1", "case-1", UpdateCaseRequest{
		Content:        "new content",
		ExpectedResult: "new expected",
		ScenarioID:     "scenario-new",
		QuestionType:   CaseTypeCore,
		Status:         CaseStatusEnabled,
		Turns:          []CaseTurn{{Role: "user", Content: "new content"}},
	})

	require.NoError(t, err)
	require.Equal(t, "new content", updated.Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskTerminalTaskReturnsNilWithoutGenerating(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_generation_tasks" WHERE id = $1 ORDER BY "workflow_test_generation_tasks"."id" LIMIT $2`)).
		WithArgs("task-1", 1).
		WillReturnRows(generationTaskRows().AddRow(
			"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCompleted, 1, 0,
			`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, now, now, now,
		))
	client := &fakeLLMClient{}

	err := service.RunGenerationTask(ctx, "task-1", client)

	require.NoError(t, err)
	require.Empty(t, client.requests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskAlreadyRunningTaskDoesNotGenerateAgain(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 10, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	client := &fakeLLMClient{}

	err := service.RunGenerationTask(ctx, "task-1", client)

	require.NoError(t, err)
	require.Empty(t, client.requests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskSuccessfulPathFinishesCompleted(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 15, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "prompt", "context", "openai", "gpt-4.1", "", nil, nil, nil, now, now,
	))
	expectMarkGenerationTaskRunning(mock, "task-1", true)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "prompt", "context", "openai", "gpt-4.1", "", now, nil, nil, now, now,
	))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "prompt", "context", "openai", "gpt-4.1", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "prompt", "context", "openai", "gpt-4.1", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectCreateCase(mock, "case-1")
	expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-1", 1)
	expectIncrementGenerationTaskCreatedCount(mock, "task-1", 1)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 1,
		`["scenario-1"]`, `["core"]`, "mixed", "prompt", "context", "openai", "gpt-4.1", "", now, nil, nil, now, now,
	))
	expectFinishGenerationTask(mock, "task-1", GenerationTaskStatusCompleted, "")
	client := &fakeLLMClient{}

	err := service.RunGenerationTask(ctx, "task-1", client)

	require.NoError(t, err)
	require.Len(t, client.requests, 1)
	require.Equal(t, "openai", client.requests[0].Provider)
	require.Equal(t, "gpt-4.1", client.requests[0].Model)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskCancelingPathFinishesCanceledWithoutGenerating(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 30, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCanceling, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, now, nil, now, now,
	))
	expectFinishGenerationTask(mock, "task-1", GenerationTaskStatusCanceled, "")
	client := &fakeLLMClient{}

	err := service.RunGenerationTask(ctx, "task-1", client)

	require.NoError(t, err)
	require.Empty(t, client.requests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskCancelingAfterCreatedCaseIncrementsBeforeCanceled(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 35, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
	))
	expectMarkGenerationTaskRunning(mock, "task-1", true)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectCreateCase(mock, "case-1")
	expectIncrementScenarioCaseCount(mock, "agent-1", "scenario-1", 1)
	expectIncrementGenerationTaskCreatedCount(mock, "task-1", 1)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCanceling, 1, 1,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, now, nil, now, now,
	))
	expectFinishGenerationTask(mock, "task-1", GenerationTaskStatusCanceled, "")

	err := service.RunGenerationTask(ctx, "task-1", &fakeLLMClient{})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskCancelingAfterLLMStopsBeforeCreate(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 40, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
	))
	expectMarkGenerationTaskRunning(mock, "task-1", true)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCanceling, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, now, nil, now, now,
	))
	expectFinishGenerationTask(mock, "task-1", GenerationTaskStatusCanceled, "")

	err := service.RunGenerationTask(ctx, "task-1", &fakeLLMClient{})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskContextCanceledWhileCancelingFinishesCanceled(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 42, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
	))
	expectMarkGenerationTaskRunning(mock, "task-1", true)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusCanceling, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, now, nil, now, now,
	))
	expectFinishGenerationTask(mock, "task-1", GenerationTaskStatusCanceled, "")

	err := service.RunGenerationTask(ctx, "task-1", &fakeLLMClient{err: context.Canceled})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskFailureFinishesFailedWithReason(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 45, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
	))
	expectMarkGenerationTaskRunning(mock, "task-1", true)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectFinishGenerationTask(mock, "task-1", GenerationTaskStatusFailed, "生成测试问题失败：llm down")

	err := service.RunGenerationTask(ctx, "task-1", &fakeLLMClient{err: fmt.Errorf("llm down")})

	require.EqualError(t, err, "llm down")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunGenerationTaskFailureReturnsFinishErrorWhenMarkFailedFails(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 17, 50, 0, 0, time.UTC)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
	))
	expectMarkGenerationTaskRunning(mock, "task-1", true)
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectGenerationTaskByID(mock, "task-1", generationTaskRows().AddRow(
		"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusRunning, 1, 0,
		`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", now, nil, nil, now, now,
	))
	expectListScenarios(mock, "agent-1", scenarioRows(now).AddRow("scenario-1", "agent-1", "Billing", "", "manual", 0, now, now))
	expectListCases(mock, "agent-1", caseRows(now))
	expectFinishGenerationTaskError(mock, "task-1", GenerationTaskStatusFailed, "生成测试问题失败：llm down", fmt.Errorf("finish failed"))

	err := service.RunGenerationTask(ctx, "task-1", &fakeLLMClient{err: fmt.Errorf("llm down")})

	require.Error(t, err)
	require.Contains(t, err.Error(), "llm down")
	require.Contains(t, err.Error(), "finish failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryRecoverStaleRunningGenerationTasksMarksOldActiveTasksFailed(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	staleBefore := time.Date(2026, 5, 25, 18, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "completed_at"=$1,"error"=$2,"status"=$3,"updated_at"=$4 WHERE status IN ($5,$6,$7) AND updated_at < $8`)).
		WithArgs(sqlmock.AnyArg(), "stale failure", GenerationTaskStatusFailed, sqlmock.AnyArg(), GenerationTaskStatusQueued, GenerationTaskStatusRunning, GenerationTaskStatusCanceling, staleBefore).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	recovered, err := repo.RecoverStaleRunningGenerationTasks(ctx, staleBefore, "stale failure", time.Now())

	require.NoError(t, err)
	require.Equal(t, int64(2), recovered)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryRecoverStaleRunningGenerationTasksForAgentOnlyMarksRouteAgentTasks(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	staleBefore := time.Date(2026, 5, 25, 18, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "completed_at"=$1,"error"=$2,"status"=$3,"updated_at"=$4 WHERE (status IN ($5,$6,$7) AND updated_at < $8) AND agent_id = $9`)).
		WithArgs(sqlmock.AnyArg(), "stale failure", GenerationTaskStatusFailed, sqlmock.AnyArg(), GenerationTaskStatusQueued, GenerationTaskStatusRunning, GenerationTaskStatusCanceling, staleBefore, "agent-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	recovered, err := repo.RecoverStaleRunningGenerationTasksForAgent(ctx, "agent-1", staleBefore, "stale failure", time.Now())

	require.NoError(t, err)
	require.Equal(t, int64(1), recovered)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryCreateAndGetActiveGenerationTaskScansJSONLists(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	older := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)

	expectCreateGenerationTask(mock, "task-older")
	require.NoError(t, repo.CreateGenerationTask(ctx, &GenerationTask{
		ID:             "task-older",
		AgentID:        "agent-1",
		WorkspaceID:    "workspace-1",
		AccountID:      "account-1",
		Status:         GenerationTaskStatusQueued,
		RequestedCount: 3,
		ScenarioIDs:    JSONList{"scenario-a", "scenario-b"},
		QuestionTypes:  JSONList{"core", "fuzzy"},
		TurnStrategy:   "mixed",
		CreatedAt:      older,
		UpdatedAt:      older,
	}))
	expectCreateGenerationTask(mock, "task-newer")
	require.NoError(t, repo.CreateGenerationTask(ctx, &GenerationTask{
		ID:             "task-newer",
		AgentID:        "agent-1",
		WorkspaceID:    "workspace-1",
		AccountID:      "account-1",
		Status:         GenerationTaskStatusRunning,
		RequestedCount: 2,
		ScenarioIDs:    JSONList{"scenario-c"},
		QuestionTypes:  JSONList{"extension"},
		TurnStrategy:   "single",
		CreatedAt:      newer,
		UpdatedAt:      newer,
	}))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_generation_tasks" WHERE agent_id = $1 AND status IN ($2,$3,$4) ORDER BY created_at DESC,"workflow_test_generation_tasks"."id" LIMIT $5`)).
		WithArgs(
			"agent-1",
			GenerationTaskStatusQueued,
			GenerationTaskStatusRunning,
			GenerationTaskStatusCanceling,
			1,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "agent_id", "status", "scenario_ids", "question_types", "created_at", "updated_at"}).
			AddRow("task-newer", "agent-1", GenerationTaskStatusRunning, `["scenario-c"]`, `["extension"]`, newer, newer))

	task, err := repo.GetActiveGenerationTask(ctx, "agent-1")

	require.NoError(t, err)
	require.Equal(t, "task-newer", task.ID)
	require.Equal(t, JSONList{"scenario-c"}, task.ScenarioIDs)
	require.Equal(t, JSONList{"extension"}, task.QuestionTypes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryCancelGenerationTaskChangesRunningToCanceling(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "cancel_requested_at"=$1,"completed_at"=$2,"error"=$3,"status"=$4,"updated_at"=$5 WHERE agent_id = $6 AND id = $7 AND status = $8`)).
		WithArgs(now, now, "", GenerationTaskStatusCanceled, now, "agent-1", "task-running", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "cancel_requested_at"=$1,"status"=$2,"updated_at"=$3 WHERE agent_id = $4 AND id = $5 AND status = $6`)).
		WithArgs(now, GenerationTaskStatusCanceling, now, "agent-1", "task-running", GenerationTaskStatusRunning).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	changed, err := repo.CancelGenerationTask(ctx, "agent-1", "task-running", now)

	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryListQueuedGenerationTasks(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_generation_tasks" WHERE status = $1 ORDER BY created_at ASC LIMIT $2`)).
		WithArgs(GenerationTaskStatusQueued, localWorkerClaimLimit).
		WillReturnRows(generationTaskRows().AddRow(
			"task-1", "agent-1", "workspace-1", "account-1", GenerationTaskStatusQueued, 1, 0,
			`["scenario-1"]`, `["core"]`, "mixed", "", "", "", "", "", nil, nil, nil, now, now,
		))

	tasks, err := repo.ListQueuedGenerationTasks(ctx, localWorkerClaimLimit)

	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "task-1", tasks[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryMarkGenerationTaskRunningOnlyChangesQueuedTasks(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Date(2026, 5, 25, 13, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4 AND status = $5`)).
		WithArgs(now, GenerationTaskStatusRunning, now, "task-queued", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	changed, err := repo.MarkGenerationTaskRunning(ctx, "task-queued", now)
	require.NoError(t, err)
	require.True(t, changed)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4 AND status = $5`)).
		WithArgs(now.Add(time.Minute), GenerationTaskStatusRunning, now.Add(time.Minute), "task-running", GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	changed, err = repo.MarkGenerationTaskRunning(ctx, "task-running", now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, changed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryIncrementGenerationTaskCreatedCount(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "created_count"=created_count + $1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs(3, sqlmock.AnyArg(), "task-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, repo.IncrementGenerationTaskCreatedCount(ctx, "task-1", 3))
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectCreateGenerationTask(mock sqlmock.Sqlmock, taskID string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workflow_test_generation_tasks"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(taskID))
	mock.ExpectCommit()
}

func expectGenerationTaskByID(mock sqlmock.Sqlmock, taskID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_generation_tasks" WHERE id = $1 ORDER BY "workflow_test_generation_tasks"."id" LIMIT $2`)).
		WithArgs(taskID, 1).
		WillReturnRows(rows)
}

func expectMarkGenerationTaskRunning(mock sqlmock.Sqlmock, taskID string, changed bool) {
	rowsAffected := int64(0)
	if changed {
		rowsAffected = 1
	}
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "started_at"=$1,"status"=$2,"updated_at"=$3 WHERE id = $4 AND status = $5`)).
		WithArgs(sqlmock.AnyArg(), GenerationTaskStatusRunning, sqlmock.AnyArg(), taskID, GenerationTaskStatusQueued).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
	mock.ExpectCommit()
}

func expectIncrementGenerationTaskCreatedCount(mock sqlmock.Sqlmock, taskID string, delta int) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "created_count"=created_count + $1,"updated_at"=$2 WHERE id = $3`)).
		WithArgs(delta, sqlmock.AnyArg(), taskID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectFinishGenerationTask(mock sqlmock.Sqlmock, taskID string, status string, reason string) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "completed_at"=$1,"error"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5`)).
		WithArgs(sqlmock.AnyArg(), reason, status, sqlmock.AnyArg(), taskID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectFinishGenerationTaskError(mock sqlmock.Sqlmock, taskID string, status string, reason string, err error) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_generation_tasks" SET "completed_at"=$1,"error"=$2,"status"=$3,"updated_at"=$4 WHERE id = $5`)).
		WithArgs(sqlmock.AnyArg(), reason, status, sqlmock.AnyArg(), taskID).
		WillReturnError(err)
	mock.ExpectRollback()
}

func requireTaskOption(t *testing.T, opts []asynq.Option, optionType asynq.OptionType, value interface{}) {
	t.Helper()
	for _, opt := range opts {
		if opt.Type() == optionType {
			require.Equal(t, value, opt.Value())
			return
		}
	}
	require.Failf(t, "missing asynq option", "type=%v", optionType)
}

func expectActiveGenerationTask(mock sqlmock.Sqlmock, agentID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_generation_tasks" WHERE agent_id = $1 AND status IN ($2,$3,$4) ORDER BY created_at DESC,"workflow_test_generation_tasks"."id" LIMIT $5`)).
		WithArgs(
			agentID,
			GenerationTaskStatusQueued,
			GenerationTaskStatusRunning,
			GenerationTaskStatusCanceling,
			1,
		).
		WillReturnRows(rows)
}

func expectListScenarios(mock sqlmock.Sqlmock, agentID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_scenarios" WHERE agent_id = $1 ORDER BY created_at DESC`)).
		WithArgs(agentID).
		WillReturnRows(rows)
}

func expectListCases(mock sqlmock.Sqlmock, agentID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_cases" WHERE agent_id = $1 ORDER BY created_at DESC`)).
		WithArgs(agentID).
		WillReturnRows(rows)
}

func expectCreateCase(mock sqlmock.Sqlmock, caseID string) {
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "workflow_test_cases"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(caseID))
	mock.ExpectCommit()
}

func expectGetCase(mock sqlmock.Sqlmock, agentID, caseID string, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_cases" WHERE agent_id = $1 AND id = $2 ORDER BY "workflow_test_cases"."id" LIMIT $3`)).
		WithArgs(agentID, caseID, 1).
		WillReturnRows(rows)
}

func expectUpdateCase(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_cases" SET "content"=$1,"expected_result"=$2,"question_type"=$3,"scenario_id"=$4,"status"=$5,"turns"=$6,"updated_at"=$7 WHERE agent_id = $8 AND id = $9`)).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectIncrementScenarioCaseCount(mock sqlmock.Sqlmock, agentID, scenarioID string, delta int) {
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "workflow_test_scenarios" SET "case_count"=case_count + $1,"updated_at"=$2 WHERE agent_id = $3 AND id = $4`)).
		WithArgs(delta, sqlmock.AnyArg(), agentID, scenarioID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func scenarioRows(now time.Time) *sqlmock.Rows {
	_ = now
	return sqlmock.NewRows([]string{
		"id", "agent_id", "name", "description", "source", "case_count", "created_at", "updated_at",
	})
}

func caseRows(now time.Time) *sqlmock.Rows {
	_ = now
	return sqlmock.NewRows([]string{
		"id", "agent_id", "scenario_id", "content", "expected_result", "question_type", "status", "turns", "created_at", "updated_at",
	})
}

func generationTaskRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"workspace_id",
		"account_id",
		"status",
		"requested_count",
		"created_count",
		"scenario_ids",
		"question_types",
		"turn_strategy",
		"prompt",
		"context",
		"model_provider",
		"model_name",
		"error",
		"started_at",
		"cancel_requested_at",
		"completed_at",
		"created_at",
		"updated_at",
	})
}

package workflowtest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type fakeRunner struct {
	results []RunCaseResult
	err     error
	calls   []RunCaseRequest
}

type fakeJudge struct {
	results []JudgeResult
	err     error
	calls   []JudgeRequest
}

type fakeSummarizer struct {
	result *SummaryResult
	err    error
	calls  []SummaryRequest
}

type fakeCaseGenerator struct {
	result *GenerateCasesResult
	err    error
	calls  []GenerateCasesRequest
}

type fakeScenarioRecognizer struct {
	result *ScenarioRecognitionResult
	err    error
	calls  []ScenarioRecognitionInput
}

type cancelingRunner struct {
	service *Service
	agentID string
	batchID string
	calls   []RunCaseRequest
}

func (r *fakeRunner) RunCase(ctx context.Context, req RunCaseRequest) (*RunCaseResult, error) {
	r.calls = append(r.calls, req)
	if r.err != nil {
		return nil, r.err
	}
	if len(r.results) == 0 {
		return &RunCaseResult{
			WorkflowRunID: uuid.NewString(),
			Outputs:       map[string]interface{}{"answer": "ok"},
		}, nil
	}
	result := r.results[0]
	r.results = r.results[1:]
	return &result, nil
}

func (j *fakeJudge) JudgeCase(ctx context.Context, req JudgeRequest) (*JudgeResult, error) {
	j.calls = append(j.calls, req)
	if j.err != nil {
		return nil, j.err
	}
	if len(j.results) == 0 {
		return &JudgeResult{
			Status:     BatchItemStatusPassed,
			Reason:     "回答解决了用户问题",
			Suggestion: "",
			Confidence: 0.9,
		}, nil
	}
	result := j.results[0]
	j.results = j.results[1:]
	return &result, nil
}

func (s *fakeSummarizer) SummarizeBatch(ctx context.Context, req SummaryRequest) (*SummaryResult, error) {
	s.calls = append(s.calls, req)
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return &SummaryResult{Summary: "默认测试总结"}, nil
	}
	return s.result, nil
}

func (g *fakeCaseGenerator) GenerateCases(ctx context.Context, req GenerateCasesRequest) (*GenerateCasesResult, error) {
	g.calls = append(g.calls, req)
	if g.err != nil {
		return nil, g.err
	}
	if g.result == nil {
		return &GenerateCasesResult{Cases: []GeneratedCase{{Content: "默认生成问题", ExpectedResult: "应回答默认生成问题。", QuestionType: CaseTypeCore}}}, nil
	}
	return g.result, nil
}

func (r *fakeScenarioRecognizer) RecognizeScenarios(ctx context.Context, req ScenarioRecognitionInput) (*ScenarioRecognitionResult, error) {
	r.calls = append(r.calls, req)
	if r.err != nil {
		return nil, r.err
	}
	return r.result, nil
}

func (r *cancelingRunner) RunCase(ctx context.Context, req RunCaseRequest) (*RunCaseResult, error) {
	r.calls = append(r.calls, req)
	if _, err := r.service.CancelBatch(ctx, r.agentID, r.batchID); err != nil {
		return nil, err
	}
	return &RunCaseResult{
		WorkflowRunID: "run-after-cancel",
		Outputs:       map[string]interface{}{"answer": "late result"},
	}, nil
}

func setupTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Setting{}, &Scenario{}, &Case{}, &Batch{}, &BatchItem{}))

	return NewService(NewRepository(db)), db
}

func requireTestScenario(t *testing.T, service *Service, ctx context.Context, agentID string) string {
	t.Helper()
	scenario, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{Name: "默认场景"})
	require.NoError(t, err)
	return scenario.ID
}

func TestGetSettingsReturnsDefaultPromptForAgent(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()

	settings, err := service.GetSettings(ctx, agentID)

	require.NoError(t, err)
	require.Equal(t, agentID, settings.AgentID)
	require.NotEmpty(t, settings.JudgePromptTemplate)
	require.Contains(t, settings.JudgePromptTemplate, "通过")
	require.Contains(t, settings.JudgePromptTemplate, "不通过")
	require.Contains(t, settings.JudgePromptTemplate, "需复核")
	require.Empty(t, settings.JudgeModelProvider)
	require.Empty(t, settings.JudgeModelName)
}

func TestUpdateSettingsIsScopedByAgent(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentA := uuid.NewString()
	agentB := uuid.NewString()

	updated, err := service.UpdateSettings(ctx, agentA, UpdateSettingsRequest{
		JudgePromptTemplate: "请根据企业客服规范判断答案是否解决用户问题。",
		JudgeModelProvider:  "openai",
		JudgeModelName:      "chatgpt-4o-latest",
	})
	require.NoError(t, err)
	require.Equal(t, "请根据企业客服规范判断答案是否解决用户问题。", updated.JudgePromptTemplate)
	require.Equal(t, "openai", updated.JudgeModelProvider)
	require.Equal(t, "chatgpt-4o-latest", updated.JudgeModelName)

	settingsA, err := service.GetSettings(ctx, agentA)
	require.NoError(t, err)
	require.Equal(t, updated.JudgePromptTemplate, settingsA.JudgePromptTemplate)
	require.Equal(t, updated.JudgeModelProvider, settingsA.JudgeModelProvider)
	require.Equal(t, updated.JudgeModelName, settingsA.JudgeModelName)

	settingsB, err := service.GetSettings(ctx, agentB)
	require.NoError(t, err)
	require.NotEqual(t, settingsA.JudgePromptTemplate, settingsB.JudgePromptTemplate)
	require.Equal(t, DefaultJudgePromptTemplate, settingsB.JudgePromptTemplate)
	require.Empty(t, settingsB.JudgeModelProvider)
	require.Empty(t, settingsB.JudgeModelName)
}

func TestCreateCaseStoresExpectedResult(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	created, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:        "企业版支持哪些能力？",
		ExpectedResult: "应说明企业版支持的核心能力。",
		ScenarioID:     scenarioID,
	})

	require.NoError(t, err)
	require.Equal(t, "应说明企业版支持的核心能力。", created.ExpectedResult)
}

func TestUpdateCaseStoresExpectedResult(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	testCase, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:        "企业版支持哪些能力？",
		ExpectedResult: "应说明企业版支持的核心能力。",
		ScenarioID:     scenarioID,
	})
	require.NoError(t, err)

	updated, err := service.UpdateCase(ctx, agentID, testCase.ID, UpdateCaseRequest{
		Content:        "如何接入 CRM？",
		ExpectedResult: "应说明 CRM 接入准备事项。",
		ScenarioID:     scenarioID,
	})

	require.NoError(t, err)
	require.Equal(t, "应说明 CRM 接入准备事项。", updated.ExpectedResult)
}

func TestCreateBatchSnapshotsExpectedResult(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	created, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:        "请介绍企业版能力",
		ExpectedResult: "应说明权限管理、数据分析和团队协作能力。",
		ScenarioID:     scenarioID,
		Status:         CaseStatusEnabled,
	})
	require.NoError(t, err)

	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "预期结果快照"})

	require.NoError(t, err)
	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, created.ExpectedResult, items[0].CaseSnapshot.ExpectedResult)
}

func TestCreateBatchSnapshotsJudgeModelSettings(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.UpdateSettings(ctx, agentID, UpdateSettingsRequest{
		JudgePromptTemplate: "按企业客服标准评分。",
		JudgeModelProvider:  "openai",
		JudgeModelName:      "chatgpt-4o-latest",
	})
	require.NoError(t, err)
	_, err = service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:      "请介绍企业版能力",
		QuestionType: CaseTypeCore,
		ScenarioID:   scenarioID,
		Status:       CaseStatusEnabled,
	})
	require.NoError(t, err)

	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "评分模型快照"})

	require.NoError(t, err)
	require.Equal(t, "按企业客服标准评分。", batch.JudgePromptSnapshot)
	require.Equal(t, "openai", batch.JudgeModelProviderSnapshot)
	require.Equal(t, "chatgpt-4o-latest", batch.JudgeModelNameSnapshot)
}

func TestCreateCasePreservesTurnAttachments(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	created, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:      "请根据附件识别客户需求",
		ScenarioID:   scenarioID,
		QuestionType: CaseTypeCore,
		Status:       CaseStatusEnabled,
		Turns: []CaseTurn{
			{
				Role:    "user",
				Content: "这是客户资料",
				Attachments: []CaseAttachment{
					{Type: "document", TransferMethod: "local_file", UploadFileID: uuid.NewString(), Name: "需求文档.pdf"},
					{Type: "image", TransferMethod: "local_file", UploadFileID: uuid.NewString(), Name: "客户截图.png"},
				},
			},
		},
	})

	require.NoError(t, err)
	require.Len(t, created.Turns, 1)
	require.Len(t, created.Turns[0].Attachments, 2)
	require.Equal(t, "local_file", created.Turns[0].Attachments[0].TransferMethod)
}

func TestCreateCaseDerivesContentFromFirstTurn(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	created, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		ScenarioID: scenarioID,
		Turns: []CaseTurn{
			{Role: "user", Content: "我想了解企业版支持哪些能力？"},
			{Role: "user", Content: "如果接入销售跟进，还需要准备什么资料？"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "我想了解企业版支持哪些能力？", created.Content)
	require.Len(t, created.Turns, 2)
	require.Equal(t, "如果接入销售跟进，还需要准备什么资料？", created.Turns[1].Content)
}

func TestCreateCaseUpdatesScenarioCaseCount(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenario, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{Name: "售前咨询"})
	require.NoError(t, err)

	_, err = service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "企业版支持哪些能力？",
		ScenarioID: scenario.ID,
	})

	require.NoError(t, err)
	scenarios, err := service.ListScenarios(ctx, agentID)
	require.NoError(t, err)
	require.Len(t, scenarios, 1)
	require.Equal(t, 1, scenarios[0].CaseCount)
}

func TestCreateCaseRejectsOtherAgentScenario(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	scenario, err := service.CreateScenario(ctx, uuid.NewString(), CreateScenarioRequest{Name: "其他场景"})
	require.NoError(t, err)

	_, err = service.CreateCase(ctx, uuid.NewString(), CreateCaseRequest{
		Content:    "问题",
		ScenarioID: scenario.ID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "scenario not found")
}

func TestCreateCaseRejectsInvalidStatusAndQuestionType(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "非法状态问题",
		ScenarioID: scenarioID,
		Status:     "archived",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid case status")

	_, err = service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:      "非法类型问题",
		ScenarioID:   scenarioID,
		QuestionType: "unknown",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid question type")
}

func TestUpdateCaseChangesContentScenarioAndStatus(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenario, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{Name: "订单查询"})
	require.NoError(t, err)
	created, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "原始问题",
		ScenarioID: scenario.ID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)

	updated, err := service.UpdateCase(ctx, agentID, created.ID, UpdateCaseRequest{
		Content:      "订单现在到哪里了？",
		ScenarioID:   scenario.ID,
		QuestionType: CaseTypeExtension,
		Status:       CaseStatusDisabled,
		Turns: []CaseTurn{{
			Role:    "user",
			Content: "订单现在到哪里了？",
			Attachments: []CaseAttachment{
				{Type: "document", TransferMethod: "local_file", UploadFileID: uuid.NewString(), Name: "订单.pdf"},
			},
		}},
	})

	require.NoError(t, err)
	require.Equal(t, "订单现在到哪里了？", updated.Content)
	require.Equal(t, scenario.ID, *updated.ScenarioID)
	require.Equal(t, CaseTypeExtension, updated.QuestionType)
	require.Equal(t, CaseStatusDisabled, updated.Status)
	require.Len(t, updated.Turns[0].Attachments, 1)
}

func TestUpdateCaseRejectsOtherAgentCase(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	created, err := service.CreateCase(ctx, agentID, CreateCaseRequest{Content: "其他智能体问题", ScenarioID: scenarioID})
	require.NoError(t, err)

	_, err = service.UpdateCase(ctx, uuid.NewString(), created.ID, UpdateCaseRequest{Content: "非法更新"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "case not found")
}

func TestCreateScenarioMergesExactSameName(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()

	first, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{
		Name:        "售前咨询",
		Description: "原始描述",
	})
	require.NoError(t, err)

	second, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{
		Name:        " 售前咨询 ",
		Description: "新的描述不应创建第二条",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	items, err := service.ListScenarios(ctx, agentID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "原始描述", items[0].Description)
}

func TestSaveScenariosUpdatesCreatesAndDeletes(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	kept, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{
		Name:        "售前咨询",
		Description: "原说明",
	})
	require.NoError(t, err)
	deleted, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{Name: "订单查询"})
	require.NoError(t, err)
	testCase, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:      "我的订单到哪里了？",
		ScenarioID:   deleted.ID,
		QuestionType: CaseTypeCore,
		Status:       CaseStatusEnabled,
	})
	require.NoError(t, err)

	scenarios, err := service.SaveScenarios(ctx, agentID, SaveScenariosRequest{
		Scenarios: []SaveScenarioItemRequest{
			{ID: kept.ID, Name: "售前问询", Description: "更新后的说明"},
			{Name: "售后退款", Description: "退款相关问题"},
		},
	})

	require.NoError(t, err)
	require.Len(t, scenarios, 2)
	names := map[string]Scenario{}
	for _, scenario := range scenarios {
		names[scenario.Name] = scenario
	}
	require.Contains(t, names, "售前问询")
	require.Equal(t, "更新后的说明", names["售前问询"].Description)
	require.Contains(t, names, "售后退款")
	cases, err := service.ListCases(ctx, agentID, "")
	require.NoError(t, err)
	require.Len(t, cases, 1)
	require.Equal(t, testCase.ID, cases[0].ID)
	require.Nil(t, cases[0].ScenarioID)
}

func TestGenerateCasesCreatesEnabledCasesFromGenerator(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenario, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{
		Name:        "售前咨询",
		Description: "产品能力咨询",
	})
	require.NoError(t, err)
	generator := &fakeCaseGenerator{result: &GenerateCasesResult{Cases: []GeneratedCase{
		{Content: "企业版支持哪些能力？", QuestionType: CaseTypeCore},
		{Content: "如果客户已有 CRM，该如何接入？", QuestionType: CaseTypeExtension},
	}}}

	result, err := service.GenerateCases(ctx, agentID, GenerateCasesRequest{
		Count:      2,
		ScenarioID: scenario.ID,
		Context:    "线索跟进助手",
	}, generator)

	require.NoError(t, err)
	require.Len(t, generator.calls, 1)
	require.Equal(t, "线索跟进助手", generator.calls[0].Context)
	require.Len(t, result.Items, 2)
	require.Equal(t, scenario.ID, *result.Items[0].ScenarioID)
	require.Equal(t, CaseStatusEnabled, result.Items[0].Status)
	require.Equal(t, "企业版支持哪些能力？", result.Items[0].Content)
}

func TestGenerateCasesRejectsInvalidCount(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()

	_, err := service.GenerateCases(ctx, uuid.NewString(), GenerateCasesRequest{Count: 0}, &fakeCaseGenerator{})

	require.Error(t, err)
	require.Contains(t, err.Error(), "count must be between")
}

func TestGenerateCasesLimitsCreatedCasesToRequestedCount(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	generator := &fakeCaseGenerator{result: &GenerateCasesResult{Cases: []GeneratedCase{
		{Content: "问题一"},
		{Content: "问题二"},
		{Content: "问题三"},
	}}}

	result, err := service.GenerateCases(ctx, agentID, GenerateCasesRequest{Count: 2, ScenarioID: scenarioID}, generator)

	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	require.Equal(t, "问题二", result.Items[1].Content)
}

func TestRecognizeScenariosMergesSameNameAndAssignsCases(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	existing, err := service.CreateScenario(ctx, agentID, CreateScenarioRequest{
		Name:        "售前咨询",
		Description: "已有场景",
	})
	require.NoError(t, err)
	first, err := service.CreateCase(ctx, agentID, CreateCaseRequest{Content: "企业版支持哪些能力？", ScenarioID: existing.ID})
	require.NoError(t, err)
	second, err := service.CreateCase(ctx, agentID, CreateCaseRequest{Content: "订单现在到哪里了？", ScenarioID: existing.ID})
	require.NoError(t, err)
	recognizer := &fakeScenarioRecognizer{result: &ScenarioRecognitionResult{
		Scenarios: []RecognizedScenario{
			{Name: "售前咨询", Description: "新的描述不会覆盖已有场景"},
			{Name: "订单查询", Description: "订单状态和物流查询"},
		},
		Assignments: []RecognizedCaseAssignment{
			{CaseID: first.ID, ScenarioName: "售前咨询"},
			{CaseID: second.ID, ScenarioName: "订单查询"},
		},
	}}

	result, err := service.RecognizeScenarios(ctx, agentID, RecognizeScenariosRequest{Context: "线索跟进助手"}, recognizer)

	require.NoError(t, err)
	require.Len(t, recognizer.calls, 1)
	require.Equal(t, "线索跟进助手", recognizer.calls[0].Context)
	require.Len(t, result.Scenarios, 2)
	require.Len(t, result.Cases, 2)

	cases, err := service.ListCases(ctx, agentID, "")
	require.NoError(t, err)
	assigned := map[string]string{}
	for _, item := range cases {
		if item.ScenarioID != nil {
			assigned[item.Content] = *item.ScenarioID
		}
	}
	require.Equal(t, existing.ID, assigned["企业版支持哪些能力？"])
	require.NotEmpty(t, assigned["订单现在到哪里了？"])

	scenarios, err := service.ListScenarios(ctx, agentID)
	require.NoError(t, err)
	sourceByName := map[string]string{}
	for _, scenario := range scenarios {
		sourceByName[scenario.Name] = scenario.Source
	}
	require.Equal(t, "manual", sourceByName["售前咨询"])
	require.Equal(t, "ai", sourceByName["订单查询"])
}

func TestRecognizeScenariosPassesPromptAndModel(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{Content: "订单现在到哪里了？", ScenarioID: scenarioID})
	require.NoError(t, err)
	recognizer := &fakeScenarioRecognizer{result: &ScenarioRecognitionResult{
		Scenarios: []RecognizedScenario{
			{Name: "订单查询", Description: "订单状态和物流查询"},
		},
	}}

	_, err = service.RecognizeScenarios(ctx, agentID, RecognizeScenariosRequest{
		Context: "客服助手",
		Prompt:  "按用户意图识别",
		Model:   &Model{Provider: " openai ", Name: " gpt-4.1 "},
	}, recognizer)

	require.NoError(t, err)
	require.Len(t, recognizer.calls, 1)
	require.Equal(t, "按用户意图识别", recognizer.calls[0].Prompt)
	require.Equal(t, &Model{Provider: "openai", Name: "gpt-4.1"}, recognizer.calls[0].Model)
}

func TestRecognizeScenariosAllowsEmptyCaseLibrary(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	recognizer := &fakeScenarioRecognizer{result: &ScenarioRecognitionResult{
		Scenarios: []RecognizedScenario{
			{Name: "售前咨询", Description: "产品咨询和销售跟进"},
		},
	}}

	result, err := service.RecognizeScenarios(ctx, agentID, RecognizeScenariosRequest{Context: "客服支持助手"}, recognizer)

	require.NoError(t, err)
	require.Len(t, recognizer.calls, 1)
	require.Empty(t, recognizer.calls[0].Cases)
	require.Len(t, result.Scenarios, 1)
	require.Empty(t, result.Assignments)
	require.Empty(t, result.Cases)

	scenarios, err := service.ListScenarios(ctx, agentID)
	require.NoError(t, err)
	require.Len(t, scenarios, 1)
	require.Equal(t, "ai", scenarios[0].Source)
}

func TestCreateBatchSnapshotsEnabledCases(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	enabledCase, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:      "已启用问题",
		QuestionType: CaseTypeCore,
		ScenarioID:   scenarioID,
		Status:       CaseStatusEnabled,
	})
	require.NoError(t, err)
	_, err = service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:      "未启用问题",
		QuestionType: CaseTypeFuzzy,
		ScenarioID:   scenarioID,
		Status:       CaseStatusDisabled,
	})
	require.NoError(t, err)

	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{
		Name: "回归测试",
	})
	require.NoError(t, err)
	require.Equal(t, 1, batch.CaseCount)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, enabledCase.ID, items[0].CaseID)
	require.Equal(t, enabledCase.Content, items[0].CaseSnapshot.Content)
}

func TestCreateBatchCanUseSelectedCaseIDsOnly(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	first, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "第一个问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	second, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "第二个问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)

	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{
		Name:    "指定范围测试",
		CaseIDs: []string{second.ID},
	})
	require.NoError(t, err)
	require.Equal(t, 1, batch.CaseCount)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, second.ID, items[0].CaseID)
	require.NotEqual(t, first.ID, items[0].CaseID)
}

func TestCreateBatchDraftVersionStoresNullVersionUUID(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	testCase, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "草稿版本问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)

	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{
		Name:                "草稿版本测试",
		CaseIDs:             []string{testCase.ID},
		WorkflowVersionMode: WorkflowVersionModeDraft,
	})
	require.NoError(t, err)
	require.Equal(t, WorkflowVersionModeDraft, batch.WorkflowVersionMode)
	require.Nil(t, batch.WorkflowVersionUUID)

	persisted, err := service.repo.GetBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Nil(t, persisted.WorkflowVersionUUID)
}

func TestCreateBatchRejectsDisabledSelectedCases(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	disabled, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "未启用问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusDisabled,
	})
	require.NoError(t, err)

	_, err = service.CreateBatch(ctx, agentID, CreateBatchRequest{
		Name:    "指定未启用问题",
		CaseIDs: []string{disabled.ID},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "selected cases must all be enabled")
}

func TestRetestBatchCreatesQueuedBatchFromOriginalSnapshots(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	createdCase, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "原始问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	original, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "回归测试"})
	require.NoError(t, err)
	_, err = service.UpdateCase(ctx, agentID, createdCase.ID, UpdateCaseRequest{
		Content:      "已修改问题",
		ScenarioID:   scenarioID,
		QuestionType: CaseTypeCore,
		Status:       CaseStatusDisabled,
	})
	require.NoError(t, err)

	retest, err := service.RetestBatch(ctx, agentID, original.ID, "Regression retest")

	require.NoError(t, err)
	require.NotEqual(t, original.ID, retest.ID)
	require.Equal(t, BatchStatusQueued, retest.Status)
	require.Equal(t, "Regression retest", retest.Name)
	require.Equal(t, 1, retest.CaseCount)

	items, err := service.ListBatchItems(ctx, agentID, retest.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, createdCase.ID, items[0].CaseID)
	require.Equal(t, "原始问题", items[0].CaseSnapshot.Content)
	require.Equal(t, string(BatchItemStatusPending), items[0].Status)
}

func TestExecuteBatchWithoutRunnerStoresFailure(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "无 runner 问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "无 runner 执行"})
	require.NoError(t, err)

	executed, err := service.ExecuteBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, BatchStatusCompleted, executed.Status)
	require.Equal(t, 0, executed.PassedCount)
	require.Equal(t, 1, executed.FailedCount)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, string(BatchItemStatusFailed), items[0].Status)
	require.Contains(t, items[0].Error, "workflow runner is not configured")
}

func TestStartBatchMarksBatchAndItemsRunning(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "执行问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "执行骨架测试"})
	require.NoError(t, err)

	started, err := service.StartBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, BatchStatusRunning, started.Status)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, string(BatchItemStatusRunning), items[0].Status)
}

func TestStartBatchUsesConditionalStatusTransition(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "执行问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "条件启动测试"})
	require.NoError(t, err)

	updated, err := service.repo.UpdateBatchStatusIfCurrent(ctx, agentID, batch.ID, BatchStatusQueued, BatchStatusRunning)
	require.NoError(t, err)
	require.True(t, updated)

	updated, err = service.repo.UpdateBatchStatusIfCurrent(ctx, agentID, batch.ID, BatchStatusQueued, BatchStatusRunning)
	require.NoError(t, err)
	require.False(t, updated)
}

func TestBatchStatusUpdatesTouchUpdatedAt(t *testing.T) {
	service, db := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "更新时间问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "更新时间测试"})
	require.NoError(t, err)
	oldTime := time.Now().Add(-time.Hour)
	require.NoError(t, db.Model(&Batch{}).Where("id = ?", batch.ID).Update("updated_at", oldTime).Error)
	require.NoError(t, db.Model(&BatchItem{}).Where("batch_id = ?", batch.ID).Update("updated_at", oldTime).Error)

	_, err = service.StartBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)

	started, err := service.repo.GetBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.True(t, started.UpdatedAt.After(oldTime), "expected batch updated_at to advance after start")

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.True(t, items[0].UpdatedAt.After(oldTime), "expected item updated_at to advance after start")
}

func TestCancelBatchMarksUnfinishedItemsCanceled(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "取消问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "取消测试"})
	require.NoError(t, err)
	_, err = service.StartBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)

	canceled, err := service.CancelBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, BatchStatusCanceled, canceled.Status)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, string(BatchItemStatusCanceled), items[0].Status)
}

func TestExecuteStartedBatchDoesNotOverwriteCanceledItem(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "执行中取消问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "执行中取消"})
	require.NoError(t, err)
	_, err = service.StartBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)

	runner := &cancelingRunner{service: service, agentID: agentID, batchID: batch.ID}
	executed, err := service.ExecuteStartedBatchWithRunnerJudgeAndSummarizer(ctx, agentID, batch.ID, runner, &fakeJudge{}, &fakeSummarizer{})

	require.NoError(t, err)
	require.Equal(t, BatchStatusCanceled, executed.Status)
	require.Len(t, runner.calls, 1)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, string(BatchItemStatusCanceled), items[0].Status)
	require.Empty(t, items[0].WorkflowRunID)
}

func TestExecuteBatchRunsCasesAndStoresResults(t *testing.T) {
	service, _ := setupTestService(t)
	runner := &fakeRunner{
		results: []RunCaseResult{{
			WorkflowRunID: "run-1",
			Outputs:       map[string]interface{}{"answer": "测试回答"},
		}},
	}
	service.SetRunner(runner)
	judge := &fakeJudge{
		results: []JudgeResult{{
			Status:     BatchItemStatusPassed,
			Reason:     "回答解决了用户问题",
			Suggestion: "继续保持",
			Confidence: 0.91,
		}},
	}
	service.SetJudge(judge)
	summarizer := &fakeSummarizer{result: &SummaryResult{Summary: "本次测试整体通过。"}}
	service.SetSummarizer(summarizer)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "真实执行问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "真实执行"})
	require.NoError(t, err)

	executed, err := service.ExecuteBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, BatchStatusCompleted, executed.Status)
	require.Len(t, runner.calls, 1)
	require.Len(t, judge.calls, 1)
	require.Len(t, summarizer.calls, 1)
	require.Equal(t, "真实执行问题", runner.calls[0].CaseSnapshot.Content)
	require.Equal(t, "真实执行问题", judge.calls[0].CaseSnapshot.Content)
	require.Contains(t, judge.calls[0].PromptTemplate, "评分")
	require.Equal(t, "本次测试整体通过。", executed.Summary)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, string(BatchItemStatusPassed), items[0].Status)
	require.Equal(t, "run-1", items[0].WorkflowRunID)
	require.Equal(t, "测试回答", items[0].Outputs["answer"])
	require.Equal(t, "回答解决了用户问题", items[0].JudgeReason)
	require.Equal(t, "继续保持", items[0].JudgeSuggestion)
	require.Equal(t, 0.91, items[0].JudgeConfidence)
	require.Empty(t, items[0].Error)
}

func TestExecuteBatchFallsBackToReviewWhenJudgeIsMissing(t *testing.T) {
	service, _ := setupTestService(t)
	service.SetRunner(&fakeRunner{})
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "需要判分的问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "缺少 judge 执行"})
	require.NoError(t, err)

	executed, err := service.ExecuteBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, 0, executed.PassedCount)
	require.Equal(t, 0, executed.FailedCount)
	require.Equal(t, 1, executed.ReviewCount)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, string(BatchItemStatusReview), items[0].Status)
	require.Contains(t, items[0].JudgeReason, "judge is not configured")
}

func TestExecuteBatchAggregatesJudgeStatuses(t *testing.T) {
	service, _ := setupTestService(t)
	service.SetRunner(&fakeRunner{})
	service.SetJudge(&fakeJudge{
		results: []JudgeResult{
			{Status: BatchItemStatusPassed, Reason: "通过", Confidence: 0.92},
			{Status: BatchItemStatusFailed, Reason: "不通过", Confidence: 0.88},
			{Status: BatchItemStatusReview, Reason: "需复核", Confidence: 0.51},
		},
	})
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	for _, content := range []string{"问题一", "问题二", "问题三"} {
		_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
			Content:    content,
			ScenarioID: scenarioID,
			Status:     CaseStatusEnabled,
		})
		require.NoError(t, err)
	}
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "判分聚合"})
	require.NoError(t, err)

	executed, err := service.ExecuteBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, 1, executed.PassedCount)
	require.Equal(t, 1, executed.FailedCount)
	require.Equal(t, 1, executed.ReviewCount)
}

func TestExecuteStartedBatchStopsWhenBatchIsCanceled(t *testing.T) {
	service, _ := setupTestService(t)
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)
	for _, content := range []string{"问题一", "问题二"} {
		_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
			Content:    content,
			ScenarioID: scenarioID,
			Status:     CaseStatusEnabled,
		})
		require.NoError(t, err)
	}
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "可取消执行"})
	require.NoError(t, err)
	_, err = service.StartBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	runner := &fakeRunner{}
	_, err = service.CancelBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)

	executed, err := service.ExecuteStartedBatchWithRunnerJudgeAndSummarizer(ctx, agentID, batch.ID, runner, &fakeJudge{}, &fakeSummarizer{})

	require.NoError(t, err)
	require.Equal(t, BatchStatusCanceled, executed.Status)
	require.Empty(t, runner.calls)
}

func TestExecuteBatchFallsBackToRuleSummaryWhenSummarizerIsMissing(t *testing.T) {
	service, _ := setupTestService(t)
	service.SetRunner(&fakeRunner{})
	service.SetJudge(&fakeJudge{
		results: []JudgeResult{{Status: BatchItemStatusFailed, Reason: "不通过", Confidence: 0.88}},
	})
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "总结问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "总结测试"})
	require.NoError(t, err)

	executed, err := service.ExecuteBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Contains(t, executed.Summary, "1 个问题未通过")
}

func TestExecuteBatchStoresRunnerErrors(t *testing.T) {
	service, _ := setupTestService(t)
	service.SetRunner(&fakeRunner{err: fmt.Errorf("runner failed")})
	ctx := context.Background()
	agentID := uuid.NewString()
	scenarioID := requireTestScenario(t, service, ctx, agentID)

	_, err := service.CreateCase(ctx, agentID, CreateCaseRequest{
		Content:    "失败问题",
		ScenarioID: scenarioID,
		Status:     CaseStatusEnabled,
	})
	require.NoError(t, err)
	batch, err := service.CreateBatch(ctx, agentID, CreateBatchRequest{Name: "失败执行"})
	require.NoError(t, err)

	executed, err := service.ExecuteBatch(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, BatchStatusCompleted, executed.Status)
	require.Equal(t, 1, executed.FailedCount)

	items, err := service.ListBatchItems(ctx, agentID, batch.ID)
	require.NoError(t, err)
	require.Equal(t, string(BatchItemStatusFailed), items[0].Status)
	require.Contains(t, items[0].Error, "runner failed")
}

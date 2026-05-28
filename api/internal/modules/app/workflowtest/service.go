package workflowtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

const batchStaleFailureMessage = "测试执行长时间无进展，系统已自动停止未完成问题"

var batchItemExecutionTimeout = 10 * time.Minute

const (
	generationPromptExistingCasesMaxTotal       = 30
	generationPromptExistingCasesMaxPerScenario = 5
)

var ErrJudgeModelNotConfigured = errors.New("judge model is not configured")

type Service struct {
	repo                    *Repository
	runner                  Runner
	judge                   Judge
	summarizer              Summarizer
	workflowContextProvider WorkflowContextProvider
	defaultModelResolver    llmdefaultservice.DefaultModelResolver
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetRunner(runner Runner) {
	s.runner = runner
}

func (s *Service) SetJudge(judge Judge) {
	s.judge = judge
}

func (s *Service) SetSummarizer(summarizer Summarizer) {
	s.summarizer = summarizer
}

func (s *Service) SetWorkflowContextProvider(provider WorkflowContextProvider) {
	s.workflowContextProvider = provider
}

func (s *Service) SetDefaultModelResolver(resolver llmdefaultservice.DefaultModelResolver) {
	s.defaultModelResolver = resolver
}

type UpdateSettingsRequest struct {
	JudgePromptTemplate string `json:"judge_prompt_template"`
	JudgeModelProvider  string `json:"judge_model_provider"`
	JudgeModelName      string `json:"judge_model_name"`
}

type CreateScenarioRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"`
}

type SaveScenarioItemRequest struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SaveScenariosRequest struct {
	Scenarios []SaveScenarioItemRequest `json:"scenarios"`
}

type CreateCaseRequest struct {
	Content        string     `json:"content"`
	ExpectedResult string     `json:"expected_result,omitempty"`
	ScenarioID     string     `json:"scenario_id,omitempty"`
	QuestionType   string     `json:"question_type"`
	Status         string     `json:"status"`
	Turns          []CaseTurn `json:"turns"`
}

type UpdateCaseRequest struct {
	Content        string     `json:"content"`
	ExpectedResult string     `json:"expected_result,omitempty"`
	ScenarioID     string     `json:"scenario_id,omitempty"`
	QuestionType   string     `json:"question_type"`
	Status         string     `json:"status"`
	Turns          []CaseTurn `json:"turns"`
}

type DeleteCasesRequest struct {
	CaseIDs []string `json:"case_ids"`
}

type CreateBatchRequest struct {
	Name                string   `json:"name"`
	CaseIDs             []string `json:"case_ids,omitempty"`
	WorkflowVersionMode string   `json:"workflow_version_mode,omitempty"`
	WorkflowVersionUUID string   `json:"workflow_version_uuid,omitempty"`
}

type RetestBatchRequest struct {
	Name string `json:"name"`
}

func normalizeCaseStatus(status string) (string, error) {
	if status == "" {
		return CaseStatusEnabled, nil
	}
	if status != CaseStatusEnabled && status != CaseStatusDisabled {
		return "", fmt.Errorf("invalid case status")
	}
	return status, nil
}

func normalizeQuestionType(questionType string) (string, error) {
	if questionType == "" {
		return CaseTypeCore, nil
	}
	if questionType != CaseTypeCore && questionType != CaseTypeExtension && questionType != CaseTypeFuzzy {
		return "", fmt.Errorf("invalid question type")
	}
	return questionType, nil
}

const (
	WorkflowVersionModeDraft             = "draft"
	WorkflowVersionModeLatestPublished   = "latest_published"
	WorkflowVersionModeSpecificPublished = "specific_published"
	WorkflowVersionLabelCurrentDraft     = "current_draft"
)

func normalizeWorkflowVersionScope(mode string, versionUUID string) (string, string, string, error) {
	mode = strings.TrimSpace(mode)
	versionUUID = strings.TrimSpace(versionUUID)
	if mode == "" {
		mode = WorkflowVersionModeDraft
	}
	switch mode {
	case WorkflowVersionModeDraft:
		if versionUUID != "" {
			return "", "", "", fmt.Errorf("workflow version uuid is only supported for published versions")
		}
		return mode, "", WorkflowVersionLabelCurrentDraft, nil
	case WorkflowVersionModeLatestPublished, WorkflowVersionModeSpecificPublished:
		return "", "", "", fmt.Errorf("workflow version mode %s is reserved but not supported yet", mode)
	default:
		return "", "", "", fmt.Errorf("invalid workflow version mode")
	}
}

func (s *Service) GetSettings(ctx context.Context, agentID string) (*Setting, error) {
	settings, err := s.repo.GetSettings(ctx, agentID)
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now()
	return &Setting{
		ID:                  newID(),
		AgentID:             agentID,
		JudgePromptTemplate: DefaultJudgePromptTemplate,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, nil
}

func (s *Service) UpdateSettings(ctx context.Context, agentID string, req UpdateSettingsRequest) (*Setting, error) {
	prompt := strings.TrimSpace(req.JudgePromptTemplate)
	if prompt == "" {
		return nil, fmt.Errorf("judge_prompt_template is required")
	}
	provider := strings.TrimSpace(req.JudgeModelProvider)
	modelName := strings.TrimSpace(req.JudgeModelName)
	if (provider == "") != (modelName == "") {
		return nil, fmt.Errorf("judge model provider and name must be provided together")
	}

	now := time.Now()
	settings := &Setting{
		ID:                  newID(),
		AgentID:             agentID,
		JudgePromptTemplate: prompt,
		JudgeModelProvider:  provider,
		JudgeModelName:      modelName,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := s.repo.UpsertSettings(ctx, settings); err != nil {
		return nil, err
	}
	return s.GetSettings(ctx, agentID)
}

func (s *Service) ResetSettings(ctx context.Context, agentID string) (*Setting, error) {
	existing, err := s.GetSettings(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return s.UpdateSettings(ctx, agentID, UpdateSettingsRequest{
		JudgePromptTemplate: DefaultJudgePromptTemplate,
		JudgeModelProvider:  existing.JudgeModelProvider,
		JudgeModelName:      existing.JudgeModelName,
	})
}

func (s *Service) resolveBatchJudgeSettings(ctx context.Context, agentID string) (*Setting, error) {
	settings, err := s.GetSettings(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(settings.JudgeModelProvider) != "" && strings.TrimSpace(settings.JudgeModelName) != "" {
		return settings, nil
	}
	if s.defaultModelResolver == nil {
		return nil, ErrJudgeModelNotConfigured
	}

	organizationID, err := s.repo.GetAgentOrganizationID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("resolve judge model organization: %w", err)
	}
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return nil, ErrJudgeModelNotConfigured
	}

	resolved, err := s.defaultModelResolver.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseTextChat, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve default judge model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Provider) == "" || strings.TrimSpace(resolved.Model) == "" {
		return nil, ErrJudgeModelNotConfigured
	}

	copy := *settings
	copy.JudgeModelProvider = strings.TrimSpace(resolved.Provider)
	copy.JudgeModelName = strings.TrimSpace(resolved.Model)
	return &copy, nil
}

func (s *Service) CreateScenario(ctx context.Context, agentID string, req CreateScenarioRequest) (*Scenario, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	existing, err := s.repo.GetScenarioByName(ctx, agentID, name)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	now := time.Now()
	scenario := &Scenario{
		ID:          newID(),
		AgentID:     agentID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Source:      normalizeScenarioSource(req.Source),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.CreateScenario(ctx, scenario); err != nil {
		return nil, err
	}
	return scenario, nil
}

func (s *Service) ListScenarios(ctx context.Context, agentID string) ([]Scenario, error) {
	return s.repo.ListScenarios(ctx, agentID)
}

func (s *Service) SaveScenarios(ctx context.Context, agentID string, req SaveScenariosRequest) ([]Scenario, error) {
	existing, err := s.ListScenarios(ctx, agentID)
	if err != nil {
		return nil, err
	}
	existingByID := make(map[string]Scenario, len(existing))
	for _, scenario := range existing {
		existingByID[scenario.ID] = scenario
	}

	type normalizedScenarioItem struct {
		ID          string
		Name        string
		Description string
	}
	seenNames := map[string]struct{}{}
	keptExistingIDs := map[string]struct{}{}
	normalizedItems := make([]normalizedScenarioItem, 0, len(req.Scenarios))
	for _, item := range req.Scenarios {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		if _, ok := seenNames[name]; ok {
			return nil, fmt.Errorf("duplicate scenario name")
		}
		seenNames[name] = struct{}{}
		if item.ID != "" {
			if _, ok := existingByID[item.ID]; !ok {
				return nil, fmt.Errorf("scenario not found")
			}
			keptExistingIDs[item.ID] = struct{}{}
		}
		normalizedItems = append(normalizedItems, normalizedScenarioItem{
			ID:          item.ID,
			Name:        name,
			Description: strings.TrimSpace(item.Description),
		})
	}

	for _, scenario := range existing {
		if _, ok := keptExistingIDs[scenario.ID]; !ok {
			if scenario.CaseCount > 0 {
				return nil, fmt.Errorf("scenario has assigned cases")
			}
			count, err := s.repo.CountCasesByScenario(ctx, agentID, scenario.ID)
			if err != nil {
				return nil, err
			}
			if count > 0 {
				return nil, fmt.Errorf("scenario has assigned cases")
			}
		}
	}

	keptIDs := map[string]struct{}{}
	for _, item := range normalizedItems {
		if item.ID == "" {
			created, err := s.CreateScenario(ctx, agentID, CreateScenarioRequest{
				Name:        item.Name,
				Description: item.Description,
				Source:      "manual",
			})
			if err != nil {
				return nil, err
			}
			keptIDs[created.ID] = struct{}{}
			continue
		}

		scenario := existingByID[item.ID]
		scenario.Name = item.Name
		scenario.Description = item.Description
		scenario.UpdatedAt = time.Now()
		if err := s.repo.UpdateScenario(ctx, &scenario); err != nil {
			return nil, err
		}
		keptIDs[scenario.ID] = struct{}{}
	}

	for _, scenario := range existing {
		if _, ok := keptIDs[scenario.ID]; !ok {
			if err := s.repo.DeleteScenario(ctx, agentID, scenario.ID); err != nil {
				return nil, err
			}
		}
	}
	if err := s.refreshScenarioCaseCounts(ctx, agentID); err != nil {
		return nil, err
	}
	return s.ListScenarios(ctx, agentID)
}

func (s *Service) CreateCase(ctx context.Context, agentID string, req CreateCaseRequest) (*Case, error) {
	content, turns, err := normalizeCaseContentAndTurns(req.Content, req.Turns)
	if err != nil {
		return nil, err
	}
	expectedResult := normalizeExpectedResult(req.ExpectedResult)
	status, err := normalizeCaseStatus(req.Status)
	if err != nil {
		return nil, err
	}
	questionType, err := normalizeQuestionType(req.QuestionType)
	if err != nil {
		return nil, err
	}
	scenarioID, err := s.requireScenarioBelongsToAgent(ctx, agentID, req.ScenarioID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	testCase := &Case{
		ID:             newID(),
		AgentID:        agentID,
		ScenarioID:     scenarioID,
		Content:        content,
		ExpectedResult: expectedResult,
		QuestionType:   questionType,
		Status:         status,
		Turns:          turns,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateCase(ctx, testCase); err != nil {
		return nil, err
	}
	if scenarioID != nil {
		if err := s.repo.IncrementScenarioCaseCount(ctx, agentID, *scenarioID, 1); err != nil {
			return nil, err
		}
	}
	if scenarioID == nil {
		if err := s.refreshScenarioCaseCounts(ctx, agentID); err != nil {
			return nil, err
		}
	}
	return testCase, nil
}

func (s *Service) UpdateCase(ctx context.Context, agentID string, caseID string, req UpdateCaseRequest) (*Case, error) {
	existing, err := s.repo.GetCase(ctx, agentID, caseID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("case not found")
		}
		return nil, err
	}
	content, turns, err := normalizeCaseContentAndTurns(req.Content, req.Turns)
	if err != nil {
		return nil, err
	}
	expectedResult := normalizeExpectedResult(req.ExpectedResult)
	status, err := normalizeCaseStatus(req.Status)
	if err != nil {
		return nil, err
	}
	questionType, err := normalizeQuestionType(req.QuestionType)
	if err != nil {
		return nil, err
	}
	scenarioID, err := s.requireScenarioBelongsToAgent(ctx, agentID, req.ScenarioID)
	if err != nil {
		return nil, err
	}
	previousScenarioID := ""
	if existing.ScenarioID != nil {
		previousScenarioID = *existing.ScenarioID
	}
	existing.ScenarioID = scenarioID
	existing.Content = content
	existing.ExpectedResult = expectedResult
	existing.QuestionType = questionType
	existing.Status = status
	existing.Turns = turns
	existing.UpdatedAt = time.Now()
	if err := s.repo.UpdateCase(ctx, existing); err != nil {
		return nil, err
	}
	nextScenarioID := ""
	if scenarioID != nil {
		nextScenarioID = *scenarioID
	}
	if previousScenarioID != nextScenarioID {
		if previousScenarioID != "" {
			if err := s.repo.IncrementScenarioCaseCount(ctx, agentID, previousScenarioID, -1); err != nil {
				return nil, err
			}
		}
		if nextScenarioID != "" {
			if err := s.repo.IncrementScenarioCaseCount(ctx, agentID, nextScenarioID, 1); err != nil {
				return nil, err
			}
		}
	}
	return s.repo.GetCase(ctx, agentID, caseID)
}

func (s *Service) DeleteCases(ctx context.Context, agentID string, caseIDs []string) error {
	uniqueIDs := make([]string, 0, len(caseIDs))
	seen := make(map[string]struct{}, len(caseIDs))
	for _, id := range caseIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return fmt.Errorf("case_ids is required")
	}
	if err := s.repo.DeleteCases(ctx, agentID, uniqueIDs); err != nil {
		return err
	}
	return s.refreshScenarioCaseCounts(ctx, agentID)
}

func normalizeExpectedResult(expectedResult string) string {
	return strings.TrimSpace(expectedResult)
}

func normalizeCaseContentAndTurns(content string, reqTurns []CaseTurn) (string, CaseTurns, error) {
	normalizedContent := strings.TrimSpace(content)
	turns := make(CaseTurns, 0, len(reqTurns))
	for _, turn := range reqTurns {
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "user"
		}
		turnContent := strings.TrimSpace(turn.Content)
		if turnContent == "" && len(turn.Attachments) == 0 {
			continue
		}
		turns = append(turns, CaseTurn{
			Role:        role,
			Content:     turnContent,
			Attachments: turn.Attachments,
		})
		if normalizedContent == "" && turnContent != "" {
			normalizedContent = turnContent
		}
	}
	if len(turns) == 0 && normalizedContent != "" {
		turns = CaseTurns{{Role: "user", Content: normalizedContent}}
	}
	if normalizedContent == "" {
		return "", nil, fmt.Errorf("content is required")
	}
	return normalizedContent, turns, nil
}

func (s *Service) ensureScenarioBelongsToAgent(ctx context.Context, agentID string, scenarioID string) error {
	scenarios, err := s.ListScenarios(ctx, agentID)
	if err != nil {
		return err
	}
	for _, scenario := range scenarios {
		if scenario.ID == scenarioID {
			return nil
		}
	}
	return fmt.Errorf("scenario not found")
}

func (s *Service) requireScenarioBelongsToAgent(ctx context.Context, agentID string, scenarioID string) (*string, error) {
	value := strings.TrimSpace(scenarioID)
	if value == "" {
		return nil, fmt.Errorf("scenario_id is required")
	}
	if err := s.ensureScenarioBelongsToAgent(ctx, agentID, value); err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *Service) ListCases(ctx context.Context, agentID string, status string) ([]Case, error) {
	return s.repo.ListCases(ctx, agentID, strings.TrimSpace(status))
}

func (s *Service) CreateGenerationTask(ctx context.Context, agentID, workspaceID, accountID string, req CreateGenerationTaskRequest) (*GenerationTask, error) {
	if req.Count < minGeneratedCaseCount || req.Count > maxGeneratedCaseCount {
		return nil, fmt.Errorf("count must be between %d and %d", minGeneratedCaseCount, maxGeneratedCaseCount)
	}
	scenarioIDs := normalizeGenerateScenarioIDs(req)
	if len(scenarioIDs) == 0 {
		return nil, fmt.Errorf("at least one scenario is required")
	}
	for _, scenarioID := range scenarioIDs {
		if err := s.ensureScenarioBelongsToAgent(ctx, agentID, scenarioID); err != nil {
			return nil, err
		}
	}
	activeTask, err := s.repo.GetActiveGenerationTask(ctx, agentID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if activeTask != nil {
		return nil, fmt.Errorf("generation task is already running")
	}

	model := normalizeModel(req.Model)
	modelProvider := ""
	modelName := ""
	if model != nil {
		modelProvider = model.Provider
		modelName = model.Name
	}
	turnStrategy := strings.TrimSpace(req.TurnStrategy)
	if turnStrategy == "" {
		turnStrategy = "mixed"
	}
	questionTypes := normalizeQuestionTypes(req.QuestionTypes)
	if len(questionTypes) == 0 {
		questionTypes = []string{CaseTypeCore, CaseTypeExtension, CaseTypeFuzzy}
	}
	now := time.Now()
	task := &GenerationTask{
		ID:             newID(),
		AgentID:        agentID,
		WorkspaceID:    workspaceID,
		AccountID:      accountID,
		Status:         GenerationTaskStatusQueued,
		RequestedCount: req.Count,
		ScenarioIDs:    JSONList(scenarioIDs),
		QuestionTypes:  JSONList(questionTypes),
		TurnStrategy:   turnStrategy,
		Prompt:         strings.TrimSpace(req.Prompt),
		Context:        strings.TrimSpace(req.Context),
		ModelProvider:  modelProvider,
		ModelName:      modelName,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateGenerationTask(ctx, task); err != nil {
		if isActiveGenerationTaskConflict(err) {
			return nil, fmt.Errorf("generation task is already running")
		}
		return nil, err
	}
	return task, nil
}

func (s *Service) GetActiveGenerationTask(ctx context.Context, agentID string) (*GenerationTask, error) {
	task, err := s.repo.GetActiveGenerationTask(ctx, agentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return task, err
}

func (s *Service) GetLatestGenerationTask(ctx context.Context, agentID string) (*GenerationTask, error) {
	task, err := s.repo.GetLatestGenerationTask(ctx, agentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return task, err
}

func (s *Service) GetGenerationTask(ctx context.Context, agentID, taskID string) (*GenerationTask, error) {
	return s.repo.GetGenerationTask(ctx, agentID, taskID)
}

func (s *Service) CancelGenerationTask(ctx context.Context, agentID, taskID string) (*GenerationTask, error) {
	if _, err := s.repo.CancelGenerationTask(ctx, agentID, taskID, time.Now()); err != nil {
		return nil, err
	}
	return s.GetGenerationTask(ctx, agentID, taskID)
}

func (s *Service) RunGenerationTask(ctx context.Context, taskID string, client llmclient.LLMClient) error {
	task, err := s.repo.GetGenerationTaskByID(ctx, taskID)
	if err != nil {
		return err
	}
	if isTerminalGenerationTaskStatus(task.Status) {
		return nil
	}
	if task.Status == GenerationTaskStatusCanceling {
		return s.finishGenerationTask(ctx, task.ID, GenerationTaskStatusCanceled, "")
	}
	if task.Status != GenerationTaskStatusQueued {
		return nil
	}
	changed, err := s.repo.MarkGenerationTaskRunning(ctx, task.ID, time.Now())
	if err != nil {
		return err
	}
	if !changed {
		task, err = s.repo.GetGenerationTaskByID(ctx, task.ID)
		if err != nil {
			return err
		}
		if isTerminalGenerationTaskStatus(task.Status) {
			return nil
		}
		if task.Status == GenerationTaskStatusCanceling {
			return s.finishGenerationTask(ctx, task.ID, GenerationTaskStatusCanceled, "")
		}
		return nil
	}
	task, err = s.repo.GetGenerationTaskByID(ctx, task.ID)
	if err != nil {
		return err
	}
	if isTerminalGenerationTaskStatus(task.Status) {
		return nil
	}
	if task.Status == GenerationTaskStatusCanceling {
		return s.finishGenerationTask(ctx, task.ID, GenerationTaskStatusCanceled, "")
	}
	if task.Status != GenerationTaskStatusRunning {
		return nil
	}

	checkCanceled := func(ctx context.Context) error {
		current, err := s.repo.GetGenerationTaskByID(ctx, task.ID)
		if err != nil {
			return err
		}
		if current.Status == GenerationTaskStatusCanceling {
			if err := s.finishGenerationTask(ctx, task.ID, GenerationTaskStatusCanceled, ""); err != nil {
				return err
			}
			return errGenerationTaskCanceled
		}
		return nil
	}
	generator := &LLMCaseGenerator{
		Client:      client,
		WorkspaceID: task.WorkspaceID,
		AccountID:   task.AccountID,
		AgentID:     task.AgentID,
	}
	req := generationTaskGenerateCasesRequest(task)
	_, err = s.generateCasesForScenarios(ctx, task.AgentID, req, []string(task.ScenarioIDs), generator, generateCaseProgressHooks{
		BeforeScenario: checkCanceled,
		AfterCreate: func(ctx context.Context, item Case) error {
			if err := s.repo.IncrementGenerationTaskCreatedCount(ctx, task.ID, 1); err != nil {
				return err
			}
			return checkCanceled(ctx)
		},
	})
	if err != nil {
		if errors.Is(err, errGenerationTaskCanceled) {
			return nil
		}
		reason := generationTaskFailureReason(err)
		if finishErr := s.finishGenerationTask(ctx, task.ID, GenerationTaskStatusFailed, reason); finishErr != nil {
			// Do not retry the whole asynq task here: generation may have already
			// created cases. Stale running rows are repaired by the repository
			// recovery hook without re-running generation.
			return errors.Join(err, finishErr)
		}
		return err
	}
	return s.finishGenerationTask(ctx, task.ID, GenerationTaskStatusCompleted, "")
}

func (s *Service) RecoverStaleRunningGenerationTasks(ctx context.Context, staleBefore time.Time) (int64, error) {
	return s.repo.RecoverStaleRunningGenerationTasks(ctx, staleBefore, generationTaskFailureReason(fmt.Errorf("worker stopped before marking task terminal")), time.Now())
}

type generateCaseProgressHooks struct {
	BeforeScenario func(context.Context) error
	AfterCreate    func(context.Context, Case) error
}

var errGenerationTaskCanceled = errors.New("generation task canceled")

func (s *Service) finishGenerationTask(ctx context.Context, taskID, status, reason string) error {
	now := time.Now()
	return s.repo.UpdateGenerationTaskStatus(ctx, taskID, status, map[string]interface{}{
		"completed_at": now,
		"error":        reason,
	})
}

func isTerminalGenerationTaskStatus(status string) bool {
	return status == GenerationTaskStatusCompleted ||
		status == GenerationTaskStatusCanceled ||
		status == GenerationTaskStatusFailed
}

func generationTaskGenerateCasesRequest(task *GenerationTask) GenerateCasesRequest {
	req := GenerateCasesRequest{
		Count:         task.RequestedCount,
		ScenarioIDs:   []string(task.ScenarioIDs),
		QuestionTypes: []string(task.QuestionTypes),
		TurnStrategy:  task.TurnStrategy,
		Prompt:        task.Prompt,
		Context:       task.Context,
	}
	if strings.TrimSpace(task.ModelProvider) != "" && strings.TrimSpace(task.ModelName) != "" {
		req.Model = &Model{
			Provider: strings.TrimSpace(task.ModelProvider),
			Name:     strings.TrimSpace(task.ModelName),
		}
	}
	return req
}

func generationTaskFailureReason(err error) string {
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		return "生成测试问题失败"
	}
	return "生成测试问题失败：" + err.Error()
}

func isActiveGenerationTaskConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "idx_workflow_test_generation_tasks_active_agent") &&
		!strings.Contains(message, "workflow_test_generation_tasks") {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return strings.Contains(message, "unique") ||
		strings.Contains(message, "duplicate") ||
		strings.Contains(message, "duplicated")
}

func isActiveScenarioRecognitionTaskConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if !strings.Contains(message, "idx_workflow_test_scenario_recognition_tasks_active_agent") &&
		!strings.Contains(message, "workflow_test_scenario_recognition_tasks") {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return strings.Contains(message, "unique") ||
		strings.Contains(message, "duplicate") ||
		strings.Contains(message, "duplicated")
}

func (s *Service) GenerateCases(ctx context.Context, agentID string, req GenerateCasesRequest, generator CaseGenerator) (*GenerateCasesResult, error) {
	if req.Count < minGeneratedCaseCount || req.Count > maxGeneratedCaseCount {
		return nil, fmt.Errorf("count must be between %d and %d", minGeneratedCaseCount, maxGeneratedCaseCount)
	}
	if generator == nil {
		return nil, fmt.Errorf("case generator is not configured")
	}
	scenarioIDs := normalizeGenerateScenarioIDs(req)
	if len(scenarioIDs) == 0 {
		return nil, fmt.Errorf("at least one scenario is required")
	}
	for _, scenarioID := range scenarioIDs {
		if err := s.ensureScenarioBelongsToAgent(ctx, agentID, scenarioID); err != nil {
			return nil, err
		}
	}
	return s.generateCasesForScenarios(ctx, agentID, req, scenarioIDs, generator, generateCaseProgressHooks{})
}

func (s *Service) generateCasesForScenarios(ctx context.Context, agentID string, req GenerateCasesRequest, scenarioIDs []string, generator CaseGenerator, hooks generateCaseProgressHooks) (*GenerateCasesResult, error) {
	items := make([]Case, 0, req.Count)
	generatedCases := make([]GeneratedCase, 0, req.Count)
	if hooks.BeforeScenario != nil {
		if err := hooks.BeforeScenario(ctx); err != nil {
			return nil, err
		}
	}
	generateReq := req
	generateReq.Count = req.Count
	generateReq.ScenarioID = ""
	generateReq.ScenarioIDs = scenarioIDs
	generateReq.WorkflowContext = s.resolveWorkflowRecognitionContext(ctx, agentID)
	scenarios, err := s.ListScenarios(ctx, agentID)
	if err != nil {
		return nil, err
	}
	scenarioByID := make(map[string]Scenario, len(scenarios))
	for _, scenario := range scenarios {
		scenarioByID[scenario.ID] = scenario
	}
	generateReq.Scenarios = make([]Scenario, 0, len(scenarioIDs))
	for _, scenarioID := range scenarioIDs {
		if scenario, ok := scenarioByID[scenarioID]; ok {
			generateReq.Scenarios = append(generateReq.Scenarios, scenario)
		}
	}
	existingCases, err := s.ListCases(ctx, agentID, "")
	if err != nil {
		return nil, err
	}
	generateReq.ExistingCases = selectExistingCasesForGenerationPrompt(existingCases, scenarioIDs)
	generated, err := generator.GenerateCases(ctx, generateReq)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeGeneratedCases(generated)
	if err != nil {
		return nil, err
	}
	if len(normalized) > req.Count {
		normalized = normalized[:req.Count]
	}
	for index, item := range normalized {
		scenarioID := scenarioIDs[index%len(scenarioIDs)]
		if itemScenarioID := strings.TrimSpace(item.ScenarioID); itemScenarioID != "" {
			if _, ok := scenarioByID[itemScenarioID]; ok {
				scenarioID = itemScenarioID
			}
		}
		created, err := s.CreateCase(ctx, agentID, CreateCaseRequest{
			Content:        item.Content,
			ExpectedResult: item.ExpectedResult,
			ScenarioID:     scenarioID,
			QuestionType:   item.QuestionType,
			Status:         CaseStatusEnabled,
			Turns:          []CaseTurn{{Role: "user", Content: item.Content}},
		})
		if err != nil {
			return nil, err
		}
		items = append(items, *created)
		generatedCases = append(generatedCases, item)
		if hooks.AfterCreate != nil {
			if err := hooks.AfterCreate(ctx, *created); err != nil {
				return nil, err
			}
		}
	}
	return &GenerateCasesResult{Cases: generatedCases, Items: items}, nil
}

func selectExistingCasesForGenerationPrompt(cases []Case, scenarioIDs []string) []Case {
	if len(cases) == 0 || len(scenarioIDs) == 0 {
		return nil
	}

	selectedScenarioSet := make(map[string]struct{}, len(scenarioIDs))
	orderedScenarioIDs := make([]string, 0, len(scenarioIDs))
	for _, scenarioID := range scenarioIDs {
		scenarioID = strings.TrimSpace(scenarioID)
		if scenarioID == "" {
			continue
		}
		if _, exists := selectedScenarioSet[scenarioID]; exists {
			continue
		}
		selectedScenarioSet[scenarioID] = struct{}{}
		orderedScenarioIDs = append(orderedScenarioIDs, scenarioID)
	}
	if len(orderedScenarioIDs) == 0 {
		return nil
	}

	grouped := make(map[string][]Case, len(orderedScenarioIDs))
	for _, testCase := range cases {
		if testCase.ScenarioID == nil {
			continue
		}
		scenarioID := strings.TrimSpace(*testCase.ScenarioID)
		if _, ok := selectedScenarioSet[scenarioID]; !ok {
			continue
		}
		if len(grouped[scenarioID]) >= generationPromptExistingCasesMaxPerScenario {
			continue
		}
		grouped[scenarioID] = append(grouped[scenarioID], testCase)
	}

	selected := make([]Case, 0, minInt(len(cases), generationPromptExistingCasesMaxTotal))
	for _, scenarioID := range orderedScenarioIDs {
		for _, testCase := range grouped[scenarioID] {
			if len(selected) >= generationPromptExistingCasesMaxTotal {
				return selected
			}
			selected = append(selected, testCase)
		}
	}
	return selected
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func normalizeGenerateScenarioIDs(req GenerateCasesRequest) []string {
	values := make([]string, 0, len(req.ScenarioIDs)+1)
	values = append(values, req.ScenarioIDs...)
	if req.ScenarioID != "" {
		values = append(values, req.ScenarioID)
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		scenarioID := strings.TrimSpace(value)
		if scenarioID == "" {
			continue
		}
		if _, ok := seen[scenarioID]; ok {
			continue
		}
		seen[scenarioID] = struct{}{}
		result = append(result, scenarioID)
	}
	return result
}

func (s *Service) RecognizeScenarios(ctx context.Context, agentID string, req RecognizeScenariosRequest, recognizer ScenarioRecognizer) (*ScenarioRecognitionResult, error) {
	return s.recognizeScenarios(ctx, agentID, req, s.resolveWorkflowRecognitionContext(ctx, agentID), recognizer)
}

func (s *Service) recognizeScenarios(ctx context.Context, agentID string, req RecognizeScenariosRequest, workflowContext string, recognizer ScenarioRecognizer) (*ScenarioRecognitionResult, error) {
	if recognizer == nil {
		return nil, fmt.Errorf("scenario recognizer is not configured")
	}
	cases, err := s.ListCases(ctx, agentID, "")
	if err != nil {
		return nil, err
	}
	existingScenarios, err := s.ListScenarios(ctx, agentID)
	if err != nil {
		return nil, err
	}
	recognized, err := recognizer.RecognizeScenarios(ctx, ScenarioRecognitionInput{
		AgentID:           agentID,
		Context:           strings.TrimSpace(req.Context),
		WorkflowContext:   strings.TrimSpace(workflowContext),
		Prompt:            strings.TrimSpace(req.Prompt),
		Model:             normalizeModel(req.Model),
		Cases:             cases,
		ExistingScenarios: existingScenarios,
	})
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeScenarioRecognitionResult(recognized)
	if err != nil {
		return nil, err
	}

	scenariosByName := make(map[string]Scenario)
	for _, scenario := range normalized.Scenarios {
		created, err := s.CreateScenario(ctx, agentID, CreateScenarioRequest{
			Name:        scenario.Name,
			Description: scenario.Description,
			Source:      "ai",
		})
		if err != nil {
			return nil, err
		}
		scenariosByName[scenario.Name] = *created
	}
	caseIDs := map[string]struct{}{}
	for _, item := range cases {
		caseIDs[item.ID] = struct{}{}
	}
	for _, assignment := range normalized.Assignments {
		if _, ok := caseIDs[assignment.CaseID]; !ok {
			continue
		}
		scenario, ok := scenariosByName[assignment.ScenarioName]
		if !ok {
			continue
		}
		scenarioID := scenario.ID
		if err := s.repo.UpdateCaseScenario(ctx, agentID, assignment.CaseID, &scenarioID); err != nil {
			return nil, err
		}
	}
	if err := s.refreshScenarioCaseCounts(ctx, agentID); err != nil {
		return nil, err
	}
	updatedScenarios, err := s.ListScenarios(ctx, agentID)
	if err != nil {
		return nil, err
	}
	updatedCases, err := s.ListCases(ctx, agentID, "")
	if err != nil {
		return nil, err
	}
	return &ScenarioRecognitionResult{
		Scenarios:   scenariosToRecognizedScenarios(updatedScenarios),
		Assignments: normalized.Assignments,
		Cases:       updatedCases,
	}, nil
}

type ScenarioRecognitionTaskResponse struct {
	Task *ScenarioRecognitionTask `json:"task"`
}

func (s *Service) CreateScenarioRecognitionTask(ctx context.Context, agentID, workspaceID, accountID string, req RecognizeScenariosRequest) (*ScenarioRecognitionTask, error) {
	activeTask, err := s.repo.GetActiveScenarioRecognitionTask(ctx, agentID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if activeTask != nil {
		return nil, fmt.Errorf("scenario recognition task is already running")
	}

	model := normalizeModel(req.Model)
	modelProvider := ""
	modelName := ""
	if model != nil {
		modelProvider = model.Provider
		modelName = model.Name
	}
	now := time.Now()
	task := &ScenarioRecognitionTask{
		ID:                      newID(),
		AgentID:                 agentID,
		WorkspaceID:             workspaceID,
		AccountID:               accountID,
		Status:                  GenerationTaskStatusQueued,
		Prompt:                  strings.TrimSpace(req.Prompt),
		Context:                 strings.TrimSpace(req.Context),
		WorkflowContextSnapshot: s.resolveWorkflowRecognitionContext(ctx, agentID),
		ModelProvider:           modelProvider,
		ModelName:               modelName,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := s.repo.CreateScenarioRecognitionTask(ctx, task); err != nil {
		if isActiveScenarioRecognitionTaskConflict(err) {
			return nil, fmt.Errorf("scenario recognition task is already running")
		}
		return nil, err
	}
	return task, nil
}

func (s *Service) GetActiveScenarioRecognitionTask(ctx context.Context, agentID string) (*ScenarioRecognitionTask, error) {
	task, err := s.repo.GetActiveScenarioRecognitionTask(ctx, agentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return task, err
}

func (s *Service) GetLatestScenarioRecognitionTask(ctx context.Context, agentID string) (*ScenarioRecognitionTask, error) {
	task, err := s.repo.GetLatestScenarioRecognitionTask(ctx, agentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return task, err
}

func (s *Service) GetScenarioRecognitionTask(ctx context.Context, agentID, taskID string) (*ScenarioRecognitionTask, error) {
	return s.repo.GetScenarioRecognitionTask(ctx, agentID, taskID)
}

func (s *Service) RunScenarioRecognitionTask(ctx context.Context, taskID string, recognizer ScenarioRecognizer) error {
	task, err := s.repo.GetScenarioRecognitionTaskByID(ctx, taskID)
	if err != nil {
		return err
	}
	if isTerminalGenerationTaskStatus(task.Status) {
		return nil
	}
	if task.Status == GenerationTaskStatusCanceling {
		return s.finishScenarioRecognitionTask(ctx, task.ID, GenerationTaskStatusCanceled, "", 0, 0)
	}
	if task.Status != GenerationTaskStatusQueued {
		return nil
	}
	changed, err := s.repo.MarkScenarioRecognitionTaskRunning(ctx, task.ID, time.Now())
	if err != nil {
		return err
	}
	if !changed {
		task, err = s.repo.GetScenarioRecognitionTaskByID(ctx, task.ID)
		if err != nil {
			return err
		}
		if isTerminalGenerationTaskStatus(task.Status) {
			return nil
		}
		if task.Status == GenerationTaskStatusCanceling {
			return s.finishScenarioRecognitionTask(ctx, task.ID, GenerationTaskStatusCanceled, "", 0, 0)
		}
		return nil
	}
	task, err = s.repo.GetScenarioRecognitionTaskByID(ctx, task.ID)
	if err != nil {
		return err
	}
	if isTerminalGenerationTaskStatus(task.Status) {
		return nil
	}
	if task.Status == GenerationTaskStatusCanceling {
		return s.finishScenarioRecognitionTask(ctx, task.ID, GenerationTaskStatusCanceled, "", 0, 0)
	}
	if task.Status != GenerationTaskStatusRunning {
		return nil
	}

	result, err := s.recognizeScenarios(ctx, task.AgentID, scenarioRecognitionTaskRequest(task), task.WorkflowContextSnapshot, recognizer)
	if err != nil {
		reason := scenarioRecognitionTaskFailureReason(err)
		if finishErr := s.finishScenarioRecognitionTask(ctx, task.ID, GenerationTaskStatusFailed, reason, 0, 0); finishErr != nil {
			return errors.Join(err, finishErr)
		}
		return err
	}
	return s.finishScenarioRecognitionTask(ctx, task.ID, GenerationTaskStatusCompleted, "", result.RecognizedCount(), result.AssignedCaseCount())
}

func (s *Service) RecoverStaleRunningScenarioRecognitionTasks(ctx context.Context, staleBefore time.Time) (int64, error) {
	return s.repo.RecoverStaleRunningScenarioRecognitionTasks(ctx, staleBefore, scenarioRecognitionTaskFailureReason(fmt.Errorf("worker stopped before marking task terminal")), time.Now())
}

func (s *Service) finishScenarioRecognitionTask(ctx context.Context, taskID, status, reason string, recognizedCount, assignedCaseCount int) error {
	now := time.Now()
	return s.repo.UpdateScenarioRecognitionTaskStatus(ctx, taskID, status, map[string]interface{}{
		"completed_at":        now,
		"error":               reason,
		"recognized_count":    recognizedCount,
		"assigned_case_count": assignedCaseCount,
	})
}

func scenarioRecognitionTaskRequest(task *ScenarioRecognitionTask) RecognizeScenariosRequest {
	req := RecognizeScenariosRequest{
		Prompt:  task.Prompt,
		Context: task.Context,
	}
	if strings.TrimSpace(task.ModelProvider) != "" && strings.TrimSpace(task.ModelName) != "" {
		req.Model = &Model{
			Provider: strings.TrimSpace(task.ModelProvider),
			Name:     strings.TrimSpace(task.ModelName),
		}
	}
	return req
}

func scenarioRecognitionTaskFailureReason(err error) string {
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		return "识别业务场景失败"
	}
	return "识别业务场景失败：" + err.Error()
}

func (s *Service) resolveWorkflowRecognitionContext(ctx context.Context, agentID string) string {
	if s == nil || s.workflowContextProvider == nil {
		return ""
	}
	return strings.TrimSpace(s.workflowContextProvider.WorkflowRecognitionContext(ctx, agentID))
}

func normalizeModel(model *Model) *Model {
	if model == nil {
		return nil
	}
	provider := strings.TrimSpace(model.Provider)
	name := strings.TrimSpace(model.Name)
	if provider == "" || name == "" {
		return nil
	}
	return &Model{Provider: provider, Name: name}
}

func normalizeScenarioSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "manual"
	}
	return source
}

func (s *Service) refreshScenarioCaseCounts(ctx context.Context, agentID string) error {
	if err := s.repo.ResetScenarioCaseCounts(ctx, agentID); err != nil {
		return err
	}
	cases, err := s.ListCases(ctx, agentID, "")
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, testCase := range cases {
		if testCase.ScenarioID != nil && *testCase.ScenarioID != "" {
			counts[*testCase.ScenarioID]++
		}
	}
	for scenarioID, count := range counts {
		if err := s.repo.UpdateScenarioCaseCount(ctx, agentID, scenarioID, count); err != nil {
			return err
		}
	}
	return nil
}

func scenariosToRecognizedScenarios(scenarios []Scenario) []RecognizedScenario {
	items := make([]RecognizedScenario, 0, len(scenarios))
	for _, scenario := range scenarios {
		items = append(items, RecognizedScenario{
			Name:        scenario.Name,
			Description: scenario.Description,
		})
	}
	return items
}

func (s *Service) CreateBatch(ctx context.Context, agentID string, req CreateBatchRequest) (*Batch, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	settings, err := s.resolveBatchJudgeSettings(ctx, agentID)
	if err != nil {
		return nil, err
	}
	selectedCases, err := s.selectBatchCases(ctx, agentID, req.CaseIDs)
	if err != nil {
		return nil, err
	}
	if len(selectedCases) == 0 {
		return nil, fmt.Errorf("at least one enabled case is required")
	}
	versionMode, versionUUID, versionLabel, err := normalizeWorkflowVersionScope(req.WorkflowVersionMode, req.WorkflowVersionUUID)
	if err != nil {
		return nil, err
	}
	var versionUUIDPtr *string
	if versionUUID != "" {
		versionUUIDPtr = &versionUUID
	}
	now := time.Now()
	batch := &Batch{
		ID:                         newID(),
		AgentID:                    agentID,
		Name:                       name,
		Status:                     BatchStatusQueued,
		CaseCount:                  len(selectedCases),
		JudgePromptSnapshot:        settings.JudgePromptTemplate,
		JudgeModelProviderSnapshot: settings.JudgeModelProvider,
		JudgeModelNameSnapshot:     settings.JudgeModelName,
		WorkflowVersionMode:        versionMode,
		WorkflowVersionUUID:        versionUUIDPtr,
		WorkflowVersionLabel:       versionLabel,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	}
	items := make([]BatchItem, 0, len(selectedCases))
	for _, testCase := range selectedCases {
		items = append(items, BatchItem{
			ID:      newID(),
			AgentID: agentID,
			BatchID: batch.ID,
			CaseID:  testCase.ID,
			CaseSnapshot: JSONCaseSnapshot{
				ID:             testCase.ID,
				ScenarioID:     testCase.ScenarioID,
				Content:        testCase.Content,
				ExpectedResult: testCase.ExpectedResult,
				QuestionType:   testCase.QuestionType,
				Turns:          testCase.Turns,
			},
			Status:          string(BatchItemStatusPending),
			WorkflowRunID:   "",
			Outputs:         JSONMap{},
			Error:           "",
			JudgeReason:     "",
			JudgeSuggestion: "",
			JudgeConfidence: 0,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	if err := s.repo.CreateBatchWithItems(ctx, batch, items); err != nil {
		return nil, err
	}
	return batch, nil
}

func (s *Service) ListBatches(ctx context.Context, agentID string) ([]Batch, error) {
	return s.repo.ListBatches(ctx, agentID)
}

func (s *Service) ListBatchItems(ctx context.Context, agentID string, batchID string) ([]BatchItem, error) {
	return s.repo.ListBatchItems(ctx, agentID, batchID)
}

func (s *Service) RetestBatch(ctx context.Context, agentID string, batchID string, names ...string) (*Batch, error) {
	original, err := s.repo.GetBatch(ctx, agentID, batchID)
	if err != nil {
		return nil, err
	}
	originalItems, err := s.repo.ListBatchItems(ctx, agentID, batchID)
	if err != nil {
		return nil, err
	}
	if len(originalItems) == 0 {
		return nil, fmt.Errorf("batch has no test items")
	}
	settings, err := s.resolveBatchJudgeSettings(ctx, agentID)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(original.Name)
	if len(names) > 0 && strings.TrimSpace(names[0]) != "" {
		name = strings.TrimSpace(names[0])
	}
	if name == "" {
		return nil, fmt.Errorf("batch name is required")
	}
	now := time.Now()
	retest := &Batch{
		ID:                         newID(),
		AgentID:                    agentID,
		Name:                       name,
		Status:                     BatchStatusQueued,
		CaseCount:                  len(originalItems),
		JudgePromptSnapshot:        settings.JudgePromptTemplate,
		JudgeModelProviderSnapshot: settings.JudgeModelProvider,
		JudgeModelNameSnapshot:     settings.JudgeModelName,
		WorkflowVersionMode:        original.WorkflowVersionMode,
		WorkflowVersionUUID:        original.WorkflowVersionUUID,
		WorkflowVersionLabel:       original.WorkflowVersionLabel,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	}
	items := make([]BatchItem, 0, len(originalItems))
	for _, originalItem := range originalItems {
		items = append(items, BatchItem{
			ID:              newID(),
			AgentID:         agentID,
			BatchID:         retest.ID,
			CaseID:          originalItem.CaseID,
			CaseSnapshot:    originalItem.CaseSnapshot,
			Status:          string(BatchItemStatusPending),
			WorkflowRunID:   "",
			Outputs:         JSONMap{},
			Error:           "",
			JudgeReason:     "",
			JudgeSuggestion: "",
			JudgeConfidence: 0,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
	}
	if err := s.repo.CreateBatchWithItems(ctx, retest, items); err != nil {
		return nil, err
	}
	return retest, nil
}

func (s *Service) StartBatch(ctx context.Context, agentID string, batchID string) (*Batch, error) {
	batch, err := s.repo.GetBatch(ctx, agentID, batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status != BatchStatusQueued {
		return nil, fmt.Errorf("batch can only be started from queued status")
	}
	updated, err := s.repo.UpdateBatchStatusIfCurrent(ctx, agentID, batchID, BatchStatusQueued, BatchStatusRunning)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("batch can only be started from queued status")
	}
	return s.repo.GetBatch(ctx, agentID, batchID)
}

func (s *Service) CancelBatch(ctx context.Context, agentID string, batchID string) (*Batch, error) {
	batch, err := s.repo.GetBatch(ctx, agentID, batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status != BatchStatusQueued && batch.Status != BatchStatusRunning {
		return nil, fmt.Errorf("batch can only be canceled from queued or running status")
	}
	if err := s.repo.UpdateBatchStatus(ctx, agentID, batchID, BatchStatusCanceled); err != nil {
		return nil, err
	}
	unfinished := []string{string(BatchItemStatusPending), string(BatchItemStatusRunning)}
	if err := s.repo.UpdateBatchItemsStatus(ctx, agentID, batchID, unfinished, string(BatchItemStatusCanceled)); err != nil {
		return nil, err
	}
	return s.repo.GetBatch(ctx, agentID, batchID)
}

func (s *Service) ExecuteBatch(ctx context.Context, agentID string, batchID string) (*Batch, error) {
	return s.ExecuteBatchWithRunnerAndJudge(ctx, agentID, batchID, s.runner, s.judge)
}

func (s *Service) ExecuteBatchWithRunner(ctx context.Context, agentID string, batchID string, runner Runner) (*Batch, error) {
	return s.ExecuteBatchWithRunnerAndJudge(ctx, agentID, batchID, runner, s.judge)
}

func (s *Service) ExecuteBatchWithRunnerAndJudge(ctx context.Context, agentID string, batchID string, runner Runner, judge Judge) (*Batch, error) {
	return s.ExecuteBatchWithRunnerJudgeAndSummarizer(ctx, agentID, batchID, runner, judge, s.summarizer)
}

func (s *Service) ExecuteBatchWithRunnerJudgeAndSummarizer(ctx context.Context, agentID string, batchID string, runner Runner, judge Judge, summarizer Summarizer) (*Batch, error) {
	if _, err := s.StartBatch(ctx, agentID, batchID); err != nil {
		return nil, err
	}
	return s.ExecuteStartedBatchWithRunnerJudgeAndSummarizer(ctx, agentID, batchID, runner, judge, summarizer)
}

func (s *Service) ExecuteStartedBatchWithRunnerJudgeAndSummarizer(ctx context.Context, agentID string, batchID string, runner Runner, judge Judge, summarizer Summarizer) (*Batch, error) {
	batch, err := s.repo.GetBatch(ctx, agentID, batchID)
	if err != nil {
		return nil, err
	}
	if batch.Status == BatchStatusCanceled {
		return batch, nil
	}
	if batch.Status != BatchStatusRunning {
		return nil, fmt.Errorf("batch must be running")
	}
	items, err := s.repo.ListBatchItems(ctx, agentID, batchID)
	if err != nil {
		return nil, err
	}
	processedItems := make([]BatchItem, 0, len(items))
	passed := 0
	failed := 0
	review := 0
	for _, item := range items {
		currentBatch, err := s.repo.GetBatch(ctx, agentID, batchID)
		if err != nil {
			return nil, err
		}
		if currentBatch.Status == BatchStatusCanceled {
			return currentBatch, nil
		}
		if item.Status == string(BatchItemStatusCanceled) {
			continue
		}
		if item.Status == string(BatchItemStatusPending) {
			updated, err := s.repo.UpdateBatchItemStatusIfCurrent(ctx, agentID, item.ID, string(BatchItemStatusPending), string(BatchItemStatusRunning))
			if err != nil {
				return nil, err
			}
			if !updated {
				continue
			}
			item.Status = string(BatchItemStatusRunning)
		}
		if item.Status != string(BatchItemStatusRunning) {
			continue
		}
		snapshot := CaseSnapshot(item.CaseSnapshot)
		itemCtx, cancel := context.WithTimeout(ctx, batchItemExecutionTimeout)
		result, runErr := runBatchItem(itemCtx, runner, RunCaseRequest{
			AgentID:      agentID,
			BatchID:      batchID,
			BatchItemID:  item.ID,
			CaseSnapshot: snapshot,
		})
		timedOut := errors.Is(itemCtx.Err(), context.DeadlineExceeded)
		cancel()
		item.Outputs = JSONMap{}
		if runErr != nil {
			item.Status = string(BatchItemStatusFailed)
			if timedOut {
				item.Error = "测试问题执行超时"
			} else {
				item.Error = runErr.Error()
			}
			failed++
		} else {
			if batch.JudgeModelNameSnapshot != "" {
				if configuredJudge, ok := judge.(*LLMJudge); ok {
					configuredJudge.Provider = batch.JudgeModelProviderSnapshot
					configuredJudge.Model = batch.JudgeModelNameSnapshot
				}
			}
			judgeResult := runJudge(ctx, judge, JudgeRequest{
				AgentID:        agentID,
				BatchID:        batchID,
				BatchItemID:    item.ID,
				PromptTemplate: batch.JudgePromptSnapshot,
				CaseSnapshot:   snapshot,
				RunResult:      *result,
			})
			item.Status = string(judgeResult.Status)
			item.WorkflowRunID = result.WorkflowRunID
			item.Outputs = JSONMap(result.Outputs)
			item.Error = ""
			item.JudgeReason = judgeResult.Reason
			item.JudgeSuggestion = judgeResult.Suggestion
			item.JudgeConfidence = judgeResult.Confidence
			switch judgeResult.Status {
			case BatchItemStatusPassed:
				passed++
			case BatchItemStatusFailed:
				failed++
			default:
				review++
			}
		}
		if err := s.repo.UpdateBatchItemResult(ctx, &item); err != nil {
			currentBatch, batchErr := s.repo.GetBatch(ctx, agentID, batchID)
			if batchErr == nil && currentBatch.Status == BatchStatusCanceled {
				return currentBatch, nil
			}
			return nil, err
		}
		processedItems = append(processedItems, item)
		if err := s.repo.TouchBatch(ctx, agentID, batchID); err != nil {
			return nil, err
		}
	}
	batch.PassedCount = passed
	batch.FailedCount = failed
	batch.ReviewCount = review
	if batch.JudgeModelNameSnapshot != "" {
		if configuredSummarizer, ok := summarizer.(*LLMSummarizer); ok {
			configuredSummarizer.Provider = batch.JudgeModelProviderSnapshot
			configuredSummarizer.Model = batch.JudgeModelNameSnapshot
		}
	}
	summary := runSummarizer(ctx, summarizer, SummaryRequest{
		AgentID: agentID,
		Batch:   *batch,
		Items:   processedItems,
	})
	if err := s.repo.UpdateBatchSummary(ctx, agentID, batchID, BatchStatusCompleted, passed, failed, review, summary); err != nil {
		return nil, err
	}
	return s.repo.GetBatch(ctx, agentID, batchID)
}

func (s *Service) MarkBatchExecutionFailed(ctx context.Context, agentID string, batchID string, err error) {
	if err == nil {
		return
	}
	if batch, getErr := s.repo.GetBatch(ctx, agentID, batchID); getErr == nil && batch.Status == BatchStatusCanceled {
		return
	}
	summary := fmt.Sprintf("测试执行异常：%s", err.Error())
	if updateErr := s.repo.UpdateBatchSummary(ctx, agentID, batchID, BatchStatusStopped, 0, 0, 0, summary); updateErr != nil {
		logger.Error("workflow test mark batch failed", updateErr)
	}
	unfinished := []string{string(BatchItemStatusPending), string(BatchItemStatusRunning)}
	if updateErr := s.repo.UpdateBatchItemsStatus(ctx, agentID, batchID, unfinished, string(BatchItemStatusFailed)); updateErr != nil {
		logger.Error("workflow test mark batch items failed", updateErr)
	}
}

func (s *Service) RecoverStaleRunningBatches(ctx context.Context, agentID string, staleBefore time.Time) (int64, error) {
	return s.repo.RecoverStaleRunningBatches(ctx, agentID, staleBefore, batchStaleFailureMessage, batchStaleFailureMessage, time.Now())
}

func runBatchItem(ctx context.Context, runner Runner, req RunCaseRequest) (*RunCaseResult, error) {
	if runner == nil {
		return nil, fmt.Errorf("workflow runner is not configured")
	}
	return runner.RunCase(ctx, req)
}

func (s *Service) selectBatchCases(ctx context.Context, agentID string, caseIDs []string) ([]Case, error) {
	if len(caseIDs) == 0 {
		return s.repo.ListCases(ctx, agentID, CaseStatusEnabled)
	}
	uniqueIDs := make([]string, 0, len(caseIDs))
	seen := map[string]struct{}{}
	for _, id := range caseIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return nil, fmt.Errorf("case_ids must not be empty")
	}
	cases, err := s.repo.ListCasesByIDs(ctx, agentID, uniqueIDs)
	if err != nil {
		return nil, err
	}
	if len(cases) != len(uniqueIDs) {
		return nil, fmt.Errorf("selected cases include missing or unauthorized cases")
	}
	enabled := make([]Case, 0, len(cases))
	for _, testCase := range cases {
		if testCase.Status == CaseStatusEnabled {
			enabled = append(enabled, testCase)
		}
	}
	if len(enabled) != len(uniqueIDs) {
		return nil, fmt.Errorf("selected cases must all be enabled")
	}
	return enabled, nil
}

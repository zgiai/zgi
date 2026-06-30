package workflowtest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	staleRunningBatchThreshold = 60 * time.Minute
	staleAsyncTaskThreshold    = 30 * time.Minute
)

type Handler struct {
	service                *Service
	workflowService        interfaces.WorkflowService
	agentWorkspaceResolver workflowTestAgentWorkspaceResolver
	organizationService    workflowTestWorkspacePermissionChecker
	llmClient              llmclient.LLMClient
	taskManager            *queue.TaskManager
	taskBackend            string
}

type workflowTestAgentWorkspaceResolver interface {
	GetAgentWorkspaceID(ctx context.Context, agentID string) (string, error)
}

type workflowTestWorkspacePermissionChecker interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error)
}

func NewHandler(service *Service, workflowService interfaces.WorkflowService, args ...interface{}) *Handler {
	var wf interfaces.WorkflowService
	wf = workflowService
	var client llmclient.LLMClient
	var organizationService workflowTestWorkspacePermissionChecker
	var taskManager *queue.TaskManager
	taskBackend := "local"
	for _, arg := range args {
		switch value := arg.(type) {
		case llmclient.LLMClient:
			client = value
		case interfaces.OrganizationService:
			organizationService = value
		case *queue.TaskManager:
			taskManager = value
		case string:
			if strings.TrimSpace(value) != "" {
				taskBackend = strings.TrimSpace(value)
			}
		}
	}
	if service != nil {
		service.SetWorkflowContextProvider(WorkflowServiceContextProvider{WorkflowService: wf})
	}
	return &Handler{
		service:                service,
		workflowService:        wf,
		agentWorkspaceResolver: wf,
		organizationService:    organizationService,
		llmClient:              client,
		taskManager:            taskManager,
		taskBackend:            normalizeTaskBackend(taskBackend),
	}
}

func (h *Handler) GetSettings(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowView) {
		return
	}
	settings, err := h.service.GetSettings(c.Request.Context(), agentID)
	if err != nil {
		logger.Error("workflow test get settings failed", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, settings)
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	var req UpdateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	settings, err := h.service.UpdateSettings(c.Request.Context(), agentID, req)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, settings)
}

func (h *Handler) ResetSettings(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	settings, err := h.service.ResetSettings(c.Request.Context(), agentID)
	if err != nil {
		logger.Error("workflow test reset settings failed", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, settings)
}

func (h *Handler) ListScenarios(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowView) {
		return
	}
	items, err := h.service.ListScenarios(c.Request.Context(), agentID)
	if err != nil {
		logger.Error("workflow test list scenarios failed", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *Handler) CreateScenario(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	var req CreateScenarioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	item, err := h.service.CreateScenario(c.Request.Context(), agentID, req)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, item)
}

func (h *Handler) SaveScenarios(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	var req SaveScenariosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	items, err := h.service.SaveScenarios(c.Request.Context(), agentID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *Handler) RecognizeScenarios(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return
	}
	var req RecognizeScenariosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	recognizer := &LLMScenarioRecognizer{
		Client:      h.llmClient,
		WorkspaceID: workspaceID,
		AccountID:   accountID,
		AgentID:     agentID,
	}
	result, err := h.service.RecognizeScenarios(c.Request.Context(), agentID, req, recognizer)
	if err != nil {
		logger.Error(fmt.Sprintf("workflow test recognize scenarios failed: agent_id=%s", agentID), err)
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateScenarioRecognitionTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return
	}
	var req RecognizeScenariosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if _, err := h.service.RecoverStaleRunningScenarioRecognitionTasksForAgent(c.Request.Context(), agentID, time.Now().Add(-staleAsyncTaskThreshold)); err != nil {
		logger.Warn("workflow test recover stale scenario recognition tasks failed", map[string]interface{}{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}
	task, err := h.service.CreateScenarioRecognitionTask(c.Request.Context(), agentID, workspaceID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if h.taskBackend != WorkflowTestTaskBackendAsynq {
		response.Success(c, ScenarioRecognitionTaskResponse{Task: task})
		return
	}
	if h.taskManager == nil {
		_ = h.service.finishScenarioRecognitionTask(c.Request.Context(), task.ID, GenerationTaskStatusFailed, scenarioRecognitionTaskFailureReason(fmt.Errorf("task manager is not configured")), 0, 0)
		response.Fail(c, response.ErrSystemError)
		return
	}
	asynqTask, err := NewScenarioRecognitionTaskAsynqTask(task.ID, h.taskManager)
	if err != nil {
		_ = h.service.finishScenarioRecognitionTask(c.Request.Context(), task.ID, GenerationTaskStatusFailed, scenarioRecognitionTaskFailureReason(err), 0, 0)
		response.Fail(c, response.ErrSystemError)
		return
	}
	if _, err := h.taskManager.EnqueueTask(asynqTask); err != nil {
		_ = h.service.finishScenarioRecognitionTask(c.Request.Context(), task.ID, GenerationTaskStatusFailed, scenarioRecognitionTaskFailureReason(err), 0, 0)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, ScenarioRecognitionTaskResponse{Task: task})
}

func (h *Handler) GetActiveScenarioRecognitionTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleScenarioRecognitionTasks(c, agentID)
	task, err := h.service.GetActiveScenarioRecognitionTask(c.Request.Context(), agentID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, ScenarioRecognitionTaskResponse{Task: task})
}

func (h *Handler) GetLatestScenarioRecognitionTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleScenarioRecognitionTasks(c, agentID)
	task, err := h.service.GetLatestScenarioRecognitionTask(c.Request.Context(), agentID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, ScenarioRecognitionTaskResponse{Task: task})
}

func (h *Handler) GetScenarioRecognitionTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleScenarioRecognitionTasks(c, agentID)
	task, err := h.service.GetScenarioRecognitionTask(c.Request.Context(), agentID, c.Param("task_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, ScenarioRecognitionTaskResponse{Task: task})
}

func (h *Handler) CancelScenarioRecognitionTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	task, err := h.service.CancelScenarioRecognitionTask(c.Request.Context(), agentID, c.Param("task_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, ScenarioRecognitionTaskResponse{Task: task})
}

func (h *Handler) ListCases(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowView) {
		return
	}
	items, err := h.service.ListCases(c.Request.Context(), agentID, c.Query("status"))
	if err != nil {
		logger.Error("workflow test list cases failed", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *Handler) CreateCase(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	var req CreateCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	item, err := h.service.CreateCase(c.Request.Context(), agentID, req)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, item)
}

func (h *Handler) UpdateCase(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	caseID, ok := bindCaseID(c)
	if !ok {
		return
	}
	var req UpdateCaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	item, err := h.service.UpdateCase(c.Request.Context(), agentID, caseID, req)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, item)
}

func (h *Handler) DeleteCase(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	caseID, ok := bindCaseID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteCases(c.Request.Context(), agentID, []string{caseID}); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, gin.H{"deleted": 1})
}

func (h *Handler) DeleteCases(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	var req DeleteCasesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if err := h.service.DeleteCases(c.Request.Context(), agentID, req.CaseIDs); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, gin.H{"deleted": len(req.CaseIDs)})
}

func (h *Handler) GenerateCases(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return
	}
	var req GenerateCasesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	generator := &LLMCaseGenerator{
		Client:      h.llmClient,
		WorkspaceID: workspaceID,
		AccountID:   accountID,
		AgentID:     agentID,
	}
	result, err := h.service.GenerateCases(c.Request.Context(), agentID, req, generator)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, result)
}

func (h *Handler) CreateGenerationTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return
	}
	var req CreateGenerationTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, err := h.service.RecoverStaleRunningGenerationTasksForAgent(c.Request.Context(), agentID, time.Now().Add(-staleAsyncTaskThreshold)); err != nil {
		logger.Warn("workflow test recover stale generation tasks failed", map[string]interface{}{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}
	task, err := h.service.CreateGenerationTask(c.Request.Context(), agentID, workspaceID, accountID, req)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if h.taskBackend != WorkflowTestTaskBackendAsynq {
		response.Success(c, GenerationTaskResponse{Task: task})
		return
	}
	if h.taskManager == nil {
		_ = h.service.finishGenerationTask(c.Request.Context(), task.ID, GenerationTaskStatusFailed, generationTaskFailureReason(fmt.Errorf("task manager is not configured")))
		response.Fail(c, response.ErrSystemError)
		return
	}
	asynqTask, err := NewGenerationTaskAsynqTask(task.ID, h.taskManager)
	if err != nil {
		_ = h.service.finishGenerationTask(c.Request.Context(), task.ID, GenerationTaskStatusFailed, generationTaskFailureReason(err))
		response.Fail(c, response.ErrSystemError)
		return
	}
	if _, err := h.taskManager.EnqueueTask(asynqTask); err != nil {
		_ = h.service.finishGenerationTask(c.Request.Context(), task.ID, GenerationTaskStatusFailed, generationTaskFailureReason(err))
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, GenerationTaskResponse{Task: task})
}

func (h *Handler) GetActiveGenerationTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleGenerationTasks(c, agentID)
	task, err := h.service.GetActiveGenerationTask(c.Request.Context(), agentID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, GenerationTaskResponse{Task: task})
}

func (h *Handler) GetLatestGenerationTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleGenerationTasks(c, agentID)
	task, err := h.service.GetLatestGenerationTask(c.Request.Context(), agentID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, GenerationTaskResponse{Task: task})
}

func (h *Handler) GetGenerationTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleGenerationTasks(c, agentID)
	task, err := h.service.GetGenerationTask(c.Request.Context(), agentID, c.Param("task_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, GenerationTaskResponse{Task: task})
}

func (h *Handler) CancelGenerationTask(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	task, err := h.service.CancelGenerationTask(c.Request.Context(), agentID, c.Param("task_id"))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, GenerationTaskResponse{Task: task})
}

func (h *Handler) recoverStaleGenerationTasks(c *gin.Context, agentID string) {
	if _, err := h.service.RecoverStaleRunningGenerationTasksForAgent(c.Request.Context(), agentID, time.Now().Add(-staleAsyncTaskThreshold)); err != nil {
		logger.Warn("workflow test recover stale generation tasks failed", map[string]interface{}{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}
}

func (h *Handler) recoverStaleScenarioRecognitionTasks(c *gin.Context, agentID string) {
	if _, err := h.service.RecoverStaleRunningScenarioRecognitionTasksForAgent(c.Request.Context(), agentID, time.Now().Add(-staleAsyncTaskThreshold)); err != nil {
		logger.Warn("workflow test recover stale scenario recognition tasks failed", map[string]interface{}{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}
}

func (h *Handler) ListBatches(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	h.recoverStaleRunningBatches(c, agentID)
	items, err := h.service.ListBatches(c.Request.Context(), agentID)
	if err != nil {
		logger.Error("workflow test list batches failed", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *Handler) CreateBatch(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowUpdate) {
		return
	}
	var req CreateBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	item, err := h.service.CreateBatch(c.Request.Context(), agentID, req)
	if err != nil {
		if errors.Is(err, ErrJudgeModelNotConfigured) {
			response.FailWithMessage(c, response.ErrInvalidParam, "judge model is not configured")
			return
		}
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, item)
}

func (h *Handler) ListBatchItems(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowLogsView) {
		return
	}
	batchID := c.Param("batch_id")
	if _, err := uuid.Parse(batchID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	items, err := h.service.ListBatchItems(c.Request.Context(), agentID, batchID)
	if err != nil {
		logger.Error("workflow test list batch items failed", err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *Handler) RetestBatch(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	batchID, ok := bindBatchID(c)
	if !ok {
		return
	}
	var req RetestBatchRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}
	batch, err := h.service.RetestBatch(c.Request.Context(), agentID, batchID, req.Name)
	if err != nil {
		if errors.Is(err, ErrJudgeModelNotConfigured) {
			response.FailWithMessage(c, response.ErrInvalidParam, "judge model is not configured")
			return
		}
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, batch)
}

func (h *Handler) StartBatch(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	batchID, ok := bindBatchID(c)
	if !ok {
		return
	}
	batch, err := h.service.StartBatch(c.Request.Context(), agentID, batchID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, batch)
}

func (h *Handler) ExecuteBatch(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	batchID, ok := bindBatchID(c)
	if !ok {
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return
	}
	runner := &WorkflowServiceRunner{
		WorkflowService: h.workflowService,
		WorkspaceID:     workspaceID,
		AccountID:       accountID,
	}
	judge := &LLMJudge{
		Client:      h.llmClient,
		WorkspaceID: workspaceID,
		AccountID:   accountID,
	}
	summarizer := &LLMSummarizer{
		Client:      h.llmClient,
		WorkspaceID: workspaceID,
		AccountID:   accountID,
	}
	h.recoverStaleRunningBatches(c, agentID)
	batch, err := h.service.StartBatch(c.Request.Context(), agentID, batchID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("panic: %v", recovered)
				logger.Error("workflow test async execute panic", err)
				h.service.MarkBatchExecutionFailed(context.Background(), agentID, batchID, err)
			}
		}()
		if _, err := h.service.ExecuteStartedBatchWithRunnerJudgeAndSummarizer(context.Background(), agentID, batchID, runner, judge, summarizer); err != nil {
			logger.Error("workflow test async execute failed", err)
			h.service.MarkBatchExecutionFailed(context.Background(), agentID, batchID, err)
		}
	}()
	response.Success(c, batch)
}

func (h *Handler) recoverStaleRunningBatches(c *gin.Context, agentID string) {
	if _, err := h.service.RecoverStaleRunningBatches(c.Request.Context(), agentID, time.Now().Add(-staleRunningBatchThreshold)); err != nil {
		logger.Warn("workflow test recover stale batches failed", map[string]interface{}{
			"agent_id": agentID,
			"error":    err.Error(),
		})
	}
}

func (h *Handler) CancelBatch(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionWorkflowRunDraft) {
		return
	}
	batchID, ok := bindBatchID(c)
	if !ok {
		return
	}
	batch, err := h.service.CancelBatch(c.Request.Context(), agentID, batchID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, batch)
}

func (h *Handler) resolveAgentWorkspaceID(c *gin.Context, agentID string) (string, bool) {
	workspaceResolver := h.getAgentWorkspaceResolver()
	if workspaceResolver == nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}
	workspaceID, err := workspaceResolver.GetAgentWorkspaceID(c.Request.Context(), agentID)
	if err != nil {
		logger.Error(fmt.Sprintf("workflow test get agent workspace failed: agent_id=%s", agentID), err)
		if err.Error() == "agent not found" {
			response.Fail(c, response.ErrAppNotFound)
			return "", false
		}
		response.Fail(c, response.ErrSystemError)
		return "", false
	}
	if workspaceID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return "", false
	}
	return workspaceID, true
}

func (h *Handler) ensureAgentPermission(c *gin.Context, agentID string, permissionCode workspace_model.WorkspacePermissionCode) bool {
	if h.organizationService == nil {
		return true
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return false
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return false
	}
	workspaceResolver := h.getAgentWorkspaceResolver()
	if workspaceResolver == nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	workspaceID, err := workspaceResolver.GetAgentWorkspaceID(c.Request.Context(), agentID)
	if err != nil {
		logger.Error(fmt.Sprintf("workflow test get agent workspace failed: agent_id=%s", agentID), err)
		if err.Error() == "agent not found" {
			response.Fail(c, response.ErrAppNotFound)
			return false
		}
		response.Fail(c, response.ErrSystemError)
		return false
	}
	if workspaceID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return false
	}
	hasPermission, err := h.organizationService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		permissionCode,
	)
	if err != nil {
		logger.Error("workflow test permission check failed", err)
		response.Fail(c, response.ErrSystemError)
		return false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}
	return true
}

func (h *Handler) getAgentWorkspaceResolver() workflowTestAgentWorkspaceResolver {
	if h.agentWorkspaceResolver != nil {
		return h.agentWorkspaceResolver
	}
	return h.workflowService
}

func bindAgentID(c *gin.Context) (string, bool) {
	agentID := c.Param("agent_id")
	if _, err := uuid.Parse(agentID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return "", false
	}
	return agentID, true
}

func bindCaseID(c *gin.Context) (string, bool) {
	caseID := c.Param("case_id")
	if _, err := uuid.Parse(caseID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return "", false
	}
	return caseID, true
}

func bindBatchID(c *gin.Context) (string, bool) {
	batchID := c.Param("batch_id")
	if _, err := uuid.Parse(batchID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return "", false
	}
	return batchID, true
}

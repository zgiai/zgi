package workflowtest

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

type Handler struct {
	service             *Service
	workflowService     interfaces.WorkflowService
	organizationService interfaces.OrganizationService
	llmClient           llmclient.LLMClient
}

func NewHandler(service *Service, workflowService interfaces.WorkflowService, args ...interface{}) *Handler {
	var wf interfaces.WorkflowService
	wf = workflowService
	var client llmclient.LLMClient
	var organizationService interfaces.OrganizationService
	for _, arg := range args {
		switch value := arg.(type) {
		case llmclient.LLMClient:
			client = value
		case interfaces.OrganizationService:
			organizationService = value
		}
	}
	return &Handler{service: service, workflowService: wf, organizationService: organizationService, llmClient: client}
}

func (h *Handler) GetSettings(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentView) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentView) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
		return
	}
	var req SaveScenariosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	items, err := h.service.SaveScenarios(c.Request.Context(), agentID, req)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *Handler) RecognizeScenarios(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
		response.Fail(c, response.ErrInvalidParam)
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
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, result)
}

func (h *Handler) ListCases(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentView) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	agentID, caseID, ok := bindAgentAndCaseID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	agentID, caseID, ok := bindAgentAndCaseID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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

func (h *Handler) ListBatches(c *gin.Context) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentView) {
		return
	}
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
		return
	}
	var req CreateBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	item, err := h.service.CreateBatch(c.Request.Context(), agentID, req)
	if err != nil {
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
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentView) {
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
	agentID, batchID, ok := bindAgentAndBatchID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	response.Success(c, batch)
}

func (h *Handler) StartBatch(c *gin.Context) {
	agentID, batchID, ok := bindAgentAndBatchID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	agentID, batchID, ok := bindAgentAndBatchID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	batch, err := h.service.StartBatch(c.Request.Context(), agentID, batchID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	go func() {
		if _, err := h.service.ExecuteStartedBatchWithRunnerJudgeAndSummarizer(context.Background(), agentID, batchID, runner, judge, summarizer); err != nil {
			logger.Error("workflow test async execute failed", err)
			h.service.MarkBatchExecutionFailed(context.Background(), agentID, batchID, err)
		}
	}()
	response.Success(c, batch)
}

func (h *Handler) CancelBatch(c *gin.Context) {
	agentID, batchID, ok := bindAgentAndBatchID(c)
	if !ok {
		return
	}
	if !h.ensureAgentPermission(c, agentID, workspace_model.WorkspacePermissionAgentManage) {
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
	if h.workflowService == nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}
	workspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), agentID)
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
	if h.workflowService == nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	workspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), agentID)
	if err != nil {
		logger.Error(fmt.Sprintf("workflow test get agent workspace failed: agent_id=%s", agentID), err)
		if err.Error() == "agent not found" {
			response.Fail(c, response.ErrAppNotFound)
			return false
		}
		response.Fail(c, response.ErrSystemError)
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

func bindAgentID(c *gin.Context) (string, bool) {
	agentID := c.Param("agent_id")
	if _, err := uuid.Parse(agentID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return "", false
	}
	return agentID, true
}

func bindAgentAndCaseID(c *gin.Context) (string, string, bool) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return "", "", false
	}
	caseID := c.Param("case_id")
	if _, err := uuid.Parse(caseID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return "", "", false
	}
	return agentID, caseID, true
}

func bindAgentAndBatchID(c *gin.Context) (string, string, bool) {
	agentID, ok := bindAgentID(c)
	if !ok {
		return "", "", false
	}
	batchID := c.Param("batch_id")
	if _, err := uuid.Parse(batchID); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return "", "", false
	}
	return agentID, batchID, true
}

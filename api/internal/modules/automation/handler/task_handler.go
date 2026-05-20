package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	automationrepo "github.com/zgiai/zgi/api/internal/modules/automation/repository"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	automationdraftgen "github.com/zgiai/zgi/api/internal/modules/automation/service/draftgen"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
)

// TaskHandler exposes MVP automation task APIs for console callers.
type TaskHandler struct {
	service              automationdefinition.Service
	actionRepo           automationrepo.ActionRepository
	runRepo              automationrepo.RunRepository
	accountService       interfaces.AccountService
	organizationService  interfaces.OrganizationService
	tenantService        interfaces.WorkspaceManagementService
	llmClient            llmclient.LLMClient
	defaultModelResolver llmdefaultservice.DefaultModelResolver
}

// NewTaskHandler creates a task handler.
func NewTaskHandler(
	service automationdefinition.Service,
	actionRepo automationrepo.ActionRepository,
	runRepo automationrepo.RunRepository,
	accountService interfaces.AccountService,
	organizationService interfaces.OrganizationService,
	tenantService interfaces.WorkspaceManagementService,
	llmClient llmclient.LLMClient,
	defaultModelResolver llmdefaultservice.DefaultModelResolver,
) *TaskHandler {
	return &TaskHandler{
		service:              service,
		actionRepo:           actionRepo,
		runRepo:              runRepo,
		accountService:       accountService,
		organizationService:  organizationService,
		tenantService:        tenantService,
		llmClient:            llmClient,
		defaultModelResolver: defaultModelResolver,
	}
}

type createTaskRequest struct {
	WorkspaceID    string                                 `json:"workspace_id,omitempty"`
	Name           string                                 `json:"name" binding:"required"`
	Description    *string                                `json:"description,omitempty"`
	ScheduleType   automationmodel.AutomationScheduleType `json:"schedule_type" binding:"required"`
	Timezone       string                                 `json:"timezone"`
	ScheduleConfig map[string]interface{}                 `json:"schedule_config" binding:"required"`
	Actions        []createTaskActionRequest              `json:"actions" binding:"required"`
}

type createTaskActionRequest struct {
	ActionType  automationmodel.AutomationActionType `json:"action_type" binding:"required"`
	ActionOrder int                                  `json:"action_order,omitempty"`
	Enabled     *bool                                `json:"enabled,omitempty"`
	Config      map[string]interface{}               `json:"config" binding:"required"`
}

type updateTaskRequest struct {
	WorkspaceID    string                                 `json:"workspace_id,omitempty"`
	Name           string                                 `json:"name" binding:"required"`
	Description    *string                                `json:"description,omitempty"`
	ScheduleType   automationmodel.AutomationScheduleType `json:"schedule_type" binding:"required"`
	Timezone       string                                 `json:"timezone"`
	ScheduleConfig map[string]interface{}                 `json:"schedule_config" binding:"required"`
	Actions        []updateTaskActionRequest              `json:"actions" binding:"required"`
}

type updateTaskActionRequest struct {
	ActionType  automationmodel.AutomationActionType `json:"action_type" binding:"required"`
	ActionOrder int                                  `json:"action_order,omitempty"`
	Enabled     *bool                                `json:"enabled,omitempty"`
	Config      map[string]interface{}               `json:"config" binding:"required"`
}

type listTasksQuery struct {
	WorkspaceID string `form:"workspace_id"`
	Statuses    string `form:"statuses"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

type listRunsQuery struct {
	WorkspaceID string `form:"workspace_id"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

type taskMutationQuery struct {
	WorkspaceID string `form:"workspace_id"`
}

type generateTaskDraftRequest struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
	Prompt      string `json:"prompt" binding:"required"`
	Locale      string `json:"locale,omitempty"`
	Timezone    string `json:"timezone,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
}

type taskDetailResponse struct {
	Task    *automationmodel.AutomationTask         `json:"task"`
	Actions []*automationmodel.AutomationTaskAction `json:"actions"`
}

type taskRunDetailResponse struct {
	Run        *automationmodel.AutomationTaskRun     `json:"run"`
	ActionRuns []*automationmodel.AutomationActionRun `json:"action_runs"`
}

type taskRunsResponse struct {
	TaskID  string                  `json:"task_id"`
	Total   int64                   `json:"total"`
	Page    int                     `json:"page"`
	Limit   int                     `json:"limit"`
	HasMore bool                    `json:"has_more"`
	Runs    []taskRunDetailResponse `json:"runs"`
}

type runTaskNowResponse struct {
	TaskID string                             `json:"task_id"`
	Run    *automationmodel.AutomationTaskRun `json:"run"`
}

// GenerateTaskDraft handles POST /automations/tasks/draft/generate.
func (h *TaskHandler) GenerateTaskDraft(c *gin.Context) {
	var req generateTaskDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	if strings.TrimSpace(req.Prompt) == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "prompt is required")
		return
	}

	scope, accountID, ok := h.resolveScope(c, req.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceManage)
	if !ok {
		return
	}

	provider, model, err := h.resolveTaskDraftModel(c.Request.Context(), scope.OrganizationID, req.Provider, req.Model)
	if err != nil {
		if errors.Is(err, automationdraftgen.ErrModelNotConfigured) {
			response.FailWithMessage(c, response.ErrConfigError, "Please configure a default LLM model before generating a scheduled task draft.")
			return
		}
		response.FailWithMessage(c, response.ErrServiceUnavailable, "Failed to resolve the default LLM model.")
		return
	}

	generator := automationdraftgen.NewGenerator(h.llmClient)
	result, err := generator.Generate(c.Request.Context(), automationdraftgen.GenerateRequest{
		Prompt:         req.Prompt,
		Locale:         req.Locale,
		Timezone:       req.Timezone,
		Provider:       provider,
		Model:          model,
		WorkspaceID:    scope.WorkspaceID,
		OrganizationID: scope.OrganizationID,
		AccountID:      accountID,
	})
	if err != nil {
		logger.Error("failed to generate automation task draft",
			zap.String("organization_id", scope.OrganizationID),
			zap.String("workspace_id", scope.WorkspaceID),
			zap.String("account_id", accountID),
			zap.String("provider", provider),
			zap.String("model", model),
			zap.Error(err),
		)
		if errors.Is(err, automationdraftgen.ErrModelNotConfigured) {
			response.FailWithMessage(c, response.ErrConfigError, "Please configure a default LLM model before generating a scheduled task draft.")
			return
		}
		if errors.Is(err, automationdraftgen.ErrModelOutputInvalid) {
			response.FailWithMessage(c, response.ErrServiceUnavailable, "The model did not return a usable scheduled task draft. Please try again.")
			return
		}
		response.FailWithMessage(c, response.ErrServiceUnavailable, "Failed to generate a scheduled task draft. Please try again.")
		return
	}

	response.Success(c, result)
}

// CreateTask handles POST /automations/tasks.
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	scope, accountID, ok := h.resolveScope(c, req.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceManage)
	if !ok {
		return
	}

	actions := make([]automationdto.CreateTaskActionRequest, 0, len(req.Actions))
	for _, actionReq := range req.Actions {
		actions = append(actions, automationdto.CreateTaskActionRequest{
			ActionType:  actionReq.ActionType,
			ActionOrder: actionReq.ActionOrder,
			Enabled:     actionReq.Enabled,
			Config:      actionReq.Config,
		})
	}

	result, err := h.service.CreateTask(c.Request.Context(), automationdto.CreateTaskRequest{
		TaskScope:      scope,
		Name:           req.Name,
		Description:    req.Description,
		ScheduleType:   req.ScheduleType,
		Timezone:       req.Timezone,
		ScheduleConfig: req.ScheduleConfig,
		SourceType:     automationmodel.AutomationSourceTypeManual,
		SourceSnapshot: map[string]interface{}{"created_via": "console_api"},
		CreatedBy:      accountID,
		UpdatedBy:      accountID,
		Actions:        actions,
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, taskDetailResponse{
		Task:    result.Task,
		Actions: result.Actions,
	})
}

// UpdateTask handles PATCH /automations/tasks/:id.
func (h *TaskHandler) UpdateTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid request body: "+err.Error())
		return
	}

	scope, accountID, ok := h.resolveScope(c, req.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceManage)
	if !ok {
		return
	}

	actions := make([]automationdto.UpdateTaskActionRequest, 0, len(req.Actions))
	for _, actionReq := range req.Actions {
		actions = append(actions, automationdto.UpdateTaskActionRequest{
			ActionType:  actionReq.ActionType,
			ActionOrder: actionReq.ActionOrder,
			Enabled:     actionReq.Enabled,
			Config:      actionReq.Config,
		})
	}

	result, err := h.service.UpdateTask(c.Request.Context(), scope, taskID, automationdto.UpdateTaskRequest{
		Name:           req.Name,
		Description:    req.Description,
		ScheduleType:   req.ScheduleType,
		Timezone:       req.Timezone,
		ScheduleConfig: req.ScheduleConfig,
		Actions:        actions,
		UpdatedBy:      accountID,
	})
	if err != nil {
		h.handleTaskMutationError(c, err)
		return
	}

	response.Success(c, taskDetailResponse{
		Task:    result.Task,
		Actions: result.Actions,
	})
}

// GetTask handles GET /automations/tasks/:id.
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	scope, _, ok := h.resolveScope(c, strings.TrimSpace(c.Query("workspace_id")), workspacemodel.WorkspacePermissionWorkspaceView)
	if !ok {
		return
	}

	task, err := h.service.GetTask(c.Request.Context(), scope, taskID)
	if err != nil {
		h.handleTaskLookupError(c, err)
		return
	}

	actions, err := h.actionRepo.ListByTaskID(c.Request.Context(), nil, task.ID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, taskDetailResponse{
		Task:    task,
		Actions: actions,
	})
}

// ListTasks handles GET /automations/tasks.
func (h *TaskHandler) ListTasks(c *gin.Context) {
	var query listTasksQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	scope, _, ok := h.resolveScope(c, query.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceView)
	if !ok {
		return
	}

	filter := automationdto.TaskFilter{
		TaskScope: scope,
		Statuses:  parseStatuses(query.Statuses),
		Page:      normalizePage(query.Page),
		Limit:     normalizeLimit(query.Limit),
	}

	total, err := h.service.CountTasks(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	tasks, err := h.service.ListTasks(c.Request.Context(), filter)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	items := make([]taskDetailResponse, 0, len(tasks))
	for _, task := range tasks {
		actions, actionErr := h.actionRepo.ListByTaskID(c.Request.Context(), nil, task.ID)
		if actionErr != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		items = append(items, taskDetailResponse{
			Task:    task,
			Actions: actions,
		})
	}

	response.Success(c, gin.H{
		"items":    items,
		"total":    total,
		"page":     filter.Page,
		"limit":    filter.Limit,
		"has_more": int64(filter.Page*filter.Limit) < total,
	})
}

// ListTaskRuns handles GET /automations/tasks/:id/runs.
func (h *TaskHandler) ListTaskRuns(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	var query listRunsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	scope, _, ok := h.resolveScope(c, query.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceView)
	if !ok {
		return
	}

	if _, err := h.service.GetTask(c.Request.Context(), scope, taskID); err != nil {
		h.handleTaskLookupError(c, err)
		return
	}

	page := normalizePage(query.Page)
	limit := normalizeLimit(query.Limit)

	total, err := h.runRepo.CountTaskRuns(c.Request.Context(), nil, scope, taskID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	runs, err := h.runRepo.ListTaskRuns(c.Request.Context(), nil, scope, taskID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	items := make([]taskRunDetailResponse, 0, len(runs))
	for _, run := range runs {
		actionRuns, actionErr := h.runRepo.ListActionRunsByTaskRunID(c.Request.Context(), nil, run.ID)
		if actionErr != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		items = append(items, taskRunDetailResponse{
			Run:        run,
			ActionRuns: actionRuns,
		})
	}

	response.Success(c, taskRunsResponse{
		TaskID:  taskID,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: int64(page*limit) < total,
		Runs:    items,
	})
}

// RunTaskNow handles POST /automations/tasks/:id/run.
func (h *TaskHandler) RunTaskNow(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	var query taskMutationQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	scope, _, ok := h.resolveScope(c, query.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceManage)
	if !ok {
		return
	}

	run, err := h.service.RunTaskNow(c.Request.Context(), scope, taskID)
	if err != nil {
		h.handleTaskMutationError(c, err)
		return
	}

	response.Success(c, runTaskNowResponse{
		TaskID: taskID,
		Run:    run,
	})
}

// PauseTask handles POST /automations/tasks/:id/pause.
func (h *TaskHandler) PauseTask(c *gin.Context) {
	h.mutateTaskState(c, "pause", func(scope automationdto.TaskScope, taskID, accountID string) error {
		return h.service.PauseTask(c.Request.Context(), scope, taskID, accountID)
	})
}

// ResumeTask handles POST /automations/tasks/:id/resume.
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	h.mutateTaskState(c, "resume", func(scope automationdto.TaskScope, taskID, accountID string) error {
		return h.service.ResumeTask(c.Request.Context(), scope, taskID, accountID)
	})
}

// ArchiveTask handles POST /automations/tasks/:id/archive.
func (h *TaskHandler) ArchiveTask(c *gin.Context) {
	h.mutateTaskState(c, "archive", func(scope automationdto.TaskScope, taskID, accountID string) error {
		return h.service.ArchiveTask(c.Request.Context(), scope, taskID, accountID)
	})
}

// DeleteTask handles DELETE /automations/tasks/:id for physical removal.
func (h *TaskHandler) DeleteTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	var query taskMutationQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	scope, _, ok := h.resolveScope(c, query.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceManage)
	if !ok {
		return
	}

	if err := h.service.DeleteTask(c.Request.Context(), scope, taskID); err != nil {
		h.handleTaskMutationError(c, err)
		return
	}

	response.Success(c, gin.H{
		"task_id": taskID,
		"status":  "deleted",
	})
}

// RegisterRoutes registers automation task routes.
func (h *TaskHandler) RegisterRoutes(router *gin.RouterGroup) {
	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))

	authWithTenant.POST("/automations/tasks", h.CreateTask)
	authWithTenant.GET("/automations/tasks", h.ListTasks)
	authWithTenant.POST("/automations/tasks/draft/generate", h.GenerateTaskDraft)
	authWithTenant.PATCH("/automations/tasks/:id", h.UpdateTask)
	authWithTenant.GET("/automations/tasks/:id", h.GetTask)
	authWithTenant.GET("/automations/tasks/:id/runs", h.ListTaskRuns)
	authWithTenant.POST("/automations/tasks/:id/run", h.RunTaskNow)
	authWithTenant.POST("/automations/tasks/:id/pause", h.PauseTask)
	authWithTenant.POST("/automations/tasks/:id/resume", h.ResumeTask)
	authWithTenant.POST("/automations/tasks/:id/archive", h.ArchiveTask)
	authWithTenant.DELETE("/automations/tasks/:id", h.DeleteTask)
}

func (h *TaskHandler) resolveScope(
	c *gin.Context,
	requestedWorkspaceID string,
	permission workspacemodel.WorkspacePermissionCode,
) (automationdto.TaskScope, string, bool) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return automationdto.TaskScope{}, "", false
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return automationdto.TaskScope{}, "", false
	}

	workspaceID, err := h.resolveWorkspaceID(c, requestedWorkspaceID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrWorkspaceNotFound, err.Error())
		return automationdto.TaskScope{}, "", false
	}

	if err := h.ensureWorkspaceInOrganization(c, organizationID, workspaceID); err != nil {
		response.FailWithMessage(c, response.ErrWorkspaceNotFound, err.Error())
		return automationdto.TaskScope{}, "", false
	}

	if h.organizationService != nil {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			organizationID,
			workspaceID,
			accountID,
			permission,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return automationdto.TaskScope{}, "", false
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return automationdto.TaskScope{}, "", false
		}
	}

	return automationdto.TaskScope{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
	}, accountID, true
}

func (h *TaskHandler) resolveWorkspaceID(c *gin.Context, requestedWorkspaceID, accountID string) (string, error) {
	workspaceID := strings.TrimSpace(requestedWorkspaceID)
	if workspaceID != "" {
		return workspaceID, nil
	}

	if h.tenantService == nil {
		return "", fmt.Errorf("workspace service is not configured")
	}

	currentJoin, err := h.tenantService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil {
		return "", fmt.Errorf("resolve current workspace: %w", err)
	}
	if currentJoin == nil || strings.TrimSpace(currentJoin.WorkspaceID) == "" {
		return "", fmt.Errorf("current workspace is not available")
	}

	return currentJoin.WorkspaceID, nil
}

func (h *TaskHandler) ensureWorkspaceInOrganization(c *gin.Context, organizationID, workspaceID string) error {
	if h.tenantService == nil {
		return nil
	}

	workspace, err := h.tenantService.GetWorkspaceByID(c.Request.Context(), workspaceID)
	if err != nil {
		return err
	}
	if workspace == nil {
		return gorm.ErrRecordNotFound
	}
	if workspace.OrganizationID != nil && *workspace.OrganizationID != "" && *workspace.OrganizationID != organizationID {
		return fmt.Errorf("workspace %s does not belong to organization %s", workspaceID, organizationID)
	}
	return nil
}

func (h *TaskHandler) handleTaskLookupError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Fail(c, response.ErrSystemError)
}

func (h *TaskHandler) handleTaskMutationError(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
}

func (h *TaskHandler) mutateTaskState(
	c *gin.Context,
	operation string,
	mutate func(scope automationdto.TaskScope, taskID, accountID string) error,
) {
	taskID := strings.TrimSpace(c.Param("id"))
	if taskID == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "id is required")
		return
	}

	var query taskMutationQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid query parameters: "+err.Error())
		return
	}

	scope, accountID, ok := h.resolveScope(c, query.WorkspaceID, workspacemodel.WorkspacePermissionWorkspaceManage)
	if !ok {
		return
	}

	if err := mutate(scope, taskID, accountID); err != nil {
		h.handleTaskMutationError(c, err)
		return
	}

	response.Success(c, gin.H{
		"task_id":   taskID,
		"operation": operation,
		"status":    "ok",
	})
}

func (h *TaskHandler) resolveTaskDraftModel(ctx context.Context, organizationID, explicitProvider, explicitModel string) (string, string, error) {
	provider := strings.TrimSpace(explicitProvider)
	model := strings.TrimSpace(explicitModel)
	if model != "" {
		return provider, model, nil
	}

	if strings.TrimSpace(organizationID) == "" || h.defaultModelResolver == nil {
		return "", "", automationdraftgen.ErrModelNotConfigured
	}

	resolved, err := h.defaultModelResolver.ResolveModelType(ctx, organizationID, nil, nil, sharedmodel.ModelTypeLLM)
	if err != nil {
		return "", "", err
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", "", automationdraftgen.ErrModelNotConfigured
	}

	return strings.TrimSpace(resolved.Provider), strings.TrimSpace(resolved.Model), nil
}

func parseStatuses(raw string) []automationmodel.AutomationTaskStatus {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	statuses := make([]automationmodel.AutomationTaskStatus, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" || value == "all" {
			continue
		}
		statuses = append(statuses, automationmodel.AutomationTaskStatus(value))
	}
	return statuses
}

func normalizePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

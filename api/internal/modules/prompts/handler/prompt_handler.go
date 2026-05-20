package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type PromptHandler struct {
	service        promptservice.PromptService
	accountService interfaces.AccountService
}

func NewPromptHandler(service promptservice.PromptService, accountService interfaces.AccountService) *PromptHandler {
	return &PromptHandler{
		service:        service,
		accountService: accountService,
	}
}

func (h *PromptHandler) ListPrompts(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.List(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) GetPrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	result, err := h.service.GetDetail(c.Request.Context(), organizationID, accountID, c.Param("prompt_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) GetPromptUsage(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	result, err := h.service.GetUsageSummary(c.Request.Context(), organizationID, accountID, c.Param("prompt_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) CreatePrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.CreatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.Create(c.Request.Context(), organizationID, accountID, req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) UpdatePrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.UpdatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.Update(c.Request.Context(), organizationID, accountID, c.Param("prompt_id"), req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) CreatePromptVersion(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptVersionInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.CreateVersion(c.Request.Context(), organizationID, accountID, c.Param("prompt_id"), req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) SetPromptLabels(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.SetPromptLabelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.SetLabels(c.Request.Context(), organizationID, accountID, c.Param("prompt_id"), req)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) OptimizePrompt(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptOptimizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.Optimize(
		c.Request.Context(),
		organizationID,
		accountID,
		util.GetWorkspaceID(c),
		req,
	)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) OptimizePromptStream(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptOptimizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	setupPromptOptimizerSSE(c)
	_, err := h.service.OptimizeStream(
		c.Request.Context(),
		organizationID,
		accountID,
		util.GetWorkspaceID(c),
		req,
		func(event promptservice.PromptOptimizeStreamEvent) error {
			return writePromptOptimizerSSE(c, event.Event, event.Data)
		},
	)
	if err != nil {
		_ = writePromptOptimizerSSE(c, "error", gin.H{
			"message": err.Error(),
		})
	}
}

func (h *PromptHandler) PlaygroundPromptStream(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptPlaygroundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	setupPromptOptimizerSSE(c)
	err := h.service.PlaygroundStream(
		c.Request.Context(),
		organizationID,
		accountID,
		util.GetWorkspaceID(c),
		req,
		func(event promptservice.PromptOptimizeStreamEvent) error {
			return writePromptOptimizerSSE(c, event.Event, event.Data)
		},
	)
	if err != nil {
		_ = writePromptOptimizerSSE(c, "error", gin.H{
			"message": err.Error(),
		})
	}
}

func (h *PromptHandler) ListOptimizationRuns(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptOptimizationRunListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.ListOptimizationRuns(
		c.Request.Context(),
		organizationID,
		accountID,
		c.Param("prompt_id"),
		req,
	)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) AdoptOptimizationRun(c *gin.Context) {
	organizationID := util.GetOrganizationIDCompat(c)
	accountID := c.GetString("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PromptOptimizationAdoptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	result, err := h.service.AdoptOptimizationRun(
		c.Request.Context(),
		organizationID,
		accountID,
		c.Param("prompt_id"),
		c.Param("run_id"),
		req,
	)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PromptHandler) RegisterRoutes(router *gin.RouterGroup) {
	auth := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))
	auth.GET("/prompts", h.ListPrompts)
	auth.POST("/prompts/optimize", h.OptimizePrompt)
	auth.POST("/prompts/optimize/stream", h.OptimizePromptStream)
	auth.POST("/prompts/playground/stream", h.PlaygroundPromptStream)
	auth.GET("/prompts/:prompt_id/optimization-runs", h.ListOptimizationRuns)
	auth.POST("/prompts/:prompt_id/optimization-runs/:run_id/adopt", h.AdoptOptimizationRun)
	auth.POST("/prompts", h.CreatePrompt)
	auth.GET("/prompts/:prompt_id/usage", h.GetPromptUsage)
	auth.GET("/prompts/:prompt_id", h.GetPrompt)
	auth.PATCH("/prompts/:prompt_id", h.UpdatePrompt)
	auth.POST("/prompts/:prompt_id/versions", h.CreatePromptVersion)
	auth.POST("/prompts/:prompt_id/labels", h.SetPromptLabels)
}

func setupPromptOptimizerSSE(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Status(http.StatusOK)
	c.Writer.Flush()
}

func writePromptOptimizerSSE(c *gin.Context, event string, data interface{}) error {
	payload := gin.H{
		"event": event,
		"data":  data,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", encoded); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

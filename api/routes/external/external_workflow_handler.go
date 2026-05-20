package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

type WorkflowRunRequest struct {
	Inputs       map[string]interface{} `json:"inputs" binding:"required"`
	ResponseMode string                 `json:"response_mode" binding:"required"`
	User         string                 `json:"user,omitempty"` // Changed to optional field
	Files        []interface{}          `json:"files,omitempty"`
}

type ChatWorkflowRunRequest struct {
	Query             string                 `json:"query" binding:"required"`
	Inputs            map[string]interface{} `json:"inputs,omitempty"`
	ResponseMode      string                 `json:"response_mode" binding:"required"`
	User              string                 `json:"user,omitempty"` // Changed to optional field
	ConversationID    string                 `json:"conversation_id,omitempty"`
	HistoryWindowSize *int                   `json:"history_window_size,omitempty"`
	Files             []interface{}          `json:"files,omitempty"`
}

// ExternalWorkflowHandler handles external workflow API requests with API key authentication
type ExternalWorkflowHandler struct {
	workflowService   *workflow.WorkflowService
	fileService       interfaces.FileService
	contentExtractor  workflow_file.ContentExtractor
	enterpriseService interfaces.OrganizationService
	quotaService      interfaces.QuotaService
	db                *gorm.DB
}

// NewExternalWorkflowHandler creates a new external workflow handler
func NewExternalWorkflowHandler(workflowService *workflow.WorkflowService, fileService interfaces.FileService, contentExtractor workflow_file.ContentExtractor, enterpriseService interfaces.OrganizationService, quotaService interfaces.QuotaService, db *gorm.DB) *ExternalWorkflowHandler {
	return &ExternalWorkflowHandler{
		workflowService:   workflowService,
		fileService:       fileService,
		contentExtractor:  contentExtractor,
		enterpriseService: enterpriseService,
		quotaService:      quotaService,
		db:                db,
	}
}

// getContextWithWorkflowParams creates a new context with workflow execution parameters from gin.Context
func getContextWithWorkflowParams(c *gin.Context) context.Context {
	ctx := c.Request.Context()

	// Transfer workflow execution parameters from gin.Context to request.Context
	if invokeFrom, exists := c.Get("invoke_from"); exists {
		ctx = context.WithValue(ctx, "invoke_from", invokeFrom)
	}
	if createdFrom, exists := c.Get("created_from"); exists {
		ctx = context.WithValue(ctx, "created_from", createdFrom)
	}
	if createdByRole, exists := c.Get("created_by_role"); exists {
		ctx = context.WithValue(ctx, "created_by_role", createdByRole)
	}

	return ctx
}

// RunWorkflow runs the latest published workflow for the agent associated with the API key
func (h *ExternalWorkflowHandler) RunWorkflow(c *gin.Context) {
	// Record workflow start time for elapsed time calculation
	workflowStartTime := time.Now()

	// Get API key info from context (set by middleware)
	apiKeyInfo, exists := c.Get("api_key_info")
	if !exists {
		response.Fail(c, response.ErrorCode{Code: 401001, Message: "API key info not found", UserVisible: true})
		return
	}

	keyInfo, ok := apiKeyInfo.(*middleware.APIKeyInfo)
	if !ok {
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Invalid API key info format", UserVisible: false})
		return
	}

	var req WorkflowRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{Code: 400001, Message: "Invalid request body", UserVisible: true})
		return
	}

	// Set default user if not provided
	if req.User == "" {
		req.User = "api_user_" + keyInfo.ID.String()[:8] // Use first 8 characters of API key as default user identifier
	}

	// Get agent and tenant IDs from API key
	agentID := keyInfo.AgentID.String()
	tenantID := keyInfo.TenantID.String()

	// Check if there's a published workflow for this agent
	publishedWorkflow, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), tenantID, agentID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404001, Message: "No published workflow found for this agent", UserVisible: true})
		return
	}

	if publishedWorkflow == nil {
		response.Fail(c, response.ErrorCode{Code: 404002, Message: "No published workflow available, only draft version exists", UserVisible: true})
		return
	}

	// Process file variables if present in inputs
	if err := h.validateAndProcessFileVariables(c.Request.Context(), req.Inputs, tenantID); err != nil {
		logger.WarnContext(c.Request.Context(), "external api file variable validation failed",
			"agent_id", agentID,
			"tenant_id", tenantID,
			err,
		)
		response.Fail(c, response.ErrorCode{Code: 400004, Message: fmt.Sprintf("file variable validation failed: %v", err), UserVisible: true})
		return
	}

	// Convert to internal request format
	internalReq := &dto.DraftWorkflowRunRequest{
		Inputs:       req.Inputs,
		ResponseMode: req.ResponseMode,
		StreamMode:   req.ResponseMode == "streaming",
	}

	// Check if streaming mode is requested
	if req.ResponseMode == "streaming" {
		h.runPublishedWorkflowStream(c, tenantID, agentID, internalReq, keyInfo.ID.String())
		return
	}

	// Run the published workflow (blocking mode) with context parameters
	ctx := getContextWithWorkflowParams(c)
	result, err := h.workflowService.RunPublishedWorkflow(ctx, tenantID, agentID, internalReq, keyInfo.ID.String())
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "external api workflow execution failed", "agent_id", agentID, "tenant_id", tenantID, err)
		response.Fail(c, response.ErrorCode{Code: 500002, Message: fmt.Sprintf("workflow execution failed: %v", err), UserVisible: true})
		return
	}

	// Update API key usage
	go h.updateAPIKeyUsage(keyInfo.ID)

	h.sendBlockingResponse(c, result, req.User, workflowStartTime)
}

// RunChatWorkflow runs the chat workflow for the agent associated with the API key
func (h *ExternalWorkflowHandler) RunChatWorkflow(c *gin.Context) {
	// Get API key info from context (set by middleware)
	apiKeyInfo, exists := c.Get("api_key_info")
	if !exists {
		response.Fail(c, response.ErrorCode{Code: 401001, Message: "API key info not found", UserVisible: true})
		return
	}

	keyInfo, ok := apiKeyInfo.(*middleware.APIKeyInfo)
	if !ok {
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Invalid API key info format", UserVisible: false})
		return
	}

	var req ChatWorkflowRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{Code: 400001, Message: "Invalid request body", UserVisible: true})
		return
	}

	// Set default user if not provided
	if req.User == "" {
		req.User = "api_user_" + keyInfo.ID.String()[:8] // Use first 8 characters of API key as default user identifier
	}

	// Get agent and tenant IDs from API key
	agentID := keyInfo.AgentID.String()
	tenantID := keyInfo.TenantID.String()

	// Check if there's a published workflow for this agent
	publishedWorkflow, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), tenantID, agentID)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404001, Message: "No published workflow found for this agent", UserVisible: true})
		return
	}

	if publishedWorkflow == nil {
		response.Fail(c, response.ErrorCode{Code: 404002, Message: "No published workflow available, only draft version exists", UserVisible: true})
		return
	}

	// Process file variables if present in inputs
	if req.Inputs != nil {
		if err := h.validateAndProcessFileVariables(c.Request.Context(), req.Inputs, tenantID); err != nil {
			logger.WarnContext(c.Request.Context(), "external api chat file variable validation failed",
				"agent_id", agentID,
				"tenant_id", tenantID,
				err,
			)
			response.Fail(c, response.ErrorCode{Code: 400004, Message: fmt.Sprintf("file variable validation failed: %v", err), UserVisible: true})
			return
		}
	}

	// Convert to internal advanced chat request format
	internalReq := &dto.AdvancedChatDraftWorkflowRunRequest{
		Query:             req.Query,
		Inputs:            req.Inputs,
		ResponseMode:      req.ResponseMode,
		UserID:            req.User,
		ConversationID:    req.ConversationID,
		HistoryWindowSize: req.HistoryWindowSize,
	}

	// Convert files if present
	if len(req.Files) > 0 {
		internalReq.Files = make([]dto.FileInfo, 0, len(req.Files))
		for _, file := range req.Files {
			if fileMap, ok := file.(map[string]interface{}); ok {
				fileInfo := dto.FileInfo{}
				if fileType, exists := fileMap["type"]; exists {
					if typeStr, ok := fileType.(string); ok {
						fileInfo.Type = typeStr
					}
				}
				if transferMethod, exists := fileMap["transfer_method"]; exists {
					if methodStr, ok := transferMethod.(string); ok {
						fileInfo.TransferMethod = methodStr
					}
				}
				if url, exists := fileMap["url"]; exists {
					if urlStr, ok := url.(string); ok {
						fileInfo.URL = urlStr
					}
				}
				if uploadFileID, exists := fileMap["upload_file_id"]; exists {
					if idStr, ok := uploadFileID.(string); ok {
						fileInfo.UploadFileID = idStr

						// Validate file ID exists and is accessible
						if err := h.validateFileAccess(c.Request.Context(), idStr, tenantID); err != nil {
							logger.WarnContext(c.Request.Context(), "external api file access validation failed",
								"file_id", idStr,
								"tenant_id", tenantID,
								err,
							)
							response.Fail(c, response.ErrorCode{Code: 403002, Message: fmt.Sprintf("file not accessible: %v", err), UserVisible: true})
							return
						}
					}
				}
				internalReq.Files = append(internalReq.Files, fileInfo)
			}
		}
	}

	// Check if streaming mode is requested
	logger.Info("External API - Checking response mode", "responseMode", req.ResponseMode, "isStreaming", req.ResponseMode == "streaming")
	if req.ResponseMode == "streaming" {
		logger.Info("External API - Calling runChatWorkflowStream")
		h.runChatWorkflowStream(c, tenantID, agentID, internalReq, req.User, keyInfo.ID.String())
		return
	}

	// Run the chat workflow (blocking mode) with context parameters
	ctx := getContextWithWorkflowParams(c)
	result, err := h.workflowService.RunAdvancedChatWorkflow(ctx, tenantID, agentID, internalReq, keyInfo.ID.String())
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "external api chat workflow execution failed", "agent_id", agentID, "tenant_id", tenantID, err)
		response.Fail(c, response.ErrorCode{Code: 500002, Message: fmt.Sprintf("chat workflow execution failed: %v", err), UserVisible: true})
		return
	}

	// Update API key usage
	go h.updateAPIKeyUsage(keyInfo.ID)

	h.sendChatBlockingResponse(c, result, req.User, req.ConversationID)
}

// RunSpecificWorkflow runs a specific workflow by ID
func (h *ExternalWorkflowHandler) RunSpecificWorkflow(c *gin.Context) {
	// Get workflow ID from path parameter
	workflowIDStr := c.Param("workflow_id")
	workflowID, err := uuid.Parse(workflowIDStr)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 400002, Message: "Invalid workflow ID format", UserVisible: true})
		return
	}

	// Get API key info from context (set by middleware)
	apiKeyInfo, exists := c.Get("api_key_info")
	if !exists {
		response.Fail(c, response.ErrorCode{Code: 401001, Message: "API key info not found", UserVisible: true})
		return
	}

	keyInfo, ok := apiKeyInfo.(*middleware.APIKeyInfo)
	if !ok {
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Invalid API key info format", UserVisible: false})
		return
	}

	// Parse request body
	var req dto.DraftWorkflowRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrorCode{Code: 400001, Message: "Invalid request body", UserVisible: true})
		return
	}

	// Get agent and tenant IDs from API key
	agentID := keyInfo.AgentID.String()
	tenantID := keyInfo.TenantID.String()

	// Verify the workflow belongs to the agent associated with the API key
	_, err = h.workflowService.GetWorkflowByID(c.Request.Context(), tenantID, agentID, workflowID.String())
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404003, Message: "Workflow not found or not accessible", UserVisible: true})
		return
	}

	// Check if the workflow has a published version
	publishedWorkflow, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), tenantID, agentID)
	if err != nil || publishedWorkflow == nil {
		response.Fail(c, response.ErrorCode{Code: 404004, Message: "No published version available for this workflow", UserVisible: true})
		return
	}

	// Check if streaming mode is requested
	if req.StreamMode || req.ResponseMode == "streaming" {
		h.runPublishedWorkflowStream(c, tenantID, agentID, &req, keyInfo.ID.String())
		return
	}

	// Run the specific published workflow (non-streaming) with context parameters
	ctx := getContextWithWorkflowParams(c)
	result, err := h.workflowService.RunPublishedWorkflow(ctx, tenantID, agentID, &req, keyInfo.ID.String())
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to run specific published workflow", "agent_id", agentID, "tenant_id", tenantID, "workflow_id", workflowID.String(), err)
		response.Fail(c, response.ErrorCode{Code: 500002, Message: fmt.Sprintf("Failed to run workflow: %v", err), UserVisible: true})
		return
	}

	// Update API key usage
	go h.updateAPIKeyUsage(keyInfo.ID)

	response.Success(c, result)
}

// GetWorkflowRunDetail gets the details of a workflow run
func (h *ExternalWorkflowHandler) GetWorkflowRunDetail(c *gin.Context) {
	// Get run ID from path parameter
	runIDStr := c.Param("run_id")
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 400003, Message: "Invalid run ID format", UserVisible: true})
		return
	}

	// Get API key info from context (set by middleware)
	apiKeyInfo, exists := c.Get("api_key_info")
	if !exists {
		response.Fail(c, response.ErrorCode{Code: 401001, Message: "API key info not found", UserVisible: true})
		return
	}

	keyInfo, ok := apiKeyInfo.(*middleware.APIKeyInfo)
	if !ok {
		response.Fail(c, response.ErrorCode{Code: 500001, Message: "Invalid API key info format", UserVisible: false})
		return
	}

	// Get agent and tenant IDs from API key
	agentID := keyInfo.AgentID.String()
	tenantID := keyInfo.TenantID.String()

	// Get workflow run detail
	runDetail, err := h.workflowService.GetWorkflowRunDetail(c.Request.Context(), tenantID, agentID, runID.String())
	if err != nil {
		response.Fail(c, response.ErrorCode{Code: 404005, Message: "Workflow run not found or not accessible", UserVisible: true})
		return
	}

	response.Success(c, runDetail)
}

// runPublishedWorkflowStream handles streaming execution for published workflows via external API
func (h *ExternalWorkflowHandler) runPublishedWorkflowStream(c *gin.Context, tenantID, agentID string, req *dto.DraftWorkflowRunRequest, apiKeyID string) {
	// Record workflow start time for elapsed time calculation
	workflowStartTime := time.Now()

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Get published workflow
	publishedWorkflow, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), tenantID, agentID)
	if err != nil || publishedWorkflow == nil {
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to load published workflow for stream", "agent_id", agentID, "tenant_id", tenantID, err)
		}
		h.sendSSEError(c.Request.Context(), c.Writer, "No published workflow found")
		return
	}

	workflowMap, ok := publishedWorkflow.(map[string]interface{})
	if !ok {
		logger.CriticalContext(c.Request.Context(), "invalid published workflow format for stream", "agent_id", agentID, "tenant_id", tenantID, fmt.Errorf("unexpected workflow type %T", publishedWorkflow))
		h.sendSSEError(c.Request.Context(), c.Writer, "Invalid workflow format")
		return
	}

	workflowID, _ := workflowMap["id"].(string)
	if workflowID == "" {
		workflowID = agentID
	}

	// Generate task ID
	taskID := fmt.Sprintf("task-%s-%d", agentID, time.Now().UnixNano())
	sequenceNumber := 1

	// System inputs
	systemInputs := make(map[string]interface{})
	for k, v := range req.Inputs {
		systemInputs[k] = v
	}
	systemInputs["sys.agent_id"] = agentID
	systemInputs["sys.workflow_id"] = workflowID

	// Create workflow run log with context parameters
	ctx := getContextWithWorkflowParams(c)
	var workflowRunLogID string
	workflowRunLogInterface, err := h.workflowService.CreateWorkflowRunLog(ctx, tenantID, agentID, workflowID, "external-api", req.Inputs, apiKeyID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to create external workflow run log", "agent_id", agentID, "tenant_id", tenantID, err)
		workflowRunLogID = fmt.Sprintf("run-%s-%d", agentID, time.Now().UnixNano())
	} else if workflowRunLog, ok := workflowRunLogInterface.(*workflow.WorkflowRunLog); ok {
		workflowRunLogID = workflowRunLog.ID
		systemInputs["sys.workflow_run_id"] = workflowRunLogID
	} else {
		workflowRunLogID = fmt.Sprintf("run-%s-%d", agentID, time.Now().UnixNano())
	}

	// Send workflow started event
	h.sendSSEEvent(c.Request.Context(), c.Writer, "workflow_started", map[string]interface{}{
		"id":              workflowRunLogID,
		"workflow_id":     workflowID,
		"sequence_number": sequenceNumber,
		"inputs":          systemInputs,
		"created_at":      time.Now().Unix(),
	})

	// Create channels for streaming
	resultChan := make(chan *workflow.WorkflowStreamEvent, 100)
	errorChan := make(chan error, 10)
	doneChan := make(chan map[string]interface{}, 1)

	// Start workflow execution in goroutine
	go func() {
		defer close(doneChan)
		defer close(resultChan)
		defer close(errorChan)

		// Execute workflow with streaming callback
		executor := h.workflowService.GetExecutor()

		// Get graph data from workflow
		graphData, ok := workflowMap["graph"].(map[string]interface{})
		if !ok {
			errorChan <- fmt.Errorf("invalid graph data format")
			return
		}

		// Execute workflow
		workflowExecutor, ok := executor.(*workflow.WorkflowExecutor)
		if !ok {
			errorChan <- fmt.Errorf("invalid executor type")
			return
		}

		result, err := workflowExecutor.ExecuteSimpleWorkflowWithRunID(c.Request.Context(), workflowRunLogID, graphData, req.Inputs)
		if err != nil {
			errorChan <- err
			return
		}

		doneChan <- result.NodeResults
	}()

	// Handle streaming response
	c.Stream(func(w io.Writer) bool {
		select {
		case event := <-resultChan:
			if event == nil {
				return false
			}
			h.sendSSEEvent(c.Request.Context(), c.Writer, event.EventType, event.Data)
			return true

		case err := <-errorChan:
			if err == nil {
				return false
			}
			workflowElapsedTime := h.workflowService.WorkflowRunElapsedMillisecondsForEvent(c.Request.Context(), workflowRunLogID, workflow.ElapsedMillisecondsSince(workflowStartTime))

			h.sendSSEEvent(c.Request.Context(), c.Writer, "workflow_finished", map[string]interface{}{
				"id":               workflowRunLogID,
				"workflow_id":      workflowID,
				"sequence_number":  sequenceNumber,
				"status":           "failed",
				"outputs":          map[string]interface{}{},
				"error":            map[string]interface{}{"message": err.Error()},
				"elapsed_time":     workflowElapsedTime,
				"total_tokens":     0,
				"total_steps":      0,
				"created_at":       time.Now().Unix(),
				"finished_at":      time.Now().Unix(),
				"exceptions_count": 1,
			})
			return false

		case outputs, ok := <-doneChan:
			if !ok {
				return false
			}
			fallbackElapsed := h.workflowService.WorkflowRunElapsedMillisecondsForEvent(c.Request.Context(), workflowRunLogID, workflow.ElapsedMillisecondsSince(workflowStartTime))
			workflowElapsedTime := workflow.WorkflowElapsedMillisecondsFromResult(outputs, fallbackElapsed)

			// Send workflow finished event
			h.sendSSEEvent(c.Request.Context(), c.Writer, "workflow_finished", map[string]interface{}{
				"id":               workflowRunLogID,
				"workflow_id":      workflowID,
				"sequence_number":  sequenceNumber,
				"status":           "succeeded",
				"outputs":          outputs,
				"error":            nil,
				"elapsed_time":     workflowElapsedTime,
				"total_tokens":     0,
				"total_steps":      0,
				"created_at":       time.Now().Unix(),
				"finished_at":      time.Now().Unix(),
				"exceptions_count": 0,
			})
			// Update API key usage
			go h.updateAPIKeyUsage(uuid.MustParse(apiKeyID))
			return false

		case <-c.Request.Context().Done():
			// Client disconnected
			logger.Info("Client disconnected from workflow stream", "taskID", taskID)
			return false
		}
	})
}

// sendSSEEvent sends a Server-Sent Event
func (h *ExternalWorkflowHandler) sendSSEEvent(ctx context.Context, w gin.ResponseWriter, eventType string, data interface{}) {
	event := map[string]interface{}{
		"event": eventType,
		"data":  data,
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal sse event", "event_type", eventType, err)
		return
	}

	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// sendSSEError sends an error event
func (h *ExternalWorkflowHandler) sendSSEError(ctx context.Context, w gin.ResponseWriter, errorMsg string) {
	h.sendSSEEvent(ctx, w, "error", map[string]interface{}{
		"message": errorMsg,
	})
}

// updateAPIKeyUsage updates API key usage statistics
func (h *ExternalWorkflowHandler) updateAPIKeyUsage(keyID uuid.UUID) {
	// The middleware already handles basic usage updates
	// This is a placeholder for additional usage tracking if needed
	logger.Info("API key used for workflow execution", "keyID", keyID.String())
}

func (h *ExternalWorkflowHandler) sendBlockingResponse(c *gin.Context, result interface{}, user string, startTime time.Time) {
	// Generate task_id and workflow_run_id
	taskID := uuid.New().String()
	workflowRunID := uuid.New().String()

	workflowElapsedTime := workflow.WorkflowElapsedMillisecondsFromResult(result, workflow.ElapsedMillisecondsSince(startTime))

	// Extract outputs from result
	outputs := make(map[string]interface{})
	if resultMap, ok := result.(map[string]interface{}); ok {
		if nodeResults, ok := resultMap["NodeResults"].(map[string]interface{}); ok {
			outputs = nodeResults
		} else if resultMap["outputs"] != nil {
			outputs, _ = resultMap["outputs"].(map[string]interface{})
		}
	}

	response := gin.H{
		"workflow_run_id": workflowRunID,
		"task_id":         taskID,
		"data": gin.H{
			"id":           workflowRunID,
			"workflow_id":  "",
			"status":       "succeeded",
			"outputs":      outputs,
			"error":        nil,
			"elapsed_time": workflowElapsedTime,
			"total_tokens": 0,
			"total_steps":  0,
			"created_at":   time.Now().Unix(),
			"finished_at":  time.Now().Unix(),
		},
	}

	c.JSON(200, response)
}

func (h *ExternalWorkflowHandler) sendChatBlockingResponse(c *gin.Context, result interface{}, user string, conversationID string) {
	// Extract outputs from result and answer
	var answer string
	if resultMap, ok := result.(map[string]interface{}); ok {
		if nodeResults, ok := resultMap["NodeResults"].(map[string]interface{}); ok {
			// Try to extract answer from any node that has an "answer" field
			for _, nodeOutput := range nodeResults {
				if nodeMap, ok := nodeOutput.(map[string]interface{}); ok {
					if answerValue, exists := nodeMap["answer"]; exists {
						if answerStr, ok := answerValue.(string); ok && answerStr != "" {
							answer = answerStr
							break
						}
					}
					// Also try "text" field as fallback
					if textValue, exists := nodeMap["text"]; exists {
						if textStr, ok := textValue.(string); ok && textStr != "" {
							answer = textStr
							break
						}
					}
				}
			}
		}
	}

	// Generate conversation_id if not provided
	if conversationID == "" {
		conversationID = uuid.New().String()
	}

	response := gin.H{
		"message_id":      uuid.New().String(),
		"conversation_id": conversationID,
		"mode":            "chat",
		"answer":          answer,
		"metadata": gin.H{
			"usage": gin.H{
				"prompt_tokens":     0,
				"completion_tokens": 0,
				"total_tokens":      0,
			},
			"retriever_resources": []interface{}{},
		},
		"created_at": time.Now().Unix(),
	}

	c.JSON(200, response)
}

func (h *ExternalWorkflowHandler) runChatWorkflowStream(c *gin.Context, tenantID, agentID string, req *dto.AdvancedChatDraftWorkflowRunRequest, user string, apiKeyID string) {
	logger.Info("External API - runChatWorkflowStream called", "tenantID", tenantID, "agentID", agentID, "user", user)
	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// Generate message_id and conversation_id
	messageID := uuid.New().String()
	conversationID := req.ConversationID
	if conversationID == "" {
		conversationID = uuid.New().String()
	}

	// Get published workflow
	publishedWorkflow, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), tenantID, agentID)
	if err != nil || publishedWorkflow == nil {
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to load published workflow for chat stream", "agent_id", agentID, "tenant_id", tenantID, err)
		}
		h.sendChatSSEError(c.Request.Context(), c.Writer, messageID, conversationID, "No published workflow found")
		return
	}

	// Add system variables to request inputs (like Web API does)
	if req.Inputs == nil {
		req.Inputs = make(map[string]interface{})
	}
	req.Inputs["sys.tenant_id"] = tenantID
	req.Inputs["sys.agent_id"] = agentID
	req.Inputs["sys.user_id"] = user
	req.Inputs["sys.conversation_id"] = conversationID
	req.Inputs["sys.query"] = req.Query

	h.sendChatSSEEvent(c.Request.Context(), c.Writer, "message", gin.H{
		"id":              messageID,
		"conversation_id": conversationID,
		"answer":          "",
		"created_at":      time.Now().Unix(),
	})

	// Execute chat workflow with context parameters
	ctx := getContextWithWorkflowParams(c)
	logger.Info("External API - About to call RunAdvancedChatWorkflow", "tenantID", tenantID, "agentID", agentID)
	result, err := h.workflowService.RunAdvancedChatWorkflow(ctx, tenantID, agentID, req, apiKeyID)
	logger.Info("External API - RunAdvancedChatWorkflow completed", "hasError", err != nil, "hasResult", result != nil)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "external api chat stream workflow execution failed", "agent_id", agentID, "tenant_id", tenantID, err)
		h.sendChatSSEEvent(c.Request.Context(), c.Writer, "message", gin.H{
			"id":              messageID,
			"conversation_id": conversationID,
			"answer":          "",
			"created_at":      time.Now().Unix(),
		})
		h.sendChatSSEEvent(c.Request.Context(), c.Writer, "message_end", gin.H{
			"id":              messageID,
			"conversation_id": conversationID,
			"metadata": gin.H{
				"usage": gin.H{
					"prompt_tokens":     0,
					"completion_tokens": 0,
					"total_tokens":      0,
				},
			},
		})
		return
	}

	// Extract answer from result
	var answer string

	logger.Debug("External API - Workflow result received", "resultType", fmt.Sprintf("%T", result))

	if resultMap, ok := result.(map[string]interface{}); ok {
		logger.Info("External API - Result is a map", "keys", func() []string {
			keys := make([]string, 0, len(resultMap))
			for k := range resultMap {
				keys = append(keys, k)
			}
			return keys
		}())

		if nodeResults, ok := resultMap["NodeResults"].(map[string]interface{}); ok {
			logger.Info("External API - Found NodeResults", "nodeCount", len(nodeResults))
			// Try to extract answer from any node that has an "answer" field
			for nodeID, nodeOutput := range nodeResults {
				if nodeMap, ok := nodeOutput.(map[string]interface{}); ok {
					if answerValue, exists := nodeMap["answer"]; exists {
						if answerStr, ok := answerValue.(string); ok && answerStr != "" {
							answer = answerStr
							logger.Debug("External API - Found answer in node", "nodeID", nodeID)
							break
						}
					}
					// Also try "text" field as fallback
					if textValue, exists := nodeMap["text"]; exists {
						if textStr, ok := textValue.(string); ok && textStr != "" {
							answer = textStr
							logger.Debug("External API - Found text in node", "nodeID", nodeID)
							break
						}
					}
				}
			}
		} else {
			logger.Info("External API - NodeResults not found or not a map")
		}
	} else {
		logger.Info("External API - Result is not a map", "resultType", fmt.Sprintf("%T", result))
	}

	// Send message with answer
	h.sendChatSSEEvent(c.Request.Context(), c.Writer, "message", gin.H{
		"id":              messageID,
		"conversation_id": conversationID,
		"answer":          answer,
		"created_at":      time.Now().Unix(),
	})

	// Send message_end event
	h.sendChatSSEEvent(c.Request.Context(), c.Writer, "message_end", gin.H{
		"id":              messageID,
		"conversation_id": conversationID,
		"metadata": gin.H{
			"usage": gin.H{
				"prompt_tokens":     0,
				"completion_tokens": 0,
				"total_tokens":      0,
			},
		},
	})

	// Update API key usage
	go h.updateAPIKeyUsage(uuid.MustParse(apiKeyID))
}

func (h *ExternalWorkflowHandler) sendChatSSEEvent(ctx context.Context, w gin.ResponseWriter, eventType string, data interface{}) {
	event := gin.H{
		"event": eventType,
		"data":  data,
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal chat sse event", "event_type", eventType, err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (h *ExternalWorkflowHandler) sendChatSSEError(ctx context.Context, w gin.ResponseWriter, messageID, conversationID, errorMsg string) {
	h.sendChatSSEEvent(ctx, w, "error", gin.H{
		"message_id":      messageID,
		"conversation_id": conversationID,
		"status":          500,
		"code":            "chat_workflow_execution_error",
		"message":         errorMsg,
	})
}

func (h *ExternalWorkflowHandler) StopWorkflowTask(c *gin.Context) {
	var req struct {
		User string `json:"user" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "user parameter is required"})
		return
	}

	c.JSON(http.StatusNotImplemented, gin.H{
		"message": "workflow task stop is not implemented",
	})
}

func (h *ExternalWorkflowHandler) UploadFile(c *gin.Context) {
	user := c.PostForm("user")
	if user == "" {
		c.JSON(400, gin.H{"message": "user parameter is required"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"message": "file is required"})
		return
	}

	// Get API key info for tenant and user context
	apiKeyInfo, exists := c.Get("api_key_info")
	if !exists {
		c.JSON(401, gin.H{"message": "API key info not found"})
		return
	}

	keyInfo, _ := apiKeyInfo.(*middleware.APIKeyInfo)
	tenantID := keyInfo.TenantID.String()
	// Use API Key ID as user ID for external API calls
	userID := keyInfo.ID.String()

	logger.Info("External API - File upload started",
		"filename", file.Filename,
		"size", file.Size,
		"tenant_id", tenantID,
		"user", user,
	)

	// Open uploaded file
	fileContent, err := file.Open()
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "external api failed to open uploaded file", "tenant_id", tenantID, "filename", file.Filename, err)
		c.JSON(500, gin.H{"message": "failed to open uploaded file"})
		return
	}
	defer fileContent.Close()

	// Read file content
	content, err := io.ReadAll(fileContent)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "external api failed to read file content", "tenant_id", tenantID, "filename", file.Filename, err)
		c.JSON(500, gin.H{"message": "failed to read file content"})
		return
	}

	// Get MIME type
	mimeType := file.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = http.DetectContentType(content)
	}

	// Use file service to upload
	// Note: FileService.UploadFile already triggers async content extraction via ParseFileContent
	source := interfaces.FileSourceWorkflow
	uploadedFile, err := h.fileService.UploadFile(
		c.Request.Context(),
		file.Filename,
		content,
		mimeType,
		userID,
		tenantID,
		model.CreatedByRoleEndUser,
		&source,
		nil,
		true,
		false,
	)

	if err != nil {
		// Handle specific errors
		if errors.Is(err, model.ErrFileTooLarge) {
			logger.Warn("External API - File too large",
				"filename", file.Filename,
				"size", file.Size,
			)
			c.JSON(413, gin.H{"message": "file too large"})
			return
		}
		if errors.Is(err, model.ErrUnsupportedFileType) {
			logger.Warn("External API - Unsupported file type",
				"filename", file.Filename,
				"mime_type", mimeType,
			)
			c.JSON(415, gin.H{"message": "unsupported file type"})
			return
		}
		logger.CriticalContext(c.Request.Context(), "external api failed to upload file", "tenant_id", tenantID, "filename", file.Filename, err)
		c.JSON(500, gin.H{"message": "failed to upload file"})
		return
	}

	// Get file extension
	extension := strings.ToLower(filepath.Ext(file.Filename))
	if extension != "" && strings.HasPrefix(extension, ".") {
		extension = extension[1:] // Remove dot
	}

	logger.Info("External API - File uploaded successfully",
		"file_id", uploadedFile.ID,
		"filename", file.Filename,
		"size", uploadedFile.Size,
		"tenant_id", tenantID,
		"user", user,
		"content_extraction", "triggered_async",
	)

	// Return file ID immediately - content extraction happens in background
	c.JSON(201, gin.H{
		"id":         uploadedFile.ID,
		"name":       uploadedFile.Name,
		"size":       uploadedFile.Size,
		"extension":  extension,
		"mime_type":  uploadedFile.MimeType,
		"created_by": user,
		"created_at": uploadedFile.CreatedAt.Unix(),
	})
}

func (h *ExternalWorkflowHandler) GetAppInfo(c *gin.Context) {
	// Get API key info
	apiKeyInfo, exists := c.Get("api_key_info")
	if !exists {
		c.JSON(401, gin.H{"message": "API key info not found"})
		return
	}

	keyInfo, _ := apiKeyInfo.(*middleware.APIKeyInfo)
	agentID := keyInfo.AgentID.String()

	// TODO: Get actual agent info from database
	c.JSON(200, gin.H{
		"name":        "My Workflow App",
		"description": "Workflow application",
		"tags":        []string{"workflow"},
		"mode":        "workflow",
		"author_name": "ZGI",
		"agent_id":    agentID,
	})
}

func (h *ExternalWorkflowHandler) GetAppParameters(c *gin.Context) {
	// Get API key info
	_, exists := c.Get("api_key_info")
	if !exists {
		c.JSON(401, gin.H{"message": "API key info not found"})
		return
	}

	// TODO: Get actual workflow parameters from database
	c.JSON(200, gin.H{
		"user_input_form": []interface{}{
			gin.H{
				"paragraph": gin.H{
					"label":    "Query",
					"variable": "query",
					"required": true,
					"default":  "",
				},
			},
		},
		"file_upload": gin.H{
			"image": gin.H{
				"enabled":          false,
				"number_limits":    3,
				"transfer_methods": []string{"remote_url", "local_file"},
			},
		},
		"system_parameters": gin.H{
			"file_size_limit":       15,
			"image_file_size_limit": 10,
			"audio_file_size_limit": 50,
			"video_file_size_limit": 100,
		},
	})
}

// validateAndProcessFileVariables validates file variables in workflow inputs
// It checks if file IDs exist and are accessible, extracts file content, and adds _content variables
func (h *ExternalWorkflowHandler) validateAndProcessFileVariables(ctx context.Context, inputs map[string]interface{}, tenantID string) error {
	if inputs == nil {
		return nil
	}

	// Check if content extractor is available
	if h.contentExtractor == nil {
		logger.Warn("External API - Content extractor not available, skipping file content extraction")
		return nil
	}

	fileCount := 0
	for key, value := range inputs {
		// Check if this is a file variable (has upload_file_id field)
		if fileData, ok := value.(map[string]interface{}); ok {
			if fileID, exists := fileData["upload_file_id"]; exists {
				if fileIDStr, ok := fileID.(string); ok && fileIDStr != "" {
					fileCount++

					// Validate file exists and is accessible
					if err := h.validateFileAccess(ctx, fileIDStr, tenantID); err != nil {
						return fmt.Errorf("file variable '%s' validation failed: %w", key, err)
					}

					logger.Info("External API - File variable validated",
						"variable_name", key,
						"file_id", fileIDStr,
						"tenant_id", tenantID,
					)

					// Extract file content and add _content variable
					processedVars, err := h.contentExtractor.ProcessFileVariable(ctx, key, fileData, tenantID)
					if err != nil {
						logger.Warn("External API - File content extraction failed",
							"variable_name", key,
							"file_id", fileIDStr,
							"error", err.Error(),
						)
						// Continue with metadata only
					} else {
						// Add _content variable to inputs
						for k, v := range processedVars {
							if k != key { // Don't overwrite the original variable
								inputs[k] = v
								logger.Info("External API - Added file content variable",
									"variable_name", k,
									"tenant_id", tenantID,
								)
							}
						}
					}
				}
			}
		}

		// Check if this is a file list variable (array of file objects)
		if fileList, ok := value.([]interface{}); ok {
			hasFileObjects := false
			for i, item := range fileList {
				if fileData, ok := item.(map[string]interface{}); ok {
					if fileID, exists := fileData["upload_file_id"]; exists {
						if fileIDStr, ok := fileID.(string); ok && fileIDStr != "" {
							hasFileObjects = true
							fileCount++

							// Validate file exists and is accessible
							if err := h.validateFileAccess(ctx, fileIDStr, tenantID); err != nil {
								return fmt.Errorf("file list variable '%s[%d]' validation failed: %w", key, i, err)
							}

							logger.Info("External API - File list item validated",
								"variable_name", key,
								"index", i,
								"file_id", fileIDStr,
								"tenant_id", tenantID,
							)
						}
					}
				}
			}

			// Extract content for file list
			if hasFileObjects {
				processedVars, err := h.contentExtractor.ProcessFileListVariable(ctx, key, fileList, tenantID)
				if err != nil {
					logger.Warn("External API - File list content extraction failed",
						"variable_name", key,
						"error", err.Error(),
					)
					// Continue with metadata only
				} else {
					// Add _content variable to inputs
					for k, v := range processedVars {
						if k != key { // Don't overwrite the original variable
							inputs[k] = v
							logger.Info("External API - Added file list content variable",
								"variable_name", k,
								"tenant_id", tenantID,
							)
						}
					}
				}
			}
		}
	}

	if fileCount > 0 {
		logger.Info("External API - File variables processed",
			"file_count", fileCount,
			"tenant_id", tenantID,
		)
	}

	return nil
}

// validateFileAccess validates that a file exists and is accessible by the tenant
func (h *ExternalWorkflowHandler) validateFileAccess(ctx context.Context, fileID string, tenantID string) error {
	// Get file by ID to verify it exists
	uploadFile, err := h.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Verify file belongs to the tenant
	if uploadFile.TenantID != tenantID {
		return fmt.Errorf("file does not belong to tenant")
	}

	return nil
}

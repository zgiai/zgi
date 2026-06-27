package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// WorkflowStreamEvent represents a workflow streaming event.
type WorkflowStreamEvent struct {
	EventType     string         `json:"event"`
	WorkflowRunID string         `json:"workflow_run_id,omitempty"`
	TaskID        string         `json:"task_id,omitempty"`
	Data          map[string]any `json:"data"`
}

func prepareWorkflowStreamSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}); err != nil {
		logger.WarnContext(c.Request.Context(), "workflow stream SSE write deadline is not configurable", err)
	}
	flushWorkflowSSE(c.Request.Context(), c.Writer, "workflow_stream_open")
}

func (h *WorkflowHandler) workflowElapsedMillisecondsForEvent(ctx context.Context, workflowRunID string, fallback float64) float64 {
	workflowService, ok := h.workflowService.(*WorkflowService)
	if !ok {
		return fallback
	}
	return workflowService.workflowRunElapsedMillisecondsForEvent(ctx, workflowRunID, fallback)
}

// sendSSEError sends a Server-Sent Event error.
func (h *WorkflowHandler) sendSSEError(ctx context.Context, w http.ResponseWriter, message string) {
	if w == nil {
		logger.CriticalContext(ctx, "response writer is nil in send sse error")
		return
	}

	event := map[string]interface{}{
		"event": "error",
		"data": map[string]interface{}{
			"message": message,
		},
	}

	jsonData, err := json.Marshal(event)
	if err != nil {
		logger.ErrorContext(ctx, "failed to marshal sse error", err)
		return
	}

	fmt.Fprintf(w, "data: %s\n\n", string(jsonData))

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	} else {
		logger.CriticalContext(ctx, "response writer does not implement http flusher for sse error")
	}
}

type workflowNodeStartedEventParams struct {
	NodeExecutionID   string
	NodeID            string
	NodeType          string
	Title             string
	NodeIndex         int
	PredecessorNodeID *string
	Inputs            map[string]interface{}
	CreatedAt         time.Time
}

func buildWorkflowNodeStartedStreamEvent(params workflowNodeStartedEventParams) *WorkflowStreamEvent {
	return &WorkflowStreamEvent{
		EventType: "node_started",
		Data: map[string]interface{}{
			"id":                            params.NodeExecutionID,
			"node_id":                       params.NodeID,
			"node_type":                     params.NodeType,
			"title":                         params.Title,
			"index":                         params.NodeIndex,
			"predecessor_node_id":           params.PredecessorNodeID,
			"inputs":                        FilterFrontendInputs(params.NodeType, params.Inputs),
			"created_at":                    params.CreatedAt.Unix(),
			"extras":                        map[string]interface{}{},
			"parallel_id":                   nil,
			"parallel_start_node_id":        nil,
			"parent_parallel_id":            nil,
			"parent_parallel_start_node_id": nil,
			"iteration_id":                  nil,
			"loop_id":                       nil,
			"parallel_run_id":               nil,
			"agent_strategy":                nil,
		},
	}
}

type workflowNodeFinishedEventParams struct {
	NodeExecutionID   string
	NodeID            string
	NodeType          string
	Title             string
	NodeIndex         int
	PredecessorNodeID *string
	Inputs            map[string]interface{}
	ProcessData       interface{}
	Outputs           map[string]interface{}
	OutputHandle      string
	Status            string
	Error             interface{}
	ElapsedTime       float64
	ExecutionMetadata map[string]interface{}
	CreatedAt         time.Time
	FinishedAt        time.Time
}

func buildWorkflowNodeFinishedStreamEvent(params workflowNodeFinishedEventParams) *WorkflowStreamEvent {
	data := map[string]interface{}{
		"id":                            params.NodeExecutionID,
		"node_id":                       params.NodeID,
		"node_type":                     params.NodeType,
		"title":                         params.Title,
		"index":                         params.NodeIndex,
		"predecessor_node_id":           params.PredecessorNodeID,
		"inputs":                        FilterFrontendInputs(params.NodeType, params.Inputs),
		"process_data":                  params.ProcessData,
		"outputs":                       FilterFrontendOutputs(params.NodeType, params.Outputs),
		"output_handle":                 params.OutputHandle,
		"status":                        params.Status,
		"error":                         params.Error,
		"elapsed_time":                  params.ElapsedTime,
		"execution_metadata":            params.ExecutionMetadata,
		"created_at":                    params.CreatedAt.Unix(),
		"finished_at":                   params.FinishedAt.Unix(),
		"files":                         []interface{}{},
		"parallel_id":                   nil,
		"parallel_start_node_id":        nil,
		"parent_parallel_id":            nil,
		"parent_parallel_start_node_id": nil,
		"iteration_id":                  nil,
		"loop_id":                       nil,
	}
	return &WorkflowStreamEvent{
		EventType: "node_finished",
		Data:      data,
	}
}

type workflowStoppedEventParams struct {
	AccountLookupContext context.Context
	AccountID            string
	WorkflowRunID        string
	WorkflowID           string
	SequenceNumber       int
	Outputs              map[string]interface{}
	ErrorMessage         string
	ElapsedTime          float64
	TotalTokens          int
	TotalSteps           int
}

func (h *WorkflowHandler) sendWorkflowStoppedEvent(ctx context.Context, resultChan chan<- *WorkflowStreamEvent, params workflowStoppedEventParams) {
	userEmail := ""
	userName := ""
	lookupCtx := params.AccountLookupContext
	if lookupCtx == nil {
		lookupCtx = ctx
	}
	if account, accErr := h.accountService.GetAccountByID(lookupCtx, params.AccountID); accErr == nil && account != nil {
		userEmail = account.Email
		userName = account.Name
	}

	resultChan <- &WorkflowStreamEvent{
		EventType: "workflow_finished",
		Data: map[string]interface{}{
			"id":               params.WorkflowRunID,
			"workflow_id":      params.WorkflowID,
			"sequence_number":  params.SequenceNumber,
			"status":           "stopped",
			"outputs":          params.Outputs,
			"error":            map[string]interface{}{"message": params.ErrorMessage},
			"elapsed_time":     params.ElapsedTime,
			"total_tokens":     params.TotalTokens,
			"total_steps":      params.TotalSteps,
			"created_by":       map[string]interface{}{"id": params.AccountID, "name": userName, "email": userEmail},
			"created_at":       time.Now().Unix(),
			"finished_at":      time.Now().Unix(),
			"exceptions_count": 0,
			"files":            []interface{}{},
		},
	}
}

type workflowStreamFinalizeParams struct {
	Ctx                    context.Context
	WorkflowRunID          string
	WorkflowService        *WorkflowService
	WorkflowElapsedTracker *workflowElapsedTracker
	WorkflowStartTime      time.Time
	FailedNodes            map[string]string
	AllNodeOutputs         map[string]interface{}
	TotalWorkflowTokens    int
	NodeIndex              int
	DoneChan               chan<- map[string]interface{}
	AnswerCoordinator      *answerOutputCoordinator
}

const workflowRunFinalizePersistTimeout = 5 * time.Second

func finalizeWorkflowStreamExecution(params workflowStreamFinalizeParams) {
	trackedWorkflowElapsedTime := params.WorkflowElapsedTracker.elapsedOrFallback(ElapsedMillisecondsSince(params.WorkflowStartTime))
	actualWorkflowElapsedTime := trackedWorkflowElapsedTime
	allNodeOutputs := copyWorkflowAnyMap(params.AllNodeOutputs)
	if params.AnswerCoordinator != nil && params.AnswerCoordinator.HasCompleteOutput() {
		allNodeOutputs["answer"] = params.AnswerCoordinator.FullAnswer()
	}

	if params.WorkflowRunID != "" && params.WorkflowService != nil {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(params.Ctx), workflowRunFinalizePersistTimeout)
		defer cancel()
		actualWorkflowElapsedTime = params.WorkflowService.workflowRunElapsedMillisecondsForEvent(persistCtx, params.WorkflowRunID, trackedWorkflowElapsedTime)
		finalStatus, finalError := workflowFinalStatusFromFailedNodes(params.FailedNodes)
		updateErr := params.WorkflowService.UpdateWorkflowRunLogStatus(persistCtx, params.WorkflowRunID, finalStatus, allNodeOutputs, actualWorkflowElapsedTime, 0, params.NodeIndex-1, finalError)
		if updateErr != nil {
			logger.ErrorContext(persistCtx, "failed to update workflow run log", "workflow_run_id", params.WorkflowRunID, updateErr)
		}
	}

	finalOutputs := copyWorkflowAnyMap(allNodeOutputs)
	if workflowHasActualFailures(params.FailedNodes) {
		finalOutputs["__workflow_status__"] = "failed"
		finalOutputs["__workflow_error__"] = fmt.Sprintf("%d node(s) failed", len(params.FailedNodes))
	} else {
		finalOutputs["__workflow_status__"] = "succeeded"
	}
	finalOutputs["__total_tokens__"] = params.TotalWorkflowTokens
	finalOutputs[workflowInternalElapsedTimeKey] = actualWorkflowElapsedTime
	select {
	case params.DoneChan <- finalOutputs:
	default:
	}
}

func workflowFinalStatusFromFailedNodes(failedNodes map[string]string) (string, string) {
	finalStatus := "succeeded"
	finalError := ""
	for nodeID, errMsg := range failedNodes {
		if errMsg == "skipped" {
			continue
		}
		finalStatus = "failed"
		if finalError != "" {
			finalError += "; "
		}
		finalError += fmt.Sprintf("node %s: %s", nodeID, errMsg)
	}
	return finalStatus, finalError
}

func workflowHasActualFailures(failedNodes map[string]string) bool {
	for _, errMsg := range failedNodes {
		if errMsg != "skipped" {
			return true
		}
	}
	return false
}

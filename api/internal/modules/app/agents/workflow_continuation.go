package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"io"
	"strings"
	"time"
)

const agentWorkflowContinuationMaxDuration = 35 * time.Minute

type agentWorkflowContinuationRequest struct {
	Type          string                 `json:"type"`
	Inputs        map[string]interface{} `json:"inputs"`
	Action        string                 `json:"action"`
	ApprovalToken string                 `json:"approval_token"`
}

func (h *AgentsHandler) ContinueAgentRuntimeWorkflowApproval(c *gin.Context) {
	runtimeCtx, ok := h.agentRuntimeContext(c)
	if !ok {
		return
	}
	h.continueRuntimeWorkflowApproval(c, runtimeCtx)
}

func (h *AgentsHandler) continueRuntimeWorkflowApproval(c *gin.Context, runtimeCtx agentRuntimeContext) {
	conversationID, ok := uuidParam(c, "conversation_id")
	if !ok {
		return
	}
	messageID, ok := uuidParam(c, "message_id")
	if !ok {
		return
	}
	req, err := readAgentWorkflowContinuationRequest(c)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	questionContinuation := isAgentWorkflowQuestionContinuation(req)
	approvalContinuation := isAgentWorkflowApprovalContinuation(req)
	var resumeInputs map[string]interface{}
	if questionContinuation || approvalContinuation {
		if h.workflowContinuationRunner == nil {
			h.failRuntime(c, fmt.Errorf("%w: workflow continuation runner is not configured", runtimeservice.ErrInvalidInput))
			return
		}
	}
	if questionContinuation {
		resumeInputs = normalizeAgentWorkflowQuestionInputs(req.Inputs)
		if len(resumeInputs) == 0 {
			h.failRuntime(c, fmt.Errorf("%w: question answer continuation inputs are required", runtimeservice.ErrInvalidInput))
			return
		}
	}
	if approvalContinuation {
		if strings.TrimSpace(req.ApprovalToken) == "" || strings.TrimSpace(req.Action) == "" {
			h.failRuntime(c, fmt.Errorf("%w: approval token and action are required", runtimeservice.ErrInvalidInput))
			return
		}
	}
	continuation, err := h.chatRuntimeService.BeginWorkflowApprovalContinuation(c.Request.Context(), runtimeCtx.Scope, runtimeCtx.Caller, runtimeCtx.RunConfig, conversationID, messageID)
	if err != nil {
		h.failRuntime(c, err)
		return
	}
	setupAgentSSE(c)
	emit := func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	}
	h.emitAgentWorkflowContinuationStreamEvent(c.Request.Context(), continuation, "message_start", gin.H{
		"conversation_id": conversationID.String(),
		"message_id":      messageID.String(),
		"workflow_run_id": continuation.WorkflowRunID,
		"created_at":      time.Now().Unix(),
		"continuation":    true,
	}, emit)
	if continuation.Completed {
		h.emitAgentWorkflowContinuationStreamEvent(c.Request.Context(), continuation, "message_end", gin.H{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"status":          runtimemodel.MessageStatusCompleted,
			"metadata":        continuation.Metadata,
		}, emit)
		return
	}
	if approvalContinuation && h.finishAgentWorkflowContinuationIfRunTerminal(
		c.Request.Context(),
		runtimeCtx.Scope,
		continuation,
		"",
		false,
		emit,
	) {
		return
	}
	if h.streamWorkflowApprovalContinuationDirect(c, runtimeCtx.Scope, continuation, req, resumeInputs, approvalContinuation, questionContinuation) {
		return
	}
	afterSequence := 0
	if questionContinuation || approvalContinuation {
		run, err := h.loadAgentWorkflowRunLog(c.Request.Context(), continuation.WorkflowRunID)
		if err != nil {
			h.failRuntime(c, err)
			return
		}
		afterSequence = h.latestAgentWorkflowContinuationSequence(c.Request.Context(), run.TenantID, continuation.WorkflowRunID)
	}
	resumeErrCh := make(chan error, 1)
	if approvalContinuation {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), agentWorkflowContinuationMaxDuration)
			defer cancel()
			if err := h.resumeAgentWorkflowApproval(ctx, runtimeCtx.Scope, continuation, req); err != nil {
				resumeErrCh <- err
			}
		}()
	}
	if questionContinuation {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), agentWorkflowContinuationMaxDuration)
			defer cancel()
			if err := h.workflowContinuationRunner.ResumeQuestionAnswerWorkflow(ctx, continuation.WorkflowRunID, resumeInputs); err != nil {
				resumeErrCh <- err
			}
		}()
	}
	h.streamWorkflowApprovalContinuation(c, runtimeCtx.Scope, continuation, afterSequence, resumeErrCh)
}

func readAgentWorkflowContinuationRequest(c *gin.Context) (agentWorkflowContinuationRequest, error) {
	var req agentWorkflowContinuationRequest
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return req, nil
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return req, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return req, nil
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return req, fmt.Errorf("invalid workflow continuation request")
	}
	return req, nil
}

func isAgentWorkflowQuestionContinuation(req agentWorkflowContinuationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Type), "question_answer")
}

func isAgentWorkflowApprovalContinuation(req agentWorkflowContinuationRequest) bool {
	return strings.EqualFold(strings.TrimSpace(req.Type), "approval")
}

func (h *AgentsHandler) resumeAgentWorkflowApproval(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest) error {
	if h.db == nil {
		return errors.New("database is not available")
	}
	if h.workflowContinuationRunner == nil {
		return fmt.Errorf("%w: workflow continuation runner is not configured", runtimeservice.ErrInvalidInput)
	}
	approvalService := approvalruntime.NewService(h.db)
	accountID := scope.AccountID.String()
	form, err := submitAgentWorkflowApprovalForm(ctx, approvalService, continuation, req, &accountID)
	if err != nil {
		return err
	}
	resumeReady, err := approvalService.ActivePauseApprovalFormsSubmitted(ctx, form.WorkflowRunID)
	if err != nil {
		return err
	}
	if !resumeReady {
		if err := approvalService.AppendApprovalResultFilledEvent(ctx, form); err != nil {
			logger.WarnContext(ctx, "failed to append agent workflow approval result filled event", "form_id", form.ID, err)
		}
		return fmt.Errorf("workflow run %s is waiting for additional approvals", continuation.WorkflowRunID)
	}
	return h.workflowContinuationRunner.ResumeApprovalWorkflow(ctx, form)
}

func (h *AgentsHandler) resumeAgentWorkflowApprovalStream(ctx context.Context, scope runtimeservice.Scope, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest, runner workflowContinuationStreamRunner, onEvent func(string, map[string]interface{}) error) error {
	if h.db == nil {
		return errors.New("database is not available")
	}
	approvalService := approvalruntime.NewService(h.db)
	accountID := scope.AccountID.String()
	form, err := submitAgentWorkflowApprovalForm(ctx, approvalService, continuation, req, &accountID)
	if err != nil {
		return err
	}
	resumeReady, err := approvalService.ActivePauseApprovalFormsSubmitted(ctx, form.WorkflowRunID)
	if err != nil {
		return err
	}
	if !resumeReady {
		if err := approvalService.AppendApprovalResultFilledEvent(ctx, form); err != nil {
			logger.WarnContext(ctx, "failed to append agent workflow approval result filled event", "form_id", form.ID, err)
		}
		return fmt.Errorf("workflow run %s is waiting for additional approvals", continuation.WorkflowRunID)
	}
	return runner.ResumeApprovalWorkflowStream(ctx, form, onEvent)
}

func submitAgentWorkflowApprovalForm(ctx context.Context, approvalService *approvalruntime.Service, continuation *runtimeservice.WorkflowApprovalContinuation, req agentWorkflowContinuationRequest, accountID *string) (*approvalruntime.Form, error) {
	workflowRunID := ""
	if continuation != nil {
		workflowRunID = continuation.WorkflowRunID
	}
	form, err := approvalService.SubmitByTokenForWorkflowRun(ctx, strings.TrimSpace(req.ApprovalToken), workflowRunID, approvalruntime.SubmitRequest{
		Inputs: copyMapForAgentWorkflowContinuation(req.Inputs),
		Action: strings.TrimSpace(req.Action),
	}, accountID, nil)
	if err != nil {
		return nil, mapAgentWorkflowApprovalError(err)
	}
	if err := ensureAgentWorkflowContinuationApprovalForm(continuation, form); err != nil {
		return nil, err
	}
	return form, nil
}

func mapAgentWorkflowApprovalError(err error) error {
	if errors.Is(err, approvalruntime.ErrFormNotFound) {
		return fmt.Errorf("%w: workflow continuation approval form not found", runtimeservice.ErrNotFound)
	}
	return err
}

func ensureAgentWorkflowContinuationApprovalForm(continuation *runtimeservice.WorkflowApprovalContinuation, form *approvalruntime.Form) error {
	if continuation == nil || form == nil {
		return fmt.Errorf("%w: workflow continuation approval form is required", runtimeservice.ErrInvalidInput)
	}
	continuationRunID := strings.TrimSpace(continuation.WorkflowRunID)
	formRunID := strings.TrimSpace(form.WorkflowRunID)
	if continuationRunID == "" || formRunID == "" || formRunID != continuationRunID {
		return fmt.Errorf("%w: workflow continuation approval form not found", runtimeservice.ErrNotFound)
	}
	return nil
}

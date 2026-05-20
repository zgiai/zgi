package workflow

import (
	"context"
	"sync"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/conversation"
	"github.com/zgiai/ginext/pkg/logger"
)

const workflowEventAnswerSnapshotReady = "answer_snapshot_ready"
const answerSnapshotPersistTimeout = 10 * time.Second

type answerSnapshotWriter struct {
	mu            sync.Mutex
	lastAsyncDone chan struct{}
	handler       *WorkflowHandler
	workflowRunID string
	agentID       string
	accountID     string
	systemInputs  map[string]interface{}
	requestInputs map[string]interface{}
	triggeredFrom string

	lastPersistedAnswer string
	loadedExisting      bool
}

func newAnswerSnapshotWriter(handler *WorkflowHandler, workflowRunID, agentID, accountID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom string) *answerSnapshotWriter {
	if handler == nil || handler.advancedChatHandler == nil || workflowRunID == "" {
		return nil
	}
	return &answerSnapshotWriter{
		handler:       handler,
		workflowRunID: workflowRunID,
		agentID:       agentID,
		accountID:     accountID,
		systemInputs:  systemInputs,
		requestInputs: requestInputs,
		triggeredFrom: triggeredFrom,
	}
}

func (w *answerSnapshotWriter) Persist(ctx context.Context, answer string, status string, force bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	w.loadExistingAnswer(ctx)
	if !force && answer == w.lastPersistedAnswer {
		return
	}
	if !w.handler.persistWorkflowConversationAnswerSnapshot(ctx, w.workflowRunID, w.agentID, w.accountID, w.systemInputs, w.requestInputs, w.triggeredFrom, answer, status) {
		return
	}
	w.lastPersistedAnswer = answer
}

func (w *answerSnapshotWriter) PersistAsync(ctx context.Context, answer string, status string, force bool) {
	if w == nil {
		return
	}
	w.mu.Lock()
	previous := w.lastAsyncDone
	done := make(chan struct{})
	w.lastAsyncDone = done
	w.mu.Unlock()

	go func() {
		if previous != nil {
			<-previous
		}
		defer close(done)
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), answerSnapshotPersistTimeout)
		defer cancel()
		w.Persist(persistCtx, answer, status, force)
	}()
}

func (w *answerSnapshotWriter) loadExistingAnswer(ctx context.Context) {
	if w.loadedExisting {
		return
	}
	w.loadedExisting = true
	existingMessages, err := w.handler.advancedChatHandler.GetFirstMessagesByWorkflowRunIDs(ctx, []string{w.workflowRunID})
	if err != nil {
		logger.WarnContext(ctx, "failed to load existing workflow answer snapshot", "workflow_run_id", w.workflowRunID, err)
		return
	}
	if existing := existingMessages[w.workflowRunID]; existing != nil {
		w.lastPersistedAnswer = existing.Answer
	}
}

func (h *WorkflowHandler) persistWorkflowConversationAnswerSnapshot(ctx context.Context, workflowRunID, agentID, accountID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom string, answer string, status string) bool {
	if h == nil || h.advancedChatHandler == nil || workflowRunID == "" {
		return false
	}
	conversationID := workflowConversationID(systemInputs, requestInputs)
	if conversationID == "" {
		return false
	}

	messageData, err := buildApprovalPauseConversationMessageData(workflowRunID, agentID, accountID, conversationID, systemInputs, requestInputs, triggeredFrom, answer)
	if err != nil {
		logger.WarnContext(ctx, "invalid workflow answer snapshot data", "workflow_run_id", workflowRunID, err)
		return false
	}
	messageData.Status = status
	if messageData.Status == "" {
		messageData.Status = conversation.AgentMessageStatusRunning
	}

	existingMessages, err := h.advancedChatHandler.GetFirstMessagesByWorkflowRunIDs(ctx, []string{workflowRunID})
	if err != nil {
		logger.ErrorContext(ctx, "failed to check existing workflow answer snapshot", "workflow_run_id", workflowRunID, err)
		return false
	}
	if existing := existingMessages[workflowRunID]; existing != nil {
		if err := updateApprovalConversationMessage(ctx, h, existing, messageData); err != nil {
			logger.ErrorContext(ctx, "failed to update workflow answer snapshot", "conversation_id", conversationID, "workflow_run_id", workflowRunID, err)
			return false
		}
		return true
	}

	_, err = h.advancedChatHandler.CreateWorkflowMessageWithInputsAndStatus(
		messageData.AgentID,
		messageData.ConversationID,
		messageData.WorkflowRunID,
		messageData.Query,
		messageData.Answer,
		messageData.FromSource,
		messageData.InvokeFrom,
		messageData.FromUserID,
		messageData.CreatedBy,
		messageData.WebAppID,
		messageData.Inputs,
		messageData.Status,
	)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create workflow answer snapshot", "conversation_id", conversationID, "workflow_run_id", workflowRunID, err)
		return false
	}
	return true
}

func workflowAnswerSnapshotText(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	answer, _ := data["answer"].(string)
	return answer
}

package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	workflowEventAnswerSnapshotReady = "answer_snapshot_ready"
	answerSnapshotPersistTimeout     = 10 * time.Second
	answerSnapshotFlushInterval      = 750 * time.Millisecond
	answerSnapshotFlushBytes         = 4 * 1024
)

type answerSnapshot struct {
	answer string
	status string
	force  bool
	ctx    context.Context
}

type answerSnapshotWriter struct {
	mu            sync.Mutex
	flushMu       sync.Mutex
	handler       *WorkflowHandler
	workflowRunID string
	agentID       string
	accountID     string
	systemInputs  map[string]interface{}
	requestInputs map[string]interface{}
	triggeredFrom string
	wake          chan struct{}
	stop          chan struct{}
	done          chan struct{}
	latest        *answerSnapshot
	closed        bool

	lastPersistedAnswer string
	lastRevision        int64
	lastError           error
	persistCheckpoint   func(context.Context, string, string, string) (int64, error)
}

func newAnswerSnapshotWriter(handler *WorkflowHandler, workflowRunID, agentID, accountID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom string) *answerSnapshotWriter {
	if handler == nil || handler.advancedChatHandler == nil || workflowRunID == "" {
		return nil
	}
	w := &answerSnapshotWriter{
		handler:       handler,
		workflowRunID: workflowRunID,
		agentID:       agentID,
		accountID:     accountID,
		systemInputs:  systemInputs,
		requestInputs: requestInputs,
		triggeredFrom: triggeredFrom,
		wake:          make(chan struct{}, 1),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	w.persistCheckpoint = func(ctx context.Context, previousAnswer, answer, status string) (int64, error) {
		return handler.persistWorkflowConversationAnswerCheckpoint(
			ctx,
			workflowRunID,
			agentID,
			accountID,
			systemInputs,
			requestInputs,
			triggeredFrom,
			previousAnswer,
			answer,
			status,
		)
	}
	go w.run()
	return w
}

func newWorkflowAnswerSnapshotWriter(runType string, handler *WorkflowHandler, workflowRunID, agentID, accountID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom string) *answerSnapshotWriter {
	if runType != "CONVERSATION_WORKFLOW" {
		return nil
	}
	return newAnswerSnapshotWriter(handler, workflowRunID, agentID, accountID, systemInputs, requestInputs, triggeredFrom)
}

func (w *answerSnapshotWriter) run() {
	defer close(w.done)
	ticker := time.NewTicker(answerSnapshotFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.flushLatest()
		case <-w.wake:
			w.flushLatest()
		case <-w.stop:
			w.flushLatest()
			return
		}
	}
}

func (w *answerSnapshotWriter) Persist(ctx context.Context, answer string, status string, force bool) error {
	if w == nil {
		return nil
	}
	w.offer(ctx, answer, status, force)
	return w.flushLatest()
}

func (w *answerSnapshotWriter) PersistAsync(ctx context.Context, answer string, status string, force bool) {
	if w == nil {
		return
	}
	shouldWake := w.offer(ctx, answer, status, force)
	if shouldWake {
		select {
		case w.wake <- struct{}{}:
		default:
		}
	}
}

func (w *answerSnapshotWriter) PersistFinal(ctx context.Context, answer string, status string) error {
	if w == nil {
		return nil
	}
	w.offer(ctx, answer, status, true)
	err := w.flushLatest()
	w.close()
	return err
}

// SeedPersistedAnswer establishes the durable answer that existed before this
// writer took ownership. Continuations need this baseline so their first
// checkpoint contains only newly produced text instead of replaying the whole
// pre-pause answer as a delta.
func (w *answerSnapshotWriter) SeedPersistedAnswer(answer string) {
	if w == nil || answer == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.latest != nil || w.lastPersistedAnswer != "" {
		return
	}
	w.lastPersistedAnswer = answer
}

func (w *answerSnapshotWriter) offer(ctx context.Context, answer string, status string, force bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	previousLength := len(w.lastPersistedAnswer)
	if w.latest != nil && len(w.latest.answer) > previousLength {
		previousLength = len(w.latest.answer)
	}
	w.latest = &answerSnapshot{answer: answer, status: status, force: force, ctx: context.WithoutCancel(ctx)}
	return force || len(answer)-previousLength >= answerSnapshotFlushBytes
}

func (w *answerSnapshotWriter) flushLatest() error {
	if w == nil {
		return nil
	}
	w.flushMu.Lock()
	defer w.flushMu.Unlock()

	w.mu.Lock()
	snapshot := w.latest
	w.latest = nil
	lastAnswer := w.lastPersistedAnswer
	w.mu.Unlock()
	if snapshot == nil || (!snapshot.force && snapshot.answer == lastAnswer) {
		return nil
	}

	persistCtx, cancel := context.WithTimeout(snapshot.ctx, answerSnapshotPersistTimeout)
	defer cancel()
	if w.persistCheckpoint == nil {
		return fmt.Errorf("workflow answer checkpoint persistence is not configured")
	}
	revision, err := w.persistCheckpoint(persistCtx, lastAnswer, snapshot.answer, snapshot.status)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.lastError = err
		if w.latest == nil {
			w.latest = snapshot
		}
		return err
	}
	w.lastPersistedAnswer = snapshot.answer
	w.lastRevision = revision
	w.lastError = nil
	return nil
}

func (w *answerSnapshotWriter) close() {
	w.closeWithPending(true)
}

// closeWithoutFlush hands the terminal boundary to FinalizeRun. Running
// checkpoints may already be durable, but a pending snapshot must not update
// the message status before the run and terminal events commit atomically.
func (w *answerSnapshotWriter) closeWithoutFlush() {
	w.closeWithPending(false)
}

func (w *answerSnapshotWriter) closeWithPending(flushPending bool) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	if !flushPending {
		w.latest = nil
	}
	w.closed = true
	close(w.stop)
	w.mu.Unlock()
	select {
	case <-w.done:
	case <-time.After(time.Second):
	}
}

func (h *WorkflowHandler) persistWorkflowConversationAnswerCheckpoint(ctx context.Context, workflowRunID, agentID, accountID string, systemInputs map[string]interface{}, requestInputs map[string]interface{}, triggeredFrom, previousAnswer, answer, status string) (int64, error) {
	if h == nil || h.advancedChatHandler == nil || workflowRunID == "" {
		return 0, fmt.Errorf("workflow answer persistence is not configured")
	}
	conversationID := workflowConversationID(systemInputs, requestInputs)
	if conversationID == "" {
		return 0, fmt.Errorf("workflow conversation id is empty")
	}
	messageData, err := buildApprovalPauseConversationMessageData(workflowRunID, agentID, accountID, conversationID, systemInputs, requestInputs, triggeredFrom, answer)
	if err != nil {
		return 0, err
	}
	messageData.Status = status
	if messageData.Status == "" {
		messageData.Status = conversation.AgentMessageStatusRunning
	}
	owner, hasOwner := workflowExecutionOwnerFromContext(ctx)
	var revision int64
	var projectionMessageID uuid.UUID
	db := database.GetDB()
	finishTransactionMetric := beginWorkflowDBTransaction(ctx, "answer_checkpoint")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runScope struct {
			TenantID               string  `gorm:"column:tenant_id"`
			AgentID                string  `gorm:"column:agent_id"`
			RuntimeProtocolVersion int     `gorm:"column:runtime_protocol_version"`
			ExecutionGeneration    int64   `gorm:"column:execution_generation"`
			ActiveExecutionID      *string `gorm:"column:active_execution_id"`
		}
		runQuery := tx.Table("workflow_run_logs").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("tenant_id, agent_id, runtime_protocol_version, execution_generation, active_execution_id").
			Where("id = ? AND deleted_at IS NULL", workflowRunID)
		if hasOwner {
			runQuery = runQuery.Where("execution_generation = ? AND active_execution_id = ?", owner.Generation, owner.ExecutionID)
		}
		if err := runQuery.Take(&runScope).Error; err != nil {
			if hasOwner && errors.Is(err, gorm.ErrRecordNotFound) {
				return workflowpause.ErrExecutionOwnershipLost
			}
			if err != nil {
				return fmt.Errorf("verify workflow answer owner: %w", err)
			}
		}
		if runScope.RuntimeProtocolVersion >= workflowRuntimeProtocolVersionV2 &&
			(!hasOwner || runScope.ExecutionGeneration != owner.Generation || runScope.ActiveExecutionID == nil || *runScope.ActiveExecutionID != owner.ExecutionID) {
			return workflowpause.ErrExecutionOwnershipLost
		}

		message, err := workflowConversationMessageFromData(messageData, owner.Generation)
		if err != nil {
			return err
		}
		updates := workflowConversationMessageUpdates(message, owner.Generation)
		updated := &conversation.AgentMessage{}
		query := tx.Model(updated).Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "projection_revision"}}}).
			Where("workflow_run_id = ? AND deleted_at IS NULL", messageData.WorkflowRunID)
		if hasOwner {
			query = query.Where("execution_generation <= ?", owner.Generation)
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("update workflow answer projection: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			message.ProjectionRevision = 1
			if err := tx.Create(message).Error; err != nil {
				if !isUniqueConstraintError(err) {
					return fmt.Errorf("create workflow answer projection: %w", err)
				}
				updated = &conversation.AgentMessage{}
				retry := tx.Model(updated).Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "projection_revision"}}}).
					Where("workflow_run_id = ? AND deleted_at IS NULL", messageData.WorkflowRunID)
				if hasOwner {
					retry = retry.Where("execution_generation <= ?", owner.Generation)
				}
				retryResult := retry.Updates(updates)
				if retryResult.Error != nil {
					return fmt.Errorf("retry workflow answer projection: %w", retryResult.Error)
				}
				if retryResult.RowsAffected != 1 {
					return workflowpause.ErrExecutionOwnershipLost
				}
				revision = updated.ProjectionRevision
				projectionMessageID = updated.ID
			} else {
				if err := tx.Model(&conversation.AgentConversation{}).
					Where("id = ?", message.ConversationID).
					UpdateColumn("dialogue_count", gorm.Expr("dialogue_count + 1")).Error; err != nil {
					return fmt.Errorf("increment workflow conversation dialogue count: %w", err)
				}
				revision = message.ProjectionRevision
				projectionMessageID = message.ID
			}
		} else {
			revision = updated.ProjectionRevision
			projectionMessageID = updated.ID
		}

		delta, replace := workflowAnswerDelta(previousAnswer, answer)
		digest := sha256.Sum256([]byte(answer))
		_, err = workflowpause.NewService(tx).AppendEventPayloadTx(ctx, tx, workflowpause.AppendEventParams{
			TenantID:                    runScope.TenantID,
			AppID:                       runScope.AgentID,
			WorkflowRunID:               workflowRunID,
			EventType:                   workflowEventMessage,
			Category:                    workflowpause.EventCategoryAnswerCheckpoint,
			ExecutionID:                 owner.ExecutionID,
			ExpectedExecutionID:         owner.ExecutionID,
			ExpectedExecutionGeneration: owner.Generation,
			IdempotencyKey:              fmt.Sprintf("answer-checkpoint:%s:%d:%d", workflowRunID, owner.Generation, revision),
			EventData: map[string]interface{}{
				"id":                    workflowRunID,
				"message_id":            projectionMessageID.String(),
				"conversation_id":       messageData.ConversationID.String(),
				"answer_delta":          delta,
				"answer_revision":       revision,
				"answer_length":         len(answer),
				"answer_digest":         hex.EncodeToString(digest[:]),
				"replace":               replace,
				"projection_generation": owner.Generation,
			},
		})
		if err != nil {
			return fmt.Errorf("append workflow answer checkpoint: %w", err)
		}
		return nil
	})
	finishTransactionMetric()
	if err != nil {
		if errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
			recordWorkflowProjectionConflict(ctx, "ownership_lost")
		}
		return 0, err
	}
	recordWorkflowAnswerCheckpoint(ctx)
	// Checkpoints usually create the row while it is still running. Trigger
	// title generation on the first non-empty lifecycle boundary instead of on
	// INSERT; the title service's compare-and-swap makes subsequent boundaries
	// harmless once the default title has changed.
	if strings.TrimSpace(answer) != "" {
		h.enqueueWebAppConversationTitleGeneration(ctx, systemInputs, requestInputs, messageData)
	}
	return revision, nil
}

func workflowConversationMessageFromData(data approvalConversationMessageData, generation int64) (*conversation.AgentMessage, error) {
	message := &conversation.AgentMessage{
		ID:                  uuid.New(),
		AgentID:             data.AgentID,
		ConversationID:      data.ConversationID,
		Query:               data.Query,
		Answer:              data.Answer,
		Status:              approvalConversationMessageStatus(data.Status),
		Currency:            "USD",
		InvokeFrom:          &data.InvokeFrom,
		FromSource:          data.FromSource,
		CreatedBy:           data.CreatedBy,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		AgentBased:          true,
		WorkflowRunID:       &data.WorkflowRunID,
		WebAppID:            data.WebAppID,
		ExecutionGeneration: generation,
	}
	zero := 0.0
	priceUnit := 0.001
	message.MessageUnitPrice = &zero
	message.AnswerUnitPrice = &zero
	message.MessagePriceUnit = &priceUnit
	message.AnswerPriceUnit = &priceUnit
	if data.FromSource == string(UserFromEndUser) {
		message.FromEndUserID = &data.FromUserID
	} else {
		message.FromAccountID = &data.FromUserID
	}
	if err := message.SetInputsFromMap(data.Inputs); err != nil {
		return nil, err
	}
	if err := message.SetMessageFromArray([]interface{}{
		map[string]interface{}{"role": "user", "content": data.Query},
		map[string]interface{}{"role": "assistant", "content": data.Answer},
	}); err != nil {
		return nil, err
	}
	return message, nil
}

func workflowConversationMessageUpdates(message *conversation.AgentMessage, generation int64) map[string]interface{} {
	return map[string]interface{}{
		"query":                message.Query,
		"answer":               message.Answer,
		"status":               message.Status,
		"error":                nil,
		"inputs":               message.Inputs,
		"message":              message.Message,
		"from_source":          message.FromSource,
		"invoke_from":          message.InvokeFrom,
		"from_end_user_id":     message.FromEndUserID,
		"from_account_id":      message.FromAccountID,
		"created_by":           message.CreatedBy,
		"web_app_id":           message.WebAppID,
		"execution_generation": generation,
		"projection_revision":  gorm.Expr("projection_revision + 1"),
		"updated_at":           time.Now(),
	}
}

func workflowAnswerDelta(previous, current string) (string, bool) {
	if strings.HasPrefix(current, previous) {
		return current[len(previous):], false
	}
	return current, true
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

func workflowAnswerSnapshotText(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	answer, _ := data["answer"].(string)
	return answer
}

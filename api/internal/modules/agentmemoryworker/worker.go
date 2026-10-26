package agentmemoryworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/gorm"
)

const (
	maxExtractionTurns = 6
	// Earlier turns get their own budget on top of the memory files and the
	// newest turn, so full files do not crowd out the conversation; the whole
	// request stays capped.
	maxExtractionTurnTokens     = 8000
	maxExtractionInputTokens    = 96000
	maxExtractionCASRetries     = 3
	maxExtractionJobAttempts    = 5
	maxExtractionOutputTokens   = 1200
	extractionRetryBaseDelay    = time.Minute
	extractionRetryMaximumDelay = 30 * time.Minute
	extractionJobRetention      = 30 * 24 * time.Hour
	extractionCleanupBatchSize  = 500
	auditRetention              = 180 * 24 * time.Hour
)

type Runner struct {
	db        *gorm.DB
	memory    extractionMemoryService
	llmClient llmclient.LLMClient
	estimator *tokenestimate.Estimator
}

type extractionMemoryService interface {
	ListDueExtractionJobs(context.Context, int) ([]*agentmemory.AgentMemoryExtractionJob, error)
	ClaimExtractionJob(context.Context, uuid.UUID) (*agentmemory.AgentMemoryExtractionJob, error)
	FinishExtractionJob(context.Context, uuid.UUID, string, string) error
	RescheduleExtractionJob(context.Context, uuid.UUID, string, time.Time) error
	DeleteTerminalExtractionJobs(context.Context, time.Time, int) (int64, error)
	ReadUserMemory(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID) ([]agentmemory.SlotValueResponse, error)
	MutateValues(context.Context, uuid.UUID, uuid.UUID, []agentmemory.RuntimeSlot, string, uuid.UUID, agentmemory.MutateValuesRequest, agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error)
	ExtractionBounds(context.Context, *agentmemory.AgentMemoryExtractionJob, time.Time) (*time.Time, *time.Time, error)
	CleanupExpiredRelationshipMemory(context.Context, time.Time, time.Duration) error
}

func NewRunner(db *gorm.DB, memory extractionMemoryService, client llmclient.LLMClient) *Runner {
	return &Runner{db: db, memory: memory, llmClient: client, estimator: tokenestimate.NewEstimator()}
}

func (r *Runner) ProcessDue(ctx context.Context, limit int) error {
	if r == nil || r.memory == nil {
		return nil
	}
	if cleanupErr := r.cleanupExpired(ctx); cleanupErr != nil {
		return cleanupErr
	}
	if !globalAutomaticExtractionEnabled() {
		return nil
	}
	jobs, err := r.memory.ListDueExtractionJobs(ctx, limit)
	if err != nil {
		return fmt.Errorf("list due agent memory jobs: %w", err)
	}
	var firstErr error
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if err := r.RunJob(ctx, job.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Runner) cleanupExpired(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	now := time.Now()
	if err := r.memory.CleanupExpiredRelationshipMemory(ctx, now, auditRetention); err != nil {
		return err
	}
	if _, err := r.memory.DeleteTerminalExtractionJobs(ctx, now.Add(-extractionJobRetention), extractionCleanupBatchSize); err != nil {
		return fmt.Errorf("delete expired agent memory extraction jobs: %w", err)
	}
	return nil
}

func globalAutomaticExtractionEnabled() bool {
	return relationshipmemory.AutomaticExtractionEnabled()
}

func (r *Runner) RunJob(ctx context.Context, jobID uuid.UUID) error {
	if r == nil || r.db == nil || r.memory == nil || r.llmClient == nil {
		return nil
	}
	job, err := r.memory.ClaimExtractionJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, agentmemory.ErrConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if job == nil {
		return nil
	}
	status, errorCode := agentmemory.ExtractionJobCompleted, ""
	var retryAt *time.Time
	defer func() {
		finishCtx := context.WithoutCancel(ctx)
		if retryAt != nil {
			_ = r.memory.RescheduleExtractionJob(finishCtx, jobID, errorCode, *retryAt)
			return
		}
		_ = r.memory.FinishExtractionJob(finishCtx, jobID, status, errorCode)
	}()
	markFailed := func(code string) {
		errorCode = code
		next, retry := extractionRetryAt(job.AttemptCount, time.Now())
		if !retry {
			status = agentmemory.ExtractionJobExhausted
			retryAt = nil
			return
		}
		status = agentmemory.ExtractionJobFailed
		retryAt = &next
	}

	turns, scope, err := r.loadCompletedTurns(ctx, job)
	if err != nil {
		if errors.Is(err, agentmemory.ErrConflict) {
			status, errorCode = agentmemory.ExtractionJobCancelled, "epoch_changed"
			return nil
		}
		markFailed("load_turns")
		return err
	}
	if len(turns) == 0 {
		return nil
	}

	for attempt := 0; attempt < maxExtractionCASRetries; attempt++ {
		conflict, runErr := r.extractAndApply(ctx, job, scope, turns)
		if runErr != nil {
			markFailed(extractionErrorCode(runErr))
			return runErr
		}
		if !conflict {
			return nil
		}
	}
	markFailed("revision_conflict")
	return agentmemory.ErrConflict
}

func extractionRetryDelay(attemptCount int) time.Duration {
	delay := extractionRetryBaseDelay
	for attempt := 1; attempt < attemptCount && delay < extractionRetryMaximumDelay; attempt++ {
		delay *= 2
		if delay >= extractionRetryMaximumDelay {
			return extractionRetryMaximumDelay
		}
	}
	return delay
}

func extractionRetryAt(attemptCount int, now time.Time) (time.Time, bool) {
	if attemptCount >= maxExtractionJobAttempts {
		return time.Time{}, false
	}
	return now.Add(extractionRetryDelay(attemptCount)), true
}

type completedTurn struct {
	ID            uuid.UUID
	Query         string
	Answer        string
	CompletedAt   time.Time
	ModelProvider string
	ModelName     string
	// UserInputResponses holds the user's answers to questions asked in the
	// turn; withUserInputAnswers folds them into Query.
	UserInputResponses []byte
}

// withUserInputAnswers appends the user's answers to the turn's query: they
// are the user's words for the turn, which extraction reads and evidence may
// quote.
func (t completedTurn) withUserInputAnswers() completedTurn {
	var responses []struct {
		Answers []struct {
			Value string `json:"value"`
		} `json:"answers"`
	}
	if len(t.UserInputResponses) == 0 || json.Unmarshal(t.UserInputResponses, &responses) != nil {
		return t
	}
	lines := []string{t.Query}
	for _, response := range responses {
		for _, answer := range response.Answers {
			if value := strings.TrimSpace(answer.Value); value != "" {
				lines = append(lines, value)
			}
		}
	}
	t.Query = strings.Join(lines, "\n")
	t.UserInputResponses = nil
	return t
}

type extractionScope struct {
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	WorkspaceID    uuid.UUID
}

func (r *Runner) loadCompletedTurns(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob) ([]completedTurn, extractionScope, error) {
	var scope extractionScope
	if err := r.db.WithContext(ctx).Table("chat_runtime_conversations").
		Select("organization_id, account_id, workspace_id").
		Where("id = ? AND deleted_at IS NULL", job.ConversationID).Take(&scope).Error; err != nil {
		return nil, scope, err
	}
	if scope.WorkspaceID != job.WorkspaceID {
		return nil, scope, agentmemory.ErrUnauthorized
	}
	var watermark struct{ CreatedAt time.Time }
	if err := r.db.WithContext(ctx).Table("chat_runtime_messages").Select("created_at").
		Where("id = ? AND conversation_id = ? AND status = ? AND deleted_at IS NULL", job.MessageWatermarkID, job.ConversationID, "completed").Take(&watermark).Error; err != nil {
		return nil, scope, err
	}
	lowerBound, previousCreatedAt, err := r.memory.ExtractionBounds(ctx, job, watermark.CreatedAt)
	if err != nil {
		return nil, scope, err
	}
	if previousCreatedAt != nil && (lowerBound == nil || previousCreatedAt.After(*lowerBound)) {
		value := *previousCreatedAt
		lowerBound = &value
	}
	query := r.db.WithContext(ctx).Table("chat_runtime_messages").
		Select("id, query, COALESCE(answer, '') AS answer, updated_at AS completed_at, COALESCE(model_provider, '') AS model_provider, model_name, metadata -> 'user_input_responses' AS user_input_responses").
		Where("conversation_id = ? AND status = ? AND created_at <= ? AND deleted_at IS NULL", job.ConversationID, "completed", watermark.CreatedAt)
	if lowerBound != nil {
		query = query.Where("created_at > ?", *lowerBound)
	}
	var turns []completedTurn
	if err := query.Order("created_at DESC").Limit(maxExtractionTurns).Find(&turns).Error; err != nil {
		return nil, scope, err
	}
	for i, j := 0, len(turns)-1; i < j; i, j = i+1, j-1 {
		turns[i], turns[j] = turns[j], turns[i]
	}
	for i := range turns {
		turns[i] = turns[i].withUserInputAnswers()
	}
	return turns, scope, nil
}

type chatRequestBuilder func([]completedTurn, []agentmemory.RuntimeSlot, []agentmemory.SlotValueResponse) (*adapter.ChatRequest, error)

func (r *Runner) fitTurnBudget(turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse, build chatRequestBuilder) ([]completedTurn, error) {
	if len(turns) == 0 {
		return nil, nil
	}
	newest, err := build(turns[len(turns)-1:], slots, values)
	if err != nil {
		return nil, err
	}
	limit := min(r.estimator.EstimateChatRequest(newest).Tokens+maxExtractionTurnTokens, maxExtractionInputTokens)
	selected := make([]completedTurn, 0, len(turns))
	for i := len(turns) - 1; i >= 0; i-- {
		candidate := make([]completedTurn, 0, len(selected)+1)
		candidate = append(candidate, turns[i])
		candidate = append(candidate, selected...)
		request, err := build(candidate, slots, values)
		if err != nil {
			return nil, err
		}
		if r.estimator.EstimateChatRequest(request).Tokens > limit {
			break
		}
		selected = candidate
	}
	return selected, nil
}

func (r *Runner) extractAndApply(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob, scope extractionScope, turns []completedTurn) (bool, error) {
	runtimeSlots, err := extractionJobRuntimeSlots(job)
	if err != nil {
		return false, err
	}
	if len(runtimeSlots) == 0 {
		return false, nil
	}
	values, err := r.memory.ReadUserMemory(ctx, job.WorkspaceID, job.AgentID, runtimeSlots, job.UserScope, job.UserID)
	if err != nil {
		return false, err
	}
	instructions := r.memoryInstructions(ctx, job)
	build := func(turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) (*adapter.ChatRequest, error) {
		return r.consolidationChatRequest(turns, slots, values, instructions)
	}
	turns, err = r.fitTurnBudget(turns, runtimeSlots, values, build)
	if err != nil {
		return false, err
	}
	if len(turns) == 0 {
		return false, nil
	}
	return r.consolidateAndApply(ctx, job, scope, turns, runtimeSlots, values, instructions)
}

func extractionJobRuntimeSlots(job *agentmemory.AgentMemoryExtractionJob) ([]agentmemory.RuntimeSlot, error) {
	if job == nil || len(job.RuntimeSlots) == 0 {
		return nil, fmt.Errorf("memory extraction job is missing its slot snapshot")
	}
	var stored []agentmemory.RuntimeSlot
	if err := json.Unmarshal(job.RuntimeSlots, &stored); err != nil {
		return nil, fmt.Errorf("decode memory extraction slot snapshot: %w", err)
	}
	runtimeSlots := make([]agentmemory.RuntimeSlot, 0, len(stored))
	seen := make(map[string]struct{}, len(stored))
	for _, slot := range stored {
		slot.Key = strings.ToLower(strings.TrimSpace(slot.Key))
		if !slot.Enabled || slot.Key == "" || slot.MaxChars <= 0 {
			continue
		}
		if _, ok := seen[slot.Key]; ok {
			continue
		}
		seen[slot.Key] = struct{}{}
		runtimeSlots = append(runtimeSlots, slot)
	}
	if len(runtimeSlots) == 0 {
		return nil, fmt.Errorf("memory extraction job has no enabled slot snapshot")
	}
	return runtimeSlots, nil
}

func stripJSONFence(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```JSON")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	return strings.TrimSpace(value)
}
func extractionErrorCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "model_timeout"
	}
	return "extraction_failed"
}

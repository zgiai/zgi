package agentmemoryworker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"gorm.io/gorm"
)

const (
	maxExtractionTurns          = 6
	maxExtractionInputTokens    = 8000
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
	memory    *agentmemory.Service
	llmClient llmclient.LLMClient
	estimator *tokenestimate.Estimator
}

func NewRunner(db *gorm.DB, memory *agentmemory.Service, client llmclient.LLMClient) *Runner {
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
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			WITH batch AS (
				SELECT id FROM agent_memory_events
				WHERE before_snapshot IS NOT NULL OR after_snapshot IS NOT NULL
				ORDER BY created_at ASC
				LIMIT 500
			)
			UPDATE agent_memory_events AS events
			SET before_snapshot = NULL, after_snapshot = NULL
			FROM batch WHERE events.id = batch.id
		`).Error; err != nil {
			return err
		}
		if err := tx.Where("expires_at <= ?", now).Delete(&agentmemory.AgentMemoryUndoRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("created_at < ?", now.Add(-auditRetention)).Delete(&agentmemory.AgentMemoryEvent{}).Error
	}); err != nil {
		return err
	}
	if _, err := r.memory.DeleteTerminalExtractionJobs(ctx, now.Add(-extractionJobRetention), extractionCleanupBatchSize); err != nil {
		return fmt.Errorf("delete expired agent memory extraction jobs: %w", err)
	}
	return nil
}

func globalAutomaticExtractionEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ZGI_AGENT_MEMORY_AUTO_EXTRACTION_ENABLED")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
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
	CompletedAt   time.Time
	ModelProvider string
	ModelName     string
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
	var subject agentmemory.AgentMemorySubjectState
	if err := r.db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND user_scope = ? AND user_id = ?", job.WorkspaceID, job.AgentID, job.UserScope, job.UserID).
		Take(&subject).Error; err != nil {
		return nil, scope, err
	}
	if subject.MemoryEpoch != job.MemoryEpoch {
		return nil, scope, agentmemory.ErrConflict
	}
	var watermark struct{ CreatedAt time.Time }
	if err := r.db.WithContext(ctx).Table("chat_runtime_messages").Select("created_at").
		Where("id = ? AND conversation_id = ? AND status = ? AND deleted_at IS NULL", job.MessageWatermarkID, job.ConversationID, "completed").Take(&watermark).Error; err != nil {
		return nil, scope, err
	}
	var previous struct{ CreatedAt *time.Time }
	if err := r.db.WithContext(ctx).Table("agent_memory_extraction_jobs AS jobs").
		Select("MAX(messages.created_at) AS created_at").
		Joins("JOIN chat_runtime_messages AS messages ON messages.id = jobs.message_watermark_id").
		Where("jobs.id <> ? AND jobs.workspace_id = ? AND jobs.agent_id = ? AND jobs.user_scope = ? AND jobs.user_id = ? AND jobs.conversation_id = ? AND jobs.memory_epoch = ? AND jobs.status = ? AND messages.created_at < ?",
			job.ID, job.WorkspaceID, job.AgentID, job.UserScope, job.UserID, job.ConversationID, job.MemoryEpoch, agentmemory.ExtractionJobCompleted, watermark.CreatedAt).
		Scan(&previous).Error; err != nil {
		return nil, scope, err
	}
	lowerBound := subject.ExtractionCutoffAt
	// During a rolling deploy, an older node may increment memory_epoch without
	// knowing the additive cutoff column. The epoch row's update time is still a
	// safe delete fence, so new workers do not resurrect pre-delete messages.
	if lowerBound == nil && subject.MemoryEpoch > 0 && !subject.UpdatedAt.IsZero() {
		value := subject.UpdatedAt
		lowerBound = &value
	}
	if previous.CreatedAt != nil && (lowerBound == nil || previous.CreatedAt.After(*lowerBound)) {
		value := *previous.CreatedAt
		lowerBound = &value
	}
	query := r.db.WithContext(ctx).Table("chat_runtime_messages").
		Select("id, query, updated_at AS completed_at, COALESCE(model_provider, '') AS model_provider, model_name").
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
	return turns, scope, nil
}

func (r *Runner) fitTurnBudget(turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) ([]completedTurn, error) {
	selected := make([]completedTurn, 0, len(turns))
	for i := len(turns) - 1; i >= 0; i-- {
		candidate := make([]completedTurn, 0, len(selected)+1)
		candidate = append(candidate, turns[i])
		candidate = append(candidate, selected...)
		request, err := r.extractionChatRequest(candidate, slots, values)
		if err != nil {
			return nil, err
		}
		if r.estimator.EstimateChatRequest(request).Tokens > maxExtractionInputTokens {
			break
		}
		selected = candidate
	}
	return selected, nil
}

type automaticOperation struct {
	Action     string   `json:"action"`
	Key        string   `json:"key"`
	Content    string   `json:"content"`
	Evidence   string   `json:"evidence"`
	Confidence *float64 `json:"confidence,omitempty"`
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
	turns, err = r.fitTurnBudget(turns, runtimeSlots, values)
	if err != nil {
		return false, err
	}
	if len(turns) == 0 {
		return false, nil
	}
	operations, err := r.decide(ctx, job, scope, turns, runtimeSlots, values)
	if err != nil {
		return false, err
	}
	current := map[string]agentmemory.SlotValueResponse{}
	for _, value := range values {
		current[value.Key] = value
	}
	type validatedOperation struct {
		operation automaticOperation
		slot      agentmemory.RuntimeSlot
		source    completedTurn
	}
	validated := make([]validatedOperation, 0, len(operations))
	turnIDs := make(map[string]struct{}, len(turns))
	for _, turn := range turns {
		turnIDs[turn.ID.String()] = struct{}{}
	}
	for _, operation := range operations {
		if operation.Action == "none" {
			continue
		}
		slot, ok := automaticSlot(runtimeSlots, operation.Key)
		if !ok {
			return false, fmt.Errorf("invalid automatic slot")
		}
		evidenceTurn, ok := evidenceSourceTurn(operation.Evidence, turns)
		if !ok {
			return false, fmt.Errorf("invalid automatic evidence")
		}
		content := strings.TrimSpace(operation.Content)
		if content == "" || len([]rune(content)) > slot.MaxChars || agentmemory.ContainsSensitiveContent(content) {
			return false, fmt.Errorf("invalid automatic content")
		}
		if value, exists := current[slot.Key]; exists {
			_, fromThisBatch := turnIDs[value.SourceMessageID]
			if fromThisBatch && (value.SourceKind == agentmemory.SourceKindExplicit || value.ExtractorVersion == "inline-agent-memory-v1") {
				continue
			}
		}
		operation.Content = content
		validated = append(validated, validatedOperation{operation: operation, slot: slot, source: *evidenceTurn})
	}
	mutations := make([]agentmemory.ValueMutation, 0, len(validated))
	for _, candidate := range validated {
		operation, slot := candidate.operation, candidate.slot
		expected := current[slot.Key].Revision
		operationID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(job.ID.String()+":"+slot.Key))
		sourceMessageID := candidate.source.ID
		sourceCompletedAt := candidate.source.CompletedAt
		mutations = append(mutations, agentmemory.ValueMutation{
			Action: agentmemory.MutationActionUpsert, Key: slot.Key, Content: operation.Content,
			Mode: agentmemory.MutationModeProactive, ExpectedRevision: expected, OperationID: operationID,
			SourceMessageID: &sourceMessageID, SourceCompletedAt: &sourceCompletedAt,
		})
	}
	if len(mutations) == 0 {
		return false, nil
	}
	_, err = r.memory.MutateValues(ctx, job.WorkspaceID, job.AgentID, runtimeSlots, job.UserScope, job.UserID, agentmemory.MutateValuesRequest{Operations: mutations}, agentmemory.MutationMetadata{
		ActorType: agentmemory.EventActorModel, Source: agentmemory.EventSourceAgent,
		SourceConversationID: &job.ConversationID, ExtractorVersion: job.ExtractorVersion, MemoryEpoch: &job.MemoryEpoch,
		ConfigScope: job.ConfigScope, ConfigRevision: job.ConfigRevision,
	})
	if errors.Is(err, agentmemory.ErrConflict) || (err != nil && errors.Is(errors.Unwrap(err), agentmemory.ErrConflict)) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
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

func (r *Runner) decide(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob, scope extractionScope, turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) ([]automaticOperation, error) {
	request, err := r.extractionChatRequest(turns, slots, values)
	if err != nil {
		return nil, err
	}
	resp, err := r.llmClient.AppChat(ctx, &llmclient.AppContext{
		OrganizationID: scope.OrganizationID.String(), WorkspaceID: scope.WorkspaceID.String(), BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID: job.AgentID.String(), AppType: "agent", AccountID: scope.AccountID.String(), SessionID: job.ConversationID.String(), ConversationID: job.ConversationID.String(), ModelUseCase: "agent-memory-extraction", SuppressInvocationContent: true,
	}, request)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty extraction response")
	}
	content, _ := resp.Choices[0].Message.Content.(string)
	return parseAutomaticOperations(content)
}

func (r *Runner) extractionChatRequest(turns []completedTurn, slots []agentmemory.RuntimeSlot, values []agentmemory.SlotValueResponse) (*adapter.ChatRequest, error) {
	if len(turns) == 0 {
		return nil, fmt.Errorf("no completed turns available for extraction")
	}
	userMessages := make([]map[string]string, 0, len(turns))
	for _, turn := range turns {
		userMessages = append(userMessages, map[string]string{"message_id": turn.ID.String(), "content": turn.Query})
	}
	payload := map[string]interface{}{"user_messages": userMessages, "slots": slots, "current_values": values}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode extraction prompt: %w", err)
	}
	system := strings.Join([]string{
		"You are a conservative background Agent-memory extractor.",
		"Only save stable facts, preferences, long-lived instructions, or explicit corrections directly stated by the user in user_messages.",
		"Never infer from assistant text, tools, behavior patterns, hypotheticals, temporary situations, third-party facts, or ambiguous ownership.",
		"Never output clear. Only enabled slots are present. Evidence must be an exact quote from one user message.",
		`Return JSON only: {"operations":[{"action":"upsert|none","key":"slot key","content":"complete merged value","evidence":"exact user quote","confidence":0.0}]}`,
		"Confidence is telemetry only. When uncertain use none.", string(raw),
	}, "\n")
	provider, model := turns[len(turns)-1].ModelProvider, turns[len(turns)-1].ModelName
	temperature, maxTokens := 0.0, maxExtractionOutputTokens
	return &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Messages: []adapter.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: "Extract durable user memory now. Return JSON only."},
		},
		Temperature:    &temperature,
		MaxTokens:      &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	}, nil
}

func parseAutomaticOperations(content string) ([]automaticOperation, error) {
	content = stripJSONFence(content)
	var output struct {
		Operations []automaticOperation `json:"operations"`
	}
	if err := json.Unmarshal([]byte(content), &output); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for i := range output.Operations {
		op := &output.Operations[i]
		switch strings.ToLower(strings.TrimSpace(op.Action)) {
		case "upsert", "update":
			op.Action = "upsert"
		case "none", "":
			op.Action = "none"
		default:
			return nil, fmt.Errorf("unsupported automatic action")
		}
		op.Key, op.Content, op.Evidence = strings.TrimSpace(op.Key), strings.TrimSpace(op.Content), strings.TrimSpace(op.Evidence)
		if op.Action == "none" {
			continue
		}
		if _, exists := seen[op.Key]; exists {
			return nil, fmt.Errorf("duplicate automatic slot")
		}
		seen[op.Key] = struct{}{}
	}
	return output.Operations, nil
}

func automaticSlot(slots []agentmemory.RuntimeSlot, key string) (agentmemory.RuntimeSlot, bool) {
	for _, slot := range slots {
		if slot.Enabled && slot.Key == strings.TrimSpace(key) {
			return slot, true
		}
	}
	return agentmemory.RuntimeSlot{}, false
}
func evidenceSourceTurn(evidence string, turns []completedTurn) (*completedTurn, bool) {
	evidence = strings.TrimSpace(evidence)
	if evidence == "" {
		return nil, false
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if strings.Contains(turns[i].Query, evidence) {
			return &turns[i], true
		}
	}
	return nil, false
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

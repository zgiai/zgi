package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/gorm"
)

var errTaskOperationMemorySource = errors.New("task operation is not a memory source")

const (
	defaultMemoryExtractionBatch       = 100
	maxMemoryExtractionAttempts        = 5
	memoryExtractionRetryBaseDelay     = time.Minute
	memoryExtractionRetryMaximumDelay  = 30 * time.Minute
	memoryExtractionJobRetention       = 30 * 24 * time.Hour
	memoryExtractionMaxProposals       = 8
	memoryExtractionMaxSourceRunes     = 12_000
	memoryExtractionMaxCurrentMemories = 20
	memoryExtractionMaxMemoryRunes     = 1_000
	memoryExtractionMaxOutputTokens    = 1_600
)

// MemoryExtractionSource is loaded transiently from durable run and chat
// references. It is never copied into a memory job row.
type MemoryExtractionSource struct {
	AccountID     uuid.UUID
	Query         string
	Answer        string
	ModelProvider string
	ModelName     string
	CompletedAt   time.Time
}

// MemoryExtractionInput contains the bounded evidence available to an
// asynchronous DigitalHuman memory extractor.
type MemoryExtractionInput struct {
	Job             *MemoryExtractionJob
	Run             *Run
	Event           invocation.Event
	Source          MemoryExtractionSource
	CurrentMemories []*DigitalHumanMemory
}

// MemoryExtractionDraft is an untrusted model proposal. The regular memory
// policy and fence checks remain authoritative when it is applied.
// Convergence: drafts carry no memory class or business scope; every accepted
// proposal is stored as self memory for the owning DigitalHuman.
type MemoryExtractionDraft struct {
	Key                        string                   `json:"key"`
	MemoryType                 memoryruntime.MemoryType `json:"memory_type"`
	Content                    string                   `json:"content"`
	Confidence                 float64                  `json:"confidence,omitempty"`
	SupersedesID               *uuid.UUID               `json:"supersedes_id,omitempty"`
	ExpectedSupersededRevision int64                    `json:"expected_superseded_revision,omitempty"`
}

// MemoryExtractor proposes memory changes but cannot commit them directly.
type MemoryExtractor interface {
	Extract(context.Context, MemoryExtractionInput) ([]MemoryExtractionDraft, error)
}

// LLMMemoryExtractor uses the run's already-resolved model and suppresses
// prompt/response persistence in invocation-content telemetry.
type LLMMemoryExtractor struct {
	client llmclient.LLMClient
}

func NewLLMMemoryExtractor(client llmclient.LLMClient) *LLMMemoryExtractor {
	if client == nil {
		panic("DigitalHuman memory extractor requires LLM client")
	}
	return &LLMMemoryExtractor{client: client}
}

func (e *LLMMemoryExtractor) Extract(ctx context.Context, input MemoryExtractionInput) ([]MemoryExtractionDraft, error) {
	if input.Job == nil || input.Run == nil || input.Source.AccountID == uuid.Nil || input.Source.ModelName == "" {
		return nil, ErrInvalidRequest
	}
	prompt, err := memoryExtractionPrompt(input)
	if err != nil {
		return nil, err
	}
	temperature, maxTokens := 0.0, memoryExtractionMaxOutputTokens
	response, err := e.client.AppChat(ctx, &llmclient.AppContext{
		OrganizationID: input.Run.OrganizationID.String(), WorkspaceID: input.Run.WorkspaceID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              input.Run.DigitalHumanID.String(), AppType: runtimemodel.ConversationCallerDigitalHuman,
		AccountID: input.Source.AccountID.String(), SessionID: input.Job.ConversationID.String(),
		ConversationID: input.Job.ConversationID.String(), ModelUseCase: "agent-memory-extraction",
		SuppressInvocationContent: true,
	}, &adapter.ChatRequest{
		Provider: input.Source.ModelProvider, Model: input.Source.ModelName,
		Messages: []adapter.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: "Propose durable DigitalHuman memory now. Return JSON only."},
		},
		Temperature: &temperature, MaxTokens: &maxTokens,
		ResponseFormat: &adapter.ResponseFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, err
	}
	if response == nil || len(response.Choices) == 0 {
		return nil, errors.New("digital human memory extractor returned no choices")
	}
	content, ok := response.Choices[0].Message.Content.(string)
	if !ok {
		return nil, errors.New("digital human memory extractor returned non-text content")
	}
	return parseMemoryExtractionDrafts(content)
}

// MemoryExtractionRunner claims, evaluates, and commits reference-only jobs.
type MemoryExtractionRunner struct {
	repository *GormRepository
	service    *Service
	extractor  MemoryExtractor
	now        func() time.Time
}

func NewMemoryExtractionRunner(repository *GormRepository, service *Service, extractor MemoryExtractor) *MemoryExtractionRunner {
	if repository == nil || service == nil || extractor == nil {
		panic("DigitalHuman memory extraction runner requires repository, service, and extractor")
	}
	return &MemoryExtractionRunner{
		repository: repository, service: service, extractor: extractor,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (r *MemoryExtractionRunner) ProcessDue(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = defaultMemoryExtractionBatch
	}
	if _, err := r.repository.DeleteTerminalMemoryExtractionJobs(ctx, r.now().Add(-memoryExtractionJobRetention), 500); err != nil {
		return fmt.Errorf("clean DigitalHuman memory extraction jobs: %w", err)
	}
	jobs, err := r.repository.ListDueMemoryExtractionJobs(ctx, r.now(), limit)
	if err != nil {
		return fmt.Errorf("list due DigitalHuman memory extraction jobs: %w", err)
	}
	var combined error
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if err := r.RunJob(ctx, job.ID); err != nil {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func (r *MemoryExtractionRunner) RunJob(ctx context.Context, jobID uuid.UUID) error {
	job, err := r.claim(ctx, jobID)
	if err != nil || job == nil {
		return err
	}
	status, errorCode := MemoryExtractionJobSucceeded, ""
	var retryAt *time.Time
	defer func() {
		finishCtx := context.WithoutCancel(ctx)
		now := r.now()
		if retryAt != nil {
			_ = r.repository.RescheduleMemoryExtractionJob(finishCtx, job.ID, errorCode, *retryAt, now)
			return
		}
		_ = r.repository.FinishMemoryExtractionJob(finishCtx, job.ID, status, errorCode, now)
	}()
	fail := func(code string) {
		errorCode = code
		if job.AttemptCount >= maxMemoryExtractionAttempts {
			status = MemoryExtractionJobCanceled
			errorCode = "attempts_exhausted:" + code
			return
		}
		status = MemoryExtractionJobFailed
		next := r.now().Add(memoryExtractionRetryDelay(job.AttemptCount))
		retryAt = &next
	}

	run, event, source, err := r.loadSource(ctx, job)
	if err != nil {
		if errors.Is(err, errTaskOperationMemorySource) {
			status, errorCode = MemoryExtractionJobCanceled, "task_operation_not_memory"
			return nil
		}
		// Extraction jobs intentionally keep references instead of copying chat
		// content. A user may remove the referenced conversation or message before
		// the delayed worker runs. That source cannot reappear, so retrying only
		// produces recurring scheduler errors; cancel the stale job terminally.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, errorCode = MemoryExtractionJobCanceled, "source_not_found"
			return nil
		}
		fail(memoryExtractionErrorCode(err))
		return err
	}
	if run.Status != RunStatusSucceeded {
		switch run.Status {
		case RunStatusQueued, RunStatusRunning, RunStatusCanceling:
			fail("run_not_complete")
			return ErrRunConflict
		default:
			status, errorCode = MemoryExtractionJobCanceled, "run_not_successful"
			return nil
		}
	}
	// The claim already verified automatic extraction for this epoch.
	target := admitMemoryWrite(run, event, memoryWriteBackground, true)
	if !target.Allowed {
		r.recordExtractionLedger(ctx, job, "rejected", map[string]any{"reason": target.Reason, "source_trust": target.Trust})
		return nil
	}
	scoped := memoryReadScopeForRun(run, event)
	readScope := &scoped
	scope := Scope{OrganizationID: job.OrganizationID, WorkspaceID: job.WorkspaceID, ActorID: source.AccountID}
	current, err := r.service.retrieveMemories(ctx, scope, job.DigitalHumanID, 50, readScope)
	if err != nil {
		fail("retrieve_current_memory")
		return err
	}
	drafts, err := r.extractor.Extract(ctx, MemoryExtractionInput{
		Job: job, Run: run, Event: event, Source: source,
		CurrentMemories: append([]*DigitalHumanMemory(nil), current...),
	})
	if err != nil {
		fail(memoryExtractionErrorCode(err))
		return err
	}
	committed, rejected, err := r.applyDrafts(ctx, scope, job, run, source, current, drafts, target)
	if err != nil {
		fail(memoryExtractionErrorCode(err))
		return err
	}
	r.recordExtractionLedger(ctx, job, "completed", map[string]any{"committed": committed, "rejected": rejected})
	return nil
}

func (r *MemoryExtractionRunner) claim(ctx context.Context, jobID uuid.UUID) (*MemoryExtractionJob, error) {
	var claimed *MemoryExtractionJob
	err := r.repository.Transaction(ctx, func(tx *GormRepository) error {
		job, err := tx.MemoryExtractionJobForUpdate(ctx, jobID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if job.Status != MemoryExtractionJobPending && job.Status != MemoryExtractionJobFailed {
			return nil
		}
		state, err := tx.MemoryStateForUpdate(ctx, Scope{OrganizationID: job.OrganizationID, WorkspaceID: job.WorkspaceID}, job.DigitalHumanID)
		if err != nil {
			return err
		}
		if !state.AutoExtractionEnabled || state.ConfigRevision != job.ConfigRevision || state.MemoryEpoch != job.MemoryEpoch {
			return tx.CancelMemoryExtractionJob(ctx, job.ID, "memory_fence_changed", r.now())
		}
		claimed, err = tx.ClaimMemoryExtractionJob(ctx, job.ID, job.MemoryEpoch, r.now())
		if errors.Is(err, ErrMemoryConflict) {
			return nil
		}
		return err
	})
	return claimed, err
}

func (r *MemoryExtractionRunner) loadSource(ctx context.Context, job *MemoryExtractionJob) (*Run, invocation.Event, MemoryExtractionSource, error) {
	if job == nil || job.RunID == nil || job.ConversationID == nil || job.MessageID == nil {
		return nil, invocation.Event{}, MemoryExtractionSource{}, ErrInvalidRequest
	}
	scope := Scope{OrganizationID: job.OrganizationID, WorkspaceID: job.WorkspaceID}
	run, err := r.repository.RunByID(ctx, scope, job.DigitalHumanID, *job.RunID)
	if err != nil {
		return nil, invocation.Event{}, MemoryExtractionSource{}, err
	}
	var event invocation.Event
	if err := json.Unmarshal(run.InvocationEvent, &event); err != nil {
		return nil, invocation.Event{}, MemoryExtractionSource{}, fmt.Errorf("decode extraction invocation: %w", err)
	}
	event, err = invocation.ValidateEvent(event)
	if err != nil {
		return nil, invocation.Event{}, MemoryExtractionSource{}, err
	}
	var source MemoryExtractionSource
	err = r.repository.db.WithContext(ctx).Table("chat_runtime_messages AS messages").
		Select("conversations.account_id, messages.query, messages.answer, COALESCE(messages.model_provider, '') AS model_provider, messages.model_name, messages.updated_at AS completed_at").
		Joins("JOIN chat_runtime_conversations AS conversations ON conversations.id = messages.conversation_id").
		Where(`messages.id = ? AND messages.conversation_id = ? AND messages.source_message_id = ?
			AND messages.status = ? AND messages.deleted_at IS NULL
			AND conversations.organization_id = ? AND conversations.workspace_id = ?
			AND conversations.caller_type = ? AND conversations.caller_id = ? AND conversations.deleted_at IS NULL`,
			*job.MessageID, *job.ConversationID, run.ID, runtimemodel.MessageStatusCompleted,
			job.OrganizationID, job.WorkspaceID, runtimemodel.ConversationCallerDigitalHuman, job.DigitalHumanID,
		).Take(&source).Error
	if err != nil {
		return nil, invocation.Event{}, MemoryExtractionSource{}, fmt.Errorf("load DigitalHuman memory extraction source: %w", err)
	}
	if source.ModelName == "" {
		var snapshot ExecutionSnapshot
		if json.Unmarshal(run.ExecutionSnapshot, &snapshot) == nil {
			source.ModelName = snapshot.Model
		}
	}
	// Task proposals and lifecycle mutations are operational state. They must
	// never be promoted into the DigitalHuman's long-term self memory, even if
	// the model also produced a successful conversational answer.
	if bytes.Contains(run.ExecutionTrace, []byte("manage_digital_human_task")) {
		return nil, invocation.Event{}, MemoryExtractionSource{}, errTaskOperationMemorySource
	}
	return run, event, source, nil
}

func (r *MemoryExtractionRunner) applyDrafts(
	ctx context.Context,
	scope Scope,
	job *MemoryExtractionJob,
	run *Run,
	source MemoryExtractionSource,
	current []*DigitalHumanMemory,
	drafts []MemoryExtractionDraft,
	target memoryWriteAdmission,
) (int, int, error) {
	if len(drafts) > memoryExtractionMaxProposals {
		return 0, 0, ErrMemoryPolicyRejected
	}
	// Only memories in the target class and scope can be revised or
	// superseded by this job; other readable memories only inform the model.
	currentByKey := make(map[string]*DigitalHumanMemory, len(current))
	currentByID := make(map[uuid.UUID]struct{}, len(current))
	for _, item := range current {
		if item != nil && memoryInWriteTarget(item, target) {
			currentByKey[memoryIdentity(item.MemoryKey)] = item
			currentByID[item.ID] = struct{}{}
		}
	}
	committed, rejected := 0, 0
	for _, draft := range drafts {
		draft.Key = strings.ToLower(strings.TrimSpace(draft.Key))
		draft.Content = strings.TrimSpace(draft.Content)
		identity := memoryIdentity(draft.Key)
		expectedRevision := int64(0)
		if existing := currentByKey[identity]; existing != nil {
			expectedRevision = existing.Revision
		}
		operationID := uuid.NewSHA1(job.ID, []byte(identity))
		if draft.SupersedesID != nil {
			if _, ok := currentByID[*draft.SupersedesID]; !ok {
				draft.SupersedesID, draft.ExpectedSupersededRevision = nil, 0
			}
		}
		request := MemoryProposalRequest{
			OperationID: operationID, Key: draft.Key, Action: memoryruntime.MutationActionUpsert,
			MemoryType: draft.MemoryType, Content: draft.Content,
			ExpectedRevision: expectedRevision, ConfigRevision: job.ConfigRevision, MemoryEpoch: job.MemoryEpoch,
			SupersedesID: draft.SupersedesID, ExpectedSupersededRevision: draft.ExpectedSupersededRevision,
			Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeRun, ID: run.ID.String(), Digest: run.RunSpecRevision}},
			Confidence: draft.Confidence, RunID: &run.ID,
			SourceContext: MemorySourceContext{PrivateConversation: false, SourceTrust: target.Trust}, OccurredAt: source.CompletedAt,
		}
		target.stamp(&request)
		request.noteSourceKind = relationshipmemory.SourceAutomatic
		result, err := r.service.ApplyAutonomousMemoryProposal(ctx, scope, job.DigitalHumanID, request)
		if err != nil {
			if errors.Is(err, ErrMemoryPolicyRejected) || errors.Is(err, ErrInvalidRequest) {
				r.service.recordRejectedMemoryProposal(ctx, scope, job.DigitalHumanID, request, err)
				rejected++
				continue
			}
			return committed, rejected, err
		}
		if result != nil && result.Memory != nil {
			currentByKey[identity] = result.Memory
		}
		committed++
	}
	return committed, rejected, nil
}

func (r *MemoryExtractionRunner) recordExtractionLedger(ctx context.Context, job *MemoryExtractionJob, status string, values map[string]any) {
	if job == nil {
		return
	}
	detail := map[string]any{"job_id": job.ID, "attempt_count": job.AttemptCount}
	for key, value := range values {
		detail[key] = value
	}
	_ = r.repository.CreateActionLedger(context.WithoutCancel(ctx), &ActionLedgerEntry{
		ID: uuid.New(), OrganizationID: job.OrganizationID, WorkspaceID: job.WorkspaceID,
		DigitalHumanID: job.DigitalHumanID, RunID: job.RunID, ActionType: "memory.extraction",
		Status: status, Detail: mustJSON(detail), CreatedAt: r.now(),
	})
}

func memoryExtractionPrompt(input MemoryExtractionInput) (string, error) {
	current := make([]map[string]any, 0, min(len(input.CurrentMemories), memoryExtractionMaxCurrentMemories))
	for _, item := range input.CurrentMemories {
		if item == nil || len(current) == memoryExtractionMaxCurrentMemories {
			continue
		}
		current = append(current, map[string]any{
			"id": item.ID, "key": item.MemoryKey, "memory_type": item.MemoryType,
			"content": truncateRunes(item.Content, memoryExtractionMaxMemoryRunes), "revision": item.Revision,
		})
	}
	payload := map[string]any{
		"run_id": input.Run.ID, "query": truncateRunes(input.Source.Query, memoryExtractionMaxSourceRunes/2),
		"answer":         truncateRunes(input.Source.Answer, memoryExtractionMaxSourceRunes/2),
		"current_memory": current,
		"artifact_refs":  json.RawMessage(input.Job.ArtifactRefs),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode DigitalHuman memory extraction prompt: %w", err)
	}
	return strings.Join([]string{
		"You are a conservative background memory proposer for one DigitalHuman.",
		"Propose only durable work experience, decisions, open loops, reflections, or evidence-backed facts.",
		"Treat an explicit durable working directive, such as a future role, recurring responsibility, or default behavior, as a decision. It does not change the published identity, persona, or rules.",
		"Never propose identity changes, user profiles, personal preferences, credentials, tokens, cookies, document bodies, artifact bodies, full business objects, or temporary chat details.",
		"All memory is self memory for this DigitalHuman; do not emit class or business scope fields.",
		"To replace an active memory with a new semantic key, copy its id and revision from current_memory into supersedes_id and expected_superseded_revision. Never invent a memory id.",
		"Use concise redacted summaries. Do not propose clear/delete operations. Use none when uncertain or when current memory already captures the same fact.",
		`Return JSON only: {"proposals":[{"key":"stable-semantic-key","memory_type":"experience|decision|fact|open_loop|reflection","content":"concise durable summary","confidence":0.0,"supersedes_id":"optional UUID copied from current_memory","expected_superseded_revision":0}]}`,
		string(raw),
	}, "\n"), nil
}

func parseMemoryExtractionDrafts(content string) ([]MemoryExtractionDraft, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```JSON")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	var output struct {
		Proposals []MemoryExtractionDraft `json:"proposals"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &output); err != nil {
		return nil, err
	}
	if len(output.Proposals) > memoryExtractionMaxProposals {
		return nil, ErrMemoryPolicyRejected
	}
	seen := make(map[string]struct{}, len(output.Proposals))
	for i := range output.Proposals {
		draft := &output.Proposals[i]
		draft.Key = strings.ToLower(strings.TrimSpace(draft.Key))
		draft.MemoryType = memoryruntime.MemoryType(strings.ToLower(strings.TrimSpace(string(draft.MemoryType))))
		draft.Content = strings.TrimSpace(draft.Content)
		if draft.Confidence == 0 {
			draft.Confidence = 0.75
		}
		key := memoryIdentity(draft.Key)
		if _, ok := seen[key]; ok {
			return nil, ErrMemoryPolicyRejected
		}
		seen[key] = struct{}{}
	}
	return output.Proposals, nil
}

// memoryInWriteTarget reports whether an existing memory lives in the class
// and scope a write admission targets.
func memoryInWriteTarget(memory *DigitalHumanMemory, target memoryWriteAdmission) bool {
	if target.NoteInstanceID != "" || memory.NoteInstanceID != "" {
		return memory.NoteInstanceID == target.NoteInstanceID
	}
	return !target.InternalOnly || memory.Visibility == MemoryVisibilityInternal
}

// memoryIdentity keys converged self memory by its semantic key alone; class
// and business scope no longer partition the store.
func memoryIdentity(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func memoryExtractionRetryDelay(attempt int) time.Duration {
	delay := memoryExtractionRetryBaseDelay
	for current := 1; current < attempt && delay < memoryExtractionRetryMaximumDelay; current++ {
		delay *= 2
		if delay >= memoryExtractionRetryMaximumDelay {
			return memoryExtractionRetryMaximumDelay
		}
	}
	return delay
}

func memoryExtractionErrorCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "model_timeout"
	case errors.Is(err, ErrMemoryConflict), errors.Is(err, ErrRunConflict):
		return "revision_conflict"
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "source_not_found"
	default:
		return "extraction_failed"
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

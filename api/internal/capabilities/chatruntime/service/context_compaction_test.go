package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

type contextCompactionMessageRepo struct {
	repository.MessageRepository
	metadata     map[string]interface{}
	err          error
	updateErrors []error
	updates      int
	messages     map[uuid.UUID]*runtimemodel.Message
}

func (r *contextCompactionMessageRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updates++
	if r.updates <= len(r.updateErrors) && r.updateErrors[r.updates-1] != nil {
		return r.updateErrors[r.updates-1]
	}
	if r.err != nil {
		return r.err
	}
	r.metadata = copyStringAnyMap(metadata)
	return nil
}

func (r *contextCompactionMessageRepo) GetScoped(_ context.Context, id, _, _ uuid.UUID) (*runtimemodel.Message, error) {
	message := r.messages[id]
	if message == nil {
		return nil, errors.New("message not found")
	}
	return message, nil
}

func (r *contextCompactionMessageRepo) ListBranchPage(_ context.Context, conversationID, leafID uuid.UUID, stopExclusive *uuid.UUID, _ int) (*repository.MessageBranchPage, error) {
	if stopExclusive != nil && leafID == *stopExclusive {
		return &repository.MessageBranchPage{ReachedBoundary: true}, nil
	}
	message := r.messages[leafID]
	if message == nil || message.ConversationID != conversationID {
		return nil, errors.New("message not found")
	}
	page := &repository.MessageBranchPage{Messages: []*runtimemodel.Message{message}}
	if message.ParentID == nil {
		page.ReachedRoot = true
	} else if stopExclusive != nil && *message.ParentID == *stopExclusive {
		page.ReachedBoundary = true
	} else {
		page.NextLeafID = message.ParentID
	}
	return page, nil
}

type contextCompactionLLM struct {
	llmclient.LLMClient
	responses   []*adapter.ChatResponse
	errors      []error
	requests    []*adapter.ChatRequest
	streamCalls int
}

func (f *contextCompactionLLM) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	f.streamCalls++
	return nil, errors.New("unexpected main model call")
}

type contextCompactionModelSpecResolver struct{ spec ModelSpec }

func (r contextCompactionModelSpecResolver) Resolve(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
	return r.spec, true, nil
}

func (f *contextCompactionLLM) AppChat(_ context.Context, _ *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.requests = append(f.requests, cloneChatRequest(req))
	index := len(f.requests) - 1
	if index < len(f.errors) && f.errors[index] != nil {
		return nil, f.errors[index]
	}
	if index >= len(f.responses) {
		return nil, errors.New("missing test response")
	}
	return f.responses[index], nil
}

func TestContextCompactionMetadataRoundTrip(t *testing.T) {
	ownerID := uuid.New()
	boundaryID := uuid.New()
	input := &contextCompactionMetadata{
		Status: contextCompactionStatusSucceeded,
		Snapshot: &contextSnapshot{
			Version:                 contextCompactionSnapshotVersion,
			Summary:                 validContextSummary("remember the constraint"),
			CoveredThroughMessageID: boundaryID,
			SummaryTokens:           80,
			SourceTurnCount:         12,
			CreatedAt:               time.Now().UTC(),
		},
		SnapshotRef: &contextSnapshotRef{
			OwnerMessageID:          ownerID,
			CoveredThroughMessageID: boundaryID,
			Version:                 contextCompactionSnapshotVersion,
		},
	}
	messageMetadata := map[string]interface{}{
		"context_control": putContextCompactionMetadata(nil, input),
	}
	decoded, err := decodeContextCompactionMetadata(messageMetadata)
	if err != nil {
		t.Fatalf("decodeContextCompactionMetadata() error = %v", err)
	}
	if decoded == nil || decoded.Snapshot == nil || decoded.SnapshotRef == nil {
		t.Fatalf("decoded metadata = %#v", decoded)
	}
	if decoded.Snapshot.Summary != input.Snapshot.Summary || decoded.SnapshotRef.OwnerMessageID != ownerID {
		t.Fatalf("decoded metadata = %#v, want round trip", decoded)
	}
}

func TestLoadContextStateInheritsOnlyAncestorSnapshotReference(t *testing.T) {
	conversationID := uuid.New()
	boundaryID := uuid.New()
	ownerID := uuid.New()
	childID := uuid.New()
	boundary := &runtimemodel.Message{ID: boundaryID, ConversationID: conversationID}
	owner := &runtimemodel.Message{ID: ownerID, ConversationID: conversationID, ParentID: &boundaryID}
	child := &runtimemodel.Message{ID: childID, ConversationID: conversationID, ParentID: &ownerID}
	snapshot := &contextSnapshot{
		Version:                 contextCompactionSnapshotVersion,
		Summary:                 validContextSummary("ancestor snapshot"),
		CoveredThroughMessageID: boundaryID,
		SummaryTokens:           80,
		SourceTurnCount:         1,
		CreatedAt:               time.Now().UTC(),
	}
	ref := &contextSnapshotRef{OwnerMessageID: ownerID, CoveredThroughMessageID: boundaryID, Version: contextCompactionSnapshotVersion}
	owner.Metadata = map[string]interface{}{"context_control": putContextCompactionMetadata(nil, &contextCompactionMetadata{Status: contextCompactionStatusSucceeded, Snapshot: snapshot, SnapshotRef: ref})}
	child.Metadata = map[string]interface{}{"context_control": putContextCompactionMetadata(nil, &contextCompactionMetadata{Status: contextCompactionStatusNotRequired, SnapshotRef: ref})}
	repo := &contextCompactionMessageRepo{messages: map[uuid.UUID]*runtimemodel.Message{
		boundaryID: boundary,
		ownerID:    owner,
		childID:    child,
	}}
	svc := &service{repos: &repository.Repositories{Message: repo}}
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}

	state, err := svc.loadContextState(context.Background(), scope, conversationID, &childID)
	if err != nil {
		t.Fatalf("loadContextState() error = %v", err)
	}
	if state.Snapshot == nil || state.SnapshotRef == nil || state.SnapshotRef.OwnerMessageID != ownerID {
		t.Fatalf("loaded snapshot = %#v ref = %#v", state.Snapshot, state.SnapshotRef)
	}
	if len(state.RawMessages) != 2 || state.RawMessages[0].ID != ownerID || state.RawMessages[1].ID != childID {
		t.Fatalf("raw suffix = %#v, want owner then child", state.RawMessages)
	}
}

func TestLoadContextStateRebuildsRawHistoryForInvalidFutureBoundary(t *testing.T) {
	conversationID := uuid.New()
	rootID := uuid.New()
	ownerID := uuid.New()
	childID := uuid.New()
	root := &runtimemodel.Message{ID: rootID, ConversationID: conversationID}
	owner := &runtimemodel.Message{ID: ownerID, ConversationID: conversationID, ParentID: &rootID}
	child := &runtimemodel.Message{ID: childID, ConversationID: conversationID, ParentID: &ownerID}
	invalidSnapshot := &contextSnapshot{
		Version:                 contextCompactionSnapshotVersion,
		Summary:                 validContextSummary("invalid future boundary"),
		CoveredThroughMessageID: childID,
		SummaryTokens:           80,
		SourceTurnCount:         1,
		CreatedAt:               time.Now().UTC(),
	}
	invalidRef := &contextSnapshotRef{OwnerMessageID: ownerID, CoveredThroughMessageID: childID, Version: contextCompactionSnapshotVersion}
	owner.Metadata = map[string]interface{}{"context_control": putContextCompactionMetadata(nil, &contextCompactionMetadata{Status: contextCompactionStatusSucceeded, Snapshot: invalidSnapshot, SnapshotRef: invalidRef})}
	child.Metadata = map[string]interface{}{"context_control": putContextCompactionMetadata(nil, &contextCompactionMetadata{Status: contextCompactionStatusNotRequired, SnapshotRef: invalidRef})}
	repo := &contextCompactionMessageRepo{messages: map[uuid.UUID]*runtimemodel.Message{
		rootID:  root,
		ownerID: owner,
		childID: child,
	}}
	svc := &service{repos: &repository.Repositories{Message: repo}}

	state, err := svc.loadContextState(context.Background(), Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}, conversationID, &childID)
	if err != nil {
		t.Fatalf("loadContextState() error = %v", err)
	}
	if state.Snapshot != nil || state.SnapshotRef != nil {
		t.Fatalf("invalid snapshot should not be used: %#v", state)
	}
	if len(state.RawMessages) != 3 || state.RawMessages[0].ID != rootID || state.RawMessages[2].ID != childID {
		t.Fatalf("raw rebuild = %#v, want complete root-to-child history", state.RawMessages)
	}
}

func TestCompactContextForRunPersistsBeforeCompletingProgress(t *testing.T) {
	repo := &contextCompactionMessageRepo{}
	llm := &contextCompactionLLM{responses: []*adapter.ChatResponse{contextSummaryResponse(validContextSummary("keep project alpha"))}}
	svc := &service{
		repos:          &repository.Repositories{Message: repo},
		llmClient:      llm,
		tokenEstimator: tokenestimate.NewEstimator(),
	}
	conversationID := uuid.New()
	messageID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: uuid.New(), AccountID: uuid.New()},
		Message:      &runtimemodel.Message{ID: messageID, ConversationID: conversationID, Metadata: map[string]interface{}{}},
		parts: &chatRequestParts{
			Query:      "continue the project",
			Provider:   "test-provider",
			ModelName:  "test-model",
			Parameters: map[string]interface{}{},
		},
	}
	raw := make([]*runtimemodel.Message, 0, 10)
	for index := 0; index < 10; index++ {
		raw = append(raw, &runtimemodel.Message{
			ID:             uuid.New(),
			ConversationID: conversationID,
			Query:          "user request",
			Answer:         "assistant response",
			Status:         runtimemodel.MessageStatusCompleted,
		})
	}
	initial := &contextBudgetResult{
		Metadata:            map[string]interface{}{"prompt_budget": 8000},
		Budget:              &budgetComputation{PromptBudget: 8000, Tokenizer: "test"},
		HistoryTokens:       7000,
		HistoryBudgetTokens: 8000,
		HistoryPressure:     0.875,
		RawMessages:         raw,
		Spec:                ModelSpec{ContextWindow: 16000, MaxOutputTokens: 4096},
		SystemPrompt:        "system",
	}
	statuses := []string{}
	result, err := svc.compactContextForRun(context.Background(), prepared, initial, func(event StreamEvent) error {
		if event.EventType == streamEventAgentProgress {
			statuses = append(statuses, stringFromAny(event.Payload["status"]))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("compactContextForRun() error = %v", err)
	}
	if result == nil || result.HistoryPressure >= contextCompactionTriggerPressure {
		t.Fatalf("result pressure = %v, want safe result", result)
	}
	if len(llm.requests) != 1 {
		t.Fatalf("AppChat calls = %d, want 1", len(llm.requests))
	}
	if repo.updates < 2 || repo.metadata == nil {
		t.Fatalf("metadata updates = %d, metadata = %#v", repo.updates, repo.metadata)
	}
	if len(statuses) != 2 || statuses[0] != "running" || statuses[1] != "completed" {
		t.Fatalf("progress statuses = %#v", statuses)
	}
	metadata, err := decodeContextCompactionMetadata(repo.metadata)
	if err != nil || metadata == nil || metadata.Snapshot == nil || metadata.SnapshotRef == nil {
		t.Fatalf("persisted compaction metadata = %#v, err = %v", metadata, err)
	}
	if metadata.SnapshotRef.OwnerMessageID != messageID {
		t.Fatalf("snapshot owner = %s, want %s", metadata.SnapshotRef.OwnerMessageID, messageID)
	}
}

func TestCompactContextForRunBlocksAfterThreeFailures(t *testing.T) {
	originalDelays := contextCompactionRetryDelays
	contextCompactionRetryDelays = []time.Duration{0, 0}
	defer func() { contextCompactionRetryDelays = originalDelays }()

	repo := &contextCompactionMessageRepo{}
	llm := &contextCompactionLLM{errors: []error{errors.New("upstream unavailable"), errors.New("upstream unavailable"), errors.New("upstream unavailable")}}
	svc := &service{
		repos:          &repository.Repositories{Message: repo},
		llmClient:      llm,
		tokenEstimator: tokenestimate.NewEstimator(),
	}
	conversationID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: uuid.New(), AccountID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Metadata: map[string]interface{}{}},
		parts:        &chatRequestParts{Query: "continue", Provider: "test-provider", ModelName: "test-model", Parameters: map[string]interface{}{}},
	}
	raw := []*runtimemodel.Message{{ID: uuid.New(), ConversationID: conversationID, Query: "old", Answer: "answer", Status: runtimemodel.MessageStatusCompleted}}
	initial := &contextBudgetResult{
		Metadata:            map[string]interface{}{},
		Budget:              &budgetComputation{PromptBudget: 8000, Tokenizer: "test"},
		HistoryTokens:       7000,
		HistoryBudgetTokens: 8000,
		HistoryPressure:     0.875,
		RawMessages:         raw,
		Spec:                ModelSpec{ContextWindow: 16000},
		SystemPrompt:        "system",
	}
	statuses := []string{}
	_, err := svc.compactContextForRun(context.Background(), prepared, initial, func(event StreamEvent) error {
		if event.EventType == streamEventAgentProgress {
			statuses = append(statuses, stringFromAny(event.Payload["status"]))
		}
		return nil
	})
	if !apperror.IsCode(err, AppCodeContextCompactionUnavailable) {
		t.Fatalf("error = %v, want compaction unavailable", err)
	}
	if len(llm.requests) != contextCompactionMaxAttempts {
		t.Fatalf("AppChat calls = %d, want %d", len(llm.requests), contextCompactionMaxAttempts)
	}
	if len(statuses) != 1 || statuses[0] != "running" {
		t.Fatalf("progress statuses = %#v, want running only", statuses)
	}
	metadata, decodeErr := decodeContextCompactionMetadata(repo.metadata)
	if decodeErr != nil || metadata == nil || metadata.Status != contextCompactionStatusFailedBlocked {
		t.Fatalf("failure metadata = %#v, err = %v", metadata, decodeErr)
	}
}

func TestCompactContextForRunSucceedsOnThirdModelAttempt(t *testing.T) {
	originalDelays := contextCompactionRetryDelays
	contextCompactionRetryDelays = []time.Duration{0, 0}
	defer func() { contextCompactionRetryDelays = originalDelays }()

	repo := &contextCompactionMessageRepo{}
	llm := &contextCompactionLLM{
		errors:    []error{errors.New("first failure"), errors.New("second failure"), nil},
		responses: []*adapter.ChatResponse{nil, nil, contextSummaryResponse(validContextSummary("third attempt succeeds"))},
	}
	svc := &service{repos: &repository.Repositories{Message: repo}, llmClient: llm, tokenEstimator: tokenestimate.NewEstimator()}
	conversationID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: uuid.New(), AccountID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Metadata: map[string]interface{}{}},
		parts:        &chatRequestParts{Query: "continue", Provider: "test-provider", ModelName: "test-model", Parameters: map[string]interface{}{}},
	}
	raw := make([]*runtimemodel.Message, 10)
	for index := range raw {
		raw[index] = &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Query: "old", Answer: "answer", Status: runtimemodel.MessageStatusCompleted}
	}
	initial := &contextBudgetResult{
		Metadata:            map[string]interface{}{},
		Budget:              &budgetComputation{PromptBudget: 8000, Tokenizer: "test"},
		HistoryTokens:       7000,
		HistoryBudgetTokens: 8000,
		HistoryPressure:     0.875,
		RawMessages:         raw,
		Spec:                ModelSpec{ContextWindow: 16000, MaxOutputTokens: 4096},
		SystemPrompt:        "system",
	}
	result, err := svc.compactContextForRun(context.Background(), prepared, initial, nil)
	if err != nil {
		t.Fatalf("compactContextForRun() error = %v", err)
	}
	if result == nil || result.HistoryPressure >= contextCompactionTriggerPressure {
		t.Fatalf("result = %#v, want safe third-attempt candidate", result)
	}
	if len(llm.requests) != contextCompactionMaxAttempts {
		t.Fatalf("AppChat calls = %d, want %d", len(llm.requests), contextCompactionMaxAttempts)
	}
}

func TestCompactContextForRunUsesSafeAboveTargetCandidateAfterLaterFailures(t *testing.T) {
	originalDelays := contextCompactionRetryDelays
	contextCompactionRetryDelays = []time.Duration{0, 0}
	defer func() { contextCompactionRetryDelays = originalDelays }()

	repo := &contextCompactionMessageRepo{}
	llm := &contextCompactionLLM{
		errors:    []error{nil, errors.New("second failure"), errors.New("third failure")},
		responses: []*adapter.ChatResponse{contextSummaryResponse(validContextSummary("safe fallback")), nil, nil},
	}
	svc := &service{repos: &repository.Repositories{Message: repo}, llmClient: llm, tokenEstimator: tokenestimate.NewEstimator()}
	conversationID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: uuid.New(), AccountID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Metadata: map[string]interface{}{}},
		parts:        &chatRequestParts{Query: "continue", Provider: "test-provider", ModelName: "test-model", Parameters: map[string]interface{}{}},
	}
	raw := make([]*runtimemodel.Message, 10)
	for index := range raw {
		raw[index] = &runtimemodel.Message{
			ID:             uuid.New(),
			ConversationID: conversationID,
			Query:          strings.Repeat("context ", 118),
			Answer:         strings.Repeat("response ", 118),
			Status:         runtimemodel.MessageStatusCompleted,
		}
	}
	initial := &contextBudgetResult{
		Metadata:            map[string]interface{}{},
		Budget:              &budgetComputation{PromptBudget: 8750, Tokenizer: "test"},
		HistoryTokens:       8000,
		HistoryBudgetTokens: 8750,
		HistoryPressure:     0.91,
		RawMessages:         raw,
		Spec:                ModelSpec{ContextWindow: 12000, MaxOutputTokens: 2048},
		SystemPrompt:        "system",
	}
	result, err := svc.compactContextForRun(context.Background(), prepared, initial, nil)
	if err != nil {
		t.Fatalf("compactContextForRun() error = %v", err)
	}
	if result.HistoryPressure < contextCompactionTargetPressure || result.HistoryPressure >= contextCompactionTriggerPressure {
		t.Fatalf("history pressure = %.3f, want safe fallback in [%.2f, %.2f)", result.HistoryPressure, contextCompactionTargetPressure, contextCompactionTriggerPressure)
	}
	if len(llm.requests) != contextCompactionMaxAttempts {
		t.Fatalf("AppChat calls = %d, want all attempts while target remains unmet", len(llm.requests))
	}
}

func TestCompactContextForRunBlocksWhenSnapshotPersistenceFails(t *testing.T) {
	originalDelays := contextCompactionRetryDelays
	contextCompactionRetryDelays = []time.Duration{0, 0}
	defer func() { contextCompactionRetryDelays = originalDelays }()

	persistErr := errors.New("database temporarily unavailable")
	repo := &contextCompactionMessageRepo{updateErrors: []error{nil, persistErr, persistErr, persistErr, nil}}
	llm := &contextCompactionLLM{responses: []*adapter.ChatResponse{contextSummaryResponse(validContextSummary("keep project alpha"))}}
	svc := &service{
		repos:          &repository.Repositories{Message: repo},
		llmClient:      llm,
		tokenEstimator: tokenestimate.NewEstimator(),
	}
	conversationID := uuid.New()
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: uuid.New(), AccountID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Metadata: map[string]interface{}{}},
		parts:        &chatRequestParts{Query: "continue", Provider: "test-provider", ModelName: "test-model", Parameters: map[string]interface{}{}},
	}
	raw := make([]*runtimemodel.Message, 10)
	for index := range raw {
		raw[index] = &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, Query: "old", Answer: "answer", Status: runtimemodel.MessageStatusCompleted}
	}
	initial := &contextBudgetResult{
		Metadata:            map[string]interface{}{},
		Budget:              &budgetComputation{PromptBudget: 8000, Tokenizer: "test"},
		HistoryTokens:       7000,
		HistoryBudgetTokens: 8000,
		HistoryPressure:     0.875,
		RawMessages:         raw,
		Spec:                ModelSpec{ContextWindow: 16000},
		SystemPrompt:        "system",
	}
	statuses := []string{}
	_, err := svc.compactContextForRun(context.Background(), prepared, initial, func(event StreamEvent) error {
		if event.EventType == streamEventAgentProgress {
			statuses = append(statuses, stringFromAny(event.Payload["status"]))
		}
		return nil
	})
	if !apperror.IsCode(err, AppCodeContextCompactionUnavailable) {
		t.Fatalf("error = %v, want compaction unavailable", err)
	}
	if len(llm.requests) != 1 {
		t.Fatalf("AppChat calls = %d, want one generated summary reused across persistence retries", len(llm.requests))
	}
	if len(statuses) != 1 || statuses[0] != "running" {
		t.Fatalf("progress statuses = %#v, want running only", statuses)
	}
	metadata, decodeErr := decodeContextCompactionMetadata(repo.metadata)
	if decodeErr != nil || metadata == nil || metadata.Status != contextCompactionStatusFailedBlocked {
		t.Fatalf("failure metadata = %#v, err = %v", metadata, decodeErr)
	}
	if metadata.Snapshot != nil {
		t.Fatalf("failed persistence must not expose an uncommitted snapshot: %#v", metadata.Snapshot)
	}
	if metadata.FailureCategory != "persist" {
		t.Fatalf("failure category = %q, want persist", metadata.FailureCategory)
	}
}

func TestPrepareLLMRequestBlocksBeforeMainModelWhenCompactionFails(t *testing.T) {
	originalDelays := contextCompactionRetryDelays
	contextCompactionRetryDelays = []time.Duration{0, 0}
	defer func() { contextCompactionRetryDelays = originalDelays }()

	conversationID := uuid.New()
	parentID := uuid.New()
	parent := &runtimemodel.Message{
		ID:             parentID,
		ConversationID: conversationID,
		Query:          strings.Repeat("old request ", 400),
		Answer:         strings.Repeat("old answer ", 400),
		Status:         runtimemodel.MessageStatusCompleted,
		Metadata:       map[string]interface{}{},
	}
	repo := &contextCompactionMessageRepo{messages: map[uuid.UUID]*runtimemodel.Message{parentID: parent}}
	llm := &contextCompactionLLM{errors: []error{errors.New("unavailable"), errors.New("unavailable"), errors.New("unavailable")}}
	svc := &service{
		repos:             &repository.Repositories{Message: repo},
		llmClient:         llm,
		tokenEstimator:    tokenestimate.NewEstimator(),
		modelSpecResolver: contextCompactionModelSpecResolver{spec: ModelSpec{ContextWindow: 2400, MaxOutputTokens: 512}},
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID, OrganizationID: uuid.New(), AccountID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), ConversationID: conversationID, ParentID: &parentID, Metadata: map[string]interface{}{}},
		ParentID:     &parentID,
		Scope:        Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		parts: &chatRequestParts{
			Query:      "continue",
			Provider:   "test-provider",
			ModelName:  "test-model",
			Parameters: map[string]interface{}{},
		},
	}
	prepared.Conversation.OrganizationID = prepared.Scope.OrganizationID
	prepared.Conversation.AccountID = prepared.Scope.AccountID

	err := svc.prepareLLMRequestForRun(context.Background(), prepared, nil)
	if !apperror.IsCode(err, AppCodeContextCompactionUnavailable) {
		t.Fatalf("prepare error = %v, want compaction unavailable", err)
	}
	if len(llm.requests) != contextCompactionMaxAttempts {
		t.Fatalf("summary calls = %d, want %d", len(llm.requests), contextCompactionMaxAttempts)
	}
	if llm.streamCalls != 0 || prepared.LLMRequest != nil {
		t.Fatalf("main stream calls = %d request = %#v, want none", llm.streamCalls, prepared.LLMRequest)
	}
}

func TestRedactPrivateContextMetadataRemovesSnapshot(t *testing.T) {
	marker := "private-summary-marker"
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{
			"prompt_budget": 1000,
			"compaction": map[string]interface{}{
				"status":                contextCompactionStatusSucceeded,
				"snapshot":              map[string]interface{}{"summary": marker},
				"snapshot_ref":          map[string]interface{}{"owner_message_id": uuid.New().String()},
				"history_tokens_before": 900,
			},
		},
	}
	encoded, err := json.Marshal(RedactPrivateContextMetadata(metadata))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), marker) || strings.Contains(string(encoded), "snapshot_ref") || strings.Contains(string(encoded), "history_tokens_before") {
		t.Fatalf("client metadata leaked private context: %s", encoded)
	}
}

func contextSummaryResponse(summary string) *adapter.ChatResponse {
	return &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: summary}, FinishReason: "stop"}}}
}

func validContextSummary(value string) string {
	return "Current objective\n" + value +
		"\nActive constraints\nNone" +
		"\nConfirmed decisions\nNone" +
		"\nCompleted results\nNone" +
		"\nImportant entities\nNone" +
		"\nOpen work\nContinue" +
		"\nFailed attempts\nNone" +
		"\nUncertainties\nNone"
}

package contextmgr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

type Manager struct {
	mu           sync.Mutex
	config       Config
	estimator    *tokenestimate.Estimator
	compactor    Compactor
	toolStore    ToolResultStore
	observer     RequestObserver
	checkpoint   CheckpointFunc
	state        AgentContextState
	pendingUsage *adapter.Usage
	// originals holds the complete content of projected tool results by
	// content hash, read back with read_context_artifact (ADR 0037).
	originals map[string]string
	// lastRequestEstimate is the estimate of the last request sent, compared
	// with the provider's prompt tokens to calibrate the estimator.
	lastRequestEstimate int
}

func New(config Config, compactor Compactor, toolStore ToolResultStore, observer RequestObserver, checkpoint CheckpointFunc) (*Manager, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		config:     normalized,
		estimator:  tokenestimate.NewEstimator(),
		compactor:  compactor,
		toolStore:  toolStore,
		observer:   observer,
		checkpoint: checkpoint,
		state: AgentContextState{
			SchemaVersion:       1,
			AgentRunID:          strings.TrimSpace(normalized.AgentRunID),
			NextRound:           1,
			ContentReplacements: map[string]ContentReplacement{},
			EstimatorScale:      clampEstimateScale(normalized.EstimateScale),
			CreatedAt:           time.Now().UTC(),
		},
	}
	if manager.state.AgentRunID == "" {
		return nil, fmt.Errorf("agent run id is required")
	}
	return manager, nil
}

// Checkpoint hands the transcript produced so far to the registered writer.
// Callers use it at interruption points: the assistant message of a round and
// the tool results of its calls must be durable before the next model call, so
// a process that dies mid-turn loses at most the round in flight.
func (m *Manager) Checkpoint(ctx context.Context) {
	if m == nil || m.checkpoint == nil {
		return
	}
	m.mu.Lock()
	transcript := cloneMessages(m.state.TurnTranscript)
	m.mu.Unlock()
	if len(transcript) == 0 {
		return
	}
	m.checkpoint(ctx, transcript)
}

func (m *Manager) PrepareBeforeModelCall(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatRequest, Decision, error) {
	if m == nil || request == nil {
		return nil, Decision{}, fmt.Errorf("context manager request is required")
	}
	m.Checkpoint(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()

	// System messages keep their positions here; only the request handed back
	// is placed for the provider, so later guidance stays after the cached
	// prefix (ADR 0032).
	prepared := cloneRequest(request)
	round := m.state.NextRound
	if round <= 0 {
		round = 1
	}
	m.state.Messages = cloneMessages(prepared.Messages)

	requestedOutput := 0
	if prepared.MaxTokens != nil {
		requestedOutput = *prepared.MaxTokens
	}
	budget, err := budgetForRequest(m.config, requestedOutput)
	if err != nil {
		return nil, Decision{}, err
	}
	// The budget is in real tokens; comparing it with estimates needs it in
	// estimated tokens, through the scale the provider's usage showed
	// (ADR 0038).
	budget = scaledBudget(budget, clampEstimateScale(m.state.EstimatorScale))
	m.state.SchemaVersion = 1
	m.state.Provider = strings.TrimSpace(prepared.Provider)
	m.state.Model = strings.TrimSpace(prepared.Model)
	m.state.ModelContextWindowTokens = budget.ModelContextWindow
	m.state.ConfiguredAgentWindowTokens = budget.ConfiguredAgentWindowK * 1000
	m.state.EffectiveAgentWindowTokens = budget.AgentContextWindow
	if m.state.EstimatorScale <= 0 {
		m.state.EstimatorScale = 1
	}
	if m.state.CreatedAt.IsZero() {
		m.state.CreatedAt = time.Now().UTC()
	}
	decision := Decision{
		AgentRunID:    m.state.AgentRunID,
		APIRound:      round,
		RequestType:   RequestTypeMain,
		Action:        DecisionNone,
		Budget:        budget,
		EstimateScale: clampEstimateScale(m.state.EstimatorScale),
	}

	// An oversized tool result reaches the model as its beginning and end
	// with a reference to the rest. Only the outgoing request is projected:
	// the run and its transcript keep the original, so the projection is the
	// same in every request and the rest can be read back (ADR 0037).
	projectedMessages, projection, projectErr := m.projectOversizedToolResults(ctx, prepared.Messages)
	if projectErr != nil {
		return nil, decision, projectErr
	}
	prepared.Messages = projectedMessages
	decision.ToolProjectionCount = projection.count

	estimate := m.estimator.EstimateChatRequest(prepared)
	decision.BeforeTokens = estimate.Tokens
	decision.ToolResultOriginalTokens, decision.ToolResultProjectedTokens = m.measureToolResults(prepared.Messages, prepared.Model)
	if err := validateToolPairing(prepared.Messages); err != nil {
		return nil, decision, fmt.Errorf("validate model request tool pairing: %w", err)
	}

	if estimate.Tokens >= budget.PromptBudget && withoutEarlierTurnReasoning(prepared) {
		// The reasoning of earlier turns is the first thing to give up when
		// the request does not fit (ADR 0045).
		decision.ReasoningDropped = true
		estimate = m.estimator.EstimateChatRequest(prepared)
	}
	if estimate.Tokens >= budget.PromptBudget {
		compressed, compression, compressErr := m.compressHistoricalToolResults(ctx, prepared, budget.PromptBudget)
		if compressErr != nil {
			return nil, decision, compressErr
		}
		prepared = compressed
		decision.ToolResultOriginalTokens = compression.originalTokens
		decision.ToolResultProjectedTokens = compression.projectedTokens
		decision.ToolProjectionCount = compression.count
		estimate = m.estimator.EstimateChatRequest(prepared)
		if compression.count > 0 {
			decision.Action = DecisionMicrocompact
			m.syncTurnTranscriptMessages(prepared.Messages)
			if m.checkpoint != nil {
				m.checkpoint(context.WithoutCancel(ctx), cloneMessages(m.state.TurnTranscript))
			}
		}
	}
	if estimate.Tokens >= budget.PromptBudget && m.compactor != nil && m.state.Compaction.RunCompactions < maxCompactionsPerRun {
		beforeSemantic := cloneRequest(prepared)
		compacted, summary, compactDecision, compactErr := m.semanticCompact(ctx, prepared, decision, false)
		if compactErr != nil {
			m.state.Compaction.ConsecutiveFailures++
			m.state.Compaction.LastFailure = compactErr.Error()
			decision.CompactionFailure = compactErr.Error()
			prepared = beforeSemantic
		} else {
			prepared = compacted
			decision = compactDecision
			decision.CompactionFailure = ""
			m.state.Summary = summary
			m.state.Compaction.ConsecutiveFailures = 0
			m.state.Compaction.LastFailure = ""
			m.state.Compaction.LastCompactedAt = time.Now().UTC()
			m.state.Compaction.RunCompactions++
		}
		estimate = m.estimator.EstimateChatRequest(prepared)
	}
	if estimate.Tokens >= budget.PromptBudget {
		// Compaction failed, was capped or could not make the request fit:
		// leave the oldest history out instead of failing the turn (ADR 0038).
		if fitted, ok := m.fitByDroppingHistory(prepared, budget.PromptBudget); ok {
			prepared = fitted
			decision.Action = DecisionDropHistory
			estimate = m.estimator.EstimateChatRequest(prepared)
		}
	}
	m.syncTurnTranscriptMessages(prepared.Messages)

	decision.AfterTokens = estimate.Tokens
	decision.FinalPromptTokens = estimate.Tokens
	decision.Estimator = estimate.Tokenizer
	m.state.Tokenizer = estimate.Tokenizer
	decision.ToolResultProjectedTokens = m.currentToolResultTokens(prepared.Messages, prepared.Model)
	decision.ComponentTokens = componentTokenMap(estimate)
	decision.Budget.ContextPressure = float64(estimate.Tokens) / float64(budget.PromptBudget)
	decision.FixedRequestTokens, decision.CompressibleTokens = m.requestTokenClasses(prepared)
	decision.ConsecutiveCompactionFailures = m.state.Compaction.ConsecutiveFailures
	if estimate.Tokens >= budget.PromptBudget {
		m.state.Messages = cloneMessages(prepared.Messages)
		if decision.CompactionFailure != "" {
			return nil, decision, fmt.Errorf("%w: context compaction failed: %s; prompt=%d limit=%d", ErrContextExhausted, decision.CompactionFailure, estimate.Tokens, budget.PromptBudget)
		}
		return nil, decision, fmt.Errorf("%w: final model request exceeds 80%% of the model context window: prompt=%d limit=%d", ErrContextExhausted, estimate.Tokens, budget.PromptBudget)
	}

	m.state.Messages = cloneMessages(prepared.Messages)
	m.lastRequestEstimate = estimate.Tokens
	prepared.Messages = adapter.PlaceSystemMessages(prepared.Messages)
	return prepared, decision, nil
}

func (m *Manager) measureToolResults(messages []adapter.Message, model string) (int, int) {
	original, projected := 0, 0
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			tokens := m.estimator.EstimateMessages([]adapter.Message{message}, model).Tokens
			original += tokens
			projected += tokens
		}
	}
	return original, projected
}

func (m *Manager) currentToolResultTokens(messages []adapter.Message, model string) int {
	total := 0
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			total += m.estimator.EstimateMessages([]adapter.Message{message}, model).Tokens
		}
	}
	return total
}

// PrepareReactiveCompact forces one semantic compaction after an upstream
// prompt-too-long response. Callers retry the main request only once.
func (m *Manager) PrepareReactiveCompact(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatRequest, Decision, error) {
	if m == nil || request == nil {
		return nil, Decision{}, fmt.Errorf("reactive context request is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	prepared := cloneRequest(request)
	requestedOutput := 0
	if prepared.MaxTokens != nil {
		requestedOutput = *prepared.MaxTokens
	}
	budget, err := budgetForRequest(m.config, requestedOutput)
	if err != nil {
		return nil, Decision{}, err
	}
	decision := Decision{AgentRunID: m.state.AgentRunID, APIRound: max(1, m.state.NextRound), RequestType: RequestTypeReactiveCompact, Action: DecisionReactiveCompact, Budget: budget, EstimateScale: 1}
	decision.BeforeTokens = m.estimator.EstimateChatRequest(prepared).Tokens
	compacted, summary, decision, err := m.semanticCompact(ctx, prepared, decision, true)
	if err != nil {
		return nil, decision, err
	}
	m.state.Messages = cloneMessages(compacted.Messages)
	m.syncTurnTranscriptMessages(compacted.Messages)
	m.state.Summary = summary
	m.state.Compaction.ConsecutiveFailures = 0
	m.state.Compaction.LastFailure = ""
	m.state.Compaction.LastCompactedAt = time.Now().UTC()
	compacted.Messages = adapter.PlaceSystemMessages(compacted.Messages)
	return compacted, decision, nil
}

func (m *Manager) ObserveModelResponse(ctx context.Context, message adapter.Message, usage *adapter.Usage) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	// The active request keeps reasoning for the rest of the turn; the
	// durable transcript stores it only for models that replay it (ADR 0045).
	message = cloneMessage(message)
	m.state.Messages = append(m.state.Messages, message)
	if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
		transcript := cloneMessage(message)
		if !adapter.KeepsReasoningHistory(m.state.Model) {
			transcript.ReasoningContent = ""
		}
		m.state.TurnTranscript = append(m.state.TurnTranscript, transcript)
	}
	if m.state.NextRound <= 0 {
		m.state.NextRound = 1
	}
	m.state.NextRound++
	if usage != nil {
		cloned := *usage
		m.state.LastUsage = &cloned
		if usage.PromptTokens > 0 && m.lastRequestEstimate > 0 {
			m.state.EstimatorScale = clampEstimateScale(float64(usage.PromptTokens) / float64(m.lastRequestEstimate))
		}
	}
	m.mu.Unlock()
	// The response is durable before anything acts on it: a tool call that is
	// about to be executed must never be discovered only in a lost process.
	m.Checkpoint(ctx)
	return nil
}

func (m *Manager) ConsumeCompactionUsage() *adapter.Usage {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	usage := m.pendingUsage
	m.pendingUsage = nil
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func (m *Manager) ReplaceMessages(ctx context.Context, messages []adapter.Message) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	m.appendNewTurnToolMessages(messages)
	m.syncTurnTranscriptMessages(messages)
	m.state.Messages = cloneMessages(messages)
	m.mu.Unlock()
	// A tool result becomes durable as soon as it exists: a crash in the middle
	// of a batch loses at most the call that was still running.
	m.Checkpoint(ctx)
	return nil
}

// TurnTranscript returns the model-protocol messages produced by this run.
// Callers may persist this value as the durable, cross-turn representation of
// the Agent's intermediate assistant/tool exchange.
func (m *Manager) TurnTranscript() []adapter.Message {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneMessages(m.state.TurnTranscript)
}

// BindTurnTranscript attaches durable entry identities to the manager's
// in-memory transcript after a checkpoint has written them.  EntryID is not
// part of the provider protocol, but compaction needs it to persist an exact
// parent-chain boundary for messages produced during the current run.
func (m *Manager) BindTurnTranscript(messages []adapter.Message) {
	if m == nil || len(messages) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.state.TurnTranscript) != len(messages) {
		return
	}
	m.state.TurnTranscript = cloneMessages(messages)
}

func (m *Manager) appendNewTurnToolMessages(messages []adapter.Message) {
	knownCalls := make(map[string]struct{})
	knownResults := make(map[string]struct{})
	for _, message := range m.state.TurnTranscript {
		for _, call := range message.ToolCalls {
			if id := strings.TrimSpace(call.ID); id != "" {
				knownCalls[id] = struct{}{}
			}
		}
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			if id := strings.TrimSpace(message.ToolCallID); id != "" {
				knownResults[id] = struct{}{}
			}
		}
	}
	for _, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		id := strings.TrimSpace(message.ToolCallID)
		if id == "" {
			continue
		}
		if _, ok := knownCalls[id]; !ok {
			continue
		}
		if _, exists := knownResults[id]; exists {
			continue
		}
		message.ReasoningContent = ""
		m.state.TurnTranscript = append(m.state.TurnTranscript, cloneMessage(message))
		knownResults[id] = struct{}{}
	}
	if adapter.KeepsReasoningHistory(m.state.Model) {
		m.appendNewTurnReminders(messages, knownCalls)
	}
}

// appendNewTurnReminders records the guidance the loop added after this
// turn's tool calls, such as the instructions of a skill activated mid-turn,
// as the user turn a provider receives it in. With reasoning kept across
// turns, the next turn's request then extends this turn's requests past its
// tool calls, and the provider's cache covers them (ADR 0043).
func (m *Manager) appendNewTurnReminders(messages []adapter.Message, knownCalls map[string]struct{}) {
	recorded := make(map[string]struct{})
	for _, message := range m.state.TurnTranscript {
		if strings.EqualFold(strings.TrimSpace(message.Role), "user") {
			if text, ok := message.Content.(string); ok {
				recorded[text] = struct{}{}
			}
		}
	}
	afterTurnCall := false
	pending := make([]string, 0, 2)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		content := strings.Join(pending, "\n\n")
		pending = pending[:0]
		if _, exists := recorded[content]; exists {
			return
		}
		recorded[content] = struct{}{}
		m.state.TurnTranscript = append(m.state.TurnTranscript, adapter.Message{Role: "user", Content: content})
	}
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role == "assistant" {
			for _, call := range message.ToolCalls {
				if _, known := knownCalls[strings.TrimSpace(call.ID)]; known {
					afterTurnCall = true
				}
			}
		}
		if role == "system" && afterTurnCall {
			if text, ok := message.Content.(string); ok && strings.TrimSpace(text) != "" {
				pending = append(pending, adapter.SystemReminder(text))
				continue
			}
		}
		flush()
	}
	flush()
}

// syncTurnTranscriptMessages keeps the durable turn transcript aligned with
// the exact projected assistant arguments and projected or microcompacted tool
// content sent to the model. Semantic compaction may remove old rounds from
// the active request; their last model-visible projected form remains here.
func (m *Manager) syncTurnTranscriptMessages(messages []adapter.Message) {
	if len(m.state.TurnTranscript) == 0 || len(messages) == 0 {
		return
	}
	currentAssistants := make(map[string]adapter.Message)
	current := make(map[string]adapter.Message)
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
			if !adapter.KeepsReasoningHistory(m.state.Model) {
				message.ReasoningContent = ""
			}
			for _, call := range message.ToolCalls {
				if id := strings.TrimSpace(call.ID); id != "" {
					currentAssistants[id] = cloneMessage(message)
				}
			}
		}
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		if id := strings.TrimSpace(message.ToolCallID); id != "" {
			message.ReasoningContent = ""
			current[id] = cloneMessage(message)
		}
	}
	for index, message := range m.state.TurnTranscript {
		if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") && len(message.ToolCalls) > 0 {
			if replacement, ok := currentAssistants[strings.TrimSpace(message.ToolCalls[0].ID)]; ok {
				m.state.TurnTranscript[index] = replacement
			}
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		if replacement, ok := current[strings.TrimSpace(message.ToolCallID)]; ok {
			// A projection is applied to each request again; the transcript
			// keeps the original it is made from.
			if receipt, projected := decodeToolResultReceipt(contentString(replacement.Content)); projected &&
				strings.EqualFold(strings.TrimSpace(fmt.Sprint(receipt["status"])), "projected") {
				continue
			}
			m.state.TurnTranscript[index] = replacement
		}
	}
}

func (m *Manager) State() AgentContextState {
	if m == nil {
		return AgentContextState{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneState(m.state)
}

type projectionStats struct {
	originalTokens  int
	projectedTokens int
	count           int
}

// compressHistoricalToolResults is deliberately gated by the overall prompt
// budget. A large result is kept verbatim while the request is still below the
// context trigger. Once the trigger is reached, progressively protect the
// newest four, three, then two API rounds and lossily compress only older tool
// results. The compressed text is the canonical value; there is no recovery
// artifact or second read path.
func (m *Manager) compressHistoricalToolResults(ctx context.Context, request *adapter.ChatRequest, promptBudget int) (*adapter.ChatRequest, projectionStats, error) {
	if request == nil || promptBudget <= 0 {
		return request, projectionStats{}, nil
	}
	current := cloneRequest(request)
	stats := projectionStats{}
	for _, message := range current.Messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			tokens := m.estimator.EstimateMessages([]adapter.Message{message}, current.Model).Tokens
			stats.originalTokens += tokens
			stats.projectedTokens += tokens
		}
	}
	for protectedRounds := defaultPreservedRounds; protectedRounds >= 2; protectedRounds-- {
		start := historicalToolBoundary(current.Messages, protectedRounds)
		for index := 0; index < start && index < len(current.Messages); index++ {
			message := current.Messages[index]
			if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
				continue
			}
			content := contentString(message.Content)
			if strings.HasPrefix(content, "[tool result compressed]") {
				continue
			}
			original := m.estimator.EstimateText(content, current.Model).Tokens
			if original <= m.config.MaxToolResultTokens {
				continue
			}
			compressed := m.compressToolResult(content, current.Model)
			message.Content = compressed
			current.Messages[index] = message
			stats.projectedTokens -= original
			stats.projectedTokens += m.estimator.EstimateText(compressed, current.Model).Tokens
			stats.count++
		}
		if m.estimator.EstimateChatRequest(current).Tokens < promptBudget {
			return current, stats, nil
		}
	}
	// System messages keep their positions; only the request handed to the
	// provider is placed for it (ADR 0032).
	return current, stats, nil
}

// historicalToolBoundary returns the index in messages where the protected
// rounds begin. System messages stay where they are, so the dialogue offset is
// mapped back to its position among them.
func historicalToolBoundary(messages []adapter.Message, protectedRounds int) int {
	_, dialogue := splitSystemMessages(messages)
	if len(dialogue) == 0 {
		return len(messages)
	}
	start := selectPreservedTailMessagesByRounds(dialogue, protectedRounds)
	if start <= 0 {
		return len(messages)
	}
	seen := 0
	for index, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			continue
		}
		if seen == start {
			return index
		}
		seen++
	}
	return len(messages)
}

func (m *Manager) compressToolResult(content, model string) string {
	const marker = "[tool result compressed]"
	const header = "\nOriginal result exceeded the tool-result limit. The middle section was permanently omitted.\n"
	runes := []rune(content)
	if len(runes) == 0 {
		return marker + header
	}
	build := func(keep int) string {
		if keep >= len(runes) {
			return content
		}
		head := keep * 6 / 10
		if head < 1 {
			head = 1
		}
		tail := keep - head
		if tail < 1 {
			tail = 1
			head = keep - tail
		}
		return marker + header + "\n===== beginning =====\n" + string(runes[:head]) + "\n===== middle omitted =====\n" + string(runes[len(runes)-tail:]) + "\n===== end =====\n"
	}
	low, high := 2, len(runes)
	best := build(2)
	for low <= high {
		middle := low + (high-low)/2
		candidate := build(middle)
		if m.estimator.EstimateText(candidate, model).Tokens <= DefaultCompressedToolResultTokens {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	return best
}

func (m *Manager) projectOversizedToolResults(ctx context.Context, messages []adapter.Message) ([]adapter.Message, projectionStats, error) {
	out := cloneMessages(messages)
	stats := projectionStats{}
	for index := range out {
		if !strings.EqualFold(strings.TrimSpace(out[index].Role), "tool") {
			continue
		}
		result := m.estimator.EstimateMessages([]adapter.Message{out[index]}, "")
		originalTokens := result.Tokens
		if header, _, expanded := decodeContextArtifactToolResult(contentString(out[index].Content)); expanded {
			if recorded := intFromJSONNumber(header["total_tokens"]); recorded > 0 {
				originalTokens = recorded
			}
			stats.originalTokens += originalTokens
			stats.projectedTokens += result.Tokens
			continue
		}
		if receipt, ok := decodeToolResultReceipt(contentString(out[index].Content)); ok {
			if recorded := intFromJSONNumber(receipt["original_tokens"]); recorded > 0 {
				originalTokens = recorded
			}
		}
		stats.originalTokens += originalTokens
		if result.Tokens <= m.config.ProjectToolResultTokens {
			stats.projectedTokens += result.Tokens
			continue
		}
		replacement, err := m.toolReplacement(ctx, out[index], false)
		if err != nil {
			return nil, stats, err
		}
		out[index].Content = replacement.Replacement
		stats.projectedTokens += m.estimator.EstimateMessages([]adapter.Message{out[index]}, "").Tokens
		stats.count++
	}
	return out, stats, nil
}

func (m *Manager) microcompactOldToolResults(ctx context.Context, messages []adapter.Message) ([]adapter.Message, bool) {
	out := cloneMessages(messages)
	const minimumRecentToolResultsToKeep = 4
	protected := latestPendingToolBatchIndexes(out)
	for index := len(out) - 1; index >= 0 && len(protected) < minimumRecentToolResultsToKeep; index-- {
		if strings.EqualFold(strings.TrimSpace(out[index].Role), "tool") {
			protected[index] = struct{}{}
		}
	}
	changed := false
	for index := range out {
		if _, keep := protected[index]; keep || !strings.EqualFold(strings.TrimSpace(out[index].Role), "tool") {
			continue
		}
		if receipt, ok := decodeToolResultReceipt(contentString(out[index].Content)); ok {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprint(receipt["status"])), "compacted") {
				continue
			}
		}
		replacement, err := m.toolReplacement(ctx, out[index], true)
		if err != nil {
			continue
		}
		out[index].Content = replacement.Replacement
		changed = true
	}
	return out, changed
}

// latestPendingToolBatchIndexes protects every result produced by the most
// recent model response when that response has not yet been consumed by a
// later user or assistant message. Parallel calls are one batch: none of their
// results may lose its preview before the model has seen the batch once.
func latestPendingToolBatchIndexes(messages []adapter.Message) map[int]struct{} {
	protected := map[int]struct{}{}
	callIDs := map[string]struct{}{}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		switch strings.ToLower(strings.TrimSpace(message.Role)) {
		case "system", "tool":
			continue
		case "assistant":
			if len(message.ToolCalls) == 0 {
				return protected
			}
			for _, call := range message.ToolCalls {
				if id := strings.TrimSpace(call.ID); id != "" {
					callIDs[id] = struct{}{}
				}
			}
		default:
			return protected
		}
		break
	}
	if len(callIDs) == 0 {
		return protected
	}
	for index, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			continue
		}
		if _, ok := callIDs[strings.TrimSpace(message.ToolCallID)]; ok {
			protected[index] = struct{}{}
		}
	}
	return protected
}

type pendingToolPreview struct {
	messageIndex int
	receipt      map[string]interface{}
	preview      string
}

func (m *Manager) toolReplacement(ctx context.Context, message adapter.Message, compact bool) (ContentReplacement, error) {
	content := contentString(message.Content)
	if header, expandedContent, expanded := decodeContextArtifactToolResult(content); expanded {
		if !compact {
			return ContentReplacement{}, fmt.Errorf("expanded context artifact results cannot be projected recursively")
		}
		return m.compactContextArtifactToolResult(message, header, expandedContent)
	}
	if compact {
		if receipt, ok := decodeToolResultReceipt(content); ok {
			return m.compactToolResultReceipt(message, receipt)
		}
	}
	hashBytes := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(hashBytes[:])
	key := strings.TrimSpace(message.ToolCallID) + ":" + hash
	if existing, ok := m.state.ContentReplacements[key]; ok && (!compact || strings.Contains(existing.Replacement, `"status":"compacted"`)) {
		return existing, nil
	}
	originalTokens := m.estimator.EstimateText(content, "").Tokens
	artifactRef := ""
	if m.toolStore != nil {
		ref, err := m.toolStore.Put(ctx, m.state.AgentRunID, hash, content)
		if err != nil {
			return ContentReplacement{}, fmt.Errorf("store oversized tool result: %w", err)
		}
		artifactRef = ref
	} else if ref, err := m.rememberOriginal(hash, content); err == nil {
		artifactRef = ref
	}
	preview := headAndTail(content, m.config.ToolResultPreviewRunes)
	status := "projected"
	if compact {
		status = "compacted"
		preview = ""
	}
	payload := map[string]interface{}{
		"status":          status,
		"tool_call_id":    strings.TrimSpace(message.ToolCallID),
		"content_hash":    hash,
		"original_tokens": originalTokens,
		"truncated":       true,
	}
	if artifactRef != "" {
		payload["artifact_ref"] = artifactRef
	}
	if preview != "" {
		payload["preview"] = preview
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ContentReplacement{}, fmt.Errorf("encode tool result replacement: %w", err)
	}
	replacement := ContentReplacement{
		ToolCallID:     strings.TrimSpace(message.ToolCallID),
		ContentHash:    hash,
		ArtifactRef:    artifactRef,
		OriginalTokens: originalTokens,
		PreviewTokens:  m.estimator.EstimateText(preview, "").Tokens,
		Replacement:    string(encoded),
	}
	m.state.ContentReplacements[key] = replacement
	return replacement, nil
}

func (m *Manager) compactContextArtifactToolResult(message adapter.Message, header map[string]interface{}, expandedContent string) (ContentReplacement, error) {
	toolCallID := strings.TrimSpace(message.ToolCallID)
	contentHash := strings.TrimSpace(fmt.Sprint(header["content_hash"]))
	artifactRef := strings.TrimSpace(fmt.Sprint(header["artifact_ref"]))
	if contentHash == "" || contentHash == "<nil>" || artifactRef == "" || artifactRef == "<nil>" {
		return ContentReplacement{}, fmt.Errorf("expanded context artifact result is missing its original reference")
	}
	key := toolCallID + ":" + contentHash
	if existing, ok := m.state.ContentReplacements[key]; ok && strings.Contains(existing.Replacement, `"status":"compacted"`) {
		return existing, nil
	}
	originalTokens := intFromJSONNumber(header["total_tokens"])
	if originalTokens <= 0 {
		originalTokens = m.estimator.EstimateText(expandedContent, m.state.Model).Tokens
	}
	payload := map[string]interface{}{
		"status":          "compacted",
		"kind":            contextArtifactReadKind,
		"tool_call_id":    toolCallID,
		"artifact_ref":    artifactRef,
		"content_hash":    contentHash,
		"original_tokens": originalTokens,
		"truncated":       true,
	}
	if sourceToolCallID := strings.TrimSpace(fmt.Sprint(header["source_tool_call_id"])); sourceToolCallID != "" && sourceToolCallID != "<nil>" {
		payload["source_tool_call_id"] = sourceToolCallID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ContentReplacement{}, fmt.Errorf("encode compacted context artifact result: %w", err)
	}
	replacement := ContentReplacement{
		ToolCallID:     toolCallID,
		ContentHash:    contentHash,
		ArtifactRef:    artifactRef,
		OriginalTokens: originalTokens,
		Replacement:    string(encoded),
	}
	m.state.ContentReplacements[key] = replacement
	return replacement, nil
}

func (m *Manager) compactToolResultReceipt(message adapter.Message, receipt map[string]interface{}) (ContentReplacement, error) {
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(receipt["status"])))
	if status != "projected" && status != "compacted" {
		return ContentReplacement{}, fmt.Errorf("unsupported tool result receipt status %q", status)
	}
	toolCallID := strings.TrimSpace(message.ToolCallID)
	if recordedID := strings.TrimSpace(fmt.Sprint(receipt["tool_call_id"])); recordedID != "" && recordedID != "<nil>" {
		toolCallID = recordedID
	}
	contentHash := strings.TrimSpace(fmt.Sprint(receipt["content_hash"]))
	if contentHash == "<nil>" {
		contentHash = ""
	}
	if contentHash == "" {
		return ContentReplacement{}, fmt.Errorf("projected tool result receipt is missing content_hash")
	}
	key := toolCallID + ":" + contentHash
	if existing, ok := m.state.ContentReplacements[key]; ok && status == "compacted" {
		return existing, nil
	}

	compacted := make(map[string]interface{}, len(receipt))
	for key, value := range receipt {
		compacted[key] = value
	}
	compacted["status"] = "compacted"
	delete(compacted, "preview")
	encoded, err := json.Marshal(compacted)
	if err != nil {
		return ContentReplacement{}, fmt.Errorf("encode compacted tool result receipt: %w", err)
	}
	replacement := ContentReplacement{
		ToolCallID:     toolCallID,
		ContentHash:    contentHash,
		ArtifactRef:    strings.TrimSpace(fmt.Sprint(receipt["artifact_ref"])),
		OriginalTokens: intFromJSONNumber(receipt["original_tokens"]),
		Replacement:    string(encoded),
	}
	if replacement.ArtifactRef == "<nil>" {
		replacement.ArtifactRef = ""
	}
	m.state.ContentReplacements[key] = replacement
	return replacement, nil
}

func decodeToolResultReceipt(content string) (map[string]interface{}, bool) {
	receipt := map[string]interface{}{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &receipt); err != nil {
		return nil, false
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(receipt["status"])))
	if status != "projected" && status != "compacted" {
		return nil, false
	}
	if strings.TrimSpace(fmt.Sprint(receipt["content_hash"])) == "" {
		return nil, false
	}
	return receipt, true
}

func intFromJSONNumber(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		return 0
	}
}

func (m *Manager) semanticCompact(ctx context.Context, request *adapter.ChatRequest, decision Decision, reactive bool) (*adapter.ChatRequest, *ContextSummary, Decision, error) {
	maxPreservedRounds := defaultPreservedRounds
	if reactive {
		maxPreservedRounds = 1
	}
	if m.config.TailMinMessages > 0 {
		return m.semanticCompactWithRounds(ctx, request, decision, reactive, maxPreservedRounds)
	}
	var lastErr error
	for keep := maxPreservedRounds; keep >= 1; keep-- {
		compacted, summary, compactDecision, err := m.semanticCompactWithRounds(ctx, request, decision, reactive, keep)
		if err == nil {
			return compacted, summary, compactDecision, nil
		}
		lastErr = err
		if !strings.Contains(err.Error(), "compacted request still exceeds 80%") {
			break
		}
	}
	return nil, nil, decision, lastErr
}

func (m *Manager) semanticCompactWithRounds(ctx context.Context, request *adapter.ChatRequest, decision Decision, reactive bool, preservedRoundCount int) (*adapter.ChatRequest, *ContextSummary, Decision, error) {
	systems, dialogue := splitSystemMessages(request.Messages)
	rounds := groupMessagesByAPIRoundForRun(dialogue, m.state.NextRound)
	if len(dialogue) < 2 {
		return nil, nil, decision, fmt.Errorf("no message prefix is available for compaction")
	}
	if reactive {
		preservedRoundCount = 1
	}
	preserveStart := 0
	if m.config.TailMinMessages > 0 {
		// Legacy callers may still request the old message-count boundary. The
		// application no longer sets this field; runtime compaction uses rounds.
		preserveStart = selectPreservedTailMessages(dialogue, m.config.TailMinMessages)
	}
	for keep := preservedRoundCount; preserveStart == 0 && keep >= 1; keep-- {
		candidateStart := selectPreservedTailMessagesByRounds(dialogue, keep)
		if candidateStart <= 0 || candidateStart >= len(dialogue) {
			continue
		}
		candidateWithoutSummary := cloneRequest(request)
		candidateWithoutSummary.Messages = append(cloneMessages(systems), cloneMessages(dialogue[candidateStart:])...)
		candidateWithoutSummary.Messages = adapter.NormalizeSystemMessages(candidateWithoutSummary.Messages)
		candidateTokens := m.estimator.EstimateChatRequest(candidateWithoutSummary).Tokens
		if candidateTokens+decision.Budget.SummaryOutputReserve < decision.Budget.PromptBudget {
			preserveStart = candidateStart
			break
		}
	}
	if preserveStart <= 0 || preserveStart >= len(dialogue) {
		return nil, nil, decision, fmt.Errorf("no preserved model-round tail fits below the 80%% context limit")
	}
	prefix := cloneMessages(dialogue[:preserveStart])
	tail := cloneMessages(dialogue[preserveStart:])
	compactRequest := cloneRequest(request)
	compactRequest.Messages = cloneMessages(prefix)
	compactRequest.Messages = append(compactRequest.Messages, adapter.Message{Role: "user", Content: GetPartialCompactPrompt("", "up_to")})
	compactRequest.Tools = nil
	compactRequest.ToolChoice = nil
	compactRequest.Functions = nil
	compactRequest.FunctionCall = nil
	compactRequest.ResponseFormat = nil
	compactRequest.Stream = false
	compactRequest.StreamOptions = nil
	maxTokens := decision.Budget.SummaryOutputReserve
	compactRequest.MaxTokens = &maxTokens

	lossyDroppedBatches := 0
	for attempt := 0; attempt < 3 && m.estimator.EstimateChatRequest(compactRequest).Tokens > decision.Budget.CompactInputLimit; attempt++ {
		prefixBatches := groupMessagesByCompactionBatch(compactRequest.Messages[:len(compactRequest.Messages)-1])
		if len(prefixBatches) <= 1 {
			break
		}
		compactRequest.Messages = flattenCompactionBatches(prefixBatches[1:])
		compactRequest.Messages = append(compactRequest.Messages, adapter.Message{Role: "user", Content: GetPartialCompactPrompt("", "up_to")})
		lossyDroppedBatches++
	}
	if m.estimator.EstimateChatRequest(compactRequest).Tokens > decision.Budget.CompactInputLimit {
		return nil, nil, decision, fmt.Errorf("compaction input exceeds compact input limit")
	}
	callType := RequestTypeCompact
	if reactive {
		callType = RequestTypeReactiveCompact
	}
	compactDecision := decision
	compactDecision.RequestType = callType
	compactDecision.Action = DecisionSemanticCompact
	if reactive {
		compactDecision.Action = DecisionReactiveCompact
	}
	compactDecision.LossyRecoveryDroppedRounds = lossyDroppedBatches
	compactDecision.LossyRecoveryDroppedBatches = lossyDroppedBatches
	var rawSummary string
	var compactErr error
	for attempt := 0; attempt < 3; attempt++ {
		attemptDecision := m.decisionForRequest(compactRequest, compactDecision)
		if m.observer != nil {
			m.observer(callType, decision.APIRound, cloneRequest(compactRequest), attemptDecision)
		}
		var compactUsage *adapter.Usage
		compactCall := CompactCall{
			AgentRunID:        m.state.AgentRunID,
			APIRound:          decision.APIRound,
			Type:              callType,
			Decision:          attemptDecision,
			Transcript:        cloneMessages(m.state.TurnTranscript),
			SummarizedEntryID: summarizedEntryID(compactRequest.Messages),
		}
		rawSummary, compactUsage, compactErr = m.compactor.Compact(ctx, compactRequest, compactCall)
		// A compactor may bind newly persisted entry identities to the
		// transcript snapshot. Keep those identities for the next compaction and
		// for the eventual terminal checkpoint.
		if len(compactCall.Transcript) == len(m.state.TurnTranscript) {
			m.state.TurnTranscript = cloneMessages(compactCall.Transcript)
		}
		m.pendingUsage = mergeUsage(m.pendingUsage, compactUsage)
		if compactErr == nil {
			break
		}
		if !isPromptTooLong(compactErr) || attempt == 2 || !dropOldestCompactBatch(compactRequest) {
			return nil, nil, decision, compactErr
		}
		lossyDroppedBatches++
		compactDecision.LossyRecoveryDroppedRounds = lossyDroppedBatches
		compactDecision.LossyRecoveryDroppedBatches = lossyDroppedBatches
	}
	if compactErr != nil {
		return nil, nil, decision, compactErr
	}
	if !validCompactSummary(rawSummary) {
		return nil, nil, decision, fmt.Errorf("compaction response is missing a non-empty summary block")
	}
	formatted := FormatCompactSummary(rawSummary)
	if strings.TrimSpace(formatted) == "" {
		return nil, nil, decision, fmt.Errorf("compaction returned an empty summary")
	}
	summaryMessage := adapter.Message{Role: "user", Content: GetCompactUserSummaryMessage(formatted, true, "", true)}
	candidate := cloneRequest(request)
	candidate.Messages = append(cloneMessages(systems), summaryMessage)
	candidate.Messages = append(candidate.Messages, cloneMessages(tail)...)
	candidate.Messages = adapter.NormalizeSystemMessages(candidate.Messages)
	if err := validateToolPairing(candidate.Messages); err != nil {
		return nil, nil, decision, fmt.Errorf("validate compacted tool pairing: %w", err)
	}
	candidateEstimate := m.estimator.EstimateChatRequest(candidate)
	if candidateEstimate.Tokens >= decision.Budget.PromptBudget {
		return nil, nil, decision, fmt.Errorf("compacted request still exceeds 80%% of the model context window: after=%d limit=%d", candidateEstimate.Tokens, decision.Budget.PromptBudget)
	}
	compactedThrough := compactedRoundThroughMessagePrefix(rounds, preserveStart)
	if m.state.Summary != nil {
		compactedThrough = max(compactedThrough, m.state.Summary.CompactedThroughRound)
	}
	newSummary := &ContextSummary{Content: contentString(summaryMessage.Content), CompactedThroughRound: compactedThrough, CreatedAt: time.Now().UTC()}
	m.state.Messages = cloneMessages(candidate.Messages)
	m.state.Summary = newSummary
	decision.Action = compactDecision.Action
	decision.LossyRecoveryDroppedRounds = lossyDroppedBatches
	decision.LossyRecoveryDroppedBatches = lossyDroppedBatches
	decision.PreservedMessageCount = len(tail)
	decision.AfterTokens = candidateEstimate.Tokens
	decision.FinalPromptTokens = candidateEstimate.Tokens
	decision.Estimator = candidateEstimate.Tokenizer
	decision.ComponentTokens = componentTokenMap(candidateEstimate)
	decision.CompactedThroughRound = newSummary.CompactedThroughRound
	decision.PreservedRounds = preservedRoundSequences(rounds, preserveStart)
	decision.Budget.ContextPressure = float64(candidateEstimate.Tokens) / float64(decision.Budget.PromptBudget)
	return candidate, newSummary, decision, nil
}

func (m *Manager) decisionForRequest(request *adapter.ChatRequest, base Decision) Decision {
	estimate := m.estimator.EstimateChatRequest(request)
	base.BeforeTokens = estimate.Tokens
	base.AfterTokens = estimate.Tokens
	base.FinalPromptTokens = estimate.Tokens
	base.Estimator = estimate.Tokenizer
	base.ComponentTokens = componentTokenMap(estimate)
	base.FixedRequestTokens, base.CompressibleTokens = m.requestTokenClasses(request)
	if base.Budget.PromptBudget > 0 {
		base.Budget.ContextPressure = float64(estimate.Tokens) / float64(base.Budget.PromptBudget)
	}
	return base
}

func dropOldestCompactBatch(request *adapter.ChatRequest) bool {
	if request == nil || len(request.Messages) < 2 {
		return false
	}
	prompt := cloneMessage(request.Messages[len(request.Messages)-1])
	prefixBatches := groupMessagesByCompactionBatch(request.Messages[:len(request.Messages)-1])
	if len(prefixBatches) <= 1 {
		return false
	}
	request.Messages = flattenCompactionBatches(prefixBatches[1:])
	request.Messages = append(request.Messages, prompt)
	return true
}

func isPromptTooLong(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"prompt_too_long",
		"context_length_exceeded",
		"maximum context length",
		"context window exceeded",
		"input is too long",
		"too many input tokens",
		"request too large for model",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func compactedRoundThroughMessagePrefix(rounds []apiRound, messageCount int) int {
	consumed := 0
	through := 0
	for _, round := range rounds {
		if consumed >= messageCount {
			break
		}
		remaining := messageCount - consumed
		if remaining > 0 && round.Sequence > 0 {
			through = max(through, round.Sequence)
		}
		consumed += min(remaining, len(round.Messages))
	}
	return through
}

func preservedRoundSequences(rounds []apiRound, messageStart int) []int {
	sequences := make([]int, 0)
	consumed := 0
	for _, round := range rounds {
		end := consumed + len(round.Messages)
		if end > messageStart && round.Sequence > 0 {
			sequences = append(sequences, round.Sequence)
		}
		consumed = end
	}
	return sequences
}

func splitSystemMessages(messages []adapter.Message) ([]adapter.Message, []adapter.Message) {
	systems := make([]adapter.Message, 0)
	dialogue := make([]adapter.Message, 0)
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			systems = append(systems, cloneMessage(message))
		} else {
			dialogue = append(dialogue, cloneMessage(message))
		}
	}
	return systems, dialogue
}

func (m *Manager) requestTokenClasses(request *adapter.ChatRequest) (int, int) {
	systems, dialogue := splitSystemMessages(request.Messages)
	fixedMessages := cloneMessages(systems)
	for index := len(dialogue) - 1; index >= 0; index-- {
		if strings.EqualFold(strings.TrimSpace(dialogue[index].Role), "user") && !strings.HasPrefix(contentString(dialogue[index].Content), "This session is being continued") {
			fixedMessages = append(fixedMessages, cloneMessage(dialogue[index]))
			break
		}
	}
	fixed := cloneRequest(request)
	fixed.Messages = adapter.NormalizeSystemMessages(fixedMessages)
	fixedTokens := m.estimator.EstimateChatRequest(fixed).Tokens
	fullTokens := m.estimator.EstimateChatRequest(request).Tokens
	return fixedTokens, max(0, fullTokens-fixedTokens)
}

func cloneState(state AgentContextState) AgentContextState {
	state.Messages = cloneMessages(state.Messages)
	state.TurnTranscript = cloneMessages(state.TurnTranscript)
	if state.Summary != nil {
		summary := *state.Summary
		state.Summary = &summary
	}
	if state.LastUsage != nil {
		usage := *state.LastUsage
		state.LastUsage = &usage
	}
	replacements := state.ContentReplacements
	state.ContentReplacements = make(map[string]ContentReplacement, len(replacements))
	for key, value := range replacements {
		state.ContentReplacements[key] = value
	}
	return state
}

func componentTokenMap(estimate tokenestimate.ChatRequestResult) map[string]int {
	components := make(map[string]int, len(estimate.Components))
	for key, value := range estimate.Components {
		components[key] = value.Tokens
	}
	return components
}

func contentString(content interface{}) string {
	if text, ok := content.(string); ok {
		return text
	}
	encoded, err := json.Marshal(content)
	if err == nil {
		return string(encoded)
	}
	return fmt.Sprint(content)
}

func mergeUsage(left *adapter.Usage, right *adapter.Usage) *adapter.Usage {
	if left == nil && right == nil {
		return nil
	}
	merged := &adapter.Usage{}
	if left != nil {
		*merged = *left
	}
	if right != nil {
		merged.PromptTokens += right.PromptTokens
		merged.CompletionTokens += right.CompletionTokens
		merged.TotalTokens += right.TotalTokens
	}
	return merged
}

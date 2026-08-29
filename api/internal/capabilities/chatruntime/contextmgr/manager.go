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
	"unicode/utf8"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

const maxConsecutiveCompactionFailures = 3

type Manager struct {
	mu           sync.Mutex
	config       Config
	estimator    *tokenestimate.Estimator
	compactor    Compactor
	toolStore    ToolResultStore
	observer     RequestObserver
	state        AgentContextState
	pendingUsage *adapter.Usage
}

func New(config Config, compactor Compactor, toolStore ToolResultStore, observer RequestObserver) (*Manager, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		config:    normalized,
		estimator: tokenestimate.NewEstimator(),
		compactor: compactor,
		toolStore: toolStore,
		observer:  observer,
		state: AgentContextState{
			SchemaVersion:       1,
			AgentRunID:          strings.TrimSpace(normalized.AgentRunID),
			NextRound:           1,
			ContentReplacements: map[string]ContentReplacement{},
			EstimatorScale:      1,
			CreatedAt:           time.Now().UTC(),
		},
	}
	if manager.state.AgentRunID == "" {
		return nil, fmt.Errorf("agent run id is required")
	}
	return manager, nil
}

func (m *Manager) PrepareBeforeModelCall(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatRequest, Decision, error) {
	if m == nil || request == nil {
		return nil, Decision{}, fmt.Errorf("context manager request is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	prepared := cloneRequest(request)
	prepared.Messages = adapter.NormalizeSystemMessages(prepared.Messages)
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
		EstimateScale: 1,
	}

	projected, projection, err := m.projectOversizedToolResults(ctx, prepared.Messages)
	if err != nil {
		return nil, decision, err
	}
	prepared.Messages = adapter.NormalizeSystemMessages(projected)
	m.syncTurnTranscriptMessages(prepared.Messages)
	decision.ToolResultOriginalTokens = projection.originalTokens
	decision.ToolResultProjectedTokens = projection.projectedTokens
	decision.ToolProjectionCount = projection.count
	if projection.count > 0 {
		decision.Action = DecisionToolProjection
	}

	estimate := m.estimator.EstimateChatRequest(prepared)
	decision.BeforeTokens = estimate.Tokens
	if estimate.Tokens >= budget.SoftLimit {
		compactedTools, changed := m.microcompactOldToolResults(ctx, prepared.Messages)
		if changed {
			prepared.Messages = adapter.NormalizeSystemMessages(compactedTools)
			m.syncTurnTranscriptMessages(prepared.Messages)
			estimate = m.estimator.EstimateChatRequest(prepared)
			decision.Action = DecisionMicrocompact
		}
	}
	if err := validateToolPairing(prepared.Messages); err != nil {
		return nil, decision, fmt.Errorf("validate model request tool pairing: %w", err)
	}

	if estimate.Tokens >= budget.SoftLimit && m.compactor != nil && m.state.Compaction.ConsecutiveFailures < maxConsecutiveCompactionFailures {
		beforeSemantic := cloneRequest(prepared)
		compacted, summary, compactDecision, compactErr := m.semanticCompact(ctx, prepared, decision, false)
		if compactErr != nil {
			m.state.Compaction.ConsecutiveFailures++
			m.state.Compaction.LastFailure = compactErr.Error()
			decision.CompactionFailure = compactErr.Error()
			decision.ConsecutiveCompactionFailures = m.state.Compaction.ConsecutiveFailures
			prepared = beforeSemantic
		} else {
			prepared = compacted
			decision = compactDecision
			m.state.Summary = summary
			m.state.Compaction.ConsecutiveFailures = 0
			m.state.Compaction.LastFailure = ""
			m.state.Compaction.LastCompactedAt = time.Now().UTC()
		}
		estimate = m.estimator.EstimateChatRequest(prepared)
	}
	if estimate.Tokens > budget.HardLimit {
		fitted, changed, projectedCount, fitErr := m.fitPendingToolBatchPreviews(ctx, prepared, budget.HardLimit)
		if fitErr != nil {
			return nil, decision, fitErr
		}
		if changed {
			prepared = fitted
			estimate = m.estimator.EstimateChatRequest(prepared)
			decision.Action = DecisionMicrocompact
			decision.ToolProjectionCount += projectedCount
		}
	}
	var finalRecoveryErr error
	if estimate.Tokens > budget.HardLimit && m.compactor != nil {
		beforeRecovery := cloneRequest(prepared)
		recovered, summary, recoveryDecision, recoveryErr := m.semanticCompact(ctx, prepared, decision, true)
		if recoveryErr != nil {
			finalRecoveryErr = recoveryErr
			m.state.Compaction.LastFailure = recoveryErr.Error()
			decision.CompactionFailure = recoveryErr.Error()
			prepared = beforeRecovery
		} else {
			prepared = recovered
			decision = recoveryDecision
			decision.CompactionFailure = ""
			m.state.Summary = summary
			m.state.Compaction.ConsecutiveFailures = 0
			m.state.Compaction.LastFailure = ""
			m.state.Compaction.LastCompactedAt = time.Now().UTC()
		}
		estimate = m.estimator.EstimateChatRequest(prepared)
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
	if estimate.Tokens > budget.HardLimit {
		m.state.Messages = cloneMessages(prepared.Messages)
		if finalRecoveryErr != nil {
			return nil, decision, fmt.Errorf("%w: final recovery compaction failed: %v; prompt=%d hard_limit=%d", ErrContextExhausted, finalRecoveryErr, estimate.Tokens, budget.HardLimit)
		}
		return nil, decision, fmt.Errorf("%w: final model request exceeds Agent hard context limit: prompt=%d hard_limit=%d", ErrContextExhausted, estimate.Tokens, budget.HardLimit)
	}

	m.state.Messages = cloneMessages(prepared.Messages)
	return prepared, decision, nil
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
	return compacted, decision, nil
}

func (m *Manager) ObserveModelResponse(_ context.Context, message adapter.Message, usage *adapter.Usage) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	message.ReasoningContent = ""
	message = cloneMessage(message)
	m.state.Messages = append(m.state.Messages, message)
	if strings.EqualFold(strings.TrimSpace(message.Role), "assistant") {
		m.state.TurnTranscript = append(m.state.TurnTranscript, cloneMessage(message))
	}
	if m.state.NextRound <= 0 {
		m.state.NextRound = 1
	}
	m.state.NextRound++
	if usage != nil {
		cloned := *usage
		m.state.LastUsage = &cloned
	}
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

func (m *Manager) ReplaceMessages(_ context.Context, messages []adapter.Message) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendNewTurnToolMessages(messages)
	m.syncTurnTranscriptMessages(messages)
	m.state.Messages = cloneMessages(messages)
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
			message.ReasoningContent = ""
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
		if result.Tokens <= m.config.MaxToolResultTokens {
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

// fitPendingToolBatchPreviews is the hard-limit fallback for an unusually
// large parallel batch. It externalizes every result in the pending batch and
// applies one shared preview cap, so earlier siblings are not discarded merely
// because they appeared first in provider order.
func (m *Manager) fitPendingToolBatchPreviews(ctx context.Context, request *adapter.ChatRequest, tokenLimit int) (*adapter.ChatRequest, bool, int, error) {
	if request == nil || tokenLimit <= 0 {
		return request, false, 0, nil
	}
	protected := latestPendingToolBatchIndexes(request.Messages)
	if len(protected) == 0 {
		return request, false, 0, nil
	}
	base := cloneRequest(request)
	previews := make([]pendingToolPreview, 0, len(protected))
	newlyProjected := 0
	for index := range protected {
		message := base.Messages[index]
		if _, _, expanded := decodeContextArtifactToolResult(contentString(message.Content)); expanded {
			// Expanded artifact content must reach the model verbatim at least
			// once. It cannot be projected into another artifact recursively.
			continue
		}
		receipt, ok := decodeToolResultReceipt(contentString(message.Content))
		if !ok {
			replacement, err := m.toolReplacement(ctx, message, false)
			if err != nil {
				return nil, false, newlyProjected, err
			}
			message.Content = replacement.Replacement
			base.Messages[index] = message
			receipt, ok = decodeToolResultReceipt(replacement.Replacement)
			if !ok {
				return nil, false, newlyProjected, fmt.Errorf("decode projected pending tool result")
			}
			newlyProjected++
		}
		if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(receipt["status"])), "projected") {
			continue
		}
		preview := strings.TrimSpace(fmt.Sprint(receipt["preview"]))
		if preview == "" || preview == "<nil>" {
			continue
		}
		previews = append(previews, pendingToolPreview{messageIndex: index, receipt: receipt, preview: preview})
	}
	if len(previews) == 0 {
		return request, false, newlyProjected, nil
	}
	before := m.estimator.EstimateChatRequest(request).Tokens
	baseEstimate := m.estimator.EstimateChatRequest(base).Tokens
	selected := base
	if baseEstimate > tokenLimit {
		maxPreviewRunes := 0
		for _, item := range previews {
			maxPreviewRunes = max(maxPreviewRunes, utf8.RuneCountInString(item.preview))
		}
		low, high := 0, maxPreviewRunes
		selected = m.requestWithPendingPreviewCap(base, previews, 0)
		if m.estimator.EstimateChatRequest(selected).Tokens <= tokenLimit {
			for low < high {
				middle := low + (high-low+1)/2
				candidate := m.requestWithPendingPreviewCap(base, previews, middle)
				if m.estimator.EstimateChatRequest(candidate).Tokens <= tokenLimit {
					low = middle
					selected = candidate
				} else {
					high = middle - 1
				}
			}
		}
	}
	after := m.estimator.EstimateChatRequest(selected).Tokens
	if after >= before {
		return request, false, newlyProjected, nil
	}
	m.rememberPendingPreviewReplacements(selected.Messages, previews)
	return selected, true, newlyProjected, nil
}

func (m *Manager) requestWithPendingPreviewCap(base *adapter.ChatRequest, previews []pendingToolPreview, previewRunes int) *adapter.ChatRequest {
	candidate := cloneRequest(base)
	for _, item := range previews {
		receipt := make(map[string]interface{}, len(item.receipt))
		for key, value := range item.receipt {
			receipt[key] = value
		}
		preview := boundedToolPreview(item.preview, previewRunes)
		if preview == "" {
			delete(receipt, "preview")
		} else {
			receipt["preview"] = preview
		}
		encoded, err := json.Marshal(receipt)
		if err != nil {
			continue
		}
		candidate.Messages[item.messageIndex].Content = string(encoded)
	}
	return candidate
}

func boundedToolPreview(preview string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(preview)
	if len(runes) <= limit {
		return preview
	}
	return string(runes[:limit]) + "\n...[truncated]"
}

func (m *Manager) rememberPendingPreviewReplacements(messages []adapter.Message, previews []pendingToolPreview) {
	for _, item := range previews {
		if item.messageIndex < 0 || item.messageIndex >= len(messages) {
			continue
		}
		receipt, ok := decodeToolResultReceipt(contentString(messages[item.messageIndex].Content))
		if !ok {
			continue
		}
		toolCallID := strings.TrimSpace(fmt.Sprint(receipt["tool_call_id"]))
		contentHash := strings.TrimSpace(fmt.Sprint(receipt["content_hash"]))
		if toolCallID == "" || contentHash == "" || toolCallID == "<nil>" || contentHash == "<nil>" {
			continue
		}
		key := toolCallID + ":" + contentHash
		replacement := m.state.ContentReplacements[key]
		replacement.ToolCallID = toolCallID
		replacement.ContentHash = contentHash
		replacement.ArtifactRef = strings.TrimSpace(fmt.Sprint(receipt["artifact_ref"]))
		if replacement.ArtifactRef == "<nil>" {
			replacement.ArtifactRef = ""
		}
		replacement.OriginalTokens = intFromJSONNumber(receipt["original_tokens"])
		preview := strings.TrimSpace(fmt.Sprint(receipt["preview"]))
		if preview == "<nil>" {
			preview = ""
		}
		replacement.PreviewTokens = m.estimator.EstimateText(preview, m.state.Model).Tokens
		replacement.Replacement = contentString(messages[item.messageIndex].Content)
		m.state.ContentReplacements[key] = replacement
	}
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
	}
	preview := truncateRunes(content, m.config.ToolResultPreviewRunes)
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
	systems, dialogue := splitSystemMessages(request.Messages)
	rounds := groupMessagesByAPIRoundForRun(dialogue, m.state.NextRound)
	if len(rounds) < 2 {
		return nil, nil, decision, fmt.Errorf("no complete API-round prefix is available for compaction")
	}
	minimumTailRounds := m.config.TailMinTextRounds
	if reactive {
		minimumTailRounds = 1
	}
	preserveStart := selectPreservedTail(rounds, decision.Budget, minimumTailRounds, m.estimator, request.Model)
	if preserveStart <= 0 || preserveStart >= len(rounds) {
		return nil, nil, decision, fmt.Errorf("no safe API-round compaction boundary is available")
	}
	prefix := flattenRounds(rounds[:preserveStart])
	tail := flattenRounds(rounds[preserveStart:])
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

	lossyDroppedRounds := 0
	for attempt := 0; attempt < 3 && m.estimator.EstimateChatRequest(compactRequest).Tokens > decision.Budget.CompactInputLimit; attempt++ {
		prefixRounds := groupMessagesByAPIRound(compactRequest.Messages[:len(compactRequest.Messages)-1])
		if len(prefixRounds) <= 1 {
			break
		}
		compactRequest.Messages = flattenRounds(prefixRounds[1:])
		compactRequest.Messages = append(compactRequest.Messages, adapter.Message{Role: "user", Content: GetPartialCompactPrompt("", "up_to")})
		lossyDroppedRounds++
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
	compactDecision.LossyRecoveryDroppedRounds = lossyDroppedRounds
	var rawSummary string
	var compactErr error
	for attempt := 0; attempt < 3; attempt++ {
		attemptDecision := m.decisionForRequest(compactRequest, compactDecision)
		if m.observer != nil {
			m.observer(callType, decision.APIRound, cloneRequest(compactRequest), attemptDecision)
		}
		var compactUsage *adapter.Usage
		rawSummary, compactUsage, compactErr = m.compactor.Compact(ctx, compactRequest, CompactCall{AgentRunID: m.state.AgentRunID, APIRound: decision.APIRound, Type: callType, Decision: attemptDecision})
		m.pendingUsage = mergeUsage(m.pendingUsage, compactUsage)
		if compactErr == nil {
			break
		}
		if !isPromptTooLong(compactErr) || attempt == 2 || !dropOldestCompactRound(compactRequest) {
			return nil, nil, decision, compactErr
		}
		lossyDroppedRounds++
		compactDecision.LossyRecoveryDroppedRounds = lossyDroppedRounds
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
	before := m.estimator.EstimateChatRequest(request).Tokens
	meaningfulSavings := before-candidateEstimate.Tokens >= m.config.HysteresisTokens
	if candidateEstimate.Tokens >= before || candidateEstimate.Tokens >= decision.Budget.SoftLimit || (candidateEstimate.Tokens > decision.Budget.TargetTokens && !meaningfulSavings) {
		return nil, nil, decision, fmt.Errorf("compacted request did not reach a safe lower token count: before=%d after=%d target=%d soft_limit=%d", before, candidateEstimate.Tokens, decision.Budget.TargetTokens, decision.Budget.SoftLimit)
	}
	compactedThrough := rounds[preserveStart-1].Sequence
	if m.state.Summary != nil {
		compactedThrough = max(compactedThrough, m.state.Summary.CompactedThroughRound)
	}
	newSummary := &ContextSummary{Content: contentString(summaryMessage.Content), CompactedThroughRound: compactedThrough, CreatedAt: time.Now().UTC()}
	m.state.Messages = cloneMessages(candidate.Messages)
	m.state.Summary = newSummary
	decision.Action = compactDecision.Action
	decision.LossyRecoveryDroppedRounds = lossyDroppedRounds
	decision.AfterTokens = candidateEstimate.Tokens
	decision.FinalPromptTokens = candidateEstimate.Tokens
	decision.Estimator = candidateEstimate.Tokenizer
	decision.ComponentTokens = componentTokenMap(candidateEstimate)
	decision.CompactedThroughRound = newSummary.CompactedThroughRound
	for _, round := range rounds[preserveStart:] {
		if round.Sequence > 0 {
			decision.PreservedRounds = append(decision.PreservedRounds, round.Sequence)
		}
	}
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

func dropOldestCompactRound(request *adapter.ChatRequest) bool {
	if request == nil || len(request.Messages) < 2 {
		return false
	}
	prompt := cloneMessage(request.Messages[len(request.Messages)-1])
	prefixRounds := groupMessagesByAPIRound(request.Messages[:len(request.Messages)-1])
	if len(prefixRounds) <= 1 {
		return false
	}
	request.Messages = flattenRounds(prefixRounds[1:])
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

func selectPreservedTail(rounds []apiRound, budget Budget, minimumRounds int, estimator *tokenestimate.Estimator, model string) int {
	if len(rounds) <= 1 {
		return 0
	}
	preserveStart := len(rounds) - 1
	tokens := estimator.EstimateMessages(rounds[preserveStart].Messages, model).Tokens
	preserved := 1
	for preserveStart > 1 && (preserved < minimumRounds || tokens < budget.TailMinTokens) {
		candidateTokens := estimator.EstimateMessages(rounds[preserveStart-1].Messages, model).Tokens
		if preserved >= minimumRounds && tokens+candidateTokens > budget.TailMaxTokens {
			break
		}
		preserveStart--
		preserved++
		tokens += candidateTokens
	}
	return preserveStart
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

func flattenRounds(rounds []apiRound) []adapter.Message {
	messages := make([]adapter.Message, 0)
	for _, round := range rounds {
		messages = append(messages, cloneMessages(round.Messages)...)
	}
	return messages
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

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "\n...[truncated]"
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

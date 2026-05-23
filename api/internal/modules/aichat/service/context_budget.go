package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

const (
	contextBudgetSafetyNumerator   = 9
	contextBudgetSafetyDenominator = 10
	maxContextCandidateMessages    = 100

	contextControlStrategyTokenBudget = "token_budget"

	recentToolHistoryTurnLimit          = 1
	recentIntermediateAnswerTurnLimit   = 3
	recentExecutionContextBudgetChars   = 12000
	recentIntermediateAnswerBudgetChars = 4000
	recentTraceArgumentBudgetChars      = 600
)

type contextBudgetResult struct {
	Messages []adapter.Message
	Metadata map[string]interface{}
}

type budgetComputation struct {
	SafeLimit            int
	PromptBudget         int
	ReservedOutputTokens int
	BasePromptTokens     int
	OriginalMaxTokens    *int
	EffectiveMaxTokens   *int
	MaxTokensClamped     bool
	Tokenizer            string
}

func (s *service) buildTokenBudgetMessages(
	ctx context.Context,
	spec ModelSpec,
	parts *chatRequestParts,
	systemPrompt string,
	parentMessages []*aichatmodel.Message,
) (*contextBudgetResult, error) {
	baseMessages := []adapter.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: s.currentUserContent(parts, parts.Query)},
	}
	recentExecutionContext, recentExecutionMetadata := buildRecentExecutionContextMessage(parentMessages)
	if recentExecutionContext != nil {
		baseMessages = []adapter.Message{
			{Role: "system", Content: systemPrompt},
			*recentExecutionContext,
			{Role: "user", Content: s.currentUserContent(parts, parts.Query)},
		}
	}
	budget, err := s.computeContextBudget(spec, parts, baseMessages)
	if err != nil {
		return nil, err
	}
	recentExecutionTokens := 0
	if recentExecutionContext != nil {
		recentExecutionTokens = s.tokenEstimator.EstimateMessages([]adapter.Message{*recentExecutionContext}, parts.ModelName).Tokens
	}
	currentContent, attachmentMetadata, estimatedPromptTokens := s.buildBudgetedCurrentUserContent(parts, systemPrompt, budget, recentExecutionTokens)

	groups := s.historyMessageGroups(ctx, parentMessages, parts.ModelSupportsVision)
	historyBefore := countAdapterMessages(groups)
	selected := make([][]adapter.Message, 0, len(groups))
	for i := len(groups) - 1; i >= 0; i-- {
		groupTokens := s.tokenEstimator.EstimateMessages(groups[i], parts.ModelName).Tokens
		if estimatedPromptTokens+groupTokens > budget.PromptBudget {
			break
		}
		selected = append(selected, groups[i])
		estimatedPromptTokens += groupTokens
	}

	messages := make([]adapter.Message, 0, 2+historyBefore)
	messages = append(messages, adapter.Message{Role: "system", Content: systemPrompt})
	for i := len(selected) - 1; i >= 0; i-- {
		messages = append(messages, selected[i]...)
	}
	if recentExecutionContext != nil {
		messages = append(messages, *recentExecutionContext)
	}
	messages = append(messages, adapter.Message{Role: "user", Content: currentContent})

	historyAfter := len(messages) - 2
	if recentExecutionContext != nil {
		historyAfter--
	}
	metadata := contextControlMetadata(spec, budget, estimatedPromptTokens, historyBefore, historyAfter)
	mergeAttachmentContextMetadata(metadata, attachmentMetadata)
	mergeRecentExecutionContextMetadata(metadata, recentExecutionMetadata)
	return &contextBudgetResult{
		Messages: messages,
		Metadata: metadata,
	}, nil
}

func (s *service) buildBudgetedCurrentUserContent(
	parts *chatRequestParts,
	systemPrompt string,
	budget *budgetComputation,
	extraPromptTokens int,
) (interface{}, map[string]interface{}, int) {
	promptBudget := budget.PromptBudget - extraPromptTokens
	if promptBudget < 0 {
		promptBudget = 0
	}
	if parts == nil || parts.Attachments == nil || len(parts.Attachments.Files) == 0 {
		currentContent := s.currentUserContent(parts, parts.Query)
		return currentContent, nil, s.estimateCurrentPromptTokens(systemPrompt, currentContent, parts.ModelName) + extraPromptTokens
	}

	fullSections := parts.Attachments.fullContentSections()
	if strings.TrimSpace(fullSections) == "" {
		currentContent := s.currentUserContent(parts, parts.Query)
		return currentContent, nil, s.estimateCurrentPromptTokens(systemPrompt, currentContent, parts.ModelName) + extraPromptTokens
	}

	attachmentTokensBefore := s.estimateAttachmentTokens(parts, fullSections)
	fullContent := userContentWithAttachments(parts.Query, fullSections)
	fullUserContent := s.currentUserContent(parts, fullContent)
	fullEstimate := s.estimateCurrentPromptTokens(systemPrompt, fullUserContent, parts.ModelName) + extraPromptTokens
	if fullEstimate <= budget.PromptBudget {
		return fullUserContent, map[string]interface{}{
			"attachment_tokens_before": attachmentTokensBefore,
			"attachment_tokens_after":  attachmentTokensBefore,
			"attachments_truncated":    false,
		}, fullEstimate
	}

	sections, truncated := s.fitAttachmentSectionsToBudget(parts, systemPrompt, promptBudget)
	currentContent := userContentWithAttachments(parts.Query, sections)
	currentUserContent := s.currentUserContent(parts, currentContent)
	estimatedPromptTokens := s.estimateCurrentPromptTokens(systemPrompt, currentUserContent, parts.ModelName) + extraPromptTokens
	attachmentTokensAfter := s.estimateAttachmentTokens(parts, sections)
	return currentUserContent, map[string]interface{}{
		"attachment_tokens_before": attachmentTokensBefore,
		"attachment_tokens_after":  attachmentTokensAfter,
		"attachments_truncated":    truncated || attachmentTokensAfter < attachmentTokensBefore,
	}, estimatedPromptTokens
}

func (s *service) fitAttachmentSectionsToBudget(parts *chatRequestParts, systemPrompt string, promptBudget int) (string, bool) {
	selected := make([]attachmentFile, 0, len(parts.Attachments.Files))
	for _, file := range parts.Attachments.Files {
		candidate := appendAttachmentFile(selected, file)
		sections := formatAttachmentSections(candidate, func(item attachmentFile) string {
			return item.Content
		})
		content := userContentWithAttachments(parts.Query, sections)
		if s.estimateCurrentPromptTokens(systemPrompt, s.currentUserContent(parts, content), parts.ModelName) <= promptBudget {
			selected = candidate
			continue
		}

		partial, ok := s.truncateAttachmentFileToBudget(parts, systemPrompt, promptBudget, selected, file)
		if ok {
			selected = appendAttachmentFile(selected, partial)
		}
		return formatAttachmentSections(selected, func(item attachmentFile) string {
			return item.Content
		}), true
	}
	return formatAttachmentSections(selected, func(item attachmentFile) string {
		return item.Content
	}), false
}

func (s *service) truncateAttachmentFileToBudget(
	parts *chatRequestParts,
	systemPrompt string,
	promptBudget int,
	selected []attachmentFile,
	file attachmentFile,
) (attachmentFile, bool) {
	runes := []rune(file.Content)
	low, high := 0, len(runes)
	best := -1
	for low <= high {
		mid := (low + high) / 2
		partial := file
		partial.Content = string(runes[:mid])
		candidate := appendAttachmentFile(selected, partial)
		sections := formatAttachmentSections(candidate, func(item attachmentFile) string {
			return item.Content
		})
		content := userContentWithAttachments(parts.Query, sections)
		if s.estimateCurrentPromptTokens(systemPrompt, s.currentUserContent(parts, content), parts.ModelName) <= promptBudget {
			best = mid
			low = mid + 1
			continue
		}
		high = mid - 1
	}
	if best < 0 {
		return attachmentFile{}, false
	}
	partial := file
	partial.Content = string(runes[:best])
	return partial, true
}

func (s *service) estimateCurrentPromptTokens(systemPrompt string, currentContent interface{}, modelName string) int {
	return s.tokenEstimator.EstimateMessages([]adapter.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: currentContent},
	}, modelName).Tokens
}

func (s *service) estimateAttachmentTokens(parts *chatRequestParts, sections string) int {
	if strings.TrimSpace(sections) == "" {
		return 0
	}
	return s.tokenEstimator.EstimateMessages([]adapter.Message{
		{Role: "user", Content: sections},
	}, parts.ModelName).Tokens
}

func (s *service) buildFallbackCurrentUserContent(parts *chatRequestParts) (interface{}, map[string]interface{}) {
	if parts == nil || parts.Attachments == nil || len(parts.Attachments.Files) == 0 {
		return s.currentUserContent(parts, parts.Query), nil
	}

	selected := make([]attachmentFile, 0, len(parts.Attachments.Files))
	remaining := fallbackAttachmentContextRuneLimit
	truncated := false
	for _, file := range parts.Attachments.Files {
		contentRunes := []rune(file.Content)
		if remaining <= 0 {
			truncated = true
			break
		}
		partial := file
		if len(contentRunes) > remaining {
			partial.Content = string(contentRunes[:remaining])
			truncated = true
		}
		selected = append(selected, partial)
		remaining -= len([]rune(partial.Content))
	}

	sections := formatAttachmentSections(selected, func(item attachmentFile) string {
		return item.Content
	})
	content := userContentWithAttachments(parts.Query, sections)
	if !truncated {
		return s.currentUserContent(parts, content), nil
	}
	fullSections := parts.Attachments.fullContentSections()
	return s.currentUserContent(parts, content), map[string]interface{}{
		"strategy":                 "message_limit",
		"attachments_truncated":    true,
		"attachment_tokens_before": s.estimateAttachmentTokens(parts, fullSections),
		"attachment_tokens_after":  s.estimateAttachmentTokens(parts, sections),
	}
}

func appendAttachmentFile(files []attachmentFile, file attachmentFile) []attachmentFile {
	output := make([]attachmentFile, 0, len(files)+1)
	output = append(output, files...)
	output = append(output, file)
	return output
}

func mergeAttachmentContextMetadata(target map[string]interface{}, source map[string]interface{}) {
	if target == nil || source == nil {
		return
	}
	for key, value := range source {
		target[key] = value
	}
}

func (s *service) computeContextBudget(spec ModelSpec, parts *chatRequestParts, baseMessages []adapter.Message) (*budgetComputation, error) {
	if spec.ContextWindow <= 0 {
		return nil, fmt.Errorf("%w: model context_window is required", ErrInvalidInput)
	}
	safeLimit := spec.ContextWindow * contextBudgetSafetyNumerator / contextBudgetSafetyDenominator
	minOutput := minOutputReserve(spec.ContextWindow)
	if safeLimit <= minOutput {
		return nil, fmt.Errorf("%w: model context budget is too small", ErrInvalidInput)
	}

	baseEstimate := s.tokenEstimator.EstimateMessages(baseMessages, parts.ModelName)
	minPrompt := minPromptBudget(spec.ContextWindow)
	maxMinPrompt := safeLimit - minOutput
	if minPrompt > maxMinPrompt {
		minPrompt = maxMinPrompt
	}
	if minPrompt < baseEstimate.Tokens {
		minPrompt = baseEstimate.Tokens
	}

	maxAllowedOutput := safeLimit - minPrompt
	if maxAllowedOutput < minOutput {
		return nil, fmt.Errorf("%w: query exceeds model context budget", ErrInvalidInput)
	}

	originalMaxTokens, hasRequestedMaxTokens := requestedMaxTokens(parts)
	desiredOutput := defaultOutputReserve(spec.ContextWindow)
	if hasRequestedMaxTokens {
		desiredOutput = *originalMaxTokens
	}
	if spec.MaxOutputTokens > 0 && desiredOutput > spec.MaxOutputTokens {
		desiredOutput = spec.MaxOutputTokens
	}
	if desiredOutput < 0 {
		desiredOutput = 0
	}

	reservedOutput := desiredOutput
	if reservedOutput > maxAllowedOutput {
		reservedOutput = maxAllowedOutput
	}
	if reservedOutput < minOutput {
		return nil, fmt.Errorf("%w: query exceeds model context budget", ErrInvalidInput)
	}

	effectiveMaxTokens := originalMaxTokens
	maxTokensClamped := false
	if hasRequestedMaxTokens && reservedOutput < *originalMaxTokens {
		value := reservedOutput
		effectiveMaxTokens = &value
		parts.Parameters["max_tokens"] = value
		maxTokensClamped = true
	}

	promptBudget := safeLimit - reservedOutput
	if spec.MaxInputTokens > 0 && promptBudget > spec.MaxInputTokens {
		promptBudget = spec.MaxInputTokens
	}
	if baseEstimate.Tokens > promptBudget {
		return nil, fmt.Errorf("%w: query exceeds model context budget", ErrInvalidInput)
	}

	return &budgetComputation{
		SafeLimit:            safeLimit,
		PromptBudget:         promptBudget,
		ReservedOutputTokens: reservedOutput,
		BasePromptTokens:     baseEstimate.Tokens,
		OriginalMaxTokens:    originalMaxTokens,
		EffectiveMaxTokens:   effectiveMaxTokens,
		MaxTokensClamped:     maxTokensClamped,
		Tokenizer:            baseEstimate.Tokenizer,
	}, nil
}

func (s *service) historyMessageGroups(ctx context.Context, branch []*aichatmodel.Message, includeImages bool) [][]adapter.Message {
	groups := make([][]adapter.Message, 0, len(branch))
	for _, item := range branch {
		if item == nil {
			continue
		}
		group := make([]adapter.Message, 0, 2)
		userMessage := s.historicalUserMessage(ctx, item, includeImages)
		if userMessage != nil {
			group = append(group, *userMessage)
		}
		if isUsableAssistantHistoryStatus(item.Status) && item.Answer != "" {
			group = append(group, adapter.Message{Role: "assistant", Content: item.Answer})
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	return groups
}

type recentExecutionContextStats struct {
	ToolHistoryTurns          int
	IntermediateAnswerTurns   int
	IncludedToolEvents        int
	IncludedIntermediate      int
	ToolHistoryTruncated      bool
	IntermediateTruncated     bool
	ExecutionContextTruncated bool
}

func buildRecentExecutionContextMessage(branch []*aichatmodel.Message) (*adapter.Message, recentExecutionContextStats) {
	stats := recentExecutionContextStats{}
	if len(branch) == 0 {
		return nil, stats
	}

	var builder strings.Builder
	builder.WriteString("Recent AIChat execution context for continuity.\n")
	builder.WriteString("Older turns are represented only by their final assistant answers in the conversation history.\n")
	builder.WriteString("Use these notes as context; do not mention these storage rules to the user.\n")
	builder.WriteString("Do not resubmit these notes as intermediate answers; reuse them directly for export, save, convert, or file-generation requests.\n")

	remaining := recentExecutionContextBudgetChars - builder.Len()
	toolSection, toolStats := recentToolHistorySection(branch, remaining)
	stats.ToolHistoryTurns = toolStats.ToolHistoryTurns
	stats.IncludedToolEvents = toolStats.IncludedToolEvents
	stats.ToolHistoryTruncated = toolStats.ToolHistoryTruncated
	stats.ExecutionContextTruncated = stats.ExecutionContextTruncated || toolStats.ExecutionContextTruncated
	if toolSection != "" {
		builder.WriteString("\nMost recent skill/tool history:\n")
		builder.WriteString(toolSection)
	}

	remaining = recentExecutionContextBudgetChars - builder.Len()
	intermediateSection, intermediateStats := recentIntermediateAnswerSection(branch, remaining)
	stats.IntermediateAnswerTurns = intermediateStats.IntermediateAnswerTurns
	stats.IncludedIntermediate = intermediateStats.IncludedIntermediate
	stats.IntermediateTruncated = intermediateStats.IntermediateTruncated
	stats.ExecutionContextTruncated = stats.ExecutionContextTruncated || intermediateStats.ExecutionContextTruncated
	if intermediateSection != "" {
		builder.WriteString("\nRecent intermediate answers:\n")
		builder.WriteString(intermediateSection)
	}

	content := strings.TrimSpace(builder.String())
	if stats.IncludedToolEvents == 0 && stats.IncludedIntermediate == 0 {
		return nil, stats
	}
	return &adapter.Message{Role: "system", Content: content}, stats
}

func recentToolHistorySection(branch []*aichatmodel.Message, budget int) (string, recentExecutionContextStats) {
	stats := recentExecutionContextStats{}
	if budget <= 0 {
		stats.ExecutionContextTruncated = true
		return "", stats
	}
	var builder strings.Builder
	for i := len(branch) - 1; i >= 0; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		invocations := toolHistoryInvocations(message.Metadata)
		if len(invocations) == 0 {
			continue
		}
		stats.ToolHistoryTurns++
		if !appendBudgetedLine(&builder, budget, fmt.Sprintf("- Turn query: %s\n", compactForPrompt(message.Query, 240))) {
			stats.ExecutionContextTruncated = true
			break
		}
		for idx, invocation := range invocations {
			line := formatToolHistoryInvocation(idx+1, invocation)
			if !appendBudgetedLine(&builder, budget, line) {
				stats.ToolHistoryTruncated = true
				stats.ExecutionContextTruncated = true
				return builder.String(), stats
			}
			stats.IncludedToolEvents++
		}
		if stats.ToolHistoryTurns >= recentToolHistoryTurnLimit {
			break
		}
	}
	return builder.String(), stats
}

func recentIntermediateAnswerSection(branch []*aichatmodel.Message, budget int) (string, recentExecutionContextStats) {
	stats := recentExecutionContextStats{}
	if budget <= 0 {
		stats.ExecutionContextTruncated = true
		return "", stats
	}
	var builder strings.Builder
	for i := len(branch) - 1; i >= 0; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		invocations := intermediateAnswerInvocations(message.Metadata)
		if len(invocations) == 0 {
			continue
		}
		stats.IntermediateAnswerTurns++
		if !appendBudgetedLine(&builder, budget, fmt.Sprintf("- Turn query: %s\n", compactForPrompt(message.Query, 240))) {
			stats.ExecutionContextTruncated = true
			break
		}
		for _, invocation := range invocations {
			title := strings.TrimSpace(stringFromAny(invocation["title"]))
			if title == "" {
				title = "Intermediate answer"
			}
			content := compactForPrompt(stringFromAny(invocation["message"]), recentIntermediateAnswerBudgetChars)
			line := fmt.Sprintf("  - %s:\n%s\n", compactForPrompt(title, 120), indentPromptBlock(content, "    "))
			if !appendBudgetedLine(&builder, budget, line) {
				stats.IntermediateTruncated = true
				stats.ExecutionContextTruncated = true
				return builder.String(), stats
			}
			stats.IncludedIntermediate++
		}
		if stats.IntermediateAnswerTurns >= recentIntermediateAnswerTurnLimit {
			break
		}
	}
	return builder.String(), stats
}

func toolHistoryInvocations(metadata map[string]interface{}) []map[string]interface{} {
	invocations := skillInvocationMaps(metadata)
	out := make([]map[string]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		switch stringFromAny(invocation["kind"]) {
		case "skill_load", "reference_read", "tool_call", "guardrail":
			out = append(out, invocation)
		}
	}
	return out
}

func intermediateAnswerInvocations(metadata map[string]interface{}) []map[string]interface{} {
	invocations := skillInvocationMaps(metadata)
	out := make([]map[string]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		if stringFromAny(invocation["kind"]) == "intermediate_answer" && strings.TrimSpace(stringFromAny(invocation["message"])) != "" {
			out = append(out, invocation)
		}
	}
	return out
}

func skillInvocationMaps(metadata map[string]interface{}) []map[string]interface{} {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata["skill_invocations"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []map[string]interface{}:
		return append([]map[string]interface{}{}, typed...)
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if invocation, ok := item.(map[string]interface{}); ok {
				out = append(out, invocation)
			}
		}
		return out
	default:
		return nil
	}
}

func formatToolHistoryInvocation(index int, invocation map[string]interface{}) string {
	parts := []string{
		fmt.Sprintf("  %d. kind=%s", index, compactForPrompt(stringFromAny(invocation["kind"]), 80)),
		fmt.Sprintf("status=%s", compactForPrompt(stringFromAny(invocation["status"]), 80)),
	}
	if skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"])); skillID != "" {
		parts = append(parts, "skill_id="+compactForPrompt(skillID, 120))
	}
	if toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"])); toolName != "" {
		parts = append(parts, "tool_name="+compactForPrompt(toolName, 120))
	}
	if path := strings.TrimSpace(stringFromAny(invocation["path"])); path != "" {
		parts = append(parts, "path="+compactForPrompt(path, 180))
	}
	if message := strings.TrimSpace(stringFromAny(invocation["message"])); message != "" {
		parts = append(parts, "message="+compactForPrompt(message, 240))
	}
	if errText := strings.TrimSpace(stringFromAny(invocation["error"])); errText != "" {
		parts = append(parts, "error="+compactForPrompt(errText, 240))
	}
	if args := compactJSONForPrompt(invocation["arguments"], recentTraceArgumentBudgetChars); args != "" {
		parts = append(parts, "arguments="+args)
	}
	return strings.Join(parts, " ") + "\n"
}

func compactJSONForPrompt(value interface{}, maxChars int) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return compactForPrompt(string(data), maxChars)
}

func compactForPrompt(value string, maxChars int) string {
	text := strings.TrimSpace(value)
	if maxChars <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	if maxChars <= 12 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-12]) + "...[truncated]"
}

func appendBudgetedLine(builder *strings.Builder, budget int, line string) bool {
	if strings.TrimSpace(line) == "" {
		return true
	}
	if builder.Len()+len(line) > budget {
		return false
	}
	builder.WriteString(line)
	return true
}

func indentPromptBlock(value string, prefix string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func mergeRecentExecutionContextMetadata(target map[string]interface{}, stats recentExecutionContextStats) {
	if target == nil {
		return
	}
	if stats.IncludedToolEvents == 0 && stats.IncludedIntermediate == 0 {
		return
	}
	target["recent_execution_context"] = map[string]interface{}{
		"tool_history_turns":            stats.ToolHistoryTurns,
		"intermediate_answer_turns":     stats.IntermediateAnswerTurns,
		"included_tool_events":          stats.IncludedToolEvents,
		"included_intermediate_answers": stats.IncludedIntermediate,
		"tool_history_truncated":        stats.ToolHistoryTruncated,
		"intermediate_truncated":        stats.IntermediateTruncated,
		"truncated":                     stats.ExecutionContextTruncated,
	}
}

func countAdapterMessages(groups [][]adapter.Message) int {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	return total
}

func contextControlMetadata(spec ModelSpec, budget *budgetComputation, estimatedPromptTokens int, historyBefore int, historyAfter int) map[string]interface{} {
	return map[string]interface{}{
		"strategy":                contextControlStrategyTokenBudget,
		"context_window":          spec.ContextWindow,
		"safe_context_limit":      budget.SafeLimit,
		"prompt_budget":           budget.PromptBudget,
		"estimated_prompt_tokens": estimatedPromptTokens,
		"history_messages_before": historyBefore,
		"history_messages_after":  historyAfter,
		"truncated":               historyAfter < historyBefore,
		"max_tokens_clamped":      budget.MaxTokensClamped,
		"original_max_tokens":     optionalIntValue(budget.OriginalMaxTokens),
		"effective_max_tokens":    optionalIntValue(budget.EffectiveMaxTokens),
		"reserved_output_tokens":  budget.ReservedOutputTokens,
		"tokenizer":               budget.Tokenizer,
	}
}

func requestedMaxTokens(parts *chatRequestParts) (*int, bool) {
	if parts == nil || parts.Parameters == nil {
		return nil, false
	}
	value, ok := parts.Parameters["max_tokens"].(int)
	if !ok {
		return nil, false
	}
	return &value, true
}

func optionalIntValue(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func defaultOutputReserve(contextWindow int) int {
	switch {
	case contextWindow <= 4096:
		return 512
	case contextWindow <= 8192:
		return 1024
	case contextWindow <= 32768:
		return 2048
	case contextWindow <= 128000:
		return 4096
	default:
		return 8192
	}
}

func minOutputReserve(contextWindow int) int {
	switch {
	case contextWindow <= 4096:
		return 256
	case contextWindow <= 8192:
		return 512
	case contextWindow <= 32768:
		return 1024
	default:
		return 2048
	}
}

func minPromptBudget(contextWindow int) int {
	switch {
	case contextWindow <= 4096:
		return 1024
	case contextWindow <= 8192:
		return 2048
	case contextWindow <= 32768:
		return 4096
	default:
		return 8192
	}
}

func newTokenEstimator() *tokenestimate.Estimator {
	return tokenestimate.NewEstimator()
}

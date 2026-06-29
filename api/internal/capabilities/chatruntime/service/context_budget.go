package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	recentTraceResultBudgetChars        = 900
	recentContinuationTurnLimit         = 5
	recentContinuationBudgetChars       = 5000
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
	parentMessages []*runtimemodel.Message,
) (*contextBudgetResult, error) {
	applyRecentAssetCandidatesFromBranch(parts, parentMessages)
	applyRecentGeneratedArtifactsFromBranch(parts, parentMessages)
	applyRecentOperationPlansFromBranch(parts, parentMessages)
	recentExecutionContext, recentExecutionMetadata := buildRecentExecutionContextMessage(parentMessages)
	continuationContext := buildContinuationTaskStateMessage(parts, parentMessages)
	extraContextMessages := make([]adapter.Message, 0, 2)
	if recentExecutionContext != nil {
		extraContextMessages = append(extraContextMessages, *recentExecutionContext)
	}
	if continuationContext != nil {
		extraContextMessages = append(extraContextMessages, *continuationContext)
	}

	baseMessages := []adapter.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: s.currentUserContent(parts, parts.Query)},
	}
	if len(extraContextMessages) > 0 {
		baseMessages = make([]adapter.Message, 0, len(extraContextMessages)+2)
		baseMessages = append(baseMessages, adapter.Message{Role: "system", Content: systemPrompt})
		baseMessages = append(baseMessages, extraContextMessages...)
		baseMessages = append(baseMessages, adapter.Message{Role: "user", Content: s.currentUserContent(parts, parts.Query)})
	}
	budget, err := s.computeContextBudget(spec, parts, baseMessages)
	if err != nil {
		return nil, err
	}
	extraContextTokens := 0
	if len(extraContextMessages) > 0 {
		extraContextTokens = s.tokenEstimator.EstimateMessages(extraContextMessages, parts.ModelName).Tokens
	}
	currentContent, attachmentMetadata, estimatedPromptTokens := s.buildBudgetedCurrentUserContent(parts, systemPrompt, budget, extraContextTokens)

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
	messages = append(messages, extraContextMessages...)
	messages = append(messages, adapter.Message{Role: "user", Content: currentContent})

	historyAfter := len(messages) - 2
	if recentExecutionContext != nil {
		historyAfter--
	}
	if continuationContext != nil {
		historyAfter--
	}
	metadata := contextControlMetadata(spec, budget, estimatedPromptTokens, historyBefore, historyAfter)
	mergeAttachmentContextMetadata(metadata, attachmentMetadata)
	mergeRecentExecutionContextMetadata(metadata, recentExecutionMetadata)
	if continuationContext != nil {
		metadata["continuation_task_state_included"] = true
	}
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

func (s *service) historyMessageGroups(ctx context.Context, branch []*runtimemodel.Message, includeImages bool) [][]adapter.Message {
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
	ToolHistoryTurns           int
	IntermediateAnswerTurns    int
	IncludedToolEvents         int
	IncludedOperationSummaries int
	IncludedIntermediate       int
	IncludedGeneratedFiles     int
	ToolHistoryTruncated       bool
	IntermediateTruncated      bool
	ExecutionContextTruncated  bool
}

func buildRecentExecutionContextMessage(branch []*runtimemodel.Message) (*adapter.Message, recentExecutionContextStats) {
	stats := recentExecutionContextStats{}
	if len(branch) == 0 {
		return nil, stats
	}

	var builder strings.Builder
	builder.WriteString("Recent AIChat execution context for continuity.\n")
	builder.WriteString("Older turns are represented only by their final assistant answers in the conversation history.\n")
	builder.WriteString("Use these notes as context; do not mention these storage rules to the user.\n")
	builder.WriteString("Do not resubmit these notes as intermediate answers; reuse them directly for export, save, convert, or file-generation requests.\n")
	builder.WriteString("For a new user request, do not recap or claim completion of prior generated files unless the user explicitly references them or asks to continue.\n")
	builder.WriteString("Prior generated files are candidates for reuse only when the current request asks to reuse, save, convert, download, preview, or continue previous output.\n")

	remaining := recentExecutionContextBudgetChars - builder.Len()
	generatedSection, generatedStats := recentGeneratedFilesSection(branch, remaining)
	stats.IncludedGeneratedFiles = generatedStats.IncludedGeneratedFiles
	stats.ExecutionContextTruncated = stats.ExecutionContextTruncated || generatedStats.ExecutionContextTruncated
	if generatedSection != "" {
		builder.WriteString("\nMost recent generated/downloadable files:\n")
		builder.WriteString(generatedSection)
	}

	remaining = recentExecutionContextBudgetChars - builder.Len()
	operationSection, operationStats := recentOperationResultSummarySection(branch, remaining)
	stats.IncludedOperationSummaries = operationStats.IncludedOperationSummaries
	stats.ExecutionContextTruncated = stats.ExecutionContextTruncated || operationStats.ExecutionContextTruncated
	if operationSection != "" {
		builder.WriteString("\nMost recent operation result facts:\n")
		builder.WriteString(operationSection)
	}

	remaining = recentExecutionContextBudgetChars - builder.Len()
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
	if stats.IncludedToolEvents == 0 && stats.IncludedOperationSummaries == 0 && stats.IncludedIntermediate == 0 && stats.IncludedGeneratedFiles == 0 {
		return nil, stats
	}
	return &adapter.Message{Role: "system", Content: content}, stats
}

func buildContinuationTaskStateMessage(parts *chatRequestParts, branch []*runtimemodel.Message) *adapter.Message {
	if parts == nil || !isContinuationIntent(parts.Query) || len(branch) == 0 {
		return nil
	}
	goal := recentNonContinuationUserGoal(branch)
	if goal == "" {
		return nil
	}

	var builder strings.Builder
	appendBudgetedLine(&builder, recentContinuationBudgetChars, "AIChat continuation task state.\n")
	appendBudgetedLine(&builder, recentContinuationBudgetChars, "The user is asking to continue the most recent unfinished task, not starting a new generic page Q&A turn.\n")
	appendBudgetedLine(&builder, recentContinuationBudgetChars, "Use this compact state as authoritative continuity guidance; do not mention these internal notes to the user.\n")
	appendBudgetedLine(&builder, recentContinuationBudgetChars, "Most recent non-continuation user goal: "+compactForPrompt(goal, 500)+"\n")
	if activeGoals := continuationTaskGoals(goal, branch, 3); len(activeGoals) > 1 {
		appendBudgetedLine(&builder, recentContinuationBudgetChars, "\nPrior active operation goals still relevant to this continuation:\n")
		for _, activeGoal := range activeGoals[1:] {
			appendBudgetedLine(&builder, recentContinuationBudgetChars, "- "+compactForPrompt(activeGoal, 500)+"\n")
		}
	}
	appendBudgetedLine(&builder, recentContinuationBudgetChars, "Rules for this continuation: do not repeat successful side-effecting tool calls; continue only missing requested steps; verify asset mutations from tool results or refreshed page context before claiming success.\n")

	if planState := recentContinuationOperationPlanSection(branch, recentContinuationBudgetChars-builder.Len()); planState != "" {
		appendBudgetedLine(&builder, recentContinuationBudgetChars, "\nAuthoritative operation plan state:\n")
		appendBudgetedLine(&builder, recentContinuationBudgetChars, planState)
	}
	if resultState := recentContinuationOperationResultSummarySection(branch, recentContinuationBudgetChars-builder.Len()); resultState != "" {
		appendBudgetedLine(&builder, recentContinuationBudgetChars, "\nAuthoritative operation result facts:\n")
		appendBudgetedLine(&builder, recentContinuationBudgetChars, resultState)
	}
	if toolState := recentContinuationToolStateSection(branch, recentContinuationBudgetChars-builder.Len()); toolState != "" {
		appendBudgetedLine(&builder, recentContinuationBudgetChars, "\nRecent completed/blocked execution state:\n")
		appendBudgetedLine(&builder, recentContinuationBudgetChars, toolState)
	}
	if pending := continuationPendingHints(goal, branch); len(pending) > 0 {
		appendBudgetedLine(&builder, recentContinuationBudgetChars, "\nPending-step hints:\n")
		for _, line := range pending {
			appendBudgetedLine(&builder, recentContinuationBudgetChars, "- "+line+"\n")
		}
	}

	content := strings.TrimSpace(builder.String())
	if content == "" {
		return nil
	}
	return &adapter.Message{Role: "system", Content: content}
}

func recentNonContinuationUserGoal(branch []*runtimemodel.Message) string {
	for i := len(branch) - 1; i >= 0; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		query := strings.TrimSpace(message.Query)
		if query == "" || isContinuationIntent(query) {
			continue
		}
		return query
	}
	return ""
}

func recentContinuationToolStateSection(branch []*runtimemodel.Message, budget int) string {
	if budget <= 0 {
		return ""
	}
	var builder strings.Builder
	turns := 0
	for i := len(branch) - 1; i >= 0 && turns < recentContinuationTurnLimit; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		invocations := continuationStateInvocations(message.Metadata)
		if len(invocations) == 0 {
			continue
		}
		turns++
		if !appendBudgetedLine(&builder, budget, fmt.Sprintf("- Prior turn query: %s\n", compactForPrompt(message.Query, 240))) {
			return builder.String()
		}
		for idx, invocation := range invocations {
			line := formatToolHistoryInvocation(idx+1, invocation)
			if !appendBudgetedLine(&builder, budget, line) {
				return builder.String()
			}
		}
	}
	return builder.String()
}

func recentContinuationOperationResultSummarySection(branch []*runtimemodel.Message, budget int) string {
	if budget <= 0 {
		return ""
	}
	section, _ := recentOperationResultSummarySection(branch, budget)
	return section
}

func recentOperationResultSummarySection(branch []*runtimemodel.Message, budget int) (string, recentExecutionContextStats) {
	stats := recentExecutionContextStats{}
	if budget <= 0 {
		stats.ExecutionContextTruncated = true
		return "", stats
	}
	var builder strings.Builder
	turns := 0
	for i := len(branch) - 1; i >= 0 && turns < recentToolHistoryTurnLimit; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		summary := operationResultSummaryForPrompt(message.Metadata)
		if len(summary) == 0 {
			continue
		}
		turns++
		if !appendBudgetedLine(&builder, budget, fmt.Sprintf("- Turn query: %s\n", compactForPrompt(message.Query, 240))) {
			stats.ExecutionContextTruncated = true
			return builder.String(), stats
		}
		line := "  operation_result_summary=" + compactJSONForPrompt(summary, minInt(1400, budget-builder.Len())) + "\n"
		if !appendBudgetedLine(&builder, budget, line) {
			stats.ExecutionContextTruncated = true
			return builder.String(), stats
		}
		stats.IncludedOperationSummaries++
	}
	return builder.String(), stats
}

func operationResultSummaryForPrompt(metadata map[string]interface{}) map[string]interface{} {
	if summary := mapFromOperationContext(metadataValue(metadata, "operation_result_summary")); len(summary) > 0 {
		return sanitizeOperationResultSummaryForPrompt(summary)
	}
	executionSummary := skillLoopCompletionExecutionSummary(metadata)
	if len(executionSummary) == 0 {
		return nil
	}
	return sanitizeOperationResultSummaryForPrompt(skillLoopCompletionOperationResultSummary(executionSummary))
}

func sanitizeOperationResultSummaryForPrompt(summary map[string]interface{}) map[string]interface{} {
	if len(summary) == 0 {
		return nil
	}
	out := copyStringAnyMap(summary)
	if latest := mapFromOperationContext(out["latest_tool_result"]); strings.TrimSpace(stringFromAny(latest["kind"])) == "guardrail" {
		delete(out, "latest_tool_result")
		delete(out, "latest_tool_status")
		delete(out, "skill_id")
		delete(out, "tool_name")
		if strings.EqualFold(strings.TrimSpace(stringFromAny(out["status"])), "blocked") {
			delete(out, "status")
		}
	}
	for _, key := range []string{
		"operation",
		"operation_group",
		"target_count",
		"success_count",
		"failed_count",
		"generated_file_count",
		"generated_files",
		"latest_tool_result",
		"latest_client_action",
	} {
		if value, ok := out[key]; ok && value != nil {
			return out
		}
	}
	return nil
}

func recentContinuationOperationPlanSection(branch []*runtimemodel.Message, budget int) string {
	if budget <= 0 {
		return ""
	}
	plans := recentContinuationOperationPlans(branch, 2)
	if len(plans) == 0 {
		return ""
	}
	var builder strings.Builder
	for idx, plan := range plans {
		compact := compactOperationPlanForPrompt(plan)
		if len(compact) == 0 {
			continue
		}
		line := fmt.Sprintf("- Plan %d: %s\n", idx+1, compactJSONForPrompt(compact, minInt(1600, budget-builder.Len())))
		if !appendBudgetedLine(&builder, budget, line) {
			break
		}
	}
	return builder.String()
}

func recentContinuationOperationPlans(branch []*runtimemodel.Message, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, limit)
	seenTasks := map[string]struct{}{}
	for i := len(branch) - 1; i >= 0 && len(out) < limit; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		plan := mapFromOperationContext(metadataValue(message.Metadata, "operation_plan"))
		if len(plan) == 0 {
			continue
		}
		taskID := strings.TrimSpace(stringFromAny(plan["task_id"]))
		if taskID != "" {
			if _, ok := seenTasks[taskID]; ok {
				continue
			}
			seenTasks[taskID] = struct{}{}
		}
		out = append(out, plan)
	}
	return out
}

func compactOperationPlanForPrompt(plan map[string]interface{}) map[string]interface{} {
	if len(plan) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for _, key := range []string{"version", "task_id", "intent", "status", "pending_next_action", "original_user_goal"} {
		if value := strings.TrimSpace(stringFromAny(plan[key])); value != "" {
			out[key] = compactForPrompt(value, 500)
		}
	}
	if target := mapFromOperationContext(plan["asset_target"]); len(target) > 0 {
		out["asset_target"] = target
	}
	if stepStatus := mapFromOperationContext(plan["step_status"]); len(stepStatus) > 0 {
		out["step_status"] = stepStatus
	}
	if result := mapFromOperationContext(plan["tool_result"]); len(result) > 0 {
		out["tool_result"] = result
	}
	if assetState := mapFromOperationContext(plan["asset_state"]); len(assetState) > 0 {
		out["asset_state"] = assetState
	}
	if deviations := skillLoopCompletionPlanDeviations(plan["deviations"], 6); len(deviations) > 0 {
		out["deviations"] = deviations
	}
	if blockedDeviations := skillLoopCompletionPlanDeviations(plan["blocked_deviations"], 6); len(blockedDeviations) > 0 {
		out["blocked_deviations"] = blockedDeviations
	}
	steps := mapSliceFromAny(plan["steps"])
	if len(steps) > 0 {
		promptSteps := operationPlanPromptSteps(steps, 8)
		compacted := make([]interface{}, 0, len(promptSteps))
		for _, step := range promptSteps {
			item := map[string]interface{}{}
			for _, key := range []string{"id", "title", "status", "skill_id", "tool_name"} {
				if value := strings.TrimSpace(stringFromAny(step[key])); value != "" {
					item[key] = compactForPrompt(value, 180)
				}
			}
			if index := intValueFromAny(step["sequence_index"]); index > 0 {
				item["sequence_index"] = index
			}
			if target := mapFromOperationContext(step["asset_target"]); len(target) > 0 {
				item["asset_target"] = target
			}
			if len(item) > 0 {
				compacted = append(compacted, item)
			}
		}
		if len(compacted) > 0 {
			out["steps"] = compacted
		}
	}
	return out
}

func operationPlanPromptSteps(steps []map[string]interface{}, limit int) []map[string]interface{} {
	if limit <= 0 || len(steps) == 0 {
		return nil
	}
	if len(steps) <= limit {
		return steps
	}
	selected := make([]map[string]interface{}, 0, limit)
	used := map[int]struct{}{}
	add := func(index int) bool {
		if index < 0 || index >= len(steps) {
			return false
		}
		if _, ok := used[index]; ok {
			return len(selected) < limit
		}
		step := copyStringAnyMap(steps[index])
		step["sequence_index"] = index + 1
		selected = append(selected, step)
		used[index] = struct{}{}
		return len(selected) < limit
	}
	for index, step := range steps {
		status := operationPlanNormalizeStepStatus(stringFromAny(step["status"]))
		if status != operationPlanStepStatusCompleted {
			if !add(index) {
				return selected
			}
		}
	}
	for index := range steps {
		if !add(index) {
			return selected
		}
	}
	return selected
}

func continuationStateInvocations(metadata map[string]interface{}) []map[string]interface{} {
	invocations := skillInvocationMaps(metadata)
	out := make([]map[string]interface{}, 0, len(invocations))
	for _, invocation := range invocations {
		switch strings.TrimSpace(stringFromAny(invocation["kind"])) {
		case "tool_call", "client_action":
			out = append(out, invocation)
		}
	}
	return out
}

func continuationTaskGoals(goal string, branch []*runtimemodel.Message, limit int) []string {
	if limit <= 0 {
		return nil
	}
	goals := make([]string, 0, limit)
	seen := map[string]struct{}{}
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || len(goals) >= limit {
			return
		}
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		goals = append(goals, candidate)
	}
	add(goal)
	for _, plan := range recentContinuationOperationPlans(branch, recentContinuationTurnLimit) {
		if !operationPlanHasIncompleteWork(plan) {
			continue
		}
		add(stringFromAny(plan["original_user_goal"]))
	}
	return goals
}

func operationPlanHasIncompleteWork(plan map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	status := strings.TrimSpace(stringFromAny(plan["status"]))
	if status != "" && status != operationPlanStatusCompleted {
		return true
	}
	if pending := strings.TrimSpace(stringFromAny(plan["pending_next_action"])); pending != "" && !strings.EqualFold(pending, "none") {
		return true
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	blockingStepIDs := map[string]struct{}{}
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		id := stringFromAny(step["id"])
		blockingStepIDs[id] = struct{}{}
		if operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id])) != operationPlanStepStatusCompleted {
			return true
		}
	}
	for id, value := range stepStatus {
		if _, ok := blockingStepIDs[id]; !ok {
			continue
		}
		if operationPlanNormalizeStepStatus(stringFromAny(value)) != operationPlanStepStatusCompleted {
			return true
		}
	}
	return false
}

func continuationPendingHints(goal string, branch []*runtimemodel.Message) []string {
	hints := []string{}
	addHint := func(hint string) {
		for _, existing := range hints {
			if existing == hint {
				return
			}
		}
		hints = append(hints, hint)
	}
	goals := continuationTaskGoals(goal, branch, recentContinuationTurnLimit)
	if len(goals) == 0 && strings.TrimSpace(goal) != "" {
		goals = []string{strings.TrimSpace(goal)}
	}
	hasGeneratedArtifact := continuationHasSuccessfulGeneratedArtifact(branch)
	hasManagedSave := continuationHasSuccessfulTool(branch, skills.SkillFileManager, "save_file_to_management")
	hasDelete := continuationHasSuccessfulTool(branch, skills.SkillFileManager, "delete_file")
	hasManagedCreateGoal := false
	hasDeleteGoal := continuationHasPendingOperationPlanTool(branch, skills.SkillFileManager, "delete_file")

	for _, candidate := range goals {
		if isManagedFileCreateIntent(candidate) {
			hasManagedCreateGoal = true
		}
		if isFileDeleteIntent(candidate) {
			hasDeleteGoal = true
		}
	}

	if hasManagedCreateGoal {
		if hasGeneratedArtifact && !hasManagedSave {
			addHint("A temporary artifact has already been generated; save that artifact with file-manager/save_file_to_management instead of generating another one.")
		}
		if hasManagedSave {
			addHint("At least one generated artifact has already been saved to File Management; do not repeat that save unless the original goal requires another distinct file.")
		}
	}
	if hasDeleteGoal && !hasDelete {
		addHint("The prior goal still appears to include a file deletion, and no successful file-manager/delete_file call is recorded; resolve the current visible target, call file-manager/delete_file, and wait for the governed tool result. Do not ask for a separate natural-language confirmation because tool governance handles the approval card.")
	}
	if len(hints) == 0 && hasGeneratedArtifact {
		addHint("A generated artifact already exists in recent execution state; reuse it for save/export requests instead of regenerating the same file.")
	}
	return hints
}

func continuationHasPendingOperationPlanTool(branch []*runtimemodel.Message, skillID, toolName string) bool {
	for _, plan := range recentContinuationOperationPlans(branch, recentContinuationTurnLimit) {
		if operationPlanHasPendingToolStep(plan, skillID, toolName) {
			return true
		}
	}
	return false
}

func operationPlanHasPendingToolStep(plan map[string]interface{}, skillID, toolName string) bool {
	if len(plan) == 0 {
		return false
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	for _, step := range mapSliceFromAny(plan["steps"]) {
		if !operationPlanStepMatchesExactTool(step, skillID, toolName) {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[stringFromAny(step["id"])]))
		return status == operationPlanStepStatusPending
	}
	stepID := operationPlanToolStepID(skillID, toolName)
	if stepID == "" {
		return false
	}
	status, ok := stepStatus[stepID]
	if !ok {
		return false
	}
	return operationPlanNormalizeStepStatus(stringFromAny(status)) == operationPlanStepStatusPending
}

func operationPlanStepMatchesExactTool(step map[string]interface{}, skillID, toolName string) bool {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skillID) {
		return false
	}
	stepToolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName == "" {
		return stepToolName == ""
	}
	if stepToolName != "" {
		return strings.EqualFold(stepToolName, toolName)
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(step["id"])), operationPlanToolStepID(skillID, toolName))
}

func continuationHasSuccessfulTool(branch []*runtimemodel.Message, skillID, toolName string) bool {
	for _, message := range branch {
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		for _, invocation := range skillInvocationMaps(message.Metadata) {
			if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" ||
				!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skillID) &&
				strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), toolName) {
				return true
			}
		}
	}
	return false
}

func continuationHasSuccessfulGeneratedArtifact(branch []*runtimemodel.Message) bool {
	for _, message := range branch {
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		for _, invocation := range skillInvocationMaps(message.Metadata) {
			if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" ||
				!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["status"])), "success") {
				continue
			}
			if isKnownArtifactGeneratorToolCall(stringFromAny(invocation["skill_id"]), stringFromAny(invocation["tool_name"])) {
				return true
			}
		}
	}
	return false
}

func recentGeneratedFilesSection(branch []*runtimemodel.Message, budget int) (string, recentExecutionContextStats) {
	stats := recentExecutionContextStats{}
	if budget <= 0 {
		stats.ExecutionContextTruncated = true
		return "", stats
	}
	var builder strings.Builder
	artifacts := recentGeneratedArtifactsFromBranch(branch)
	if len(artifacts) == 0 {
		return "", stats
	}
	if !appendBudgetedLine(&builder, budget, "Use tool_file_id/file_id values only as tool arguments; do not reveal them to the user.\n") {
		stats.ExecutionContextTruncated = true
		return "", stats
	}
	for index, artifact := range artifacts {
		line := formatRecentGeneratedFile(index+1, artifact)
		if !appendBudgetedLine(&builder, budget, line) {
			stats.ExecutionContextTruncated = true
			return builder.String(), stats
		}
		stats.IncludedGeneratedFiles++
	}
	return builder.String(), stats
}

func formatRecentGeneratedFile(index int, artifact map[string]interface{}) string {
	parts := []string{
		fmt.Sprintf("  %d. filename=%s", index, compactForPrompt(firstNonEmptyString(artifact["filename"], artifact["name"]), 180)),
		"tool_file_id=" + compactForPrompt(stringFromAny(artifact["tool_file_id"]), 160),
	}
	if artifactID := strings.TrimSpace(stringFromAny(artifact["artifact_id"])); artifactID != "" {
		parts = append(parts, "artifact_id="+compactForPrompt(artifactID, 160))
	}
	if status := strings.TrimSpace(stringFromAny(artifact["status"])); status != "" {
		parts = append(parts, "status="+compactForPrompt(status, 80))
	}
	if extension := strings.TrimSpace(stringFromAny(artifact["extension"])); extension != "" {
		parts = append(parts, "extension="+compactForPrompt(extension, 40))
	}
	if mimeType := strings.TrimSpace(stringFromAny(artifact["mime_type"])); mimeType != "" {
		parts = append(parts, "mime_type="+compactForPrompt(mimeType, 120))
	}
	if skillID := strings.TrimSpace(stringFromAny(artifact["skill_id"])); skillID != "" {
		parts = append(parts, "skill_id="+compactForPrompt(skillID, 120))
	}
	if toolName := strings.TrimSpace(stringFromAny(artifact["tool_name"])); toolName != "" {
		parts = append(parts, "tool_name="+compactForPrompt(toolName, 120))
	}
	if messageID := strings.TrimSpace(stringFromAny(artifact["source_message_id"])); messageID != "" {
		parts = append(parts, "source_message_id="+compactForPrompt(messageID, 160))
	}
	return strings.Join(parts, " ") + "\n"
}

func recentToolHistorySection(branch []*runtimemodel.Message, budget int) (string, recentExecutionContextStats) {
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

func recentIntermediateAnswerSection(branch []*runtimemodel.Message, budget int) (string, recentExecutionContextStats) {
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
		case "skill_load", "reference_read", "tool_call":
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
	if result := compactToolHistoryResultForPrompt(invocation); result != "" {
		parts = append(parts, "result="+result)
	}
	return strings.Join(parts, " ") + "\n"
}

func compactToolHistoryResultForPrompt(invocation map[string]interface{}) string {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
		return ""
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	if skillID != skills.SkillFileReader && skillID != skills.SkillFileManager {
		return ""
	}
	switch strings.TrimSpace(stringFromAny(invocation["tool_name"])) {
	case "list_visible_files", "read_file":
		if skillID != skills.SkillFileReader {
			return ""
		}
	case "delete_file", "save_file_to_management":
		if skillID != skills.SkillFileManager {
			return ""
		}
	default:
		return ""
	}
	result := redactedToolHistoryResultValue(invocation["result"], 0)
	return compactJSONForPrompt(result, recentTraceResultBudgetChars)
}

func redactedToolHistoryResultValue(value interface{}, depth int) interface{} {
	if value == nil || depth > 5 {
		return nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		if len(typed) == 0 {
			return nil
		}
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if shouldRedactToolHistoryResultKey(key) {
				out[key+"_redacted"] = true
				continue
			}
			if redacted := redactedToolHistoryResultValue(item, depth+1); redacted != nil {
				out[key] = redacted
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []interface{}:
		if len(typed) == 0 {
			return nil
		}
		limit := minInt(len(typed), 20)
		out := make([]interface{}, 0, limit)
		for _, item := range typed[:limit] {
			if redacted := redactedToolHistoryResultValue(item, depth+1); redacted != nil {
				out = append(out, redacted)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []map[string]interface{}:
		if len(typed) == 0 {
			return nil
		}
		limit := minInt(len(typed), 20)
		out := make([]interface{}, 0, limit)
		for _, item := range typed[:limit] {
			if redacted := redactedToolHistoryResultValue(item, depth+1); redacted != nil {
				out = append(out, redacted)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		return compactForPrompt(text, 240)
	default:
		return typed
	}
}

func shouldRedactToolHistoryResultKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "content", "content_preview", "content_error":
		return true
	default:
		return false
	}
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
	if stats.IncludedToolEvents == 0 && stats.IncludedIntermediate == 0 && stats.IncludedGeneratedFiles == 0 {
		return
	}
	target["recent_execution_context"] = map[string]interface{}{
		"tool_history_turns":            stats.ToolHistoryTurns,
		"intermediate_answer_turns":     stats.IntermediateAnswerTurns,
		"included_tool_events":          stats.IncludedToolEvents,
		"included_intermediate_answers": stats.IncludedIntermediate,
		"included_generated_files":      stats.IncludedGeneratedFiles,
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

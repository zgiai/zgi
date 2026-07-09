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
	recentExecutionContext, recentExecutionMetadata := buildRecentExecutionContextMessageForRequest(parts, parentMessages)
	continuationContext := buildContinuationTaskStateMessage(parts, parentMessages)
	turnBoundaryContext := currentTurnBoundaryMessage(parts)
	extraContextMessages := make([]adapter.Message, 0, 3)
	if recentExecutionContext != nil {
		extraContextMessages = append(extraContextMessages, *recentExecutionContext)
	}
	if continuationContext != nil {
		extraContextMessages = append(extraContextMessages, *continuationContext)
	}
	if turnBoundaryContext != nil {
		extraContextMessages = append(extraContextMessages, *turnBoundaryContext)
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

	groups, err := s.historyMessageGroupsForCurrentRequest(ctx, parentMessages, parts)
	if err != nil {
		return nil, err
	}
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

	historyAfter := len(messages) - 2 - len(extraContextMessages)
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

func (s *service) historyMessageGroups(ctx context.Context, branch []*runtimemodel.Message, includeImages bool) ([][]adapter.Message, error) {
	groups := make([][]adapter.Message, 0, len(branch))
	for _, item := range branch {
		if item == nil {
			continue
		}
		group := make([]adapter.Message, 0, 2)
		userMessage, err := s.historicalUserMessage(ctx, item, includeImages)
		if err != nil {
			return nil, err
		}
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
	return groups, nil
}

func (s *service) historyMessageGroupsForCurrentRequest(ctx context.Context, branch []*runtimemodel.Message, parts *chatRequestParts) ([][]adapter.Message, error) {
	if shouldIsolateHistoryForCurrentTurn(parts) {
		return nil, nil
	}
	includeImages := false
	if parts != nil {
		includeImages = parts.ModelSupportsVision
	}
	return s.historyMessageGroups(ctx, branch, includeImages)
}

func shouldIsolateHistoryForCurrentTurn(parts *chatRequestParts) bool {
	if parts == nil || !isContextualAIChatSurface(parts.Surface) {
		return false
	}
	if partsRequestsContinuationWithFallback(parts, "") || queryReferencesRecentExecutionContext(parts.Query) {
		return false
	}
	if intent := parts.ModelTurnIntent; intent != nil {
		switch strings.ToLower(strings.TrimSpace(intent.AssetEffect)) {
		case "create", "update", "delete", "write", "mutation", "mutate":
			return true
		}
	}
	strategy := contextualAIChatTurnStrategyFromParts(parts)
	if strategy == nil {
		return false
	}
	if strategy.RouteRequired || strategy.RequiredNextTool != nil || len(strategy.PlannedTools) > 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(strategy.AssetEffect)) {
	case "create", "update", "delete", "write", "mutation", "mutate":
		return true
	}
	return false
}

type recentExecutionContextStats struct {
	ToolHistoryTurns           int
	IntermediateAnswerTurns    int
	IncludedToolEvents         int
	IncludedOperationSummaries int
	IncludedTurnStateFacts     int
	IncludedIntermediate       int
	IncludedGeneratedFiles     int
	ToolHistoryTruncated       bool
	IntermediateTruncated      bool
	ExecutionContextTruncated  bool
}

func buildRecentExecutionContextMessageForRequest(parts *chatRequestParts, branch []*runtimemodel.Message) (*adapter.Message, recentExecutionContextStats) {
	if !recentExecutionContextAppliesToCurrentRequest(parts) {
		return nil, recentExecutionContextStats{}
	}
	return buildRecentExecutionContextMessage(branch)
}

func recentExecutionContextAppliesToCurrentRequest(parts *chatRequestParts) bool {
	if parts == nil {
		return true
	}
	query := strings.TrimSpace(parts.Query)
	if query == "" {
		return false
	}
	if partsRequestsContinuationWithFallback(parts, "") {
		return true
	}
	return queryReferencesRecentExecutionContext(query)
}

func queryReferencesRecentExecutionContext(query string) bool {
	normalized := strings.ToLower(strings.TrimSpace(query))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"刚才", "刚刚", "上次", "上一", "前面", "之前", "先前", "继续", "接着", "后续", "用这个", "把这个", "该文件", "这个文件", "这个图", "上述", "以上", "已生成", "生成的文件", "保存它", "下载它", "预览它",
		"previous", "last", "earlier", "continue", "reuse", "use it", "that file", "this file", "generated file", "save it", "download it", "preview it",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func currentTurnBoundaryMessage(parts *chatRequestParts) *adapter.Message {
	if parts == nil || partsRequestsContinuationWithFallback(parts, "") {
		return nil
	}
	content := strings.Join([]string{
		"Current AIChat turn boundary:",
		"The latest user request below is the task to execute now.",
		"Use older conversation messages only as background facts.",
		"Do not continue, repeat, or complete earlier tasks unless the latest request explicitly asks to continue or reuse previous outputs.",
		"If older history or recent execution facts conflict with the latest user request, follow the latest user request.",
	}, "\n")
	return &adapter.Message{Role: "system", Content: content}
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
	turnStateSection, turnStateStats := recentTurnStateSection(branch, remaining)
	stats.IncludedTurnStateFacts = turnStateStats.IncludedTurnStateFacts
	stats.ExecutionContextTruncated = stats.ExecutionContextTruncated || turnStateStats.ExecutionContextTruncated
	if turnStateSection != "" {
		builder.WriteString("\nMost recent turn state facts:\n")
		builder.WriteString(turnStateSection)
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
	if stats.IncludedToolEvents == 0 && stats.IncludedOperationSummaries == 0 && stats.IncludedTurnStateFacts == 0 && stats.IncludedIntermediate == 0 && stats.IncludedGeneratedFiles == 0 {
		return nil, stats
	}
	return &adapter.Message{Role: "system", Content: content}, stats
}

func buildContinuationTaskStateMessage(parts *chatRequestParts, branch []*runtimemodel.Message) *adapter.Message {
	if parts == nil || !partsRequestsContinuationWithFallback(parts, "") || len(branch) == 0 {
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
	if pending := continuationPendingHints(branch); len(pending) > 0 {
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
	if summary := rebuiltOperationResultSummaryForPrompt(metadata); len(summary) > 0 {
		return summary
	}
	if summary := mapFromOperationContext(metadataValue(metadata, "operation_result_summary")); len(summary) > 0 {
		return sanitizeOperationResultSummaryForPrompt(summary)
	}
	return nil
}

func rebuiltOperationResultSummaryForPrompt(metadata map[string]interface{}) map[string]interface{} {
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
	modelDecides := operationPlanModelDecidesTools(plan)
	for _, key := range []string{"version", "task_id", "intent", "task_type", "status", "pending_next_action", "original_user_goal", "risk_level", "approval", "planning_mode"} {
		if value := strings.TrimSpace(stringFromAny(plan[key])); value != "" {
			out[key] = compactForPrompt(value, 500)
		}
	}
	if value, ok := plan["needs_exact_agent_runtime"].(bool); ok {
		out["needs_exact_agent_runtime"] = value
	}
	if value, ok := plan["current_context_may_be_summary"].(bool); ok {
		out["current_context_may_be_summary"] = value
	}
	if value, ok := plan["approval_required"].(bool); ok {
		out["approval_required"] = value
	}
	if phases := stringSliceFromAny(plan["phase_goals"]); len(phases) > 0 {
		out["phase_goals"] = compactStringSliceForPrompt(phases, 8, 180)
	}
	if evidence := stringSliceFromAny(plan["evidence_required"]); len(evidence) > 0 {
		out["evidence_required"] = compactStringSliceForPrompt(evidence, 10, 180)
	}
	if capabilities := stringSliceFromAny(plan["recommended_capabilities"]); len(capabilities) > 0 {
		out["recommended_capabilities"] = compactStringSliceForPrompt(capabilities, 10, 160)
	}
	if actions := stringSliceFromAny(plan["approval_actions"]); len(actions) > 0 {
		out["approval_actions"] = compactStringSliceForPrompt(actions, 8, 180)
	}
	if criteria := stringSliceFromAny(plan["success_criteria"]); len(criteria) > 0 {
		out["success_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if criteria := stringSliceFromAny(plan["completion_criteria"]); len(criteria) > 0 {
		out["completion_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if goals := operationPlanCompactCapabilityGoals(plan["capability_goals"], 6); len(goals) > 0 {
		out["capability_goals"] = goals
	}
	if contract := mapFromOperationContext(plan["task_contract"]); len(contract) > 0 {
		out["task_contract"] = compactTaskContractForPrompt(contract)
	}
	if target := mapFromOperationContext(plan["asset_target"]); len(target) > 0 {
		out["asset_target"] = target
	}
	if stepStatus := mapFromOperationContext(plan["step_status"]); len(stepStatus) > 0 && !modelDecides {
		out["step_status"] = stepStatus
	}
	if result := mapFromOperationContext(plan["tool_result"]); len(result) > 0 {
		out["tool_result"] = result
	}
	if assetState := mapFromOperationContext(plan["asset_state"]); len(assetState) > 0 {
		out["asset_state"] = assetState
	}
	if pageEvidence := operationPlanCompactPageEvidence(mapFromOperationContext(plan["page_evidence"])); len(pageEvidence) > 0 {
		out["page_evidence"] = pageEvidence
	}
	if phases := operationPlanCompactPhasesForPrompt(plan["phases"], 8); len(phases) > 0 {
		out["phases"] = phases
	}
	if completedSteps := operationPlanCompactProgressStepRecords(plan["completed_steps"], 8); len(completedSteps) > 0 && !modelDecides {
		out["completed_steps"] = completedSteps
	}
	if failedSteps := operationPlanCompactProgressStepRecords(plan["failed_steps"], 8); len(failedSteps) > 0 && !modelDecides {
		out["failed_steps"] = failedSteps
	}
	if deviations := skillLoopCompletionPlanDeviations(plan["deviations"], 6); len(deviations) > 0 {
		out["deviations"] = deviations
	}
	if blockedDeviations := skillLoopCompletionPlanDeviations(plan["blocked_deviations"], 6); len(blockedDeviations) > 0 {
		out["blocked_deviations"] = blockedDeviations
	}
	if steps := operationPlanCompactStepsForPrompt(plan["steps"], 8); len(steps) > 0 && !modelDecides {
		out["steps"] = steps
	}
	return out
}

func compactTaskContractForPrompt(contract map[string]interface{}) map[string]interface{} {
	if len(contract) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for _, key := range []string{
		"source",
		"intent_label",
		"compatibility",
		"tool_choice",
		"source_reason",
		"task_type",
		"target_page",
		"asset_effect",
		"asset_risk",
		"approval",
		"reason",
	} {
		if value := strings.TrimSpace(stringFromAny(contract[key])); value != "" {
			out[key] = compactForPrompt(value, 500)
		}
	}
	for _, key := range []string{"route_required", "needs_exact_agent_runtime", "current_context_may_be_summary", "open_created_agent_detail", "low_confidence"} {
		if value, ok := contract[key].(bool); ok {
			out[key] = value
		}
	}
	if confidence, ok := floatValue(contract["confidence"]); ok {
		out["confidence"] = confidence
	}
	if idx := intValueFromAny(contract["target_visible_index"]); idx > 0 {
		out["target_visible_index"] = idx
	}
	if phases := stringSliceFromAny(contract["phases"]); len(phases) > 0 {
		out["phases"] = compactStringSliceForPrompt(phases, 8, 180)
	}
	if evidence := stringSliceFromAny(contract["evidence_required"]); len(evidence) > 0 {
		out["evidence_required"] = compactStringSliceForPrompt(evidence, 10, 180)
	}
	if capabilities := stringSliceFromAny(contract["recommended_capabilities"]); len(capabilities) > 0 {
		out["recommended_capabilities"] = compactStringSliceForPrompt(capabilities, 10, 160)
	}
	if criteria := stringSliceFromAny(contract["completion_criteria"]); len(criteria) > 0 {
		out["completion_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if goals := operationPlanCompactCapabilityGoals(contract["capability_goals"], 6); len(goals) > 0 {
		out["capability_goals"] = goals
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
	if operationPlanIsTerminal(plan) {
		return false
	}
	if status := strings.TrimSpace(stringFromAny(plan["status"])); status != "" {
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

func continuationPendingHints(branch []*runtimemodel.Message) []string {
	hints := []string{}
	addHint := func(hint string) {
		for _, existing := range hints {
			if existing == hint {
				return
			}
		}
		hints = append(hints, hint)
	}
	hasGeneratedArtifact := continuationHasSuccessfulGeneratedArtifact(branch)
	hasManagedSave := continuationHasSuccessfulTool(branch, skills.SkillFileManager, "save_file_to_management")
	hasDelete := continuationHasSuccessfulTool(branch, skills.SkillFileManager, "delete_file")
	hasManagedCreateGoal := continuationHasPendingOperationPlanTool(branch, skills.SkillFileManager, "save_file_to_management")
	hasDeleteGoal := continuationHasPendingOperationPlanTool(branch, skills.SkillFileManager, "delete_file")

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

func recentTurnStateSection(branch []*runtimemodel.Message, budget int) (string, recentExecutionContextStats) {
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
		items := turnStatePromptItems(message.Metadata)
		if len(items) == 0 {
			continue
		}
		if !appendBudgetedLine(&builder, budget, fmt.Sprintf("- Turn query: %s\n", compactForPrompt(message.Query, 240))) {
			stats.ExecutionContextTruncated = true
			break
		}
		for _, item := range items {
			line := formatTurnStatePromptItem(item)
			if line == "" {
				continue
			}
			if !appendBudgetedLine(&builder, budget, line) {
				stats.ExecutionContextTruncated = true
				return builder.String(), stats
			}
			stats.IncludedTurnStateFacts++
			if stats.IncludedTurnStateFacts >= 12 {
				return builder.String(), stats
			}
		}
	}
	return builder.String(), stats
}

func turnStatePromptItems(metadata map[string]interface{}) []map[string]interface{} {
	state := mapFromOperationContext(metadata["turn_state"])
	items := mapSliceFromAny(state["items"])
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		kind := strings.TrimSpace(stringFromAny(item["kind"]))
		if kind == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func formatTurnStatePromptItem(item map[string]interface{}) string {
	kind := strings.TrimSpace(stringFromAny(item["kind"]))
	key := strings.TrimSpace(stringFromAny(item["key"]))
	value := strings.TrimSpace(stringFromAny(item["value"]))
	if value == "" {
		value = strings.TrimSpace(stringFromAny(item["content"]))
	}
	if kind == "" || value == "" {
		return ""
	}
	parts := []string{
		"  - kind=" + compactForPrompt(kind, 80),
	}
	if key != "" {
		parts = append(parts, "key="+compactForPrompt(key, 120))
	}
	parts = append(parts, "value="+compactForPrompt(value, 500))
	if source := strings.TrimSpace(stringFromAny(item["source"])); source != "" {
		parts = append(parts, "source="+compactForPrompt(source, 160))
	}
	return strings.Join(parts, " ") + "\n"
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
	if args := compactToolHistoryArgumentsForPrompt(invocation); args != "" {
		parts = append(parts, "arguments="+args)
	}
	if result := compactToolHistoryResultForPrompt(invocation); result != "" {
		parts = append(parts, "result="+result)
	}
	return strings.Join(parts, " ") + "\n"
}

func compactToolHistoryArgumentsForPrompt(invocation map[string]interface{}) string {
	args := mapFromOperationContext(invocation["arguments"])
	if len(args) == 0 {
		return ""
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	if skillID == skills.SkillAgentManagement {
		return ""
	}
	filtered := map[string]interface{}{}
	for key, value := range args {
		key = strings.TrimSpace(key)
		if key == "" || toolHistoryArgumentValueIsShapeSummary(value) {
			continue
		}
		filtered[key] = value
	}
	if len(filtered) == 0 {
		return ""
	}
	switch skillID {
	case skills.SkillFileReader, skills.SkillFileManager, skills.SkillConsoleNavigator:
	default:
		if toolName == "" {
			return ""
		}
	}
	return compactJSONForPrompt(filtered, recentTraceArgumentBudgetChars)
}

func toolHistoryArgumentValueIsShapeSummary(value interface{}) bool {
	shape := mapFromOperationContext(value)
	if len(shape) == 0 {
		return false
	}
	typeValue := strings.TrimSpace(stringFromAny(shape["type"]))
	if typeValue == "" {
		return false
	}
	switch typeValue {
	case "string", "array", "object":
	default:
		return false
	}
	if _, ok := shape["length"]; ok {
		return true
	}
	if _, ok := shape["keys"]; ok {
		return true
	}
	return false
}

func compactToolHistoryResultForPrompt(invocation map[string]interface{}) string {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
		return ""
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	if skillID == skills.SkillAgentManagement {
		return compactAgentManagementToolHistoryResultForPrompt(invocation)
	}
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

func compactAgentManagementToolHistoryResultForPrompt(invocation map[string]interface{}) string {
	result := mapFromOperationContext(invocation["result"])
	if len(result) == 0 {
		return ""
	}
	out := map[string]interface{}{}
	for _, key := range []string{
		"status",
		"agent_id",
		"agent_name",
		"name",
		"href",
		"agent_href",
		"workspace_id",
		"agent_workspace_id",
		"model_provider",
		"model",
		"requested_fields",
		"satisfied_fields",
		"updated_fields",
		"config_changes",
		"binding_changes",
		"enabled_skill_ids",
		"knowledge_dataset_ids",
		"database_bindings",
		"workflow_bindings",
		"home_title",
		"input_placeholder",
		"theme_color",
		"suggested_questions",
		"agent_memory_enabled",
		"file_upload_enabled",
	} {
		if value, ok := result[key]; ok {
			if redacted := redactedToolHistoryResultValue(value, 0); redacted != nil {
				out[key] = redacted
			}
		}
	}
	if len(out) == 0 {
		return ""
	}
	return compactJSONForPrompt(out, recentTraceResultBudgetChars)
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

func compactStringSliceForPrompt(values []string, limit int, maxChars int) []string {
	if limit <= 0 || maxChars <= 0 || len(values) == 0 {
		return nil
	}
	out := make([]string, 0, minInt(len(values), limit))
	for _, value := range values {
		value = compactForPrompt(value, maxChars)
		if value == "" {
			continue
		}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return []string{text}
		}
		return nil
	}
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
	if stats.IncludedToolEvents == 0 && stats.IncludedTurnStateFacts == 0 && stats.IncludedIntermediate == 0 && stats.IncludedGeneratedFiles == 0 {
		return
	}
	target["recent_execution_context"] = map[string]interface{}{
		"tool_history_turns":            stats.ToolHistoryTurns,
		"intermediate_answer_turns":     stats.IntermediateAnswerTurns,
		"included_tool_events":          stats.IncludedToolEvents,
		"included_turn_state_facts":     stats.IncludedTurnStateFacts,
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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/config"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/tokenestimate"
)

const (
	contextControlStrategyTokenBudget = "token_budget"

	recentContinuationTurnLimit = 5
)

type contextBudgetResult struct {
	Messages     []adapter.Message
	Metadata     map[string]interface{}
	Budget       *budgetComputation
	RawMessages  []*runtimemodel.Message
	Spec         ModelSpec
	SystemPrompt string
}

type budgetComputation struct {
	ConfiguredAgentWindowK int
	AgentContextWindow     int
	AgentWindowClamped     bool
	SafeLimit              int
	PromptBudget           int
	ReservedOutputTokens   int
	BasePromptTokens       int
	OriginalMaxTokens      *int
	EffectiveMaxTokens     *int
	MaxTokensClamped       bool
	Tokenizer              string
}

func (s *service) buildTokenBudgetMessages(
	ctx context.Context,
	spec ModelSpec,
	parts *chatRequestParts,
	systemPrompt string,
	parentMessages []*runtimemodel.Message,
) (*contextBudgetResult, error) {
	if s.tokenEstimator == nil {
		s.tokenEstimator = newTokenEstimator()
	}
	applyRecentAssetCandidatesFromBranch(parts, parentMessages)
	applyRecentGeneratedArtifactsFromBranch(parts, parentMessages)
	applyRecentOperationPlansFromBranch(parts, parentMessages)
	turnBoundaryContext := currentTurnBoundaryMessage(parts)
	extraContextMessages := make([]adapter.Message, 0, 1)
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
	selected := groups
	for _, group := range selected {
		estimatedPromptTokens += s.tokenEstimator.EstimateMessages(group, parts.ModelName).Tokens
	}

	systemMessage := adapter.Message{Role: "system", Content: systemPrompt}
	currentUserMessage := adapter.Message{Role: "user", Content: currentContent}
	historyMessages := make([]adapter.Message, 0, historyBefore)
	for _, group := range selected {
		historyMessages = append(historyMessages, group...)
	}
	messages := make([]adapter.Message, 0, 2+historyBefore)
	messages = append(messages, systemMessage)
	messages = append(messages, historyMessages...)
	messages = append(messages, extraContextMessages...)
	messages = append(messages, currentUserMessage)

	metadata := contextControlMetadata(spec, budget, estimatedPromptTokens, historyBefore, historyBefore)
	metadata["truncated"] = false
	mergeAttachmentContextMetadata(metadata, attachmentMetadata)
	return &contextBudgetResult{
		Messages:     messages,
		Metadata:     metadata,
		Budget:       budget,
		Spec:         spec,
		SystemPrompt: systemPrompt,
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
	agentWindowK := 256
	if config.GlobalConfig != nil && config.GlobalConfig.ChatRuntime.AgentContextWindowK > 0 {
		agentWindowK = config.GlobalConfig.ChatRuntime.AgentContextWindowK
	}
	configuredAgentWindow := agentWindowK * 1000
	agentContextWindow := min(configuredAgentWindow, spec.ContextWindow)
	safeLimit := agentContextWindow
	minOutput := minOutputReserve(agentContextWindow)
	if safeLimit <= minOutput {
		return nil, fmt.Errorf("%w: model context budget is too small", ErrInvalidInput)
	}

	baseEstimate := s.tokenEstimator.EstimateMessages(baseMessages, parts.ModelName)
	minPrompt := minPromptBudget(agentContextWindow)
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
	desiredOutput := defaultOutputReserve(agentContextWindow)
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
		ConfiguredAgentWindowK: agentWindowK,
		AgentContextWindow:     agentContextWindow,
		AgentWindowClamped:     agentContextWindow < configuredAgentWindow,
		SafeLimit:              safeLimit,
		PromptBudget:           promptBudget,
		ReservedOutputTokens:   reservedOutput,
		BasePromptTokens:       baseEstimate.Tokens,
		OriginalMaxTokens:      originalMaxTokens,
		EffectiveMaxTokens:     effectiveMaxTokens,
		MaxTokensClamped:       maxTokensClamped,
		Tokenizer:              baseEstimate.Tokenizer,
	}, nil
}

func (s *service) historyMessageGroups(ctx context.Context, branch []*runtimemodel.Message, includeImages bool) ([][]adapter.Message, error) {
	groups := make([][]adapter.Message, 0, len(branch))
	for _, item := range branch {
		if item == nil {
			continue
		}
		group := make([]adapter.Message, 0, 4)
		userMessage, err := s.historicalUserMessage(ctx, item, includeImages)
		if err != nil {
			return nil, err
		}
		if userMessage != nil {
			group = append(group, *userMessage)
		}
		if isUsableAgentTranscriptHistoryStatus(item.Status) {
			group = append(group, agentTranscriptFromMetadata(item.Metadata, item.Answer)...)
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
	if partsRequestsContinuationWithFallback(parts, "") {
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
	if strategy.RouteRequired || len(strategy.PlannedTools) > 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(strategy.AssetEffect)) {
	case "create", "update", "delete", "write", "mutation", "mutate":
		return true
	}
	return false
}

func currentTurnBoundaryMessage(parts *chatRequestParts) *adapter.Message {
	if parts == nil || partsRequestsContinuationWithFallback(parts, "") {
		return nil
	}
	content := strings.Join([]string{
		"Current assistant turn boundary:",
		"The latest user request below is the task to execute now.",
		"Use older conversation messages only as background facts.",
		"Do not continue, repeat, or complete earlier tasks unless the latest request explicitly asks to continue or reuse previous outputs.",
		"If older history or recent execution facts conflict with the latest user request, follow the latest user request.",
	}, "\n")
	return &adapter.Message{Role: "system", Content: content}
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
	for _, key := range []string{"version", "task_id", "intent", "task_type", "status", "original_user_goal", "risk_level", "approval", "planning_mode", "plan_sync_status"} {
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
	if criteria := stringSliceFromAny(plan["success_criteria"]); len(criteria) > 0 {
		out["success_criteria"] = compactStringSliceForPrompt(criteria, 8, 240)
	}
	if goals := operationPlanCompactCapabilityGoals(plan["capability_goals"], 6); len(goals) > 0 {
		out["capability_goals"] = goals
	}
	if contract := mapFromOperationContext(plan["task_contract"]); len(contract) > 0 {
		out["task_contract"] = compactTaskContractForPrompt(contract)
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
	if outcomes := operationPlanCompactOutcomesForPrompt(plan[operationPlanOutcomesKey], 8); len(outcomes) > 0 {
		out["outcomes"] = outcomes
	}
	for _, key := range []string{"last_plan_update_round", "evidence_revision", "evidence_revision_at_plan_update", "evidence_sequence_at_plan_update", "evidence_after_last_plan_update"} {
		if value := intValueFromAny(plan[key]); value > 0 {
			out[key] = value
		}
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
	if outcomes := operationPlanCompactOutcomesForPrompt(contract["outcomes"], 8); len(outcomes) > 0 {
		out["outcomes"] = outcomes
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
	for _, phase := range mapSliceFromAny(plan["phases"]) {
		status := strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"])))
		if status != operationPlanStepStatusCompleted && status != "skipped" {
			return true
		}
	}
	if len(mapSliceFromAny(plan["phases"])) == 0 {
		for _, step := range mapSliceFromAny(plan["steps"]) {
			status := strings.ToLower(strings.TrimSpace(stringFromAny(step["status"])))
			if status != operationPlanStepStatusCompleted && status != "skipped" && status != operationPlanStepStatusFailed {
				return true
			}
		}
		if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
			for _, operation := range mapSliceFromAny(structured["operations"]) {
				status := strings.ToLower(strings.TrimSpace(stringFromAny(operation["status"])))
				if status != operationPlanStepStatusCompleted && status != "skipped" && status != operationPlanStepStatusFailed {
					return true
				}
			}
		}
	}
	return false
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

func countAdapterMessages(groups [][]adapter.Message) int {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	return total
}

func contextControlMetadata(spec ModelSpec, budget *budgetComputation, estimatedPromptTokens int, historyBefore int, historyAfter int) map[string]interface{} {
	return map[string]interface{}{
		"strategy":                  contextControlStrategyTokenBudget,
		"model_context_window":      spec.ContextWindow,
		"configured_agent_window_k": budget.ConfiguredAgentWindowK,
		"agent_context_window":      budget.AgentContextWindow,
		"agent_window_clamped":      budget.AgentWindowClamped,
		"model_max_output_tokens":   spec.MaxOutputTokens,
		"safe_context_limit":        budget.SafeLimit,
		"prompt_budget":             budget.PromptBudget,
		"estimated_prompt_tokens":   estimatedPromptTokens,
		"history_messages_before":   historyBefore,
		"history_messages_after":    historyAfter,
		"truncated":                 historyAfter < historyBefore,
		"max_tokens_clamped":        budget.MaxTokensClamped,
		"original_max_tokens":       optionalIntValue(budget.OriginalMaxTokens),
		"effective_max_tokens":      optionalIntValue(budget.EffectiveMaxTokens),
		"reserved_output_tokens":    budget.ReservedOutputTokens,
		"tokenizer":                 budget.Tokenizer,
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
		return 16384
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

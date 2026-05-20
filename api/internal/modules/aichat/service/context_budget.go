package service

import (
	"context"
	"fmt"
	"strings"

	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/internal/modules/llm/tokenestimate"
)

const (
	contextBudgetSafetyNumerator   = 9
	contextBudgetSafetyDenominator = 10
	maxContextCandidateMessages    = 100

	contextControlStrategyTokenBudget = "token_budget"
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
	budget, err := s.computeContextBudget(spec, parts, baseMessages)
	if err != nil {
		return nil, err
	}
	currentContent, attachmentMetadata, estimatedPromptTokens := s.buildBudgetedCurrentUserContent(parts, systemPrompt, budget)

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
	messages = append(messages, adapter.Message{Role: "user", Content: currentContent})

	historyAfter := len(messages) - 2
	metadata := contextControlMetadata(spec, budget, estimatedPromptTokens, historyBefore, historyAfter)
	mergeAttachmentContextMetadata(metadata, attachmentMetadata)
	return &contextBudgetResult{
		Messages: messages,
		Metadata: metadata,
	}, nil
}

func (s *service) buildBudgetedCurrentUserContent(
	parts *chatRequestParts,
	systemPrompt string,
	budget *budgetComputation,
) (interface{}, map[string]interface{}, int) {
	if parts == nil || parts.Attachments == nil || len(parts.Attachments.Files) == 0 {
		return s.currentUserContent(parts, parts.Query), nil, budget.BasePromptTokens
	}

	fullSections := parts.Attachments.fullContentSections()
	if strings.TrimSpace(fullSections) == "" {
		return s.currentUserContent(parts, parts.Query), nil, budget.BasePromptTokens
	}

	attachmentTokensBefore := s.estimateAttachmentTokens(parts, fullSections)
	fullContent := userContentWithAttachments(parts.Query, fullSections)
	fullUserContent := s.currentUserContent(parts, fullContent)
	fullEstimate := s.estimateCurrentPromptTokens(systemPrompt, fullUserContent, parts.ModelName)
	if fullEstimate <= budget.PromptBudget {
		return fullUserContent, map[string]interface{}{
			"attachment_tokens_before": attachmentTokensBefore,
			"attachment_tokens_after":  attachmentTokensBefore,
			"attachments_truncated":    false,
		}, fullEstimate
	}

	sections, truncated := s.fitAttachmentSectionsToBudget(parts, systemPrompt, budget.PromptBudget)
	currentContent := userContentWithAttachments(parts.Query, sections)
	currentUserContent := s.currentUserContent(parts, currentContent)
	estimatedPromptTokens := s.estimateCurrentPromptTokens(systemPrompt, currentUserContent, parts.ModelName)
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

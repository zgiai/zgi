package service

import (
	"context"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	defaultMaxSkillStepsPerTurn = 6
)

func (p *PreparedChat) skillsEnabled() bool {
	if p == nil || p.parts == nil {
		return false
	}
	return p.parts.SkillMode != skillModeDisabled && len(p.parts.SkillIDs) > 0
}

func (s *service) runPreparedSkillStream(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onChunk func(string) error,
	onEvent func(StreamEvent) error,
) (string, *adapter.Usage, error) {
	if s.skillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.llmClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}

	execCtx := s.skillExecutionContext(prepared)
	custom, err := s.customSkillCatalogEntries(ctx, prepared.Scope.OrganizationID)
	if err != nil {
		return "", nil, err
	}
	resolved, err := s.skillRuntime.ResolveEnabledSkillsWithCustom(ctx, prepared.parts.SkillIDs, custom)
	if err != nil {
		return "", nil, err
	}
	if len(resolved.Skills) == 0 {
		return "", nil, fmt.Errorf("%w: no skills available for configured skill ids", ErrInvalidInput)
	}

	messages := append([]adapter.Message{}, prepared.LLMRequest.Messages...)
	metadataMessage, metadataStats := skills.SkillMetadataSystemMessageWithBudget(
		resolved.PromptMetadata(),
		skills.DefaultSkillMetadataPromptBudgetChars,
	)
	messages = append(messages, metadataMessage)
	metaTools := skills.MetaToolsForSkills(resolved)
	traces := []skills.SkillTrace{metadataExposedTrace(resolved.SkillIDs(), metadataStats)}
	s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
	logger.DebugContext(ctx, "aichat skill metadata exposed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_ids", resolved.SkillIDs(),
		"skill_mode", prepared.parts.SkillMode,
	)

	stepCount := 0
	toolCallCount := 0
	skillToolCallCounts := map[string]int{}
	skillUsed := false
	loadedSkills := map[string]struct{}{}
	maxSkillSteps := maxSkillStepsForTurn(resolved)

	for {
		planningReq := cloneChatRequest(prepared.LLMRequest)
		planningReq.Messages = messages
		planningReq.Stream = false
		planningReq.Tools = metaTools
		planningReq.ToolChoice = "auto"

		planningResp, err := s.llmClient.AppChat(ctx, newBillingAppContext(prepared), planningReq)
		if err != nil {
			return "", nil, err
		}
		planningMessage := firstPlanningMessage(planningResp)
		toolCalls := normalizeToolCalls(planningMessage.ToolCalls)
		if len(toolCalls) == 0 {
			if prepared.parts.SkillMode == skillModeRequired && !skillUsed {
				return "", nil, fmt.Errorf("%w: required skill was not used", ErrInvalidInput)
			}
			logger.DebugContext(ctx, "aichat skill planning completed",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"skill_step_count", stepCount,
				"tool_call_count", toolCallCount,
			)
			break
		}
		if stepCount+len(toolCalls) > maxSkillSteps {
			logger.WarnContext(ctx, "aichat skill step limit exceeded",
				"conversation_id", prepared.Conversation.ID.String(),
				"message_id", prepared.Message.ID.String(),
				"current_step_count", stepCount,
				"requested_tool_calls", len(toolCalls),
				"max_steps", maxSkillSteps,
			)
			return "", nil, fmt.Errorf("%w: too many skill steps", ErrInvalidInput)
		}
		logger.DebugContext(ctx, "aichat skill planning requested tool calls",
			"conversation_id", prepared.Conversation.ID.String(),
			"message_id", prepared.Message.ID.String(),
			"tool_call_count", len(toolCalls),
			"step_count", stepCount,
		)

		planningMessage.Role = "assistant"
		planningMessage.ToolCalls = toolCalls
		messages = append(messages, planningMessage)

		for _, call := range toolCalls {
			stepCount++
			trace, toolMessage, usedSkill, usedTool, err := s.handleProgressiveSkillCall(ctx, prepared, resolved, call, execCtx, toolCallCount, skillToolCallCounts, loadedSkills, onEvent)
			traces = append(traces, trace)
			s.persistSkillTracesBestEffort(persistCtx, prepared, traces)
			s.logSkillTrace(ctx, prepared, trace)
			if err != nil {
				s.emitSkillError(ctx, prepared, trace, onEvent)
				return "", nil, err
			}
			if usedSkill {
				skillUsed = true
			}
			if usedTool {
				toolCallCount++
				incrementSkillToolCallCount(skillToolCallCounts, trace.SkillID)
			}
			messages = append(messages, toolMessage)
		}
	}

	finalReq := cloneChatRequest(prepared.LLMRequest)
	finalReq.Messages = messages
	finalReq.Stream = true
	finalReq.Tools = nil
	finalReq.ToolChoice = nil
	prepared.LLMRequest = finalReq

	stream, err := s.openChatStream(ctx, prepared)
	if err != nil {
		return "", nil, err
	}
	return s.collectStreamAnswer(ctx, prepared, stream, onChunk)
}

func (s *service) handleProgressiveSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	call adapter.ToolCall,
	execCtx skills.ExecutionContext,
	currentToolCalls int,
	skillToolCallCounts map[string]int,
	loadedSkills map[string]struct{},
	onEvent func(StreamEvent) error,
) (skills.SkillTrace, adapter.Message, bool, bool, error) {
	args, err := skills.ParseArguments(call.Function.Arguments)
	if err != nil {
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return trace, skills.ToolResultMessage(call.ID, errorPayload(err)), false, false, err
	}
	switch call.Function.Name {
	case skills.MetaToolLoadSkill:
		return s.handleLoadSkillCall(ctx, prepared, resolved, call.ID, args, loadedSkills, onEvent)
	case skills.MetaToolReadSkillReference:
		if _, ok := loadedSkills[normalizedSkillArg(args, "skill_id")]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), "", "skill must be loaded before reading references")
			trace.SkillID = stringArg(args, "skill_id")
			return trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false, nil
		}
		return s.handleReadReferenceCall(ctx, prepared, resolved, call.ID, args, onEvent)
	case skills.MetaToolCallSkillTool:
		skillID := normalizedSkillArg(args, "skill_id")
		toolName := stringArg(args, "tool_name")
		toolArgs := mapArg(args, "arguments")
		if _, ok := loadedSkills[skillID]; !ok {
			trace := blockedSkillGuardrailTrace(stringArg(args, "skill_id"), toolName, "skill must be loaded before calling its tools")
			return trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), false, false, nil
		}
		if doc, ok := resolved.Get(skillID); ok && len(doc.Tools) == 0 {
			trace := blockedSkillGuardrailTrace(skillID, toolName, "skill does not provide callable tools")
			return trace, skills.ToolResultMessage(call.ID, guardrailPayload(trace)), true, false, nil
		}
		if currentToolCalls >= maxBusinessToolCalls(resolved) {
			err := fmt.Errorf("%w: too many skill tool calls", ErrInvalidInput)
			trace := skillToolLimitExceededTrace(skillID, toolName, toolArgs, err)
			return trace, skills.ToolResultMessage(call.ID, errorPayload(err)), false, false, err
		}
		if skillToolCallCounts[skillID] >= maxBusinessToolCallsForSkill(resolved, skillID) {
			err := fmt.Errorf("%w: too many skill tool calls for skill %s", ErrInvalidInput, skillID)
			trace := skillToolLimitExceededTrace(skillID, toolName, toolArgs, err)
			return trace, skills.ToolResultMessage(call.ID, errorPayload(err)), false, false, err
		}
		return s.handleCallSkillTool(ctx, prepared, resolved, call.ID, args, execCtx, onEvent)
	default:
		err := fmt.Errorf("%w: unsupported skill meta tool %s", ErrInvalidInput, call.Function.Name)
		trace := failedSkillTrace("meta_tool", call.Function.Name, err)
		return trace, skills.ToolResultMessage(call.ID, errorPayload(err)), false, false, err
	}
}

func (s *service) handleLoadSkillCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	loadedSkills map[string]struct{},
	onEvent func(StreamEvent) error,
) (skills.SkillTrace, adapter.Message, bool, bool, error) {
	skillID := stringArg(args, "skill_id")
	s.emitPreparedEvent(ctx, prepared, streamEventSkillLoadStart, skillLoadPayload(prepared, skillID), onEvent)
	doc, trace, err := s.skillRuntime.LoadSkill(ctx, resolved, skillID)
	if err != nil {
		return trace, skills.ToolResultMessage(callID, errorPayload(err)), false, false, err
	}
	loadedSkills[doc.Metadata.ID] = struct{}{}
	logger.DebugContext(ctx, "aichat skill loaded",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", doc.Metadata.ID,
	)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillLoadEnd, skillLoadEndPayload(prepared, trace), onEvent)
	return trace, skills.ToolResultMessage(callID, skillDocumentPayload(doc)), true, false, nil
}

func (s *service) handleReadReferenceCall(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	onEvent func(StreamEvent) error,
) (skills.SkillTrace, adapter.Message, bool, bool, error) {
	skillID := stringArg(args, "skill_id")
	path := stringArg(args, "path")
	content, trace, err := s.skillRuntime.ReadReference(ctx, resolved, skillID, path)
	if err != nil {
		return trace, skills.ToolResultMessage(callID, errorPayload(err)), false, false, err
	}
	logger.DebugContext(ctx, "aichat skill reference read",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", trace.SkillID,
		"path", path,
		"duration_ms", trace.DurationMS,
	)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillReferenceRead, skillReferenceReadPayload(prepared, trace, path), onEvent)
	return trace, skills.ToolResultMessage(callID, map[string]interface{}{
		"skill_id": skillID,
		"path":     path,
		"content":  content,
	}), true, false, nil
}

func (s *service) handleCallSkillTool(
	ctx context.Context,
	prepared *PreparedChat,
	resolved *skills.ResolvedSkills,
	callID string,
	args map[string]interface{},
	execCtx skills.ExecutionContext,
	onEvent func(StreamEvent) error,
) (skills.SkillTrace, adapter.Message, bool, bool, error) {
	skillID := stringArg(args, "skill_id")
	toolName := stringArg(args, "tool_name")
	toolArgs := mapArg(args, "arguments")
	argumentSummary := summarizeSkillToolArguments(skillID, toolName, toolArgs)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallStart, skillCallStartPayload(prepared, skillID, toolName, argumentSummary), onEvent)
	invocation, err := s.skillRuntime.CallSkillTool(ctx, resolved, skillID, toolName, toolArgs, execCtx, callID)
	if invocation == nil {
		trace := failedSkillTrace("tool_call", toolName, err)
		trace.SkillID = skillID
		trace.Arguments = argumentSummary
		return trace, skills.ToolResultMessage(callID, errorPayload(err)), false, false, err
	}
	invocation.Trace.Arguments = argumentSummary
	if err != nil {
		return invocation.Trace, skills.ToolResultMessage(callID, errorPayload(err)), false, false, err
	}
	logger.DebugContext(ctx, "aichat skill tool completed",
		"conversation_id", prepared.Conversation.ID.String(),
		"message_id", prepared.Message.ID.String(),
		"skill_id", invocation.Trace.SkillID,
		"tool_name", invocation.Trace.ToolName,
		"duration_ms", invocation.Trace.DurationMS,
	)
	s.emitPreparedEvent(ctx, prepared, streamEventSkillCallEnd, skillCallEndPayload(prepared, invocation.Trace), onEvent)
	for _, artifact := range skillArtifactsFromToolMessages(prepared, invocation.Trace, invocation.Messages) {
		s.persistGeneratedArtifactBestEffort(ctx, prepared, artifact)
		s.emitPreparedEvent(ctx, prepared, streamEventSkillArtifactCreated, artifact, onEvent)
	}
	return invocation.Trace, invocation.ToolMessage, true, true, nil
}

func (s *service) skillExecutionContext(prepared *PreparedChat) skills.ExecutionContext {
	tenantID := prepared.Scope.OrganizationID.String()
	if prepared.Scope.WorkspaceID != nil {
		tenantID = prepared.Scope.WorkspaceID.String()
	}
	return skills.ExecutionContext{
		TenantID:       tenantID,
		UserID:         prepared.Scope.AccountID.String(),
		ConversationID: prepared.Conversation.ID.String(),
		AppID:          prepared.Conversation.ID.String(),
		MessageID:      prepared.Message.ID.String(),
	}
}

func cloneChatRequest(source *adapter.ChatRequest) *adapter.ChatRequest {
	if source == nil {
		return &adapter.ChatRequest{}
	}
	cloned := *source
	cloned.Messages = append([]adapter.Message{}, source.Messages...)
	cloned.Stop = append([]string{}, source.Stop...)
	if source.AdditionalParameters != nil {
		cloned.AdditionalParameters = copyStringAnyMap(source.AdditionalParameters)
	}
	if source.LogitBias != nil {
		cloned.LogitBias = make(map[string]float64, len(source.LogitBias))
		for key, value := range source.LogitBias {
			cloned.LogitBias[key] = value
		}
	}
	return &cloned
}

func firstPlanningMessage(resp *adapter.ChatResponse) adapter.Message {
	if resp == nil || len(resp.Choices) == 0 {
		return adapter.Message{Role: "assistant"}
	}
	message := resp.Choices[0].Message
	if strings.TrimSpace(message.Role) == "" {
		message.Role = "assistant"
	}
	return message
}

func normalizeToolCalls(calls []adapter.ToolCall) []adapter.ToolCall {
	out := make([]adapter.ToolCall, 0, len(calls))
	for idx, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("call_%d", idx+1)
		}
		if strings.TrimSpace(call.Type) == "" {
			call.Type = "function"
		}
		index := idx
		if call.Index == nil {
			call.Index = &index
		}
		out = append(out, call)
	}
	return out
}

func (s *service) persistSkillTracesBestEffort(ctx context.Context, prepared *PreparedChat, traces []skills.SkillTrace) {
	if prepared == nil || prepared.Message == nil {
		return
	}
	metadata := mergeSkillTraceMetadata(prepared.Message.Metadata, traces)
	prepared.Message.Metadata = metadata
	_ = s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata)
}

func (s *service) persistGeneratedArtifactBestEffort(ctx context.Context, prepared *PreparedChat, artifact map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(artifact) == 0 {
		return
	}
	metadata := mergeGeneratedArtifactMetadata(prepared.Message.Metadata, artifact)
	prepared.Message.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat generated artifact metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func mergeGeneratedArtifactMetadata(source map[string]interface{}, artifact map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	storedArtifact := persistentGeneratedArtifact(artifact)
	files := generatedFilesFromMetadata(metadata["generated_files"])
	fileID := stringFromAny(storedArtifact["file_id"])
	for idx, item := range files {
		if fileID != "" && stringFromAny(item["file_id"]) == fileID {
			files[idx] = storedArtifact
			metadata["generated_files"] = files
			metadata["generated_file_count"] = len(files)
			return metadata
		}
	}
	files = append(files, storedArtifact)
	metadata["generated_files"] = files
	metadata["generated_file_count"] = len(files)
	return metadata
}

func mergeSkillTraceMetadata(source map[string]interface{}, traces []skills.SkillTrace) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if len(traces) == 0 {
		return metadata
	}
	selected := make([]interface{}, 0)
	loaded := make([]interface{}, 0)
	toolsUsed := make([]interface{}, 0)
	invocations := make([]interface{}, 0, len(traces))
	selectedSeen := map[string]struct{}{}
	loadedSeen := map[string]struct{}{}
	toolSeen := map[string]struct{}{}
	toolCallCount := 0
	guardrailCount := 0
	addConfiguredSkillIDs(metadata, selectedSeen, &selected)

	for _, trace := range traces {
		if trace.SkillID != "" {
			if _, exists := selectedSeen[trace.SkillID]; !exists {
				selectedSeen[trace.SkillID] = struct{}{}
				selected = append(selected, trace.SkillID)
			}
		}
		if trace.Kind == "skill_load" && trace.Status == "success" {
			if _, exists := loadedSeen[trace.SkillID]; trace.SkillID != "" && !exists {
				loadedSeen[trace.SkillID] = struct{}{}
				loaded = append(loaded, trace.SkillID)
			}
		}
		if trace.Kind == "tool_call" {
			toolCallCount++
			if _, exists := toolSeen[trace.ToolName]; trace.ToolName != "" && !exists {
				toolSeen[trace.ToolName] = struct{}{}
				toolsUsed = append(toolsUsed, trace.ToolName)
			}
		}
		if trace.Kind == "guardrail" {
			guardrailCount++
		}
		invocations = append(invocations, map[string]interface{}{
			"kind":        trace.Kind,
			"skill_id":    trace.SkillID,
			"tool_name":   trace.ToolName,
			"status":      trace.Status,
			"duration_ms": trace.DurationMS,
			"arguments":   trace.Arguments,
			"error":       trace.Error,
		})
	}
	metadata["has_trace"] = true
	metadata["selected_skill_ids"] = selected
	metadata["loaded_skill_ids"] = loaded
	actionTraceCount := countSkillActionTraces(traces)
	metadata["skill_step_count"] = actionTraceCount
	metadata["skill_call_count"] = actionTraceCount
	metadata["tool_call_count"] = toolCallCount
	metadata["guardrail_count"] = guardrailCount
	metadata["skill_names"] = selected
	metadata["tool_names"] = toolsUsed
	metadata["skill_invocations"] = invocations
	return metadata
}

func countSkillActionTraces(traces []skills.SkillTrace) int {
	count := 0
	for _, trace := range traces {
		switch trace.Kind {
		case "skill_load", "reference_read", "tool_call", "guardrail":
			count++
		}
	}
	return count
}

func addConfiguredSkillIDs(metadata map[string]interface{}, seen map[string]struct{}, out *[]interface{}) {
	value, ok := metadata["configured_skill_ids"]
	if !ok {
		return
	}
	add := func(raw string) {
		id := strings.TrimSpace(raw)
		if id == "" {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}
		seen[id] = struct{}{}
		*out = append(*out, id)
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
}

func generatedFilesFromMetadata(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return append([]map[string]interface{}{}, typed...)
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if file, ok := item.(map[string]interface{}); ok {
				out = append(out, file)
			}
		}
		return out
	default:
		return []map[string]interface{}{}
	}
}

func firstNonEmptyString(values ...interface{}) string {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text != "" {
			return text
		}
	}
	return ""
}

func appendDownloadQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if strings.Contains(rawURL, "download=") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func stringFromAny(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func maxBusinessToolCalls(resolved *skills.ResolvedSkills) int {
	if resolved == nil || len(resolved.Skills) == 0 {
		return 3
	}
	total := 0
	for _, doc := range resolved.Skills {
		if doc.Metadata.MaxCallsPerTurn <= 0 {
			total += 3
			continue
		}
		total += doc.Metadata.MaxCallsPerTurn
	}
	if total <= 0 {
		return 3
	}
	return total
}

func maxBusinessToolCallsForSkill(resolved *skills.ResolvedSkills, skillID string) int {
	if resolved == nil {
		return 3
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	for _, doc := range resolved.Skills {
		if strings.ToLower(strings.TrimSpace(doc.Metadata.ID)) != skillID {
			continue
		}
		if doc.Metadata.MaxCallsPerTurn > 0 {
			return doc.Metadata.MaxCallsPerTurn
		}
		return 3
	}
	return 3
}

func incrementSkillToolCallCount(counts map[string]int, skillID string) {
	if counts == nil {
		return
	}
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	if skillID == "" {
		return
	}
	counts[skillID]++
}

func maxSkillStepsForTurn(resolved *skills.ResolvedSkills) int {
	limit := maxBusinessToolCalls(resolved)
	if resolved != nil {
		limit += len(resolved.Skills) * 2
	}
	if limit < defaultMaxSkillStepsPerTurn {
		return defaultMaxSkillStepsPerTurn
	}
	return limit
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func normalizedSkillArg(args map[string]interface{}, key string) string {
	return strings.ToLower(stringArg(args, key))
}

func mapArg(args map[string]interface{}, key string) map[string]interface{} {
	if args == nil {
		return map[string]interface{}{}
	}
	value, ok := args[key]
	if !ok || value == nil {
		return map[string]interface{}{}
	}
	if typed, ok := value.(map[string]interface{}); ok {
		return typed
	}
	return map[string]interface{}{}
}

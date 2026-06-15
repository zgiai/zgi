package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
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
	return s.runPreparedSkillStreamWithFinalAnswerGuard(ctx, persistCtx, prepared, onChunk, onEvent, nil)
}

func (s *service) runPreparedSkillStreamWithFinalAnswerGuard(
	ctx context.Context,
	persistCtx context.Context,
	prepared *PreparedChat,
	onChunk func(string) error,
	onEvent func(StreamEvent) error,
	extraFinalAnswerGuard skillloop.FinalAnswerGuard,
) (string, *adapter.Usage, error) {
	if s.skillRuntime == nil {
		return "", nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.llmClient == nil {
		return "", nil, fmt.Errorf("llm client is not configured")
	}
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

	timeline := newProcessTimelineRecorder(ctx, persistCtx, s, prepared, onEvent)
	runner := &skillloop.Runner{
		LLMClient:    s.llmClient,
		SkillRuntime: s.skillRuntime,
		AppContext:   newBillingAppContext(prepared),
		OnEvent: func(event skillloop.Event) error {
			if event.Type == skillloop.EventUserInputRequested {
				s.persistUserInputRequestBestEffort(persistCtx, prepared, event.Payload)
			}
			timeline.RecordEvent(event.Type, event.Payload)
			return nil
		},
		OnTrace: func(traces []skills.SkillTrace, trace skills.SkillTrace) {
			timeline.RecordTrace(traces, trace)
		},
		OnArtifact: func(artifact map[string]interface{}) {
			s.persistGeneratedArtifactBestEffort(ctx, prepared, artifact)
		},
		OnModelInvocation: func(trace skillloop.ModelInvocationTrace) {
			s.persistModelInvocationBestEffort(persistCtx, prepared, trace)
		},
	}
	return runner.Run(ctx, skillloop.RunRequest{
		Prepared: skillloop.NewPreparedChat(
			prepared.Conversation.ID.String(),
			prepared.Message.ID.String(),
			prepared.parts.Provider,
			prepared.parts.SkillMode,
			prepared.LLMRequest,
		),
		Resolved:                 resolved,
		ExecutionContext:         s.skillExecutionContext(prepared),
		AdditionalSystemMessages: skillLoopAdditionalSystemMessages(prepared),
		FinalAnswerGuard:         combineFinalAnswerGuards(skillLoopFinalAnswerGuard(prepared), extraFinalAnswerGuard),
		UserInputGuard:           skillLoopUserInputGuard(prepared),
		OnChunk:                  onChunk,
	})
}

func combineFinalAnswerGuards(guards ...skillloop.FinalAnswerGuard) skillloop.FinalAnswerGuard {
	active := make([]skillloop.FinalAnswerGuard, 0, len(guards))
	for _, guard := range guards {
		if guard != nil {
			active = append(active, guard)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		for _, guard := range active {
			if result, blocked := guard(req); blocked {
				return result, true
			}
		}
		return skillloop.FinalAnswerGuardResult{}, false
	}
}

func (s *service) skillExecutionContext(prepared *PreparedChat) skills.ExecutionContext {
	appID := prepared.Conversation.ID.String()
	if strings.TrimSpace(prepared.RunConfig.BillingAppID) != "" {
		appID = strings.TrimSpace(prepared.RunConfig.BillingAppID)
	}
	invokeFrom := tools.ToolInvokeFromAIChat
	if normalizeCallerType(prepared.Caller.Type) == runtimemodel.ConversationCallerAgent {
		invokeFrom = tools.ToolInvokeFromAgent
	}
	return skills.ExecutionContext{
		OrganizationID:    prepared.Scope.OrganizationID.String(),
		UserID:            prepared.Scope.AccountID.String(),
		ConversationID:    prepared.Conversation.ID.String(),
		AppID:             appID,
		MessageID:         prepared.Message.ID.String(),
		InvokeFrom:        invokeFrom,
		RuntimeParameters: skillRuntimeParametersForPrepared(prepared),
	}
}

func skillRuntimeParameters(scope Scope, config RunConfig) map[string]interface{} {
	return runtimeCapabilityConfigFromRunConfig(config).RuntimeParameters(scope, config.BillingAppType)
}

func skillRuntimeParametersForPrepared(prepared *PreparedChat) map[string]interface{} {
	params := skillRuntimeParameters(prepared.Scope, prepared.RunConfig)
	params = applySkillToolGovernanceRuntimeParameters(params, prepared)
	if prepared != nil && prepared.parts != nil && isConsoleFilesContext(prepared.parts.RuntimeContext, prepared.parts.RawOperationContext, prepared.parts.OperationContext) {
		if visibleFiles := consoleFilesPromptVisibleFiles(prepared.parts); len(visibleFiles) > 0 {
			params["console_files_visible_files"] = visibleFiles
		}
	}
	if history := workflowConversationHistoryFromPrepared(prepared); len(history) > 0 {
		params["workflow_context"] = map[string]interface{}{
			"conversation_history": history,
		}
	}
	return params
}

func skillLoopAdditionalSystemMessages(prepared *PreparedChat) []adapter.Message {
	if prepared == nil {
		return nil
	}
	messages := make([]adapter.Message, 0, 2)
	if message, ok := agentWorkflowAvailableBindingsMessage(prepared.RunConfig.WorkflowBindings); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualConsoleFilesSkillMessage(prepared); ok {
		messages = append(messages, message)
	}
	return messages
}

func contextualConsoleFilesSkillMessage(prepared *PreparedChat) (adapter.Message, bool) {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillFileReader) {
		return adapter.Message{}, false
	}
	parts := prepared.parts
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return adapter.Message{}, false
	}
	hasRead := hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
	hasDelete := hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext)
	if !hasRead && !hasDelete {
		return adapter.Message{}, false
	}

	payload := map[string]interface{}{
		"page":            "console.files",
		"preferred_skill": skills.SkillFileReader,
		"visible_files":   consoleFilesPromptVisibleFiles(parts),
	}
	tools := make([]map[string]string, 0, 3)
	if hasRead {
		tools = append(tools, map[string]string{
			"capability_id": "file.list_visible",
			"skill_id":      skills.SkillFileReader,
			"tool_name":     "list_visible_files",
		})
		tools = append(tools, map[string]string{
			"capability_id": "file.read",
			"skill_id":      skills.SkillFileReader,
			"tool_name":     "read_file",
		})
	}
	if hasDelete {
		tools = append(tools, map[string]string{
			"capability_id": "file.delete",
			"skill_id":      skills.SkillFileReader,
			"tool_name":     "delete_file",
		})
	}
	payload["tools"] = tools
	if targets := consoleFilesPromptResolvedTargets(parts); len(targets) > 0 {
		payload["resolved_targets_from_user_request"] = targets
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}

	content := strings.Join([]string{
		"Contextual files-page tool guidance:",
		"The user is operating on the Console Files page. Treat visible file resources in operation_context as concrete user assets.",
		"For requests that only ask what files are visible, available, selected, or present on the Files page, use file-reader/list_visible_files and answer from the tool result.",
		"For requests about reading, previewing, summarizing, analyzing, or translating visible file contents, use file-reader/read_file with the resolved file_id.",
		"When resolved_targets_from_user_request is present, the target is already resolved from the current page. Use the listed file_id exactly for read_file/delete_file; it overrides any other ordinal or file-type interpretation.",
		"Do not ask the user to select a file, repeat the file name, or choose another visible file with the same type when a resolved target is present.",
		"After read_file returns content_status \"extracted\", answer from the returned content field and continue requested post-processing such as summary or translation. Do not say the file cannot be read.",
		"For requests about deleting or removing a resolved visible file, use file-reader/delete_file with exactly that file_id. Tool governance handles the approval card before deletion; do not ask for a separate natural-language confirmation first.",
		"If a prior approval or session grant exists, it only skips the approval prompt. You must still call file-reader/delete_file in this turn and wait for the tool result before saying the file was deleted.",
		"Never claim a file was deleted, removed, updated, created, saved, or otherwise changed based only on previous conversation context.",
		"If the target file is missing or ambiguous, call request_user_input with a concise clarification instead of guessing.",
		"Do not call unrelated discovery or domain tools, such as database, knowledge, calculator, or file-generation tools, before completing the requested files-page asset operation.",
		"Files-page context JSON: " + string(encoded),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func skillLoopFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillFileReader) {
		return nil
	}
	parts := prepared.parts
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return nil
	}
	if hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		isFileListIntent(parts.Query) {
		return consoleFilesListRequiredToolFinalAnswerGuard()
	}
	targets := consoleFilesPromptResolvedTargets(parts)
	if len(targets) == 0 {
		return nil
	}
	if hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext) &&
		isFileDeleteIntent(parts.Query) {
		return consoleFilesRequiredToolFinalAnswerGuard(targets, "delete_file", []string{
			"The user's current files-page request is a concrete file deletion request for {target}.",
			"Do not finish with a natural-language success message yet.",
			"Load the file-reader skill if needed, then call call_skill_tool with skill_id \"file-reader\", tool_name \"delete_file\", and the resolved file_id for the target file.",
			"A session approval grant may skip the approval card, but it does not replace the delete_file tool call.",
			"Only after delete_file succeeds in this turn may you tell the user that the file was deleted. If the tool fails or the file is already missing, report the actual tool result.",
		})
	}
	if hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		isFileReadIntent(parts.Query) {
		return consoleFilesRequiredToolFinalAnswerGuard(targets, "read_file", []string{
			"The user's current files-page request requires reading the actual content of {target}.",
			"Do not finish from visible page metadata, file names, or prior conversation context.",
			"Load the file-reader skill if needed, then call call_skill_tool with skill_id \"file-reader\", tool_name \"read_file\", and the resolved file_id for the target file.",
			"Only after read_file succeeds in this turn may you summarize, translate, quote, or answer from the file content. If the tool fails or returns empty content, report the actual tool result.",
		})
	}
	return nil
}

func skillLoopUserInputGuard(prepared *PreparedChat) skillloop.UserInputGuard {
	finalGuard := skillLoopFinalAnswerGuard(prepared)
	if finalGuard == nil {
		return nil
	}
	return func(req skillloop.UserInputGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		result, blocked := finalGuard(skillloop.FinalAnswerGuardRequest{
			Answer:              req.Message,
			Round:               req.Round,
			SkillUsed:           req.SkillUsed,
			ToolCallCount:       req.ToolCallCount,
			AttemptedToolCalls:  req.AttemptedToolCalls,
			SuccessfulToolCalls: req.SuccessfulToolCalls,
		})
		if !blocked {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		result.Message = strings.Join([]string{
			"The request_user_input call was blocked because the files-page target is already resolved in runtime context.",
			"Do not ask the user to choose between visible files, repeat a known file name, or confirm information already represented by resolved_targets_from_user_request.",
			result.Message,
		}, " ")
		return result, true
	}
}

func consoleFilesListRequiredToolFinalAnswerGuard() skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulTool(req, skills.SkillFileReader, "list_visible_files") ||
			finalAnswerGuardHasAttemptedTool(req, skills.SkillFileReader, "list_visible_files") {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:  skills.SkillFileReader,
			ToolName: "list_visible_files",
			Message: strings.Join([]string{
				"The user's current files-page request asks which files are visible or available.",
				"Do not finish from visible page metadata or prior conversation context.",
				"Load the file-reader skill if needed, then call call_skill_tool with skill_id \"file-reader\" and tool_name \"list_visible_files\".",
				"Only after list_visible_files succeeds in this turn may you list the current visible files.",
			}, " "),
		}, true
	}
}

func consoleFilesRequiredToolFinalAnswerGuard(targets []map[string]interface{}, toolName string, messageTemplates []string) skillloop.FinalAnswerGuard {
	targetSummary := consoleFilesGuardTargetSummary(targets)
	targetFileIDs := consoleFilesGuardTargetFileIDs(targets)
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulToolForTargets(req, skills.SkillFileReader, toolName, targetFileIDs) ||
			finalAnswerGuardHasAttemptedToolForTargets(req, skills.SkillFileReader, toolName, targetFileIDs) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		lines := make([]string, 0, len(messageTemplates))
		for _, template := range messageTemplates {
			lines = append(lines, strings.ReplaceAll(template, "{target}", targetSummary))
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:  skills.SkillFileReader,
			ToolName: toolName,
			Message:  strings.Join(lines, " "),
		}, true
	}
}

func finalAnswerGuardHasSuccessfulTool(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string) bool {
	return finalAnswerGuardHasSuccessfulToolForTargets(req, skillID, toolName, nil)
}

func finalAnswerGuardHasSuccessfulToolForTargets(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, targetFileIDs []string) bool {
	return finalAnswerGuardHasToolForTargets(req.SuccessfulToolCalls, skillID, toolName, targetFileIDs)
}

func finalAnswerGuardHasAttemptedTool(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string) bool {
	return finalAnswerGuardHasAttemptedToolForTargets(req, skillID, toolName, nil)
}

func finalAnswerGuardHasAttemptedToolForTargets(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, targetFileIDs []string) bool {
	return finalAnswerGuardHasToolForTargets(req.AttemptedToolCalls, skillID, toolName, targetFileIDs)
}

func finalAnswerGuardHasToolForTargets(calls []skillloop.SkillToolCallRef, skillID string, toolName string, targetFileIDs []string) bool {
	if len(targetFileIDs) == 0 {
		for _, call := range calls {
			if strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) &&
				strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
				return true
			}
		}
		return false
	}
	required := map[string]struct{}{}
	for _, id := range targetFileIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			required[id] = struct{}{}
		}
	}
	if len(required) == 0 {
		return false
	}
	matched := map[string]struct{}{}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			continue
		}
		actual := skillToolCallFileIDs(call.Arguments)
		for _, got := range actual {
			if _, ok := required[got]; ok {
				matched[got] = struct{}{}
			}
		}
	}
	return len(matched) == len(required)
}

func skillToolCallFileIDs(arguments map[string]interface{}) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(value interface{}) {
		switch typed := value.(type) {
		case []string:
			for _, item := range typed {
				if id := strings.TrimSpace(item); id != "" {
					if _, ok := seen[id]; !ok {
						seen[id] = struct{}{}
						out = append(out, id)
					}
				}
			}
		case []interface{}:
			for _, item := range typed {
				if id := strings.TrimSpace(stringFromAny(item)); id != "" {
					if _, ok := seen[id]; !ok {
						seen[id] = struct{}{}
						out = append(out, id)
					}
				}
			}
		default:
			if id := strings.TrimSpace(stringFromAny(value)); id != "" {
				if _, ok := seen[id]; !ok {
					seen[id] = struct{}{}
					out = append(out, id)
				}
			}
		}
	}
	add(arguments["file_id"])
	add(arguments["file_ids"])
	return out
}

func consoleFilesGuardTargetFileIDs(targets []map[string]interface{}) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, target := range targets {
		id := strings.TrimSpace(stringFromAny(target["file_id"]))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func consoleFilesGuardTargetSummary(targets []map[string]interface{}) string {
	if len(targets) == 0 {
		return "the resolved visible file"
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		fileID := strings.TrimSpace(stringFromAny(target["file_id"]))
		name := strings.TrimSpace(stringFromAny(target["name"]))
		switch {
		case name != "" && fileID != "":
			parts = append(parts, fmt.Sprintf("%s (%s)", name, fileID))
		case name != "":
			parts = append(parts, name)
		case fileID != "":
			parts = append(parts, fileID)
		}
	}
	if len(parts) == 0 {
		return "the resolved visible file"
	}
	return strings.Join(parts, ", ")
}

func isFileListIntent(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || isFileReadIntent(query) || isFileDeleteIntent(query) {
		return false
	}
	for _, phrase := range []string{
		"what files",
		"which files",
		"list files",
		"list the files",
		"visible files",
		"current files",
		"available files",
		"files do i have",
		"files are there",
		"files on this page",
		"有哪些文件",
		"哪些文件",
		"有什么文件",
		"有几个文件",
		"当前文件",
		"可见文件",
		"文件列表",
		"列出文件",
		"列一下文件",
		"看到哪些文件",
		"现在有文件",
	} {
		if strings.Contains(query, phrase) {
			return true
		}
	}
	return false
}

func skillIDEnabled(skillIDs []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	for _, raw := range skillIDs {
		if strings.EqualFold(strings.TrimSpace(raw), target) {
			return true
		}
	}
	return false
}

func consoleFilesPromptVisibleFiles(parts *chatRequestParts) []map[string]interface{} {
	if parts == nil {
		return nil
	}
	files := visibleFileResources(parts.RawOperationContext)
	if len(files) == 0 {
		files = visibleFileResources(parts.OperationContext)
	}
	out := make([]map[string]interface{}, 0, min(len(files), 10))
	for idx, file := range files {
		if idx >= 10 {
			break
		}
		out = append(out, map[string]interface{}{
			"visible_index": idx + 1,
			"file_id":       file.ID,
			"name":          file.Title,
			"extension":     file.Extension,
			"mime_type":     file.MimeType,
			"selected":      file.Selected,
		})
	}
	return out
}

func consoleFilesPromptResolvedTargets(parts *chatRequestParts) []map[string]interface{} {
	refs := plannerResourceRefsFromConsoleFilesQuery(parts)
	if len(refs) == 0 {
		return nil
	}
	result := resolveChatResourceRefs(parts, refs)
	if !allResourceRefsResolved(result.Results) || len(result.FileIDs) == 0 {
		return nil
	}
	namesByID := map[string]string{}
	for _, file := range append(visibleFileResources(parts.RawOperationContext), visibleFileResources(parts.OperationContext)...) {
		if file.ID != "" && file.Title != "" {
			namesByID[file.ID] = file.Title
		}
	}
	out := make([]map[string]interface{}, 0, len(result.FileIDs))
	for _, id := range result.FileIDs {
		item := map[string]interface{}{"file_id": id}
		if name := namesByID[id]; name != "" {
			item["name"] = name
		}
		out = append(out, item)
	}
	return out
}

func agentWorkflowAvailableBindingsMessage(bindings []AgentWorkflowBinding) (adapter.Message, bool) {
	items := agentWorkflowPromptBindings(bindings)
	if len(items) == 0 {
		return adapter.Message{}, false
	}
	payload, err := json.Marshal(map[string]interface{}{"available_workflows": items})
	if err != nil {
		return adapter.Message{}, false
	}
	content := strings.Join([]string{
		"The current Agent can call these bound workflows through the agent-workflow skill.",
		"Use this injected available_workflows list first when selecting a workflow binding. Call list_agent_workflows only if this list is missing, ambiguous, or stale.",
		"Never invent workflow IDs or pass workflow_id/agent_id. Call run_agent_workflow with a binding_id from available_workflows.",
		"For single-input or conversational workflows, pass the user's current request in inputs.query unless the binding's input_schema, required_inputs, or default_input_key says otherwise.",
		"Available workflows JSON: " + string(payload),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func agentWorkflowPromptBindings(bindings []AgentWorkflowBinding) []map[string]interface{} {
	normalized := copyAgentWorkflowBindings(bindings)
	out := make([]map[string]interface{}, 0, len(normalized))
	seen := map[string]struct{}{}
	for _, binding := range normalized {
		if strings.TrimSpace(binding.BindingID) == "" {
			continue
		}
		if _, exists := seen[binding.BindingID]; exists {
			continue
		}
		seen[binding.BindingID] = struct{}{}
		defaultInputKey := agentWorkflowDefaultInputKey(binding)
		requiredInputs := agentWorkflowRequiredInputs(binding)
		out = append(out, map[string]interface{}{
			"binding_id":        binding.BindingID,
			"label":             binding.Label,
			"description":       binding.Description,
			"agent_type":        binding.AgentType,
			"version_strategy":  agentWorkflowVersionStrategy(binding.VersionStrategy),
			"timeout_seconds":   agentWorkflowTimeoutSeconds(binding.TimeoutSeconds),
			"input_schema":      agentWorkflowInputSchema(binding, requiredInputs),
			"required_inputs":   requiredInputs,
			"default_input_key": defaultInputKey,
			"start_inputs":      binding.StartInputs,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.Compare(fmt.Sprint(out[i]["binding_id"]), fmt.Sprint(out[j]["binding_id"])) < 0
	})
	return out
}

func agentWorkflowInputSchema(binding AgentWorkflowBinding, requiredInputs []string) map[string]interface{} {
	if len(binding.StartInputs) > 0 {
		properties := map[string]interface{}{}
		for _, input := range binding.StartInputs {
			variable := strings.TrimSpace(input.Variable)
			if variable == "" {
				continue
			}
			description := strings.TrimSpace(input.Label)
			if description == "" {
				description = "Workflow start input."
			}
			properties[variable] = map[string]interface{}{
				"type":        agentWorkflowJSONSchemaType(input.Type),
				"description": description,
			}
		}
		return map[string]interface{}{
			"type":                 "object",
			"properties":           properties,
			"required":             requiredInputs,
			"additionalProperties": true,
		}
	}
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The user's current request or instruction to pass into the workflow.",
			},
		},
		"required":             []string{"query"},
		"additionalProperties": true,
	}
}

func agentWorkflowRequiredInputs(binding AgentWorkflowBinding) []string {
	if len(binding.RequiredInputs) > 0 {
		allowed := map[string]struct{}{}
		for _, input := range binding.StartInputs {
			if variable := strings.TrimSpace(input.Variable); variable != "" {
				allowed[variable] = struct{}{}
			}
		}
		out := make([]string, 0, len(binding.RequiredInputs))
		seen := map[string]struct{}{}
		for _, item := range binding.RequiredInputs {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[item]; !ok {
					continue
				}
			}
			if _, exists := seen[item]; exists {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
		if len(out) > 0 {
			return out
		}
	}
	out := make([]string, 0, len(binding.StartInputs))
	for _, input := range binding.StartInputs {
		if input.Required && strings.TrimSpace(input.Variable) != "" {
			out = append(out, strings.TrimSpace(input.Variable))
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(binding.StartInputs) == 0 {
		return []string{"query"}
	}
	return []string{}
}

func agentWorkflowDefaultInputKey(binding AgentWorkflowBinding) string {
	key := strings.TrimSpace(binding.DefaultInputKey)
	if key != "" && agentWorkflowStartInputExists(binding.StartInputs, key) {
		return key
	}
	required := agentWorkflowRequiredInputs(binding)
	if len(required) == 1 {
		return required[0]
	}
	if agentWorkflowStartInputExists(binding.StartInputs, "query") {
		return "query"
	}
	if len(binding.StartInputs) == 1 {
		return strings.TrimSpace(binding.StartInputs[0].Variable)
	}
	return "query"
}

func agentWorkflowStartInputExists(inputs []AgentWorkflowStartInput, key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	for _, input := range inputs {
		if strings.TrimSpace(input.Variable) == key {
			return true
		}
	}
	return false
}

func agentWorkflowJSONSchemaType(inputType string) string {
	switch strings.ToLower(strings.TrimSpace(inputType)) {
	case "number", "integer":
		return "number"
	case "boolean", "bool":
		return "boolean"
	case "object":
		return "object"
	case "array":
		return "array"
	default:
		return "string"
	}
}

func agentWorkflowVersionStrategy(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "latest_published"
	}
	return value
}

func agentWorkflowTimeoutSeconds(value int) int {
	if value <= 0 {
		return 600
	}
	if value < 30 {
		return 30
	}
	if value > 1800 {
		return 1800
	}
	return value
}

func workflowConversationHistoryFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.LLMRequest == nil || len(prepared.LLMRequest.Messages) == 0 {
		return nil
	}
	messages := prepared.LLMRequest.Messages
	lastUserIndex := -1
	for idx := len(messages) - 1; idx >= 0; idx-- {
		if strings.EqualFold(strings.TrimSpace(messages[idx].Role), "user") {
			lastUserIndex = idx
			break
		}
	}
	out := make([]map[string]interface{}, 0, len(messages))
	for idx, message := range messages {
		if idx == lastUserIndex {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(messageContentText(message.Content))
		if content == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func messageContentText(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return typed
	case []adapter.MessageContentPart:
		var builder strings.Builder
		for _, part := range typed {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(part.Text)
		}
		return builder.String()
	case []interface{}:
		var builder strings.Builder
		for _, raw := range typed {
			part, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			text := strings.TrimSpace(fmt.Sprint(part["text"]))
			if text == "" || text == "<nil>" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(text)
		}
		return builder.String()
	default:
		return ""
	}
}

func copyAgentDatabaseBindings(input []AgentDatabaseBinding) []AgentDatabaseBinding {
	out := make([]AgentDatabaseBinding, 0, len(input))
	for _, binding := range input {
		if strings.TrimSpace(binding.DataSourceID) == "" || len(binding.TableIDs) == 0 {
			continue
		}
		out = append(out, AgentDatabaseBinding{
			DataSourceID:     strings.TrimSpace(binding.DataSourceID),
			TableIDs:         append([]string(nil), binding.TableIDs...),
			WritableTableIDs: append([]string(nil), binding.WritableTableIDs...),
		})
	}
	return out
}

func copyAgentWorkflowBindings(input []AgentWorkflowBinding) []AgentWorkflowBinding {
	out := make([]AgentWorkflowBinding, 0, len(input))
	for _, binding := range input {
		if strings.TrimSpace(binding.BindingID) == "" || strings.TrimSpace(binding.AgentID) == "" || strings.TrimSpace(binding.WorkflowID) == "" {
			continue
		}
		out = append(out, AgentWorkflowBinding{
			BindingID:       strings.TrimSpace(binding.BindingID),
			Label:           strings.TrimSpace(binding.Label),
			Description:     strings.TrimSpace(binding.Description),
			AgentID:         strings.TrimSpace(binding.AgentID),
			WorkflowID:      strings.TrimSpace(binding.WorkflowID),
			AgentType:       strings.TrimSpace(binding.AgentType),
			VersionStrategy: strings.TrimSpace(binding.VersionStrategy),
			VersionUUID:     strings.TrimSpace(binding.VersionUUID),
			TimeoutSeconds:  binding.TimeoutSeconds,
			StartInputs:     copyAgentWorkflowStartInputs(binding.StartInputs),
			RequiredInputs:  append([]string(nil), binding.RequiredInputs...),
			DefaultInputKey: strings.TrimSpace(binding.DefaultInputKey),
		})
	}
	return out
}

func copyAgentWorkflowStartInputs(input []AgentWorkflowStartInput) []AgentWorkflowStartInput {
	out := make([]AgentWorkflowStartInput, 0, len(input))
	for _, item := range input {
		variable := strings.TrimSpace(item.Variable)
		if variable == "" {
			continue
		}
		out = append(out, AgentWorkflowStartInput{
			Variable: variable,
			Label:    strings.TrimSpace(item.Label),
			Type:     strings.TrimSpace(item.Type),
			Required: item.Required,
		})
	}
	return out
}

func mergeUsage(current *adapter.Usage, next *adapter.Usage) *adapter.Usage {
	if next == nil {
		return current
	}
	if current == nil {
		cloned := *next
		return &cloned
	}
	current.PromptTokens += next.PromptTokens
	current.CompletionTokens += next.CompletionTokens
	current.TotalTokens += next.TotalTokens
	return current
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

func agenticSkillLoopSystemMessage() adapter.Message {
	return skillloop.AgenticSkillLoopSystemMessage()
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
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
		FinalAnswerGuard:         combineFinalAnswerGuards(extraFinalAnswerGuard, skillLoopFinalAnswerGuard(prepared)),
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
	if workspaceID := preparedSkillWorkspaceID(prepared); workspaceID != "" {
		params["workspace_id"] = workspaceID
	}
	params = applySkillToolGovernanceRuntimeParameters(params, prepared)
	if prepared != nil && prepared.parts != nil && isConsoleFilesContext(prepared.parts.RuntimeContext, prepared.parts.RawOperationContext, prepared.parts.OperationContext) {
		params["console_files_page"] = true
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

func preparedSkillWorkspaceID(prepared *PreparedChat) string {
	if prepared == nil {
		return ""
	}
	if prepared.Scope.WorkspaceID != nil && *prepared.Scope.WorkspaceID != uuid.Nil {
		return prepared.Scope.WorkspaceID.String()
	}
	if prepared.Conversation != nil && prepared.Conversation.WorkspaceID != nil && *prepared.Conversation.WorkspaceID != uuid.Nil {
		return prepared.Conversation.WorkspaceID.String()
	}
	return ""
}

func skillLoopAdditionalSystemMessages(prepared *PreparedChat) []adapter.Message {
	if prepared == nil {
		return nil
	}
	messages := make([]adapter.Message, 0, 3)
	if message, ok := agentWorkflowAvailableBindingsMessage(prepared.RunConfig.WorkflowBindings); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualConsoleNavigationSkillMessage(prepared); ok {
		messages = append(messages, message)
	}
	if message, ok := contextualConsoleFilesSkillMessage(prepared); ok {
		messages = append(messages, message)
	}
	return messages
}

type consoleNavigationRouteHint struct {
	Href     string   `json:"href"`
	Label    string   `json:"label"`
	Keywords []string `json:"keywords,omitempty"`
}

var consoleNavigationRouteHints = []consoleNavigationRouteHint{
	{Href: "/console", Label: "首页", Keywords: []string{"首页", "主页", "控制台首页", "home"}},
	{Href: "/console/work/chat", Label: "对话", Keywords: []string{"对话页面", "聊天页面", "会话页面", "conversation", "chat page"}},
	{Href: "/console/work/image", Label: "绘图", Keywords: []string{"绘图", "图像生成", "图片生成", "image page", "drawing"}},
	{Href: "/console/work/app", Label: "应用", Keywords: []string{"应用页面", "应用管理", "app page", "apps page"}},
	{Href: "/console/work/task", Label: "定时任务", Keywords: []string{"定时任务", "计划任务", "任务页面", "scheduled task", "tasks page"}},
	{Href: "/console/agents", Label: "智能体", Keywords: []string{"智能体", "agent page", "agents page", "agent list"}},
	{Href: "/console/agents", Label: "工作流", Keywords: []string{"工作流页面", "工作流列表", "workflow page", "workflows page"}},
	{Href: "/console/dataset", Label: "知识库", Keywords: []string{"知识库", "数据集", "dataset", "knowledge base"}},
	{Href: "/console/db", Label: "数据库", Keywords: []string{"数据库", "数据表", "database", "db page"}},
	{Href: "/console/files", Label: "文件管理", Keywords: []string{"文件管理", "文件页", "文件页面", "文件模块", "files page", "file management"}},
	{Href: "/console/prompts", Label: "提示词", Keywords: []string{"提示词", "prompt", "prompts page"}},
	{Href: "/console/developer/content-parse", Label: "文件识别", Keywords: []string{"文件识别", "内容解析", "content parse", "file recognition"}},
	{Href: "/console/workspace", Label: "工作空间", Keywords: []string{"工作空间", "workspace"}},
	{Href: "/console/settings", Label: "系统设置", Keywords: []string{"系统设置", "设置页面", "settings"}},
}

func contextualConsoleNavigationSkillMessage(prepared *PreparedChat) (adapter.Message, bool) {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator) {
		return adapter.Message{}, false
	}

	routes := make([]map[string]string, 0, len(consoleNavigationRouteHints))
	seen := map[string]struct{}{}
	for _, route := range consoleNavigationRouteHints {
		key := route.Href + "\x00" + route.Label
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, map[string]string{
			"href":  route.Href,
			"label": route.Label,
		})
	}
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"routes":    routes,
	}
	if target, ok := resolveConsoleNavigationTarget(prepared.parts.Query); ok {
		payload["resolved_target_from_user_request"] = map[string]string{
			"href":  target.Href,
			"label": target.Label,
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}

	content := strings.Join([]string{
		"ZGI console navigation guidance:",
		"Use console-navigator/navigate when the user asks to open, go to, enter, switch to, or navigate to a known ZGI console module page.",
		"Do not use request_user_input when the destination is resolved from the site map.",
		"Do not say a page has been opened unless console-navigator/navigate succeeded in this turn. If the navigate tool fails, report that failure plainly.",
		"Navigation does not mutate user assets and must use only whitelisted internal /console routes.",
		"Console navigation JSON: " + string(encoded),
	}, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func contextualConsoleFilesSkillMessage(prepared *PreparedChat) (adapter.Message, bool) {
	if prepared == nil || prepared.parts == nil {
		return adapter.Message{}, false
	}
	parts := prepared.parts
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return adapter.Message{}, false
	}
	fileReaderEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileReader)
	fileManagerEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileManager)
	fileGeneratorEnabled := skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator)
	if !fileReaderEnabled && !fileManagerEnabled && !fileGeneratorEnabled {
		return adapter.Message{}, false
	}
	hasRead := fileReaderEnabled && hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
	hasDelete := fileManagerEnabled && hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext)
	hasCreate := fileGeneratorEnabled && hasConsoleFilesCreateCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
	if !hasRead && !hasDelete && !hasCreate {
		return adapter.Message{}, false
	}

	payload := map[string]interface{}{
		"page":          "console.files",
		"visible_files": consoleFilesPromptVisibleFiles(parts),
	}
	preferredSkills := []string{}
	if hasRead {
		preferredSkills = append(preferredSkills, skills.SkillFileReader)
	}
	if hasDelete {
		preferredSkills = append(preferredSkills, skills.SkillFileManager)
	}
	if hasCreate {
		preferredSkills = append(preferredSkills, skills.SkillFileGenerator)
	}
	payload["preferred_skills"] = preferredSkills
	if len(preferredSkills) == 1 {
		payload["preferred_skill"] = preferredSkills[0]
	}
	tools := make([]map[string]string, 0, 7)
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
			"skill_id":      skills.SkillFileManager,
			"tool_name":     "delete_file",
		})
	}
	if hasCreate {
		for _, toolName := range []string{"generate_file", "generate_docx", "generate_pdf", "generate_pptx"} {
			tools = append(tools, map[string]string{
				"capability_id": "file.create",
				"skill_id":      skills.SkillFileGenerator,
				"tool_name":     toolName,
			})
		}
	}
	payload["tools"] = tools
	if targets := consoleFilesPromptResolvedTargets(parts); len(targets) > 0 {
		payload["resolved_targets_from_user_request"] = targets
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return adapter.Message{}, false
	}

	lines := []string{
		"Contextual files-page tool guidance:",
		"The user is operating on the Console Files page. Treat visible file resources in operation_context as concrete user assets.",
		"Answer in the user's language. Use internal file and workspace identifiers only as tool arguments; do not mention internal IDs, UUIDs, workspace identifiers, raw JSON field names, or tool count fields in user-visible answers.",
		"When reporting file operation outcomes to a normal user, mention only the file name and the user-visible action result. For successful deletion, say that the named file was deleted; do not report raw counters or internal identifiers.",
		"When a file tool fails, explain the failure plainly in the user's language, do not claim success, and ask for the next safe step only when needed.",
		"For requests that only ask what files are visible, available, selected, or present on the Files page, answer directly from visible_files in the Files-page context JSON when it is present and sufficient. Use file-reader/list_visible_files only when that context is missing, ambiguous, or needs an authoritative refresh.",
		"For typed ordinal requests such as \"the second Excel file\", \"\u7b2c\u4e8c\u4e2a Excel\", or \"\u6700\u540e\u4e00\u4e2a PDF\", resolve among files of that type using file_type_rank or extension_rank. Do not treat \"second Excel\" as visible_index 2 unless that file is also the second Excel in visible_files.",
		"For requests about reading, previewing, summarizing, analyzing, or translating visible file contents, use file-reader/read_file with the resolved file_id.",
		"When resolved_targets_from_user_request is present, the target is already resolved from the current page. Use the listed file_id exactly for file-reader/read_file or file-manager/delete_file; it overrides any other ordinal or file-type interpretation.",
		"Do not ask the user to select a file, repeat the file name, or choose another visible file with the same type when a resolved target is present.",
		"After read_file returns content_status \"extracted\", answer from the returned content field and continue requested post-processing such as summary or translation. Do not say the file cannot be read.",
		"For requests about deleting or removing a resolved visible file, use file-manager/delete_file with exactly that file_id. Tool governance handles the approval card before deletion; do not ask for a separate natural-language confirmation first.",
		"If a prior approval or session grant exists, it only skips the approval prompt. You must still call file-manager/delete_file in this turn and wait for the tool result before saying the file was deleted.",
		"Never claim a file was deleted, removed, updated, created, saved, or otherwise changed based only on previous conversation context.",
		"If the target file is missing or ambiguous, call request_user_input with a concise clarification instead of guessing.",
		"For requests to create, generate, write, save, upload, or export a new file into File Management or the current Files page, use file-generator and pass target \"managed_file\". Select the file-generator tool by output format; generate_file handles TXT, Markdown, HTML, JSON, CSV, and simple text-like files.",
		"For generated or downloadable files without an explicit File Management, current Files page, workspace folder, save, create, or upload target, keep the default temporary artifact behavior by omitting target or using target \"temporary_artifact\".",
		"Creating a managed file is a governed file.create operation. Tool governance handles the approval card when the permission tier requires it; do not ask for a separate natural-language confirmation first.",
		"Do not call unrelated discovery or domain tools, such as database, knowledge, or calculator, before completing the requested files-page operation.",
		"For existing-file read/delete operations, do not call file-generation tools before the requested read/delete is completed.",
		"Files-page context JSON: " + string(encoded),
	}
	if hint := consoleFilesGuardTargetArgumentHint(consoleFilesPromptResolvedTargets(prepared.parts), ""); hint != "" {
		lines = append(lines, hint)
	}
	content := strings.Join(lines, "\n")
	return adapter.Message{Role: "system", Content: content}, true
}

func skillLoopFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	guards := make([]skillloop.FinalAnswerGuard, 0, 3)
	if guard := skillLoopConsoleNavigationFinalAnswerGuard(prepared); guard != nil {
		guards = append(guards, guard)
	}
	if guard := skillLoopConsoleFilesFinalAnswerGuard(prepared); guard != nil {
		guards = append(guards, guard)
	}
	return combineFinalAnswerGuards(guards...)
}

func skillLoopConsoleFilesFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	parts := prepared.parts
	if !isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return nil
	}
	if hasConsoleFilesCreateCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		skillIDEnabled(parts.SkillIDs, skills.SkillFileGenerator) &&
		isManagedFileCreateIntent(parts.Query) {
		return consoleFilesManagedFileCreateFinalAnswerGuard()
	}
	targets := consoleFilesPromptResolvedTargets(parts)
	if len(targets) == 0 {
		return nil
	}
	if hasConsoleFilesCapability(parts.RuntimeContext, consoleFilesDeleteCapabilityPattern, parts.RawOperationContext, parts.OperationContext) &&
		isFileDeleteIntent(parts.Query) {
		if !skillIDEnabled(parts.SkillIDs, skills.SkillFileManager) {
			return nil
		}
		return consoleFilesRequiredToolFinalAnswerGuard(skills.SkillFileManager, targets, "delete_file", []string{
			"The user's current files-page request is a concrete file deletion request for {target}.",
			"Do not finish with a natural-language success message yet.",
			"Load the file-manager skill if needed, then call call_skill_tool with skill_id \"file-manager\", tool_name \"delete_file\", and the resolved file_id for the target file.",
			"A session approval grant may skip the approval card, but it does not replace the delete_file tool call.",
			"Only after delete_file succeeds in this turn may you tell the user that the file was deleted. If the tool fails or the file is already missing, report the actual tool result.",
		})
	}
	if hasConsoleFilesReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		isFileReadIntent(parts.Query) {
		if !skillIDEnabled(parts.SkillIDs, skills.SkillFileReader) {
			return nil
		}
		return consoleFilesRequiredToolFinalAnswerGuard(skills.SkillFileReader, targets, "read_file", []string{
			"The user's current files-page request requires reading the actual content of {target}.",
			"Do not finish from visible page metadata, file names, or prior conversation context.",
			"Load the file-reader skill if needed, then call call_skill_tool with skill_id \"file-reader\", tool_name \"read_file\", and the resolved file_id for the target file.",
			"Only after read_file succeeds in this turn may you summarize, translate, quote, or answer from the file content. If the tool fails or returns empty content, report the actual tool result.",
		})
	}
	return nil
}

func consoleFilesManagedFileCreateFinalAnswerGuard() skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasManagedFileGeneratorTool(req) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		message := strings.Join([]string{
			"The user's current files-page request explicitly asks to create or save a new file into File Management or the current Files page.",
			"Do not finish by saying this is unsupported.",
			"Load the file-generator skill if needed, select the appropriate generator tool for the requested file format, and call it with target \"managed_file\".",
			"Keep generated files temporary only when the user did not explicitly ask for File Management, current Files page, workspace folder, save, create, or upload as the target.",
			"Only after the governed file-generator tool succeeds may you say the managed file was created. If approval is required, wait for tool governance instead of asking for a separate natural-language confirmation.",
		}, " ")
		return skillloop.FinalAnswerGuardResult{
			SkillID:       skills.SkillFileGenerator,
			ToolName:      "generate_file",
			Message:       message,
			SystemMessage: message,
		}, true
	}
}

func skillLoopConsoleNavigationFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator) {
		return nil
	}
	target, ok := resolveConsoleNavigationTarget(prepared.parts.Query)
	if !ok {
		return nil
	}
	if clientActionContinuationLoadedRoute(prepared.parts, target.Href) {
		return nil
	}
	return consoleNavigationRequiredToolFinalAnswerGuard(target)
}

func clientActionContinuationLoadedRoute(parts *chatRequestParts, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if parts == nil || href == "" {
		return false
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if len(source) == 0 {
			continue
		}
		continuation := governanceMapFromAny(source["client_action_continuation"])
		if len(continuation) == 0 {
			continue
		}
		if strings.TrimSpace(stringFromAny(continuation["action_type"])) != "route_navigation" {
			continue
		}
		if strings.TrimSpace(stringFromAny(continuation["status"])) != clientActionStatusSucceeded {
			continue
		}
		if normalizeConsoleNavigationGuardHref(stringFromAny(continuation["href"])) == href {
			return true
		}
	}
	return false
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
		if strings.EqualFold(strings.TrimSpace(result.SkillID), skills.SkillConsoleNavigator) {
			result.Message = strings.Join([]string{
				"The request_user_input call was blocked because the user asked for a known ZGI console route.",
				"Do not ask which page to open when the destination is already resolved from the site map.",
				result.Message,
			}, " ")
		} else {
			result.Message = strings.Join([]string{
				"The request_user_input call was blocked because the files-page target is already resolved in runtime context.",
				"Do not ask the user to choose between visible files, repeat a known file name, or confirm information already represented by resolved_targets_from_user_request.",
				result.Message,
			}, " ")
		}
		return result, true
	}
}

func consoleNavigationRequiredToolFinalAnswerGuard(target consoleNavigationRouteHint) skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulToolForConsoleHref(req, skills.SkillConsoleNavigator, "navigate", target.Href) ||
			finalAnswerGuardHasAttemptedToolForConsoleHref(req, skills.SkillConsoleNavigator, "navigate", target.Href) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		message := strings.Join([]string{
			fmt.Sprintf("The user's current request is to open the ZGI console page %s (%s).", target.Label, target.Href),
			"Do not finish with a natural-language message saying the page has opened yet.",
			fmt.Sprintf("Load the console-navigator skill if needed, then call call_skill_tool with skill_id %q, tool_name %q, and href %q.", skills.SkillConsoleNavigator, "navigate", target.Href),
			"Only after navigate succeeds in this turn may you tell the user that the page was opened. If the tool fails, report the actual tool result.",
		}, " ")
		payload := map[string]interface{}{
			"skill_id":  skills.SkillConsoleNavigator,
			"tool_name": "navigate",
			"arguments": map[string]interface{}{
				"href": target.Href,
			},
		}
		if target.Label != "" {
			payload["label"] = target.Label
		}
		encoded, err := json.Marshal(payload)
		systemMessage := message
		if err == nil {
			systemMessage = systemMessage + " Resolved route JSON for tool arguments: " + string(encoded)
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:       skills.SkillConsoleNavigator,
			ToolName:      "navigate",
			Message:       message,
			SystemMessage: systemMessage,
		}, true
	}
}

func consoleFilesRequiredToolFinalAnswerGuard(skillID string, targets []map[string]interface{}, toolName string, messageTemplates []string) skillloop.FinalAnswerGuard {
	targetSummary := consoleFilesGuardTargetSummary(targets)
	targetFileIDs := consoleFilesGuardTargetFileIDs(targets)
	targetArgumentHint := consoleFilesGuardTargetArgumentHint(targets, toolName)
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulToolForTargets(req, skillID, toolName, targetFileIDs) ||
			finalAnswerGuardHasAttemptedToolForTargets(req, skillID, toolName, targetFileIDs) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		lines := make([]string, 0, len(messageTemplates))
		for _, template := range messageTemplates {
			lines = append(lines, strings.ReplaceAll(template, "{target}", targetSummary))
		}
		systemLines := append([]string{}, lines...)
		if targetArgumentHint != "" {
			systemLines = append(systemLines, targetArgumentHint)
		}
		return skillloop.FinalAnswerGuardResult{
			SkillID:       skillID,
			ToolName:      toolName,
			Message:       strings.Join(lines, " "),
			SystemMessage: strings.Join(systemLines, " "),
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

func finalAnswerGuardHasManagedFileGeneratorTool(req skillloop.FinalAnswerGuardRequest) bool {
	return finalAnswerGuardHasManagedFileGeneratorCall(req.SuccessfulToolCalls) ||
		finalAnswerGuardHasManagedFileGeneratorCall(req.AttemptedToolCalls)
}

func finalAnswerGuardHasManagedFileGeneratorCall(calls []skillloop.SkillToolCallRef) bool {
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillFileGenerator) {
			continue
		}
		if !isFileGeneratorToolName(call.ToolName) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(skillToolCallArgumentString(call.Arguments, "target")), "managed_file") {
			return true
		}
	}
	return false
}

func isFileGeneratorToolName(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
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

func finalAnswerGuardHasSuccessfulToolForConsoleHref(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, href string) bool {
	return finalAnswerGuardHasToolForConsoleHref(req.SuccessfulToolCalls, skillID, toolName, href)
}

func finalAnswerGuardHasAttemptedToolForConsoleHref(req skillloop.FinalAnswerGuardRequest, skillID string, toolName string, href string) bool {
	return finalAnswerGuardHasToolForConsoleHref(req.AttemptedToolCalls, skillID, toolName, href)
}

func finalAnswerGuardHasToolForConsoleHref(calls []skillloop.SkillToolCallRef, skillID string, toolName string, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			continue
		}
		if normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(call.Arguments, "href")) == href {
			return true
		}
	}
	return false
}

func skillToolCallArgumentString(arguments map[string]interface{}, key string) string {
	if len(arguments) == 0 {
		return ""
	}
	return strings.TrimSpace(stringFromAny(arguments[key]))
}

func normalizeConsoleNavigationGuardHref(rawHref string) string {
	rawHref = strings.TrimSpace(rawHref)
	if rawHref == "" {
		return ""
	}
	if parsed, err := strings.CutPrefix(rawHref, "http://localhost:2780"); err {
		rawHref = parsed
	}
	if parsed, err := strings.CutPrefix(rawHref, "https://localhost:2780"); err {
		rawHref = parsed
	}
	if !strings.HasPrefix(rawHref, "/") {
		rawHref = "/" + rawHref
	}
	if idx := strings.IndexAny(rawHref, "?#"); idx >= 0 {
		rawHref = rawHref[:idx]
	}
	rawHref = strings.TrimRight(rawHref, "/")
	if rawHref == "" {
		return "/"
	}
	return rawHref
}

func resolveConsoleNavigationTarget(query string) (consoleNavigationRouteHint, bool) {
	if !isConsoleNavigationIntent(query) {
		return consoleNavigationRouteHint{}, false
	}
	normalized := normalizeConsoleNavigationQuery(query)
	for _, route := range consoleNavigationRouteHints {
		if strings.Contains(normalized, strings.ToLower(route.Href)) {
			return route, true
		}
		for _, keyword := range route.Keywords {
			keyword = normalizeConsoleNavigationQuery(keyword)
			if keyword != "" && strings.Contains(normalized, keyword) {
				return route, true
			}
		}
	}
	return consoleNavigationRouteHint{}, false
}

func isConsoleNavigationIntent(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"带我去", "带我到", "打开", "跳转", "切换到", "进入", "前往", "导航到", "转到",
		"go to", "open", "switch to", "navigate to", "take me to", "show me",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func normalizeConsoleNavigationQuery(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("，", " ", "。", " ", "？", " ", "?", " ", "！", " ", "!", " ", ",", " ", ".", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
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

func consoleFilesGuardTargetArgumentHint(targets []map[string]interface{}, toolName string) string {
	type targetRef struct {
		Name   string `json:"name,omitempty"`
		FileID string `json:"file_id"`
	}
	refs := []targetRef{}
	seen := map[string]struct{}{}
	for _, target := range targets {
		id := strings.TrimSpace(stringFromAny(target["file_id"]))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		refs = append(refs, targetRef{
			Name:   strings.TrimSpace(stringFromAny(target["name"])),
			FileID: id,
		})
	}
	if len(refs) == 0 {
		return ""
	}
	payload := map[string]interface{}{
		"skill_id":                             skills.SkillFileReader,
		"resolved_targets_for_tool_arguments":  refs,
		"tool_argument_visibility_restriction": "internal_only_do_not_reveal_to_user",
	}
	if toolName = strings.TrimSpace(toolName); toolName != "" {
		payload["tool_name"] = toolName
	}
	if len(refs) == 1 {
		payload["arguments"] = map[string]interface{}{"file_id": refs[0].FileID}
	} else {
		payload["arguments"] = map[string]interface{}{"file_ids": consoleFilesGuardTargetFileIDs(targets)}
		payload["call_instruction"] = "Call the required tool once per resolved target if the tool schema accepts only a single file_id."
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return "Resolved internal target JSON for tool arguments only; do not reveal internal IDs to the user: " + string(encoded)
}

func consoleFilesGuardTargetSummary(targets []map[string]interface{}) string {
	if len(targets) == 0 {
		return "the resolved visible file"
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		name := strings.TrimSpace(stringFromAny(target["name"]))
		if name != "" {
			parts = append(parts, name)
		}
	}
	if len(parts) == 0 {
		return "the resolved visible file"
	}
	return strings.Join(parts, ", ")
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
		item := map[string]interface{}{
			"visible_index": file.VisibleIndex,
			"file_id":       file.ID,
			"name":          file.Title,
			"extension":     file.Extension,
			"mime_type":     file.MimeType,
			"selected":      file.Selected,
		}
		if file.FileTypeRank > 0 {
			item["file_type_rank"] = file.FileTypeRank
		}
		if file.ExtensionRank > 0 {
			item["extension_rank"] = file.ExtensionRank
		}
		if strings.TrimSpace(file.FileType) != "" {
			item["file_type"] = strings.TrimSpace(file.FileType)
		}
		if strings.TrimSpace(file.WorkspaceID) != "" {
			item["workspace_id"] = strings.TrimSpace(file.WorkspaceID)
		}
		out = append(out, item)
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
	for _, resource := range result.Resources {
		if strings.TrimSpace(resource.ID) != "" && strings.TrimSpace(resource.Name) != "" {
			namesByID[strings.TrimSpace(resource.ID)] = strings.TrimSpace(resource.Name)
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
	case "datetime", "date-time":
		return "string"
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
			Variable:            variable,
			Label:               strings.TrimSpace(item.Label),
			Type:                strings.TrimSpace(item.Type),
			Required:            item.Required,
			Default:             item.Default,
			DefaultDateTimeMode: strings.TrimSpace(item.DefaultDateTimeMode),
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

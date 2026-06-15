package service

import (
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type finalAnswerGuard func(finalAnswerGuardRequest) (finalAnswerGuardResult, bool)

type finalAnswerGuardRequest struct {
	Answer              string
	Round               int
	SkillUsed           bool
	ToolCallCount       int
	AttemptedToolCalls  []skillToolCallRef
	SuccessfulToolCalls []skillToolCallRef
}

type finalAnswerGuardResult struct {
	SkillID  string
	ToolName string
	Message  string
}

type skillToolCallRef struct {
	SkillID   string
	ToolName  string
	Arguments map[string]interface{}
}

type consoleFilesVisibleFileResource struct {
	ID           string
	Title        string
	Extension    string
	MimeType     string
	WorkspaceID  string
	Selected     bool
	VisibleIndex int
}

func skillLoopFinalAnswerGuard(prepared *PreparedChat) finalAnswerGuard {
	if prepared == nil || prepared.parts == nil || !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillFileReader) {
		return nil
	}
	if !isConsoleFilesContext(prepared.parts) {
		return nil
	}
	if isConsoleFilesListIntent(prepared.parts.Query) {
		return consoleFilesListRequiredToolFinalAnswerGuard()
	}
	return nil
}

func consoleFilesListRequiredToolFinalAnswerGuard() finalAnswerGuard {
	return func(req finalAnswerGuardRequest) (finalAnswerGuardResult, bool) {
		if finalAnswerGuardHasTool(req.SuccessfulToolCalls, skills.SkillFileReader, "list_visible_files") ||
			finalAnswerGuardHasTool(req.AttemptedToolCalls, skills.SkillFileReader, "list_visible_files") {
			return finalAnswerGuardResult{}, false
		}
		return finalAnswerGuardResult{
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

func runFinalAnswerGuard(guard finalAnswerGuard, req finalAnswerGuardRequest) (finalAnswerGuardResult, bool) {
	if guard == nil {
		return finalAnswerGuardResult{}, false
	}
	result, blocked := guard(req)
	if !blocked {
		return finalAnswerGuardResult{}, false
	}
	result.Message = strings.TrimSpace(result.Message)
	if result.Message == "" {
		result.Message = "The previous candidate final answer was blocked because a required skill/tool call has not succeeded in this turn. Continue planning and call the required skill/tool before claiming completion."
	}
	return result, true
}

func finalAnswerGuardrailTrace(result finalAnswerGuardResult) skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "guardrail",
		SkillID:  strings.TrimSpace(result.SkillID),
		ToolName: strings.TrimSpace(result.ToolName),
		Status:   "blocked",
		Error:    strings.TrimSpace(result.Message),
		Arguments: map[string]interface{}{
			"next_step": "continue_planning",
		},
	}
}

func finalAnswerGuardSystemMessage(result finalAnswerGuardResult, candidateAnswer string) adapter.Message {
	lines := []string{
		"Runtime guardrail feedback:",
		strings.TrimSpace(result.Message),
	}
	if text := strings.TrimSpace(candidateAnswer); text != "" {
		lines = append(lines, "Blocked candidate answer:\n"+text)
	}
	return adapter.Message{Role: "system", Content: strings.Join(lines, "\n")}
}

func finalAnswerGuardHasTool(calls []skillToolCallRef, skillID string, toolName string) bool {
	for _, call := range calls {
		if strings.EqualFold(strings.TrimSpace(call.SkillID), skillID) &&
			strings.EqualFold(strings.TrimSpace(call.ToolName), toolName) {
			return true
		}
	}
	return false
}

func consoleFilesRuntimeVisibleFiles(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.parts == nil || !isConsoleFilesContext(prepared.parts) {
		return nil
	}
	files := visibleFileResourcesFromContext(prepared.parts.RawOperationContext)
	if len(files) == 0 {
		files = visibleFileResourcesFromContext(prepared.parts.OperationContext)
	}
	out := make([]map[string]interface{}, 0, len(files))
	for idx, file := range files {
		item := map[string]interface{}{}
		visibleIndex := file.VisibleIndex
		if visibleIndex <= 0 {
			visibleIndex = idx + 1
		}
		item["visible_index"] = visibleIndex
		if file.ID != "" {
			item["file_id"] = file.ID
		}
		if file.Title != "" {
			item["name"] = file.Title
		}
		if file.Extension != "" {
			item["extension"] = file.Extension
		}
		if file.MimeType != "" {
			item["mime_type"] = file.MimeType
		}
		if file.WorkspaceID != "" {
			item["workspace_id"] = file.WorkspaceID
		}
		if file.Selected {
			item["selected"] = true
		}
		out = append(out, item)
	}
	return out
}

func visibleFileResourcesFromContext(context map[string]interface{}) []consoleFilesVisibleFileResource {
	if len(context) == 0 {
		return nil
	}
	items := operationItemsFromValue(context["resources"])
	out := make([]consoleFilesVisibleFileResource, 0, len(items))
	for _, item := range items {
		resource, ok := item.(map[string]interface{})
		if !ok || !isFileResourceMap(resource) {
			continue
		}
		metadata := mapFromContextValue(resource["metadata"])
		id := firstNonEmptyString(
			firstContextMapValue(resource, "resource_id", "id", "file_id", "upload_file_id"),
			firstContextMapValue(metadata, "resource_id", "id", "file_id", "upload_file_id"),
		)
		if id == "" {
			continue
		}
		title := firstNonEmptyString(
			firstContextMapValue(resource, "title", "name", "filename", "file_name", "label"),
			firstContextMapValue(metadata, "title", "name", "filename", "file_name", "label"),
		)
		out = append(out, consoleFilesVisibleFileResource{
			ID:    id,
			Title: title,
			Extension: firstNonEmptyString(
				firstContextMapValue(resource, "extension", "ext", "suffix"),
				firstContextMapValue(metadata, "extension", "file_extension", "ext", "suffix"),
				fileNameExtension(title),
			),
			MimeType: firstNonEmptyString(
				firstContextMapValue(resource, "mime_type", "mimeType", "mime"),
				firstContextMapValue(metadata, "mime_type", "mimeType", "mime"),
			),
			WorkspaceID: firstNonEmptyString(
				firstContextMapValue(resource, "workspace_id", "workspaceId"),
				firstContextMapValue(metadata, "workspace_id", "workspaceId"),
			),
			Selected: boolContextValue(firstContextMapValue(resource, "selected", "is_selected")) ||
				boolContextValue(firstContextMapValue(metadata, "selected", "is_selected")),
			VisibleIndex: firstPositiveInt(
				firstContextMapValue(resource, "visible_index", "visible_ordinal", "ordinal"),
				firstContextMapValue(metadata, "visible_index", "visible_ordinal", "ordinal"),
			),
		})
	}
	return out
}

func isConsoleFilesContext(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	if textContainsConsoleFilesMarker(parts.RuntimeContext) ||
		contextContainsConsoleFilesMarker(parts.RawOperationContext) ||
		contextContainsConsoleFilesMarker(parts.OperationContext) {
		return true
	}
	return false
}

func textContainsConsoleFilesMarker(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(text, "console.files") || strings.Contains(text, "/console/files")
}

func contextContainsConsoleFilesMarker(value interface{}) bool {
	return contextContainsString(value, 0, func(text string) bool {
		return textContainsConsoleFilesMarker(text)
	})
}

func contextContainsString(value interface{}, depth int, match func(string) bool) bool {
	if depth > 5 || value == nil || match == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return match(typed)
	case fmt.Stringer:
		return match(typed.String())
	case map[string]interface{}:
		for key, item := range typed {
			if match(key) || contextContainsString(item, depth+1, match) {
				return true
			}
		}
	case []interface{}:
		for _, item := range typed {
			if contextContainsString(item, depth+1, match) {
				return true
			}
		}
	case []map[string]interface{}:
		for _, item := range typed {
			if contextContainsString(item, depth+1, match) {
				return true
			}
		}
	}
	return false
}

func isConsoleFilesListIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	for _, phrase := range []string{
		"what files",
		"which files",
		"visible files",
		"available files",
		"current files",
		"list files",
		"list the files",
		"files do i have",
		"files are visible",
		"files are available",
		"show me the files",
	} {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	for _, phrase := range []string{
		"\u6709\u54ea\u4e9b\u6587\u4ef6",
		"\u54ea\u4e9b\u6587\u4ef6",
		"\u6709\u4ec0\u4e48\u6587\u4ef6",
		"\u6587\u4ef6\u5217\u8868",
		"\u5217\u51fa\u6587\u4ef6",
		"\u53ef\u89c1\u6587\u4ef6",
		"\u5f53\u524d\u6587\u4ef6\u5217\u8868",
		"\u5f53\u524d\u6709\u54ea\u4e9b",
		"\u6211\u6709\u54ea\u4e9b\u6587\u4ef6",
		"\u6709\u51e0\u4e2a\u6587\u4ef6",
		"\u591a\u5c11\u4e2a\u6587\u4ef6",
		"\u9875\u9762\u6587\u4ef6",
	} {
		if strings.Contains(text, phrase) {
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

func operationItemsFromValue(value interface{}) []interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case []interface{}:
		return typed
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			items = append(items, item)
		}
		return items
	case map[string]interface{}:
		return []interface{}{typed}
	default:
		return nil
	}
}

func isFileResourceMap(value map[string]interface{}) bool {
	for _, key := range []string{"type", "resource_type", "kind", "resource_kind"} {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(value[key])), "file") {
			return true
		}
	}
	metadata := mapFromContextValue(value["metadata"])
	return strings.EqualFold(strings.TrimSpace(stringFromAny(metadata["resource_kind"])), "file")
}

func mapFromContextValue(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		return nil
	}
}

func firstContextMapValue(value map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value == nil {
			return nil
		}
		if item, ok := value[key]; ok {
			return item
		}
	}
	return nil
}

func boolContextValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func firstPositiveInt(values ...interface{}) int {
	for _, value := range values {
		if intValue, ok := intValue(value); ok && intValue > 0 {
			return intValue
		}
	}
	return 0
}

func fileNameExtension(name string) string {
	name = strings.TrimSpace(name)
	index := strings.LastIndex(name, ".")
	if index < 0 || index == len(name)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(name[index+1:]))
}

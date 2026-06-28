package skillloop

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// FastPathFinalAnswerForToolTrace returns a user-visible final answer when a
// completed tool result is already sufficient evidence for this turn.
func FastPathFinalAnswerForToolTrace(trace skills.SkillTrace) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(trace.Status), "success") {
		return "", false
	}
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	switch {
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		switch toolName {
		case "delete_agent":
			return agentDeleteFastPathAnswer(trace.Result)
		case "delete_agents":
			return agentBatchDeleteFastPathAnswer(trace.Result)
		case "update_agent_config",
			"replace_agent_skill_bindings",
			"replace_agent_knowledge_bindings",
			"replace_agent_database_bindings",
			"replace_agent_workflow_bindings",
			"replace_agent_memory_slots":
			return agentConfigUpdateFastPathAnswer(trace.Result)
		}
	case strings.EqualFold(skillID, skills.SkillFileManager):
		switch toolName {
		case "save_file_to_management":
			return fileManagementSaveFastPathAnswer(trace.Result)
		case "delete_file":
			return fileManagementDeleteFastPathAnswer(trace.Result)
		}
	default:
		return "", false
	}
	return "", false
}

// FastPathFinalAnswerForToolTraceWithEvidence keeps the fast path from ending
// a longer user turn when the operation plan still names a different pending
// action.
func FastPathFinalAnswerForToolTraceWithEvidence(trace skills.SkillTrace, evidence map[string]interface{}) (string, bool) {
	answer, ok := FastPathFinalAnswerForToolTrace(trace)
	if !ok {
		return "", false
	}
	if fastPathBlockedByPendingPlanAction(trace, evidence) {
		return "", false
	}
	return answer, true
}

func completionEvidenceForFastPath(req RunRequest) map[string]interface{} {
	if req.CompletionEvidence == nil {
		return nil
	}
	return req.CompletionEvidence()
}

func fastPathBlockedByPendingPlanAction(trace skills.SkillTrace, evidence map[string]interface{}) bool {
	plan := evidenceMapFromAny(evidence["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	pendingActions, hasPlanSteps := fastPathPendingExecutablePlanActions(plan, 8)
	if !hasPlanSteps {
		pending := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(plan["pending_next_action"])))
		if pending != "" {
			pendingActions = append(pendingActions, pending)
		}
	}
	for _, pending := range pendingActions {
		if fastPathPendingActionBlocksTrace(trace, pending) {
			return true
		}
	}
	return false
}

func fastPathPendingActionBlocksTrace(trace skills.SkillTrace, pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" || pending == "none" || pending == "done" || pending == "completed" {
		return false
	}
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	skillID := strings.ToLower(strings.TrimSpace(trace.SkillID))
	if toolName == "" {
		return true
	}
	if pending == toolName || strings.Contains(pending, toolName) {
		return false
	}
	if skillID != "" && strings.Contains(pending, skillID+"/"+toolName) {
		return false
	}
	if fastPathAuthoritativeMutation(trace) && fastPathPendingActionIsPostVerification(pending) {
		return false
	}
	return true
}

func fastPathPendingExecutablePlanActions(plan map[string]interface{}, limit int) ([]string, bool) {
	steps := mapSliceFromAny(plan["steps"])
	if len(steps) == 0 || limit <= 0 {
		return nil, len(steps) > 0
	}
	actions := make([]string, 0, limit)
	for _, step := range steps {
		if !fastPathPlanStepBlocksCompletion(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := fastPathNormalizePlanStepStatus(firstNonEmptyString(step["status"], fastPathPlanStepStatusValue(plan["step_status"], id)))
		if status == "completed" || status == "failed" {
			continue
		}
		action := fastPathPlanStepAction(step)
		if action == "" {
			continue
		}
		actions = append(actions, action)
		if len(actions) >= limit || fastPathPlanStepIsRoute(step) {
			break
		}
	}
	return actions, true
}

func fastPathPlanStepStatusValue(statuses interface{}, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	switch typed := statuses.(type) {
	case map[string]interface{}:
		return stringFromAny(typed[id])
	case map[string]string:
		return typed[id]
	default:
		return ""
	}
}

func fastPathPlanStepBlocksCompletion(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	if strings.HasPrefix(id, "skill:") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["role"])), "supporting") &&
		strings.TrimSpace(stringFromAny(step["tool_name"])) == "" {
		return false
	}
	return true
}

func fastPathPlanStepAction(step map[string]interface{}) string {
	if len(step) == 0 {
		return ""
	}
	skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName != "" {
		if skillID != "" {
			return strings.ToLower(skillID + "/" + toolName)
		}
		return strings.ToLower(toolName)
	}
	return strings.ToLower(strings.TrimSpace(firstNonEmptyString(step["id"], step["title"])))
}

func fastPathPlanStepIsRoute(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.ToLower(strings.TrimSpace(stringFromAny(step["id"])))
	skillID := strings.ToLower(strings.TrimSpace(stringFromAny(step["skill_id"])))
	toolName := strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"])))
	return strings.Contains(id, "route") ||
		strings.Contains(id, "navigate") ||
		skillID == skills.SkillConsoleNavigator ||
		toolName == "navigate" ||
		toolName == "route"
}

func fastPathNormalizePlanStepStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "allowed", "approved", "completed":
		return "completed"
	case "failure", "error", "rejected", "denied", "failed":
		return "failed"
	case "running", "streaming", "in_progress", "waiting_approval", "pending":
		return "pending"
	default:
		return "pending"
	}
}

func fastPathAuthoritativeMutation(trace skills.SkillTrace) bool {
	skillID := strings.TrimSpace(trace.SkillID)
	toolName := strings.ToLower(strings.TrimSpace(trace.ToolName))
	switch {
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		switch toolName {
		case "delete_agent", "delete_agents", "update_agent_config",
			"replace_agent_skill_bindings", "replace_agent_knowledge_bindings",
			"replace_agent_database_bindings", "replace_agent_workflow_bindings",
			"replace_agent_memory_slots":
			return true
		}
	case strings.EqualFold(skillID, skills.SkillFileManager):
		return toolName == "save_file_to_management" || toolName == "delete_file"
	}
	return false
}

func fastPathPendingActionIsPostVerification(pending string) bool {
	pending = strings.ToLower(strings.TrimSpace(pending))
	if pending == "" {
		return false
	}
	for _, marker := range []string{
		"observe",
		"observation",
		"asset_observation",
		"page_refresh",
		"refresh",
		"route_loaded",
		"navigate",
		"console-navigator",
		"list_",
		"get_",
		"search",
		"read",
		"inspect",
	} {
		if strings.Contains(pending, marker) {
			return true
		}
	}
	return false
}

func agentDeleteFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	effect := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["effect"], result["operation"])))
	if effect != "" && effect != "deleted" && effect != "delete" && effect != "agent.delete" {
		return "", false
	}
	name := agentDeleteItemName(result)
	if name == "" {
		name = "指定智能体"
	}
	return "已删除智能体：" + name + "。", true
}

func agentBatchDeleteFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "partial_failed" && status != "failed" {
		return "", false
	}
	group := payloadMap(result, "operation_group")
	if group == nil {
		group = map[string]interface{}{}
	}
	operation := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["operation_type"], group["operation"])))
	if operation != "" && operation != "agent.delete.batch" && operation != "agent.delete" {
		return "", false
	}

	items := mapSliceFromAny(firstNonEmptyValue(group["item_results"], result["item_results"]))
	targetCount := firstPositiveInt(result["target_count"], group["target_count"], len(items))
	if targetCount <= 0 {
		return "", false
	}
	deletedCount := firstNonNegativeInt(result["deleted_count"], group["success_count"], countAgentDeleteItems(items, "succeeded"))
	failedCount := firstNonNegativeInt(result["failed_count"], group["failed_count"], countAgentDeleteItems(items, "failed"))
	if deletedCount+failedCount == 0 && status == "completed" {
		deletedCount = targetCount
	}

	successNames := agentDeleteItemNames(items, "succeeded")
	failedItems := agentDeleteFailedItems(items)

	switch {
	case failedCount == 0:
		return fmt.Sprintf("已完成批量删除：成功删除 %d 个智能体%s。", deletedCount, formatNameList(successNames)), true
	case deletedCount == 0:
		detail := formatFailedItems(failedItems)
		if detail != "" {
			detail = "：" + detail
		}
		return fmt.Sprintf("批量删除未完成：%d 个智能体删除失败%s。", failedCount, detail), true
	default:
		successDetail := formatNameList(successNames)
		failedDetail := formatFailedItems(failedItems)
		if failedDetail != "" {
			failedDetail = "：" + failedDetail
		}
		return fmt.Sprintf("批量删除已结束：成功删除 %d 个智能体%s，%d 个删除失败%s。", deletedCount, successDetail, failedCount, failedDetail), true
	}
}

func agentConfigUpdateFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" {
		return "", false
	}

	agentName := agentConfigResultAgentName(result)
	details := agentConfigUpdateDetails(result)
	if len(details) == 0 {
		return "", false
	}

	target := "该智能体"
	if agentName != "" {
		target = "智能体「" + agentName + "」"
	}
	return fmt.Sprintf("已更新%s配置：%s。", target, strings.Join(details, "；")), true
}

func agentConfigResultAgentName(result map[string]interface{}) string {
	if len(result) == 0 {
		return ""
	}
	if name := strings.TrimSpace(firstNonEmptyString(result["agent_name"], result["name"])); name != "" {
		return name
	}
	agent := payloadMap(result, "agent")
	return strings.TrimSpace(firstNonEmptyString(agent["name"], agent["agent_name"]))
}

func agentConfigUpdateDetails(result map[string]interface{}) []string {
	seenFields := map[string]struct{}{}
	details := make([]string, 0)
	for _, change := range agentConfigChangeMaps(result) {
		if field := strings.TrimSpace(stringFromAny(change["field"])); field != "" {
			seenFields[field] = struct{}{}
		}
		if detail := formatAgentConfigBindingChange(change); detail != "" {
			details = append(details, detail)
		}
	}
	for _, field := range sanitizedStringListArgumentValue(result["updated_fields"]) {
		if agentConfigFieldCoveredByBindingChange(field, seenFields) {
			continue
		}
		if label := agentConfigFieldLabel(field); label != "" {
			details = append(details, "更新"+label)
		}
	}
	return dedupeStrings(details)
}

func fileManagementSaveFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	target := strings.ToLower(strings.TrimSpace(stringFromAny(result["target"])))
	if target != "" && target != "managed_file" && target != "file_management" {
		return "", false
	}
	fileID := strings.TrimSpace(firstNonEmptyString(result["file_id"], result["upload_file_id"], result["id"]))
	if fileID == "" {
		return "", false
	}
	filename := strings.TrimSpace(firstNonEmptyString(result["filename"], result["name"], result["file_name"]))
	if filename == "" {
		return "", false
	}
	return fmt.Sprintf("文件「%s」已保存到文件管理。", filename), true
}

func fileManagementDeleteFastPathAnswer(result map[string]interface{}) (string, bool) {
	if len(result) == 0 {
		return "", false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(result["status"])))
	if status != "" && status != "completed" && status != "success" && status != "succeeded" {
		return "", false
	}
	if deleted := firstNonNegativeInt(result["deleted_count"]); deleted <= 0 && !boolFromAny(result["deleted"]) {
		return "", false
	}
	filename := strings.TrimSpace(firstNonEmptyString(
		result["filename"],
		result["name"],
		result["file_name"],
		result["file_title"],
	))
	if filename == "" {
		filename = "指定文件"
	}
	return fmt.Sprintf("已删除文件「%s」。", filename), true
}

func agentConfigChangeMaps(result map[string]interface{}) []map[string]interface{} {
	changes := mapSliceFromAny(result["binding_changes"])
	if len(changes) == 0 {
		changes = mapSliceFromAny(result["config_changes"])
	}
	return changes
}

func formatAgentConfigBindingChange(change map[string]interface{}) string {
	if len(change) == 0 {
		return ""
	}
	kind := agentConfigBindingKindLabel(firstNonEmptyString(change["binding_kind"], change["field"]))
	action := strings.ToLower(strings.TrimSpace(stringFromAny(change["change_action"])))
	if kind == "" || action == "" {
		return ""
	}
	switch action {
	case "bind":
		return formatAgentConfigCountChange("绑定", kind, firstPositiveInt(change["resource_count"], change["added_resource_count"]), sanitizedStringListArgumentValue(firstNonEmptyValue(change["resource_names"], change["added_resource_names"])))
	case "unbind":
		return formatAgentConfigCountChange("解绑", kind, firstPositiveInt(change["resource_count"], change["removed_resource_count"]), sanitizedStringListArgumentValue(firstNonEmptyValue(change["resource_names"], change["removed_resource_names"])))
	case "replace":
		added := firstNonNegativeInt(change["added_resource_count"])
		removed := firstNonNegativeInt(change["removed_resource_count"])
		if added == 0 && removed == 0 {
			return formatAgentConfigCountChange("替换", kind, firstPositiveInt(change["resource_count"]), sanitizedStringListArgumentValue(change["resource_names"]))
		}
		parts := make([]string, 0, 2)
		if added > 0 {
			parts = append(parts, fmt.Sprintf("新增 %d 个", added))
		}
		if removed > 0 {
			parts = append(parts, fmt.Sprintf("移除 %d 个", removed))
		}
		return "替换" + kind + "（" + strings.Join(parts, "；") + "）"
	case "update":
		if count := firstNonNegativeInt(change["final_resource_count"]); count > 0 {
			return fmt.Sprintf("更新%s（当前 %d 个）", kind, count)
		}
		return "更新" + kind
	default:
		return ""
	}
}

func formatAgentConfigCountChange(action string, kind string, count int, names []string) string {
	if action == "" || kind == "" {
		return ""
	}
	if len(names) > 0 {
		return action + kind + formatNameList(names)
	}
	if count > 0 {
		return fmt.Sprintf("%s %d 个%s", action, count, kind)
	}
	return ""
}

func agentConfigBindingKindLabel(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "agent_skill", "enabled_skill_ids":
		return "技能"
	case "knowledge_base", "knowledge_dataset_ids", "dataset_ids":
		return "知识库"
	case "database_table", "database_bindings":
		return "数据表"
	case "workflow", "workflow_bindings":
		return "工作流"
	case "multiple":
		return "资源绑定"
	default:
		return ""
	}
}

func agentConfigFieldCoveredByBindingChange(field string, seen map[string]struct{}) bool {
	field = strings.TrimSpace(field)
	if field == "" {
		return true
	}
	if _, ok := seen[field]; ok {
		return true
	}
	switch field {
	case "enabled_skill_ids":
		_, ok := seen["agent_skill"]
		return ok
	case "knowledge_dataset_ids", "dataset_ids":
		_, ok := seen["knowledge_base"]
		return ok
	case "database_bindings":
		_, ok := seen["database_table"]
		return ok
	case "workflow_bindings":
		_, ok := seen["workflow"]
		return ok
	default:
		return false
	}
}

func agentConfigFieldLabel(field string) string {
	switch strings.TrimSpace(field) {
	case "system_prompt":
		return "系统提示词"
	case "model", "model_provider":
		return "模型"
	case "model_parameters":
		return "模型参数"
	case "agent_memory_enabled":
		return "记忆开关"
	case "file_upload_enabled":
		return "文件上传开关"
	case "home_title":
		return "首页标题"
	case "input_placeholder":
		return "输入框占位文案"
	case "theme_color":
		return "主题色"
	case "suggested_questions":
		return "建议问题"
	case "knowledge_retrieval_config":
		return "知识库检索配置"
	case "enabled_skill_ids":
		return "技能"
	case "knowledge_dataset_ids", "dataset_ids":
		return "知识库"
	case "database_bindings":
		return "数据表绑定"
	case "workflow_bindings":
		return "工作流绑定"
	case "agent_memory_slots":
		return "记忆槽位"
	default:
		return ""
	}
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func mapSliceFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		return typed
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]interface{}); ok {
				out = append(out, mapped)
			}
		}
		return out
	default:
		return nil
	}
}

func firstNonEmptyValue(values ...interface{}) interface{} {
	for _, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case []map[string]interface{}:
			if len(typed) > 0 {
				return typed
			}
		case []interface{}:
			if len(typed) > 0 {
				return typed
			}
		default:
			if strings.TrimSpace(fmt.Sprint(value)) != "" {
				return value
			}
		}
	}
	return nil
}

func firstPositiveInt(values ...interface{}) int {
	for _, value := range values {
		if intValue, ok := intFromAny(value); ok && intValue > 0 {
			return intValue
		}
	}
	return 0
}

func firstNonNegativeInt(values ...interface{}) int {
	for _, value := range values {
		if intValue, ok := intFromAny(value); ok && intValue >= 0 {
			return intValue
		}
	}
	return 0
}

func boolFromAny(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "1", "y":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func intFromAny(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return 0, false
		}
		var out int
		if _, err := fmt.Sscanf(typed, "%d", &out); err != nil {
			return 0, false
		}
		return out, true
	default:
		return 0, false
	}
}

func countAgentDeleteItems(items []map[string]interface{}, status string) int {
	count := 0
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(item["status"])), status) {
			count++
		}
	}
	return count
}

func agentDeleteItemNames(items []map[string]interface{}, status string) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if status != "" && !strings.EqualFold(strings.TrimSpace(stringFromAny(item["status"])), status) {
			continue
		}
		if name := agentDeleteItemName(item); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func agentDeleteFailedItems(items []map[string]interface{}) []string {
	out := make([]string, 0)
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(item["status"])), "failed") {
			continue
		}
		name := agentDeleteItemName(item)
		if name == "" {
			index := 0
			if value, ok := intFromAny(item["index"]); ok {
				index = value + 1
			}
			if index > 0 {
				name = fmt.Sprintf("第 %d 个", index)
			} else {
				name = "某个智能体"
			}
		}
		if reason := strings.TrimSpace(stringFromAny(item["error"])); reason != "" {
			name += "（" + reason + "）"
		}
		out = append(out, name)
	}
	return out
}

func agentDeleteItemName(item map[string]interface{}) string {
	if len(item) == 0 {
		return ""
	}
	if name := strings.TrimSpace(firstNonEmptyString(item["agent_name"], item["name"])); name != "" {
		return name
	}
	agent := payloadMap(item, "agent")
	return strings.TrimSpace(firstNonEmptyString(agent["name"], agent["agent_name"]))
}

func formatNameList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	if len(names) > 5 {
		names = names[:5]
		return "（" + strings.Join(names, "、") + " 等）"
	}
	return "（" + strings.Join(names, "、") + "）"
}

func formatFailedItems(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) > 5 {
		items = items[:5]
		return strings.Join(items, "、") + " 等"
	}
	return strings.Join(items, "、")
}

package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

var runtimeContextRoutePattern = regexp.MustCompile(`(?i)(?:^|[\s,;])route=([^\s,;]+)`)

func createdAgentDetailHrefFromCalls(calls []skillloop.SkillToolCallRef) string {
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "create_agent") {
			continue
		}
		if href := normalizeAgentDetailHref(firstNonEmptyString(
			stringFromAny(call.Result["href"]),
			stringFromAny(call.Result["detail_href"]),
		)); href != "" {
			return href
		}
		if agent := governanceMapFromAny(call.Result["agent"]); len(agent) > 0 {
			if href := normalizeAgentDetailHref(firstNonEmptyString(
				stringFromAny(agent["href"]),
				stringFromAny(agent["detail_href"]),
			)); href != "" {
				return href
			}
			if agentID := strings.TrimSpace(firstNonEmptyString(
				stringFromAny(agent["agent_id"]),
				stringFromAny(agent["id"]),
			)); agentID != "" {
				return consoleAgentDetailHref(agentID)
			}
		}
		if agentID := strings.TrimSpace(firstNonEmptyString(
			stringFromAny(call.Result["agent_id"]),
			stringFromAny(call.Result["id"]),
		)); agentID != "" {
			return consoleAgentDetailHref(agentID)
		}
	}
	return ""
}

func consoleAgentDetailHref(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return ""
	}
	return "/console/agents/" + agentID + "/agent"
}

func consoleAgentIDFromDetailHref(href string) string {
	href = normalizeConsoleNavigationGuardHref(href)
	if !strings.HasPrefix(href, "/console/agents/") {
		return ""
	}
	rest := strings.TrimPrefix(href, "/console/agents/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return ""
	}
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

func normalizeAgentDetailHref(href string) string {
	if agentID := consoleAgentIDFromDetailHref(href); agentID != "" {
		return consoleAgentDetailHref(agentID)
	}
	return ""
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
		if consoleNavigationLoadedHrefMatchesTarget(stringFromAny(continuation["href"]), href) {
			return true
		}
		if consoleNavigationResultMatchesTarget(governanceMapFromAny(continuation["result"]), href) {
			return true
		}
	}
	return false
}

func managedFileCreateShouldBlockReturningToCompletedRoute(metadata map[string]interface{}, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" || consoleNavigationLoadedHrefMatchesTarget(href, consoleFilesRouteHint().Href) {
		return false
	}
	if !clientActionMetadataHasCompletedRoute(metadata, consoleFilesRouteHint().Href) {
		return false
	}
	return clientActionMetadataHasCompletedRoute(metadata, href)
}

func consoleNavigationResultMatchesTarget(result map[string]interface{}, href string) bool {
	if len(result) == 0 {
		return false
	}
	for _, key := range []string{"href", "observed_path", "loaded_href", "target_page"} {
		if consoleNavigationLoadedHrefMatchesTarget(stringFromAny(result[key]), href) {
			return true
		}
	}
	return false
}

func consoleFilesRouteAlreadyAvailable(parts *chatRequestParts) bool {
	if parts == nil {
		return false
	}
	return isConsoleFilesContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext)
}

func consoleNavigationRouteAlreadyAvailable(parts *chatRequestParts, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if parts == nil || href == "" {
		return false
	}
	if runtimeHref := consoleRouteFromRuntimeContext(parts.RuntimeContext); consoleNavigationLoadedHrefMatchesTarget(runtimeHref, href) {
		return true
	}
	switch href {
	case "/console/files":
		if consoleFilesRouteAlreadyAvailable(parts) {
			return true
		}
	case "/console/agents":
		if isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
			return true
		}
	case "/console/workflows":
		if runtimeHref := consoleRouteFromRuntimeContext(parts.RuntimeContext); consoleNavigationLoadedHrefMatchesTarget(runtimeHref, href) {
			return true
		}
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		if consoleNavigationContextContainsRoute(source, href) {
			return true
		}
	}
	return false
}

func consoleRouteFromRuntimeContext(runtimeContext string) string {
	runtimeContext = strings.TrimSpace(runtimeContext)
	if runtimeContext == "" {
		return ""
	}
	matches := runtimeContextRoutePattern.FindStringSubmatch(runtimeContext)
	if len(matches) < 2 {
		return ""
	}
	return normalizeConsoleNavigationGuardHref(matches[1])
}

func consoleNavigationContextContainsRoute(context map[string]interface{}, href string) bool {
	if len(context) == 0 || strings.TrimSpace(href) == "" {
		return false
	}
	for _, item := range operationItemsFromValue(context["resources"]) {
		resource, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		metadata := mapFromOperationContext(resource["metadata"])
		resourceType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
			resource["resource_type"],
			resource["type"],
			resource["kind"],
			metadata["resource_kind"],
			metadata["type"],
		)))
		if resourceType != "page" {
			continue
		}
		for _, value := range []interface{}{
			resource["href"],
			resource["route"],
			metadata["href"],
			metadata["route"],
		} {
			if consoleNavigationLoadedHrefMatchesTarget(stringMetadataValue(value), href) {
				return true
			}
		}
	}
	return false
}

func requiresConsoleFilesRouteBeforeManagedFileCreate(parts *chatRequestParts) bool {
	if parts == nil || !turnTaskContractRequestsManagedFileCreate(parts, nil, "") {
		return false
	}
	if consoleFilesRouteAlreadyAvailable(parts) {
		return false
	}
	if clientActionContinuationLoadedRoute(parts, consoleFilesRouteHint().Href) {
		return false
	}
	return skillIDEnabled(parts.SkillIDs, skills.SkillConsoleNavigator)
}

func resolveConsoleNavigationTargetForPrepared(prepared *PreparedChat) (consoleNavigationRouteHint, bool) {
	if prepared == nil {
		return consoleNavigationRouteHint{}, false
	}
	return resolveConsoleNavigationTargetForParts(prepared.parts)
}

func resolveConsoleNavigationTargetForParts(parts *chatRequestParts) (consoleNavigationRouteHint, bool) {
	if parts == nil {
		return consoleNavigationRouteHint{}, false
	}
	if requiresConsoleFilesRouteBeforeManagedFileCreate(parts) {
		return consoleFilesRouteHint(), true
	}
	targets := consoleNavigationResolvedTargetsForParts(parts)
	if len(targets) == 0 {
		return consoleNavigationRouteHint{}, false
	}
	if len(targets) == 1 {
		return targets[0], true
	}
	completedHrefs := completedConsoleNavigationHrefsFromParts(parts)
	for _, target := range targets {
		if consumeCompletedConsoleNavigationHref(&completedHrefs, target.Href) {
			continue
		}
		if !clientActionContinuationLoadedRoute(parts, target.Href) {
			return target, true
		}
	}
	return targets[len(targets)-1], true
}

func normalizeConsoleNavigationRouteHint(route consoleNavigationRouteHint) consoleNavigationRouteHint {
	if route.Href != "/console/agents" {
		return route
	}
	for _, keyword := range route.Keywords {
		if strings.Contains(strings.ToLower(strings.TrimSpace(keyword)), "workflow") {
			route.Href = "/console/workflows"
			if strings.TrimSpace(route.Label) == "" {
				route.Label = "Workflows"
			}
			return route
		}
	}
	return route
}

func remainingConsoleNavigationRouteSequence(parts *chatRequestParts, nextTarget consoleNavigationRouteHint) []AIChatTurnStrategyRouteStep {
	if parts == nil {
		return nil
	}
	targets := consoleNavigationResolvedTargetsForParts(parts)
	if len(targets) == 0 && strings.TrimSpace(nextTarget.Href) != "" {
		targets = []consoleNavigationRouteHint{nextTarget}
	}
	sequence := make([]AIChatTurnStrategyRouteStep, 0, len(targets))
	completedHrefs := completedConsoleNavigationHrefsFromParts(parts)
	for _, target := range targets {
		if strings.TrimSpace(target.Href) == "" {
			continue
		}
		if consumeCompletedConsoleNavigationHref(&completedHrefs, target.Href) {
			continue
		}
		status := "pending"
		if consoleNavigationLoadedHrefMatchesTarget(target.Href, nextTarget.Href) {
			status = "next"
		}
		sequence = append(sequence, AIChatTurnStrategyRouteStep{
			Href:   target.Href,
			Label:  target.Label,
			Status: status,
		})
	}
	if len(sequence) == 0 && strings.TrimSpace(nextTarget.Href) != "" {
		sequence = append(sequence, AIChatTurnStrategyRouteStep{
			Href:   nextTarget.Href,
			Label:  nextTarget.Label,
			Status: "next",
		})
	}
	return sequence
}

func consoleNavigationRouteHintForHref(href string) consoleNavigationRouteHint {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return consoleNavigationRouteHint{}
	}
	for _, route := range consoleNavigationRouteHints {
		if consoleNavigationLoadedHrefMatchesTarget(route.Href, href) {
			return route
		}
	}
	return consoleNavigationRouteHint{Href: href, Label: "Console page"}
}

func completedConsoleNavigationHrefsFromParts(parts *chatRequestParts) []string {
	if parts == nil {
		return nil
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		completed := completedConsoleNavigationHrefsFromOperationContext(source)
		if len(completed) > 0 {
			return completed
		}
	}
	return nil
}

func completedConsoleNavigationHrefsFromOperationContext(source map[string]interface{}) []string {
	if len(source) == 0 {
		return nil
	}
	hrefs := make([]string, 0, 4)
	for _, action := range mapSliceFromAny(source["completed_client_actions"]) {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["status"])), clientActionStatusSucceeded) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(action["action_type"])), "route_navigation") {
			continue
		}
		if href := clientActionRouteHref(action); href != "" {
			hrefs = append(hrefs, href)
		}
	}
	continuation := governanceMapFromAny(source["client_action_continuation"])
	if strings.EqualFold(strings.TrimSpace(stringFromAny(continuation["status"])), clientActionStatusSucceeded) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(continuation["action_type"])), "route_navigation") {
		if href := clientActionRouteHref(continuation); href != "" && !lastConsoleNavigationHrefMatches(hrefs, href) {
			hrefs = append(hrefs, href)
		}
	}
	return hrefs
}

func lastConsoleNavigationHrefMatches(hrefs []string, href string) bool {
	if len(hrefs) == 0 {
		return false
	}
	return consoleNavigationLoadedHrefMatchesTarget(hrefs[len(hrefs)-1], href)
}

func consumeCompletedConsoleNavigationHref(completed *[]string, targetHref string) bool {
	if completed == nil || len(*completed) == 0 {
		return false
	}
	for len(*completed) > 0 {
		href := (*completed)[0]
		*completed = (*completed)[1:]
		if consoleNavigationLoadedHrefMatchesTarget(href, targetHref) {
			return true
		}
	}
	return false
}

func consoleFilesRouteHint() consoleNavigationRouteHint {
	for _, route := range consoleNavigationRouteHints {
		if consoleNavigationLoadedHrefMatchesTarget(route.Href, "/console/files") {
			return route
		}
	}
	return consoleNavigationRouteHint{Href: "/console/files", Label: "File Management"}
}

func isConsoleNavigatorNavigateTool(skillID string, toolName string) bool {
	return strings.EqualFold(strings.TrimSpace(skillID), skills.SkillConsoleNavigator) &&
		strings.EqualFold(strings.TrimSpace(toolName), "navigate")
}

func consoleNavigationRequiredToolFinalAnswerGuard(target consoleNavigationRouteHint) skillloop.FinalAnswerGuard {
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if finalAnswerGuardHasSuccessfulToolForConsoleHref(req, skills.SkillConsoleNavigator, "navigate", target.Href) ||
			finalAnswerGuardHasAttemptedToolForConsoleHref(req, skills.SkillConsoleNavigator, "navigate", target.Href) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return consoleNavigationRequiredToolGuardResult(target), true
	}
}

func consoleNavigationRequiredToolGuardResult(target consoleNavigationRouteHint) skillloop.FinalAnswerGuardResult {
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
	}
}

func createdAgentRequiresDetailNavigationGuardResult(target consoleNavigationRouteHint) skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		fmt.Sprintf("The Agent was created successfully and the user asked to open its detail page (%s).", target.Href),
		"Do not finish with a natural-language message saying the detail page is open yet.",
		fmt.Sprintf("Call call_skill_tool with skill_id %q, tool_name %q, and href %q.", skills.SkillConsoleNavigator, "navigate", target.Href),
		"Only after navigate succeeds in this turn may you tell the user that the Agent was created and opened.",
	}, " ")
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"arguments": map[string]interface{}{
			"href": target.Href,
		},
		"label": target.Label,
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
	}
}

func agentDeleteRequiresListNavigationGuardResult(target consoleNavigationRouteHint) skillloop.FinalAnswerGuardResult {
	message := strings.Join([]string{
		"The current Agent detail page was deleted successfully.",
		"Do not finish with a natural-language message saying the Agent list is open yet.",
		fmt.Sprintf("Call call_skill_tool with skill_id %q, tool_name %q, and href %q to navigate back to the Agent list.", skills.SkillConsoleNavigator, "navigate", target.Href),
		"Only after navigate succeeds in this turn may you tell the user that the Agent was deleted and the list page is open.",
	}, " ")
	payload := map[string]interface{}{
		"skill_id":  skills.SkillConsoleNavigator,
		"tool_name": "navigate",
		"arguments": map[string]interface{}{
			"href": target.Href,
		},
		"label": target.Label,
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
	}
}

func consoleNavigationLoadedHrefMatchesTarget(loadedHref string, targetHref string) bool {
	loadedHref = normalizeConsoleNavigationGuardHref(loadedHref)
	targetHref = normalizeConsoleNavigationGuardHref(targetHref)
	if loadedHref == "" || targetHref == "" {
		return false
	}
	if loadedAgentHref := normalizeAgentDetailHref(loadedHref); loadedAgentHref != "" {
		if targetAgentHref := normalizeAgentDetailHref(targetHref); targetAgentHref != "" {
			return loadedAgentHref == targetAgentHref
		}
	}
	if loadedHref == targetHref {
		return true
	}
	if targetHref == "/" || targetHref == "/console" {
		return false
	}
	switch targetHref {
	case "/console/agents", "/console/workflows":
		return strings.HasPrefix(loadedHref, targetHref+"/")
	default:
		return false
	}
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

func clientActionRouteHref(event map[string]interface{}) string {
	if len(event) == 0 {
		return ""
	}
	if href := normalizeConsoleNavigationGuardHref(stringFromAny(event["href"])); href != "" {
		return href
	}
	result := governanceMapFromAny(event["result"])
	for _, key := range []string{"observed_path", "href", "loaded_href", "target_page"} {
		if href := normalizeConsoleNavigationGuardHref(stringFromAny(result[key])); href != "" {
			return href
		}
	}
	return ""
}

func consoleNavigationHrefAllowedForCurrentScope(href string, targets []consoleNavigationRouteHint) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return false
	}
	for _, target := range targets {
		if consoleNavigationLoadedHrefMatchesTarget(href, target.Href) {
			return true
		}
	}
	return false
}

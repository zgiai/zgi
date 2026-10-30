package service

import (
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

var runtimeContextRoutePattern = regexp.MustCompile(`(?i)(?:^|[\s,;])route=([^\s,;]+)`)

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

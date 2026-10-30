package service

import "strings"

func agentIDFromConsoleAgentPageRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}
	const prefix = "/console/agents/"
	idx := strings.Index(route, prefix)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimPrefix(route[idx:], prefix)
	agentID, _, _ := strings.Cut(rest, "/")
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || agentID == "new" {
		return ""
	}
	return agentID
}

func agentManagementTargetPage(parts *chatRequestParts, strategy *AIChatTurnStrategy) string {
	runtimeContext := ""
	rawOperationContext := map[string]interface{}(nil)
	operationContext := map[string]interface{}(nil)
	if parts != nil {
		runtimeContext = firstNonEmptyString(parts.RuntimeContext, "")
		rawOperationContext = parts.RawOperationContext
		operationContext = parts.OperationContext
	}
	explicitTargetPage := normalizeConsoleNavigationGuardHref(stringFromAny(strategy.TargetPage))
	currentPage := normalizeConsoleNavigationGuardHref(stringFromAny(firstNonEmptyString(strategy.CurrentPage, "")))
	if explicitTargetPage != "" && explicitTargetPage != currentPage && strings.HasPrefix(explicitTargetPage, "/console/agents") {
		return explicitTargetPage
	}
	if explicitTargetPage != "" && explicitTargetPage != currentPage &&
		strings.HasPrefix(explicitTargetPage, "/console/files") &&
		agentManagementNeedsFileReadPrecondition(parts) {
		return explicitTargetPage
	}
	for _, candidate := range []string{
		currentPage,
		consoleRouteFromRuntimeContext(runtimeContext),
	} {
		if isConsoleAgentDetailRoute(candidate) {
			return normalizeConsoleNavigationGuardHref(candidate)
		}
	}
	for _, source := range []map[string]interface{}{rawOperationContext, operationContext} {
		agents := visibleAgentResources(source)
		for _, agent := range agents {
			if agent.Selected && isConsoleAgentDetailRoute(agent.Href) {
				return normalizeConsoleNavigationGuardHref(agent.Href)
			}
		}
		if len(agents) == 1 && isConsoleAgentDetailRoute(agents[0].Href) {
			return normalizeConsoleNavigationGuardHref(agents[0].Href)
		}
	}
	return "/console/agents"
}

func agentManagementExplicitDetailNavigationTarget(parts *chatRequestParts) (consoleNavigationRouteHint, bool) {
	if parts == nil {
		return consoleNavigationRouteHint{}, false
	}
	if target, ok := agentManagementModelDetailNavigationTarget(parts); ok {
		return target, true
	}
	return consoleNavigationRouteHint{}, false
}

func agentManagementModelDetailNavigationTarget(parts *chatRequestParts) (consoleNavigationRouteHint, bool) {
	if parts == nil || parts.ModelTurnIntent == nil {
		return consoleNavigationRouteHint{}, false
	}
	href := normalizeAgentDetailHref(parts.ModelTurnIntent.TargetPage)
	if href == "" {
		return consoleNavigationRouteHint{}, false
	}
	return consoleNavigationRouteHintForHref(href), true
}

func isConsoleAgentDetailRoute(value string) bool {
	return normalizeAgentDetailHref(value) != ""
}

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
	normalized := normalizeConsoleNavigationQuery(parts.Query)
	if normalized == "" || createdAgentDetailNavigationNegated(normalized) {
		return consoleNavigationRouteHint{}, false
	}
	if !containsAnySubstring(normalized, []string{
		"open", "enter", "detail", "details", "edit page", "configuration page", "config page", "settings page",
		"\u6253\u5f00", "\u8fdb\u5165", "\u8be6\u60c5", "\u7f16\u8f91\u9875", "\u914d\u7f6e\u9875", "\u8bbe\u7f6e\u9875",
	}) {
		return consoleNavigationRouteHint{}, false
	}
	if parts.ModelTurnIntent != nil && modelTurnIntentAssetEffectIsDelete(parts.ModelTurnIntent.AssetEffect) {
		return consoleNavigationRouteHint{}, false
	}
	agents := visibleAgentsForAgentManagementTargetResolution(parts)
	matches := make([]visibleConsoleAgentResource, 0, 1)
	for _, agent := range agents {
		name := normalizeConsoleNavigationQuery(agent.Title)
		if name == "" || !strings.Contains(normalized, name) {
			continue
		}
		matches = append(matches, agent)
	}
	if len(matches) == 1 {
		href := normalizeAgentDetailHref(matches[0].Href)
		if href == "" && strings.TrimSpace(matches[0].ID) != "" {
			href = consoleAgentDetailHref(matches[0].ID)
		}
		if href != "" {
			return consoleNavigationRouteHint{Href: href, Label: firstNonEmptyString(matches[0].Title, "Agent detail")}, true
		}
	}
	if len(matches) > 1 {
		return consoleNavigationRouteHint{}, false
	}
	if containsAnySubstring(normalized, []string{"current agent", "this agent", "\u5f53\u524d\u667a\u80fd\u4f53", "\u8fd9\u4e2a\u667a\u80fd\u4f53"}) {
		for _, agent := range agents {
			if !agent.Selected {
				continue
			}
			if href := normalizeAgentDetailHref(agent.Href); href != "" {
				return consoleNavigationRouteHint{Href: href, Label: firstNonEmptyString(agent.Title, "Agent detail")}, true
			}
		}
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

func visibleAgentsForAgentManagementTargetResolution(parts *chatRequestParts) []visibleConsoleAgentResource {
	if parts == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []visibleConsoleAgentResource{}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, agent := range visibleAgentResources(source) {
			id := strings.TrimSpace(agent.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, agent)
		}
	}
	return out
}

func isConsoleAgentDetailRoute(value string) bool {
	return normalizeAgentDetailHref(value) != ""
}

func currentConsoleAgentID(parts *chatRequestParts) string {
	if parts == nil {
		return ""
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		agents := visibleAgentResources(source)
		for _, agent := range agents {
			if agent.Selected && strings.TrimSpace(agent.ID) != "" {
				return strings.TrimSpace(agent.ID)
			}
		}
		if len(agents) == 1 && strings.TrimSpace(agents[0].ID) != "" {
			return strings.TrimSpace(agents[0].ID)
		}
	}
	return ""
}

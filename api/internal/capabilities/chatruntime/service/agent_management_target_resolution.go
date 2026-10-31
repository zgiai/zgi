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

func isConsoleAgentDetailRoute(value string) bool {
	return normalizeAgentDetailHref(value) != ""
}

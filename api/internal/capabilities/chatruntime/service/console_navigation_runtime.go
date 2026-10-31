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

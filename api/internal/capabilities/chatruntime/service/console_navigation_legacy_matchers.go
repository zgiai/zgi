package service

import (
	"sort"
	"strconv"
	"strings"
)

func consoleNavigationResolvedTargets(query string) []consoleNavigationRouteHint {
	if !isConsoleNavigationIntent(query) {
		return nil
	}
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return nil
	}
	resolved := make([]resolvedConsoleNavigationTarget, 0, 4)
	seen := map[string]struct{}{}
	for _, route := range consoleNavigationRouteHints {
		route = normalizeConsoleNavigationRouteHint(route)
		href := strings.ToLower(route.Href)
		for _, position := range consoleRouteHrefIndexes(normalized, href) {
			key := route.Href + "\x00" + strconv.Itoa(position)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			resolved = append(resolved, resolvedConsoleNavigationTarget{Hint: route, Position: position})
		}
		for _, keyword := range consoleNavigationRouteKeywords(route) {
			keyword = normalizeConsoleNavigationQuery(keyword)
			if keyword == "" {
				continue
			}
			for _, position := range allStringIndexes(normalized, keyword) {
				if !consoleNavigationKeywordPositionAllowed(keyword, normalized, position) {
					continue
				}
				key := route.Href + "\x00" + strconv.Itoa(position)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				resolved = append(resolved, resolvedConsoleNavigationTarget{Hint: route, Position: position})
			}
		}
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		return resolved[i].Position < resolved[j].Position
	})
	targets := make([]consoleNavigationRouteHint, 0, len(resolved))
	for _, item := range resolved {
		if len(targets) > 0 && consoleNavigationLoadedHrefMatchesTarget(targets[len(targets)-1].Href, item.Hint.Href) {
			continue
		}
		targets = append(targets, item.Hint)
	}
	return targets
}

func consoleRouteHrefIndexes(haystack string, href string) []int {
	indexes := allStringIndexes(haystack, href)
	if href != "/console" || len(indexes) == 0 {
		return indexes
	}
	filtered := indexes[:0]
	for _, position := range indexes {
		next := position + len(href)
		if next < len(haystack) && haystack[next] == '/' {
			continue
		}
		filtered = append(filtered, position)
	}
	return filtered
}

func consoleNavigationKeywordPositionAllowed(keyword, normalized string, position int) bool {
	if keyword == "" || normalized == "" || position < 0 || position+len(keyword) > len(normalized) {
		return false
	}
	if consoleNavigationWorkspaceKeywordIsScopePhrase(keyword, normalized, position) {
		return false
	}
	if consoleNavigationKeywordHasExplicitPageCue(keyword) {
		return true
	}
	suffix := normalized[position+len(keyword):]
	for _, blockedSuffix := range []string{
		"\u540d\u79f0",
		"\u540d\u5b57",
		"id",
		"\u7f16\u53f7",
		"\u914d\u7f6e",
		"\u5185\u5bb9",
		"\u63cf\u8ff0",
		"\u7ed3\u679c",
		"\u8f93\u51fa",
		"\u7ed1\u5b9a",
		"\u6570\u91cf",
		"\u72b6\u6001",
	} {
		if strings.HasPrefix(suffix, blockedSuffix) {
			return false
		}
	}
	return true
}

func consoleNavigationWorkspaceKeywordIsScopePhrase(keyword, normalized string, position int) bool {
	keyword = strings.TrimSpace(keyword)
	if keyword != "工作空间" && keyword != "workspace" {
		return false
	}
	prefix := normalized[:position]
	suffix := normalized[position+len(keyword):]
	for _, pageCue := range []string{"页", "页面", "管理", " page", " module"} {
		if strings.HasPrefix(suffix, pageCue) {
			return false
		}
	}
	for _, scopePrefix := range []string{"当前", "本", "这个", "该", "所在", "当前 ", "current ", "this "} {
		if strings.HasSuffix(prefix, scopePrefix) {
			return true
		}
	}
	return false
}

func consoleNavigationKeywordHasExplicitPageCue(keyword string) bool {
	return containsAnySubstring(keyword, []string{
		"/console",
		"page",
		"list",
		"\u9875",
		"\u9875\u9762",
		"\u7ba1\u7406",
		"\u6a21\u5757",
		"\u9996\u9875",
		"\u4e3b\u9875",
		"\u63a7\u5236\u53f0",
	})
}

func allStringIndexes(haystack string, needle string) []int {
	if haystack == "" || needle == "" {
		return nil
	}
	indexes := []int{}
	offset := 0
	for {
		idx := strings.Index(haystack[offset:], needle)
		if idx < 0 {
			return indexes
		}
		position := offset + idx
		indexes = append(indexes, position)
		offset = position + len(needle)
		if offset >= len(haystack) {
			return indexes
		}
	}
}

func consoleNavigationRouteKeywords(route consoleNavigationRouteHint) []string {
	keywords := append([]string(nil), route.Keywords...)
	switch route.Href {
	case "/console":
		keywords = append(keywords, "\u9996\u9875", "\u4e3b\u9875", "\u63a7\u5236\u53f0\u9996\u9875")
	case "/console/work/chat":
		keywords = append(keywords, "\u5bf9\u8bdd", "\u804a\u5929", "\u4f1a\u8bdd", "\u5bf9\u8bdd\u9875", "\u804a\u5929\u9875")
	case "/console/work/image":
		keywords = append(keywords, "\u7ed8\u56fe", "\u56fe\u50cf\u751f\u6210", "\u56fe\u7247\u751f\u6210")
	case "/console/work/app":
		keywords = append(keywords, "\u5e94\u7528", "\u5e94\u7528\u9875", "\u5e94\u7528\u7ba1\u7406")
	case "/console/work/task":
		keywords = append(keywords, "\u5b9a\u65f6\u4efb\u52a1", "\u8ba1\u5212\u4efb\u52a1", "\u4efb\u52a1\u9875")
	case "/console/agents":
		keywords = append(keywords, "\u667a\u80fd\u4f53", "\u667a\u80fd\u4f53\u9875", "\u667a\u80fd\u4f53\u9875\u9762", "\u667a\u80fd\u4f53\u7ba1\u7406", "\u5de5\u4f5c\u6d41", "\u5de5\u4f5c\u6d41\u9875")
	case "/console/dataset":
		keywords = append(keywords, "\u77e5\u8bc6\u5e93", "\u77e5\u8bc6\u5e93\u9875", "\u6570\u636e\u96c6")
	case "/console/db":
		keywords = append(keywords, "\u6570\u636e\u5e93", "\u6570\u636e\u5e93\u9875", "\u6570\u636e\u8868")
	case "/console/files":
		keywords = append(keywords, "\u6587\u4ef6\u7ba1\u7406", "\u6587\u4ef6\u9875", "\u6587\u4ef6\u9875\u9762", "\u6587\u4ef6\u6a21\u5757")
	case "/console/prompts":
		keywords = append(keywords, "\u63d0\u793a\u8bcd", "\u63d0\u793a\u8bcd\u9875")
	case "/console/developer/content-parse":
		keywords = append(keywords, "\u6587\u4ef6\u8bc6\u522b", "\u5185\u5bb9\u89e3\u6790")
	case "/console/workspace":
		keywords = append(keywords, "\u5de5\u4f5c\u7a7a\u95f4")
	case "/console/settings":
		keywords = append(keywords, "\u7cfb\u7edf\u8bbe\u7f6e", "\u8bbe\u7f6e\u9875")
	}
	return keywords
}

func wantsCreatedAgentDetailNavigation(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	if createdAgentDetailNavigationNegated(normalized) {
		return false
	}
	hasCreate := false
	for _, marker := range []string{
		"\u521b\u5efa", "\u65b0\u5efa", "\u65b0\u589e",
		"create", "new agent",
	} {
		if strings.Contains(normalized, marker) {
			hasCreate = true
			break
		}
	}
	for _, marker := range []string{"鍒涘缓", "鏂板缓", "create", "new agent"} {
		if strings.Contains(normalized, marker) {
			hasCreate = true
			break
		}
	}
	if !hasCreate {
		return false
	}
	for _, marker := range []string{
		"\u6253\u5f00", "\u8fdb\u5165", "\u8fdb\u5230",
		"\u8be6\u60c5", "\u8be6\u7ec6", "\u7f16\u8f91\u9875", "\u914d\u7f6e\u9875",
		"open", "enter", "detail", "details", "view", "config page", "settings page", "edit page",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	for _, marker := range []string{"鎵撳紑", "杩涘叆", "璇︽儏", "open", "enter", "detail", "view"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func createdAgentDetailNavigationNegated(normalized string) bool {
	return containsAnySubstring(normalized, []string{
		"do not navigate", "don't navigate", "dont navigate",
		"do not switch pages", "don't switch pages", "dont switch pages",
		"do not open", "don't open", "dont open",
		"do not enter", "don't enter", "dont enter",
		"without navigating", "without opening", "without switching pages",
		"stay on the list", "stay on current page", "stay on the current page",
		"\u4e0d\u8981\u5bfc\u822a", "\u4e0d\u8981\u8df3\u8f6c", "\u4e0d\u8981\u5207\u6362", "\u4e0d\u8981\u5207\u6362\u9875\u9762", "\u4e0d\u8981\u5207\u6362\u5230\u5176\u4ed6\u9875\u9762", "\u4e0d\u8981\u6253\u5f00", "\u4e0d\u8981\u8fdb\u5165",
		"\u4e0d\u7528\u5bfc\u822a", "\u4e0d\u7528\u8df3\u8f6c", "\u4e0d\u7528\u5207\u6362", "\u4e0d\u7528\u5207\u6362\u9875\u9762", "\u4e0d\u7528\u6253\u5f00", "\u4e0d\u7528\u8fdb\u5165",
		"\u65e0\u9700\u5bfc\u822a", "\u65e0\u9700\u8df3\u8f6c", "\u65e0\u9700\u5207\u6362", "\u65e0\u9700\u5207\u6362\u9875\u9762", "\u65e0\u9700\u6253\u5f00", "\u65e0\u9700\u8fdb\u5165",
		"\u7559\u5728\u5217\u8868", "\u7559\u5728\u5f53\u524d\u9875", "\u7559\u5728\u5f53\u524d\u9875\u9762",
	})
}

func consoleNavigationRequestNegated(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	return createdAgentDetailNavigationNegated(normalized)
}

package service

import "strings"

func isConsoleNavigationIntent(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	return containsAnySubstring(normalized, []string{
		"\u6253\u5f00",
		"\u8df3\u8f6c",
		"\u5207\u6362",
		"\u8fdb\u5165",
		"\u5bfc\u822a",
		"鍒囧埌",
		"璺宠浆",
		"杩涘叆",
		"go to",
		"open",
		"switch to",
		"navigate",
	})
}

func consoleNavigationResolvedTargets(query string) []consoleNavigationRouteHint {
	normalized := normalizeConsoleNavigationQuery(query)
	type resolved struct {
		href string
		pos  int
	}
	resolvedTargets := []resolved{}
	seen := map[string]struct{}{}
	addMatches := func(href string, markers []string) {
		for _, marker := range markers {
			for _, pos := range allStringIndexes(normalized, marker) {
				if href == "/console/agents" {
					suffix := normalized[pos+len(marker):]
					if strings.HasPrefix(suffix, "\u540d\u79f0") || strings.HasPrefix(suffix, "\u540d\u5b57") {
						continue
					}
				}
				key := href + "\x00" + string(rune(pos))
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				resolvedTargets = append(resolvedTargets, resolved{href: href, pos: pos})
			}
		}
	}
	addMatches("/console/files", []string{"\u6587\u4ef6\u7ba1\u7406", "\u6587\u4ef6\u9875", "/console/files", "鏂囦欢绠＄悊"})
	addMatches("/console/agents", []string{"\u667a\u80fd\u4f53", "\u667a\u80fd\u4f53\u9875", "/console/agents", "櫤鑳戒綋"})
	addMatches("/console/db", []string{"\u6570\u636e\u5e93", "\u6570\u636e\u5e93\u9875", "/console/db"})
	for i := 0; i < len(resolvedTargets); i++ {
		for j := i + 1; j < len(resolvedTargets); j++ {
			if resolvedTargets[j].pos < resolvedTargets[i].pos {
				resolvedTargets[i], resolvedTargets[j] = resolvedTargets[j], resolvedTargets[i]
			}
		}
	}
	targets := make([]consoleNavigationRouteHint, 0, len(resolvedTargets))
	for _, target := range resolvedTargets {
		targets = append(targets, consoleNavigationRouteHintForHref(target.href))
	}
	return targets
}

func wantsCreatedAgentDetailNavigation(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	if containsAnySubstring(normalized, []string{
		"do not navigate", "don't navigate", "dont navigate",
		"do not open", "don't open", "dont open",
		"\u4e0d\u8981\u5bfc\u822a", "\u4e0d\u8981\u8df3\u8f6c", "\u4e0d\u8981\u6253\u5f00", "\u4e0d\u8981\u8fdb\u5165",
	}) {
		return false
	}
	return containsAnySubstring(normalized, []string{
		"\u521b\u5efa", "\u65b0\u5efa", "\u65b0\u589e", "create", "new agent",
	}) && containsAnySubstring(normalized, []string{
		"\u6253\u5f00", "\u8fdb\u5165", "\u8be6\u60c5", "\u7f16\u8f91\u9875", "\u914d\u7f6e\u9875",
		"open", "enter", "detail", "details", "edit page", "config page",
	})
}

func isFileReadIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if containsAnySubstring(text, []string{
		"read", "preview", "summary", "summarize", "analyse", "analyze",
		"\u8bfb\u53d6", "\u603b\u7ed3", "\u6458\u8981", "\u5206\u6790", "\u9884\u89c8",
	}) {
		return true
	}
	return strings.Contains(text, "\u8bfb") && containsAnySubstring(text, []string{
		"\u7b2c", "\u5f53\u524d", "\u8fd9\u4e2a", "\u5185\u5bb9", "pdf", "excel",
	})
}

func isFileDeleteIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if containsAnySubstring(text, []string{
		"do not delete", "don't delete", "dont delete", "not delete",
		"do not create or delete", "don't create or delete", "dont create or delete",
		"\u522b\u5220\u9664", "\u4e0d\u8981\u5220\u9664", "\u4e0d\u7528\u5220\u9664", "\u65e0\u9700\u5220\u9664",
		"\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664",
		"\u4e0d\u8981\u521b\u5efa\u6216\u5220\u9664",
		"\u4e0d\u8981\u521b\u5efa\u3001\u5220\u9664",
	}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"delete", "remove", "trash", "discard",
		"\u5220\u9664", "\u5220\u6389", "\u5220\u4e86", "\u79fb\u9664", "\u6e05\u7406",
	})
}

func isManagedFileCreateIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if containsAnySubstring(text, []string{
		"do not save", "don't save", "dont save", "temporary only",
		"\u4e0d\u8981\u4fdd\u5b58", "\u4e0d\u7528\u4fdd\u5b58", "\u522b\u4fdd\u5b58", "\u4ec5\u4e34\u65f6",
		"\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664",
	}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"save", "upload", "export", "file management", "managed file",
		"\u4fdd\u5b58", "\u4e0a\u4f20", "\u5bfc\u51fa", "\u6587\u4ef6\u7ba1\u7406",
	}) && containsAnySubstring(text, []string{"file", "\u6587\u4ef6", "txt", "svg", "pdf"})
}

func isTemporaryFileGenerateIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" || isManagedFileCreateIntent(query) || isFileReadIntent(query) || isFileDeleteIntent(query) {
		return false
	}
	if containsAnySubstring(text, []string{
		"do not create", "don't create", "dont create", "read only", "answer only",
		"\u4e0d\u8981\u521b\u5efa", "\u4e0d\u7528\u521b\u5efa", "\u53ea\u56de\u7b54", "\u4ec5\u8bfb",
	}) {
		return false
	}
	return containsAnySubstring(text, []string{
		"create", "generate", "write", "export", "make",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u5199", "\u5bfc\u51fa",
	}) && containsAnySubstring(text, []string{
		"file", ".txt", ".md", ".json", ".csv", ".svg", ".pdf", "txt", "svg", "pdf",
		"\u6587\u4ef6", "\u4e34\u65f6\u6587\u4ef6", "\u6587\u6863",
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

func agentManagementConfigOnlyCapabilityMarkers() []string {
	out := []string{}
	for _, descriptor := range agentManagementConfigOnlyCapabilityDescriptors() {
		out = appendUniqueStrings(out, descriptor.Markers...)
	}
	return out
}

func agentManagementCapabilityStatusTargetMarkers() []string {
	markers := []string{
		"agent",
		"skill",
		"tool",
		"model",
		"provider",
		"\u667a\u80fd\u4f53",
		"\u6280\u80fd",
		"\u5de5\u5177",
		"\u6a21\u578b",
		"\u4f9b\u5e94\u5546",
		"\u80fd",
		"\u53ef\u4ee5",
		"\u652f\u6301",
	}
	markers = appendUniqueStrings(markers, agentManagementConfigOnlyCapabilityMarkers()...)
	for _, descriptor := range agentSkillBackedCapabilityDescriptors() {
		markers = appendUniqueStrings(markers, descriptor.Markers...)
	}
	for _, descriptor := range agentManagementBindingCapabilityDescriptors() {
		markers = appendUniqueStrings(markers, descriptor.markers...)
	}
	return markers
}

func agentManagementExplicitConfigMarkerPresent(text string, marker string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	marker = strings.ToLower(strings.TrimSpace(marker))
	if text == "" || marker == "" {
		return false
	}
	searchFrom := 0
	for {
		idx := strings.Index(text[searchFrom:], marker)
		if idx < 0 {
			return false
		}
		absoluteIdx := searchFrom + idx
		if !agentManagementResourceMarkerNegatedInClause(text, absoluteIdx) {
			return true
		}
		searchFrom = absoluteIdx + len(marker)
		if searchFrom >= len(text) {
			return false
		}
	}
}

func agentManagementResourceMarkerNegatedInClause(text string, markerStart int) bool {
	if markerStart <= 0 {
		return false
	}
	clauseStart := agentManagementClauseStart(text, markerStart)
	prefix := strings.TrimSpace(text[clauseStart:markerStart])
	if prefix == "" {
		return false
	}
	if containsAnySubstring(strings.ToLower(prefix), []string{
		"do not", "don't", "dont", "without", "never", "not ", "no ",
		"keep unchanged", "leave unchanged", "keep the same", "preserve ", "retain ", "unchanged",
	}) {
		return true
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		".", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\u3002", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(prefix)
	return containsAnySubstring(compact, []string{
		"\u4e0d\u6539",
		"\u4e0d\u8981\u6539",
		"\u522b\u6539",
		"\u4e0d\u7528\u6539",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u4e0d\u8981\u7ed1\u5b9a",
		"\u4e0d\u8981\u89e3\u7ed1",
		"\u4e0d\u8981\u521b\u5efa",
		"\u4e0d\u8981\u5220\u9664",
		"\u4e0d\u8bbe\u7f6e",
		"\u4e0d\u5207\u6362",
		"\u4e0d\u66ff\u6362",
		"\u4e0d\u7ed1\u5b9a",
		"\u4e0d\u5173\u8054",
		"\u4e0d\u89e3\u7ed1",
		"\u4e0d\u8981\u52a8",
		"\u65e0\u9700",
		"\u4e0d\u9700\u8981",
		"\u4fdd\u6301\u4e0d\u53d8",
		"\u4fdd\u6301\u539f\u6837",
		"\u7ef4\u6301\u539f\u6837",
		"\u4fdd\u7559",
	}) || agentManagementMutationMarkerNegated(text, markerStart)
}

func agentManagementClauseStart(text string, markerStart int) int {
	if markerStart <= 0 || markerStart > len(text) {
		return 0
	}
	start := 0
	for _, separator := range []string{
		"\uff0c", "\u3002", "\uff1b", "\uff1a", "\uff01", "\uff1f",
		",", ".", ";", ":", "!", "?", "\n", "\r",
	} {
		if separator == "" {
			continue
		}
		if idx := strings.LastIndex(text[:markerStart], separator); idx >= 0 && idx+len(separator) > start {
			start = idx + len(separator)
		}
	}
	return start
}

func agentManagementMutationMarkerNegated(text string, markerStart int) bool {
	if markerStart <= 0 {
		return false
	}
	prefixStart := markerStart - 48
	if prefixStart < 0 {
		prefixStart = 0
	}
	prefix := strings.ToLower(strings.TrimSpace(text[prefixStart:markerStart]))
	if prefix == "" {
		return false
	}
	if containsAnySuffix(prefix, []string{
		"do not", "don't", "dont", "without", "never", "no", "do not make any",
		"do not bind or", "don't bind or", "dont bind or", "without binding or",
		"do not add or", "don't add or", "dont add or", "without adding or",
		"do not create or", "don't create or", "dont create or", "without creating or",
		"do not enable or", "don't enable or", "dont enable or", "without enabling or",
		"do not associate or", "don't associate or", "dont associate or", "without associating or",
	}) {
		return true
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		".", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\u3002", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(prefix)
	if strings.Contains(compact, "\u4e0d\u8981") && containsAnySubstring(compact, []string{
		"\u4fee\u6539", "\u7ed1\u5b9a", "\u89e3\u7ed1", "\u521b\u5efa", "\u5220\u9664",
	}) {
		return true
	}
	return containsAnySuffix(compact, []string{
		"\u4e0d",
		"\u522b",
		"\u7981\u6b62",
		"\u4e0d\u8981",
		"\u4e0d\u7528",
		"\u65e0\u9700",
		"\u4e0d\u9700\u8981",
		"\u4e0d\u53ef",
		"\u4e0d\u505a",
		"\u4e0d\u8981\u505a",
		"\u4e0d\u505a\u4efb\u4f55",
		"\u4e0d\u8981\u505a\u4efb\u4f55",
	})
}

func containsAnySuffix(text string, suffixes []string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	if text == "" {
		return false
	}
	for _, suffix := range suffixes {
		suffix = strings.TrimSpace(strings.ToLower(suffix))
		if suffix != "" && strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

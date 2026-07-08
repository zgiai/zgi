package service

import "strings"

func agentManagementResourceMarkerNegatedInClause(text string, markerStart int) bool {
	if markerStart <= 0 {
		return false
	}
	if agentManagementResourceMarkerNegatedInListScope(text, markerStart) {
		return true
	}
	clauseStart := agentManagementClauseStart(text, markerStart)
	prefix := strings.TrimSpace(text[clauseStart:markerStart])
	if prefix == "" {
		return false
	}
	if containsAnySubstring(prefix, []string{
		"do not", "don't", "dont", "without", "never", "not ", "no ",
		"keep unchanged", "leave unchanged", "keep the same", "keep ", "preserve ", "retain ", "unchanged",
		"do not say", "don't say", "dont say", "do not claim", "don't claim", "dont claim",
		"do not mention", "don't mention", "dont mention", "do not report", "don't report", "dont report",
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
	if containsAnySubstring(compact, []string{
		"\u4e0d\u6539",
		"\u4e0d\u8981\u6539",
		"\u522b\u6539",
		"\u4e0d\u7528\u6539",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u53d8\u66f4",
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
		"\u4e0d\u8981\u8bf4",
		"\u522b\u8bf4",
		"\u4e0d\u8981\u58f0\u79f0",
		"\u522b\u58f0\u79f0",
		"\u4e0d\u8981\u5ba3\u79f0",
		"\u4e0d\u8981\u56de\u7b54",
		"\u4e0d\u8981\u56de\u590d",
		"\u4e0d\u8981\u63d0\u5230",
		"\u4e0d\u8981\u5199",
		"\u4e0d\u8981\u8f93\u51fa",
	}) {
		return true
	}
	return agentManagementMutationMarkerNegated(text, markerStart)
}

func agentManagementResourceMarkerNegatedInListScope(text string, markerStart int) bool {
	if markerStart <= 0 || markerStart > len(text) {
		return false
	}
	sentenceStart := agentManagementNegatedListScopeStart(text, markerStart)
	prefix := strings.ToLower(strings.TrimSpace(text[sentenceStart:markerStart]))
	if prefix == "" {
		return false
	}
	negationStart := agentManagementLastNegatedListIntro(prefix)
	if negationStart < 0 {
		return false
	}
	tail := prefix[negationStart:]
	if containsAnySubstring(tail, []string{
		" but ", " however ", " except ", " unless ", " then ", " and then ",
		"\u4f46", "\u4f46\u662f", "\u4e0d\u8fc7", "\u9664\u975e", "\u7136\u540e",
	}) {
		return false
	}
	return true
}

func agentManagementNegatedListScopeStart(text string, markerStart int) int {
	if markerStart <= 0 || markerStart > len(text) {
		return 0
	}
	start := 0
	for _, separator := range []string{
		"\u3002", "\uff1b", "\uff01", "\uff1f",
		".", ";", "!", "?", "\n", "\r",
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

func agentManagementLastNegatedListIntro(prefix string) int {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return -1
	}
	best := -1
	for _, marker := range []string{
		"do not change", "don't change", "dont change",
		"do not modify", "don't modify", "dont modify",
		"do not update", "don't update", "dont update",
		"do not edit", "don't edit", "dont edit",
		"do not set", "don't set", "dont set",
		"do not switch", "don't switch", "dont switch",
		"do not replace", "don't replace", "dont replace",
		"without changing", "without modifying", "without updating",
		"keep unchanged", "leave unchanged", "keep the same", "preserve",
	} {
		if idx := strings.LastIndex(prefix, marker); idx > best {
			best = idx
		}
	}
	compact := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\n", "",
		"\r", "",
		",", "",
		";", "",
		":", "",
		"\uff0c", "",
		"\uff1b", "",
		"\uff1a", "",
		"\u3001", "",
	).Replace(prefix)
	for _, marker := range []string{
		"\u4e0d\u6539",
		"\u4e0d\u8981\u6539",
		"\u522b\u6539",
		"\u4e0d\u7528\u6539",
		"\u4e0d\u4fee\u6539",
		"\u4e0d\u8981\u4fee\u6539",
		"\u522b\u4fee\u6539",
		"\u4e0d\u66f4\u6539",
		"\u4e0d\u8981\u66f4\u6539",
		"\u4e0d\u8981\u52a8",
		"\u65e0\u9700\u4fee\u6539",
		"\u4fdd\u6301\u4e0d\u53d8",
		"\u4fdd\u6301\u539f\u6837",
		"\u7ef4\u6301\u539f\u6837",
		"\u4fdd\u7559",
	} {
		if idx := strings.LastIndex(compact, marker); idx >= 0 && idx > best {
			best = idx
		}
	}
	return best
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
	prefix := strings.TrimSpace(text[prefixStart:markerStart])
	if prefix == "" {
		return false
	}
	if agentManagementEnglishOrNegationPrefix(prefix) {
		return true
	}
	if containsAnySuffix(prefix, []string{
		"do not", "don't", "dont", "without", "never", "no", "do not make any",
		"do not bind or", "don't bind or", "dont bind or", "without binding or",
		"do not add or", "don't add or", "dont add or", "without adding or",
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
		"\u4e0d\u8981\u6dfb\u52a0\u6216",
		"\u4e0d\u7528\u6dfb\u52a0\u6216",
		"\u65e0\u9700\u6dfb\u52a0\u6216",
		"\u4e0d\u9700\u8981\u6dfb\u52a0\u6216",
		"\u4e0d\u8981\u7ed1\u5b9a\u6216",
		"\u4e0d\u7ed1\u5b9a\u6216",
		"\u4e0d\u7528\u7ed1\u5b9a\u6216",
		"\u65e0\u9700\u7ed1\u5b9a\u6216",
		"\u4e0d\u9700\u8981\u7ed1\u5b9a\u6216",
		"\u4e0d\u8981\u542f\u7528\u6216",
		"\u4e0d\u542f\u7528\u6216",
		"\u4e0d\u7528\u542f\u7528\u6216",
		"\u65e0\u9700\u542f\u7528\u6216",
		"\u4e0d\u9700\u8981\u542f\u7528\u6216",
		"\u4e0d\u8981\u5173\u8054\u6216",
		"\u4e0d\u5173\u8054\u6216",
		"\u4e0d\u7528\u5173\u8054\u6216",
		"\u65e0\u9700\u5173\u8054\u6216",
		"\u4e0d\u9700\u8981\u5173\u8054\u6216",
	}) || agentManagementMutationMarkerHasListNegationPrefix(prefix)
}

func agentManagementEnglishOrNegationPrefix(prefix string) bool {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" || !strings.HasSuffix(prefix, " or") {
		return false
	}
	for _, marker := range []string{
		"do not ",
		"don't ",
		"dont ",
		"without ",
		"never ",
		"no ",
	} {
		if strings.LastIndex(prefix, marker) >= 0 {
			return true
		}
	}
	return false
}

func agentManagementMutationMarkerHasListNegationPrefix(prefix string) bool {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" {
		return false
	}
	for _, marker := range []string{
		"do not", "don't", "dont", "without", "never", "no",
		"\u4e0d\u8981", "\u4e0d\u7528", "\u65e0\u9700", "\u4e0d\u9700\u8981", "\u4e0d\u505a", "\u7981\u6b62", "\u522b", "\u4e0d",
	} {
		idx := strings.LastIndex(prefix, marker)
		if idx < 0 {
			continue
		}
		tail := strings.TrimSpace(prefix[idx+len(marker):])
		if tail == "" {
			continue
		}
		if agentManagementEnglishMutationListTail(tail) || agentManagementChineseMutationListTail(tail) {
			return true
		}
	}
	return false
}

func agentManagementEnglishMutationListTail(tail string) bool {
	tail = strings.NewReplacer(
		",", " ",
		".", " ",
		";", " ",
		":", " ",
		"/", " ",
		"\\", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
	).Replace(strings.ToLower(strings.TrimSpace(tail)))
	if tail == "" {
		return false
	}
	for _, token := range strings.Fields(tail) {
		switch token {
		case "and", "or", "nor", "also", "then", "any", "the", "this", "that",
			"modify", "modifying", "modified",
			"edit", "editing", "edited",
			"change", "changing", "changed",
			"update", "updating", "updated",
			"set", "setting",
			"replace", "replacing", "replaced",
			"switch", "switching", "switched",
			"enable", "enabling", "enabled",
			"disable", "disabling", "disabled",
			"bind", "binding", "bound",
			"unbind", "unbinding", "unbound",
			"create", "creating", "created",
			"add", "adding", "added",
			"delete", "deleting", "deleted",
			"remove", "removing", "removed",
			"save", "saving", "saved",
			"asset", "assets", "resource", "resources", "config", "configuration":
			continue
		default:
			return false
		}
	}
	return true
}

func agentManagementChineseMutationListTail(tail string) bool {
	tail = strings.NewReplacer(
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
	).Replace(strings.TrimSpace(tail))
	if tail == "" {
		return false
	}
	for steps := 0; tail != "" && steps < 128; steps++ {
		matched := false
		for _, piece := range []string{
			"\u4fee\u6539", "\u66f4\u6539", "\u7f16\u8f91", "\u8c03\u6574", "\u53d8\u66f4", "\u66f4\u65b0",
			"\u8bbe\u7f6e", "\u66ff\u6362", "\u5207\u6362", "\u542f\u7528", "\u7981\u7528", "\u505c\u7528",
			"\u7ed1\u5b9a", "\u89e3\u7ed1", "\u5173\u8054", "\u6dfb\u52a0", "\u521b\u5efa", "\u65b0\u5efa",
			"\u65b0\u589e", "\u5220\u9664", "\u79fb\u9664", "\u4fdd\u5b58",
			"\u6216\u8005", "\u4ee5\u53ca", "\u4efb\u4f55", "\u914d\u7f6e", "\u8d44\u6e90", "\u8d44\u4ea7",
			"\u548c", "\u6216", "\u53ca", "\u4e0e", "\u4e5f",
		} {
			if strings.HasPrefix(tail, piece) {
				tail = strings.TrimPrefix(tail, piece)
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return tail == ""
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

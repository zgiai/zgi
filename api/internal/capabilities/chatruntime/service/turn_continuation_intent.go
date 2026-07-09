package service

import "strings"

func isContinuationIntent(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	if isStagedContinuationInstruction(normalized) || isWaitForFutureContinuationInstruction(normalized) {
		return false
	}
	if isCanonicalChineseContinuationIntent(normalized) {
		return true
	}
	runeCount := len([]rune(normalized))
	if runeCount > 24 {
		return isLongContinuationIntent(normalized, runeCount)
	}
	for _, exact := range []string{
		"continue",
		"continue please",
		"go on",
		"keep going",
		"next",
		"next step",
		"proceed",
	} {
		if normalized == exact {
			return true
		}
	}
	return containsAnySubstring(normalized, []string{
		"continue",
		"go on",
		"keep going",
		"next step",
	})
}

func isExplicitContinuationCommand(query string) bool {
	normalized := normalizeConsoleNavigationQuery(query)
	if normalized == "" {
		return false
	}
	if isStagedContinuationInstruction(normalized) || isWaitForFutureContinuationInstruction(normalized) {
		return false
	}
	if isCanonicalChineseContinuationIntent(normalized) {
		return true
	}
	for _, exact := range []string{
		"continue",
		"continue please",
		"continue processing",
		"continue execution",
		"continue the task",
		"go on",
		"keep going",
		"next",
		"next step",
		"proceed",
	} {
		if normalized == exact {
			return true
		}
	}
	return len([]rune(normalized)) <= 64 && hasContinuationPrefix(normalized)
}

func isCanonicalChineseContinuationIntent(normalized string) bool {
	if normalized == "" {
		return false
	}
	exact := map[string]struct{}{
		"\u7ee7\u7eed":                               {},
		"\u7ee7\u7eed\u5427":                         {},
		"\u7ee7\u7eed\u5904\u7406":                   {},
		"\u7ee7\u7eed\u6267\u884c":                   {},
		"\u63a5\u7740":                               {},
		"\u63a5\u7740\u505a":                         {},
		"\u4e0b\u4e00\u6b65":                         {},
		"\u8fdb\u884c\u5904\u7406":                   {},
		"\u90a3\u5c31\u505a":                         {},
		"\u90a3\u5c31\u5904\u7406":                   {},
		"\u90a3\u5c31\u6267\u884c":                   {},
		"\u5c31\u8fd9\u4e48\u505a":                   {},
		"\u6309\u8fd9\u4e2a\u505a":                   {},
		"\u6309\u8fd9\u4e2a\u5904\u7406":             {},
		"\u6309\u65b9\u6848\u505a":                   {},
		"\u6309\u65b9\u6848\u5904\u7406":             {},
		"\u5f00\u59cb\u5904\u7406":                   {},
		"\u5f00\u59cb\u5904\u7406\u5427":             {},
		"\u5f00\u59cb\u6267\u884c":                   {},
		"\u5f00\u59cb\u6267\u884c\u5427":             {},
		"\u53ef\u4ee5\u5f00\u59cb":                   {},
		"\u7ee7\u7eed\u4e0a\u4e00\u6b65":             {},
		"\u7ee7\u7eed\u521a\u624d\u7684\u4efb\u52a1": {},
	}
	if _, ok := exact[normalized]; ok {
		return true
	}
	if len([]rune(normalized)) > 24 {
		return false
	}
	return containsAnySubstring(normalized, []string{
		"\u7ee7\u7eed",
		"\u63a5\u7740",
		"\u4e0b\u4e00\u6b65",
		"\u90a3\u5c31\u505a",
		"\u90a3\u5c31\u5904\u7406",
		"\u90a3\u5c31\u6267\u884c",
		"\u5c31\u8fd9\u4e48\u505a",
		"\u6309\u8fd9\u4e2a\u505a",
		"\u6309\u8fd9\u4e2a\u5904\u7406",
		"\u6309\u65b9\u6848\u505a",
		"\u6309\u65b9\u6848\u5904\u7406",
	})
}

func isWaitForFutureContinuationInstruction(normalized string) bool {
	if normalized == "" {
		return false
	}
	compact := strings.NewReplacer(
		" ", "",
		"\u201c", "",
		"\u201d", "",
		"\"", "",
		"'", "",
		"`", "",
	).Replace(normalized)
	return containsAnySubstring(compact, []string{
		"\u7b49\u5f85\u7ee7\u7eed",
		"\u7b49\u7ee7\u7eed",
		"\u7b49\u5f85\u7528\u6237\u7ee7\u7eed",
		"\u7b49\u7528\u6237\u7ee7\u7eed",
	}) || containsAnySubstring(normalized, []string{
		"wait for continue",
		"wait for the user to continue",
		"wait for user continue",
	})
}

func isStagedContinuationInstruction(normalized string) bool {
	if normalized == "" {
		return false
	}
	compact := strings.NewReplacer(
		" ", "",
		"\u201c", "",
		"\u201d", "",
		"\u2018", "",
		"\u2019", "",
		"\"", "",
		"'", "",
		"`", "",
		"\u300c", "",
		"\u300d", "",
		"\u300e", "",
		"\u300f", "",
		"\u300a", "",
		"\u300b", "",
	).Replace(normalized)
	if containsAnySubstring(compact, []string{
		"\u7b49\u5f85\u6211\u8bf4\u7ee7\u7eed",
		"\u7b49\u6211\u8bf4\u7ee7\u7eed",
		"\u7b49\u6211\u8f93\u5165\u7ee7\u7eed",
		"\u7b49\u6211\u53d1\u7ee7\u7eed",
		"\u6211\u8bf4\u7ee7\u7eed\u540e",
		"\u8bf4\u7ee7\u7eed\u540e",
		"\u5f53\u6211\u8bf4\u7ee7\u7eed",
		"\u6211\u8bf4\u4e86\u7ee7\u7eed\u518d",
		"\u6211\u8bf4\u7ee7\u7eed\u518d",
		"\u7b49\u5f85\u7ee7\u7eed\u6307\u4ee4",
	}) {
		return true
	}
	return containsAnySubstring(normalized, []string{
		"wait for me to say continue",
		"wait until i say continue",
		"after i say continue",
		"when i say continue",
		"once i say continue",
		"wait for my continue",
	})
}

func isLongContinuationIntent(normalized string, runeCount int) bool {
	if normalized == "" {
		return false
	}
	if hasContinuationPrefix(normalized) {
		return true
	}
	if !containsAnySubstring(normalized, []string{
		"\u7ee7\u7eed",
		"\u63a5\u7740",
		"\u4e0b\u4e00\u6b65",
		"continue",
		"go on",
		"keep going",
		"next step",
	}) {
		return false
	}
	return containsAnySubstring(normalized, []string{
		"\u4efb\u52a1\u6807\u8bb0",
		"task marker",
		"task tag",
		"smoke-",
	})
}

func hasContinuationPrefix(normalized string) bool {
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return false
	}
	for _, prefix := range []string{
		"\u7ee7\u7eed",
		"\u63a5\u7740",
		"\u4e0b\u4e00\u6b65",
		"continue",
		"go on",
		"keep going",
		"next step",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func normalizeConsoleNavigationQuery(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(
		"\uff0c", " ",
		"\u3001", " ",
		"\uff1b", " ",
		";", " ",
		"\uff1a", " ",
		":", " ",
		"\uff1f", " ",
		"?", " ",
		"\uff01", " ",
		"!", " ",
		",", " ",
		".", " ",
	)
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

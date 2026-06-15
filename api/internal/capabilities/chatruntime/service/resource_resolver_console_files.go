package service

import (
	"strconv"
	"strings"
)

func resolveConsoleFileIDsFromActionDecision(parts *chatRequestParts, decision AIChatActionDecision) []string {
	refGroups := make([][]PlannerResourceRef, 0, 3)
	if refs := plannerResourceRefsFromActionDecision(decision); len(refs) > 0 {
		refGroups = append(refGroups, refs)
	}
	if refs := plannerResourceRefsFromConsoleFilesQuery(parts); len(refs) > 0 {
		refGroups = append(refGroups, refs)
	}
	refGroups = append(refGroups, []PlannerResourceRef{{Type: resourceTypeFile}})

	for _, refs := range refGroups {
		result := resolveChatResourceRefs(parts, refs)
		if allResourceRefsResolved(result.Results) {
			return result.FileIDs
		}
	}
	return nil
}

func plannerResourceRefsFromActionDecision(decision AIChatActionDecision) []PlannerResourceRef {
	if len(decision.ResourceRefs) == 0 {
		return nil
	}
	refs := make([]PlannerResourceRef, 0, len(decision.ResourceRefs))
	for _, ref := range decision.ResourceRefs {
		ref.Metadata = copyStringAnyMap(ref.Metadata)
		refs = append(refs, PlannerResourceRef(ref))
	}
	return refs
}

func resolveConsoleFileIDsFromQuery(parts *chatRequestParts) []string {
	refs := plannerResourceRefsFromConsoleFilesQuery(parts)
	if len(refs) == 0 {
		return nil
	}
	result := resolveChatResourceRefs(parts, refs)
	if !allResourceRefsResolved(result.Results) {
		return nil
	}
	return result.FileIDs
}

func plannerResourceRefsFromConsoleFilesQuery(parts *chatRequestParts) []PlannerResourceRef {
	if parts == nil {
		return nil
	}
	query := strings.TrimSpace(parts.Query)
	if consoleFilesSelectedReferenceFromQuery(parts.Query) {
		return []PlannerResourceRef{{
			Type:     resourceTypeFile,
			Selector: query,
			Selected: true,
			Scope:    "selected",
		}}
	}
	ordinal, last, ok := consoleFilesOrdinalFromQuery(parts.Query)
	if ok {
		ref := PlannerResourceRef{Type: resourceTypeFile, Selector: query}
		if last {
			ref.OrdinalText = "last"
		} else {
			ref.Ordinal = ordinal
		}
		if fileType := consoleFilesFormatFromQuery(parts.Query); fileType != "" {
			if fileType == "pdf" {
				ref.Extension = "pdf"
			} else {
				ref.FileType = fileType
			}
		}
		return []PlannerResourceRef{ref}
	}
	return plannerResourceRefsFromNamedVisibleFiles(parts)
}

func consoleFilesSelectedReferenceFromQuery(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	for _, token := range []string{
		"selected file",
		"selected files",
		"current selected file",
		"currently selected file",
		"the selected",
		"\u5f53\u524d\u9009\u4e2d\u6587\u4ef6",
		"\u5f53\u524d\u9009\u4e2d\u7684\u6587\u4ef6",
		"\u9009\u4e2d\u6587\u4ef6",
		"\u9009\u4e2d\u7684\u6587\u4ef6",
		"\u88ab\u9009\u4e2d\u7684\u6587\u4ef6",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func plannerResourceRefsFromNamedVisibleFiles(parts *chatRequestParts) []PlannerResourceRef {
	if parts == nil {
		return nil
	}
	collector := newUniqueStringCollector()
	for _, context := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, id := range collectNamedVisibleFileIDs(parts.Query, context) {
			collector.add(id)
		}
	}
	ids := collector.values()
	if len(ids) == 0 {
		return nil
	}
	refs := make([]PlannerResourceRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, PlannerResourceRef{
			Type:   resourceTypeFile,
			FileID: id,
		})
	}
	return refs
}

func allResourceRefsResolved(results []ResourceResolution) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if result.Status != ResourceResolutionStatusResolved {
			return false
		}
	}
	return true
}

func consoleFilesOrdinalFromQuery(query string) (int, bool, bool) {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return 0, false, false
	}
	if strings.Contains(text, "last") || strings.Contains(text, "\u6700\u540e") {
		return 0, true, true
	}
	if ordinal, ok := chineseOrdinalFromText(text); ok {
		return ordinal, false, true
	}
	if ordinal, ok := englishOrdinalFromText(text); ok {
		return ordinal, false, true
	}
	return 0, false, false
}

func chineseOrdinalFromText(text string) (int, bool) {
	start := strings.Index(text, "\u7b2c")
	if start < 0 {
		return 0, false
	}
	after := text[start+len("\u7b2c"):]
	end := strings.Index(after, "\u4e2a")
	if end < 0 {
		end = strings.Index(after, "\u4efd")
	}
	if end < 0 {
		end = strings.Index(after, "\u5f20")
	}
	if end < 0 {
		return 0, false
	}
	return parseChineseOrdinalToken(after[:end])
}

func parseChineseOrdinalToken(token string) (int, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, false
	}
	if ordinal, err := strconv.Atoi(token); err == nil && ordinal > 0 {
		return ordinal, true
	}
	digit := map[rune]int{
		'\u96f6': 0,
		'\u4e00': 1,
		'\u4e8c': 2,
		'\u4e24': 2,
		'\u4e09': 3,
		'\u56db': 4,
		'\u4e94': 5,
		'\u516d': 6,
		'\u4e03': 7,
		'\u516b': 8,
		'\u4e5d': 9,
	}
	runes := []rune(token)
	if len(runes) == 1 {
		value, ok := digit[runes[0]]
		return value, ok && value > 0
	}
	if len(runes) == 2 && runes[0] == '\u5341' {
		value, ok := digit[runes[1]]
		return 10 + value, ok
	}
	if len(runes) == 2 && runes[1] == '\u5341' {
		value, ok := digit[runes[0]]
		return value * 10, ok && value > 0
	}
	if len(runes) == 3 && runes[1] == '\u5341' {
		tens, tensOK := digit[runes[0]]
		ones, onesOK := digit[runes[2]]
		if tensOK && onesOK && tens > 0 {
			return tens*10 + ones, true
		}
	}
	return 0, false
}

func englishOrdinalFromText(text string) (int, bool) {
	for _, token := range strings.Fields(text) {
		token = strings.Trim(token, ".,;:!?()[]{}")
		if token == "" {
			continue
		}
		if ordinal, ok := parseEnglishOrdinalToken(token); ok {
			return ordinal, true
		}
	}
	return 0, false
}

func parseEnglishOrdinalToken(token string) (int, bool) {
	token = strings.ToLower(strings.TrimSpace(token))
	switch token {
	case "first":
		return 1, true
	case "second":
		return 2, true
	case "third":
		return 3, true
	case "fourth":
		return 4, true
	case "fifth":
		return 5, true
	}
	for _, suffix := range []string{"st", "nd", "rd", "th"} {
		if strings.HasSuffix(token, suffix) {
			value := strings.TrimSuffix(token, suffix)
			ordinal, err := strconv.Atoi(value)
			return ordinal, err == nil && ordinal > 0
		}
	}
	return 0, false
}

func consoleFilesFormatFromQuery(query string) string {
	text := strings.ToLower(strings.TrimSpace(query))
	switch {
	case strings.Contains(text, "excel") ||
		strings.Contains(text, "xlsx") ||
		strings.Contains(text, "xls") ||
		strings.Contains(text, "\u8868\u683c") ||
		strings.Contains(text, "\u7535\u5b50\u8868\u683c") ||
		strings.Contains(text, "\u5de5\u4f5c\u8868") ||
		strings.Contains(text, "\u5de5\u4f5c\u7c3f"):
		return "excel"
	case strings.Contains(text, "pdf"):
		return "pdf"
	default:
		return ""
	}
}

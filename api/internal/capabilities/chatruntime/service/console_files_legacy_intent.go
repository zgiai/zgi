package service

import (
	"regexp"
	"strings"
)

var (
	consoleFilesReadIntentPattern   = regexp.MustCompile(`(?i)\b(read|preview|summari[sz]e|summary|analy[sz]e|analysis|inspect|show|translate|translation|abstract|digest|extract)\b`)
	consoleFilesDeleteIntentPattern = regexp.MustCompile(`(?i)\b(delete|remove|trash|discard)\b`)
)

func isFileReadIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if isConsoleFilesPageSummaryQuestion(text) {
		return false
	}
	if consoleFilesReadIntentPattern.MatchString(text) {
		return true
	}
	for _, token := range []string{
		"\u8bfb\u53d6",
		"\u8bfb\u4e00\u4e0b",
		"\u8bfb\u4e0b",
		"\u603b\u7ed3",
		"\u6458\u8981",
		"\u7ffb\u8bd1",
		"\u6982\u62ec",
		"\u63d0\u70bc",
		"\u63d0\u53d6",
		"\u89e3\u91ca",
		"\u5206\u6790",
		"\u67e5\u770b\u5185\u5bb9",
		"\u770b\u770b\u5185\u5bb9",
		"\u770b\u4e00\u4e0b\u5185\u5bb9",
		"\u6587\u4ef6\u5185\u5bb9",
		"\u9884\u89c8",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	if strings.Contains(text, "\u8bfb") && containsAnySubstring(text, []string{
		"\u7b2c",
		"\u6700\u540e",
		"\u5f53\u524d",
		"\u9009\u4e2d",
		"\u8fd9\u4e2a",
		"\u5185\u5bb9",
		"pdf",
		"excel",
		"\u8868\u683c",
		"\u6587\u6863",
	}) {
		return true
	}
	return false
}

func isConsoleFilesPageSummaryQuestion(text string) bool {
	if text == "" {
		return false
	}
	if !containsAnySubstring(text, []string{
		"how many",
		"count",
		"total",
		"table total",
		"file count",
		"files count",
		"items",
		"file list",
		"list files",
		"\u5171\u591a\u5c11",
		"\u5171\u51e0",
		"\u6709\u51e0",
		"\u591a\u5c11\u4e2a",
		"\u603b\u6570",
		"\u603b\u5171",
		"\u6587\u4ef6\u6570",
		"\u6587\u4ef6\u5217\u8868",
		"\u54ea\u4e9b\u6587\u4ef6",
		"\u6709\u54ea\u4e9b\u6587\u4ef6",
	}) {
		return false
	}
	return !containsAnySubstring(text, []string{
		"content",
		"contents",
		"inside",
		"body",
		"read file",
		"file content",
		"\u5185\u5bb9",
		"\u6587\u4ef6\u5185\u5bb9",
		"\u8bfb\u53d6",
		"\u7ffb\u8bd1",
		"\u6458\u8981",
		"\u603b\u7ed3",
		"\u5206\u6790",
	})
}

func isFileDeleteIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if hasFileDeleteNegation(text) {
		return false
	}
	if consoleFilesDeleteIntentPattern.MatchString(text) {
		return true
	}
	for _, token := range []string{
		"\u5220\u9664",
		"\u5220\u6389",
		"\u5220\u4e86",
		"\u79fb\u9664",
		"\u6e05\u7406",
	} {
		if strings.Contains(text, token) {
			return true
		}
	}
	return false
}

func isManagedFileCreateIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" {
		return false
	}
	if hasManagedFileCreateNegation(text) {
		return false
	}
	createTerms := []string{
		"create", "generate", "save", "upload", "export", "write",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u4fdd\u5b58", "\u4e0a\u4f20", "\u5bfc\u51fa", "\u5199\u5165",
	}
	targetTerms := []string{
		"file management", "files page", "current files page", "managed file", "workspace file",
		"\u6587\u4ef6\u7ba1\u7406", "\u6587\u4ef6\u9875", "\u5f53\u524d\u6587\u4ef6\u9875", "\u6587\u4ef6\u5217\u8868", "\u5de5\u4f5c\u533a\u6587\u4ef6",
	}
	if !containsAnySubstring(text, createTerms) {
		return false
	}
	if containsAnySubstring(text, targetTerms) {
		return true
	}
	fileTerms := []string{"file", "\u6587\u4ef6"}
	managementTerms := []string{"management", "manage", "\u7ba1\u7406", "\u7ba1\u7406\u9875", "\u7ba1\u7406\u91cc", "\u7ba1\u7406\u91cc\u9762"}
	return containsAnySubstring(text, fileTerms) && containsAnySubstring(text, managementTerms)
}

func isTemporaryFileGenerateIntent(query string) bool {
	text := strings.ToLower(strings.TrimSpace(query))
	if text == "" ||
		isManagedFileCreateIntent(query) ||
		isFileReadIntent(query) ||
		isFileDeleteIntent(query) {
		return false
	}
	if hasTemporaryFileGenerateNegation(text) {
		return false
	}
	createTerms := []string{
		"create", "generate", "write", "export", "make", "produce",
		"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u5199", "\u5199\u4e00\u4e2a", "\u5bfc\u51fa", "\u505a\u4e00\u4e2a",
	}
	artifactTerms := []string{
		"file", ".txt", ".md", ".markdown", ".json", ".csv", ".tsv", ".xlsx", ".docx", ".pptx", ".pdf", ".html", ".svg",
		"txt", "markdown", "json", "csv", "tsv", "xlsx", "docx", "pptx", "pdf", "html", "svg",
		" txt", " md", " json", " csv", " xlsx", " docx", " pptx", " pdf", " html", " svg",
		"\u6587\u4ef6", "\u4e34\u65f6\u6587\u4ef6", "\u6587\u6863", "\u8868\u683c", "\u56fe\u7247",
	}
	return containsAnySubstring(text, createTerms) && containsAnySubstring(text, artifactTerms)
}

func hasTemporaryFileGenerateNegation(text string) bool {
	negativePhrases := []string{
		"do not create", "don't create", "dont create", "not create", "without creating",
		"do not generate", "don't generate", "dont generate", "not generate", "without generating",
		"do not write", "don't write", "dont write", "not write", "without writing",
		"do not export", "don't export", "dont export", "not export", "without exporting",
		"do not make", "don't make", "dont make", "not make", "without making",
		"do not produce", "don't produce", "dont produce", "not produce", "without producing",
		"read only", "answer only",
		"\u4e0d\u8981\u521b\u5efa", "\u4e0d\u7528\u521b\u5efa", "\u4e0d\u521b\u5efa", "\u65e0\u9700\u521b\u5efa", "\u522b\u521b\u5efa",
		"\u4e0d\u8981\u65b0\u5efa", "\u4e0d\u7528\u65b0\u5efa", "\u4e0d\u65b0\u5efa", "\u65e0\u9700\u65b0\u5efa", "\u522b\u65b0\u5efa",
		"\u4e0d\u8981\u751f\u6210", "\u4e0d\u7528\u751f\u6210", "\u4e0d\u751f\u6210", "\u65e0\u9700\u751f\u6210", "\u522b\u751f\u6210",
		"\u4e0d\u8981\u5199", "\u4e0d\u7528\u5199", "\u4e0d\u5199", "\u65e0\u9700\u5199", "\u522b\u5199",
		"\u4e0d\u8981\u5bfc\u51fa", "\u4e0d\u7528\u5bfc\u51fa", "\u4e0d\u5bfc\u51fa", "\u65e0\u9700\u5bfc\u51fa", "\u522b\u5bfc\u51fa",
		"\u4ec5\u56de\u7b54", "\u53ea\u56de\u7b54", "\u4ec5\u8bfb", "\u53ea\u8bfb",
	}
	return containsAnySubstring(text, negativePhrases)
}

func hasManagedFileCreateNegation(text string) bool {
	negativePhrases := []string{
		"do not create", "don't create", "dont create", "not create", "without creating",
		"do not generate", "don't generate", "dont generate", "not generate", "without generating",
		"do not write", "don't write", "dont write", "not write", "without writing",
		"do not export", "don't export", "dont export", "not export", "without exporting",
		"do not save", "don't save", "dont save", "not save", "without saving", "temporary only",
		"do not add", "don't add", "dont add", "do not upload", "don't upload", "dont upload",
		"read only", "answer only",
		"\u4e0d\u8981\u521b\u5efa", "\u4e0d\u7528\u521b\u5efa", "\u4e0d\u521b\u5efa", "\u65e0\u9700\u521b\u5efa", "\u522b\u521b\u5efa",
		"\u4e0d\u8981\u65b0\u5efa", "\u4e0d\u7528\u65b0\u5efa", "\u4e0d\u65b0\u5efa", "\u65e0\u9700\u65b0\u5efa", "\u522b\u65b0\u5efa",
		"\u4e0d\u8981\u751f\u6210", "\u4e0d\u7528\u751f\u6210", "\u4e0d\u751f\u6210", "\u65e0\u9700\u751f\u6210", "\u522b\u751f\u6210",
		"\u4e0d\u8981\u5199\u5165", "\u4e0d\u7528\u5199\u5165", "\u4e0d\u5199\u5165", "\u65e0\u9700\u5199\u5165", "\u522b\u5199\u5165",
		"\u4e0d\u8981\u5bfc\u51fa", "\u4e0d\u7528\u5bfc\u51fa", "\u4e0d\u5bfc\u51fa", "\u65e0\u9700\u5bfc\u51fa", "\u522b\u5bfc\u51fa",
		"\u4e0d\u8981\u4fdd\u5b58", "\u4e0d\u7528\u4fdd\u5b58", "\u4e0d\u4fdd\u5b58", "\u65e0\u9700\u4fdd\u5b58", "\u522b\u4fdd\u5b58",
		"\u4e0d\u8981\u5b58", "\u4e0d\u7528\u5b58", "\u522b\u5b58",
		"\u4e0d\u8981\u6dfb\u52a0", "\u4e0d\u7528\u6dfb\u52a0", "\u522b\u6dfb\u52a0",
		"\u4e0d\u8981\u4e0a\u4f20", "\u4e0d\u7528\u4e0a\u4f20", "\u522b\u4e0a\u4f20",
		"\u4ec5\u4e34\u65f6", "\u53ea\u751f\u6210\u4e34\u65f6", "\u4e34\u65f6\u5373\u53ef",
	}
	return containsAnySubstring(text, negativePhrases)
}

func hasFileMutationNegation(text string) bool {
	if text == "" || !containsAnySubstring(text, []string{"file", "\u6587\u4ef6", "asset", "resource", "\u8d44\u4ea7", "\u8d44\u6e90"}) {
		return false
	}
	englishNegative := []string{
		"do not create", "don't create", "dont create", "not create",
		"do not save", "don't save", "dont save", "not save",
		"do not delete", "don't delete", "dont delete", "not delete",
		"do not remove", "don't remove", "dont remove", "not remove",
	}
	if containsAnySubstring(text, englishNegative) {
		return true
	}
	if containsAnySubstring(text, []string{
		"\u522b\u521b\u5efa", "\u522b\u65b0\u5efa", "\u522b\u751f\u6210", "\u522b\u4fdd\u5b58", "\u522b\u5b58", "\u522b\u4e0a\u4f20", "\u522b\u6dfb\u52a0",
		"\u522b\u5220\u9664", "\u522b\u5220\u6389", "\u522b\u79fb\u9664", "\u522b\u6e05\u7406",
	}) {
		return true
	}
	return containsAnySubstring(text, []string{"\u4e0d\u8981", "\u4e0d\u7528", "\u65e0\u9700"}) &&
		containsAnySubstring(text, []string{
			"\u521b\u5efa", "\u65b0\u5efa", "\u751f\u6210", "\u4fdd\u5b58", "\u5b58", "\u4e0a\u4f20", "\u6dfb\u52a0",
			"\u5220\u9664", "\u5220\u6389", "\u5220\u4e86", "\u79fb\u9664", "\u6e05\u7406",
		})
}

func hasFileDeleteNegation(text string) bool {
	if text == "" || !containsAnySubstring(text, []string{"file", "\u6587\u4ef6", "asset", "resource", "\u8d44\u4ea7", "\u8d44\u6e90"}) {
		return false
	}
	if containsAnySubstring(text, []string{
		"do not delete", "don't delete", "dont delete", "not delete",
		"do not remove", "don't remove", "dont remove", "not remove",
		"do not create or delete", "don't create or delete", "dont create or delete",
		"\u522b\u5220\u9664", "\u522b\u5220\u6389", "\u522b\u79fb\u9664", "\u522b\u6e05\u7406",
		"\u4e0d\u8981\u5220\u9664", "\u4e0d\u8981\u5220\u6389", "\u4e0d\u8981\u79fb\u9664", "\u4e0d\u8981\u6e05\u7406",
		"\u4e0d\u7528\u5220\u9664", "\u4e0d\u7528\u5220\u6389", "\u4e0d\u7528\u79fb\u9664", "\u4e0d\u7528\u6e05\u7406",
		"\u65e0\u9700\u5220\u9664", "\u65e0\u9700\u5220\u6389", "\u65e0\u9700\u79fb\u9664", "\u65e0\u9700\u6e05\u7406",
		"\u4e0d\u8981\u521b\u5efa\u6216\u5220\u9664", "\u4e0d\u8981\u521b\u5efa\u548c\u5220\u9664", "\u4e0d\u8981\u521b\u5efa\u3001\u5220\u9664",
		"\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u3001\u5220\u9664", "\u4e0d\u8981\u521b\u5efa\u3001\u4fdd\u5b58\u6216\u5220\u9664",
		"\u4e0d\u7528\u521b\u5efa\u6216\u5220\u9664", "\u65e0\u9700\u521b\u5efa\u6216\u5220\u9664",
	}) {
		return true
	}
	return hasNegatedFileDeleteClause(text)
}

func hasNegatedFileDeleteClause(text string) bool {
	for _, clause := range strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '.', ',', ';', ':', '\uff0c', '\u3002', '\uff1b', '\uff1a':
			return true
		default:
			return false
		}
	}) {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		if !containsAnySubstring(clause, []string{"file", "\u6587\u4ef6", "asset", "resource", "\u8d44\u4ea7", "\u8d44\u6e90"}) {
			continue
		}
		negativeAt := firstSubstringIndex(clause, []string{"do not", "don't", "dont", "not ", "without", "never", "\u4e0d\u8981", "\u4e0d\u7528", "\u65e0\u9700", "\u522b"})
		if negativeAt < 0 {
			continue
		}
		if containsAnySubstring(clause[negativeAt:], []string{"delete", "remove", "trash", "discard", "\u5220\u9664", "\u5220\u6389", "\u5220\u4e86", "\u79fb\u9664", "\u6e05\u7406"}) {
			return true
		}
	}
	return false
}

func firstSubstringIndex(text string, needles []string) int {
	first := -1
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		idx := strings.Index(text, needle)
		if idx >= 0 && (first < 0 || idx < first) {
			first = idx
		}
	}
	return first
}

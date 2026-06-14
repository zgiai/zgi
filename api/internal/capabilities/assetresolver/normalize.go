package assetresolver

import (
	"strings"
	"unicode"
)

func normalizeSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func stripCandidateExtension(value string) string {
	value = strings.TrimSpace(value)
	extension := fileNameExtension(value)
	if extension == "" {
		return value
	}
	return strings.TrimSuffix(value, "."+extension)
}

func extensionFromCandidate(candidate Candidate) string {
	if extension := normalizedExtension(candidate.Extension); extension != "" {
		return extension
	}
	for _, value := range []string{candidate.Title, candidate.Name} {
		if extension := fileNameExtension(value); extension != "" {
			return extension
		}
	}
	return ""
}

func fileNameExtension(value string) string {
	value = strings.TrimSpace(value)
	dot := strings.LastIndex(value, ".")
	if dot < 0 || dot == len(value)-1 {
		return ""
	}
	return normalizedExtension(value[dot+1:])
}

func inferFileType(candidate Candidate) string {
	extension := extensionFromCandidate(candidate)
	switch {
	case extension == "pdf":
		return "pdf"
	case containsString(fileTypeExtensions("excel"), extension):
		return "excel"
	case containsString(fileTypeExtensions("image"), extension):
		return "image"
	case strings.HasPrefix(normalizedMimeType(candidate.MimeType), "image/"):
		return "image"
	default:
		return ""
	}
}

func normalizedExtension(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func normalizedExtensions(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, normalizedExtension(value))
	}
	return out
}

func normalizedMimeType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedMimeTypes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, normalizedMimeType(value))
	}
	return out
}

func normalizeAssetKind(value string) string {
	return normalizeToken(value)
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedFileType(value string) string {
	token := normalizeToken(value)
	switch token {
	case "xls", "xlsx", "xlsm", "xlsb", "excel", "spreadsheet", "sheet", "\u8868\u683c", "\u7535\u5b50\u8868\u683c", "\u5de5\u4f5c\u8868", "\u5de5\u4f5c\u7c3f":
		return "excel"
	case "pdf", "pdf\u6587\u4ef6":
		return "pdf"
	case "csv":
		return "csv"
	case "image", "img", "picture", "photo", "\u56fe\u7247", "\u56fe\u50cf", "\u7167\u7247":
		return "image"
	case "document", "doc", "docx", "txt", "markdown", "md", "\u6587\u6863":
		return "document"
	default:
		return token
	}
}

func fileTypesFromReferenceText(value string) []string {
	text := normalizeToken(value)
	if text == "" {
		return nil
	}
	collector := newUniqueStringCollector()
	if strings.Contains(text, "excel") ||
		strings.Contains(text, "xlsx") ||
		strings.Contains(text, "xls") ||
		strings.Contains(text, "\u7535\u5b50\u8868\u683c") ||
		strings.Contains(text, "\u5de5\u4f5c\u8868") ||
		strings.Contains(text, "\u5de5\u4f5c\u7c3f") ||
		strings.Contains(text, "\u8868\u683c") {
		collector.add("excel")
	}
	if strings.Contains(text, "pdf") {
		collector.add("pdf")
	}
	if strings.Contains(text, "csv") {
		collector.add("csv")
	}
	if strings.Contains(text, "image") ||
		strings.Contains(text, "picture") ||
		strings.Contains(text, "photo") ||
		strings.Contains(text, "\u56fe\u7247") ||
		strings.Contains(text, "\u56fe\u50cf") ||
		strings.Contains(text, "\u7167\u7247") {
		collector.add("image")
	}
	if strings.Contains(text, "document") ||
		strings.Contains(text, "docx") ||
		strings.Contains(text, "\u6587\u6863") {
		collector.add("document")
	}
	return collector.values()
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func isSelectedScope(value string) bool {
	scope := normalizeKey(value)
	return scope == "selected" || scope == "selectedfile" || scope == "selectedfiles"
}

func compactStrings(values []string) []string {
	collector := newUniqueStringCollector()
	for _, value := range values {
		collector.add(value)
	}
	return collector.values()
}

func limitCandidates(candidates []Candidate, limit int) []Candidate {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]Candidate, len(candidates))
	copy(out, candidates)
	for i := range out {
		out[i].Metadata = nil
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

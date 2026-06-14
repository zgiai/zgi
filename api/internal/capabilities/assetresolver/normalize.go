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

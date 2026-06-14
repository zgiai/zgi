package assetresolver

import "strings"

func findCandidateByID(candidates []Candidate, id string) (Candidate, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Candidate{}, false
	}
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func selectedCandidates(candidates []Candidate) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Selected {
			out = append(out, candidate)
		}
	}
	return out
}

func visibleCandidates(candidates []Candidate) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Visible {
			out = append(out, candidate)
		}
	}
	return out
}

func filterCandidates(candidates []Candidate, criteria criteria) []Candidate {
	out := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidateMatches(candidate, criteria) {
			out = append(out, candidate)
		}
	}
	return out
}

func candidateMatches(candidate Candidate, criteria criteria) bool {
	if len(criteria.extensions) > 0 && !candidateMatchesExtension(candidate, criteria.extensions) {
		return false
	}
	if len(criteria.mimeTypes) > 0 && !candidateMatchesMimeType(candidate, criteria.mimeTypes) {
		return false
	}
	if len(criteria.fileTypes) > 0 && !candidateMatchesAnyFileType(candidate, criteria.fileTypes) {
		return false
	}
	if criteria.titleContains != "" && !candidateTextContains(candidate, criteria.titleContains) {
		return false
	}
	if criteria.nameContains != "" && !candidateTextContains(candidate, criteria.nameContains) {
		return false
	}
	if criteria.fuzzyName != "" && !candidateFuzzyNameMatches(candidate, criteria.fuzzyName) {
		return false
	}
	return true
}

func candidateMatchesExtension(candidate Candidate, extensions []string) bool {
	candidateExtension := extensionFromCandidate(candidate)
	for _, extension := range extensions {
		if extension != "" && candidateExtension == extension {
			return true
		}
	}
	return false
}

func candidateMatchesMimeType(candidate Candidate, mimeTypes []string) bool {
	candidateMime := normalizedMimeType(candidate.MimeType)
	if candidateMime == "" {
		return false
	}
	for _, mimeType := range mimeTypes {
		if mimeType == "" {
			continue
		}
		if candidateMime == mimeType || strings.HasPrefix(candidateMime, strings.TrimSuffix(mimeType, "/*")+"/") {
			return true
		}
	}
	return false
}

func candidateMatchesAnyFileType(candidate Candidate, fileTypes []string) bool {
	for _, fileType := range fileTypes {
		if fileType != "" && candidateMatchesFileType(candidate, fileType) {
			return true
		}
	}
	return false
}

func candidateMatchesFileType(candidate Candidate, fileType string) bool {
	fileType = normalizedFileType(fileType)
	if fileType == "" {
		return true
	}
	if normalizedFileType(candidate.FileType) == fileType {
		return true
	}
	if candidateMatchesExtension(candidate, fileTypeExtensions(fileType)) {
		return true
	}
	candidateMime := normalizedMimeType(candidate.MimeType)
	switch fileType {
	case "image":
		return strings.HasPrefix(candidateMime, "image/")
	case "document":
		return candidateMime != "" && !strings.HasPrefix(candidateMime, "image/")
	default:
		return false
	}
}

func fileTypeExtensions(fileType string) []string {
	switch normalizeToken(fileType) {
	case "pdf":
		return []string{"pdf"}
	case "excel", "spreadsheet":
		return []string{"xls", "xlsx", "xlsm", "xlsb"}
	case "csv":
		return []string{"csv"}
	case "image":
		return []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"}
	case "document":
		return []string{"pdf", "doc", "docx", "txt", "md", "rtf"}
	default:
		extension := normalizedExtension(fileType)
		if extension == "" {
			return nil
		}
		return []string{extension}
	}
}

func candidateTextContains(candidate Candidate, query string) bool {
	needle := normalizeSearchText(query)
	if needle == "" {
		return true
	}
	for _, value := range candidateSearchTexts(candidate) {
		if strings.Contains(normalizeSearchText(value), needle) {
			return true
		}
	}
	return false
}

func candidateFuzzyNameMatches(candidate Candidate, query string) bool {
	needle := normalizeSearchText(query)
	if needle == "" {
		return true
	}
	needleBase := normalizeSearchText(stripCandidateExtension(query))
	for _, value := range candidateSearchTexts(candidate) {
		haystack := normalizeSearchText(value)
		haystackBase := normalizeSearchText(stripCandidateExtension(value))
		if haystack == needle || haystackBase == needleBase {
			return true
		}
		if strings.Contains(haystack, needle) || strings.Contains(haystackBase, needleBase) {
			return true
		}
		if allSearchTokensContained(haystack, needle) {
			return true
		}
	}
	return false
}

func candidateSearchTexts(candidate Candidate) []string {
	values := []string{candidate.Title, candidate.Name}
	if candidate.Title != "" && candidate.Extension != "" && !strings.HasSuffix(strings.ToLower(candidate.Title), "."+candidate.Extension) {
		values = append(values, candidate.Title+"."+candidate.Extension)
	}
	if candidate.Name != "" && candidate.Extension != "" && !strings.HasSuffix(strings.ToLower(candidate.Name), "."+candidate.Extension) {
		values = append(values, candidate.Name+"."+candidate.Extension)
	}
	return values
}

func allSearchTokensContained(haystack string, needle string) bool {
	tokens := strings.Fields(needle)
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

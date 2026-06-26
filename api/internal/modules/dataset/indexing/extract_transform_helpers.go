package indexing

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

var (
	datasetChunkBarcodePlaceholderRE = regexp.MustCompile(`(?i)^\s*\[\s*(barcode|bar\s*code|qr(?:\s*code)?)\s*\]\s*$`)
	datasetChunkFigurePlaceholderRE  = regexp.MustCompile(`(?i)^\s*\[\s*(figure|fig|image|picture)\s*\]\s*$`)
	datasetChunkPageCounterZHRE      = regexp.MustCompile(`^\s*\x{7b2c}\s*\d+\s*\x{9875}\s*\x{5171}\s*\d+\s*\x{9875}\s*$`)
	datasetChunkPageCounterENRE      = regexp.MustCompile(`(?i)^\s*page\s+\d+\s*(of|/)\s*\d+\s*$`)
	datasetChunkBlankDateZHRE        = regexp.MustCompile(`^\s*\x{65e5}\x{671f}\s*[:\x{ff1a}]?\s*\x{5e74}\s*\x{6708}\s*\x{65e5}\s*$`)
	datasetChunkLongNumberRE         = regexp.MustCompile(`^\s*\d{8,}\s*$`)
	datasetChunkStandaloneNumberRE   = regexp.MustCompile(`^\s*\d{1,3}\s*$`)
	datasetChunkQuotedNumberRE       = regexp.MustCompile(`^\s*['\x{2019}]\d{1,6}\s*$`)
	datasetChunkSingleCJKRE          = regexp.MustCompile(`^\s*\p{Han}\s*$`)
	datasetChunkRepeatedCodeRE       = regexp.MustCompile(`(?i)^[a-z]{2,}[a-z0-9]*[-_][a-z0-9][a-z0-9_-]{4,}$`)
)

const (
	datasetChunkContractNumberLabel = "\u5408\u540c\u7f16\u53f7"
	datasetChunkCompanySuffix       = "\u6709\u9650\u516c\u53f8"
)

type datasetChunkFilterStats struct {
	normalizedCounts          map[string]int
	hasSequentialPageCounters bool
}

func sortedExtractElements(output *dto.ExtractOutput) []dto.ExtractElement {
	if output == nil || len(output.Elements) == 0 {
		return nil
	}

	elements := append([]dto.ExtractElement(nil), output.Elements...)
	sort.SliceStable(elements, func(i, j int) bool {
		return elements[i].Ordinal < elements[j].Ordinal
	})
	return elements
}

func isParagraphTextElement(elementType string) bool {
	switch strings.ToLower(strings.TrimSpace(elementType)) {
	case "", "text", "paragraph", "heading", "title", "list", "caption", "footer", "header":
		return true
	default:
		return false
	}
}

func isStandaloneElement(elementType string) bool {
	switch strings.ToLower(strings.TrimSpace(elementType)) {
	case "table", "figure", "image", "formula", "equation":
		return true
	default:
		return !isParagraphTextElement(elementType)
	}
}

func bboxFromElements(elements []dto.ExtractElement) *dto.ExtractBoundingBox {
	for _, element := range elements {
		if element.BBox != nil {
			return element.BBox
		}
	}
	return nil
}

func metadataForElements(output *dto.ExtractOutput, elements []dto.ExtractElement) map[string]any {
	metadata := make(map[string]any)
	if output != nil {
		for key, value := range output.Metadata {
			metadata[key] = value
		}
		if output.Source != "" {
			metadata["provider"] = output.Source
		}
	}

	if len(elements) == 0 {
		return metadata
	}

	first := elements[0]
	for key, value := range first.Metadata {
		metadata[key] = value
	}
	if _, exists := metadata["page"]; !exists {
		metadata["page"] = first.Page
	}
	if _, exists := metadata["source"]; !exists && output != nil && output.Source != "" {
		metadata["source"] = output.Source
	}
	if first.Type != "" {
		metadata["element_type"] = first.Type
	}
	if first.Subtype != "" {
		metadata["element_subtype"] = first.Subtype
	}
	if len(elements) > 1 {
		metadata["element_count"] = len(elements)
	}
	setQualityIssueMetadata(metadata, collectQualityIssuesFromElements(elements))

	return metadata
}

func collectQualityIssuesFromElements(elements []dto.ExtractElement) []any {
	issues := make([]any, 0)
	seen := make(map[string]struct{})
	for _, element := range elements {
		issues = appendQualityIssues(issues, seen, element.Metadata)
	}
	return issues
}

func appendQualityIssues(issues []any, seen map[string]struct{}, metadata map[string]any) []any {
	if metadata == nil {
		return issues
	}
	switch value := metadata["quality_issues"].(type) {
	case []any:
		for _, issue := range value {
			issues = appendUniqueQualityIssue(issues, seen, issue)
		}
	case []map[string]any:
		for _, issue := range value {
			issues = appendUniqueQualityIssue(issues, seen, issue)
		}
	}
	return issues
}

func appendUniqueQualityIssue(issues []any, seen map[string]struct{}, issue any) []any {
	id := qualityIssueID(issue)
	if id != "" {
		if _, exists := seen[id]; exists {
			return issues
		}
		seen[id] = struct{}{}
	}
	return append(issues, issue)
}

func qualityIssueID(issue any) string {
	typed, ok := issue.(map[string]any)
	if !ok {
		return ""
	}
	id, _ := typed["id"].(string)
	return strings.TrimSpace(id)
}

func setQualityIssueMetadata(metadata map[string]any, issues []any) {
	if len(issues) == 0 {
		return
	}
	metadata["quality_issues"] = issues
	metadata["quality_issue_count"] = len(issues)
	metadata["has_quality_issues"] = true
}

func addChunkMetadata(chunks []dto.TransformedChunk) {
	totalChunks := len(chunks)
	for i := range chunks {
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]any)
		}
		chunks[i].Metadata["chunk_index"] = i
		chunks[i].Metadata["total_chunks"] = totalChunks
		if _, ok := chunks[i].Metadata["doc_id"]; !ok {
			chunks[i].Metadata["doc_id"] = uuid.New().String()
		}
		if _, ok := chunks[i].Metadata["doc_hash"]; !ok {
			chunks[i].Metadata["doc_hash"] = simpleHash(chunks[i].Content)
		}
	}
}

func cleanDatasetTransformedChunks(chunks []dto.TransformedChunk) []dto.TransformedChunk {
	if len(chunks) == 0 {
		return chunks
	}

	stats := collectDatasetChunkFilterStats(chunks)
	hasUsefulContent := false
	for _, chunk := range chunks {
		normalized := normalizeDatasetChunkContent(chunk.Content)
		if normalized != "" && !isLowValueDatasetChunkContent(normalized, stats) && datasetChunkRuneLen(normalized) >= 24 {
			hasUsefulContent = true
			break
		}
	}
	if !hasUsefulContent {
		return chunks
	}

	filtered := make([]dto.TransformedChunk, 0, len(chunks))
	for _, chunk := range chunks {
		normalized := normalizeDatasetChunkContent(chunk.Content)
		if isLowValueDatasetChunkContent(normalized, stats) {
			continue
		}

		chunk.Children = cleanDatasetChildChunks(chunk.Children, stats)
		if chunk.Metadata != nil && chunk.Children != nil {
			chunk.Metadata["child_count"] = len(chunk.Children)
		}
		filtered = append(filtered, chunk)
	}

	if len(filtered) == 0 {
		return chunks
	}
	addChunkMetadata(filtered)
	return filtered
}

func cleanDatasetChildChunks(children []dto.TransformedChildChunk, stats datasetChunkFilterStats) []dto.TransformedChildChunk {
	if len(children) == 0 {
		return children
	}

	filtered := make([]dto.TransformedChildChunk, 0, len(children))
	for _, child := range children {
		normalized := normalizeDatasetChunkContent(child.Content)
		if isLowValueDatasetChunkContent(normalized, stats) {
			continue
		}
		filtered = append(filtered, child)
	}
	return filtered
}

func collectDatasetChunkFilterStats(chunks []dto.TransformedChunk) datasetChunkFilterStats {
	counts := make(map[string]int)
	pageNumbers := make(map[int]struct{})
	for _, chunk := range chunks {
		collectDatasetChunkTextStats(normalizeDatasetChunkContent(chunk.Content), counts, pageNumbers)
		for _, child := range chunk.Children {
			collectDatasetChunkTextStats(normalizeDatasetChunkContent(child.Content), counts, pageNumbers)
		}
	}
	return datasetChunkFilterStats{
		normalizedCounts:          counts,
		hasSequentialPageCounters: hasSequentialPageCounterNoise(pageNumbers),
	}
}

func collectDatasetChunkTextStats(normalized string, counts map[string]int, pageNumbers map[int]struct{}) {
	if normalized == "" {
		return
	}
	if datasetChunkRuneLen(normalized) <= 96 {
		counts[normalized]++
	}
	if datasetChunkStandaloneNumberRE.MatchString(normalized) {
		pageNumber, err := strconv.Atoi(normalized)
		if err == nil && pageNumber > 0 && pageNumber <= 300 {
			pageNumbers[pageNumber] = struct{}{}
		}
	}
}

func hasSequentialPageCounterNoise(pageNumbers map[int]struct{}) bool {
	if len(pageNumbers) < 4 {
		return false
	}
	if _, ok := pageNumbers[1]; !ok {
		return false
	}
	if _, ok := pageNumbers[2]; !ok {
		return false
	}

	runLength := 0
	for page := 1; page <= 300; page++ {
		if _, ok := pageNumbers[page]; !ok {
			break
		}
		runLength++
	}
	return runLength >= 4
}

func normalizeDatasetChunkContent(content string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
}

func isLowValueDatasetChunkContent(normalized string, stats datasetChunkFilterStats) bool {
	if normalized == "" {
		return true
	}
	if datasetChunkBarcodePlaceholderRE.MatchString(normalized) ||
		datasetChunkFigurePlaceholderRE.MatchString(normalized) ||
		datasetChunkPageCounterZHRE.MatchString(normalized) ||
		datasetChunkPageCounterENRE.MatchString(normalized) ||
		datasetChunkBlankDateZHRE.MatchString(normalized) ||
		datasetChunkLongNumberRE.MatchString(normalized) ||
		datasetChunkQuotedNumberRE.MatchString(normalized) ||
		datasetChunkSingleCJKRE.MatchString(normalized) {
		return true
	}
	if stats.hasSequentialPageCounters && datasetChunkStandaloneNumberRE.MatchString(normalized) {
		return true
	}
	if isVLMNoTextDatasetChunkContent(normalized) {
		return true
	}
	if isRepeatedShortDatasetHeader(normalized, stats.normalizedCounts) {
		return true
	}
	if isShortCompanyFragment(normalized) {
		return true
	}
	return false
}

func isRepeatedShortDatasetHeader(normalized string, normalizedCounts map[string]int) bool {
	if datasetChunkRuneLen(normalized) > 96 {
		return false
	}
	count := normalizedCounts[normalized]
	if count >= 2 && strings.Contains(normalized, datasetChunkContractNumberLabel) {
		return true
	}
	return count >= 3 && datasetChunkRepeatedCodeRE.MatchString(normalized)
}

func isVLMNoTextDatasetChunkContent(normalized string) bool {
	lower := strings.ToLower(normalized)
	if strings.Contains(lower, "the image provided is a qr code") {
		return true
	}
	if strings.Contains(lower, "no textual content can be extracted") {
		return true
	}
	return strings.Contains(lower, "does not contain any text") &&
		strings.Contains(lower, "ocr instructions")
}

func isShortCompanyFragment(normalized string) bool {
	if datasetChunkRuneLen(normalized) > 16 || !strings.HasSuffix(normalized, datasetChunkCompanySuffix) {
		return false
	}
	return !strings.ContainsAny(normalized, "0123456789:;,./\\")
}

func datasetChunkRuneLen(content string) int {
	return len([]rune(content))
}

func getMetadataByKey(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}

	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

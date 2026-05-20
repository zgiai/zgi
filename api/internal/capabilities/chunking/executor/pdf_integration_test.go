package executor

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	datasetadapter "github.com/zgiai/zgi/api/internal/capabilities/chunking/adapters/dataset"
	chunkquality "github.com/zgiai/zgi/api/internal/capabilities/chunking/quality"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestPDFParseToChunkQualityReport(t *testing.T) {
	pdfDir := os.Getenv("PDF_BENCH_PDF_DIR")
	if pdfDir == "" {
		t.Skip("set PDF_BENCH_PDF_DIR to run PDF parse-to-chunk integration quality checks")
	}
	entries, err := os.ReadDir(pdfDir)
	if err != nil {
		t.Fatal(err)
	}

	valid := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".pdf" {
			continue
		}
		pdfPath := filepath.Join(pdfDir, entry.Name())
		report, err := runPDFParseToChunkQuality(context.Background(), pdfPath)
		if err != nil {
			t.Logf("pdf_quality_skip name=%s error=%v", entry.Name(), err)
			continue
		}
		valid++
		payload, _ := json.Marshal(report)
		t.Logf("pdf_quality_report %s", payload)
	}
	if valid == 0 {
		t.Fatalf("no valid PDFs were parsed from %s", pdfDir)
	}
}

type pdfChunkQualityReport struct {
	Name                   string             `json:"name"`
	PDFBytes               int64              `json:"pdf_bytes"`
	ParseMS                int64              `json:"parse_ms"`
	TextBytes              int                `json:"text_bytes"`
	SourceElements         int                `json:"source_elements"`
	SourceChars            int                `json:"source_chars"`
	PlanSegmentation       string             `json:"plan_segmentation"`
	PlanParentMode         string             `json:"plan_parent_mode"`
	ChunkMS                int64              `json:"chunk_ms"`
	AdapterMS              int64              `json:"adapter_ms"`
	UnitCount              int                `json:"unit_count"`
	ParentCount            int                `json:"parent_count"`
	ChildCount             int                `json:"child_count"`
	ChunkChars             int                `json:"chunk_chars"`
	TextRetentionRatio     float64            `json:"text_retention_ratio"`
	EmptyChunkCount        int                `json:"empty_chunk_count"`
	DuplicateChunkCount    int                `json:"duplicate_chunk_count"`
	ChunkCharDistribution  map[string]int     `json:"chunk_char_distribution"`
	QualityScore           int                `json:"quality_score"`
	QualityLabel           string             `json:"quality_label"`
	QualityWarnings        []string           `json:"quality_warnings,omitempty"`
	FilterReasons          map[string]int     `json:"filter_reasons,omitempty"`
	SourceElementFilter    map[string]int     `json:"source_element_filter_reasons,omitempty"`
	Throughput             map[string]float64 `json:"throughput"`
	ValidForDatasetCutover bool               `json:"valid_for_dataset_cutover"`
	DatasetCutoverBlockers []string           `json:"dataset_cutover_blockers,omitempty"`
}

func runPDFParseToChunkQuality(ctx context.Context, pdfPath string) (*pdfChunkQualityReport, error) {
	info, err := os.Stat(pdfPath)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "zgi-pdf-parse-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	textPath := filepath.Join(tempDir, strings.TrimSuffix(filepath.Base(pdfPath), filepath.Ext(pdfPath))+".txt")
	parseStarted := time.Now()
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", pdfPath, textPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftotext: %w: %s", err, strings.TrimSpace(string(output)))
	}
	parseMS := time.Since(parseStarted).Milliseconds()

	raw, err := os.ReadFile(textPath)
	if err != nil {
		return nil, err
	}
	doc := chunkSourceDocumentFromPDFText(filepath.Base(pdfPath), string(raw))
	plan := &contracts.ChunkPlan{
		UseCase:       contracts.ChunkUseCaseDatasetIndex,
		ParentMode:    "page_aware",
		Segmentation:  "page_layout_aware",
		PreserveOrder: true,
		TargetKinds: []contracts.ChunkKind{
			contracts.ChunkKindText,
			contracts.ChunkKindHeading,
			contracts.ChunkKindTable,
		},
		Metadata: map[string]any{
			"benchmark_source": "pdftotext",
		},
	}

	execSvc := New(WithLimits(Limits{MaxWorkers: 8, MaxPartitionSize: 64}))
	chunkStarted := time.Now()
	result, err := execSvc.Execute(ctx, doc, plan)
	if err != nil {
		return nil, err
	}
	chunkMS := time.Since(chunkStarted).Milliseconds()

	adapterStarted := time.Now()
	chunks := datasetadapter.UnitsToTransformedChunksWithOptions(result.Units, datasetadapter.AdapterOptions{
		BuildChildren:     true,
		SubchunkMaxTokens: 1000,
		SubchunkOverlap:   50,
		SubchunkSeparator: "\n\n",
	})
	adapterMS := time.Since(adapterStarted).Milliseconds()

	unitStats := pdfChunkUnitStats(result.Units)
	chunkStats := pdfTransformedChunkStats(chunks)
	sourceChars := sourceDocumentChars(doc)
	score := chunkquality.EvaluateChunkScore(chunkquality.ScoreInput{
		UnitCount:            len(result.Units),
		TotalChars:           unitStats.totalChars,
		AvgChars:             averagePDFInt(unitStats.totalChars, len(result.Units)),
		StableOrder:          result.Metrics.StableOrder,
		LowValueRemovedCount: result.Metrics.FilteredUnitCount + result.Metrics.SourceElementFilteredCount,
		LegacyUnitCount:      len(doc.Elements),
		LegacyTotalChars:     sourceChars,
	})
	blockers := datasetCutoverBlockers(score, sourceChars, unitStats, result)

	return &pdfChunkQualityReport{
		Name:                   filepath.Base(pdfPath),
		PDFBytes:               info.Size(),
		ParseMS:                parseMS,
		TextBytes:              len(raw),
		SourceElements:         len(doc.Elements),
		SourceChars:            sourceChars,
		PlanSegmentation:       plan.Segmentation,
		PlanParentMode:         plan.ParentMode,
		ChunkMS:                chunkMS,
		AdapterMS:              adapterMS,
		UnitCount:              len(result.Units),
		ParentCount:            chunkStats.parentCount,
		ChildCount:             chunkStats.childCount,
		ChunkChars:             unitStats.totalChars,
		TextRetentionRatio:     ratioPDF(unitStats.totalChars, sourceChars),
		EmptyChunkCount:        unitStats.emptyCount,
		DuplicateChunkCount:    unitStats.duplicateCount,
		ChunkCharDistribution:  unitStats.distribution,
		QualityScore:           score.Overall,
		QualityLabel:           score.Label,
		QualityWarnings:        score.Warnings,
		FilterReasons:          result.Metrics.FilterReasons,
		SourceElementFilter:    result.Metrics.SourceElementFilterReasons,
		Throughput:             throughputPDF(info.Size(), int64(len(raw)), parseMS, chunkMS, adapterMS),
		ValidForDatasetCutover: len(blockers) == 0,
		DatasetCutoverBlockers: blockers,
	}, nil
}

func chunkSourceDocumentFromPDFText(name, text string) *contracts.ChunkSourceDocument {
	pages := strings.Split(text, "\f")
	elements := make([]contracts.ChunkSourceElement, 0, len(text)/1024+1)
	ordinal := 1
	for pageIndex, page := range pages {
		for _, paragraph := range splitPDFTextParagraphs(page) {
			elements = append(elements, contracts.ChunkSourceElement{
				ElementID: fmt.Sprintf("%s-element-%d", name, ordinal),
				Type:      "text",
				Page:      pageIndex + 1,
				Content:   paragraph,
				Markdown:  paragraph,
				Ordinal:   ordinal,
				Metadata: map[string]any{
					"parse_source": "pdftotext",
				},
			})
			ordinal++
		}
	}
	return &contracts.ChunkSourceDocument{
		DocumentID: name,
		FileID:     name,
		Source:     "pdf_integration_benchmark",
		Title:      name,
		Elements:   elements,
		Metadata: map[string]any{
			"parse_source": "pdftotext",
		},
	}
}

func splitPDFTextParagraphs(text string) []string {
	const targetBytes = 1024
	lines := strings.Split(text, "\n")
	paragraphs := make([]string, 0, len(lines)/4+1)
	var builder strings.Builder
	flush := func() {
		content := strings.TrimSpace(builder.String())
		builder.Reset()
		if content != "" {
			paragraphs = append(paragraphs, content)
		}
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		if builder.Len()+len(line)+1 > targetBytes {
			flush()
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
	}
	flush()
	return paragraphs
}

type pdfUnitStats struct {
	totalChars     int
	emptyCount     int
	duplicateCount int
	distribution   map[string]int
}

func pdfChunkUnitStats(units []contracts.ChunkUnit) pdfUnitStats {
	lengths := make([]int, 0, len(units))
	seen := make(map[string]int)
	stats := pdfUnitStats{distribution: map[string]int{}}
	for _, unit := range units {
		content := strings.TrimSpace(unit.Content)
		length := len([]rune(content))
		lengths = append(lengths, length)
		stats.totalChars += length
		if length == 0 {
			stats.emptyCount++
		}
		if content != "" {
			hash := sha1.Sum([]byte(content))
			seen[hex.EncodeToString(hash[:])]++
		}
	}
	for _, count := range seen {
		if count > 1 {
			stats.duplicateCount += count - 1
		}
	}
	sort.Ints(lengths)
	stats.distribution["min"] = percentilePDF(lengths, 0)
	stats.distribution["p50"] = percentilePDF(lengths, 50)
	stats.distribution["p90"] = percentilePDF(lengths, 90)
	stats.distribution["p99"] = percentilePDF(lengths, 99)
	stats.distribution["max"] = percentilePDF(lengths, 100)
	return stats
}

type pdfTransformedStats struct {
	parentCount int
	childCount  int
}

func pdfTransformedChunkStats(chunks []dto.TransformedChunk) pdfTransformedStats {
	stats := pdfTransformedStats{parentCount: len(chunks)}
	for _, chunk := range chunks {
		stats.childCount += len(chunk.Children)
	}
	return stats
}

func sourceDocumentChars(doc *contracts.ChunkSourceDocument) int {
	total := 0
	for _, element := range doc.Elements {
		total += len([]rune(strings.TrimSpace(element.Content)))
	}
	return total
}

func datasetCutoverBlockers(score chunkquality.Score, sourceChars int, stats pdfUnitStats, result *Result) []string {
	blockers := make([]string, 0)
	if score.Overall < 85 {
		blockers = append(blockers, "quality_score_below_ready_threshold")
	}
	if ratioPDF(stats.totalChars, sourceChars) < 0.9 {
		blockers = append(blockers, "text_retention_below_90_percent")
	}
	if stats.emptyCount > 0 {
		blockers = append(blockers, "empty_chunks")
	}
	if stats.duplicateCount > 0 && ratioPDF(stats.duplicateCount, len(result.Units)) > 0.02 {
		blockers = append(blockers, "duplicate_chunk_rate_above_2_percent")
	}
	if result == nil || !result.Metrics.StableOrder {
		blockers = append(blockers, "unstable_chunk_order")
	}
	return blockers
}

func throughputPDF(pdfBytes int64, textBytes, parseMS, chunkMS, adapterMS int64) map[string]float64 {
	return map[string]float64{
		"parse_pdf_mb_per_sec":       mbPerSecondPDF(float64(pdfBytes), parseMS),
		"chunk_text_mb_per_sec":      mbPerSecondPDF(float64(textBytes), chunkMS),
		"adapter_text_mb_per_sec":    mbPerSecondPDF(float64(textBytes), adapterMS),
		"end_to_end_pdf_mb_per_sec":  mbPerSecondPDF(float64(pdfBytes), parseMS+chunkMS+adapterMS),
		"end_to_end_text_mb_per_sec": mbPerSecondPDF(float64(textBytes), parseMS+chunkMS+adapterMS),
	}
}

func percentilePDF(sorted []int, percentile int) int {
	if len(sorted) == 0 {
		return 0
	}
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 100 {
		return sorted[len(sorted)-1]
	}
	index := (len(sorted) - 1) * percentile / 100
	return sorted[index]
}

func averagePDFInt(total, count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func ratioPDF(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func mbPerSecondPDF(bytes float64, milliseconds int64) float64 {
	if milliseconds <= 0 {
		return 0
	}
	return bytes / 1024 / 1024 / (float64(milliseconds) / 1000)
}

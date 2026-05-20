package service

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
)

type RunQueryService interface {
	CreateParseRun(ctx context.Context, item *model.ParseRun) error
	GetParseRunByID(ctx context.Context, id uuid.UUID) (*model.ParseRun, error)
	ListParseRunsByDocumentID(ctx context.Context, documentID uuid.UUID, limit int) ([]*model.ParseRun, error)
	ListParseRunsByDatasetID(ctx context.Context, datasetID uuid.UUID, limit int) ([]*model.ParseRun, error)
	GetLatestDatasetShadowSummary(ctx context.Context, datasetID uuid.UUID, limit int) (*DatasetShadowSummary, error)
	CreateChunkingRun(ctx context.Context, item *model.ChunkingRun) error
	ListChunkingRunsByParseRunID(ctx context.Context, parseRunID uuid.UUID) ([]*model.ChunkingRun, error)
}

type DatasetShadowDocumentSummary struct {
	DocumentID           *uuid.UUID `json:"document_id,omitempty"`
	RunID                uuid.UUID  `json:"run_id"`
	Status               string     `json:"status"`
	QualityLevel         string     `json:"quality_level"`
	Error                string     `json:"error,omitempty"`
	ProviderKey          string     `json:"provider_key,omitempty"`
	FallbackUsed         bool       `json:"fallback_used"`
	TextLength           int        `json:"text_length"`
	ElementCount         int        `json:"element_count"`
	ParentMode           string     `json:"parent_mode,omitempty"`
	Segmentation         string     `json:"segmentation,omitempty"`
	ChunkQualityScore    *int       `json:"chunk_quality_score,omitempty"`
	ChunkQualityLabel    string     `json:"chunk_quality_label,omitempty"`
	ChunkQualityWarnings []string   `json:"chunk_quality_warnings,omitempty"`
	TextRetentionRatio   *float64   `json:"text_retention_ratio,omitempty"`
	ChunkCountDelta      *int       `json:"chunk_count_delta,omitempty"`
	StableChunkOrder     *bool      `json:"stable_chunk_order,omitempty"`
	LegacyTextLength     int        `json:"legacy_text_length,omitempty"`
	ReadinessDecision    string     `json:"readiness_decision,omitempty"`
	ReadinessReasons     []string   `json:"readiness_reasons,omitempty"`
}

type DatasetShadowChunkQualitySummary struct {
	EvaluatedDocumentCount int                            `json:"evaluated_document_count"`
	HighCount              int                            `json:"high_count"`
	StandardCount          int                            `json:"standard_count"`
	DegradedCount          int                            `json:"degraded_count"`
	FailedCount            int                            `json:"failed_count"`
	HighRate               float64                        `json:"high_rate"`
	StandardRate           float64                        `json:"standard_rate"`
	DegradedRate           float64                        `json:"degraded_rate"`
	FailedRate             float64                        `json:"failed_rate"`
	AvgQualityScore        float64                        `json:"avg_quality_score"`
	AvgTextRetentionRatio  float64                        `json:"avg_text_retention_ratio"`
	AvgChunkCountDelta     float64                        `json:"avg_chunk_count_delta"`
	LargeTextLossCount     int                            `json:"large_text_loss_count"`
	TextLossCount          int                            `json:"text_loss_count"`
	ChunkExpansionCount    int                            `json:"chunk_expansion_count"`
	UnstableOrderCount     int                            `json:"unstable_order_count"`
	LowBBoxCoverageCount   int                            `json:"low_bbox_coverage_count"`
	Decision               string                         `json:"decision"`
	Reasons                []string                       `json:"reasons,omitempty"`
	TopRiskDocuments       []DatasetShadowDocumentSummary `json:"top_risk_documents,omitempty"`
}

type DatasetShadowReadinessSummary struct {
	DocumentCount          int                            `json:"document_count"`
	EvaluatedDocumentCount int                            `json:"evaluated_document_count"`
	ReadyCount             int                            `json:"ready_count"`
	ObserveCount           int                            `json:"observe_count"`
	BlockedCount           int                            `json:"blocked_count"`
	UnknownCount           int                            `json:"unknown_count"`
	ReadyRate              float64                        `json:"ready_rate"`
	ObserveRate            float64                        `json:"observe_rate"`
	BlockedRate            float64                        `json:"blocked_rate"`
	UnknownRate            float64                        `json:"unknown_rate"`
	AvgQualityScore        float64                        `json:"avg_quality_score"`
	Decision               string                         `json:"decision"`
	Reasons                []string                       `json:"reasons,omitempty"`
	TopRiskDocuments       []DatasetShadowDocumentSummary `json:"top_risk_documents,omitempty"`
}

type DatasetShadowSummary struct {
	DatasetID                 uuid.UUID                        `json:"dataset_id"`
	DocumentCount             int                              `json:"document_count"`
	SuccessCount              int                              `json:"success_count"`
	DegradedCount             int                              `json:"degraded_count"`
	FailedCount               int                              `json:"failed_count"`
	FallbackCount             int                              `json:"fallback_count"`
	ProviderBreakdown         map[string]int                   `json:"provider_breakdown,omitempty"`
	ProviderFallbackBreakdown map[string]int                   `json:"provider_fallback_breakdown,omitempty"`
	OCREngineBreakdown        map[string]int                   `json:"ocr_engine_breakdown,omitempty"`
	OCRStrategyBreakdown      map[string]int                   `json:"ocr_strategy_breakdown,omitempty"`
	AvgTextLength             float64                          `json:"avg_text_length"`
	AvgElementCount           float64                          `json:"avg_element_count"`
	LowTextLengthDocs         int                              `json:"low_text_length_docs"`
	ChunkingParentModes       map[string]int                   `json:"chunking_parent_modes,omitempty"`
	ChunkingSegmentations     map[string]int                   `json:"chunking_segmentations,omitempty"`
	ChunkQuality              DatasetShadowChunkQualitySummary `json:"chunk_quality"`
	Readiness                 DatasetShadowReadinessSummary    `json:"readiness"`
	TopFailedDocuments        []DatasetShadowDocumentSummary   `json:"top_failed_documents,omitempty"`
	TopDegradedDocuments      []DatasetShadowDocumentSummary   `json:"top_degraded_documents,omitempty"`
	LatestDocuments           []DatasetShadowDocumentSummary   `json:"latest_documents,omitempty"`
}

type runQueryService struct {
	parseRuns    repository.ParseRunRepository
	chunkingRuns repository.ChunkingRunRepository
}

func NewRunQueryService(parseRuns repository.ParseRunRepository, chunkingRuns repository.ChunkingRunRepository) RunQueryService {
	return &runQueryService{
		parseRuns:    parseRuns,
		chunkingRuns: chunkingRuns,
	}
}

func (s *runQueryService) CreateParseRun(ctx context.Context, item *model.ParseRun) error {
	return s.parseRuns.Create(ctx, item)
}

func (s *runQueryService) GetParseRunByID(ctx context.Context, id uuid.UUID) (*model.ParseRun, error) {
	return s.parseRuns.GetByID(ctx, id)
}

func (s *runQueryService) ListParseRunsByDocumentID(ctx context.Context, documentID uuid.UUID, limit int) ([]*model.ParseRun, error) {
	return s.parseRuns.ListByDocumentID(ctx, documentID, limit)
}

func (s *runQueryService) ListParseRunsByDatasetID(ctx context.Context, datasetID uuid.UUID, limit int) ([]*model.ParseRun, error) {
	return s.parseRuns.ListByDatasetID(ctx, datasetID, limit)
}

func (s *runQueryService) GetLatestDatasetShadowSummary(ctx context.Context, datasetID uuid.UUID, limit int) (*DatasetShadowSummary, error) {
	runs, err := s.parseRuns.ListLatestByDatasetID(ctx, datasetID, limit)
	if err != nil {
		return nil, err
	}

	summary := &DatasetShadowSummary{
		DatasetID:                 datasetID,
		DocumentCount:             len(runs),
		ProviderBreakdown:         map[string]int{},
		ProviderFallbackBreakdown: map[string]int{},
		OCREngineBreakdown:        map[string]int{},
		OCRStrategyBreakdown:      map[string]int{},
		ChunkingParentModes:       map[string]int{},
		ChunkingSegmentations:     map[string]int{},
		ChunkQuality: DatasetShadowChunkQualitySummary{
			Decision:         "unknown",
			Reasons:          []string{"no_chunk_shadow_runs"},
			TopRiskDocuments: []DatasetShadowDocumentSummary{},
		},
		Readiness: DatasetShadowReadinessSummary{
			Decision:         "unknown",
			Reasons:          []string{"no_shadow_runs"},
			TopRiskDocuments: []DatasetShadowDocumentSummary{},
		},
		TopFailedDocuments:   []DatasetShadowDocumentSummary{},
		TopDegradedDocuments: []DatasetShadowDocumentSummary{},
		LatestDocuments:      make([]DatasetShadowDocumentSummary, 0, len(runs)),
	}

	totalTextLength := 0
	totalElementCount := 0
	latestChunkingByRunID := map[uuid.UUID]*model.ChunkingRun{}
	if s.chunkingRuns != nil {
		runIDs := make([]uuid.UUID, 0, len(runs))
		for _, run := range runs {
			if run != nil {
				runIDs = append(runIDs, run.ID)
			}
		}
		chunkingRuns, chunkErr := s.chunkingRuns.ListLatestByParseRunIDs(ctx, runIDs)
		if chunkErr == nil {
			for _, chunkingRun := range chunkingRuns {
				if chunkingRun == nil {
					continue
				}
				if _, exists := latestChunkingByRunID[chunkingRun.ParseRunID]; !exists {
					latestChunkingByRunID[chunkingRun.ParseRunID] = chunkingRun
				}
			}
		}
	}

	for _, run := range runs {
		if run == nil {
			continue
		}
		docSummary := DatasetShadowDocumentSummary{
			DocumentID:   run.DocumentID,
			RunID:        run.ID,
			Status:       run.Status,
			QualityLevel: run.QualityLevel,
			Error:        readSummaryString(run.SummaryJSON, "error"),
			ProviderKey:  run.FinalProviderKey,
			FallbackUsed: run.FallbackUsed,
			TextLength:   readSummaryInt(run.SummaryJSON, "text_length"),
			ElementCount: readSummaryInt(run.SummaryJSON, "element_count"),
		}

		if chunkingRun := latestChunkingByRunID[run.ID]; chunkingRun != nil {
			docSummary.ParentMode = chunkingRun.ParentMode
			docSummary.Segmentation = chunkingRun.Segmentation
			if docSummary.ParentMode != "" {
				summary.ChunkingParentModes[docSummary.ParentMode]++
			}
			if docSummary.Segmentation != "" {
				summary.ChunkingSegmentations[docSummary.Segmentation]++
			}
			applyChunkQualityToDatasetSummary(&summary.ChunkQuality, &docSummary, chunkingRun.PlanJSON)
		}
		applyDocumentReadinessToDatasetSummary(&summary.Readiness, &docSummary)

		summary.LatestDocuments = append(summary.LatestDocuments, docSummary)
		totalTextLength += docSummary.TextLength
		totalElementCount += docSummary.ElementCount

		if run.FinalProviderKey != "" {
			summary.ProviderBreakdown[run.FinalProviderKey]++
		}
		if diagnostics, ok := run.SummaryJSON["diagnostics"].(map[string]interface{}); ok {
			if engine, ok := diagnostics["ocr_engine"].(string); ok && engine != "" {
				summary.OCREngineBreakdown[engine]++
			}
			if strategy, ok := diagnostics["ocr_strategy"].(string); ok && strategy != "" {
				summary.OCRStrategyBreakdown[strategy]++
			}
		}
		if run.FallbackUsed {
			summary.FallbackCount++
			if run.FinalProviderKey != "" {
				summary.ProviderFallbackBreakdown[run.FinalProviderKey]++
			}
		}
		if docSummary.TextLength < 300 {
			summary.LowTextLengthDocs++
		}

		switch run.Status {
		case "succeeded":
			summary.SuccessCount++
		case "degraded":
			summary.DegradedCount++
			summary.TopDegradedDocuments = append(summary.TopDegradedDocuments, docSummary)
		default:
			summary.FailedCount++
			summary.TopFailedDocuments = append(summary.TopFailedDocuments, docSummary)
		}
	}

	if summary.DocumentCount > 0 {
		summary.AvgTextLength = float64(totalTextLength) / float64(summary.DocumentCount)
		summary.AvgElementCount = float64(totalElementCount) / float64(summary.DocumentCount)
	}
	finalizeDatasetChunkQuality(&summary.ChunkQuality)
	finalizeDatasetShadowReadiness(&summary.Readiness)

	sort.SliceStable(summary.TopFailedDocuments, func(i, j int) bool {
		return summary.TopFailedDocuments[i].TextLength < summary.TopFailedDocuments[j].TextLength
	})
	sort.SliceStable(summary.TopDegradedDocuments, func(i, j int) bool {
		return summary.TopDegradedDocuments[i].ElementCount < summary.TopDegradedDocuments[j].ElementCount
	})

	if len(summary.TopFailedDocuments) > 10 {
		summary.TopFailedDocuments = summary.TopFailedDocuments[:10]
	}
	if len(summary.TopDegradedDocuments) > 10 {
		summary.TopDegradedDocuments = summary.TopDegradedDocuments[:10]
	}
	if len(summary.ChunkQuality.TopRiskDocuments) > 10 {
		summary.ChunkQuality.TopRiskDocuments = summary.ChunkQuality.TopRiskDocuments[:10]
	}
	if len(summary.Readiness.TopRiskDocuments) > 10 {
		summary.Readiness.TopRiskDocuments = summary.Readiness.TopRiskDocuments[:10]
	}

	return summary, nil
}

func (s *runQueryService) CreateChunkingRun(ctx context.Context, item *model.ChunkingRun) error {
	return s.chunkingRuns.Create(ctx, item)
}

func (s *runQueryService) ListChunkingRunsByParseRunID(ctx context.Context, parseRunID uuid.UUID) ([]*model.ChunkingRun, error) {
	return s.chunkingRuns.ListByParseRunID(ctx, parseRunID)
}

func applyChunkQualityToDatasetSummary(summary *DatasetShadowChunkQualitySummary, doc *DatasetShadowDocumentSummary, plan map[string]any) {
	if summary == nil || doc == nil || len(plan) == 0 {
		return
	}
	qualityScore := readNestedMap(plan, "quality_score")
	if len(qualityScore) == 0 {
		return
	}

	score := readSummaryInt(qualityScore, "overall")
	label := readSummaryString(qualityScore, "label")
	warnings := readSummaryStringSlice(qualityScore, "warnings")
	doc.ChunkQualityScore = intPtr(score)
	doc.ChunkQualityLabel = label
	doc.ChunkQualityWarnings = warnings

	comparison := readNestedMap(plan, "comparison")
	if len(comparison) > 0 {
		doc.LegacyTextLength = readSummaryInt(comparison, "legacy_text_length")
		if doc.LegacyTextLength > 0 {
			doc.TextRetentionRatio = float64Ptr(readSummaryFloat(comparison, "text_retention_ratio"))
		}
		doc.ChunkCountDelta = intPtr(readSummaryInt(comparison, "unit_count_delta"))
	}
	execution := readNestedMap(plan, "execution")
	if len(execution) > 0 {
		doc.StableChunkOrder = boolPtr(readSummaryBool(execution, "stable_order"))
	}

	summary.EvaluatedDocumentCount++
	summary.AvgQualityScore += float64(score)
	if doc.TextRetentionRatio != nil {
		summary.AvgTextRetentionRatio += *doc.TextRetentionRatio
	}
	if doc.ChunkCountDelta != nil {
		summary.AvgChunkCountDelta += float64(*doc.ChunkCountDelta)
	}

	switch label {
	case "high":
		summary.HighCount++
	case "standard":
		summary.StandardCount++
	case "degraded":
		summary.DegradedCount++
	default:
		summary.FailedCount++
	}

	if containsString(warnings, "large_text_loss") {
		summary.LargeTextLossCount++
	}
	if containsString(warnings, "text_loss") {
		summary.TextLossCount++
	}
	if containsString(warnings, "chunk_count_expansion") {
		summary.ChunkExpansionCount++
	}
	if containsString(warnings, "unstable_order") || doc.StableChunkOrder != nil && !*doc.StableChunkOrder {
		summary.UnstableOrderCount++
	}
	if containsString(warnings, "low_bbox_coverage") {
		summary.LowBBoxCoverageCount++
	}
	if label == "degraded" || label == "failed" || len(warnings) > 0 {
		summary.TopRiskDocuments = append(summary.TopRiskDocuments, *doc)
	}
}

func applyDocumentReadinessToDatasetSummary(summary *DatasetShadowReadinessSummary, doc *DatasetShadowDocumentSummary) {
	if summary == nil || doc == nil {
		return
	}
	decision, reasons := evaluateDatasetShadowDocumentReadiness(doc)
	doc.ReadinessDecision = decision
	doc.ReadinessReasons = reasons

	summary.DocumentCount++
	if doc.ChunkQualityScore != nil {
		summary.EvaluatedDocumentCount++
		summary.AvgQualityScore += float64(*doc.ChunkQualityScore)
	}

	switch decision {
	case "ready":
		summary.ReadyCount++
	case "observe":
		summary.ObserveCount++
		summary.TopRiskDocuments = append(summary.TopRiskDocuments, *doc)
	case "blocked":
		summary.BlockedCount++
		summary.TopRiskDocuments = append(summary.TopRiskDocuments, *doc)
	default:
		summary.UnknownCount++
		summary.TopRiskDocuments = append(summary.TopRiskDocuments, *doc)
	}
}

func evaluateDatasetShadowDocumentReadiness(doc *DatasetShadowDocumentSummary) (string, []string) {
	if doc == nil {
		return "unknown", []string{"missing_document_summary"}
	}

	reasons := make([]string, 0)
	switch doc.Status {
	case "succeeded":
	case "degraded":
		reasons = append(reasons, "parse_shadow_degraded")
	default:
		if doc.Status == "" {
			return "unknown", []string{"missing_parse_status"}
		}
		if isSourceUnavailableError(doc.Error) {
			return "unknown", []string{"source_unavailable"}
		}
		return "blocked", []string{"parse_shadow_failed"}
	}

	if doc.ChunkQualityScore == nil {
		if len(reasons) > 0 {
			reasons = append(reasons, "missing_chunk_quality_score")
			return "observe", reasons
		}
		return "unknown", []string{"missing_chunk_quality_score"}
	}

	score := *doc.ChunkQualityScore
	blocked := false
	if score < 70 {
		blocked = true
		reasons = append(reasons, "low_quality_score")
	}
	if doc.ChunkQualityLabel == "failed" {
		blocked = true
		reasons = append(reasons, "failed_chunk_quality")
	}
	if containsString(doc.ChunkQualityWarnings, "large_text_loss") {
		blocked = true
		reasons = append(reasons, "large_text_loss")
	}
	if containsString(doc.ChunkQualityWarnings, "unstable_order") || doc.StableChunkOrder != nil && !*doc.StableChunkOrder {
		blocked = true
		reasons = append(reasons, "unstable_order")
	}
	if doc.TextRetentionRatio != nil && *doc.TextRetentionRatio < 0.8 {
		blocked = true
		reasons = append(reasons, "low_text_retention")
	}
	if blocked {
		return "blocked", uniqueStrings(reasons)
	}

	if doc.ChunkQualityLabel == "" {
		reasons = append(reasons, "missing_quality_label")
	}
	if doc.ChunkQualityLabel == "standard" {
		reasons = append(reasons, "standard_quality")
	}
	if doc.ChunkQualityLabel == "degraded" {
		reasons = append(reasons, "degraded_quality")
	}
	if score < 90 {
		reasons = append(reasons, "quality_below_ready_threshold")
	}
	if containsString(doc.ChunkQualityWarnings, "text_loss") {
		reasons = append(reasons, "text_loss")
	}
	if containsString(doc.ChunkQualityWarnings, "chunk_count_expansion") {
		reasons = append(reasons, "chunk_count_expansion")
	}
	if containsString(doc.ChunkQualityWarnings, "low_bbox_coverage") {
		reasons = append(reasons, "low_bbox_coverage")
	}
	if doc.TextRetentionRatio != nil && *doc.TextRetentionRatio < 0.95 {
		reasons = append(reasons, "text_retention_below_ready_threshold")
	}
	if len(reasons) > 0 {
		return "observe", uniqueStrings(reasons)
	}
	return "ready", nil
}

func isSourceUnavailableError(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return false
	}
	return strings.Contains(normalized, "download file") ||
		strings.Contains(normalized, "failed to load file") ||
		strings.Contains(normalized, "no such file") ||
		strings.Contains(normalized, "source file")
}

func finalizeDatasetChunkQuality(summary *DatasetShadowChunkQualitySummary) {
	if summary == nil {
		return
	}
	evaluated := summary.EvaluatedDocumentCount
	if evaluated <= 0 {
		summary.Decision = "unknown"
		summary.Reasons = []string{"no_chunk_shadow_runs"}
		return
	}

	summary.AvgQualityScore = summary.AvgQualityScore / float64(evaluated)
	summary.AvgTextRetentionRatio = summary.AvgTextRetentionRatio / float64(evaluated)
	summary.AvgChunkCountDelta = summary.AvgChunkCountDelta / float64(evaluated)
	summary.HighRate = datasetShadowRate(summary.HighCount, evaluated)
	summary.StandardRate = datasetShadowRate(summary.StandardCount, evaluated)
	summary.DegradedRate = datasetShadowRate(summary.DegradedCount, evaluated)
	summary.FailedRate = datasetShadowRate(summary.FailedCount, evaluated)

	reasons := make([]string, 0)
	blocked := false
	if summary.FailedCount > 0 {
		blocked = true
		reasons = append(reasons, "failed_chunk_quality")
	}
	if summary.LargeTextLossCount > 0 {
		blocked = true
		reasons = append(reasons, "large_text_loss")
	}
	if summary.UnstableOrderCount > 0 {
		blocked = true
		reasons = append(reasons, "unstable_order")
	}
	if summary.AvgQualityScore < 70 {
		blocked = true
		reasons = append(reasons, "low_average_quality_score")
	}
	if summary.AvgTextRetentionRatio > 0 && summary.AvgTextRetentionRatio < 0.8 {
		blocked = true
		reasons = append(reasons, "low_text_retention")
	}
	if datasetShadowRate(summary.DegradedCount+summary.FailedCount, evaluated) >= 0.2 {
		blocked = true
		reasons = append(reasons, "high_degraded_or_failed_rate")
	}

	if blocked {
		summary.Decision = "blocked"
		summary.Reasons = reasons
		sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
		return
	}

	if evaluated < 5 {
		reasons = append(reasons, "small_shadow_sample")
	}
	if summary.AvgQualityScore < 85 {
		reasons = append(reasons, "average_quality_below_ready_threshold")
	}
	if summary.DegradedCount > 0 {
		reasons = append(reasons, "degraded_documents_present")
	}
	if summary.StandardRate > 0.15 {
		reasons = append(reasons, "standard_quality_rate_above_ready_threshold")
	}
	if summary.TextLossCount > 0 {
		reasons = append(reasons, "text_loss")
	}
	if summary.AvgTextRetentionRatio > 0 && summary.AvgTextRetentionRatio < 0.95 {
		reasons = append(reasons, "text_retention_below_ready_threshold")
	}
	if summary.ChunkExpansionCount > 0 {
		reasons = append(reasons, "chunk_count_expansion")
	}
	if summary.LowBBoxCoverageCount > 0 {
		reasons = append(reasons, "low_bbox_coverage")
	}

	if len(reasons) > 0 {
		summary.Decision = "observe"
		summary.Reasons = reasons
		sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
		return
	}

	summary.Decision = "ready"
	summary.Reasons = nil
	sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
}

func finalizeDatasetShadowReadiness(summary *DatasetShadowReadinessSummary) {
	if summary == nil {
		return
	}
	total := summary.DocumentCount
	if total <= 0 {
		summary.Decision = "unknown"
		summary.Reasons = []string{"no_shadow_runs"}
		return
	}
	if summary.EvaluatedDocumentCount > 0 {
		summary.AvgQualityScore = summary.AvgQualityScore / float64(summary.EvaluatedDocumentCount)
	}
	summary.ReadyRate = datasetShadowRate(summary.ReadyCount, total)
	summary.ObserveRate = datasetShadowRate(summary.ObserveCount, total)
	summary.BlockedRate = datasetShadowRate(summary.BlockedCount, total)
	summary.UnknownRate = datasetShadowRate(summary.UnknownCount, total)

	reasons := make([]string, 0)
	if summary.BlockedCount > 0 {
		reasons = append(reasons, "blocked_documents_present")
	}
	if datasetShadowRate(summary.BlockedCount, total) >= 0.05 {
		reasons = append(reasons, "blocked_rate_above_threshold")
	}
	if len(reasons) > 0 {
		summary.Decision = "blocked"
		summary.Reasons = uniqueStrings(reasons)
		sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
		return
	}

	if summary.EvaluatedDocumentCount == 0 {
		summary.Decision = "unknown"
		summary.Reasons = []string{"no_quality_scores"}
		sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
		return
	}

	if summary.UnknownCount > 0 {
		reasons = append(reasons, "documents_missing_quality_score")
	}
	if summary.ObserveCount > 0 {
		reasons = append(reasons, "documents_need_observation")
	}
	if total < 5 {
		reasons = append(reasons, "small_shadow_sample")
	}
	if summary.ReadyRate < 0.95 {
		reasons = append(reasons, "ready_rate_below_threshold")
	}
	if summary.AvgQualityScore < 90 {
		reasons = append(reasons, "average_quality_below_ready_threshold")
	}
	if len(reasons) > 0 {
		summary.Decision = "observe"
		summary.Reasons = uniqueStrings(reasons)
		sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
		return
	}

	summary.Decision = "ready"
	summary.Reasons = nil
	sortDatasetChunkRiskDocuments(summary.TopRiskDocuments)
}

func sortDatasetChunkRiskDocuments(items []DatasetShadowDocumentSummary) {
	sort.SliceStable(items, func(i, j int) bool {
		left := 101
		right := 101
		if items[i].ChunkQualityScore != nil {
			left = *items[i].ChunkQualityScore
		}
		if items[j].ChunkQualityScore != nil {
			right = *items[j].ChunkQualityScore
		}
		if left == right {
			return items[i].TextLength < items[j].TextLength
		}
		return left < right
	})
}

func uniqueStrings(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func datasetShadowRate(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func readNestedMap(summary map[string]any, key string) map[string]any {
	if len(summary) == 0 {
		return nil
	}
	raw, ok := summary[key]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case map[string]any:
		return value
	default:
		return nil
	}
}

func readSummaryInt(summary map[string]any, key string) int {
	if len(summary) == 0 {
		return 0
	}
	raw, ok := summary[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func readSummaryFloat(summary map[string]any, key string) float64 {
	if len(summary) == 0 {
		return 0
	}
	raw, ok := summary[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case float32:
		return float64(value)
	case float64:
		return value
	case int:
		return float64(value)
	case int32:
		return float64(value)
	case int64:
		return float64(value)
	default:
		return 0
	}
}

func readSummaryBool(summary map[string]any, key string) bool {
	if len(summary) == 0 {
		return false
	}
	raw, ok := summary[key]
	if !ok || raw == nil {
		return false
	}
	value, _ := raw.(bool)
	return value
}

func readSummaryString(summary map[string]any, key string) string {
	if len(summary) == 0 {
		return ""
	}
	raw, ok := summary[key]
	if !ok || raw == nil {
		return ""
	}
	value, _ := raw.(string)
	return value
}

func readSummaryStringSlice(summary map[string]any, key string) []string {
	if len(summary) == 0 {
		return nil
	}
	raw, ok := summary[key]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func intPtr(value int) *int {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

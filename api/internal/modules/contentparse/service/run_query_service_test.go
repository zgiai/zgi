package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
)

type fakeParseRunRepository struct {
	runs []*model.ParseRun
}

func (f *fakeParseRunRepository) Create(context.Context, *model.ParseRun) error {
	return nil
}

func (f *fakeParseRunRepository) GetByID(context.Context, uuid.UUID) (*model.ParseRun, error) {
	return nil, nil
}

func (f *fakeParseRunRepository) ListByDocumentID(context.Context, uuid.UUID, int) ([]*model.ParseRun, error) {
	return nil, nil
}

func (f *fakeParseRunRepository) ListByDatasetID(context.Context, uuid.UUID, int) ([]*model.ParseRun, error) {
	return f.runs, nil
}

func (f *fakeParseRunRepository) ListLatestByDatasetID(context.Context, uuid.UUID, int) ([]*model.ParseRun, error) {
	return f.runs, nil
}

type fakeChunkingRunRepository struct {
	runs []*model.ChunkingRun
}

func (f *fakeChunkingRunRepository) Create(context.Context, *model.ChunkingRun) error {
	return nil
}

func (f *fakeChunkingRunRepository) ListByParseRunID(context.Context, uuid.UUID) ([]*model.ChunkingRun, error) {
	return nil, nil
}

func (f *fakeChunkingRunRepository) ListLatestByParseRunIDs(context.Context, []uuid.UUID) ([]*model.ChunkingRun, error) {
	return f.runs, nil
}

func TestDatasetShadowSummaryChunkQualityReady(t *testing.T) {
	datasetID := uuid.New()
	parseRuns := make([]*model.ParseRun, 0, 5)
	chunkingRuns := make([]*model.ChunkingRun, 0, 5)
	for i := 0; i < 5; i++ {
		run := newDatasetShadowParseRun(datasetID, "succeeded", 1200+i, 20+i)
		parseRuns = append(parseRuns, run)
		chunkingRuns = append(chunkingRuns, newDatasetShadowChunkingRun(run.ID, 92, "high", nil, 0.98, -2, true))
	}

	summary, err := NewRunQueryService(
		&fakeParseRunRepository{runs: parseRuns},
		&fakeChunkingRunRepository{runs: chunkingRuns},
	).GetLatestDatasetShadowSummary(context.Background(), datasetID, 200)
	if err != nil {
		t.Fatalf("summary error: %v", err)
	}
	if summary.ChunkQuality.Decision != "ready" {
		t.Fatalf("decision=%q reasons=%v", summary.ChunkQuality.Decision, summary.ChunkQuality.Reasons)
	}
	if summary.Readiness.Decision != "ready" {
		t.Fatalf("readiness=%q reasons=%v", summary.Readiness.Decision, summary.Readiness.Reasons)
	}
	if summary.ChunkQuality.EvaluatedDocumentCount != 5 {
		t.Fatalf("evaluated=%d", summary.ChunkQuality.EvaluatedDocumentCount)
	}
	if summary.ChunkQuality.HighCount != 5 {
		t.Fatalf("high=%d", summary.ChunkQuality.HighCount)
	}
	if summary.LatestDocuments[0].ChunkQualityScore == nil || *summary.LatestDocuments[0].ChunkQualityScore != 92 {
		t.Fatalf("latest document quality=%v", summary.LatestDocuments[0].ChunkQualityScore)
	}
	if summary.LatestDocuments[0].ReadinessDecision != "ready" {
		t.Fatalf("document readiness=%q reasons=%v", summary.LatestDocuments[0].ReadinessDecision, summary.LatestDocuments[0].ReadinessReasons)
	}
}

func TestDatasetShadowSummaryChunkQualityObserveWhenSampleSmall(t *testing.T) {
	datasetID := uuid.New()
	run := newDatasetShadowParseRun(datasetID, "succeeded", 1200, 20)

	summary, err := NewRunQueryService(
		&fakeParseRunRepository{runs: []*model.ParseRun{run}},
		&fakeChunkingRunRepository{runs: []*model.ChunkingRun{newDatasetShadowChunkingRun(run.ID, 95, "high", nil, 0.99, -1, true)}},
	).GetLatestDatasetShadowSummary(context.Background(), datasetID, 200)
	if err != nil {
		t.Fatalf("summary error: %v", err)
	}
	if summary.ChunkQuality.Decision != "observe" {
		t.Fatalf("decision=%q reasons=%v", summary.ChunkQuality.Decision, summary.ChunkQuality.Reasons)
	}
	if summary.Readiness.Decision != "observe" {
		t.Fatalf("readiness=%q reasons=%v", summary.Readiness.Decision, summary.Readiness.Reasons)
	}
	if !containsString(summary.ChunkQuality.Reasons, "small_shadow_sample") {
		t.Fatalf("reasons=%v", summary.ChunkQuality.Reasons)
	}
	if !containsString(summary.Readiness.Reasons, "small_shadow_sample") {
		t.Fatalf("readiness reasons=%v", summary.Readiness.Reasons)
	}
}

func TestDatasetShadowSummaryChunkQualityBlockedOnLargeTextLoss(t *testing.T) {
	datasetID := uuid.New()
	run := newDatasetShadowParseRun(datasetID, "succeeded", 300, 8)
	chunkingRun := newDatasetShadowChunkingRun(run.ID, 45, "failed", []string{"large_text_loss"}, 0.2, -5, true)

	summary, err := NewRunQueryService(
		&fakeParseRunRepository{runs: []*model.ParseRun{run}},
		&fakeChunkingRunRepository{runs: []*model.ChunkingRun{chunkingRun}},
	).GetLatestDatasetShadowSummary(context.Background(), datasetID, 200)
	if err != nil {
		t.Fatalf("summary error: %v", err)
	}
	if summary.ChunkQuality.Decision != "blocked" {
		t.Fatalf("decision=%q reasons=%v", summary.ChunkQuality.Decision, summary.ChunkQuality.Reasons)
	}
	if summary.Readiness.Decision != "blocked" {
		t.Fatalf("readiness=%q reasons=%v", summary.Readiness.Decision, summary.Readiness.Reasons)
	}
	if summary.ChunkQuality.LargeTextLossCount != 1 {
		t.Fatalf("large text loss=%d", summary.ChunkQuality.LargeTextLossCount)
	}
	if summary.LatestDocuments[0].ReadinessDecision != "blocked" {
		t.Fatalf("document readiness=%q reasons=%v", summary.LatestDocuments[0].ReadinessDecision, summary.LatestDocuments[0].ReadinessReasons)
	}
	if len(summary.ChunkQuality.TopRiskDocuments) != 1 {
		t.Fatalf("top risk documents=%d", len(summary.ChunkQuality.TopRiskDocuments))
	}
}

func TestDatasetShadowReadinessUnknownWhenChunkQualityMissing(t *testing.T) {
	datasetID := uuid.New()
	run := newDatasetShadowParseRun(datasetID, "succeeded", 1200, 20)

	summary, err := NewRunQueryService(
		&fakeParseRunRepository{runs: []*model.ParseRun{run}},
		&fakeChunkingRunRepository{runs: nil},
	).GetLatestDatasetShadowSummary(context.Background(), datasetID, 200)
	if err != nil {
		t.Fatalf("summary error: %v", err)
	}
	if summary.Readiness.Decision != "unknown" {
		t.Fatalf("readiness=%q reasons=%v", summary.Readiness.Decision, summary.Readiness.Reasons)
	}
	if summary.LatestDocuments[0].ReadinessDecision != "unknown" {
		t.Fatalf("document readiness=%q reasons=%v", summary.LatestDocuments[0].ReadinessDecision, summary.LatestDocuments[0].ReadinessReasons)
	}
}

func TestDatasetShadowReadinessUnknownWhenSourceUnavailable(t *testing.T) {
	datasetID := uuid.New()
	run := newDatasetShadowParseRun(datasetID, "failed", 0, 0)
	run.SummaryJSON["error"] = "download file: failed to load file: no such file or directory"

	summary, err := NewRunQueryService(
		&fakeParseRunRepository{runs: []*model.ParseRun{run}},
		&fakeChunkingRunRepository{runs: nil},
	).GetLatestDatasetShadowSummary(context.Background(), datasetID, 200)
	if err != nil {
		t.Fatalf("summary error: %v", err)
	}
	if summary.Readiness.Decision != "unknown" {
		t.Fatalf("readiness=%q reasons=%v", summary.Readiness.Decision, summary.Readiness.Reasons)
	}
	if summary.LatestDocuments[0].ReadinessDecision != "unknown" {
		t.Fatalf("document readiness=%q reasons=%v", summary.LatestDocuments[0].ReadinessDecision, summary.LatestDocuments[0].ReadinessReasons)
	}
	if !containsString(summary.LatestDocuments[0].ReadinessReasons, "source_unavailable") {
		t.Fatalf("document reasons=%v", summary.LatestDocuments[0].ReadinessReasons)
	}
}

func newDatasetShadowParseRun(datasetID uuid.UUID, status string, textLength, elementCount int) *model.ParseRun {
	documentID := uuid.New()
	return &model.ParseRun{
		ID:               uuid.New(),
		DatasetID:        &datasetID,
		DocumentID:       &documentID,
		Status:           status,
		QualityLevel:     "high",
		FinalProviderKey: "local",
		SummaryJSON: map[string]any{
			"text_length":   textLength,
			"element_count": elementCount,
		},
	}
}

func newDatasetShadowChunkingRun(parseRunID uuid.UUID, score int, label string, warnings []string, retention float64, chunkDelta int, stable bool) *model.ChunkingRun {
	warningItems := make([]any, 0, len(warnings))
	for _, warning := range warnings {
		warningItems = append(warningItems, warning)
	}
	return &model.ChunkingRun{
		ID:           uuid.New(),
		ParseRunID:   parseRunID,
		ParentMode:   "section",
		Segmentation: "section_aware",
		PlanJSON: map[string]any{
			"quality_score": map[string]any{
				"overall":  score,
				"label":    label,
				"warnings": warningItems,
			},
			"comparison": map[string]any{
				"text_retention_ratio": retention,
				"unit_count_delta":     chunkDelta,
			},
			"execution": map[string]any{
				"stable_order": stable,
			},
		},
	}
}

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestParseArtifactQualityServiceBuildsConfirmationItems(t *testing.T) {
	low := 0.7
	tableConfidence := 0.95
	imageConfidence := 0.75
	assetID := uuid.New()
	runID := uuid.New()
	svc := NewParseArtifactQualityService(&parseArtifactQualityItemRepo{})

	items, err := svc.BuildConfirmationItems(ParseArtifactQualityInput{
		OrganizationID:      "org-1",
		AssetID:             assetID,
		ProcessingRunID:     runID,
		GenerationNo:        2,
		CreatedBy:           "account-1",
		SourceFileExtension: "pdf",
		Artifact: &contracts.ParseArtifact{
			ArtifactID: "artifact-1",
			Elements: []contracts.ParsedElement{
				{
					ID:         "text-1",
					Type:       "text",
					Page:       1,
					Ordinal:    1,
					Content:    "uncertain text",
					Confidence: &low,
				},
				{
					ID:         "table-1",
					Type:       "table",
					Page:       1,
					Ordinal:    2,
					Content:    "| a | b |",
					Confidence: &tableConfidence,
					Metadata: map[string]any{
						"table_rows":       4,
						"table_columns":    3,
						"empty_cell_ratio": 0.5,
					},
				},
				{
					ID:         "image-1",
					Type:       "image",
					Page:       2,
					Ordinal:    3,
					Content:    "ocr text",
					Confidence: &imageConfidence,
					Metadata: map[string]any{
						"ocr_fallback": true,
					},
				},
				{
					ID:      "clean-1",
					Type:    "text",
					Page:    2,
					Ordinal: 4,
					Content: "clean",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildConfirmationItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items=%d want 3", len(items))
	}
	if items[0].ItemType != model.ParseConfirmationItemTypeLowConfidenceText ||
		items[0].SourceLocatorJSON["artifact_element_id"] != "text-1" ||
		items[0].ReviewReason == nil ||
		*items[0].ReviewReason != parseQualityReasonLowConfidenceText {
		t.Fatalf("text item=%+v", items[0])
	}
	if items[1].ItemType != model.ParseConfirmationItemTypeTable ||
		items[1].ReviewReason == nil ||
		*items[1].ReviewReason != parseQualityReasonTableStructureRisk {
		t.Fatalf("table item=%+v", items[1])
	}
	if items[2].ItemType != model.ParseConfirmationItemTypeImageOCR ||
		items[2].ReviewReason == nil ||
		!strings.Contains(*items[2].ReviewReason, parseQualityReasonLowConfidenceImageOCR) ||
		!strings.Contains(*items[2].ReviewReason, parseQualityReasonOCRFallback) {
		t.Fatalf("image item=%+v", items[2])
	}
}

func TestParseArtifactQualityServiceCreatesConfirmationItems(t *testing.T) {
	confidence := 0.5
	repo := &parseArtifactQualityItemRepo{}
	svc := NewParseArtifactQualityService(repo)
	result, err := svc.CreateConfirmationItems(context.Background(), ParseArtifactQualityInput{
		OrganizationID:     "org-1",
		AssetID:            uuid.New(),
		ProcessingRunID:    uuid.New(),
		GenerationNo:       1,
		SourceFileMimeType: "application/pdf",
		Artifact: &contracts.ParseArtifact{
			Elements: []contracts.ParsedElement{
				{ID: "text-1", Type: "text", Content: "needs review", Confidence: &confidence},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateConfirmationItems: %v", err)
	}
	if result.PendingCount != 1 || len(repo.created) != 1 {
		t.Fatalf("result=%+v created=%d", result, len(repo.created))
	}
}

func TestParseArtifactQualityServiceSkipsNonPDFConfirmationItems(t *testing.T) {
	confidence := 0.5
	repo := &parseArtifactQualityItemRepo{}
	svc := NewParseArtifactQualityService(repo)

	result, err := svc.CreateConfirmationItems(context.Background(), ParseArtifactQualityInput{
		OrganizationID:      "org-1",
		AssetID:             uuid.New(),
		ProcessingRunID:     uuid.New(),
		GenerationNo:        1,
		SourceFileExtension: "docx",
		SourceFileMimeType:  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Artifact: &contracts.ParseArtifact{
			Elements: []contracts.ParsedElement{
				{ID: "text-1", Type: "text", Content: "needs review", Confidence: &confidence},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateConfirmationItems: %v", err)
	}
	if result.PendingCount != 0 || len(repo.created) != 0 {
		t.Fatalf("result=%+v created=%d", result, len(repo.created))
	}
}

type parseArtifactQualityItemRepo struct {
	created []*model.ParseConfirmationItem
}

func (r *parseArtifactQualityItemRepo) Create(ctx context.Context, item *model.ParseConfirmationItem) error {
	if err := item.BeforeCreate(nil); err != nil {
		return err
	}
	r.created = append(r.created, item)
	return nil
}

func (r *parseArtifactQualityItemRepo) CreateBatch(ctx context.Context, items []*model.ParseConfirmationItem) error {
	for _, item := range items {
		if err := item.BeforeCreate(nil); err != nil {
			return err
		}
	}
	r.created = append(r.created, items...)
	return nil
}

func (r *parseArtifactQualityItemRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.ParseConfirmationItem, error) {
	return nil, nil
}

func (r *parseArtifactQualityItemRepo) List(ctx context.Context, filter repository.ParseConfirmationItemListFilter) ([]*model.ParseConfirmationItem, int64, error) {
	return r.created, int64(len(r.created)), nil
}

func (r *parseArtifactQualityItemRepo) CountPendingByRun(ctx context.Context, organizationID string, assetID uuid.UUID, processingRunID uuid.UUID, generationNo int64) (int64, error) {
	var count int64
	for _, item := range r.created {
		if item.Status == model.ParseConfirmationItemStatusPending {
			count++
		}
	}
	return count, nil
}

func (r *parseArtifactQualityItemRepo) Resolve(ctx context.Context, id uuid.UUID, patch repository.ParseConfirmationItemResolvePatch) (*model.ParseConfirmationItem, error) {
	now := time.Now()
	return &model.ParseConfirmationItem{ID: id, Status: patch.Status, ResolvedAt: &now}, nil
}

var _ repository.ParseConfirmationItemRepository = (*parseArtifactQualityItemRepo)(nil)

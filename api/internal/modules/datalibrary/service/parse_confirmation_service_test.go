package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestParseConfirmationServiceResolveEditTriggersGenerateWhenPendingCleared(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	itemID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusConfirming,
			ProcessingRunID: &runID,
			GenerationNo:    2,
		},
	}
	itemRepo := newParseConfirmationServiceItemRepo([]*model.ParseConfirmationItem{
		{
			ID:              itemID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    2,
			ItemType:        model.ParseConfirmationItemTypeLowConfidenceText,
			Status:          model.ParseConfirmationItemStatusPending,
			OriginalContent: "teh",
		},
	})
	svc := NewParseConfirmationService(assetRepo, itemRepo)
	finalContent := "the"

	result, err := svc.ResolveCurrentConfirmationItem(context.Background(), ParseConfirmationResolveInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ItemID:         itemID,
		Action:         ParseConfirmationActionEdit,
		FinalContent:   &finalContent,
		UpdatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("ResolveCurrentConfirmationItem: %v", err)
	}
	if !result.ShouldGenerate || result.PendingCount != 0 {
		t.Fatalf("result=%+v", result)
	}
	if result.Item.Status != model.ParseConfirmationItemStatusEdited || result.Item.FinalContent == nil || *result.Item.FinalContent != finalContent {
		t.Fatalf("item=%+v", result.Item)
	}
}

func TestParseConfirmationServiceBatchIgnoreAllPending(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusConfirming,
			ProcessingRunID: &runID,
			GenerationNo:    2,
		},
	}
	itemRepo := newParseConfirmationServiceItemRepo([]*model.ParseConfirmationItem{
		{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    2,
			Status:          model.ParseConfirmationItemStatusPending,
			OriginalContent: "a",
		},
		{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    2,
			Status:          model.ParseConfirmationItemStatusPending,
			OriginalContent: "b",
		},
	})
	svc := NewParseConfirmationService(assetRepo, itemRepo)

	result, err := svc.BatchIgnoreCurrentConfirmationItems(context.Background(), ParseConfirmationBatchIgnoreInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		UpdatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("BatchIgnoreCurrentConfirmationItems: %v", err)
	}
	if len(result.Items) != 2 || result.PendingCount != 0 || !result.ShouldGenerate {
		t.Fatalf("result=%+v", result)
	}
}

type parseConfirmationServiceItemRepo struct {
	items map[uuid.UUID]*model.ParseConfirmationItem
}

func newParseConfirmationServiceItemRepo(items []*model.ParseConfirmationItem) *parseConfirmationServiceItemRepo {
	repo := &parseConfirmationServiceItemRepo{items: map[uuid.UUID]*model.ParseConfirmationItem{}}
	for _, item := range items {
		if item.ID == uuid.Nil {
			item.ID = uuid.New()
		}
		if item.Status == "" {
			item.Status = model.ParseConfirmationItemStatusPending
		}
		repo.items[item.ID] = cloneParseConfirmationItem(item)
	}
	return repo
}

func (r *parseConfirmationServiceItemRepo) Create(ctx context.Context, item *model.ParseConfirmationItem) error {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	r.items[item.ID] = cloneParseConfirmationItem(item)
	return nil
}

func (r *parseConfirmationServiceItemRepo) CreateBatch(ctx context.Context, items []*model.ParseConfirmationItem) error {
	for _, item := range items {
		if err := r.Create(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *parseConfirmationServiceItemRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.ParseConfirmationItem, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, nil
	}
	return cloneParseConfirmationItem(item), nil
}

func (r *parseConfirmationServiceItemRepo) List(ctx context.Context, filter repository.ParseConfirmationItemListFilter) ([]*model.ParseConfirmationItem, int64, error) {
	matched := make([]*model.ParseConfirmationItem, 0)
	for _, item := range r.items {
		if filter.OrganizationID != "" && item.OrganizationID != filter.OrganizationID {
			continue
		}
		if filter.AssetID != uuid.Nil && item.AssetID != filter.AssetID {
			continue
		}
		if filter.ProcessingRunID != uuid.Nil && item.ProcessingRunID != filter.ProcessingRunID {
			continue
		}
		if filter.GenerationNo != nil && item.GenerationNo != *filter.GenerationNo {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		matched = append(matched, cloneParseConfirmationItem(item))
	}
	total := int64(len(matched))
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= len(matched) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end], total, nil
}

func (r *parseConfirmationServiceItemRepo) CountPendingByRun(ctx context.Context, organizationID string, assetID uuid.UUID, processingRunID uuid.UUID, generationNo int64) (int64, error) {
	var count int64
	for _, item := range r.items {
		if item.OrganizationID == organizationID &&
			item.AssetID == assetID &&
			item.ProcessingRunID == processingRunID &&
			item.GenerationNo == generationNo &&
			item.Status == model.ParseConfirmationItemStatusPending {
			count++
		}
	}
	return count, nil
}

func (r *parseConfirmationServiceItemRepo) Resolve(ctx context.Context, id uuid.UUID, patch repository.ParseConfirmationItemResolvePatch) (*model.ParseConfirmationItem, error) {
	item, ok := r.items[id]
	if !ok || !parseConfirmationResolvePatchMatches(item, patch) {
		return nil, nil
	}
	if len(patch.AllowedFrom) > 0 {
		allowed := false
		for _, status := range patch.AllowedFrom {
			if item.Status == status {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, nil
		}
	}
	item.Status = patch.Status
	item.FinalContent = patch.FinalContent
	item.UpdatedBy = patch.UpdatedBy
	now := time.Now()
	item.ResolvedAt = &now
	return cloneParseConfirmationItem(item), nil
}

func parseConfirmationResolvePatchMatches(item *model.ParseConfirmationItem, patch repository.ParseConfirmationItemResolvePatch) bool {
	if patch.OrganizationID != "" && item.OrganizationID != patch.OrganizationID {
		return false
	}
	if patch.AssetID != uuid.Nil && item.AssetID != patch.AssetID {
		return false
	}
	if patch.ProcessingRunID != uuid.Nil && item.ProcessingRunID != patch.ProcessingRunID {
		return false
	}
	if patch.GenerationNo != nil && item.GenerationNo != *patch.GenerationNo {
		return false
	}
	return true
}

func cloneParseConfirmationItem(item *model.ParseConfirmationItem) *model.ParseConfirmationItem {
	if item == nil {
		return nil
	}
	copied := *item
	if item.SourceLocatorJSON != nil {
		copied.SourceLocatorJSON = map[string]any{}
		for key, value := range item.SourceLocatorJSON {
			copied.SourceLocatorJSON[key] = value
		}
	}
	return &copied
}

var _ repository.ParseConfirmationItemRepository = (*parseConfirmationServiceItemRepo)(nil)

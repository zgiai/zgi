package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

const (
	ParseConfirmationActionKeep   = "keep"
	ParseConfirmationActionEdit   = "edit"
	ParseConfirmationActionIgnore = "ignore"
)

var (
	ErrParseConfirmationItemNotFound         = errors.New("parse confirmation item not found")
	ErrParseConfirmationActionInvalid        = errors.New("parse confirmation action is invalid")
	ErrParseConfirmationFinalContentRequired = errors.New("final_content is required")
	ErrParseConfirmationStateInvalid         = errors.New("parse confirmation state is invalid")
)

type ParseConfirmationService interface {
	ListCurrentConfirmationItems(ctx context.Context, input ParseConfirmationListInput) (*ParseConfirmationListView, error)
	ResolveCurrentConfirmationItem(ctx context.Context, input ParseConfirmationResolveInput) (*ParseConfirmationResolveResult, error)
	BatchIgnoreCurrentConfirmationItems(ctx context.Context, input ParseConfirmationBatchIgnoreInput) (*ParseConfirmationBatchIgnoreResult, error)
}

type ParseConfirmationListInput struct {
	OrganizationID string
	SourceFileID   string
	Status         string
	Limit          int
	Offset         int
}

type ParseConfirmationListView struct {
	AssetID         uuid.UUID                      `json:"asset_id"`
	FileID          string                         `json:"file_id"`
	ProductStatus   string                         `json:"product_status"`
	ProcessingRunID *uuid.UUID                     `json:"processing_run_id,omitempty"`
	GenerationNo    int64                          `json:"generation_no"`
	Items           []*model.ParseConfirmationItem `json:"items"`
	Total           int64                          `json:"total"`
	PendingCount    int64                          `json:"pending_count"`
}

type ParseConfirmationResolveInput struct {
	OrganizationID string
	SourceFileID   string
	ItemID         uuid.UUID
	Action         string
	FinalContent   *string
	UpdatedBy      string
}

type ParseConfirmationResolveResult struct {
	Asset          *model.DocumentAsset         `json:"asset"`
	Item           *model.ParseConfirmationItem `json:"item"`
	PendingCount   int64                        `json:"pending_count"`
	ShouldGenerate bool                         `json:"should_generate"`
}

type ParseConfirmationBatchIgnoreInput struct {
	OrganizationID string
	SourceFileID   string
	ItemIDs        []uuid.UUID
	UpdatedBy      string
}

type ParseConfirmationBatchIgnoreResult struct {
	Asset          *model.DocumentAsset           `json:"asset"`
	Items          []*model.ParseConfirmationItem `json:"items"`
	PendingCount   int64                          `json:"pending_count"`
	ShouldGenerate bool                           `json:"should_generate"`
}

type parseConfirmationService struct {
	assets repository.DocumentAssetRepository
	items  repository.ParseConfirmationItemRepository
}

func NewParseConfirmationService(assets repository.DocumentAssetRepository, items repository.ParseConfirmationItemRepository) ParseConfirmationService {
	return &parseConfirmationService{assets: assets, items: items}
}

func (s *parseConfirmationService) ListCurrentConfirmationItems(ctx context.Context, input ParseConfirmationListInput) (*ParseConfirmationListView, error) {
	asset, err := s.loadCurrentConfirmingAsset(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	generationNo := asset.GenerationNo
	items, total, err := s.items.List(ctx, repository.ParseConfirmationItemListFilter{
		OrganizationID:  input.OrganizationID,
		AssetID:         asset.ID,
		ProcessingRunID: *asset.ProcessingRunID,
		GenerationNo:    &generationNo,
		Status:          input.Status,
		Limit:           input.Limit,
		Offset:          input.Offset,
	})
	if err != nil {
		return nil, err
	}
	pending, err := s.items.CountPendingByRun(ctx, input.OrganizationID, asset.ID, *asset.ProcessingRunID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	return &ParseConfirmationListView{
		AssetID:         asset.ID,
		FileID:          asset.SourceFileID,
		ProductStatus:   asset.ProductStatus,
		ProcessingRunID: asset.ProcessingRunID,
		GenerationNo:    asset.GenerationNo,
		Items:           items,
		Total:           total,
		PendingCount:    pending,
	}, nil
}

func (s *parseConfirmationService) ResolveCurrentConfirmationItem(ctx context.Context, input ParseConfirmationResolveInput) (*ParseConfirmationResolveResult, error) {
	asset, err := s.loadCurrentConfirmingAsset(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	status, finalContent, err := normalizeParseConfirmationAction(input.Action, input.FinalContent)
	if err != nil {
		return nil, err
	}
	generationNo := asset.GenerationNo
	item, err := s.items.Resolve(ctx, input.ItemID, repository.ParseConfirmationItemResolvePatch{
		OrganizationID:  input.OrganizationID,
		AssetID:         asset.ID,
		ProcessingRunID: *asset.ProcessingRunID,
		GenerationNo:    &generationNo,
		Status:          status,
		FinalContent:    finalContent,
		UpdatedBy:       input.UpdatedBy,
		AllowedFrom:     []string{model.ParseConfirmationItemStatusPending},
	})
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrParseConfirmationItemNotFound
	}
	pending, err := s.items.CountPendingByRun(ctx, input.OrganizationID, asset.ID, *asset.ProcessingRunID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	return &ParseConfirmationResolveResult{
		Asset:          asset,
		Item:           item,
		PendingCount:   pending,
		ShouldGenerate: pending == 0,
	}, nil
}

func (s *parseConfirmationService) BatchIgnoreCurrentConfirmationItems(ctx context.Context, input ParseConfirmationBatchIgnoreInput) (*ParseConfirmationBatchIgnoreResult, error) {
	asset, err := s.loadCurrentConfirmingAsset(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	pendingItems, err := s.listPendingItemsForBatch(ctx, asset, input.ItemIDs)
	if err != nil {
		return nil, err
	}
	resolved := make([]*model.ParseConfirmationItem, 0, len(pendingItems))
	generationNo := asset.GenerationNo
	for _, item := range pendingItems {
		updated, err := s.items.Resolve(ctx, item.ID, repository.ParseConfirmationItemResolvePatch{
			OrganizationID:  input.OrganizationID,
			AssetID:         asset.ID,
			ProcessingRunID: *asset.ProcessingRunID,
			GenerationNo:    &generationNo,
			Status:          model.ParseConfirmationItemStatusIgnored,
			UpdatedBy:       input.UpdatedBy,
			AllowedFrom:     []string{model.ParseConfirmationItemStatusPending},
		})
		if err != nil {
			return nil, err
		}
		if updated != nil {
			resolved = append(resolved, updated)
		}
	}
	pending, err := s.items.CountPendingByRun(ctx, input.OrganizationID, asset.ID, *asset.ProcessingRunID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	return &ParseConfirmationBatchIgnoreResult{
		Asset:          asset,
		Items:          resolved,
		PendingCount:   pending,
		ShouldGenerate: len(resolved) > 0 && pending == 0,
	}, nil
}

func (s *parseConfirmationService) loadCurrentConfirmingAsset(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if sourceFileID == "" {
		return nil, ErrSourceFileIDRequired
	}
	asset, err := s.assets.FindAssetBySourceFileID(ctx, organizationID, sourceFileID)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.ProcessingRunID == nil || asset.GenerationNo == 0 {
		return nil, ErrProcessingRunMismatch
	}
	if asset.ProductStatus != model.DocumentAssetProductStatusConfirming {
		return nil, ErrParseConfirmationStateInvalid
	}
	return asset, nil
}

func (s *parseConfirmationService) listPendingItemsForBatch(ctx context.Context, asset *model.DocumentAsset, itemIDs []uuid.UUID) ([]*model.ParseConfirmationItem, error) {
	generationNo := asset.GenerationNo
	allPending := make([]*model.ParseConfirmationItem, 0)
	offset := 0
	for {
		page, total, err := s.items.List(ctx, repository.ParseConfirmationItemListFilter{
			OrganizationID:  asset.OrganizationID,
			AssetID:         asset.ID,
			ProcessingRunID: *asset.ProcessingRunID,
			GenerationNo:    &generationNo,
			Status:          model.ParseConfirmationItemStatusPending,
			Limit:           200,
			Offset:          offset,
		})
		if err != nil {
			return nil, err
		}
		allPending = append(allPending, page...)
		offset += len(page)
		if int64(offset) >= total || len(page) == 0 {
			break
		}
	}
	if len(itemIDs) == 0 {
		return allPending, nil
	}
	allowed := make(map[uuid.UUID]struct{}, len(itemIDs))
	for _, id := range itemIDs {
		if id != uuid.Nil {
			allowed[id] = struct{}{}
		}
	}
	filtered := make([]*model.ParseConfirmationItem, 0, len(allPending))
	for _, item := range allPending {
		if _, ok := allowed[item.ID]; ok {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) != len(allowed) {
		return nil, ErrParseConfirmationItemNotFound
	}
	return filtered, nil
}

func normalizeParseConfirmationAction(action string, finalContent *string) (string, *string, error) {
	switch action {
	case ParseConfirmationActionKeep:
		return model.ParseConfirmationItemStatusKept, nil, nil
	case ParseConfirmationActionIgnore:
		return model.ParseConfirmationItemStatusIgnored, nil, nil
	case ParseConfirmationActionEdit:
		if finalContent == nil {
			return "", nil, ErrParseConfirmationFinalContentRequired
		}
		return model.ParseConfirmationItemStatusEdited, finalContent, nil
	default:
		return "", nil, ErrParseConfirmationActionInvalid
	}
}

package service

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparserepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var ErrParsePreviewNotReady = errors.New("parse preview is not ready")

type ParsePreviewService interface {
	GetParsePreview(ctx context.Context, input ParsePreviewInput) (*ParsePreviewView, error)
}

type ParsePreviewInput struct {
	OrganizationID string
	SourceFileID   string
}

type ParsePreviewView struct {
	AssetID                  uuid.UUID                   `json:"asset_id"`
	FileID                   string                      `json:"file_id"`
	ProductStatus            string                      `json:"product_status"`
	ProcessingRunID          *uuid.UUID                  `json:"processing_run_id,omitempty"`
	GenerationNo             int64                       `json:"generation_no"`
	ParseArtifactID          uuid.UUID                   `json:"parse_artifact_id"`
	ArtifactStatus           contracts.ParseStatus       `json:"artifact_status"`
	ArtifactQualityLevel     contracts.ParseQualityLevel `json:"artifact_quality_level"`
	EngineUsed               contracts.ParseEngine       `json:"engine_used,omitempty"`
	Text                     string                      `json:"text,omitempty"`
	Markdown                 string                      `json:"markdown,omitempty"`
	Elements                 []ParsePreviewElement       `json:"elements"`
	ConfirmationItems        []ParsePreviewConfirmation  `json:"confirmation_items"`
	TotalConfirmationCount   int                         `json:"total_confirmation_count"`
	PendingConfirmationCount int                         `json:"pending_confirmation_count"`
}

type ParsePreviewElement struct {
	ID           string                      `json:"id,omitempty"`
	Type         string                      `json:"type"`
	Subtype      string                      `json:"subtype,omitempty"`
	Page         int                         `json:"page"`
	Content      string                      `json:"content,omitempty"`
	BBox         *contracts.ParseBoundingBox `json:"bbox,omitempty"`
	Ordinal      int                         `json:"ordinal"`
	Precision    string                      `json:"precision,omitempty"`
	Confidence   *float64                    `json:"confidence,omitempty"`
	Metadata     map[string]any              `json:"metadata,omitempty"`
	Confirmation *ParsePreviewConfirmation   `json:"confirmation,omitempty"`
}

type ParsePreviewConfirmation struct {
	ID                uuid.UUID      `json:"id"`
	ArtifactElementID string         `json:"artifact_element_id,omitempty"`
	ElementIndex      *int64         `json:"element_index,omitempty"`
	ItemType          string         `json:"item_type"`
	Status            string         `json:"status"`
	OriginalContent   string         `json:"original_content"`
	SuggestedContent  *string        `json:"suggested_content,omitempty"`
	FinalContent      *string        `json:"final_content,omitempty"`
	Confidence        *float64       `json:"confidence,omitempty"`
	ReviewReason      *string        `json:"review_reason,omitempty"`
	SourceLocator     map[string]any `json:"source_locator,omitempty"`
}

type parsePreviewService struct {
	assets              repository.DocumentAssetRepository
	artifacts           contentparserepo.ArtifactRepository
	artifactPersistence ParseArtifactPersistenceService
	confirmationItems   repository.ParseConfirmationItemRepository
}

func NewParsePreviewService(
	assets repository.DocumentAssetRepository,
	artifacts contentparserepo.ArtifactRepository,
	artifactPersistence ParseArtifactPersistenceService,
	confirmationItems repository.ParseConfirmationItemRepository,
) ParsePreviewService {
	return &parsePreviewService{
		assets:              assets,
		artifacts:           artifacts,
		artifactPersistence: artifactPersistence,
		confirmationItems:   confirmationItems,
	}
}

func (s *parsePreviewService) GetParsePreview(ctx context.Context, input ParsePreviewInput) (*ParsePreviewView, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.SourceFileID == "" {
		return nil, ErrSourceFileIDRequired
	}
	asset, err := s.assets.FindAssetBySourceFileID(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.ParseArtifactID == nil || *asset.ParseArtifactID == uuid.Nil {
		return nil, ErrParsePreviewNotReady
	}
	artifactRecord, err := s.artifacts.GetByID(ctx, *asset.ParseArtifactID)
	if err != nil {
		return nil, err
	}
	if artifactRecord == nil || artifactRecord.ArtifactStorageKey == "" {
		return nil, ErrParsePreviewNotReady
	}
	artifact, err := s.artifactPersistence.LoadParseArtifact(ctx, artifactRecord.ArtifactStorageKey)
	if err != nil {
		return nil, err
	}
	items, err := s.listCurrentConfirmationItems(ctx, asset)
	if err != nil {
		return nil, err
	}
	confirmations := make([]ParsePreviewConfirmation, 0, len(items))
	byElementID := make(map[string]*ParsePreviewConfirmation, len(items))
	byElementIndex := make(map[int64]*ParsePreviewConfirmation, len(items))
	pendingCount := 0
	for _, item := range items {
		confirmation := parsePreviewConfirmationFromItem(item)
		confirmations = append(confirmations, confirmation)
		if item.Status == model.ParseConfirmationItemStatusPending {
			pendingCount++
		}
		if confirmation.ArtifactElementID != "" {
			copied := confirmation
			byElementID[confirmation.ArtifactElementID] = &copied
		}
		if confirmation.ElementIndex != nil {
			copied := confirmation
			byElementIndex[*confirmation.ElementIndex] = &copied
		}
	}

	elements := make([]ParsePreviewElement, 0, len(artifact.Elements))
	for index, element := range artifact.Elements {
		view := parsePreviewElementFromArtifact(element)
		if element.ID != "" {
			view.Confirmation = byElementID[element.ID]
		}
		if view.Confirmation == nil {
			view.Confirmation = byElementIndex[int64(index)]
		}
		elements = append(elements, view)
	}

	return &ParsePreviewView{
		AssetID:                  asset.ID,
		FileID:                   asset.SourceFileID,
		ProductStatus:            asset.ProductStatus,
		ProcessingRunID:          asset.ProcessingRunID,
		GenerationNo:             asset.GenerationNo,
		ParseArtifactID:          *asset.ParseArtifactID,
		ArtifactStatus:           artifact.Status,
		ArtifactQualityLevel:     artifact.QualityLevel,
		EngineUsed:               artifact.EngineUsed,
		Text:                     artifact.Text,
		Markdown:                 artifact.Markdown,
		Elements:                 elements,
		ConfirmationItems:        confirmations,
		TotalConfirmationCount:   len(confirmations),
		PendingConfirmationCount: pendingCount,
	}, nil
}

func (s *parsePreviewService) listCurrentConfirmationItems(ctx context.Context, asset *model.DocumentAsset) ([]*model.ParseConfirmationItem, error) {
	if asset.ProcessingRunID == nil {
		return nil, nil
	}
	generationNo := asset.GenerationNo
	items := make([]*model.ParseConfirmationItem, 0)
	offset := 0
	for {
		page, total, err := s.confirmationItems.List(ctx, repository.ParseConfirmationItemListFilter{
			OrganizationID:  asset.OrganizationID,
			AssetID:         asset.ID,
			ProcessingRunID: *asset.ProcessingRunID,
			GenerationNo:    &generationNo,
			Limit:           200,
			Offset:          offset,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, page...)
		offset += len(page)
		if int64(offset) >= total || len(page) == 0 {
			break
		}
	}
	return items, nil
}

func parsePreviewElementFromArtifact(element contracts.ParsedElement) ParsePreviewElement {
	return ParsePreviewElement{
		ID:         element.ID,
		Type:       element.Type,
		Subtype:    element.Subtype,
		Page:       element.Page,
		Content:    element.Content,
		BBox:       element.BBox,
		Ordinal:    element.Ordinal,
		Precision:  element.Precision,
		Confidence: element.Confidence,
		Metadata:   element.Metadata,
	}
}

func parsePreviewConfirmationFromItem(item *model.ParseConfirmationItem) ParsePreviewConfirmation {
	confirmation := ParsePreviewConfirmation{
		ID:                item.ID,
		ArtifactElementID: sourceLocatorString(item.SourceLocatorJSON, "artifact_element_id"),
		ElementIndex:      sourceLocatorInt64(item.SourceLocatorJSON, "element_index"),
		ItemType:          item.ItemType,
		Status:            item.Status,
		OriginalContent:   item.OriginalContent,
		SuggestedContent:  item.SuggestedContent,
		FinalContent:      item.FinalContent,
		Confidence:        item.Confidence,
		ReviewReason:      item.ReviewReason,
		SourceLocator:     item.SourceLocatorJSON,
	}
	return confirmation
}

func sourceLocatorString(locator map[string]any, key string) string {
	if locator == nil {
		return ""
	}
	if value, ok := locator[key].(string); ok {
		return value
	}
	return ""
}

func sourceLocatorInt64(locator map[string]any, key string) *int64 {
	if locator == nil {
		return nil
	}
	switch value := locator[key].(type) {
	case int:
		v := int64(value)
		return &v
	case int64:
		v := value
		return &v
	case float64:
		v := int64(value)
		return &v
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

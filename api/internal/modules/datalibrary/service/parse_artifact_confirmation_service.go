package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparserepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var ErrParseConfirmationPatchTargetNotFound = errors.New("parse confirmation patch target not found")

type ParseArtifactConfirmationService interface {
	ApplyResolvedConfirmations(ctx context.Context, input ApplyResolvedConfirmationsInput) (*ApplyResolvedConfirmationsResult, error)
}

type ApplyResolvedConfirmationsInput struct {
	OrganizationID string
	SourceFileID   string
	UpdatedBy      string
}

type ApplyResolvedConfirmationsResult struct {
	Asset              *model.DocumentAsset `json:"asset"`
	ParseArtifactID    uuid.UUID            `json:"parse_artifact_id"`
	ArtifactStorageKey string               `json:"artifact_storage_key"`
	AppliedItemCount   int                  `json:"applied_item_count"`
	EditedItemCount    int                  `json:"edited_item_count"`
}

type parseArtifactConfirmationService struct {
	assets              repository.DocumentAssetRepository
	artifacts           contentparserepo.ArtifactRepository
	artifactPersistence ParseArtifactPersistenceService
	confirmationItems   repository.ParseConfirmationItemRepository
}

func NewParseArtifactConfirmationService(
	assets repository.DocumentAssetRepository,
	artifacts contentparserepo.ArtifactRepository,
	artifactPersistence ParseArtifactPersistenceService,
	confirmationItems repository.ParseConfirmationItemRepository,
) ParseArtifactConfirmationService {
	return &parseArtifactConfirmationService{
		assets:              assets,
		artifacts:           artifacts,
		artifactPersistence: artifactPersistence,
		confirmationItems:   confirmationItems,
	}
}

func (s *parseArtifactConfirmationService) ApplyResolvedConfirmations(ctx context.Context, input ApplyResolvedConfirmationsInput) (*ApplyResolvedConfirmationsResult, error) {
	asset, err := s.loadConfirmingAsset(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil {
		return nil, err
	}
	pending, err := s.confirmationItems.CountPendingByRun(ctx, input.OrganizationID, asset.ID, *asset.ProcessingRunID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	if pending > 0 {
		return nil, ErrParseConfirmationStateInvalid
	}
	items, err := s.listAllConfirmationItems(ctx, asset)
	if err != nil {
		return nil, err
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
	editedCount, err := applyResolvedConfirmationItems(artifact, items)
	if err != nil {
		return nil, err
	}
	updated, err := s.artifactPersistence.UpdateAssetParseArtifact(ctx, UpdateAssetParseArtifactInput{
		OrganizationID:  input.OrganizationID,
		AssetID:         asset.ID,
		ProcessingRunID: *asset.ProcessingRunID,
		GenerationNo:    asset.GenerationNo,
		ArtifactID:      *asset.ParseArtifactID,
		Artifact:        artifact,
		SummaryPatch: map[string]any{
			"confirmed":                 true,
			"confirmed_at":              time.Now().UTC().Format(time.RFC3339Nano),
			"confirmed_by":              input.UpdatedBy,
			"confirmation_item_count":   len(items),
			"confirmation_edited_count": editedCount,
		},
	})
	if err != nil {
		return nil, err
	}
	return &ApplyResolvedConfirmationsResult{
		Asset:              updated.Asset,
		ParseArtifactID:    updated.Artifact.ID,
		ArtifactStorageKey: updated.ArtifactStorageKey,
		AppliedItemCount:   len(items),
		EditedItemCount:    editedCount,
	}, nil
}

func (s *parseArtifactConfirmationService) loadConfirmingAsset(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
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
	if asset.ProductStatus != model.DocumentAssetProductStatusConfirming ||
		asset.ProcessingRunID == nil ||
		asset.GenerationNo == 0 ||
		asset.ParseArtifactID == nil {
		return nil, ErrParseConfirmationStateInvalid
	}
	return asset, nil
}

func (s *parseArtifactConfirmationService) listAllConfirmationItems(ctx context.Context, asset *model.DocumentAsset) ([]*model.ParseConfirmationItem, error) {
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

func applyResolvedConfirmationItems(artifact *contracts.ParseArtifact, items []*model.ParseConfirmationItem) (int, error) {
	if artifact == nil {
		return 0, ErrParseArtifactRequired
	}
	byID := make(map[string]int, len(artifact.Elements))
	for index, element := range artifact.Elements {
		if element.ID != "" {
			byID[element.ID] = index
		}
	}
	editedCount := 0
	rebuildText := false
	rebuildMarkdown := false
	for _, item := range items {
		if item.Status != model.ParseConfirmationItemStatusEdited {
			continue
		}
		if item.FinalContent == nil {
			return 0, ErrParseConfirmationFinalContentRequired
		}
		index, ok := confirmationItemElementIndex(item, byID)
		if !ok || index < 0 || index >= len(artifact.Elements) {
			return 0, ErrParseConfirmationPatchTargetNotFound
		}
		textPatched, markdownPatched := patchArtifactAggregateContent(artifact, item.OriginalContent, *item.FinalContent)
		rebuildText = rebuildText || (artifact.Text != "" && !textPatched)
		rebuildMarkdown = rebuildMarkdown || (artifact.Markdown != "" && !markdownPatched)
		artifact.Elements[index].Content = *item.FinalContent
		editedCount++
	}
	if editedCount > 0 {
		ensureArtifactAggregateContent(artifact, rebuildText, rebuildMarkdown)
	}
	return editedCount, nil
}

func confirmationItemElementIndex(item *model.ParseConfirmationItem, byID map[string]int) (int, bool) {
	elementID := sourceLocatorString(item.SourceLocatorJSON, "artifact_element_id")
	if elementID != "" {
		if index, ok := byID[elementID]; ok {
			return index, true
		}
	}
	elementIndex := sourceLocatorInt64(item.SourceLocatorJSON, "element_index")
	if elementIndex == nil {
		return 0, false
	}
	return int(*elementIndex), true
}

func patchArtifactAggregateContent(artifact *contracts.ParseArtifact, originalContent string, finalContent string) (bool, bool) {
	original := strings.TrimSpace(originalContent)
	if original == "" || artifact == nil {
		return false, false
	}
	textPatched := false
	if artifact.Text != "" {
		artifact.Text, textPatched = replaceFirstContent(artifact.Text, originalContent, original, finalContent)
	}
	markdownPatched := false
	if artifact.Markdown != "" {
		artifact.Markdown, markdownPatched = replaceFirstContent(artifact.Markdown, originalContent, original, finalContent)
	}
	return textPatched, markdownPatched
}

func replaceFirstContent(input string, originalContent string, trimmedOriginal string, finalContent string) (string, bool) {
	if strings.Contains(input, originalContent) {
		return strings.Replace(input, originalContent, finalContent, 1), true
	}
	if originalContent != trimmedOriginal && strings.Contains(input, trimmedOriginal) {
		return strings.Replace(input, trimmedOriginal, finalContent, 1), true
	}
	return input, false
}

func ensureArtifactAggregateContent(artifact *contracts.ParseArtifact, rebuildText bool, rebuildMarkdown bool) {
	if artifact == nil {
		return
	}
	text := strings.TrimSpace(artifact.Text)
	markdown := strings.TrimSpace(artifact.Markdown)
	if text != "" && markdown != "" && !rebuildText && !rebuildMarkdown {
		return
	}
	contents := make([]string, 0, len(artifact.Elements))
	for _, element := range artifact.Elements {
		content := strings.TrimSpace(element.Content)
		if content != "" {
			contents = append(contents, content)
		}
	}
	if len(contents) == 0 {
		return
	}
	joined := strings.Join(contents, "\n")
	if text == "" || rebuildText {
		artifact.Text = joined
	}
	if markdown == "" || rebuildMarkdown {
		artifact.Markdown = joined
	}
}

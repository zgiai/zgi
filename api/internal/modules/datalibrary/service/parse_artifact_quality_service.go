package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var ErrParseConfirmationItemRepositoryRequired = errors.New("parse confirmation item repository is required")

const (
	parseQualityReasonLowConfidenceText     = "low_confidence_text"
	parseQualityReasonLowConfidenceTable    = "low_confidence_table"
	parseQualityReasonLowConfidenceImageOCR = "low_confidence_image_ocr"
	parseQualityReasonReviewRequired        = "review_required"
	parseQualityReasonOCRFallback           = "ocr_fallback"
	parseQualityReasonVLMFallback           = "local_vlm_fallback"
	parseQualityReasonTableStructureRisk    = "table_structure_risk"

	parseQualityTextConfidenceThreshold      = 0.85
	parseQualityTableConfidenceThreshold     = 0.90
	parseQualityImageOCRConfidenceThreshold  = 0.80
	parseQualityTableEmptyCellRatioThreshold = 0.30
)

type ParseArtifactQualityService interface {
	CreateConfirmationItems(ctx context.Context, input ParseArtifactQualityInput) (*ParseArtifactQualityResult, error)
	BuildConfirmationItems(input ParseArtifactQualityInput) ([]*model.ParseConfirmationItem, error)
}

type ParseArtifactQualityInput struct {
	OrganizationID  string
	WorkspaceID     *string
	AssetID         uuid.UUID
	ProcessingRunID uuid.UUID
	GenerationNo    int64
	CreatedBy       string
	Artifact        *contracts.ParseArtifact
}

type ParseArtifactQualityResult struct {
	Items        []*model.ParseConfirmationItem
	PendingCount int64
}

type parseArtifactQualityService struct {
	items repository.ParseConfirmationItemRepository
}

func NewParseArtifactQualityService(items repository.ParseConfirmationItemRepository) ParseArtifactQualityService {
	return &parseArtifactQualityService{items: items}
}

func (s *parseArtifactQualityService) CreateConfirmationItems(ctx context.Context, input ParseArtifactQualityInput) (*ParseArtifactQualityResult, error) {
	if s.items == nil {
		return nil, ErrParseConfirmationItemRepositoryRequired
	}
	items, err := s.BuildConfirmationItems(input)
	if err != nil {
		return nil, err
	}
	if err := s.items.CreateBatch(ctx, items); err != nil {
		return nil, err
	}
	return &ParseArtifactQualityResult{
		Items:        items,
		PendingCount: int64(len(items)),
	}, nil
}

func (s *parseArtifactQualityService) BuildConfirmationItems(input ParseArtifactQualityInput) ([]*model.ParseConfirmationItem, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.ProcessingRunID == uuid.Nil || input.GenerationNo <= 0 {
		return nil, ErrProcessingRunMismatch
	}
	if input.Artifact == nil {
		return nil, ErrParseArtifactRequired
	}

	items := make([]*model.ParseConfirmationItem, 0)
	for index, element := range input.Artifact.Elements {
		reasons := parseElementReviewReasons(element)
		if len(reasons) == 0 {
			continue
		}
		reason := strings.Join(reasons, ",")
		items = append(items, &model.ParseConfirmationItem{
			OrganizationID:    input.OrganizationID,
			WorkspaceID:       input.WorkspaceID,
			AssetID:           input.AssetID,
			ProcessingRunID:   input.ProcessingRunID,
			GenerationNo:      input.GenerationNo,
			ItemType:          parseConfirmationItemType(element),
			Status:            model.ParseConfirmationItemStatusPending,
			SourceLocatorJSON: parseElementSourceLocator(input.Artifact, element, index),
			OriginalContent:   element.Content,
			Confidence:        element.Confidence,
			ReviewReason:      &reason,
			CreatedBy:         input.CreatedBy,
		})
	}
	return items, nil
}

func parseElementReviewReasons(element contracts.ParsedElement) []string {
	reasons := make([]string, 0, 4)
	elementType := strings.TrimSpace(strings.ToLower(element.Type))
	if element.Confidence != nil {
		switch {
		case isTextLikeElement(element) && *element.Confidence < parseQualityTextConfidenceThreshold:
			reasons = append(reasons, parseQualityReasonLowConfidenceText)
		case elementType == "table" && *element.Confidence < parseQualityTableConfidenceThreshold:
			reasons = append(reasons, parseQualityReasonLowConfidenceTable)
		case elementType == "image" && *element.Confidence < parseQualityImageOCRConfidenceThreshold:
			reasons = append(reasons, parseQualityReasonLowConfidenceImageOCR)
		}
	}
	if metadataBool(element.Metadata, "review_required") {
		reasons = append(reasons, parseQualityReasonReviewRequired)
	}
	if isFallbackReviewElement(element) && metadataBool(element.Metadata, "ocr_fallback") {
		reasons = append(reasons, parseQualityReasonOCRFallback)
	}
	if isFallbackReviewElement(element) && metadataBool(element.Metadata, "local_vlm_fallback") {
		reasons = append(reasons, parseQualityReasonVLMFallback)
	}
	if elementType == "table" && hasTableStructureRisk(element.Metadata) {
		reasons = append(reasons, parseQualityReasonTableStructureRisk)
	}
	return uniqueStrings(reasons)
}

func parseConfirmationItemType(element contracts.ParsedElement) string {
	switch strings.TrimSpace(strings.ToLower(element.Type)) {
	case "table":
		return model.ParseConfirmationItemTypeTable
	case "image":
		return model.ParseConfirmationItemTypeImageOCR
	case "text", "title", "heading", "paragraph":
		return model.ParseConfirmationItemTypeLowConfidenceText
	default:
		return model.ParseConfirmationItemTypeStructure
	}
}

func parseElementSourceLocator(artifact *contracts.ParseArtifact, element contracts.ParsedElement, index int) map[string]any {
	elementID := strings.TrimSpace(element.ID)
	if elementID == "" {
		elementID = fmt.Sprintf("element:%d", index)
	}
	locator := map[string]any{
		"artifact_id":         artifact.ArtifactID,
		"artifact_element_id": elementID,
		"element_index":       index,
		"element_type":        element.Type,
		"element_subtype":     element.Subtype,
		"page":                element.Page,
		"ordinal":             element.Ordinal,
	}
	if element.BBox != nil {
		locator["bbox"] = map[string]any{
			"left":   element.BBox.Left,
			"top":    element.BBox.Top,
			"right":  element.BBox.Right,
			"bottom": element.BBox.Bottom,
		}
	}
	return locator
}

func isTextLikeElement(element contracts.ParsedElement) bool {
	switch strings.TrimSpace(strings.ToLower(element.Type)) {
	case "text", "title", "heading", "paragraph":
		return true
	default:
		return false
	}
}

func isFallbackReviewElement(element contracts.ParsedElement) bool {
	switch strings.TrimSpace(strings.ToLower(element.Type)) {
	case "text", "title", "heading", "paragraph", "table", "image":
		return true
	default:
		return false
	}
}

func hasTableStructureRisk(metadata map[string]any) bool {
	if metadataBool(metadata, "table_structure_risk") || metadataBool(metadata, "structure_missing") {
		return true
	}
	rows := metadataInt(metadata, "table_rows")
	columns := metadataInt(metadata, "table_columns")
	if rows == 0 || columns == 0 {
		return true
	}
	return metadataFloat(metadata, "empty_cell_ratio") > parseQualityTableEmptyCellRatioThreshold
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	switch value := metadata[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func metadataFloat(metadata map[string]any, key string) float64 {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case jsonNumber:
		parsed, _ := value.Float64()
		return parsed
	default:
		return 0
	}
}

func metadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case jsonNumber:
		parsed, _ := value.Int64()
		return int(parsed)
	default:
		return 0
	}
}

type jsonNumber interface {
	Float64() (float64, error)
	Int64() (int64, error)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

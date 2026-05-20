package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrExtractionArtifactIDRequired = errors.New("extraction_artifact_id is required")
	ErrExtractionArtifactNotFound   = errors.New("extraction_artifact not found")
)

type ExtractionArtifactService interface {
	CreateArtifact(ctx context.Context, item *model.ExtractionArtifact) (*ExtractionArtifactView, error)
	GetArtifactViewByID(ctx context.Context, id uuid.UUID) (*ExtractionArtifactView, error)
	ListArtifactViews(ctx context.Context, filter repository.ExtractionArtifactListFilter) ([]*ExtractionArtifactView, int64, error)
	LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*ExtractionArtifactView, error)
}

type ExtractionArtifactView struct {
	ID                uuid.UUID      `json:"id"`
	OrganizationID    string         `json:"organization_id"`
	WorkspaceID       *string        `json:"workspace_id,omitempty"`
	AssetID           uuid.UUID      `json:"asset_id"`
	VersionID         uuid.UUID      `json:"version_id"`
	ParseArtifactID   *uuid.UUID     `json:"parse_artifact_id,omitempty"`
	DataSourceID      *string        `json:"data_source_id,omitempty"`
	TableID           *string        `json:"table_id,omitempty"`
	SchemaName        string         `json:"schema_name,omitempty"`
	SchemaHash        string         `json:"schema_hash,omitempty"`
	ExtractorProvider string         `json:"extractor_provider,omitempty"`
	ExtractorModel    string         `json:"extractor_model,omitempty"`
	RecordCount       int64          `json:"record_count"`
	FieldCount        int64          `json:"field_count"`
	EvidenceCount     int64          `json:"evidence_count"`
	Status            string         `json:"status"`
	QualityScore      *float64       `json:"quality_score,omitempty"`
	ContentHash       string         `json:"content_hash,omitempty"`
	OutputURI         string         `json:"output_uri,omitempty"`
	MetadataJSON      map[string]any `json:"metadata_json,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type extractionArtifactService struct {
	repo repository.ExtractionArtifactRepository
}

func NewExtractionArtifactService(repo repository.ExtractionArtifactRepository) ExtractionArtifactService {
	return &extractionArtifactService{repo: repo}
}

func (s *extractionArtifactService) CreateArtifact(ctx context.Context, item *model.ExtractionArtifact) (*ExtractionArtifactView, error) {
	if item == nil || item.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if item.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if item.VersionID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return newExtractionArtifactView(item), nil
}

func (s *extractionArtifactService) GetArtifactViewByID(ctx context.Context, id uuid.UUID) (*ExtractionArtifactView, error) {
	if id == uuid.Nil {
		return nil, ErrExtractionArtifactIDRequired
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrExtractionArtifactNotFound
	}
	return newExtractionArtifactView(item), nil
}

func (s *extractionArtifactService) ListArtifactViews(ctx context.Context, filter repository.ExtractionArtifactListFilter) ([]*ExtractionArtifactView, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*ExtractionArtifactView, 0, len(items))
	for _, item := range items {
		views = append(views, newExtractionArtifactView(item))
	}
	return views, total, nil
}

func (s *extractionArtifactService) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*ExtractionArtifactView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if versionID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	item, err := s.repo.LatestReadyByVersionID(ctx, organizationID, versionID)
	if err != nil || item == nil {
		return nil, err
	}
	return newExtractionArtifactView(item), nil
}

func newExtractionArtifactView(item *model.ExtractionArtifact) *ExtractionArtifactView {
	if item == nil {
		return nil
	}
	return &ExtractionArtifactView{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		WorkspaceID:       item.WorkspaceID,
		AssetID:           item.AssetID,
		VersionID:         item.VersionID,
		ParseArtifactID:   item.ParseArtifactID,
		DataSourceID:      item.DataSourceID,
		TableID:           item.TableID,
		SchemaName:        item.SchemaName,
		SchemaHash:        item.SchemaHash,
		ExtractorProvider: item.ExtractorProvider,
		ExtractorModel:    item.ExtractorModel,
		RecordCount:       item.RecordCount,
		FieldCount:        item.FieldCount,
		EvidenceCount:     item.EvidenceCount,
		Status:            item.Status,
		QualityScore:      item.QualityScore,
		ContentHash:       item.ContentHash,
		OutputURI:         item.OutputURI,
		MetadataJSON:      item.MetadataJSON,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}

package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"github.com/zgiai/ginext/internal/modules/datalibrary/repository"
)

var (
	ErrDatabaseAssetRefIDRequired = errors.New("database_asset_ref_id is required")
	ErrDatabaseAssetRefNotFound   = errors.New("database_asset_ref not found")
	ErrDataSourceIDRequired       = errors.New("data_source_id is required")
)

type DatabaseAssetRefService interface {
	CreateRef(ctx context.Context, item *model.DatabaseAssetRef) (*DatabaseAssetRefView, error)
	GetRefViewByID(ctx context.Context, id uuid.UUID) (*DatabaseAssetRefView, error)
	ListRefViews(ctx context.Context, filter repository.DatabaseAssetRefListFilter) ([]*DatabaseAssetRefView, int64, error)
	FindActiveRefView(ctx context.Context, organizationID string, dataSourceID string, tableID *string, assetID uuid.UUID, versionID uuid.UUID) (*DatabaseAssetRefView, error)
	DisableRef(ctx context.Context, organizationID string, id uuid.UUID) (*DatabaseAssetRefView, error)
}

type DatabaseAssetRefView struct {
	ID                   uuid.UUID      `json:"id"`
	OrganizationID       string         `json:"organization_id"`
	WorkspaceID          *string        `json:"workspace_id,omitempty"`
	DataSourceID         string         `json:"data_source_id"`
	TableID              *string        `json:"table_id,omitempty"`
	AssetID              uuid.UUID      `json:"asset_id"`
	VersionID            uuid.UUID      `json:"version_id"`
	ParseArtifactID      *uuid.UUID     `json:"parse_artifact_id,omitempty"`
	ExtractionArtifactID *uuid.UUID     `json:"extraction_artifact_id,omitempty"`
	Status               string         `json:"status"`
	MetadataJSON         map[string]any `json:"metadata_json,omitempty"`
	CreatedBy            string         `json:"created_by,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
}

type databaseAssetRefService struct {
	repo      repository.DatabaseAssetRefRepository
	reuseRepo repository.ReuseEventRepository
}

func NewDatabaseAssetRefService(repo repository.DatabaseAssetRefRepository, reuseRepos ...repository.ReuseEventRepository) DatabaseAssetRefService {
	var reuseRepo repository.ReuseEventRepository
	if len(reuseRepos) > 0 {
		reuseRepo = reuseRepos[0]
	}
	return &databaseAssetRefService{repo: repo, reuseRepo: reuseRepo}
}

func (s *databaseAssetRefService) CreateRef(ctx context.Context, item *model.DatabaseAssetRef) (*DatabaseAssetRefView, error) {
	if err := validateDatabaseAssetRef(item); err != nil {
		return nil, err
	}
	existing, err := s.repo.FindActive(ctx, item.OrganizationID, item.DataSourceID, item.TableID, item.AssetID, item.VersionID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return newDatabaseAssetRefView(existing), nil
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	if err := s.recordDatabaseReuse(ctx, item); err != nil {
		return nil, err
	}
	return newDatabaseAssetRefView(item), nil
}

func (s *databaseAssetRefService) GetRefViewByID(ctx context.Context, id uuid.UUID) (*DatabaseAssetRefView, error) {
	if id == uuid.Nil {
		return nil, ErrDatabaseAssetRefIDRequired
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return newDatabaseAssetRefView(item), nil
}

func (s *databaseAssetRefService) ListRefViews(ctx context.Context, filter repository.DatabaseAssetRefListFilter) ([]*DatabaseAssetRefView, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*DatabaseAssetRefView, 0, len(items))
	for _, item := range items {
		views = append(views, newDatabaseAssetRefView(item))
	}
	return views, total, nil
}

func (s *databaseAssetRefService) FindActiveRefView(ctx context.Context, organizationID string, dataSourceID string, tableID *string, assetID uuid.UUID, versionID uuid.UUID) (*DatabaseAssetRefView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if dataSourceID == "" {
		return nil, ErrDataSourceIDRequired
	}
	if assetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if versionID == uuid.Nil {
		return nil, ErrVersionIDRequired
	}
	item, err := s.repo.FindActive(ctx, organizationID, dataSourceID, tableID, assetID, versionID)
	if err != nil {
		return nil, err
	}
	return newDatabaseAssetRefView(item), nil
}

func (s *databaseAssetRefService) DisableRef(ctx context.Context, organizationID string, id uuid.UUID) (*DatabaseAssetRefView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if id == uuid.Nil {
		return nil, ErrDatabaseAssetRefIDRequired
	}
	item, err := s.repo.UpdateStatus(ctx, organizationID, id, model.DatabaseAssetRefStatusDisabled)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrDatabaseAssetRefNotFound
	}
	return newDatabaseAssetRefView(item), nil
}

func validateDatabaseAssetRef(item *model.DatabaseAssetRef) error {
	if item == nil || item.OrganizationID == "" {
		return ErrOrganizationIDRequired
	}
	if item.DataSourceID == "" {
		return ErrDataSourceIDRequired
	}
	if item.AssetID == uuid.Nil {
		return ErrAssetIDRequired
	}
	if item.VersionID == uuid.Nil {
		return ErrVersionIDRequired
	}
	return nil
}

func (s *databaseAssetRefService) recordDatabaseReuse(ctx context.Context, item *model.DatabaseAssetRef) error {
	if s.reuseRepo == nil {
		return nil
	}
	artifactType := model.ReuseArtifactDocumentVersion
	artifactID := item.VersionID
	if item.ParseArtifactID != nil {
		artifactType = model.ReuseArtifactParseArtifact
		artifactID = *item.ParseArtifactID
	}
	if item.ExtractionArtifactID != nil {
		artifactType = model.ReuseArtifactExtraction
		artifactID = *item.ExtractionArtifactID
	}
	return s.reuseRepo.Create(ctx, &model.ReuseEvent{
		OrganizationID: item.OrganizationID,
		WorkspaceID:    item.WorkspaceID,
		AssetID:        item.AssetID,
		VersionID:      &item.VersionID,
		ArtifactType:   artifactType,
		ArtifactID:     &artifactID,
		ConsumerType:   model.ReuseConsumerDatabase,
		ConsumerID:     item.DataSourceID,
		MetadataJSON: map[string]any{
			"database_asset_ref_id": item.ID.String(),
			"table_id":              item.TableID,
		},
		CreatedBy: item.CreatedBy,
	})
}

func newDatabaseAssetRefView(item *model.DatabaseAssetRef) *DatabaseAssetRefView {
	if item == nil {
		return nil
	}
	return &DatabaseAssetRefView{
		ID:                   item.ID,
		OrganizationID:       item.OrganizationID,
		WorkspaceID:          item.WorkspaceID,
		DataSourceID:         item.DataSourceID,
		TableID:              item.TableID,
		AssetID:              item.AssetID,
		VersionID:            item.VersionID,
		ParseArtifactID:      item.ParseArtifactID,
		ExtractionArtifactID: item.ExtractionArtifactID,
		Status:               item.Status,
		MetadataJSON:         item.MetadataJSON,
		CreatedBy:            item.CreatedBy,
		CreatedAt:            item.CreatedAt,
		UpdatedAt:            item.UpdatedAt,
	}
}

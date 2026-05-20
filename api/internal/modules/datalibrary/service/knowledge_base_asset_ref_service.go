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
	ErrKnowledgeBaseAssetRefIDRequired = errors.New("knowledge_base_asset_ref_id is required")
	ErrKnowledgeBaseAssetRefNotFound   = errors.New("knowledge_base_asset_ref not found")
	ErrDatasetIDRequired               = errors.New("dataset_id is required")
	ErrVersionIDRequired               = errors.New("version_id is required")
)

type KnowledgeBaseAssetRefService interface {
	CreateRef(ctx context.Context, item *model.KnowledgeBaseAssetRef) (*KnowledgeBaseAssetRefView, error)
	GetRefViewByID(ctx context.Context, id uuid.UUID) (*KnowledgeBaseAssetRefView, error)
	ListRefViews(ctx context.Context, filter repository.KnowledgeBaseAssetRefListFilter) ([]*KnowledgeBaseAssetRefView, int64, error)
	FindActiveRefView(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*KnowledgeBaseAssetRefView, error)
	DisableRef(ctx context.Context, organizationID string, id uuid.UUID) (*KnowledgeBaseAssetRefView, error)
}

type KnowledgeBaseAssetRefView struct {
	ID                 uuid.UUID      `json:"id"`
	OrganizationID     string         `json:"organization_id"`
	WorkspaceID        *string        `json:"workspace_id,omitempty"`
	DatasetID          string         `json:"dataset_id"`
	AssetID            uuid.UUID      `json:"asset_id"`
	VersionID          uuid.UUID      `json:"version_id"`
	ChunkArtifactSetID *uuid.UUID     `json:"chunk_artifact_set_id,omitempty"`
	VectorArtifactID   *uuid.UUID     `json:"vector_artifact_id,omitempty"`
	Status             string         `json:"status"`
	MetadataJSON       map[string]any `json:"metadata_json,omitempty"`
	CreatedBy          string         `json:"created_by,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type knowledgeBaseAssetRefService struct {
	repo      repository.KnowledgeBaseAssetRefRepository
	reuseRepo repository.ReuseEventRepository
}

func NewKnowledgeBaseAssetRefService(repo repository.KnowledgeBaseAssetRefRepository, reuseRepos ...repository.ReuseEventRepository) KnowledgeBaseAssetRefService {
	var reuseRepo repository.ReuseEventRepository
	if len(reuseRepos) > 0 {
		reuseRepo = reuseRepos[0]
	}
	return &knowledgeBaseAssetRefService{repo: repo, reuseRepo: reuseRepo}
}

func (s *knowledgeBaseAssetRefService) CreateRef(ctx context.Context, item *model.KnowledgeBaseAssetRef) (*KnowledgeBaseAssetRefView, error) {
	if err := validateKnowledgeBaseAssetRef(item); err != nil {
		return nil, err
	}
	existing, err := s.repo.FindActive(ctx, item.OrganizationID, item.DatasetID, item.AssetID, item.VersionID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return newKnowledgeBaseAssetRefView(existing), nil
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	if err := s.recordKnowledgeBaseReuse(ctx, item); err != nil {
		return nil, err
	}
	return newKnowledgeBaseAssetRefView(item), nil
}

func (s *knowledgeBaseAssetRefService) GetRefViewByID(ctx context.Context, id uuid.UUID) (*KnowledgeBaseAssetRefView, error) {
	if id == uuid.Nil {
		return nil, ErrKnowledgeBaseAssetRefIDRequired
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return newKnowledgeBaseAssetRefView(item), nil
}

func (s *knowledgeBaseAssetRefService) ListRefViews(ctx context.Context, filter repository.KnowledgeBaseAssetRefListFilter) ([]*KnowledgeBaseAssetRefView, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*KnowledgeBaseAssetRefView, 0, len(items))
	for _, item := range items {
		views = append(views, newKnowledgeBaseAssetRefView(item))
	}
	return views, total, nil
}

func (s *knowledgeBaseAssetRefService) FindActiveRefView(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*KnowledgeBaseAssetRefView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if datasetID == "" {
		return nil, ErrDatasetIDRequired
	}
	if assetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if versionID == uuid.Nil {
		return nil, ErrVersionIDRequired
	}
	item, err := s.repo.FindActive(ctx, organizationID, datasetID, assetID, versionID)
	if err != nil {
		return nil, err
	}
	return newKnowledgeBaseAssetRefView(item), nil
}

func (s *knowledgeBaseAssetRefService) DisableRef(ctx context.Context, organizationID string, id uuid.UUID) (*KnowledgeBaseAssetRefView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if id == uuid.Nil {
		return nil, ErrKnowledgeBaseAssetRefIDRequired
	}
	item, err := s.repo.UpdateStatus(ctx, organizationID, id, model.KnowledgeBaseAssetRefStatusDisabled)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrKnowledgeBaseAssetRefNotFound
	}
	return newKnowledgeBaseAssetRefView(item), nil
}

func validateKnowledgeBaseAssetRef(item *model.KnowledgeBaseAssetRef) error {
	if item == nil || item.OrganizationID == "" {
		return ErrOrganizationIDRequired
	}
	if item.DatasetID == "" {
		return ErrDatasetIDRequired
	}
	if item.AssetID == uuid.Nil {
		return ErrAssetIDRequired
	}
	if item.VersionID == uuid.Nil {
		return ErrVersionIDRequired
	}
	return nil
}

func (s *knowledgeBaseAssetRefService) recordKnowledgeBaseReuse(ctx context.Context, item *model.KnowledgeBaseAssetRef) error {
	if s.reuseRepo == nil {
		return nil
	}
	artifactType := model.ReuseArtifactDocumentVersion
	artifactID := item.VersionID
	if item.ChunkArtifactSetID != nil {
		artifactType = model.ReuseArtifactChunkArtifact
		artifactID = *item.ChunkArtifactSetID
	}
	if item.VectorArtifactID != nil {
		artifactType = model.ReuseArtifactVectorArtifact
		artifactID = *item.VectorArtifactID
	}
	return s.reuseRepo.Create(ctx, &model.ReuseEvent{
		OrganizationID: item.OrganizationID,
		WorkspaceID:    item.WorkspaceID,
		AssetID:        item.AssetID,
		VersionID:      &item.VersionID,
		ArtifactType:   artifactType,
		ArtifactID:     &artifactID,
		ConsumerType:   model.ReuseConsumerKnowledgeBase,
		ConsumerID:     item.DatasetID,
		MetadataJSON: map[string]any{
			"knowledge_base_asset_ref_id": item.ID.String(),
		},
		CreatedBy: item.CreatedBy,
	})
}

func newKnowledgeBaseAssetRefView(item *model.KnowledgeBaseAssetRef) *KnowledgeBaseAssetRefView {
	if item == nil {
		return nil
	}
	return &KnowledgeBaseAssetRefView{
		ID:                 item.ID,
		OrganizationID:     item.OrganizationID,
		WorkspaceID:        item.WorkspaceID,
		DatasetID:          item.DatasetID,
		AssetID:            item.AssetID,
		VersionID:          item.VersionID,
		ChunkArtifactSetID: item.ChunkArtifactSetID,
		VectorArtifactID:   item.VectorArtifactID,
		Status:             item.Status,
		MetadataJSON:       item.MetadataJSON,
		CreatedBy:          item.CreatedBy,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

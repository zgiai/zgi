package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ArtifactRepository interface {
	Create(ctx context.Context, item *model.Artifact) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Artifact, error)
	GetBySignature(ctx context.Context, sourceContentHash, profile, canonicalIRVersion, providerSignature string) (*model.Artifact, error)
	Upsert(ctx context.Context, item *model.Artifact) error
}

type artifactRepository struct {
	db *gorm.DB
}

func NewArtifactRepository(db *gorm.DB) ArtifactRepository {
	return &artifactRepository{db: db}
}

func (r *artifactRepository) Create(ctx context.Context, item *model.Artifact) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *artifactRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Artifact, error) {
	var item model.Artifact
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *artifactRepository) GetBySignature(ctx context.Context, sourceContentHash, profile, canonicalIRVersion, providerSignature string) (*model.Artifact, error) {
	var item model.Artifact
	err := r.db.WithContext(ctx).
		Where("source_content_hash = ? AND profile = ? AND canonical_ir_version = ? AND provider_signature = ?", sourceContentHash, profile, canonicalIRVersion, providerSignature).
		First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *artifactRepository) Upsert(ctx context.Context, item *model.Artifact) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "source_content_hash"},
			{Name: "profile"},
			{Name: "canonical_ir_version"},
			{Name: "provider_signature"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"artifact_storage_key",
			"diagnostics_storage_key",
			"summary_json",
			"updated_at",
		}),
	}).Create(item).Error
}

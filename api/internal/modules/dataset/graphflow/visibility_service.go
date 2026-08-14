package graphflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrStaleDocumentSnapshot   = errors.New("stale document snapshot")
	ErrStaleVisibilityRevision = errors.New("stale graph visibility revision")
)

type VisibilityChangeRequest struct {
	OrganizationID   uuid.UUID
	WorkspaceID      *uuid.UUID
	DatasetID        uuid.UUID
	SourceRefID      uuid.UUID
	DocumentID       uuid.UUID
	RetrievalEnabled bool
}

type VisibilityService struct {
	db               *gorm.DB
	outboxRepo       *repository.GraphOutboxRepository
	entityRepo       *repository.EntityRepository
	relationshipRepo *repository.RelationshipRepository
}

func graphSourceIsActive(currentRef bool, retrievalEnabled bool, documentEnabled bool) bool {
	return currentRef && retrievalEnabled && documentEnabled
}

func NewVisibilityService(db *gorm.DB) *VisibilityService {
	return &VisibilityService{
		db:               db,
		outboxRepo:       repository.NewGraphOutboxRepository(db),
		entityRepo:       repository.NewEntityRepository(db),
		relationshipRepo: repository.NewRelationshipRepository(db),
	}
}

func (s *VisibilityService) SetDocumentRetrievalEnabled(
	ctx context.Context,
	request VisibilityChangeRequest,
) (int64, bool, error) {
	var revision int64
	var changed bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ref datalibrarymodel.KnowledgeBaseAssetRef
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND dataset_id = ? AND deleted_at IS NULL", request.SourceRefID, request.DatasetID).
			First(&ref).Error; err != nil {
			return err
		}
		if ref.OrganizationID != request.OrganizationID.String() ||
			(request.WorkspaceID != nil && (ref.WorkspaceID == nil || *ref.WorkspaceID != request.WorkspaceID.String())) {
			return ErrGraphFlowTenantScopeMismatch
		}
		if ref.DatasetDocumentID == nil || *ref.DatasetDocumentID != request.DocumentID {
			return ErrStaleDocumentSnapshot
		}

		var dataset datasetmodel.Dataset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&dataset, "id = ?", request.DatasetID).Error; err != nil {
			return err
		}
		if dataset.OrganizationID != request.OrganizationID.String() ||
			(request.WorkspaceID != nil && dataset.WorkspaceID != request.WorkspaceID.String()) {
			return ErrGraphFlowTenantScopeMismatch
		}
		if ref.RetrievalEnabled == request.RetrievalEnabled {
			revision = dataset.GraphVisibilityRevision
			return nil
		}

		if err := tx.Model(&datalibrarymodel.KnowledgeBaseAssetRef{}).
			Where("id = ?", ref.ID).
			Update("retrieval_enabled", request.RetrievalEnabled).Error; err != nil {
			return err
		}
		revision = dataset.GraphVisibilityRevision + 1
		if err := tx.Model(&datasetmodel.Dataset{}).Where("id = ?", request.DatasetID).
			Updates(map[string]any{
				"graph_visibility_revision": revision,
				"graph_updated_at":          time.Now().UTC(),
			}).Error; err != nil {
			return err
		}

		event := &graphmodel.GraphOutboxEvent{
			OrganizationID: request.OrganizationID,
			WorkspaceID:    request.WorkspaceID,
			DatasetID:      request.DatasetID,
			EventType:      graphmodel.GraphOutboxEventVisibility,
			AggregateKey:   fmt.Sprintf("visibility:%s:%d", request.DatasetID, revision),
			Payload: map[string]any{
				"dataset_id":        request.DatasetID.String(),
				"document_id":       request.DocumentID.String(),
				"source_ref_id":     request.SourceRefID.String(),
				"retrieval_enabled": request.RetrievalEnabled,
				"revision":          revision,
			},
			Status:      graphmodel.GraphOutboxStatusPending,
			AvailableAt: time.Now().UTC(),
		}
		if _, _, err := s.outboxRepo.WithTx(tx).CreateOrGet(ctx, event); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return revision, changed, err
}

func (s *VisibilityService) RecalculateAndConfirm(ctx context.Context, datasetID uuid.UUID, revision int64) error {
	if err := s.Recalculate(ctx, datasetID, revision); err != nil {
		return err
	}
	return s.ConfirmProjection(ctx, datasetID, revision)
}

func (s *VisibilityService) Recalculate(ctx context.Context, datasetID uuid.UUID, revision int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dataset datasetmodel.Dataset
		if err := tx.First(&dataset, "id = ?", datasetID).Error; err != nil {
			return err
		}
		if dataset.GraphVisibilityRevision != revision {
			return ErrStaleVisibilityRevision
		}
		if err := repository.NewEntityRepository(tx).RecalculateSourceCounts(ctx, datasetID); err != nil {
			return err
		}
		if err := repository.NewRelationshipRepository(tx).RecalculateSourceCounts(ctx, datasetID); err != nil {
			return err
		}
		if err := tx.Model(&graphmodel.Entity{}).Where("kb_id = ?", datasetID).
			Update("visibility_revision", revision).Error; err != nil {
			return err
		}
		return tx.Model(&graphmodel.Relationship{}).Where("kb_id = ?", datasetID).
			Update("visibility_revision", revision).Error
	})
}

func (s *VisibilityService) ConfirmProjection(ctx context.Context, datasetID uuid.UUID, revision int64) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&datasetmodel.Dataset{}).
			Where("id = ? AND graph_visibility_revision = ?", datasetID, revision).
			Updates(map[string]any{
				"graph_projected_visibility_revision": revision,
				"graph_updated_at":                    time.Now().UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrStaleVisibilityRevision
		}
		return nil
	})
}

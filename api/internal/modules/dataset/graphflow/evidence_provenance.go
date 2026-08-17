package graphflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

type legacyEvidenceDocument struct {
	DatasetID  uuid.UUID `gorm:"column:dataset_id"`
	DocumentID uuid.UUID `gorm:"column:document_id"`
}

func (s *Service) RepairLegacyEvidenceProvenance(ctx context.Context) (int, error) {
	if s == nil || s.DB == nil {
		return 0, nil
	}

	var documents []legacyEvidenceDocument
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT DISTINCT evidence.kb_id AS dataset_id, segment.document_id
		FROM (
			SELECT kb_id, segment_id FROM kb_entity_mentions
			WHERE document_id IS NULL OR source_ref_id IS NULL OR run_id IS NULL
			UNION
			SELECT kb_id, segment_id FROM kb_triple_mentions
			WHERE document_id IS NULL OR source_ref_id IS NULL OR run_id IS NULL OR relationship_id IS NULL
		) AS evidence
		JOIN document_segments AS segment ON segment.id = evidence.segment_id
	`).Scan(&documents).Error; err != nil {
		return 0, fmt.Errorf("find legacy graph evidence: %w", err)
	}

	affected := make(map[uuid.UUID]struct{})
	for _, document := range documents {
		updates := map[string]interface{}{
			"document_id": document.DocumentID,
		}

		var ref datalibrarymodel.KnowledgeBaseAssetRef
		refErr := s.DB.WithContext(ctx).
			Where("dataset_id = ? AND dataset_document_id = ? AND deleted_at IS NULL", document.DatasetID, document.DocumentID).
			Order("updated_at DESC").
			First(&ref).Error
		if refErr == nil {
			updates["source_ref_id"] = ref.ID
			if ref.GraphRunID != nil {
				updates["run_id"] = *ref.GraphRunID
			}
			if organizationID, err := uuid.Parse(ref.OrganizationID); err == nil {
				updates["organization_id"] = organizationID
			}
		} else if !errors.Is(refErr, gorm.ErrRecordNotFound) {
			return len(affected), fmt.Errorf("find graph evidence source reference: %w", refErr)
		} else {
			var sourceDocument datasetmodel.Document
			if err := s.DB.WithContext(ctx).Select("organization_id").First(&sourceDocument, "id = ?", document.DocumentID).Error; err == nil {
				if organizationID, parseErr := uuid.Parse(sourceDocument.OrganizationID); parseErr == nil {
					updates["organization_id"] = organizationID
				}
			}
		}

		segmentScope := "segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)"
		if err := s.DB.WithContext(ctx).Model(&graphmodel.EntityMention{}).
			Where("kb_id = ? AND (document_id IS NULL OR source_ref_id IS NULL OR run_id IS NULL) AND "+segmentScope, document.DatasetID, document.DocumentID).
			Updates(updates).Error; err != nil {
			return len(affected), fmt.Errorf("repair entity evidence provenance: %w", err)
		}
		if err := s.DB.WithContext(ctx).Model(&graphmodel.TripleMention{}).
			Where("kb_id = ? AND (document_id IS NULL OR source_ref_id IS NULL OR run_id IS NULL) AND "+segmentScope, document.DatasetID, document.DocumentID).
			Updates(updates).Error; err != nil {
			return len(affected), fmt.Errorf("repair relationship evidence provenance: %w", err)
		}
		affected[document.DatasetID] = struct{}{}
	}

	for datasetID := range affected {
		if err := s.DB.WithContext(ctx).Exec(`
			UPDATE kb_triple_mentions AS mention
			SET relationship_id = (
				SELECT relationship.id
				FROM kb_relationships AS relationship
				WHERE relationship.kb_id = mention.kb_id
					AND relationship.head_entity_id = mention.head_entity_id
					AND relationship.tail_entity_id = mention.tail_entity_id
					AND relationship.relation_type = mention.raw_predicate
					AND relationship.is_deleted = false
				LIMIT 1
			)
			WHERE mention.kb_id = ?
				AND mention.relationship_id IS NULL
				AND mention.head_entity_id IS NOT NULL
				AND mention.tail_entity_id IS NOT NULL
		`, datasetID).Error; err != nil {
			return len(affected), fmt.Errorf("repair relationship evidence binding: %w", err)
		}
		if err := s.RefreshVisibilityProjection(ctx, datasetID); err != nil {
			return len(affected), fmt.Errorf("refresh repaired graph visibility: %w", err)
		}
	}
	return len(affected), nil
}

func (s *Service) RefreshVisibilityProjection(ctx context.Context, datasetID uuid.UUID) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("graph service is not configured")
	}
	var dataset datasetmodel.Dataset
	if err := s.DB.WithContext(ctx).First(&dataset, "id = ?", datasetID).Error; err != nil {
		return err
	}
	visibility := NewVisibilityService(s.DB)
	if err := visibility.Recalculate(ctx, datasetID, dataset.GraphVisibilityRevision); err != nil {
		return err
	}
	return s.ProjectVisibilityProjection(ctx, datasetID, dataset.GraphVisibilityRevision)
}

// ProjectVisibilityProjection copies already calculated visibility counts to
// the serving stores and confirms the projected revision. Alignment and
// cleanup calculate those counts before graph_sync, so that pipeline calls
// this method directly instead of repeating two KB-wide PostgreSQL aggregate
// updates immediately after alignment.
func (s *Service) ProjectVisibilityProjection(
	ctx context.Context,
	datasetID uuid.UUID,
	expectedRevision int64,
	progressCallbacks ...func(completed, total int),
) error {
	return s.projectVisibilityProjection(ctx, datasetID, expectedRevision, nil, progressCallbacks...)
}

// ProjectVisibilityProjectionForRun projects only canonical rows whose
// evidence belongs to source refs changed by this run. Full rebuilds naturally
// cover all refs; normal incremental runs avoid rewriting the entire graph.
func (s *Service) ProjectVisibilityProjectionForRun(
	ctx context.Context,
	datasetID uuid.UUID,
	expectedRevision int64,
	runID uuid.UUID,
	progressCallbacks ...func(completed, total int),
) error {
	var sourceRefIDs []uuid.UUID
	if err := s.DB.WithContext(ctx).Model(&graphmodel.GraphFlowRunItem{}).
		Distinct("source_ref_id").
		Where("run_id = ? AND source_ref_id IS NOT NULL", runID).
		Pluck("source_ref_id", &sourceRefIDs).Error; err != nil {
		return err
	}
	if len(sourceRefIDs) == 0 {
		var run graphmodel.GraphFlowRun
		if err := s.DB.WithContext(ctx).Select("source_ref_id").First(&run, "id = ?", runID).Error; err != nil {
			return err
		}
		if run.SourceRefID == nil {
			return s.ProjectVisibilityProjection(ctx, datasetID, expectedRevision, progressCallbacks...)
		}
		sourceRefIDs = append(sourceRefIDs, *run.SourceRefID)
	}
	return s.projectVisibilityProjection(ctx, datasetID, expectedRevision, sourceRefIDs, progressCallbacks...)
}

// ProjectVisibilityProjectionForSourceRef is used by retrieval visibility
// toggles, where exactly one stable source ref is affected.
func (s *Service) ProjectVisibilityProjectionForSourceRef(
	ctx context.Context,
	datasetID uuid.UUID,
	expectedRevision int64,
	sourceRefID uuid.UUID,
) error {
	return s.projectVisibilityProjection(ctx, datasetID, expectedRevision, []uuid.UUID{sourceRefID})
}

func (s *Service) projectVisibilityProjection(
	ctx context.Context,
	datasetID uuid.UUID,
	expectedRevision int64,
	sourceRefIDs []uuid.UUID,
	progressCallbacks ...func(completed, total int),
) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("graph service is not configured")
	}
	var dataset datasetmodel.Dataset
	if err := s.DB.WithContext(ctx).First(&dataset, "id = ?", datasetID).Error; err != nil {
		return err
	}
	if dataset.GraphVisibilityRevision != expectedRevision {
		return ErrStaleVisibilityRevision
	}
	var progress func(completed, total int)
	if len(progressCallbacks) > 0 {
		progress = progressCallbacks[0]
	}

	var entities []*graphmodel.Entity
	var relationships []*graphmodel.Relationship
	if len(sourceRefIDs) == 0 {
		var err error
		entities, err = s.EntityRepo.FindByKBID(ctx, datasetID)
		if err != nil {
			return err
		}
		relationships, err = s.RelationshipRepo.FindByKBID(ctx, datasetID)
		if err != nil {
			return err
		}
	} else {
		if err := s.DB.WithContext(ctx).
			Where("kb_id = ? AND is_deleted = false AND id IN (?)", datasetID,
				s.DB.WithContext(ctx).Model(&graphmodel.EntityMention{}).
					Select("DISTINCT entity_id").
					Where("kb_id = ? AND source_ref_id IN ? AND entity_id IS NOT NULL", datasetID, sourceRefIDs)).
			Find(&entities).Error; err != nil {
			return err
		}
		if err := s.DB.WithContext(ctx).
			Where("kb_id = ? AND id IN (?)", datasetID,
				s.DB.WithContext(ctx).Model(&graphmodel.TripleMention{}).
					Select("DISTINCT relationship_id").
					Where("kb_id = ? AND source_ref_id IN ? AND relationship_id IS NOT NULL", datasetID, sourceRefIDs)).
			Find(&relationships).Error; err != nil {
			return err
		}
	}
	entityUpdates := make([]map[string]interface{}, 0, len(entities))
	for _, entity := range entities {
		entityUpdates = append(entityUpdates, map[string]interface{}{
			"id":                  entity.ID.String(),
			"source_count":        entity.SourceCount,
			"active_source_count": entity.ActiveSourceCount,
			"content_revision":    entity.ContentRevision,
			"visibility_revision": expectedRevision,
		})
	}
	relationshipUpdates := make([]map[string]interface{}, 0, len(relationships))
	for _, relationship := range relationships {
		if relationship.IsDeleted {
			continue
		}
		relationshipUpdates = append(relationshipUpdates, map[string]interface{}{
			"id":                  relationship.ID.String(),
			"head_id":             relationship.HeadEntityID.String(),
			"tail_id":             relationship.TailEntityID.String(),
			"weight":              relationship.Weight,
			"active_weight":       relationship.ActiveWeight,
			"content_revision":    relationship.ContentRevision,
			"visibility_revision": expectedRevision,
		})
	}
	if s.Neo4jClient == nil {
		return fmt.Errorf("neo4j client not configured")
	}
	neo4jTotal := len(entityUpdates) + len(relationshipUpdates)
	weaviateUpdates := make([]vectordb.ObjectPropertyUpdate, 0, len(entities))
	if s.WeaviateClient != nil {
		for _, entity := range entities {
			if entity.EmbeddingID == "" {
				continue
			}
			weaviateUpdates = append(weaviateUpdates, vectordb.ObjectPropertyUpdate{
				ID: entity.ID.String(),
				Properties: map[string]interface{}{
					"source_count":        entity.SourceCount,
					"active_source_count": entity.ActiveSourceCount,
					"content_revision":    entity.ContentRevision,
					"visibility_revision": expectedRevision,
				},
			})
		}
	}
	total := neo4jTotal + len(weaviateUpdates)
	if err := s.Neo4jClient.UpdateVisibilityProjection(ctx, datasetID.String(), entityUpdates, relationshipUpdates, func(completed, _ int) {
		if progress != nil {
			progress(completed, total)
		}
	}); err != nil {
		return err
	}
	if len(weaviateUpdates) > 0 {
		className := fmt.Sprintf("Entity_%s", datasetID)
		if err := s.WeaviateClient.UpdateObjectPropertiesBatch(ctx, className, weaviateUpdates, 8, func(completed, _ int) {
			if progress != nil {
				progress(neo4jTotal+completed, total)
			}
		}); err != nil {
			return err
		}
	}
	if progress != nil {
		progress(total, total)
	}
	return NewVisibilityService(s.DB).ConfirmProjection(ctx, datasetID, expectedRevision)
}

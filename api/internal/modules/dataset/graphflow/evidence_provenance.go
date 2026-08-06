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
	return s.ProjectVisibilityProjection(ctx, datasetID)
}

// ProjectVisibilityProjection copies already calculated visibility counts to
// the serving stores and confirms the projected revision. Alignment and
// cleanup calculate those counts before graph_sync, so that pipeline calls
// this method directly instead of repeating two KB-wide PostgreSQL aggregate
// updates immediately after alignment.
func (s *Service) ProjectVisibilityProjection(
	ctx context.Context,
	datasetID uuid.UUID,
	progressCallbacks ...func(completed, total int),
) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("graph service is not configured")
	}
	var dataset datasetmodel.Dataset
	if err := s.DB.WithContext(ctx).First(&dataset, "id = ?", datasetID).Error; err != nil {
		return err
	}
	var progress func(completed, total int)
	if len(progressCallbacks) > 0 {
		progress = progressCallbacks[0]
	}

	entities, err := s.EntityRepo.FindByKBID(ctx, datasetID)
	if err != nil {
		return err
	}
	relationships, err := s.RelationshipRepo.FindByKBID(ctx, datasetID)
	if err != nil {
		return err
	}
	entityUpdates := make([]map[string]interface{}, 0, len(entities))
	for _, entity := range entities {
		entityUpdates = append(entityUpdates, map[string]interface{}{
			"id":                  entity.ID.String(),
			"source_count":        entity.SourceCount,
			"active_source_count": entity.ActiveSourceCount,
			"content_revision":    entity.ContentRevision,
			"visibility_revision": dataset.GraphVisibilityRevision,
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
			"visibility_revision": dataset.GraphVisibilityRevision,
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
					"visibility_revision": dataset.GraphVisibilityRevision,
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
	return NewVisibilityService(s.DB).ConfirmProjection(ctx, datasetID, dataset.GraphVisibilityRevision)
}

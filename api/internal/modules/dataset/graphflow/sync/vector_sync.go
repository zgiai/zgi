package sync

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

// VectorSyncResult contains the result of vector synchronization
type VectorSyncResult struct {
	EntitiesSynced int      `json:"entities_synced"`
	EntitiesFailed int      `json:"entities_failed"`
	FailedIDs      []string `json:"failed_ids,omitempty"`
}

// VectorSync handles synchronization of entity embeddings to vector store
type VectorSync struct {
	entityRepo           *repository.EntityRepository
	llmClient            client.LLMClient
	vectorDB             vectordb.VectorDB
	embeddingModel       string
	embeddingProvider    string
	embeddingDimension   int
	embeddingFingerprint string
	batchSize            int
}

// NewVectorSync creates a new VectorSync instance
func NewVectorSync(
	entityRepo *repository.EntityRepository,
	llmClient client.LLMClient,
	vectorDB vectordb.VectorDB,
) *VectorSync {
	return &VectorSync{
		entityRepo: entityRepo,
		llmClient:  llmClient,
		vectorDB:   vectorDB,
		batchSize:  10,
	}
}

// SyncPendingVectors synchronizes all pending entity vectors for a KB
func (s *VectorSync) SyncPendingVectors(ctx context.Context, tenantID string, kbID uuid.UUID) (*VectorSyncResult, error) {
	result := &VectorSyncResult{
		FailedIDs: make([]string, 0),
	}

	// Check if LLM client is available
	if s.llmClient == nil {
		logger.Warn("LLM client not configured, skipping vector sync", nil)
		return result, nil
	}
	if s.embeddingModel == "" || s.embeddingProvider == "" || s.embeddingDimension <= 0 {
		return nil, fmt.Errorf("knowledge base embedding identity is required")
	}

	// Get pending entities
	pendingEntities, err := s.entityRepo.FindPendingVectorSync(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending entities: %w", err)
	}

	if len(pendingEntities) == 0 {
		logger.Info("No pending entities for vector sync", map[string]interface{}{
			"kb_id": kbID.String(),
		})
		return result, nil
	}

	// Process in batches
	for i := 0; i < len(pendingEntities); i += s.batchSize {
		end := i + s.batchSize
		if end > len(pendingEntities) {
			end = len(pendingEntities)
		}
		batch := pendingEntities[i:end]

		// Prepare texts for embedding
		texts := make([]string, len(batch))
		for j, entity := range batch {
			// Create rich text representation for embedding
			texts[j] = fmt.Sprintf("%s (%s)", entity.Name, entity.Type)
			if entity.Description != "" {
				texts[j] = fmt.Sprintf("%s: %s", texts[j], entity.Description)
			}
		}

		// Generate embeddings
		embeddingReq := &adapter.EmbeddingsRequest{
			Model: s.embeddingModel,
			Input: texts,
		}

		embeddingResp, err := s.llmClient.Embed(ctx, tenantID, embeddingReq)
		if err != nil {
			logger.Error("Failed to generate embeddings", err)
			errorMsg := err.Error()
			// Mark all entities in batch as failed
			for _, entity := range batch {
				s.entityRepo.UpdateVectorState(ctx, entity.ID, "failed", "", errorMsg)
				result.EntitiesFailed++
				result.FailedIDs = append(result.FailedIDs, entity.ID.String())
			}
			continue
		}

		// Store embeddings and update entity states
		for j, entity := range batch {
			if j >= len(embeddingResp.Data) {
				result.EntitiesFailed++
				result.FailedIDs = append(result.FailedIDs, entity.ID.String())
				continue
			}

			embedding := embeddingResp.Data[j]
			if len(embedding.Embedding) != s.embeddingDimension {
				s.entityRepo.UpdateVectorState(ctx, entity.ID, "failed", "", "embedding dimension mismatch")
				result.EntitiesFailed++
				result.FailedIDs = append(result.FailedIDs, entity.ID.String())
				continue
			}

			// Store in vector DB if available
			if s.vectorDB != nil {
				className := fmt.Sprintf("Entity_%s", kbID.String())
				properties := map[string]interface{}{
					"id":                  entity.ID.String(),
					"name":                entity.Name,
					"canonical_name":      entity.CanonicalName,
					"type":                entity.Type,
					"kb_id":               kbID.String(),
					"source_count":        entity.SourceCount,
					"active_source_count": entity.ActiveSourceCount,
					"content_revision":    entity.ContentRevision,
					"visibility_revision": entity.VisibilityRevision,
				}

				// Convert embedding to float64
				vector := make([]float64, len(embedding.Embedding))
				for k, v := range embedding.Embedding {
					vector[k] = float64(v)
				}

				err := s.vectorDB.StoreVector(ctx, entity.ID.String(), className, properties, vector)
				if err != nil {
					logger.Error("Failed to store vector", err)
					s.entityRepo.UpdateVectorState(ctx, entity.ID, "failed", "", err.Error())
					result.EntitiesFailed++
					result.FailedIDs = append(result.FailedIDs, entity.ID.String())
					continue
				}
			}

			// Update entity with embedding ID
			embeddingID := fmt.Sprintf("entity_%s", entity.ID.String())
			if err := s.entityRepo.UpdateVectorState(ctx, entity.ID, "synced", embeddingID, ""); err != nil {
				logger.Error("Failed to update entity vector state", err)
				result.EntitiesFailed++
				continue
			}
			if err := s.entityRepo.UpdateEmbeddingIdentityBatch(
				ctx,
				[]uuid.UUID{entity.ID},
				s.embeddingProvider,
				s.embeddingModel,
				s.embeddingDimension,
				s.embeddingFingerprint,
				entity.ContentRevision,
			); err != nil {
				logger.Error("Failed to update entity embedding identity", err)
				result.EntitiesFailed++
				continue
			}

			result.EntitiesSynced++
		}
	}

	logger.Info("Vector sync completed", map[string]interface{}{
		"kb_id":           kbID.String(),
		"entities_synced": result.EntitiesSynced,
		"entities_failed": result.EntitiesFailed,
	})

	return result, nil
}

// SetBatchSize sets the batch size for embedding requests
func (s *VectorSync) SetBatchSize(size int) {
	if size > 0 {
		s.batchSize = size
	}
}

// SetEmbeddingModel sets the embedding model to use
func (s *VectorSync) SetEmbeddingModel(model string) {
	if model != "" {
		s.embeddingModel = model
	}
}

func (s *VectorSync) SetEmbeddingIdentity(provider, modelName string, dimension int, fingerprint string) {
	if provider == "" || modelName == "" || dimension <= 0 {
		return
	}
	s.embeddingProvider = provider
	s.embeddingModel = modelName
	s.embeddingDimension = dimension
	s.embeddingFingerprint = fingerprint
}

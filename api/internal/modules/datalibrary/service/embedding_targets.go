package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibRepo "github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

type EmbeddingTarget struct {
	Provider string
	Model    string
}

type embeddingTargetEmbeddingReader interface {
	ListModelTargetsByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]datalibRepo.DocumentChunkEmbeddingModelTarget, error)
	ListModelTargetsByChunkIDs(ctx context.Context, organizationID string, chunkIDs []uuid.UUID) ([]datalibRepo.DocumentChunkEmbeddingModelTarget, error)
}

type embeddingTargetRefReader interface {
	ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*datalibModel.KnowledgeBaseAssetRef, error)
}

type embeddingTargetDatasetReader interface {
	GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error)
}

type CollectEmbeddingTargetsInput struct {
	OrganizationID    string
	Asset             *datalibModel.DocumentAsset
	AssetID           uuid.UUID
	ChunkIDs          []uuid.UUID
	EmbeddingProvider string
	EmbeddingModel    string
	Embeddings        embeddingTargetEmbeddingReader
	Refs              embeddingTargetRefReader
	Datasets          embeddingTargetDatasetReader
}

func CollectEmbeddingTargets(ctx context.Context, input CollectEmbeddingTargetsInput) ([]EmbeddingTarget, error) {
	collector := newEmbeddingTargetCollector()
	collector.add(input.EmbeddingProvider, input.EmbeddingModel)
	if input.Asset != nil {
		if input.Asset.EmbeddingProvider != nil || input.Asset.EmbeddingModel != nil {
			collector.add(stringPtrValue(input.Asset.EmbeddingProvider), stringPtrValue(input.Asset.EmbeddingModel))
		}
		if input.AssetID == uuid.Nil {
			input.AssetID = input.Asset.ID
		}
		if input.OrganizationID == "" {
			input.OrganizationID = input.Asset.OrganizationID
		}
	}

	if input.Embeddings != nil {
		var targets []datalibRepo.DocumentChunkEmbeddingModelTarget
		var err error
		if len(input.ChunkIDs) > 0 {
			targets, err = input.Embeddings.ListModelTargetsByChunkIDs(ctx, input.OrganizationID, input.ChunkIDs)
		} else if input.AssetID != uuid.Nil {
			targets, err = input.Embeddings.ListModelTargetsByAsset(ctx, input.OrganizationID, input.AssetID)
		}
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			collector.add(target.EmbeddingProvider, target.EmbeddingModel)
		}
	}

	if input.Refs != nil && input.Datasets != nil && input.AssetID != uuid.Nil {
		refs, err := input.Refs.ListActiveByAsset(ctx, input.OrganizationID, input.AssetID)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			if ref == nil || strings.TrimSpace(ref.DatasetID) == "" {
				continue
			}
			dataset, err := input.Datasets.GetByID(ctx, ref.DatasetID)
			if err != nil {
				return nil, err
			}
			if dataset == nil {
				continue
			}
			collector.add(stringPtrValue(dataset.EmbeddingModelProvider), stringPtrValue(dataset.EmbeddingModel))
		}
	}

	targets := collector.list()
	if len(targets) == 0 {
		targets = append(targets, EmbeddingTarget{
			Provider: strings.TrimSpace(input.EmbeddingProvider),
			Model:    strings.TrimSpace(input.EmbeddingModel),
		})
	}
	return targets, nil
}

type embeddingTargetCollector struct {
	seen  map[string]struct{}
	items []EmbeddingTarget
}

func newEmbeddingTargetCollector() *embeddingTargetCollector {
	return &embeddingTargetCollector{seen: map[string]struct{}{}}
}

func (c *embeddingTargetCollector) add(provider string, modelName string) {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return
	}
	key := provider + "\x00" + modelName
	if _, ok := c.seen[key]; ok {
		return
	}
	c.seen[key] = struct{}{}
	c.items = append(c.items, EmbeddingTarget{Provider: provider, Model: modelName})
}

func (c *embeddingTargetCollector) list() []EmbeddingTarget {
	return append([]EmbeddingTarget(nil), c.items...)
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

package indexing

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	datasetrepository "github.com/zgiai/ginext/internal/modules/dataset/repository"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/ginext/pkg/embedding"
	"github.com/zgiai/ginext/pkg/storage"
	"github.com/zgiai/ginext/pkg/vectordb"
)

// TableIndexProcessor keeps one extracted table row as one segment.
type TableIndexProcessor struct {
	*BaseIndexProcessorImpl
}

// NewTableIndexProcessor creates a new table index processor.
func NewTableIndexProcessor(storage storage.Storage, defaultModelSvc llmdefaultservice.DefaultModelService, llmClient llmclient.LLMClient, tenantID string) BaseIndexProcessor {
	return &TableIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(storage, defaultModelSvc, llmClient, tenantID),
	}
}

// Extract reuses the shared extraction path.
func (p *TableIndexProcessor) Extract(ctx context.Context, setting *ExtractSetting, options *ProcessOptions) (*dto.ExtractOutput, error) {
	return p.BaseIndexProcessorImpl.Extract(ctx, setting, options)
}

// Transform keeps each extracted table element as a single segment.
func (p *TableIndexProcessor) Transform(ctx context.Context, output *dto.ExtractOutput, options *ProcessOptions) ([]dto.TransformedChunk, error) {
	elements := sortedExtractElements(output)
	transformed := make([]dto.TransformedChunk, 0, len(elements))

	for _, element := range elements {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		content := strings.TrimSpace(element.Content)
		if content == "" {
			continue
		}

		transformed = append(transformed, dto.TransformedChunk{
			Content:  content,
			BBox:     element.BBox,
			Metadata: element.Metadata,
		})
	}
	return transformed, nil
}

// Load stores flat table row segments in the vector database.
func (p *TableIndexProcessor) Load(ctx context.Context, dataset *model.Dataset, chunks []dto.TransformedChunk, withKeywords bool, embeddingService embedding.EmbeddingService,
	documentRepo datasetrepository.DocumentRepository,
	vectorDB vectordb.VectorDB) (int, error) {
	_ = withKeywords

	items, err := p.buildIndexingItems(dataset, chunks)
	if err != nil {
		return 0, err
	}
	return processIndexingItems(ctx, dataset, items, embeddingService, documentRepo, vectorDB, indexingBatchOptions{
		Name:          "table",
		FailOnPartial: true,
	})
}

func (p *TableIndexProcessor) buildIndexingItems(dataset *model.Dataset, chunks []dto.TransformedChunk) ([]indexingItem, error) {
	items := make([]indexingItem, 0, len(chunks))
	className := model.GenCollectionNameByID(dataset.ID)
	for _, chunk := range chunks {
		indexNodeID := getMetadataByKey(chunk.Metadata, "doc_id")
		if indexNodeID == "" {
			return nil, fmt.Errorf("failed to get doc_id from table chunk metadata")
		}
		items = append(items, indexingItem{
			IndexNodeID: indexNodeID,
			Text:        chunk.Content,
			ClassName:   className,
			Properties: map[string]interface{}{
				"text":        chunk.Content,
				"doc_id":      indexNodeID,
				"doc_hash":    getMetadataByKey(chunk.Metadata, "doc_hash"),
				"document_id": getMetadataByKey(chunk.Metadata, "document_id"),
				"dataset_id":  dataset.ID,
			},
			ItemType: indexingItemTypeSegment,
		})
	}
	return items, nil
}

// Clean deletes table row vectors from the vector database.
func (p *TableIndexProcessor) Clean(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	_ = withKeywords
	_ = deleteChildChunks

	vectorDB := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)

	className := model.GenCollectionNameByID(dataset.ID)

	if len(nodeIDs) > 0 {
		if err := vectorDB.DeleteObjectsByIDs(ctx, className, nodeIDs); err != nil {
			return fmt.Errorf("failed to delete vectors by IDs: %w", err)
		}
	}

	return nil
}

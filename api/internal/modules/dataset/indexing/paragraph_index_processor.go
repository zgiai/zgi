package indexing

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/ginext/internal/modules/dataset/repository"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/ginext/pkg/embedding"
	"github.com/zgiai/ginext/pkg/storage"
	"github.com/zgiai/ginext/pkg/vectordb"
)

// NewParagraphIndexProcessor creates a new paragraph index processor
func NewParagraphIndexProcessor(storage storage.Storage, defaultModelSvc llmdefaultservice.DefaultModelService, llmClient llmclient.LLMClient, tenantID string) BaseIndexProcessor {
	return &ParagraphIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(storage, defaultModelSvc, llmClient, tenantID),
	}
}

// ParagraphIndexProcessor paragraph index processor
type ParagraphIndexProcessor struct {
	*BaseIndexProcessorImpl
}

// Extract extracts documents
func (p *ParagraphIndexProcessor) Extract(ctx context.Context, setting *ExtractSetting, options *ProcessOptions) (*dto.ExtractOutput, error) {
	return p.BaseIndexProcessorImpl.Extract(ctx, setting, options)
}

// Transform transforms extracted elements into structure-aware chunks.
func (p *ParagraphIndexProcessor) Transform(ctx context.Context, output *dto.ExtractOutput, options *ProcessOptions) ([]dto.TransformedChunk, error) {
	// Get processing rules
	mode := options.Mode

	// Check if segment content auto fill is enabled
	// Check if segment content auto fill is enabled
	autoFillEnabled := false
	// Auto-fill logic removed as per requirement

	// Get segmentation parameters based on processing rules
	var chunkSize, chunkOverlap int
	var separator string

	if mode == "automatic" {
		// Automatic mode uses default parameters
		chunkSize = 500
		chunkOverlap = 50
		separator = "\n\n"
	} else {
		// Custom mode gets parameters from rules
		rule, err := ParseRule(options.ProcessRule)
		if err != nil {
			return nil, fmt.Errorf("failed to parse rule: %w", err)
		}

		chunkSize = rule.Segmentation.MaxTokens
		chunkOverlap = rule.Segmentation.ChunkOverlap
		separator = rule.Segmentation.Separator
	}

	// Use base class method to create text splitter
	textSplitter := p._get_splitter(chunkSize, chunkOverlap, separator)

	elements := sortedExtractElements(output)
	rawChunks := make([]dto.TransformedChunk, 0, len(elements))
	textElements := make([]dto.ExtractElement, 0)
	for _, element := range elements {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if isParagraphTextElement(element.Type) {
			textElements = append(textElements, element)
			continue
		}

		rawChunks = append(rawChunks, p.buildTextChunks(ctx, output, textElements, textSplitter, autoFillEnabled, options)...)
		textElements = textElements[:0]

		if isStandaloneElement(element.Type) {
			content := strings.TrimSpace(element.Content)
			if content == "" {
				continue
			}
			if autoFillEnabled {
				enhancedContent, err := p.enhanceContent(ctx, content, options)
				if err == nil {
					content = enhancedContent
				}
			}
			rawChunks = append(rawChunks, dto.TransformedChunk{
				Content:  content,
				BBox:     bboxFromElements([]dto.ExtractElement{element}),
				Metadata: metadataForElements(output, []dto.ExtractElement{element}),
			})
		}
	}

	rawChunks = append(rawChunks, p.buildTextChunks(ctx, output, textElements, textSplitter, autoFillEnabled, options)...)
	addChunkMetadata(rawChunks)
	return rawChunks, nil
}

// enhanceContent enhances document content using LLM when segment_content_auto_fill is enabled
func (p *ParagraphIndexProcessor) enhanceContent(ctx context.Context, content string, options *ProcessOptions) (string, error) {
	return p.BaseIndexProcessorImpl.enhanceContent(ctx, content, options)
}

// cleanText cleans text content
func (p *ParagraphIndexProcessor) cleanText(text string, options *ProcessOptions) string {
	// Simple example: remove extra whitespace characters
	// text = strings.Join(strings.Fields(text), " ")
	return text
}

func (p *ParagraphIndexProcessor) buildTextChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	elements []dto.ExtractElement,
	textSplitter interface {
		SplitText(string) []string
	},
	autoFillEnabled bool,
	options *ProcessOptions,
) []dto.TransformedChunk {
	if len(elements) == 0 {
		return nil
	}

	contents := make([]string, 0, len(elements))
	for _, element := range elements {
		content := strings.TrimSpace(element.Content)
		if content != "" {
			contents = append(contents, content)
		}
	}

	cleanedText := p.cleanText(strings.Join(contents, "\n"), options)
	chunks := textSplitter.SplitText(cleanedText)
	transformed := make([]dto.TransformedChunk, 0, len(chunks))
	for _, chunk := range chunks {
		content := strings.TrimSpace(chunk)
		if content == "" {
			continue
		}

		if autoFillEnabled {
			enhancedContent, err := p.enhanceContent(ctx, content, options)
			if err == nil {
				content = enhancedContent
			}
		}

		transformed = append(transformed, dto.TransformedChunk{
			Content:  content,
			BBox:     bboxFromElements(elements),
			Metadata: metadataForElements(output, elements),
		})
	}

	return transformed
}

// Load loads documents
func (p *ParagraphIndexProcessor) Load(ctx context.Context, dataset *model.Dataset, chunks []dto.TransformedChunk, withKeywords bool, embeddingService embedding.EmbeddingService,
	documentRepo dataset_repository.DocumentRepository,
	vectorDB vectordb.VectorDB) (int, error) {
	items, err := p.buildIndexingItems(dataset, chunks)
	if err != nil {
		return 0, err
	}
	tokens, err := processIndexingItems(ctx, dataset, items, embeddingService, documentRepo, vectorDB, indexingBatchOptions{
		Name:          "paragraph",
		FailOnPartial: true,
	})
	if err != nil {
		return tokens, err
	}

	// Handle keywords if needed
	if withKeywords {
		// TODO: Implement logic for handling keyword-related data
		// This would involve creating a Keyword instance and adding texts to it
		// Keyword indexing will follow this flow:
		// keyword = Keyword(dataset)
		// keyword.add_texts(documents)
	}

	return tokens, nil
}

func (p *ParagraphIndexProcessor) buildIndexingItems(dataset *model.Dataset, chunks []dto.TransformedChunk) ([]indexingItem, error) {
	items := make([]indexingItem, 0, len(chunks))
	className := model.GenCollectionNameByID(dataset.ID)
	for _, chunk := range chunks {
		indexNodeID := getMetadataByKey(chunk.Metadata, "doc_id")
		if indexNodeID == "" {
			return nil, fmt.Errorf("failed to get doc_id from document metadata")
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

// Clean cleans documents
func (p *ParagraphIndexProcessor) Clean(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	// Get vector database instance
	vectorDB := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)

	// Generate collection name
	className := model.GenCollectionNameByID(dataset.ID)

	// If node ID list is provided, delete the specified vector objects
	if len(nodeIDs) > 0 {
		if err := vectorDB.DeleteObjectsByIDs(ctx, className, nodeIDs); err != nil {
			return fmt.Errorf("failed to delete vectors by IDs: %w", err)
		}
	}

	// If child chunks need to be deleted, process child chunk data (paragraph mode usually has no child chunks)
	if deleteChildChunks {
		// Paragraph mode usually does not involve child chunks, but this parameter is retained for interface consistency
		// If future functionality is expanded, relevant logic can be added here
	}

	// If keywords need to be processed, handle keyword-related data
	if withKeywords {
		// TODO: Implement logic for cleaning keyword-related data
	}

	return nil
}

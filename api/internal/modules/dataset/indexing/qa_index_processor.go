package indexing

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

// NewQAIndexProcessor
func NewQAIndexProcessor(storage storage.Storage, defaultModelSvc llmdefaultservice.DefaultModelService, llmClient llmclient.LLMClient, tenantID string) BaseIndexProcessor {
	return &QAIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(storage, defaultModelSvc, llmClient, tenantID),
	}
}

// QAIndexProcessor
type QAIndexProcessor struct {
	*BaseIndexProcessorImpl
}

// ExtractFromFile
func (q *QAIndexProcessor) ExtractFromFile(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error) {
	if setting.UploadFile == nil {
		return &dto.ExtractOutput{Source: "file"}, nil
	}

	return dto.NewExtractOutputFromDocuments("file", []dto.Document{
		{
			PageContent: "This is a sample QA document content extracted from file.",
			Metadata: map[string]interface{}{
				"source":  "file",
				"file_id": setting.UploadFile,
			},
		},
	}), nil
}

// ExtractFromNotion
func (q *QAIndexProcessor) ExtractFromNotion(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error) {
	// TODO: Notion
	return dto.NewExtractOutputFromDocuments("notion", []dto.Document{
		{
			PageContent: "This is a sample QA document content extracted from Notion.",
			Metadata: map[string]interface{}{
				"source":      "notion",
				"notion_info": setting.NotionInfo,
			},
		},
	}), nil
}

// ExtractFromWebsite
func (q *QAIndexProcessor) ExtractFromWebsite(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error) {
	// TODO:
	return dto.NewExtractOutputFromDocuments("website", []dto.Document{
		{
			PageContent: "This is a sample QA document content extracted from website.",
			Metadata: map[string]interface{}{
				"source":       "website",
				"website_info": setting.WebsiteInfo,
			},
		},
	}), nil
}

// Extract
func (q *QAIndexProcessor) Extract(ctx context.Context, setting *ExtractSetting, options *ProcessOptions) (*dto.ExtractOutput, error) {
	return q.BaseIndexProcessorImpl.Extract(ctx, setting, options)
}

// Transform transforms documents for QA processing
func (q *QAIndexProcessor) Transform(ctx context.Context, output *dto.ExtractOutput, options *ProcessOptions) ([]dto.TransformedChunk, error) {
	documents := dto.ExtractOutputToDocuments(output)
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
	textSplitter := q._get_splitter(chunkSize, chunkOverlap, separator)

	// Process all documents
	var allDocuments []dto.Document
	for _, document := range documents {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Clean document content
		cleanedText := document.PageContent

		// Split document
		chunks := textSplitter.SplitText(cleanedText)

		// Create split documents
		for i, chunk := range chunks {
			if strings.TrimSpace(chunk) != "" {
				content := strings.TrimSpace(chunk)

				// If auto fill is enabled, enhance each chunk using LLM
				if autoFillEnabled {
					enhancedContent, err := q.enhanceContent(ctx, content, options)
					if err == nil {
						// Use enhanced content if enhancement is successful
						content = enhancedContent
					}
				}

				newDoc := dto.Document{
					PageContent: content,
					Metadata:    map[string]interface{}{},
				}

				// Copy original document metadata
				for k, v := range document.Metadata {
					newDoc.Metadata[k] = v
				}

				// Add segmentation related metadata
				newDoc.Metadata["chunk_index"] = i
				newDoc.Metadata["total_chunks"] = len(chunks)
				newDoc.Metadata["doc_id"] = uuid.New().String()
				newDoc.Metadata["doc_hash"] = simpleHash(content)

				allDocuments = append(allDocuments, newDoc)
			}
		}
	}

	return dto.DocumentsToTransformedChunks(allDocuments), nil
}

// enhanceContent enhances document content using LLM when segment_content_auto_fill is enabled
func (q *QAIndexProcessor) enhanceContent(ctx context.Context, content string, options *ProcessOptions) (string, error) {
	return q.BaseIndexProcessorImpl.enhanceContent(ctx, content, options)
}

// Load
func (q *QAIndexProcessor) Load(ctx context.Context, dataset *model.Dataset, chunks []dto.TransformedChunk, withKeywords bool, embeddingService embedding.EmbeddingService,
	documentRepo dataset_repository.DocumentRepository,
	vectorDB vectordb.VectorDB) (int, error) {
	items, err := q.buildIndexingItems(dataset, chunks)
	if err != nil {
		return 0, err
	}
	tokens, err := processIndexingItems(ctx, dataset, items, embeddingService, documentRepo, vectorDB, indexingBatchOptions{
		Name:          "qa",
		FailOnPartial: false,
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

func (q *QAIndexProcessor) buildIndexingItems(dataset *model.Dataset, chunks []dto.TransformedChunk) ([]indexingItem, error) {
	items := make([]indexingItem, 0, len(chunks))
	for _, chunk := range chunks {
		indexNodeID := getMetadataByKey(chunk.Metadata, "doc_id")
		if indexNodeID == "" {
			return nil, fmt.Errorf("failed to get doc_id from document metadata")
		}

		questionID := getMetadataByKey(chunk.Metadata, "question_id")
		itemID := indexNodeID
		itemType := indexingItemTypeSegment
		className := model.GenCollectionNameByID(dataset.ID)
		if questionID != "" {
			itemID = questionID
			itemType = indexingItemTypeQuestion
			className = model.GenQuestionCollectionNameByID(dataset.ID)
		}

		properties := map[string]interface{}{
			"text":        chunk.Content,
			"doc_id":      indexNodeID,
			"doc_hash":    getMetadataByKey(chunk.Metadata, "doc_hash"),
			"document_id": getMetadataByKey(chunk.Metadata, "document_id"),
			"dataset_id":  dataset.ID,
		}
		for key, value := range chunk.Metadata {
			if key == "text" || key == "doc_id" || key == "doc_hash" || key == "document_id" || key == "dataset_id" {
				continue
			}
			properties[key] = value
		}

		items = append(items, indexingItem{
			IndexNodeID: itemID,
			Text:        chunk.Content,
			ClassName:   className,
			Properties:  properties,
			ItemType:    itemType,
		})
	}
	return items, nil
}

// Clean
func (q *QAIndexProcessor) Clean(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	// Default behavior: clean both regular and question classes
	return q.cleanInternal(ctx, dataset, nodeIDs, withKeywords, deleteChildChunks, false)
}

// CleanQuestionsOnly implements QAIndexProcessorExtension interface
func (q *QAIndexProcessor) CleanQuestionsOnly(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	// Clean only question classes
	return q.cleanInternal(ctx, dataset, nodeIDs, withKeywords, deleteChildChunks, true)
}

// cleanInternal is the internal implementation of Clean with additional control
// onlyQuestions: if true, only clean question class; if false, clean both regular and question classes
func (q *QAIndexProcessor) cleanInternal(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool, onlyQuestions bool) error {
	// Get vector database instance
	vectorDB := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)

	// Generate collection names for both regular segments and questions
	className := model.GenCollectionNameByID(dataset.ID)
	questionClassName := model.GenQuestionCollectionNameByID(dataset.ID)

	// If node ID list is provided, delete the specified vector objects
	if len(nodeIDs) > 0 {
		// Delete from regular segment class if not onlyQuestions
		if !onlyQuestions {
			if err := vectorDB.DeleteObjectsByIDs(ctx, className, nodeIDs); err != nil {
				return fmt.Errorf("failed to delete vectors by IDs from regular class: %w", err)
			}
		}

		// Delete from question class
		// We don't return error here because question class may not exist for some datasets
		if err := vectorDB.DeleteObjectsByIDs(ctx, questionClassName, nodeIDs); err != nil {
			logger.Warn("failed to delete vectors by IDs from question class (this may be expected if no questions exist)", map[string]interface{}{
				"dataset_id": dataset.ID,
				"error":      err,
			})
			// Continue execution as this error is not critical
		}
	}

	// If child chunks need to be deleted, process child chunk data (QA mode usually has no child chunks)
	if deleteChildChunks {
		// QA mode usually does not involve child chunks, but this parameter is retained for interface consistency
		// If future functionality is expanded, relevant logic can be added here
	}

	// If keywords need to be processed, handle keyword-related data
	if withKeywords {
		// TODO: Implement logic for cleaning keyword-related data
	}

	return nil
}

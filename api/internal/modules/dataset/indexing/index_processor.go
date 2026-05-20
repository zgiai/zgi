package indexing

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/splitter"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/storage"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

// ProcessOptions defines options for processing documents
type ProcessOptions struct {
	ProcessRule map[string]interface{}
	// Chunking options
	ChunkSize    int
	ChunkOverlap int
	Separator    string

	// Processing mode
	Mode string

	// Other options
	WithKeywords bool
}

// BaseIndexProcessor interface defines the contract for all index processors
type BaseIndexProcessor interface {
	ExtractFromFile(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error)
	ExtractFromNotion(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error)
	ExtractFromWebsite(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error)
	Extract(ctx context.Context, extractSetting *ExtractSetting, options *ProcessOptions) (*dto.ExtractOutput, error)
	Transform(ctx context.Context, output *dto.ExtractOutput, options *ProcessOptions) ([]dto.TransformedChunk, error)
	Load(ctx context.Context, dataset *model.Dataset, chunks []dto.TransformedChunk, withKeywords bool, embeddingService embedding.EmbeddingService,
		documentRepo dataset_repository.DocumentRepository,
		vectorDB vectordb.VectorDB) (int, error) // Return tokens count
	Clean(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error
}

// QAIndexProcessorExtension interface defines additional methods for QA index processors
type QAIndexProcessorExtension interface {
	CleanQuestionsOnly(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error
}

// ParentChildIndexProcessorExtension interface defines additional methods for parent-child index processors
type ParentChildIndexProcessorExtension interface {
	CleanParentOnly(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error
	CleanChildOnly(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error
}

// BaseIndexProcessorImpl provides common functionality for all index processors
type BaseIndexProcessorImpl struct {
	extractProcessor *extractor.ExtractProcessor
	defaultModelSvc  llmdefaultservice.DefaultModelService
	llmClient        llmclient.LLMClient
	tenantID         string
}

// NewBaseIndexProcessorImpl creates a new base index processor implementation
func NewBaseIndexProcessorImpl(storage storage.Storage, defaultModelSvc llmdefaultservice.DefaultModelService, llmClient llmclient.LLMClient, tenantID string) *BaseIndexProcessorImpl {
	return &BaseIndexProcessorImpl{
		extractProcessor: extractor.NewExtractProcessor(storage),
		defaultModelSvc:  defaultModelSvc,
		llmClient:        llmClient,
		tenantID:         tenantID,
	}
}

// getMaxSegmentationTokens gets the maximum segmentation token count
func (b *BaseIndexProcessorImpl) getMaxSegmentationTokens() int {
	// Check if config.GlobalConfig is nil
	if config.GlobalConfig == nil {
		return 1000 // Default value
	}

	tokens := config.GlobalConfig.VectorStore.IndexingMaxTokens
	if tokens < 50 {
		return 50
	}
	return tokens
}

// _get_splitter creates a text splitter
func (b *BaseIndexProcessorImpl) _get_splitter(chunkSize, chunkOverlap int, separator string) splitter.TextSplitter {
	// Limit chunkSize range
	maxSegmentationTokens := b.getMaxSegmentationTokens()
	if chunkSize < 50 || chunkSize > maxSegmentationTokens {
		// Here we log but don't return an error because this is a base method
		// Actual error handling should be done where this method is called
		if chunkSize < 50 {
			chunkSize = 50
		} else {
			chunkSize = maxSegmentationTokens
		}
	}

	separators := []string{"\n\n", "。", ". ", " ", ""}
	// Create text splitter
	textSplitter := splitter.NewFixedRecursiveCharacterTextSplitter(separator, separators, chunkSize, chunkOverlap, nil, false, false)
	return textSplitter
}

// Extract provides a unified implementation for document extraction
func (b *BaseIndexProcessorImpl) Extract(ctx context.Context, setting *ExtractSetting, options *ProcessOptions) (*dto.ExtractOutput, error) {

	var output *dto.ExtractOutput
	var err error

	switch setting.DataSourceType {
	case "upload_file":
		output, err = b.ExtractFromFile(ctx, *setting)
		if err != nil {
			return nil, err
		}
	case "notion_import":
		output, err = b.ExtractFromNotion(ctx, *setting)
		if err != nil {
			return nil, err
		}
	case "website_crawl":
		output, err = b.ExtractFromWebsite(ctx, *setting)
		if err != nil {
			return nil, err
		}
	case "reading":
		// Handle reading (text) data source
		if setting.Content == "" {
			return nil, fmt.Errorf("content is empty for reading data source")
		}
		output = dto.NewExtractOutputFromDocuments("reading", []dto.Document{
			{
				PageContent: setting.Content,
				Metadata: map[string]interface{}{
					"source":         "reading",
					"document_model": setting.DocumentModel,
				},
			},
		})
	case "text_input":
		if setting.Content == "" {
			return nil, fmt.Errorf("content is empty for text_input data source")
		}
		output = dto.NewExtractOutputFromDocuments("text_input", []dto.Document{
			{
				PageContent: setting.Content,
				Metadata: map[string]interface{}{
					"source":         "text_input",
					"document_model": setting.DocumentModel,
				},
			},
		})
	default:
		return nil, fmt.Errorf("[DEBUG PROCESSOR] unsupported data source type: %s", setting.DataSourceType)
	}

	return output, nil
}

// Transform
func (b *BaseIndexProcessorImpl) Transform(ctx context.Context, output *dto.ExtractOutput, options *ProcessOptions) ([]dto.TransformedChunk, error) {
	// TODO:
	return dto.DocumentsToTransformedChunks(dto.ExtractOutputToDocuments(output)), nil
}

// Load
func (b *BaseIndexProcessorImpl) Load(ctx context.Context, dataset *model.Dataset, chunks []dto.TransformedChunk, withKeywords bool, embeddingService embedding.EmbeddingService,
	documentRepo dataset_repository.DocumentRepository,
	vectorDB vectordb.VectorDB) (int, error) {
	// TODO:
	return 0, nil
}

// Clean cleans up documents
func (b *BaseIndexProcessorImpl) Clean(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
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

	// TODO: Implement withKeywords parameter processing logic (if needed)

	// TODO: Implement deleteChildChunks parameter processing logic (if needed)

	return nil
}

// ExtractFromFile
func (b *BaseIndexProcessorImpl) ExtractFromFile(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error) {
	if setting.UploadFile == nil {
		return &dto.ExtractOutput{Source: "upload_file"}, nil
	}

	if b.extractProcessor == nil {
		return dto.NewExtractOutputFromDocuments("file", []dto.Document{
			{
				PageContent: "This is a sample document content extracted from file.",
				Metadata: map[string]interface{}{
					"source":  "file",
					"file_id": setting.UploadFile,
				},
			},
		}), nil
	}

	// Convert indexing.UploadFile to model.UploadFile
	indexUploadFile := setting.UploadFile
	modelUploadFile := &file_model.UploadFile{
		ID:   indexUploadFile.ID,
		Name: indexUploadFile.Name,
		Size: indexUploadFile.Size,
		Key:  indexUploadFile.FilePath,
	}

	// Create extractor.ExtractSetting from local ExtractSetting
	extractorSetting := &extractor.ExtractSetting{
		DatasourceType:            setting.DataSourceType,
		UploadFile:                modelUploadFile,
		DocumentModel:             setting.DocumentModel,
		ProcessRule:               setting.ProcessRule,
		ExtractionStrategy:        setting.ExtractionStrategy,
		ExtractionFallbackEnabled: setting.ExtractionFallbackEnabled,
	}

	output, _, err := b.extractProcessor.LoadFromUploadFileWithSetting(ctx, modelUploadFile, false, false, extractorSetting)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// ExtractFromNotion
func (b *BaseIndexProcessorImpl) ExtractFromNotion(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error) {
	// TODO:
	return &dto.ExtractOutput{Source: "notion"}, nil
}

// ExtractFromWebsite
func (b *BaseIndexProcessorImpl) ExtractFromWebsite(ctx context.Context, setting ExtractSetting) (*dto.ExtractOutput, error) {
	// TODO:
	return &dto.ExtractOutput{Source: "website"}, nil
}

// enhanceContent enhances document content using LLM when segment_content_auto_fill is enabled
func (b *BaseIndexProcessorImpl) enhanceContent(ctx context.Context, content string, options *ProcessOptions) (string, error) {
	// Check if model manager is available
	if b.defaultModelSvc == nil || b.llmClient == nil {
		return content, nil // Return original content if runtime model services are unavailable
	}

	// Check if tenant ID is provided
	if b.tenantID == "" {
		return content, nil // Return original content if no tenant ID
	}

	resolvedModel, err := llmruntime.NewModelResolver(b.defaultModelSvc).ResolveDefault(ctx, b.tenantID, shared_model.ModelTypeLLM)
	if err != nil {
		return content, fmt.Errorf("failed to resolve chat model: %w", err)
	}

	// Create prompt for content enhancement
	prompt := fmt.Sprintf("请完善以下内容，使其更加完整连贯，保持原文风格和信息准确性：\n\n%s\n\n完善后的内容：", content)

	resp, err := b.llmClient.Chat(ctx, b.tenantID, &llmadapter.ChatRequest{
		Model: resolvedModel.Model,
		Messages: []llmadapter.Message{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	})
	if err != nil {
		return content, fmt.Errorf("failed to enhance content with LLM: %w", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return content, fmt.Errorf("failed to enhance content with LLM: empty chat response")
	}
	generatedContent, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(generatedContent) == "" {
		return content, fmt.Errorf("failed to enhance content with LLM: empty chat result")
	}

	return generatedContent, nil
}

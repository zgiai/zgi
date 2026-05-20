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
	"github.com/zgiai/zgi/api/internal/modules/dataset/splitter"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

var defaultSubchunkSeparators = []string{
	"\n\n",
	"\n",
	"\u3002",
	"\uff01",
	"\uff1f",
	"\uff1b",
	"\uff1a",
	". ",
	"! ",
	"? ",
	"; ",
	": ",
	".",
	"!",
	"?",
	";",
	":",
	"\uff0c",
	", ",
	",",
	"\u3001",
	" ",
	"",
}

// NewParentChildIndexProcessor creates a new parent-child index processor
func NewParentChildIndexProcessor(storage storage.Storage, documentRepo dataset_repository.DocumentRepository, defaultModelSvc llmdefaultservice.DefaultModelService, llmClient llmclient.LLMClient, tenantID string) BaseIndexProcessor {
	return &ParentChildIndexProcessor{
		BaseIndexProcessorImpl: NewBaseIndexProcessorImpl(storage, defaultModelSvc, llmClient, tenantID),
		documentRepo:           documentRepo,
	}
}

// ParentChildIndexProcessor parent-child index processor
type ParentChildIndexProcessor struct {
	*BaseIndexProcessorImpl
	documentRepo dataset_repository.DocumentRepository
}

// Extract extracts documents
func (p *ParentChildIndexProcessor) Extract(ctx context.Context, setting *ExtractSetting, options *ProcessOptions) (*dto.ExtractOutput, error) {
	return p.BaseIndexProcessorImpl.Extract(ctx, setting, options)
}

// getMaxSegmentationTokens gets the maximum segmentation token count
func (p *ParentChildIndexProcessor) getMaxSegmentationTokens() int {
	tokens := config.GlobalConfig.VectorStore.IndexingMaxTokens
	if tokens < 50 {
		return 50
	}
	return tokens
}

// Transform transforms extracted elements into parent chunks with child chunks.
func (p *ParentChildIndexProcessor) Transform(ctx context.Context, output *dto.ExtractOutput, options *ProcessOptions) ([]dto.TransformedChunk, error) {
	if p.BaseIndexProcessorImpl == nil {
		return nil, fmt.Errorf("BaseIndexProcessorImpl is not initialized")
	}

	if options == nil {
		return nil, fmt.Errorf("process options is nil")
	}

	processRule := options.ProcessRule
	if processRule == nil {
		return nil, fmt.Errorf("process rule is nil")
	}

	autoFillEnabled := false

	rule, err := ParseRule(processRule)
	if err != nil {
		return nil, fmt.Errorf("failed to parse rule: %w", err)
	}
	if rule.SubchunkSegmentation == nil {
		return nil, fmt.Errorf("subchunk segmentation rule is nil")
	}

	parentMode := "paragraph"
	if rule.ParentMode != nil {
		parentMode = strings.ToLower(strings.TrimSpace(*rule.ParentMode))
	}

	var parentChunks []dto.TransformedChunk
	switch parentMode {
	case "", "paragraph", "parent_child":
		if rule.Segmentation == nil {
			return nil, fmt.Errorf("segmentation rule is nil")
		}
		parentChunks, err = p.buildParagraphParentChunks(ctx, output, rule.Segmentation, autoFillEnabled, options)
	case "full-doc":
		parentChunks, err = p.buildFullDocParentChunks(ctx, output, autoFillEnabled, options)
	case "section":
		parentChunks, err = p.buildSectionParentChunks(ctx, output, autoFillEnabled, options)
	default:
		return nil, fmt.Errorf("unsupported parent mode: %s", parentMode)
	}
	if err != nil {
		return nil, err
	}
	if len(parentChunks) == 0 {
		return parentChunks, nil
	}

	addChunkMetadata(parentChunks)
	transformedChunks, err := p._splitChildNodes(ctx, rule, parentChunks)
	if err != nil {
		return nil, fmt.Errorf("failed to split child nodes: %w", err)
	}

	return transformedChunks, nil
}

func (p *ParentChildIndexProcessor) buildParagraphParentChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	segmentation *SegmentationRule,
	autoFillEnabled bool,
	options *ProcessOptions,
) ([]dto.TransformedChunk, error) {
	textSplitter := p._get_splitter(segmentation.MaxTokens, segmentation.ChunkOverlap, segmentation.Separator)
	elements := sortedExtractElements(output)
	if len(elements) == 0 {
		return p.buildFallbackParentChunks(ctx, output, textSplitter, autoFillEnabled, options)
	}

	parentChunks := make([]dto.TransformedChunk, 0, len(elements))
	textElements := make([]dto.ExtractElement, 0)
	for _, element := range elements {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if isParagraphTextElement(element.Type) {
			textElements = append(textElements, element)
			continue
		}

		textChunks, err := p.buildParentTextChunks(ctx, output, textElements, textSplitter, autoFillEnabled, options)
		if err != nil {
			return nil, err
		}
		parentChunks = append(parentChunks, textChunks...)
		textElements = textElements[:0]

		if !isStandaloneElement(element.Type) {
			continue
		}

		chunk, ok, err := p.buildStandaloneParentChunk(ctx, output, element, autoFillEnabled, options)
		if err != nil {
			return nil, err
		}
		if ok {
			parentChunks = append(parentChunks, chunk)
		}
	}

	textChunks, err := p.buildParentTextChunks(ctx, output, textElements, textSplitter, autoFillEnabled, options)
	if err != nil {
		return nil, err
	}
	parentChunks = append(parentChunks, textChunks...)

	return parentChunks, nil
}

func (p *ParentChildIndexProcessor) buildParentTextChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	elements []dto.ExtractElement,
	textSplitter interface {
		SplitText(string) []string
	},
	autoFillEnabled bool,
	options *ProcessOptions,
) ([]dto.TransformedChunk, error) {
	if len(elements) == 0 {
		return nil, nil
	}

	contents := make([]string, 0, len(elements))
	for _, element := range elements {
		content := strings.TrimSpace(element.Content)
		if content != "" {
			contents = append(contents, content)
		}
	}
	if len(contents) == 0 {
		return nil, nil
	}

	contentChunks := textSplitter.SplitText(strings.Join(contents, "\n"))
	parentChunks := make([]dto.TransformedChunk, 0, len(contentChunks))
	for _, chunk := range contentChunks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

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

		parentChunks = append(parentChunks, dto.TransformedChunk{
			Content:  content,
			BBox:     bboxFromElements(elements),
			Metadata: parentChildMetadataForElements(output, elements),
		})
	}

	return parentChunks, nil
}

func (p *ParentChildIndexProcessor) buildStandaloneParentChunk(
	ctx context.Context,
	output *dto.ExtractOutput,
	element dto.ExtractElement,
	autoFillEnabled bool,
	options *ProcessOptions,
) (dto.TransformedChunk, bool, error) {
	if ctx.Err() != nil {
		return dto.TransformedChunk{}, false, ctx.Err()
	}

	content := strings.TrimSpace(element.Content)
	if content == "" {
		return dto.TransformedChunk{}, false, nil
	}
	if autoFillEnabled {
		enhancedContent, err := p.enhanceContent(ctx, content, options)
		if err == nil {
			content = enhancedContent
		}
	}

	return dto.TransformedChunk{
		Content:  content,
		BBox:     bboxFromElements([]dto.ExtractElement{element}),
		Metadata: parentChildMetadataForElements(output, []dto.ExtractElement{element}),
	}, true, nil
}

func (p *ParentChildIndexProcessor) buildFullDocParentChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	autoFillEnabled bool,
	options *ProcessOptions,
) ([]dto.TransformedChunk, error) {
	elements := sortedExtractElements(output)
	content := strings.TrimSpace(dto.ExtractOutputText(output))
	if content == "" {
		return nil, nil
	}
	if autoFillEnabled {
		enhancedContent, err := p.enhanceContent(ctx, content, options)
		if err == nil {
			content = enhancedContent
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return []dto.TransformedChunk{
		{
			Content:  content,
			BBox:     bboxFromElements(elements),
			Metadata: parentChildMetadataForElements(output, elements),
		},
	}, nil
}

func (p *ParentChildIndexProcessor) buildSectionParentChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	autoFillEnabled bool,
	options *ProcessOptions,
) ([]dto.TransformedChunk, error) {
	elements := sortedExtractElements(output)
	if len(elements) == 0 {
		return nil, nil
	}

	var parentChunks []dto.TransformedChunk
	var currentSectionElements []dto.ExtractElement

	flushSection := func() error {
		if len(currentSectionElements) == 0 {
			return nil
		}

		contents := make([]string, 0, len(currentSectionElements))
		for _, elem := range currentSectionElements {
			if strings.TrimSpace(elem.Content) != "" {
				contents = append(contents, elem.Content)
			}
		}

		if len(contents) > 0 {
			content := strings.Join(contents, "\n")
			if autoFillEnabled {
				enhancedContent, err := p.enhanceContent(ctx, content, options)
				if err == nil {
					content = enhancedContent
				}
			}

			parentChunks = append(parentChunks, dto.TransformedChunk{
				Content:  content,
				BBox:     bboxFromElements(currentSectionElements),
				Metadata: parentChildMetadataForElements(output, currentSectionElements),
			})
		}

		currentSectionElements = nil
		return nil
	}

	for _, element := range elements {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if isStandaloneElement(element.Type) {
			if err := flushSection(); err != nil {
				return nil, err
			}
			chunk, ok, err := p.buildStandaloneParentChunk(ctx, output, element, autoFillEnabled, options)
			if err != nil {
				return nil, err
			}
			if ok {
				parentChunks = append(parentChunks, chunk)
			}
		} else if strings.ToLower(element.Type) == "heading" {
			if err := flushSection(); err != nil {
				return nil, err
			}
			currentSectionElements = append(currentSectionElements, element)
		} else {
			currentSectionElements = append(currentSectionElements, element)
		}
	}

	if err := flushSection(); err != nil {
		return nil, err
	}

	return parentChunks, nil
}

func (p *ParentChildIndexProcessor) buildFallbackParentChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	textSplitter interface {
		SplitText(string) []string
	},
	autoFillEnabled bool,
	options *ProcessOptions,
) ([]dto.TransformedChunk, error) {
	content := strings.TrimSpace(dto.ExtractOutputText(output))
	if content == "" {
		return nil, nil
	}

	contentChunks := textSplitter.SplitText(content)
	parentChunks := make([]dto.TransformedChunk, 0, len(contentChunks))
	for _, chunk := range contentChunks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		parentContent := strings.TrimSpace(chunk)
		if parentContent == "" {
			continue
		}
		if autoFillEnabled {
			enhancedContent, err := p.enhanceContent(ctx, parentContent, options)
			if err == nil {
				parentContent = enhancedContent
			}
		}

		parentChunks = append(parentChunks, dto.TransformedChunk{
			Content:  parentContent,
			Metadata: parentChildMetadataForElements(output, nil),
		})
	}

	return parentChunks, nil
}

func parentChildMetadataForElements(output *dto.ExtractOutput, elements []dto.ExtractElement) map[string]any {
	metadata := metadataForElements(output, elements)
	delete(metadata, "children")
	return metadata
}

// enhanceContent enhances document content using LLM when segment_content_auto_fill is enabled
func (p *ParentChildIndexProcessor) enhanceContent(ctx context.Context, content string, options *ProcessOptions) (string, error) {
	return p.BaseIndexProcessorImpl.enhanceContent(ctx, content, options)
}

// Load loads documents
func (p *ParentChildIndexProcessor) Load(ctx context.Context, dataset *model.Dataset, chunks []dto.TransformedChunk, withKeywords bool, embeddingService embedding.EmbeddingService,
	documentRepo dataset_repository.DocumentRepository,
	vectorDB vectordb.VectorDB) (int, error) {
	items, err := p.buildIndexingItems(dataset, chunks)
	if err != nil {
		return 0, err
	}
	return processIndexingItems(ctx, dataset, items, embeddingService, documentRepo, vectorDB, indexingBatchOptions{
		Name:          "parent-child",
		FailOnPartial: false,
	})
}

func (p *ParentChildIndexProcessor) buildIndexingItems(dataset *model.Dataset, chunks []dto.TransformedChunk) ([]indexingItem, error) {
	className := model.GenCollectionNameByID(dataset.ID)
	items := make([]indexingItem, 0, len(chunks))
	for _, parentChunk := range chunks {
		parentIndexNodeID := getMetadataByKey(parentChunk.Metadata, "doc_id")
		if parentIndexNodeID == "" {
			return nil, fmt.Errorf("failed to get doc_id from parent chunk metadata")
		}

		if len(parentChunk.Children) == 0 {
			logger.Warn("No child nodes found for document", parentChunk.Metadata["doc_id"])
			items = append(items, indexingItem{
				IndexNodeID: parentIndexNodeID,
				Text:        parentChunk.Content,
				ClassName:   className,
				Properties: map[string]interface{}{
					"text":        parentChunk.Content,
					"doc_id":      parentIndexNodeID,
					"doc_hash":    getMetadataByKey(parentChunk.Metadata, "doc_hash"),
					"document_id": getMetadataByKey(parentChunk.Metadata, "document_id"),
					"dataset_id":  dataset.ID,
				},
				ItemType: indexingItemTypeSegment,
			})
			continue
		}

		for _, child := range parentChunk.Children {
			indexNodeID := getMetadataByKey(child.Metadata, "doc_id")
			if indexNodeID == "" {
				return nil, fmt.Errorf("failed to get doc_id from child chunk metadata")
			}
			items = append(items, indexingItem{
				IndexNodeID:       indexNodeID,
				Text:              child.Content,
				ClassName:         className,
				ParentIndexNodeID: parentIndexNodeID,
				Properties: map[string]interface{}{
					"text":        child.Content,
					"doc_id":      indexNodeID,
					"doc_hash":    getMetadataByKey(child.Metadata, "doc_hash"),
					"document_id": getMetadataByKey(parentChunk.Metadata, "document_id"),
					"dataset_id":  dataset.ID,
				},
				ItemType: indexingItemTypeChild,
			})
		}
	}
	return items, nil
}

// Clean cleans documents
func (p *ParentChildIndexProcessor) Clean(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	// Default behavior: clean child first, then parent if needed
	if deleteChildChunks {
		if err := p.CleanChildOnly(ctx, dataset, nodeIDs, withKeywords, false); err != nil {
			logger.Error("Failed to clean child chunks", err)
		}
	}
	return p.CleanParentOnly(ctx, dataset, nodeIDs, withKeywords, deleteChildChunks)
}

// CleanParentOnly implements ParentChildIndexProcessorExtension interface
func (p *ParentChildIndexProcessor) CleanParentOnly(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	// Get vector database instance
	vectorDB := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)

	// Generate collection name
	className := model.GenCollectionNameByID(dataset.ID)

	// If node ID list is provided, delete the specified vector objects
	if len(nodeIDs) > 0 {
		if err := vectorDB.DeleteObjectsByIDs(ctx, className, nodeIDs); err != nil {
			return fmt.Errorf("failed to delete vectors by IDs: %w", err)
		}

		// If child chunks need to be deleted, process child chunk data
		if deleteChildChunks {
			// Query child node IDs based on parent node IDs (segment IDs)
			childChunks, err := p.documentRepo.GetChildChunksByIndexNodeIDs(ctx, nodeIDs)
			if err != nil {
				logger.Error("Failed to query child chunks by node IDs", err)
				return err
			}

			var childNodeIDs []string
			for _, childChunk := range childChunks {
				if childChunk.IndexNodeID != nil {
					childNodeIDs = append(childNodeIDs, *childChunk.IndexNodeID)
				}
			}

			// Delete child nodes from vector database
			if len(childNodeIDs) > 0 {
				if err := vectorDB.DeleteObjectsByIDs(ctx, className, childNodeIDs); err != nil {
					logger.Error("Failed to delete child vectors by IDs", err)
					// Continue with database deletion anyway
				}
			}

			// Delete child chunks from database
			if err := p.documentRepo.DeleteChildChunksByIndexNodeIDs(ctx, childNodeIDs); err != nil {
				logger.Error("Failed to delete child chunks from database", err)
				return err
			}
		}
	}

	// If keywords need to be processed, handle keyword-related data
	if withKeywords {
		// TODO: Implement logic for cleaning keyword-related data
	}

	return nil
}

// CleanChildOnly implements ParentChildIndexProcessorExtension interface
func (p *ParentChildIndexProcessor) CleanChildOnly(ctx context.Context, dataset *model.Dataset, nodeIDs []string, withKeywords bool, deleteChildChunks bool) error {
	// Get vector database instance
	vectorDB := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)

	// Generate collection name
	className := model.GenCollectionNameByID(dataset.ID)

	// If node ID list is provided (these are parent segment IDs), query child node IDs
	if len(nodeIDs) > 0 {
		childChunks, err := p.documentRepo.GetChildChunksByIndexNodeIDs(ctx, nodeIDs)
		if err != nil {
			logger.Error("Failed to query child chunks by node IDs", err)
			return err
		}

		var childNodeIDs []string
		for _, childChunk := range childChunks {
			if childChunk.IndexNodeID != nil {
				childNodeIDs = append(childNodeIDs, *childChunk.IndexNodeID)
			}
		}

		// Delete child nodes from vector database
		if len(childNodeIDs) > 0 {
			if err := vectorDB.DeleteObjectsByIDs(ctx, className, childNodeIDs); err != nil {
				logger.Error("Failed to delete child vectors by IDs", err)
				// Continue with database deletion anyway
			}

			// Delete child chunks from database
			if err := p.documentRepo.DeleteChildChunksByIndexNodeIDs(ctx, childNodeIDs); err != nil {
				logger.Error("Failed to delete child chunks from database", err)
				return err
			}
		}
	}

	return nil
}

// _splitChildNodes attaches subchunks to each parent chunk.
func (p *ParentChildIndexProcessor) _splitChildNodes(ctx context.Context, rule *Rule, chunks []dto.TransformedChunk) ([]dto.TransformedChunk, error) {
	if rule.SubchunkSegmentation == nil {
		return chunks, nil
	}

	fixedSeparator, separators := buildSubchunkSeparators(rule.SubchunkSegmentation.Separator)

	subchunkSplitter := splitter.NewFixedRecursiveCharacterTextSplitter(
		fixedSeparator,
		separators,
		rule.SubchunkSegmentation.MaxTokens,
		rule.SubchunkSegmentation.ChunkOverlap,
		nil,   // Use default length function
		false, // Do not keep separator
		false, // Do not add start index
	)

	transformedChunks := make([]dto.TransformedChunk, 0, len(chunks))

	for _, chunk := range chunks {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		strippedContent := strings.TrimSpace(chunk.Content)
		contentChunks := subchunkSplitter.SplitText(strippedContent)

		nonEmptyChunks := make([]string, 0, len(contentChunks))
		for _, chunk := range contentChunks {
			trimmedChunk := strings.TrimSpace(chunk)
			if trimmedChunk != "" {
				nonEmptyChunks = append(nonEmptyChunks, trimmedChunk)
			}
		}

		if len(nonEmptyChunks) < 1 {
			chunk.Content = strippedContent
			transformedChunks = append(transformedChunks, chunk)
			continue
		}

		parentMetadata := cloneChunkMetadata(chunk.Metadata)
		if _, ok := parentMetadata["doc_id"]; !ok {
			parentMetadata["doc_id"] = uuid.New().String()
		}
		if _, ok := parentMetadata["doc_hash"]; !ok {
			parentMetadata["doc_hash"] = simpleHash(strippedContent)
		}
		parentMetadata["is_parent"] = true
		parentMetadata["child_count"] = len(nonEmptyChunks)
		parentID := parentMetadata["doc_id"]

		childChunks := make([]dto.TransformedChildChunk, 0, len(nonEmptyChunks))
		for i, content := range nonEmptyChunks {
			childMetadata := cloneChunkMetadata(parentMetadata)
			childMetadata["parent_id"] = parentID
			childMetadata["is_child"] = true
			childMetadata["child_index"] = i
			childMetadata["doc_id"] = uuid.New().String()
			childMetadata["doc_hash"] = simpleHash(content)

			childChunks = append(childChunks, dto.TransformedChildChunk{
				Content:  content,
				BBox:     chunk.BBox,
				Metadata: childMetadata,
			})
		}

		chunk.Content = strippedContent
		chunk.Metadata = parentMetadata
		chunk.Children = childChunks
		transformedChunks = append(transformedChunks, chunk)
	}

	return transformedChunks, nil
}

func cloneChunkMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}

	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func buildSubchunkSeparators(preferredSeparator string) (string, []string) {
	fixedSeparator := preferredSeparator
	if strings.TrimSpace(fixedSeparator) == "" {
		fixedSeparator = "\n\n"
	}

	separators := make([]string, 0, len(defaultSubchunkSeparators)+1)
	seen := make(map[string]struct{}, len(defaultSubchunkSeparators)+1)
	addSeparator := func(separator string) {
		if _, ok := seen[separator]; ok {
			return
		}
		seen[separator] = struct{}{}
		separators = append(separators, separator)
	}

	addSeparator(fixedSeparator)
	for _, separator := range defaultSubchunkSeparators {
		addSeparator(separator)
	}

	return fixedSeparator, separators
}

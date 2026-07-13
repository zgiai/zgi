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

const defaultParentChunkMergeTarget = 500

const structuredElementsMetadataKey = "structured_elements"

const (
	defaultElementGroupParentMinChars    = 1000
	defaultElementGroupParentTargetChars = 1200
	defaultElementGroupParentMaxChars    = 1500
	defaultElementGroupChildMinChars     = 120
	defaultElementGroupChildTargetChars  = 220
	defaultElementGroupChildMaxChars     = 256
	defaultElementGroupChildOverlapChars = 30
	defaultElementGroupTableMaxChars     = 256
)

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
	fallbackToParagraph := false
	switch parentMode {
	case "", "paragraph":
		if rule.Segmentation == nil {
			return nil, fmt.Errorf("segmentation rule is nil")
		}
		parentChunks, err = p.buildParagraphParentChunks(ctx, output, rule.Segmentation, autoFillEnabled, options)
	case "parent_child":
		if rule.Segmentation == nil {
			return nil, fmt.Errorf("segmentation rule is nil")
		}
		parentChunks, err = p.buildParagraphParentChunks(ctx, output, rule.Segmentation, autoFillEnabled, options)
	case "element_group":
		if hasStructuredElementInput(output) {
			return p.buildElementGroupTransformedChunks(ctx, output, rule, autoFillEnabled, options)
		}
		if rule.Segmentation == nil {
			return nil, fmt.Errorf("segmentation rule is nil")
		}
		fallbackToParagraph = true
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
	if parentMode == "" || parentMode == "paragraph" || parentMode == "parent_child" || fallbackToParagraph {
		parentChunks = mergeSmallParentChunks(parentChunks, parentChunkMergeTarget(rule.Segmentation))
	}
	if len(parentChunks) == 0 {
		return parentChunks, nil
	}

	addChunkMetadata(parentChunks)
	if fallbackToParagraph {
		for i := range parentChunks {
			if parentChunks[i].Metadata == nil {
				parentChunks[i].Metadata = map[string]any{}
			}
			parentChunks[i].Metadata["requested_parent_mode"] = "element_group"
			parentChunks[i].Metadata["effective_parent_mode"] = "paragraph"
			parentChunks[i].Metadata["fallback_reason"] = "structured_elements_unavailable"
		}
	}
	transformedChunks, err := p._splitChildNodes(ctx, rule, parentChunks)
	if err != nil {
		return nil, fmt.Errorf("failed to split child nodes: %w", err)
	}

	return transformedChunks, nil
}

func hasStructuredElementInput(output *dto.ExtractOutput) bool {
	if output == nil || len(output.Elements) == 0 || output.Metadata == nil {
		return false
	}
	structured, ok := output.Metadata[structuredElementsMetadataKey].(bool)
	return ok && structured
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

type elementGroupParams struct {
	ParentMinChars    int
	ParentTargetChars int
	ParentMaxChars    int
	ChildMinChars     int
	ChildTargetChars  int
	ChildMaxChars     int
	ChildOverlapChars int
	TableMaxChars     int
}

func elementGroupParamsFromRule(rule *Rule) elementGroupParams {
	params := elementGroupParams{
		ParentMinChars:    defaultElementGroupParentMinChars,
		ParentTargetChars: defaultElementGroupParentTargetChars,
		ParentMaxChars:    defaultElementGroupParentMaxChars,
		ChildMinChars:     defaultElementGroupChildMinChars,
		ChildTargetChars:  defaultElementGroupChildTargetChars,
		ChildMaxChars:     defaultElementGroupChildMaxChars,
		ChildOverlapChars: defaultElementGroupChildOverlapChars,
		TableMaxChars:     defaultElementGroupTableMaxChars,
	}
	if rule == nil {
		return params
	}
	if rule.ParentMinChars > 0 {
		params.ParentMinChars = rule.ParentMinChars
	}
	if rule.ParentTargetChars > 0 {
		params.ParentTargetChars = rule.ParentTargetChars
	}
	if rule.ParentMaxChars > 0 {
		params.ParentMaxChars = rule.ParentMaxChars
	}
	if rule.ChildMinChars > 0 {
		params.ChildMinChars = rule.ChildMinChars
	}
	if rule.ChildTargetChars > 0 {
		params.ChildTargetChars = rule.ChildTargetChars
	}
	if rule.ChildMaxChars > 0 {
		params.ChildMaxChars = rule.ChildMaxChars
	}
	if rule.ChildOverlapChars > 0 {
		params.ChildOverlapChars = rule.ChildOverlapChars
	}
	if rule.TableChildMaxChars > 0 {
		params.TableMaxChars = rule.TableChildMaxChars
	}
	if params.ParentMinChars > params.ParentTargetChars {
		params.ParentMinChars = params.ParentTargetChars
	}
	if params.ParentTargetChars > params.ParentMaxChars {
		params.ParentTargetChars = params.ParentMaxChars
	}
	if params.ChildMinChars > params.ChildTargetChars {
		params.ChildMinChars = params.ChildTargetChars
	}
	if params.ChildTargetChars > params.ChildMaxChars {
		params.ChildTargetChars = params.ChildMaxChars
	}
	if params.ChildOverlapChars >= params.ChildTargetChars {
		params.ChildOverlapChars = params.ChildTargetChars / 5
	}
	return params
}

func (p *ParentChildIndexProcessor) buildElementGroupTransformedChunks(
	ctx context.Context,
	output *dto.ExtractOutput,
	rule *Rule,
	autoFillEnabled bool,
	options *ProcessOptions,
) ([]dto.TransformedChunk, error) {
	params := elementGroupParamsFromRule(rule)
	groups, err := p.buildElementGroups(ctx, output, params)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, nil
	}

	chunks := make([]dto.TransformedChunk, 0, len(groups))
	for _, group := range groups {
		content := strings.TrimSpace(joinElementContents(group))
		if content == "" {
			continue
		}
		if autoFillEnabled {
			enhancedContent, err := p.enhanceContent(ctx, content, options)
			if err == nil {
				content = enhancedContent
			}
		}
		metadata := parentChildMetadataForElements(output, group)
		metadata["parent_mode"] = "element_group"
		metadata["source_element_ids"] = sourceElementIDsFromExtract(group)
		metadata["source_start_order"] = group[0].Ordinal
		metadata["source_end_order"] = group[len(group)-1].Ordinal
		metadata["source_char_count"] = elementGroupRuneLen(group)
		chunks = append(chunks, dto.TransformedChunk{
			Content:  content,
			BBox:     bboxFromElements(group),
			Metadata: metadata,
			Children: p.buildElementGroupChildren(group, params),
		})
	}

	addChunkMetadata(chunks)
	for i := range chunks {
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = map[string]any{}
		}
		chunks[i].Metadata["is_parent"] = true
		chunks[i].Metadata["child_count"] = len(chunks[i].Children)
		parentID := chunks[i].Metadata["doc_id"]
		for j := range chunks[i].Children {
			if chunks[i].Children[j].Metadata == nil {
				chunks[i].Children[j].Metadata = map[string]any{}
			}
			chunks[i].Children[j].Metadata["parent_id"] = parentID
			chunks[i].Children[j].Metadata["is_child"] = true
			chunks[i].Children[j].Metadata["child_index"] = j
			if _, ok := chunks[i].Children[j].Metadata["doc_id"]; !ok {
				chunks[i].Children[j].Metadata["doc_id"] = uuid.New().String()
			}
			if _, ok := chunks[i].Children[j].Metadata["doc_hash"]; !ok {
				chunks[i].Children[j].Metadata["doc_hash"] = simpleHash(chunks[i].Children[j].Content)
			}
		}
	}

	return chunks, nil
}

func (p *ParentChildIndexProcessor) buildElementGroups(ctx context.Context, output *dto.ExtractOutput, params elementGroupParams) ([][]dto.ExtractElement, error) {
	elements := sortedExtractElements(output)
	if len(elements) == 0 {
		return nil, nil
	}

	groups := make([][]dto.ExtractElement, 0)
	current := make([]dto.ExtractElement, 0)
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		groups = append(groups, append([]dto.ExtractElement(nil), current...))
		current = current[:0]
		currentLen = 0
	}

	for _, element := range elements {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if strings.TrimSpace(elementSearchText(element)) == "" {
			continue
		}
		elementLen := elementRuneLen(element)
		if len(current) == 0 {
			current = append(current, element)
			currentLen = elementLen
			continue
		}
		nextLen := currentLen + 1 + elementLen
		if currentLen >= params.ParentMinChars && nextLen > params.ParentMaxChars {
			flush()
			current = append(current, element)
			currentLen = elementLen
			continue
		}
		if currentLen < params.ParentMinChars && nextLen > params.ParentMaxChars && elementGroupDistance(nextLen, params.ParentTargetChars) > elementGroupDistance(currentLen, params.ParentTargetChars) {
			flush()
			current = append(current, element)
			currentLen = elementLen
			continue
		}
		current = append(current, element)
		currentLen = nextLen
	}
	flush()
	return mergeSmallTrailingElementGroup(groups, params.ParentMinChars, params.ParentMaxChars), nil
}

func mergeSmallTrailingElementGroup(groups [][]dto.ExtractElement, minChars, maxChars int) [][]dto.ExtractElement {
	if len(groups) < 2 {
		return groups
	}
	last := groups[len(groups)-1]
	if elementGroupRuneLen(last) >= minChars {
		return groups
	}
	prev := groups[len(groups)-2]
	if elementGroupRuneLen(prev)+1+elementGroupRuneLen(last) > maxChars {
		return groups
	}
	groups[len(groups)-2] = append(append([]dto.ExtractElement(nil), prev...), last...)
	return groups[:len(groups)-1]
}

func elementGroupDistance(value, target int) int {
	if value > target {
		return value - target
	}
	return target - value
}

func (p *ParentChildIndexProcessor) buildElementGroupChildren(elements []dto.ExtractElement, params elementGroupParams) []dto.TransformedChildChunk {
	children := make([]dto.TransformedChildChunk, 0)
	textGroup := make([]dto.ExtractElement, 0)
	flushText := func() {
		if len(textGroup) == 0 {
			return
		}
		children = append(children, buildTextElementChildren(textGroup, params)...)
		textGroup = textGroup[:0]
	}

	for _, element := range elements {
		kind := normalizedElementGroupKind(element.Type)
		switch kind {
		case "image", "formula":
			flushText()
			if child, ok := buildAtomicElementChild(element, kind); ok {
				children = append(children, child)
			}
		case "table":
			flushText()
			children = append(children, buildTableElementChildren(element, params)...)
		default:
			textGroup = append(textGroup, element)
		}
	}
	flushText()
	return children
}

func buildTextElementChildren(elements []dto.ExtractElement, params elementGroupParams) []dto.TransformedChildChunk {
	content := strings.TrimSpace(joinElementContents(elements))
	if content == "" {
		return nil
	}
	parts := splitElementGroupText(content, params.ChildTargetChars, params.ChildMinChars, params.ChildMaxChars, params.ChildOverlapChars)
	children := make([]dto.TransformedChildChunk, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		children = append(children, dto.TransformedChildChunk{
			Content:  trimmed,
			BBox:     bboxFromElements(elements),
			Metadata: childMetadataForElements(elements, "text"),
		})
	}
	return children
}

func buildAtomicElementChild(element dto.ExtractElement, kind string) (dto.TransformedChildChunk, bool) {
	content := strings.TrimSpace(elementSearchText(element))
	if content == "" {
		return dto.TransformedChildChunk{}, false
	}
	return dto.TransformedChildChunk{
		Content:  content,
		BBox:     element.BBox,
		Metadata: childMetadataForElements([]dto.ExtractElement{element}, kind),
	}, true
}

func buildTableElementChildren(element dto.ExtractElement, params elementGroupParams) []dto.TransformedChildChunk {
	content := strings.TrimSpace(elementSearchText(element))
	if content == "" {
		return nil
	}
	if len([]rune(content)) <= params.TableMaxChars {
		return []dto.TransformedChildChunk{{
			Content:  content,
			BBox:     element.BBox,
			Metadata: childMetadataForElements([]dto.ExtractElement{element}, "table"),
		}}
	}
	header, rows := parseElementMarkdownTable(content)
	if len(header) == 0 || len(rows) == 0 {
		return buildFallbackTableChildren(element, content, params)
	}
	groups := groupElementTableRows(header, rows, params.TableMaxChars)
	children := make([]dto.TransformedChildChunk, 0, len(groups))
	for i, group := range groups {
		childContent := renderElementTableRows(header, group)
		if len([]rune(childContent)) > params.TableMaxChars && len(group) == 1 {
			children = append(children, buildLongTableRowChildren(element, header, group[0], i, len(groups), params)...)
			continue
		}
		metadata := childMetadataForElements([]dto.ExtractElement{element}, "table")
		metadata["table_part_index"] = i
		metadata["table_part_count"] = len(groups)
		metadata["table_header_repeated"] = true
		children = append(children, dto.TransformedChildChunk{
			Content:  strings.TrimSpace(childContent),
			BBox:     element.BBox,
			Metadata: metadata,
		})
	}
	return children
}

func buildLongTableRowChildren(element dto.ExtractElement, header []string, row []string, partIndex, partCount int, params elementGroupParams) []dto.TransformedChildChunk {
	context := "Table columns: " + strings.Join(header, " | ")
	children := make([]dto.TransformedChildChunk, 0, len(row))
	for i, cell := range row {
		column := fmt.Sprintf("Column %d", i+1)
		if i < len(header) && strings.TrimSpace(header[i]) != "" {
			column = strings.TrimSpace(header[i])
		}
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		prefix := context + "\n" + column + ": "
		budget := params.ChildMaxChars - len([]rune(prefix))
		if budget < 20 {
			budget = params.ChildMaxChars
			prefix = column + ": "
		}
		parts := splitElementGroupText(cell, budget, 1, budget, 0)
		for splitIndex, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			metadata := childMetadataForElements([]dto.ExtractElement{element}, "table")
			metadata["table_part_index"] = partIndex
			metadata["table_part_count"] = partCount
			metadata["table_row_split_index"] = len(children)
			metadata["table_column_index"] = i
			metadata["table_column_name"] = column
			metadata["table_column_split_index"] = splitIndex
			metadata["table_column_split_count"] = len(parts)
			metadata["table_header_repeated"] = true
			children = append(children, dto.TransformedChildChunk{
				Content:  strings.TrimSpace(prefix + trimmed),
				BBox:     element.BBox,
				Metadata: metadata,
			})
		}
	}
	for i := range children {
		children[i].Metadata["table_row_split_count"] = len(children)
	}
	return children
}

func buildFallbackTableChildren(element dto.ExtractElement, content string, params elementGroupParams) []dto.TransformedChildChunk {
	parts := splitElementGroupText(content, params.ChildTargetChars, params.ChildMinChars, params.ChildMaxChars, params.ChildOverlapChars)
	children := make([]dto.TransformedChildChunk, 0, len(parts))
	for i, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		metadata := childMetadataForElements([]dto.ExtractElement{element}, "table")
		metadata["table_part_index"] = i
		metadata["table_part_count"] = len(parts)
		children = append(children, dto.TransformedChildChunk{
			Content:  trimmed,
			BBox:     element.BBox,
			Metadata: metadata,
		})
	}
	return children
}

func childMetadataForElements(elements []dto.ExtractElement, kind string) map[string]any {
	metadata := map[string]any{}
	if len(elements) == 0 {
		metadata["child_kind"] = kind
		return metadata
	}
	for key, value := range elements[0].Metadata {
		metadata[key] = value
	}
	metadata["child_kind"] = kind
	metadata["source_element_ids"] = sourceElementIDsFromExtract(elements)
	metadata["source_start_order"] = elements[0].Ordinal
	metadata["source_end_order"] = elements[len(elements)-1].Ordinal
	if elements[0].Type != "" {
		metadata["element_type"] = elements[0].Type
	}
	if elements[0].Subtype != "" {
		metadata["element_subtype"] = elements[0].Subtype
	}
	if elements[0].Page > 0 {
		metadata["page"] = elements[0].Page
	}
	return metadata
}

func normalizedElementGroupKind(elementType string) string {
	switch strings.ToLower(strings.TrimSpace(elementType)) {
	case "image", "figure", "picture":
		return "image"
	case "formula", "equation":
		return "formula"
	case "table":
		return "table"
	default:
		return "text"
	}
}

func joinElementContents(elements []dto.ExtractElement) string {
	parts := make([]string, 0, len(elements))
	for _, element := range elements {
		if text := strings.TrimSpace(elementSearchText(element)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func elementSearchText(element dto.ExtractElement) string {
	if text := strings.TrimSpace(element.Content); text != "" {
		return text
	}
	switch normalizedElementGroupKind(element.Type) {
	case "image":
		if text := joinMetadataText(element.Metadata, "caption", "summary", "ocr_text"); text != "" {
			return text
		}
		return fmt.Sprintf("[image page=%d]", element.Page)
	case "formula":
		if text := joinMetadataText(element.Metadata, "latex", "text"); text != "" {
			return text
		}
		return fmt.Sprintf("[formula page=%d]", element.Page)
	default:
		return joinMetadataText(element.Metadata, "text", "markdown", "caption")
	}
}

func joinMetadataText(metadata map[string]any, keys ...string) string {
	if len(metadata) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func elementRuneLen(element dto.ExtractElement) int {
	return len([]rune(strings.TrimSpace(elementSearchText(element))))
}

func elementGroupRuneLen(elements []dto.ExtractElement) int {
	total := 0
	for i, element := range elements {
		if i > 0 {
			total++
		}
		total += elementRuneLen(element)
	}
	return total
}

func sourceElementIDsFromExtract(elements []dto.ExtractElement) []any {
	ids := make([]any, 0, len(elements))
	for _, element := range elements {
		if id := strings.TrimSpace(fmt.Sprint(element.Metadata["element_id"])); id != "" && id != "<nil>" {
			ids = append(ids, id)
			continue
		}
		ids = append(ids, element.Ordinal)
	}
	return ids
}

func splitElementGroupText(text string, target, minSize, maxSize, overlap int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxSize <= 0 {
		maxSize = defaultElementGroupChildMaxChars
	}
	if target <= 0 {
		target = defaultElementGroupChildTargetChars
	}
	if target > maxSize {
		target = maxSize
	}
	if minSize <= 0 || minSize > target {
		minSize = target / 2
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= target {
		overlap = target / 5
	}

	runes := []rune(text)
	if len(runes) <= maxSize {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)+target-1)/target)
	for start := 0; start < len(runes); {
		end := chooseElementTextWindowEnd(runes, start, target, minSize, maxSize)
		if end <= start {
			end = start + target
			if end > len(runes) {
				end = len(runes)
			}
		}
		if chunk := strings.TrimSpace(string(runes[start:end])); chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return mergeSmallTrailingTextChunk(chunks, minSize)
}

func chooseElementTextWindowEnd(runes []rune, start, target, minSize, maxSize int) int {
	remaining := len(runes) - start
	if remaining <= maxSize {
		return len(runes)
	}
	minEnd := start + minSize
	if minEnd > len(runes) {
		minEnd = len(runes)
	}
	idealEnd := start + target
	if idealEnd > len(runes) {
		idealEnd = len(runes)
	}
	maxEnd := start + maxSize
	if maxEnd > len(runes) {
		maxEnd = len(runes)
	}
	for i := idealEnd; i >= minEnd; i-- {
		if i > start && isElementTextBoundary(runes[i-1]) {
			return i
		}
	}
	for i := idealEnd; i < maxEnd; i++ {
		if isElementTextBoundary(runes[i]) {
			return i + 1
		}
	}
	return maxEnd
}

func isElementTextBoundary(r rune) bool {
	switch r {
	case '\n', ' ', '\t', '.', '!', '?', ';', ':', ',', '|':
		return true
	case '\u3002', '\uff01', '\uff1f', '\uff1b', '\uff1a', '\uff0c', '\u3001':
		return true
	default:
		return false
	}
}

func mergeSmallTrailingTextChunk(chunks []string, minSize int) []string {
	if len(chunks) < 2 || minSize <= 0 {
		return chunks
	}
	last := chunks[len(chunks)-1]
	if len([]rune(last)) >= minSize {
		return chunks
	}
	chunks[len(chunks)-2] = strings.TrimSpace(chunks[len(chunks)-2] + "\n" + last)
	return chunks[:len(chunks)-1]
}

func parseElementMarkdownTable(text string) ([]string, [][]string) {
	lines := strings.Split(text, "\n")
	rows := make([][]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "|") || isElementMarkdownTableSeparator(trimmed) {
			continue
		}
		cells := splitElementMarkdownTableRow(trimmed)
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], rows[1:]
}

func isElementMarkdownTableSeparator(line string) bool {
	trimmed := strings.Trim(line, "| ")
	if trimmed == "" {
		return false
	}
	for _, r := range trimmed {
		if r != '-' && r != ':' && r != '|' && r != ' ' {
			return false
		}
	}
	return strings.Contains(trimmed, "-")
}

func splitElementMarkdownTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cells = append(cells, strings.TrimSpace(part))
	}
	return cells
}

func groupElementTableRows(header []string, rows [][]string, maxChars int) [][][]string {
	if maxChars <= 0 {
		maxChars = defaultElementGroupTableMaxChars
	}
	groups := make([][][]string, 0)
	current := make([][]string, 0)
	currentLen := len([]rune(renderElementTableRows(header, nil)))
	flush := func() {
		if len(current) == 0 {
			return
		}
		groups = append(groups, append([][]string(nil), current...))
		current = current[:0]
		currentLen = len([]rune(renderElementTableRows(header, nil)))
	}
	for _, row := range rows {
		rowLen := len([]rune("| " + strings.Join(row, " | ") + " |"))
		if len(current) > 0 && currentLen+1+rowLen > maxChars {
			flush()
		}
		current = append(current, row)
		currentLen += 1 + rowLen
		if currentLen >= maxChars {
			flush()
		}
	}
	flush()
	return groups
}

func renderElementTableRows(header []string, rows [][]string) string {
	out := make([]string, 0, len(rows)+2)
	out = append(out, "| "+strings.Join(header, " | ")+" |")
	separator := make([]string, len(header))
	for i := range separator {
		separator[i] = "---"
	}
	out = append(out, "| "+strings.Join(separator, " | ")+" |")
	for _, row := range rows {
		out = append(out, "| "+strings.Join(row, " | ")+" |")
	}
	return strings.Join(out, "\n")
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

func parentChunkMergeTarget(segmentation *SegmentationRule) int {
	if segmentation != nil && segmentation.MaxTokens > 0 {
		return segmentation.MaxTokens
	}
	return defaultParentChunkMergeTarget
}

func mergeSmallParentChunks(chunks []dto.TransformedChunk, target int) []dto.TransformedChunk {
	if len(chunks) <= 1 || target <= 0 {
		return chunks
	}

	merged := make([]dto.TransformedChunk, 0, len(chunks))
	var current *dto.TransformedChunk
	currentLen := 0

	flush := func() {
		if current == nil {
			return
		}
		merged = append(merged, *current)
		current = nil
		currentLen = 0
	}

	for _, chunk := range chunks {
		content := strings.TrimSpace(chunk.Content)
		if content == "" {
			continue
		}

		chunk.Content = content
		if !isMergeableParentChunk(chunk) {
			flush()
			merged = append(merged, chunk)
			continue
		}

		chunkLen := len([]rune(content))
		if current == nil {
			current = cloneParentChunkForMerge(chunk)
			currentLen = chunkLen
			if currentLen >= target {
				flush()
			}
			continue
		}

		separatorLen := 1
		if currentLen+separatorLen+chunkLen > target && currentLen > 0 {
			flush()
			current = cloneParentChunkForMerge(chunk)
			currentLen = chunkLen
			if currentLen >= target {
				flush()
			}
			continue
		}

		current.Content += "\n" + content
		currentLen += separatorLen + chunkLen
		current.BBox = mergeParentChunkBBox(current.BBox, chunk.BBox)
		current.Metadata = mergeParentChunkMetadata(current.Metadata, chunk.Metadata)
	}

	flush()
	return merged
}

func cloneParentChunkForMerge(chunk dto.TransformedChunk) *dto.TransformedChunk {
	return &dto.TransformedChunk{
		Content:  strings.TrimSpace(chunk.Content),
		BBox:     chunk.BBox,
		Metadata: cloneChunkMetadata(chunk.Metadata),
	}
}

func mergeParentChunkMetadata(left, right map[string]any) map[string]any {
	merged := cloneChunkMetadata(left)
	if len(right) == 0 {
		deleteMergedParentIdentity(merged)
		return merged
	}

	if page, ok := right["page"]; ok {
		if _, exists := merged["page"]; !exists {
			merged["page"] = page
		}
	}
	issues := make([]any, 0)
	seen := make(map[string]struct{})
	issues = appendQualityIssues(issues, seen, merged)
	issues = appendQualityIssues(issues, seen, right)
	setQualityIssueMetadata(merged, issues)

	deleteMergedParentIdentity(merged)
	return merged
}

func deleteMergedParentIdentity(metadata map[string]any) {
	delete(metadata, "doc_id")
	delete(metadata, "doc_hash")
	delete(metadata, "chunk_index")
	delete(metadata, "total_chunks")
	delete(metadata, "child_count")
}

func mergeParentChunkBBox(left, right *dto.ExtractBoundingBox) *dto.ExtractBoundingBox {
	if left != nil {
		return left
	}
	return right
}

func isMergeableParentChunk(chunk dto.TransformedChunk) bool {
	if len(chunk.Children) > 0 {
		return false
	}
	elementType, _ := chunk.Metadata["element_type"].(string)
	return isParagraphTextElement(elementType)
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
	if fixedSeparator == "" {
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

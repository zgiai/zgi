package hyperparse

import (
	"sort"
	"strings"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	"github.com/zgiai/zgi/api/internal/dto"
)

func mapResultToExtractOutput(result *extractcommon.DocumentResult, filePath, backend string) *dto.ExtractOutput {
	if result == nil {
		return nil
	}

	recognitionSource := "hyperparse_sdk:" + backend
	output := &dto.ExtractOutput{
		Markdown: strings.TrimSpace(result.Markdown),
		Source:   recognitionSource,
		Metadata: buildMetadata(result, filePath, recognitionSource),
	}

	chunks := normalizedChunks(result.Chunks)
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].Ordinal != chunks[j].Ordinal {
			return chunks[i].Ordinal < chunks[j].Ordinal
		}
		if chunks[i].Page != chunks[j].Page {
			return chunks[i].Page < chunks[j].Page
		}
		return chunks[i].ID < chunks[j].ID
	})

	for _, chunk := range chunks {
		content := chunkContent(chunk)
		if content == "" {
			continue
		}

		elementType := strings.TrimSpace(chunk.Type)
		if elementType == "" {
			elementType = "text"
		}

		page := chunk.Page
		if page < 0 {
			page = 0
		}

		output.Elements = append(output.Elements, dto.ExtractElement{
			Type:      elementType,
			Subtype:   strings.TrimSpace(chunk.Subtype),
			Page:      page,
			Content:   content,
			BBox:      mapBoundingBox(chunk.BBox),
			Ordinal:   chunk.Ordinal,
			Precision: strings.TrimSpace(chunk.Precision),
			Metadata:  buildElementMetadata(output.Metadata, chunk, page, elementType),
		})
	}

	if len(output.Elements) == 0 && output.Markdown != "" {
		output.Elements = append(output.Elements, dto.ExtractElement{
			Type:     "text",
			Page:     0,
			Content:  output.Markdown,
			Ordinal:  1,
			Metadata: buildFallbackElementMetadata(output.Metadata),
		})
	}

	if output.Markdown == "" && len(output.Elements) > 0 {
		output.Markdown = elementsMarkdown(output.Elements)
	}

	return output
}

func normalizedChunks(chunks []extractcommon.Chunk) []extractcommon.Chunk {
	normalized := make([]extractcommon.Chunk, 0, len(chunks))
	for i, chunk := range chunks {
		if chunk.Ordinal <= 0 {
			chunk.Ordinal = i + 1
		}
		normalized = append(normalized, chunk)
	}
	return normalized
}

func chunkContent(chunk extractcommon.Chunk) string {
	if content := strings.TrimSpace(chunk.Markdown); content != "" {
		return content
	}
	return strings.TrimSpace(chunk.Text)
}

func mapBoundingBox(box *extractcommon.BBox) *dto.ExtractBoundingBox {
	if box == nil {
		return nil
	}
	return &dto.ExtractBoundingBox{
		Left:   box.Left,
		Top:    box.Top,
		Right:  box.Right,
		Bottom: box.Bottom,
	}
}

func buildElementMetadata(base map[string]any, chunk extractcommon.Chunk, page int, elementType string) map[string]any {
	meta := cloneMetadata(base)
	meta["page"] = page
	meta["element_type"] = elementType
	if chunk.ID != "" {
		meta["hyperparse_chunk_id"] = chunk.ID
	}
	if chunk.ParentID != "" {
		meta["hyperparse_parent_id"] = chunk.ParentID
	}
	if markdown := strings.TrimSpace(chunk.Markdown); markdown != "" {
		meta["markdown"] = markdown
	}
	if chunk.Confidence > 0 {
		meta["confidence"] = chunk.Confidence
	}
	return meta
}

func buildFallbackElementMetadata(base map[string]any) map[string]any {
	meta := cloneMetadata(base)
	meta["page"] = 0
	meta["element_type"] = "text"
	return meta
}

func elementsMarkdown(elements []dto.ExtractElement) string {
	contents := make([]string, 0, len(elements))
	for _, element := range elements {
		if content := strings.TrimSpace(element.Content); content != "" {
			contents = append(contents, content)
		}
	}
	return strings.Join(contents, "\n\n")
}

func buildMetadata(result *extractcommon.DocumentResult, filePath, recognitionSource string) map[string]any {
	meta := map[string]any{
		"source":             filePath,
		"recognition_source": recognitionSource,
	}
	if result.Source != "" {
		meta["hyperparse_source"] = result.Source
	}
	if result.FileName != "" {
		meta["file_name"] = result.FileName
	}
	if result.DocID != "" {
		meta["hyperparse_doc_id"] = result.DocID
	}
	if result.PageCount > 0 {
		meta["hyperparse_page_count"] = result.PageCount
	}
	if len(result.Diagnostics) > 0 {
		meta["hyperparse_diagnostics"] = result.Diagnostics
	}
	return meta
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

package dataset

import (
	"sort"
	"strings"

	"github.com/zgiai/ginext/internal/contracts"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/dataset/splitter"
)

type AdapterOptions struct {
	BuildChildren     bool
	SubchunkMaxTokens int
	SubchunkOverlap   int
	SubchunkSeparator string
}

// UnitsToTransformedChunks is the future bridge from provider-agnostic chunk
// output to the existing dataset indexing DTO. It is intentionally not wired
// into the current indexing runner yet.
func UnitsToTransformedChunks(units []contracts.ChunkUnit) []dto.TransformedChunk {
	return UnitsToTransformedChunksWithOptions(units, AdapterOptions{})
}

func UnitsToTransformedChunksWithOptions(units []contracts.ChunkUnit, options AdapterOptions) []dto.TransformedChunk {
	if len(units) == 0 {
		return nil
	}
	units = append([]contracts.ChunkUnit(nil), units...)
	sort.SliceStable(units, func(i, j int) bool {
		return units[i].Order < units[j].Order
	})

	chunks := make([]dto.TransformedChunk, 0, len(units))
	parentIndex := make(map[string]int, len(units))
	deferredChildren := make(map[string][]dto.TransformedChildChunk)
	for _, unit := range units {
		if unitText(unit) == "" {
			continue
		}

		if unit.Kind == contracts.ChunkKindChild || unit.ParentChunkID != "" {
			if parentPos, ok := parentIndex[unit.ParentChunkID]; ok {
				child := unitToTransformedChildChunk(unit, len(chunks[parentPos].Children))
				chunks[parentPos].Children = append(chunks[parentPos].Children, child)
				chunks[parentPos].Metadata["is_parent"] = true
				chunks[parentPos].Metadata["child_count"] = len(chunks[parentPos].Children)
			} else {
				child := unitToTransformedChildChunk(unit, len(deferredChildren[unit.ParentChunkID]))
				deferredChildren[unit.ParentChunkID] = append(deferredChildren[unit.ParentChunkID], child)
			}
			continue
		}

		chunk := unitToTransformedChunk(unit)
		parentIndex[unit.ChunkID] = len(chunks)
		if children := deferredChildren[unit.ChunkID]; len(children) > 0 {
			chunk.Children = append(chunk.Children, children...)
			chunk.Metadata["is_parent"] = true
			chunk.Metadata["child_count"] = len(chunk.Children)
		}
		chunks = append(chunks, chunk)
	}

	if options.BuildChildren {
		for i := range chunks {
			if len(chunks[i].Children) > 0 {
				continue
			}
			chunks[i].Children = splitGeneratedChildChunks(chunks[i], normalizeAdapterOptions(options))
			if len(chunks[i].Children) > 0 {
				chunks[i].Metadata["is_parent"] = true
				chunks[i].Metadata["child_count"] = len(chunks[i].Children)
			}
		}
	}

	return chunks
}

func unitToTransformedChunk(unit contracts.ChunkUnit) dto.TransformedChunk {
	metadata := unitMetadata(unit)
	return dto.TransformedChunk{
		Content:  unitText(unit),
		BBox:     toExtractBBox(unit.BBox),
		Metadata: metadata,
	}
}

func unitToTransformedChildChunk(unit contracts.ChunkUnit, childIndex int) dto.TransformedChildChunk {
	metadata := unitMetadata(unit)
	metadata["is_child"] = true
	metadata["child_index"] = childIndex
	if unit.ParentChunkID != "" {
		metadata["parent_id"] = unit.ParentChunkID
	}
	return dto.TransformedChildChunk{
		Content:  unitText(unit),
		BBox:     toExtractBBox(unit.BBox),
		Metadata: metadata,
	}
}

func unitText(unit contracts.ChunkUnit) string {
	content := strings.TrimSpace(unit.Content)
	if content != "" {
		return content
	}
	return strings.TrimSpace(unit.Markdown)
}

func unitMetadata(unit contracts.ChunkUnit) map[string]any {
	metadata := cloneMetadata(unit.Metadata)
	metadata["chunk_id"] = unit.ChunkID
	metadata["chunk_kind"] = string(unit.Kind)
	if unit.ParentChunkID != "" {
		metadata["parent_chunk_id"] = unit.ParentChunkID
	}
	if unit.DocumentID != "" {
		metadata["document_id"] = unit.DocumentID
	}
	if unit.DatasetID != "" {
		metadata["dataset_id"] = unit.DatasetID
	}
	if unit.FileID != "" {
		metadata["file_id"] = unit.FileID
	}
	if len(unit.Pages) > 0 {
		metadata["pages"] = unit.Pages
	}
	metadata["order"] = unit.Order
	return metadata
}

func splitGeneratedChildChunks(parent dto.TransformedChunk, options AdapterOptions) []dto.TransformedChildChunk {
	content := strings.TrimSpace(parent.Content)
	if content == "" {
		return nil
	}
	fixedSeparator, separators := subchunkSeparators(options.SubchunkSeparator)
	textSplitter := splitter.NewFixedRecursiveCharacterTextSplitter(
		fixedSeparator,
		separators,
		options.SubchunkMaxTokens,
		options.SubchunkOverlap,
		nil,
		false,
		false,
	)
	parts := textSplitter.SplitText(content)
	children := make([]dto.TransformedChildChunk, 0, len(parts))
	parentID, _ := parent.Metadata["chunk_id"].(string)
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		metadata := cloneMetadata(parent.Metadata)
		metadata["chunk_kind"] = string(contracts.ChunkKindChild)
		metadata["is_child"] = true
		metadata["child_index"] = len(children)
		if parentID != "" {
			metadata["parent_id"] = parentID
		}
		children = append(children, dto.TransformedChildChunk{
			Content:  trimmed,
			BBox:     parent.BBox,
			Metadata: metadata,
		})
	}
	return children
}

func normalizeAdapterOptions(options AdapterOptions) AdapterOptions {
	if options.SubchunkMaxTokens <= 0 {
		options.SubchunkMaxTokens = 1000
	}
	if options.SubchunkOverlap < 0 {
		options.SubchunkOverlap = 0
	}
	if options.SubchunkOverlap >= options.SubchunkMaxTokens {
		options.SubchunkOverlap = options.SubchunkMaxTokens / 5
	}
	return options
}

func subchunkSeparators(preferred string) (string, []string) {
	defaults := []string{
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
	separators := make([]string, 0, len(defaults)+1)
	seen := make(map[string]struct{}, len(defaults)+1)
	fixed := preferred
	if strings.TrimSpace(preferred) != "" {
		separators = append(separators, preferred)
		seen[preferred] = struct{}{}
	}
	for _, separator := range defaults {
		if _, ok := seen[separator]; ok {
			continue
		}
		separators = append(separators, separator)
		seen[separator] = struct{}{}
	}
	return fixed, separators
}

func cloneMetadata(src map[string]any) map[string]any {
	dst := make(map[string]any)
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func toExtractBBox(bbox *contracts.ParseBoundingBox) *dto.ExtractBoundingBox {
	if bbox == nil {
		return nil
	}
	return &dto.ExtractBoundingBox{
		Left:   bbox.Left,
		Top:    bbox.Top,
		Right:  bbox.Right,
		Bottom: bbox.Bottom,
	}
}

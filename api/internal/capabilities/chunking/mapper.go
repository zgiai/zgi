package chunking

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/zgiai/ginext/internal/contracts"
)

// CanonicalMapper transforms parse artifacts into the canonical chunk source
// model used by planners and future chunk builders.
type CanonicalMapper struct{}

func NewCanonicalMapper() *CanonicalMapper {
	return &CanonicalMapper{}
}

func (m *CanonicalMapper) FromParseArtifact(artifact *contracts.ParseArtifact) (*contracts.ChunkSourceDocument, error) {
	if artifact == nil {
		return nil, fmt.Errorf("parse artifact is nil")
	}

	doc := &contracts.ChunkSourceDocument{
		DocumentID:  readStringMeta(artifact.Metadata, "document_id"),
		DatasetID:   readStringMeta(artifact.Metadata, "dataset_id"),
		FileID:      coalesce(readStringMeta(artifact.Metadata, "file_id"), artifact.SourceRef),
		Source:      coalesce(readStringMeta(artifact.Metadata, "recognition_source"), readStringMeta(artifact.Metadata, "source"), artifact.SourceRef, artifact.FileName),
		Title:       coalesce(artifact.FileName, readStringMeta(artifact.Metadata, "title")),
		Language:    readStringMeta(artifact.Metadata, "language"),
		Metadata:    cloneMap(artifact.Metadata),
		Diagnostics: cloneMap(artifact.Diagnostics),
		Elements:    make([]contracts.ChunkSourceElement, 0, len(artifact.Elements)),
	}

	for _, element := range artifact.Elements {
		doc.Elements = append(doc.Elements, contracts.ChunkSourceElement{
			ElementID:  element.ID,
			Type:       element.Type,
			Subtype:    element.Subtype,
			Page:       element.Page,
			Content:    element.Content,
			Markdown:   stringMetaValue(element.Metadata, "markdown", element.Content),
			BBox:       element.BBox,
			Ordinal:    normalizedOrdinal(element.Ordinal, len(doc.Elements)),
			Precision:  element.Precision,
			Confidence: element.Confidence,
			Metadata:   cloneMap(element.Metadata),
		})
	}

	if shouldApplyLayoutOrder(doc.Elements) {
		sort.SliceStable(doc.Elements, func(i, j int) bool {
			return layoutElementLess(doc.Elements[i], doc.Elements[j], i, j)
		})
		for i := range doc.Elements {
			doc.Elements[i].Ordinal = i + 1
		}
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		doc.Metadata["layout_order_applied"] = true
		doc.Metadata["layout_order_strategy"] = "bbox_row_major"
	} else {
		sort.SliceStable(doc.Elements, func(i, j int) bool {
			return doc.Elements[i].Ordinal < doc.Elements[j].Ordinal
		})
	}

	if doc.DocumentID == "" {
		doc.DocumentID = readStringMeta(artifact.Metadata, "doc_id")
	}

	return doc, nil
}

func shouldApplyLayoutOrder(elements []contracts.ChunkSourceElement) bool {
	if len(elements) < 2 {
		return false
	}
	bboxCount := 0
	ocrBBoxCount := 0
	for _, element := range elements {
		if !validBBox(element.BBox) {
			continue
		}
		bboxCount++
		if isOCRElement(element) {
			ocrBBoxCount++
		}
	}
	if bboxCount == 0 {
		return false
	}
	bboxRatio := float64(bboxCount) / float64(len(elements))
	ocrRatio := float64(ocrBBoxCount) / float64(bboxCount)
	return bboxRatio >= 0.7 && ocrRatio >= 0.5
}

func layoutElementLess(left, right contracts.ChunkSourceElement, leftFallback, rightFallback int) bool {
	leftPage := normalizedLayoutPage(left.Page)
	rightPage := normalizedLayoutPage(right.Page)
	if leftPage != rightPage {
		return leftPage < rightPage
	}
	if validBBox(left.BBox) && validBBox(right.BBox) {
		if !sameLayoutRow(left.BBox, right.BBox) {
			return left.BBox.Top < right.BBox.Top
		}
		if !nearFloat(left.BBox.Left, right.BBox.Left, 0.002) {
			return left.BBox.Left < right.BBox.Left
		}
	}
	return normalizedOrdinal(left.Ordinal, leftFallback) < normalizedOrdinal(right.Ordinal, rightFallback)
}

func normalizedLayoutPage(page int) int {
	if page < 0 {
		return 0
	}
	return page
}

func sameLayoutRow(left, right *contracts.ParseBoundingBox) bool {
	if left == nil || right == nil {
		return false
	}
	leftMid := (left.Top + left.Bottom) / 2
	rightMid := (right.Top + right.Bottom) / 2
	rowTolerance := math.Max(0.012, math.Min(bboxHeight(left), bboxHeight(right))*0.9)
	return math.Abs(leftMid-rightMid) <= rowTolerance
}

func validBBox(box *contracts.ParseBoundingBox) bool {
	return box != nil && box.Right > box.Left && box.Bottom > box.Top
}

func bboxHeight(box *contracts.ParseBoundingBox) float64 {
	if box == nil {
		return 0
	}
	return box.Bottom - box.Top
}

func nearFloat(left, right, tolerance float64) bool {
	return math.Abs(left-right) <= tolerance
}

func isOCRElement(element contracts.ChunkSourceElement) bool {
	if len(element.Metadata) == 0 {
		return false
	}
	if strings.Contains(strings.ToLower(readStringMeta(element.Metadata, "bbox_source")), "ocr") {
		return true
	}
	if strings.EqualFold(readStringMeta(element.Metadata, "extraction_method"), "ocr") {
		return true
	}
	payload, ok := element.Metadata["payload"].(map[string]any)
	if !ok || len(payload) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(readStringMeta(payload, "bbox_source")), "ocr") ||
		strings.EqualFold(readStringMeta(payload, "extraction_method"), "ocr")
}

func normalizedOrdinal(ordinal, fallbackIndex int) int {
	if ordinal > 0 {
		return ordinal
	}
	return fallbackIndex + 1
}

func stringMetaValue(metadata map[string]any, key, fallback string) string {
	if value := readStringMeta(metadata, key); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func readStringMeta(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

package chunking

import (
	"fmt"
	"math"
	"strings"

	"github.com/zgiai/zgi/api/internal/contracts"
)

const (
	parentModeParentChild = "parent_child"
	parentModeSection     = "section"
	parentModeTableFirst  = "table_first"
	parentModePageAware   = "page_aware"
	parentModeFullDoc     = "full_doc"
)

// DefaultPlanner provides a policy layer for downstream chunking use cases
// without affecting current business behavior.
type DefaultPlanner struct{}

func NewDefaultPlanner() *DefaultPlanner {
	return &DefaultPlanner{}
}

func (p *DefaultPlanner) Plan(doc *contracts.ChunkSourceDocument, useCase contracts.ChunkUseCase) (*contracts.ChunkPlan, error) {
	if doc == nil {
		return nil, fmt.Errorf("chunk source document is nil")
	}

	profile := analyzeDocument(doc)
	plan := &contracts.ChunkPlan{
		UseCase:       useCase,
		PreserveOrder: true,
		Metadata:      profile.metadata(),
	}

	switch useCase {
	case contracts.ChunkUseCaseDatasetIndex:
		plan.ParentMode = inferDatasetParentMode(profile)
		plan.Segmentation = inferDatasetSegmentation(profile)
		plan.TargetKinds = datasetTargetKinds(profile)
	case contracts.ChunkUseCaseChatContext:
		plan.ParentMode = inferChatParentMode(profile)
		plan.Segmentation = "context_first"
		plan.TargetKinds = []contracts.ChunkKind{
			contracts.ChunkKindText,
			contracts.ChunkKindHeading,
			contracts.ChunkKindTable,
		}
	case contracts.ChunkUseCaseQAIndex:
		plan.ParentMode = "qa"
		plan.Segmentation = "qa_friendly"
		plan.TargetKinds = []contracts.ChunkKind{
			contracts.ChunkKindQuestion,
			contracts.ChunkKindAnswer,
			contracts.ChunkKindText,
		}
	default:
		plan.ParentMode = inferPreviewParentMode(profile)
		plan.Segmentation = inferPreviewSegmentation(profile)
		plan.TargetKinds = []contracts.ChunkKind{
			contracts.ChunkKindText,
			contracts.ChunkKindHeading,
			contracts.ChunkKindTable,
			contracts.ChunkKindFigure,
		}
	}

	plan.Metadata["selected_parent_mode"] = plan.ParentMode
	plan.Metadata["selected_segmentation"] = plan.Segmentation
	return plan, nil
}

type documentProfile struct {
	ElementCount    int
	TextCount       int
	HeadingCount    int
	TableCount      int
	FigureCount     int
	FormulaCount    int
	PageCount       int
	BBoxCount       int
	ConfidenceCount int
	ConfidenceTotal float64
	TotalTextLength int
	Domain          string
	Source          string
}

func analyzeDocument(doc *contracts.ChunkSourceDocument) documentProfile {
	profile := documentProfile{
		ElementCount: len(doc.Elements),
		Domain:       strings.ToLower(readStringMeta(doc.Metadata, "doc_domain")),
		Source:       strings.ToLower(strings.TrimSpace(doc.Source)),
	}
	pages := make(map[int]struct{})
	for _, element := range doc.Elements {
		if element.Page > 0 {
			pages[element.Page] = struct{}{}
		} else if element.Page == 0 && element.BBox != nil {
			pages[1] = struct{}{}
		}
		if element.BBox != nil {
			profile.BBoxCount++
		}
		if element.Confidence != nil {
			profile.ConfidenceCount++
			profile.ConfidenceTotal += *element.Confidence
		}
		text := strings.TrimSpace(element.Content)
		profile.TotalTextLength += len([]rune(text))
		switch normalizedElementType(element.Type) {
		case "heading", "title":
			profile.HeadingCount++
			profile.TextCount++
		case "table":
			profile.TableCount++
		case "figure", "image":
			profile.FigureCount++
		case "formula", "equation":
			profile.FormulaCount++
		case "text", "paragraph", "list", "caption", "header", "footer", "":
			profile.TextCount++
		}
	}
	profile.PageCount = len(pages)
	return profile
}

func (p documentProfile) metadata() map[string]any {
	metadata := map[string]any{
		"element_count":       p.ElementCount,
		"text_count":          p.TextCount,
		"heading_count":       p.HeadingCount,
		"table_count":         p.TableCount,
		"figure_count":        p.FigureCount,
		"formula_count":       p.FormulaCount,
		"page_count":          p.PageCount,
		"bbox_coverage_ratio": p.bboxCoverage(),
		"text_density":        p.textDensity(),
		"likely_scanned":      p.likelyScanned(),
	}
	if p.Domain != "" {
		metadata["doc_domain"] = p.Domain
	}
	if p.ConfidenceCount > 0 {
		metadata["avg_confidence"] = p.ConfidenceTotal / float64(p.ConfidenceCount)
	}
	return metadata
}

func (p documentProfile) bboxCoverage() float64 {
	if p.ElementCount == 0 {
		return 0
	}
	return roundRatio(float64(p.BBoxCount) / float64(p.ElementCount))
}

func (p documentProfile) textDensity() float64 {
	if p.ElementCount == 0 {
		return 0
	}
	return roundRatio(float64(p.TotalTextLength) / float64(p.ElementCount))
}

func (p documentProfile) tableRatio() float64 {
	if p.ElementCount == 0 {
		return 0
	}
	return float64(p.TableCount) / float64(p.ElementCount)
}

func (p documentProfile) headingRatio() float64 {
	if p.ElementCount == 0 {
		return 0
	}
	return float64(p.HeadingCount) / float64(p.ElementCount)
}

func (p documentProfile) likelyScanned() bool {
	sourceHintsOCR := strings.Contains(p.Source, "ocr") || strings.Contains(p.Source, "vlm")
	if sourceHintsOCR && p.BBoxCount > 0 {
		return true
	}
	if p.PageCount >= 1 && p.BBoxCount > 0 && p.textDensity() < 32 && p.HeadingCount == 0 {
		return true
	}
	return false
}

func inferDatasetParentMode(profile documentProfile) string {
	switch {
	case profile.Domain == "resume" || profile.Domain == "invoice":
		return parentModeFullDoc
	case profile.TableCount > 0 && (profile.HeadingCount == 0 || profile.tableRatio() >= 0.25):
		return parentModeTableFirst
	case profile.likelyScanned():
		return parentModePageAware
	case profile.HeadingCount >= 3 || profile.headingRatio() > 0.05:
		return parentModeSection
	default:
		return parentModeParentChild
	}
}

func inferDatasetSegmentation(profile documentProfile) string {
	switch inferDatasetParentMode(profile) {
	case parentModeTableFirst:
		return "table_aware"
	case parentModePageAware:
		return "page_layout_aware"
	case parentModeSection:
		return "section_aware"
	case parentModeFullDoc:
		return "full_document"
	default:
		return "structure_aware"
	}
}

func inferChatParentMode(profile documentProfile) string {
	if profile.likelyScanned() {
		return "page_context"
	}
	if profile.TableCount > 0 && profile.tableRatio() >= 0.2 {
		return "table_context"
	}
	return "compact_context"
}

func inferPreviewParentMode(profile documentProfile) string {
	if profile.BBoxCount > 0 {
		return "visual_preview"
	}
	return "preview"
}

func inferPreviewSegmentation(profile documentProfile) string {
	if profile.likelyScanned() {
		return "page_layout_aware"
	}
	return "light"
}

func datasetTargetKinds(profile documentProfile) []contracts.ChunkKind {
	kinds := []contracts.ChunkKind{
		contracts.ChunkKindText,
		contracts.ChunkKindHeading,
	}
	if profile.TableCount > 0 {
		kinds = append(kinds, contracts.ChunkKindTable)
	}
	if profile.FigureCount > 0 || profile.likelyScanned() {
		kinds = append(kinds, contracts.ChunkKindFigure)
	}
	if profile.FormulaCount > 0 {
		kinds = append(kinds, contracts.ChunkKindFormula)
	}
	return kinds
}

func normalizedElementType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func roundRatio(value float64) float64 {
	return math.Round(value*10000) / 10000
}

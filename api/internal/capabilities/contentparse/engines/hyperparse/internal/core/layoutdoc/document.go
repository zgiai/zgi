package layoutdoc

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	SourceNative   = "native"
	SourceRepaired = "repaired"
	SourceOCR      = "ocr"
	SourceVLM      = "vlm"
	SourceUnknown  = "unknown"
)

type BBox struct {
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
}

func (b BBox) Valid() bool {
	return b.Right > b.Left && b.Bottom > b.Top
}

func (b BBox) Width() float64 {
	return math.Max(0, b.Right-b.Left)
}

func (b BBox) Height() float64 {
	return math.Max(0, b.Bottom-b.Top)
}

func (b BBox) CenterX() float64 {
	return (b.Left + b.Right) / 2
}

func (b BBox) CenterY() float64 {
	return (b.Top + b.Bottom) / 2
}

func (b BBox) Area() float64 {
	return b.Width() * b.Height()
}

func (b BBox) Map() map[string]any {
	return map[string]any{
		"left":   round(b.Left, 6),
		"right":  round(b.Right, 6),
		"top":    round(b.Top, 6),
		"bottom": round(b.Bottom, 6),
	}
}

type Element struct {
	ID          string
	Type        string
	Text        string
	PageIndex   int
	Order       int
	Source      string
	Stage       string
	Method      string
	Confidence  float64
	SourceTrace string
	BBox        *BBox
	Payload     map[string]any
	Raw         map[string]any
	OriginalPos int
}

type Document struct {
	Source    string
	PageCount int
	Elements  []Element
	Meta      map[string]any
}

type StageReport struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Count    int            `json:"count,omitempty"`
	Detail   string         `json:"detail,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Report struct {
	ElementCount int            `json:"element_count"`
	PageCount    int            `json:"page_count"`
	SourceCounts map[string]int `json:"source_counts"`
	Stages       []StageReport  `json:"stages"`
}

type Processor interface {
	Name() string
	Process(*Document) (StageReport, error)
}

func Run(doc *Document, processors ...Processor) (Report, error) {
	report := Report{
		PageCount:    doc.PageCount,
		ElementCount: len(doc.Elements),
		SourceCounts: map[string]int{},
	}
	for _, processor := range processors {
		if processor == nil {
			continue
		}
		stage, err := processor.Process(doc)
		if stage.ID == "" {
			stage.ID = processor.Name()
		}
		if stage.Status == "" {
			stage.Status = "done"
		}
		report.Stages = append(report.Stages, stage)
		if err != nil {
			stage.Status = "error"
			report.Stages[len(report.Stages)-1] = stage
			return report, err
		}
	}
	report.ElementCount = len(doc.Elements)
	report.SourceCounts = SourceCounts(doc.Elements)
	return report, nil
}

func SourceCounts(elements []Element) map[string]int {
	out := map[string]int{}
	for _, element := range elements {
		src := CanonicalSource(element.Source, element.Stage, element.SourceTrace)
		out[src]++
	}
	return out
}

func CanonicalSource(rawSource, stage, trace string) string {
	if raw := canonicalSourceFromText(rawSource); raw != "" {
		return raw
	}
	merged := strings.ToLower(strings.TrimSpace(stage + " " + trace))
	if strings.TrimSpace(merged) == "" {
		return SourceNative
	}
	if strings.Contains(merged, "repair") || strings.Contains(merged, "repaired") {
		return SourceRepaired
	}
	if strings.Contains(merged, "ocr") && !strings.Contains(merged, "vlm") {
		return SourceOCR
	}
	if strings.Contains(merged, "vlm") && !strings.Contains(merged, "ocr") {
		return SourceVLM
	}
	if strings.Contains(merged, "native") || strings.Contains(merged, "pdf") {
		return SourceNative
	}
	return SourceUnknown
}

func canonicalSourceFromText(value string) string {
	raw := strings.ToLower(strings.TrimSpace(value))
	if raw == "" {
		return ""
	}
	switch {
	case strings.Contains(raw, "vlm"):
		return SourceVLM
	case strings.Contains(raw, "ocr"):
		return SourceOCR
	case strings.Contains(raw, "repair") || strings.Contains(raw, "repaired"):
		return SourceRepaired
	case strings.Contains(raw, "native") || strings.Contains(raw, "pdf"):
		return SourceNative
	default:
		return SourceUnknown
	}
}

func InferStage(rawSource, source, trace string, payload map[string]any, typ string) string {
	raw := strings.ToLower(strings.TrimSpace(rawSource + " " + trace))
	if payload != nil {
		if firstNonEmptyString(payload["vlm_caption"], payload["vlm_caption_model"]) != "" || boolFromAny(payload["vlm_image_caption"]) {
			return "image_caption"
		}
		repair := strings.ToLower(firstNonEmptyString(payload["repair"]))
		if boolFromAny(payload["regional_fallback"]) || strings.Contains(repair, "regional") {
			return "regional_ocr_vlm"
		}
		if repair != "" || boolFromAny(payload["table_repair"]) {
			if source == SourceOCR {
				return "ocr_layout_repair"
			}
			return "bbox_alignment_repair"
		}
	}
	if strings.Contains(raw, "regional") {
		return "regional_ocr_vlm"
	}
	switch source {
	case SourceVLM:
		return "vlm_fallback"
	case SourceOCR:
		return "ocr_layout_repair"
	case SourceRepaired:
		return "bbox_alignment_repair"
	}
	t := strings.ToLower(strings.TrimSpace(typ))
	if t == "image" || t == "stamp" || strings.Contains(raw, "image") {
		return "image_detection"
	}
	if t == "table" {
		return "table_structure"
	}
	return "native_text"
}

func InferMethod(rawSource, source string, payload map[string]any) string {
	if payload != nil {
		if engine := firstNonEmptyString(payload["ocr_engine"]); engine != "" {
			return engine
		}
		if firstNonEmptyString(payload["vlm_caption"]) != "" {
			if mode := firstNonEmptyString(payload["vlm_caption_mode"]); mode != "" {
				return "vlm_" + mode
			}
			return "vlm_caption"
		}
		if strategy := firstNonEmptyString(payload["strategy"]); strategy != "" {
			return strategy
		}
	}
	if raw := strings.TrimSpace(rawSource); raw != "" {
		return raw
	}
	return source
}

func SortElementsByReadingOrder(elements []Element) []Element {
	byPage := map[int][]Element{}
	var pages []int
	seen := map[int]bool{}
	for _, element := range elements {
		page := element.PageIndex
		if page <= 0 {
			page = 1
			element.PageIndex = 1
		}
		byPage[page] = append(byPage[page], element)
		if !seen[page] {
			seen[page] = true
			pages = append(pages, page)
		}
	}
	sort.Ints(pages)
	out := make([]Element, 0, len(elements))
	for _, page := range pages {
		items := byPage[page]
		var withBox, withoutBox []Element
		for _, item := range items {
			if item.BBox != nil && item.BBox.Valid() && !isDiagnosticElement(item) {
				withBox = append(withBox, item)
				continue
			}
			withoutBox = append(withoutBox, item)
		}
		out = append(out, sortXYCut(withBox)...)
		sort.SliceStable(withoutBox, func(i, j int) bool {
			if withoutBox[i].Order != withoutBox[j].Order {
				return withoutBox[i].Order < withoutBox[j].Order
			}
			return withoutBox[i].OriginalPos < withoutBox[j].OriginalPos
		})
		out = append(out, withoutBox...)
	}
	for i := range out {
		out[i].Order = i
	}
	return out
}

func isDiagnosticElement(element Element) bool {
	t := strings.ToLower(strings.TrimSpace(element.Type))
	return strings.HasSuffix(t, "_debug") || t == "debug" || t == "diagnostic"
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		s := strings.TrimSpace(fmt.Sprint(value))
		if s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func boolFromAny(value any) bool {
	switch x := value.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func round(v float64, places int) float64 {
	if places <= 0 {
		return math.Round(v)
	}
	p := math.Pow10(places)
	return math.Round(v*p) / p
}

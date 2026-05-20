package layoutdoc

import (
	"fmt"
	"strings"
)

func FromChunkMaps(source string, pageCount int, chunks []map[string]any) *Document {
	doc := &Document{
		Source:    source,
		PageCount: pageCount,
		Elements:  make([]Element, 0, len(chunks)),
		Meta:      map[string]any{},
	}
	for i, chunk := range chunks {
		el := ElementFromChunkMap(chunk, i)
		doc.Elements = append(doc.Elements, el)
	}
	return doc
}

func ElementFromChunkMap(chunk map[string]any, originalPos int) Element {
	raw := shallowCopyMap(chunk)
	payload, _ := raw["payload"].(map[string]any)
	rawSource := stringFromAny(raw["source"])
	trace := stringFromAny(raw["source_trace"])
	source := CanonicalSource(rawSource, stringFromAny(raw["pipeline_stage"]), trace)
	stage := stringFromAny(raw["pipeline_stage"])
	if stage == "" {
		stage = InferStage(rawSource, source, trace, payload, stringFromAny(raw["type"]))
	}
	return Element{
		ID:          firstNonEmptyString(raw["chunk_id"], raw["id"]),
		Type:        stringFromAny(raw["type"]),
		Text:        stringFromAny(raw["text"]),
		PageIndex:   intFromAny(raw["page_index"]),
		Order:       intFromAny(raw["order"]),
		Source:      source,
		Stage:       stage,
		Method:      InferMethod(rawSource, source, payload),
		Confidence:  floatFromAnyDefault(raw["confidence"], 0),
		SourceTrace: trace,
		BBox:        bboxFromAny(raw["bbox"]),
		Payload:     payload,
		Raw:         raw,
		OriginalPos: originalPos,
	}
}

func ToChunkMaps(doc *Document) []map[string]any {
	out := make([]map[string]any, 0, len(doc.Elements))
	for _, element := range doc.Elements {
		out = append(out, ChunkMapFromElement(element))
	}
	return out
}

func ChunkMapFromElement(element Element) map[string]any {
	chunk := shallowCopyMap(element.Raw)
	if chunk == nil {
		chunk = map[string]any{}
	}
	if element.ID != "" {
		chunk["chunk_id"] = element.ID
	}
	if element.Type != "" {
		chunk["type"] = element.Type
	}
	if element.Text != "" {
		chunk["text"] = element.Text
	}
	if element.PageIndex > 0 {
		chunk["page_index"] = element.PageIndex
	}
	chunk["order"] = element.Order
	if element.Source != "" {
		chunk["source"] = element.Source
	}
	if element.SourceTrace != "" {
		chunk["source_trace"] = element.SourceTrace
	}
	if element.Confidence > 0 {
		chunk["confidence"] = round(element.Confidence, 3)
	}
	if element.BBox != nil && element.BBox.Valid() {
		chunk["bbox"] = element.BBox.Map()
	}
	if element.Payload != nil {
		chunk["payload"] = element.Payload
	}
	ApplyChunkProvenance(chunk, element.Stage)
	return chunk
}

type ProvenanceProcessor struct{}

func NewProvenanceProcessor() ProvenanceProcessor {
	return ProvenanceProcessor{}
}

func (ProvenanceProcessor) Name() string {
	return "provenance"
}

func (ProvenanceProcessor) Process(doc *Document) (StageReport, error) {
	for i := range doc.Elements {
		el := &doc.Elements[i]
		if el.Source == "" || el.Source == SourceUnknown {
			el.Source = CanonicalSource(stringFromAny(el.Raw["source"]), el.Stage, el.SourceTrace)
		}
		if el.Stage == "" {
			el.Stage = InferStage(stringFromAny(el.Raw["source"]), el.Source, el.SourceTrace, el.Payload, el.Type)
		}
		if el.Method == "" {
			el.Method = InferMethod(stringFromAny(el.Raw["source"]), el.Source, el.Payload)
		}
		if el.Confidence <= 0 {
			el.Confidence = floatFromAnyDefault(el.Raw["confidence"], defaultConfidenceForSource(el.Source))
		}
	}
	return StageReport{
		ID:     "provenance",
		Status: "done",
		Count:  len(doc.Elements),
		Metadata: map[string]any{
			"source_counts": SourceCounts(doc.Elements),
		},
	}, nil
}

func NormalizeAndOrderChunkMaps(source string, pageCount int, chunks []map[string]any) ([]map[string]any, Report, error) {
	doc := FromChunkMaps(source, pageCount, chunks)
	report, err := Run(doc, NewProvenanceProcessor(), NewReadingOrderProcessor())
	return ToChunkMaps(doc), report, err
}

func ApplyChunkProvenance(chunk map[string]any, stageHint string) {
	if chunk == nil {
		return
	}
	payload, _ := chunk["payload"].(map[string]any)
	rawSource := stringFromAny(chunk["source"])
	trace := stringFromAny(chunk["source_trace"])
	source := CanonicalSource(rawSource, stageHint, trace)
	if source == SourceUnknown && rawSource != "" {
		source = rawSource
	}
	stage := strings.TrimSpace(stageHint)
	if stage == "" {
		stage = stringFromAny(chunk["pipeline_stage"])
	}
	if stage == "" {
		stage = InferStage(rawSource, source, trace, payload, stringFromAny(chunk["type"]))
	}
	method := InferMethod(rawSource, source, payload)
	conf := floatFromAnyDefault(chunk["confidence"], defaultConfidenceForSource(source))

	chunk["source"] = source
	chunk["pipeline_stage"] = stage
	if conf > 0 {
		chunk["confidence"] = round(conf, 3)
	}

	prov := mapFromAny(chunk["provenance"])
	if prov == nil {
		prov = map[string]any{}
	}
	prov["source"] = source
	if stage != "" {
		prov["stage"] = stage
	}
	if method != "" {
		prov["method"] = method
	}
	if trace != "" {
		prov["source_trace"] = trace
	}
	if conf > 0 {
		prov["confidence"] = round(conf, 3)
	}
	if rawSource != "" && rawSource != source {
		prov["raw_source"] = rawSource
	}
	if len(prov) > 0 {
		chunk["provenance"] = prov
	}
}

func ReportMap(report Report) map[string]any {
	stages := make([]map[string]any, 0, len(report.Stages))
	for _, stage := range report.Stages {
		m := map[string]any{
			"id":     stage.ID,
			"status": stage.Status,
		}
		if stage.Count > 0 {
			m["count"] = stage.Count
		}
		if stage.Detail != "" {
			m["detail"] = stage.Detail
		}
		if len(stage.Metadata) > 0 {
			m["metadata"] = stage.Metadata
		}
		stages = append(stages, m)
	}
	return map[string]any{
		"element_count":  report.ElementCount,
		"page_count":     report.PageCount,
		"source_counts":  report.SourceCounts,
		"stages":         stages,
		"architecture":   "layout_document_processor_pipeline",
		"reading_order":  "xycut",
		"provenance":     "normalized",
		"processor_path": []string{"provenance", "reading_order"},
	}
}

func defaultConfidenceForSource(source string) float64 {
	switch source {
	case SourceNative:
		return 0.82
	case SourceRepaired:
		return 0.76
	case SourceOCR:
		return 0.70
	case SourceVLM:
		return 0.74
	default:
		return 0.65
	}
}

func bboxFromAny(value any) *BBox {
	raw := mapFromAny(value)
	if raw == nil {
		return nil
	}
	box := BBox{
		Left:   clamp01(floatFromAnyDefault(raw["left"], 0)),
		Right:  clamp01(floatFromAnyDefault(raw["right"], 0)),
		Top:    clamp01(floatFromAnyDefault(raw["top"], 0)),
		Bottom: clamp01(floatFromAnyDefault(raw["bottom"], 0)),
	}
	if box.Right < box.Left {
		box.Left, box.Right = box.Right, box.Left
	}
	if box.Bottom < box.Top {
		box.Top, box.Bottom = box.Bottom, box.Top
	}
	if !box.Valid() {
		return nil
	}
	return &box
}

func mapFromAny(value any) map[string]any {
	switch x := value.(type) {
	case map[string]any:
		return x
	default:
		return nil
	}
}

func shallowCopyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprint(value))
	if s == "<nil>" {
		return ""
	}
	return s
}

func intFromAny(value any) int {
	switch x := value.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(x), "%d", &n); err == nil {
			return n
		}
	}
	return 0
}

func floatFromAnyDefault(value any, def float64) float64 {
	switch x := value.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(x), "%f", &f); err == nil {
			return f
		}
	}
	return def
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

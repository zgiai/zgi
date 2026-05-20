package chunking

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestCanonicalMapperFromParseArtifact(t *testing.T) {
	mapper := NewCanonicalMapper()

	confidence := 0.91

	doc, err := mapper.FromParseArtifact(&contracts.ParseArtifact{
		ArtifactID: "artifact-1",
		SourceRef:  "file-1",
		FileName:   "demo.pdf",
		Metadata: map[string]any{
			"document_id": "doc-1",
			"dataset_id":  "dataset-1",
			"source":      "native+ocr",
		},
		Elements: []contracts.ParsedElement{
			{ID: "b", Type: "text", Content: "body", Ordinal: 2},
			{ID: "a", Type: "heading", Content: "title", Ordinal: 1, Confidence: &confidence},
		},
	})
	if err != nil {
		t.Fatalf("FromParseArtifact() error = %v", err)
	}
	if doc.DocumentID != "doc-1" {
		t.Fatalf("DocumentID = %q", doc.DocumentID)
	}
	if len(doc.Elements) != 2 {
		t.Fatalf("elements=%d", len(doc.Elements))
	}
	if doc.Elements[0].Type != "heading" {
		t.Fatalf("first element type=%q", doc.Elements[0].Type)
	}
	if doc.Elements[0].Confidence == nil || *doc.Elements[0].Confidence != confidence {
		t.Fatalf("first element confidence=%v", doc.Elements[0].Confidence)
	}
	if doc.Source != "native+ocr" {
		t.Fatalf("source=%q", doc.Source)
	}
}

func TestCanonicalMapperPrefersRecognitionSourceAndLayoutOrder(t *testing.T) {
	mapper := NewCanonicalMapper()

	doc, err := mapper.FromParseArtifact(&contracts.ParseArtifact{
		FileName: "scan.jpg",
		Metadata: map[string]any{
			"source":             "content_parse_playground",
			"recognition_source": "local:ocr:image",
		},
		Elements: []contracts.ParsedElement{
			{
				ID:      "right",
				Type:    "text",
				Content: "right",
				Ordinal: 1,
				BBox:    &contracts.ParseBoundingBox{Left: 0.55, Top: 0.2, Right: 0.7, Bottom: 0.22},
				Metadata: map[string]any{
					"payload": map[string]any{"extraction_method": "ocr", "bbox_source": "ocr_line"},
				},
			},
			{
				ID:      "left",
				Type:    "text",
				Content: "left",
				Ordinal: 2,
				BBox:    &contracts.ParseBoundingBox{Left: 0.1, Top: 0.2, Right: 0.3, Bottom: 0.22},
				Metadata: map[string]any{
					"payload": map[string]any{"extraction_method": "ocr", "bbox_source": "ocr_line"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("FromParseArtifact() error = %v", err)
	}
	if doc.Source != "local:ocr:image" {
		t.Fatalf("source=%q", doc.Source)
	}
	if doc.Elements[0].ElementID != "left" || doc.Elements[1].ElementID != "right" {
		t.Fatalf("layout order = %s,%s", doc.Elements[0].ElementID, doc.Elements[1].ElementID)
	}
	if doc.Metadata["layout_order_applied"] != true {
		t.Fatalf("layout metadata = %#v", doc.Metadata)
	}
	if doc.Elements[0].Ordinal != 1 || doc.Elements[1].Ordinal != 2 {
		t.Fatalf("ordinals = %d,%d", doc.Elements[0].Ordinal, doc.Elements[1].Ordinal)
	}
}

func TestDefaultPlannerPlanSectionLikeDataset(t *testing.T) {
	planner := NewDefaultPlanner()

	plan, err := planner.Plan(&contracts.ChunkSourceDocument{
		Elements: []contracts.ChunkSourceElement{
			{Type: "heading"},
			{Type: "heading"},
			{Type: "heading"},
			{Type: "text"},
		},
	}, contracts.ChunkUseCaseDatasetIndex)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.ParentMode != parentModeSection {
		t.Fatalf("ParentMode = %q", plan.ParentMode)
	}
	if plan.Segmentation != "section_aware" {
		t.Fatalf("Segmentation = %q", plan.Segmentation)
	}
	if !plan.PreserveOrder {
		t.Fatal("expected PreserveOrder")
	}
}

func TestDefaultPlannerPlanScannedDocument(t *testing.T) {
	planner := NewDefaultPlanner()

	plan, err := planner.Plan(&contracts.ChunkSourceDocument{
		Source: "local+ocr",
		Elements: []contracts.ChunkSourceElement{
			{Type: "text", Content: "A", Page: 1, BBox: &contracts.ParseBoundingBox{}},
			{Type: "text", Content: "B", Page: 2, BBox: &contracts.ParseBoundingBox{}},
		},
	}, contracts.ChunkUseCaseDatasetIndex)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.ParentMode != parentModePageAware {
		t.Fatalf("ParentMode = %q", plan.ParentMode)
	}
	if plan.Segmentation != "page_layout_aware" {
		t.Fatalf("Segmentation = %q", plan.Segmentation)
	}
	if plan.Metadata["likely_scanned"] != true {
		t.Fatalf("likely_scanned metadata = %#v", plan.Metadata["likely_scanned"])
	}
}

func TestDefaultPlannerPreviewUsesPageLayoutForScannedDocument(t *testing.T) {
	planner := NewDefaultPlanner()

	plan, err := planner.Plan(&contracts.ChunkSourceDocument{
		Source: "local:ocr:image",
		Elements: []contracts.ChunkSourceElement{
			{Type: "text", Content: "A", Page: 0, BBox: &contracts.ParseBoundingBox{Left: 0.1, Top: 0.1, Right: 0.2, Bottom: 0.12}},
			{Type: "text", Content: "B", Page: 0, BBox: &contracts.ParseBoundingBox{Left: 0.1, Top: 0.2, Right: 0.2, Bottom: 0.22}},
		},
	}, contracts.ChunkUseCasePreview)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Segmentation != "page_layout_aware" {
		t.Fatalf("segmentation=%q", plan.Segmentation)
	}
	if plan.Metadata["page_count"] != 1 {
		t.Fatalf("page_count=%#v", plan.Metadata["page_count"])
	}
	if plan.Metadata["likely_scanned"] != true {
		t.Fatalf("likely_scanned=%#v", plan.Metadata["likely_scanned"])
	}
}

func TestServicePlanFromArtifact(t *testing.T) {
	service := NewService(nil, nil)

	result, err := service.PlanFromArtifact(context.Background(), &contracts.ParseArtifact{
		Metadata: map[string]any{"document_id": "doc-1"},
		Elements: []contracts.ParsedElement{
			{ID: "t1", Type: "text", Content: "hello", Ordinal: 1},
		},
	}, contracts.ChunkUseCasePreview)
	if err != nil {
		t.Fatalf("PlanFromArtifact() error = %v", err)
	}
	if result.Source.DocumentID != "doc-1" {
		t.Fatalf("DocumentID = %q", result.Source.DocumentID)
	}
	if result.Plan.UseCase != contracts.ChunkUseCasePreview {
		t.Fatalf("UseCase = %q", result.Plan.UseCase)
	}
}

func TestServiceExecuteFromArtifact(t *testing.T) {
	service := NewService(nil, nil)

	result, err := service.ExecuteFromArtifact(context.Background(), &contracts.ParseArtifact{
		Metadata: map[string]any{"document_id": "doc-1"},
		Elements: []contracts.ParsedElement{
			{ID: "h1", Type: "heading", Content: "Intro", Ordinal: 1},
			{ID: "t1", Type: "text", Content: "Hello", Ordinal: 2},
		},
	}, contracts.ChunkUseCaseDatasetIndex)
	if err != nil {
		t.Fatalf("ExecuteFromArtifact() error = %v", err)
	}
	if result.Execution == nil || len(result.Execution.Units) != 1 {
		t.Fatalf("execution result = %#v", result.Execution)
	}
	if result.Execution.Units[0].DocumentID != "doc-1" {
		t.Fatalf("unit document id = %q", result.Execution.Units[0].DocumentID)
	}
}

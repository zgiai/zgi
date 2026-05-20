package pdf

import (
	"fmt"
	"strings"
	"testing"

	pdfadapter "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/chunking"
)

func buildOnePagePDFWithAnnot() []byte {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm 6 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Contents 4 0 R /Annots [5 0 R] >>",
		"<< /Length 38 >>\nstream\nBT /F1 12 Tf 10 10 Td (Hello) Tj ET\nendstream",
		"<< /Type /Annot /Subtype /Text /Rect [10 10 20 20] /Contents (hi) >>",
		"<< /Fields [7 0 R] >>",
		"<< /FT /Tx /T (name) /V (alice) /Ff 0 /P 3 0 R /Rect [20 20 120 40] >>",
		"<< /Type /Filespec /F (demo.txt) /UF (demo.txt) /EF << /F 9 0 R >> >>",
		"<< /Type /EmbeddedFile /Length 5 >>\nstream\nhello\nendstream",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, body := range objs {
		offsets[i+1] = b.Len()
		b.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, body))
	}
	xrefPos := b.Len()
	b.WriteString("xref\n")
	b.WriteString(fmt.Sprintf("0 %d\n", len(objs)+1))
	b.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		b.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	b.WriteString("trailer\n")
	b.WriteString(fmt.Sprintf("<< /Size %d /Root 1 0 R >>\n", len(objs)+1))
	b.WriteString("startxref\n")
	b.WriteString(fmt.Sprintf("%d\n", xrefPos))
	b.WriteString("%%EOF\n")
	return []byte(b.String())
}

func TestParseFullDocumentBytesV7(t *testing.T) {
	payload, err := ParseFullDocumentBytes(buildOnePagePDFWithAnnot(), "mem://sample.pdf", "relaxed")
	if err != nil {
		t.Fatalf("ParseFullDocumentBytes: %v", err)
	}
	if got, _ := payload["schema_version"].(string); got != "full_document_v7" {
		t.Fatalf("schema_version=%q", got)
	}
	doc, _ := payload["document"].(map[string]any)
	if got, _ := doc["source"].(string); got != "mem://sample.pdf" {
		t.Fatalf("document.source=%q", got)
	}
	if _, ok := doc["image_like_pdf"]; !ok {
		t.Fatalf("document.image_like_pdf missing")
	}
	if _, ok := doc["suggest_vlm"].(bool); !ok {
		t.Fatalf("document.suggest_vlm missing")
	}
	routeDecision, ok := doc["route_decision"].(map[string]any)
	if !ok {
		t.Fatalf("document.route_decision missing")
	}
	if got, _ := routeDecision["recommended_mode"].(string); got != "native_only" {
		t.Fatalf("route_decision.recommended_mode=%q", got)
	}
	pageCandidates, ok := doc["page_route_candidates"].([]map[string]any)
	if !ok || len(pageCandidates) != 1 {
		t.Fatalf("document.page_route_candidates=%T len=%d", doc["page_route_candidates"], len(pageCandidates))
	}
	if got, _ := pageCandidates[0]["recommended_mode"].(string); got != "native_only" {
		t.Fatalf("page_route_candidates[0].recommended_mode=%q", got)
	}
	chunks, _ := payload["chunks"].(map[string]any)
	if cnt, _ := chunks["count"].(int); cnt < 4 {
		t.Fatalf("chunks.count=%v", chunks["count"])
	}
	items, _ := chunks["items"].([]map[string]any)
	if len(items) == 0 {
		t.Fatalf("chunks.items empty")
	}
	bbox, _ := items[0]["bbox"].(map[string]any)
	if _, ok := bbox["left"]; !ok {
		t.Fatalf("chunk bbox missing left: %+v", bbox)
	}
	layout, _ := doc["layout"].(map[string]any)
	pages, _ := layout["pages"].([]pageBoxRow)
	if len(pages) == 0 {
		t.Fatalf("document.layout.pages empty")
	}
	if len(pages[0].LineElements) == 0 {
		t.Fatalf("document.layout.pages[0].line_elements empty")
	}
	lineBox, _ := pages[0].LineElements[0]["box"].(map[string]any)
	if lineBox == nil {
		t.Fatalf("document.layout.pages[0].line_elements[0].box missing")
	}
	top, _ := lineBox["top"].(float64)
	bottom, _ := lineBox["bottom"].(float64)
	if !(top < bottom) {
		t.Fatalf("expected top-left bbox semantics in line element, got top=%v bottom=%v", top, bottom)
	}
}

func TestParseFullDocumentBytesRouteDecisionForceVLM(t *testing.T) {
	t.Setenv("DOCSTILL_FORCE_VLM", "1")

	payload, err := ParseFullDocumentBytes(buildOnePagePDFWithAnnot(), "mem://force.pdf", "relaxed")
	if err != nil {
		t.Fatalf("ParseFullDocumentBytes: %v", err)
	}
	doc, _ := payload["document"].(map[string]any)
	if got, _ := doc["suggest_vlm"].(bool); !got {
		t.Fatalf("document.suggest_vlm=%v, want true", doc["suggest_vlm"])
	}
	routeDecision, _ := doc["route_decision"].(map[string]any)
	if got, _ := routeDecision["recommended_mode"].(string); got != "force_vlm" {
		t.Fatalf("route_decision.recommended_mode=%q", got)
	}
	pageCandidates, _ := doc["page_route_candidates"].([]map[string]any)
	if len(pageCandidates) != 1 {
		t.Fatalf("document.page_route_candidates len=%d", len(pageCandidates))
	}
	if got, _ := pageCandidates[0]["recommended_mode"].(string); got != "force_vlm" {
		t.Fatalf("page_route_candidates[0].recommended_mode=%q", got)
	}
	if got, _ := doc["force_vlm"].(bool); !got {
		t.Fatalf("document.force_vlm=%v, want true", doc["force_vlm"])
	}
}

func TestAttachLayoutLineElements_UsesGeometryLineBBoxBeforeAnchor(t *testing.T) {
	pages := []pageBoxRow{{PageIndex: 1, ObjectNumber: 7}}
	lines := []pdfadapter.GeometryLine{
		{
			PageIndex:   1,
			SourceTrace: "page#1 obj#7",
			Order:       3,
			Text:        "Customer service",
			GeomX:       90,
			GeomY:       90,
			BBox: &pdfadapter.TextBBox{
				Left:   10,
				Bottom: 20,
				Right:  40,
				Top:    50,
			},
		},
	}
	pageGeoms := map[int]chunking.PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	}

	attachLayoutLineElements(pages, lines, pageGeoms)

	if len(pages[0].LineElements) != 1 {
		t.Fatalf("line_elements=%+v", pages[0].LineElements)
	}
	box, _ := pages[0].LineElements[0]["box"].(map[string]any)
	if box == nil {
		t.Fatalf("missing box: %+v", pages[0].LineElements[0])
	}
	if box["left"] != 0.1 || box["right"] != 0.4 || box["top"] != 0.5 || box["bottom"] != 0.8 {
		t.Fatalf("expected normalized line bbox, got %+v", box)
	}
}

package pdf

import (
	"strings"
	"testing"

	pdfadapter "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/chunking"
)

func TestAnalyzeAndRepairLocalLayout_AddsMissingRightColumnChunks(t *testing.T) {
	pageGeoms := map[int]chunking.PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	}
	lines := []pdfadapter.GeometryLine{
		{PageIndex: 1, Order: 1, Text: "Your electricity bill in more detail", GeomX: 5, GeomY: 94},
		{PageIndex: 1, Order: 2, Text: "Customer service", GeomX: 70, GeomY: 94},
		{PageIndex: 1, Order: 3, Text: "Phone: 1800 372 372 8am-8pm Mon-Fri", GeomX: 70, GeomY: 90},
		{PageIndex: 1, Order: 4, Text: "Email: service@example.test", GeomX: 70, GeomY: 86},
		{PageIndex: 1, Order: 5, Text: "Payment options", GeomX: 70, GeomY: 78},
		{PageIndex: 1, Order: 6, Text: "You can also call 1800 372 372 or use online services for all payment options.", GeomX: 70, GeomY: 74},
	}
	chunks := []map[string]any{
		{
			"chunk_id":   "left_1",
			"type":       "paragraph",
			"text":       "Your electricity bill in more detail",
			"page_index": 1,
			"order":      0,
			"bbox": map[string]any{
				"left": 0.04, "right": 0.48, "top": 0.04, "bottom": 0.10,
			},
		},
	}

	got := analyzeAndRepairLocalLayout("bill.pdf", nil, chunks, lines, pageGeoms)
	if got.RepairAdded == 0 {
		t.Fatalf("expected right column repair chunks, got none: %+v", got.Payload)
	}
	if !strings.Contains(joinChunkText(got.Chunks), "Customer service") {
		t.Fatalf("repaired chunks missing right column text: %+v", got.Chunks)
	}
	if got.Payload["repair_chunks_added"] != got.RepairAdded {
		t.Fatalf("payload repair count mismatch: %+v", got.Payload)
	}
}

func TestAnalyzeAndRepairLocalLayout_AddsMissingGenericRegionChunks(t *testing.T) {
	pageGeoms := map[int]chunking.PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	}
	lines := []pdfadapter.GeometryLine{
		{PageIndex: 1, Order: 1, Text: "Implementation plan", GeomX: 8, GeomY: 88, BBox: &pdfadapter.TextBBox{Left: 0.08, Right: 0.35, Top: 0.90, Bottom: 0.86}},
		{PageIndex: 1, Order: 2, Text: "Define reusable layout regions", GeomX: 8, GeomY: 82, BBox: &pdfadapter.TextBBox{Left: 0.08, Right: 0.54, Top: 0.84, Bottom: 0.80}},
		{PageIndex: 1, Order: 3, Text: "Measure extracted text coverage", GeomX: 8, GeomY: 76, BBox: &pdfadapter.TextBBox{Left: 0.08, Right: 0.52, Top: 0.78, Bottom: 0.74}},
		{PageIndex: 1, Order: 4, Text: "Recover missing visual rows", GeomX: 8, GeomY: 70, BBox: &pdfadapter.TextBBox{Left: 0.08, Right: 0.50, Top: 0.72, Bottom: 0.68}},
		{PageIndex: 1, Order: 5, Text: "Preserve reading order across columns", GeomX: 8, GeomY: 64, BBox: &pdfadapter.TextBBox{Left: 0.08, Right: 0.60, Top: 0.66, Bottom: 0.62}},
	}
	chunks := []map[string]any{
		{
			"chunk_id":   "existing",
			"type":       "paragraph",
			"text":       "Existing unrelated paragraph",
			"page_index": 1,
			"order":      0,
			"bbox": map[string]any{
				"left": 0.70, "right": 0.90, "top": 0.20, "bottom": 0.30,
			},
		},
	}

	got := analyzeAndRepairLocalLayout("generic.pdf", nil, chunks, lines, pageGeoms)
	if got.RepairAdded == 0 {
		t.Fatalf("expected generic layout repair chunks, got none: %+v", got.Payload)
	}
	if !strings.Contains(joinChunkText(got.Chunks), "Recover missing visual rows") {
		t.Fatalf("repaired chunks missing generic region text: %+v", got.Chunks)
	}
	regions, _ := got.Payload["low_confidence_regions"].([]map[string]any)
	if len(regions) == 0 {
		t.Fatalf("expected low confidence regions: %+v", got.Payload)
	}
	if regions[0]["route"] != "ocr_vlm_region" {
		t.Fatalf("unexpected region route: %+v", regions[0])
	}
}

func TestAnalyzeAndRepairLocalLayout_ReportsUnstableBBoxRegion(t *testing.T) {
	pageGeoms := map[int]chunking.PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	}
	lines := []pdfadapter.GeometryLine{
		{
			PageIndex: 1,
			Order:     1,
			Text:      "完善AI工具链: 提供覆盖AI开发全流程的工具套件",
			GeomX:     8,
			GeomY:     82,
			BBox:      &pdfadapter.TextBBox{Left: 0.08, Right: 0.86, Top: 0.84, Bottom: 0.80},
		},
	}
	chunks := []map[string]any{
		{
			"chunk_id":   "fragment_bbox",
			"type":       "paragraph",
			"text":       "完善AI工具链: 提供覆盖AI开发全流程的工具套件",
			"page_index": 1,
			"order":      0,
			"bbox": map[string]any{
				"left": 0.42, "right": 0.58, "top": 0.805, "bottom": 0.835,
			},
		},
	}

	got := analyzeAndRepairLocalLayout("slide.pdf", nil, chunks, lines, pageGeoms)
	regions, _ := got.Payload["low_confidence_regions"].([]map[string]any)
	if len(regions) == 0 {
		t.Fatalf("expected unstable bbox region: %+v", got.Payload)
	}
	if regions[0]["reason"] != "bbox_mismatch_geometry_lines" {
		t.Fatalf("unexpected region: %+v", regions[0])
	}
	box := regions[0]["bbox"].(map[string]any)
	if box["left"].(float64) >= 0.10 || box["right"].(float64) <= 0.80 {
		t.Fatalf("expected region to use geometry line bbox, got %+v", box)
	}
}

func TestPrepareLocalLayoutLines_UsesActualGeometryLineBBox(t *testing.T) {
	pageGeoms := map[int]chunking.PageGeom{
		1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
	}
	lines := []pdfadapter.GeometryLine{
		{
			PageIndex: 1,
			Order:     1,
			Text:      "A long visual row",
			GeomX:     20,
			GeomY:     80,
			BBox:      &pdfadapter.TextBBox{Left: 0.12, Right: 0.78, Top: 0.84, Bottom: 0.79},
		},
	}

	got := prepareLocalLayoutLines(lines, pageGeoms)
	if len(got[1]) != 1 {
		t.Fatalf("expected one layout line: %+v", got)
	}
	box := got[1][0].BBox
	if box["left"] != 0.12 || box["right"] != 0.78 {
		t.Fatalf("expected actual geometry bbox, got %+v", box)
	}
	if got[1][0].X < 0.44 || got[1][0].X > 0.46 {
		t.Fatalf("expected center x from bbox, got %+v", got[1][0])
	}
}

func TestOrderChunkMapsByLayout_SortsTwoColumnsByXYCut(t *testing.T) {
	chunks := []map[string]any{
		layoutSortTestChunk("right_top", 1, 0, 0.66, 0.94, 0.05, 0.10),
		layoutSortTestChunk("left_bottom", 1, 1, 0.05, 0.52, 0.35, 0.42),
		layoutSortTestChunk("left_top", 1, 2, 0.05, 0.52, 0.05, 0.10),
		layoutSortTestChunk("right_bottom", 1, 3, 0.66, 0.94, 0.35, 0.42),
	}

	got := orderChunkMapsByLayout(chunks)
	ids := []string{
		got[0]["chunk_id"].(string),
		got[1]["chunk_id"].(string),
		got[2]["chunk_id"].(string),
		got[3]["chunk_id"].(string),
	}
	want := []string{"left_top", "left_bottom", "right_top", "right_bottom"}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order mismatch got=%v want=%v", ids, want)
		}
		if got[i]["order"] != i {
			t.Fatalf("reassigned order[%d]=%v", i, got[i]["order"])
		}
	}
}

func TestParseTesseractTSV_MapsCropCoordinatesToPageBBox(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t10\t92\tCustomer\n" +
		"5\t1\t1\t1\t1\t2\t45\t20\t35\t10\t90\tservice\n"

	lines := parseTesseractTSV(tsv, 100, 100, 2)
	if len(lines) != 1 {
		t.Fatalf("expected one OCR line, got %+v", lines)
	}
	if lines[0].PageIndex != 2 || lines[0].Text != "Customer service" {
		t.Fatalf("unexpected line: %+v", lines[0])
	}
	if lines[0].BBox["left"] <= localOCRCropLeft || lines[0].BBox["right"] <= lines[0].BBox["left"] {
		t.Fatalf("bbox not mapped into crop: %+v", lines[0].BBox)
	}
}

func TestMarkdownTableFromNumericRows(t *testing.T) {
	got, ok := markdownTableFromNumericRows(strings.Join([]string{
		"Coal 0.8% 1.12%",
		"Natural Gas 31% 34.72%",
		"Renewables 67% 62.35%",
		"Total 100% 100%",
	}, "\n"))
	if !ok {
		t.Fatalf("expected numeric rows to become a markdown table")
	}
	if !strings.Contains(got, "| Natural Gas | 31% | 34.72% |") {
		t.Fatalf("unexpected table markdown: %s", got)
	}
}

func layoutSortTestChunk(id string, page, order int, left, right, top, bottom float64) map[string]any {
	return map[string]any{
		"chunk_id":   id,
		"type":       "paragraph",
		"text":       id,
		"page_index": page,
		"order":      order,
		"bbox": map[string]any{
			"left": left, "right": right, "top": top, "bottom": bottom,
		},
	}
}

func joinChunkText(chunks []map[string]any) string {
	var b strings.Builder
	for _, ch := range chunks {
		if text, _ := ch["text"].(string); text != "" {
			b.WriteString(text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

package chunking

import (
	"strings"
	"testing"
)

type fixedRule struct{}

func (fixedRule) Name() string { return "fixed" }
func (fixedRule) Apply(_ BuildInput, chunks []Chunk) []Chunk {
	return append(chunks, Chunk{
		ChunkID:    "c1",
		Type:       "paragraph",
		Text:       "hello",
		PageIndex:  1,
		Order:      1,
		Source:     "test",
		Confidence: 1,
	})
}

func TestBuildChunks_UsesProvidedRules(t *testing.T) {
	got := BuildChunks(BuildInput{}, fixedRule{})
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].ChunkID != "c1" {
		t.Fatalf("chunk_id=%q", got[0].ChunkID)
	}
}

func TestBuild_DefaultRulesProduceMaps(t *testing.T) {
	input := BuildInput{
		Texts: []TextLike{{
			Order:       7,
			SourceTrace: "page#1",
			Text:        "x",
			ChunkType:   "paragraph",
		}},
	}
	got := Build(input)
	if len(got) == 0 {
		t.Fatal("empty chunks")
	}
	if got[0]["chunk_id"] != "seg_7" {
		t.Fatalf("chunk_id=%v", got[0]["chunk_id"])
	}
}

func TestBuild_DefaultRulesDetectTableChunk(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{Order: 1, SourceTrace: "page#1", Text: "H1", ChunkType: "paragraph", GeomX: 20, GeomY: 180},
			{Order: 2, SourceTrace: "page#1", Text: "H2", ChunkType: "paragraph", GeomX: 120, GeomY: 180},
			{Order: 3, SourceTrace: "page#1", Text: "A1", ChunkType: "paragraph", GeomX: 20, GeomY: 150},
			{Order: 4, SourceTrace: "page#1", Text: "A2", ChunkType: "paragraph", GeomX: 120, GeomY: 150},
		},
	}
	chunks := Build(input)
	found := false
	for _, c := range chunks {
		typ, _ := c["type"].(string)
		if typ != "table" {
			continue
		}
		found = true
		payload, _ := c["payload"].(map[string]any)
		if payload == nil {
			t.Fatal("table payload missing")
		}
		if rc, _ := payload["row_count"].(int); rc < 2 {
			t.Fatalf("row_count=%v", payload["row_count"])
		}
		break
	}
	if !found {
		t.Fatal("table chunk not found")
	}
}

func TestBuild_SkipTableRule_OmitsTableChunks(t *testing.T) {
	input := BuildInput{
		SkipTableRule: true,
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{Order: 1, SourceTrace: "page#1", Text: "H1", ChunkType: "paragraph", GeomX: 20, GeomY: 180},
			{Order: 2, SourceTrace: "page#1", Text: "H2", ChunkType: "paragraph", GeomX: 120, GeomY: 180},
			{Order: 3, SourceTrace: "page#1", Text: "A1", ChunkType: "paragraph", GeomX: 20, GeomY: 150},
			{Order: 4, SourceTrace: "page#1", Text: "A2", ChunkType: "paragraph", GeomX: 120, GeomY: 150},
		},
	}
	chunks := Build(input)
	for _, c := range chunks {
		typ, _ := c["type"].(string)
		if typ == "table" || typ == "table_debug" {
			t.Fatalf("unexpected chunk type %q when SkipTableRule", typ)
		}
	}
}

func TestBuild_RepeatedSameSizeStampLikeImagesRemainFigures(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{1: {Left: 0, Bottom: 0, Right: 612, Top: 792}},
		Images: []ImageLike{
			{PageIndex: 1, ObjectNumber: 27, Format: "jpeg", Width: 347, Height: 260, ByteSize: 27138},
			{PageIndex: 1, ObjectNumber: 28, Format: "jpeg", Width: 347, Height: 260, ByteSize: 24349},
		},
	}
	chunks := Build(input)
	var images, stamps int
	for _, c := range chunks {
		switch c["type"] {
		case "image":
			images++
		case "stamp":
			stamps++
		}
	}
	if images != 2 || stamps != 0 {
		t.Fatalf("images=%d stamps=%d chunks=%v", images, stamps, chunks)
	}
}

func TestBuild_GeometryTokenV2_TableFromMultiTokenRows(t *testing.T) {
	g := PageGeom{Left: 0, Bottom: 0, Right: 100, Top: 100}
	input := BuildInput{
		PageGeoms: map[int]PageGeom{1: g},
		GeometryTokens: []GeometryTokenLike{
			{Order: 0, SourceTrace: "page#1", PageIndex: 1, Text: "Item", GeomX: 10, GeomY: 82},
			{Order: 1, SourceTrace: "page#1", PageIndex: 1, Text: "Amount", GeomX: 55, GeomY: 82},
			{Order: 2, SourceTrace: "page#1", PageIndex: 1, Text: "Total", GeomX: 12, GeomY: 77},
			{Order: 3, SourceTrace: "page#1", PageIndex: 1, Text: "€12.34", GeomX: 58, GeomY: 77},
		},
		Texts: []TextLike{{Order: 99, SourceTrace: "page#1", Text: "p", ChunkType: "paragraph"}},
	}
	chunks := Build(input)
	var mode string
	for _, c := range chunks {
		if typ, _ := c["type"].(string); typ != "table" {
			continue
		}
		p, _ := c["payload"].(map[string]any)
		if p == nil {
			continue
		}
		mode, _ = p["detection_mode"].(string)
		if mode == "geometry_token_v2" {
			return
		}
	}
	t.Fatalf("expected geometry_token_v2 table, got mode=%q", mode)
}

func TestBuild_GeometryTokenV2_AvoidsTwoColumnNarrativeFalseTable(t *testing.T) {
	g := PageGeom{Left: 0, Bottom: 0, Right: 100, Top: 100}
	input := BuildInput{
		PageGeoms: map[int]PageGeom{1: g},
		GeometryTokens: []GeometryTokenLike{
			{Order: 0, SourceTrace: "page#1", PageIndex: 1, Text: "Customer account details", GeomX: 8, GeomY: 88},
			{Order: 1, SourceTrace: "page#1", PageIndex: 1, Text: "Please have your account number ready when contacting support", GeomX: 60, GeomY: 88},
			{Order: 2, SourceTrace: "page#1", PageIndex: 1, Text: "Billing period and consumption summary", GeomX: 8, GeomY: 82},
			{Order: 3, SourceTrace: "page#1", PageIndex: 1, Text: "Emergency information and customer service opening hours", GeomX: 60, GeomY: 82},
			{Order: 4, SourceTrace: "page#1", PageIndex: 1, Text: "Payment advice and account notes", GeomX: 8, GeomY: 76},
			{Order: 5, SourceTrace: "page#1", PageIndex: 1, Text: "Complaints process and regulatory contact details", GeomX: 60, GeomY: 76},
		},
	}
	chunks := Build(input)
	for _, c := range chunks {
		if typ, _ := c["type"].(string); typ == "table" {
			t.Fatalf("unexpected narrative table chunk: %+v", c)
		}
	}
}

func TestSplitRowCellsIntoTableBlocks_RejectsSparseSingleCellRows(t *testing.T) {
	r1 := cellRun{points: []textPoint{{x: 0.1, y: 0.1, bbox: &BBox{Left: 0.1, Right: 0.2, Top: 0.9, Bottom: 0.8}}}}
	r2 := cellRun{points: []textPoint{{x: 0.2, y: 0.35, bbox: &BBox{Left: 0.2, Right: 0.3, Top: 0.9, Bottom: 0.8}}}}
	rowCells := [][]cellRun{
		{r1},
		{r2},
	}
	blocks := splitRowCellsIntoTableBlocks(rowCells)
	if len(blocks) != 0 {
		t.Fatalf("blocks=%d, want=0", len(blocks))
	}
}

func TestBuild_DefaultRulesAvoidListFalsePositive(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{Order: 1, SourceTrace: "page#1", Text: "1. item one", ChunkType: "paragraph", GeomX: 20, GeomY: 180},
			{Order: 2, SourceTrace: "page#1", Text: "2. item two", ChunkType: "paragraph", GeomX: 20, GeomY: 160},
			{Order: 3, SourceTrace: "page#1", Text: "3. item three", ChunkType: "paragraph", GeomX: 20, GeomY: 140},
			{Order: 4, SourceTrace: "page#1", Text: "4. item four", ChunkType: "paragraph", GeomX: 20, GeomY: 120},
		},
	}
	chunks := Build(input)
	for _, c := range chunks {
		if typ, _ := c["type"].(string); typ == "table" {
			t.Fatalf("unexpected table chunk: %+v", c)
		}
	}
}

func TestBuild_DefaultRulesDetectFormulaChunk(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       1,
				SourceTrace: "page#1",
				Text:        "$ST_i(总) = T_i(统筹) + T_i(修龄) + T_i(待遇) + T_i(浮动)$",
				ChunkType:   "paragraph",
				GeomX:       30,
				GeomY:       120,
			},
		},
	}
	chunks := Build(input)
	for _, c := range chunks {
		typ, _ := c["type"].(string)
		if typ != "formula" {
			continue
		}
		txt, _ := c["text"].(string)
		if txt == "" || !strings.HasPrefix(txt, "formula:") {
			t.Fatalf("unexpected formula text: %q", txt)
		}
		return
	}
	t.Fatal("formula chunk not found")
}

func TestBuild_DefaultRulesUseProvidedBBoxForTextChunks(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       1,
				SourceTrace: "page#1",
				Text:        "精准坐标段落",
				ChunkType:   "paragraph",
				GeomX:       10,
				GeomY:       10,
				BBox: &BBox{
					Left:   0.11,
					Right:  0.43,
					Top:    0.88,
					Bottom: 0.80,
				},
			},
		},
	}
	chunks := Build(input)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	bb, _ := chunks[0]["bbox"].(map[string]any)
	if bb == nil {
		t.Fatal("expected bbox")
	}
	if bb["left"] != 0.11 || bb["right"] != 0.43 || bb["top"] != 0.12 || bb["bottom"] != 0.2 {
		t.Fatalf("bbox=%v", bb)
	}
}

func TestBuild_AlignsMultilineChildChunksToGeometryLines(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 1, Top: 1},
		},
		Texts: []TextLike{
			{
				Order:       1,
				SourceTrace: "page#1 obj#7",
				Text:        "6.1 参数设置\n这里是第一行正文内容较长\n这里是第二行正文内容较长",
				ChunkType:   "paragraph",
				GeomX:       0.1,
				GeomY:       0.9,
			},
		},
		GeometryLines: []GeometryLineLike{
			{
				Order:       1,
				SourceTrace: "page#1 obj#7",
				PageIndex:   1,
				Text:        "6.1 参数设置",
				BBox: &BBox{
					Left:   0.10,
					Right:  0.36,
					Top:    0.94,
					Bottom: 0.90,
				},
			},
			{
				Order:       2,
				SourceTrace: "page#1 obj#7",
				PageIndex:   1,
				Text:        "这里是第一行正文内容较长",
				BBox: &BBox{
					Left:   0.12,
					Right:  0.68,
					Top:    0.84,
					Bottom: 0.79,
				},
			},
			{
				Order:       3,
				SourceTrace: "page#1 obj#7",
				PageIndex:   1,
				Text:        "这里是第二行正文内容较长",
				BBox: &BBox{
					Left:   0.12,
					Right:  0.72,
					Top:    0.77,
					Bottom: 0.71,
				},
			},
		},
	}
	chunks := Build(input)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	var headingBB, paraBB map[string]any
	for _, c := range chunks {
		switch c["text"] {
		case "6.1 参数设置":
			headingBB, _ = c["bbox"].(map[string]any)
		case "这里是第一行正文内容较长\n这里是第二行正文内容较长":
			paraBB, _ = c["bbox"].(map[string]any)
		}
	}
	if headingBB == nil {
		t.Fatal("missing heading bbox")
	}
	if paraBB == nil {
		t.Fatal("missing paragraph bbox")
	}
	if headingBB["left"] != 0.1 || headingBB["right"] != 0.36 || headingBB["top"] != 0.06 || headingBB["bottom"] != 0.1 {
		t.Fatalf("unexpected heading bbox=%v", headingBB)
	}
	if paraBB["left"] != 0.12 || paraBB["right"] != 0.72 || paraBB["top"] != 0.16 || paraBB["bottom"] != 0.29 {
		t.Fatalf("unexpected paragraph bbox=%v", paraBB)
	}
}

func TestBuild_DefaultRulesAvoidFormulaFalsePositive(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 200, Top: 200},
		},
		Texts: []TextLike{
			{
				Order:       1,
				SourceTrace: "page#1",
				Text:        "https://example.com/a-b-c",
				ChunkType:   "paragraph",
				GeomX:       30,
				GeomY:       120,
			},
		},
	}
	chunks := Build(input)
	for _, c := range chunks {
		if typ, _ := c["type"].(string); typ == "formula" {
			t.Fatalf("unexpected formula chunk: %+v", c)
		}
	}
}

func TestBuild_SplitTextUsesGeometryLineBBoxes(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
		},
		Texts: []TextLike{
			{
				Order:       5,
				SourceTrace: "page#1/stream#7",
				Text:        "前端面试题\n\n第一题正文\n第二题正文",
				ChunkType:   "paragraph",
				GeomX:       12,
				GeomY:       95,
			},
		},
		GeometryLines: []GeometryLineLike{
			{Order: 0, PageIndex: 1, SourceTrace: "page#1/stream#7", Text: "前端面试题", GeomX: 12, GeomY: 95},
			{Order: 1, PageIndex: 1, SourceTrace: "page#1/stream#7", Text: "第一题正文", GeomX: 12, GeomY: 72},
			{Order: 2, PageIndex: 1, SourceTrace: "page#1/stream#7", Text: "第二题正文", GeomX: 12, GeomY: 61},
		},
	}

	chunks := BuildChunks(input)
	var heading, body *Chunk
	for i := range chunks {
		switch chunks[i].ChunkID {
		case "seg_5_L0":
			heading = &chunks[i]
		case "seg_5_L1":
			body = &chunks[i]
		}
	}
	if heading == nil || body == nil {
		t.Fatalf("expected split chunks, got %+v", chunks)
	}
	if heading.BBox == nil || body.BBox == nil {
		t.Fatalf("missing bbox heading=%+v body=%+v", heading, body)
	}
	if heading.BBox.Top <= body.BBox.Top {
		t.Fatalf("heading should sit above paragraph: heading=%+v body=%+v", heading.BBox, body.BBox)
	}
	if body.BBox.Top < 0.71 || body.BBox.Bottom > 0.62 {
		t.Fatalf("paragraph bbox should follow geometry lines, got %+v", body.BBox)
	}
	if body.BBox.Bottom >= 0.9 {
		t.Fatalf("paragraph bbox unexpectedly kept segment anchor bbox: %+v", body.BBox)
	}
}

func TestBuild_PrefersActualGeometryLineBBox(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
		},
		Texts: []TextLike{
			{
				Order:       8,
				SourceTrace: "page#1",
				Text:        "A long visual row",
				ChunkType:   "paragraph",
				GeomX:       20,
				GeomY:       80,
			},
		},
		GeometryLines: []GeometryLineLike{
			{
				Order:       0,
				PageIndex:   1,
				SourceTrace: "page#1",
				Text:        "A long visual row",
				GeomX:       20,
				GeomY:       80,
				BBox:        &BBox{Left: 0.12, Right: 0.76, Top: 0.84, Bottom: 0.78},
			},
		},
	}

	chunks := BuildChunks(input)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	got := chunks[0].BBox
	if got == nil {
		t.Fatalf("missing bbox: %+v", chunks[0])
	}
	if got.Left != 0.12 || got.Right != 0.76 || got.Top != 0.84 || got.Bottom != 0.78 {
		t.Fatalf("expected actual geometry bbox, got %+v", got)
	}
}

func TestBuildChunks_PrefersGeometryBlocksWhenNativeSpanCrossesRegionGap(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
		},
		Texts: []TextLike{
			{
				Order:       29,
				SourceTrace: "page#1 obj#4",
				Text:        "Payment terms are 14 days from date of bill issue or\nimmediately if overdue.\nInformation on the Fuel Mix and environmental impact is on\nthe back of this bill.\n0001 9518578800 000000182010 798258",
				ChunkType:   "paragraph",
				GeomX:       4,
				GeomY:       28,
			},
		},
		GeometryLines: []GeometryLineLike{
			{Order: 37, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "Payment terms are 14 days from date of bill issue or", GeomX: 4, GeomY: 28},
			{Order: 38, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "immediately if overdue.", GeomX: 4, GeomY: 27},
			{Order: 39, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "Information on the Fuel Mix and environmental impact is on", GeomX: 4, GeomY: 26},
			{Order: 40, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "the back of this bill.", GeomX: 4, GeomY: 25},
			{Order: 41, PageIndex: 1, SourceTrace: "page#1 obj#4", Text: "0001 9518578800 000000182010 798258", GeomX: 10, GeomY: 14},
		},
	}

	chunks := BuildChunks(input)
	foundFooter := false
	var barcode *Chunk
	for i := range chunks {
		if strings.Contains(chunks[i].Text, "Payment terms") && strings.Contains(chunks[i].Text, "0001 9518578800") {
			t.Fatalf("native span was not split by geometry blocks: %+v", chunks[i])
		}
		if strings.HasPrefix(chunks[i].ChunkID, "seg_37") && strings.Contains(chunks[i].Text, "Payment terms") {
			foundFooter = true
		}
		if chunks[i].ChunkID == "seg_41" && strings.Contains(chunks[i].Text, "0001 9518578800") {
			barcode = &chunks[i]
		}
	}
	if !foundFooter || barcode == nil {
		t.Fatalf("expected footer and barcode chunks, got %+v", chunks)
	}
	if barcode.BBox == nil {
		t.Fatalf("expected geometry bbox for barcode=%+v", barcode)
	}
	if barcode.BBox.Top > 0.2 || barcode.BBox.Bottom < 0.1 {
		t.Fatalf("barcode should keep its bottom-region bbox, got %+v", barcode.BBox)
	}
}

func TestTextBBoxForTextLike_UsesProvidedGeometryLineBBoxWithoutAnchor(t *testing.T) {
	input := BuildInput{
		PageGeoms: map[int]PageGeom{
			1: {Left: 0, Bottom: 0, Right: 100, Top: 100},
		},
		GeometryLines: []GeometryLineLike{
			{
				Order:       1,
				PageIndex:   1,
				SourceTrace: "page#1 obj#9",
				Text:        "Customer service",
				BBox: &BBox{
					Left:   0.62,
					Right:  0.84,
					Top:    0.94,
					Bottom: 0.90,
				},
			},
		},
	}

	got := textBBoxForTextLike(TextLike{
		SourceTrace: "page#1 obj#9",
		Text:        "Customer service",
	}, input)

	if got == nil {
		t.Fatal("expected bbox from geometry line")
	}
	if *got != (BBox{Left: 0.62, Right: 0.84, Top: 0.94, Bottom: 0.90}) {
		t.Fatalf("bbox=%+v", got)
	}
}

func TestAnchorToBBox_MakesReasonableSizedWindow(t *testing.T) {
	g := PageGeom{Left: 0, Bottom: 0, Right: 1000, Top: 1000}
	b := AnchorToBBox(500, 500, g)
	if b == nil {
		t.Fatal("bbox should not be nil")
	}
	width := b.Right - b.Left
	height := b.Top - b.Bottom
	if width <= 0 || height <= 0 {
		t.Fatalf("invalid bbox width=%f height=%f: %+v", width, height, b)
	}
	if width > 0.07 {
		t.Fatalf("anchor bbox width too wide=%f: %+v", width, b)
	}
	if height > 0.05 {
		t.Fatalf("anchor bbox height too tall=%f: %+v", height, b)
	}
}

func TestAnchorToBBox_InvalidPageGeometryReturnsNil(t *testing.T) {
	g := PageGeom{Left: 10, Bottom: 10, Right: 10, Top: 20}
	if b := AnchorToBBox(100, 100, g); b != nil {
		t.Fatalf("expected nil bbox for degenerate page geometry: %+v", b)
	}
	g = PageGeom{Left: 10, Bottom: 20, Right: 30, Top: 10}
	if b := AnchorToBBox(100, 100, g); b != nil {
		t.Fatalf("expected nil bbox for invalid orientation: %+v", b)
	}
	if b := AnchorToBBox(0, 0, g); b != nil {
		t.Fatalf("expected nil bbox for zero anchor: %+v", b)
	}
}

func TestAnchorToBBox_ClampsToPageEdges(t *testing.T) {
	g := PageGeom{Left: 100, Bottom: 200, Right: 1100, Top: 1400}
	b := AnchorToBBox(100, 200, g)
	if b == nil {
		t.Fatal("bbox should not be nil")
	}
	if b.Left != 0 || b.Bottom != 0 || b.Right > 0.06 || b.Top > 0.04 {
		t.Fatalf("edge anchor should clamp to page start with sensible size: %+v", b)
	}
	if b.Left < 0 || b.Bottom < 0 || b.Right > 1 || b.Top > 1 {
		t.Fatalf("clamped bbox out of bounds: %+v", b)
	}
}

func TestBBoxTopLeftMap_FlipsYAxisForAPI(t *testing.T) {
	box := &BBox{
		Left:   0.111,
		Right:  0.589,
		Top:    0.622,
		Bottom: 0.569,
	}
	got := BBoxTopLeftMap(box)
	if got == nil {
		t.Fatal("expected bbox map")
	}
	if got["left"] != 0.111 || got["right"] != 0.589 {
		t.Fatalf("unexpected x coords: %+v", got)
	}
	if got["top"] != 0.378 || got["bottom"] != 0.431 {
		t.Fatalf("expected flipped y coords, got %+v", got)
	}
}

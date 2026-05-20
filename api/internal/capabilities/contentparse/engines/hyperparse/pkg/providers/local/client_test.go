package local

import (
	"context"
	"strings"
	"testing"

	coremodel "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	localocr "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestNormalizeMode(t *testing.T) {
	if got := normalizeMode("strict"); got != "strict" {
		t.Fatalf("got=%q", got)
	}
	if got := normalizeMode("anything"); got != "relaxed" {
		t.Fatalf("got=%q", got)
	}
}

func TestRecoverablePDFStructureErrors(t *testing.T) {
	for _, msg := range []string{
		"missing page tree object: 12",
		"unsupported xref section at offset 848577",
		"invalid xref subsection header: \"0 0\"",
		"strict mode: trailer /Size is required",
	} {
		if !isRecoverablePDFStructureError(assertErr(msg)) {
			t.Fatalf("expected recoverable: %s", msg)
		}
	}
	if isRecoverablePDFStructureError(assertErr("permission denied")) {
		t.Fatal("unexpected recoverable error")
	}
}

func TestNeedsOCRFallback(t *testing.T) {
	cases := []struct {
		name string
		doc  *extractcommon.DocumentResult
		want bool
	}{
		{name: "nil", doc: nil, want: true},
		{name: "empty", doc: &extractcommon.DocumentResult{}, want: true},
		{name: "figure_nil", doc: &extractcommon.DocumentResult{Chunks: []extractcommon.Chunk{{Type: "figure", Text: "<nil>", Markdown: "<a id='x'></a>\n\n<nil>"}}}, want: true},
		{name: "figure_placeholder", doc: &extractcommon.DocumentResult{Chunks: []extractcommon.Chunk{{Type: "figure", Text: "Embedded image (jpeg, 231x310)", Markdown: "<a id='x'></a>\n\nEmbedded image (jpeg, 231x310)"}}}, want: true},
		{name: "figure_with_marginalia", doc: &extractcommon.DocumentResult{Chunks: []extractcommon.Chunk{{Type: "figure", Text: "Embedded image"}, {Type: "marginalia", Text: "Page 1 of 1"}}}, want: true},
		{name: "has_text", doc: &extractcommon.DocumentResult{Chunks: []extractcommon.Chunk{{Type: "text", Text: "hello"}}}, want: false},
		{name: "suspect_garbled", doc: &extractcommon.DocumentResult{Chunks: []extractcommon.Chunk{
			{Type: "text", Text: "EjQpIEkhjZIghqQjPgIjIjQgjINIGjPIGIpIYdZIjGYkEPNqIDGZDQYIddhNgGEjghGEYQQEhGgQpQOGQOQjYGdjQ"},
			{Type: "text", Text: "GkEjIGkhIggIhIgEPGkhIgQjIgpQIqhjkEpIgpYkDYIQhQOPjhjQNgZdgGkEjGIEQhQh"},
			{Type: "text", Text: "INQIIpIjhGDkQYGgIdgjhQEIDXYsjQEhYYDgjIqQjPjPIIhQOIgjGINQIkhIgNYqhGjIhjjPIdgjjsdIhjGIYQpIg"},
			{Type: "text", Text: "YYDgjIGqQjPjPIIGNGkEjjDkQYGjPIdgGkEjgGZdGIrIEkjIdgGkEjhjgjIOsjPjYQOhqQjP"},
			{Type: "text", Text: "hkDhEgQdjQGQdddkgEPhIEfkQgIGZjPYsEjQpIEkhjZIghqQjPQZjPhNjIgYkEPOIIgjQOZjPYs"},
		}}, want: true},
		{name: "vietnamese_text", doc: &extractcommon.DocumentResult{Chunks: []extractcommon.Chunk{
			{Type: "text", Text: "Tôi mong muốn trở thành một chuyên gia Marketing sáng tạo và có chiến lược, đóng góp vào sự phát triển bền vững của thương hiệu."},
			{Type: "text", Text: "Với kinh nghiệm làm việc tại ba công ty lớn, tôi tự tin rằng mình đã có một góc nhìn tổng quan về ngành sản phẩm."},
			{Type: "text", Text: "Thiết lập mạng lưới nhà phân phối, đại lý ở khu vực thông qua các hình thức quảng cáo và giới thiệu chương trình."},
			{Type: "text", Text: "Tôi hi vọng công ty mình sẽ cho tôi một cơ hội để chứng minh những kỹ năng và cống hiến lâu dài."},
			{Type: "text", Text: "HỌC VẤN ĐẠI HỌC THỦY LỢI chuyên ngành quản trị marketing và truyền thông thương hiệu."},
		}}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsOCRFallback(tc.doc); got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestRecoverablePDFStructureError(t *testing.T) {
	if !isRecoverablePDFStructureError(assertErr("build full document failed: missing page tree object: 12")) {
		t.Fatalf("missing page tree should be recoverable")
	}
	if isRecoverablePDFStructureError(assertErr("permission denied")) {
		t.Fatalf("unrelated errors should not be recoverable")
	}
}

func TestMakeFallbackPages(t *testing.T) {
	pages := makeFallbackPages(3)
	if len(pages) != 3 {
		t.Fatalf("pages=%d want=3", len(pages))
	}
	for i, page := range pages {
		if page.PageIndex != i {
			t.Fatalf("page %d index=%d", i, page.PageIndex)
		}
	}
}

func TestLocalOCRConcurrencyDefaultAndClamp(t *testing.T) {
	t.Setenv("DOCSTILL_LOCAL_OCR_CONCURRENCY", "")
	t.Setenv("LOCAL_OCR_CONCURRENCY", "")
	if got := localOCRConcurrency(); got < 1 || got > 2 {
		t.Fatalf("default concurrency=%d want 1..2", got)
	}
	t.Setenv("DOCSTILL_LOCAL_OCR_CONCURRENCY", "99")
	if got := localOCRConcurrency(); got != 8 {
		t.Fatalf("clamped concurrency=%d want=8", got)
	}
	t.Setenv("DOCSTILL_LOCAL_OCR_CONCURRENCY", "4")
	if got := localOCRConcurrency(); got != 4 {
		t.Fatalf("configured concurrency=%d want=4", got)
	}
}

func TestParseBytes_TextFileProducesChunks(t *testing.T) {
	c := New()
	doc, err := c.ParseBytes(context.Background(), "notes.txt", []byte("alpha\n\nbeta\nline2"), extractcommon.ParseOptions{})
	if err != nil {
		t.Fatalf("parse txt: %v", err)
	}
	if doc.Source != "local:light" {
		t.Fatalf("source=%q", doc.Source)
	}
	if len(doc.Chunks) < 2 {
		t.Fatalf("chunks=%d want>=2", len(doc.Chunks))
	}
	if strings.TrimSpace(doc.Markdown) == "" {
		t.Fatalf("markdown should not be empty")
	}
	if doc.Chunks[0].Type != "text" {
		t.Fatalf("chunk0 type=%q", doc.Chunks[0].Type)
	}
}

func TestDocumentToResultCarriesBlockGeometry(t *testing.T) {
	c := New()
	doc := c.documentToResult(&coremodel.Document{
		PageCount: 1,
		Sections: []coremodel.Section{{
			Path: "root",
			Blocks: []coremodel.Block{{
				Type:      "text",
				Text:      "Customer service",
				Page:      0,
				Order:     1,
				BBox:      &coremodel.BBox{Left: 0.1, Top: 0.2, Right: 0.5, Bottom: 0.3},
				Precision: "reliable",
				Payload: map[string]any{
					"bbox_source": "ocr_line",
				},
			}},
		}},
	}, "scan.jpg")
	if len(doc.Chunks) != 1 {
		t.Fatalf("chunks=%d want 1", len(doc.Chunks))
	}
	ch := doc.Chunks[0]
	if ch.BBox == nil {
		t.Fatal("expected chunk bbox")
	}
	if ch.BBox.Left != 0.1 || ch.BBox.Top != 0.2 || ch.BBox.Right != 0.5 || ch.BBox.Bottom != 0.3 {
		t.Fatalf("bbox=%+v", ch.BBox)
	}
	if ch.Precision != "reliable" {
		t.Fatalf("precision=%q want reliable", ch.Precision)
	}
	if ch.Payload["bbox_source"] != "ocr_line" {
		t.Fatalf("payload=%v", ch.Payload)
	}
	if doc.ExtractOutput != nil {
		t.Fatal("documentToResult should leave structured enrichment to caller")
	}
}

func assertErr(msg string) error {
	return errString(msg)
}

type errString string

func (e errString) Error() string {
	return string(e)
}

func TestLocalImageVLMEnabledDefaultsDisabled(t *testing.T) {
	t.Setenv("LOCAL_IMAGE_VLM", "")
	if localImageVLMEnabled() {
		t.Fatalf("LOCAL_IMAGE_VLM default should keep local image parsing OCR-first")
	}
}

func TestLocalOCRConfigForFileWithOptionsOverridesEngine(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "tesseract")
	t.Setenv("DOCSTILL_OCR_LANG", "")
	t.Setenv("LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_LOCAL_OCR_LANG", "")

	cfg := localOCRConfigForFileWithOptions("中文材料.pdf", extractcommon.ParseOptions{OCREngine: "paddleocr"})
	if cfg.EngineName() != localocr.EnginePaddleOCR {
		t.Fatalf("engine=%q want paddleocr", cfg.EngineName())
	}
	if cfg.Lang != "ch" {
		t.Fatalf("lang=%q want ch", cfg.Lang)
	}
}

func TestLocalOCRConfigForChineseTesseractUsesChineseFirst(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "tesseract")
	t.Setenv("DOCSTILL_OCR_LANG", "")
	t.Setenv("LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_TESSERACT_PSM", "")
	t.Setenv("DOCSTILL_OCR_TESSERACT_PSM", "")

	cfg := localOCRConfigForFileWithOptions("全运村合同续签.pdf", extractcommon.ParseOptions{})
	if cfg.EngineName() != localocr.EngineTesseract {
		t.Fatalf("engine=%q want tesseract", cfg.EngineName())
	}
	if cfg.Lang != "chi_sim" {
		t.Fatalf("lang=%q want chi_sim", cfg.Lang)
	}
	if cfg.TesseractPSM != 11 {
		t.Fatalf("psm=%d want 11", cfg.TesseractPSM)
	}
}

func TestLocalOCRConfigPreservesExplicitTesseractPSM(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "tesseract")
	t.Setenv("DOCSTILL_OCR_LANG", "")
	t.Setenv("LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_TESSERACT_PSM", "6")

	cfg := localOCRConfigForFileWithOptions("全运村合同续签.pdf", extractcommon.ParseOptions{})
	if cfg.TesseractPSM != 6 {
		t.Fatalf("psm=%d want explicit 6", cfg.TesseractPSM)
	}
}

func TestNormalizeLocalOCRTextCollapsesCJKSpaces(t *testing.T) {
	got := normalizeLocalOCRText("甲 方 与 乙 方 于 2024 年 7 月 1 日")
	want := "甲方与乙方于 2024 年 7 月 1 日"
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestLocalImageVLMEnabledCanBeDisabled(t *testing.T) {
	t.Setenv("LOCAL_IMAGE_VLM", "0")
	if localImageVLMEnabled() {
		t.Fatalf("LOCAL_IMAGE_VLM=0 should disable image VLM")
	}
}

func TestLocalImageVLMEnabledCanBeForced(t *testing.T) {
	t.Setenv("LOCAL_IMAGE_VLM", "force")
	if !localImageVLMEnabled() {
		t.Fatalf("LOCAL_IMAGE_VLM=force should enable image VLM")
	}
}

func TestSplitOCRTextBlocks_Paragraphs(t *testing.T) {
	raw := "第一段第一行\n第一段第二行\n\n第二段\n\n第三段"
	blocks := splitOCRTextBlocks(raw)
	if len(blocks) != 3 {
		t.Fatalf("blocks=%d want=3 (%v)", len(blocks), blocks)
	}
	if blocks[0] != "第一段第一行 第一段第二行" {
		t.Fatalf("block0=%q", blocks[0])
	}
}

func TestSplitOCRTextBlocks_LineFallback(t *testing.T) {
	raw := "A\nB line two\nC line three"
	blocks := splitOCRTextBlocks(raw)
	if len(blocks) < 2 {
		t.Fatalf("blocks=%d want>=2 (%v)", len(blocks), blocks)
	}
}

func TestOCRLinesToTextBlocksKeepsNormalizedBBox(t *testing.T) {
	blocks := ocrLinesToTextBlocks([]localocr.Line{
		{Text: "Customer service", Left: 20, Top: 10, Right: 120, Bottom: 30},
	}, 200, 100)
	if len(blocks) != 1 {
		t.Fatalf("blocks=%d want 1", len(blocks))
	}
	if blocks[0].Text != "Customer service" {
		t.Fatalf("text=%q", blocks[0].Text)
	}
	if blocks[0].BBox == nil {
		t.Fatal("expected normalized bbox")
	}
	if blocks[0].BBox.Left != 0.1 || blocks[0].BBox.Top != 0.1 || blocks[0].BBox.Right != 0.6 || blocks[0].BBox.Bottom != 0.3 {
		t.Fatalf("bbox=%+v", blocks[0].BBox)
	}
	if blocks[0].BBoxSource != "ocr_line" {
		t.Fatalf("bbox source=%q", blocks[0].BBoxSource)
	}
}

func TestOCRTextBlocksFromTextHasNoBBox(t *testing.T) {
	blocks := ocrTextBlocksFromText("Line one\n\nLine two")
	if len(blocks) != 2 {
		t.Fatalf("blocks=%d want 2", len(blocks))
	}
	if blocks[0].BBox != nil || blocks[0].BBoxSource != "ocr_text" {
		t.Fatalf("unexpected block metadata: %+v", blocks[0])
	}
}

func TestOCRResultHasLanguageLoadError(t *testing.T) {
	res := localocr.Result{Raw: "Error opening data file /opt/homebrew/share/tessdata/chi_sim.traineddata\nFailed loading language 'chi_sim'"}
	if !ocrResultHasLanguageLoadError(res) {
		t.Fatal("expected language-load error")
	}
	if ocrResultHasLanguageLoadError(localocr.Result{Text: "normal OCR text"}) {
		t.Fatal("did not expect language-load error")
	}
}

func TestLocalOCRConfigForFileUsesPaddleLangDefaults(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "paddleocr")
	t.Setenv("DOCSTILL_OCR_LANG", "")
	t.Setenv("LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_LOCAL_OCR_LANG", "")

	if got := localOCRConfigForFile("中文材料.pdf").Lang; got != "ch" {
		t.Fatalf("chinese paddle lang=%q want ch", got)
	}
	if got := localOCRConfigForFile("statement.pdf").Lang; got != "en" {
		t.Fatalf("english paddle lang=%q want en", got)
	}
}

func TestLocalOCRConfigForFileKeepsExplicitLang(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "paddleocr")
	t.Setenv("DOCSTILL_OCR_LANG", "fr")

	if got := localOCRConfigForFile("中文材料.pdf").Lang; got != "fr" {
		t.Fatalf("explicit lang=%q want fr", got)
	}
}

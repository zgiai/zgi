package mineru

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestMineruToDocumentResult_Shape(t *testing.T) {
	resp := &parseResponse{
		TaskID:  "task-1",
		Backend: "pipeline",
		Results: map[string]fileResults{
			"sample": {
				MdContent:   "# Title",
				ContentList: `[{"type":"text","text_level":2,"text":"Title","bbox":[100,200,300,800],"page_idx":0},{"type":"table","table_body":"| A |\n|---|\n| 1 |","table_caption":["Table 1"],"bbox":[10,20,500,600],"page_idx":0}]`,
				MiddleJSON:  `{"pdf_info":[{"page_idx":0,"page_size":[615,870],"preproc_blocks":[{"score":0.5058,"bbox":[100,200,300,800],"index":1,"type":"title"},{"score":0.93,"bbox":[10,20,500,600],"index":2,"type":"table"}]}]}`,
			},
		},
	}

	doc, err := mineruToDocumentResult("sample.pdf", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.FileName != "sample.pdf" {
		t.Fatalf("filename mismatch: %q", doc.FileName)
	}
	if doc.PageCount != 1 || len(doc.Pages) != 1 {
		t.Fatalf("page shape mismatch: page_count=%d pages=%d", doc.PageCount, len(doc.Pages))
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(doc.Chunks))
	}
	ch := doc.Chunks[0]
	if ch.Type != "heading" || ch.Subtype != "h2" {
		t.Fatalf("type mapping mismatch: type=%q subtype=%q", ch.Type, ch.Subtype)
	}
	if ch.BBox == nil {
		t.Fatalf("expected bbox for mineru chunk")
	}
	if ch.Precision != "reliable" {
		t.Fatalf("expected reliable precision, got %q", ch.Precision)
	}
	if ch.Confidence != 0.5058 {
		t.Fatalf("expected mineru block score as confidence, got %f", ch.Confidence)
	}
	if ch.Payload["mineru_block_score"] != 0.5058 {
		t.Fatalf("expected mineru block score in payload: %#v", ch.Payload)
	}
	if ch.Payload["mineru_type"] != "text" || ch.Payload["reading_order"] != 1 {
		t.Fatalf("mineru payload mismatch: %#v", ch.Payload)
	}

	table := doc.Chunks[1]
	if table.Type != "table" || table.Markdown == "" || table.Text == "[table]" {
		t.Fatalf("table structure not preserved: %+v", table)
	}
	if _, ok := table.Payload["table_caption"]; !ok {
		t.Fatalf("expected table caption in payload: %#v", table.Payload)
	}
	if table.Confidence != 0.93 {
		t.Fatalf("expected table confidence, got %f", table.Confidence)
	}
	enriched := extractcommon.EnrichStructuredOutput(doc)
	if enriched.ExtractOutput == nil || len(enriched.ExtractOutput.Elements) != 2 {
		t.Fatalf("missing enriched elements: %+v", enriched.ExtractOutput)
	}
	if enriched.ExtractOutput.Elements[0].Metadata["confidence"] != 0.5058 {
		t.Fatalf("expected confidence metadata: %#v", enriched.ExtractOutput.Elements[0].Metadata)
	}
	diag, _ := doc.Diagnostics["mineru_structure"].(map[string]any)
	if diag == nil || diag["content_items"] != 2 {
		t.Fatalf("expected mineru structure diagnostics, got %#v", doc.Diagnostics)
	}
}

func TestMineruToDocumentResult_ImageAssetsAndChartFigure(t *testing.T) {
	resp := &parseResponse{
		TaskID:  "task-images",
		Backend: "pipeline",
		Results: map[string]fileResults{
			"sample": {
				MdContent:   "![](images/chart.jpg)",
				ContentList: `[{"type":"chart","img_path":"images/chart.jpg","chart_caption":["Trend chart"],"bbox":[10,20,500,600],"page_idx":0}]`,
				Images:      map[string]string{"chart.jpg": "data:image/jpeg;base64,aGVsbG8="},
			},
		},
	}

	doc, err := mineruToDocumentResult("sample.pdf", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.ImageAssets) != 0 {
		t.Fatalf("sidecar mineru image assets should be bound to chunks, got %#v", doc.ImageAssets)
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(doc.Chunks))
	}
	chunk := doc.Chunks[0]
	if chunk.Type != "figure" {
		t.Fatalf("expected chart image to map to figure, got %q", chunk.Type)
	}
	if chunk.Markdown != "![Trend chart](data:image/jpeg;base64,aGVsbG8=)" {
		t.Fatalf("unexpected figure markdown: %q", chunk.Markdown)
	}
	if chunk.Payload["original_img_path"] != "images/chart.jpg" {
		t.Fatalf("expected original image path in payload, got %#v", chunk.Payload)
	}
	if chunk.Payload["img_path"] != "data:image/jpeg;base64,aGVsbG8=" || chunk.Payload["image_data_uri"] != "data:image/jpeg;base64,aGVsbG8=" {
		t.Fatalf("expected bound data URI in payload, got %#v", chunk.Payload)
	}
	if chunk.Payload["chart_caption"] == nil {
		t.Fatalf("expected chart caption payload, got %#v", chunk.Payload)
	}
}

func TestMineruToDocumentResult_OfficialBindsImageAssetsToChunks(t *testing.T) {
	resp := &parseResponse{
		TaskID:  "task-images",
		Backend: "official:vlm",
		Results: map[string]fileResults{
			"sample": {
				MdContent:   "![](images/chart.jpg)",
				ContentList: `[{"type":"chart","img_path":"images/chart.jpg","chart_caption":["Trend chart"],"bbox":[10,20,500,600],"page_idx":0}]`,
				Images:      map[string]string{"sample/images/chart.jpg": "data:image/jpeg;base64,aGVsbG8=", "images/chart.jpg": "data:image/jpeg;base64,aGVsbG8=", "chart.jpg": "data:image/jpeg;base64,aGVsbG8="},
			},
		},
	}

	doc, err := mineruToDocumentResult("sample.pdf", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.ImageAssets) != 0 {
		t.Fatalf("official mineru image assets should be bound to chunks, got %#v", doc.ImageAssets)
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(doc.Chunks))
	}
	chunk := doc.Chunks[0]
	if chunk.Markdown != "![Trend chart](data:image/jpeg;base64,aGVsbG8=)" {
		t.Fatalf("unexpected figure markdown: %q", chunk.Markdown)
	}
	if chunk.Payload["original_img_path"] != "images/chart.jpg" {
		t.Fatalf("expected original image path in payload, got %#v", chunk.Payload)
	}
	if chunk.Payload["img_path"] != "data:image/jpeg;base64,aGVsbG8=" || chunk.Payload["image_data_uri"] != "data:image/jpeg;base64,aGVsbG8=" {
		t.Fatalf("expected bound data URI in official payload, got %#v", chunk.Payload)
	}
}

func TestMineruToDocumentResult_TableWithImagePathUsesTableContent(t *testing.T) {
	resp := &parseResponse{
		TaskID:  "task-table-image",
		Backend: "pipeline",
		Results: map[string]fileResults{
			"sample": {
				MdContent:   "![table](images/table.png)",
				ContentList: `[{"type":"table","table_body":"| A |\n|---|\n| 1 |","img_path":"images/table.png","table_caption":["Amount table"],"bbox":[10,20,500,600],"page_idx":0}]`,
			},
		},
	}

	doc, err := mineruToDocumentResult("sample.pdf", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(doc.Chunks))
	}
	chunk := doc.Chunks[0]
	if chunk.Type != "table" {
		t.Fatalf("expected table image to map to table, got %q", chunk.Type)
	}
	if chunk.Text != "| A |\n|---|\n| 1 |" {
		t.Fatalf("expected table text, got %q", chunk.Text)
	}
	if chunk.Markdown != "| A |\n|---|\n| 1 |" {
		t.Fatalf("unexpected table markdown: %q", chunk.Markdown)
	}
	if chunk.Payload["table_body"] == "" {
		t.Fatalf("expected original table body in payload for diagnostics, got %#v", chunk.Payload)
	}
	extractcommon.EnrichStructuredOutput(doc)
	if doc.ExtractOutput == nil || len(doc.ExtractOutput.Elements) != 1 {
		t.Fatalf("expected structured figure output, got %#v", doc.ExtractOutput)
	}
	element := doc.ExtractOutput.Elements[0]
	if element.Type != "table" || element.Content != "| A |\n|---|\n| 1 |" {
		t.Fatalf("expected table element with table text, got %+v", element)
	}
	if element.Metadata["markdown"] != "| A |\n|---|\n| 1 |" {
		t.Fatalf("expected table markdown metadata, got %#v", element.Metadata)
	}
}

func TestMineruToDocumentResult_TableHTMLImageSourcesBindToDataURI(t *testing.T) {
	resp := &parseResponse{
		TaskID:  "task-table-html-images",
		Backend: "pipeline",
		Results: map[string]fileResults{
			"sample": {
				ContentList: `[{"type":"table","table_body":"<table><tr><td><p><img src=\"images/a.png\"/></p></td><td><p><img src='images/b.png'/></p></td></tr></table>","bbox":[10,20,500,600],"page_idx":0}]`,
				Images: map[string]string{
					"a.png": "data:image/png;base64,YQ==",
					"b.png": "data:image/png;base64,Yg==",
				},
			},
		},
	}

	doc, err := mineruToDocumentResult("sample.pdf", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(doc.Chunks))
	}
	chunk := doc.Chunks[0]
	if chunk.Type != "table" {
		t.Fatalf("expected table element, got %q", chunk.Type)
	}
	if strings.Contains(chunk.Markdown, "images/a.png") || strings.Contains(chunk.Markdown, "images/b.png") {
		t.Fatalf("table markdown still contains mineru image paths: %q", chunk.Markdown)
	}
	if !strings.Contains(chunk.Markdown, `src="data:image/png;base64,YQ=="`) || !strings.Contains(chunk.Markdown, `src='data:image/png;base64,Yg=='`) {
		t.Fatalf("table markdown did not bind image data URIs: %q", chunk.Markdown)
	}
	if chunk.Payload["table_body"] != chunk.Markdown {
		t.Fatalf("payload table_body was not rewritten: %#v", chunk.Payload)
	}
}

func TestOfficialReadZipArtifacts(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"sample/full.md":                     "# Title",
		"sample/sample_content_list.json":    `[{"type":"text","text":"Title","bbox":[0,0,1000,100],"page_idx":0}]`,
		"sample/sample_middle.json":          `{"pdf_info":[{"page_idx":0,"page_size":[1000,1000]}]}`,
		"sample/sample_content_list_v2.json": `[[{"type":"title"}]]`,
		"sample/images/chart.jpg":            "hello",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	artifacts, err := officialReadZipArtifacts(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if artifacts.Markdown != "# Title" {
		t.Fatalf("markdown mismatch: %q", artifacts.Markdown)
	}
	if artifacts.ContentList == "" || artifacts.ContentListPath != "sample/sample_content_list.json" {
		t.Fatalf("content list mismatch: path=%q body=%q", artifacts.ContentListPath, artifacts.ContentList)
	}
	if artifacts.MiddleJSON == "" || artifacts.MiddleJSONPath != "sample/sample_middle.json" {
		t.Fatalf("middle json mismatch: path=%q body=%q", artifacts.MiddleJSONPath, artifacts.MiddleJSON)
	}
	wantImage := "data:image/jpeg;base64,aGVsbG8="
	for _, name := range []string{"sample/images/chart.jpg", "images/chart.jpg", "chart.jpg"} {
		if artifacts.Images[name] != wantImage {
			t.Fatalf("image asset %q mismatch: %q", name, artifacts.Images[name])
		}
	}
}

func TestDecodeContentItemsV2(t *testing.T) {
	raw := `[[{"type":"title","content":{"level":1,"title_content":[{"type":"text","content":"Hello"}]},"bbox":[0,0,100,100]},{"type":"paragraph","content":{"paragraph_content":[{"type":"text","content":"World"}]},"bbox":[0,100,100,200]},{"type":"table","content":{"table_content":[{"type":"text","content":"| A |\n|---|"}]},"bbox":[0,200,100,300]}]]`

	items, err := decodeContentItems(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	if items[0].Type != "text" || items[0].Text != "Hello" || items[0].TextLevel != 1 || items[0].PageIdx != 0 {
		t.Fatalf("title mismatch: %+v", items[0])
	}
	if items[1].Type != "text" || items[1].Text != "World" {
		t.Fatalf("paragraph mismatch: %+v", items[1])
	}
	if items[2].Type != "table" || items[2].TableBody == "" {
		t.Fatalf("table mismatch: %+v", items[2])
	}
}

func TestSyntheticContentListFromMarkdown(t *testing.T) {
	got := syntheticContentListFromMarkdown("# Title\n\nBody text")
	if got == "" {
		t.Fatalf("expected synthetic content list")
	}
	resp := &parseResponse{
		TaskID:  "task-markdown",
		Backend: "official:MinerU-HTML",
		Results: map[string]fileResults{
			"preview.html": {
				MdContent:   "# Title\n\nBody text",
				ContentList: got,
			},
		},
	}
	doc, err := mineruToDocumentResult("preview.html", resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(doc.Chunks))
	}
	if doc.Chunks[0].Type != "heading" || doc.Chunks[0].Text != "Title" {
		t.Fatalf("heading mismatch: %+v", doc.Chunks[0])
	}
	if doc.Chunks[1].Text != "Body text" {
		t.Fatalf("body mismatch: %+v", doc.Chunks[1])
	}
}

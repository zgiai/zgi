package local

import (
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestBuildPopplerRecoveryDocument(t *testing.T) {
	diag := map[string]any{"recognition_source": "native_recovered"}
	doc := buildPopplerRecoveryDocument("sample.pdf", []popplerXMLPage{{
		Number: 1,
		Width:  600,
		Height: 800,
		Lines: []popplerTextLine{
			{Text: "Title", Box: extractcommon.BBox{Left: 0.1, Top: 0.1, Right: 0.4, Bottom: 0.14}},
			{Text: "Body text", Box: extractcommon.BBox{Left: 0.1, Top: 0.2, Right: 0.6, Bottom: 0.22}},
		},
		Figs: []popplerImageItem{
			{Box: extractcommon.BBox{Left: 0.2, Top: 0.5, Right: 0.6, Bottom: 0.7}},
		},
	}}, diag)
	if doc.Source != "native+poppler:text" {
		t.Fatalf("source=%q", doc.Source)
	}
	if doc.PageCount != 1 || len(doc.Pages) != 1 {
		t.Fatalf("pages=%d/%d", doc.PageCount, len(doc.Pages))
	}
	if len(doc.Chunks) != 3 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Type != "heading" {
		t.Fatalf("first chunk type=%q", doc.Chunks[0].Type)
	}
	if doc.Chunks[2].Type != "figure" {
		t.Fatalf("figure chunk type=%q", doc.Chunks[2].Type)
	}
	if doc.Chunks[0].BBox == nil {
		t.Fatal("expected bbox")
	}
	if doc.Diagnostics["recognition_source"] != "native_recovered" {
		t.Fatalf("diagnostics not preserved: %+v", doc.Diagnostics)
	}
	if doc.Markdown == "" {
		t.Fatal("expected markdown")
	}
}

func TestMergePopplerRecoveryLines(t *testing.T) {
	lines := []popplerTextLine{
		{Text: "World", Box: extractcommon.BBox{Left: 0.15, Top: 0.1, Right: 0.22, Bottom: 0.12}},
		{Text: "Hello", Box: extractcommon.BBox{Left: 0.1, Top: 0.101, Right: 0.14, Bottom: 0.121}},
		{Text: "右栏", Box: extractcommon.BBox{Left: 0.7, Top: 0.102, Right: 0.78, Bottom: 0.122}},
	}
	merged := mergePopplerRecoveryLines(lines)
	if len(merged) != 2 {
		t.Fatalf("merged=%d want 2", len(merged))
	}
	if merged[0].Text != "Hello World" {
		t.Fatalf("merged[0]=%q", merged[0].Text)
	}
	if merged[1].Text != "右栏" {
		t.Fatalf("merged[1]=%q", merged[1].Text)
	}
}

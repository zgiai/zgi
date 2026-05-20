package text

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdapterParse_SplitsParagraphs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	body := "第一段第一行\n第一段第二行\n\n第二段\n\n第三段"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	doc, err := (Adapter{}).Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Format != "text" {
		t.Fatalf("format=%q", doc.Format)
	}
	if doc.PageCount != 1 {
		t.Fatalf("page_count=%d", doc.PageCount)
	}
	if len(doc.Sections) != 1 {
		t.Fatalf("sections=%d", len(doc.Sections))
	}
	blocks := doc.Sections[0].Blocks
	if len(blocks) != 3 {
		t.Fatalf("blocks=%d", len(blocks))
	}
	if blocks[0].Text != "第一段第一行\n第一段第二行" {
		t.Fatalf("unexpected block0=%q", blocks[0].Text)
	}
	if blocks[2].Order != 3 {
		t.Fatalf("order3=%d", blocks[2].Order)
	}
}

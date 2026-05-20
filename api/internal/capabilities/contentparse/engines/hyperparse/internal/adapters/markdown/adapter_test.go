package markdown

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAdapterParse_MarkdownToBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	body := "# Title\n\npara one\nline two\n\n## Sub\n\npara two"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	doc, err := (Adapter{}).Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Format != "markdown" {
		t.Fatalf("format=%q", doc.Format)
	}
	if doc.Title != "Title" {
		t.Fatalf("title=%q", doc.Title)
	}
	if len(doc.Sections) != 1 || len(doc.Sections[0].Blocks) < 3 {
		t.Fatalf("blocks too few: %d", len(doc.Sections[0].Blocks))
	}
}

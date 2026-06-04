package hyperparse

import (
	"strings"
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestMapResultToExtractOutput_SortByOrdinal(t *testing.T) {
	result := &extractcommon.DocumentResult{
		DocID:     "doc-1",
		FileName:  "sample.pdf",
		PageCount: 2,
		Source:    "mineru:pipeline",
		Chunks: []extractcommon.Chunk{
			{ID: "c2", Type: "text", Page: 1, Ordinal: 2, Text: "page1-b"},
			{ID: "c1", Type: "text", Page: 1, Ordinal: 1, Text: "page1-a"},
			{ID: "c3", Type: "heading", Page: 0, Ordinal: 3, Markdown: "# page0"},
		},
	}

	output := mapResultToExtractOutput(result, "/tmp/sample.pdf", "mineru")
	if output == nil {
		t.Fatal("expected output")
	}
	if len(output.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(output.Elements))
	}
	if output.Source != "hyperparse_sdk:mineru" {
		t.Fatalf("unexpected source: %q", output.Source)
	}
	if output.Elements[0].Content != "page1-a" || output.Elements[1].Content != "page1-b" {
		t.Fatalf("expected ordinal order, got %#v", output.Elements[:2])
	}
	if output.Elements[2].Type != "heading" {
		t.Fatalf("expected heading element, got %q", output.Elements[2].Type)
	}
	if output.Elements[0].Metadata["page"] != 1 {
		t.Fatalf("expected page metadata, got %v", output.Elements[0].Metadata["page"])
	}
	if output.Metadata["recognition_source"] != "hyperparse_sdk:mineru" {
		t.Fatalf("unexpected recognition source: %v", output.Metadata["recognition_source"])
	}
	if !strings.Contains(output.Markdown, "page1-a") || !strings.Contains(output.Markdown, "# page0") {
		t.Fatalf("expected generated markdown, got %q", output.Markdown)
	}
}

func TestMapResultToExtractOutput_FallbackToMarkdown(t *testing.T) {
	result := &extractcommon.DocumentResult{
		Markdown: "full markdown",
		Source:   "local:light",
	}

	output := mapResultToExtractOutput(result, "/tmp/a.pdf", "local")
	if output == nil {
		t.Fatal("expected output")
	}
	if output.Markdown != "full markdown" {
		t.Fatalf("unexpected markdown fallback: %q", output.Markdown)
	}
	if len(output.Elements) != 1 {
		t.Fatalf("expected 1 fallback element, got %d", len(output.Elements))
	}
	if output.Elements[0].Content != "full markdown" {
		t.Fatalf("unexpected fallback element content: %q", output.Elements[0].Content)
	}
	if output.Elements[0].Metadata["recognition_source"] != "hyperparse_sdk:local" {
		t.Fatalf("unexpected recognition source: %v", output.Elements[0].Metadata["recognition_source"])
	}
}

func TestMapResultToExtractOutput_PreservesImageMetadata(t *testing.T) {
	result := &extractcommon.DocumentResult{
		DocID:    "doc-1",
		FileName: "sample.pdf",
		Chunks: []extractcommon.Chunk{
			{
				ID:       "figure-1",
				Type:     "figure",
				Page:     0,
				Ordinal:  1,
				Markdown: "![figure](/console/api/files/mineru-images?key=mineru%2Fimages%2Fdoc-1%2Fimg.jpg)",
				Payload: map[string]any{
					"image_key":         "mineru/images/doc-1/img.jpg",
					"image_url":         "/console/api/files/mineru-images?key=mineru%2Fimages%2Fdoc-1%2Fimg.jpg",
					"original_img_path": "images/img.jpg",
				},
			},
		},
	}

	output := mapResultToExtractOutput(result, "/tmp/sample.pdf", "mineru")
	if len(output.Elements) != 1 {
		t.Fatalf("expected one element, got %d", len(output.Elements))
	}
	meta := output.Elements[0].Metadata
	if meta["image_key"] != "mineru/images/doc-1/img.jpg" {
		t.Fatalf("expected image_key metadata, got %#v", meta)
	}
	if meta["image_url"] == "" {
		t.Fatalf("expected image_url metadata, got %#v", meta)
	}
	if payload, ok := meta["payload"].(map[string]any); !ok || payload["original_img_path"] != "images/img.jpg" {
		t.Fatalf("expected payload metadata, got %#v", meta["payload"])
	}
}

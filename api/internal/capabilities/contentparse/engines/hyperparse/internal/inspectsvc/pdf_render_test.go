package inspectsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRenderPagesSequential(t *testing.T) {
	got, err := resolveRenderPages(4, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestResolveRenderPagesSparseSubset(t *testing.T) {
	got, err := resolveRenderPages(6, []int{5, 2, 2, 9})
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 5}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got=%v want=%v", got, want)
		}
	}
}

func TestResolveRenderPagesOutOfRange(t *testing.T) {
	if _, err := resolveRenderPages(3, []int{7, 9}); err == nil {
		t.Fatal("expected error")
	}
}

func TestPageListIsSequential(t *testing.T) {
	if !pageListIsSequential([]int{1, 2, 3}, 3) {
		t.Fatal("expected sequential pages to be treated as full render")
	}
	if pageListIsSequential([]int{1, 3}, 3) {
		t.Fatal("unexpected sparse page list treated as sequential")
	}
}

func TestImagePathToDataURLMissing(t *testing.T) {
	if _, err := imagePathToDataURL("does-not-exist.png"); err == nil {
		t.Fatal("expected missing image error")
	}
}

func TestRenderPDFPreviewPrefersPDFToCairo(t *testing.T) {
	if _, err := resolveRendererBinary("pdftocairo", "CONTENT_PARSE_PDFTOCAIRO_PATH"); err != nil {
		t.Skipf("pdftocairo unavailable: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "..", "..", "tests", "file_process", "hyperparse", "fixtures", "sample.pdf"))
	if err != nil {
		t.Fatal(err)
	}

	pages, engine, err := RenderPDFPreviewPagesToDataURLs(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if engine != "pdftocairo" {
		t.Fatalf("engine=%q want pdftocairo", engine)
	}
	if len(pages) != 1 {
		t.Fatalf("pages=%d want 1", len(pages))
	}
	if !strings.HasPrefix(pages[0], "data:image/") {
		t.Fatalf("page data url has unexpected prefix: %.32q", pages[0])
	}
}

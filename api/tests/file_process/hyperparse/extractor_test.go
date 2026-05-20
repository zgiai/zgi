package hyperparse_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	hyperparseextractor "github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor/hyperparse"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	return filepath.Join(filepath.Dir(file), "fixtures", name)
}

func TestHyperparseExtractor_TextFile(t *testing.T) {
	extractor := hyperparseextractor.NewHyperparseExtractor(fixturePath(t, "sample.txt"), "local")
	output, err := extractor.Extract(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if output == nil || len(output.Elements) == 0 {
		t.Fatalf("expected at least one extracted element")
	}
	if !strings.Contains(strings.ToLower(output.Markdown), "hello") {
		t.Fatalf("expected extracted text to contain fixture marker, got: %q", output.Markdown)
	}
	if output.Metadata["source"] == nil {
		t.Fatalf("expected metadata source to be present")
	}
}

func TestHyperparseExtractor_EmptyPDF(t *testing.T) {
	extractor := hyperparseextractor.NewHyperparseExtractor(fixturePath(t, "empty.pdf"), "local")
	_, err := extractor.Extract(context.Background())
	if err == nil {
		t.Fatalf("expected error for empty pdf")
	}
}

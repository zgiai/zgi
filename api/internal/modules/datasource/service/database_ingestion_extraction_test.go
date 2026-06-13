package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/prompt"
)

func TestFileIngestExtractionInfoIncludesSourceAndContentHash(t *testing.T) {
	info := fileIngestExtractionInfo(databaseIngestionExtractionResult{
		PrimaryStrategy: databaseIngestionStrategyContentParse,
		ActualStrategy:  databaseIngestionStrategyContentParse,
		FallbackReason:  databaseIngestionFallbackNone,
		SourceType:      databaseIngestionSourceContentParse,
	}, "recognized content")

	if info.SourceType != databaseIngestionSourceContentParse {
		t.Fatalf("source type = %q, want %q", info.SourceType, databaseIngestionSourceContentParse)
	}
	if info.ContentHash == "" {
		t.Fatal("content hash should be populated")
	}
	if info.PrimaryStrategy != databaseIngestionStrategyContentParse ||
		info.ActualStrategy != databaseIngestionStrategyContentParse ||
		info.FallbackReason != databaseIngestionFallbackNone {
		t.Fatalf("extraction info = %#v", info)
	}
}

func TestIsDataSourceTableNotFound(t *testing.T) {
	if !IsDataSourceTableNotFound(errDataSourceTableNotFound) {
		t.Fatal("sentinel error should be recognized")
	}
	wrapped := errors.New("another error")
	if IsDataSourceTableNotFound(wrapped) {
		t.Fatal("unrelated error should not be recognized")
	}
}

func TestDatasourceFileConversionPromptConstrainsDatesMoneyAndPartialRecords(t *testing.T) {
	tmpl, err := prompt.GetTemplate(prompt.DatasourceFileConversion)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	raw := tmpl.RawContent()
	for _, want := range []string{
		"Do not return Excel serial dates",
		"45160",
		"explicitly asks for an Excel serial date",
		"keep the original major currency unit",
		"never 3260000 cents/fen",
		"return at least one record with all target columns represented",
		"Use null or an empty string for missing fields",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("prompt template missing %q", want)
		}
	}
}

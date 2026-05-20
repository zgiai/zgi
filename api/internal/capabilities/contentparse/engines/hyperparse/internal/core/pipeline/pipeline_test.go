package pipeline

import (
	"errors"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/apperr"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	"testing"
)

type mockAdapter struct {
	format string
}

func (m mockAdapter) Format() string {
	return m.format
}

func (m mockAdapter) Parse(path string) (*model.Document, error) {
	_ = path
	return &model.Document{Format: m.format}, nil
}

func TestParseByExtension(t *testing.T) {
	r := NewRouter()
	r.RegisterAdapter(mockAdapter{format: "pdf"}, ".pdf")

	doc, err := r.Parse("sample.PDF")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if doc == nil || doc.Format != "pdf" {
		t.Fatalf("expected format pdf, got: %+v", doc)
	}
}

func TestParseWithForcedFormat(t *testing.T) {
	r := NewRouter()
	r.RegisterAdapter(mockAdapter{format: "markdown"}, ".md")

	doc, err := r.ParseWithFormat("anything.unknown", "MARKDOWN")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if doc == nil || doc.Format != "markdown" {
		t.Fatalf("expected format markdown, got: %+v", doc)
	}
}

func TestUnknownExtensionReturnsCode(t *testing.T) {
	r := NewRouter()
	r.RegisterAdapter(mockAdapter{format: "pdf"}, ".pdf")

	_, err := r.Parse("sample.xyz")
	if err == nil {
		t.Fatal("expected an error")
	}

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected apperr.Error, got: %T", err)
	}
	if appErr.Code != apperr.CodeUnknownFormat {
		t.Fatalf("expected code %s, got %s", apperr.CodeUnknownFormat, appErr.Code)
	}
}

func TestUnknownForcedFormatReturnsCode(t *testing.T) {
	r := NewRouter()
	r.RegisterAdapter(mockAdapter{format: "pdf"}, ".pdf")

	_, err := r.ParseWithFormat("sample.pdf", "docx")
	if err == nil {
		t.Fatal("expected an error")
	}

	var appErr *apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected apperr.Error, got: %T", err)
	}
	if appErr.Code != apperr.CodeNoAdapterRegistered {
		t.Fatalf("expected code %s, got %s", apperr.CodeNoAdapterRegistered, appErr.Code)
	}
}

package hyperparsesdk

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	"github.com/zgiai/ginext/internal/contracts"
)

func TestAdapterParsesPlainTextBytes(t *testing.T) {
	adapter := NewAdapter()

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.txt",
		Data:       []byte("hello from content parse foundation"),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.Status != contracts.ParseStatusSucceeded {
		t.Fatalf("Status = %q, want %q", artifact.Status, contracts.ParseStatusSucceeded)
	}
	if artifact.EngineUsed != contracts.ParseEngineLocal {
		t.Fatalf("EngineUsed = %q, want %q", artifact.EngineUsed, contracts.ParseEngineLocal)
	}
	if !strings.Contains(artifact.Text, "hello from content parse foundation") {
		t.Fatalf("Text = %q, want substring", artifact.Text)
	}
	if len(artifact.Elements) == 0 {
		t.Fatal("expected parsed elements")
	}
}

func TestMapDocumentResultCarriesConfidence(t *testing.T) {
	result := &extractcommon.DocumentResult{
		DocID:    "doc-1",
		Markdown: "hello",
		ExtractOutput: &extractcommon.ExtractOutput{
			Elements: []extractcommon.ExtractElement{
				{
					ID:      "el-1",
					Type:    "text",
					Page:    0,
					Content: "hello",
					Ordinal: 1,
					Metadata: map[string]any{
						"confidence": 0.73,
					},
				},
			},
			Metadata: map[string]any{},
		},
	}
	artifact := mapDocumentResult(contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.txt",
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
	}, extractcommon.EngineLocal, result)
	if len(artifact.Elements) != 1 {
		t.Fatalf("elements=%d", len(artifact.Elements))
	}
	if artifact.Elements[0].Confidence == nil || *artifact.Elements[0].Confidence != 0.73 {
		t.Fatalf("confidence=%v", artifact.Elements[0].Confidence)
	}
}

func TestMapDocumentResultMarksEmptyOutputDegraded(t *testing.T) {
	result := &extractcommon.DocumentResult{
		DocID:     "doc-empty",
		PageCount: 3,
		ExtractOutput: &extractcommon.ExtractOutput{
			Metadata: map[string]any{},
		},
	}
	artifact := mapDocumentResult(contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "scan.pdf",
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
	}, extractcommon.EngineLocal, result)
	if artifact.Status != contracts.ParseStatusDegraded {
		t.Fatalf("Status = %q, want %q", artifact.Status, contracts.ParseStatusDegraded)
	}
	if artifact.QualityLevel != contracts.ParseQualityDegraded {
		t.Fatalf("QualityLevel = %q, want %q", artifact.QualityLevel, contracts.ParseQualityDegraded)
	}
	if artifact.Diagnostics["empty_output"] == nil {
		t.Fatal("expected empty_output diagnostic")
	}
}

func TestReadConfidence(t *testing.T) {
	if got := readConfidence(map[string]any{"confidence": 0.73}); got == nil || *got != 0.73 {
		t.Fatalf("readConfidence() = %v", got)
	}
	if got := readConfidence(nil); got != nil {
		t.Fatalf("expected nil confidence, got %v", got)
	}
}

func TestAdapterParsesCSVBytes(t *testing.T) {
	adapter := NewAdapter()

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.csv",
		Data:       []byte("name,age\nalice,30\nbob,28\n"),
		Intent:     contracts.ParseIntentDatasetIndex,
		Profile:    contracts.ParseProfileDatasetIndex,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.Status != contracts.ParseStatusSucceeded {
		t.Fatalf("Status = %q, want %q", artifact.Status, contracts.ParseStatusSucceeded)
	}
	if !strings.Contains(artifact.Markdown, "alice,30") {
		t.Fatalf("Markdown = %q, want parsed csv content", artifact.Markdown)
	}
}

func TestAdapterParsesXLSXBytes(t *testing.T) {
	adapter := NewAdapter()

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.xlsx",
		Data: buildZipFixture(t, map[string]string{
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c><v>A1</v></c><c><v>B1</v></c></row></sheetData></worksheet>`,
			"xl/sharedStrings.xml":     `<sst><si><t>Revenue</t></si><si><t>42</t></si></sst>`,
		}),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.EngineUsed != contracts.ParseEngineLocal {
		t.Fatalf("EngineUsed = %q, want %q", artifact.EngineUsed, contracts.ParseEngineLocal)
	}
	if len(artifact.Elements) == 0 {
		t.Fatal("expected xlsx elements")
	}
}

func TestAdapterParsesPPTXBytes(t *testing.T) {
	adapter := NewAdapter()

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "deck.pptx",
		Data: buildZipFixture(t, map[string]string{
			"ppt/slides/slide1.xml": `<p:sld><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Quarterly Review</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`,
		}),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !strings.Contains(artifact.Markdown, "Quarterly Review") {
		t.Fatalf("Markdown = %q, want pptx text", artifact.Markdown)
	}
}

func TestAdapterParsesPDFFixture(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		if _, terr := exec.LookPath("tesseract"); terr != nil {
			t.Skip("pdf fixture requires pdftoppm or tesseract to be available")
		}
	}

	adapter := NewAdapter()
	data := mustReadFixture(t, "sample.pdf")

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.pdf",
		Data:       data,
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.EngineUsed != contracts.ParseEngineLocal {
		t.Fatalf("EngineUsed = %q, want %q", artifact.EngineUsed, contracts.ParseEngineLocal)
	}
	if len(artifact.Elements) == 0 {
		t.Fatal("expected pdf elements")
	}
}

func TestAdapterParsesImageBytes(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("image parsing requires tesseract in current local adapter path")
	}

	adapter := NewAdapter()
	data := buildPNGFixture(t)

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "fixture-image.png",
		Data:       data,
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.Status != contracts.ParseStatusSucceeded {
		t.Fatalf("Status = %q, want %q", artifact.Status, contracts.ParseStatusSucceeded)
	}
	if len(artifact.Elements) == 0 {
		t.Fatal("expected image elements")
	}
}

func TestAdapterParsesLegacyDocBytes(t *testing.T) {
	adapter := NewAdapter()

	artifact, err := adapter.Parse(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "legacy.doc",
		Data:       []byte("Project Plan 2026\x00\x00Budget Overview\x00\x00Delivery Milestones\x00"),
		Intent:     contracts.ParseIntentPreview,
		Profile:    contracts.ParseProfileFastPreview,
		EngineHint: contracts.ParseEngineLocal,
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if artifact.Status != contracts.ParseStatusSucceeded {
		t.Fatalf("Status = %q, want %q", artifact.Status, contracts.ParseStatusSucceeded)
	}
	if !strings.Contains(artifact.Markdown, "Project Plan 2026") {
		t.Fatalf("Markdown = %q, want legacy office text", artifact.Markdown)
	}
}

func TestAdapterHealthSurface(t *testing.T) {
	adapter := NewAdapter()

	health, err := adapter.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if health.Name != "hyperparse_sdk" {
		t.Fatalf("Name = %q", health.Name)
	}
	if !health.Available {
		t.Fatal("expected adapter to report available")
	}
}

func TestSupportsInputRef(t *testing.T) {
	if !supportsInputRef("reducto", contracts.ParseSourceTypeURL, "https://example.com/doc.pdf") {
		t.Fatal("expected reducto URL passthrough support")
	}
	if !supportsInputRef("reducto", contracts.ParseSourceTypeUploadFile, "reducto://abc.pdf") {
		t.Fatal("expected reducto upload-file passthrough support")
	}
	if supportsInputRef("local", contracts.ParseSourceTypeURL, "https://example.com/doc.pdf") {
		t.Fatal("did not expect local URL passthrough support")
	}
}

func mustReadFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	path := filepath.Join(filepath.Dir(file), "../../../../../tests/file_process/hyperparse/fixtures", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func buildZipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func buildPNGFixture(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.White)
		}
	}
	for y := 6; y < 18; y++ {
		for x := 10; x < 38; x++ {
			img.Set(x, y, color.Black)
		}
	}
	buf := bytes.NewBuffer(nil)
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

package hyperparse

import (
	"os"
	"testing"
)

func TestApplyEnvironForceVLM(t *testing.T) {
	t.Setenv("DOCSTILL_FORCE_VLM", "")
	tr := true
	var c Config
	c.VLM.ForceVLM = &tr
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_FORCE_VLM") != "1" {
		t.Fatalf("want DOCSTILL_FORCE_VLM=1, got %q", os.Getenv("DOCSTILL_FORCE_VLM"))
	}
}

func TestApplyEnvironForceVLMFalse(t *testing.T) {
	t.Setenv("DOCSTILL_FORCE_VLM", "")
	fa := false
	var c Config
	c.VLM.ForceVLM = &fa
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_FORCE_VLM") != "0" {
		t.Fatalf("want DOCSTILL_FORCE_VLM=0, got %q", os.Getenv("DOCSTILL_FORCE_VLM"))
	}
}

func TestApplyEnvironFullPageFallback(t *testing.T) {
	t.Setenv("DOCSTILL_VLM_FULL_PAGE_FALLBACK", "")
	tr := true
	var c Config
	c.VLM.FullPageFallback = &tr
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_VLM_FULL_PAGE_FALLBACK") != "1" {
		t.Fatalf("want DOCSTILL_VLM_FULL_PAGE_FALLBACK=1, got %q", os.Getenv("DOCSTILL_VLM_FULL_PAGE_FALLBACK"))
	}
}

func TestApplyEnvironFullPageFallbackFalse(t *testing.T) {
	t.Setenv("DOCSTILL_VLM_FULL_PAGE_FALLBACK", "")
	fa := false
	var c Config
	c.VLM.FullPageFallback = &fa
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_VLM_FULL_PAGE_FALLBACK") != "0" {
		t.Fatalf("want DOCSTILL_VLM_FULL_PAGE_FALLBACK=0, got %q", os.Getenv("DOCSTILL_VLM_FULL_PAGE_FALLBACK"))
	}
}

func TestApplyEnvironImageCaptionEnabled(t *testing.T) {
	t.Setenv("DOCSTILL_VLM_IMAGE_CAPTION", "")
	tr := true
	var c Config
	c.VLM.ImageCaptionEnabled = &tr
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_VLM_IMAGE_CAPTION") != "1" {
		t.Fatalf("want DOCSTILL_VLM_IMAGE_CAPTION=1, got %q", os.Getenv("DOCSTILL_VLM_IMAGE_CAPTION"))
	}
}

func TestApplyEnvironImageCaptionEnabledFalse(t *testing.T) {
	t.Setenv("DOCSTILL_VLM_IMAGE_CAPTION", "")
	fa := false
	var c Config
	c.VLM.ImageCaptionEnabled = &fa
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_VLM_IMAGE_CAPTION") != "0" {
		t.Fatalf("want DOCSTILL_VLM_IMAGE_CAPTION=0, got %q", os.Getenv("DOCSTILL_VLM_IMAGE_CAPTION"))
	}
}

func TestApplyEnvironOCR(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "")
	t.Setenv("DOCSTILL_OCR_LANG", "")
	t.Setenv("DOCSTILL_OCR_TIMEOUT_SECONDS", "")
	t.Setenv("DOCSTILL_LOCAL_OCR_CONCURRENCY", "")
	t.Setenv("DOCSTILL_PADDLEOCR_ARGS", "")

	c := Config{
		OCR: OCRConfig{
			Engine:         "paddleocr",
			Lang:           "ch",
			TimeoutSeconds: 15,
			Concurrency:    4,
			PaddleArgs:     "--image_dir {image} --lang {lang}",
		},
	}
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_OCR_ENGINE") != "paddleocr" {
		t.Fatalf("want paddleocr, got %q", os.Getenv("DOCSTILL_OCR_ENGINE"))
	}
	if os.Getenv("DOCSTILL_OCR_LANG") != "ch" {
		t.Fatalf("want ch, got %q", os.Getenv("DOCSTILL_OCR_LANG"))
	}
	if os.Getenv("DOCSTILL_OCR_TIMEOUT_SECONDS") != "15" {
		t.Fatalf("want timeout 15, got %q", os.Getenv("DOCSTILL_OCR_TIMEOUT_SECONDS"))
	}
	if os.Getenv("DOCSTILL_LOCAL_OCR_CONCURRENCY") != "4" {
		t.Fatalf("want concurrency 4, got %q", os.Getenv("DOCSTILL_LOCAL_OCR_CONCURRENCY"))
	}
	if os.Getenv("DOCSTILL_PADDLEOCR_ARGS") == "" {
		t.Fatal("expected paddle args to be set")
	}
}

func TestApplyEnvironPDFRenderConcurrency(t *testing.T) {
	t.Setenv("DOCSTILL_PDF_RENDER_CONCURRENCY", "")
	c := Config{PDF: PDFRenderConfig{RenderConcurrency: 4}}
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_PDF_RENDER_CONCURRENCY") != "4" {
		t.Fatalf("want render concurrency 4, got %q", os.Getenv("DOCSTILL_PDF_RENDER_CONCURRENCY"))
	}
}

func TestApplyEnvironInspectDebugDumpDir(t *testing.T) {
	t.Setenv("DOCSTILL_UI_INSPECT_DEBUG_DUMP_DIR", "")
	var c Config
	c.UI.InspectDebugDumpDir = "D:/tmp/hyperparse-debug"
	c.ApplyEnviron()
	if os.Getenv("DOCSTILL_UI_INSPECT_DEBUG_DUMP_DIR") != "D:/tmp/hyperparse-debug" {
		t.Fatalf("want DOCSTILL_UI_INSPECT_DEBUG_DUMP_DIR set, got %q", os.Getenv("DOCSTILL_UI_INSPECT_DEBUG_DUMP_DIR"))
	}
}

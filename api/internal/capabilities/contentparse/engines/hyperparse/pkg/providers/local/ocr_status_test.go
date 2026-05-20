package local

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOCREngineStatusesAutoUsesAvailableFallback(t *testing.T) {
	tmpDir := t.TempDir()
	tesseractPath := filepath.Join(tmpDir, "tesseract")
	if err := os.WriteFile(tesseractPath, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatalf("write fake tesseract: %v", err)
	}

	t.Setenv("DOCSTILL_OCR_ENGINE", "paddleocr")
	t.Setenv("DOCSTILL_TESSERACT_PATH", tesseractPath)
	t.Setenv("DOCSTILL_PADDLEOCR_CMD", filepath.Join(tmpDir, "missing-paddleocr"))

	statuses := OCREngineStatuses()
	auto := findOCRStatus(t, statuses, "auto")
	if !auto.Available {
		t.Fatal("auto OCR should be available through tesseract fallback")
	}
	if auto.Provider != "tesseract" {
		t.Fatalf("auto provider=%q want tesseract", auto.Provider)
	}

	paddle := findOCRStatus(t, statuses, "paddleocr")
	if paddle.Available {
		t.Fatal("paddleocr should be unavailable when configured command is missing")
	}
}

func findOCRStatus(t *testing.T, statuses []OCREngineStatus, key string) OCREngineStatus {
	t.Helper()
	for _, status := range statuses {
		if status.Key == key {
			return status
		}
	}
	t.Fatalf("missing OCR status %q", key)
	return OCREngineStatus{}
}

package inspectsvc

import (
	"strings"
	"testing"
)

func TestPageHasRightSidebarCoverage(t *testing.T) {
	items := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 2,
			"text":       "Customer service\nPhone 1800 372 372",
			"bbox": map[string]any{
				"left":   0.72,
				"right":  0.92,
				"top":    0.18,
				"bottom": 0.46,
			},
		},
	}
	if !pageHasRightSidebarCoverage(items, 2) {
		t.Fatalf("pageHasRightSidebarCoverage=false, want true")
	}
	if pageHasRightSidebarCoverage(items, 1) {
		t.Fatalf("pageHasRightSidebarCoverage(page=1)=true, want false")
	}
}

func TestPageHasRightSidebarCoverageIgnoresLowerRightTable(t *testing.T) {
	items := []map[string]any{
		{
			"type":       "table",
			"page_index": 2,
			"text":       "Fuel type Natural Gas Renewables",
			"bbox": map[string]any{
				"left":   0.72,
				"right":  0.92,
				"top":    0.76,
				"bottom": 0.96,
			},
		},
	}
	if pageHasRightSidebarCoverage(items, 2) {
		t.Fatalf("lower-right footer table should not count as sidebar coverage")
	}
}

func TestPageHasSidebarRegionCoverageForPaymentBlock(t *testing.T) {
	items := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 2,
			"text":       "Payment options\nIBAN: IE67...\nBIC: DABAIE2D",
			"bbox": map[string]any{
				"left":   0.70,
				"right":  0.94,
				"top":    0.48,
				"bottom": 0.72,
			},
		},
	}
	region := normalizedCropRect{Left: 0.60, Top: 0.38, Right: 0.97, Bottom: 0.80}
	if !pageHasSidebarRegionCoverage(items, 2, region) {
		t.Fatalf("pageHasSidebarRegionCoverage=false, want true")
	}
}

func TestPageHasSidebarRegionCoverageDoesNotLetPaymentCoverFuelMix(t *testing.T) {
	items := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 2,
			"text":       "Payment options\nIBAN: IE67...\nInformation on Benefits of Switching",
			"bbox": map[string]any{
				"left":   0.62,
				"right":  0.97,
				"top":    0.38,
				"bottom": 0.78,
			},
		},
	}
	fuelRegion := normalizedCropRect{Left: 0.62, Top: 0.72, Right: 0.97, Bottom: 0.98}
	if pageHasSidebarRegionCoverage(items, 2, fuelRegion) {
		t.Fatalf("payment crop should not count as full fuel-mix coverage")
	}
}

func TestPageHasSidebarRegionCoverageRequiresMeaningfulOverlap(t *testing.T) {
	items := []map[string]any{
		{
			"type":       "paragraph",
			"page_index": 2,
			"text":       "Customer service",
			"bbox": map[string]any{
				"left":   0.70,
				"right":  0.92,
				"top":    0.03,
				"bottom": 0.12,
			},
		},
	}
	region := normalizedCropRect{Left: 0.60, Top: 0.10, Right: 0.97, Bottom: 0.45}
	if pageHasSidebarRegionCoverage(items, 2, region) {
		t.Fatalf("small boundary overlap should not count as full region coverage")
	}
}

func TestSidebarTextAddsNovelty(t *testing.T) {
	pageText := normalizeSidebarComparableText("Your electricity bill at a glance")
	if sidebarTextAddsNovelty("Customer service\nPhone 1800 372 372", pageText) != true {
		t.Fatalf("expected novel sidebar text")
	}
	if sidebarTextAddsNovelty("Your electricity bill at a glance", pageText) {
		t.Fatalf("expected duplicate text to be ignored")
	}
}

func TestSanitizeSidebarOCRTextDropsLowSignalLines(t *testing.T) {
	got := sanitizeSidebarOCRText("Q\nPage 2 of 2\nCustomer service\nPhone: 1800 372 372\n")
	want := "Customer service\nPhone: 1800 372 372"
	if got != want {
		t.Fatalf("sanitizeSidebarOCRText=%q, want %q", got, want)
	}
}

func TestBuildSidebarRecoveryChunksSplitsHeadingsAndNumericTable(t *testing.T) {
	region := sidebarRegionSpec{
		Name: "sidebar_right_column",
		Crop: normalizedCropRect{Left: 0.62, Top: 0.03, Right: 0.97, Bottom: 0.98},
	}
	text := "Customer service\nPhone: 1800 372 372\nPayment options\nYou can also call 1800 372 372\n" +
		"Electric Ireland fuel mix disclosure label\nCoal 0.8% 1.12%\nNatural Gas 31% 34.72%\nRenewables 67% 62.35%\nTotal 100% 100%"

	chunks := buildSidebarRecoveryChunks(text, region, 2, "ocr", "tesseract", 10, 0, 0.68)
	if len(chunks) < 3 {
		t.Fatalf("expected split sidebar chunks, got %+v", chunks)
	}
	foundTable := false
	for _, chunk := range chunks {
		if chunk["type"] == "table" {
			foundTable = true
			if !strings.Contains(chunk["text"].(string), "| Natural Gas | 31% | 34.72% |") {
				t.Fatalf("unexpected table text: %s", chunk["text"])
			}
		}
	}
	if !foundTable {
		t.Fatalf("expected table chunk, got %+v", chunks)
	}
}

func TestSidebarOCRLangForEngineUsesPaddleDefault(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_LANG", "")
	t.Setenv("LOCAL_SIDEBAR_OCR_LANG", "")
	t.Setenv("LOCAL_OCR_LANG", "")
	t.Setenv("DOCSTILL_LOCAL_OCR_LANG", "")

	if got := sidebarOCRLangForEngine("paddleocr"); got != "en" {
		t.Fatalf("paddle sidebar lang=%q want en", got)
	}
	if got := sidebarOCRLangForEngine("tesseract"); got != "eng" {
		t.Fatalf("tesseract sidebar lang=%q want eng", got)
	}
}

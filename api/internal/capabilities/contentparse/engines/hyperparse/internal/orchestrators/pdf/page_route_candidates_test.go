package pdf

import (
	"testing"

	pdfadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
)

func TestBuildPageRouteCandidates(t *testing.T) {
	lines := []pdfadapter.GeometryLine{
		{PageIndex: 2, Text: "Invoice Date: 2025-03-01", GeomX: 24},
		{PageIndex: 2, Text: "Account Number: 123456789012", GeomX: 24},
		{PageIndex: 2, Text: "Amount Due: $102.30", GeomX: 24},
		{PageIndex: 2, Text: "Payment Due: 2025-03-20", GeomX: 24},
		{PageIndex: 2, Text: "Name:", GeomX: 24},
		{PageIndex: 2, Text: "DOB:", GeomX: 24},
		{PageIndex: 2, Text: "Policy ID:", GeomX: 184},
		{PageIndex: 2, Text: "Plan:", GeomX: 184},
		{PageIndex: 2, Text: "Member ID:", GeomX: 328},
		{PageIndex: 2, Text: "Group:", GeomX: 328},
		{PageIndex: 2, Text: "Checked: ☑", GeomX: 24},
		{PageIndex: 2, Text: "Opt in: ☑", GeomX: 184},
	}
	tokens := []pdfadapter.GeometryToken{
		{PageIndex: 1, Text: "OK", GeomX: 12},
		{PageIndex: 2, Text: "Invoice", GeomX: 24},
	}
	images := []pdfadapter.ExtractedImageBytes{
		{ExtractedImage: pdfadapter.ExtractedImage{PageIndex: 1}},
	}

	candidates := buildPageRouteCandidates(2, lines, tokens, images, pdfadapter.BusinessDocVLMRouteHint{}, false)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates)=%d", len(candidates))
	}

	page1 := candidates[0]
	if got, _ := page1["recommended_mode"].(string); got != "vlm_candidate" {
		t.Fatalf("page1.recommended_mode=%q", got)
	}
	assertCandidateHasReason(t, page1, "scan_like")

	page2 := candidates[1]
	if got, _ := page2["recommended_mode"].(string); got != "vlm_candidate" {
		t.Fatalf("page2.recommended_mode=%q", got)
	}
	assertCandidateHasReason(t, page2, "business_form_like")
	assertCandidateHasReason(t, page2, "native_quality_low")
}

func TestBuildPageRouteCandidatesEscalatesFromDocumentBusinessHint(t *testing.T) {
	lines := []pdfadapter.GeometryLine{
		{PageIndex: 2, Text: "Balance brought forward", GeomX: 24},
		{PageIndex: 2, Text: "2025-03-01", GeomX: 24},
		{PageIndex: 2, Text: "$102.30", GeomX: 260},
		{PageIndex: 2, Text: "2025-03-07", GeomX: 24},
		{PageIndex: 2, Text: "$18.99", GeomX: 260},
		{PageIndex: 2, Text: "2025-03-10", GeomX: 24},
		{PageIndex: 2, Text: "$60.72", GeomX: 260},
		{PageIndex: 2, Text: "VAT", GeomX: 24},
		{PageIndex: 2, Text: "$15.03", GeomX: 260},
		{PageIndex: 2, Text: "Total due", GeomX: 24},
		{PageIndex: 2, Text: "$182.01", GeomX: 260},
	}
	tokens := []pdfadapter.GeometryToken{
		{PageIndex: 2, Text: "Balance", GeomX: 24},
		{PageIndex: 2, Text: "$102.30", GeomX: 260},
	}
	images := []pdfadapter.ExtractedImageBytes{
		{ExtractedImage: pdfadapter.ExtractedImage{PageIndex: 2}},
	}

	candidates := buildPageRouteCandidates(2, lines, tokens, images, pdfadapter.BusinessDocVLMRouteHint{
		Suggest: true,
		Kinds:   []string{"account_statement"},
		Reasons: []string{"text_account_statement"},
	}, false)
	if len(candidates) != 2 {
		t.Fatalf("len(candidates)=%d", len(candidates))
	}

	page2 := candidates[1]
	if got, _ := page2["recommended_mode"].(string); got != "vlm_candidate" {
		t.Fatalf("page2.recommended_mode=%q", got)
	}
	assertCandidateHasReason(t, page2, "business_form_like")
}

func assertCandidateHasReason(t *testing.T, candidate map[string]any, code string) {
	t.Helper()
	reasons, _ := candidate["reasons"].([]map[string]any)
	for _, reason := range reasons {
		if got, _ := reason["code"].(string); got == code {
			return
		}
	}
	t.Fatalf("candidate reasons missing %q: %+v", code, candidate["reasons"])
}

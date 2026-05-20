package pdf

import (
	"strings"
	"testing"
)

func TestBuildImageLikePDFHints_OnePageAnnotPDF_NotLikely(t *testing.T) {
	// buildOnePagePDFWithAnnot 来自 main 包测试不便复用：短内容流 + 小文件，不应判为 image_like。
	data := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 15 >>
stream
BT ET
endstream
endobj
trailer
<< /Size 5 /Root 1 0 R >>
startxref
0
%%EOF
`)
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) != 1 {
		t.Fatalf("specs: err=%v n=%d", err, len(specs))
	}
	segs := ExtractTextBasicSegmentsFromBytesWithSpecs(data, specs)
	h := BuildImageLikePDFHints(data, ValidationModeRelaxed, specs, segs)
	if h.Likely {
		t.Fatalf("small synthetic pdf should not be image_like: %+v", h)
	}
}

func TestBuildImageLikePDFHints_HugeFileThinStream_Likely(t *testing.T) {
	// 模拟：两页、内容流极短，但文件 padding 很大（类似扫描件体积）。
	core := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 4 >>
stream
q Q
endstream
endobj
5 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Contents 6 0 R >>
endobj
6 0 obj
<< /Length 4 >>
stream
q Q
endstream
endobj
trailer
<< /Size 7 /Root 1 0 R >>
startxref
0
%%EOF
`)
	padding := strings.Repeat("% ", 90000)
	data := append(core, []byte(padding)...)
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) != 2 {
		t.Fatalf("specs: err=%v n=%d", err, len(specs))
	}
	segs := ExtractTextBasicSegmentsFromBytesWithSpecs(data, specs)
	h := BuildImageLikePDFHints(data, ValidationModeRelaxed, specs, segs)
	if !h.Likely {
		t.Fatalf("expected image_like hints, got %+v", h)
	}
}

func TestBuildImageLikePDFHintsWithLayout_VectorFormLikely(t *testing.T) {
	data := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << >> /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 15 >>
stream
BT ET
endstream
endobj
trailer
<< /Size 5 /Root 1 0 R >>
startxref
0
%%EOF
`)
	data = append(data, []byte(strings.Repeat("% padding\n", 7000))...)
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) != 1 {
		t.Fatalf("specs: err=%v n=%d", err, len(specs))
	}
	geometryLines := []GeometryLine{
		{Text: "Account number"},
		{Text: "Billing period"},
		{Text: "Consumption type"},
		{Text: "Payments/Transactions"},
		{Text: "Balance brought forward"},
		{Text: "Charges for this period"},
		{Text: "Your Savings"},
		{Text: "VAT"},
		{Text: "Total due"},
		{Text: "Invoice number"},
		{Text: "Payment due date"},
		{Text: "Customer number"},
		{Text: "Meter number"},
		{Text: "Statement date"},
		{Text: "Amount due"},
		{Text: "MPRN"},
		{Text: "Opening balance"},
		{Text: "Closing balance"},
	}
	h := BuildImageLikePDFHintsWithLayout(data, ValidationModeRelaxed, specs, nil, geometryLines)
	if !h.Likely {
		t.Fatalf("expected geometry-driven image_like hints, got %+v", h)
	}
	if !stringSliceContains(h.Reasons, "dense_short_geometry_lines") {
		t.Fatalf("expected dense_short_geometry_lines, got %+v", h)
	}
}

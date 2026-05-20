package pdf

import (
	"math"
	"strings"
	"testing"
)

func TestParseSimpleFontWidths_FromDict(t *testing.T) {
	fontDict := `<< /Type /Font /Subtype /TrueType /BaseFont /TestFont
/FirstChar 65 /LastChar 67 /Widths [ 600 200 600 ]
>>`
	m := parseSimpleFontWidths(nil, fontDict, ValidationModeRelaxed)
	if m == nil || !m.ok() || m.kind != "simple" || m.firstChar != 65 || len(m.widths) != 3 {
		t.Fatalf("parse simple widths: %#v", m)
	}
	if m.widths[0] != 600 || m.widths[1] != 200 {
		t.Fatalf("widths: %#v", m.widths)
	}
}

func TestGeomHorizontalAdvanceUsesWidthsForLiteral(t *testing.T) {
	cm := cmapUnicodeMap{
		byCodeHex: map[string]string{"41": "A", "42": "B"},
		keyLens:   []int{1},
	}
	m := &pdfFontWidthModel{kind: "simple", firstChar: 65, widths: []float64{600, 200, 600}}
	raw := []byte{0x41, 0x42} // "AB"
	dec := "AB"
	advW := geomHorizontalAdvanceForTextShow(dec, raw, cm, m, 12, 0, 0, 100)
	advE := estimateGeomShowAdvance(dec, 12, 0, 0, 100)
	if advW <= 0 || math.Abs(advW-9.6) > 0.01 {
		t.Fatalf("width advance got %v want ~9.6", advW)
	}
	if advW >= advE-0.01 {
		t.Fatalf("expected width advance < estimate, got w=%v est=%v", advW, advE)
	}
}

func TestParseCIDWArray_RangeTriplet(t *testing.T) {
	inner := "10 [ 500 510 ] 12 14 400"
	m := parseCIDWArray(inner)
	if m[10] != 500 || m[11] != 510 {
		t.Fatalf("sequential block: %#v", m)
	}
	for cid := 12; cid <= 14; cid++ {
		if m[cid] != 400 {
			t.Fatalf("range cid=%d got %v", cid, m[cid])
		}
	}
}

func TestBuildPageFontWidthsMap_MinimalPDF(t *testing.T) {
	content := `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
/Resources << /Font << /F1 4 0 R >> >>
/Contents 5 0 R
>>
endobj
4 0 obj
<< /Type /Font /Subtype /TrueType /BaseFont /ZapfDingbats
/FirstChar 65 /LastChar 66 /Widths 6 0 R
>>
endobj
5 0 obj
<< /Length 52 >>
stream
BT /F1 12 Tf 1 0 0 1 100 700 Tm

(A) Tj (B) Tj ET
endstream
endobj
6 0 obj
[ 500 300 ]
endobj
xref
0 7
0000000000 65535 f 
trailer
<< /Size 7 /Root 1 0 R >>
startxref
0
%%EOF
`
	data := []byte(strings.ReplaceAll(content, "\n", "\r\n"))
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil || len(specs) == 0 {
		t.Fatalf("specs: %v len=%d", err, len(specs))
	}
	wm := buildPageFontWidthsMap(data, specs[0], ValidationModeRelaxed)
	f1 := wm["F1"]
	if f1 == nil || !f1.ok() || f1.kind != "simple" {
		t.Fatalf("F1 widths: %#v", f1)
	}
	if f1.firstChar != 65 || len(f1.widths) != 2 || f1.widths[0] != 500 {
		t.Fatalf("unexpected F1 model: %#v", f1)
	}
	runs := extractGeomTextRuns([]byte(`BT /F1 12 Tf 1 0 0 1 100 700 Tm (A) Tj (B) Tj ET`), nil, nil, wm)
	if len(runs) != 2 {
		t.Fatalf("runs: %#v", runs)
	}
	gap := runs[1].x - runs[0].x
	want := 500.0 / 1000 * 12
	if math.Abs(gap-want) > 0.02 {
		t.Fatalf("second x - first x got %v want ~%v (tm advance after A)", gap, want)
	}
}

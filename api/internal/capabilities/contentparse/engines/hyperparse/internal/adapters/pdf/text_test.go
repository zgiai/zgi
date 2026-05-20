package pdf

import (
	"bytes"
	golzw "compress/lzw"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTextBasicFromBytes(t *testing.T) {
	content := `%PDF-1.4
1 0 obj
<< /Length 48 >>
stream
BT
(Hello) Tj
(ZGI Parse) Tj
ET
endstream
endobj
xref
0 1
0000000000 65535 f
trailer
<< /Size 1 >>
startxref
0
%%EOF
`
	text := ExtractTextBasicFromBytes([]byte(content))
	if !strings.Contains(text, "Hello") {
		t.Fatalf("expected Hello in extracted text, got: %q", text)
	}
	if !strings.Contains(text, "ZGI Parse") {
		t.Fatalf("expected ZGI Parse in extracted text, got: %q", text)
	}
}

func TestExtractTextBasic_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.pdf")
	content := `%PDF-1.4
1 0 obj
<<>>
stream
(X) Tj
endstream
endobj
%%EOF
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	text, err := ExtractTextBasic(path)
	if err != nil {
		t.Fatalf("ExtractTextBasic error: %v", err)
	}
	if strings.TrimSpace(text) != "X" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestGeomTextAnchorUserSpace_TsMatchesManual(t *testing.T) {
	ctm := affIdentity()
	tm := affFromPDFMatrix(1, 0, 0, 1, 100, 200)
	x0, y0 := geomTextAnchorUserSpace(ctm, tm, 0)
	x5, y5 := geomTextAnchorUserSpace(ctm, tm, 5)
	if x0 != 100 || y0 != 200 || x5 != 100 || y5 != 205 {
		t.Fatalf("got (%v,%v) (%v,%v) want (100,200) (100,205)", x0, y0, x5, y5)
	}
}

func TestExtractGeomTextRuns_TwoTjAdvancesX(t *testing.T) {
	b := []byte(`BT 12 Tf 1 0 0 1 0 0 Tm (A) Tj (B) Tj ET`)
	r := extractGeomTextRuns(b, nil, nil, nil)
	if len(r) != 2 {
		t.Fatalf("want 2 runs got %d", len(r))
	}
	if r[1].x <= r[0].x {
		t.Fatalf("second run should be to the right: %#v", r)
	}
}

func TestExtractGeomTextRuns_TJArraySegmentsAndKern(t *testing.T) {
	b := []byte(`BT /F1 12 Tf 1 0 0 1 0 0 Tm [(Doc) 120 (Still)] TJ ET`)
	r := extractGeomTextRuns(b, nil, nil, nil)
	if len(r) != 2 {
		t.Fatalf("want 2 runs from TJ got %d", len(r))
	}
	if r[0].s != "Doc" || r[1].s != "Still" {
		t.Fatalf("unexpected fragments: %#v", r)
	}
	if r[1].x <= r[0].x {
		t.Fatalf("Still should be to the right of Doc: %#v", r)
	}
}

func TestExtractGeomTextRuns_TzDoublesHorizontalGap(t *testing.T) {
	base := []byte(`BT 12 Tf 1 0 0 1 0 0 Tm (A) Tj (B) Tj ET`)
	tz200 := []byte(`BT 12 Tf 200 Tz 1 0 0 1 0 0 Tm (A) Tj (B) Tj ET`)
	rb := extractGeomTextRuns(base, nil, nil, nil)
	rz := extractGeomTextRuns(tz200, nil, nil, nil)
	if len(rb) != 2 || len(rz) != 2 {
		t.Fatalf("runs base=%d tz=%d", len(rb), len(rz))
	}
	ga := rb[1].x - rb[0].x
	gb := rz[1].x - rz[0].x
	if ga <= 0 || gb <= 0 {
		t.Fatalf("bad gaps: ga=%v gb=%v", ga, gb)
	}
	if math.Abs(gb/ga-2) > 0.01 {
		t.Fatalf("Tz 200 gap ratio got %v want ~2 (ga=%v gb=%v)", gb/ga, ga, gb)
	}
}

func TestExtractGeomTextRuns_TcAddsPerGlyph(t *testing.T) {
	noTc := []byte(`BT 12 Tf 1 0 0 1 0 0 Tm (A) Tj (B) Tj ET`)
	withTc := []byte(`BT 12 Tf 4 Tc 1 0 0 1 0 0 Tm (A) Tj (B) Tj ET`)
	r0 := extractGeomTextRuns(noTc, nil, nil, nil)
	r1 := extractGeomTextRuns(withTc, nil, nil, nil)
	g0 := r0[1].x - r0[0].x
	g1 := r1[1].x - r1[0].x
	if g1-g0 < 3.9 || g1-g0 > 4.1 {
		t.Fatalf("Tc=4 adds ~4 to advance after one glyph: g0=%v g1=%v delta=%v", g0, g1, g1-g0)
	}
}

func TestExtractGeomTextRuns_TwAddsForSpace(t *testing.T) {
	noTw := []byte(`BT 12 Tf 1 0 0 1 0 0 Tm (A ) Tj (B) Tj ET`)
	wTw := []byte(`BT 12 Tf 100 Tw 1 0 0 1 0 0 Tm (A ) Tj (B) Tj ET`)
	r0 := extractGeomTextRuns(noTw, nil, nil, nil)
	r1 := extractGeomTextRuns(wTw, nil, nil, nil)
	g0 := r0[1].x - r0[0].x
	g1 := r1[1].x - r1[0].x
	if g1-g0 < 99 || g1-g0 > 101 {
		t.Fatalf("Tw=100 on space: g0=%v g1=%v delta=%v", g0, g1, g1-g0)
	}
}

func TestExtractGeomTextRuns_TsAndTL(t *testing.T) {
	withTs := []byte(`BT 1 0 0 1 100 200 Tm 5 Ts (Hi) Tj ET`)
	noTs := []byte(`BT 1 0 0 1 100 200 Tm (Hi) Tj ET`)
	rTs := extractGeomTextRuns(withTs, nil, nil, nil)
	rNo := extractGeomTextRuns(noTs, nil, nil, nil)
	if len(rTs) != 1 || len(rNo) != 1 {
		t.Fatalf("runs: ts=%d noTs=%d", len(rTs), len(rNo))
	}
	if rTs[0].y-rNo[0].y != 5 {
		t.Fatalf("Ts y delta got %v want 5 (ts=%v no=%v)", rTs[0].y-rNo[0].y, rTs[0].y, rNo[0].y)
	}
	tlStar := []byte(`BT 1 0 0 1 0 0 Tm 12 TL 0 -12 Td (A) Tj T* (B) Tj ET`)
	r2 := extractGeomTextRuns(tlStar, nil, nil, nil)
	if len(r2) != 2 {
		t.Fatalf("want 2 runs got %d", len(r2))
	}
	if r2[0].y-r2[1].y != 12 {
		t.Fatalf("leading line gap got %v want 12: %#v", r2[0].y-r2[1].y, r2)
	}
}

func TestExtractGeomTextRuns_TdStartsFromLineMatrixNotShownTextAdvance(t *testing.T) {
	b := []byte(`BT /F1 12 Tf 1 0 0 1 0 0 Tm (A) Tj 0 -12 Td (B) Tj ET`)
	r := extractGeomTextRuns(b, nil, nil, nil)
	if len(r) != 2 {
		t.Fatalf("want 2 runs got %d", len(r))
	}
	if r[1].y >= r[0].y {
		t.Fatalf("second run should move to next line: %#v", r)
	}
	if math.Abs(r[1].x-r[0].x) > 0.001 {
		t.Fatalf("Td should reset x from line matrix, got first=%v second=%v", r[0].x, r[1].x)
	}
}

func TestAssembleGeomTextRuns_InsertsSpaceWhenGapWide(t *testing.T) {
	runs := []geomTextRun{
		{s: "RIGHT", x: 100, y: 200, b: 1},
		{s: "ARM", x: 160, y: 200, b: 2},
	}
	got := assembleGeomTextRuns(runs, nil)
	if got != "RIGHT ARM" {
		t.Fatalf("got %q want RIGHT ARM", got)
	}
}

func TestAssembleGeomTextRuns_MixedCJKLatin_BlockMinXOrder(t *testing.T) {
	runs := []geomTextRun{
		{s: "使命", x: 80, y: 100, b: 1, fontSizePt: 12},
		{s: "AGIC", x: 10, y: 100, b: 2, fontSizePt: 12},
	}
	got := assembleGeomTextRuns(runs, nil)
	if got != "AGIC 使命" {
		t.Fatalf("got %q want %q", got, "AGIC 使命")
	}
}

func TestAssembleGeomTextRuns_YSortWithinStream(t *testing.T) {
	runs := []geomTextRun{
		{s: "Bottom", x: 10, y: 50, b: 20},
		{s: "Top", x: 10, y: 200, b: 5},
	}
	got := assembleGeomTextRuns(runs, nil)
	if got != "Top\nBottom" {
		t.Fatalf("got %q want Top\\nBottom", got)
	}
}

func TestAssembleGeomTextRuns_AttachSubscript(t *testing.T) {
	runs := []geomTextRun{
		{s: "T", x: 100, y: 200, b: 1, fontSizePt: 12, baseFont: "CambriaMath"},
		{s: "i", x: 103, y: 196, b: 2, fontSizePt: 7, baseFont: "CambriaMath"},
		{s: "=", x: 118, y: 200, b: 3, fontSizePt: 12, baseFont: "CambriaMath"},
		{s: "1", x: 126, y: 200, b: 4, fontSizePt: 12, baseFont: "CambriaMath"},
	}
	got := assembleGeomTextRuns(runs, nil)
	if got != "T_{i}=1" {
		t.Fatalf("got %q want T_{i}=1", got)
	}
}

func TestAssembleGeomTextRuns_AttachSuperscript(t *testing.T) {
	runs := []geomTextRun{
		{s: "x", x: 100, y: 200, b: 1, fontSizePt: 12, baseFont: "CambriaMath"},
		{s: "2", x: 103, y: 204, b: 2, fontSizePt: 7, baseFont: "CambriaMath"},
	}
	got := assembleGeomTextRuns(runs, nil)
	if got != "x^{2}" {
		t.Fatalf("got %q want x^{2}", got)
	}
}

func TestAssembleGeomTextRuns_AttachSubscript_WideHorizontalGap(t *testing.T) {
	runs := []geomTextRun{
		{s: "T", x: 100, y: 200, b: 1, fontSizePt: 12, baseFont: "CambriaMath"},
		{s: "i", x: 116, y: 196, b: 2, fontSizePt: 7, baseFont: "CambriaMath"},
	}
	got := assembleGeomTextRuns(runs, nil)
	if got != "T_{i}" {
		t.Fatalf("got %q want T_{i}", got)
	}
}

func TestExtractTextBasicSegmentsFromBytes(t *testing.T) {
	content := `%PDF-1.4
1 0 obj
<<>>
stream
(A) Tj
endstream
endobj
2 0 obj
<<>>
stream
[(B) 120 (C)] TJ
endstream
endobj
%%EOF
`
	segs := ExtractTextBasicSegmentsFromBytes([]byte(content))
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	if segs[0].StreamIndex != 0 || segs[0].Text != "A" {
		t.Fatalf("unexpected first segment: %+v", segs[0])
	}
	if segs[0].Order != 0 || segs[0].SourceTrace != "stream#0" {
		t.Fatalf("unexpected first segment trace: %+v", segs[0])
	}
	if segs[1].StreamIndex != 1 || segs[1].Text != "BC" {
		t.Fatalf("unexpected second segment: %+v", segs[1])
	}
	if segs[1].Order != 1 || segs[1].SourceTrace != "stream#1" {
		t.Fatalf("unexpected second segment trace: %+v", segs[1])
	}
}

func TestExtractTextBasic_FlateDecodeStream(t *testing.T) {
	streamPlain := "BT\n(Compressed) Tj\n(ZGI Parse) Tj\nET\n"
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(streamPlain)); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	compressed := buf.Bytes()

	content := buildPDFWithFlateStream(compressed)
	text := ExtractTextBasicFromBytes([]byte(content))
	if !strings.Contains(text, "Compressed") {
		t.Fatalf("expected Compressed in extracted text, got: %q", text)
	}
	if !strings.Contains(text, "ZGI Parse") {
		t.Fatalf("expected ZGI Parse in extracted text, got: %q", text)
	}
}

func TestExtractTextBasic_LZWDecodeStream(t *testing.T) {
	streamPlain := "BT\n(LZW Text) Tj\n(ZGI Parse) Tj\nET\n"
	var buf bytes.Buffer
	zw := golzw.NewWriter(&buf, golzw.MSB, 8)
	if _, err := zw.Write([]byte(streamPlain)); err != nil {
		t.Fatalf("lzw write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("lzw close: %v", err)
	}

	content := buildPDFWithLZWStream(buf.Bytes())
	text := ExtractTextBasicFromBytes([]byte(content))
	if !strings.Contains(text, "LZW Text") {
		t.Fatalf("expected LZW Text in extracted text, got: %q", text)
	}
	if !strings.Contains(text, "ZGI Parse") {
		t.Fatalf("expected ZGI Parse in extracted text, got: %q", text)
	}
}

func TestExtractTextBasic_ASCII85DecodeStream(t *testing.T) {
	streamPlain := "BT\n(ASCII85 Text) Tj\n(ZGI Parse) Tj\nET\n"
	src := []byte(streamPlain)
	enc := make([]byte, ascii85.MaxEncodedLen(len(src)))
	n := ascii85.Encode(enc, src)
	encoded := append(enc[:n], []byte("~>")...)

	content := buildPDFWithFilterStream("ASCII85Decode", encoded)
	text := ExtractTextBasicFromBytes([]byte(content))
	if !strings.Contains(text, "ASCII85 Text") {
		t.Fatalf("expected ASCII85 Text in extracted text, got: %q", text)
	}
	if !strings.Contains(text, "ZGI Parse") {
		t.Fatalf("expected ZGI Parse in extracted text, got: %q", text)
	}
}

func TestExtractTextBasic_ASCIIHexDecodeStream(t *testing.T) {
	streamPlain := "BT\n(ASCIIHex Text) Tj\n(ZGI Parse) Tj\nET\n"
	hexEncoded := strings.ToUpper(hex.EncodeToString([]byte(streamPlain))) + ">"

	content := buildPDFWithFilterStream("ASCIIHexDecode", []byte(hexEncoded))
	text := ExtractTextBasicFromBytes([]byte(content))
	if !strings.Contains(text, "ASCIIHex Text") {
		t.Fatalf("expected ASCIIHex Text in extracted text, got: %q", text)
	}
	if !strings.Contains(text, "ZGI Parse") {
		t.Fatalf("expected ZGI Parse in extracted text, got: %q", text)
	}
}

func TestExtractTextBasic_HexStringTjAndTJ(t *testing.T) {
	content := `%PDF-1.4
1 0 obj
<<>>
stream
<48656C6C6F> Tj
<5A4749205061727365> Tj
ET
endstream
endobj
%%EOF
`
	text := ExtractTextBasicFromBytes([]byte(content))
	if !strings.Contains(text, "Hello") {
		t.Fatalf("expected Hello in extracted text, got: %q", text)
	}
	if !strings.Contains(text, "ZGI Parse") {
		t.Fatalf("expected ZGI Parse in extracted text, got: %q", text)
	}
}

func TestDecodePDFHexToken_UTF16BE(t *testing.T) {
	got := decodePDFHexToken([]byte("<FEFF4F60597D>"))
	if got != "你好" {
		t.Fatalf("utf16 decode mismatch: %q", got)
	}
}

func TestDecodePDFLiteralBytes_OctalEscape(t *testing.T) {
	b := decodePDFLiteralBytes([]byte("(\\101\\102\\103)"))
	if string(b) != "ABC" {
		t.Fatalf("octal decode mismatch: %q", string(b))
	}
}

func TestParseToUnicodeCMap_BFCharAndBFRange(t *testing.T) {
	cmap := []byte(`
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
1 begincodespacerange
<00> <FF>
endcodespacerange
2 beginbfchar
<41> <0041>
<42> <0042>
endbfchar
1 beginbfrange
<43> <44> <0043>
endbfrange
endcmap
CMapName currentdict /CMap defineresource pop
end
end
`)
	m, use := parseToUnicodeCMap(cmap)
	if use != "" {
		t.Fatalf("unexpected usecmap: %q", use)
	}
	if m.byCodeHex["41"] != "A" || m.byCodeHex["42"] != "B" || m.byCodeHex["43"] != "C" || m.byCodeHex["44"] != "D" {
		t.Fatalf("unexpected cmap map: %+v", m.byCodeHex)
	}
	if got := decodePDFHexTokenWithCMap([]byte("<41424344>"), m); got != "ABCD" {
		t.Fatalf("decode with cmap mismatch: %q", got)
	}
}

func TestParseToUnicodeCMap_UseCMapName(t *testing.T) {
	cmap := []byte(`/BaseMap usecmap`)
	_, use := parseToUnicodeCMap(cmap)
	if use != "BaseMap" {
		t.Fatalf("usecmap parse mismatch: %q", use)
	}
}

func TestCleanupCJKExtractionArtifacts(t *testing.T) {
	in := "数据准确D≥98%，异常发Q；闭P。末尾残字�"
	got := cleanupCJKExtractionArtifacts(in)
	if strings.Contains(got, "D") || strings.Contains(got, "Q") || strings.Contains(got, "P") || strings.Contains(got, "�") {
		t.Fatalf("cleanup should remove CJK-context artifacts, got: %q", got)
	}
	if !strings.Contains(got, "≥98%") {
		t.Fatalf("cleanup should keep numeric context, got: %q", got)
	}
}

func TestCleanupCJKExtractionArtifacts_PUAListAndNoise(t *testing.T) {
	in := "我们提供:\n丰富 Ÿ 模型选择\n强大 ™ 开发工具"
	got := cleanupCJKExtractionArtifacts(in)
	if !strings.Contains(got, "- 丰富") || !strings.Contains(got, "模型选择") {
		t.Fatalf("expected normalized bullet line, got: %q", got)
	}
	if !strings.Contains(got, "- 强大") || !strings.Contains(got, "开发工具") {
		t.Fatalf("expected normalized bullet line, got: %q", got)
	}
	if strings.ContainsAny(got, "Ÿ™") {
		t.Fatalf("expected noisy glyphs removed, got: %q", got)
	}
}

func TestCleanupCJKExtractionArtifacts_NormalizeCJKPunctuation(t *testing.T) {
	in := "你好,世界:欢迎"
	got := cleanupCJKExtractionArtifacts(in)
	if got != "你好，世界：欢迎" {
		t.Fatalf("unexpected punctuation normalize: %q", got)
	}
}

func TestNormalizeLineForExtraction_GenericCleanup(t *testing.T) {
	in := "  AGICTO 使命､愿景､价值观\tŸ™ \x00 "
	got := normalizeLineForExtraction(in)
	if got != "AGICTO 使命、愿景、价值观" {
		t.Fatalf("normalizeLineForExtraction mismatch: %q", got)
	}
}

func TestAssembleGeomTextRuns_SplitsDistinctLinesByY(t *testing.T) {
	fs := 12.0
	runs := []geomTextRun{
		{s: "标题", x: 10, y: 700, b: 0, fontSizePt: fs},
		{s: "正文第一句很长。", x: 10, y: 684, b: 1, fontSizePt: fs},
	}
	got := assembleGeomTextRuns(runs, nil)
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected newline between visual lines, got: %q", got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 2 || lines[0] != "标题" || !strings.Contains(lines[1], "正文") {
		t.Fatalf("unexpected assemble result: %q", got)
	}
}

func TestBuildPageFontUnicodeMaps_TargetPDFIfPresent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "传统国企监管痛点与智能化穿透式监管平台方案优势对比表.pdf")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip("target pdf not present; skip")
	}
	specs, err := DetectPageRenderSpecsBytes(b, ValidationModeRelaxed)
	if err != nil || len(specs) == 0 {
		t.Fatalf("DetectPageRenderSpecsBytes: err=%v len=%d", err, len(specs))
	}
	m := buildPageFontUnicodeMaps(b, specs[0], ValidationModeRelaxed, nil)
	if len(m) == 0 {
		t.Fatalf("expected non-empty page font cmap")
	}
	total := 0
	for _, cm := range m {
		total += len(cm.byCodeHex)
	}
	if total == 0 {
		t.Fatalf("expected non-empty cmap entries")
	}
}

func TestNormalizeExtractedText(t *testing.T) {
	in := "  Hello   ZGI Parse  \n\n\n\tWorld\t \n \n"
	got := normalizeExtractedText(in)
	want := "Hello ZGI Parse\n\n\nWorld\n\n"
	if got != want {
		t.Fatalf("normalize mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestNormalizeExtractedText_CJKPreservesSingleSpaces(t *testing.T) {
	in := "AGICTO 使命  愿景"
	got := normalizeExtractedText(in)
	want := "AGICTO 使命 愿景"
	if got != want {
		t.Fatalf("normalize mismatch\nwant: %q\ngot:  %q", want, got)
	}
}

func TestCollapseSpacedUpperLatinPairsIter(t *testing.T) {
	if g := collapseSpacedUpperLatinPairsIter("A G I C T O使命"); g != "AGICTO使命" {
		t.Fatalf("got %q", g)
	}
	if g := collapseSpacedUpperLatinPairsIter("A G IC T O"); g != "AGICTO" {
		t.Fatalf("got %q", g)
	}
	if g := collapseSpacedUpperLatinPairsIter("在A GICTO我"); g != "在AGICTO我" {
		t.Fatalf("got %q", g)
	}
	if g := collapseSpacedUpperLatinPairsIter("在A THE我"); strings.Contains(g, "ATHE") {
		t.Fatalf("got %q", g)
	}
	if g := collapseSpacedUpperLatinPairsIter("使用 A LLM 很好"); g != "使用 A LLM 很好" {
		t.Fatalf("got %q", g)
	}
}

func TestNormalizeExtractedText_SpacedAcronymOnCJKLine(t *testing.T) {
	in := "在A G I C T O我们相信人工智能"
	got := normalizeExtractedText(in)
	if !strings.Contains(got, "AGICTO") || strings.Contains(got, "A G ") {
		t.Fatalf("expected merged AGICTO, got %q", got)
	}
}

func TestNormalizeExtractedText_SpacedCapsLineWithoutHan(t *testing.T) {
	in := "A G I C T O"
	got := normalizeExtractedText(in)
	if got != "AGICTO" {
		t.Fatalf("expected AGICTO, got %q", got)
	}
}

func TestNormalizeExtractedText_NoMergeXYZebra(t *testing.T) {
	got := normalizeExtractedText("中文 X Y Zebra 结束")
	if strings.Contains(got, "XYZebra") {
		t.Fatalf("must not swallow lowercase word after Z, got %q", got)
	}
}

func TestOrderGeomLineRunsForReadingOrder_CJKUsesStreamOrder(t *testing.T) {
	ln := []geomTextRun{
		{s: "我们提", x: 10, y: 100, b: 0},
		{s: "供", x: 28, y: 100, b: 10},
		{s: "：", x: 22, y: 100, b: 20},
	}
	got := orderGeomLineRunsForReadingOrder(append([]geomTextRun(nil), ln...))
	var sb strings.Builder
	for _, r := range got {
		sb.WriteString(r.s)
	}
	if sb.String() != "我们提供：" {
		t.Fatalf("want stream order for pure CJK-ish block, got %q (#%v)", sb.String(), got)
	}
}

func TestOrderGeomLineRunsForReadingOrder_LatinUsesXOrder(t *testing.T) {
	ln := []geomTextRun{
		{s: "B", x: 20, y: 0, b: 10},
		{s: "A", x: 10, y: 0, b: 0},
	}
	got := orderGeomLineRunsForReadingOrder(append([]geomTextRun(nil), ln...))
	if got[0].s != "A" || got[1].s != "B" {
		t.Fatalf("want x order for Latin block, got %#v", got)
	}
}

func TestClassifyTextChunkType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "heading by prefix", in: "Chapter 1 Introduction", want: "heading"},
		{name: "paragraph by punctuation", in: "This is a normal paragraph.", want: "paragraph"},
		{name: "code by braces", in: "func a() { return 1; }", want: "code"},
		{name: "empty as other", in: "   ", want: "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyTextChunkType(tt.in)
			if got != tt.want {
				t.Fatalf("want %s got %s", tt.want, got)
			}
		})
	}
}

func TestGeomLineBBoxFromRuns_CoversWholeVisualLine(t *testing.T) {
	spec := PageRenderSpec{MediaBox: "0 0 200 100"}
	runs := []geomTextRun{
		{s: "未来", x: 20, y: 80, fontSizePt: 10},
		{s: "AI", x: 48, y: 80, fontSizePt: 10},
		{s: "平台", x: 64, y: 80, fontSizePt: 10},
	}

	got := geomLineBBoxFromRuns(runs, nil, spec)
	if got == nil {
		t.Fatal("expected bbox")
	}
	if got.Left > 0.101 || got.Right < 0.36 {
		t.Fatalf("bbox should span the visual row, got %+v", got)
	}
	if got.Bottom >= got.Top {
		t.Fatalf("invalid vertical bbox: %+v", got)
	}
}

func buildPDFWithFlateStream(streamData []byte) string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3Prefix := fmt.Sprintf("3 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(streamData))
	obj3Suffix := "\nendstream\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj3Prefix + string(streamData) + obj3Suffix)

	xref := "xref\n0 4\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3)
	trailer := "trailer\n<< /Size 4 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + obj3Prefix + string(streamData) + obj3Suffix + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithLZWStream(streamData []byte) string {
	return buildPDFWithFilterStream("LZWDecode", streamData)
}

func buildPDFWithFilterStream(filter string, streamData []byte) string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3Prefix := fmt.Sprintf("3 0 obj\n<< /Length %d /Filter /%s >>\nstream\n", len(streamData), filter)
	obj3Suffix := "\nendstream\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj3Prefix + string(streamData) + obj3Suffix)

	xref := "xref\n0 4\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3)
	trailer := "trailer\n<< /Size 4 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + obj3Prefix + string(streamData) + obj3Suffix + xref + trailer + startxref + "%%EOF\n"
}

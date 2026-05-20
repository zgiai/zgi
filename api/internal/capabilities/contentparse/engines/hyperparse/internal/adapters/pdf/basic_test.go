package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestInspectBasic_ValidXRefTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok.pdf")
	if err := os.WriteFile(path, []byte(buildMinimalPDF()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info, err := InspectBasic(path)
	if err != nil {
		t.Fatalf("InspectBasic should pass: %v", err)
	}
	if info.Version != "1.4" {
		t.Fatalf("unexpected version: %s", info.Version)
	}
	if info.XRefType != "table" {
		t.Fatalf("unexpected xref type: %s", info.XRefType)
	}
	if info.PageCount != 0 {
		t.Fatalf("unexpected page count: %d", info.PageCount)
	}
	if !info.HasTrailer || !info.HasEOFMarker {
		t.Fatalf("unexpected flags: trailer=%t eof=%t", info.HasTrailer, info.HasEOFMarker)
	}
}

func TestInspectBasic_PageCountDetected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ok_pages.pdf")
	content := buildPDFWithPageCount(7)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info, err := InspectBasic(path)
	if err != nil {
		t.Fatalf("InspectBasic should pass: %v", err)
	}
	if info.PageCount != 7 {
		t.Fatalf("expected page count 7, got %d", info.PageCount)
	}
}

func TestInspectBasic_MissingEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pdf")
	content := "%PDF-1.4\n1 0 obj\n<<>>\nendobj\nstartxref\n9\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := InspectBasic(path)
	if err == nil {
		t.Fatal("expected error for missing EOF marker")
	}
}

func TestInspectBasic_InvalidXRefEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_xref.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithBrokenXRefEntry()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := InspectBasic(path)
	if err == nil {
		t.Fatal("expected error for invalid xref entry")
	}
}

func TestInspectBasicWithMode_StrictVsRelaxed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "loose_xref.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithLooseXRefEntry()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass loose xref entry, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail loose xref entry")
	}
}

func TestInspectBasicWithMode_StrictStartXRefOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_startxref.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithWhitespaceBeforeXRef()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass shifted startxref, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail shifted startxref")
	}
}

func TestInspectBasicWithMode_StrictTrailerSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_trailer_size.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithSmallTrailerSize()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass small trailer size, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail small trailer /Size")
	}
}

func TestInspectBasicWithMode_StrictXRefOffsetToObjectHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_xref_offset.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithWrongInUseOffset()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass wrong in-use offset, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail wrong in-use offset")
	}
}

func TestInspectBasicWithMode_StrictTrailerRootCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_root_catalog.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithNonCatalogRoot()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass non-catalog root, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail non-catalog root")
	}
}

func TestInspectBasicWithMode_StrictCatalogPagesLink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_catalog_pages_link.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithBrokenCatalogPagesRef()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass broken catalog pages link, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail broken catalog pages link")
	}
}

func TestInspectBasicWithMode_StrictCatalogCountConsistency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_catalog_count.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithInvalidCatalogCount()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass invalid catalog count sample, got: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail invalid catalog /Count")
	}
}

func TestInspectBasic_PageCountFallbackSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fallback_count_source.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithInvalidCatalogCount()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info, err := InspectBasicWithMode(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("InspectBasicWithMode should pass: %v", err)
	}
	if info.CountSource != CountSourceScanFallback {
		t.Fatalf("expected count source %q, got %q", CountSourceScanFallback, info.CountSource)
	}
	if info.PageCount != 1 {
		t.Fatalf("expected fallback page count 1, got %d", info.PageCount)
	}
}

func TestInspectBasic_MetadataInfoDictionary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta_info.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithInfoMetadata()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info, err := InspectBasicWithMode(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("InspectBasicWithMode should pass: %v", err)
	}
	if info.Title != "ZGI Parse Test" {
		t.Fatalf("unexpected title: %q", info.Title)
	}
	if info.Author != "ZGI" {
		t.Fatalf("unexpected author: %q", info.Author)
	}
	if info.Producer != "ZGIParseEngine" {
		t.Fatalf("unexpected producer: %q", info.Producer)
	}
}

func TestInspectBasic_PageCountPreferRootPages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "root_preferred_count.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithRootPreferredCount()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info, err := InspectBasicWithMode(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("InspectBasicWithMode should pass: %v", err)
	}
	if info.PageCount != 5 {
		t.Fatalf("expected page count 5 from root pages, got %d", info.PageCount)
	}
	if info.CountSource != CountSourceRootChain {
		t.Fatalf("expected count source %q, got %q", CountSourceRootChain, info.CountSource)
	}
}

func TestDetectPageObjectNumbers_NestedPagesTree(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested_pages.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithNestedPagesTree()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got, err := DetectPageObjectNumbers(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageObjectNumbers should pass: %v", err)
	}
	if len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("unexpected page objects: %+v", got)
	}
}

func TestDetectPageObjectNumbers_PageInObjStm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page_in_objstm.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithPageLeafInObjStm()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	got, err := DetectPageObjectNumbers(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageObjectNumbers should pass for ObjStm page: %v", err)
	}
	if len(got) != 1 || got[0] != 6 {
		t.Fatalf("unexpected page objects from ObjStm: %+v", got)
	}
}

func TestDetectPageInfos_MediaBox(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page_infos_mediabox.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithNestedPagesTreeWithMediaBoxes()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	infos, err := DetectPageInfos(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfos should pass: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("unexpected infos len: %d", len(infos))
	}
	if infos[0].ObjectNumber != 3 || infos[0].MediaBox != "0 0 500 700" {
		t.Fatalf("unexpected first page info: %+v", infos[0])
	}
	if infos[1].ObjectNumber != 4 || infos[1].MediaBox != "0 0 600 800" {
		t.Fatalf("unexpected second page info: %+v", infos[1])
	}
}

func TestDetectPageInfos_CatalogPagesBracketRef(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_pages_bracket.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithNestedPagesTreeCatalogBracket()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	infos, err := DetectPageInfos(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfos should accept /Pages [ n 0 R ]: %v", err)
	}
	if len(infos) != 2 || infos[0].ObjectNumber != 3 || infos[1].ObjectNumber != 4 {
		t.Fatalf("unexpected page infos: %+v", infos)
	}
}

func buildPDFCompactTypeTokensOnePage() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<</Pages 2 0 R/Type/Catalog>>\nendobj\n"
	obj2 := "2 0 obj\n<</Count 1/Kids[3 0 R]/Type/Pages>>\nendobj\n"
	obj3 := "3 0 obj\n<</MediaBox[0 0 100 100]/Parent 2 0 R/Type/Page>>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj3)

	xref := "xref\n0 4\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3)
	trailer := "trailer\n<< /Size 4 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)
	return header + obj1 + obj2 + obj3 + xref + trailer + startxref + "%%EOF\n"
}

func TestDetectPageInfos_CompactTypeTokens(t *testing.T) {
	data := []byte(buildPDFCompactTypeTokensOnePage())
	infos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfosBytes: %v", err)
	}
	if len(infos) != 1 || infos[0].ObjectNumber != 3 || infos[0].MediaBox != "0 0 100 100" {
		t.Fatalf("unexpected page infos: %+v", infos)
	}
}

func buildPDFOnePageWithPageBoxes() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /CropBox [10 10 190 190] /BleedBox [5 5 195 195] /TrimBox [12 12 188 188] /ArtBox [20 20 180 180] >>\nendobj\n"
	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj3)
	xref := "xref\n0 4\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3)
	trailer := "trailer\n<< /Size 4 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)
	return header + obj1 + obj2 + obj3 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFOnePageIndirectMediaBoxAndCrop() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox 4 0 R /CropBox 5 0 R >>\nendobj\n"
	obj4 := "4 0 obj\n[0 0 300 400]\nendobj\n"
	obj5 := "5 0 obj\n[10 10 290 390]\nendobj\n"
	chunks := []string{obj1, obj2, obj3, obj4, obj5}
	var body strings.Builder
	body.WriteString(header)
	cur := len(header)
	offsets := make([]int, 0, len(chunks))
	for _, ch := range chunks {
		offsets = append(offsets, cur)
		body.WriteString(ch)
		cur += len(ch)
	}
	s := body.String()
	xrefOffset := len(s)
	var sb strings.Builder
	sb.WriteString(s)
	sb.WriteString("xref\n0 6\n")
	sb.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	sb.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\n")
	sb.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	sb.WriteString("%%EOF\n")
	return sb.String()
}

func buildPDFOnePageBracketIndirectMediaBox() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [ 4 0 R ] >>\nendobj\n"
	obj4 := "4 0 obj\n[0 0 250 350]\nendobj\n"
	chunks := []string{obj1, obj2, obj3, obj4}
	var body strings.Builder
	body.WriteString(header)
	cur := len(header)
	offsets := make([]int, 0, len(chunks))
	for _, ch := range chunks {
		offsets = append(offsets, cur)
		body.WriteString(ch)
		cur += len(ch)
	}
	s := body.String()
	xrefOffset := len(s)
	var sb strings.Builder
	sb.WriteString(s)
	sb.WriteString("xref\n0 5\n")
	sb.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	sb.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\n")
	sb.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	sb.WriteString("%%EOF\n")
	return sb.String()
}

func buildPDFPageBoxesInheritedFromPages() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] /MediaBox [0 0 300 400] /CropBox [10 20 290 380] /BleedBox [0 0 300 400] /TrimBox [15 25 285 375] /ArtBox [30 40 270 360] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"
	chunks := []string{obj1, obj2, obj3}
	var body strings.Builder
	body.WriteString(header)
	cur := len(header)
	offsets := make([]int, 0, len(chunks))
	for _, ch := range chunks {
		offsets = append(offsets, cur)
		body.WriteString(ch)
		cur += len(ch)
	}
	s := body.String()
	xrefOffset := len(s)
	var sb strings.Builder
	sb.WriteString(s)
	sb.WriteString("xref\n0 4\n")
	sb.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	sb.WriteString("trailer\n<< /Size 4 /Root 1 0 R >>\n")
	sb.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	sb.WriteString("%%EOF\n")
	return sb.String()
}

func buildPDFPageBoxesInheritedWithChildOverrides() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] /MediaBox [0 0 300 400] /CropBox [10 20 290 380] /BleedBox [0 0 300 400] /TrimBox [15 25 285 375] /ArtBox [30 40 270 360] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /CropBox [20 30 280 370] /ArtBox [40 50 260 350] >>\nendobj\n"
	chunks := []string{obj1, obj2, obj3}
	var body strings.Builder
	body.WriteString(header)
	cur := len(header)
	offsets := make([]int, 0, len(chunks))
	for _, ch := range chunks {
		offsets = append(offsets, cur)
		body.WriteString(ch)
		cur += len(ch)
	}
	s := body.String()
	xrefOffset := len(s)
	var sb strings.Builder
	sb.WriteString(s)
	sb.WriteString("xref\n0 4\n")
	sb.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	sb.WriteString("trailer\n<< /Size 4 /Root 1 0 R >>\n")
	sb.WriteString(fmt.Sprintf("startxref\n%d\n", xrefOffset))
	sb.WriteString("%%EOF\n")
	return sb.String()
}

func TestDetectPageInfos_BracketIndirectMediaBox(t *testing.T) {
	data := []byte(buildPDFOnePageBracketIndirectMediaBox())
	infos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfosBytes: %v", err)
	}
	if len(infos) != 1 || infos[0].MediaBox != "0 0 250 350" {
		t.Fatalf("got %+v", infos[0])
	}
}

func TestContentsIndirectRefObjectNumbers(t *testing.T) {
	single := []byte("<< /Type /Page /Contents 9 0 R >>")
	if got := ContentsIndirectRefObjectNumbers(single); len(got) != 1 || got[0] != 9 {
		t.Fatalf("single: %v", got)
	}
	arr := []byte("<< /Contents [ 4 0 R ] /Type /Page >>")
	if got := ContentsIndirectRefObjectNumbers(arr); len(got) != 1 || got[0] != 4 {
		t.Fatalf("array one: %v", got)
	}
	arr2 := []byte("<< /Contents [ 4 0 R 5 0 R ] >>")
	if got := ContentsIndirectRefObjectNumbers(arr2); len(got) != 2 {
		t.Fatalf("array two: %v", got)
	}
}

func TestDetectPageInfos_IndirectPageBoxes(t *testing.T) {
	data := []byte(buildPDFOnePageIndirectMediaBoxAndCrop())
	infos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfosBytes: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("len=%d", len(infos))
	}
	pi := infos[0]
	if pi.MediaBox != "0 0 300 400" || pi.CropBox != "10 10 290 390" {
		t.Fatalf("indirect boxes: %+v", pi)
	}
}

func TestDetectPageInfos_PageBoxes(t *testing.T) {
	data := []byte(buildPDFOnePageWithPageBoxes())
	infos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfosBytes: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("len=%d", len(infos))
	}
	pi := infos[0]
	if pi.ObjectNumber != 3 || pi.MediaBox != "0 0 200 200" {
		t.Fatalf("media: %+v", pi)
	}
	if pi.CropBox != "10 10 190 190" || pi.BleedBox != "5 5 195 195" || pi.TrimBox != "12 12 188 188" || pi.ArtBox != "20 20 180 180" {
		t.Fatalf("boxes: %+v", pi)
	}
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageRenderSpecsBytes: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs len=%d", len(specs))
	}
	sp := specs[0]
	if sp.CropBox != pi.CropBox || sp.BleedBox != pi.BleedBox || sp.TrimBox != pi.TrimBox || sp.ArtBox != pi.ArtBox {
		t.Fatalf("spec boxes: %+v", sp)
	}
}

func TestDetectPageInfos_InheritedPageBoxesFromPagesNode(t *testing.T) {
	data := []byte(buildPDFPageBoxesInheritedFromPages())
	infos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfosBytes: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("len=%d", len(infos))
	}
	pi := infos[0]
	if pi.MediaBox != "0 0 300 400" {
		t.Fatalf("media inherited failed: %+v", pi)
	}
	if pi.CropBox != "10 20 290 380" || pi.BleedBox != "0 0 300 400" || pi.TrimBox != "15 25 285 375" || pi.ArtBox != "30 40 270 360" {
		t.Fatalf("optional inherited boxes failed: %+v", pi)
	}
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageRenderSpecsBytes: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("specs len=%d", len(specs))
	}
	if specs[0].CropBox != pi.CropBox || specs[0].BleedBox != pi.BleedBox || specs[0].TrimBox != pi.TrimBox || specs[0].ArtBox != pi.ArtBox {
		t.Fatalf("spec inherited boxes mismatch: %+v", specs[0])
	}
}

func TestDetectPageInfos_InheritedAndOverriddenPageBoxes(t *testing.T) {
	data := []byte(buildPDFPageBoxesInheritedWithChildOverrides())
	infos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageInfosBytes: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("len=%d", len(infos))
	}
	pi := infos[0]
	if pi.MediaBox != "0 0 300 400" {
		t.Fatalf("media inherit failed: %+v", pi)
	}
	if pi.CropBox != "20 30 280 370" {
		t.Fatalf("crop override failed: %+v", pi)
	}
	if pi.BleedBox != "0 0 300 400" || pi.TrimBox != "15 25 285 375" {
		t.Fatalf("bleed/trim inherit failed: %+v", pi)
	}
	if pi.ArtBox != "40 50 260 350" {
		t.Fatalf("art override failed: %+v", pi)
	}
}

func TestParseIndirectRefObjectNumberByKey_GluedR(t *testing.T) {
	b := []byte("<</Resources 30 0 R/Rotate 0/Type/Page>>")
	n, ok := parseIndirectRefObjectNumberByKey(b, "/Resources")
	if !ok || n != 30 {
		t.Fatalf("Resources ref: n=%d ok=%v", n, ok)
	}
	b2 := []byte("<</Parent 12 0 R/Resources 30 0 R>>")
	p, ok := parseIndirectRefObjectNumberByKey(b2, "/Parent")
	if !ok || p != 12 {
		t.Fatalf("Parent ref: n=%d ok=%v", p, ok)
	}
	b3 := []byte("<</Contents [18 0 R 19 0 R 20 0 R]>>")
	c, ok := parseIndirectRefObjectNumberByKey(b3, "/Contents")
	if !ok || c != 18 {
		t.Fatalf("Contents array first ref: n=%d ok=%v", c, ok)
	}
}

func TestDetectPageRenderSpecs_BasicRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page_render_specs.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithPageRenderRefs()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	specs, err := DetectPageRenderSpecs(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("DetectPageRenderSpecs should pass: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("unexpected specs len: %d", len(specs))
	}
	if specs[0].ObjectNumber != 3 || specs[0].ContentsRefObject != 4 || specs[0].ResourcesRefObject != 5 {
		t.Fatalf("unexpected refs: %+v", specs[0])
	}
	if !bytes.Contains([]byte(specs[0].ContentsObject), []byte("stream")) {
		t.Fatalf("missing contents object payload: %q", specs[0].ContentsObject)
	}
	if !bytes.Contains([]byte(specs[0].ResourcesObject), []byte("/ProcSet")) {
		t.Fatalf("missing resources object payload: %q", specs[0].ResourcesObject)
	}
}

func TestExtractObjectBlockByNumber(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "object_by_number.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithPageRenderRefs()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	b, err := ExtractObjectBlockByNumber(path, 4, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("ExtractObjectBlockByNumber should pass: %v", err)
	}
	if !strings.Contains(b, "4 0 obj") || !strings.Contains(b, "stream") {
		t.Fatalf("unexpected object block: %q", b)
	}
	if _, err := ExtractObjectBlockByNumber(path, 999, ValidationModeRelaxed); err == nil {
		t.Fatal("expected missing object error")
	}
}

func TestInspectBasicBytes(t *testing.T) {
	data := []byte(buildPDFWithPageCount(3))
	info, err := InspectBasicBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("InspectBasicBytes should pass: %v", err)
	}
	if info.PageCount != 3 {
		t.Fatalf("expected page count 3, got %d", info.PageCount)
	}
	if info.CountSource != CountSourceRootChain {
		t.Fatalf("expected count source %q, got %q", CountSourceRootChain, info.CountSource)
	}
}

func TestInspectBasic_XRefStreamRelaxed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_ok.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStream(true)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	info, err := InspectBasicWithMode(path, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("relaxed should pass xref stream sample: %v", err)
	}
	if info.XRefType != "stream" {
		t.Fatalf("expected xref type stream, got %q", info.XRefType)
	}
}

func TestInspectBasic_XRefStreamStrictStartOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_shifted.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStream(false)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass shifted xref stream: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail shifted xref stream startxref")
	}
}

func TestInspectBasic_XRefStreamStrictRootCatalog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_bad_root.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamNonCatalogRoot()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass non-catalog root in xref stream: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail non-catalog root in xref stream")
	}
}

func TestInspectBasic_XRefStreamMissingW(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_missing_w.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamMissingW()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err == nil {
		t.Fatal("relaxed should fail xref stream without /W")
	}
}

func TestInspectBasic_XRefStreamBadWAlignment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_bad_w_align.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamBadWAlignment()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err == nil {
		t.Fatal("relaxed should fail xref stream with unaligned stream length")
	}
}

func TestInspectBasic_XRefStreamIndexCoverageMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_bad_index_coverage.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamBadIndexCoverage()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err == nil {
		t.Fatal("relaxed should fail when /Index coverage mismatches entries")
	}
}

func TestInspectBasic_XRefStreamInvalidIndexFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_bad_index_format.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamInvalidIndexFormat()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err == nil {
		t.Fatal("relaxed should fail when /Index format is invalid")
	}
}

func TestInspectBasic_XRefStreamInvalidEntryType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_bad_entry_type.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamInvalidEntryType()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err == nil {
		t.Fatal("relaxed should fail when xref stream entry type is invalid")
	}
}

func TestInspectBasic_XRefStreamStrictZeroOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_strict_zero_offset.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamZeroOffsetType1()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass zero-offset type1 sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail zero-offset type1 entry")
	}
}

func TestInspectBasic_XRefStreamStrictWrongOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_strict_wrong_offset.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamWrongOffsetType1()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass wrong-offset type1 sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail wrong-offset type1 entry")
	}
}

func TestInspectBasic_XRefStreamStrictType2ObjStmValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_valid.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2ObjStmValid()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err != nil {
		t.Fatalf("strict should pass valid type2->objstm linkage: %v", err)
	}
}

func TestInspectBasic_XRefStreamStrictType2ObjStmInvalidIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_bad_index.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2ObjStmInvalidIndex()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass type2 bad index sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail type2 objstm index overflow")
	}
}

func TestInspectBasic_XRefStreamStrictType2ObjMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_obj_mismatch.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2ObjMismatch()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass type2 obj mismatch sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail when type2 points to mismatched obj id")
	}
}

func TestInspectBasic_XRefStreamMultiIndexStrictPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_multi_index_ok.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamMultiIndex(true)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err != nil {
		t.Fatalf("strict should pass multi-index xref stream: %v", err)
	}
}

func TestInspectBasic_XRefStreamMultiIndexStrictFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_multi_index_bad.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamMultiIndex(false)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass multi-index bad mapping sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail multi-index xref stream wrong object mapping")
	}
}

func TestInspectBasic_XRefStreamStrictType2ObjStmDecodeFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_objstm_decode_fail.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2ObjStmDecodeFail()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass objstm decode-fail sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail when type2 objstm stream cannot decode")
	}
}

func TestInspectBasic_XRefStreamStrictType0Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type0_valid.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType0Valid()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err != nil {
		t.Fatalf("strict should pass valid type0 free entry: %v", err)
	}
}

func TestInspectBasic_XRefStreamStrictType0InvalidGen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type0_bad_gen.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType0InvalidGen()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass invalid type0 generation sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail invalid type0 generation")
	}
}

func TestInspectBasic_XRefStreamStrictType2MultiObjStmPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_multi_objstm_ok.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2MultiObjStm(true)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err != nil {
		t.Fatalf("strict should pass type2 with multi-object objstm: %v", err)
	}
}

func TestInspectBasic_XRefStreamStrictType2MultiObjStmFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_multi_objstm_bad.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2MultiObjStm(false)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass type2 multi-object mismatch sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail type2 multi-object objstm mapping mismatch")
	}
}

func TestInspectBasic_XRefStreamStrictType2MultiObjStmFlatePass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_multi_objstm_flate_ok.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2MultiObjStmFlate(true)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err != nil {
		t.Fatalf("strict should pass flate multi-object objstm mapping: %v", err)
	}
}

func TestInspectBasic_XRefStreamStrictType2MultiObjStmFlateFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_type2_multi_objstm_flate_bad.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamType2MultiObjStmFlate(false)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass flate multi-object mismatch sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail flate multi-object objstm mapping mismatch")
	}
}

func TestInspectBasic_XRefStreamW0DefaultType1StrictPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_w0_default_type1_ok.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamW0DefaultType1(true)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err != nil {
		t.Fatalf("strict should pass /W[0 4 1] default type=1 sample: %v", err)
	}
}

func TestInspectBasic_XRefStreamW0DefaultType1StrictFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_w0_default_type1_bad.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamW0DefaultType1(false)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass /W[0 4 1] bad-offset sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail /W[0 4 1] bad-offset sample")
	}
}

func TestInspectBasic_XRefStreamW1ZeroStrictFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_w1_zero_strict_fail.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamW1Zero()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass /W[1 0 1] sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail when /W has zero offset width (w1=0)")
	}
}

func TestInspectBasic_XRefStreamW00OneStrictFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_w00one_strict_fail.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamW00One()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass /W[0 0 1] sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail /W[0 0 1] due to missing type1 offset")
	}
}

func TestInspectBasic_XRefStreamStrictIndexOutOfSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_index_out_of_size.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamIndexOutOfSize()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass /Index out-of-size sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail /Index range exceeding /Size")
	}
}

func TestInspectBasic_XRefStreamStrictIndexOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_index_overlap.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamIndexOverlap()), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeRelaxed); err != nil {
		t.Fatalf("relaxed should pass /Index overlap sample: %v", err)
	}
	if _, err := InspectBasicWithMode(path, ValidationModeStrict); err == nil {
		t.Fatal("strict should fail overlapping /Index ranges")
	}
}

func TestInspectBasic_XRefStreamStrictPagesInObjStm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_objstm_pages.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamPagesInObjStm(false)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := InspectBasicWithMode(path, ValidationModeStrict)
	if err != nil {
		t.Fatalf("strict should pass when /Pages is in object stream: %v", err)
	}
	if info.PageCount != 4 {
		t.Fatalf("expected page count 4 from ObjStm pages, got %d", info.PageCount)
	}
}

func TestInspectBasic_XRefStreamStrictPagesInFlateObjStm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "xref_stream_objstm_pages_flate.pdf")
	if err := os.WriteFile(path, []byte(buildPDFWithXRefStreamPagesInObjStm(true)), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	info, err := InspectBasicWithMode(path, ValidationModeStrict)
	if err != nil {
		t.Fatalf("strict should pass when /Pages is in flate object stream: %v", err)
	}
	if info.PageCount != 4 {
		t.Fatalf("expected page count 4 from flate ObjStm pages, got %d", info.PageCount)
	}
}

func TestFindObjectBlockByNumber_FromObjStm(t *testing.T) {
	data := []byte(buildPDFWithXRefStreamPagesInObjStm(false))
	obj, ok := findObjectBlockByNumber(data, 2)
	if !ok {
		t.Fatal("expected to find object 2 from ObjStm")
	}
	if !bytes.Contains(obj, []byte("/Type /Pages")) {
		t.Fatalf("unexpected object block: %q", string(obj))
	}
}

func TestFindObjectBlockByNumber_FromFlateObjStm(t *testing.T) {
	data := []byte(buildPDFWithXRefStreamPagesInObjStm(true))
	obj, ok := findObjectBlockByNumber(data, 2)
	if !ok {
		t.Fatal("expected to find object 2 from flate ObjStm")
	}
	if !bytes.Contains(obj, []byte("/Type /Pages")) || !bytes.Contains(obj, []byte("/Count 4")) {
		t.Fatalf("unexpected flate object block: %q", string(obj))
	}
}

func buildMinimalPDF() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 0 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithPageCount(count int) string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := fmt.Sprintf("2 0 obj\n<< /Type /Pages /Count %d /Kids [] >>\nendobj\n", count)

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithBrokenXRefEntry() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d XXXXX n \n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithLooseXRefEntry() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n extra\n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithWhitespaceBeforeXRef() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "\n" + "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithSmallTrailerSize() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2)

	trailer := "trailer\n<< /Size 2 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithWrongInUseOffset() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		"0000000000 00000 n \n"

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithNonCatalogRoot() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithBrokenCatalogPagesRef() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 9 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	xrefOffset := len(header + obj1 + obj2)

	xref := "xref\n0 3\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2)

	trailer := "trailer\n<< /Size 3 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithInvalidCatalogCount() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 3 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Pages /Count X /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj3)

	xref := "xref\n0 4\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3)

	trailer := "trailer\n<< /Size 4 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + obj3 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithRootPreferredCount() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 3 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Pages /Count 5 /Kids [] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj3)

	xref := "xref\n0 4\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3)

	trailer := "trailer\n<< /Size 4 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + obj3 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithNestedPagesTreeCatalogBracket() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages [ 2 0 R ] >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 2 /Kids [3 0 R 4 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4)

	xref := "xref\n0 5\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4)
	trailer := "trailer\n<< /Size 5 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)
	return header + obj1 + obj2 + obj3 + obj4 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithNestedPagesTree() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 2 /Kids [3 0 R 4 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4)

	xref := "xref\n0 5\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4)
	trailer := "trailer\n<< /Size 5 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)
	return header + obj1 + obj2 + obj3 + obj4 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithPageLeafInObjStm() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [6 0 R] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length 36 >>\nstream\n6 0 << /Type /Page /Parent 2 0 R >>\nendstream\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset5 := len(header + obj1 + obj2)
	xrefOffset := len(header + obj1 + obj2 + obj5)

	xref := "xref\n0 6\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		"0000000000 00000 f \n" +
		"0000000000 00000 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset5)
	trailer := "trailer\n<< /Size 6 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)
	return header + obj1 + obj2 + obj5 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithNestedPagesTreeWithMediaBoxes() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 2 /Kids [3 0 R 4 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 500 700] >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4)

	xref := "xref\n0 5\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4)
	trailer := "trailer\n<< /Size 5 /Root 1 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)
	return header + obj1 + obj2 + obj3 + obj4 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithPageRenderRefs() string {
	var sb strings.Builder
	sb.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, 5)
	writeObj := func(objNum int, body string) {
		offsets = append(offsets, sb.Len())
		sb.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", objNum, body))
	}
	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Count 1 /Kids [3 0 R] >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources 5 0 R >>")
	writeObj(4, "<< /Length 34 >>\nstream\nBT /F1 12 Tf 72 720 Td (Hello) Tj ET\nendstream")
	writeObj(5, "<< /ProcSet [/PDF /Text] /Font << /F1 6 0 R >> >>")
	writeObj(6, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	xrefPos := sb.Len()
	sb.WriteString("xref\n0 7\n")
	sb.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		sb.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}
	sb.WriteString("trailer\n<< /Size 7 /Root 1 0 R >>\n")
	sb.WriteString(fmt.Sprintf("startxref\n%d\n", xrefPos))
	sb.WriteString("%%EOF\n")
	return sb.String()
}

func buildPDFWithInfoMetadata() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [4 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Title (ZGI Parse Test) /Author (ZGI) /Producer (ZGIParseEngine) /Creator (ZGI Parse CLI) /Subject (PDF Parse) >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n"

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4)

	xref := "xref\n0 5\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4)

	trailer := "trailer\n<< /Size 5 /Root 1 0 R /Info 3 0 R >>\n"
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + obj3 + obj4 + xref + trailer + startxref + "%%EOF\n"
}

func buildPDFWithXRefStream(exactStart bool) string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	prefix := ""
	if !exactStart {
		prefix = "\n"
	}
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(offset1, 0)
	obj3 := prefix + fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamNonCatalogRoot() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(offset1, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamPagesInObjStm(flate bool) string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	offset1 := len(header)

	embedded := "<< /Type /Pages /Count 4 /Kids [] >>"
	prefix := "2 0 "
	rawObjStmData := prefix + embedded
	streamData := rawObjStmData
	filter := ""
	if flate {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		_, _ = zw.Write([]byte(rawObjStmData))
		_ = zw.Close()
		streamData = buf.String()
		filter = " /Filter /FlateDecode"
	}
	first := len(prefix)
	obj5 := fmt.Sprintf("5 0 obj\n<< /Type /ObjStm /N 1 /First %d /Length %d%s >>\nstream\n%s\nendstream\nendobj\n",
		first, len(streamData), filter, streamData)

	offset3 := len(header + obj1 + obj5)
	stream := xrefStreamEntryType1(offset1, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 6 /Root 1 0 R /W [1 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamMissingW() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(offset1, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamBadWAlignment() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	obj3 := "3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Length 5 >>\nstream\nABCDE\nendstream\nendobj\n"
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamBadIndexCoverage() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(offset1, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [0 2] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamInvalidIndexFormat() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(offset1, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [0 1 2] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamInvalidEntryType() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2)
	stream := string([]byte{9, byte((offset1 >> 24) & 0xff), byte((offset1 >> 16) & 0xff), byte((offset1 >> 8) & 0xff), byte(offset1 & 0xff), 0})
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamZeroOffsetType1() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(0, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamWrongOffsetType1() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(123, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType2ObjStmValid() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length 32 >>\nstream\n4 0 << /Length 1 >>\nendstream\nendobj\n"
	offset3 := len(header + obj1 + obj2 + obj5)
	stream := xrefStreamEntryType2(5, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 6 /Root 1 0 R /W [1 4 1] /Index [4 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType2ObjStmInvalidIndex() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length 32 >>\nstream\n4 0 << /Length 1 >>\nendstream\nendobj\n"
	offset3 := len(header + obj1 + obj2 + obj5)
	stream := xrefStreamEntryType2(5, 2)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 6 /Root 1 0 R /W [1 4 1] /Index [4 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType2ObjMismatch() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length 32 >>\nstream\n7 0 << /Length 1 >>\nendstream\nendobj\n"
	offset3 := len(header + obj1 + obj2 + obj5)
	stream := xrefStreamEntryType2(5, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 8 /Root 1 0 R /W [1 4 1] /Index [4 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamMultiIndex(strictPass bool) string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length 32 >>\nstream\n6 0 << /Length 1 >>\nendstream\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2 + obj5)

	entry1 := xrefStreamEntryType1(offset1, 0)
	type2Index := 0
	if !strictPass {
		type2Index = 1
	}
	entry2 := xrefStreamEntryType2(5, type2Index)
	stream := entry1 + entry2
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 8 /Root 1 0 R /W [1 4 1] /Index [1 1 6 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType2ObjStmDecodeFail() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length 6 /Filter /FlateDecode >>\nstream\nBADZIP\nendstream\nendobj\n"
	offset3 := len(header + obj1 + obj2 + obj5)
	stream := xrefStreamEntryType2(5, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 8 /Root 1 0 R /W [1 4 1] /Index [4 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType0Valid() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType0(0, 255)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [0 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType0InvalidGen() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := string([]byte{
		0,          // type
		0, 0, 0, 0, // next free obj
		1, 0, 0, // generation = 65536
	})
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 3] /Index [0 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType2MultiObjStm(strictPass bool) string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	embedded1 := "<< /Length 1 >>"
	embedded2 := "<< /Length 2 >>"
	indexHeader := "6 0 7 " + strconv.Itoa(len(embedded1)+1) + " "
	first := len(indexHeader)
	payload := indexHeader + embedded1 + "\n" + embedded2
	obj5 := fmt.Sprintf("5 0 obj\n<< /Type /ObjStm /N 2 /First %d /Length %d >>\nstream\n%s\nendstream\nendobj\n", first, len(payload), payload)

	offset3 := len(header + obj1 + obj2 + obj5)
	entry6 := xrefStreamEntryType2(5, 0)
	entry7Index := 1
	if !strictPass {
		entry7Index = 0
	}
	entry7 := xrefStreamEntryType2(5, entry7Index)
	stream := entry6 + entry7
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 10 /Root 1 0 R /W [1 4 1] /Index [6 2] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamType2MultiObjStmFlate(strictPass bool) string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"

	embedded1 := "<< /Length 1 >>"
	embedded2 := "<< /Length 2 >>"
	indexHeader := "6 0 7 " + strconv.Itoa(len(embedded1)+1) + " "
	first := len(indexHeader)
	rawPayload := indexHeader + embedded1 + "\n" + embedded2
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	_, _ = zw.Write([]byte(rawPayload))
	_ = zw.Close()
	flatePayload := buf.String()
	obj5 := fmt.Sprintf("5 0 obj\n<< /Type /ObjStm /N 2 /First %d /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream\nendobj\n", first, len(flatePayload), flatePayload)

	offset3 := len(header + obj1 + obj2 + obj5)
	entry6 := xrefStreamEntryType2(5, 0)
	entry7Index := 1
	if !strictPass {
		entry7Index = 0
	}
	entry7 := xrefStreamEntryType2(5, entry7Index)
	stream := entry6 + entry7
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 10 /Root 1 0 R /W [1 4 1] /Index [6 2] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj5 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamW0DefaultType1(strictPass bool) string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset1 := len(header)
	offset3 := len(header + obj1 + obj2)
	off := offset1
	if !strictPass {
		off = 123
	}
	stream := xrefStreamEntryW0(off, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [0 4 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamW1Zero() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := string([]byte{1, 0})
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 0 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamW00One() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := string([]byte{0})
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [0 0 1] /Index [1 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamIndexOutOfSize() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(16, 0) + xrefStreamEntryType1(16, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 4 /Root 1 0 R /W [1 4 1] /Index [3 2] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func buildPDFWithXRefStreamIndexOverlap() string {
	header := "%PDF-1.5\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [] >>\nendobj\n"
	offset3 := len(header + obj1 + obj2)
	stream := xrefStreamEntryType1(16, 0) + xrefStreamEntryType1(16, 0) + xrefStreamEntryType1(16, 0)
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /XRef /Size 6 /Root 1 0 R /W [1 4 1] /Index [1 2 2 1] /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)
	startxref := fmt.Sprintf("startxref\n%d\n", offset3)
	return header + obj1 + obj2 + obj3 + startxref + "%%EOF\n"
}

func xrefStreamEntryType1(offset int, gen int) string {
	return string([]byte{
		1,
		byte((offset >> 24) & 0xff),
		byte((offset >> 16) & 0xff),
		byte((offset >> 8) & 0xff),
		byte(offset & 0xff),
		byte(gen & 0xff),
	})
}

func xrefStreamEntryType2(objStmObj int, objStmIndex int) string {
	return string([]byte{
		2,
		byte((objStmObj >> 24) & 0xff),
		byte((objStmObj >> 16) & 0xff),
		byte((objStmObj >> 8) & 0xff),
		byte(objStmObj & 0xff),
		byte(objStmIndex & 0xff),
	})
}

func xrefStreamEntryType0(nextObj int, gen int) string {
	return string([]byte{
		0,
		byte((nextObj >> 24) & 0xff),
		byte((nextObj >> 16) & 0xff),
		byte((nextObj >> 8) & 0xff),
		byte(nextObj & 0xff),
		byte(gen & 0xff),
	})
}

func xrefStreamEntryW0(offset int, gen int) string {
	return string([]byte{
		byte((offset >> 24) & 0xff),
		byte((offset >> 16) & 0xff),
		byte((offset >> 8) & 0xff),
		byte(offset & 0xff),
		byte(gen & 0xff),
	})
}

func TestSplitNonEmptyRawLines_normalizesCR(t *testing.T) {
	lines := splitNonEmptyRawLines([]byte("a\rb\nc\r\nd"))
	want := []string{"a", "b", "c", "d"}
	if len(lines) != len(want) {
		t.Fatalf("got %q", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("line %d: got %q want %q", i, lines[i], want[i])
		}
	}
}

func TestExpandMergedTrailerLines(t *testing.T) {
	in := []string{"xref", "0 1", "0000000000 65535 f", "trailer<</Size 2 /Root 1 0 R>>"}
	out := expandMergedTrailerLines(in)
	if len(out) != 5 {
		t.Fatalf("got %d lines %q", len(out), out)
	}
	if out[3] != "trailer" || !strings.HasPrefix(strings.TrimSpace(out[4]), "<<") {
		t.Fatalf("got %q", out)
	}
}

func TestSplitXRefRegionNonEmptyLines_trailerCRSeparated(t *testing.T) {
	region := []byte("xref\n0 1\n0000000000 65535 f\r\ntrailer<</Size 2 /Root 1 0 R>>\rstartxref\r99\r")
	lines := splitXRefRegionNonEmptyLines(region)
	var sawTrailer, sawStart bool
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "trailer" {
			sawTrailer = true
		}
		if s == "startxref" {
			sawStart = true
		}
	}
	if !sawTrailer || !sawStart {
		t.Fatalf("lines=%q", lines)
	}
}

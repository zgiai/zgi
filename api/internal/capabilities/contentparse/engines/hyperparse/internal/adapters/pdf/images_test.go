package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// minimalJPEG1x1 is a valid baseline JPEG (1x1 pixel).
var minimalJPEG1x1 = []byte{
	0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
	0x00, 0x01, 0x00, 0x00, 0xff, 0xdb, 0x00, 0x43, 0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08,
	0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0a, 0x0c, 0x14, 0x0d, 0x0c, 0x0b, 0x0b, 0x0c, 0x19, 0x12,
	0x13, 0x0f, 0x14, 0x1d, 0x1a, 0x1f, 0x1e, 0x1d, 0x1a, 0x1c, 0x1c, 0x20, 0x24, 0x2e, 0x27, 0x20,
	0x22, 0x2c, 0x23, 0x1c, 0x1c, 0x28, 0x37, 0x29, 0x2c, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1f, 0x27,
	0x39, 0x3d, 0x38, 0x32, 0x3c, 0x2e, 0x33, 0x34, 0x32, 0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xff, 0xc4, 0x00, 0x1f, 0x00, 0x00, 0x01, 0x05, 0x01, 0x01,
	0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04,
	0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0xff, 0xc4, 0x00, 0xb5, 0x10, 0x00, 0x02, 0x01, 0x03,
	0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7d, 0x01, 0x02, 0x03, 0x00,
	0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06, 0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32,
	0x81, 0x91, 0xa1, 0x08, 0x23, 0x42, 0xb1, 0xc1, 0x15, 0x52, 0xd1, 0xf0, 0x24, 0x33, 0x62, 0x72,
	0x82, 0x09, 0x0a, 0x16, 0x17, 0x18, 0x19, 0x1a, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x34, 0x35,
	0x36, 0x37, 0x38, 0x39, 0x3a, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49, 0x4a, 0x53, 0x54, 0x55,
	0x56, 0x57, 0x58, 0x59, 0x5a, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x73, 0x74, 0x75,
	0x76, 0x77, 0x78, 0x79, 0x7a, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x92, 0x93, 0x94,
	0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xb2,
	0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, 0xc8, 0xc9,
	0xca, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xe1, 0xe2, 0xe3, 0xe4, 0xe5, 0xe6,
	0xe7, 0xe8, 0xe9, 0xea, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9, 0xfa, 0xff, 0xda,
	0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3f, 0x00, 0xfb, 0xd3, 0xfc, 0xcf, 0xc0, 0xff, 0xd9,
}

func buildPDFOnePageJPEGImageCompactSubtype() string {
	header := "%PDF-1.4\n"
	L := len(minimalJPEG1x1)
	imgBody := fmt.Sprintf("<</BitsPerComponent 8/ColorSpace/DeviceRGB/Filter/DCTDecode/Height 1/Length %d/Subtype/Image/Type/XObject/Width 1>>\nstream\n%s\nendstream",
		L, string(minimalJPEG1x1))
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources 5 0 R /Contents 4 0 R >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Length 16 >>\nstream\nq 100 0 0 100 0 0 cm /Im1 Do Q\nendstream\nendobj\n"
	obj5 := "5 0 obj\n<< /XObject << /Im1 6 0 R >> >>\nendobj\n"
	obj6 := fmt.Sprintf("6 0 obj\n%s\nendobj\n", imgBody)

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	offset5 := len(header + obj1 + obj2 + obj3 + obj4)
	offset6 := len(header + obj1 + obj2 + obj3 + obj4 + obj5)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6)

	xref := "xref\n0 7\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4) +
		fmt.Sprintf("%010d 00000 n \n", offset5) +
		fmt.Sprintf("%010d 00000 n \n", offset6)
	trailer := "trailer\n<< /Size 7 /Root 1 0 R >>\n"
	return header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + xref + trailer + fmt.Sprintf("startxref\n%d\n", xrefOffset) + "%%EOF\n"
}

// Resources on /Pages parent; /Page leaf has no /Resources (common for PDF 1.5+).
func buildPDFOnePageJPEGInheritedResources() string {
	header := "%PDF-1.4\n"
	L := len(minimalJPEG1x1)
	imgBody := fmt.Sprintf("<</BitsPerComponent 8/ColorSpace/DeviceRGB/Filter/DCTDecode/Height 1/Length %d/Subtype/Image/Type/XObject/Width 1>>\nstream\n%s\nendstream",
		L, string(minimalJPEG1x1))
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] /Resources 5 0 R >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Length 16 >>\nstream\nq 100 0 0 100 0 0 cm /Im1 Do Q\nendstream\nendobj\n"
	obj5 := "5 0 obj\n<< /XObject << /Im1 6 0 R >> >>\nendobj\n"
	obj6 := fmt.Sprintf("6 0 obj\n%s\nendobj\n", imgBody)

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	offset5 := len(header + obj1 + obj2 + obj3 + obj4)
	offset6 := len(header + obj1 + obj2 + obj3 + obj4 + obj5)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6)

	xref := "xref\n0 7\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4) +
		fmt.Sprintf("%010d 00000 n \n", offset5) +
		fmt.Sprintf("%010d 00000 n \n", offset6)
	trailer := "trailer\n<< /Size 7 /Root 1 0 R >>\n"
	return header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + xref + trailer + fmt.Sprintf("startxref\n%d\n", xrefOffset) + "%%EOF\n"
}

func buildPDFOnePageResourcesHasTwoImagesButUseOne() string {
	header := "%PDF-1.4\n"
	L := len(minimalJPEG1x1)
	imgBody := fmt.Sprintf("<</BitsPerComponent 8/ColorSpace/DeviceRGB/Filter/DCTDecode/Height 1/Length %d/Subtype/Image/Type/XObject/Width 1>>\nstream\n%s\nendstream",
		L, string(minimalJPEG1x1))
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources 5 0 R /Contents 4 0 R >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Length 16 >>\nstream\nq 100 0 0 100 0 0 cm /Im1 Do Q\nendstream\nendobj\n"
	obj5 := "5 0 obj\n<< /XObject << /Im1 6 0 R /Im2 7 0 R >> >>\nendobj\n"
	obj6 := fmt.Sprintf("6 0 obj\n%s\nendobj\n", imgBody)
	obj7 := fmt.Sprintf("7 0 obj\n%s\nendobj\n", imgBody)

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	offset5 := len(header + obj1 + obj2 + obj3 + obj4)
	offset6 := len(header + obj1 + obj2 + obj3 + obj4 + obj5)
	offset7 := len(header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + obj7)

	xref := "xref\n0 8\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4) +
		fmt.Sprintf("%010d 00000 n \n", offset5) +
		fmt.Sprintf("%010d 00000 n \n", offset6) +
		fmt.Sprintf("%010d 00000 n \n", offset7)
	trailer := "trailer\n<< /Size 8 /Root 1 0 R >>\n"
	return header + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + obj7 + xref + trailer + fmt.Sprintf("startxref\n%d\n", xrefOffset) + "%%EOF\n"
}

func TestParseObjStmFirstAndN_CompactKeys(t *testing.T) {
	b := []byte("<</Filter/FlateDecode/First 144/Length 1126/N 19/Type/ObjStm>>")
	f, n, ok := parseObjStmFirstAndN(b)
	if !ok || f != 144 || n != 19 {
		t.Fatalf("parseObjStmFirstAndN: f=%d n=%d ok=%v", f, n, ok)
	}
}

func TestExtractEmbeddedImages_CompactSubtype(t *testing.T) {
	data := []byte(buildPDFOnePageJPEGImageCompactSubtype())
	imgs, err := ExtractEmbeddedImagesFromBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 {
		t.Fatalf("want 1 image, got %d", len(imgs))
	}
	if imgs[0].Format != "jpeg" {
		t.Fatalf("format: %q", imgs[0].Format)
	}
	if len(imgs[0].Bytes) < 10 || imgs[0].Bytes[0] != 0xff || imgs[0].Bytes[1] != 0xd8 {
		t.Fatalf("invalid jpeg bytes")
	}
}

func TestExtractEmbeddedImages_InheritedResourcesOnPagesNode(t *testing.T) {
	data := []byte(buildPDFOnePageJPEGInheritedResources())
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].ResourcesRefObject != 5 {
		t.Fatalf("Resources should resolve via /Pages parent: %+v", specs)
	}
	imgs, err := ExtractEmbeddedImagesFromBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 {
		t.Fatalf("want 1 image, got %d", len(imgs))
	}
	if imgs[0].Format != "jpeg" {
		t.Fatalf("format: %q", imgs[0].Format)
	}
}

// Inline /Resources on /Page (no separate Resources object number) — common for some generators.
func buildPDFOnePageJPEGInlineResourcesCompact() string {
	header := "%PDF-1.4\n"
	L := len(minimalJPEG1x1)
	imgBody := fmt.Sprintf("<</BitsPerComponent 8/ColorSpace/DeviceRGB/Filter/DCTDecode/Height 1/Length %d/Subtype/Image/Type/XObject/Width 1>>\nstream\n%s\nendstream",
		L, string(minimalJPEG1x1))
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<</Type/Page/MediaBox[0 0 100 100]/Resources<</XObject<</Im1 5 0 R>>>>/Contents 4 0 R>>\nendobj\n"
	obj4 := "4 0 obj\n<< /Length 16 >>\nstream\nq 100 0 0 100 0 0 cm /Im1 Do Q\nendstream\nendobj\n"
	obj5 := fmt.Sprintf("5 0 obj\n%s\nendobj\n", imgBody)

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	offset5 := len(header + obj1 + obj2 + obj3 + obj4)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4 + obj5)

	xref := "xref\n0 6\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4) +
		fmt.Sprintf("%010d 00000 n \n", offset5)
	trailer := "trailer\n<< /Size 6 /Root 1 0 R >>\n"
	return header + obj1 + obj2 + obj3 + obj4 + obj5 + xref + trailer + fmt.Sprintf("startxref\n%d\n", xrefOffset) + "%%EOF\n"
}

func buildPDFOnePageFlateRaster8x8InlineResources() string {
	rawPix := bytes.Repeat([]byte{0x7f}, 8*8)
	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	_, _ = zw.Write(rawPix)
	_ = zw.Close()
	enc := zbuf.Bytes()

	header := "%PDF-1.4\n"
	imgBody := fmt.Sprintf("<</Type/XObject/Subtype/Image/Width 8/Height 8/ColorSpace/DeviceGray/BitsPerComponent 8/Filter/FlateDecode/Length %d>>\nstream\n%s\nendstream",
		len(enc), string(enc))
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<</Type/Page/MediaBox[0 0 100 100]/Resources<</XObject<</Im1 5 0 R>>>>/Contents 4 0 R>>\nendobj\n"
	obj4 := "4 0 obj\n<< /Length 16 >>\nstream\nq 8 0 0 8 0 0 cm /Im1 Do Q\nendstream\nendobj\n"
	obj5 := fmt.Sprintf("5 0 obj\n%s\nendobj\n", imgBody)

	offset1 := len(header)
	offset2 := len(header + obj1)
	offset3 := len(header + obj1 + obj2)
	offset4 := len(header + obj1 + obj2 + obj3)
	offset5 := len(header + obj1 + obj2 + obj3 + obj4)
	xrefOffset := len(header + obj1 + obj2 + obj3 + obj4 + obj5)

	xref := "xref\n0 6\n" +
		"0000000000 65535 f \n" +
		fmt.Sprintf("%010d 00000 n \n", offset1) +
		fmt.Sprintf("%010d 00000 n \n", offset2) +
		fmt.Sprintf("%010d 00000 n \n", offset3) +
		fmt.Sprintf("%010d 00000 n \n", offset4) +
		fmt.Sprintf("%010d 00000 n \n", offset5)
	trailer := "trailer\n<< /Size 6 /Root 1 0 R >>\n"
	return header + obj1 + obj2 + obj3 + obj4 + obj5 + xref + trailer + fmt.Sprintf("startxref\n%d\n", xrefOffset) + "%%EOF\n"
}

func TestExtractEmbeddedImages_FlateRasterInlineResources(t *testing.T) {
	data := []byte(buildPDFOnePageFlateRaster8x8InlineResources())
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	light, err := ExtractEmbeddedImagesFromBytesWithSpecsLight(data, ValidationModeRelaxed, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(light) != 1 || light[0].Format != "pdf_raster" || light[0].Width != 8 || light[0].Height != 8 {
		t.Fatalf("light: %+v", light)
	}
	full, err := ExtractEmbeddedImagesFromBytesWithSpecs(data, ValidationModeRelaxed, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 1 || full[0].Format != "png" {
		t.Fatalf("full: %+v", full)
	}
	if len(full[0].Bytes) < 8 || string(full[0].Bytes[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("full bytes not PNG: %v", full[0].Bytes[:min(8, len(full[0].Bytes))])
	}
}

func TestExtractEmbeddedImages_InlineResourcesOnPage(t *testing.T) {
	data := []byte(buildPDFOnePageJPEGInlineResourcesCompact())
	specs, err := DetectPageRenderSpecsBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].ResourcesRefObject != 0 {
		t.Fatalf("want inline Resources (resRef=0), got specs=%v", specs)
	}
	imgs, err := ExtractEmbeddedImagesFromBytesWithSpecsLight(data, ValidationModeRelaxed, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 {
		t.Fatalf("want 1 image with inline /Resources, got %d", len(imgs))
	}
	if imgs[0].XObjectName != "Im1" || imgs[0].ObjectNumber != 5 || imgs[0].Format != "jpeg" {
		t.Fatalf("unexpected: %+v", imgs[0])
	}
}

func TestExtractEmbeddedImages_OnlyDoReferencedXObjects(t *testing.T) {
	data := []byte(buildPDFOnePageResourcesHasTwoImagesButUseOne())
	imgs, err := ExtractEmbeddedImagesFromBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 {
		t.Fatalf("want 1 image (only /Im1 used by Do), got %d", len(imgs))
	}
	if imgs[0].XObjectName != "Im1" || imgs[0].ObjectNumber != 6 {
		t.Fatalf("unexpected selected image: name=%q obj=%d", imgs[0].XObjectName, imgs[0].ObjectNumber)
	}
}

func TestDuizhangPDF_embeddedImagesIfPresent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdoc", "对账单.pdf")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip("testdoc/对账单.pdf not available")
	}
	specs, err := DetectPageRenderSpecsBytes(b, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	light, err := ExtractEmbeddedImagesFromBytesWithSpecsLight(b, ValidationModeRelaxed, specs)
	if err != nil {
		t.Fatal(err)
	}
	if len(light) < 2 {
		t.Fatalf("对账单.pdf: want >= 2 Flate image XObjects in light scan, got %d", len(light))
	}
	for _, im := range light {
		if im.Format != "pdf_raster" {
			t.Fatalf("unexpected format %q for %s obj %d", im.Format, im.XObjectName, im.ObjectNumber)
		}
	}
}

func TestExtractEmbeddedImages_LabReportPDFIfPresent(t *testing.T) {
	path := filepath.Join("..", "..", "..", "LabReport.pdf")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip("LabReport.pdf not in repo root; skipping integration test")
	}
	rb, ok := findResourcesObjectBlockForImageScan(b, 30)
	if !ok {
		t.Fatal("expected Resources 30 resolved for image scan")
	}
	body := objectBodyFromBlockBytes(rb)
	xFrag := extractInlineDictAfterKey(body, "/XObject")
	if xFrag == "" {
		t.Fatalf("no /XObject fragment; body prefix: %.200q", body)
	}
	refs := parseNamedXObjectRefs(xFrag)
	if len(refs) < 1 {
		t.Fatalf("no xobject refs in %q", xFrag)
	}
	imgs, err := ExtractEmbeddedImagesFromBytes(b, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) < 1 {
		t.Fatalf("LabReport.pdf: want at least 1 image, got %d (refs=%d)", len(imgs), len(refs))
	}
}

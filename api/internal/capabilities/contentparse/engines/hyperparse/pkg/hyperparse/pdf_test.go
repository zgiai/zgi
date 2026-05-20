package hyperparse

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPDFHasOversizedPagesRelaxed(t *testing.T) {
	normalPDF := buildPDFWithPageMediaBoxes([]string{"0 0 595 842"})
	if PDFHasOversizedPagesRelaxed(normalPDF) {
		t.Fatalf("normal A4-ish page should not be treated as oversized")
	}

	oversizedPDF := buildPDFWithPageMediaBoxes([]string{"0 0 3213 5712"})
	if !PDFHasOversizedPagesRelaxed(oversizedPDF) {
		t.Fatalf("expected oversized page to be detected")
	}
}

func TestInspectPDFFullDocumentNativeOnlyIgnoresForceVLM(t *testing.T) {
	t.Setenv("DOCSTILL_FORCE_VLM", "1")

	fullDoc, err := InspectPDFFullDocument(context.Background(), InspectPDFOptions{
		Filename:   "native-only.pdf",
		Data:       buildPDFWithPageMediaBoxes([]string{"0 0 595 842"}),
		Mode:       "relaxed",
		NativeOnly: true,
	})
	if err != nil {
		t.Fatalf("InspectPDFFullDocument NativeOnly failed: %v", err)
	}
	doc, _ := fullDoc["document"].(map[string]any)
	if doc == nil {
		t.Fatalf("missing document payload")
	}
	if got, _ := doc["force_vlm"].(bool); got {
		t.Fatalf("NativeOnly should ignore DOCSTILL_FORCE_VLM")
	}
}

func buildPDFWithPageMediaBoxes(mediaBoxes []string) []byte {
	var sb strings.Builder
	sb.WriteString("%PDF-1.4\n")

	offsets := map[int]int{}
	writeObj := func(objNum int, body string) {
		offsets[objNum] = sb.Len()
		fmt.Fprintf(&sb, "%d 0 obj\n%s\nendobj\n", objNum, body)
	}

	kids := make([]string, 0, len(mediaBoxes))
	for idx := range mediaBoxes {
		kids = append(kids, fmt.Sprintf("%d 0 R", idx+3))
	}

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(mediaBoxes), strings.Join(kids, " ")))

	for idx, mediaBox := range mediaBoxes {
		pageObjNum := idx + 3
		writeObj(pageObjNum, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [%s] >>", mediaBox))
	}

	xrefPos := sb.Len()
	totalObjs := len(mediaBoxes) + 2
	fmt.Fprintf(&sb, "xref\n0 %d\n", totalObjs+1)
	sb.WriteString("0000000000 65535 f \n")
	for objNum := 1; objNum <= totalObjs; objNum++ {
		fmt.Fprintf(&sb, "%010d 00000 n \n", offsets[objNum])
	}
	fmt.Fprintf(&sb, "trailer\n<< /Size %d /Root 1 0 R >>\n", totalObjs+1)
	fmt.Fprintf(&sb, "startxref\n%d\n", xrefPos)
	sb.WriteString("%%EOF\n")
	return []byte(sb.String())
}

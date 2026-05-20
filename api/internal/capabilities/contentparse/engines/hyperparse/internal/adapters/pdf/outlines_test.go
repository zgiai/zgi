package pdf

import (
	"fmt"
	"strings"
	"testing"
)

func buildPDFWithOneOutlineBookmark() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Outlines 5 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Title (Chapter 1) /Parent 5 0 R /Dest [3 0 R /XYZ 0 0 null] >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /Outlines /First 4 0 R /Last 4 0 R >>\nendobj\n"

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
	startxref := fmt.Sprintf("startxref\n%d\n", xrefOffset)

	return header + obj1 + obj2 + obj3 + obj4 + obj5 + xref + trailer + startxref + "%%EOF\n"
}

func TestExtractOutlineEntriesFromBytes_OneItem(t *testing.T) {
	data := []byte(buildPDFWithOneOutlineBookmark())
	entries, err := ExtractOutlineEntriesFromBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d: %+v", len(entries), entries)
	}
	if entries[0].Title != "Chapter 1" {
		t.Fatalf("title: %q", entries[0].Title)
	}
	if entries[0].Level != 0 {
		t.Fatalf("level: %d", entries[0].Level)
	}
	if entries[0].PageObject != 3 {
		t.Fatalf("page object: %d", entries[0].PageObject)
	}
	if entries[0].Object != 4 {
		t.Fatalf("outline item object: %d", entries[0].Object)
	}
	if entries[0].TargetKind != "page" {
		t.Fatalf("target kind: %q", entries[0].TargetKind)
	}
	if entries[0].TargetRaw != "[3 0 R /XYZ 0 0 null]" {
		t.Fatalf("target raw: %q", entries[0].TargetRaw)
	}
}

func TestExtractOutlineEntriesFromBytes_ActionGoTo(t *testing.T) {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Outlines 5 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>\nendobj\n"
	obj4 := "4 0 obj\n<< /Title (GoTo) /Parent 5 0 R /A << /S /GoTo /D [3 0 R /XYZ 0 0 null] >> >>\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /Outlines /First 4 0 R /Last 4 0 R >>\nendobj\n"
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
	data := []byte(header + obj1 + obj2 + obj3 + obj4 + obj5 + xref + "trailer\n<< /Size 6 /Root 1 0 R >>\n" + fmt.Sprintf("startxref\n%d\n", xrefOffset) + "%%EOF\n")

	entries, err := ExtractOutlineEntriesFromBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].PageObject != 3 {
		t.Fatalf("page object from action /D not parsed: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Dest, "/A << /S /GoTo /D [3 0 R") {
		t.Fatalf("dest snippet should contain action: %q", entries[0].Dest)
	}
	if entries[0].TargetKind != "page" {
		t.Fatalf("target kind: %q", entries[0].TargetKind)
	}
	if entries[0].TargetRaw == "" || !strings.HasPrefix(entries[0].TargetRaw, "<< /S /GoTo /D [3 0 R") {
		t.Fatalf("target raw should preserve action dict: %q", entries[0].TargetRaw)
	}
}

func TestExtractOutlineEntriesFromBytes_NoOutlines(t *testing.T) {
	data := []byte(buildPDFWithInfoMetadata())
	entries, err := ExtractOutlineEntriesFromBytes(data, ValidationModeRelaxed)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("want 0 entries, got %d", len(entries))
	}
}

func TestAppendIncrementalOutlinesFromEntries(t *testing.T) {
	src := []byte(buildPDFWithInfoMetadata())
	out, err := AppendIncrementalOutlinesFromEntries(src, []OutlineWriteEntry{
		{Title: "A", Level: 0, PageIndex: 1},
		{Title: "A.1", Level: 1, PageIndex: 1},
		{Title: "B", Level: 0, PageIndex: 1},
	})
	if err != nil {
		t.Fatalf("append outlines: %v", err)
	}
	got, err := ExtractOutlineEntriesFromBytes(out, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("read outlines: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 entries, got %d", len(got))
	}
	if got[0].Title != "A" || got[0].Level != 0 {
		t.Fatalf("entry0 mismatch: %+v", got[0])
	}
	if got[1].Title != "A.1" || got[1].Level != 1 {
		t.Fatalf("entry1 mismatch: %+v", got[1])
	}
	if got[2].Title != "B" || got[2].Level != 0 {
		t.Fatalf("entry2 mismatch: %+v", got[2])
	}
}

func TestAppendIncrementalOutlinesFromEntries_PreserveCatalogFields(t *testing.T) {
	src := []byte(buildPDFWithCatalogExtraFields())
	out, err := AppendIncrementalOutlinesFromEntries(src, []OutlineWriteEntry{
		{Title: "Root", Level: 0, PageIndex: 1},
	})
	if err != nil {
		t.Fatalf("append outlines: %v", err)
	}
	rootObj, ok := parseRootObjectFromTrailer(out)
	if !ok {
		t.Fatal("cannot parse new root")
	}
	catBlock, ok := findObjectBlockByNumber(out, rootObj)
	if !ok {
		t.Fatal("cannot find new catalog")
	}
	s := string(catBlock)
	if !strings.Contains(s, "/PageMode /UseOutlines") {
		t.Fatalf("catalog field not preserved: %q", s)
	}
	if !strings.Contains(s, "/Lang (zh-CN)") {
		t.Fatalf("catalog field not preserved: %q", s)
	}
	if !strings.Contains(s, "/Outlines ") {
		t.Fatalf("catalog outlines ref missing: %q", s)
	}
}

func TestDetectNextObjectNumberByScan(t *testing.T) {
	in := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\n12 0 obj\n<<>>\nendobj\n%%EOF\n")
	got := detectNextObjectNumberByScan(in)
	if got != 13 {
		t.Fatalf("detectNextObjectNumberByScan: got %d want 13", got)
	}
}

func TestAppendIncrementalOutlinesFromEntries_DestRaw(t *testing.T) {
	src := []byte(buildPDFWithInfoMetadata())
	out, err := AppendIncrementalOutlinesFromEntries(src, []OutlineWriteEntry{
		{Title: "Named", Level: 0, Dest: "/MyNamedDest"},
	})
	if err != nil {
		t.Fatalf("append outlines with dest: %v", err)
	}
	got, err := ExtractOutlineEntriesFromBytes(out, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("read outlines: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].Title != "Named" {
		t.Fatalf("title: %q", got[0].Title)
	}
	if got[0].Dest == "" || !strings.Contains(got[0].Dest, "/Dest /MyNamedDest") {
		t.Fatalf("dest not preserved, got: %q", got[0].Dest)
	}
	if got[0].TargetKind != "dest" {
		t.Fatalf("target kind: %q", got[0].TargetKind)
	}
	if got[0].TargetRaw != "/MyNamedDest" {
		t.Fatalf("target raw: %q", got[0].TargetRaw)
	}
}

func TestAppendIncrementalOutlinesFromEntries_ActionRaw(t *testing.T) {
	src := []byte(buildPDFWithInfoMetadata())
	out, err := AppendIncrementalOutlinesFromEntries(src, []OutlineWriteEntry{
		{Title: "Act", Level: 0, Action: "<< /S /GoTo /D [3 0 R /XYZ null null null] >>"},
	})
	if err != nil {
		t.Fatalf("append outlines with action: %v", err)
	}
	got, err := ExtractOutlineEntriesFromBytes(out, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("read outlines: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].PageObject != 3 {
		t.Fatalf("page object from written action /D not parsed: %+v", got[0])
	}
	if !strings.Contains(got[0].Dest, "/A << /S /GoTo /D [3 0 R") {
		t.Fatalf("action not preserved in snippet: %q", got[0].Dest)
	}
}

func TestAppendIncrementalOutlinesFromEntries_TargetRawDestRoundtrip(t *testing.T) {
	src := []byte(buildPDFWithInfoMetadata())
	out, err := AppendIncrementalOutlinesFromEntries(src, []OutlineWriteEntry{
		{Title: "RawDest", Level: 0, Dest: "/MyNamedDest"},
	})
	if err != nil {
		t.Fatalf("append outlines with raw dest: %v", err)
	}
	got, err := ExtractOutlineEntriesFromBytes(out, ValidationModeRelaxed)
	if err != nil {
		t.Fatalf("read outlines: %v", err)
	}
	if len(got) != 1 || got[0].TargetRaw != "/MyNamedDest" {
		t.Fatalf("target_raw roundtrip mismatch: %+v", got)
	}
}

func buildPDFWithCatalogExtraFields() string {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R /PageMode /UseOutlines /Lang (zh-CN) >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Count 1 /Kids [3 0 R] >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] >>\nendobj\n"
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

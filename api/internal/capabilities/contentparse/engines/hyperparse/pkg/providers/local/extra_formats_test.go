package local

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestSupportsLocalExtraExt(t *testing.T) {
	if !supportsLocalExtraExt(".xls") || !supportsLocalExtraExt(".ppt") || !supportsLocalExtraExt(".xlsx") {
		t.Fatalf("expected office extensions to be supported")
	}
	if supportsLocalExtraExt(".pdf") {
		t.Fatalf("pdf should not be handled by extra ext path")
	}
}

func TestParseDelimitedRowsAsDoc(t *testing.T) {
	doc, err := parseDelimitedRowsAsDoc("a.csv", []byte("姓名,年龄,职位\n杨一,23,高管\n张三,89,管事"), ',', "local:csv")
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if doc.Chunks[0].Type != "table" {
		t.Fatalf("chunk type=%q", doc.Chunks[0].Type)
	}
	if got, want := doc.Chunks[0].Text, `"姓名":"杨一";"年龄":"23";"职位":"高管"`; got != want {
		t.Fatalf("chunk text=%q, want %q", got, want)
	}
	if strings.Contains(doc.Markdown, "姓名,年龄,职位") {
		t.Fatalf("markdown should not contain raw header row: %q", doc.Markdown)
	}
}

func TestParseXLSXRowsAsDoc(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	if err := f.SetSheetRow(sheet, "A1", &[]any{"姓名", "年龄", "职位"}); err != nil {
		t.Fatalf("set header: %v", err)
	}
	if err := f.SetSheetRow(sheet, "A2", &[]any{"杨一", 23, "高管"}); err != nil {
		t.Fatalf("set row 1: %v", err)
	}
	if err := f.SetSheetRow(sheet, "A3", &[]any{"张三", 89, "管事"}); err != nil {
		t.Fatalf("set row 2: %v", err)
	}
	if err := f.Write(buf); err != nil {
		t.Fatalf("write xlsx: %v", err)
	}

	doc, err := parseXLSXRowsAsDoc("a.xlsx", buf.Bytes())
	if err != nil {
		t.Fatalf("parse xlsx: %v", err)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
	}
	if got, want := doc.Chunks[0].Text, `"姓名":"杨一";"年龄":"23";"职位":"高管"`; got != want {
		t.Fatalf("chunk text=%q, want %q", got, want)
	}

	enriched := extractcommon.EnrichStructuredOutput(doc)
	if enriched.ExtractOutput == nil || len(enriched.ExtractOutput.Elements) != 2 {
		t.Fatalf("extract output=%+v", enriched.ExtractOutput)
	}
	if enriched.ExtractOutput.Elements[0].Type != "table" {
		t.Fatalf("element type=%q", enriched.ExtractOutput.Elements[0].Type)
	}
}

func TestExtractTextFromZipXML(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("ppt/slides/slide1.xml")
	_, _ = w.Write([]byte(`<p:sp><a:t>Hello</a:t><a:t>World</a:t></p:sp>`))
	_ = zw.Close()

	got := extractTextFromZipXML(buf.Bytes(), func(name string) bool {
		return name == "ppt/slides/slide1.xml"
	})
	if len(got) < 2 {
		t.Fatalf("expected >=2 tokens, got %v", got)
	}
}

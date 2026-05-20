package local

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestSupportsLocalExtraExt(t *testing.T) {
	if !supportsLocalExtraExt(".xls") || !supportsLocalExtraExt(".ppt") || !supportsLocalExtraExt(".xlsx") {
		t.Fatalf("expected office extensions to be supported")
	}
	if supportsLocalExtraExt(".pdf") {
		t.Fatalf("pdf should not be handled by extra ext path")
	}
}

func TestParseDelimitedAsDoc(t *testing.T) {
	doc, err := parseDelimitedAsDoc("a.csv", []byte("a,b,c\n1,2,3"), ",", "local:csv")
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(doc.Chunks) != 2 {
		t.Fatalf("chunks=%d", len(doc.Chunks))
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

package pdf

import (
	"testing"
)

func TestReadDocumentMetadataBytes(t *testing.T) {
	data := []byte(buildPDFWithInfoMetadata())
	m, err := ReadDocumentMetadataBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if m["Title"] != "ZGI Parse Test" {
		t.Fatalf("Title: got %q", m["Title"])
	}
	if m["Author"] != "ZGI" {
		t.Fatalf("Author: got %q", m["Author"])
	}
	if m["Producer"] != "ZGIParseEngine" {
		t.Fatalf("Producer: got %q", m["Producer"])
	}
}

func TestAppendIncrementalInfoMetadata(t *testing.T) {
	data := []byte(buildPDFWithInfoMetadata())
	out, err := AppendIncrementalInfoMetadata(data, map[string]string{"Title": "Patched Title"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := ReadDocumentMetadataBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	if m["Title"] != "Patched Title" {
		t.Fatalf("after incremental, Title: got %q", m["Title"])
	}
}

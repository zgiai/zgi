package pdf

import (
	"bytes"
	"compress/zlib"
	"testing"
)

func TestParseFilterPipeline_ArrayOrder(t *testing.T) {
	dict := []byte("<< /Filter [/ASCII85Decode /FlateDecode] /Length 1 >>")
	got := ParseFilterPipeline(dict)
	if len(got) != 2 || got[0] != "ascii85" || got[1] != "flate" {
		t.Fatalf("want [ascii85 flate], got %v", got)
	}
}

func TestParseFilterPipeline_SingleName(t *testing.T) {
	dict := []byte("<< /Filter /FlateDecode /Length 10 >>")
	got := ParseFilterPipeline(dict)
	if len(got) != 1 || got[0] != "flate" {
		t.Fatalf("want [flate], got %v", got)
	}
}

func TestParseFilterPipeline_CompactFilterFlateNoSpace(t *testing.T) {
	dict := []byte("<< /Filter/FlateDecode /Length 1889 >>")
	got := ParseFilterPipeline(dict)
	if len(got) != 1 || got[0] != "flate" {
		t.Fatalf("want [flate], got %v", got)
	}
}

func TestDecodeStreamFilters_Flate(t *testing.T) {
	plain := []byte("hello pdf flate")
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(plain); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dict := []byte("<< /Filter /FlateDecode >>")
	got, err := DecodeStreamFilters(dict, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("got %q want %q", got, plain)
	}
}

func TestDecodeStreamFilters_DCTPassthrough(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff, 0xe0}
	dict := []byte("<< /Filter /DCTDecode >>")
	got, err := DecodeStreamFilters(dict, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("dct passthrough mismatch")
	}
}

func TestDecodeRunLengthPDF(t *testing.T) {
	in := []byte{2, 'a', 'b', 'c', 255, 0xab, 128}
	out, err := decodeRunLengthPDF(in)
	if err != nil {
		t.Fatal(err)
	}
	want := "abc\xab\xab"
	if string(out) != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

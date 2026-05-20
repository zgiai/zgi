package pdf

import "testing"

func TestExtractBaseFontNameFromFontObjectBlock(t *testing.T) {
	b := []byte(`<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>`)
	if got := extractBaseFontNameFromFontObjectBlock(b); got != "Helvetica" {
		t.Fatalf("got %q", got)
	}
}

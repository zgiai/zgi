package pdf

import (
	"bytes"
	"testing"
)

func TestPdfRasterDecodedToPNG_gray(t *testing.T) {
	dict := []byte("/Width 2/Height 2/ColorSpace/DeviceGray/BitsPerComponent 8")
	pix := []byte{0, 64, 128, 255}
	pngb, err := pdfRasterDecodedToPNG(dict, pix)
	if err != nil {
		t.Fatal(err)
	}
	if len(pngb) < 8 || !bytes.HasPrefix(pngb, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatalf("not png: %v", pngb[:min(16, len(pngb))])
	}
}

func TestPdfRasterDecodedToPNG_rgb(t *testing.T) {
	dict := []byte("/Width 1/Height 1/ColorSpace/DeviceRGB/BitsPerComponent 8")
	pix := []byte{10, 20, 30}
	pngb, err := pdfRasterDecodedToPNG(dict, pix)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pngb, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("not png")
	}
}

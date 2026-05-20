package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
)

// pdfRasterDecodedToPNG converts decoded raw raster pixels to PNG for vision
// interfaces that accept only JPEG/PNG. It supports common 8 bpc DeviceGray and
// DeviceRGB data; RGB with /SMask is still treated as raw RGB samples.
func pdfRasterDecodedToPNG(dict, pix []byte) ([]byte, error) {
	w := parseIntKey(dict, "/Width")
	h := parseIntKey(dict, "/Height")
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("pdf raster: invalid width/height")
	}
	bpc := parseIntKey(dict, "/BitsPerComponent")
	if bpc <= 0 {
		bpc = 8
	}
	if bpc != 8 {
		return nil, fmt.Errorf("pdf raster: unsupported BitsPerComponent=%d", bpc)
	}
	// Do not parse /Indexed, /ICCBased, /Separation, and similar color spaces yet.
	if bytes.Contains(dict, []byte("/ColorSpace[")) || bytes.Contains(dict, []byte("/ColorSpace [")) {
		return nil, fmt.Errorf("pdf raster: array ColorSpace not supported")
	}

	switch {
	case bytes.Contains(dict, []byte("/DeviceGray")):
		want := w * h
		if len(pix) < want {
			return nil, fmt.Errorf("pdf raster: gray want %d bytes got %d", want, len(pix))
		}
		g := image.NewGray(image.Rect(0, 0, w, h))
		copy(g.Pix, pix[:want])
		var buf bytes.Buffer
		if err := png.Encode(&buf, g); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	case bytes.Contains(dict, []byte("/DeviceRGB")):
		want := w * h * 3
		if len(pix) < want {
			return nil, fmt.Errorf("pdf raster: rgb want %d bytes got %d", want, len(pix))
		}
		rgba := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				o := (y*w + x) * 3
				i := y*rgba.Stride + x*4
				rgba.Pix[i] = pix[o]
				rgba.Pix[i+1] = pix[o+1]
				rgba.Pix[i+2] = pix[o+2]
				rgba.Pix[i+3] = 255
			}
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, rgba); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	default:
		return nil, fmt.Errorf("pdf raster: unsupported ColorSpace (need DeviceGray or DeviceRGB)")
	}
}

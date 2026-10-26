package socialmcp

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxImageSourceBytes = 8 << 20
	maxImageBytes       = 147456
	maxImageDataChars   = 196608
	maxImageEdge        = 8192
	maxImagePixels      = 24000000
	preparedImageEdge   = 1600
)

// PreparedImage contains bounded, metadata-free image bytes for native social
// operation inputs. Bytes counts the decoded base64 payload, not source bytes.
type PreparedImage struct {
	ImageData string `json:"image_data"`
	MIMEType  string `json:"mime_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Bytes     int    `json:"bytes"`
}

// PrepareImage validates and re-encodes a submitted JPEG, PNG or WebP. It never
// resolves URLs or preserves source metadata. Large/complex alpha images fall
// back to JPEG composited on white when PNG cannot fit the bridge payload cap.
func PrepareImage(data []byte) (PreparedImage, error) {
	img, format, err := decodeImage(data)
	if err != nil {
		return PreparedImage{}, err
	}
	return prepareDecodedImage(img, format)
}

func prepareDecodedImage(img image.Image, format string) (PreparedImage, error) {
	img = resizeImage(img, preparedImageEdge)
	if format != "jpeg" {
		var out cappedImageBuffer
		if png.Encode(&out, img) == nil {
			return preparedImage(out.Bytes(), "image/png", img.Bounds()), nil
		}
	}
	// JPEG cannot represent alpha. A white background avoids turning transparent
	// pixels black and keeps the result usable on social platforms.
	opaque := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(opaque, opaque.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(opaque, opaque.Bounds(), img, img.Bounds().Min, draw.Over)
	img = opaque
	for {
		for _, quality := range []int{90, 75, 60, 45} {
			var out cappedImageBuffer
			if jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}) == nil {
				return preparedImage(out.Bytes(), "image/jpeg", img.Bounds()), nil
			}
		}
		edge := max(img.Bounds().Dx(), img.Bounds().Dy())
		if edge <= 1 {
			return PreparedImage{}, providers.Invalid()
		}
		img = resizeImage(img, max(1, edge*3/4))
	}
}

func decodeImage(data []byte) (image.Image, string, error) {
	if len(data) == 0 || len(data) > maxImageSourceBytes {
		return nil, "", providers.Invalid()
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageEdge || cfg.Height > maxImageEdge || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, "", providers.Invalid()
	}
	img, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || img.Bounds().Dx() != cfg.Width || img.Bounds().Dy() != cfg.Height {
		return nil, "", providers.Invalid()
	}
	return img, format, nil
}

func resizeImage(img image.Image, edge int) image.Image {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= edge && h <= edge {
		return img
	}
	if w >= h {
		h = max(1, h*edge/w)
		w = edge
	} else {
		w = max(1, w*edge/h)
		h = edge
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	return dst
}

// Encoders stop as soon as the final payload cap is exceeded, without retaining
// an unbounded intermediate PNG or JPEG.
type cappedImageBuffer struct{ bytes.Buffer }

func (b *cappedImageBuffer) Write(p []byte) (int, error) {
	if len(p) > maxImageBytes-b.Len() {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(p)
}
func preparedImage(data []byte, mime string, bounds image.Rectangle) PreparedImage {
	return PreparedImage{ImageData: base64.StdEncoding.EncodeToString(data), MIMEType: mime, Width: bounds.Dx(), Height: bounds.Dy(), Bytes: len(data)}
}

func normalizeImageInput(input map[string]any) error {
	value, hasData := input["image_data"]
	mime, hasMIME := input["mime_type"]
	if !hasData {
		if hasMIME {
			return providers.Invalid()
		}
		return nil
	}
	encoded, ok := value.(string)
	if !ok || !hasMIME || len(encoded) == 0 || len(encoded) > maxImageDataChars {
		return providers.Invalid()
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) > maxImageBytes || base64.StdEncoding.EncodeToString(raw) != encoded {
		return providers.Invalid()
	}
	// Compare the declared type against the source before any normalization.
	img, format, err := decodeImage(raw)
	if err != nil || mime != "image/"+format {
		return providers.Invalid()
	}
	prepared, err := prepareDecodedImage(img, format)
	if err != nil {
		return err
	}
	input["image_data"], input["mime_type"] = prepared.ImageData, prepared.MIMEType
	return nil
}

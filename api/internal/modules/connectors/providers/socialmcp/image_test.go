package socialmcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/pkg/mcpclient"
)

func imageBytes(t *testing.T, img image.Image, format string) []byte {
	t.Helper()
	var b bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func checkPrepared(t *testing.T, data []byte) PreparedImage {
	t.Helper()
	p, err := PrepareImage(data)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(p.ImageData)
	if err != nil || len(raw) > 147456 || len(p.ImageData) > 196608 || p.Bytes != len(raw) {
		t.Fatalf("invalid encoded bounds: %#v %v", p, err)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if p.MIMEType != "image/"+format || p.Width != img.Bounds().Dx() || p.Height != img.Bounds().Dy() || p.Width > 1600 || p.Height > 1600 {
		t.Fatal("invalid prepared image dimensions or MIME")
	}
	return p
}
func TestPrepareImageFormatsAndAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	img.Set(1, 1, color.NRGBA{R: 200, A: 100})
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			p := checkPrepared(t, imageBytes(t, img, format))
			if format == "png" {
				if p.MIMEType != "image/png" {
					t.Fatal("lost alpha format")
				}
				b, _ := base64.StdEncoding.DecodeString(p.ImageData)
				got, _, _ := image.Decode(bytes.NewReader(b))
				_, _, _, a := got.At(1, 1).RGBA()
				if a == 0xffff || a == 0 {
					t.Fatal("lost transparency")
				}
			}
		})
	}
	// A complete 1x1 lossless WebP fixture.
	webp, _ := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4TBUAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
	checkPrepared(t, webp)
}
func TestPrepareImageRejectsMalformedAndBombs(t *testing.T) {
	pngData := imageBytes(t, image.NewRGBA(image.Rect(0, 0, 4, 4)), "png")
	oversized := append([]byte(nil), pngData...)
	binary.BigEndian.PutUint32(oversized[16:20], 8193)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	pixels := append([]byte(nil), pngData...)
	binary.BigEndian.PutUint32(pixels[16:20], 6000)
	binary.BigEndian.PutUint32(pixels[20:24], 6000)
	binary.BigEndian.PutUint32(pixels[29:33], crc32.ChecksumIEEE(pixels[12:29]))
	webp, _ := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4TBUAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
	jpg := imageBytes(t, image.NewRGBA(image.Rect(0, 0, 4, 4)), "jpeg")
	for name, data := range map[string][]byte{"empty": nil, "header": pngData[:33], "truncated PNG": pngData[:len(pngData)/2], "truncated JPEG": jpg[:len(jpg)/2], "truncated WebP": webp[:len(webp)/2], "edge": oversized, "pixels": pixels, "large": make([]byte, (8<<20)+1), "URL": []byte("https://example.com/private.png")} {
		t.Run(name, func(t *testing.T) {
			_, err := PrepareImage(data)
			if err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}
func TestPrepareImageCompressesEntropy(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1700, 1100))
	r := rand.New(rand.NewSource(42))
	_, _ = r.Read(img.Pix)
	checkPrepared(t, imageBytes(t, img, "png"))
}
func TestInvokeRejectsInvalidImageBeforeBridge(t *testing.T) {
	c, err := New("x-social", Options{})
	if err != nil {
		t.Fatal(err)
	}
	c.open = func(context.Context, string) (*mcpclient.Connection, func(), error) {
		t.Fatal("invalid image started bridge")
		return nil, nil, nil
	}
	good := base64.StdEncoding.EncodeToString(imageBytes(t, image.NewRGBA(image.Rect(0, 0, 2, 2)), "png"))
	for _, in := range []map[string]any{{"text": "hi", "image_data": "iVBORw0KGgo=", "mime_type": "image/png"}, {"text": "hi", "image_data": good, "mime_type": "image/jpeg"}, {"text": "hi", "mime_type": "image/png"}, {"text": "hi", "image_data": good}, {"text": "hi", "image_data": strings.Repeat("A", 196609), "mime_type": "image/png"}, {"text": "hi", "image_data": good + "\n", "mime_type": "image/png"}} {
		_, err := c.Invoke(t.Context(), "x-social.post.create", "token", in)
		if err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
			t.Fatalf("accepted image: %v", err)
		}
	}
}
func TestInvokeImageLeavesCallerInputUntouched(t *testing.T) {
	c, calls := fixture(t, "", map[string]any{"id": "post"}, false)
	data := "UklGRiIAAABXRUJQVlA4TBUAAAAvAAAAAAfQ//73v/+BiOh/AAA="
	input := map[string]any{"text": "hi", "image_data": data, "mime_type": "image/webp"}
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "untouched")
	if _, err := c.Invoke(ctx, "x-social.post.create", "token", input); err != nil {
		t.Fatal(err)
	}
	if input["image_data"] != data || input["mime_type"] != "image/webp" || len(input) != 3 || ctx.Value(contextKey{}) != "untouched" || *calls != 1 {
		t.Fatal("caller input or context changed")
	}
}

func TestPrepareImageStripsSourceMetadata(t *testing.T) {
	jpg := imageBytes(t, image.NewRGBA(image.Rect(0, 0, 4, 4)), "jpeg")
	marker := []byte("private-metadata-sentinel")
	comment := []byte{0xff, 0xfe, 0, byte(len(marker) + 2)}
	source := append(append(append([]byte(nil), jpg[:2]...), append(comment, marker...)...), jpg[2:]...)
	p := checkPrepared(t, source)
	raw, _ := base64.StdEncoding.DecodeString(p.ImageData)
	if bytes.Contains(raw, marker) {
		t.Fatal("source metadata survived preparation")
	}
}

package socialops

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	stddraw "image/draw"
	_ "image/jpeg"
	"image/png"
	"strings"
	"unicode"
)

type ComposeImageRequest struct {
	LogoArtifactID       uuid.UUID  `json:"logo_artifact_id"`
	BackgroundArtifactID *uuid.UUID `json:"background_artifact_id,omitempty"`
	Headline             string     `json:"headline"`
	Body                 string     `json:"body"`
}

const MaxCompositionImagePixels = 12_000_000

// ImageDimensions bounds decompression before a full image decode.
func ImageDimensions(data []byte) (int, int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 12000 || config.Height > 12000 || int64(config.Width)*int64(config.Height) > MaxCompositionImagePixels {
		return 0, 0, invalid()
	}
	return config.Width, config.Height, nil
}

// ComposeImage produces a consistent 16:9 publication asset from actual
// approved logo pixels and literal text. It never asks an image model to
// regenerate a logo, guesses a logo, or accepts paths and remote URLs.
func (s *Service) ComposeImage(ctx context.Context, scope Scope, r ComposeImageRequest) (Artifact, error) {
	cfg, _, err := s.LoadCurrentConfiguration(ctx, scope)
	if err != nil {
		return Artifact{}, err
	}
	allowed := false
	for _, id := range cfg.BrandArtifactRefs {
		if id == r.LogoArtifactID {
			allowed = true
			break
		}
	}
	if !allowed || r.LogoArtifactID == uuid.Nil || len(r.Headline) > 200 || len(r.Body) > 400 {
		return Artifact{}, invalid()
	}
	logo, err := s.scopedImage(ctx, scope, r.LogoArtifactID)
	if err != nil {
		return Artifact{}, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	stddraw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{10, 24, 33, 255}), image.Point{}, stddraw.Src)
	card := image.Rect(72, 64, 392, 196)
	stddraw.Draw(canvas, card, image.NewUniform(color.White), image.Point{}, stddraw.Src)
	fitImage(canvas, image.Rect(88, 80, 376, 180), logo)
	width := 1440
	if r.BackgroundArtifactID != nil {
		background, err := s.scopedImage(ctx, scope, *r.BackgroundArtifactID)
		if err != nil {
			return Artifact{}, err
		}
		frame := image.Rect(924, 64, 1528, 836)
		stddraw.Draw(canvas, frame, image.NewUniform(color.RGBA{24, 44, 56, 255}), image.Point{}, stddraw.Src)
		fitImage(canvas, image.Rect(944, 84, 1508, 816), background)
		width = 760
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return Artifact{}, err
	}
	regular, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return Artifact{}, err
	}
	titleFace, err := opentype.NewFace(bold, &opentype.FaceOptions{Size: 72, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return Artifact{}, err
	}
	defer titleFace.Close()
	bodyFace, err := opentype.NewFace(regular, &opentype.FaceOptions{Size: 36, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return Artifact{}, err
	}
	defer bodyFace.Close()
	headline, err := imageTextLines(titleFace, r.Headline, width, 3)
	if err != nil {
		return Artifact{}, err
	}
	body, err := imageTextLines(bodyFace, r.Body, width, 4)
	if err != nil {
		return Artifact{}, err
	}
	title := font.Drawer{Dst: canvas, Src: image.NewUniform(color.White), Face: titleFace}
	for i, line := range headline {
		title.Dot = fixed.P(80, 308+i*88)
		title.DrawString(line)
	}
	text := font.Drawer{Dst: canvas, Src: image.NewUniform(color.RGBA{190, 212, 225, 255}), Face: bodyFace}
	for i, line := range body {
		text.Dot = fixed.P(80, 650+i*48)
		text.DrawString(line)
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, canvas); err != nil {
		return Artifact{}, err
	}
	return s.PutArtifact(ctx, scope, "image/png", encoded.Bytes())
}
func (s *Service) scopedImage(ctx context.Context, scope Scope, id uuid.UUID) (image.Image, error) {
	raw, err := s.ReadArtifact(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	if _, _, err = ImageDimensions(raw); err != nil {
		return nil, invalid()
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, invalid()
	}
	return decoded, nil
}
func fitImage(dst *image.RGBA, box image.Rectangle, src image.Image) {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	width := box.Dx()
	height := h * width / w
	if height > box.Dy() {
		height = box.Dy()
		width = w * height / h
	}
	x, y := box.Min.X+(box.Dx()-width)/2, box.Min.Y+(box.Dy()-height)/2
	draw.CatmullRom.Scale(dst, image.Rect(x, y, x+width, y+height), src, src.Bounds(), draw.Over, nil)
}
func imageTextLines(face font.Face, text string, width, maxLines int) ([]string, error) {
	if strings.TrimSpace(text) == "" {
		return nil, invalid()
	}
	for _, r := range text {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return nil, invalid()
		}
		if !unicode.IsSpace(r) {
			if _, ok := face.GlyphAdvance(r); !ok {
				return nil, invalid()
			}
		}
	}
	drawer := font.Drawer{Face: face}
	lines := []string{}
	line := ""
	for _, word := range strings.Fields(text) {
		if drawer.MeasureString(word) > fixed.I(width) {
			return nil, invalid()
		}
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if drawer.MeasureString(candidate) > fixed.I(width) {
			lines = append(lines, line)
			line = word
		} else {
			line = candidate
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) == 0 || len(lines) > maxLines {
		return nil, invalid()
	}
	return lines, nil
}

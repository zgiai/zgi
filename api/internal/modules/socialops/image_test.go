package socialops

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestComposeSocialImageUsesActualScopedLogoAndBoundedText(t *testing.T) {
	s, scope, cfg := configuredPublicationFixture(t, 2)
	logo := image.NewRGBA(image.Rect(0, 0, 200, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 200; x++ {
			logo.Set(x, y, color.RGBA{220, 30, 50, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, logo); err != nil {
		t.Fatal(err)
	}
	artifact, err := s.PutArtifact(context.Background(), scope, "image/png", b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Revision = 2
	cfg.BrandArtifactRefs = []uuid.UUID{artifact.ID}
	savePolicyConfiguration(t, s, cfg, 1)
	request := ComposeImageRequest{LogoArtifactID: artifact.ID, Headline: "Ship useful AI workflows", Body: "Keep product facts, tools and feedback connected."}
	result, err := s.ComposeImage(context.Background(), scope, request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := s.ReadArtifact(context.Background(), scope, result.ID)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := png.Decode(bytes.NewReader(data))
	if err != nil || rendered.Bounds().Dx() != 1600 || rendered.Bounds().Dy() != 900 {
		t.Fatal(rendered, err)
	}
	if red, _, _, _ := rendered.At(150, 110).RGBA(); red < 50000 {
		t.Fatal("actual logo pixels not composed")
	}
	if path := os.Getenv("ZGI_SOCIAL_IMAGE_PREVIEW"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	foreign := scope
	foreign.OwnerID = uuid.New()
	if _, err = s.ComposeImage(context.Background(), foreign, request); err == nil {
		t.Fatal("foreign logo composed")
	}
	request.LogoArtifactID = uuid.New()
	if _, err = s.ComposeImage(context.Background(), scope, request); err == nil {
		t.Fatal("unapproved logo composed")
	}
	request.LogoArtifactID = artifact.ID
	request.Headline = string(bytes.Repeat([]byte("long headline "), 40))
	if _, err = s.ComposeImage(context.Background(), scope, request); err == nil {
		t.Fatal("overflow silently clipped")
	}
}

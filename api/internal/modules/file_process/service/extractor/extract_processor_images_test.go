package extractor

import (
	"strings"
	"testing"
)

func TestImageDataURLFromStorageKey(t *testing.T) {
	store := newExtractorMemoryStorage()
	const key = "mineru/images/doc/table.png"
	if err := store.Save(key, []byte{0x89, 0x50, 0x4e, 0x47}); err != nil {
		t.Fatalf("save image: %v", err)
	}

	got, err := imageDataURLFromStorageKey(store, key)
	if err != nil {
		t.Fatalf("imageDataURLFromStorageKey returned error: %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("data URL = %q, want image/png data URL", got)
	}
}

func TestImageFileDataURLRejectsPreviewURL(t *testing.T) {
	_, err := imageFileDataURL("/console/api/files/mineru-images?key=mineru%2Fimages%2Fdoc%2Ftable.png")
	if err == nil {
		t.Fatalf("imageFileDataURL should reject preview URLs")
	}
	if !strings.Contains(err.Error(), "not a local file path") {
		t.Fatalf("error = %q, want non-local diagnostic", err.Error())
	}
}

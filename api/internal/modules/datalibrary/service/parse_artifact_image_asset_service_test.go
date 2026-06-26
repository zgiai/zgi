package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestParseArtifactImageAssetServicePersistsReductoFigureImages(t *testing.T) {
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0x18, 0xdd, 0x8d,
		0xb0, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer server.Close()

	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactImageAssetService(store)
	artifact := &contracts.ParseArtifact{
		Markdown: "original",
		Text:     "original",
		Elements: []contracts.ParsedElement{
			{
				Type:    "figure",
				Content: "The image displays ginseng.",
				Metadata: map[string]any{
					"payload": map[string]any{
						"source":    "reducto",
						"image_url": server.URL + "/figure.png?signature=temporary",
					},
				},
			},
			{
				Type:    "table",
				Content: "<table><tr><td>text</td></tr></table>",
				Metadata: map[string]any{
					"payload": map[string]any{
						"source":    "reducto",
						"image_url": server.URL + "/table.png",
					},
				},
			},
		},
	}

	result, err := svc.Normalize(context.Background(), ParseArtifactImageAssetNormalizeInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		Artifact:       artifact,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if result.StoredImageCount != 1 {
		t.Fatalf("StoredImageCount=%d, want 1", result.StoredImageCount)
	}
	if len(store.files) != 1 {
		t.Fatalf("stored files=%d, want 1", len(store.files))
	}
	var storedKey string
	for key, data := range store.files {
		storedKey = key
		if !strings.HasPrefix(key, "document-images/org-1/file-1/") {
			t.Fatalf("unexpected storage key: %s", key)
		}
		if !bytes.Equal(data, png) {
			t.Fatalf("stored image data mismatch")
		}
	}

	imageURL, _ := artifact.Elements[0].Metadata["image_url"].(string)
	if !strings.HasPrefix(imageURL, documentImageAssetEndpoint+"?key=") {
		t.Fatalf("image_url=%q, want document image endpoint", imageURL)
	}
	if artifact.Elements[0].Metadata["image_key"] != storedKey {
		t.Fatalf("image_key=%v, want %s", artifact.Elements[0].Metadata["image_key"], storedKey)
	}
	if !strings.HasPrefix(artifact.Elements[0].Content, "![figure]("+documentImageAssetEndpoint+"?key=") {
		t.Fatalf("figure content was not prefixed with markdown image: %q", artifact.Elements[0].Content)
	}
	if !strings.Contains(artifact.Markdown, "![figure]("+documentImageAssetEndpoint+"?key=") {
		t.Fatalf("artifact markdown was not rebuilt with image: %q", artifact.Markdown)
	}
	if _, ok := artifact.Elements[1].Metadata["image_key"]; ok {
		t.Fatalf("table image should not be persisted: %#v", artifact.Elements[1].Metadata)
	}
}

func TestParseArtifactImageAssetServiceSkipsMineruTableImagesEvenWhenLegacyFigure(t *testing.T) {
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactImageAssetService(store)
	artifact := &contracts.ParseArtifact{
		Metadata: map[string]any{
			"image_assets": map[string]any{
				"table.png": "data:image/png;base64,aGVsbG8=",
			},
		},
		Elements: []contracts.ParsedElement{
			{
				Type:    "figure",
				Content: "| A |\n|---|\n| 1 |",
				Metadata: map[string]any{
					"payload": map[string]any{
						"mineru_type":       "table",
						"structure_version": "mineru_content_list_v1",
						"img_path":          "images/table.png",
					},
				},
			},
		},
	}

	result, err := svc.Normalize(context.Background(), ParseArtifactImageAssetNormalizeInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		Artifact:       artifact,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if result.StoredImageCount != 0 {
		t.Fatalf("StoredImageCount=%d, want 0", result.StoredImageCount)
	}
	if len(store.files) != 0 {
		t.Fatalf("stored files=%d, want 0", len(store.files))
	}
	if _, ok := artifact.Elements[0].Metadata["image_key"]; ok {
		t.Fatalf("table image should not be persisted: %#v", artifact.Elements[0].Metadata)
	}
	if _, ok := artifact.Metadata["image_assets"]; ok {
		t.Fatalf("image_assets should be removed from persisted artifact metadata")
	}
}

func TestParseArtifactImageAssetServicePersistsMineruFigureImages(t *testing.T) {
	imageData := []byte("hello-image")
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactImageAssetService(store)
	artifact := &contracts.ParseArtifact{
		Metadata: map[string]any{
			"image_assets": map[string]any{
				"chart.jpg": "data:image/jpeg;base64,aGVsbG8taW1hZ2U=",
			},
		},
		Elements: []contracts.ParsedElement{
			{
				Type:    "figure",
				Content: "[figure]",
				Metadata: map[string]any{
					"markdown": "![Trend chart](images/chart.jpg)",
					"payload": map[string]any{
						"mineru_type":       "chart",
						"structure_version": "mineru_content_list_v1",
						"img_path":          "images/chart.jpg",
					},
				},
			},
			{
				Type:    "table",
				Content: "| A |\n|---|\n| 1 |",
				Metadata: map[string]any{
					"payload": map[string]any{
						"mineru_type":       "table",
						"structure_version": "mineru_content_list_v1",
						"img_path":          "images/table.png",
					},
				},
			},
		},
	}

	result, err := svc.Normalize(context.Background(), ParseArtifactImageAssetNormalizeInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		Artifact:       artifact,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if result.StoredImageCount != 1 {
		t.Fatalf("StoredImageCount=%d, want 1", result.StoredImageCount)
	}
	if _, ok := artifact.Metadata["image_assets"]; ok {
		t.Fatalf("image_assets should be removed from persisted artifact metadata")
	}
	var storedKey string
	for key, data := range store.files {
		storedKey = key
		if !strings.HasPrefix(key, "document-images/org-1/file-1/") {
			t.Fatalf("unexpected storage key: %s", key)
		}
		if !bytes.Equal(data, imageData) {
			t.Fatalf("stored image data mismatch")
		}
	}
	if storedKey == "" {
		t.Fatal("expected stored mineru image")
	}
	if artifact.Elements[0].Content != "![figure]("+artifact.Elements[0].Metadata["image_url"].(string)+")" {
		t.Fatalf("expected placeholder content to be replaced by image markdown, got %q", artifact.Elements[0].Content)
	}
	if artifact.Elements[0].Metadata["image_asset_source"] != "mineru" {
		t.Fatalf("image_asset_source=%v, want mineru", artifact.Elements[0].Metadata["image_asset_source"])
	}
	if artifact.Elements[0].Metadata["image_key"] != storedKey {
		t.Fatalf("image_key=%v, want %s", artifact.Elements[0].Metadata["image_key"], storedKey)
	}
	if payload := artifact.Elements[0].Metadata["payload"].(map[string]any); payload["img_path"] != artifact.Elements[0].Metadata["image_url"] {
		t.Fatalf("payload img_path was not rewritten: %#v", payload)
	}
	if _, ok := artifact.Elements[1].Metadata["image_key"]; ok {
		t.Fatalf("table image should not be persisted: %#v", artifact.Elements[1].Metadata)
	}
}

func TestParseArtifactImageAssetServiceKeepsMineruVisualSummaryBelowImage(t *testing.T) {
	imageData := "data:image/jpeg;base64,aGVsbG8taW1hZ2U="
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactImageAssetService(store)
	artifact := &contracts.ParseArtifact{
		Markdown: "original",
		Text:     "original",
		Elements: []contracts.ParsedElement{
			{
				Type:    "figure",
				Content: "图片展示了一张趋势图。",
				Metadata: map[string]any{
					"payload": map[string]any{
						"mineru_type":       "chart",
						"original_img_path": "images/chart.jpg",
						"img_path":          imageData,
						"image_data_uri":    imageData,
					},
				},
			},
		},
	}

	_, err := svc.Normalize(context.Background(), ParseArtifactImageAssetNormalizeInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		Artifact:       artifact,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	imageURL, _ := artifact.Elements[0].Metadata["image_url"].(string)
	expected := "![figure](" + imageURL + ")\n\n图片展示了一张趋势图。"
	if artifact.Elements[0].Content != expected {
		t.Fatalf("content=%q, want %q", artifact.Elements[0].Content, expected)
	}
}

func TestParseArtifactImageAssetServicePersistsMineruFigureDataURIFromPayload(t *testing.T) {
	imageData := []byte("hello-image")
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactImageAssetService(store)
	artifact := &contracts.ParseArtifact{
		Elements: []contracts.ParsedElement{
			{
				Type:    "figure",
				Content: "[figure]",
				Metadata: map[string]any{
					"markdown": "![Trend chart](data:image/jpeg;base64,aGVsbG8taW1hZ2U=)",
					"payload": map[string]any{
						"mineru_type":       "chart",
						"structure_version": "mineru_content_list_v1",
						"original_img_path": "images/chart.jpg",
						"img_path":          "data:image/jpeg;base64,aGVsbG8taW1hZ2U=",
						"image_data_uri":    "data:image/jpeg;base64,aGVsbG8taW1hZ2U=",
						"image_ref_type":    "data_uri",
					},
				},
			},
		},
	}

	result, err := svc.Normalize(context.Background(), ParseArtifactImageAssetNormalizeInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		Artifact:       artifact,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if result.StoredImageCount != 1 {
		t.Fatalf("StoredImageCount=%d, want 1", result.StoredImageCount)
	}
	var storedKey string
	for key, data := range store.files {
		storedKey = key
		if !strings.HasPrefix(key, "document-images/org-1/file-1/") {
			t.Fatalf("unexpected storage key: %s", key)
		}
		if !bytes.Equal(data, imageData) {
			t.Fatalf("stored image data mismatch")
		}
	}
	if storedKey == "" {
		t.Fatal("expected stored mineru image")
	}
	imageURL, _ := artifact.Elements[0].Metadata["image_url"].(string)
	if !strings.HasPrefix(imageURL, documentImageAssetEndpoint+"?key=") {
		t.Fatalf("image_url=%q, want document image endpoint", imageURL)
	}
	if artifact.Elements[0].Metadata["original_image_path"] != "images/chart.jpg" {
		t.Fatalf("original_image_path=%v", artifact.Elements[0].Metadata["original_image_path"])
	}
	if artifact.Elements[0].Metadata["image_key"] != storedKey {
		t.Fatalf("image_key=%v, want %s", artifact.Elements[0].Metadata["image_key"], storedKey)
	}
	payload := artifact.Elements[0].Metadata["payload"].(map[string]any)
	if payload["original_img_path"] != "images/chart.jpg" {
		t.Fatalf("original_img_path was not preserved: %#v", payload)
	}
	if payload["img_path"] != imageURL {
		t.Fatalf("payload img_path was not rewritten: %#v", payload)
	}
	if payload["image_url"] != imageURL {
		t.Fatalf("payload image_url was not set: %#v", payload)
	}
	if _, ok := payload["image_data_uri"]; ok {
		t.Fatalf("image_data_uri should be removed after persistence: %#v", payload)
	}
	if artifact.Elements[0].Content != "![figure]("+imageURL+")" {
		t.Fatalf("expected figure content to be rewritten with stored URL, got %q", artifact.Elements[0].Content)
	}
	if !strings.Contains(artifact.Markdown, imageURL) {
		t.Fatalf("artifact markdown was not rebuilt with image URL: %q", artifact.Markdown)
	}
}

func TestParseArtifactImageAssetServicePersistsEmbeddedTableDataURIImages(t *testing.T) {
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactImageAssetService(store)
	tableHTML := `<table><tr><td><p><img src="data:image/png;base64,aGVsbG8x"/></p></td><td><p><img src='data:image/png;base64,aGVsbG8y'/></p></td></tr></table>`
	artifact := &contracts.ParseArtifact{
		Elements: []contracts.ParsedElement{
			{
				Type:    "table",
				Content: tableHTML,
				Metadata: map[string]any{
					"markdown": tableHTML,
					"payload": map[string]any{
						"mineru_type":       "table",
						"structure_version": "mineru_content_list_v1",
						"table_body":        tableHTML,
					},
				},
			},
		},
	}

	result, err := svc.Normalize(context.Background(), ParseArtifactImageAssetNormalizeInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		Artifact:       artifact,
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if result.StoredImageCount != 2 {
		t.Fatalf("StoredImageCount=%d, want 2", result.StoredImageCount)
	}
	if len(store.files) != 2 {
		t.Fatalf("stored files=%d, want 2", len(store.files))
	}
	if strings.Contains(artifact.Elements[0].Content, "data:image/") {
		t.Fatalf("table content still contains data URI: %q", artifact.Elements[0].Content)
	}
	if got := strings.Count(artifact.Elements[0].Content, documentImageAssetEndpoint+"?key="); got != 2 {
		t.Fatalf("document image URL count=%d, want 2: %q", got, artifact.Elements[0].Content)
	}
	if artifact.Elements[0].Metadata["embedded_image_count"] != 2 {
		t.Fatalf("embedded_image_count=%v, want 2", artifact.Elements[0].Metadata["embedded_image_count"])
	}
	embedded, ok := artifact.Elements[0].Metadata["embedded_images"].([]embeddedParseArtifactImage)
	if !ok || len(embedded) != 2 {
		t.Fatalf("embedded_images=%#v, want 2 embedded image records", artifact.Elements[0].Metadata["embedded_images"])
	}
	payload := artifact.Elements[0].Metadata["payload"].(map[string]any)
	if strings.Contains(payload["table_body"].(string), "data:image/") {
		t.Fatalf("payload table_body still contains data URI: %q", payload["table_body"])
	}
	if strings.Contains(artifact.Markdown, "data:image/") {
		t.Fatalf("artifact markdown still contains data URI: %q", artifact.Markdown)
	}
}

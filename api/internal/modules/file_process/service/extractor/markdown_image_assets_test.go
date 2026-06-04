package extractor

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/storage"
)

type extractorMemoryStorage struct {
	files map[string][]byte
}

func newExtractorMemoryStorage() *extractorMemoryStorage {
	return &extractorMemoryStorage{files: map[string][]byte{}}
}

func (s *extractorMemoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *extractorMemoryStorage) Load(filename string) ([]byte, error) {
	return append([]byte(nil), s.files[filename]...), nil
}

func (s *extractorMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	ch <- s.files[filename]
	close(ch)
	return ch, nil
}

func (s *extractorMemoryStorage) Download(filename string, targetPath string) error { return nil }
func (s *extractorMemoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}
func (s *extractorMemoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}
func (s *extractorMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	return []storage.FileInfo{{Key: prefix, LastModified: time.Now()}}, nil
}

func TestPersistMarkdownImageAssetsStoresExternalMarkdownAndHTMLImages(t *testing.T) {
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
		if strings.Contains(r.URL.Path, "plant") {
			_, _ = w.Write(append(append([]byte(nil), png...), 0x01))
			return
		}
		_, _ = w.Write(png)
	}))
	defer server.Close()

	store := newExtractorMemoryStorage()
	processor := &ExtractProcessor{storage: store}
	output := &dto.ExtractOutput{
		Markdown: "text ![root](" + server.URL + "/root.png)",
		Elements: []dto.ExtractElement{
			{Content: `<table><tr><td><img src="` + server.URL + `/plant.png"></td></tr></table>`},
		},
	}

	uploadFile := &model.UploadFile{OrganizationID: "org-1", ID: "file-1"}
	processor.persistMarkdownImageAssets(context.Background(), output, uploadFile)

	if len(store.files) != 2 {
		t.Fatalf("expected two stored images, got %d", len(store.files))
	}
	for key, data := range store.files {
		if !strings.HasPrefix(key, "document-images/org-1/file-1/") {
			t.Fatalf("unexpected storage key: %s", key)
		}
		if !bytes.HasPrefix(data, png) {
			t.Fatalf("unexpected stored data for %s", key)
		}
	}
	if strings.Contains(output.Markdown, server.URL) {
		t.Fatalf("markdown still contains external image URL: %q", output.Markdown)
	}
	if strings.Contains(output.Elements[0].Content, server.URL) {
		t.Fatalf("html content still contains external image URL: %q", output.Elements[0].Content)
	}
	if strings.Count(output.Markdown+output.Elements[0].Content, documentImageEndpoint+"?key=") != 2 {
		t.Fatalf("expected rewritten document image URLs, got markdown=%q element=%q", output.Markdown, output.Elements[0].Content)
	}
}

func TestPersistMarkdownImageAssetsKeepsUnavailableExternalImage(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	store := newExtractorMemoryStorage()
	processor := &ExtractProcessor{storage: store}
	original := "![expired](" + server.URL + "/expired.png)"
	output := &dto.ExtractOutput{Markdown: original}

	processor.persistMarkdownImageAssets(context.Background(), output, &model.UploadFile{ID: "file-1"})

	if len(store.files) != 0 {
		t.Fatalf("expected no stored files, got %d", len(store.files))
	}
	if output.Markdown != original {
		t.Fatalf("expected original markdown to be preserved, got %q", output.Markdown)
	}
}

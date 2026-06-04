package hyperparse

import (
	"bytes"
	"strings"
	"testing"
	"time"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	"github.com/zgiai/zgi/api/pkg/storage"
)

type memoryStorage struct {
	files map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{files: map[string][]byte{}}
}

func (s *memoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *memoryStorage) Load(filename string) ([]byte, error) {
	return append([]byte(nil), s.files[filename]...), nil
}

func (s *memoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	ch <- s.files[filename]
	close(ch)
	return ch, nil
}

func (s *memoryStorage) Download(filename string, targetPath string) error { return nil }
func (s *memoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}
func (s *memoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}
func (s *memoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	return []storage.FileInfo{{Key: prefix, LastModified: time.Now()}}, nil
}

func TestPersistMinerUImagesRewritesMarkdownAndPayload(t *testing.T) {
	store := newMemoryStorage()
	result := &extractcommon.DocumentResult{
		DocID:    "doc-1",
		Markdown: "before ![](images/chart.jpg) after",
		Chunks: []extractcommon.Chunk{
			{
				ID:       "c1",
				Type:     "figure",
				Markdown: "![figure](images/chart.jpg)",
				Payload:  map[string]any{"img_path": "images/chart.jpg"},
			},
		},
		ImageAssets: map[string]string{
			"chart.jpg": "data:image/jpeg;base64,aGVsbG8=",
		},
	}

	if err := persistMinerUImages(store, result, "org-1/file-1"); err != nil {
		t.Fatalf("persist mineru images: %v", err)
	}
	if len(store.files) != 1 {
		t.Fatalf("expected one stored image, got %d", len(store.files))
	}
	var storedKey string
	for key, data := range store.files {
		storedKey = key
		if !bytes.Equal(data, []byte("hello")) {
			t.Fatalf("unexpected stored data: %q", data)
		}
	}
	if result.ImageAssets != nil {
		t.Fatalf("expected image assets to be cleared")
	}
	if want := buildMinerUImageAssetURL(storedKey); !bytes.Contains([]byte(result.Markdown), []byte(want)) {
		t.Fatalf("markdown was not rewritten with %q: %q", want, result.Markdown)
	}
	payload := result.Chunks[0].Payload
	if payload["image_key"] != storedKey {
		t.Fatalf("expected image_key %q, got %#v", storedKey, payload["image_key"])
	}
	if payload["original_img_path"] != "images/chart.jpg" {
		t.Fatalf("expected original path, got %#v", payload["original_img_path"])
	}
	if payload["img_path"] != buildMinerUImageAssetURL(storedKey) {
		t.Fatalf("expected img_path URL, got %#v", payload["img_path"])
	}
}

func TestPersistMinerUImagesRewritesHTMLImagesInsideTables(t *testing.T) {
	store := newMemoryStorage()
	result := &extractcommon.DocumentResult{
		DocID:    "doc-table",
		Markdown: `<table><tr><td><img src="images/root.jpg"></td><td><img alt="plant" src='images/plant.jpg'/></td></tr></table>`,
		Chunks: []extractcommon.Chunk{
			{
				ID:       "table-1",
				Type:     "table",
				Markdown: `<table><tr><td><img src="images/root.jpg"></td><td><img src=images/plant.jpg></td></tr></table>`,
			},
		},
		ImageAssets: map[string]string{
			"root.jpg":  "data:image/jpeg;base64,cm9vdA==",
			"plant.jpg": "data:image/jpeg;base64,cGxhbnQ=",
		},
	}

	if err := persistMinerUImages(store, result, "org-1/file-1"); err != nil {
		t.Fatalf("persist mineru images: %v", err)
	}
	if len(store.files) != 2 {
		t.Fatalf("expected two stored images, got %d", len(store.files))
	}
	if strings.Contains(result.Markdown, "images/root.jpg") || strings.Contains(result.Markdown, "images/plant.jpg") {
		t.Fatalf("document markdown still contains mineru relative image paths: %q", result.Markdown)
	}
	if strings.Contains(result.Chunks[0].Markdown, "images/root.jpg") || strings.Contains(result.Chunks[0].Markdown, "images/plant.jpg") {
		t.Fatalf("table markdown still contains mineru relative image paths: %q", result.Chunks[0].Markdown)
	}
	if count := strings.Count(result.Chunks[0].Markdown, mineruImageEndpoint+"?key="); count != 2 {
		t.Fatalf("expected two rewritten image URLs in table chunk, got %d: %q", count, result.Chunks[0].Markdown)
	}
}

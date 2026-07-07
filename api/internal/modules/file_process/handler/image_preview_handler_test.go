package handler

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/storage"
)

func TestGetMinerUImageRejectsUnsafeStorageKeyBeforeLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &minerUImagePreviewStorage{}
	handler := NewImagePreviewHandler(nil, nil, nil, store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/files/mineru-images?key=../secret.png", nil)

	handler.GetMinerUImage(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if store.loadCalls != 0 {
		t.Fatalf("storage Load calls = %d, want 0", store.loadCalls)
	}
}

func TestGetMinerUImageRejectsUnknownStoragePrefixBeforeLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &minerUImagePreviewStorage{}
	handler := NewImagePreviewHandler(nil, nil, nil, store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/files/mineru-images?key=uploads/secret.png", nil)

	handler.GetMinerUImage(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if store.loadCalls != 0 {
		t.Fatalf("storage Load calls = %d, want 0", store.loadCalls)
	}
}

func TestGetMinerUImageAllowsKnownStorageKeyPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const key = "mineru/images/doc/figure.png"
	store := &minerUImagePreviewStorage{
		files: map[string][]byte{key: mustTestPNG(t)},
	}
	handler := NewImagePreviewHandler(nil, nil, nil, store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/files/mineru-images?key="+key, nil)

	handler.GetMinerUImage(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if store.lastLoadKey != key {
		t.Fatalf("storage key = %q, want %q", store.lastLoadKey, key)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q, want image/png", got)
	}
}

func TestGetMinerUImageAllowsSignedStorageKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreMinerUImagePreviewConfig(t)

	const key = "document-images/doc/figure.png"
	store := &minerUImagePreviewStorage{
		files: map[string][]byte{key: mustTestPNG(t)},
	}
	handler := NewImagePreviewHandler(nil, nil, nil, store)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	signedURL := util.GetSignedParserImageKeyURL(key)
	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/files/mineru-images?"+parsed.RawQuery, nil)

	handler.GetMinerUImage(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if store.lastLoadKey != key {
		t.Fatalf("storage key = %q, want %q", store.lastLoadKey, key)
	}
}

func TestGetMinerUImageRejectsTamperedSignatureBeforeLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreMinerUImagePreviewConfig(t)

	const key = "mineru/images/doc/figure.png"
	store := &minerUImagePreviewStorage{
		files: map[string][]byte{key: mustTestPNG(t)},
	}
	handler := NewImagePreviewHandler(nil, nil, nil, store)
	signedURL := util.GetSignedParserImageKeyURL(key)
	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	query := parsed.Query()
	query.Set("sign", "tampered")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/files/mineru-images?"+query.Encode(), nil)

	handler.GetMinerUImage(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if store.loadCalls != 0 {
		t.Fatalf("storage Load calls = %d, want 0", store.loadCalls)
	}
}

func TestGetMinerUImageRejectsNonMinerULocalPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	secretPath := filepath.Join(tempDir, "secret.png")
	handler := NewImagePreviewHandler(nil, nil, nil, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/files/mineru-images?path="+secretPath, nil)

	handler.GetMinerUImage(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAllowMinerULocalImagePathRequiresAbsoluteMinerUImagePath(t *testing.T) {
	allowedPath := filepath.Join(t.TempDir(), "storage", "mineru", "images", "doc", "figure.png")
	if !allowMinerULocalImagePath(allowedPath) {
		t.Fatalf("allowMinerULocalImagePath(%q) = false, want true", allowedPath)
	}
	if allowMinerULocalImagePath(filepath.Join("storage", "mineru", "images", "doc", "figure.png")) {
		t.Fatalf("relative mineru image path was allowed")
	}
	if allowMinerULocalImagePath(filepath.Join(t.TempDir(), "storage", "mineru", "images", "..", "secret.png")) {
		t.Fatalf("path with traversal segment was allowed")
	}
}

func restoreMinerUImagePreviewConfig(t *testing.T) {
	t.Helper()

	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
}

type minerUImagePreviewStorage struct {
	files       map[string][]byte
	loadCalls   int
	lastLoadKey string
}

func (s *minerUImagePreviewStorage) Save(filename string, data []byte) error {
	if s.files == nil {
		s.files = map[string][]byte{}
	}
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *minerUImagePreviewStorage) Load(filename string) ([]byte, error) {
	s.loadCalls++
	s.lastLoadKey = filename
	data, ok := s.files[filename]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *minerUImagePreviewStorage) LoadStream(filename string) (<-chan []byte, error) {
	data, err := s.Load(filename)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	ch <- data
	close(ch)
	return ch, nil
}

func (s *minerUImagePreviewStorage) Download(filename string, targetPath string) error {
	return nil
}

func (s *minerUImagePreviewStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}

func (s *minerUImagePreviewStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}

func (s *minerUImagePreviewStorage) List(prefix string) ([]storage.FileInfo, error) {
	return []storage.FileInfo{{Key: prefix, LastModified: time.Now()}}, nil
}

func mustTestPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

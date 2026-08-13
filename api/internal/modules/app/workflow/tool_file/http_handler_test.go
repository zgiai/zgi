package tool_file

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHTTPHandlerGetToolFileSupportsByteRanges(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	manager := NewToolFileManager(db, &toolFileHandlerTestStorage{files: make(map[string][]byte)})
	file, err := manager.CreateFileByRaw(t.Context(), CreateFileByRawParams{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		FileData:  []byte("0123456789"),
		MimeType:  "audio/mpeg",
		Lifecycle: ToolFileLifecyclePersistent,
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw() error = %v", err)
	}

	signature := NewFileSignature(&config.Config{App: config.AppConfig{
		SecretKey: "test-secret",
		FilesURL:  "http://example.com",
	}})
	signedURL, err := signature.SignToolFileWithMode(file.ID, ".mp3", ToolFileURLModePermanent)
	if err != nil {
		t.Fatalf("SignToolFileWithMode() error = %v", err)
	}
	parsedURL, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	query := parsedURL.Query()
	query.Set("delivery", "direct")
	parsedURL.RawQuery = query.Encode()

	handler := &HTTPHandler{manager: manager, signature: signature}
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", handler.GetToolFile)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, parsedURL.RequestURI(), nil)
	request.Header.Set("Range", "bytes=2-5")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusPartialContent, recorder.Body.String())
	}
	if got := recorder.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Errorf("Accept-Ranges = %q, want %q", got, "bytes")
	}
	if got := recorder.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Errorf("Content-Range = %q, want %q", got, "bytes 2-5/10")
	}
	if got := recorder.Header().Get("Content-Type"); got != "audio/mpeg" {
		t.Errorf("Content-Type = %q, want %q", got, "audio/mpeg")
	}
	if got := recorder.Body.String(); got != "2345" {
		t.Errorf("body = %q, want %q", got, "2345")
	}
	if got := recorder.Header().Get("Content-Length"); got != "4" {
		t.Errorf("Content-Length = %q, want %q", got, "4")
	}
	if strings.Contains(strings.ToLower(recorder.Header().Get("Content-Disposition")), "attachment") {
		t.Errorf("Content-Disposition = %q, want inline response", recorder.Header().Get("Content-Disposition"))
	}
}

func TestHTTPHandlerGetToolFileRedirectsDirectDeliveryToStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	storageBackend := &toolFileHandlerPresignStorage{
		toolFileHandlerTestStorage: toolFileHandlerTestStorage{files: make(map[string][]byte)},
		presignedURL:               "https://storage.example.com/music.mp3?signature=storage",
	}
	manager := NewToolFileManager(db, storageBackend)
	file, err := manager.CreateFileByRaw(t.Context(), CreateFileByRawParams{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		FileData:  []byte("0123456789"),
		MimeType:  "audio/mpeg",
		Lifecycle: ToolFileLifecyclePersistent,
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw() error = %v", err)
	}

	signature := NewFileSignature(&config.Config{App: config.AppConfig{
		SecretKey: "test-secret",
		FilesURL:  "http://example.com",
	}})
	signedURL, err := signature.SignToolFile(file.ID, ".mp3")
	if err != nil {
		t.Fatalf("SignToolFile() error = %v", err)
	}
	parsedURL, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	query := parsedURL.Query()
	query.Set("delivery", "direct")
	parsedURL.RawQuery = query.Encode()

	handler := &HTTPHandler{manager: manager, signature: signature}
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", handler.GetToolFile)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, parsedURL.RequestURI(), nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusTemporaryRedirect, recorder.Body.String())
	}
	if got := recorder.Header().Get("Location"); got != storageBackend.presignedURL {
		t.Errorf("Location = %q, want %q", got, storageBackend.presignedURL)
	}
	if storageBackend.loadCalls != 0 {
		t.Errorf("Load() calls = %d, want 0", storageBackend.loadCalls)
	}
	if storageBackend.presignCalls != 1 {
		t.Errorf("PresignedGetURL() calls = %d, want 1", storageBackend.presignCalls)
	}
	if storageBackend.presignedKey != file.FileKey {
		t.Errorf("presigned key = %q, want %q", storageBackend.presignedKey, file.FileKey)
	}
	if storageBackend.presignedOptions.ResponseContentType != "audio/mpeg" {
		t.Errorf("response content type = %q, want %q", storageBackend.presignedOptions.ResponseContentType, "audio/mpeg")
	}
	if storageBackend.presignedOptions.Expires <= 0 || storageBackend.presignedOptions.Expires > time.Hour {
		t.Errorf("presigned expiry = %v, want within (0, 1h]", storageBackend.presignedOptions.Expires)
	}
}

func TestHTTPHandlerGetToolFileKeepsDownloadOnProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	storageBackend := &toolFileHandlerPresignStorage{
		toolFileHandlerTestStorage: toolFileHandlerTestStorage{files: make(map[string][]byte)},
		presignedURL:               "https://storage.example.com/music.mp3?signature=storage",
	}
	manager := NewToolFileManager(db, storageBackend)
	filename := "track.mp3"
	file, err := manager.CreateFileByRaw(t.Context(), CreateFileByRawParams{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		FileData:  []byte("0123456789"),
		MimeType:  "audio/mpeg",
		Filename:  &filename,
		Lifecycle: ToolFileLifecyclePersistent,
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw() error = %v", err)
	}

	signature := NewFileSignature(&config.Config{App: config.AppConfig{
		SecretKey: "test-secret",
		FilesURL:  "http://example.com",
	}})
	signedURL, err := signature.SignToolFile(file.ID, ".mp3")
	if err != nil {
		t.Fatalf("SignToolFile() error = %v", err)
	}
	parsedURL, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	query := parsedURL.Query()
	query.Set("delivery", "direct")
	query.Set("download", "1")
	parsedURL.RawQuery = query.Encode()

	handler := &HTTPHandler{manager: manager, signature: signature}
	router := gin.New()
	router.GET("/console/api/files/tools/:tool_file_id", handler.GetToolFile)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, parsedURL.RequestURI(), nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %q", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if got := recorder.Body.String(); got != "0123456789" {
		t.Errorf("body = %q, want %q", got, "0123456789")
	}
	if storageBackend.presignCalls != 0 {
		t.Errorf("PresignedGetURL() calls = %d, want 0", storageBackend.presignCalls)
	}
	if storageBackend.loadCalls != 1 {
		t.Errorf("Load() calls = %d, want 1", storageBackend.loadCalls)
	}
	if got := recorder.Header().Get("Content-Disposition"); !strings.Contains(got, "track.mp3") {
		t.Errorf("Content-Disposition = %q, want track.mp3", got)
	}
}

type toolFileHandlerTestStorage struct {
	storage.Storage
	files     map[string][]byte
	loadCalls int
}

func (s *toolFileHandlerTestStorage) Save(filename string, data []byte) error {
	s.files[filename] = bytes.Clone(data)
	return nil
}

func (s *toolFileHandlerTestStorage) Load(filename string) ([]byte, error) {
	s.loadCalls++
	data, ok := s.files[filename]
	if !ok {
		return nil, errors.New("file not found")
	}
	return bytes.Clone(data), nil
}

type toolFileHandlerPresignStorage struct {
	toolFileHandlerTestStorage
	presignedURL     string
	presignedKey     string
	presignedOptions storage.PresignedGetOptions
	presignCalls     int
}

func (s *toolFileHandlerPresignStorage) PresignedGetURL(key string, options storage.PresignedGetOptions) (string, error) {
	s.presignCalls++
	s.presignedKey = key
	s.presignedOptions = options
	return s.presignedURL, nil
}

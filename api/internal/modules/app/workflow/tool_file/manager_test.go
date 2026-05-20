package tool_file

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/ginext/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type memoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{
		files: make(map[string][]byte),
	}
}

func (m *memoryStorage) Save(filename string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf := make([]byte, len(data))
	copy(buf, data)
	m.files[filename] = buf
	return nil
}

func (m *memoryStorage) Load(filename string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.files[filename]...), nil
}

func (m *memoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	data, _ := m.Load(filename)
	ch <- data
	close(ch)
	return ch, nil
}

func (m *memoryStorage) Download(filename string, targetPath string) error {
	data, _ := m.Load(filename)
	return os.WriteFile(targetPath, data, 0o600)
}

func (m *memoryStorage) Exists(filename string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[filename]
	return ok, nil
}

func (m *memoryStorage) Delete(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, filename)
	return nil
}

func (m *memoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]storage.FileInfo, 0)
	for key, value := range m.files {
		if prefix != "" && len(key) >= len(prefix) && key[:len(prefix)] != prefix {
			continue
		}
		result = append(result, storage.FileInfo{
			Key:  key,
			Size: int64(len(value)),
		})
	}
	return result, nil
}

func TestCreateFileByRaw_DefaultLifecyclePersistent(t *testing.T) {
	db := openToolFileTestDB(t)
	manager := NewToolFileManager(db, newMemoryStorage())

	toolFile, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:   "user-1",
		TenantID: "tenant-1",
		FileData: []byte("hello"),
		MimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw returned error: %v", err)
	}

	if got := toolFile.LifecycleValue(); got != ToolFileLifecyclePersistent {
		t.Fatalf("LifecycleValue = %s, want %s", got, ToolFileLifecyclePersistent)
	}
	if toolFile.ExpiresAt != nil {
		t.Fatalf("ExpiresAt = %v, want nil", toolFile.ExpiresAt)
	}
}

func TestCreateFileByRaw_TemporaryLifecycleSetsExpiresAt(t *testing.T) {
	db := openToolFileTestDB(t)
	manager := NewToolFileManager(db, newMemoryStorage())

	before := time.Now()
	toolFile, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		FileData:  []byte("hello"),
		MimeType:  "image/png",
		Lifecycle: ToolFileLifecycleTemporary,
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw returned error: %v", err)
	}

	if got := toolFile.LifecycleValue(); got != ToolFileLifecycleTemporary {
		t.Fatalf("LifecycleValue = %s, want %s", got, ToolFileLifecycleTemporary)
	}
	if toolFile.ExpiresAt == nil {
		t.Fatalf("ExpiresAt is nil, want non-nil")
	}
	if toolFile.ExpiresAt.Before(before.Add(23*time.Hour)) || toolFile.ExpiresAt.After(before.Add(25*time.Hour)) {
		t.Fatalf("ExpiresAt = %v, want approximately now+24h", toolFile.ExpiresAt)
	}
}

func TestCleanupExpiredTemporaryFiles_DeletesOnlyExpiredTemporary(t *testing.T) {
	db := openToolFileTestDB(t)
	memStorage := newMemoryStorage()
	manager := NewToolFileManager(db, memStorage)

	expiredAt := time.Now().Add(-1 * time.Hour)
	futureAt := time.Now().Add(2 * time.Hour)

	expired, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		FileData:  []byte("expired"),
		MimeType:  "image/png",
		Lifecycle: ToolFileLifecycleTemporary,
		ExpiresAt: &expiredAt,
	})
	if err != nil {
		t.Fatalf("create expired file: %v", err)
	}

	active, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:    "user-1",
		TenantID:  "tenant-1",
		FileData:  []byte("active"),
		MimeType:  "image/png",
		Lifecycle: ToolFileLifecycleTemporary,
		ExpiresAt: &futureAt,
	})
	if err != nil {
		t.Fatalf("create active file: %v", err)
	}

	persistent, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:   "user-1",
		TenantID: "tenant-1",
		FileData: []byte("persistent"),
		MimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("create persistent file: %v", err)
	}

	deletedCount, err := manager.CleanupExpiredTemporaryFiles(context.Background())
	if err != nil {
		t.Fatalf("CleanupExpiredTemporaryFiles returned error: %v", err)
	}
	if deletedCount != 1 {
		t.Fatalf("deletedCount = %d, want 1", deletedCount)
	}

	if exists, _ := memStorage.Exists(expired.FileKey); exists {
		t.Fatalf("expired file still exists in storage")
	}
	if exists, _ := memStorage.Exists(active.FileKey); !exists {
		t.Fatalf("active temporary file unexpectedly deleted")
	}
	if exists, _ := memStorage.Exists(persistent.FileKey); !exists {
		t.Fatalf("persistent file unexpectedly deleted")
	}

	var count int64
	if err := db.Model(&ToolFile{}).Count(&count).Error; err != nil {
		t.Fatalf("count tool_files: %v", err)
	}
	if count != 2 {
		t.Fatalf("tool_files count = %d, want 2", count)
	}
}

func TestCreateFileByRaw_LegacyToolFilesSchemaUsesMimetypeColumn(t *testing.T) {
	db := openLegacyToolFileTestDB(t)
	manager := NewToolFileManager(db, newMemoryStorage())

	toolFile, err := manager.CreateFileByRaw(context.Background(), CreateFileByRawParams{
		UserID:   "user-1",
		TenantID: "tenant-1",
		FileData: []byte("hello"),
		MimeType: "image/png",
	})
	if err != nil {
		t.Fatalf("CreateFileByRaw returned error: %v", err)
	}
	if toolFile == nil {
		t.Fatalf("CreateFileByRaw returned nil toolFile")
	}

	var count int64
	if err := db.Model(&ToolFile{}).Count(&count).Error; err != nil {
		t.Fatalf("count tool_files: %v", err)
	}
	if count != 1 {
		t.Fatalf("tool_files count = %d, want 1", count)
	}
}

func TestCreateFileByURL_DetectsImageMimeTypeFromBinaryWhenHeaderIsOctetStream(t *testing.T) {
	db := openToolFileTestDB(t)
	manager := NewToolFileManager(db, newMemoryStorage())

	pngData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jx1cAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(pngData)
	}))
	defer server.Close()

	toolFile, err := manager.CreateFileByURL(context.Background(), CreateFileByURLParams{
		UserID:   "user-1",
		TenantID: "tenant-1",
		FileURL:  server.URL + "/generated",
	})
	if err != nil {
		t.Fatalf("CreateFileByURL returned error: %v", err)
	}

	if toolFile.MimeType != "image/png" {
		t.Fatalf("MimeType = %q, want %q", toolFile.MimeType, "image/png")
	}
	if !strings.HasSuffix(toolFile.Name, ".png") {
		t.Fatalf("Name = %q, want .png suffix", toolFile.Name)
	}

	fileData, mimeType, err := manager.GetFileBinary(context.Background(), toolFile.ID)
	if err != nil {
		t.Fatalf("GetFileBinary returned error: %v", err)
	}
	if mimeType != "image/png" {
		t.Fatalf("GetFileBinary mimeType = %q, want %q", mimeType, "image/png")
	}
	if len(fileData) != len(pngData) {
		t.Fatalf("GetFileBinary len = %d, want %d", len(fileData), len(pngData))
	}
}

func openToolFileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "tool_file_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatalf("auto migrate tool_files: %v", err)
	}
	return db
}

func openLegacyToolFileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "tool_file_legacy_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE tool_files (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			conversation_id TEXT,
			file_key TEXT NOT NULL,
			mimetype TEXT NOT NULL,
			original_url TEXT,
			name TEXT NOT NULL,
			size INTEGER NOT NULL,
			lifecycle TEXT NOT NULL DEFAULT 'persistent',
			expires_at DATETIME NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME NULL
		)
	`).Error; err != nil {
		t.Fatalf("create legacy tool_files: %v", err)
	}

	return db
}

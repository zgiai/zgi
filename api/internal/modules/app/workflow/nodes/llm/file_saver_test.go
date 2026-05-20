package llm

import (
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	appconfig "github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	toolfile "github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/ginext/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fileSaverMemoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newFileSaverMemoryStorage() *fileSaverMemoryStorage {
	return &fileSaverMemoryStorage{
		files: make(map[string][]byte),
	}
}

func (m *fileSaverMemoryStorage) Save(filename string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf := make([]byte, len(data))
	copy(buf, data)
	m.files[filename] = buf
	return nil
}

func (m *fileSaverMemoryStorage) Load(filename string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.files[filename]...), nil
}

func (m *fileSaverMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	data, _ := m.Load(filename)
	ch <- data
	close(ch)
	return ch, nil
}

func (m *fileSaverMemoryStorage) Download(filename string, targetPath string) error {
	data, _ := m.Load(filename)
	return os.WriteFile(targetPath, data, 0o600)
}

func (m *fileSaverMemoryStorage) Exists(filename string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[filename]
	return ok, nil
}

func (m *fileSaverMemoryStorage) Delete(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, filename)
	return nil
}

func (m *fileSaverMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
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

func openFileSaverTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "file_saver_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&toolfile.ToolFile{}); err != nil {
		t.Fatalf("auto migrate tool_files: %v", err)
	}
	return db
}

func TestFileSaverImpl_SaveBinaryString_DefaultSignedURLMode(t *testing.T) {
	cfg := &appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesURL:           "https://api.zgi.im",
			FilesAccessTimeout: 3600,
		},
	}

	manager := toolfile.NewToolFileManager(openFileSaverTestDB(t), newFileSaverMemoryStorage())
	signer := toolfile.NewFileSignature(cfg)
	fileSaver := NewFileSaverImpl("user-1", "workspace-1", manager, signer)

	savedFile, err := fileSaver.SaveBinaryString([]byte("png-bytes"), "image/png", file.FileTypeImage, nil)
	if err != nil {
		t.Fatalf("SaveBinaryString returned error: %v", err)
	}

	parsed, err := url.Parse(*savedFile.URL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if parsed.Query().Get("expires_at") == "" || parsed.Query().Get("expires_at") == "0" {
		t.Fatalf("expires_at = %q, want future timestamp", parsed.Query().Get("expires_at"))
	}
}

func TestFileSaverImpl_SaveBinaryString_PermanentURLMode(t *testing.T) {
	cfg := &appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesURL:           "https://api.zgi.im",
			FilesAccessTimeout: 3600,
		},
	}

	manager := toolfile.NewToolFileManager(openFileSaverTestDB(t), newFileSaverMemoryStorage())
	signer := toolfile.NewFileSignature(cfg)
	fileSaver := NewFileSaverImplWithLifecycleAndURLMode(
		"user-1",
		"workspace-1",
		manager,
		signer,
		toolfile.ToolFileLifecyclePersistent,
		nil,
		toolfile.ToolFileURLModePermanent,
	)

	savedFile, err := fileSaver.SaveBinaryString([]byte("png-bytes"), "image/png", file.FileTypeImage, nil)
	if err != nil {
		t.Fatalf("SaveBinaryString returned error: %v", err)
	}

	parsed, err := url.Parse(*savedFile.URL)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if parsed.Query().Get("expires_at") != "0" {
		t.Fatalf("expires_at = %q, want %q", parsed.Query().Get("expires_at"), "0")
	}
}

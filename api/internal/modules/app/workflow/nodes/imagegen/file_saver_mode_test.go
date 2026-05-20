package imagegen

import (
	"context"
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

type imagegenMemoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newImagegenMemoryStorage() *imagegenMemoryStorage {
	return &imagegenMemoryStorage{
		files: make(map[string][]byte),
	}
}

func (m *imagegenMemoryStorage) Save(filename string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf := make([]byte, len(data))
	copy(buf, data)
	m.files[filename] = buf
	return nil
}

func (m *imagegenMemoryStorage) Load(filename string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.files[filename]...), nil
}

func (m *imagegenMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	data, _ := m.Load(filename)
	ch <- data
	close(ch)
	return ch, nil
}

func (m *imagegenMemoryStorage) Download(filename string, targetPath string) error {
	data, _ := m.Load(filename)
	return os.WriteFile(targetPath, data, 0o600)
}

func (m *imagegenMemoryStorage) Exists(filename string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.files[filename]
	return ok, nil
}

func (m *imagegenMemoryStorage) Delete(filename string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, filename)
	return nil
}

func (m *imagegenMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
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

func openImagegenTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "imagegen_file_saver_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&toolfile.ToolFile{}); err != nil {
		t.Fatalf("auto migrate tool_files: %v", err)
	}
	return db
}

func TestBuildFileSaver_UsesPermanentToolFileURLs(t *testing.T) {
	previousConfig := appconfig.GlobalConfig
	previousManager := toolfile.GlobalToolFileManager
	previousSignature := toolfile.GlobalFileSignature
	appconfig.GlobalConfig = &appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesURL:           "https://api.zgi.im",
			FilesAccessTimeout: 3600,
		},
	}
	toolfile.GlobalToolFileManager = toolfile.NewToolFileManager(openImagegenTestDB(t), newImagegenMemoryStorage())
	toolfile.GlobalFileSignature = toolfile.NewFileSignature(appconfig.GlobalConfig)
	t.Cleanup(func() {
		appconfig.GlobalConfig = previousConfig
		toolfile.GlobalToolFileManager = previousManager
		toolfile.GlobalFileSignature = previousSignature
	})

	testCases := []struct {
		name      string
		lifecycle string
	}{
		{name: "persistent", lifecycle: string(toolfile.ToolFileLifecyclePersistent)},
		{name: "temporary", lifecycle: string(toolfile.ToolFileLifecycleTemporary)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			saver := buildFileSaver("user-1", "workspace-1", tc.lifecycle)
			savedFile, err := saver.SaveBinaryString([]byte("png-bytes"), "image/png", file.FileTypeImage, nil)
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

			fileData, mimeType, err := toolfile.GlobalToolFileManager.GetFileBinary(context.Background(), *savedFile.RelatedID)
			if err != nil {
				t.Fatalf("GetFileBinary returned error: %v", err)
			}
			if mimeType != "image/png" || string(fileData) != "png-bytes" {
				t.Fatalf("unexpected persisted file, mimeType=%q body=%q", mimeType, string(fileData))
			}
		})
	}
}

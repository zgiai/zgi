package filegenerator_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	appconfig "github.com/zgiai/ginext/config"
	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/ginext/internal/modules/tools"
	filegenerator "github.com/zgiai/ginext/internal/modules/tools/builtin/filegenerator"
	"github.com/zgiai/ginext/pkg/storage"
	"gorm.io/gorm"
)

type memoryStorage struct {
	mu    sync.RWMutex
	files map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{files: make(map[string][]byte)}
}

func (m *memoryStorage) Save(filename string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[filename] = append([]byte(nil), data...)
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
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		result = append(result, storage.FileInfo{Key: key, Size: int64(len(value))})
	}
	return result, nil
}

func TestGenerateFileTool_Invoke_CreatesMarkdownToolFile(t *testing.T) {
	memStorage := setupFileGeneratorGlobals(t)
	tool := filegenerator.NewGenerateFileTool("tenant-1")

	messages, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"content":  "# Hello\nGenerated content",
		"format":   "md",
		"filename": "报告",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages length = %d, want 2", len(messages))
	}
	if messages[0].Type != tools.ToolInvokeMessageTypeFile {
		t.Fatalf("message type = %s, want file", messages[0].Type)
	}

	fileData, ok := messages[0].Meta["file"].(map[string]interface{})
	if !ok {
		t.Fatalf("file meta = %#v, want map", messages[0].Meta["file"])
	}
	if fileData["type"] != workflowfile.FileTypeDocument {
		t.Fatalf("file type = %v, want document", fileData["type"])
	}
	if fileData["transfer_method"] != workflowfile.FileTransferMethodToolFile {
		t.Fatalf("transfer method = %v, want tool_file", fileData["transfer_method"])
	}
	if fileData["filename"] != "报告.md" {
		t.Fatalf("filename = %v, want report.md", fileData["filename"])
	}

	rawURL, ok := fileData["url"].(string)
	if !ok || rawURL == "" {
		t.Fatalf("url = %v, want non-empty string", fileData["url"])
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Query().Get("expires_at") == "" {
		t.Fatalf("signed url missing expires_at: %s", rawURL)
	}
	if parsed.Query().Get("download") != "" {
		t.Fatalf("preview url download flag = %q, want empty", parsed.Query().Get("download"))
	}
	rawDownloadURL, ok := fileData["download_url"].(string)
	if !ok || rawDownloadURL == "" {
		t.Fatalf("download_url = %v, want non-empty string", fileData["download_url"])
	}
	downloadParsed, err := url.Parse(rawDownloadURL)
	if err != nil {
		t.Fatalf("parse download_url: %v", err)
	}
	if downloadParsed.Query().Get("download") != "1" {
		t.Fatalf("download url download flag = %q, want 1", downloadParsed.Query().Get("download"))
	}

	files, err := memStorage.List("tools/tenant-1/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("stored files length = %d, want 1", len(files))
	}
	data, err := memStorage.Load(files[0].Key)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(data) != "# Hello\nGenerated content" {
		t.Fatalf("stored content = %q", string(data))
	}
}

func TestGenerateFileTool_Invoke_EscapesHTMLContent(t *testing.T) {
	memStorage := setupFileGeneratorGlobals(t)
	tool := filegenerator.NewGenerateFileTool("tenant-1")

	_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"content":  `<script>alert(1)</script>`,
		"format":   "html",
		"filename": "page",
		"title":    "Demo",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	files, err := memStorage.List("tools/tenant-1/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	data, err := memStorage.Load(files[0].Key)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	html := string(data)
	if strings.Contains(html, "<script>") {
		t.Fatalf("html content contains executable script: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("html content did not escape script: %s", html)
	}
}

func TestGenerateFileTool_Invoke_RejectsInvalidJSON(t *testing.T) {
	setupFileGeneratorGlobals(t)
	tool := filegenerator.NewGenerateFileTool("tenant-1")

	_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"content": "{not-json",
		"format":  "json",
	}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("Invoke() error = %v, want valid JSON error", err)
	}
}

func TestGenerateFileTool_MetadataIncludesLocalizedHumanText(t *testing.T) {
	provider := filegenerator.NewProvider().GetEntity()
	if got := provider.Identity.Label.Get("zh_Hans"); got != "文件生成器" {
		t.Fatalf("provider zh_Hans label = %q, want localized text", got)
	}
	if got := provider.Identity.Description.Get("zh_Hans"); got == "" || got == provider.Identity.Description.Get("en_US") {
		t.Fatalf("provider zh_Hans description = %q, want localized text", got)
	}

	entity := filegenerator.NewGenerateFileTool("").GetEntity()
	if got := entity.Identity.Label.Get("zh_Hans"); got != "生成文件" {
		t.Fatalf("tool zh_Hans label = %q, want localized text", got)
	}
	for _, param := range entity.Parameters {
		if got := param.Label.Get("zh_Hans"); got == "" || got == param.Label.Get("en_US") {
			t.Fatalf("parameter %s zh_Hans label = %q, want localized text", param.Name, got)
		}
		if got := param.HumanDescription.Get("zh_Hans"); got == "" || got == param.HumanDescription.Get("en_US") {
			t.Fatalf("parameter %s zh_Hans human description = %q, want localized text", param.Name, got)
		}
		for _, option := range param.Options {
			if got := option.Label.Get("zh_Hans"); got == "" || got == option.Label.Get("en_US") {
				t.Fatalf("parameter %s option %s zh_Hans label = %q, want localized text", param.Name, option.Value, got)
			}
		}
	}
}

func setupFileGeneratorGlobals(t *testing.T) *memoryStorage {
	t.Helper()

	previousManager := tool_file.GlobalToolFileManager
	previousSignature := tool_file.GlobalFileSignature
	t.Cleanup(func() {
		tool_file.GlobalToolFileManager = previousManager
		tool_file.GlobalFileSignature = previousSignature
	})

	dbPath := filepath.Join(t.TempDir(), "file_generator_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	if err := db.AutoMigrate(&tool_file.ToolFile{}); err != nil {
		t.Fatalf("auto migrate tool_files: %v", err)
	}

	memStorage := newMemoryStorage()
	tool_file.GlobalToolFileManager = tool_file.NewToolFileManager(db, memStorage)
	tool_file.GlobalFileSignature = tool_file.NewFileSignature(&appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesURL:           "https://api.zgi.im",
			FilesAccessTimeout: 3600,
		},
	})
	return memStorage
}

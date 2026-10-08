package executionruntime

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type agentExecutionTestStorage struct{ *storage.LocalStorage }

func (s *agentExecutionTestStorage) LoadStream(key string) (<-chan []byte, error) {
	data, err := s.Load(key)
	if err != nil {
		return nil, err
	}
	stream := make(chan []byte, 1)
	stream <- data
	close(stream)
	return stream, nil
}

func (*agentExecutionTestStorage) Download(string, string) error { return nil }

func TestDefaultAgentExecutionArtifactPersisterStoresToolFile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:agent-execution-artifact?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&tool_file.ToolFile{}); err != nil {
		t.Fatal(err)
	}
	previousManager := tool_file.GlobalToolFileManager
	previousSignature := tool_file.GlobalFileSignature
	tool_file.InitToolFileManager(db, &agentExecutionTestStorage{LocalStorage: storage.NewLocalStorage(t.TempDir())})
	tool_file.InitFileSignature(&config.Config{App: config.AppConfig{SecretKey: "test-only-signing-secret", FilesURL: "https://files.example.test"}})
	t.Cleanup(func() {
		tool_file.GlobalToolFileManager = previousManager
		tool_file.GlobalFileSignature = previousSignature
	})

	data := []byte("item,qty\napple,5\nbanana,5\n")
	artifact, err := (defaultAgentExecutionArtifactPersister{}).Persist(t.Context(), agentExecutionScope{OrganizationID: "org-1", SubjectID: "account-1"}, "conversation-1", "artifacts/result.csv", data)
	if err != nil {
		t.Fatalf("persist artifact: %v", err)
	}
	fileID, _ := artifact["file_id"].(string)
	if fileID == "" || artifact["filename"] != "result.csv" || artifact["lifecycle"] != "persistent" {
		t.Fatalf("artifact metadata = %#v", artifact)
	}
	if url, _ := artifact["download_url"].(string); !strings.Contains(url, "/console/api/files/tools/"+fileID+".csv?") {
		t.Fatalf("signed download URL = %q", url)
	}
	if url, _ := artifact["download_url"].(string); !strings.HasSuffix(url, "&download=1") {
		t.Fatalf("download URL is missing download mode: %q", url)
	}
	stored, _, err := tool_file.GetFileBinaryGlobal(t.Context(), fileID)
	if err != nil || string(stored) != string(data) {
		t.Fatalf("stored file = %q, error = %v", stored, err)
	}
}

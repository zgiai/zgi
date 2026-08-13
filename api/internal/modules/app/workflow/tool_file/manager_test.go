package tool_file

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type deleteTestStorage struct {
	deleteErr error
	exists    bool
}

type localDeleteTestStorage struct {
	*storage.LocalStorage
}

func (s *localDeleteTestStorage) LoadStream(key string) (<-chan []byte, error) {
	data, err := s.Load(key)
	if err != nil {
		return nil, err
	}
	stream := make(chan []byte, 1)
	stream <- data
	close(stream)
	return stream, nil
}

func (*localDeleteTestStorage) Download(string, string) error { return nil }

func (*deleteTestStorage) Save(string, []byte) error                { return nil }
func (*deleteTestStorage) Load(string) ([]byte, error)              { return nil, nil }
func (*deleteTestStorage) LoadStream(string) (<-chan []byte, error) { return nil, nil }
func (*deleteTestStorage) Download(string, string) error            { return nil }
func (s *deleteTestStorage) Exists(string) (bool, error)            { return s.exists, nil }
func (s *deleteTestStorage) Delete(string) error                    { return s.deleteErr }
func (*deleteTestStorage) List(string) ([]storage.FileInfo, error)  { return nil, nil }

func TestResolveLifecycleUsesSevenDayTemporaryTTL(t *testing.T) {
	before := time.Now().Add(DefaultTemporaryToolFileTTL)
	lifecycle, expiresAt, err := resolveLifecycle(ToolFileLifecycleTemporary, nil)
	after := time.Now().Add(DefaultTemporaryToolFileTTL)
	if err != nil {
		t.Fatalf("resolveLifecycle() error = %v", err)
	}
	if lifecycle != ToolFileLifecycleTemporary || expiresAt == nil {
		t.Fatalf("resolveLifecycle() = %q, %v, want temporary expiry", lifecycle, expiresAt)
	}
	if expiresAt.Before(before) || expiresAt.After(after) {
		t.Fatalf("expiresAt = %v, want between %v and %v", expiresAt, before, after)
	}
}

func TestEnsureToolFileAvailableRejectsExpiredTemporaryFile(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(-time.Second)
	err := ensureToolFileAvailable(&ToolFile{
		ID:        "tool-1",
		Lifecycle: string(ToolFileLifecycleTemporary),
		ExpiresAt: &expiresAt,
	}, now)
	if !errors.Is(err, ErrToolFileExpired) {
		t.Fatalf("ensureToolFileAvailable() error = %v, want ErrToolFileExpired", err)
	}
}

func TestEnsureToolFileAvailableKeepsUnexpiredAndPersistentFiles(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	for _, toolFile := range []*ToolFile{
		{ID: "temporary", Lifecycle: string(ToolFileLifecycleTemporary), ExpiresAt: &future},
		{ID: "persistent", Lifecycle: string(ToolFileLifecyclePersistent)},
	} {
		if err := ensureToolFileAvailable(toolFile, now); err != nil {
			t.Fatalf("ensureToolFileAvailable(%s) error = %v", toolFile.ID, err)
		}
	}
}

func TestDeleteStoredObjectRemovesConfiguredLocalFileAndKeepsMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tool-file-local-delete?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatal(err)
	}
	local := &localDeleteTestStorage{LocalStorage: storage.NewLocalStorage(t.TempDir())}
	manager := NewToolFileManager(db, local)
	file, err := manager.CreateFileByRaw(t.Context(), CreateFileByRawParams{
		UserID:   "account-1",
		TenantID: "organization-1",
		FileData: []byte("music"),
		MimeType: "audio/mpeg",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.DeleteStoredObject(t.Context(), file.ID, file.TenantID, file.UserID); err != nil {
		t.Fatalf("DeleteStoredObject() error = %v", err)
	}
	exists, err := local.Exists(file.FileKey)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("local music file still exists after DeleteStoredObject()")
	}
	var count int64
	if err := db.Model(&ToolFile{}).Where("id = ?", file.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("tool file metadata count = %d, want 1", count)
	}
}

func TestDeleteToolFileKeepsMetadataWhenStorageObjectStillExists(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tool-file-delete-failure?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatal(err)
	}
	file := &ToolFile{
		ID:        "file-1",
		UserID:    "account-1",
		TenantID:  "organization-1",
		FileKey:   "tools/organization-1/file-1.mp3",
		MimeType:  "audio/mpeg",
		Name:      "music.mp3",
		Lifecycle: string(ToolFileLifecyclePersistent),
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatal(err)
	}
	manager := NewToolFileManager(db, &deleteTestStorage{
		deleteErr: errors.New("object storage unavailable"),
		exists:    true,
	})

	err = manager.DeleteToolFile(context.Background(), file.ID)
	if err == nil {
		t.Fatal("DeleteToolFile() error = nil, want storage deletion error")
	}
	var count int64
	if err := db.Model(&ToolFile{}).Where("id = ?", file.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("tool file metadata count = %d, want 1", count)
	}
}

func TestDeleteStoredObjectAllowsRetryAfterObjectIsAlreadyMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tool-file-delete-retry?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatal(err)
	}
	file := &ToolFile{
		ID:        "file-1",
		UserID:    "account-1",
		TenantID:  "organization-1",
		FileKey:   "tools/organization-1/file-1.mp3",
		MimeType:  "audio/mpeg",
		Name:      "music.mp3",
		Lifecycle: string(ToolFileLifecyclePersistent),
	}
	if err := db.Create(file).Error; err != nil {
		t.Fatal(err)
	}
	manager := NewToolFileManager(db, &deleteTestStorage{
		deleteErr: errors.New("object not found"),
		exists:    false,
	})

	if err := manager.DeleteStoredObject(t.Context(), file.ID, file.TenantID, file.UserID); err != nil {
		t.Fatalf("DeleteStoredObject() retry error = %v, want nil for an already missing object", err)
	}
}

func TestDeleteStoredObjectRejectsFileOutsideRequestedScope(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:tool-file-delete-scope?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ToolFile{}); err != nil {
		t.Fatal(err)
	}
	local := &localDeleteTestStorage{LocalStorage: storage.NewLocalStorage(t.TempDir())}
	manager := NewToolFileManager(db, local)
	file, err := manager.CreateFileByRaw(t.Context(), CreateFileByRawParams{
		UserID:   "account-1",
		TenantID: "organization-1",
		FileData: []byte("music"),
		MimeType: "audio/mpeg",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = manager.DeleteStoredObject(t.Context(), file.ID, file.TenantID, "another-account")
	if err == nil {
		t.Fatal("DeleteStoredObject() error = nil, want scope mismatch error")
	}
	exists, err := local.Exists(file.FileKey)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("out-of-scope storage object was deleted")
	}
}

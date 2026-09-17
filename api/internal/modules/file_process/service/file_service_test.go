package service

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// fakeUploadRecordRepo overrides only the lookup the compensation path performs; the
// embedded interface satisfies the rest of the repository contract.
type fakeUploadRecordRepo struct {
	repository.FileRepository
	record *model.UploadFile
	err    error
}

func (f fakeUploadRecordRepo) GetByID(context.Context, string) (*model.UploadFile, error) {
	return f.record, f.err
}

// fakeUploadStorage records deletions instead of reaching a real bucket.
type fakeUploadStorage struct {
	storage.Storage
	deleted []string
}

func (f *fakeUploadStorage) Delete(key string) error {
	f.deleted = append(f.deleted, key)
	return nil
}

func TestIsAssetProcessableExtension(t *testing.T) {
	tests := []struct {
		ext  string
		want bool
	}{
		{ext: "pdf", want: true},
		{ext: ".docx", want: true},
		{ext: "png", want: true},
		{ext: ".jpg", want: true},
		{ext: "webp", want: true},
		{ext: "mp4", want: false},
		{ext: "mp3", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := isAssetProcessableExtension(tt.ext); got != tt.want {
				t.Fatalf("processable=%v want %v", got, tt.want)
			}
		})
	}
}

func TestGetUploadConfigIncludesQueueAndConcurrencyLimits(t *testing.T) {
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Upload: config.UploadConfig{
			FileBatchLimit:   5,
			UploadQueueLimit: 200,
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})

	got := (&fileService{}).GetUploadConfig()
	if got.BatchCountLimit != 5 {
		t.Fatalf("BatchCountLimit = %d, want 5", got.BatchCountLimit)
	}
	if got.UploadQueueLimit != 200 {
		t.Fatalf("UploadQueueLimit = %d, want 200", got.UploadQueueLimit)
	}
}

// A transaction can report a failure after the database actually committed. The
// object is still referenced then, so it must survive the compensation.
func TestCompensateFailedUploadKeepsObjectWhenRecordPersisted(t *testing.T) {
	store := &fakeUploadStorage{}
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{record: &model.UploadFile{ID: "file-1"}},
		storage:  store,
	}

	svc.compensateFailedUpload(context.Background(), "upload_files/org/file-1.png", "file-1")

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want nothing removed while the record exists", store.deleted)
	}
}

func TestCompensateFailedUploadDeletesObjectWhenRecordIsAbsent(t *testing.T) {
	store := &fakeUploadStorage{}
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{err: gorm.ErrRecordNotFound},
		storage:  store,
	}

	svc.compensateFailedUpload(context.Background(), "upload_files/org/file-1.png", "file-1")

	if len(store.deleted) != 1 || store.deleted[0] != "upload_files/org/file-1.png" {
		t.Fatalf("deleted = %v, want the uploaded object removed", store.deleted)
	}
}

// When the lookup itself fails the outcome is unknown; an orphan object is the safer
// failure, so the object is kept.
func TestCompensateFailedUploadKeepsObjectWhenLookupFails(t *testing.T) {
	store := &fakeUploadStorage{}
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{err: errors.New("connection reset by peer")},
		storage:  store,
	}

	svc.compensateFailedUpload(context.Background(), "upload_files/org/file-1.png", "file-1")

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want nothing removed while the outcome is unknown", store.deleted)
	}
}

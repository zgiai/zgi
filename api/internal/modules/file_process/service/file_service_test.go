package service

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
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

// fakeUploadStorage records deletions and reads instead of reaching a real bucket.
type fakeUploadStorage struct {
	storage.Storage
	deleted   []string
	deleteErr error
	loaded    []string
	content   []byte
	loadErr   error
}

func (f *fakeUploadStorage) Delete(key string) error {
	f.deleted = append(f.deleted, key)
	return f.deleteErr
}

func (f *fakeUploadStorage) Load(key string) ([]byte, error) {
	f.loaded = append(f.loaded, key)
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return f.content, nil
}

// fakeQuotaService only has to be non-nil: the deletion preparation resolves identifiers,
// it does not record anything.
type fakeQuotaService struct {
	interfaces.QuotaService
}

// withETLType pins the ETL backend, which gates whether OCR quota is involved at all.
func withETLType(t *testing.T, etlType string) {
	t.Helper()
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{ETL: config.ETLConfig{Type: etlType}}
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})
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

// A repository that answers with a nil record and no error cannot be read as "the
// record is gone". Only a positive not-found result may remove the object, otherwise
// the guard would silently fall back to deleting on any unexpected shape.
func TestCompensateFailedUploadKeepsObjectWhenRepoAnswersAmbiguously(t *testing.T) {
	store := &fakeUploadStorage{}
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{},
		storage:  store,
	}

	svc.compensateFailedUpload(context.Background(), "upload_files/org/file-1.png", "file-1")

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want nothing removed when absence is unconfirmed", store.deleted)
	}
}

// A storage outage right after the rollback would leave the object with no record and no
// pending row at all, so the key has to reach the retry worker instead of a log line.
func TestCompensateFailedUploadRecordsAPendingRemovalWhenStorageRefuses(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	store := &fakeUploadStorage{deleteErr: errors.New("storage unavailable")}
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{err: gorm.ErrRecordNotFound},
		storage:  store,
		db:       db,
	}
	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	svc.compensateFailedUpload(context.Background(), "upload_files/org/file-1.png", "file-1")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the failed removal was not handed to the retry worker: %v", err)
	}
}

// A removal that succeeds needs no pending row.
func TestRemoveOrphanedObjectKeepsTheOutboxOutOfTheHappyPath(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, db: db}
	// Registered only to prove it stays unmatched: an insert here would mean the object was
	// handed to the worker even though the removal already succeeded.
	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	svc.removeOrphanedObject(context.Background(), "upload_files/org/file-1.png")

	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v, want the object removed inline", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err == nil {
		t.Fatal("a pending row was recorded for an object that was already removed")
	}
}

// The replacement object must survive a transaction that reported an error after the
// database actually committed, because the record then points at it.
func TestReplacementObjectIsReferencedKeepsTheObjectWhenTheRecordPointsAtIt(t *testing.T) {
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{record: &model.UploadFile{ID: "file-1", Key: "upload_files/org/new.png"}},
	}

	if !svc.replacementObjectIsReferenced(context.Background(), "file-1", "upload_files/org/new.png") {
		t.Fatal("replacementObjectIsReferenced = false, want the new object kept")
	}
}

func TestReplacementObjectIsReferencedRemovesTheObjectWhenTheRecordStillPointsAtTheOld(t *testing.T) {
	tests := []struct {
		name string
		repo fakeUploadRecordRepo
	}{
		{
			name: "the replacement did not land",
			repo: fakeUploadRecordRepo{record: &model.UploadFile{ID: "file-1", Key: "upload_files/org/old.png"}},
		},
		{
			name: "the record is gone",
			repo: fakeUploadRecordRepo{err: gorm.ErrRecordNotFound},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fileService{fileRepo: tt.repo}

			if svc.replacementObjectIsReferenced(context.Background(), "file-1", "upload_files/org/new.png") {
				t.Fatal("replacementObjectIsReferenced = true, want the unreferenced object removed")
			}
		})
	}
}

// When the record cannot be read, the outcome of the replacement is unknown: a stray object
// is the safer failure, so the new object is kept.
func TestReplacementObjectIsReferencedKeepsTheObjectWhenTheLookupFails(t *testing.T) {
	svc := &fileService{fileRepo: fakeUploadRecordRepo{err: errors.New("connection reset by peer")}}

	if !svc.replacementObjectIsReferenced(context.Background(), "file-1", "upload_files/org/new.png") {
		t.Fatal("replacementObjectIsReferenced = false, want the new object kept while the outcome is unknown")
	}
}

func uploadFileForDeletion(extension string) (*model.UploadFile, *uuid.UUID) {
	groupID := uuid.New()
	return &model.UploadFile{
		ID:             "file-1",
		Key:            "upload_files/org/file-1." + extension,
		Name:           "file-1." + extension,
		Extension:      extension,
		Size:           1024,
		CreatedBy:      uuid.New().String(),
		OrganizationID: groupID.String(),
	}, &groupID
}

// The object is read while it still exists. Reading it after removal is exactly what made
// the OCR refund collapse to a single page for every PDF.
func TestPrepareDeletionReadsTheObjectBeforeAnythingIsRemoved(t *testing.T) {
	withETLType(t, "Reducto")
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, quotaService: fakeQuotaService{}}
	file, groupID := uploadFileForDeletion("pdf")

	prep, err := svc.prepareDeletion(file, groupID)
	if err != nil {
		t.Fatalf("prepareDeletion: %v", err)
	}

	if !prep.quotaEnabled || !prep.countOCR {
		t.Fatalf("prep = %+v, want quota and OCR accounting enabled", prep)
	}
	if prep.ocrPages < 1 {
		t.Fatalf("ocrPages = %d, want at least one page", prep.ocrPages)
	}
	if len(store.loaded) != 1 || store.loaded[0] != file.Key {
		t.Fatalf("loaded = %v, want the file object read for its page count", store.loaded)
	}
	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, preparation must not remove anything", store.deleted)
	}
}

// Images and the other OCR formats count as a single page and need no read at all.
func TestPrepareDeletionDoesNotReadNonPDFFormats(t *testing.T) {
	withETLType(t, "Reducto")
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, quotaService: fakeQuotaService{}}
	file, groupID := uploadFileForDeletion("png")

	prep, err := svc.prepareDeletion(file, groupID)
	if err != nil {
		t.Fatalf("prepareDeletion: %v", err)
	}

	if !prep.quotaEnabled || !prep.countOCR || prep.ocrPages != 1 {
		t.Fatalf("prep = %+v, want one OCR page for an image", prep)
	}
	if len(store.loaded) != 0 {
		t.Fatalf("loaded = %v, want no read for an image", store.loaded)
	}
}

// OCR quota only moves under the Reducto and Mixed backends.
func TestPrepareDeletionSkipsOCROutsideReductoETL(t *testing.T) {
	withETLType(t, "zgi")
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, quotaService: fakeQuotaService{}}
	file, groupID := uploadFileForDeletion("pdf")

	prep, err := svc.prepareDeletion(file, groupID)
	if err != nil {
		t.Fatalf("prepareDeletion: %v", err)
	}

	if !prep.quotaEnabled || prep.countOCR {
		t.Fatalf("prep = %+v, want storage quota only", prep)
	}
	if len(store.loaded) != 0 {
		t.Fatalf("loaded = %v, want no OCR page read outside Reducto", store.loaded)
	}
}

// An unparseable creator used to fail inside the transaction, after the object had already
// been removed. It now fails before anything is touched.
func TestPrepareDeletionRejectsUnparseableCreatorWithoutTouchingStorage(t *testing.T) {
	withETLType(t, "Reducto")
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, quotaService: fakeQuotaService{}}
	file, groupID := uploadFileForDeletion("pdf")
	file.CreatedBy = "not-a-uuid"

	if _, err := svc.prepareDeletion(file, groupID); err == nil {
		t.Fatal("prepareDeletion = nil error, want a parse failure")
	}
	if len(store.deleted) != 0 || len(store.loaded) != 0 {
		t.Fatalf("deleted = %v loaded = %v, want storage untouched", store.deleted, store.loaded)
	}
}

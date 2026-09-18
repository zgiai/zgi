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

// WithTx and Create let the same fake serve the upload transaction, which never reaches the
// database itself: the test only decides what the record looks like afterwards.
func (f fakeUploadRecordRepo) WithTx(*gorm.DB) repository.FileRepository { return f }

func (f fakeUploadRecordRepo) Create(context.Context, *model.UploadFile) error { return nil }

// fakeUploadStorage records deletions and reads instead of reaching a real bucket.
type fakeUploadStorage struct {
	storage.Storage
	deleted   []string
	deleteErr error
	saved     []string
	saveErr   error
	loaded    []string
	content   []byte
	loadErr   error

	// beforeDelete lets a test observe when the removal happens.
	beforeDelete func()
	// afterSave lets a test follow the key the replacement wrote.
	afterSave func(key string)
}

func (f *fakeUploadStorage) Delete(key string) error {
	if f.beforeDelete != nil {
		f.beforeDelete()
	}
	f.deleted = append(f.deleted, key)
	return f.deleteErr
}

func (f *fakeUploadStorage) Save(key string, data []byte) error {
	f.saved = append(f.saved, key)
	if f.afterSave != nil {
		f.afterSave(key)
	}
	return f.saveErr
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
	db, mock := openFileObjectDeletionMockDB(t)
	store := &fakeUploadStorage{}
	svc := &fileService{
		fileRepo: fakeUploadRecordRepo{err: gorm.ErrRecordNotFound},
		storage:  store,
		db:       db,
	}
	// The removal is recorded before it is attempted, so it can be retried after a crash,
	// and the row is cleared again once the object is really gone.
	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	svc.compensateFailedUpload(context.Background(), "upload_files/org/file-1.png", "file-1")

	if len(store.deleted) != 1 || store.deleted[0] != "upload_files/org/file-1.png" {
		t.Fatalf("deleted = %v, want the uploaded object removed", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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

// The intent is durable before the removal is attempted, so a process that dies in between
// leaves a row the worker can finish. A removal that succeeds clears that row again.
func TestRemoveOrphanedObjectRecordsTheIntentBeforeRemoving(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, db: db}

	order := make([]string, 0, 2)
	// GORM callbacks are the only way to observe where the write sits relative to the
	// storage call, since the storage is a test double rather than a driver.
	if err := db.Callback().Create().After("gorm:create").Register("test:record-intent", func(*gorm.DB) {
		order = append(order, "record-intent")
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	store.beforeDelete = func() {
		order = append(order, "remove-object")
	}

	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	svc.removeOrphanedObject(context.Background(), "upload_files/org/file-1.png")

	if len(order) != 2 || order[0] != "record-intent" || order[1] != "remove-object" {
		t.Fatalf("call order = %v, want the pending row written before the removal", order)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "upload_files/org/file-1.png" {
		t.Fatalf("deleted = %v, want the object removed inline", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the pending row was not cleared after the removal: %v", err)
	}
}

// Even when the intent cannot be recorded, the object is still unreferenced: attempting the
// removal cannot make things worse, and skipping it would guarantee the leak. Nothing may
// claim the retry worker owns this removal either.
func TestRemoveOrphanedObjectStillRemovesTheObjectWhenTheIntentCannotBeRecorded(t *testing.T) {
	db, mock := openFileObjectDeletionMockDB(t)
	store := &fakeUploadStorage{}
	svc := &fileService{storage: store, db: db}
	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnError(errors.New("database unavailable"))
	// Registered only to prove it stays unmatched: clearing a row that was never written
	// would mean the code still treated the removal as owned by the retry worker.
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	svc.removeOrphanedObject(context.Background(), "upload_files/org/file-1.png")

	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v, want the object removed even without a pending row", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err == nil {
		t.Fatal("a removal without a pending row claimed the row the retry worker owns")
	}
}

// fakeReplacementRepo serves the replacement flow: the record the caller reads before the
// transaction, and whatever state the record is in afterwards.
type fakeReplacementRepo struct {
	repository.FileRepository
	current   *model.UploadFile
	after     *model.UploadFile
	updateErr error
}

func (r *fakeReplacementRepo) WithTx(*gorm.DB) repository.FileRepository { return r }

func (r *fakeReplacementRepo) GetByTenantAndID(context.Context, string, string) (*model.UploadFile, error) {
	return r.current, nil
}

func (r *fakeReplacementRepo) GetByID(context.Context, string) (*model.UploadFile, error) {
	return r.after, nil
}

func (r *fakeReplacementRepo) Update(context.Context, string, map[string]interface{}) error {
	return r.updateErr
}

func (r *fakeReplacementRepo) DeleteExtractionCaches(context.Context, string) error { return nil }

// The transaction reported an error, but the database actually committed: the record points
// at the new object, so removing it would leave the file with no content at all.
func TestReplaceFileContentKeepsTheNewObjectWhenTheCommitOutcomeIsUnknown(t *testing.T) {
	withStorageType(t, "aliyun-oss")
	db, mock := openFileObjectDeletionMockDB(t)
	organizationID := uuid.NewString()
	oldKey := "upload_files/" + organizationID + "/old.png"
	repo := &fakeReplacementRepo{
		current:   &model.UploadFile{ID: "file-1", Key: oldKey, Size: 100, Extension: "png"},
		updateErr: errors.New("connection reset by peer"),
	}
	store := &fakeUploadStorage{}
	// The transaction reported an error, but the record already points at the object the
	// replacement wrote, which is what a lost commit acknowledgement looks like.
	store.afterSave = func(key string) {
		repo.after = &model.UploadFile{ID: "file-1", Key: key, Size: 6, Extension: "png"}
	}
	svc := &fileService{fileRepo: repo, storage: store, db: db}

	mock.ExpectBegin()
	mock.ExpectRollback()

	if _, err := svc.ReplaceFileContent(context.Background(), "file-1", "new.png", []byte("pixels"), "image/png", uuid.NewString(), organizationID); err == nil {
		t.Fatal("ReplaceFileContent = nil error, want the transaction failure reported")
	}

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want the object the record points at kept", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The transaction failed before the record switched over, so the new object is unreachable
// and gets the same durable handoff as any other orphan.
func TestReplaceFileContentRemovesTheNewObjectWhenTheRecordStillPointsAtTheOld(t *testing.T) {
	withStorageType(t, "aliyun-oss")
	db, mock := openFileObjectDeletionMockDB(t)
	organizationID := uuid.NewString()
	oldKey := "upload_files/" + organizationID + "/old.png"
	repo := &fakeReplacementRepo{
		current:   &model.UploadFile{ID: "file-1", Key: oldKey, Size: 100, Extension: "png"},
		after:     &model.UploadFile{ID: "file-1", Key: oldKey, Size: 100, Extension: "png"},
		updateErr: errors.New("connection reset by peer"),
	}
	store := &fakeUploadStorage{}
	svc := &fileService{fileRepo: repo, storage: store, db: db}

	mock.ExpectBegin()
	mock.ExpectRollback()
	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)DELETE FROM "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))

	if _, err := svc.ReplaceFileContent(context.Background(), "file-1", "new.png", []byte("pixels"), "image/png", uuid.NewString(), organizationID); err == nil {
		t.Fatal("ReplaceFileContent = nil error, want the transaction failure reported")
	}

	if len(store.deleted) != 1 {
		t.Fatalf("deleted = %v, want the unreferenced replacement object removed", store.deleted)
	}
	if store.deleted[0] == oldKey {
		t.Fatalf("deleted = %v, want the new object removed rather than the one still in use", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
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

// withStorageType pins the storage backend and the upload limits the replacement flow reads.
func withStorageType(t *testing.T, storageType string) {
	t.Helper()
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Storage: config.StorageConfig{Type: storageType},
		ETL:     config.ETLConfig{Type: "zgi"},
		Upload: config.UploadConfig{
			FileSizeLimit:    10,
			ImageSizeLimit:   10,
			VideoSizeLimit:   10,
			AudioSizeLimit:   10,
			FileBatchLimit:   10,
			BatchUploadLimit: 10,
		},
	}
	t.Cleanup(func() {
		config.GlobalConfig = previous
	})
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

// The replacement ambiguity that matters is a commit that fails after every statement ran:
// the row is written, the record points at the new object, and the caller still sees an
// error. The new object must survive that.
func TestReplaceFileContentKeepsTheNewObjectWhenCommitFailsAfterTheStatementsRan(t *testing.T) {
	withStorageType(t, "aliyun-oss")
	db, mock := openFileObjectDeletionMockDB(t)
	organizationID := uuid.NewString()
	oldKey := "upload_files/" + organizationID + "/old.png"
	repo := &fakeReplacementRepo{
		current: &model.UploadFile{ID: "file-1", Key: oldKey, Size: 100, Extension: "png"},
	}
	store := &fakeUploadStorage{}
	// Every statement succeeded, so the record points at the object the replacement wrote.
	store.afterSave = func(key string) {
		repo.after = &model.UploadFile{ID: "file-1", Key: key, Size: 6, Extension: "png"}
	}
	svc := &fileService{fileRepo: repo, storage: store, db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)INSERT INTO "file_object_deletions".*`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit().WillReturnError(errors.New("connection reset by peer"))

	if _, err := svc.ReplaceFileContent(context.Background(), "file-1", "new.png", []byte("pixels"), "image/png", uuid.NewString(), organizationID); err == nil {
		t.Fatal("ReplaceFileContent = nil error, want the lost commit acknowledgement reported")
	}

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want the object the record points at kept", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// The same ambiguity on the upload path: the record was written, the commit acknowledgement
// was lost, and the compensating delete must not remove the object the record points at.
func TestUploadKeepsTheObjectWhenCommitFailsAfterTheRecordWasWritten(t *testing.T) {
	withStorageType(t, "aliyun-oss")
	db, mock := openFileObjectDeletionMockDB(t)
	// The reconciliation reads the record back, and it is there because the commit landed.
	repo := fakeUploadRecordRepo{record: &model.UploadFile{ID: "file-1", Key: "upload_files/org/file-1.png"}}
	store := &fakeUploadStorage{}
	svc := &fileService{fileRepo: repo, storage: store, db: db}

	mock.ExpectBegin()
	mock.ExpectCommit().WillReturnError(errors.New("connection reset by peer"))

	file, err := svc.UploadFileWithOptions(context.Background(), "pixel.png", []byte("pixels"), "image/png",
		uuid.NewString(), uuid.NewString(), model.CreatedByRoleAccount, nil, nil, true, false, UploadFileOptions{})
	if err == nil {
		t.Fatal("UploadFileWithOptions = nil error, want the lost commit acknowledgement reported")
	}
	if file != nil {
		t.Fatalf("file = %+v, want none for a failed upload", file)
	}

	if len(store.deleted) != 0 {
		t.Fatalf("deleted = %v, want the object of the committed record kept", store.deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

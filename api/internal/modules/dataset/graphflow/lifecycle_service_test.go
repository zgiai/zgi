package graphflow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type lifecycleTestDataset struct {
	ID                               string `gorm:"primaryKey"`
	OrganizationID                   string `gorm:"not null"`
	WorkspaceID                      string
	GraphStatus                      string `gorm:"not null;default:'disabled'"`
	GraphRevision                    int64  `gorm:"not null;default:0"`
	GraphAvailableRevision           *int64
	GraphProjectedRevision           int64 `gorm:"not null;default:0"`
	GraphVisibilityRevision          int64 `gorm:"not null;default:0"`
	GraphProjectedVisibilityRevision int64 `gorm:"not null;default:0"`
	GraphCurrentRunID                *string
	GraphProgress                    int `gorm:"not null;default:0"`
	GraphErrorCode                   *string
	GraphErrorMessage                *string
	GraphUpdatedAt                   *time.Time
	GraphReadyAt                     *time.Time
	UpdatedAt                        time.Time
}

type lifecycleTestRef struct {
	ID                uuid.UUID `gorm:"primaryKey"`
	OrganizationID    string
	WorkspaceID       *string
	DatasetID         string
	AssetID           uuid.UUID
	DatasetDocumentID *uuid.UUID
	SyncRunID         *uuid.UUID
	Status            string
	RetrievalEnabled  bool
	GraphRunID        *uuid.UUID
	GraphSyncStatus   *string
	UpdatedAt         time.Time
	DeletedAt         gorm.DeletedAt
}

type lifecycleTestTask struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	TenantID           uuid.UUID
	KBID               uuid.UUID `gorm:"column:kb_id"`
	DocumentID         uuid.UUID
	SegmentID          *uuid.UUID
	RunID              *uuid.UUID
	TaskType           string
	ExtractionStrategy string
	Status             string
	Progress           int
	StartedAt          *time.Time
	CompletedAt        *time.Time
	ErrorMessage       string
	RetryCount         int
	AttemptNo          int
	LeaseExpiresAt     *time.Time
	HeartbeatAt        *time.Time
	ErrorCode          string
	Metadata           map[string]interface{} `gorm:"serializer:json"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (lifecycleTestTask) TableName() string {
	return "graphflow_tasks"
}

func (lifecycleTestRef) TableName() string {
	return "data_library_knowledge_base_asset_refs"
}

func TestLifecycleServiceSupersedesPublishForReplacedDocument(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	datasetID := uuid.New()
	organizationID := uuid.New()
	refID := uuid.New()
	oldDocumentID := uuid.New()
	currentDocumentID := uuid.New()
	oldSyncRunID := uuid.New()
	currentSyncRunID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		GraphRevision:  1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lifecycleTestRef{
		ID:                refID,
		OrganizationID:    organizationID.String(),
		DatasetID:         datasetID.String(),
		AssetID:           uuid.New(),
		DatasetDocumentID: &currentDocumentID,
		SyncRunID:         &currentSyncRunID,
		Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	run := &graphmodel.GraphFlowRun{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		DocumentID:     &oldDocumentID,
		SourceRefID:    &refID,
		SyncRunID:      &oldSyncRunID,
		GraphRevision:  1,
		Trigger:        "test",
		Mode:           graphmodel.GraphFlowRunModeBuild,
		Status:         graphmodel.GraphFlowRunStatusProcessing,
		IdempotencyKey: "publish:stale-document",
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatal(err)
	}

	if err := NewLifecycleService(db).PublishRun(context.Background(), run.ID); err != nil {
		t.Fatal(err)
	}
	var stored graphmodel.GraphFlowRun
	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != graphmodel.GraphFlowRunStatusSuperseded {
		t.Fatalf("run status=%s", stored.Status)
	}
}

func (lifecycleTestDataset) TableName() string {
	return "datasets"
}

func openLifecycleTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:graphflow-lifecycle-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	baseModels := []any{
		&lifecycleTestDataset{},
		&graphmodel.GraphFlowRun{},
		&graphmodel.GraphOutboxEvent{},
		&lifecycleTestTask{},
	}
	baseModels = append(baseModels, models...)
	if err := db.AutoMigrate(baseModels...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLifecycleServiceReconcilesDocumentProgressAndPublishesDataset(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	firstDocumentID := uuid.New()
	secondDocumentID := uuid.New()
	firstRunID := uuid.New()
	secondRunID := uuid.New()
	readyStatus := "processing"

	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		GraphStatus:    "queued",
		GraphRevision:  2,
	}).Error; err != nil {
		t.Fatal(err)
	}
	refs := []lifecycleTestRef{
		{
			ID:                uuid.New(),
			OrganizationID:    organizationID.String(),
			DatasetID:         datasetID.String(),
			AssetID:           uuid.New(),
			DatasetDocumentID: &firstDocumentID,
			Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
			RetrievalEnabled:  true,
			GraphRunID:        &firstRunID,
			GraphSyncStatus:   &readyStatus,
		},
		{
			ID:                uuid.New(),
			OrganizationID:    organizationID.String(),
			DatasetID:         datasetID.String(),
			AssetID:           uuid.New(),
			DatasetDocumentID: &secondDocumentID,
			Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
			RetrievalEnabled:  true,
			GraphRunID:        &secondRunID,
			GraphSyncStatus:   &readyStatus,
		},
	}
	if err := db.Create(&refs).Error; err != nil {
		t.Fatal(err)
	}
	runs := []graphmodel.GraphFlowRun{
		{
			ID:             firstRunID,
			OrganizationID: organizationID,
			DatasetID:      datasetID,
			DocumentID:     &firstDocumentID,
			GraphRevision:  1,
			Trigger:        "test",
			Mode:           graphmodel.GraphFlowRunModeBuild,
			Status:         graphmodel.GraphFlowRunStatusProcessing,
			IdempotencyKey: "build:first-document",
		},
		{
			ID:             secondRunID,
			OrganizationID: organizationID,
			DatasetID:      datasetID,
			DocumentID:     &secondDocumentID,
			GraphRevision:  2,
			Trigger:        "test",
			Mode:           graphmodel.GraphFlowRunModeBuild,
			Status:         graphmodel.GraphFlowRunStatusProcessing,
			IdempotencyKey: "build:second-document",
		},
	}
	if err := db.Create(&runs).Error; err != nil {
		t.Fatal(err)
	}

	firstTasks := completedGraphDocumentTasks(organizationID, datasetID, firstDocumentID, firstRunID)
	secondExtraction := graphmodel.GraphFlowTask{
		ID:         uuid.New(),
		TenantID:   organizationID,
		KBID:       datasetID,
		DocumentID: secondDocumentID,
		RunID:      &secondRunID,
		TaskType:   "extraction",
		Status:     "processing",
		Progress:   50,
	}
	if err := db.Create(&firstTasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&secondExtraction).Error; err != nil {
		t.Fatal(err)
	}

	service := NewLifecycleService(db)
	if err := service.ReconcileRun(ctx, firstRunID); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileTask(ctx, secondExtraction.ID); err != nil {
		t.Fatal(err)
	}

	var building lifecycleTestDataset
	if err := db.First(&building, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if building.GraphStatus != "building" || building.GraphProgress != 65 {
		t.Fatalf("building status=%s progress=%d", building.GraphStatus, building.GraphProgress)
	}
	var processingRun graphmodel.GraphFlowRun
	if err := db.First(&processingRun, "id = ?", secondRunID).Error; err != nil {
		t.Fatal(err)
	}
	if processingRun.Progress != 30 {
		t.Fatalf("processing run progress=%d", processingRun.Progress)
	}

	if err := db.Model(&graphmodel.GraphFlowTask{}).
		Where("id = ?", secondExtraction.ID).
		Updates(map[string]any{"status": "completed", "progress": 100}).Error; err != nil {
		t.Fatal(err)
	}
	remainingTasks := completedGraphDocumentTasks(organizationID, datasetID, secondDocumentID, secondRunID)[1:]
	for index := range remainingTasks {
		remainingTasks[index].RunID = nil
		remainingTasks[index].Metadata = map[string]interface{}{"graph_revision": int64(2)}
	}
	if err := db.Create(&remainingTasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileRun(ctx, secondRunID); err != nil {
		t.Fatal(err)
	}
	var repairedTaskCount int64
	if err := db.Model(&graphmodel.GraphFlowTask{}).
		Where("run_id = ?", secondRunID).Count(&repairedTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if repairedTaskCount != 4 {
		t.Fatalf("repaired task count=%d", repairedTaskCount)
	}

	var ready lifecycleTestDataset
	if err := db.First(&ready, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if ready.GraphStatus != "ready" || ready.GraphProgress != 100 {
		t.Fatalf("ready status=%s progress=%d", ready.GraphStatus, ready.GraphProgress)
	}
	if ready.GraphAvailableRevision == nil || *ready.GraphAvailableRevision != 2 {
		t.Fatalf("available revision=%v", ready.GraphAvailableRevision)
	}
	var readyRefs []lifecycleTestRef
	if err := db.Find(&readyRefs).Error; err != nil {
		t.Fatal(err)
	}
	for _, ref := range readyRefs {
		if ref.GraphSyncStatus == nil || *ref.GraphSyncStatus != "ready" {
			t.Fatalf("reference %s status=%v", ref.ID, ref.GraphSyncStatus)
		}
	}
}

func TestLifecycleServiceDistinguishesQueuedFromZeroProgressBuilding(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	documentID := uuid.New()
	runID := uuid.New()
	queuedStatus := "queued"

	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		GraphStatus:    "queued",
		GraphRevision:  1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lifecycleTestRef{
		ID:                uuid.New(),
		OrganizationID:    organizationID.String(),
		DatasetID:         datasetID.String(),
		AssetID:           uuid.New(),
		DatasetDocumentID: &documentID,
		Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
		RetrievalEnabled:  true,
		GraphRunID:        &runID,
		GraphSyncStatus:   &queuedStatus,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&graphmodel.GraphFlowRun{
		ID:             runID,
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		DocumentID:     &documentID,
		GraphRevision:  1,
		Trigger:        "test",
		Mode:           graphmodel.GraphFlowRunModeBuild,
		Status:         graphmodel.GraphFlowRunStatusPending,
		IdempotencyKey: "build:zero-progress",
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewLifecycleService(db)
	if err := service.ReconcileRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	var queued lifecycleTestDataset
	if err := db.First(&queued, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if queued.GraphStatus != "queued" || queued.GraphProgress != 0 {
		t.Fatalf("pending run status=%s progress=%d", queued.GraphStatus, queued.GraphProgress)
	}

	if err := db.Model(&graphmodel.GraphFlowRun{}).
		Where("id = ?", runID).
		Update("status", graphmodel.GraphFlowRunStatusProcessing).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&graphmodel.GraphFlowTask{
		ID:         uuid.New(),
		TenantID:   organizationID,
		KBID:       datasetID,
		DocumentID: documentID,
		RunID:      &runID,
		TaskType:   "extraction",
		Status:     "processing",
		Progress:   0,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileRun(ctx, runID); err != nil {
		t.Fatal(err)
	}

	var building lifecycleTestDataset
	if err := db.First(&building, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if building.GraphStatus != "building" || building.GraphProgress != 0 {
		t.Fatalf("processing run status=%s progress=%d", building.GraphStatus, building.GraphProgress)
	}
}

func TestLifecycleServiceMarksCurrentCleanupFailureAsDatasetFailure(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	documentID := uuid.New()
	cleanupRunID := uuid.New()
	currentRunID := cleanupRunID.String()
	readyStatus := "ready"

	if err := db.Create(&lifecycleTestDataset{
		ID:                datasetID.String(),
		OrganizationID:    organizationID.String(),
		GraphStatus:       "building",
		GraphRevision:     2,
		GraphCurrentRunID: &currentRunID,
		GraphProgress:     66,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lifecycleTestRef{
		ID:                uuid.New(),
		OrganizationID:    organizationID.String(),
		DatasetID:         datasetID.String(),
		AssetID:           uuid.New(),
		DatasetDocumentID: &documentID,
		Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
		RetrievalEnabled:  true,
		GraphSyncStatus:   &readyStatus,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&graphmodel.GraphFlowRun{
		ID:             cleanupRunID,
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		DocumentID:     &documentID,
		GraphRevision:  2,
		Trigger:        "document_replaced",
		Mode:           graphmodel.GraphFlowRunModeCleanup,
		Status:         graphmodel.GraphFlowRunStatusProcessing,
		IdempotencyKey: "cleanup:old-document",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := NewLifecycleService(db).FailRun(ctx, cleanupRunID, "graph_outbox_failed", "could not schedule cleanup"); err != nil {
		t.Fatal(err)
	}

	var run graphmodel.GraphFlowRun
	if err := db.First(&run, "id = ?", cleanupRunID).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != graphmodel.GraphFlowRunStatusFailed || run.ErrorCode == nil || *run.ErrorCode != "graph_outbox_failed" {
		t.Fatalf("cleanup run status=%s error_code=%v", run.Status, run.ErrorCode)
	}
	var dataset lifecycleTestDataset
	if err := db.First(&dataset, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if dataset.GraphStatus != "failed" {
		t.Fatalf("dataset graph status=%s, want failed", dataset.GraphStatus)
	}
}

func completedGraphDocumentTasks(
	organizationID uuid.UUID,
	datasetID uuid.UUID,
	documentID uuid.UUID,
	runID uuid.UUID,
) []graphmodel.GraphFlowTask {
	taskTypes := []string{"extraction", "alignment", "graph_sync", "vector_sync"}
	tasks := make([]graphmodel.GraphFlowTask, 0, len(taskTypes))
	for _, taskType := range taskTypes {
		tasks = append(tasks, graphmodel.GraphFlowTask{
			ID:         uuid.New(),
			TenantID:   organizationID,
			KBID:       datasetID,
			DocumentID: documentID,
			RunID:      &runID,
			TaskType:   taskType,
			Status:     "completed",
			Progress:   100,
		})
	}
	return tasks
}

func TestLifecycleServiceStartBuildIsIdempotent(t *testing.T) {
	db := openLifecycleTestDB(t)
	datasetID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		WorkspaceID:    workspaceID.String(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewLifecycleService(db)
	request := LifecycleRunRequest{
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "build:first",
	}
	first, created, err := service.StartBuild(context.Background(), request)
	if err != nil || !created {
		t.Fatalf("first build: created=%v err=%v", created, err)
	}
	second, created, err := service.StartBuild(context.Background(), request)
	if err != nil || created {
		t.Fatalf("second build: created=%v err=%v", created, err)
	}
	if first.ID != second.ID {
		t.Fatalf("run IDs differ: %s != %s", first.ID, second.ID)
	}

	var runCount int64
	var eventCount int64
	if err := db.Model(&graphmodel.GraphFlowRun{}).Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if runCount != 1 || eventCount != 1 {
		t.Fatalf("run count=%d event count=%d", runCount, eventCount)
	}
}

func TestLifecycleServiceCreatesDatasetPurgeWithoutRun(t *testing.T) {
	db := openLifecycleTestDB(t)
	datasetID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		WorkspaceID:    workspaceID.String(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewLifecycleService(db)
	startedAt := time.Now().UTC()

	err := db.Transaction(func(tx *gorm.DB) error {
		return service.StartDatasetPurgeInTx(
			context.Background(),
			tx,
			organizationID,
			&workspaceID,
			datasetID,
		)
	})
	if err != nil {
		t.Fatal(err)
	}

	var event graphmodel.GraphOutboxEvent
	if err := db.Where("event_type = ?", graphmodel.GraphOutboxEventDatasetPurge).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.RunID != nil || event.DatasetID != datasetID {
		t.Fatalf("unexpected dataset purge event: %#v", event)
	}
	if event.AvailableAt.Before(startedAt.Add(datasetPurgeInitialDelay)) {
		t.Fatalf("dataset purge scheduled too early: %s", event.AvailableAt)
	}
}

func TestLifecycleServiceRejectsInvalidScopeAndKey(t *testing.T) {
	db := openLifecycleTestDB(t)
	datasetID := uuid.New()
	organizationID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{ID: datasetID.String(), OrganizationID: organizationID.String()}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewLifecycleService(db)

	_, _, err := service.StartBuild(context.Background(), LifecycleRunRequest{DatasetID: datasetID})
	if !errors.Is(err, ErrGraphFlowIdempotencyKeyRequired) || err.Error() != "graph flow idempotency key is required" {
		t.Fatalf("unexpected key error: %v", err)
	}
	_, _, err = service.StartBuild(context.Background(), LifecycleRunRequest{
		OrganizationID: uuid.New(),
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "build:wrong-tenant",
	})
	if !errors.Is(err, ErrGraphFlowTenantScopeMismatch) || err.Error() != "graph flow tenant scope mismatch" {
		t.Fatalf("unexpected scope error: %v", err)
	}

	var runCount int64
	if err := db.Model(&graphmodel.GraphFlowRun{}).Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if runCount != 0 {
		t.Fatalf("unexpected cross-tenant runs: %d", runCount)
	}
}

func TestLifecycleServiceRetryRequeuesFailedTasks(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	documentID := uuid.New()
	runID := uuid.New()

	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		GraphStatus:    "failed",
		GraphRevision:  1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	run := graphmodel.GraphFlowRun{
		ID:             runID,
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		DocumentID:     &documentID,
		GraphRevision:  1,
		Trigger:        "test",
		Mode:           graphmodel.GraphFlowRunModeBuild,
		Status:         graphmodel.GraphFlowRunStatusFailed,
		IdempotencyKey: "retry:failed-vector-sync",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	task := lifecycleTestTask{
		ID:           uuid.New(),
		TenantID:     organizationID,
		KBID:         datasetID,
		DocumentID:   documentID,
		RunID:        &runID,
		TaskType:     "vector_sync",
		Status:       "failed",
		Progress:     80,
		ErrorMessage: "embedding generation failed",
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}

	if err := NewLifecycleService(db).Retry(ctx, runID); err != nil {
		t.Fatal(err)
	}

	var storedRun graphmodel.GraphFlowRun
	if err := db.First(&storedRun, "id = ?", runID).Error; err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != graphmodel.GraphFlowRunStatusPending {
		t.Fatalf("run status = %q, want pending", storedRun.Status)
	}
	var storedTask lifecycleTestTask
	if err := db.First(&storedTask, "id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedTask.Status != "pending" || storedTask.Progress != 0 || storedTask.ErrorMessage != "" {
		t.Fatalf("task was not reset: %#v", storedTask)
	}
	var event graphmodel.GraphOutboxEvent
	if err := db.Where("run_id = ? AND status = ?", runID, graphmodel.GraphOutboxStatusPending).First(&event).Error; err != nil {
		t.Fatalf("pending retry event not found: %v", err)
	}
}

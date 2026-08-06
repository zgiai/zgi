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
	SyncBatchID       *uuid.UUID
	SyncStatus        string
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
	RunItemID          *uuid.UUID
	SourceRefID        *uuid.UUID
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
		&graphmodel.GraphFlowRunItem{},
		&graphmodel.GraphOutboxEvent{},
		&lifecycleTestTask{},
	}
	baseModels = append(baseModels, models...)
	if err := db.AutoMigrate(baseModels...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLifecycleServiceRegistersReplacementAsOneBatchRun(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	batchID := uuid.New()
	refID := uuid.New()
	syncRunID := uuid.New()
	newDocumentID := uuid.New()
	oldDocumentID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		WorkspaceID:    workspaceID.String(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&lifecycleTestRef{
		ID:                refID,
		OrganizationID:    organizationID.String(),
		DatasetID:         datasetID.String(),
		AssetID:           uuid.New(),
		DatasetDocumentID: &newDocumentID,
		SyncRunID:         &syncRunID,
		SyncBatchID:       &batchID,
		SyncStatus:        datalibrarymodel.KnowledgeBaseAssetRefSyncStatusPending,
		Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewLifecycleService(db)
	run, err := service.RegisterSyncBatchDocument(ctx, SyncBatchDocumentRequest{
		OrganizationID:    organizationID,
		WorkspaceID:       &workspaceID,
		DatasetID:         datasetID,
		SyncBatchID:       batchID,
		SourceRefID:       refID,
		SyncRunID:         syncRunID,
		DocumentID:        newDocumentID,
		CleanupDocumentID: &oldDocumentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.SyncBatchID == nil || *run.SyncBatchID != batchID || run.DocumentID != nil || run.SyncRunID != nil {
		t.Fatalf("unexpected parent run: %#v", run)
	}
	var itemCount int64
	if err := db.Model(&graphmodel.GraphFlowRunItem{}).Where("run_id = ?", run.ID).Count(&itemCount).Error; err != nil {
		t.Fatal(err)
	}
	if itemCount != 2 {
		t.Fatalf("run item count=%d, want add+cleanup", itemCount)
	}
	var eventCount int64
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("batch published before refs were terminal: %d", eventCount)
	}
	if err := service.TryStartSyncBatch(ctx, organizationID, datasetID, batchID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("pending batch published: %d", eventCount)
	}
	if err := db.Model(&lifecycleTestRef{}).Where("id = ?", refID).Update("sync_status", datalibrarymodel.KnowledgeBaseAssetRefSyncStatusSynced).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.TryStartSyncBatch(ctx, organizationID, datasetID, batchID); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("terminal batch event count=%d, want 1", eventCount)
	}
	var dataset lifecycleTestDataset
	if err := db.First(&dataset, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if dataset.GraphRevision != 1 {
		t.Fatalf("replacement incremented graph revision %d times", dataset.GraphRevision)
	}
}

func TestSummarizeSyncBatchRunTasksRequiresOneGlobalPipeline(t *testing.T) {
	runID := uuid.New()
	firstItemID := uuid.New()
	secondItemID := uuid.New()
	firstDocumentID := uuid.New()
	secondDocumentID := uuid.New()
	items := []graphmodel.GraphFlowRunItem{
		{ID: firstItemID, RunID: runID, Operation: graphmodel.GraphFlowRunItemOperationAdd, DocumentID: firstDocumentID},
		{ID: secondItemID, RunID: runID, Operation: graphmodel.GraphFlowRunItemOperationAdd, DocumentID: secondDocumentID},
	}
	tasks := []graphmodel.GraphFlowTask{
		{RunID: &runID, RunItemID: &firstItemID, DocumentID: firstDocumentID, TaskType: "extraction", Status: "completed", Progress: 100},
		{RunID: &runID, RunItemID: &secondItemID, DocumentID: secondDocumentID, TaskType: "extraction", Status: "completed", Progress: 100},
		{RunID: &runID, DocumentID: firstDocumentID, TaskType: "alignment", Status: "completed", Progress: 100},
		{RunID: &runID, DocumentID: firstDocumentID, TaskType: "graph_sync", Status: "completed", Progress: 100},
		{RunID: &runID, DocumentID: firstDocumentID, TaskType: "vector_sync", Status: "completed", Progress: 100},
	}
	summary := summarizeSyncBatchRunTasks(items, tasks)
	if !summary.Complete || summary.Failed || summary.Progress != 100 || len(summary.DocumentIDs) != 2 {
		t.Fatalf("unexpected batch summary: %#v", summary)
	}
}

func TestFullDatasetRebuildUsesBatchRunSummary(t *testing.T) {
	fullRebuild := &graphmodel.GraphFlowRun{Mode: graphmodel.GraphFlowRunModeRebuild}
	if !graphRunUsesBatchPipeline(fullRebuild) {
		t.Fatal("full dataset rebuild must use batch task reconciliation")
	}
	documentID := uuid.New()
	if graphRunUsesBatchPipeline(&graphmodel.GraphFlowRun{Mode: graphmodel.GraphFlowRunModeRebuild, DocumentID: &documentID}) {
		t.Fatal("document-scoped rebuild must use document task reconciliation")
	}
}

func TestLifecycleServiceReconcilesFullDatasetRebuildAsOneGlobalPipeline(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	organizationID := uuid.New()
	datasetID := uuid.New()
	runID := uuid.New()
	runIDString := runID.String()
	documentIDs := []uuid.UUID{uuid.New(), uuid.New()}
	if err := db.Create(&lifecycleTestDataset{
		ID:                datasetID.String(),
		OrganizationID:    organizationID.String(),
		GraphStatus:       "building",
		GraphRevision:     1,
		GraphCurrentRunID: &runIDString,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&graphmodel.GraphFlowRun{
		ID:             runID,
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		GraphRevision:  1,
		Trigger:        "manual_rebuild",
		Mode:           graphmodel.GraphFlowRunModeRebuild,
		Status:         graphmodel.GraphFlowRunStatusProcessing,
		IdempotencyKey: "rebuild:batch-summary",
	}).Error; err != nil {
		t.Fatal(err)
	}

	items := make([]graphmodel.GraphFlowRunItem, 0, len(documentIDs))
	refs := make([]lifecycleTestRef, 0, len(documentIDs))
	tasks := make([]graphmodel.GraphFlowTask, 0, len(documentIDs)+3)
	for _, documentID := range documentIDs {
		itemID := uuid.New()
		refID := uuid.New()
		items = append(items, graphmodel.GraphFlowRunItem{
			ID:             itemID,
			RunID:          runID,
			OrganizationID: organizationID,
			DatasetID:      datasetID,
			SourceRefID:    &refID,
			SyncBatchID:    runID,
			Operation:      graphmodel.GraphFlowRunItemOperationAdd,
			DocumentID:     documentID,
		})
		status := "processing"
		refs = append(refs, lifecycleTestRef{
			ID:                refID,
			OrganizationID:    organizationID.String(),
			DatasetID:         datasetID.String(),
			AssetID:           uuid.New(),
			DatasetDocumentID: &documentID,
			Status:            datalibrarymodel.KnowledgeBaseAssetRefStatusActive,
			RetrievalEnabled:  true,
			GraphRunID:        &runID,
			GraphSyncStatus:   &status,
		})
		tasks = append(tasks, graphmodel.GraphFlowTask{
			ID:         uuid.New(),
			TenantID:   organizationID,
			KBID:       datasetID,
			DocumentID: documentID,
			RunID:      &runID,
			RunItemID:  &itemID,
			TaskType:   "extraction",
			Status:     "completed",
			Progress:   100,
		})
	}
	coordinatorDocumentID := documentIDs[0]
	for _, taskType := range []string{"alignment", "graph_sync", "vector_sync"} {
		tasks = append(tasks, graphmodel.GraphFlowTask{
			ID:         uuid.New(),
			TenantID:   organizationID,
			KBID:       datasetID,
			DocumentID: coordinatorDocumentID,
			RunID:      &runID,
			TaskType:   taskType,
			Status:     "completed",
			Progress:   100,
		})
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&refs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatal(err)
	}

	if err := NewLifecycleService(db).ReconcileRun(ctx, runID); err != nil {
		t.Fatal(err)
	}
	var storedRun graphmodel.GraphFlowRun
	if err := db.First(&storedRun, "id = ?", runID).Error; err != nil {
		t.Fatal(err)
	}
	if storedRun.Status != graphmodel.GraphFlowRunStatusReady || storedRun.Progress != 100 {
		t.Fatalf("rebuild run status=%s progress=%d", storedRun.Status, storedRun.Progress)
	}
	var storedDataset lifecycleTestDataset
	if err := db.First(&storedDataset, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if storedDataset.GraphAvailableRevision == nil || *storedDataset.GraphAvailableRevision != 1 {
		t.Fatalf("available revision=%v, want 1", storedDataset.GraphAvailableRevision)
	}
}

func TestSummarizeGraphStageStatusesShowsCurrentBottleneck(t *testing.T) {
	runID := uuid.New()
	datasetID := uuid.New()
	organizationID := uuid.New()
	tasks := []graphmodel.GraphFlowTask{
		{RunID: &runID, TenantID: organizationID, KBID: datasetID, DocumentID: uuid.New(), TaskType: "extraction", Status: "completed", Progress: 100},
		{RunID: &runID, TenantID: organizationID, KBID: datasetID, DocumentID: uuid.New(), TaskType: "extraction", Status: "completed", Progress: 100},
		{RunID: &runID, TenantID: organizationID, KBID: datasetID, DocumentID: uuid.New(), TaskType: "alignment", Status: "processing", Progress: 75},
		{RunID: &runID, TenantID: organizationID, KBID: datasetID, DocumentID: uuid.New(), TaskType: "graph_sync", Status: "pending", Progress: 0},
		{RunID: &runID, TenantID: organizationID, KBID: datasetID, DocumentID: uuid.New(), TaskType: "vector_sync", Status: "pending", Progress: 0},
	}

	stages, current := summarizeGraphStageStatuses(tasks)
	if current != "alignment" {
		t.Fatalf("current stage=%q, want alignment", current)
	}
	if len(stages) != 4 {
		t.Fatalf("stage count=%d, want 4", len(stages))
	}
	if stages[0].Status != "completed" || stages[0].Progress != 100 {
		t.Fatalf("unexpected extraction stage: %#v", stages[0])
	}
	if stages[1].Status != "processing" || stages[1].Progress != 75 {
		t.Fatalf("unexpected alignment stage: %#v", stages[1])
	}
	if stages[2].Status != "pending" || stages[3].Status != "pending" {
		t.Fatalf("unexpected pending stages: %#v", stages[2:])
	}
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

func TestLifecycleServiceSerializesDatasetRunsAndPromotesInRevisionOrder(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewLifecycleService(db)
	first, _, err := service.StartBuild(ctx, LifecycleRunRequest{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "serial:first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := service.StartBuild(ctx, LifecycleRunRequest{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "serial:second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != graphmodel.GraphFlowRunStatusProcessing {
		t.Fatalf("first run status=%s, want processing", first.Status)
	}
	if second.Status != graphmodel.GraphFlowRunStatusPending {
		t.Fatalf("second run status=%s, want pending", second.Status)
	}
	var eventCount int64
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("outbox events=%d before handoff, want 1", eventCount)
	}

	if err := service.PublishRun(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(first, "id = ?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(second, "id = ?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.Status != graphmodel.GraphFlowRunStatusReady || second.Status != graphmodel.GraphFlowRunStatusProcessing {
		t.Fatalf("handoff statuses first=%s second=%s", first.Status, second.Status)
	}
	if err := db.Model(&graphmodel.GraphOutboxEvent{}).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("outbox events=%d after handoff, want 2", eventCount)
	}
	var dataset lifecycleTestDataset
	if err := db.First(&dataset, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if dataset.GraphAvailableRevision == nil || *dataset.GraphAvailableRevision != first.GraphRevision {
		t.Fatalf("available revision=%v, want %d", dataset.GraphAvailableRevision, first.GraphRevision)
	}
	if dataset.GraphCurrentRunID == nil || *dataset.GraphCurrentRunID != second.ID.String() {
		t.Fatalf("current run=%v, want %s", dataset.GraphCurrentRunID, second.ID)
	}
}

func TestLifecycleServiceFailedRunBlocksLaterRevisionUntilRetry(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	service := NewLifecycleService(db)
	first, _, err := service.StartBuild(ctx, LifecycleRunRequest{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "blocking:first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := service.StartBuild(ctx, LifecycleRunRequest{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "blocking:second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.FailRun(ctx, first.ID, "test_failure", "first revision failed"); err != nil {
		t.Fatal(err)
	}
	if err := service.ReconcileActiveRuns(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.First(second, "id = ?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if second.Status != graphmodel.GraphFlowRunStatusPending {
		t.Fatalf("later run status=%s, want pending while predecessor failed", second.Status)
	}

	if err := service.Retry(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(first, "id = ?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(second, "id = ?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.Status != graphmodel.GraphFlowRunStatusProcessing || second.Status != graphmodel.GraphFlowRunStatusPending {
		t.Fatalf("retry statuses first=%s second=%s", first.Status, second.Status)
	}
}

func TestLifecycleServiceRebuildCanRecoverPastFailedRevision(t *testing.T) {
	db := openLifecycleTestDB(t, &lifecycleTestRef{})
	ctx := context.Background()
	datasetID := uuid.New()
	organizationID := uuid.New()
	if err := db.Create(&lifecycleTestDataset{
		ID:             datasetID.String(),
		OrganizationID: organizationID.String(),
		GraphStatus:    "failed",
		GraphRevision:  1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	failedRun := &graphmodel.GraphFlowRun{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		GraphRevision:  1,
		Trigger:        "test",
		Mode:           graphmodel.GraphFlowRunModeBuild,
		Status:         graphmodel.GraphFlowRunStatusFailed,
		IdempotencyKey: "rebuild-recovery:failed",
	}
	if err := db.Create(failedRun).Error; err != nil {
		t.Fatal(err)
	}

	rebuild, created, err := NewLifecycleService(db).StartRebuild(ctx, LifecycleRunRequest{
		OrganizationID: organizationID,
		DatasetID:      datasetID,
		Trigger:        "test",
		IdempotencyKey: "rebuild-recovery:rebuild",
	})
	if err != nil || !created {
		t.Fatalf("start rebuild: created=%v err=%v", created, err)
	}
	if rebuild.Status != graphmodel.GraphFlowRunStatusProcessing {
		t.Fatalf("rebuild status=%s, want processing", rebuild.Status)
	}

	var event graphmodel.GraphOutboxEvent
	if err := db.Where("run_id = ?", rebuild.ID).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if event.EventType != graphmodel.GraphOutboxEventRun {
		t.Fatalf("outbox event type=%s, want %s", event.EventType, graphmodel.GraphOutboxEventRun)
	}
	var dataset lifecycleTestDataset
	if err := db.First(&dataset, "id = ?", datasetID).Error; err != nil {
		t.Fatal(err)
	}
	if dataset.GraphCurrentRunID == nil || *dataset.GraphCurrentRunID != rebuild.ID.String() {
		t.Fatalf("current run=%v, want %s", dataset.GraphCurrentRunID, rebuild.ID)
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
	if storedRun.Status != graphmodel.GraphFlowRunStatusProcessing {
		t.Fatalf("run status = %q, want processing", storedRun.Status)
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

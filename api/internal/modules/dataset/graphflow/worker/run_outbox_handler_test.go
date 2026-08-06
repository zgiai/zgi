package worker

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type runOutboxHandlerTestDocument struct {
	ID             string `gorm:"primaryKey"`
	OrganizationID string
	DatasetID      string
}

type runOutboxHandlerTestTask struct {
	ID                 uuid.UUID `gorm:"primaryKey"`
	TenantID           uuid.UUID
	KBID               uuid.UUID `gorm:"column:kb_id"`
	DocumentID         uuid.UUID
	SegmentID          *uuid.UUID
	RunID              *uuid.UUID `gorm:"uniqueIndex:idx_test_graphflow_run_type"`
	RunItemID          *uuid.UUID
	SourceRefID        *uuid.UUID
	TaskType           string `gorm:"uniqueIndex:idx_test_graphflow_run_type"`
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

type runOutboxHandlerTestRef struct {
	ID                 uuid.UUID `gorm:"primaryKey"`
	OrganizationID     string
	WorkspaceID        *string
	DatasetID          string
	DatasetDocumentID  *uuid.UUID
	SyncRunID          *uuid.UUID
	SyncedGenerationNo *int64
	GraphRunID         *uuid.UUID
	GraphSyncStatus    *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt
}

func (runOutboxHandlerTestDocument) TableName() string {
	return "documents"
}

func (runOutboxHandlerTestTask) TableName() string {
	return "graphflow_tasks"
}

func (runOutboxHandlerTestRef) TableName() string {
	return "data_library_knowledge_base_asset_refs"
}

func TestRunOutboxHandlerAllowsCleanupForDeletedDocument(t *testing.T) {
	db := openRunOutboxHandlerTestDB(t)
	run := runOutboxHandlerTestRun(graphmodel.GraphFlowRunModeCleanup)
	handler := &RunOutboxHandler{service: &graphflow.Service{DB: db}}

	task, err := handler.createDocumentTask(context.Background(), run, *run.DocumentID, "cleanup")
	if err != nil {
		t.Fatalf("create cleanup task: %v", err)
	}
	if task.DocumentID != *run.DocumentID || task.TaskType != "cleanup" {
		t.Fatalf("unexpected cleanup task: %#v", task)
	}
}

func TestRunOutboxHandlerRejectsBuildForDeletedDocument(t *testing.T) {
	db := openRunOutboxHandlerTestDB(t)
	run := runOutboxHandlerTestRun(graphmodel.GraphFlowRunModeBuild)
	handler := &RunOutboxHandler{service: &graphflow.Service{DB: db}}

	_, err := handler.createDocumentTask(context.Background(), run, *run.DocumentID, "extraction")
	if !errors.Is(err, graphflow.ErrStaleDocumentSnapshot) {
		t.Fatalf("build error = %v, want stale document snapshot", err)
	}

	document := &runOutboxHandlerTestDocument{
		ID:             run.DocumentID.String(),
		OrganizationID: run.OrganizationID.String(),
		DatasetID:      run.DatasetID.String(),
	}
	if err := db.Create(document).Error; err != nil {
		t.Fatal(err)
	}
	task, err := handler.createDocumentTask(context.Background(), run, *run.DocumentID, "extraction")
	if err != nil {
		t.Fatalf("create current document build task: %v", err)
	}
	if task.ExtractionStrategy != "llm" {
		t.Fatalf("extraction strategy=%q, want llm", task.ExtractionStrategy)
	}
}

func TestNextPendingDocumentTaskKeepsVectorSyncBehindGraphSync(t *testing.T) {
	graphTask := graphmodel.GraphFlowTask{ID: uuid.New(), TaskType: "graph_sync", Status: "pending"}
	vectorTask := graphmodel.GraphFlowTask{ID: uuid.New(), TaskType: "vector_sync", Status: "pending"}

	task, queueType, ok := nextPendingDocumentTask([]graphmodel.GraphFlowTask{vectorTask, graphTask})
	if !ok || task.ID != graphTask.ID || queueType != TypeGraphFlowSync {
		t.Fatalf("next task = (%s, %s, %t), want pending graph sync", task.ID, queueType, ok)
	}

	graphTask.Status = "completed"
	task, queueType, ok = nextPendingDocumentTask([]graphmodel.GraphFlowTask{vectorTask, graphTask})
	if !ok || task.ID != vectorTask.ID || queueType != TypeGraphFlowVectorSync {
		t.Fatalf("next task = (%s, %s, %t), want pending vector sync", task.ID, queueType, ok)
	}
}

func TestFullDatasetRebuildSnapshotsDocumentsForOneBatchPipeline(t *testing.T) {
	db := openRunOutboxHandlerTestDB(t)
	ctx := context.Background()
	organizationID := uuid.New()
	datasetID := uuid.New()
	workspaceID := uuid.New()
	run := &graphmodel.GraphFlowRun{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		DatasetID:      datasetID,
		Mode:           graphmodel.GraphFlowRunModeRebuild,
		Status:         graphmodel.GraphFlowRunStatusProcessing,
	}
	documentIDs := []uuid.UUID{uuid.New(), uuid.New()}
	for _, documentID := range documentIDs {
		workspace := workspaceID.String()
		syncRunID := uuid.New()
		if err := db.Create(&runOutboxHandlerTestRef{
			ID:                uuid.New(),
			OrganizationID:    organizationID.String(),
			WorkspaceID:       &workspace,
			DatasetID:         datasetID.String(),
			DatasetDocumentID: &documentID,
			SyncRunID:         &syncRunID,
			CreatedAt:         time.Now(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	handler := &RunOutboxHandler{service: &graphflow.Service{DB: db}}
	if err := handler.snapshotFullRebuildItems(ctx, run); err != nil {
		t.Fatal(err)
	}
	// A document added after dispatch belongs to a later revision and must not
	// be pulled into this run when its outbox event is retried.
	lateDocumentID := uuid.New()
	workspace := workspaceID.String()
	if err := db.Create(&runOutboxHandlerTestRef{
		ID:                uuid.New(),
		OrganizationID:    organizationID.String(),
		WorkspaceID:       &workspace,
		DatasetID:         datasetID.String(),
		DatasetDocumentID: &lateDocumentID,
		CreatedAt:         time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	// Retrying the outbox snapshot must neither duplicate items nor expand the
	// fixed rebuild snapshot.
	if err := handler.snapshotFullRebuildItems(ctx, run); err != nil {
		t.Fatal(err)
	}

	var items []graphmodel.GraphFlowRunItem
	if err := db.Order("created_at ASC").Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != len(documentIDs) {
		t.Fatalf("run items=%d, want %d", len(items), len(documentIDs))
	}
	seen := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if item.RunID != run.ID || item.SyncBatchID != run.ID || item.Operation != graphmodel.GraphFlowRunItemOperationAdd {
			t.Fatalf("unexpected rebuild item: %#v", item)
		}
		seen[item.DocumentID] = true
	}
	for _, documentID := range documentIDs {
		if !seen[documentID] {
			t.Fatalf("document %s missing from rebuild snapshot", documentID)
		}
	}

	var refs []runOutboxHandlerTestRef
	if err := db.Find(&refs).Error; err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if ref.DatasetDocumentID != nil && *ref.DatasetDocumentID == lateDocumentID {
			if ref.GraphRunID != nil {
				t.Fatalf("late document attached to earlier rebuild run: %#v", ref)
			}
			continue
		}
		if ref.GraphRunID == nil || *ref.GraphRunID != run.ID || ref.GraphSyncStatus == nil || *ref.GraphSyncStatus != "queued" {
			t.Fatalf("ref not attached to rebuild run: %#v", ref)
		}
	}
}

func TestOnlyFullDatasetRebuildUsesRebuildBatchPipeline(t *testing.T) {
	fullRebuild := &graphmodel.GraphFlowRun{Mode: graphmodel.GraphFlowRunModeRebuild}
	if !usesBatchPipeline(fullRebuild) {
		t.Fatal("full dataset rebuild must use the batch pipeline")
	}
	documentID := uuid.New()
	if usesBatchPipeline(&graphmodel.GraphFlowRun{Mode: graphmodel.GraphFlowRunModeRebuild, DocumentID: &documentID}) {
		t.Fatal("single-document rebuild must keep the document pipeline")
	}
	if usesBatchPipeline(&graphmodel.GraphFlowRun{Mode: graphmodel.GraphFlowRunModeBuild}) {
		t.Fatal("ordinary non-batch build must keep the document pipeline")
	}
}

func openRunOutboxHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:run-outbox-handler-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&runOutboxHandlerTestDocument{},
		&runOutboxHandlerTestTask{},
		&runOutboxHandlerTestRef{},
		&graphmodel.GraphFlowRunItem{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func runOutboxHandlerTestRun(mode string) *graphmodel.GraphFlowRun {
	documentID := uuid.New()
	return &graphmodel.GraphFlowRun{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		DatasetID:      uuid.New(),
		DocumentID:     &documentID,
		GraphRevision:  1,
		Trigger:        "test",
		Mode:           mode,
		Status:         graphmodel.GraphFlowRunStatusProcessing,
		IdempotencyKey: "test:" + mode,
	}
}

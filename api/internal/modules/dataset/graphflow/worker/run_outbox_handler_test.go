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
	TaskType           string     `gorm:"uniqueIndex:idx_test_graphflow_run_type"`
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

func (runOutboxHandlerTestDocument) TableName() string {
	return "documents"
}

func (runOutboxHandlerTestTask) TableName() string {
	return "graphflow_tasks"
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
	if _, err := handler.createDocumentTask(context.Background(), run, *run.DocumentID, "extraction"); err != nil {
		t.Fatalf("create current document build task: %v", err)
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

func openRunOutboxHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:run-outbox-handler-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runOutboxHandlerTestDocument{}, &runOutboxHandlerTestTask{}); err != nil {
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

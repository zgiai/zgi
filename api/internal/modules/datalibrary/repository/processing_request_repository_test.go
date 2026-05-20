package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProcessingRequestRepositoryCreateReadAndList(t *testing.T) {
	db := openProcessingRequestRepoTestDB(t)
	repo := NewProcessingRequestRepository(db)
	ctx := context.Background()
	assetID := uuid.New()

	request := &model.ProcessingRequest{
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelSplit,
		RequestedBy:    "account-1",
		PlanJSON: map[string]any{
			"will_parse": true,
			"will_split": true,
		},
	}
	if err := repo.Create(ctx, request); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if request.ID == uuid.Nil || request.Status != model.ProcessingRequestStatusPlanned {
		t.Fatalf("request=%+v", request)
	}

	got, err := repo.GetByID(ctx, request.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil || got.TargetLevel != model.DocumentProcessingLevelSplit || got.RequestedBy != "account-1" {
		t.Fatalf("got=%+v", got)
	}

	items, total, err := repo.List(ctx, ProcessingRequestListFilter{
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelSplit,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != request.ID {
		t.Fatalf("items=%+v total=%d", items, total)
	}

	queuedAt := time.Now()
	queued, err := repo.TransitionStatus(ctx, request.ID, ProcessingRequestStatusPatch{
		OrganizationID: "org-1",
		Status:         model.ProcessingRequestStatusQueued,
		AllowedFrom:    []string{model.ProcessingRequestStatusPlanned},
		QueuedAt:       &queuedAt,
	})
	if err != nil {
		t.Fatalf("TransitionStatus queued: %v", err)
	}
	if queued == nil || queued.Status != model.ProcessingRequestStatusQueued || queued.QueuedAt == nil {
		t.Fatalf("queued=%+v", queued)
	}

	executorKey := "metadata-only-test"
	startedAt := time.Now()
	running, err := repo.TransitionStatus(ctx, request.ID, ProcessingRequestStatusPatch{
		OrganizationID:    "org-1",
		Status:            model.ProcessingRequestStatusRunning,
		AllowedFrom:       []string{model.ProcessingRequestStatusQueued},
		ExecutorKey:       &executorKey,
		AttemptCountDelta: 1,
		StartedAt:         &startedAt,
	})
	if err != nil {
		t.Fatalf("TransitionStatus running: %v", err)
	}
	if running == nil ||
		running.Status != model.ProcessingRequestStatusRunning ||
		running.ExecutorKey != executorKey ||
		running.AttemptCount != 1 ||
		running.StartedAt == nil {
		t.Fatalf("running=%+v", running)
	}

	stale, err := repo.TransitionStatus(ctx, request.ID, ProcessingRequestStatusPatch{
		OrganizationID: "org-1",
		Status:         model.ProcessingRequestStatusCompleted,
		AllowedFrom:    []string{model.ProcessingRequestStatusQueued},
	})
	if err != nil {
		t.Fatalf("TransitionStatus stale: %v", err)
	}
	if stale == nil || stale.Status != model.ProcessingRequestStatusRunning {
		t.Fatalf("stale=%+v", stale)
	}

	completedAt := time.Now()
	completed, err := repo.TransitionStatus(ctx, request.ID, ProcessingRequestStatusPatch{
		OrganizationID: "org-1",
		Status:         model.ProcessingRequestStatusCompleted,
		AllowedFrom:    []string{model.ProcessingRequestStatusRunning},
		CompletedAt:    &completedAt,
		ExecutionMetadata: map[string]any{
			"chunk_artifact_set_id": "chunk-set-1",
		},
	})
	if err != nil {
		t.Fatalf("TransitionStatus completed: %v", err)
	}
	if completed == nil ||
		completed.Status != model.ProcessingRequestStatusCompleted ||
		completed.CompletedAt == nil ||
		completed.ExecutionMetadata["chunk_artifact_set_id"] != "chunk-set-1" {
		t.Fatalf("completed=%+v", completed)
	}
}

func TestProcessingRequestRepositoryClaimNextQueued(t *testing.T) {
	db := openProcessingRequestRepoTestDB(t)
	repo := NewProcessingRequestRepository(db)
	ctx := context.Background()

	older := &model.ProcessingRequest{
		OrganizationID: "org-1",
		AssetID:        uuid.New(),
		TargetLevel:    model.DocumentProcessingLevelParse,
		Status:         model.ProcessingRequestStatusQueued,
		PlanJSON: map[string]any{
			"will_parse": true,
		},
		CreatedAt: time.Now().Add(-time.Minute),
		UpdatedAt: time.Now().Add(-time.Minute),
	}
	newer := &model.ProcessingRequest{
		OrganizationID: "org-1",
		AssetID:        uuid.New(),
		TargetLevel:    model.DocumentProcessingLevelSplit,
		Status:         model.ProcessingRequestStatusQueued,
		PlanJSON: map[string]any{
			"will_parse": true,
			"will_split": true,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("Create newer: %v", err)
	}
	if err := repo.Create(ctx, older); err != nil {
		t.Fatalf("Create older: %v", err)
	}

	claimed, err := repo.ClaimNextQueued(ctx, ProcessingRequestClaimFilter{
		OrganizationID: "org-1",
		ExecutorKey:    "parse-worker",
	})
	if err != nil {
		t.Fatalf("ClaimNextQueued: %v", err)
	}
	if claimed == nil ||
		claimed.ID != older.ID ||
		claimed.Status != model.ProcessingRequestStatusRunning ||
		claimed.ExecutorKey != "parse-worker" ||
		claimed.AttemptCount != 1 ||
		claimed.StartedAt == nil {
		t.Fatalf("claimed=%+v", claimed)
	}

	next, err := repo.ClaimNextQueued(ctx, ProcessingRequestClaimFilter{
		OrganizationID: "org-1",
		ExecutorKey:    "parse-worker",
	})
	if err != nil {
		t.Fatalf("ClaimNextQueued next: %v", err)
	}
	if next == nil || next.ID != newer.ID {
		t.Fatalf("next=%+v", next)
	}
}

func TestProcessingRequestRepositoryClaimNextQueuedFiltersTargetLevels(t *testing.T) {
	db := openProcessingRequestRepoTestDB(t)
	repo := NewProcessingRequestRepository(db)
	ctx := context.Background()

	older := &model.ProcessingRequest{
		OrganizationID: "org-1",
		AssetID:        uuid.New(),
		TargetLevel:    model.DocumentProcessingLevelParse,
		Status:         model.ProcessingRequestStatusQueued,
		CreatedAt:      time.Now().Add(-time.Minute),
		UpdatedAt:      time.Now().Add(-time.Minute),
	}
	newer := &model.ProcessingRequest{
		OrganizationID: "org-1",
		AssetID:        uuid.New(),
		TargetLevel:    model.DocumentProcessingLevelSplit,
		Status:         model.ProcessingRequestStatusQueued,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := repo.Create(ctx, older); err != nil {
		t.Fatalf("Create older: %v", err)
	}
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("Create newer: %v", err)
	}

	claimed, err := repo.ClaimNextQueued(ctx, ProcessingRequestClaimFilter{
		OrganizationID: "org-1",
		ExecutorKey:    "split-worker",
		TargetLevels:   []string{model.DocumentProcessingLevelSplit},
	})
	if err != nil {
		t.Fatalf("ClaimNextQueued: %v", err)
	}
	if claimed == nil ||
		claimed.ID != newer.ID ||
		claimed.TargetLevel != model.DocumentProcessingLevelSplit ||
		claimed.ExecutorKey != "split-worker" {
		t.Fatalf("claimed=%+v", claimed)
	}

	var olderAfter model.ProcessingRequest
	if err := db.Where("id = ?", older.ID).First(&olderAfter).Error; err != nil {
		t.Fatalf("reload older: %v", err)
	}
	if olderAfter.Status != model.ProcessingRequestStatusQueued || olderAfter.ExecutorKey != "" {
		t.Fatalf("older mutated=%+v", olderAfter)
	}
}

func TestProcessingRequestRepositoryStatusSummaryByAssetID(t *testing.T) {
	db := openProcessingRequestRepoTestDB(t)
	repo := NewProcessingRequestRepository(db)
	ctx := context.Background()
	assetID := uuid.New()
	otherAssetID := uuid.New()

	requests := []*model.ProcessingRequest{
		{
			OrganizationID: "org-1",
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelParse,
			Status:         model.ProcessingRequestStatusQueued,
		},
		{
			OrganizationID: "org-1",
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusQueued,
		},
		{
			OrganizationID: "org-1",
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusFailed,
		},
		{
			OrganizationID: "org-1",
			AssetID:        otherAssetID,
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusRunning,
		},
	}
	for _, request := range requests {
		if err := repo.Create(ctx, request); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	summaries, err := repo.StatusSummaryByAssetID(ctx, "org-1", assetID)
	if err != nil {
		t.Fatalf("StatusSummaryByAssetID: %v", err)
	}
	got := map[string]int64{}
	for _, summary := range summaries {
		got[summary.Status] = summary.Count
	}
	if got[model.ProcessingRequestStatusQueued] != 2 ||
		got[model.ProcessingRequestStatusFailed] != 1 ||
		got[model.ProcessingRequestStatusRunning] != 0 {
		t.Fatalf("summaries=%+v", summaries)
	}
}

func TestProcessingRequestRepositoryQueueSummary(t *testing.T) {
	db := openProcessingRequestRepoTestDB(t)
	repo := NewProcessingRequestRepository(db)
	ctx := context.Background()
	queuedAt := time.Now().Add(-2 * time.Minute)
	createdAt := time.Now().Add(-3 * time.Minute)

	requests := []*model.ProcessingRequest{
		{
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelParse,
			Status:         model.ProcessingRequestStatusQueued,
			ExecutorKey:    "parse-worker",
			QueuedAt:       &queuedAt,
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
		},
		{
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelParse,
			Status:         model.ProcessingRequestStatusQueued,
			ExecutorKey:    "parse-worker",
		},
		{
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusFailed,
			ExecutorKey:    "split-worker",
		},
		{
			OrganizationID: "org-2",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelParse,
			Status:         model.ProcessingRequestStatusQueued,
			ExecutorKey:    "parse-worker",
		},
	}
	for _, request := range requests {
		if err := repo.Create(ctx, request); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	summaries, err := repo.QueueSummary(ctx, ProcessingRequestQueueSummaryFilter{
		OrganizationID: "org-1",
		TargetLevel:    model.DocumentProcessingLevelParse,
		Status:         model.ProcessingRequestStatusQueued,
		ExecutorKey:    "parse-worker",
	})
	if err != nil {
		t.Fatalf("QueueSummary: %v", err)
	}
	if len(summaries) != 1 ||
		summaries[0].TargetLevel != model.DocumentProcessingLevelParse ||
		summaries[0].Status != model.ProcessingRequestStatusQueued ||
		summaries[0].ExecutorKey != "parse-worker" ||
		summaries[0].Count != 2 ||
		summaries[0].OldestQueuedAt == nil ||
		summaries[0].OldestCreatedAt == nil ||
		summaries[0].NewestCreatedAt == nil {
		t.Fatalf("summaries=%+v", summaries)
	}
}

func TestProcessingRequestRepositoryClaimNextQueuedRequiresClaimContext(t *testing.T) {
	db := openProcessingRequestRepoTestDB(t)
	repo := NewProcessingRequestRepository(db)

	claimed, err := repo.ClaimNextQueued(context.Background(), ProcessingRequestClaimFilter{
		OrganizationID: "org-1",
	})
	if err != nil {
		t.Fatalf("ClaimNextQueued: %v", err)
	}
	if claimed != nil {
		t.Fatalf("claimed=%+v", claimed)
	}
}

func openProcessingRequestRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	schema := []string{
		`CREATE TABLE data_library_processing_requests (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			asset_id text NOT NULL,
			target_level text NOT NULL,
			status text NOT NULL DEFAULT 'planned',
			requested_by text,
			force boolean NOT NULL DEFAULT false,
			plan_json text NOT NULL DEFAULT '{}',
			request_metadata text NOT NULL DEFAULT '{}',
			execution_metadata text NOT NULL DEFAULT '{}',
			executor_key text,
			error_code text,
			error_message text,
			attempt_count integer NOT NULL DEFAULT 0,
			queued_at datetime,
			started_at datetime,
			completed_at datetime,
			failed_at datetime,
			cancelled_at datetime,
			created_at datetime NOT NULL,
			updated_at datetime NOT NULL,
			deleted_at datetime
		)`,
		`CREATE INDEX idx_data_library_processing_requests_asset_created
			ON data_library_processing_requests (asset_id, created_at)`,
	}
	for _, stmt := range schema {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
	}
	return db
}

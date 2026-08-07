package graphflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGraphFlowIdempotencyKeyRequired = errors.New("graph flow idempotency key is required")
	ErrGraphFlowTenantScopeMismatch    = errors.New("graph flow tenant scope mismatch")
	ErrGraphFlowDisabled               = errors.New("knowledge graph is not enabled")
)

const datasetPurgeInitialDelay = time.Minute

type GraphEmbeddingStatus struct {
	Mode          string `json:"mode"`
	ModelProvider string `json:"model_provider"`
	Model         string `json:"model"`
	Dimension     int    `json:"dimension"`
	Verified      bool   `json:"verified"`
}

type GraphRunStatus struct {
	ID        uuid.UUID `json:"id"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GraphDocumentStatus struct {
	DocumentID       uuid.UUID  `json:"document_id"`
	RefID            uuid.UUID  `json:"ref_id"`
	Status           string     `json:"status"`
	RetrievalEnabled bool       `json:"retrieval_enabled"`
	CurrentRunID     *uuid.UUID `json:"current_run_id,omitempty"`
	ErrorCode        *string    `json:"error_code,omitempty"`
}

type GraphStatusSummary struct {
	DocumentsTotal      int `json:"documents_total"`
	DocumentsReady      int `json:"documents_ready"`
	DocumentsProcessing int `json:"documents_processing"`
	DocumentsFailed     int `json:"documents_failed"`
}

type GraphStageStatus struct {
	Key      string `json:"key"`
	Status   string `json:"status"`
	Progress int    `json:"progress"`
}

type DatasetGraphStatus struct {
	DatasetID                        uuid.UUID             `json:"dataset_id"`
	Enabled                          bool                  `json:"enabled"`
	Status                           string                `json:"status"`
	Progress                         int                   `json:"progress"`
	GraphRevision                    int64                 `json:"graph_revision"`
	GraphVisibilityRevision          int64                 `json:"graph_visibility_revision"`
	GraphProjectedVisibilityRevision int64                 `json:"graph_projected_visibility_revision"`
	AvailableRevision                *int64                `json:"available_revision,omitempty"`
	CurrentRun                       *GraphRunStatus       `json:"current_run,omitempty"`
	CurrentStage                     string                `json:"current_stage,omitempty"`
	Stages                           []GraphStageStatus    `json:"stages"`
	Summary                          GraphStatusSummary    `json:"summary"`
	Documents                        []GraphDocumentStatus `json:"documents"`
	ErrorCode                        *string               `json:"error_code,omitempty"`
	ErrorMessage                     *string               `json:"error_message,omitempty"`
	CanSearch                        bool                  `json:"can_search"`
	CanRetry                         bool                  `json:"can_retry"`
	CanRebuild                       bool                  `json:"can_rebuild"`
	GraphEmbedding                   GraphEmbeddingStatus  `json:"graph_embedding"`
}

type LifecycleRunRequest struct {
	OrganizationID       uuid.UUID
	WorkspaceID          *uuid.UUID
	DatasetID            uuid.UUID
	DocumentID           *uuid.UUID
	SourceRefID          *uuid.UUID
	SyncRunID            *uuid.UUID
	SyncBatchID          *uuid.UUID
	AssetGenerationNo    *int64
	Trigger              string
	Mode                 string
	IdempotencyKey       string
	EmbeddingProvider    string
	EmbeddingModel       string
	EmbeddingDimension   int
	EmbeddingFingerprint string
}

// SyncBatchDocumentRequest registers the graph operations caused by one
// successful file-reference sync. A replacement contributes an ADD item for
// the new document and a CLEANUP item for the old document to the same run.
type SyncBatchDocumentRequest struct {
	OrganizationID       uuid.UUID
	WorkspaceID          *uuid.UUID
	DatasetID            uuid.UUID
	SyncBatchID          uuid.UUID
	SourceRefID          uuid.UUID
	SyncRunID            uuid.UUID
	DocumentID           uuid.UUID
	CleanupDocumentID    *uuid.UUID
	AssetGenerationNo    *int64
	EmbeddingProvider    string
	EmbeddingModel       string
	EmbeddingDimension   int
	EmbeddingFingerprint string
}

type LifecycleService struct {
	db         *gorm.DB
	runRepo    *repository.GraphFlowRunRepository
	outboxRepo *repository.GraphOutboxRepository
	taskRepo   *repository.GraphFlowTaskRepository
}

func NewLifecycleService(db *gorm.DB) *LifecycleService {
	return &LifecycleService{
		db:         db,
		runRepo:    repository.NewGraphFlowRunRepository(db),
		outboxRepo: repository.NewGraphOutboxRepository(db),
		taskRepo:   repository.NewGraphFlowTaskRepository(db),
	}
}

func (s *LifecycleService) StartBuild(ctx context.Context, request LifecycleRunRequest) (*graphmodel.GraphFlowRun, bool, error) {
	request.Mode = graphmodel.GraphFlowRunModeBuild
	return s.Enqueue(ctx, request)
}

func (s *LifecycleService) StartBackfill(ctx context.Context, request LifecycleRunRequest) (*graphmodel.GraphFlowRun, bool, error) {
	request.Mode = graphmodel.GraphFlowRunModeBackfill
	return s.Enqueue(ctx, request)
}

func (s *LifecycleService) StartRebuild(ctx context.Context, request LifecycleRunRequest) (*graphmodel.GraphFlowRun, bool, error) {
	request.Mode = graphmodel.GraphFlowRunModeRebuild
	return s.Enqueue(ctx, request)
}

func (s *LifecycleService) StartCleanup(ctx context.Context, request LifecycleRunRequest) (*graphmodel.GraphFlowRun, bool, error) {
	request.Mode = graphmodel.GraphFlowRunModeCleanup
	return s.Enqueue(ctx, request)
}

func (s *LifecycleService) StartCleanupInTx(ctx context.Context, tx *gorm.DB, request LifecycleRunRequest) (*graphmodel.GraphFlowRun, bool, error) {
	if tx == nil {
		return nil, false, fmt.Errorf("graph cleanup transaction is required")
	}
	if request.IdempotencyKey == "" {
		return nil, false, ErrGraphFlowIdempotencyKeyRequired
	}
	request.Mode = graphmodel.GraphFlowRunModeCleanup
	return s.enqueueTx(ctx, tx, request)
}

func (s *LifecycleService) StartDatasetPurgeInTx(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	workspaceID *uuid.UUID,
	datasetID uuid.UUID,
) error {
	if tx == nil {
		return fmt.Errorf("graph dataset purge transaction is required")
	}
	if organizationID == uuid.Nil || datasetID == uuid.Nil {
		return fmt.Errorf("graph dataset purge scope is required")
	}

	if err := tx.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		Delete(&graphmodel.GraphOutboxEvent{}).Error; err != nil {
		return err
	}

	event := &graphmodel.GraphOutboxEvent{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		DatasetID:      datasetID,
		EventType:      graphmodel.GraphOutboxEventDatasetPurge,
		AggregateKey:   fmt.Sprintf("dataset-purge:%s", datasetID),
		Payload: map[string]any{
			"dataset_id": datasetID.String(),
		},
		Status:      graphmodel.GraphOutboxStatusPending,
		AvailableAt: time.Now().UTC().Add(datasetPurgeInitialDelay),
	}
	_, _, err := s.outboxRepo.WithTx(tx).CreateOrGet(ctx, event)
	return err
}

func (s *LifecycleService) Enqueue(ctx context.Context, request LifecycleRunRequest) (*graphmodel.GraphFlowRun, bool, error) {
	if request.IdempotencyKey == "" {
		return nil, false, ErrGraphFlowIdempotencyKeyRequired
	}
	var run *graphmodel.GraphFlowRun
	var created bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		run, created, err = s.enqueueTx(ctx, tx, request)
		return err
	})
	return run, created, err
}

func (s *LifecycleService) ReplaceDocument(
	ctx context.Context,
	build LifecycleRunRequest,
	cleanup LifecycleRunRequest,
) (*graphmodel.GraphFlowRun, *graphmodel.GraphFlowRun, error) {
	if build.IdempotencyKey == "" || cleanup.IdempotencyKey == "" {
		return nil, nil, ErrGraphFlowIdempotencyKeyRequired
	}
	var buildRun *graphmodel.GraphFlowRun
	var cleanupRun *graphmodel.GraphFlowRun
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		build.Mode = graphmodel.GraphFlowRunModeBuild
		cleanup.Mode = graphmodel.GraphFlowRunModeCleanup
		var err error
		buildRun, _, err = s.enqueueTx(ctx, tx, build)
		if err != nil {
			return err
		}
		cleanupRun, _, err = s.enqueueTx(ctx, tx, cleanup)
		return err
	})
	return buildRun, cleanupRun, err
}

// RegisterSyncBatchDocument creates (or reuses) the single parent run for a
// dataset sync batch and records this ref's document operations. It deliberately
// does not publish the run outbox event; TryStartSyncBatch does that only after
// every ref in the batch has reached a terminal sync state.
func (s *LifecycleService) RegisterSyncBatchDocument(ctx context.Context, request SyncBatchDocumentRequest) (*graphmodel.GraphFlowRun, error) {
	if request.OrganizationID == uuid.Nil || request.DatasetID == uuid.Nil || request.SyncBatchID == uuid.Nil ||
		request.SourceRefID == uuid.Nil || request.SyncRunID == uuid.Nil || request.DocumentID == uuid.Nil {
		return nil, fmt.Errorf("graph sync batch scope is required")
	}
	var run *graphmodel.GraphFlowRun
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var dataset datasetmodel.Dataset
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&dataset, "id = ?", request.DatasetID).Error; err != nil {
			return err
		}
		if dataset.OrganizationID != request.OrganizationID.String() ||
			(request.WorkspaceID != nil && dataset.WorkspaceID != request.WorkspaceID.String()) {
			return ErrGraphFlowTenantScopeMismatch
		}

		var existing graphmodel.GraphFlowRun
		err := tx.WithContext(ctx).
			Where("dataset_id = ? AND sync_batch_id = ?", request.DatasetID, request.SyncBatchID).
			First(&existing).Error
		switch {
		case err == nil:
			run = &existing
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		default:
			revision := dataset.GraphRevision + 1
			run = &graphmodel.GraphFlowRun{
				OrganizationID:       request.OrganizationID,
				WorkspaceID:          request.WorkspaceID,
				DatasetID:            request.DatasetID,
				SyncBatchID:          &request.SyncBatchID,
				GraphRevision:        revision,
				EmbeddingProvider:    request.EmbeddingProvider,
				EmbeddingModel:       request.EmbeddingModel,
				EmbeddingDimension:   request.EmbeddingDimension,
				EmbeddingFingerprint: request.EmbeddingFingerprint,
				Trigger:              "document_sync_batch",
				Mode:                 graphmodel.GraphFlowRunModeBuild,
				Status:               graphmodel.GraphFlowRunStatusPending,
				IdempotencyKey:       fmt.Sprintf("ref-sync-batch:%s", request.SyncBatchID),
			}
			if err := tx.WithContext(ctx).Create(run).Error; err != nil {
				return err
			}
			now := time.Now().UTC()
			datasetUpdates := map[string]any{
				"graph_revision":   revision,
				"graph_updated_at": now,
			}
			if dataset.GraphCurrentRunID == nil && dataset.GraphStatus != "failed" && dataset.GraphStatus != "partial" {
				datasetUpdates["graph_status"] = "waiting_content"
				datasetUpdates["graph_progress"] = 0
			}
			if dataset.GraphStatus != "failed" && dataset.GraphStatus != "partial" {
				datasetUpdates["graph_error_code"] = nil
				datasetUpdates["graph_error_message"] = nil
			}
			if err := tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
				Where("id = ?", request.DatasetID).
				Updates(datasetUpdates).Error; err != nil {
				return err
			}
		}

		add := &graphmodel.GraphFlowRunItem{
			RunID:             run.ID,
			OrganizationID:    request.OrganizationID,
			DatasetID:         request.DatasetID,
			SourceRefID:       &request.SourceRefID,
			SyncRunID:         &request.SyncRunID,
			SyncBatchID:       request.SyncBatchID,
			Operation:         graphmodel.GraphFlowRunItemOperationAdd,
			DocumentID:        request.DocumentID,
			PairedDocumentID:  request.CleanupDocumentID,
			AssetGenerationNo: request.AssetGenerationNo,
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(add).Error; err != nil {
			return err
		}
		if request.CleanupDocumentID != nil && *request.CleanupDocumentID != uuid.Nil && *request.CleanupDocumentID != request.DocumentID {
			cleanup := &graphmodel.GraphFlowRunItem{
				RunID:             run.ID,
				OrganizationID:    request.OrganizationID,
				DatasetID:         request.DatasetID,
				SourceRefID:       &request.SourceRefID,
				SyncRunID:         &request.SyncRunID,
				SyncBatchID:       request.SyncBatchID,
				Operation:         graphmodel.GraphFlowRunItemOperationCleanup,
				DocumentID:        *request.CleanupDocumentID,
				PairedDocumentID:  &request.DocumentID,
				AssetGenerationNo: request.AssetGenerationNo,
			}
			if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(cleanup).Error; err != nil {
				return err
			}
		}
		return tx.WithContext(ctx).Model(&datalibrarymodel.KnowledgeBaseAssetRef{}).
			Where("id = ? AND organization_id = ? AND dataset_id = ? AND sync_run_id = ? AND deleted_at IS NULL", request.SourceRefID, request.OrganizationID, request.DatasetID, request.SyncRunID).
			Updates(map[string]any{
				"graph_run_id":      run.ID,
				"graph_sync_status": "waiting",
				"updated_at":        time.Now().UTC(),
			}).Error
	})
	return run, err
}

// TryStartSyncBatch publishes a registered batch exactly once after every ref
// sharing sync_batch_id is either synced or failed and every synced ref has an
// ADD run item. Calls made before that point are harmless no-ops.
func (s *LifecycleService) TryStartSyncBatch(ctx context.Context, organizationID, datasetID, syncBatchID uuid.UUID) error {
	if organizationID == uuid.Nil || datasetID == uuid.Nil || syncBatchID == uuid.Nil {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := s.promoteNextRunTx(ctx, tx, organizationID, datasetID)
		return err
	})
}

// promoteNextRunTx is the only path that moves a run from pending to
// processing. The dataset row lock and the partial unique index together make
// run execution strictly serial for each knowledge base.
func (s *LifecycleService) promoteNextRunTx(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	datasetID uuid.UUID,
) (*graphmodel.GraphFlowRun, error) {
	var dataset datasetmodel.Dataset
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&dataset, "id = ? AND organization_id = ?", datasetID, organizationID).Error; err != nil {
		return nil, err
	}

	var active graphmodel.GraphFlowRun
	err := tx.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND status = ?", organizationID, datasetID, graphmodel.GraphFlowRunStatusProcessing).
		Order("graph_revision ASC, created_at ASC").First(&active).Error
	if err == nil {
		if dataset.GraphCurrentRunID == nil || *dataset.GraphCurrentRunID != active.ID.String() {
			if err := tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
				Where("id = ?", datasetID).
				Updates(map[string]any{
					"graph_current_run_id": active.ID,
					"graph_status":         "building",
					"graph_updated_at":     time.Now().UTC(),
				}).Error; err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var candidate graphmodel.GraphFlowRun
	if err := tx.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND status = ?", organizationID, datasetID, graphmodel.GraphFlowRunStatusPending).
		Order("graph_revision ASC, created_at ASC, id ASC").First(&candidate).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	// Incremental runs must preserve revision continuity, so an unpublished
	// failed predecessor blocks them. A full rebuild is a new complete graph
	// snapshot and can safely recover past historical failed revisions.
	if candidate.Mode != graphmodel.GraphFlowRunModeRebuild {
		availableRevision := int64(0)
		if dataset.GraphAvailableRevision != nil {
			availableRevision = *dataset.GraphAvailableRevision
		}
		var failedPredecessors int64
		if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowRun{}).
			Where("organization_id = ? AND dataset_id = ? AND status = ? AND graph_revision > ? AND graph_revision < ?", organizationID, datasetID, graphmodel.GraphFlowRunStatusFailed, availableRevision, candidate.GraphRevision).
			Count(&failedPredecessors).Error; err != nil {
			return nil, err
		}
		if failedPredecessors > 0 {
			return nil, nil
		}
	}

	ready, err := s.runDispatchReadyTx(ctx, tx, &candidate)
	if err != nil || !ready {
		return nil, err
	}

	now := time.Now().UTC()
	result := tx.WithContext(ctx).Model(&graphmodel.GraphFlowRun{}).
		Where("id = ? AND status = ?", candidate.ID, graphmodel.GraphFlowRunStatusPending).
		Updates(map[string]any{
			"status":           graphmodel.GraphFlowRunStatusProcessing,
			"attempt_count":    gorm.Expr("attempt_count + 1"),
			"started_at":       gorm.Expr("COALESCE(started_at, ?)", now),
			"heartbeat_at":     now,
			"lease_expires_at": now.Add(10 * time.Minute),
			"updated_at":       now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, nil
	}
	candidate.Status = graphmodel.GraphFlowRunStatusProcessing
	candidate.AttemptCount++
	candidate.HeartbeatAt = &now
	leaseExpiresAt := now.Add(10 * time.Minute)
	candidate.LeaseExpiresAt = &leaseExpiresAt
	if candidate.StartedAt == nil {
		candidate.StartedAt = &now
	}

	if _, _, err := s.outboxRepo.WithTx(tx).CreateOrGet(ctx, newRunOutboxEvent(&candidate)); err != nil {
		return nil, err
	}
	if err := updateGraphReferenceRunState(ctx, tx, &candidate, nil, graphmodel.GraphFlowRunStatusPending); err != nil {
		return nil, err
	}
	if err := tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
		Where("id = ? AND organization_id = ?", datasetID, organizationID).
		Updates(map[string]any{
			"graph_current_run_id": candidate.ID,
			"graph_status":         "queued",
			"graph_progress":       candidate.Progress,
			"graph_error_code":     nil,
			"graph_error_message":  nil,
			"graph_updated_at":     now,
		}).Error; err != nil {
		return nil, err
	}
	return &candidate, nil
}

func (s *LifecycleService) runDispatchReadyTx(ctx context.Context, tx *gorm.DB, run *graphmodel.GraphFlowRun) (bool, error) {
	if run == nil || run.SyncBatchID == nil {
		return true, nil
	}
	var totalRefs int64
	refScope := tx.WithContext(ctx).Model(&datalibrarymodel.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND dataset_id = ? AND sync_batch_id = ? AND deleted_at IS NULL", run.OrganizationID, run.DatasetID, *run.SyncBatchID)
	if err := refScope.Count(&totalRefs).Error; err != nil || totalRefs == 0 {
		return false, err
	}
	var inFlight int64
	if err := refScope.Where("sync_status IN ?", []string{
		datalibrarymodel.KnowledgeBaseAssetRefSyncStatusPending,
		datalibrarymodel.KnowledgeBaseAssetRefSyncStatusSyncing,
	}).Count(&inFlight).Error; err != nil || inFlight > 0 {
		return false, err
	}
	var syncedRefs int64
	if err := refScope.Where("sync_status = ?", datalibrarymodel.KnowledgeBaseAssetRefSyncStatusSynced).Count(&syncedRefs).Error; err != nil {
		return false, err
	}
	var addItems int64
	if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowRunItem{}).
		Where("run_id = ? AND operation = ?", run.ID, graphmodel.GraphFlowRunItemOperationAdd).
		Count(&addItems).Error; err != nil {
		return false, err
	}
	return addItems > 0 && addItems >= syncedRefs, nil
}

func (s *LifecycleService) Retry(ctx context.Context, runID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runRepo := s.runRepo.WithTx(tx)
		if err := runRepo.Retry(ctx, runID); err != nil {
			return err
		}
		run, err := runRepo.FindByID(ctx, runID)
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowTask{}).
			Where("run_id = ? AND status = ?", runID, "failed").
			Updates(map[string]any{
				"status":           "pending",
				"progress":         0,
				"started_at":       nil,
				"completed_at":     nil,
				"error_message":    "",
				"error_code":       "",
				"lease_expires_at": nil,
				"heartbeat_at":     nil,
				"updated_at":       time.Now().UTC(),
			}).Error; err != nil {
			return err
		}
		if err := updateGraphReferenceRunState(ctx, tx, run, nil, graphmodel.GraphFlowRunStatusPending); err != nil {
			return err
		}
		if _, err = s.promoteNextRunTx(ctx, tx, run.OrganizationID, run.DatasetID); err != nil {
			return err
		}
		return s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID)
	})
}

func (s *LifecycleService) GetStatus(
	ctx context.Context,
	organizationID uuid.UUID,
	datasetID uuid.UUID,
) (*DatasetGraphStatus, error) {
	var dataset datasetmodel.Dataset
	if err := s.db.WithContext(ctx).First(&dataset, "id = ?", datasetID).Error; err != nil {
		return nil, err
	}
	if dataset.OrganizationID != organizationID.String() {
		return nil, ErrGraphFlowTenantScopeMismatch
	}

	status := &DatasetGraphStatus{
		DatasetID:                        datasetID,
		Enabled:                          dataset.EnableGraphFlow,
		Status:                           dataset.GraphStatus,
		Progress:                         dataset.GraphProgress,
		GraphRevision:                    dataset.GraphRevision,
		GraphVisibilityRevision:          dataset.GraphVisibilityRevision,
		GraphProjectedVisibilityRevision: dataset.GraphProjectedVisibilityRevision,
		AvailableRevision:                dataset.GraphAvailableRevision,
		ErrorCode:                        dataset.GraphErrorCode,
		ErrorMessage:                     dataset.GraphErrorMessage,
		CanRetry:                         dataset.GraphStatus == "failed" || dataset.GraphStatus == "partial",
		CanRebuild:                       dataset.EnableGraphFlow,
		Documents:                        []GraphDocumentStatus{},
		Stages:                           defaultGraphStageStatuses(dataset.GraphStatus == "ready"),
		GraphEmbedding: GraphEmbeddingStatus{
			Mode:          "inherit",
			ModelProvider: pointerValue(dataset.EmbeddingModelProvider),
			Model:         pointerValue(dataset.EmbeddingModel),
		},
	}
	status.CanSearch = dataset.EnableGraphFlow && dataset.GraphAvailableRevision != nil &&
		dataset.GraphVisibilityRevision == dataset.GraphProjectedVisibilityRevision &&
		(dataset.GraphStatus == "ready" || dataset.GraphStatus == "partial")

	if dataset.GraphCurrentRunID != nil {
		if runID, err := uuid.Parse(*dataset.GraphCurrentRunID); err == nil {
			if run, findErr := s.runRepo.FindByID(ctx, runID); findErr == nil {
				status.CurrentRun = &GraphRunStatus{
					ID:        run.ID,
					Mode:      run.Mode,
					Status:    run.Status,
					Progress:  run.Progress,
					CreatedAt: run.CreatedAt,
					UpdatedAt: run.UpdatedAt,
				}
				status.GraphEmbedding.Dimension = run.EmbeddingDimension
				status.GraphEmbedding.Verified = run.EmbeddingDimension > 0
				var tasks []graphmodel.GraphFlowTask
				if err := s.db.WithContext(ctx).Where("run_id = ?", run.ID).Find(&tasks).Error; err != nil {
					return nil, err
				}
				status.Stages, status.CurrentStage = summarizeGraphStageStatuses(tasks)
			}
		}
	}
	if status.CurrentStage == "" && (dataset.GraphStatus == "queued" || dataset.GraphStatus == "building") {
		status.CurrentStage = "extraction"
	}

	var refs []datalibrarymodel.KnowledgeBaseAssetRef
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND dataset_document_id IS NOT NULL AND deleted_at IS NULL", organizationID, datasetID).
		Order("created_at ASC").
		Find(&refs).Error; err != nil {
		return nil, err
	}
	for _, ref := range refs {
		documentStatus := GraphDocumentStatus{
			DocumentID:       *ref.DatasetDocumentID,
			RefID:            ref.ID,
			Status:           graphDocumentStatus(&ref, nil),
			RetrievalEnabled: ref.RetrievalEnabled,
			CurrentRunID:     ref.GraphRunID,
		}
		if ref.GraphRunID != nil {
			if run, err := s.runRepo.FindByID(ctx, *ref.GraphRunID); err == nil {
				documentStatus.Status = graphDocumentStatus(&ref, run)
				documentStatus.ErrorCode = run.ErrorCode
			}
		}
		status.Documents = append(status.Documents, documentStatus)
		status.Summary.DocumentsTotal++
		switch documentStatus.Status {
		case "ready":
			status.Summary.DocumentsReady++
		case "failed":
			status.Summary.DocumentsFailed++
		case "queued", "processing":
			status.Summary.DocumentsProcessing++
		}
	}
	return status, nil
}

func (s *LifecycleService) RebuildDataset(
	ctx context.Context,
	organizationID uuid.UUID,
	datasetID uuid.UUID,
	idempotencyKey string,
) (*graphmodel.GraphFlowRun, bool, error) {
	dataset, err := s.loadScopedDataset(ctx, organizationID, datasetID)
	if err != nil {
		return nil, false, err
	}
	if !dataset.EnableGraphFlow {
		return nil, false, ErrGraphFlowDisabled
	}

	// A failed incremental run already owns durable extraction/alignment
	// results. Resume that run first so the repair only repeats the failed and
	// subsequent serving stages. Starting a new rebuild here would needlessly
	// call the extraction model for every document again.
	availableRevision := int64(0)
	if dataset.GraphAvailableRevision != nil {
		availableRevision = *dataset.GraphAvailableRevision
	}
	var failedRun graphmodel.GraphFlowRun
	err = s.db.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND status = ? AND graph_revision > ?", organizationID, datasetID, graphmodel.GraphFlowRunStatusFailed, availableRevision).
		Order("graph_revision ASC, created_at ASC, id ASC").
		First(&failedRun).Error
	if err == nil {
		if err := s.Retry(ctx, failedRun.ID); err != nil {
			return nil, false, err
		}
		resumed, findErr := s.runRepo.FindByID(ctx, failedRun.ID)
		return resumed, false, findErr
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	return s.StartRebuild(ctx, lifecycleRequestFromDataset(dataset, idempotencyKey, "manual_rebuild"))
}

func (s *LifecycleService) RetryDocument(
	ctx context.Context,
	organizationID uuid.UUID,
	datasetID uuid.UUID,
	documentID uuid.UUID,
	idempotencyKey string,
) (*graphmodel.GraphFlowRun, bool, error) {
	dataset, err := s.loadScopedDataset(ctx, organizationID, datasetID)
	if err != nil {
		return nil, false, err
	}
	if !dataset.EnableGraphFlow {
		return nil, false, ErrGraphFlowDisabled
	}
	var ref datalibrarymodel.KnowledgeBaseAssetRef
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND dataset_document_id = ? AND deleted_at IS NULL", organizationID, datasetID, documentID).
		First(&ref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrStaleDocumentSnapshot
		}
		return nil, false, err
	}
	request := lifecycleRequestFromDataset(dataset, idempotencyKey, "document_retry")
	request.DocumentID = &documentID
	request.SourceRefID = &ref.ID
	request.SyncRunID = ref.SyncRunID
	request.AssetGenerationNo = ref.SyncedGenerationNo
	return s.StartBuild(ctx, request)
}

func (s *LifecycleService) PublishRun(ctx context.Context, runID uuid.UUID) error {
	return s.publishRun(ctx, runID)
}

const graphTaskFailedErrorCode = "graph_task_failed"

type graphRunTaskSummary struct {
	Progress     int
	Complete     bool
	Failed       bool
	ErrorMessage string
	DocumentIDs  []uuid.UUID
}

func (s *LifecycleService) FailRun(ctx context.Context, runID uuid.UUID, code, message string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run graphmodel.GraphFlowRun
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&run, "id = ?", runID).Error; err != nil {
			return err
		}
		if graphRunIsTerminal(run.Status) {
			return nil
		}
		if err := s.runRepo.WithTx(tx).MarkFailed(ctx, runID, code, message); err != nil {
			return err
		}
		if err := updateGraphReferenceRunState(ctx, tx, &run, nil, graphmodel.GraphFlowRunStatusFailed); err != nil {
			return err
		}
		if err := s.clearCurrentRunTx(ctx, tx, &run); err != nil {
			return err
		}
		return s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID)
	})
}

// ReconcileTask projects a task transition into its durable run and dataset state.
func (s *LifecycleService) ReconcileTask(ctx context.Context, taskID uuid.UUID) error {
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil || task == nil || task.RunID == nil {
		return err
	}
	return s.ReconcileRun(ctx, *task.RunID)
}

// ReconcileActiveRuns repairs progress and terminal state after restarts or lost callbacks.
func (s *LifecycleService) ReconcileActiveRuns(ctx context.Context) error {
	var runs []graphmodel.GraphFlowRun
	if err := s.db.WithContext(ctx).
		Where("status = ?", graphmodel.GraphFlowRunStatusProcessing).
		Order("graph_revision ASC, created_at ASC").
		Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		if err := s.ReconcileRun(ctx, run.ID); err != nil {
			return err
		}
	}

	// A restart can leave a dataset with queued work but no active run. Repair
	// those queues through the same promotion path used by normal completion;
	// never reconcile a pending run directly because that would bypass ordering.
	type queuedDataset struct {
		OrganizationID uuid.UUID `gorm:"column:organization_id"`
		DatasetID      uuid.UUID `gorm:"column:dataset_id"`
	}
	var datasets []queuedDataset
	if err := s.db.WithContext(ctx).Model(&graphmodel.GraphFlowRun{}).
		Distinct("organization_id", "dataset_id").
		Where("status = ?", graphmodel.GraphFlowRunStatusPending).
		Scan(&datasets).Error; err != nil {
		return err
	}
	for _, dataset := range datasets {
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := s.promoteNextRunTx(ctx, tx, dataset.OrganizationID, dataset.DatasetID)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileRun computes monotonic run progress and publishes terminal state atomically.
func (s *LifecycleService) ReconcileRun(ctx context.Context, runID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run graphmodel.GraphFlowRun
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&run, "id = ?", runID).Error; err != nil {
			return err
		}
		if graphRunIsTerminal(run.Status) {
			return nil
		}
		// Pending runs are queue entries. Only promoteNextRunTx may move one to
		// processing, so task reconciliation must never start a queued version.
		if run.Status == graphmodel.GraphFlowRunStatusPending {
			return nil
		}
		if run.Mode != graphmodel.GraphFlowRunModeCleanup {
			current, err := graphRunMatchesCurrentRef(ctx, tx, &run)
			if err != nil {
				return err
			}
			if !current {
				if err := repository.NewGraphFlowRunRepository(tx).Supersede(ctx, runID); err != nil {
					return err
				}
				if err := s.clearCurrentRunTx(ctx, tx, &run); err != nil {
					return err
				}
				if err := s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID); err != nil {
					return err
				}
				_, err = s.promoteNextRunTx(ctx, tx, run.OrganizationID, run.DatasetID)
				return err
			}
		}

		var tasks []graphmodel.GraphFlowTask
		if err := tx.WithContext(ctx).Where("run_id = ?", runID).Order("created_at ASC").Find(&tasks).Error; err != nil {
			return err
		}
		var summary graphRunTaskSummary
		if graphRunUsesBatchPipeline(&run) {
			var items []graphmodel.GraphFlowRunItem
			if err := tx.WithContext(ctx).Where("run_id = ?", run.ID).Order("created_at ASC").Find(&items).Error; err != nil {
				return err
			}
			summary = summarizeSyncBatchRunTasks(items, tasks)
		} else {
			attachedTasks, attachErr := attachLegacyGraphRunTasks(ctx, tx, &run, tasks)
			if attachErr != nil {
				return attachErr
			}
			summary = summarizeGraphRunTasks(run.Mode, attachedTasks)
		}
		progress := summary.Progress
		if progress < run.Progress {
			progress = run.Progress
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"progress":   progress,
			"updated_at": now,
		}
		refStatus := run.Status
		switch {
		case summary.Failed:
			updates["status"] = graphmodel.GraphFlowRunStatusFailed
			updates["error_code"] = graphTaskFailedErrorCode
			updates["error_message"] = summary.ErrorMessage
			updates["finished_at"] = now
			updates["lease_expires_at"] = nil
			updates["heartbeat_at"] = nil
			refStatus = graphmodel.GraphFlowRunStatusFailed
		case summary.Complete:
			updates["status"] = graphmodel.GraphFlowRunStatusReady
			updates["progress"] = 100
			updates["finished_at"] = now
			updates["lease_expires_at"] = nil
			updates["heartbeat_at"] = nil
			refStatus = graphmodel.GraphFlowRunStatusReady
		default:
			if len(tasks) > 0 {
				updates["status"] = graphmodel.GraphFlowRunStatusProcessing
				updates["heartbeat_at"] = now
				updates["lease_expires_at"] = now.Add(10 * time.Minute)
			}
		}
		if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowRun{}).
			Where("id = ?", runID).Updates(updates).Error; err != nil {
			return err
		}
		if err := updateGraphReferenceRunState(ctx, tx, &run, summary.DocumentIDs, refStatus); err != nil {
			return err
		}
		if summary.Complete || summary.Failed {
			if err := s.clearCurrentRunTx(ctx, tx, &run); err != nil {
				return err
			}
		}
		if summary.Complete {
			if err := s.publishRunRevisionTx(ctx, tx, &run); err != nil {
				return err
			}
		}
		if err := s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID); err != nil {
			return err
		}
		if summary.Complete {
			_, err := s.promoteNextRunTx(ctx, tx, run.OrganizationID, run.DatasetID)
			return err
		}
		return nil
	})
}

func graphRunUsesBatchPipeline(run *graphmodel.GraphFlowRun) bool {
	return run != nil && (run.SyncBatchID != nil ||
		(run.Mode == graphmodel.GraphFlowRunModeRebuild && run.DocumentID == nil))
}

func (s *LifecycleService) clearCurrentRunTx(ctx context.Context, tx *gorm.DB, run *graphmodel.GraphFlowRun) error {
	if run == nil {
		return nil
	}
	return tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
		Where("id = ? AND organization_id = ? AND graph_current_run_id = ?", run.DatasetID, run.OrganizationID, run.ID).
		Updates(map[string]any{
			"graph_current_run_id": nil,
			"graph_updated_at":     time.Now().UTC(),
		}).Error
}

func (s *LifecycleService) publishRunRevisionTx(ctx context.Context, tx *gorm.DB, run *graphmodel.GraphFlowRun) error {
	if run == nil {
		return nil
	}
	now := time.Now().UTC()
	return tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
		Where("id = ? AND organization_id = ?", run.DatasetID, run.OrganizationID).
		Updates(map[string]any{
			"graph_available_revision": run.GraphRevision,
			"graph_projected_revision": run.GraphRevision,
			"graph_ready_at":           now,
			"graph_updated_at":         now,
		}).Error
}

func attachLegacyGraphRunTasks(
	ctx context.Context,
	tx *gorm.DB,
	run *graphmodel.GraphFlowRun,
	linked []graphmodel.GraphFlowTask,
) ([]graphmodel.GraphFlowTask, error) {
	documentSet := make(map[uuid.UUID]bool)
	if run.DocumentID != nil {
		documentSet[*run.DocumentID] = true
	}
	for _, task := range linked {
		documentSet[task.DocumentID] = true
	}
	if len(documentSet) == 0 {
		return linked, nil
	}
	documentIDs := make([]uuid.UUID, 0, len(documentSet))
	for documentID := range documentSet {
		documentIDs = append(documentIDs, documentID)
	}
	var candidates []graphmodel.GraphFlowTask
	if err := tx.WithContext(ctx).
		Where("run_id IS NULL AND kb_id = ? AND document_id IN ?", run.DatasetID, documentIDs).
		Order("created_at ASC").Find(&candidates).Error; err != nil {
		return nil, err
	}
	repairedIDs := make([]uuid.UUID, 0, len(candidates))
	for _, task := range candidates {
		if graphTaskRevision(task) != run.GraphRevision {
			continue
		}
		task.RunID = &run.ID
		linked = append(linked, task)
		repairedIDs = append(repairedIDs, task.ID)
	}
	if len(repairedIDs) > 0 {
		if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowTask{}).
			Where("id IN ? AND run_id IS NULL", repairedIDs).
			Update("run_id", run.ID).Error; err != nil {
			return nil, err
		}
	}
	return linked, nil
}

func graphTaskRevision(task graphmodel.GraphFlowTask) int64 {
	if task.Metadata == nil {
		return 0
	}
	value, ok := task.Metadata["graph_revision"]
	if !ok {
		return 0
	}
	revision, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil {
		return 0
	}
	return revision
}

func (s *LifecycleService) publishRun(ctx context.Context, runID uuid.UUID) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run graphmodel.GraphFlowRun
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&run, "id = ?", runID).Error; err != nil {
			return err
		}
		if run.Mode != graphmodel.GraphFlowRunModeCleanup {
			current, err := graphRunMatchesCurrentRef(ctx, tx, &run)
			if err != nil {
				return err
			}
			if !current {
				if err := repository.NewGraphFlowRunRepository(tx).Supersede(ctx, runID); err != nil {
					return err
				}
				if err := s.clearCurrentRunTx(ctx, tx, &run); err != nil {
					return err
				}
				_, err := s.promoteNextRunTx(ctx, tx, run.OrganizationID, run.DatasetID)
				return err
			}
		}
		now := time.Now().UTC()
		if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowRun{}).
			Where("id = ?", runID).
			Updates(map[string]any{
				"status":           graphmodel.GraphFlowRunStatusReady,
				"progress":         100,
				"finished_at":      now,
				"lease_expires_at": nil,
				"heartbeat_at":     nil,
				"updated_at":       now,
			}).Error; err != nil {
			return err
		}
		if err := updateGraphReferenceRunState(ctx, tx, &run, nil, graphmodel.GraphFlowRunStatusReady); err != nil {
			return err
		}
		if err := s.clearCurrentRunTx(ctx, tx, &run); err != nil {
			return err
		}
		if err := s.publishRunRevisionTx(ctx, tx, &run); err != nil {
			return err
		}
		if err := s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID); err != nil {
			return err
		}
		_, err := s.promoteNextRunTx(ctx, tx, run.OrganizationID, run.DatasetID)
		return err
	})
}

func graphRunIsTerminal(status string) bool {
	return status == graphmodel.GraphFlowRunStatusReady ||
		status == graphmodel.GraphFlowRunStatusFailed ||
		status == graphmodel.GraphFlowRunStatusCancelled ||
		status == graphmodel.GraphFlowRunStatusSuperseded
}

var graphStageKeys = []string{"extraction", "alignment", "graph_sync", "vector_sync"}

func defaultGraphStageStatuses(completed bool) []GraphStageStatus {
	stages := make([]GraphStageStatus, 0, len(graphStageKeys))
	for _, key := range graphStageKeys {
		stage := GraphStageStatus{Key: key, Status: "pending"}
		if completed {
			stage.Status = "completed"
			stage.Progress = 100
		}
		stages = append(stages, stage)
	}
	return stages
}

func summarizeGraphStageStatuses(tasks []graphmodel.GraphFlowTask) ([]GraphStageStatus, string) {
	byType := make(map[string][]graphmodel.GraphFlowTask, len(graphStageKeys))
	for _, task := range tasks {
		byType[task.TaskType] = append(byType[task.TaskType], task)
	}

	stages := make([]GraphStageStatus, 0, len(graphStageKeys))
	currentStage := ""
	for _, key := range graphStageKeys {
		stage := GraphStageStatus{Key: key, Status: "pending"}
		stageTasks := byType[key]
		if len(stageTasks) > 0 {
			allCompleted := true
			failed := false
			started := false
			progressTotal := 0
			for _, task := range stageTasks {
				progress := clampGraphTaskProgress(task.Progress)
				if task.Status == "completed" {
					progress = 100
				} else {
					allCompleted = false
				}
				if task.Status == "failed" {
					failed = true
				}
				if task.Status == "processing" || progress > 0 {
					started = true
				}
				progressTotal += progress
			}
			stage.Progress = progressTotal / len(stageTasks)
			switch {
			case failed:
				stage.Status = "failed"
			case allCompleted:
				stage.Status = "completed"
				stage.Progress = 100
			case started:
				stage.Status = "processing"
			}
		}
		if currentStage == "" && stage.Status != "completed" {
			currentStage = key
		}
		stages = append(stages, stage)
	}
	return stages, currentStage
}

func summarizeGraphRunTasks(mode string, tasks []graphmodel.GraphFlowTask) graphRunTaskSummary {
	byDocument := make(map[uuid.UUID]map[string]graphmodel.GraphFlowTask)
	for _, task := range tasks {
		if _, ok := byDocument[task.DocumentID]; !ok {
			byDocument[task.DocumentID] = make(map[string]graphmodel.GraphFlowTask)
		}
		byDocument[task.DocumentID][task.TaskType] = task
	}
	if len(byDocument) == 0 {
		return graphRunTaskSummary{}
	}

	totalProgress := 0
	allComplete := true
	summary := graphRunTaskSummary{DocumentIDs: make([]uuid.UUID, 0, len(byDocument))}
	for documentID, documentTasks := range byDocument {
		summary.DocumentIDs = append(summary.DocumentIDs, documentID)
		progress, complete, failed, message := summarizeGraphDocumentTasks(mode, documentTasks)
		totalProgress += progress
		allComplete = allComplete && complete
		if failed && !summary.Failed {
			summary.Failed = true
			summary.ErrorMessage = message
		}
	}
	summary.Progress = totalProgress / len(byDocument)
	summary.Complete = allComplete && !summary.Failed
	return summary
}

func summarizeSyncBatchRunTasks(items []graphmodel.GraphFlowRunItem, tasks []graphmodel.GraphFlowTask) graphRunTaskSummary {
	summary := graphRunTaskSummary{}
	if len(items) == 0 {
		return summary
	}
	byItemAndType := make(map[string]graphmodel.GraphFlowTask, len(tasks))
	globalByType := make(map[string]graphmodel.GraphFlowTask, 3)
	for _, task := range tasks {
		if task.Status == "failed" && !summary.Failed {
			summary.Failed = true
			summary.ErrorMessage = task.ErrorMessage
			if summary.ErrorMessage == "" {
				summary.ErrorMessage = "Graph task failed."
			}
		}
		if task.RunItemID != nil {
			byItemAndType[task.RunItemID.String()+":"+task.TaskType] = task
		}
		if task.TaskType == "alignment" || task.TaskType == "graph_sync" || task.TaskType == "vector_sync" {
			globalByType[task.TaskType] = task
		}
	}

	itemProgress := 0
	itemsComplete := true
	for _, item := range items {
		taskType := "extraction"
		if item.Operation == graphmodel.GraphFlowRunItemOperationCleanup {
			taskType = "cleanup"
		} else {
			summary.DocumentIDs = append(summary.DocumentIDs, item.DocumentID)
		}
		task, ok := byItemAndType[item.ID.String()+":"+taskType]
		if !ok {
			itemsComplete = false
			continue
		}
		progress := task.Progress
		if task.Status == "completed" {
			progress = 100
		} else {
			itemsComplete = false
		}
		itemProgress += clampGraphTaskProgress(progress)
	}
	summary.Progress = (itemProgress / len(items)) * 60 / 100

	globalComplete := true
	for _, stage := range []struct {
		name   string
		weight int
	}{{"alignment", 20}, {"graph_sync", 10}, {"vector_sync", 10}} {
		task, ok := globalByType[stage.name]
		if !ok {
			globalComplete = false
			continue
		}
		progress := task.Progress
		if task.Status == "completed" {
			progress = 100
		} else {
			globalComplete = false
		}
		summary.Progress += stage.weight * clampGraphTaskProgress(progress) / 100
	}
	summary.Complete = itemsComplete && globalComplete && !summary.Failed
	return summary
}

func clampGraphTaskProgress(progress int) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func summarizeGraphDocumentTasks(mode string, tasks map[string]graphmodel.GraphFlowTask) (int, bool, bool, string) {
	required := []string{"extraction", "alignment", "graph_sync", "vector_sync"}
	weights := map[string]int{"extraction": 60, "alignment": 20, "graph_sync": 10, "vector_sync": 10}
	if mode == graphmodel.GraphFlowRunModeCleanup {
		required = []string{"cleanup"}
		weights = map[string]int{"cleanup": 100}
	}
	progress := 0
	complete := true
	for _, taskType := range required {
		task, ok := tasks[taskType]
		if !ok {
			complete = false
			continue
		}
		if task.Status == "failed" {
			message := task.ErrorMessage
			if message == "" {
				message = "Graph task failed."
			}
			return progress, false, true, message
		}
		taskProgress := task.Progress
		if task.Status == "completed" {
			taskProgress = 100
		} else {
			complete = false
		}
		if taskProgress < 0 {
			taskProgress = 0
		}
		if taskProgress > 100 {
			taskProgress = 100
		}
		progress += weights[taskType] * taskProgress / 100
	}
	return progress, complete, false, ""
}

func updateGraphReferenceRunState(
	ctx context.Context,
	tx *gorm.DB,
	run *graphmodel.GraphFlowRun,
	documentIDs []uuid.UUID,
	status string,
) error {
	if run == nil || run.Mode == graphmodel.GraphFlowRunModeCleanup {
		return nil
	}
	query := tx.WithContext(ctx).Model(&datalibrarymodel.KnowledgeBaseAssetRef{}).
		Where("organization_id = ? AND dataset_id = ? AND deleted_at IS NULL", run.OrganizationID, run.DatasetID)
	if run.SourceRefID != nil {
		query = query.Where("id = ?", *run.SourceRefID)
		if run.DocumentID != nil {
			query = query.Where("dataset_document_id = ?", *run.DocumentID)
		}
		if run.SyncRunID != nil {
			query = query.Where("sync_run_id = ?", *run.SyncRunID)
		}
	} else if len(documentIDs) > 0 {
		query = query.Where("dataset_document_id IN ?", documentIDs)
	} else {
		return nil
	}
	return query.Updates(map[string]any{
		"graph_run_id":      run.ID,
		"graph_sync_status": graphReferenceStatus(status),
		"updated_at":        time.Now().UTC(),
	}).Error
}

func graphReferenceStatus(runStatus string) string {
	switch runStatus {
	case graphmodel.GraphFlowRunStatusPending:
		return "queued"
	case graphmodel.GraphFlowRunStatusProcessing:
		return "processing"
	case graphmodel.GraphFlowRunStatusReady:
		return "ready"
	case graphmodel.GraphFlowRunStatusFailed:
		return "failed"
	case graphmodel.GraphFlowRunStatusSuperseded, graphmodel.GraphFlowRunStatusCancelled:
		return "superseded"
	default:
		return "waiting"
	}
}

func (s *LifecycleService) aggregateDatasetGraphState(
	ctx context.Context,
	tx *gorm.DB,
	organizationID uuid.UUID,
	datasetID uuid.UUID,
) error {
	var dataset datasetmodel.Dataset
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&dataset, "id = ? AND organization_id = ?", datasetID, organizationID).Error; err != nil {
		return err
	}

	var refs []datalibrarymodel.KnowledgeBaseAssetRef
	if err := tx.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND dataset_document_id IS NOT NULL AND deleted_at IS NULL", organizationID, datasetID).
		Find(&refs).Error; err != nil {
		return err
	}

	runIDs := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		if ref.GraphRunID != nil {
			runIDs = append(runIDs, *ref.GraphRunID)
		}
	}
	runsByID := make(map[uuid.UUID]graphmodel.GraphFlowRun, len(runIDs))
	if len(runIDs) > 0 {
		var runs []graphmodel.GraphFlowRun
		if err := tx.WithContext(ctx).Where("id IN ?", runIDs).Find(&runs).Error; err != nil {
			return err
		}
		for _, run := range runs {
			runsByID[run.ID] = run
		}
	}

	total := len(refs)
	ready := 0
	failed := 0
	queued := 0
	processing := 0
	progressTotal := 0
	for _, ref := range refs {
		if ref.GraphRunID != nil {
			if run, ok := runsByID[*ref.GraphRunID]; ok {
				progressTotal += run.Progress
				switch run.Status {
				case graphmodel.GraphFlowRunStatusReady:
					ready++
				case graphmodel.GraphFlowRunStatusFailed:
					failed++
				case graphmodel.GraphFlowRunStatusProcessing:
					processing++
				default:
					queued++
				}
				continue
			}
		}
		if ref.GraphSyncStatus != nil {
			switch *ref.GraphSyncStatus {
			case "ready":
				ready++
				progressTotal += 100
			case "failed":
				failed++
			case "processing":
				processing++
			default:
				queued++
			}
		} else {
			queued++
		}
	}

	cleanupStatuses := []string{
		graphmodel.GraphFlowRunStatusPending,
		graphmodel.GraphFlowRunStatusProcessing,
	}
	availableRevision := int64(0)
	if dataset.GraphAvailableRevision != nil {
		availableRevision = *dataset.GraphAvailableRevision
	}
	cleanupQuery := tx.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND mode = ?", organizationID, datasetID, graphmodel.GraphFlowRunModeCleanup).
		Where("(status IN ?) OR (status = ? AND graph_revision > ?)", cleanupStatuses, graphmodel.GraphFlowRunStatusFailed, availableRevision)
	var cleanupRuns []graphmodel.GraphFlowRun
	if err := cleanupQuery.Find(&cleanupRuns).Error; err != nil {
		return err
	}
	cleanupFailed := false
	for _, run := range cleanupRuns {
		total++
		progressTotal += run.Progress
		switch run.Status {
		case graphmodel.GraphFlowRunStatusProcessing:
			processing++
		case graphmodel.GraphFlowRunStatusFailed:
			failed++
			cleanupFailed = true
		default:
			queued++
		}
	}

	progress := 0
	if total > 0 {
		progress = progressTotal / total
	}
	status := "waiting_content"
	updates := map[string]any{
		"graph_progress":      progress,
		"graph_error_code":    nil,
		"graph_error_message": nil,
		"graph_updated_at":    time.Now().UTC(),
	}
	switch {
	case total == 0:
		status = "waiting_content"
	case processing > 0:
		status = "building"
	case cleanupFailed:
		status = "failed"
		updates["graph_error_code"] = graphTaskFailedErrorCode
		updates["graph_error_message"] = "Graph cleanup failed."
	case failed > 0 && ready > 0:
		status = "partial"
		updates["graph_error_code"] = graphTaskFailedErrorCode
		updates["graph_error_message"] = "One or more graph document runs failed."
	case failed > 0:
		status = "failed"
		updates["graph_error_code"] = graphTaskFailedErrorCode
		updates["graph_error_message"] = "All graph document runs failed."
	case queued > 0:
		status = "queued"
	case ready == len(refs):
		status = "ready"
		updates["graph_progress"] = 100
	}
	updates["graph_status"] = status
	return tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
		Where("id = ? AND organization_id = ?", datasetID, organizationID).
		Updates(updates).Error
}

func graphRunMatchesCurrentRef(ctx context.Context, tx *gorm.DB, run *graphmodel.GraphFlowRun) (bool, error) {
	if run == nil || run.SourceRefID == nil {
		return true, nil
	}
	if run.DocumentID == nil || run.SyncRunID == nil {
		return false, nil
	}
	var ref datalibrarymodel.KnowledgeBaseAssetRef
	err := tx.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND dataset_id = ? AND deleted_at IS NULL", run.SourceRefID, run.OrganizationID, run.DatasetID).
		First(&ref).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if run.WorkspaceID != nil && (ref.WorkspaceID == nil || *ref.WorkspaceID != run.WorkspaceID.String()) {
		return false, nil
	}
	return ref.DatasetDocumentID != nil && *ref.DatasetDocumentID == *run.DocumentID &&
		ref.SyncRunID != nil && *ref.SyncRunID == *run.SyncRunID, nil
}

func (s *LifecycleService) loadScopedDataset(ctx context.Context, organizationID, datasetID uuid.UUID) (*datasetmodel.Dataset, error) {
	var dataset datasetmodel.Dataset
	if err := s.db.WithContext(ctx).First(&dataset, "id = ?", datasetID).Error; err != nil {
		return nil, err
	}
	if dataset.OrganizationID != organizationID.String() {
		return nil, ErrGraphFlowTenantScopeMismatch
	}
	return &dataset, nil
}

func lifecycleRequestFromDataset(dataset *datasetmodel.Dataset, idempotencyKey, trigger string) LifecycleRunRequest {
	organizationID, _ := uuid.Parse(dataset.OrganizationID)
	workspaceID, _ := uuid.Parse(dataset.WorkspaceID)
	datasetID, _ := uuid.Parse(dataset.ID)
	return LifecycleRunRequest{
		OrganizationID:    organizationID,
		WorkspaceID:       &workspaceID,
		DatasetID:         datasetID,
		Trigger:           trigger,
		IdempotencyKey:    idempotencyKey,
		EmbeddingProvider: pointerValue(dataset.EmbeddingModelProvider),
		EmbeddingModel:    pointerValue(dataset.EmbeddingModel),
	}
}

func graphDocumentStatus(ref *datalibrarymodel.KnowledgeBaseAssetRef, run *graphmodel.GraphFlowRun) string {
	if run != nil {
		switch run.Status {
		case graphmodel.GraphFlowRunStatusPending:
			return "queued"
		case graphmodel.GraphFlowRunStatusProcessing:
			return "processing"
		case graphmodel.GraphFlowRunStatusReady:
			return "ready"
		case graphmodel.GraphFlowRunStatusFailed:
			return "failed"
		case graphmodel.GraphFlowRunStatusSuperseded, graphmodel.GraphFlowRunStatusCancelled:
			return "superseded"
		}
	}
	if ref.GraphSyncStatus != nil && *ref.GraphSyncStatus != "" {
		return *ref.GraphSyncStatus
	}
	return "waiting"
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *LifecycleService) enqueueTx(
	ctx context.Context,
	tx *gorm.DB,
	request LifecycleRunRequest,
) (*graphmodel.GraphFlowRun, bool, error) {
	var dataset datasetmodel.Dataset
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&dataset, "id = ?", request.DatasetID).Error; err != nil {
		return nil, false, err
	}
	if dataset.OrganizationID != request.OrganizationID.String() ||
		(request.WorkspaceID != nil && dataset.WorkspaceID != request.WorkspaceID.String()) {
		return nil, false, ErrGraphFlowTenantScopeMismatch
	}

	revision := dataset.GraphRevision + 1
	run := &graphmodel.GraphFlowRun{
		OrganizationID:       request.OrganizationID,
		WorkspaceID:          request.WorkspaceID,
		DatasetID:            request.DatasetID,
		DocumentID:           request.DocumentID,
		SourceRefID:          request.SourceRefID,
		SyncRunID:            request.SyncRunID,
		SyncBatchID:          request.SyncBatchID,
		AssetGenerationNo:    request.AssetGenerationNo,
		GraphRevision:        revision,
		EmbeddingProvider:    request.EmbeddingProvider,
		EmbeddingModel:       request.EmbeddingModel,
		EmbeddingDimension:   request.EmbeddingDimension,
		EmbeddingFingerprint: request.EmbeddingFingerprint,
		Trigger:              request.Trigger,
		Mode:                 request.Mode,
		Status:               graphmodel.GraphFlowRunStatusPending,
		IdempotencyKey:       request.IdempotencyKey,
	}
	persisted, created, err := s.runRepo.WithTx(tx).CreateOrGet(ctx, run)
	if err != nil {
		return persisted, created, err
	}
	if created {
		now := time.Now().UTC()
		updates := map[string]any{
			"graph_revision":   revision,
			"graph_updated_at": now,
		}
		if dataset.GraphCurrentRunID == nil && dataset.GraphStatus != "failed" && dataset.GraphStatus != "partial" {
			updates["graph_status"] = "queued"
			updates["graph_progress"] = 0
		}
		if dataset.GraphStatus != "failed" && dataset.GraphStatus != "partial" {
			updates["graph_error_code"] = nil
			updates["graph_error_message"] = nil
		}
		if err := tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
			Where("id = ?", request.DatasetID).
			Updates(updates).Error; err != nil {
			return nil, false, err
		}
	}
	if _, err := s.promoteNextRunTx(ctx, tx, request.OrganizationID, request.DatasetID); err != nil {
		return nil, false, err
	}
	if err := tx.WithContext(ctx).First(persisted, "id = ?", persisted.ID).Error; err != nil {
		return nil, false, err
	}
	return persisted, created, nil
}

func newRunOutboxEvent(run *graphmodel.GraphFlowRun) *graphmodel.GraphOutboxEvent {
	payload := map[string]any{
		"run_id":         run.ID.String(),
		"dataset_id":     run.DatasetID.String(),
		"mode":           run.Mode,
		"graph_revision": run.GraphRevision,
	}
	if run.DocumentID != nil {
		payload["document_id"] = run.DocumentID.String()
	}
	if run.SourceRefID != nil {
		payload["source_ref_id"] = run.SourceRefID.String()
	}
	if run.SyncBatchID != nil {
		payload["sync_batch_id"] = run.SyncBatchID.String()
	}
	return &graphmodel.GraphOutboxEvent{
		OrganizationID: run.OrganizationID,
		WorkspaceID:    run.WorkspaceID,
		DatasetID:      run.DatasetID,
		RunID:          &run.ID,
		EventType:      graphmodel.GraphOutboxEventRun,
		AggregateKey:   fmt.Sprintf("run:%s", run.ID),
		Payload:        payload,
		Status:         graphmodel.GraphOutboxStatusPending,
		AvailableAt:    time.Now().UTC(),
	}
}

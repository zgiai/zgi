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
	AssetGenerationNo    *int64
	Trigger              string
	Mode                 string
	IdempotencyKey       string
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
		event := newRunOutboxEvent(run)
		_, _, err = s.outboxRepo.WithTx(tx).CreateOrGet(ctx, event)
		return err
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
			}
		}
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
	var runIDs []uuid.UUID
	if err := s.db.WithContext(ctx).Model(&graphmodel.GraphFlowRun{}).
		Where("status IN ?", []string{
			graphmodel.GraphFlowRunStatusPending,
			graphmodel.GraphFlowRunStatusProcessing,
		}).
		Order("created_at ASC").
		Pluck("id", &runIDs).Error; err != nil {
		return err
	}
	for _, runID := range runIDs {
		if err := s.ReconcileRun(ctx, runID); err != nil {
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
		if run.Mode != graphmodel.GraphFlowRunModeCleanup {
			current, err := graphRunMatchesCurrentRef(ctx, tx, &run)
			if err != nil {
				return err
			}
			if !current {
				if err := repository.NewGraphFlowRunRepository(tx).Supersede(ctx, runID); err != nil {
					return err
				}
				return s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID)
			}
		}

		var tasks []graphmodel.GraphFlowTask
		if err := tx.WithContext(ctx).Where("run_id = ?", runID).Order("created_at ASC").Find(&tasks).Error; err != nil {
			return err
		}
		tasks, err := attachLegacyGraphRunTasks(ctx, tx, &run, tasks)
		if err != nil {
			return err
		}
		summary := summarizeGraphRunTasks(run.Mode, tasks)
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
		return s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID)
	})
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
				return repository.NewGraphFlowRunRepository(tx).Supersede(ctx, runID)
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
		return s.aggregateDatasetGraphState(ctx, tx, run.OrganizationID, run.DatasetID)
	})
}

func graphRunIsTerminal(status string) bool {
	return status == graphmodel.GraphFlowRunStatusReady ||
		status == graphmodel.GraphFlowRunStatusFailed ||
		status == graphmodel.GraphFlowRunStatusCancelled ||
		status == graphmodel.GraphFlowRunStatusSuperseded
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
				default:
					processing++
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
			default:
				processing++
			}
		} else {
			processing++
		}
	}

	var cleanupRuns []graphmodel.GraphFlowRun
	if err := tx.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND mode = ? AND status IN ?", organizationID, datasetID, graphmodel.GraphFlowRunModeCleanup, []string{
			graphmodel.GraphFlowRunStatusPending,
			graphmodel.GraphFlowRunStatusProcessing,
		}).Find(&cleanupRuns).Error; err != nil {
		return err
	}
	for _, run := range cleanupRuns {
		total++
		processing++
		progressTotal += run.Progress
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
		if progress == 0 {
			status = "queued"
		}
	case failed > 0 && ready > 0:
		status = "partial"
		updates["graph_error_code"] = graphTaskFailedErrorCode
		updates["graph_error_message"] = "One or more graph document runs failed."
		updates["graph_available_revision"] = dataset.GraphRevision
		updates["graph_projected_revision"] = dataset.GraphRevision
	case failed > 0:
		status = "failed"
		updates["graph_error_code"] = graphTaskFailedErrorCode
		updates["graph_error_message"] = "All graph document runs failed."
	case ready == len(refs):
		status = "ready"
		updates["graph_progress"] = 100
		updates["graph_available_revision"] = dataset.GraphRevision
		updates["graph_projected_revision"] = dataset.GraphRevision
		updates["graph_ready_at"] = time.Now().UTC()
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
	if err != nil || !created {
		return persisted, created, err
	}

	event := newRunOutboxEvent(persisted)
	if _, _, err := s.outboxRepo.WithTx(tx).CreateOrGet(ctx, event); err != nil {
		return nil, false, err
	}
	if err := updateGraphReferenceRunState(
		ctx,
		tx,
		persisted,
		nil,
		graphmodel.GraphFlowRunStatusPending,
	); err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	if err := tx.WithContext(ctx).Model(&datasetmodel.Dataset{}).
		Where("id = ?", request.DatasetID).
		Updates(map[string]any{
			"graph_status":         "queued",
			"graph_revision":       revision,
			"graph_current_run_id": persisted.ID,
			"graph_progress":       0,
			"graph_error_code":     nil,
			"graph_error_message":  nil,
			"graph_updated_at":     now,
		}).Error; err != nil {
		return nil, false, err
	}
	return persisted, true, nil
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

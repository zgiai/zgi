package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestValidateProcessingRequest(t *testing.T) {
	assetID := uuid.New()
	tests := []struct {
		name string
		req  ProcessingRequest
		err  error
	}{
		{
			name: "valid parse request",
			req: ProcessingRequest{
				OrganizationID: "org-1",
				AssetID:        assetID,
				TargetLevel:    model.DocumentProcessingLevelParse,
			},
		},
		{
			name: "requires organization",
			req: ProcessingRequest{
				AssetID:     assetID,
				TargetLevel: model.DocumentProcessingLevelParse,
			},
			err: ErrOrganizationIDRequired,
		},
		{
			name: "requires asset",
			req: ProcessingRequest{
				OrganizationID: "org-1",
				TargetLevel:    model.DocumentProcessingLevelParse,
			},
			err: ErrAssetIDRequired,
		},
		{
			name: "requires target level",
			req: ProcessingRequest{
				OrganizationID: "org-1",
				AssetID:        assetID,
			},
			err: ErrProcessingLevelRequired,
		},
		{
			name: "rejects archive target",
			req: ProcessingRequest{
				OrganizationID: "org-1",
				AssetID:        assetID,
				TargetLevel:    model.DocumentProcessingLevelArchive,
			},
			err: ErrProcessingLevelInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProcessingRequest(tt.req)
			if !errors.Is(err, tt.err) {
				t.Fatalf("err=%v want=%v", err, tt.err)
			}
		})
	}
}

func TestPlanProcessingRequest(t *testing.T) {
	assetID := uuid.New()
	tests := []struct {
		target          string
		wantParse       bool
		wantSplit       bool
		wantVectorize   bool
		wantExtractFull bool
	}{
		{target: model.DocumentProcessingLevelParse, wantParse: true},
		{target: model.DocumentProcessingLevelSplit, wantParse: true, wantSplit: true},
		{target: model.DocumentProcessingLevelVectorize, wantParse: true, wantSplit: true, wantVectorize: true},
		{target: model.DocumentProcessingLevelFull, wantParse: true, wantSplit: true, wantVectorize: true, wantExtractFull: true},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			plan, err := PlanProcessingRequest(ProcessingRequest{
				OrganizationID: "org-1",
				AssetID:        assetID,
				TargetLevel:    tt.target,
			})
			if err != nil {
				t.Fatalf("PlanProcessingRequest: %v", err)
			}
			if plan.AssetID != assetID ||
				plan.TargetLevel != tt.target ||
				plan.WillParse != tt.wantParse ||
				plan.WillSplit != tt.wantSplit ||
				plan.WillVectorize != tt.wantVectorize ||
				plan.WillExtractFull != tt.wantExtractFull {
				t.Fatalf("plan=%+v", plan)
			}
		})
	}
}

func TestProcessingRequestServiceCreatesPlannedRequestWithoutExecution(t *testing.T) {
	assetID := uuid.New()
	repo := &fakeProcessingRequestRepository{}
	svc := NewProcessingRequestService(repo)

	view, err := svc.CreatePlannedRequest(context.Background(), ProcessingRequest{
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		RequestedBy:    "account-1",
	})
	if err != nil {
		t.Fatalf("CreatePlannedRequest: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected persisted request")
	}
	if repo.created.Status != model.ProcessingRequestStatusPlanned ||
		repo.created.TargetLevel != model.DocumentProcessingLevelVectorize ||
		repo.created.PlanJSON["execution_attached"] != false {
		t.Fatalf("created=%+v", repo.created)
	}
	if view == nil ||
		view.Status != model.ProcessingRequestStatusPlanned ||
		view.Plan == nil ||
		!view.Plan.WillParse ||
		!view.Plan.WillSplit ||
		!view.Plan.WillVectorize ||
		view.Plan.WillExtractFull {
		t.Fatalf("view=%+v", view)
	}
}

func TestProcessingRequestServiceListsRequests(t *testing.T) {
	assetID := uuid.New()
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		items: []*model.ProcessingRequest{
			{
				ID:             requestID,
				OrganizationID: "org-1",
				AssetID:        assetID,
				TargetLevel:    model.DocumentProcessingLevelParse,
				Status:         model.ProcessingRequestStatusPlanned,
				PlanJSON: map[string]any{
					"will_parse": true,
				},
			},
		},
		total: 1,
	}
	svc := NewProcessingRequestService(repo)

	views, total, err := svc.ListRequests(context.Background(), repository.ProcessingRequestListFilter{
		OrganizationID: "org-1",
		AssetID:        assetID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if repo.lastFilter.OrganizationID != "org-1" || repo.lastFilter.AssetID != assetID {
		t.Fatalf("filter=%+v", repo.lastFilter)
	}
	if total != 1 || len(views) != 1 || views[0].ID != requestID || views[0].Plan == nil || !views[0].Plan.WillParse {
		t.Fatalf("views=%+v total=%d", views, total)
	}
}

func TestProcessingRequestServiceListsGlobalRequests(t *testing.T) {
	repo := &fakeProcessingRequestRepository{
		items: []*model.ProcessingRequest{
			{
				ID:             uuid.New(),
				OrganizationID: "org-1",
				AssetID:        uuid.New(),
				TargetLevel:    model.DocumentProcessingLevelVectorize,
				Status:         model.ProcessingRequestStatusQueued,
			},
		},
		total: 1,
	}
	svc := NewProcessingRequestService(repo)

	views, total, err := svc.ListRequests(context.Background(), repository.ProcessingRequestListFilter{
		OrganizationID: "org-1",
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		Status:         model.ProcessingRequestStatusQueued,
	})
	if err != nil {
		t.Fatalf("ListRequests: %v", err)
	}
	if total != 1 || len(views) != 1 {
		t.Fatalf("views=%+v total=%d", views, total)
	}
	if repo.lastFilter.AssetID != uuid.Nil ||
		repo.lastFilter.TargetLevel != model.DocumentProcessingLevelVectorize ||
		repo.lastFilter.Status != model.ProcessingRequestStatusQueued {
		t.Fatalf("filter=%+v", repo.lastFilter)
	}
}

func TestProcessingRequestServiceListRejectsInvalidTargetLevel(t *testing.T) {
	svc := NewProcessingRequestService(&fakeProcessingRequestRepository{})

	_, _, err := svc.ListRequests(context.Background(), repository.ProcessingRequestListFilter{
		OrganizationID: "org-1",
		TargetLevel:    "archive",
	})
	if !errors.Is(err, ErrProcessingLevelInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestProcessingRequestServiceQueueSummary(t *testing.T) {
	repo := &fakeProcessingRequestRepository{
		queueSummary: []repository.ProcessingRequestQueueSummary{
			{
				TargetLevel: model.DocumentProcessingLevelSplit,
				Status:      model.ProcessingRequestStatusQueued,
				ExecutorKey: "split-worker",
				Count:       3,
			},
		},
	}
	svc := NewProcessingRequestService(repo)

	summary, err := svc.QueueSummary(context.Background(), repository.ProcessingRequestQueueSummaryFilter{
		OrganizationID: "org-1",
		TargetLevel:    model.DocumentProcessingLevelSplit,
		Status:         model.ProcessingRequestStatusQueued,
		ExecutorKey:    "split-worker",
	})
	if err != nil {
		t.Fatalf("QueueSummary: %v", err)
	}
	if len(summary) != 1 ||
		summary[0].TargetLevel != model.DocumentProcessingLevelSplit ||
		summary[0].Status != model.ProcessingRequestStatusQueued ||
		summary[0].ExecutorKey != "split-worker" ||
		summary[0].Count != 3 {
		t.Fatalf("summary=%+v", summary)
	}
	if repo.lastQueueSummaryFilter.OrganizationID != "org-1" ||
		repo.lastQueueSummaryFilter.TargetLevel != model.DocumentProcessingLevelSplit ||
		repo.lastQueueSummaryFilter.Status != model.ProcessingRequestStatusQueued ||
		repo.lastQueueSummaryFilter.ExecutorKey != "split-worker" {
		t.Fatalf("filter=%+v", repo.lastQueueSummaryFilter)
	}
}

func TestProcessingRequestServiceQueueSummaryRejectsInvalidTargetLevel(t *testing.T) {
	svc := NewProcessingRequestService(&fakeProcessingRequestRepository{})

	_, err := svc.QueueSummary(context.Background(), repository.ProcessingRequestQueueSummaryFilter{
		OrganizationID: "org-1",
		TargetLevel:    "archive",
	})
	if !errors.Is(err, ErrProcessingLevelInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestProcessingRequestServiceStateTransitions(t *testing.T) {
	requestID := uuid.New()
	assetID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusPlanned,
			PlanJSON: map[string]any{
				"will_parse": true,
				"will_split": true,
			},
		},
	}
	svc := NewProcessingRequestService(repo)

	queued, err := svc.QueueRequest(context.Background(), "org-1", requestID)
	if err != nil {
		t.Fatalf("QueueRequest: %v", err)
	}
	if queued.Status != model.ProcessingRequestStatusQueued ||
		queued.QueuedAt == nil ||
		repo.lastPatch.AllowedFrom[0] != model.ProcessingRequestStatusPlanned {
		t.Fatalf("queued=%+v patch=%+v", queued, repo.lastPatch)
	}

	running, err := svc.StartRequest(context.Background(), "org-1", requestID, "artifact-executor")
	if err != nil {
		t.Fatalf("StartRequest: %v", err)
	}
	if running.Status != model.ProcessingRequestStatusRunning ||
		running.ExecutorKey != "artifact-executor" ||
		running.AttemptCount != 1 ||
		running.StartedAt == nil {
		t.Fatalf("running=%+v", running)
	}

	completed, err := svc.CompleteRequest(context.Background(), "org-1", requestID, map[string]any{
		"parse_artifact_id": "parse-1",
	})
	if err != nil {
		t.Fatalf("CompleteRequest: %v", err)
	}
	if completed.Status != model.ProcessingRequestStatusCompleted ||
		completed.CompletedAt == nil ||
		completed.ExecutionMetadata["parse_artifact_id"] != "parse-1" {
		t.Fatalf("completed=%+v", completed)
	}
}

func TestProcessingRequestServiceRetriesFailedRequestAsNewPlannedRequest(t *testing.T) {
	requestID := uuid.New()
	assetID := uuid.New()
	force := true
	workspaceID := "workspace-1"
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			WorkspaceID:    &workspaceID,
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelVectorize,
			Status:         model.ProcessingRequestStatusFailed,
			Force:          false,
			RequestMetadata: map[string]any{
				"source": "ui",
			},
		},
	}
	svc := NewProcessingRequestService(repo)

	retried, err := svc.RetryRequest(context.Background(), "org-1", requestID, "account-1", &force, map[string]any{
		"operator_note": "retry after parser fix",
	})
	if err != nil {
		t.Fatalf("RetryRequest: %v", err)
	}
	if retried.Status != model.ProcessingRequestStatusPlanned ||
		repo.created == nil ||
		repo.created.ID == requestID ||
		repo.created.AssetID != assetID ||
		repo.created.TargetLevel != model.DocumentProcessingLevelVectorize ||
		repo.created.RequestedBy != "account-1" ||
		!repo.created.Force {
		t.Fatalf("retried=%+v created=%+v", retried, repo.created)
	}
	if repo.created.RequestMetadata["source"] != "ui" ||
		repo.created.RequestMetadata["operator_note"] != "retry after parser fix" ||
		repo.created.RequestMetadata["retry_of_request_id"] != requestID.String() ||
		repo.created.RequestMetadata["retry_of_status"] != model.ProcessingRequestStatusFailed {
		t.Fatalf("metadata=%+v", repo.created.RequestMetadata)
	}
}

func TestProcessingRequestServiceRetryRejectsNonTerminalRequest(t *testing.T) {
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusQueued,
		},
	}
	svc := NewProcessingRequestService(repo)

	_, err := svc.RetryRequest(context.Background(), "org-1", requestID, "account-1", nil, nil)
	if !errors.Is(err, ErrProcessingRequestTransitionInvalid) {
		t.Fatalf("err=%v", err)
	}
	if repo.created != nil {
		t.Fatalf("created=%+v", repo.created)
	}
}

func TestProcessingRequestStateMachineRules(t *testing.T) {
	tests := []struct {
		status       string
		wantFrom     []string
		wantTerminal bool
	}{
		{
			status:   model.ProcessingRequestStatusQueued,
			wantFrom: []string{model.ProcessingRequestStatusPlanned},
		},
		{
			status:   model.ProcessingRequestStatusRunning,
			wantFrom: []string{model.ProcessingRequestStatusQueued},
		},
		{
			status:       model.ProcessingRequestStatusCompleted,
			wantFrom:     []string{model.ProcessingRequestStatusRunning},
			wantTerminal: true,
		},
		{
			status:       model.ProcessingRequestStatusFailed,
			wantFrom:     []string{model.ProcessingRequestStatusQueued, model.ProcessingRequestStatusRunning},
			wantTerminal: true,
		},
		{
			status: model.ProcessingRequestStatusCancelled,
			wantFrom: []string{
				model.ProcessingRequestStatusPlanned,
				model.ProcessingRequestStatusQueued,
				model.ProcessingRequestStatusRunning,
			},
			wantTerminal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := AllowedProcessingRequestPreviousStatuses(tt.status)
			if len(got) != len(tt.wantFrom) {
				t.Fatalf("got=%v want=%v", got, tt.wantFrom)
			}
			for i := range got {
				if got[i] != tt.wantFrom[i] {
					t.Fatalf("got=%v want=%v", got, tt.wantFrom)
				}
			}
			if IsProcessingRequestTerminalStatus(tt.status) != tt.wantTerminal {
				t.Fatalf("terminal=%v want=%v", IsProcessingRequestTerminalStatus(tt.status), tt.wantTerminal)
			}
		})
	}
}

func TestProcessingRequestServiceEnqueuesWithExplicitExecutor(t *testing.T) {
	requestID := uuid.New()
	assetID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusPlanned,
			Force:          true,
			PlanJSON: map[string]any{
				"will_parse": true,
				"will_split": true,
			},
		},
	}
	executor := &fakeProcessingRequestExecutor{key: "metadata-only-executor"}
	svc := NewProcessingRequestService(repo)

	view, err := svc.EnqueueRequest(context.Background(), "org-1", requestID, executor)
	if err != nil {
		t.Fatalf("EnqueueRequest: %v", err)
	}
	if view.Status != model.ProcessingRequestStatusQueued || view.QueuedAt == nil {
		t.Fatalf("view=%+v", view)
	}
	if !executor.enqueued ||
		executor.request.RequestID != requestID ||
		executor.request.OrganizationID != "org-1" ||
		executor.request.AssetID != assetID ||
		executor.request.TargetLevel != model.DocumentProcessingLevelSplit ||
		executor.request.Plan == nil ||
		!executor.request.Plan.WillSplit ||
		!executor.request.Force {
		t.Fatalf("executor=%+v", executor)
	}
}

func TestProcessingRequestServiceEnqueueRequiresExplicitExecutor(t *testing.T) {
	svc := NewProcessingRequestService(&fakeProcessingRequestRepository{})

	_, err := svc.EnqueueRequest(context.Background(), "org-1", uuid.New(), nil)
	if !errors.Is(err, ErrProcessingExecutorRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestProcessingRequestServiceEnqueueRejectsUnsupportedExecutorTarget(t *testing.T) {
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelVectorize,
			Status:         model.ProcessingRequestStatusPlanned,
			PlanJSON: map[string]any{
				"will_parse":     true,
				"will_split":     true,
				"will_vectorize": true,
			},
		},
	}
	executor := &registeredProcessingRequestExecutor{
		fakeProcessingRequestExecutor: fakeProcessingRequestExecutor{key: "parse-worker"},
		info: ProcessingRequestExecutorInfo{
			Key:          "parse-worker",
			TargetLevels: []string{model.DocumentProcessingLevelParse},
			Enabled:      true,
		},
	}
	svc := NewProcessingRequestService(repo)

	_, err := svc.EnqueueRequest(context.Background(), "org-1", requestID, executor)
	if !errors.Is(err, ErrProcessingExecutorTargetUnsupported) {
		t.Fatalf("err=%v", err)
	}
	if repo.current.Status != model.ProcessingRequestStatusPlanned ||
		repo.lastPatch.Status != "" ||
		executor.enqueued {
		t.Fatalf("request mutated status=%q patch=%+v enqueued=%v", repo.current.Status, repo.lastPatch, executor.enqueued)
	}
}

func TestProcessingRequestServiceEnqueueFailureMarksRequestFailed(t *testing.T) {
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelParse,
			Status:         model.ProcessingRequestStatusPlanned,
			PlanJSON: map[string]any{
				"will_parse": true,
			},
		},
	}
	executor := &fakeProcessingRequestExecutor{
		key: "metadata-only-executor",
		err: errors.New("queue unavailable"),
	}
	svc := NewProcessingRequestService(repo)

	view, err := svc.EnqueueRequest(context.Background(), "org-1", requestID, executor)
	if !errors.Is(err, executor.err) {
		t.Fatalf("err=%v", err)
	}
	if view == nil ||
		view.Status != model.ProcessingRequestStatusFailed ||
		view.ErrorCode != "enqueue_failed" ||
		view.FailedAt == nil ||
		view.ExecutionMetadata["executor_key"] != "metadata-only-executor" {
		t.Fatalf("view=%+v", view)
	}
}

func TestProcessingRequestServiceClaimsNextQueuedRequest(t *testing.T) {
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		claim: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusRunning,
			ExecutorKey:    "parse-worker",
			AttemptCount:   1,
			PlanJSON: map[string]any{
				"will_parse": true,
				"will_split": true,
			},
		},
	}
	svc := NewProcessingRequestService(repo)

	view, err := svc.ClaimNextQueuedRequest(context.Background(), "org-1", "parse-worker")
	if err != nil {
		t.Fatalf("ClaimNextQueuedRequest: %v", err)
	}
	if repo.lastClaimFilter.OrganizationID != "org-1" ||
		repo.lastClaimFilter.ExecutorKey != "parse-worker" {
		t.Fatalf("filter=%+v", repo.lastClaimFilter)
	}
	if view == nil ||
		view.ID != requestID ||
		view.Status != model.ProcessingRequestStatusRunning ||
		view.ExecutorKey != "parse-worker" ||
		view.AttemptCount != 1 ||
		view.Plan == nil ||
		!view.Plan.WillSplit {
		t.Fatalf("view=%+v", view)
	}
}

func TestProcessingRequestServiceClaimsNextQueuedRequestForExecutor(t *testing.T) {
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		claim: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelSplit,
			Status:         model.ProcessingRequestStatusRunning,
			ExecutorKey:    "split-worker",
			AttemptCount:   1,
			PlanJSON: map[string]any{
				"will_parse": true,
				"will_split": true,
			},
		},
	}
	executor := &registeredProcessingRequestExecutor{
		fakeProcessingRequestExecutor: fakeProcessingRequestExecutor{key: "split-worker"},
		info: ProcessingRequestExecutorInfo{
			Key:          "split-worker",
			TargetLevels: []string{model.DocumentProcessingLevelSplit},
			Enabled:      true,
		},
	}
	svc := NewProcessingRequestService(repo)

	view, err := svc.ClaimNextQueuedRequestForExecutor(context.Background(), "org-1", executor)
	if err != nil {
		t.Fatalf("ClaimNextQueuedRequestForExecutor: %v", err)
	}
	if repo.lastClaimFilter.OrganizationID != "org-1" ||
		repo.lastClaimFilter.ExecutorKey != "split-worker" ||
		len(repo.lastClaimFilter.TargetLevels) != 1 ||
		repo.lastClaimFilter.TargetLevels[0] != model.DocumentProcessingLevelSplit {
		t.Fatalf("filter=%+v", repo.lastClaimFilter)
	}
	if view == nil || view.ID != requestID || view.ExecutorKey != "split-worker" {
		t.Fatalf("view=%+v", view)
	}
}

func TestProcessingRequestServiceClaimNextQueuedRequestValidatesContext(t *testing.T) {
	svc := NewProcessingRequestService(&fakeProcessingRequestRepository{})

	_, err := svc.ClaimNextQueuedRequest(context.Background(), "", "parse-worker")
	if !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("err=%v", err)
	}

	_, err = svc.ClaimNextQueuedRequest(context.Background(), "org-1", "")
	if !errors.Is(err, ErrProcessingExecutorKeyRequired) {
		t.Fatalf("err=%v", err)
	}

	_, err = svc.ClaimNextQueuedRequestForExecutor(context.Background(), "org-1", nil)
	if !errors.Is(err, ErrProcessingExecutorRequired) {
		t.Fatalf("err=%v", err)
	}
}

func TestProcessingRequestServiceRejectsInvalidStateTransition(t *testing.T) {
	requestID := uuid.New()
	repo := &fakeProcessingRequestRepository{
		current: &model.ProcessingRequest{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        uuid.New(),
			TargetLevel:    model.DocumentProcessingLevelParse,
			Status:         model.ProcessingRequestStatusPlanned,
		},
	}
	svc := NewProcessingRequestService(repo)

	_, err := svc.StartRequest(context.Background(), "org-1", requestID, "executor")
	if !errors.Is(err, ErrProcessingRequestTransitionInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestProcessingRequestExecutionRequestShape(t *testing.T) {
	assetID := uuid.New()
	requestID := uuid.New()
	plan := &ProcessingRequestPlan{
		AssetID:     assetID,
		TargetLevel: model.DocumentProcessingLevelVectorize,
		WillParse:   true,
		WillSplit:   true,
	}
	execution := ProcessingExecutionRequest{
		RequestID:      requestID,
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		Plan:           plan,
		Force:          true,
	}
	if execution.RequestID != requestID ||
		execution.OrganizationID != "org-1" ||
		execution.AssetID != assetID ||
		execution.Plan != plan ||
		!execution.Force {
		t.Fatalf("execution=%+v", execution)
	}
}

func TestNewProcessingExecutionRequestValidatesView(t *testing.T) {
	_, err := NewProcessingExecutionRequest(nil)
	if !errors.Is(err, ErrProcessingRequestNotFound) {
		t.Fatalf("err=%v", err)
	}

	_, err = NewProcessingExecutionRequest(&ProcessingRequestView{
		ID:             uuid.New(),
		OrganizationID: "org-1",
		AssetID:        uuid.New(),
		TargetLevel:    model.DocumentProcessingLevelParse,
	})
	if !errors.Is(err, ErrProcessingLevelRequired) {
		t.Fatalf("err=%v", err)
	}
}

type fakeProcessingRequestExecutor struct {
	key      string
	err      error
	enqueued bool
	request  ProcessingExecutionRequest
}

func (e *fakeProcessingRequestExecutor) Key() string {
	return e.key
}

func (e *fakeProcessingRequestExecutor) Enqueue(ctx context.Context, req ProcessingExecutionRequest) error {
	e.enqueued = true
	e.request = req
	return e.err
}

var _ ProcessingRequestExecutor = (*fakeProcessingRequestExecutor)(nil)

type registeredProcessingRequestExecutor struct {
	fakeProcessingRequestExecutor
	info ProcessingRequestExecutorInfo
}

func (e *registeredProcessingRequestExecutor) Info() ProcessingRequestExecutorInfo {
	return e.info
}

var _ RegisteredProcessingRequestExecutor = (*registeredProcessingRequestExecutor)(nil)

type fakeProcessingRequestRepository struct {
	created                   *model.ProcessingRequest
	current                   *model.ProcessingRequest
	claim                     *model.ProcessingRequest
	items                     []*model.ProcessingRequest
	total                     int64
	statusSummary             []repository.ProcessingRequestStatusSummary
	queueSummary              []repository.ProcessingRequestQueueSummary
	lastFilter                repository.ProcessingRequestListFilter
	lastPatch                 repository.ProcessingRequestStatusPatch
	lastClaimFilter           repository.ProcessingRequestClaimFilter
	lastQueueSummaryFilter    repository.ProcessingRequestQueueSummaryFilter
	lastSummaryOrganizationID string
	lastSummaryAssetID        uuid.UUID
}

func (r *fakeProcessingRequestRepository) Create(ctx context.Context, item *model.ProcessingRequest) error {
	if err := item.BeforeCreate(nil); err != nil {
		return err
	}
	if r.current != nil && item.ID == r.current.ID {
		item.ID = uuid.New()
	}
	r.created = item
	return nil
}

func (r *fakeProcessingRequestRepository) GetByID(context.Context, uuid.UUID) (*model.ProcessingRequest, error) {
	return r.current, nil
}

func (r *fakeProcessingRequestRepository) List(ctx context.Context, filter repository.ProcessingRequestListFilter) ([]*model.ProcessingRequest, int64, error) {
	r.lastFilter = filter
	return r.items, r.total, nil
}

func (r *fakeProcessingRequestRepository) StatusSummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) ([]repository.ProcessingRequestStatusSummary, error) {
	r.lastSummaryOrganizationID = organizationID
	r.lastSummaryAssetID = assetID
	return r.statusSummary, nil
}

func (r *fakeProcessingRequestRepository) QueueSummary(ctx context.Context, filter repository.ProcessingRequestQueueSummaryFilter) ([]repository.ProcessingRequestQueueSummary, error) {
	r.lastQueueSummaryFilter = filter
	return r.queueSummary, nil
}

func (r *fakeProcessingRequestRepository) TransitionStatus(ctx context.Context, id uuid.UUID, patch repository.ProcessingRequestStatusPatch) (*model.ProcessingRequest, error) {
	r.lastPatch = patch
	if r.current == nil || r.current.ID != id || r.current.OrganizationID != patch.OrganizationID {
		return nil, nil
	}
	allowed := len(patch.AllowedFrom) == 0
	for _, status := range patch.AllowedFrom {
		if r.current.Status == status {
			allowed = true
			break
		}
	}
	if !allowed {
		return r.current, nil
	}
	r.current.Status = patch.Status
	if patch.ExecutorKey != nil {
		r.current.ExecutorKey = *patch.ExecutorKey
	}
	if patch.ErrorCode != nil {
		r.current.ErrorCode = *patch.ErrorCode
	}
	if patch.ErrorMessage != nil {
		r.current.ErrorMessage = *patch.ErrorMessage
	}
	r.current.AttemptCount += patch.AttemptCountDelta
	r.current.QueuedAt = patch.QueuedAt
	r.current.StartedAt = patch.StartedAt
	r.current.CompletedAt = patch.CompletedAt
	r.current.FailedAt = patch.FailedAt
	r.current.CanceledAt = patch.CanceledAt
	if patch.ExecutionMetadata != nil {
		r.current.ExecutionMetadata = patch.ExecutionMetadata
	}
	return r.current, nil
}

func (r *fakeProcessingRequestRepository) ClaimNextQueued(ctx context.Context, filter repository.ProcessingRequestClaimFilter) (*model.ProcessingRequest, error) {
	r.lastClaimFilter = filter
	return r.claim, nil
}

var _ repository.ProcessingRequestRepository = (*fakeProcessingRequestRepository)(nil)

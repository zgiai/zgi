package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibraryrepo "github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

func TestBeginAndQueueRunProcessingRequestFailsRequestAndAssetWhenEnqueueFails(t *testing.T) {
	enqueueErr := errors.New("redis unavailable")
	assetID := uuid.New()
	requestID := uuid.New()
	runID := requestID
	generationNo := int64(7)
	calls := []string{}
	state := &fileProcessingStateFake{
		beginResult: &datalibraryservice.BeginProcessingRequestResult{
			Asset: &datalibrarymodel.DocumentAsset{
				ID:                 assetID,
				OrganizationID:     "org-1",
				ProductStatus:      datalibrarymodel.DocumentAssetProductStatusParsing,
				ProcessingRunID:    &runID,
				GenerationNo:       generationNo,
				ProcessingProgress: 5,
			},
			ProcessingRequest: &datalibrarymodel.ProcessingRequest{
				ID:             requestID,
				OrganizationID: "org-1",
				AssetID:        assetID,
				TargetLevel:    datalibrarymodel.DocumentProcessingLevelVectorize,
				Status:         datalibrarymodel.ProcessingRequestStatusPlanned,
			},
			ProcessingRunID: runID,
			GenerationNo:    generationNo,
		},
		calls: &calls,
	}
	processing := &fileProcessingRequestServiceFake{calls: &calls}
	enqueuer := &fileProcessingTaskEnqueuerFake{fileErr: enqueueErr, calls: &calls}
	h := &FileHandler{
		assetStateService: state,
		processingService: processing,
		taskEnqueuer:      enqueuer,
	}

	_, err := h.beginAndQueueRunProcessingRequest(context.Background(), &datalibrarymodel.DocumentAsset{
		ID:             assetID,
		OrganizationID: "org-1",
	}, "file-1", "org-1", "acct-1", datalibrarymodel.DocumentProcessingLevelVectorize, FileProcessingRequestModeParseNow, false, "")

	if !errors.Is(err, enqueueErr) {
		t.Fatalf("err=%v want %v", err, enqueueErr)
	}
	assertCallOrder(t, calls, "queue", "enqueue_file")
	if processing.failedID != requestID || processing.failedCode != "enqueue_failed" {
		t.Fatalf("failed request id=%s code=%q", processing.failedID, processing.failedCode)
	}
	if state.failed == nil {
		t.Fatalf("asset was not marked failed")
	}
	if state.failed.AssetID != assetID ||
		state.failed.ProcessingRunID != runID ||
		state.failed.GenerationNo != generationNo ||
		state.failed.ErrorCode != "enqueue_failed" ||
		state.failed.ProcessingStage != datalibrarymodel.DocumentAssetProcessingStageParse {
		t.Fatalf("failed asset input=%+v", state.failed)
	}
}

func TestQueueGenerateAfterConfirmRequestFailsRequestAndAssetWhenEnqueueFails(t *testing.T) {
	enqueueErr := errors.New("redis unavailable")
	assetID := uuid.New()
	requestID := uuid.New()
	runID := uuid.New()
	generationNo := int64(11)
	calls := []string{}
	state := &fileProcessingStateFake{calls: &calls}
	processing := &fileProcessingRequestServiceFake{
		createResult: &datalibraryservice.ProcessingRequestView{
			ID:             requestID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			TargetLevel:    datalibrarymodel.DocumentProcessingLevelVectorize,
			Status:         datalibrarymodel.ProcessingRequestStatusPlanned,
		},
		calls: &calls,
	}
	enqueuer := &fileProcessingTaskEnqueuerFake{generateErr: enqueueErr, calls: &calls}
	h := &FileHandler{
		assetStateService: state,
		processingService: processing,
		taskEnqueuer:      enqueuer,
	}

	_, err := h.queueGenerateAfterConfirmRequest(context.Background(), &datalibrarymodel.DocumentAsset{
		ID:                 assetID,
		OrganizationID:     "org-1",
		ProcessingRunID:    &runID,
		GenerationNo:       generationNo,
		ProcessingProgress: 82,
	}, "file-1", "org-1", "acct-1", datalibrarymodel.DocumentProcessingLevelVectorize, false)

	if !errors.Is(err, enqueueErr) {
		t.Fatalf("err=%v want %v", err, enqueueErr)
	}
	assertCallOrder(t, calls, "queue", "enqueue_generate")
	if processing.failedID != requestID || processing.failedCode != "enqueue_failed" {
		t.Fatalf("failed request id=%s code=%q", processing.failedID, processing.failedCode)
	}
	if state.failed == nil {
		t.Fatalf("asset was not marked failed")
	}
	if state.failed.AssetID != assetID ||
		state.failed.ProcessingRunID != runID ||
		state.failed.GenerationNo != generationNo ||
		state.failed.ErrorCode != "enqueue_failed" ||
		state.failed.ProcessingStage != datalibrarymodel.DocumentAssetProcessingStageVectorize {
		t.Fatalf("failed asset input=%+v", state.failed)
	}
}

func assertCallOrder(t *testing.T, calls []string, before string, after string) {
	t.Helper()
	beforeIndex, afterIndex := -1, -1
	for i, call := range calls {
		if call == before && beforeIndex == -1 {
			beforeIndex = i
		}
		if call == after && afterIndex == -1 {
			afterIndex = i
		}
	}
	if beforeIndex == -1 || afterIndex == -1 || beforeIndex >= afterIndex {
		t.Fatalf("calls=%v want %q before %q", calls, before, after)
	}
}

type fileProcessingStateFake struct {
	beginResult *datalibraryservice.BeginProcessingRequestResult
	failed      *datalibraryservice.FailedStateInput
	calls       *[]string
}

func (s *fileProcessingStateFake) appendCall(call string) {
	if s.calls != nil {
		*s.calls = append(*s.calls, call)
	}
}

func (s *fileProcessingStateFake) CreateOrReuseStoredAsset(ctx context.Context, input datalibraryservice.FileAssetCreateInput) (*datalibrarymodel.DocumentAsset, bool, error) {
	panic("not used")
}

func (s *fileProcessingStateFake) PrepareFileReplacement(ctx context.Context, input datalibraryservice.FileReplacementInput) (*datalibrarymodel.DocumentAsset, error) {
	panic("not used")
}

func (s *fileProcessingStateFake) BeginProcessingRequest(ctx context.Context, input datalibraryservice.BeginProcessingRequestInput) (*datalibraryservice.BeginProcessingRequestResult, error) {
	s.appendCall("begin")
	return s.beginResult, nil
}

func (s *fileProcessingStateFake) MarkParsing(ctx context.Context, input datalibraryservice.RunStateInput) (*datalibrarymodel.DocumentAsset, error) {
	panic("not used")
}

func (s *fileProcessingStateFake) MarkConfirming(ctx context.Context, input datalibraryservice.RunStateInput) (*datalibrarymodel.DocumentAsset, error) {
	panic("not used")
}

func (s *fileProcessingStateFake) MarkGenerating(ctx context.Context, input datalibraryservice.RunStateInput) (*datalibrarymodel.DocumentAsset, error) {
	panic("not used")
}

func (s *fileProcessingStateFake) MarkReady(ctx context.Context, input datalibraryservice.ReadyStateInput) (*datalibrarymodel.DocumentAsset, error) {
	panic("not used")
}

func (s *fileProcessingStateFake) MarkFailed(ctx context.Context, input datalibraryservice.FailedStateInput) (*datalibrarymodel.DocumentAsset, error) {
	s.appendCall("fail_asset")
	s.failed = &input
	return &datalibrarymodel.DocumentAsset{
		ID:                 input.AssetID,
		OrganizationID:     input.OrganizationID,
		ProductStatus:      datalibrarymodel.DocumentAssetProductStatusParseFailed,
		ProcessingRunID:    &input.ProcessingRunID,
		GenerationNo:       input.GenerationNo,
		ProcessingProgress: input.ProcessingProgress,
	}, nil
}

type fileProcessingRequestServiceFake struct {
	createResult *datalibraryservice.ProcessingRequestView
	failedID     uuid.UUID
	failedCode   string
	calls        *[]string
}

func (s *fileProcessingRequestServiceFake) appendCall(call string) {
	if s.calls != nil {
		*s.calls = append(*s.calls, call)
	}
}

func (s *fileProcessingRequestServiceFake) CreatePlannedRequest(ctx context.Context, req datalibraryservice.ProcessingRequest) (*datalibraryservice.ProcessingRequestView, error) {
	s.appendCall("create")
	if s.createResult != nil {
		return s.createResult, nil
	}
	return &datalibraryservice.ProcessingRequestView{
		ID:             uuid.New(),
		OrganizationID: req.OrganizationID,
		AssetID:        req.AssetID,
		TargetLevel:    req.TargetLevel,
		Status:         datalibrarymodel.ProcessingRequestStatusPlanned,
	}, nil
}

func (s *fileProcessingRequestServiceFake) GetRequest(ctx context.Context, organizationID string, id uuid.UUID) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) ListRequests(ctx context.Context, filter datalibraryrepo.ProcessingRequestListFilter) ([]*datalibraryservice.ProcessingRequestView, int64, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) QueueSummary(ctx context.Context, filter datalibraryrepo.ProcessingRequestQueueSummaryFilter) ([]datalibraryservice.ProcessingRequestQueueSummaryView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) EnqueueRequest(ctx context.Context, organizationID string, id uuid.UUID, executor datalibraryservice.ProcessingRequestExecutor) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) ClaimNextQueuedRequest(ctx context.Context, organizationID string, executorKey string) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) ClaimNextQueuedRequestForExecutor(ctx context.Context, organizationID string, executor datalibraryservice.RegisteredProcessingRequestExecutor) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) QueueRequest(ctx context.Context, organizationID string, id uuid.UUID) (*datalibraryservice.ProcessingRequestView, error) {
	s.appendCall("queue")
	return &datalibraryservice.ProcessingRequestView{
		ID:             id,
		OrganizationID: organizationID,
		Status:         datalibrarymodel.ProcessingRequestStatusQueued,
	}, nil
}

func (s *fileProcessingRequestServiceFake) RetryRequest(ctx context.Context, organizationID string, id uuid.UUID, requestedBy string, force *bool, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) StartRequest(ctx context.Context, organizationID string, id uuid.UUID, executorKey string) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) UpdateRequestExecutionMetadata(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) CompleteRequest(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

func (s *fileProcessingRequestServiceFake) FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error) {
	s.appendCall("fail_request")
	s.failedID = id
	s.failedCode = errorCode
	return &datalibraryservice.ProcessingRequestView{
		ID:             id,
		OrganizationID: organizationID,
		Status:         datalibrarymodel.ProcessingRequestStatusFailed,
		ErrorCode:      errorCode,
		ErrorMessage:   errorMessage,
	}, nil
}

func (s *fileProcessingRequestServiceFake) CancelRequest(ctx context.Context, organizationID string, id uuid.UUID, reason string) (*datalibraryservice.ProcessingRequestView, error) {
	panic("not used")
}

type fileProcessingTaskEnqueuerFake struct {
	fileErr     error
	generateErr error
	calls       *[]string
}

func (e *fileProcessingTaskEnqueuerFake) appendCall(call string) {
	if e.calls != nil {
		*e.calls = append(*e.calls, call)
	}
}

func (e *fileProcessingTaskEnqueuerFake) EnqueueFileProcess(ctx context.Context, processingRequestID uuid.UUID) error {
	e.appendCall("enqueue_file")
	return e.fileErr
}

func (e *fileProcessingTaskEnqueuerFake) EnqueueGenerateCurrentResult(ctx context.Context, processingRequestID uuid.UUID) error {
	e.appendCall("enqueue_generate")
	return e.generateErr
}

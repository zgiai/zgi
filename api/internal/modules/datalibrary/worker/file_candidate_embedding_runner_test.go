package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	datalibmodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

func TestFileCandidateEmbeddingRunnerGeneratesEmbeddings(t *testing.T) {
	assetID := uuid.New()
	requestID := uuid.New()
	svc := &fakeFileCandidateEmbeddingService{}
	processing := &fakeFileCandidateEmbeddingProcessingService{}
	runner := NewFileCandidateEmbeddingRunner(svc, processing)

	err := runner.Run(context.Background(), datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{
		OrganizationID:      "org-1",
		DatasetID:           "dataset-1",
		AssetID:             assetID,
		RequestedBy:         "account-1",
		ProcessingRequestID: requestID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if svc.req.OrganizationID != "org-1" ||
		svc.req.DatasetID != "dataset-1" ||
		svc.req.AssetID != assetID ||
		svc.req.RequestedBy != "account-1" {
		t.Fatalf("req=%+v", svc.req)
	}
	if processing.startedID != requestID ||
		processing.completedID != requestID ||
		processing.completedMetadata["progress_completed"] != int64(12) ||
		processing.completedMetadata["progress_total"] != int64(12) ||
		processing.completedMetadata["target_embedding_count"] != int64(12) {
		t.Fatalf("processing=%+v metadata=%+v", processing, processing.completedMetadata)
	}
}

func TestFileCandidateEmbeddingRunnerPropagatesGenerationError(t *testing.T) {
	expected := errors.New("embedding failed")
	requestID := uuid.New()
	processing := &fakeFileCandidateEmbeddingProcessingService{}
	runner := NewFileCandidateEmbeddingRunner(&fakeFileCandidateEmbeddingService{err: expected}, processing)

	err := runner.Run(context.Background(), datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{
		OrganizationID:      "org-1",
		DatasetID:           "dataset-1",
		AssetID:             uuid.New(),
		ProcessingRequestID: requestID,
	})
	if !errors.Is(err, expected) {
		t.Fatalf("err=%v", err)
	}
	if processing.failedID != requestID ||
		processing.failedCode != "file_candidate_embedding_failed" ||
		processing.failedMessage != expected.Error() {
		t.Fatalf("processing=%+v", processing)
	}
}

type fakeFileCandidateEmbeddingService struct {
	req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest
	err error
}

func (s *fakeFileCandidateEmbeddingService) GenerateCandidateEmbeddings(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) (*datalibraryservice.KnowledgeBaseFileCandidateEmbeddingResult, error) {
	s.req = req
	if s.err != nil {
		return nil, s.err
	}
	return &datalibraryservice.KnowledgeBaseFileCandidateEmbeddingResult{
		AssetID:              req.AssetID,
		GenerationNo:         3,
		EmbeddingProvider:    "qwen",
		EmbeddingModel:       "text-embedding-v3",
		EmbeddingCount:       12,
		TargetEmbeddingCount: 12,
		ChunkCount:           12,
		Addable:              true,
	}, nil
}

type fakeFileCandidateEmbeddingProcessingService struct {
	startedID         uuid.UUID
	completedID       uuid.UUID
	completedMetadata map[string]any
	failedID          uuid.UUID
	failedCode        string
	failedMessage     string
}

func (s *fakeFileCandidateEmbeddingProcessingService) StartRequest(ctx context.Context, organizationID string, id uuid.UUID, executorKey string) (*datalibraryservice.ProcessingRequestView, error) {
	s.startedID = id
	return &datalibraryservice.ProcessingRequestView{ID: id, OrganizationID: organizationID, Status: datalibmodel.ProcessingRequestStatusRunning}, nil
}

func (s *fakeFileCandidateEmbeddingProcessingService) CompleteRequest(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error) {
	s.completedID = id
	s.completedMetadata = metadata
	return &datalibraryservice.ProcessingRequestView{ID: id, OrganizationID: organizationID, Status: datalibmodel.ProcessingRequestStatusCompleted, ExecutionMetadata: metadata}, nil
}

func (s *fakeFileCandidateEmbeddingProcessingService) FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error) {
	s.failedID = id
	s.failedCode = errorCode
	s.failedMessage = errorMessage
	return &datalibraryservice.ProcessingRequestView{ID: id, OrganizationID: organizationID, Status: datalibmodel.ProcessingRequestStatusFailed, ErrorCode: errorCode, ErrorMessage: errorMessage, ExecutionMetadata: metadata}, nil
}

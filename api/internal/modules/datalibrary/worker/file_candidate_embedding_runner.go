package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

const fileCandidateEmbeddingExecutorKey = "data_library:file_candidate_embedding"

type fileCandidateEmbeddingService interface {
	GenerateCandidateEmbeddings(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) (*datalibraryservice.KnowledgeBaseFileCandidateEmbeddingResult, error)
}

type fileCandidateEmbeddingProcessingService interface {
	StartRequest(ctx context.Context, organizationID string, id uuid.UUID, executorKey string) (*datalibraryservice.ProcessingRequestView, error)
	CompleteRequest(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error)
	FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*datalibraryservice.ProcessingRequestView, error)
}

type FileCandidateEmbeddingRunner struct {
	service    fileCandidateEmbeddingService
	processing fileCandidateEmbeddingProcessingService
}

func NewFileCandidateEmbeddingRunner(service fileCandidateEmbeddingService, processing ...fileCandidateEmbeddingProcessingService) *FileCandidateEmbeddingRunner {
	var processingService fileCandidateEmbeddingProcessingService
	if len(processing) > 0 {
		processingService = processing[0]
	}
	return &FileCandidateEmbeddingRunner{service: service, processing: processingService}
}

func (r *FileCandidateEmbeddingRunner) Run(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) error {
	if r == nil || r.service == nil {
		return fmt.Errorf("file candidate embedding runner is not configured")
	}
	if r.processing != nil && req.ProcessingRequestID != uuid.Nil {
		if _, err := r.processing.StartRequest(ctx, req.OrganizationID, req.ProcessingRequestID, fileCandidateEmbeddingExecutorKey); err != nil {
			return err
		}
	}
	result, err := r.service.GenerateCandidateEmbeddings(ctx, req)
	if err != nil {
		if r.processing != nil && req.ProcessingRequestID != uuid.Nil {
			_, _ = r.processing.FailRequest(ctx, req.OrganizationID, req.ProcessingRequestID, "file_candidate_embedding_failed", err.Error(), nil)
		}
		return err
	}
	if r.processing != nil && req.ProcessingRequestID != uuid.Nil {
		_, err = r.processing.CompleteRequest(ctx, req.OrganizationID, req.ProcessingRequestID, fileCandidateEmbeddingCompletionMetadata(req, result))
	}
	return err
}

func fileCandidateEmbeddingCompletionMetadata(req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest, result *datalibraryservice.KnowledgeBaseFileCandidateEmbeddingResult) map[string]any {
	metadata := map[string]any{
		"task_type":  "file_candidate_embedding",
		"dataset_id": req.DatasetID,
		"asset_id":   req.AssetID.String(),
	}
	if result == nil {
		return metadata
	}
	metadata["generation_no"] = result.GenerationNo
	metadata["embedding_provider"] = result.EmbeddingProvider
	metadata["embedding_model"] = result.EmbeddingModel
	metadata["embedding_count"] = result.EmbeddingCount
	metadata["target_embedding_count"] = result.TargetEmbeddingCount
	metadata["chunk_count"] = result.ChunkCount
	metadata["progress_completed"] = result.TargetEmbeddingCount
	metadata["progress_total"] = result.ChunkCount
	metadata["addable"] = result.Addable
	if result.Reason != "" {
		metadata["reason"] = result.Reason
	}
	return metadata
}

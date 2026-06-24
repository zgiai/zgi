package worker

import (
	"context"
	"fmt"

	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

type fileCandidateEmbeddingService interface {
	GenerateCandidateEmbeddings(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) (*datalibraryservice.KnowledgeBaseFileCandidateEmbeddingResult, error)
}

type FileCandidateEmbeddingRunner struct {
	service fileCandidateEmbeddingService
}

func NewFileCandidateEmbeddingRunner(service fileCandidateEmbeddingService) *FileCandidateEmbeddingRunner {
	return &FileCandidateEmbeddingRunner{service: service}
}

func (r *FileCandidateEmbeddingRunner) Run(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) error {
	if r == nil || r.service == nil {
		return fmt.Errorf("file candidate embedding runner is not configured")
	}
	_, err := r.service.GenerateCandidateEmbeddings(ctx, req)
	return err
}

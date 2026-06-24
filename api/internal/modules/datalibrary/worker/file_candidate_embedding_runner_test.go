package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

func TestFileCandidateEmbeddingRunnerGeneratesEmbeddings(t *testing.T) {
	assetID := uuid.New()
	svc := &fakeFileCandidateEmbeddingService{}
	runner := NewFileCandidateEmbeddingRunner(svc)

	err := runner.Run(context.Background(), datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		AssetID:        assetID,
		RequestedBy:    "account-1",
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
}

func TestFileCandidateEmbeddingRunnerPropagatesGenerationError(t *testing.T) {
	expected := errors.New("embedding failed")
	runner := NewFileCandidateEmbeddingRunner(&fakeFileCandidateEmbeddingService{err: expected})

	err := runner.Run(context.Background(), datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		AssetID:        uuid.New(),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("err=%v", err)
	}
}

type fakeFileCandidateEmbeddingService struct {
	req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest
	err error
}

func (s *fakeFileCandidateEmbeddingService) GenerateCandidateEmbeddings(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) (*datalibraryservice.KnowledgeBaseFileCandidateEmbeddingResult, error) {
	s.req = req
	return nil, s.err
}

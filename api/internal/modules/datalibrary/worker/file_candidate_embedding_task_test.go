package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

func TestNewFileCandidateEmbeddingTaskBuildsPayload(t *testing.T) {
	assetID := uuid.New()
	workspaceID := "workspace-1"

	task, err := NewFileCandidateEmbeddingTask(datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{
		OrganizationID:      "org-1",
		WorkspaceID:         &workspaceID,
		DatasetID:           "dataset-1",
		AssetID:             assetID,
		RequestedBy:         "account-1",
		ProcessingRequestID: uuid.MustParse("00000000-0000-0000-0000-000000000123"),
	}, nil)
	if err != nil {
		t.Fatalf("NewFileCandidateEmbeddingTask: %v", err)
	}
	if task.Type() != TypeDataLibraryFileCandidateEmbedding {
		t.Fatalf("task type=%q", task.Type())
	}

	var payload FileCandidateEmbeddingPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.OrganizationID != "org-1" ||
		payload.WorkspaceID == nil ||
		*payload.WorkspaceID != "workspace-1" ||
		payload.DatasetID != "dataset-1" ||
		payload.AssetID != assetID.String() ||
		payload.RequestedBy != "account-1" ||
		payload.ProcessingRequestID != "00000000-0000-0000-0000-000000000123" {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestNewFileCandidateEmbeddingTaskRequiresFields(t *testing.T) {
	assetID := uuid.New()
	tests := []struct {
		name string
		req  datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest
	}{
		{name: "organization", req: datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{DatasetID: "dataset-1", AssetID: assetID}},
		{name: "dataset", req: datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{OrganizationID: "org-1", AssetID: assetID}},
		{name: "asset", req: datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest{OrganizationID: "org-1", DatasetID: "dataset-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewFileCandidateEmbeddingTask(tt.req, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestFileCandidateEmbeddingTaskHandlerCallsRunner(t *testing.T) {
	assetID := uuid.New()
	payload, err := json.Marshal(FileCandidateEmbeddingPayload{
		OrganizationID:      "org-1",
		DatasetID:           "dataset-1",
		AssetID:             assetID.String(),
		RequestedBy:         "account-1",
		ProcessingRequestID: "00000000-0000-0000-0000-000000000123",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	runner := &fakeFileCandidateEmbeddingRunner{}
	handler := NewFileCandidateEmbeddingTaskHandler(runner)

	if err := handler(context.Background(), asynq.NewTask(TypeDataLibraryFileCandidateEmbedding, payload)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if runner.req.OrganizationID != "org-1" ||
		runner.req.DatasetID != "dataset-1" ||
		runner.req.AssetID != assetID ||
		runner.req.RequestedBy != "account-1" ||
		runner.req.ProcessingRequestID.String() != "00000000-0000-0000-0000-000000000123" {
		t.Fatalf("runner req=%+v", runner.req)
	}
}

func TestFileCandidateEmbeddingTaskHandlerRejectsInvalidPayload(t *testing.T) {
	handler := NewFileCandidateEmbeddingTaskHandler(nil)

	err := handler(context.Background(), asynq.NewTask(TypeDataLibraryFileCandidateEmbedding, []byte("{")))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("malformed payload err=%v", err)
	}

	payload, marshalErr := json.Marshal(FileCandidateEmbeddingPayload{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		AssetID:        "not-a-uuid",
	})
	if marshalErr != nil {
		t.Fatalf("marshal payload: %v", marshalErr)
	}
	err = handler(context.Background(), asynq.NewTask(TypeDataLibraryFileCandidateEmbedding, payload))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("invalid uuid err=%v", err)
	}
}

type fakeFileCandidateEmbeddingRunner struct {
	req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest
}

func (r *fakeFileCandidateEmbeddingRunner) Run(ctx context.Context, req datalibraryservice.KnowledgeBaseFileCandidateEmbeddingRequest) error {
	r.req = req
	return nil
}

package service

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

func TestValidateGraphDatasetUpdateRejectsUnsupportedChanges(t *testing.T) {
	embeddingModel := "embedding-v2"
	embeddingProvider := "provider-b"
	disable := false
	entityModel := "extractor-v2"
	dataset := &model.Dataset{
		EnableGraphFlow:        true,
		EmbeddingModel:         stringPointerForGraphTest("embedding-v1"),
		EmbeddingModelProvider: stringPointerForGraphTest("provider-a"),
		EntityModel:            stringPointerForGraphTest("extractor-v1"),
	}

	tests := []struct {
		name string
		req  *UpdateDatasetRequest
		code string
	}{
		{
			name: "embedding model",
			req:  &UpdateDatasetRequest{EmbeddingModel: &embeddingModel},
			code: GraphErrorCodeEmbeddingModelImmutable,
		},
		{
			name: "embedding provider",
			req:  &UpdateDatasetRequest{EmbeddingModelProvider: &embeddingProvider},
			code: GraphErrorCodeEmbeddingModelImmutable,
		},
		{
			name: "disable graph",
			req:  &UpdateDatasetRequest{EnableGraphFlow: &disable},
			code: GraphErrorCodeDisableNotSupported,
		},
		{
			name: "model change without confirmation",
			req:  &UpdateDatasetRequest{EntityModel: &entityModel},
			code: GraphErrorCodeModelChangeConfirmationRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGraphDatasetUpdate(dataset, tt.req)
			var graphErr *GraphOperationError
			if !errors.As(err, &graphErr) || graphErr.Code != tt.code {
				t.Fatalf("error=%v, want graph code %s", err, tt.code)
			}
		})
	}
}

func TestValidateGraphDatasetUpdateAllowsConfirmedExtractionModelChange(t *testing.T) {
	entityModel := "extractor-v2"
	dataset := &model.Dataset{EnableGraphFlow: true, EntityModel: stringPointerForGraphTest("extractor-v1")}
	err := validateGraphDatasetUpdate(dataset, &UpdateDatasetRequest{
		EntityModel:         &entityModel,
		ConfirmGraphRebuild: true,
	})
	if err != nil {
		t.Fatalf("confirmed model change failed: %v", err)
	}
}

func TestInitialGraphStatusAndDocumentFailureIsolation(t *testing.T) {
	if got := initialGraphStatus(true, 0); got != "waiting_content" {
		t.Fatalf("empty graph status=%q", got)
	}
	if got := initialGraphStatus(true, 2); got != "queued" {
		t.Fatalf("backfill graph status=%q", got)
	}
	if got := documentIndexingStatusWithGraph("completed", "failed"); got != "completed" {
		t.Fatalf("document status=%q", got)
	}
}

func stringPointerForGraphTest(value string) *string {
	return &value
}

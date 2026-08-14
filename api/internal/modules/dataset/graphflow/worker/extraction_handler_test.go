package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/extractor"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

func TestValidateExtractionOutcome_AllSegmentsFailed(t *testing.T) {
	err := validateExtractionOutcome(5, 5, 0, 0, "llm timeout")
	if err == nil {
		t.Fatal("expected error when all segment extractions fail")
	}
	if !strings.Contains(err.Error(), "llm timeout") {
		t.Fatalf("expected first failure to be preserved, got %q", err.Error())
	}
}

func TestValidateExtractionOutcome_NoGraphOutput(t *testing.T) {
	err := validateExtractionOutcome(5, 0, 0, 0, "")
	if err == nil {
		t.Fatal("expected error when extraction produces no entity or relationship")
	}
}

func TestValidateExtractionOutcome_PartialSuccessWithOutput(t *testing.T) {
	err := validateExtractionOutcome(5, 4, 1, 0, "segment 3 extraction failed")
	if err != nil {
		t.Fatalf("expected partial success with output to continue, got error: %v", err)
	}
}

func TestNormalizeExtractionResult_AddsDocumentAndRelationEntities(t *testing.T) {
	result := &extractor.ExtractionResult{
		Relationships: []extractor.ExtractedRelationship{
			{Source: "家长", Target: "班主任", Type: "CONTACTS"},
		},
	}

	normalizeExtractionResult(result, "家校沟通知识库.docx")

	entityTypes := make(map[string]string, len(result.Entities))
	for _, entity := range result.Entities {
		entityTypes[entity.Name] = entity.Type
	}

	if entityTypes["家校沟通知识库.docx"] != "Document" {
		t.Fatalf("expected document entity to be synthesized, got %#v", entityTypes)
	}
	if entityTypes["家长"] != "Concept" {
		t.Fatalf("expected relation source entity to be synthesized as Concept, got %#v", entityTypes)
	}
	if entityTypes["班主任"] != "Concept" {
		t.Fatalf("expected relation target entity to be synthesized as Concept, got %#v", entityTypes)
	}
}

type mockDatasetOrganizationGetter struct {
	dataset *dataset_model.Dataset
	err     error
}

func (m mockDatasetOrganizationGetter) GetByID(ctx context.Context, id string) (*dataset_model.Dataset, error) {
	return m.dataset, m.err
}

func TestResolveExtractionOrganizationIDPrefersDatasetOrganization(t *testing.T) {
	task := &model.GraphFlowTask{
		KBID:     uuid.MustParse("46a5dd50-b1a3-4f8b-bdac-afa325da9f8b"),
		TenantID: uuid.MustParse("e932b153-a80b-48b9-aa18-eced2bfd2fcf"),
	}

	orgID := resolveExtractionOrganizationID(context.Background(), mockDatasetOrganizationGetter{
		dataset: &dataset_model.Dataset{
			ID:             task.KBID.String(),
			WorkspaceID:    task.TenantID.String(),
			OrganizationID: "e02faa14-92dd-4677-8304-a8d52920b656",
		},
	}, task)

	if orgID != "e02faa14-92dd-4677-8304-a8d52920b656" {
		t.Fatalf("organizationID = %q, want %q", orgID, "e02faa14-92dd-4677-8304-a8d52920b656")
	}
}

func TestResolveExtractionOrganizationIDFallsBackToTaskTenant(t *testing.T) {
	task := &model.GraphFlowTask{
		KBID:     uuid.MustParse("46a5dd50-b1a3-4f8b-bdac-afa325da9f8b"),
		TenantID: uuid.MustParse("e932b153-a80b-48b9-aa18-eced2bfd2fcf"),
	}

	orgID := resolveExtractionOrganizationID(context.Background(), mockDatasetOrganizationGetter{}, task)
	if orgID != task.TenantID.String() {
		t.Fatalf("organizationID = %q, want fallback %q", orgID, task.TenantID.String())
	}
}

func TestValidateExtractionRunSnapshot(t *testing.T) {
	runID := uuid.New()
	task := &model.GraphFlowTask{RunID: &runID, KBID: uuid.New()}
	run := &model.GraphFlowRun{
		ID:                   runID,
		DatasetID:            task.KBID,
		Status:               model.GraphFlowRunStatusProcessing,
		EmbeddingProvider:    "provider-a",
		EmbeddingModel:       "embedding-v1",
		EmbeddingDimension:   1024,
		EmbeddingFingerprint: "provider-a/embedding-v1/1024",
	}
	dataset := &dataset_model.Dataset{
		ID:                     task.KBID.String(),
		EmbeddingModelProvider: stringPointer("provider-a"),
		EmbeddingModel:         stringPointer("embedding-v1"),
	}

	if err := validateExtractionRunSnapshot(task, run, dataset, 1024); err != nil {
		t.Fatalf("valid snapshot failed: %v", err)
	}
	if err := validateExtractionRunSnapshot(task, run, dataset, 768); !errors.Is(err, errEmbeddingDimensionMismatch) {
		t.Fatalf("dimension error=%v", err)
	}
	run.Status = model.GraphFlowRunStatusSuperseded
	if err := validateExtractionRunSnapshot(task, run, dataset, 1024); !errors.Is(err, errStaleGraphFlowRun) {
		t.Fatalf("stale run error=%v", err)
	}
}

func TestExtractionEvidenceFingerprintIsStableAndDocumentScoped(t *testing.T) {
	datasetID := uuid.New()
	firstDocumentID := uuid.New()
	secondDocumentID := uuid.New()
	segmentID := uuid.New()

	first := extractionEvidenceFingerprint("entity", datasetID, firstDocumentID, segmentID, "Alice", "Person")
	repeated := extractionEvidenceFingerprint("entity", datasetID, firstDocumentID, segmentID, "Alice", "Person")
	second := extractionEvidenceFingerprint("entity", datasetID, secondDocumentID, segmentID, "Alice", "Person")

	if first != repeated {
		t.Fatalf("fingerprint is not stable: %q != %q", first, repeated)
	}
	if first == second {
		t.Fatal("fingerprint must change across document snapshots")
	}
	if len(first) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(first))
	}
}

func stringPointer(value string) *string {
	return &value
}

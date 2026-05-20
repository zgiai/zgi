package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/extractor"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	dataset_model "github.com/zgiai/ginext/internal/modules/dataset/model"
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

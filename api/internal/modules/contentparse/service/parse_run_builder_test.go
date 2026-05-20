package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/contracts"
)

func TestBuildDatasetParseRun(t *testing.T) {
	workspaceID := uuid.New()
	datasetID := uuid.New()
	documentID := uuid.New()
	fileID := uuid.New()
	artifactID := uuid.New()
	plan := &routing.RoutePlan{
		Mode:            contracts.ParseProfileDatasetIndex,
		RequestedEngine: contracts.ParseEngineMineru,
		Primary: &routing.RouteCandidate{
			ProviderKey: "mineru",
			AdapterName: "remote",
			EngineName:  contracts.ParseEngineMineru,
		},
	}
	artifact := &contracts.ParseArtifact{
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityHigh,
		EngineUsed:   contracts.ParseEngineLocal,
		FallbackUsed: true,
		Metadata: map[string]any{
			"executed_provider_key": "local",
			"executed_adapter_name": "hyperparse_sdk",
		},
	}
	summary := map[string]interface{}{
		"duration_ms": float64(12),
	}

	run := BuildDatasetParseRun(DatasetParseRunBuildInput{
		WorkspaceID:    &workspaceID,
		DatasetID:      &datasetID,
		DocumentID:     &documentID,
		FileID:         &fileID,
		ArtifactID:     &artifactID,
		Request:        contracts.ParseRequest{SourceType: contracts.ParseSourceTypeBytes, SourceRef: "file-1", FileName: "a.pdf", Intent: contracts.ParseIntentDatasetIndex, Profile: contracts.ParseProfileDatasetIndex, EngineHint: contracts.ParseEngineMineru},
		RoutePlan:      plan,
		Artifact:       artifact,
		Summary:        summary,
		OrganizationID: "org-1",
	})
	if run.WorkspaceID == nil || *run.WorkspaceID != workspaceID || run.ArtifactID == nil || *run.ArtifactID != artifactID {
		t.Fatalf("ids not assigned: %+v", run)
	}
	if run.PolicyKey != string(contracts.ParseProfileDatasetIndex) || run.RequestedProviderKey != "mineru" {
		t.Fatalf("policy/requested=%q/%q", run.PolicyKey, run.RequestedProviderKey)
	}
	if run.FinalProviderKey != "local" || run.AdapterName != "hyperparse_sdk" || run.EngineName != "local" {
		t.Fatalf("provider attribution=%q/%q/%q", run.FinalProviderKey, run.AdapterName, run.EngineName)
	}
	if run.Status != "succeeded" || run.QualityLevel != "high" || !run.FallbackUsed {
		t.Fatalf("status=%q quality=%q fallback=%v", run.Status, run.QualityLevel, run.FallbackUsed)
	}
	if run.DurationMS == nil || *run.DurationMS != 12 {
		t.Fatalf("DurationMS=%v", run.DurationMS)
	}
	if run.SummaryJSON["organization_id"] != "org-1" {
		t.Fatalf("organization_id=%v", run.SummaryJSON["organization_id"])
	}
	summary["duration_ms"] = float64(99)
	if *run.DurationMS != 12 {
		t.Fatal("run must not retain mutable duration from input summary")
	}
}

func TestDatasetParseRunFallbackFields(t *testing.T) {
	run := BuildDatasetParseRun(DatasetParseRunBuildInput{
		Request: contracts.ParseRequest{SourceType: contracts.ParseSourceTypeBytes, Intent: contracts.ParseIntentDatasetIndex, Profile: contracts.ParseProfileDatasetIndex},
		Summary: map[string]interface{}{
			"status":        "failed",
			"quality_level": "failed",
			"fallback_used": true,
		},
	})
	if run.Status != "failed" || run.QualityLevel != "failed" || !run.FallbackUsed {
		t.Fatalf("fallback fields=%q/%q/%v", run.Status, run.QualityLevel, run.FallbackUsed)
	}
}

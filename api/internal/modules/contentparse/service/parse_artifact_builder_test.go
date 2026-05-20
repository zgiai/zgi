package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestBuildParseArtifactItem(t *testing.T) {
	artifact := &contracts.ParseArtifact{
		ArtifactID:   "artifact-1",
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityHigh,
		EngineUsed:   contracts.ParseEngineLocal,
		Metadata: map[string]any{
			"executed_provider_key": "local",
		},
	}
	result := BuildParseArtifactItem(ParseArtifactBuildInput{
		Request: contracts.ParseRequest{
			Profile: contracts.ParseProfileDatasetIndex,
			Data:    []byte("hello"),
		},
		RoutePlan: &routing.RoutePlan{Primary: &routing.RouteCandidate{ProviderKey: "mineru", EngineName: contracts.ParseEngineMineru}},
		Artifact:  artifact,
		Summary:   map[string]interface{}{"source_content_hash": " existing-hash "},
	})
	if result.Item == nil {
		t.Fatal("expected artifact item")
	}
	if result.SourceContentHash != "existing-hash" {
		t.Fatalf("SourceContentHash=%q", result.SourceContentHash)
	}
	if result.ProviderSignature != "local:local" {
		t.Fatalf("ProviderSignature=%q", result.ProviderSignature)
	}
	if result.Item.Profile != string(contracts.ParseProfileDatasetIndex) || result.Item.CanonicalIRVersion != "v1" {
		t.Fatalf("profile/version=%q/%q", result.Item.Profile, result.Item.CanonicalIRVersion)
	}
	if result.Item.SummaryJSON["artifact_id"] != "artifact-1" {
		t.Fatalf("summary=%v", result.Item.SummaryJSON)
	}
}

func TestBuildParseArtifactItemHashesRequestData(t *testing.T) {
	result := BuildParseArtifactItem(ParseArtifactBuildInput{
		Request:  contracts.ParseRequest{Profile: contracts.ParseProfileDatasetIndex, Data: []byte("hello")},
		Artifact: &contracts.ParseArtifact{EngineUsed: contracts.ParseEngineLocal},
	})
	if result.SourceContentHash != SHA256Hex("hello") {
		t.Fatalf("SourceContentHash=%q", result.SourceContentHash)
	}
	if result.ProviderSignature != "local:local" {
		t.Fatalf("ProviderSignature=%q", result.ProviderSignature)
	}
}

func TestBuildParseArtifactItemNilArtifact(t *testing.T) {
	if got := BuildParseArtifactItem(ParseArtifactBuildInput{}); got.Item != nil || got.SourceContentHash != "" || got.ProviderSignature != "" {
		t.Fatalf("nil artifact result=%+v", got)
	}
}

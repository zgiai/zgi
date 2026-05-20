package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
)

type ParseArtifactBuildInput struct {
	Request   contracts.ParseRequest
	RoutePlan *routing.RoutePlan
	Artifact  *contracts.ParseArtifact
	Summary   map[string]interface{}
}

type ParseArtifactBuildResult struct {
	Item              *model.Artifact
	SourceContentHash string
	ProviderSignature string
}

func BuildParseArtifactItem(input ParseArtifactBuildInput) ParseArtifactBuildResult {
	if input.Artifact == nil {
		return ParseArtifactBuildResult{}
	}
	sourceHash := strings.TrimSpace(ReadStringMap(input.Summary, "source_content_hash"))
	if sourceHash == "" && len(input.Request.Data) > 0 {
		sourceHash = SHA256Hex(string(input.Request.Data))
	}
	providerKey := FinalProviderKey(input.RoutePlan, input.Artifact)
	signature := ProviderSignature(providerKey, input.Artifact.EngineUsed)
	return ParseArtifactBuildResult{
		SourceContentHash: sourceHash,
		ProviderSignature: signature,
		Item: &model.Artifact{
			SourceContentHash:     sourceHash,
			Profile:               string(input.Request.Profile),
			CanonicalIRVersion:    "v1",
			ProviderSignature:     signature,
			ArtifactStorageKey:    "",
			DiagnosticsStorageKey: "",
			SummaryJSON:           ParseArtifactStorageSummary(input.Artifact),
		},
	}
}

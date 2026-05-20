package service

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestProviderExecutionAttributionUsesArtifactMetadata(t *testing.T) {
	plan := &routing.RoutePlan{
		Primary: &routing.RouteCandidate{
			ProviderKey: "planned-provider",
			AdapterName: "planned-adapter",
			EngineName:  contracts.ParseEngineMineru,
		},
	}
	artifact := &contracts.ParseArtifact{
		EngineUsed: contracts.ParseEngineLocal,
		Metadata: map[string]any{
			"attempted_provider_order": []any{" executed-provider ", "", "fallback-provider"},
			"executed_provider_key":    " executed-provider ",
			"executed_adapter_name":    " executed-adapter ",
			"executed_engine_name":     " local ",
		},
	}

	if got := AttemptedProviderOrder(plan, artifact); !reflect.DeepEqual(got, []string{"executed-provider", "fallback-provider"}) {
		t.Fatalf("AttemptedProviderOrder=%v", got)
	}
	if got := FinalProviderKey(plan, artifact); got != "executed-provider" {
		t.Fatalf("FinalProviderKey=%q", got)
	}
	if got := FinalAdapterName(plan, artifact); got != "executed-adapter" {
		t.Fatalf("FinalAdapterName=%q", got)
	}
	if got := FinalEngineName(plan, artifact); got != contracts.ParseEngineLocal {
		t.Fatalf("FinalEngineName=%q", got)
	}
}

func TestProviderExecutionAttributionFallbacks(t *testing.T) {
	plan := &routing.RoutePlan{
		Primary: &routing.RouteCandidate{
			ProviderKey: "planned-provider",
			AdapterName: "planned-adapter",
			EngineName:  contracts.ParseEngineMineru,
		},
	}

	if got := AttemptedProviderOrder(plan, nil); !reflect.DeepEqual(got, []string{"planned-provider"}) {
		t.Fatalf("AttemptedProviderOrder planned=%v", got)
	}
	if got := FinalProviderKey(plan, nil); got != "planned-provider" {
		t.Fatalf("FinalProviderKey planned=%q", got)
	}
	if got := FinalAdapterName(plan, nil); got != "planned-adapter" {
		t.Fatalf("FinalAdapterName planned=%q", got)
	}
	if got := FinalEngineName(plan, nil); got != contracts.ParseEngineMineru {
		t.Fatalf("FinalEngineName planned=%q", got)
	}

	artifact := &contracts.ParseArtifact{EngineUsed: contracts.ParseEngineVLM}
	if got := AttemptedProviderOrder(nil, artifact); !reflect.DeepEqual(got, []string{"vlm"}) {
		t.Fatalf("AttemptedProviderOrder artifact=%v", got)
	}
	if got := FinalProviderKey(nil, artifact); got != "vlm" {
		t.Fatalf("FinalProviderKey artifact=%q", got)
	}
}

func TestApplyRouteExecutionMetadata(t *testing.T) {
	artifact := &contracts.ParseArtifact{}
	attemptedProviders := []string{"primary"}
	attemptedAdapters := []string{"remote"}
	ApplyRouteExecutionMetadata(artifact, routing.RouteCandidate{
		ProviderKey: "fallback",
		AdapterName: "local",
		EngineName:  contracts.ParseEngineLocal,
	}, attemptedProviders, attemptedAdapters, true)

	if !artifact.FallbackUsed {
		t.Fatal("expected fallback flag")
	}
	if artifact.Metadata["executed_provider_key"] != "fallback" {
		t.Fatalf("provider=%v", artifact.Metadata["executed_provider_key"])
	}
	if artifact.Metadata["executed_adapter_name"] != "local" {
		t.Fatalf("adapter=%v", artifact.Metadata["executed_adapter_name"])
	}
	if artifact.Metadata["executed_engine_name"] != contracts.ParseEngineLocal {
		t.Fatalf("engine=%v", artifact.Metadata["executed_engine_name"])
	}
	attemptedProviders[0] = "changed"
	stored := artifact.Metadata["attempted_provider_order"].([]string)
	if stored[0] != "primary" {
		t.Fatalf("attempted providers were not copied: %v", stored)
	}
	if artifact.Metadata["route_fallback_used"] != true {
		t.Fatalf("route_fallback_used=%v", artifact.Metadata["route_fallback_used"])
	}
}

func TestArtifactMetadataStringHelpers(t *testing.T) {
	artifact := &contracts.ParseArtifact{
		Metadata: map[string]any{
			"engine": contracts.ParseEngineLocal,
			"count":  42,
			"order":  []string{" a ", "", "b"},
		},
	}
	if got := ArtifactMetadataString(artifact, "engine"); got != "local" {
		t.Fatalf("engine=%q", got)
	}
	if got := ArtifactMetadataString(artifact, "count"); got != "42" {
		t.Fatalf("count=%q", got)
	}
	if got := ArtifactMetadataStringSlice(artifact, "order"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("order=%v", got)
	}
	if got := CompactStringSlice([]string{" c ", "", "d"}); !reflect.DeepEqual(got, []string{"c", "d"}) {
		t.Fatalf("compact=%v", got)
	}
}

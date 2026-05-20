package service

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func AttemptedProviderOrder(plan *routing.RoutePlan, artifact *contracts.ParseArtifact) []string {
	if values := ArtifactMetadataStringSlice(artifact, "attempted_provider_order"); len(values) > 0 {
		return values
	}
	if plan == nil || plan.Primary == nil {
		if artifact != nil && artifact.EngineUsed != "" {
			return []string{string(artifact.EngineUsed)}
		}
		return nil
	}
	return []string{plan.Primary.ProviderKey}
}

func FinalProviderKey(plan *routing.RoutePlan, artifact *contracts.ParseArtifact) string {
	if value := ArtifactMetadataString(artifact, "executed_provider_key"); value != "" {
		return value
	}
	if plan != nil && plan.Primary != nil && strings.TrimSpace(plan.Primary.ProviderKey) != "" {
		return plan.Primary.ProviderKey
	}
	if artifact != nil && artifact.EngineUsed != "" {
		return string(artifact.EngineUsed)
	}
	return ""
}

func FinalAdapterName(plan *routing.RoutePlan, artifact *contracts.ParseArtifact) string {
	if value := ArtifactMetadataString(artifact, "executed_adapter_name"); value != "" {
		return value
	}
	if plan != nil && plan.Primary != nil {
		return plan.Primary.AdapterName
	}
	return ""
}

func FinalEngineName(plan *routing.RoutePlan, artifact *contracts.ParseArtifact) contracts.ParseEngine {
	if value := ArtifactMetadataString(artifact, "executed_engine_name"); value != "" {
		return contracts.ParseEngine(value)
	}
	if artifact != nil && artifact.EngineUsed != "" {
		return artifact.EngineUsed
	}
	if plan != nil && plan.Primary != nil {
		return plan.Primary.EngineName
	}
	return ""
}

func ApplyRouteExecutionMetadata(artifact *contracts.ParseArtifact, candidate routing.RouteCandidate, attemptedProviders []string, attemptedAdapters []string, fallbackUsed bool) {
	if artifact == nil {
		return
	}
	if fallbackUsed {
		artifact.FallbackUsed = true
	}
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]any{}
	}
	artifact.Metadata["executed_provider_key"] = candidate.ProviderKey
	artifact.Metadata["executed_adapter_name"] = candidate.AdapterName
	artifact.Metadata["executed_engine_name"] = candidate.EngineName
	artifact.Metadata["attempted_provider_order"] = append([]string(nil), attemptedProviders...)
	artifact.Metadata["attempted_adapter_order"] = append([]string(nil), attemptedAdapters...)
	artifact.Metadata["route_fallback_used"] = fallbackUsed
}

func ArtifactMetadataString(artifact *contracts.ParseArtifact, key string) string {
	if artifact == nil || artifact.Metadata == nil {
		return ""
	}
	value, ok := artifact.Metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case contracts.ParseEngine:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func ArtifactMetadataStringSlice(artifact *contracts.ParseArtifact, key string) []string {
	if artifact == nil || artifact.Metadata == nil {
		return nil
	}
	value, ok := artifact.Metadata[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return CompactStringSlice(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprintf("%v", item)); text != "" {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}

func CompactStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			out = append(out, text)
		}
	}
	return out
}

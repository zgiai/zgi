package contentparse

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

type Service struct {
	orchestrator *Orchestrator
	planner      routing.Planner
	catalog      *contracts.ParseProviderCatalog
}

func NewService(orchestrator *Orchestrator, planner routing.Planner, catalog *contracts.ParseProviderCatalog) contracts.ContentParseService {
	return &Service{
		orchestrator: orchestrator,
		planner:      planner,
		catalog:      catalog,
	}
}

func (s *Service) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	return s.orchestrator.Parse(ctx, req)
}

func (s *Service) Health(ctx context.Context) (*contracts.ParseHealth, error) {
	return s.orchestrator.Health(ctx)
}

func (s *Service) ParseWithRouting(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	if s == nil || s.orchestrator == nil {
		return nil, fmt.Errorf("content parse service is not configured")
	}
	if s.planner == nil || s.catalog == nil {
		return s.Parse(ctx, req)
	}

	health, err := s.orchestrator.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("content parse health check failed: %w", err)
	}
	plan, err := s.planner.Plan(req, s.catalog, health)
	if err != nil {
		return nil, fmt.Errorf("content parse route plan failed: %w", err)
	}

	candidates := contentParseRouteCandidates(plan)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("content parse route plan has no executable provider")
	}

	var lastErr error
	attemptedProviders := make([]string, 0, len(candidates))
	attemptedAdapters := make([]string, 0, len(candidates))
	for index, candidate := range candidates {
		adapterName := strings.TrimSpace(candidate.AdapterName)
		if adapterName == "" {
			continue
		}
		if providerKey := strings.TrimSpace(candidate.ProviderKey); providerKey != "" {
			attemptedProviders = append(attemptedProviders, providerKey)
		}
		attemptedAdapters = append(attemptedAdapters, adapterName)

		attemptReq := req
		if candidate.EngineName != "" {
			attemptReq.EngineHint = candidate.EngineName
		}
		artifact, err := s.orchestrator.ParseWithAdapter(ctx, adapterName, attemptReq)
		if err != nil {
			lastErr = err
			continue
		}
		applyContentParseRouteMetadata(artifact, candidate, attemptedProviders, attemptedAdapters, index > 0)
		return artifact, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("content parse route failed: %w", lastErr)
	}
	return nil, fmt.Errorf("content parse route plan has no executable provider")
}

func contentParseRouteCandidates(plan *routing.RoutePlan) []routing.RouteCandidate {
	if plan == nil {
		return nil
	}
	candidates := make([]routing.RouteCandidate, 0, len(plan.FallbackCandidates)+1)
	if plan.Primary != nil {
		candidates = append(candidates, *plan.Primary)
	}
	candidates = append(candidates, plan.FallbackCandidates...)
	return candidates
}

func applyContentParseRouteMetadata(artifact *contracts.ParseArtifact, candidate routing.RouteCandidate, attemptedProviders []string, attemptedAdapters []string, fallbackUsed bool) {
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

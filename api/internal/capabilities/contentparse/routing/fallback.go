package routing

import "github.com/zgiai/zgi/api/internal/contracts"

func buildCandidates(items []contracts.ParseProviderConfig, reason map[string]any) []RouteCandidate {
	candidates := make([]RouteCandidate, 0, len(items))
	for _, item := range items {
		candidates = append(candidates, RouteCandidate{
			ProviderKey:  item.Name,
			AdapterName:  item.Adapter,
			EngineName:   item.Engine,
			Priority:     item.Priority,
			FallbackOnly: item.FallbackOnly,
			Reason:       cloneReason(reason),
		})
	}
	return candidates
}

func candidateFromProvider(item contracts.ParseProviderConfig, reason map[string]any) *RouteCandidate {
	return &RouteCandidate{
		ProviderKey:  item.Name,
		AdapterName:  item.Adapter,
		EngineName:   item.Engine,
		Priority:     item.Priority,
		FallbackOnly: item.FallbackOnly,
		Reason:       cloneReason(reason),
	}
}

func cloneReason(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

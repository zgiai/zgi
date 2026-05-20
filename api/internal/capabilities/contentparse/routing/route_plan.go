package routing

import "github.com/zgiai/zgi/api/internal/contracts"

type RouteCandidate struct {
	ProviderKey  string                `json:"provider_key"`
	AdapterName  string                `json:"adapter_name"`
	EngineName   contracts.ParseEngine `json:"engine_name,omitempty"`
	Priority     int                   `json:"priority,omitempty"`
	FallbackOnly bool                  `json:"fallback_only,omitempty"`
	Reason       map[string]any        `json:"reason,omitempty"`
}

type RoutePlan struct {
	Mode               contracts.ParseProfile `json:"mode"`
	RequestedEngine    contracts.ParseEngine  `json:"requested_engine,omitempty"`
	Primary            *RouteCandidate        `json:"primary,omitempty"`
	FallbackCandidates []RouteCandidate       `json:"fallback_candidates,omitempty"`
	Metadata           map[string]any         `json:"metadata,omitempty"`
}

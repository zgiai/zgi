package dto

const (
	DocumentExtractionStrategyHyperParseMineru  = "mineru"
	DocumentExtractionStrategyHyperParseReducto = "reducto"
	DocumentExtractionStrategyHyperParseLocal   = "local"
	DocumentExtractionStrategyUnstructured      = "unstructured"
	DocumentExtractionStrategyLandingAI         = "landingai"
)

type DocumentExtractionStrategiesResponse struct {
	Strategies          []string                           `json:"strategies"`
	RecommendedStrategy string                             `json:"recommended_strategy,omitempty"`
	Items               []DocumentExtractionStrategyStatus `json:"items,omitempty"`
}

type DocumentExtractionStrategyStatus struct {
	Strategy    string `json:"strategy"`
	Available   bool   `json:"available"`
	Configured  bool   `json:"configured"`
	Recommended bool   `json:"recommended"`
	Reason      string `json:"reason,omitempty"`
}

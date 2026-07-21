package registry

type ImageModel struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	ModelLabel      string   `json:"model_label"`
	SupportedSizes  []string `json:"supported_sizes"`
	SupportedCounts []int    `json:"supported_counts"`
	DefaultSize     string   `json:"default_size"`
	DefaultCount    int      `json:"default_count"`
	Capabilities    []string `json:"capabilities,omitempty"`
	Enabled         bool     `json:"enabled"`
}

package registry

type ImageModel struct {
	Provider          string            `json:"provider"`
	Model             string            `json:"model"`
	ModelLabel        string            `json:"model_label"`
	GenerationProfile GenerationProfile `json:"generation_profile"`
}

type GenerationProfile struct {
	Size     *SizeProfile     `json:"size,omitempty"`
	Quantity *QuantityProfile `json:"quantity,omitempty"`
}

type SizeProfile struct {
	Default string       `json:"default"`
	Options []SizeOption `json:"options"`
}

type SizeOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	AspectRatio string `json:"aspect_ratio"`
}

type QuantityProfile struct {
	Mode    string `json:"mode"`
	Default int    `json:"default,omitempty"`
	Min     int    `json:"min,omitempty"`
	Max     int    `json:"max,omitempty"`
}

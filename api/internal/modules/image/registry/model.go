package registry

import "encoding/json"

type ImageModel struct {
	Provider          string            `json:"provider"`
	Model             string            `json:"model"`
	ModelLabel        string            `json:"model_label"`
	GenerationProfile GenerationProfile `json:"generation_profile"`
	Currency          string            `json:"currency,omitempty"`
	InputPrice        float64           `json:"input_price,omitempty"`
	OutputPrice       float64           `json:"output_price,omitempty"`
	InputConfigured   bool              `json:"input_price_configured"`
	OutputConfigured  bool              `json:"output_price_configured"`
	Pricing           json.RawMessage   `json:"pricing,omitempty"`
	ImagePrices       json.RawMessage   `json:"image_prices,omitempty"`
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

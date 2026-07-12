package registry

import "strings"

type Registry struct {
	models []ImageModel
}

func NewRegistry() *Registry {
	return &Registry{models: []ImageModel{
		{
			Provider:        "qwen",
			Model:           "qwen-image-2.0",
			ModelLabel:      "qwen-image-2.0",
			SupportedSizes:  []string{"1024x1024", "1792x1024", "1024x1792", "1024x768"},
			SupportedCounts: []int{1, 2, 3, 4},
			DefaultSize:     "1024x1024",
			DefaultCount:    1,
			Capabilities:    []string{"text-to-image"},
			Enabled:         true,
		},
	}}
}

func (r *Registry) ListEnabled() []ImageModel {
	if r == nil {
		return nil
	}
	items := make([]ImageModel, 0, len(r.models))
	for _, model := range r.models {
		if model.Enabled {
			items = append(items, cloneModel(model))
		}
	}
	return items
}

func (r *Registry) Get(provider, model string) (ImageModel, bool) {
	provider = strings.TrimSpace(provider)
	model = strings.TrimSpace(model)
	for _, item := range r.ListEnabled() {
		if item.Provider == provider && item.Model == model {
			return item, true
		}
	}
	return ImageModel{}, false
}

func cloneModel(model ImageModel) ImageModel {
	model.SupportedSizes = append([]string{}, model.SupportedSizes...)
	model.SupportedCounts = append([]int{}, model.SupportedCounts...)
	model.Capabilities = append([]string{}, model.Capabilities...)
	return model
}

package registry

import (
	"strings"

	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

const (
	QuantityModeExact    = "exact"
	QuantityModeFixed    = "fixed"
	QuantityModeSequence = "sequence"
)

type Registry struct{}

func NewRegistry() *Registry {
	return &Registry{}
}

// Resolve returns the parameters that are safe for every configured route.
func (r *Registry) Resolve(provider, model string, routes []*channelmodel.RouteQueryResult) GenerationProfile {
	profile := modelProfile(provider, model)
	if emptyProfile(profile) || len(routes) == 0 {
		return GenerationProfile{}
	}
	for _, route := range routes {
		profile = intersectProfiles(profile, routeProfile(provider, model, route))
		if emptyProfile(profile) {
			return GenerationProfile{}
		}
	}
	return profile
}

func modelProfile(provider, model string) GenerationProfile {
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.ToLower(strings.TrimSpace(model))
	switch {
	case provider == "openai" && (model == "gpt-image-2" || strings.HasPrefix(model, "gpt-image-2-")):
		return profile(
			"auto",
			[]SizeOption{
				size("auto", "自动", "auto"),
				size("1024x1024", "1:1 · 1024×1024", "1:1"),
				size("1536x1024", "3:2 · 1536×1024", "3:2"),
				size("1024x1536", "2:3 · 1024×1536", "2:3"),
				size("2048x1152", "16:9 · 2048×1152", "16:9"),
				size("1152x2048", "9:16 · 1152×2048", "9:16"),
			},
			&QuantityProfile{Mode: QuantityModeExact, Default: 1, Min: 1, Max: 10},
		)
	case provider == "qwen" && (strings.HasPrefix(model, "qwen-image-2.0-pro") || strings.HasPrefix(model, "qwen-image-2.0")):
		return profile(
			"2048x2048",
			[]SizeOption{
				size("2048x2048", "1:1 · 2048×2048", "1:1"),
				size("2688x1536", "16:9 · 2688×1536", "16:9"),
				size("1536x2688", "9:16 · 1536×2688", "9:16"),
				size("2368x1728", "4:3 · 2368×1728", "4:3"),
				size("1728x2368", "3:4 · 1728×2368", "3:4"),
			},
			&QuantityProfile{Mode: QuantityModeExact, Default: 1, Min: 1, Max: 6},
		)
	case provider == "qwen" && (model == "qwen-image" || strings.HasPrefix(model, "qwen-image-plus") || strings.HasPrefix(model, "qwen-image-max")):
		return profile(
			"1664x928",
			[]SizeOption{
				size("1664x928", "16:9 · 1664×928", "16:9"),
				size("1472x1104", "4:3 · 1472×1104", "4:3"),
				size("1328x1328", "1:1 · 1328×1328", "1:1"),
				size("1104x1472", "3:4 · 1104×1472", "3:4"),
				size("928x1664", "9:16 · 928×1664", "9:16"),
			},
			&QuantityProfile{Mode: QuantityModeFixed, Default: 1},
		)
	case provider == "doubao" && strings.HasPrefix(model, "doubao-seedream-4-0"):
		return profile(
			"1024x1024",
			[]SizeOption{
				size("1024x1024", "1:1 · 1024×1024", "1:1"),
				size("864x1152", "3:4 · 864×1152", "3:4"),
				size("1152x864", "4:3 · 1152×864", "4:3"),
				size("1312x736", "16:9 · 1312×736", "16:9"),
				size("736x1312", "9:16 · 736×1312", "9:16"),
				size("832x1248", "2:3 · 832×1248", "2:3"),
				size("1248x832", "3:2 · 1248×832", "3:2"),
				size("1568x672", "21:9 · 1568×672", "21:9"),
			},
			&QuantityProfile{Mode: QuantityModeSequence, Default: 1, Min: 2, Max: 15},
		)
	case provider == "doubao" && (strings.HasPrefix(model, "doubao-seedream-4-5") || strings.HasPrefix(model, "doubao-seedream-5-0")):
		return profile(
			"2048x2048",
			[]SizeOption{
				size("2048x2048", "1:1 · 2048×2048", "1:1"),
				size("2304x1728", "4:3 · 2304×1728", "4:3"),
				size("1728x2304", "3:4 · 1728×2304", "3:4"),
				size("2560x1440", "16:9 · 2560×1440", "16:9"),
				size("1440x2560", "9:16 · 1440×2560", "9:16"),
			},
			&QuantityProfile{Mode: QuantityModeFixed, Default: 1},
		)
	default:
		return GenerationProfile{}
	}
}

func routeProfile(modelProvider, model string, route *channelmodel.RouteQueryResult) GenerationProfile {
	if route == nil {
		return GenerationProfile{}
	}
	channelProvider := strings.ToLower(strings.TrimSpace(route.ChannelProvider))
	if routeSupportsNativeImageProfile(modelProvider, channelProvider) {
		return modelProfile(modelProvider, model)
	}
	return GenerationProfile{}
}

func routeSupportsNativeImageProfile(modelProvider, channelProvider string) bool {
	switch strings.ToLower(strings.TrimSpace(modelProvider)) {
	case "openai":
		return channelProvider == "openai" || channelProvider == "zgi-cloud"
	case "qwen":
		return channelProvider == "qwen" || channelProvider == "dashscope" || channelProvider == "aliyun" || channelProvider == "alibaba"
	case "doubao":
		return channelProvider == "doubao" || channelProvider == "ark"
	default:
		return false
	}
}

func profile(defaultSize string, options []SizeOption, quantity *QuantityProfile) GenerationProfile {
	return GenerationProfile{
		Size:     &SizeProfile{Default: defaultSize, Options: options},
		Quantity: quantity,
	}
}

func size(value, label, aspectRatio string) SizeOption {
	return SizeOption{Value: value, Label: label, AspectRatio: aspectRatio}
}

func intersectProfiles(left, right GenerationProfile) GenerationProfile {
	result := GenerationProfile{}
	if left.Size != nil && right.Size != nil {
		options := intersectSizes(left.Size.Options, right.Size.Options)
		if len(options) > 0 {
			defaultSize := left.Size.Default
			if !hasSize(options, defaultSize) {
				defaultSize = options[0].Value
			}
			result.Size = &SizeProfile{Default: defaultSize, Options: options}
		}
	}
	result.Quantity = intersectQuantity(left.Quantity, right.Quantity)
	return result
}

func intersectSizes(left, right []SizeOption) []SizeOption {
	rightValues := make(map[string]struct{}, len(right))
	for _, option := range right {
		rightValues[option.Value] = struct{}{}
	}
	result := make([]SizeOption, 0, len(left))
	for _, option := range left {
		if _, ok := rightValues[option.Value]; ok {
			result = append(result, option)
		}
	}
	return result
}

func intersectQuantity(left, right *QuantityProfile) *QuantityProfile {
	if left == nil || right == nil || left.Mode != right.Mode {
		return nil
	}
	switch left.Mode {
	case QuantityModeFixed:
		if left.Default == right.Default {
			value := *left
			return &value
		}
	case QuantityModeExact, QuantityModeSequence:
		minimum := max(left.Min, right.Min)
		maximum := min(left.Max, right.Max)
		if minimum > maximum {
			return nil
		}
		defaultValue := left.Default
		if defaultValue < minimum || defaultValue > maximum {
			defaultValue = minimum
		}
		return &QuantityProfile{Mode: left.Mode, Default: defaultValue, Min: minimum, Max: maximum}
	}
	return nil
}

func hasSize(options []SizeOption, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

func emptyProfile(profile GenerationProfile) bool {
	return profile.Size == nil && profile.Quantity == nil
}

package gateway

import (
	"strings"

	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func defaultImagePricingRules(provider string) []llmmodel.PricingRule {
	p := canonicalImageProvider(provider)
	switch p {
	case "qwen":
		return []llmmodel.PricingRule{
			{
				ID:       "qwen_default_1024",
				Priority: 100,
				Conditions: map[string]interface{}{
					"size": "1024x1024",
				},
				Price: llmmodel.PricingDetail{Credits: 160, Amount: 0},
			},
			{
				ID:         "qwen_default",
				Priority:   0,
				Conditions: map[string]interface{}{},
				Price:      llmmodel.PricingDetail{Credits: 160, Amount: 0},
			},
		}
	case "doubao":
		return []llmmodel.PricingRule{
			{
				ID:         "doubao_default",
				Priority:   0,
				Conditions: map[string]interface{}{},
				Price:      llmmodel.PricingDetail{Credits: 100, Amount: 0},
			},
		}
	case "openai":
		return []llmmodel.PricingRule{
			{
				ID:         "openai_default",
				Priority:   0,
				Conditions: map[string]interface{}{},
				Price:      llmmodel.PricingDetail{Credits: 200, Amount: 0},
			},
		}
	case "gcp":
		return []llmmodel.PricingRule{
			{
				ID:         "gcp_default",
				Priority:   0,
				Conditions: map[string]interface{}{},
				Price:      llmmodel.PricingDetail{Credits: 180, Amount: 0},
			},
		}
	case "midjourney":
		return []llmmodel.PricingRule{
			{
				ID:         "midjourney_default",
				Priority:   0,
				Conditions: map[string]interface{}{},
				Price:      llmmodel.PricingDetail{Credits: 300, Amount: 0},
			},
		}
	default:
		return []llmmodel.PricingRule{
			{
				ID:         "generic_default",
				Priority:   0,
				Conditions: map[string]interface{}{},
				Price:      llmmodel.PricingDetail{Credits: 200, Amount: 0},
			},
		}
	}
}

func canonicalImageProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "alibaba", "aliyun", "dashscope", "qwen":
		return "qwen"
	case "doubao", "volcengine":
		return "doubao"
	case "openai":
		return "openai"
	case "gcp-imagen", "gcp", "google", "vertex", "vertexai":
		return "gcp"
	case "midjourney", "mj-proxy":
		return "midjourney"
	default:
		return p
	}
}

func matchImageCondition(req *adapter.ImageRequest, conditions map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	for key, val := range conditions {
		switch key {
		case "size":
			if !matchStringOrArray(req.Size, val) {
				return false
			}
		case "quality":
			if !matchStringOrArray(req.Quality, val) {
				return false
			}
		case "style":
			if !matchStringOrArray(req.Style, val) {
				return false
			}
		default:
			return false
		}
	}

	return true
}

func matchStringOrArray(actual string, ruleValue interface{}) bool {
	if s, ok := ruleValue.(string); ok {
		return actual == s
	}
	if arr, ok := ruleValue.([]string); ok {
		for _, s := range arr {
			if actual == s {
				return true
			}
		}
		return false
	}
	if arr, ok := ruleValue.([]interface{}); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok && actual == s {
				return true
			}
		}
		return false
	}
	return false
}

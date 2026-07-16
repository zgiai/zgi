package workflowtest

import "strings"

const (
	modelUnavailableReason     = "当前评分模型不可用，请前往默认模型管理修改默认文本模型后重新测试。"
	modelUnavailableAction     = "请在默认模型管理中更换可用的默认文本模型，或在 AI 评分设置中指定其他可用模型后重新执行。"
	modelPricingMissingReason  = "当前评分模型未配置计费价格，暂时无法调用。"
	modelPricingMissingAction  = "请联系管理员配置模型价格，或更换已配置价格的评分模型后重新执行。"
	summaryModelUnavailable    = "AI 总结生成失败：当前默认模型不可用，请前往默认模型管理修改默认文本模型后重新测试。"
	summaryModelPricingMissing = "AI 总结生成失败：当前模型未配置计费价格，请联系管理员配置价格或更换模型后重新测试。"
)

func isModelPricingNotConfiguredError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "model_pricing_not_configured") ||
		strings.Contains(message, "model pricing is not configured") ||
		strings.Contains(message, "missing token pricing") ||
		strings.Contains(message, "missing image pricing")
}

func isModelUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}

	indicators := []string{
		"model not found",
		"no enabled route for model",
		"no enabled routes found",
		"no provider available",
		"model unavailable",
		"workflow test model is not configured",
		"judge model is not configured",
		"model field is required",
		"provider field is required",
	}
	for _, indicator := range indicators {
		if strings.Contains(message, indicator) {
			return true
		}
	}
	return false
}

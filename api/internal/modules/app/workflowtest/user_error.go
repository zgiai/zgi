package workflowtest

import "strings"

const (
	modelUnavailableReason  = "当前评分模型不可用，请前往默认模型管理修改默认文本模型后重新测试。"
	modelUnavailableAction  = "请在默认模型管理中更换可用的默认文本模型，或在 AI 评分设置中指定其他可用模型后重新执行。"
	summaryModelUnavailable = "AI 总结生成失败：当前默认模型不可用，请前往默认模型管理修改默认文本模型后重新测试。"
)

func isModelUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	if message == "" {
		return false
	}

	indicators := []string{
		"all providers failed",
		"current user api does not support http call",
		"upstream service error",
		"no provider available",
		"model unavailable",
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

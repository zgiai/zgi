package diagnosis

import (
	"strings"
)

// Classifier handles rule-based pre-classification of workflow node execution errors
type Classifier struct{}

// NewClassifier creates a new error Classifier
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify attempts to categorize an error message directly without an LLM call.
// It returns an ErrorType and a pre-defined diagnostic message for common known errors.
func (c *Classifier) Classify(errMsg string) (ErrorType, string, bool) {
	lowerMsg := strings.ToLower(errMsg)

	if strings.Contains(lowerMsg, "context deadline exceeded") || strings.Contains(lowerMsg, "timeout") {
		return ErrorTypeTimeout, "模型响应超时，建议稍后重试或切换模型", true
	}

	if strings.Contains(lowerMsg, "auth error") || strings.Contains(lowerMsg, "401") || strings.Contains(lowerMsg, "unauthorized") {
		return ErrorTypeAuthError, "认证失败，请检查 API Key 是否正确配置", true
	}

	if strings.Contains(lowerMsg, "model context length exceeded") || strings.Contains(lowerMsg, "model error") || strings.Contains(lowerMsg, "rate limit") {
		return ErrorTypeModelError, "模型服务暂时不可用，请稍后重试或切换其他模型", true
	}

	if strings.Contains(lowerMsg, "database error") || strings.Contains(lowerMsg, "sqlstate") || strings.Contains(lowerMsg, "connection") {
		if strings.Contains(lowerMsg, "connection refused") || strings.Contains(lowerMsg, "dial tcp") {
			return ErrorTypeDBError, "数据库连接失败，请检查数据库服务状态及网络配置", true
		}
		// 其他 SQL 语法或逻辑错误交给 LLM
		return ErrorTypeDBError, "", false
	}

	if strings.Contains(lowerMsg, "required variable") && strings.Contains(lowerMsg, "is missing") {
		return ErrorTypeVariableNull, "", false
	}

	if strings.Contains(lowerMsg, "no condition matched") || strings.Contains(lowerMsg, "flow branches") {
		return ErrorTypeConditionNoMatch, "", false
	}

	if strings.Contains(lowerMsg, "json parse") || strings.Contains(lowerMsg, "unexpected end of json") || strings.Contains(lowerMsg, "invalid character") {
		return ErrorTypeParseError, "", false
	}

	if strings.Contains(lowerMsg, "zgi-sandbox") || strings.Contains(lowerMsg, "sandbox/run") || (strings.Contains(lowerMsg, "dial tcp") && strings.Contains(lowerMsg, "lookup")) {
		return ErrorTypeUnknown, "代码执行沙箱服务暂时不可用，可能由于网络配置或 DNS 解析异常，请联系管理员", true
	}

	// 8. UNKNOWN (✔ 调用 LLM)
	return ErrorTypeUnknown, "", false
}

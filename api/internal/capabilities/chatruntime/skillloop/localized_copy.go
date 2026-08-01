package skillloop

import "unicode"

type ExternalActionFailureScope uint8

const (
	ExternalActionFailureOperation ExternalActionFailureScope = iota
	ExternalActionFailureSend
	ExternalActionFailureSendOrOperation
)

type externalActionFailureCopy struct {
	english           string
	simplifiedChinese string
}

var externalActionFailureCopyByScope = map[ExternalActionFailureScope]externalActionFailureCopy{
	ExternalActionFailureOperation: {
		english:           "The external operation did not complete. The system has no successful provider receipt, so it must not be treated as completed. Check the connection and execution log before retrying.",
		simplifiedChinese: "外部操作未完成。系统没有取得服务商的成功回执，因此不能视为已完成。请先检查连接状态和执行记录，确认后再重试。",
	},
	ExternalActionFailureSend: {
		english:           "The send did not complete. The system has no successful provider receipt, so it must not be treated as sent. Check the connection and execution log before retrying.",
		simplifiedChinese: "发送未完成。系统没有取得服务商的成功回执，因此不能视为已发送。请先检查连接状态和执行记录，确认后再重试。",
	},
	ExternalActionFailureSendOrOperation: {
		english:           "The external operation did not complete. The system has no successful provider receipt, so it must not be treated as sent or completed. Check the connection and execution log before retrying.",
		simplifiedChinese: "外部操作未完成。系统没有取得服务商的成功回执，因此不能视为已发送或已完成。请先检查连接状态和执行记录，确认后再重试。",
	},
}

func LocalizedExternalActionFailureAnswer(languageHint string, scope ExternalActionFailureScope) string {
	copy, exists := externalActionFailureCopyByScope[scope]
	if !exists {
		copy = externalActionFailureCopyByScope[ExternalActionFailureOperation]
	}
	for _, r := range languageHint {
		if unicode.Is(unicode.Han, r) {
			return copy.simplifiedChinese
		}
	}
	return copy.english
}

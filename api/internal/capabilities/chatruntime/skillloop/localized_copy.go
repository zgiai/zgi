package skillloop

import (
	"fmt"
	"unicode"
)

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

func localizedExternalActionPartialBatchAnswer(
	languageHint string,
	succeeded int,
	failedSafe int,
	outcomeUnknown int,
	executing int,
) string {
	for _, r := range languageHint {
		if unicode.Is(unicode.Han, r) {
			return fmt.Sprintf(
				"批量外部操作部分完成：已成功 %d 项，安全失败 %d 项，结果未知 %d 项，仍在执行 %d 项。未知或执行中的项目不会被视为成功，也不会被自动重复执行。",
				succeeded,
				failedSafe,
				outcomeUnknown,
				executing,
			)
		}
	}
	return fmt.Sprintf(
		"The batch external operation partially completed: %d item(s) succeeded, %d failed safely, %d have an unknown outcome, and %d are still executing. Unknown or in-progress items are not treated as successful and will not be replayed automatically.",
		succeeded,
		failedSafe,
		outcomeUnknown,
		executing,
	)
}

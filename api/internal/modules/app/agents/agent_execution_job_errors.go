package agents

import (
	"net/http"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func projectAgentExecutionToolFailure(result skillloop.RuntimeToolResult, projector *apptransport.Projector) skillloop.RuntimeToolResult {
	if projector == nil || result.ModelResult == nil {
		return result
	}
	if _, ok := apperror.As(result.Error); ok {
		// Model instructions use the stable English presentation. The product
		// catalog also supplies Chinese for user-facing protocol adapters.
		presentation := projector.Project(result.Error, appcatalog.LocaleEnglishUS).Presentation
		result.ModelResult["message"] = presentation.Message
		result.ModelResult["app_error_code"] = presentation.Code.String()
	}
	return result
}

var (
	appCodeAgentExecutionJobFailed              = apperror.MustCode("agent.execution.job.failed")
	appCodeAgentExecutionLanguageRecoveryFailed = apperror.MustCode("agent.execution.language_recovery.failed")
	appCodeAgentExecutionSystemRecoveryFailed   = apperror.MustCode("agent.execution.system_recovery.failed")
)

func agentExecutionJobFailure(workerCode string) (string, apperror.Code) {
	switch workerCode {
	case "language_package_recovery_failed":
		return workerCode, appCodeAgentExecutionLanguageRecoveryFailed
	case "package_recovery_failed":
		return workerCode, appCodeAgentExecutionSystemRecoveryFailed
	default:
		// Unknown codes, including old Workers without this field, never become
		// user-visible text or an inferred dependency installation instruction.
		return "job_failed", appCodeAgentExecutionJobFailed
	}
}

func agentExecutionJobErrorDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		{Code: appCodeAgentExecutionJobFailed, Category: appcatalog.CategoryUpstream, HTTPStatus: http.StatusBadGateway,
			Messages: map[appcatalog.Locale]string{
				appcatalog.LocaleEnglishUS:         "The Linux command failed. Check its output and retry after resolving the cause.",
				appcatalog.LocaleChineseSimplified: "Linux 命令执行失败，请检查输出并在解决原因后重试。",
			}},
		{Code: appCodeAgentExecutionLanguageRecoveryFailed, Category: appcatalog.CategoryUpstream, HTTPStatus: http.StatusServiceUnavailable,
			Messages: map[appcatalog.Locale]string{
				appcatalog.LocaleEnglishUS:         "Retained Python or Node dependency recovery failed. Verify the original runtime profile and retained artifact integrity before retrying; no network or latest-version fallback. The command was not started.",
				appcatalog.LocaleChineseSimplified: "Python 或 Node 依赖恢复失败。请检查原始环境配置和保留制品的完整性后重试；不会联网或回退到最新版本，命令尚未启动。",
			}},
		{Code: appCodeAgentExecutionSystemRecoveryFailed, Category: appcatalog.CategoryUpstream, HTTPStatus: http.StatusServiceUnavailable,
			Messages: map[appcatalog.Locale]string{
				appcatalog.LocaleEnglishUS:         "System dependency recovery failed. Verify the original runtime profile, retained ledger and locked package sources before retrying; no latest-version fallback. The command was not started.",
				appcatalog.LocaleChineseSimplified: "系统依赖恢复失败。请检查原始环境配置、保留记录和锁定版本的包源后重试；不会回退到最新版本，命令尚未启动。",
			}},
	}
}

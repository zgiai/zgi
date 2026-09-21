package core

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	AppCodeExecutionNotSubmitted       = apperror.MustCode("connector.execution.not_submitted")
	AppCodeExecutionValidationConflict = apperror.MustCode("connector.execution.validation_conflict")
	AppCodeExecutionInProgress         = apperror.MustCode("connector.execution.in_progress")
	AppCodeExecutionUnresolved         = apperror.MustCode("connector.execution.unresolved")
)

func executionPresentationDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		appDefinition(AppCodeExecutionNotSubmitted, appcatalog.CategoryConflict, 409, "This operation was not submitted. Check its connection and access requirements.", "本次操作未提交，请检查连接状态和操作权限。"),
		appDefinition(AppCodeExecutionValidationConflict, appcatalog.CategoryConflict, 409, "System validation rejected this operation before submission.", "系统校验冲突，本次操作未提交。"),
		appDefinition(AppCodeExecutionInProgress, appcatalog.CategoryConflict, 409, "An existing request is still processing. Do not submit it again.", "已有请求正在处理，请勿重复提交。"),
		appDefinition(AppCodeExecutionUnresolved, appcatalog.CategoryConflict, 409, "The operation result cannot be confirmed yet. Check the related records first.", "暂时无法确认操作结果，请先核查相关记录。"),
	}
}

// ExecutionPresentationError is selected only after execution establishes the
// disposition. A legacy conflict alone cannot imply that nothing was submitted.
func ExecutionPresentationError(disposition string, cause ErrorCode) error {
	code := AppCodeExecutionUnresolved
	switch disposition {
	case "not_submitted":
		code = AppCodeExecutionNotSubmitted
		if cause == ErrorCodeConflict {
			code = AppCodeExecutionValidationConflict
		}
	case "in_progress":
		code = AppCodeExecutionInProgress
	}
	return apperror.New(code)
}

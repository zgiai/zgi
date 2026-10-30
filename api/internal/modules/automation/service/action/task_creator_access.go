package action

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// AppCodeTaskCreatorAccessRevoked identifies a scheduled run whose task
// creator can no longer use the task's workspace, for example because an
// organization administrator disabled the creator's membership.
var AppCodeTaskCreatorAccessRevoked = apperror.MustCode("automation.task.creator_access_revoked")

// CatalogDefinitions returns the public application errors owned by
// automation action execution.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{{
		Code:       AppCodeTaskCreatorAccessRevoked,
		Category:   appcatalog.CategoryAuthorization,
		HTTPStatus: 403,
		Retryable:  false,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         "This task did not run because its creator no longer has access to the workspace.",
			appcatalog.LocaleChineseSimplified: "任务创建者已无权访问该工作空间，本次任务未执行。",
		},
	}}
}

// TaskCreatorAccessChecker reports whether the account an automation task runs
// as may still act in the task's workspace.
type TaskCreatorAccessChecker func(ctx context.Context, organizationID, workspaceID, accountID string) (bool, error)

// authorizeTaskCreator rechecks the task creator before an unattended run.
// Tasks without a creator account (system tasks) carry no user authority to
// recheck and keep running.
func authorizeTaskCreator(ctx context.Context, check TaskCreatorAccessChecker, organizationID, workspaceID, createdBy string) error {
	createdBy = strings.TrimSpace(createdBy)
	if check == nil || createdBy == "" || createdBy == uuid.Nil.String() {
		return nil
	}
	allowed, err := check(ctx, organizationID, workspaceID, createdBy)
	if err != nil {
		return err
	}
	if !allowed {
		return apperror.New(AppCodeTaskCreatorAccessRevoked, apperror.WithOperation("automation.run_workflow.authorize_creator"))
	}
	return nil
}

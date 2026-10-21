package digitalhuman

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

func TestTaskVisibilityForbiddenIsReportedAsMissingPermission(t *testing.T) {
	err := taskApplicationError(ErrTaskVisibilityForbidden, "digital_human.task.update")
	appErr, ok := apperror.As(err)
	if !ok || appErr.Code() != appcatalog.CodePermissionDenied {
		t.Fatalf("visibility rejection = %v, want %s", err, appcatalog.CodePermissionDenied)
	}
	if !errors.Is(ErrTaskVisibilityForbidden, ErrTaskPolicyRejected) {
		t.Fatal("visibility rejection must remain a task policy rejection")
	}
	other, _ := apperror.As(taskApplicationError(ErrTaskPolicyRejected, "digital_human.task.update"))
	if other == nil || other.Code() != AppCodeTaskPolicyRejected {
		t.Fatalf("plain policy rejection changed: %v", other)
	}
}

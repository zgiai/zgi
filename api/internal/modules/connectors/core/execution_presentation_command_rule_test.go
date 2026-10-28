package core

import (
	"testing"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

func TestForbiddenCommandPresentsAsTheOrganizationRule(t *testing.T) {
	if code, ok := apperror.CodeOf(ExecutionPresentationError("not_submitted", ErrorCodeCommandForbidden)); !ok || code != AppCodeExecutionCommandForbidden {
		t.Fatalf("ExecutionPresentationError() code = %v, %v", code, ok)
	}
}

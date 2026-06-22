package dto

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
)

func TestConvertSQLOperationModelToResponseIncludesGuardFields(t *testing.T) {
	verdict := "deny"
	action := "allow"
	op := &model.DataSourceSQLOperation{
		GuardVerdict: &verdict,
		GuardAction:  &action,
	}

	response := ConvertSQLOperationModelToResponse(op)
	if response.GuardVerdict == nil || *response.GuardVerdict != verdict {
		t.Fatalf("guard_verdict = %#v, want %q", response.GuardVerdict, verdict)
	}
	if response.GuardAction == nil || *response.GuardAction != action {
		t.Fatalf("guard_action = %#v, want %q", response.GuardAction, action)
	}
}

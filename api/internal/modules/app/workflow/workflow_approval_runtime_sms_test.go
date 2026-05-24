package workflow

import (
	"testing"

	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
)

func TestApprovalRequestedSubmitMethodsIncludesSMS(t *testing.T) {
	methods := approvalRequestedSubmitMethods(approvalruntime.SubmitMethods{
		SMS: approvalruntime.SMSSubmitMethod{Enabled: true},
	})

	sms, ok := methods["sms"].(map[string]interface{})
	if !ok {
		t.Fatalf("sms submit method missing: %#v", methods)
	}
	if sms["enabled"] != true {
		t.Fatalf("sms.enabled = %#v", sms["enabled"])
	}
}

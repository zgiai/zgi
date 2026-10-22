package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUsageBillNamesTheRuntimeWorkspaceEndUser(t *testing.T) {
	service := &BillingService{}
	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	bc := testUsageBillContext(now, now)
	runtimeWorkspaceID, subjectID := uuid.New(), uuid.New()
	bc.RuntimeWorkspaceID, bc.EndUserSubjectID = &runtimeWorkspaceID, &subjectID
	bill, err := service.buildUsageBill(bc, usageBillStatusSuccess, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bill.RuntimeWorkspaceID == nil || *bill.RuntimeWorkspaceID != runtimeWorkspaceID || bill.EndUserSubjectID == nil || *bill.EndUserSubjectID != subjectID {
		t.Fatalf("bill end user = %v %v", bill.RuntimeWorkspaceID, bill.EndUserSubjectID)
	}

	plain, err := service.buildUsageBill(testUsageBillContext(now, now), usageBillStatusSuccess, nil, nil)
	if err != nil || plain.RuntimeWorkspaceID != nil || plain.EndUserSubjectID != nil {
		t.Fatalf("a call without an end user was labelled: %+v %v", plain, err)
	}
}

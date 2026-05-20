package draftgen

import (
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
)

func TestBuildAppContextUsesWorkspaceForDraftAttribution(t *testing.T) {
	req := GenerateRequest{
		OrganizationID: "7a4f327d-3b4d-4d06-902f-860a75fb79f5",
		WorkspaceID:    "2308df2f-ad68-42a4-b208-b0ce5cdc85f5",
		AccountID:      "4628dc58-5844-41b9-96f3-ad5a875044a7",
	}

	appCtx := buildAppContext(req)

	if appCtx.AppID != req.WorkspaceID {
		t.Fatalf("expected workspace id as app id, got %q", appCtx.AppID)
	}
	if appCtx.AppType != taskDraftAppType {
		t.Fatalf("unexpected app type: %q", appCtx.AppType)
	}
	if appCtx.BillingSubjectType != llmclient.BillingSubjectTypeWorkspace {
		t.Fatalf("unexpected billing subject: %q", appCtx.BillingSubjectType)
	}
	if appCtx.WorkspaceID != req.WorkspaceID {
		t.Fatalf("unexpected workspace id: %q", appCtx.WorkspaceID)
	}
}

func TestBuildAppContextFallsBackToOrganizationWhenWorkspaceMissing(t *testing.T) {
	req := GenerateRequest{
		OrganizationID: "7a4f327d-3b4d-4d06-902f-860a75fb79f5",
		AccountID:      "4628dc58-5844-41b9-96f3-ad5a875044a7",
	}

	appCtx := buildAppContext(req)

	if appCtx.AppID != req.OrganizationID {
		t.Fatalf("expected organization id as fallback app id, got %q", appCtx.AppID)
	}
	if appCtx.BillingSubjectType != llmclient.BillingSubjectTypeOrganization {
		t.Fatalf("unexpected billing subject: %q", appCtx.BillingSubjectType)
	}
	if appCtx.WorkspaceID != "" {
		t.Fatalf("expected empty workspace id, got %q", appCtx.WorkspaceID)
	}
}

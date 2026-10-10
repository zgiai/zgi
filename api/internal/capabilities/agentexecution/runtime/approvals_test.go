package executionruntime

import (
	"errors"
	"testing"
)

func TestSharedApprovalsUseOnlyOperatorDefaults(t *testing.T) {
	for _, backend := range []string{"local-gvisor", "e2b"} {
		t.Run(backend, func(t *testing.T) {
			approvals := Approvals{Enabled: true, GatewayReady: true, DefaultProfileID: "standard", DefaultPolicyID: "offline", DefaultHomeSnapshotID: "tools",
				Profiles:      map[string]ProfileApproval{"standard": {Backend: backend, OrganizationIDs: []string{"org"}, PolicyIDs: []string{"offline", "package-install"}, DependencyInstallation: true, PackageNetworkMode: "package-install"}},
				HomeSnapshots: map[string]HomeApproval{"tools": {Backend: backend, ProfileID: "standard", OrganizationIDs: []string{"org"}}},
			}
			resolved, err := approvals.Resolve("org", true)
			if err != nil || resolved == nil || resolved.ProfileID != "standard" || resolved.PolicyID != "package-install" || resolved.HomeSnapshotID == nil || *resolved.HomeSnapshotID != "tools" {
				t.Fatalf("operator resolution=%+v err=%v", resolved, err)
			}
			*resolved.HomeSnapshotID = "changed-by-caller"
			if approvals.DefaultHomeSnapshotID != "tools" {
				t.Fatal("resolved snapshot aliases operator state")
			}
			if _, err := approvals.Resolve("foreign-org", true); err == nil {
				t.Fatal("cross-tenant runtime approved")
			}
			home := approvals.HomeSnapshots["tools"]
			home.ProfileID = "other-profile"
			approvals.HomeSnapshots["tools"] = home
			_, err = approvals.Resolve("org", true)
			var denied *ApprovalError
			if !errors.As(err, &denied) || denied.Operation != "agent_execution.home_snapshot_unavailable" {
				t.Fatalf("snapshot mismatch error=%v", err)
			}
		})
	}
	if resolved, err := (Approvals{}).Resolve("org", false); err != nil || resolved != nil {
		t.Fatalf("disabled application=%+v err=%v", resolved, err)
	}
}

func TestSharedApprovalsFromEnvFailClosed(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "true")
	t.Setenv("ZGI_AGENT_RUNTIME_URL", "https://worker.example.invalid")
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "test-key")
	t.Setenv("ZGI_AGENT_RUNTIME_DEFAULT_PROFILE_ID", "")
	t.Setenv("ZGI_AGENT_RUNTIME_DEFAULT_POLICY_ID", "")
	t.Setenv("ZGI_AGENT_RUNTIME_DEFAULT_HOME_SNAPSHOT_ID", "")
	t.Setenv("ZGI_AGENT_RUNTIME_APPROVED_HOME_SNAPSHOTS_JSON", "")
	t.Setenv("ZGI_AGENT_RUNTIME_ORG_MODE", "")
	for _, raw := range []string{"", "not-json", "null", `{}`, `{"standard":{"backend":"unknown","organization_ids":["org"],"policy_ids":["offline"]}}`} {
		t.Setenv("ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON", raw)
		if _, err := ApprovalsFromEnv().Resolve("org", true); err == nil {
			t.Fatalf("accepted unapproved deployment=%q", raw)
		}
	}
	t.Setenv("ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON", `{"standard":{"backend":"e2b","organization_ids":["org"],"policy_ids":["offline"]}}`)
	if _, err := ApprovalsFromEnv().Resolve("org", true); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_AGENT_RUNTIME_ORG_MODE", "all_authenticated")
	if _, err := ApprovalsFromEnv().Resolve("another-org", true); err != nil {
		t.Fatalf("explicit all-authenticated mode denied an authenticated organization: %v", err)
	}
	if _, err := ApprovalsFromEnv().Resolve("", true); err == nil {
		t.Fatal("explicit all-authenticated mode admitted an empty organization")
	}
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "false")
	if _, err := ApprovalsFromEnv().Resolve("org", true); err == nil {
		t.Fatal("application opt-in bypassed deployment switch")
	}
}

func TestSharedApprovalsAllowAllAuthenticatedOrganizationsWhenExplicitlyConfigured(t *testing.T) {
	approvals := Approvals{
		Enabled:                true,
		GatewayReady:           true,
		DefaultProfileID:       "standard",
		OrganizationAccessMode: OrganizationAccessModeAllAuthenticated,
		Profiles: map[string]ProfileApproval{
			"standard": {Backend: "e2b", PolicyIDs: []string{"offline"}},
		},
	}
	for _, organizationID := range []string{"org-a", "org-b"} {
		if _, err := approvals.Resolve(organizationID, true); err != nil {
			t.Fatalf("organization %q was not admitted in all-authenticated mode: %v", organizationID, err)
		}
	}
	if _, err := approvals.Resolve("", true); err == nil {
		t.Fatal("empty organization was admitted in all-authenticated mode")
	}
}

func TestSharedApprovalsUnknownOrganizationAccessModeFallsBackToAllowlist(t *testing.T) {
	approvals := Approvals{
		Enabled:                true,
		GatewayReady:           true,
		DefaultProfileID:       "standard",
		OrganizationAccessMode: "unexpected",
		Profiles: map[string]ProfileApproval{
			"standard": {Backend: "e2b", OrganizationIDs: []string{"org-a"}, PolicyIDs: []string{"offline"}},
		},
	}
	if _, err := approvals.Resolve("org-b", true); err == nil {
		t.Fatal("unknown organization access mode broadened the allowlist")
	}
	if _, err := approvals.Resolve("org-a", true); err != nil {
		t.Fatalf("allowlisted organization was denied after mode fallback: %v", err)
	}
}

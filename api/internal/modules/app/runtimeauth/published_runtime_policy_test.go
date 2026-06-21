package runtimeauth

import (
	"testing"

	"github.com/google/uuid"
)

func TestPublishedRuntimePolicySeparatesExternalSurfacesFromInternalInvocation(t *testing.T) {
	policy := PolicyFromAgentFields(WebAppStatusInactive, false)

	if policy.Allows(PublishedRuntimeSurfaceWebApp) {
		t.Fatalf("webapp surface should be disabled when web app status is inactive")
	}
	if policy.Allows(PublishedRuntimeSurfaceAPI) {
		t.Fatalf("api surface should be disabled when enable_api is false")
	}
	if policy.Allows(PublishedRuntimeSurfaceBuiltinApp) {
		t.Fatalf("builtin app surface should be disabled when the web app is inactive")
	}
	if !policy.Allows(PublishedRuntimeSurfaceInternal) {
		t.Fatalf("internal invocation should remain enabled for published runtime compatibility")
	}
	if policy.Allows(PublishedRuntimeSurface("unknown")) {
		t.Fatalf("unknown runtime surface should fail closed")
	}
}

func TestPublishedRuntimePolicyNormalizesLegacyWebAppStatus(t *testing.T) {
	policy := PolicyFromAgentFields("", true)

	if !policy.Allows(PublishedRuntimeSurfaceWebApp) {
		t.Fatalf("empty legacy web app status should normalize to active")
	}
	if !policy.Allows(PublishedRuntimeSurfaceAPI) {
		t.Fatalf("api surface should follow enable_api")
	}
	if !policy.Allows(PublishedRuntimeSurfaceBuiltinApp) {
		t.Fatalf("builtin app surface should preserve active webapp compatibility")
	}
}

func TestPublishedRuntimeSurfaceAudienceGrants(t *testing.T) {
	organizationID := uuid.New()
	otherOrganizationID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	departmentID := uuid.New()
	otherDepartmentID := uuid.New()

	tests := []struct {
		name     string
		surface  SurfaceAuthorization
		audience RuntimeAudience
		want     bool
	}{
		{
			name: "disabled surface denies matching public grant",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: false,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectPublic,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID},
			want:     false,
		},
		{
			name: "enabled surface without grants preserves legacy open compatibility",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceWebApp,
				Enabled: true,
			},
			audience: RuntimeAudience{},
			want:     true,
		},
		{
			name: "disabled grants do not allow the surface",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectPublic,
					Enabled:     false,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID},
			want:     false,
		},
		{
			name: "public grant allows any audience",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectPublic,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{},
			want:     true,
		},
		{
			name: "organization grant requires matching organization",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectOrganization,
					SubjectID:   &organizationID,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID},
			want:     true,
		},
		{
			name: "organization grant denies other organization",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectOrganization,
					SubjectID:   &organizationID,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: otherOrganizationID, AccountID: accountID},
			want:     false,
		},
		{
			name: "account grant allows matching account",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectAccount,
					SubjectID:   &accountID,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID},
			want:     true,
		},
		{
			name: "account grant denies other account",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectAccount,
					SubjectID:   &accountID,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: otherAccountID},
			want:     false,
		},
		{
			name: "department grant allows matching department",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectDepartment,
					SubjectID:   &departmentID,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID, DepartmentIDs: []uuid.UUID{otherDepartmentID, departmentID}},
			want:     true,
		},
		{
			name: "department grant denies missing department",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectDepartment,
					SubjectID:   &departmentID,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID, DepartmentIDs: []uuid.UUID{otherDepartmentID}},
			want:     false,
		},
		{
			name: "internal grant allows internal audience",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceInternal,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectInternal,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{Internal: true},
			want:     true,
		},
		{
			name: "internal grant denies external audience",
			surface: SurfaceAuthorization{
				Surface: PublishedRuntimeSurfaceInternal,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectInternal,
					Enabled:     true,
				}},
			},
			audience: RuntimeAudience{OrganizationID: organizationID, AccountID: accountID},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.surface.Allows(tt.audience); got != tt.want {
				t.Fatalf("Allows() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPublishedRuntimeSurfaceAudienceEvaluationReasons(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	departmentID := uuid.New()

	tests := []struct {
		name        string
		auth        *ResourceAuthorization
		surface     PublishedRuntimeSurface
		audience    RuntimeAudience
		wantAllowed bool
		wantReason  RuntimeAccessDecisionReason
		wantSubject PublishedRuntimeSubjectType
	}{
		{
			name:        "missing surface",
			auth:        &ResourceAuthorization{},
			surface:     PublishedRuntimeSurfaceWebApp,
			wantReason:  RuntimeAccessDeniedMissingSurface,
			wantAllowed: false,
		},
		{
			name: "disabled surface",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceWebApp,
				Enabled: false,
			}}},
			surface:     PublishedRuntimeSurfaceWebApp,
			wantReason:  RuntimeAccessDeniedDisabledSurface,
			wantAllowed: false,
		},
		{
			name: "open surface without grants",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceWebApp,
				Enabled: true,
			}}},
			surface:     PublishedRuntimeSurfaceWebApp,
			wantReason:  RuntimeAccessAllowedOpenSurface,
			wantAllowed: true,
		},
		{
			name: "public grant",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceWebApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectPublic,
					Enabled:     true,
				}},
			}}},
			surface:     PublishedRuntimeSurfaceWebApp,
			wantReason:  RuntimeAccessAllowedPublicGrant,
			wantAllowed: true,
			wantSubject: PublishedRuntimeSubjectPublic,
		},
		{
			name: "account grant",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectAccount,
					SubjectID:   &accountID,
					Enabled:     true,
				}},
			}}},
			surface:     PublishedRuntimeSurfaceBuiltinApp,
			audience:    RuntimeAudience{AccountID: accountID},
			wantReason:  RuntimeAccessAllowedAccountGrant,
			wantAllowed: true,
			wantSubject: PublishedRuntimeSubjectAccount,
		},
		{
			name: "department grant",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectDepartment,
					SubjectID:   &departmentID,
					Enabled:     true,
				}},
			}}},
			surface:     PublishedRuntimeSurfaceBuiltinApp,
			audience:    RuntimeAudience{DepartmentIDs: []uuid.UUID{departmentID}},
			wantReason:  RuntimeAccessAllowedDepartmentGrant,
			wantAllowed: true,
			wantSubject: PublishedRuntimeSubjectDepartment,
		},
		{
			name: "internal grant",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceInternal,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectInternal,
					Enabled:     true,
				}},
			}}},
			surface:     PublishedRuntimeSurfaceInternal,
			audience:    RuntimeAudience{Internal: true},
			wantReason:  RuntimeAccessAllowedInternalGrant,
			wantAllowed: true,
			wantSubject: PublishedRuntimeSubjectInternal,
		},
		{
			name: "no matching grant",
			auth: &ResourceAuthorization{Surfaces: []SurfaceAuthorization{{
				Surface: PublishedRuntimeSurfaceBuiltinApp,
				Enabled: true,
				Grants: []SurfaceGrant{{
					SubjectType: PublishedRuntimeSubjectAccount,
					SubjectID:   &accountID,
					Enabled:     true,
				}},
			}}},
			surface:     PublishedRuntimeSurfaceBuiltinApp,
			audience:    RuntimeAudience{OrganizationID: organizationID, AccountID: otherAccountID},
			wantReason:  RuntimeAccessDeniedNoMatchingGrant,
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.auth.Evaluate(tt.surface, tt.audience)
			if got.Allowed != tt.wantAllowed {
				t.Fatalf("Evaluate().Allowed = %v, want %v", got.Allowed, tt.wantAllowed)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("Evaluate().Reason = %q, want %q", got.Reason, tt.wantReason)
			}
			if got.MatchedSubjectType != tt.wantSubject {
				t.Fatalf("Evaluate().MatchedSubjectType = %q, want %q", got.MatchedSubjectType, tt.wantSubject)
			}
			if allows := tt.auth.Allows(tt.surface, tt.audience); allows != tt.wantAllowed {
				t.Fatalf("Allows() = %v, want %v", allows, tt.wantAllowed)
			}
		})
	}
}

func TestPublishedRuntimeResourceAuthorizationAudienceHelpers(t *testing.T) {
	accountID := uuid.New()
	departmentID := uuid.New()
	auth := &ResourceAuthorization{
		ResourceType: PublishedRuntimeResourceAgent,
		ResourceID:   uuid.New(),
		Surfaces: []SurfaceAuthorization{{
			Surface: PublishedRuntimeSurfaceBuiltinApp,
			Enabled: true,
			Grants: []SurfaceGrant{
				{SubjectType: PublishedRuntimeSubjectAccount, SubjectID: &accountID, Enabled: true},
				{SubjectType: PublishedRuntimeSubjectDepartment, SubjectID: &departmentID, Enabled: false},
			},
		}},
	}

	if !auth.Allows(PublishedRuntimeSurfaceBuiltinApp, RuntimeAudience{AccountID: accountID}) {
		t.Fatalf("resource authorization should allow matching account grant")
	}
	if auth.Allows(PublishedRuntimeSurfaceAPI, RuntimeAudience{AccountID: accountID}) {
		t.Fatalf("missing surface should fail closed")
	}
	if !auth.HasGrantType(PublishedRuntimeSurfaceBuiltinApp, PublishedRuntimeSubjectAccount) {
		t.Fatalf("enabled account grant type should be detected")
	}
	if auth.HasGrantType(PublishedRuntimeSurfaceBuiltinApp, PublishedRuntimeSubjectDepartment) {
		t.Fatalf("disabled department grant type should not be detected")
	}
}

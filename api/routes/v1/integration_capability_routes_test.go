package v1

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestIntegrationActionCapabilityAvailability(t *testing.T) {
	action := integrations.ActionDefinition{
		ID:                     "records.list",
		Effect:                 toolgovernance.EffectRead,
		DataEgress:             true,
		RequiredScopes:         []string{"records:read"},
		RequiredAnyScopes:      []string{"records:metadata", "records:history"},
		PreferredScopes:        []string{"records:metadata"},
		SupportedAuthMethodIDs: []string{"oauth"},
	}
	allowed := integrations.ActionPolicyDecision{
		Enabled:           true,
		DataEgressAllowed: true,
	}
	tests := []struct {
		name        string
		decision    integrations.ActionPolicyDecision
		connections []integrations.ConnectionView
		authorize   func(integrations.ConnectionView) error
		want        integrationCapabilityAvailability
		wantCount   int
		wantError   bool
	}{
		{
			name:     "policy disabled",
			decision: integrations.ActionPolicyDecision{Enabled: false, DataEgressAllowed: true},
			want:     integrationCapabilityDisabledByPolicy,
		},
		{
			name:     "data egress blocked",
			decision: integrations.ActionPolicyDecision{Enabled: true, DataEgressAllowed: false},
			want:     integrationCapabilityDataEgressBlocked,
		},
		{
			name:     "connection required",
			decision: allowed,
			want:     integrationCapabilityNeedsConnection,
		},
		{
			name:     "scope upgrade required",
			decision: allowed,
			connections: []integrations.ConnectionView{{
				Status:        integrations.ConnectionStatusActive,
				AuthStatus:    integrations.ConnectionAuthValid,
				AuthType:      integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID:  "oauth",
				GrantedScopes: []string{"profile:read"},
			}},
			want: integrationCapabilityNeedsScope,
		},
		{
			name:     "unhealthy connection is not reported as a permission gap",
			decision: allowed,
			connections: []integrations.ConnectionView{{
				Status:        integrations.ConnectionStatusActive,
				HealthStatus:  integrations.ConnectionHealthUnhealthy,
				AuthStatus:    integrations.ConnectionAuthValid,
				AuthType:      integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID:  "oauth",
				GrantedScopes: []string{"records:read", "records:history"},
			}},
			authorize: func(integrations.ConnectionView) error {
				t.Fatal("unhealthy connection reached action authorization")
				return nil
			},
			want: integrationCapabilityNeedsConnection,
		},
		{
			name:     "available",
			decision: allowed,
			connections: []integrations.ConnectionView{{
				Status:        integrations.ConnectionStatusActive,
				AuthStatus:    integrations.ConnectionAuthValid,
				AuthType:      integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID:  "oauth",
				GrantedScopes: []string{"records:read", "records:history"},
			}},
			want:      integrationCapabilityAvailable,
			wantCount: 1,
		},
		{
			name:     "visible connection without action permission",
			decision: allowed,
			connections: []integrations.ConnectionView{{
				Status:        integrations.ConnectionStatusActive,
				AuthStatus:    integrations.ConnectionAuthValid,
				AuthType:      integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID:  "oauth",
				GrantedScopes: []string{"records:read", "records:history"},
			}},
			authorize: func(integrations.ConnectionView) error {
				return integrations.NewError(integrations.ErrorCodeAccessDenied, "action grant is missing", nil)
			},
			want: integrationCapabilityNeedsPermission,
		},
		{
			name:     "only authorized connections count as compatible",
			decision: allowed,
			connections: []integrations.ConnectionView{
				{
					ID:     uuid.MustParse("10000000-0000-0000-0000-000000000001"),
					Status: integrations.ConnectionStatusActive, AuthStatus: integrations.ConnectionAuthValid,
					AuthType: integrations.ConnectionAuthTypeOAuth2, AuthMethodID: "oauth",
					GrantedScopes: []string{"records:read", "records:history"},
				},
				{
					ID:     uuid.MustParse("10000000-0000-0000-0000-000000000002"),
					Status: integrations.ConnectionStatusActive, AuthStatus: integrations.ConnectionAuthValid,
					AuthType: integrations.ConnectionAuthTypeOAuth2, AuthMethodID: "oauth",
					GrantedScopes: []string{"records:read", "records:history"},
				},
			},
			authorize: func(connection integrations.ConnectionView) error {
				if connection.ID == uuid.MustParse("10000000-0000-0000-0000-000000000001") {
					return nil
				}
				return integrations.NewError(integrations.ErrorCodeAccessDenied, "action grant is missing", nil)
			},
			want: integrationCapabilityAvailable, wantCount: 1,
		},
		{
			name:     "authorization service failure is propagated",
			decision: allowed,
			connections: []integrations.ConnectionView{{
				Status:        integrations.ConnectionStatusActive,
				AuthStatus:    integrations.ConnectionAuthValid,
				AuthType:      integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID:  "oauth",
				GrantedScopes: []string{"records:read", "records:history"},
			}},
			authorize: func(integrations.ConnectionView) error {
				return errors.New("grant repository unavailable")
			},
			wantError: true,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, count, err := integrationActionCapabilityAvailability(
				action,
				testCase.decision,
				testCase.connections,
				testCase.authorize,
			)
			if testCase.wantError {
				if err == nil {
					t.Fatal("availability error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("availability error = %v", err)
			}
			if got != testCase.want || count != testCase.wantCount {
				t.Fatalf("availability = %q, count = %d; want %q, %d", got, count, testCase.want, testCase.wantCount)
			}
		})
	}
}

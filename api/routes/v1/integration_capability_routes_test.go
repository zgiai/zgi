package v1

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestIntegrationActionCapabilityAvailability(t *testing.T) {
	action := integrations.ActionDefinition{
		ID:                     "records.list",
		Effect:                 toolgovernance.EffectRead,
		DataEgress:             true,
		RequiredScopes:         []string{"records:read"},
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
		want        integrationCapabilityAvailability
		wantCount   int
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
			name:     "available",
			decision: allowed,
			connections: []integrations.ConnectionView{{
				Status:        integrations.ConnectionStatusActive,
				AuthStatus:    integrations.ConnectionAuthValid,
				AuthType:      integrations.ConnectionAuthTypeOAuth2,
				AuthMethodID:  "oauth",
				GrantedScopes: []string{"records:read"},
			}},
			want:      integrationCapabilityAvailable,
			wantCount: 1,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got, count := integrationActionCapabilityAvailability(
				action,
				testCase.decision,
				testCase.connections,
			)
			if got != testCase.want || count != testCase.wantCount {
				t.Fatalf("availability = %q, count = %d; want %q, %d", got, count, testCase.want, testCase.wantCount)
			}
		})
	}
}

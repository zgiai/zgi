package metatools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestActionProjectionUsesPreferredAuthorizedExecutableAction(t *testing.T) {
	fixture := newMetaToolFixture(t)
	fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.preferenceAllowed[fixture.connectionTwo.ID] = true
	fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true

	service, err := NewActionProjectionService(fixture.registry, fixture.lookup, fixture.access, fixture.policies)
	if err != nil {
		t.Fatalf("NewActionProjectionService() error = %v", err)
	}
	projections, err := service.ProjectActions(context.Background(), ActionProjectionRequest{
		ExecutionContext: skills.ExecutionContext{
			OrganizationID: fixture.organizationID.String(),
			UserID:         fixture.accountID.String(),
			InvokeFrom:     tools.ToolInvokeFromAIChat,
			RuntimeParameters: map[string]interface{}{
				"integration_connection_ids": map[string]string{
					fixture.integrationID: fixture.connectionOne.ID.String(),
				},
				"integration_selected_connection_ids": map[string][]string{
					fixture.integrationID: {fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()},
				},
			},
		},
		Query: "create github issue",
	})
	if err != nil {
		t.Fatalf("ProjectActions() error = %v", err)
	}
	if len(projections) != 1 {
		t.Fatalf("ProjectActions() = %#v, want one authorized Action", projections)
	}
	action, ok := fixture.registry.ActionDetail(fixture.integrationID, fixture.actionID)
	if !ok {
		t.Fatal("registered Action is unavailable")
	}
	projection := projections[0]
	if projection.IntegrationID != fixture.integrationID || projection.ActionID != fixture.actionID ||
		projection.ToolName != action.ToolName || projection.SchemaHash != action.SchemaHash ||
		projection.SchemaRevision != action.SchemaRevision || projection.CatalogRevision != action.CatalogRevision ||
		!projection.RequiresApproval {
		t.Fatalf("projection identity/governance = %#v", projection)
	}
	if err := tools.ValidateJSONSchema(projection.InputSchema); err != nil {
		t.Fatalf("projection input schema = %#v: %v", projection.InputSchema, err)
	}
	encoded, marshalErr := json.Marshal(projection)
	if marshalErr != nil {
		t.Fatalf("json.Marshal(projection) error = %v", marshalErr)
	}
	for _, forbidden := range []string{fixture.connectionOne.ID.String(), fixture.connectionTwo.ID.String()} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("projection leaked connection UUID %q: %s", forbidden, encoded)
		}
	}
}

func TestActionProjectionOmitsUnauthorizedOrPolicyBlockedActions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*metaToolFixture)
	}{
		{
			name: "action ACL",
			setup: func(fixture *metaToolFixture) {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
			},
		},
		{
			name: "disabled policy",
			setup: func(fixture *metaToolFixture) {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
				fixture.policies.decisions[fixture.integrationID+"/"+fixture.actionID] = integrations.ActionPolicyDecision{
					Enabled: false, DataEgressAllowed: true,
				}
			},
		},
		{
			name: "data egress policy",
			setup: func(fixture *metaToolFixture) {
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
				fixture.policies.decisions[fixture.integrationID+"/"+fixture.actionID] = integrations.ActionPolicyDecision{
					Enabled: true, DataEgressAllowed: false,
				}
			},
		},
		{
			name: "scope gap",
			setup: func(fixture *metaToolFixture) {
				fixture.connectionOne.AuthType = integrations.ConnectionAuthTypeOAuth2
				fixture.connectionOne.GrantedScopes = []string{"profile:read"}
				fixture.access.preferenceAllowed[fixture.connectionOne.ID] = true
				fixture.access.actionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newMetaToolFixture(t)
			tt.setup(fixture)
			service, err := NewActionProjectionService(fixture.registry, fixture.lookup, fixture.access, fixture.policies)
			if err != nil {
				t.Fatalf("NewActionProjectionService() error = %v", err)
			}
			projections, err := service.ProjectActions(context.Background(), ActionProjectionRequest{
				ExecutionContext: skills.ExecutionContext{
					OrganizationID: fixture.organizationID.String(),
					UserID:         fixture.accountID.String(),
					InvokeFrom:     tools.ToolInvokeFromAIChat,
					RuntimeParameters: map[string]interface{}{
						"integration_connection_ids": map[string]string{
							fixture.integrationID: fixture.connectionOne.ID.String(),
						},
						"integration_selected_connection_ids": map[string][]string{
							fixture.integrationID: {fixture.connectionOne.ID.String()},
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("ProjectActions() error = %v", err)
			}
			if len(projections) != 0 {
				t.Fatalf("ProjectActions() = %#v, want blocked Action omitted", projections)
			}
		})
	}
}

func TestActionProjectionScoreUsesLocalizedChineseIntent(t *testing.T) {
	wecom := integrations.ProviderDefinition{
		ID: "wecom", Name: "WeCom",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "企业微信"},
	}
	send := integrations.ActionDefinition{
		ID: "wecom.message.send", ToolName: "wecom_send_message", Name: "Send message",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "发送消息"},
	}
	unrelated := integrations.ProviderDefinition{ID: "github", Name: "GitHub"}
	listIssues := integrations.ActionDefinition{ID: "github.issue.list", ToolName: "github_list_issues", Name: "List issues"}
	query := "帮我在企业微信里发送一条消息"
	if got, other := actionProjectionScore(query, wecom, send), actionProjectionScore(query, unrelated, listIssues); got <= other || got == 0 {
		t.Fatalf("localized score = %d, unrelated = %d", got, other)
	}
}

func TestActionProjectionForAgentRequiresSharedReadBindingAndVerifier(t *testing.T) {
	fixture := newAgentMetaToolFixture(t, toolgovernance.EffectRead, toolgovernance.ApprovalPolicyNeverAsk, true)
	fixture.access.agentPreferenceAllowed[fixture.connectionOne.ID] = true
	fixture.access.agentActionAllowed[fixture.connectionOne.ID.String()+"/"+fixture.actionID] = true
	agentID := uuid.New()
	parameters := map[string]interface{}{
		"agent_id": agentID.String(),
		"integration_connection_ids": map[string]string{
			fixture.integrationID: fixture.connectionOne.ID.String(),
		},
		"integration_selected_connection_ids": map[string][]string{
			fixture.integrationID: {fixture.connectionOne.ID.String()},
		},
		tools.AgentBindingAuthorizationsParameter: []tools.AgentBindingAuthorization{{
			BindingType: "integration_connection", ResourceID: fixture.connectionOne.ID.String(),
			ParentResourceID: fixture.integrationID, AccessMode: "read",
			AllowedActionIDs: []string{fixture.actionID}, BoundByAccountID: fixture.accountID.String(), BoundAtUnix: 123,
		}},
	}
	parameters = skills.WithAgentBindingVerifier(parameters, func(_ context.Context, check skills.AgentBindingCheck) (bool, error) {
		return check.BindingType == "integration_connection" && check.ResourceID == fixture.connectionOne.ID.String() &&
			check.ParentResourceID == fixture.integrationID && check.AccessMode == "read" && check.ActionID == fixture.actionID, nil
	})
	service, err := NewActionProjectionService(fixture.registry, fixture.lookup, fixture.access, fixture.policies)
	if err != nil {
		t.Fatalf("NewActionProjectionService() error = %v", err)
	}
	request := ActionProjectionRequest{ExecutionContext: skills.ExecutionContext{
		OrganizationID: fixture.organizationID.String(), UserID: fixture.accountID.String(),
		InvokeFrom: tools.ToolInvokeFromAgent, RuntimeParameters: parameters,
	}}
	projections, err := service.ProjectActions(context.Background(), request)
	if err != nil {
		t.Fatalf("ProjectActions() error = %v", err)
	}
	if len(projections) != 1 || projections[0].ActionID != fixture.actionID {
		t.Fatalf("authorized Agent projections = %#v", projections)
	}

	request.ExecutionContext.RuntimeParameters = skills.WithAgentBindingVerifier(parameters, func(context.Context, skills.AgentBindingCheck) (bool, error) {
		return false, nil
	})
	projections, err = service.ProjectActions(context.Background(), request)
	if err == nil || len(projections) != 0 {
		t.Fatalf("Agent projection bypassed binding verifier: projections=%#v error=%v", projections, err)
	}
}

package metatools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
	enableFixtureSuccessGuard(t, fixture, "title")

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
		projection.Effect != string(action.Effect) || !projection.IntentMatched || !projection.RequiresApproval {
		t.Fatalf("projection identity/governance = %#v", projection)
	}
	if !reflect.DeepEqual(projection.TargetArgumentPaths, []string{"title"}) {
		t.Fatalf("projection target argument paths = %#v, want success-dedup target", projection.TargetArgumentPaths)
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
	if strings.Contains(string(encoded), "projection_priority") || strings.Contains(string(encoded), "ProjectionPriority") {
		t.Fatalf("projection leaked server-only selection priority: %s", encoded)
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

func TestPrepareActionProjectionQueryBoundsLongChineseInputDeterministically(t *testing.T) {
	query := strings.Repeat("企业微信发送消息给成员并创建日历事件", 2000)
	first := prepareActionProjectionQuery(query)
	second := prepareActionProjectionQuery(query)
	if got := len([]rune(first.normalized)); got != maxActionProjectionQueryRunes {
		t.Fatalf("normalized query runes = %d, want %d", got, maxActionProjectionQueryRunes)
	}
	if len(first.tokens) == 0 || len(first.tokens) > maxActionProjectionQueryTokens {
		t.Fatalf("query tokens = %d, want 1..%d", len(first.tokens), maxActionProjectionQueryTokens)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("prepared query is unstable: first=%#v second=%#v", first, second)
	}
}

func TestActionProjectionIntentMatchRequiresSpecificExternalCapabilityEvidence(t *testing.T) {
	calendar := integrations.ProviderDefinition{
		ID: "calendar", Name: "Calendar",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "日历"},
	}
	createEvent := integrations.ActionDefinition{
		Name: "Create event", Description: "Create a calendar event.",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "创建日历事件"},
	}
	if actionProjectionIntentMatched(prepareActionProjectionQuery("创建本地报告文件"), calendar, createEvent) {
		t.Fatal("generic internal file creation was classified as external calendar intent")
	}
	if !actionProjectionIntentMatched(prepareActionProjectionQuery("在日历里创建事件"), calendar, createEvent) {
		t.Fatal("provider-specific calendar creation was not classified as external intent")
	}
	message := integrations.ActionDefinition{
		Name: "Send message", Description: "Send a message to a selected member.",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "发送消息"},
	}
	if !actionProjectionIntentMatched(prepareActionProjectionQuery("给 Alice 发送消息"), integrations.ProviderDefinition{Name: "WeCom"}, message) {
		t.Fatal("action-specific send request without provider name was not classified as external intent")
	}
}

func TestActionProjectionProviderIntentTokensScopeExplicitProviders(t *testing.T) {
	query := prepareActionProjectionQuery("在企业微信和飞书分别发送消息")
	wecom := integrations.ProviderDefinition{
		ID: "wecom", Name: "WeCom",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "企业微信"},
	}
	feishu := integrations.ProviderDefinition{
		ID: "feishu", Name: "Feishu",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "飞书"},
	}
	dingtalk := integrations.ProviderDefinition{
		ID: "dingtalk", Name: "DingTalk",
		NameI18n: integrations.LocalizedText{integrations.LocaleSimplifiedChinese: "钉钉"},
	}
	if len(actionProjectionProviderIntentTokens(query, wecom)) == 0 || len(actionProjectionProviderIntentTokens(query, feishu)) == 0 {
		t.Fatal("explicit WeCom and Feishu provider evidence was not detected")
	}
	if got := actionProjectionProviderIntentTokens(query, dingtalk); len(got) != 0 {
		t.Fatalf("unmentioned DingTalk provider evidence = %#v, want none", got)
	}
	if got := actionProjectionProviderIntentTokens(prepareActionProjectionQuery("给 Alice 发送消息"), wecom); len(got) != 0 {
		t.Fatalf("provider-free request gained scoped provider evidence: %#v", got)
	}
}

func TestDirectActionProjectionDescriptionNeverNamesUnprojectedPreparationTool(t *testing.T) {
	description := directActionProjectionDescription(
		integrations.ProviderDefinition{Name: "WeCom"},
		integrations.ActionDefinition{Name: "Send message", Description: "Send a message."},
		nil,
		[]string{"raw_contact_lookup_that_may_be_omitted_or_aliased"},
		true,
	)
	if strings.Contains(description, "raw_contact_lookup_that_may_be_omitted_or_aliased") {
		t.Fatalf("description references an unstable preparation alias: %q", description)
	}
	for _, want := range []string{"available visible read Action", "visible external-action guide or search fallback"} {
		if !strings.Contains(description, want) {
			t.Fatalf("description = %q, missing safe fallback %q", description, want)
		}
	}
	for _, forbidden := range []string{"get_action_guide", "search_actions"} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("description references a raw fallback function %q: %q", forbidden, description)
		}
	}
}

func TestProjectionPreparationActionIDsAreStableServerActionIdentities(t *testing.T) {
	hints := []interface{}{
		map[string]interface{}{"action_id": "WeCom.Contact.Search"},
		map[string]interface{}{"action_id": "wecom.contact.search"},
		map[string]interface{}{"action_id": ""},
		map[string]interface{}{"action_id": "wecom.department.list"},
	}
	if got := projectionPreparationActionIDs(hints); !reflect.DeepEqual(got, []string{"wecom.contact.search", "wecom.department.list"}) {
		t.Fatalf("projectionPreparationActionIDs() = %#v", got)
	}
}

func TestProjectionPreparationHintsPreserveObservedTargetContract(t *testing.T) {
	hints := projectionPreparationHints([]interface{}{map[string]interface{}{
		"action_id": "WeCom.Contact.Search", "relation": "resolve_target",
		"target_arguments": []interface{}{"recipient_ref"},
		"result_paths":     []interface{}{"members[].recipient_ref"},
	}})
	want := []ActionProjectionPreparationHint{{
		ActionID: "wecom.contact.search", Relation: "resolve_target",
		TargetArguments: []string{"recipient_ref"}, ResultPaths: []string{"members[].recipient_ref"},
	}}
	if !reflect.DeepEqual(hints, want) {
		t.Fatalf("projectionPreparationHints() = %#v, want %#v", hints, want)
	}
}

func TestProjectionPreparationTransformIsServerOwnedAndFingerprintBound(t *testing.T) {
	hints := projectionPreparationHints([]interface{}{map[string]interface{}{
		"action_id": "github.repository.search", "relation": "resolve_target",
		"target_arguments": []interface{}{"owner", "repo"},
		"result_paths":     []interface{}{"repositories[].full_name"},
		"result_transform": "split_slash_pair",
	}})
	want := []ActionProjectionPreparationHint{{
		ActionID: "github.repository.search", Relation: "resolve_target",
		TargetArguments: []string{"owner", "repo"}, ResultPaths: []string{"repositories[].full_name"},
		ResultTransform: "split_slash_pair",
	}}
	if !reflect.DeepEqual(hints, want) {
		t.Fatalf("projectionPreparationHints() = %#v, want %#v", hints, want)
	}

	action := integrations.ActionDefinition{
		ID: "github.issue.create", SchemaHash: "hash", SchemaRevision: "schema", CatalogRevision: "catalog",
		Effect: toolgovernance.EffectCreate,
	}
	withTransform := actionProjectionBindingFingerprint(
		"github", "connection", action, []string{"owner", "repo"}, []string{"github.repository.search"}, hints,
	)
	withoutTransformHints := cloneActionProjectionPreparationHints(hints)
	withoutTransformHints[0].ResultTransform = ""
	withoutTransform := actionProjectionBindingFingerprint(
		"github", "connection", action, []string{"owner", "repo"}, []string{"github.repository.search"}, withoutTransformHints,
	)
	if withTransform == "" || withoutTransform == "" || withTransform == withoutTransform {
		t.Fatalf("binding fingerprints did not bind the result transform: with=%q without=%q", withTransform, withoutTransform)
	}

	if got := projectionPreparationHints([]interface{}{map[string]interface{}{
		"action_id": "github.repository.search", "relation": "resolve_target",
		"target_arguments": []interface{}{"owner", "repo"},
		"result_paths":     []interface{}{"repositories[].full_name"},
		"result_transform": "model_defined_transform",
	}}); len(got) != 0 {
		t.Fatalf("unknown transform was projected: %#v", got)
	}
}

func TestActionProjectionCandidateLimitKeepsAuthorizedSameConnectionDependencies(t *testing.T) {
	makeCandidate := func(index int) rankedActionProjection {
		return rankedActionProjection{projection: ActionProjection{
			IntegrationID: "wecom", ActionID: fmt.Sprintf("action.%03d", index),
			ConnectionID: "connection-one",
		}, score: 1000 - index}
	}

	t.Run("intent dependency below 128 boundary", func(t *testing.T) {
		ranked := make([]rankedActionProjection, 130)
		for index := range ranked {
			ranked[index] = makeCandidate(index)
		}
		ranked[0].projection.IntentMatched = true
		ranked[0].projection.PreparationActionIDs = []string{"action.129"}

		got := dependencyClosedActionProjectionCandidates(ranked, 128)
		if len(got) != 128 {
			t.Fatalf("candidate count = %d, want 128", len(got))
		}
		seen := map[string]bool{}
		priorities := map[string]int{}
		intentMatched := map[string]bool{}
		for _, item := range got {
			seen[item.projection.ActionID] = true
			priorities[item.projection.ActionID] = item.projection.ProjectionPriority
			intentMatched[item.projection.ActionID] = item.projection.IntentMatched
		}
		if !seen["action.000"] || !seen["action.129"] {
			t.Fatalf("intent Action dependency closure missing: seen target=%v dependency=%v", seen["action.000"], seen["action.129"])
		}
		if seen["action.127"] || seen["action.128"] {
			t.Fatalf("unrelated tail displaced dependency: 127=%v 128=%v", seen["action.127"], seen["action.128"])
		}
		if priorities["action.129"] != 1 || intentMatched["action.129"] {
			t.Fatalf("dependency priority = %d, want inherited intent priority without changing intent semantics", priorities["action.129"])
		}
	})

	t.Run("ordinary selected target remains dependency closed", func(t *testing.T) {
		ranked := make([]rankedActionProjection, 130)
		for index := range ranked {
			ranked[index] = makeCandidate(index)
		}
		ranked[0].projection.PreparationActionIDs = []string{"action.129"}

		got := dependencyClosedActionProjectionCandidates(ranked, 128)
		seen := map[string]bool{}
		for _, item := range got {
			seen[item.projection.ActionID] = true
		}
		if len(got) != 128 || !seen["action.000"] || !seen["action.129"] {
			t.Fatalf("ordinary selected Action closure = len %d target=%v dependency=%v", len(got), seen["action.000"], seen["action.129"])
		}
	})

	t.Run("pinned dependency below 128 boundary", func(t *testing.T) {
		ranked := make([]rankedActionProjection, 130)
		for index := range ranked {
			ranked[index] = makeCandidate(index)
		}
		ranked[128].pinned = true
		ranked[128].projection.Pinned = true
		ranked[128].projection.PreparationActionIDs = []string{"action.129"}

		got := dependencyClosedActionProjectionCandidates(ranked, 128)
		seen := map[string]bool{}
		priorities := map[string]int{}
		for _, item := range got {
			seen[item.projection.ActionID] = true
			priorities[item.projection.ActionID] = item.projection.ProjectionPriority
		}
		if len(got) != 128 || !seen["action.128"] || !seen["action.129"] {
			t.Fatalf("pinned Action dependency closure = len %d target=%v dependency=%v", len(got), seen["action.128"], seen["action.129"])
		}
		if priorities["action.129"] != 2 {
			t.Fatalf("pinned dependency priority = %d, want 2", priorities["action.129"])
		}
	})

	t.Run("different selected connection is not promoted", func(t *testing.T) {
		ranked := make([]rankedActionProjection, 129)
		for index := range ranked {
			ranked[index] = makeCandidate(index)
		}
		ranked[0].projection.IntentMatched = true
		ranked[0].projection.PreparationActionIDs = []string{"action.128"}
		ranked[128].projection.ConnectionID = "connection-two"

		got := dependencyClosedActionProjectionCandidates(ranked, 128)
		seen := map[string]bool{}
		for _, item := range got {
			seen[item.projection.ActionID] = true
		}
		if seen["action.128"] {
			t.Fatal("dependency from a different selected connection was promoted")
		}
	})

	t.Run("unavailable dependency is never synthesized", func(t *testing.T) {
		ranked := make([]rankedActionProjection, 129)
		for index := range ranked {
			ranked[index] = makeCandidate(index)
		}
		ranked[0].projection.IntentMatched = true
		ranked[0].projection.PreparationActionIDs = []string{"action.not-authorized"}

		got := dependencyClosedActionProjectionCandidates(ranked, 128)
		for _, item := range got {
			if item.projection.ActionID == "action.not-authorized" {
				t.Fatal("dependency absent from the authorized projection set was synthesized")
			}
		}
	})
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

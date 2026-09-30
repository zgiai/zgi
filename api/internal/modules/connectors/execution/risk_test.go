package execution

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestTrustedRiskResolverUsesOnlyExactPlatformPolicyAndFailsClosed(t *testing.T) {
	definition := riskTestDefinition()
	definition.Operations = append(definition.Operations,
		catalog.Operation{ID: "github.issue.create", ToolName: "create_github_issue", Name: "Create issue"},
		catalog.Operation{ID: "github.repository.delete", ToolName: "delete_github_repository", Name: "Delete repository"},
	)
	definitionCatalog, err := catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTrustedRiskResolver(definitionCatalog, []TrustedOperationRiskPolicy{
		trustedRiskTestPolicy("github.user.get", "get_github_user", RiskTrustedLow),
		trustedRiskTestPolicy("github.issue.create", "create_github_issue", RiskHigh),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := validRiskRequest()

	classification, err := resolver.ResolveRisk(context.Background(), request)
	if err != nil || classification != RiskTrustedLow {
		t.Fatalf("ResolveRisk(read) = %q, %v", classification, err)
	}
	request.OperationID = core.Identifier("github.issue.create")
	classification, err = resolver.ResolveRisk(context.Background(), request)
	if err != nil || classification != RiskHigh {
		t.Fatalf("ResolveRisk(write) = %q, %v", classification, err)
	}
	request.OperationID = core.Identifier("github.repository.delete")
	classification, err = resolver.ResolveRisk(context.Background(), request)
	if err != nil || classification != RiskUnclassified {
		t.Fatalf("ResolveRisk(plugin-only operation) = %q, %v, want unclassified", classification, err)
	}
}

func TestTrustedRiskResolverRejectsCatalogToolRemapAndWrongPrincipal(t *testing.T) {
	definition := riskTestDefinition()
	definition.Operations[0].ToolName = "replacement_tool"
	definitionCatalog, err := catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTrustedRiskResolver(definitionCatalog, []TrustedOperationRiskPolicy{
		trustedRiskTestPolicy("github.user.get", "get_github_user", RiskTrustedLow),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := validRiskRequest()
	classification, err := resolver.ResolveRisk(context.Background(), request)
	if err != nil || classification != RiskUnclassified {
		t.Fatalf("ResolveRisk(remapped tool) = %q, %v", classification, err)
	}
	request.ActingPrincipalID = uuid.New()
	classification, err = resolver.ResolveRisk(context.Background(), request)
	if classification != RiskUnclassified || !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("ResolveRisk(wrong principal) = %q, %v", classification, err)
	}
}

func TestTrustedRiskResolverKeepsAcceptedContractAfterCatalogDeactivation(t *testing.T) {
	definition := riskTestDefinition()
	activeCatalog, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := NewTrustedRiskResolver(activeCatalog, []TrustedOperationRiskPolicy{
		trustedRiskTestPolicy("github.user.get", "get_github_user", RiskTrustedLow),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := validRiskRequest()
	classification, err := resolver.ResolveRisk(
		withPinnedRiskContract(context.Background(), definition, definition.Operations[0]),
		request,
	)
	if err != nil || classification != RiskTrustedLow {
		t.Fatalf("ResolveRisk(pinned after deactivate) = %q, %v", classification, err)
	}
	classification, err = resolver.ResolveRisk(context.Background(), request)
	if err != nil || classification != RiskUnclassified {
		t.Fatalf("ResolveRisk(new operation after deactivate) = %q, %v", classification, err)
	}
}

func riskTestDefinition() catalog.Definition {
	return catalog.Definition{
		ID: "github", PluginID: "zgi.github", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub connector", Classification: catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeAction}, SetupMethod: catalog.SetupMethodKey,
		Ownership: catalog.OwnershipPersonal, Surfaces: catalog.FirstReleaseSurfaces(),
		KeyFields:  []catalog.KeyField{{Name: "token", Label: "Token", Required: true, Secret: true}},
		Operations: []catalog.Operation{{ID: "github.user.get", ToolName: "get_github_user", Name: "Get user"}},
	}
}

func trustedRiskTestPolicy(operationID, toolName string, classification RiskClassification) TrustedOperationRiskPolicy {
	return TrustedOperationRiskPolicy{
		ConnectorID: core.Identifier("github"), ProviderID: core.Identifier("github"),
		DriverID: core.Identifier("github-rest"), AuthMethodID: core.Identifier("personal_access_token"),
		OperationID: core.Identifier(operationID), ToolName: toolName, Classification: classification,
	}
}

func validRiskRequest() RiskRequest {
	ownerID := uuid.New()
	return RiskRequest{
		OrganizationID: uuid.New(), OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
		ConnectorID: core.Identifier("github"), ConnectionID: uuid.New(),
		Target:      core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"},
		OperationID: core.Identifier("github.user.get"), Interactive: true,
	}
}

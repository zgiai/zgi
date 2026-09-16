package tools

import (
	"context"
	"testing"
)

func TestToolEngineRejectsConnectorBeforeProviderLookup(t *testing.T) {
	engine := NewToolEngine(nil)
	result, err := engine.Invoke(context.Background(), InvokeRequest{ProviderType: ToolProviderTypeConnector, ProviderID: "github", ToolName: "get_github_user"})
	if err == nil || result == nil || result.Success {
		t.Fatal("Connector execution entered the generic tool engine")
	}
	if _, err := engine.EnrichGovernanceArguments(context.Background(), InvokeRequest{ProviderType: ToolProviderTypeConnector}); err == nil {
		t.Fatal("Connector governance entered the generic tool engine")
	}
}

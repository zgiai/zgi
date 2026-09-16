package container

import "testing"

func TestConnectorNativeRouterInitializesWithoutRolloutOrLegacyDependencies(t *testing.T) {
	c := &ServiceContainer{config: connectorOnlyTestConfig()}
	router := c.GetConnectorExecutionRouter()
	if router == nil || router != c.GetConnectorExecutionRouter() || c.connectorInvocationStore == nil {
		t.Fatal("native invocation graph incomplete")
	}
}

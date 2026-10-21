package container

import (
	"fmt"

	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
)

// GetConnectorChatRuntimeResolver returns the owner-scoped AI Chat discovery
// and invocation resolver whenever runtime configuration is available. It is
// independent of the legacy external-integrations flag.
func (c *ServiceContainer) GetConnectorChatRuntimeResolver() *connectorruntimeauth.ChatResolver {
	if c == nil || c.config == nil {
		return nil
	}
	resolver, err := connectorruntimeauth.NewChatResolver(
		c.GetConnectorBindingService(),
		c.GetConnectorConnectionService(),
		c.GetConnectorPluginManager(),
	)
	if err != nil {
		panic(fmt.Sprintf("initialize AI Chat Connector runtime resolver: %v", err))
	}
	return resolver
}

// GetConnectorAgentRuntimeResolver returns the owner-scoped resolver used by
// interactive Agent conversations. Unattended Agent execution is deliberately
// kept on a separate resolver so it cannot inherit Interactive=true.
func (c *ServiceContainer) GetConnectorAgentRuntimeResolver() *connectorruntimeauth.AgentResolver {
	if c == nil || c.config == nil {
		return nil
	}
	resolver, err := connectorruntimeauth.NewAgentResolver(
		c.GetConnectorBindingService(),
		c.GetConnectorConnectionService(),
		c.GetConnectorPluginManager(),
	)
	if err != nil {
		panic(fmt.Sprintf("initialize Agent Connector runtime resolver: %v", err))
	}
	return resolver.WithDelegatedHostAccess(
		c.GetConnectorBindingService(),
		c.GetConnectorHostDelegationAuthorizer(),
	)
}

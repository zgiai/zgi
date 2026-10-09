# External integrations retired

The old integration runtime, providers, management UI and OAuth flows have been removed.
Use [Personal Connectors](connectors.md) for native GitHub and Exa Key connections.
For protocol details and the GitHub MCP Connector, see the [MCP client and GitHub Connector guide](mcp-client.md).
Existing data requires the [offline conversion procedure](../tools/connector-migrate/README.md).
Unsupported connections remain unavailable until explicitly replaced; old API routes return 410.

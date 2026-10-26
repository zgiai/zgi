package config

import "testing"

func TestSocialMCPDeploymentConfiguration(t *testing.T) {
	values := map[string]string{"CONNECTOR_SOCIAL_MCP_NODE": "/usr/bin/node", "CONNECTOR_SOCIAL_MCP_SCRIPT": "/opt/zgi/social-mcp/server.mjs"}
	cfg := &Config{}
	if err := loadConnectorsConfig(cfg, &envSource{lookupEnv: func(k string) (string, bool) { v, ok := values[k]; return v, ok }}); err != nil {
		t.Fatal(err)
	}
	if cfg.Connectors.SocialMCPNode != "/usr/bin/node" || cfg.Connectors.SocialMCPScript != "/opt/zgi/social-mcp/server.mjs" {
		t.Fatal("social deployment configuration not loaded")
	}
	cfg.Connectors.CredentialActiveKeyID = "default"
	cfg.Connectors.CredentialKeys = map[string]string{"default": "12345678901234567890123456789012"}
	if err := validateConnectorsConfig(cfg.Connectors); err != nil {
		t.Fatal(err)
	}
	cfg.Connectors.SocialMCPScript = "relative/server.mjs"
	if err := validateConnectorsConfig(cfg.Connectors); err == nil {
		t.Fatal("relative executable script accepted")
	}
	cfg.Connectors.SocialMCPScript = "/opt/zgi/server.mjs"
	cfg.Connectors.SocialMCPNode = "sh -c node"
	if err := validateConnectorsConfig(cfg.Connectors); err == nil {
		t.Fatal("shell command accepted")
	}
}

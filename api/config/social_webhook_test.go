package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFacebookWebhookDeploymentSecrets(t *testing.T) {
	values := map[string]string{"CONNECTOR_FACEBOOK_WEBHOOK_APP_SECRET": "0123456789abcdef0123456789abcdef", "CONNECTOR_FACEBOOK_WEBHOOK_VERIFY_TOKEN": "verify-token-sentinel-0123456789"}
	cfg := &Config{}
	if err := loadConnectorsConfig(cfg, &envSource{lookupEnv: func(k string) (string, bool) { v, ok := values[k]; return v, ok }}); err != nil {
		t.Fatal(err)
	}
	if cfg.Connectors.SocialWebhookFacebookAppSecret != values["CONNECTOR_FACEBOOK_WEBHOOK_APP_SECRET"] || cfg.Connectors.SocialWebhookFacebookVerifyToken != values["CONNECTOR_FACEBOOK_WEBHOOK_VERIFY_TOKEN"] {
		t.Fatal("webhook configuration missing")
	}
	raw, _ := json.Marshal(cfg.Connectors)
	if strings.Contains(string(raw), "sentinel") || strings.Contains(string(raw), values["CONNECTOR_FACEBOOK_WEBHOOK_APP_SECRET"]) {
		t.Fatal("webhook secret serialized")
	}
	if err := validateSocialWebhookConfig(cfg.Connectors); err != nil {
		t.Fatal(err)
	}
	cfg.Connectors.SocialWebhookFacebookVerifyToken = ""
	if validateSocialWebhookConfig(cfg.Connectors) == nil {
		t.Fatal("partial webhook configuration accepted")
	}
	cfg.Connectors.SocialWebhookFacebookAppSecret = ""
	if err := validateSocialWebhookConfig(cfg.Connectors); err != nil {
		t.Fatal("disabled webhook rejected")
	}
}

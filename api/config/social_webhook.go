package config

import (
	"fmt"
	"strings"
)

func validateSocialWebhookConfig(cfg ConnectorsConfig) error {
	secret, verify := cfg.SocialWebhookFacebookAppSecret, cfg.SocialWebhookFacebookVerifyToken
	if secret == "" && verify == "" {
		return nil
	}
	if len(secret) < 16 || len(secret) > 4096 || len(verify) < 16 || len(verify) > 256 || strings.ContainsAny(secret+verify, "\x00\r\n\t ") {
		return fmt.Errorf("Facebook webhook app secret and a random verify token of at least 16 characters must be configured together")
	}
	return nil
}

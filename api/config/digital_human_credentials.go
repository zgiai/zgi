package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
)

// DigitalHumanCredentialsConfig preserves the historical v1/v2 keyring used by
// pre-Connector DigitalHuman channels. Native Connector c1 envelopes use the
// separate Connectors configuration, so the two keyrings must never be shared.
type DigitalHumanCredentialsConfig struct {
	ActiveKeyID string            `json:"-"`
	Keys        map[string]string `json:"-"`
}

// loadDigitalHumanCredentialsConfig builds the legacy DigitalHuman keyring from
// an optional JSON keyring plus the historical API_KEY_ENCRYPTION_KEY. The
// previous container stored that setting under the literal key id "legacy", so
// it is always retained (and becomes active when nothing newer is configured)
// to keep existing v1/v2 channel envelopes readable. Deployments that have not
// configured the new variables keep working unchanged.
func loadDigitalHumanCredentialsConfig(cfg *Config, source *envSource) error {
	keys := map[string]string{}
	if raw := strings.TrimSpace(source.string("", envDigitalHumanCredentialKeysJSON)); raw != "" {
		if err := json.Unmarshal([]byte(raw), &keys); err != nil {
			return fmt.Errorf("%s must be a JSON object", envDigitalHumanCredentialKeysJSON)
		}
		if keys == nil {
			return fmt.Errorf("%s must be a JSON object", envDigitalHumanCredentialKeysJSON)
		}
	}
	active := strings.TrimSpace(source.string("", envDigitalHumanCredentialActiveKeyID))
	// The previous container used the literal key id "legacy" for this setting.
	// Keep it even after an active key is rotated so old envelopes stay readable.
	legacy := cfg.Encryption.APIKeyEncryptionKey
	if legacy != "" {
		if existing, ok := keys["legacy"]; ok && existing != legacy {
			return fmt.Errorf("DigitalHuman legacy key conflicts with API_KEY_ENCRYPTION_KEY")
		}
		keys["legacy"] = legacy
		if active == "" {
			active = "legacy"
		}
	}
	if active == "" {
		if _, ok := keys["default"]; ok {
			active = "default"
		}
	}
	if len(keys) > 0 {
		if _, err := credentialcrypto.NewCredentialKeyring(active, keys); err != nil {
			return fmt.Errorf("invalid DigitalHuman credential keyring: %w", err)
		}
	} else if active != "" {
		return fmt.Errorf("%s requires %s", envDigitalHumanCredentialActiveKeyID, envDigitalHumanCredentialKeysJSON)
	}
	cfg.DigitalHumanCredentials = DigitalHumanCredentialsConfig{ActiveKeyID: active, Keys: keys}
	return nil
}

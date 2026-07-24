package config

import "testing"

func TestLoadLLMConfigDoesNotReadRemovedUpstreamFeatureSwitches(t *testing.T) {
	removedKeys := map[string]struct{}{
		"LLM_UPSTREAM_BALANCE_POLLING_ENABLED": {},
		"LLM_UPSTREAM_GUARD_MODE":              {},
		"LLM_UPSTREAM_GUARD_PERCENTAGE":        {},
	}
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if _, removed := removedKeys[key]; removed {
			t.Fatalf("loadLLMConfig() read removed feature switch %q", key)
		}
		return "", false
	}}
	loadLLMConfig(cfg, source)
}

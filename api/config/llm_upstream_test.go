package config

import "testing"

func TestLoadLLMConfigUpstreamDefaultsAreDisabled(t *testing.T) {
	cfg := &Config{}
	loadLLMConfig(cfg, &envSource{lookupEnv: func(string) (string, bool) { return "", false }})

	if cfg.LLM.UpstreamBalancePolling {
		t.Fatal("UpstreamBalancePolling = true, want false")
	}
	if cfg.LLM.UpstreamGuardMode != "off" || cfg.LLM.UpstreamGuardPercentage != 0 {
		t.Fatalf("upstream guard = %q/%d, want off/0", cfg.LLM.UpstreamGuardMode, cfg.LLM.UpstreamGuardPercentage)
	}
}

func TestLoadLLMConfigReadsUpstreamRolloutControls(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		switch key {
		case envLLMUpstreamBalancePollingEnabled:
			return "true", true
		case envLLMUpstreamGuardMode:
			return " Enforce ", true
		case envLLMUpstreamGuardPercentage:
			return "25", true
		default:
			return "", false
		}
	}}
	loadLLMConfig(cfg, source)

	if !cfg.LLM.UpstreamBalancePolling {
		t.Fatal("UpstreamBalancePolling = false, want true")
	}
	if cfg.LLM.UpstreamGuardMode != "enforce" || cfg.LLM.UpstreamGuardPercentage != 25 {
		t.Fatalf("upstream guard = %q/%d, want enforce/25", cfg.LLM.UpstreamGuardMode, cfg.LLM.UpstreamGuardPercentage)
	}
}

package wecom

import "testing"

func TestSuiteConfigRequiresProviderSecret(t *testing.T) {
	config := SuiteConfig{
		SuiteID:            "ww_suite",
		ProviderCorpID:     "ww_provider",
		ProviderSecret:     "provider-secret",
		SuiteSecret:        "suite-secret",
		CallbackToken:      "callback-token",
		EncodingAESKey:     "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		InstallCallbackURL: "https://example.com/console/api/integrations/wecom/install/callback",
	}
	if !config.Configured() {
		t.Fatal("complete WeCom Suite config was not recognized")
	}

	config.ProviderSecret = " "
	if config.Configured() {
		t.Fatal("WeCom Suite config without a provider secret was recognized")
	}
}

package config

import "testing"

func TestConnectorRetirementDeferred(t *testing.T) {
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	GlobalConfig = &Config{source: &envSource{lookupEnv: func(key string) (string, bool) {
		if key == ConnectorRetirementModeEnv {
			return "deferred", true
		}
		return "", false
	}}}
	if !ConnectorRetirementDeferred() {
		t.Fatal("deferred retirement mode was not detected")
	}
	GlobalConfig.source.lookupEnv = func(string) (string, bool) { return "", false }
	if ConnectorRetirementDeferred() {
		t.Fatal("deferred retirement mode ignored missing configuration")
	}
}

func TestConnectorRetirementDeferredWithoutLoadedConfigReadsEnvironment(t *testing.T) {
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	GlobalConfig = nil
	t.Setenv(ConnectorRetirementModeEnv, "deferred")
	if !ConnectorRetirementDeferred() {
		t.Fatal("deferred retirement mode was not read from the environment")
	}
	t.Setenv(ConnectorRetirementModeEnv, "")
	if ConnectorRetirementDeferred() {
		t.Fatal("empty retirement mode was treated as deferred")
	}
}

func TestConnectorRetirementModeDefaultsToAutoAndRejectsTypos(t *testing.T) {
	previous := GlobalConfig
	t.Cleanup(func() { GlobalConfig = previous })
	GlobalConfig = nil
	for value, want := range map[string]string{
		"": ConnectorRetirementModeAuto, "auto": ConnectorRetirementModeAuto, " Deferred ": ConnectorRetirementModeDeferred,
		"strict": ConnectorRetirementModeStrict, "deffered": ConnectorRetirementModeStrict,
	} {
		t.Setenv(ConnectorRetirementModeEnv, value)
		if got := ConnectorRetirementMode(); got != want {
			t.Fatalf("mode for %q = %q, want %q", value, got, want)
		}
	}
}

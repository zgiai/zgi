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

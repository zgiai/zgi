package config

import (
	"testing"
	"time"
)

// consoleSyncEnvSource keeps the console control-plane tests independent from the
// process environment, mirroring digitalHumanEnvSource.
func consoleSyncEnvSource(values map[string]string) *envSource {
	return &envSource{lookupEnv: func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}}
}

// The organization inventory report cadence is a Go duration string: unset means 5m and
// an explicit "0" disables the report.
func TestLoadConsoleOrganizationReportInterval(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "default", value: "", want: defaultOrganizationReportInterval},
		{name: "custom", value: "90s", want: 90 * time.Second},
		{name: "disabled", value: "0", want: 0},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			values := map[string]string{}
			if testCase.value != "" {
				values[envConsoleSyncOrganizationReportInterval] = testCase.value
			}
			cfg := &Config{}
			if err := loadConsoleConfig(cfg, consoleSyncEnvSource(values)); err != nil {
				t.Fatalf("loadConsoleConfig() error = %v", err)
			}
			if got := cfg.Console.SyncOrganizationReportInterval; got != testCase.want {
				t.Fatalf("interval = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestLoadConsoleRejectsInvalidOrganizationReportInterval(t *testing.T) {
	source := consoleSyncEnvSource(map[string]string{
		envConsoleSyncOrganizationReportInterval: "every five minutes",
	})
	if err := loadConsoleConfig(&Config{}, source); err == nil {
		t.Fatal("an invalid duration was accepted")
	}
}

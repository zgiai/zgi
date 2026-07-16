package config

import (
	"reflect"
	"testing"
)

func TestLoadObservabilityConfigSupportsMultipleReporters(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envZGIReporters {
			return "Sentry, OTEL", true
		}
		return "", false
	}}

	loadObservabilityConfig(cfg, source)

	want := []string{"sentry", "otel"}
	if !reflect.DeepEqual(cfg.Observability.Reporters, want) {
		t.Fatalf("Reporters = %#v, want %#v", cfg.Observability.Reporters, want)
	}
	if !cfg.Observability.ReporterEnabled("sentry", true) || !cfg.Observability.ReporterEnabled("otel", true) {
		t.Fatal("selected configured reporters should be enabled")
	}
}

func TestObservabilityReporterSelection(t *testing.T) {
	tests := []struct {
		name       string
		reporters  []string
		provider   string
		configured bool
		want       bool
	}{
		{name: "auto detects configured provider", provider: "sentry", configured: true, want: true},
		{name: "auto keeps unconfigured provider off", provider: "sentry", configured: false, want: false},
		{name: "explicit selection", reporters: []string{"sentry", "otel"}, provider: "otel", configured: true, want: true},
		{name: "unselected provider", reporters: []string{"sentry"}, provider: "otel", configured: true, want: false},
		{name: "none wins", reporters: []string{"sentry", "none"}, provider: "sentry", configured: true, want: false},
		{name: "selection cannot replace provider config", reporters: []string{"sentry"}, provider: "sentry", configured: false, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := ObservabilityConfig{Reporters: test.reporters}
			if got := cfg.ReporterEnabled(test.provider, test.configured); got != test.want {
				t.Fatalf("ReporterEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}

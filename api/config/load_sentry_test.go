package config

import "testing"

func TestExpandEnvUsesConfigSource(t *testing.T) {
	prev := GlobalConfig
	t.Cleanup(func() {
		GlobalConfig = prev
	})
	GlobalConfig = &Config{
		source: &envSource{
			lookupEnv: func(key string) (string, bool) {
				switch key {
				case "FIXTURE_ROOT":
					return "/tmp/fixtures", true
				case "MISSING":
					return "", false
				default:
					return "", false
				}
			},
		},
	}

	got := ExpandEnv("$FIXTURE_ROOT/a/${MISSING}/b")
	if got != "/tmp/fixtures/a//b" {
		t.Fatalf("ExpandEnv() = %q, want /tmp/fixtures/a//b", got)
	}
}

func TestLoadSentryConfigUsesEnvironmentOverride(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Environment: "production"},
	}
	source := &envSource{
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case envSentryDSN:
				return "https://example@sentry.example.com/1", true
			case envSentryEnvironment:
				return "TEST-A", true
			default:
				return "", false
			}
		},
	}

	loadSentryConfig(cfg, source)

	if cfg.Sentry.Environment != "TEST-A" {
		t.Fatalf("Sentry.Environment = %q, want TEST-A", cfg.Sentry.Environment)
	}
}

func TestLoadSentryConfigFallsBackWhenEnvironmentOverrideEmpty(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Environment: "production"},
	}
	source := &envSource{
		lookupEnv: func(key string) (string, bool) {
			switch key {
			case envSentryDSN:
				return "https://example@sentry.example.com/1", true
			case envSentryEnvironment:
				return "   ", true
			default:
				return "", false
			}
		},
	}

	loadSentryConfig(cfg, source)

	if cfg.Sentry.Environment != "production" {
		t.Fatalf("Sentry.Environment = %q, want production", cfg.Sentry.Environment)
	}
}

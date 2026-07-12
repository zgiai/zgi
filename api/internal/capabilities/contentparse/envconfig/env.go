package envconfig

import (
	"os"
	"strconv"
	"strings"
	"sync"

	appconfig "github.com/zgiai/zgi/api/config"
)

var (
	overrideMu        sync.RWMutex
	overrideExecution sync.Mutex
	overrides         = map[string]string{}
)

// String reads runtime configuration through the application config source.
// This keeps local service runs consistent with .env loading while preserving
// Docker/system environment behavior when no .env file is present.
func String(key string) string {
	if value, ok := lookupOverride(key); ok {
		return strings.TrimSpace(value)
	}

	if isTestBinary() {
		if value, ok := os.LookupEnv(key); ok {
			return strings.TrimSpace(value)
		}
	}

	if value, ok := appconfig.Lookup(key); ok {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(os.Getenv(key))
}

func lookupOverride(key string) (string, bool) {
	overrideMu.RLock()
	defer overrideMu.RUnlock()
	value, ok := overrides[key]
	return value, ok
}

func WithOverrides(values map[string]string, fn func()) {
	_ = WithOverridesResult(values, func() error {
		fn()
		return nil
	})
}

func WithOverridesResult(values map[string]string, fn func() error) error {
	if len(values) == 0 {
		return fn()
	}

	overrideMu.Lock()
	previous := make(map[string]string, len(values))
	hadPrevious := make(map[string]bool, len(values))
	for key, value := range values {
		if current, ok := overrides[key]; ok {
			previous[key] = current
			hadPrevious[key] = true
		}
		overrides[key] = value
	}
	overrideMu.Unlock()

	defer func() {
		overrideMu.Lock()
		defer overrideMu.Unlock()
		for key := range values {
			if hadPrevious[key] {
				overrides[key] = previous[key]
			} else {
				delete(overrides, key)
			}
		}
	}()

	return fn()
}

// WithExclusiveOverridesResult isolates request-scoped provider credentials
// and endpoints from concurrent parse executions. Nested parser overrides use
// WithOverridesResult so they can safely run inside this exclusive scope.
func WithExclusiveOverridesResult(values map[string]string, fn func() error) error {
	overrideExecution.Lock()
	defer overrideExecution.Unlock()
	return WithOverridesResult(values, fn)
}

func isTestBinary() bool {
	return len(os.Args) > 0 && strings.HasSuffix(os.Args[0], ".test")
}

func First(keys ...string) string {
	for _, key := range keys {
		if value := String(key); value != "" {
			return value
		}
	}
	return ""
}

func FirstName(keys ...string) string {
	for _, key := range keys {
		if String(key) != "" {
			return key
		}
	}
	return ""
}

func Int(key string, fallback int) int {
	value := String(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func Bool(key string, fallback bool) bool {
	value := String(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

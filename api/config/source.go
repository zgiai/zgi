package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type envSource struct {
	path      string
	v         *viper.Viper
	lookupEnv func(string) (string, bool)
}

func newEnvSource(path string) (*envSource, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve env file path: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(absPath)
	v.SetConfigType("env")
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read env file %s: %w", absPath, err)
	}

	return &envSource{
		path: absPath,
		v:    v,
	}, nil
}

func newDefaultEnvSource() (*envSource, error) {
	path, found, err := resolveDefaultEnvFile()
	if err != nil {
		return nil, err
	}
	if found {
		return newEnvSource(path)
	}
	return newEnvironmentSource(), nil
}

func newEnvironmentSource() *envSource {
	return &envSource{
		lookupEnv: os.LookupEnv,
	}
}

func resolveDefaultEnvFile() (string, bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("get current directory: %w", err)
	}

	current := cwd
	for {
		candidate := filepath.Join(current, ".env")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, true, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
		current = parent
	}
}

func (s *envSource) lookup(keys ...string) (string, bool) {
	for _, key := range keys {
		if s == nil {
			return "", false
		}

		if s.v != nil {
			if !s.v.InConfig(key) {
				continue
			}
			return strings.TrimSpace(s.v.GetString(key)), true
		}

		if s.lookupEnv == nil {
			continue
		}
		value, ok := s.lookupEnv(key)
		if !ok {
			continue
		}
		return strings.TrimSpace(value), true
	}
	return "", false
}

func (s *envSource) string(defaultValue string, keys ...string) string {
	if value, ok := s.lookup(keys...); ok {
		return value
	}
	return defaultValue
}

func (s *envSource) prefixedStrings(prefix string) map[string]string {
	if s == nil || prefix == "" {
		return nil
	}

	normalizedPrefix := strings.ToUpper(prefix)
	values := make(map[string]string)
	if s.v != nil {
		for _, key := range s.v.AllKeys() {
			normalizedKey := strings.ToUpper(strings.TrimSpace(key))
			if !strings.HasPrefix(normalizedKey, normalizedPrefix) {
				continue
			}
			name := strings.TrimPrefix(normalizedKey, normalizedPrefix)
			value := strings.TrimSpace(s.v.GetString(key))
			if name != "" && value != "" {
				values[name] = value
			}
		}
		return nilIfEmpty(values)
	}

	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		normalizedKey := strings.ToUpper(strings.TrimSpace(key))
		if !strings.HasPrefix(normalizedKey, normalizedPrefix) {
			continue
		}
		name := strings.TrimPrefix(normalizedKey, normalizedPrefix)
		value = strings.TrimSpace(value)
		if name != "" && value != "" {
			values[name] = value
		}
	}
	return nilIfEmpty(values)
}

func nilIfEmpty(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values
}

func (s *envSource) int(defaultValue int, keys ...string) (int, error) {
	value, ok := s.lookup(keys...)
	if !ok || value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", keys[0], err)
	}
	return parsed, nil
}

func (s *envSource) int64(defaultValue int64, keys ...string) (int64, error) {
	value, ok := s.lookup(keys...)
	if !ok || value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", keys[0], err)
	}
	return parsed, nil
}

func (s *envSource) float64(defaultValue float64, keys ...string) (float64, error) {
	value, ok := s.lookup(keys...)
	if !ok || value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", keys[0], err)
	}
	return parsed, nil
}

func (s *envSource) bool(defaultValue bool, keys ...string) (bool, error) {
	value, ok := s.lookup(keys...)
	if !ok || value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s: %v", keys[0], err)
	}
	return parsed, nil
}

func (s *envSource) duration(defaultValue time.Duration, keys ...string) (time.Duration, error) {
	value, ok := s.lookup(keys...)
	if !ok || value == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %v", keys[0], err)
	}
	return parsed, nil
}

func (s *envSource) csv(defaultValue []string, keys ...string) []string {
	value, ok := s.lookup(keys...)
	if !ok {
		return append([]string(nil), defaultValue...)
	}

	if value == "" {
		return []string{}
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

func (s *envSource) scopeList(defaultValue []string, keys ...string) []string {
	value, ok := s.lookup(keys...)
	if !ok {
		return append([]string(nil), defaultValue...)
	}
	value = strings.ReplaceAll(value, ",", " ")
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return []string{}
	}
	return parts
}

func Current() *Config {
	if GlobalConfig != nil {
		return GlobalConfig
	}

	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

func Lookup(key string) (string, bool) {
	cfg := Current()
	if cfg == nil || cfg.source == nil {
		return "", false
	}
	return cfg.source.lookup(key)
}

func GetString(key, defaultValue string) string {
	if value, ok := Lookup(key); ok {
		return value
	}
	return defaultValue
}

func GetBool(key string, defaultValue bool) bool {
	value, ok := Lookup(key)
	if !ok {
		return defaultValue
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func GetInt(key string, defaultValue int) int {
	value, ok := Lookup(key)
	if !ok {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func GetInt64(key string, defaultValue int64) int64 {
	value, ok := Lookup(key)
	if !ok {
		return defaultValue
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

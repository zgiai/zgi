package config

import (
	"strings"
	"testing"
)

func TestLoadNeo4jConfigAllowsUnconfiguredRuntime(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}
	loadNeo4jConfig(cfg, source)
	if cfg.Neo4j.URI != "" {
		t.Fatalf("expected empty Neo4j URI, got %q", cfg.Neo4j.URI)
	}

	loadGraphFlowConfig(cfg, source)
	if cfg.GraphFlow.VectorSyncBatchSize != 10 || cfg.GraphFlow.VectorSyncConcurrency != 10 {
		t.Fatalf("unexpected GraphFlow defaults: %#v", cfg.GraphFlow)
	}
}

func TestLoadTaskQueueConfigUsesDedicatedGraphFlowQuota(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		values := map[string]string{
			envTaskQueueConcurrency:          "8",
			envGraphFlowTaskQueueConcurrency: "3",
		}
		value, ok := values[key]
		return value, ok
	}}
	if err := loadTaskQueueConfig(cfg, source); err != nil {
		t.Fatal(err)
	}
	if cfg.TaskQueue.Concurrency != 8 || cfg.TaskQueue.GraphFlowConcurrency != 3 {
		t.Fatalf("unexpected task queue concurrency: %#v", cfg.TaskQueue)
	}
}

func TestLoadTaskQueueConfigRejectsGraphFlowQuotaAboveFour(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envGraphFlowTaskQueueConcurrency {
			return "5", true
		}
		return "", false
	}}
	err := loadTaskQueueConfig(cfg, source)
	if err == nil || !strings.Contains(err.Error(), envGraphFlowTaskQueueConcurrency) {
		t.Fatalf("loadTaskQueueConfig() error=%v, want graphflow concurrency validation", err)
	}
}

// Zero once meant "disable the workers" to operators; Asynq instead runs CPU
// count workers and the scheduler worker fell back to the default queue.
func TestLoadTaskQueueConfigRejectsNonPositiveConcurrency(t *testing.T) {
	for _, value := range []string{"0", "-1"} {
		cfg := &Config{}
		source := &envSource{lookupEnv: func(key string) (string, bool) {
			if key == envTaskQueueConcurrency {
				return value, true
			}
			return "", false
		}}
		err := loadTaskQueueConfig(cfg, source)
		if err == nil || !strings.Contains(err.Error(), envTaskQueueConcurrency) {
			t.Fatalf("loadTaskQueueConfig(%s=%s) error=%v, want concurrency validation", envTaskQueueConcurrency, value, err)
		}
	}
}

func TestLoadAppConfigUsesThirtyDayChannelDeliveryTTL(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}
	if err := loadAppConfig(cfg, source); err != nil {
		t.Fatal(err)
	}
	if cfg.App.FilesAccessTimeout != 3600 {
		t.Fatalf("FilesAccessTimeout=%d, want 3600", cfg.App.FilesAccessTimeout)
	}
	if cfg.App.ChannelDeliveryFileTTL != DefaultChannelDeliveryFileTTLSeconds {
		t.Fatalf("ChannelDeliveryFileTTL=%d, want %d", cfg.App.ChannelDeliveryFileTTL, DefaultChannelDeliveryFileTTLSeconds)
	}
}

func TestLoadAppConfigRejectsChannelDeliveryTTLAboveThirtyDays(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChannelDeliveryFileTTL {
			return "2592001", true
		}
		return "", false
	}}
	if err := loadAppConfig(cfg, source); err == nil || !strings.Contains(err.Error(), envChannelDeliveryFileTTL) {
		t.Fatalf("loadAppConfig() error=%v, want channel TTL validation", err)
	}
}

func TestLoadAppConfigUsesConfiguredChannelDeliveryTTL(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(key string) (string, bool) {
		if key == envChannelDeliveryFileTTL {
			return "86400", true
		}
		return "", false
	}}
	if err := loadAppConfig(cfg, source); err != nil {
		t.Fatal(err)
	}
	if cfg.App.ChannelDeliveryFileTTL != 86400 {
		t.Fatalf("ChannelDeliveryFileTTL=%d, want 86400", cfg.App.ChannelDeliveryFileTTL)
	}
}

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

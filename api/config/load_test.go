package config

import "testing"

func TestLoadNeo4jConfigAllowsUnconfiguredRuntime(t *testing.T) {
	cfg := &Config{}
	source := &envSource{lookupEnv: func(string) (string, bool) { return "", false }}
	loadNeo4jConfig(cfg, source)
	if cfg.Neo4j.URI != "" {
		t.Fatalf("expected empty Neo4j URI, got %q", cfg.Neo4j.URI)
	}

	loadGraphFlowConfig(cfg, source)
	if cfg.GraphFlow.VectorSyncBatchSize != 50 || cfg.GraphFlow.VectorSyncConcurrency != 10 {
		t.Fatalf("unexpected GraphFlow defaults: %#v", cfg.GraphFlow)
	}
}

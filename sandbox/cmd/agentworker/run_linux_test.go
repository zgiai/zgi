//go:build linux

package main

import (
	"os"
	"testing"
	"time"
)

func TestWorkerConfigRequiresDedicatedSecretsAndProfiles(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_BACKEND", localBackendName)
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "")
	t.Setenv("ZGI_AGENT_RUNTIME_DATABASE_URL", "")
	t.Setenv("ZGI_AGENT_RUNTIME_PROFILES_JSON", `{}`)
	if _, err := workerConfigFromEnv(); err == nil {
		t.Fatal("incomplete worker config was accepted")
	}
}

func TestWorkerConfigParsesApprovedProfilesAndLimits(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_BACKEND", localBackendName)
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "dedicated-secret")
	t.Setenv("ZGI_AGENT_RUNTIME_DATABASE_URL", "postgres://runtime")
	t.Setenv("ZGI_AGENT_RUNTIME_LISTEN", "127.0.0.1:9876")
	t.Setenv("ZGI_AGENT_RUNTIME_PROFILES_JSON", `{"standard":{"image":"registry.example.invalid/zgi/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)
	t.Setenv("ZGI_AGENT_RUNTIME_MAX_WORKERS", "3")
	t.Setenv("ZGI_AGENT_RUNTIME_IDLE_TTL_SECONDS", "1800")
	t.Setenv("ZGI_AGENT_RUNTIME_STARTUP_TIMEOUT_SECONDS", "7")
	cfg, err := workerConfigFromEnv()
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.MaxWorkers != 3 || cfg.StartupTimeout != 7*time.Second || cfg.ListenAddress != "127.0.0.1:9876" || cfg.Profiles["standard"].Image == "" {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.APIKey == os.Getenv("ZGI_SANDBOX_API_KEY") && cfg.APIKey != "" {
		t.Fatal("worker reused the ordinary Sandbox API key")
	}
}

func TestWorkerConfigRejectsNonPositiveLocalStartupTimeout(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_BACKEND", localBackendName)
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "dedicated-secret")
	t.Setenv("ZGI_AGENT_RUNTIME_DATABASE_URL", "postgres://runtime")
	t.Setenv("ZGI_AGENT_RUNTIME_PROFILES_JSON", `{"standard":{"image":"registry.example.invalid/zgi/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)
	t.Setenv("ZGI_AGENT_RUNTIME_STARTUP_TIMEOUT_SECONDS", "0")
	if _, err := workerConfigFromEnv(); err == nil {
		t.Fatal("non-positive local startup timeout was accepted")
	}
}

func TestWorkerConfigParsesE2BWithoutLocalRuntimeSettings(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_BACKEND", e2bBackendName)
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "dedicated-secret")
	t.Setenv("ZGI_AGENT_RUNTIME_DATABASE_URL", "postgres://runtime")
	t.Setenv("ZGI_AGENT_RUNTIME_PROFILES_JSON", "")
	t.Setenv("ZGI_AGENT_RUNTIME_E2B_API_KEY", "e2b-provider-secret")
	t.Setenv("ZGI_AGENT_RUNTIME_E2B_PROFILES_JSON", `{"standard":{"template_id":"zgi-runtime-template"}}`)
	t.Setenv("ZGI_AGENT_RUNTIME_E2B_TIMEOUT_SECONDS", "1800")
	cfg, err := workerConfigFromEnv()
	if err != nil {
		t.Fatalf("parse E2B config: %v", err)
	}
	if cfg.Backend != e2bBackendName || cfg.E2BProfiles["standard"].TemplateID != "zgi-runtime-template" || len(cfg.Profiles) != 0 {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestWorkerConfigCheckpointStorageIsExclusive(t *testing.T) {
	t.Setenv("ZGI_AGENT_RUNTIME_BACKEND", localBackendName)
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "dedicated-secret")
	t.Setenv("ZGI_AGENT_RUNTIME_DATABASE_URL", "postgres://runtime")
	t.Setenv("ZGI_AGENT_RUNTIME_PROFILES_JSON", `{"standard":{"image":"registry.example.invalid/zgi/runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)
	t.Setenv("ZGI_AGENT_RUNTIME_CHECKPOINT_DIR", "/var/lib/zgi-checkpoints")
	t.Setenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_BUCKET", "checkpoints")
	t.Setenv("ZGI_AGENT_RUNTIME_CHECKPOINT_S3_REGION", "us-east-1")
	if _, err := workerConfigFromEnv(); err == nil {
		t.Fatal("two checkpoint stores were accepted")
	}
	t.Setenv("ZGI_AGENT_RUNTIME_CHECKPOINT_DIR", "")
	cfg, err := workerConfigFromEnv()
	if err != nil || cfg.CheckpointS3.Bucket != "checkpoints" {
		t.Fatalf("S3 checkpoint config = %+v, %v", cfg.CheckpointS3, err)
	}
}

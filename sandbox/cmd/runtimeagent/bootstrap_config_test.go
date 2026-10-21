package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

func TestBootstrapConfigAppliesApprovedSourcesWithoutTemplateEnvironment(t *testing.T) {
	for _, name := range []string{"ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED", "ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON", "ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON", "ZGI_RUNTIME_AGENT_PROFILE_IDENTITY", "ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK"} {
		t.Setenv(name, "")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	config := packagesource.Bootstrap{PlatformCLI: true, Sources: packagesource.Config{PyPI: "https://mirror.example/simple"}, AllowedHosts: []string{"mirror.example"}, ProfileIdentity: "immutable-template", Check: true}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeBootstrapConfig(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON") != config.Sources.JSON() || os.Getenv("ZGI_RUNTIME_AGENT_PROFILE_IDENTITY") != config.ProfileIdentity || os.Getenv("ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK") != "true" {
		t.Fatal("bootstrap configuration was not applied")
	}
	if os.Getenv("ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED") != "true" {
		t.Fatal("platform admission was shadowed by template environment")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("bootstrap config was not consumed: %v", err)
	}
}

func TestBootstrapConfigRejectsInvalidAndUnapprovedContent(t *testing.T) {
	for _, content := range []string{`{"unknown":"field"}`, `{"sources":{"pypi":"https://mirror.example/simple"},"allowed_hosts":["pypi.org"]}`, `{} {}`, `{"sources":{"pypi":"https://secret@mirror.example/simple"}}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := runtimeBootstrapConfig(path); err == nil {
			t.Fatalf("accepted %s", content)
		}
	}
}

func TestBootstrapRuntimePolicyOverridesPackageTemplateForOfflineCLI(t *testing.T) {
	for _, key := range []string{"ZGI_RUNTIME_AGENT_PLATFORM_CLI_ENABLED", "ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON", "ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON", "ZGI_RUNTIME_AGENT_PROFILE_IDENTITY", "ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK", "ZGI_RUNTIME_AGENT_NETWORK_MODE", "ZGI_RUNTIME_AGENT_EGRESS_MODE", "ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED", "ZGI_RUNTIME_AGENT_PACKAGE_NETWORK_MODE", "ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_ENABLED", "ZGI_RUNTIME_AGENT_DEPENDENCY_PREPARATION_MODE", "ZGI_RUNTIME_AGENT_JOB_UID", "ZGI_RUNTIME_AGENT_JOB_GID"} {
		t.Setenv(key, "template-value")
	}
	config := packagesource.Bootstrap{PlatformCLI: true, Runtime: &packagesource.RuntimeConfig{NetworkMode: "disabled", EgressMode: "bridge", JobUID: 65532, JobGID: 65532}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeBootstrapConfig(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ZGI_RUNTIME_AGENT_NETWORK_MODE") != "disabled" || os.Getenv("ZGI_RUNTIME_AGENT_EGRESS_MODE") != "bridge" || os.Getenv("ZGI_RUNTIME_AGENT_DEPENDENCY_INSTALLATION_ENABLED") != "false" || os.Getenv("ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON") != "" || os.Getenv("ZGI_RUNTIME_AGENT_JOB_UID") != "65532" {
		t.Fatal("offline policy was shadowed by template")
	}
}

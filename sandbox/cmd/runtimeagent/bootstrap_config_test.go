package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/packagesource"
)

func TestBootstrapConfigAppliesApprovedSourcesWithoutTemplateEnvironment(t *testing.T) {
	for _, name := range []string{"ZGI_RUNTIME_AGENT_PACKAGE_SOURCES_JSON", "ZGI_RUNTIME_AGENT_EGRESS_ALLOWED_HOSTS_JSON", "ZGI_RUNTIME_AGENT_PROFILE_IDENTITY", "ZGI_RUNTIME_AGENT_PACKAGE_SOURCE_CHECK"} {
		t.Setenv(name, "")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	config := packagesource.Bootstrap{Sources: packagesource.Config{PyPI: "https://mirror.example/simple"}, AllowedHosts: []string{"mirror.example"}, ProfileIdentity: "immutable-template", Check: true}
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

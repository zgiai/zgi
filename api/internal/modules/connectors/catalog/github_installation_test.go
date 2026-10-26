package catalog

import (
	"strings"
	"testing"
)

func TestGitHubInstallationCredentialPresentationPreservesSecretField(t *testing.T) {
	definitions, err := BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range definitions {
		if d.ID != "github" {
			continue
		}
		if len(d.KeyFields) != 1 || d.KeyFields[0].Name != "token" || !d.KeyFields[0].Secret || !strings.Contains(d.KeyFields[0].Label, "App installation") || !strings.Contains(d.KeyFields[0].Placeholder, "github_app_installation_v1") {
			t.Fatal("installation credential format is not discoverable")
		}
		return
	}
	t.Fatal("github missing")
}

package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestAgentExecutionDeploymentExamples(t *testing.T) {
	for _, backend := range []string{"e2b", "local"} {
		t.Run(backend, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "..", "..", "sandbox", "deploy", "api."+backend+".env.example")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				key, value, ok := strings.Cut(line, "=")
				if !ok {
					t.Fatalf("malformed example line in %s", path)
				}
				t.Setenv(key, value)
			}
			resolved, err := agentExecutionApprovalsFromEnv().resolve("replace-organization-id", &dto.AgentExecutionRuntime{Enabled: true})
			if err != nil || resolved.ProfileID != "cc-codex-standard" || resolved.PolicyID != "package-install" {
				t.Fatalf("example resolves to %+v, error=%v", resolved, err)
			}
			if err := agentExecutionApprovalsFromEnv().validate("other-org", &dto.AgentExecutionRuntime{Enabled: true}); err == nil {
				t.Fatal("example grants an unapproved organization")
			}
		})
	}
}

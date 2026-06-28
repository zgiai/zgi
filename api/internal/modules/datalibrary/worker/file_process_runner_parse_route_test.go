package worker

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestFileProcessRunnerRejectsForcedProviderOutsideFileExtensionRoute(t *testing.T) {
	runner := NewFileProcessRunner(FileProcessRunnerDeps{
		ContentParsePlanner: routing.NewDefaultPlanner(),
	})
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "vlm", Enabled: true, Adapter: "system_vlm", Engine: contracts.ParseEngineVLM},
			{Name: "local", Enabled: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal, FallbackOnly: true},
		},
	}

	_, _, err := runner.planParseRequest(contracts.ParseRequest{
		FileName: "lesson.docx",
		Profile:  contracts.ParseProfileDatasetIndex,
	}, "vlm", catalog, nil)
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("err=%v", err)
	}
}

package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestFormatParseFailureAddsFileSpecificGuidance(t *testing.T) {
	largePPT := formatParseFailure("deck.pptx", 43*1024*1024, errors.New("provider returned empty content"))
	if !strings.Contains(largePPT.Error(), "文件较大") {
		t.Fatalf("large pptx error=%q", largePPT)
	}

	legacyPPT := formatParseFailure("deck.ppt", 2*1024*1024, errors.New("unsupported format"))
	if !strings.Contains(legacyPPT.Error(), ".pptx") {
		t.Fatalf("legacy ppt error=%q", legacyPPT)
	}
}

func TestProcessingStageForFailure(t *testing.T) {
	if got := processingStageForFailure("quality_check_failed"); got != "review" {
		t.Fatalf("quality stage=%q", got)
	}
	if got := processingStageForFailure("generate_enqueue_failed"); got != "chunk" {
		t.Fatalf("generation stage=%q", got)
	}
}

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

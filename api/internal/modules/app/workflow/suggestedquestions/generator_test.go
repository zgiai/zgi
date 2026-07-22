package suggestedquestions

import (
	"context"
	"testing"
)

func TestGenerateSkipsModelForQueryIndependentWorkflow(t *testing.T) {
	var generator *Generator
	result, err := generator.Generate(context.Background(), GenerateRequest{
		Context: WorkflowContext{
			SkipGeneration:   true,
			AnalysisWarnings: []string{WarningConversationQueryUnused},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Questions) != 0 {
		t.Fatalf("Questions = %#v, want empty", result.Questions)
	}
	if len(result.Warnings) != 1 || result.Warnings[0] != WarningConversationQueryUnused {
		t.Fatalf("Warnings = %#v", result.Warnings)
	}
}

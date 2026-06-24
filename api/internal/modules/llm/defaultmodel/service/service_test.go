package service

import (
	"testing"

	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
)

func TestResolveFromCandidatesPreservesModelLimits(t *testing.T) {
	svc := &defaultModelService{}
	resolved := svc.resolveFromCandidates(
		llmmodelmodel.UseCaseTextChat,
		nil,
		[]*rankedCandidate{
			{
				model: &llmmodelservice.AvailableModel{
					Provider:        "deepseek",
					Name:            "deepseek-chat",
					ContextWindow:   64000,
					MaxOutputTokens: 8192,
					UseCases:        []string{string(llmmodelmodel.UseCaseTextChat)},
				},
			},
		},
	)

	if resolved == nil {
		t.Fatal("resolved model is nil")
	}
	if got, want := resolved.ContextWindow, 64000; got != want {
		t.Fatalf("context window = %d, want %d", got, want)
	}
	if got, want := resolved.MaxOutputTokens, 8192; got != want {
		t.Fatalf("max output tokens = %d, want %d", got, want)
	}
}

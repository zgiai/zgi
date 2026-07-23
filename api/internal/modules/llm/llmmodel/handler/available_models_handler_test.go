package handler

import (
	"testing"

	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
)

func TestIsValidUseCaseAcceptsAgentRuntimeEligibility(t *testing.T) {
	if !isValidUseCase(llmmodelservice.AgentRuntimeUseCase) {
		t.Fatalf("isValidUseCase(%q) = false, want true", llmmodelservice.AgentRuntimeUseCase)
	}
}

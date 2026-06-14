package service

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
)

func TestContainsUseCaseKeepsImageModelsOutOfTextChat(t *testing.T) {
	if containsUseCase(types.StringArray{"image-gen"}, "text-chat") {
		t.Fatal("image-gen model must not match text-chat")
	}
	if !containsUseCase(types.StringArray{"image-gen"}, "image-gen") {
		t.Fatal("image-gen model must match image-gen")
	}
	if containsUseCase(types.StringArray{"text-chat"}, "image-gen") {
		t.Fatal("text-chat model must not match image-gen")
	}
}

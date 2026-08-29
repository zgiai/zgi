package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func TestDeduplicateModelViewsKeepsCatalogModelFirst(t *testing.T) {
	catalog := &model.ModelView{ID: uuid.New(), Provider: "zgi-cloud", Model: "GPT-5", Vendor: "openai"}
	custom := &model.ModelView{ID: uuid.New(), Provider: "custom-openai", Model: " gpt-5 "}
	other := &model.ModelView{ID: uuid.New(), Provider: "custom", Model: "private-model"}

	got := deduplicateModelViews([]*model.ModelView{catalog, custom, other})
	if len(got) != 2 {
		t.Fatalf("deduplicated model count = %d, want 2", len(got))
	}
	if got[0] != catalog {
		t.Fatal("catalog model did not retain precedence over matching custom model")
	}
	if got[1] != other {
		t.Fatal("non-conflicting custom model was removed")
	}
}

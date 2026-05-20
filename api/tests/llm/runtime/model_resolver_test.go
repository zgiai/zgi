package runtime_test

import (
	"context"
	"errors"
	"testing"

	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/ginext/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/ginext/internal/modules/shared/model"
)

type stubDefaultModelGetter struct {
	resolved           *llmdefaultservice.ResolvedModel
	err                error
	lastOrganizationID string
	lastProvider       *string
	lastModel          *string
	lastType           shared_model.ModelType
}

func (s *stubDefaultModelGetter) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType shared_model.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	s.lastOrganizationID = organizationID
	s.lastProvider = explicitProvider
	s.lastModel = explicitModel
	s.lastType = modelType
	return s.resolved, s.err
}

func TestModelResolverResolveUsesExplicitModel(t *testing.T) {
	resolved, err := llmruntime.NewModelResolver(nil).Resolve(
		context.Background(),
		"org-1",
		"openai",
		"gpt-4o",
		shared_model.ModelTypeLLM,
	)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if resolved.Provider != "openai" {
		t.Fatalf("expected provider openai, got %q", resolved.Provider)
	}
	if resolved.Model != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %q", resolved.Model)
	}
}

func TestModelResolverResolveDefaultDelegatesToDefaultModelService(t *testing.T) {
	getter := &stubDefaultModelGetter{
		resolved: &llmdefaultservice.ResolvedModel{
			Provider: "openai",
			Model:    "text-embedding-3-large",
			Source:   llmdefaultservice.SourceExplicit,
		},
	}

	resolved, err := llmruntime.NewModelResolver(getter).ResolveDefault(
		context.Background(),
		"org-1",
		shared_model.ModelTypeEmbedding,
	)
	if err != nil {
		t.Fatalf("ResolveDefault returned error: %v", err)
	}
	if getter.lastOrganizationID != "org-1" {
		t.Fatalf("expected organization id org-1, got %q", getter.lastOrganizationID)
	}
	if getter.lastType != shared_model.ModelTypeEmbedding {
		t.Fatalf("expected model type embedding, got %q", getter.lastType)
	}
	if resolved.Provider != "openai" || resolved.Model != "text-embedding-3-large" {
		t.Fatalf("unexpected resolved model: %+v", resolved)
	}
}

func TestModelResolverResolveDefaultPropagatesErrors(t *testing.T) {
	getter := &stubDefaultModelGetter{
		err: errors.New("boom"),
	}

	_, err := llmruntime.NewModelResolver(getter).ResolveDefault(
		context.Background(),
		"org-1",
		shared_model.ModelTypeRerank,
	)
	if err == nil {
		t.Fatal("expected ResolveDefault to return an error")
	}
}

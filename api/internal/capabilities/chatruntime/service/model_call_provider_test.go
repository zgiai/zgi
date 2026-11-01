package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// A DigitalHuman names only its model. The tool loop still has to know the
// model's provider, or it calls the model without streaming and the answer
// reaches the chat in one piece.
func TestModelCallProviderUsesCatalogProviderWhenRequestNamesOnlyAModel(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(_ context.Context, _ uuid.UUID, provider string, model string) (ModelSpec, bool, error) {
		if provider != "" || model != "deepseek-v4-pro" {
			t.Fatalf("Resolve(%q, %q), want the model alone", provider, model)
		}
		return ModelSpec{Provider: "deepseek", SupportsToolCall: true}, true, nil
	})}
	parts := &chatRequestParts{ModelName: "deepseek-v4-pro"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if parts.Provider != "" {
		t.Fatalf("Provider = %q, the catalog provider must not change routing", parts.Provider)
	}
	if got := modelCallProvider(parts); got != "deepseek" {
		t.Fatalf("modelCallProvider() = %q, want deepseek", got)
	}
}

func TestModelCallProviderKeepsTheRequestedProvider(t *testing.T) {
	parts := &chatRequestParts{Provider: "openai", CatalogProvider: "deepseek"}
	if got := modelCallProvider(parts); got != "openai" {
		t.Fatalf("modelCallProvider() = %q, want the requested provider", got)
	}
	if got := modelCallProvider(nil); got != "" {
		t.Fatalf("modelCallProvider(nil) = %q, want empty", got)
	}
}

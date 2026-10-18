package digitalhuman

import (
	"context"
	"testing"
)

type digitalHumanIconResolverStub struct {
	url string
}

func (s digitalHumanIconResolverStub) GetFileURL(context.Context, string) (string, error) {
	return s.url, nil
}

func TestResolveDefinitionIconUsesAgentCompatibleImageReference(t *testing.T) {
	t.Parallel()

	service := &Service{iconResolver: digitalHumanIconResolverStub{url: "https://files.example/icon.png"}}
	definition := &Definition{IconType: "image", Icon: "file-id", Avatar: "legacy-avatar"}
	service.resolveDefinitionIcon(t.Context(), definition)

	if definition.IconURL != "https://files.example/icon.png" {
		t.Fatalf("icon URL = %q, want resolved file URL", definition.IconURL)
	}
}

func TestResolveDefinitionIconKeepsLegacyAvatarFallback(t *testing.T) {
	t.Parallel()

	service := &Service{}
	definition := &Definition{IconType: "text", Avatar: "https://legacy.example/avatar.png"}
	service.resolveDefinitionIcon(t.Context(), definition)

	if definition.IconURL != definition.Avatar {
		t.Fatalf("icon URL = %q, want legacy avatar %q", definition.IconURL, definition.Avatar)
	}
}

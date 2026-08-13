package client

import (
	"errors"
	"io"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestClientGenerateMusicRejectsNilRequestBeforeResolvingCredentials(t *testing.T) {
	client := &llmClientImpl{}
	err := client.GenerateMusic(t.Context(), "organization-id", nil, io.Discard)
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("GenerateMusic() error = %v, want ErrInvalidRequest", err)
	}
}

func TestClientGenerateLyricsRejectsNilRequestBeforeResolvingCredentials(t *testing.T) {
	client := &llmClientImpl{}
	_, err := client.GenerateLyrics(t.Context(), "organization-id", nil)
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("GenerateLyrics() error = %v, want ErrInvalidRequest", err)
	}
}

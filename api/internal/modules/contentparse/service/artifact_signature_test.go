package service

import (
	"testing"

	"github.com/zgiai/ginext/internal/contracts"
)

func TestProviderSignature(t *testing.T) {
	tests := []struct {
		name        string
		providerKey string
		engine      contracts.ParseEngine
		want        string
	}{
		{name: "unknown", want: "unknown"},
		{name: "engine only", engine: contracts.ParseEngineLocal, want: "local"},
		{name: "provider only", providerKey: "local-provider", want: "local-provider"},
		{name: "provider and engine", providerKey: "local-provider", engine: contracts.ParseEngineLocal, want: "local-provider:local"},
		{name: "trim provider", providerKey: " local-provider ", engine: contracts.ParseEngineLocal, want: "local-provider:local"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProviderSignature(tt.providerKey, tt.engine); got != tt.want {
				t.Fatalf("ProviderSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

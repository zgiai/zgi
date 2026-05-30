package observer

import (
	"context"
	"testing"
)

func TestMetadataWithContextAddsRequestIDWithoutMutatingInput(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req_test")
	input := map[string]any{"status": "ok"}

	metadata := MetadataWithContext(ctx, input)

	if metadata["request_id"] != "req_test" {
		t.Fatalf("expected request ID metadata, got %#v", metadata)
	}
	if _, ok := input["request_id"]; ok {
		t.Fatalf("expected input metadata to remain unchanged, got %#v", input)
	}
}

func TestMetadataWithContextLeavesMetadataWithoutRequestID(t *testing.T) {
	input := map[string]any{"status": "ok"}

	metadata := MetadataWithContext(context.Background(), input)

	if metadata["status"] != "ok" {
		t.Fatalf("expected original metadata, got %#v", metadata)
	}
	if _, ok := metadata["request_id"]; ok {
		t.Fatalf("expected no request ID metadata, got %#v", metadata)
	}
}

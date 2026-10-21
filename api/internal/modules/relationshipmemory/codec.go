package relationshipmemory

import (
	"context"
	"fmt"
	"strings"
)

type EncodedContent struct {
	Payload  string
	Encoding string
	KeyID    *string
}

// MemoryContentCodec keeps the domain API stable when encrypted storage is added.
type MemoryContentCodec interface {
	Encode(ctx context.Context, plaintext string) (EncodedContent, error)
	Decode(ctx context.Context, content EncodedContent) (string, error)
}

type PlainContentCodec struct{}

func (PlainContentCodec) Encode(_ context.Context, plaintext string) (EncodedContent, error) {
	return EncodedContent{Payload: plaintext, Encoding: ContentEncodingPlain}, nil
}

func (PlainContentCodec) Decode(_ context.Context, content EncodedContent) (string, error) {
	if strings.TrimSpace(content.Encoding) != ContentEncodingPlain || content.KeyID != nil {
		return "", fmt.Errorf("unsupported relationship memory content encoding")
	}
	return content.Payload, nil
}

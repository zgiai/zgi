package externalapikey

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateReturnsOpaqueKeyAndHash(t *testing.T) {
	key, prefix, hash, err := Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !strings.HasPrefix(key, "zgi_") || prefix != key[:12] {
		t.Fatalf("Generate() key/prefix = %q/%q", key, prefix)
	}
	if got := Hash(key); got != hash {
		t.Fatalf("Hash(key) = %q, want %q", got, hash)
	}
}

func TestExtractRejectsConflictingCredentials(t *testing.T) {
	_, err := Extract("Bearer zgi_one", "zgi_two")
	if !errors.Is(err, ErrConflictingKeys) {
		t.Fatalf("Extract() error = %v, want ErrConflictingKeys", err)
	}
}

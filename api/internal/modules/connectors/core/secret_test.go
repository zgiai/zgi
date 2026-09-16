package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDestroySecretValuesOverwritesAndDeletesEntries(t *testing.T) {
	const sentinel = "connector-key-sentinel-map-4c98e7"
	values := map[string]string{
		"api_key": sentinel,
		"token":   "another-sensitive-value",
	}

	DestroySecretValues(values)
	DestroySecretValues(values)
	DestroySecretValues(nil)

	if len(values) != 0 {
		t.Fatalf("DestroySecretValues() left %d entries", len(values))
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("json.Marshal(values) error = %v", err)
	}
	for _, rendered := range []string{string(encoded), fmt.Sprint(values)} {
		if strings.Contains(rendered, sentinel) {
			t.Fatalf("destroyed secret map retained sentinel: %q", rendered)
		}
	}
}

func TestDestroySecretBytesWipesAliasedBuffers(t *testing.T) {
	secret := []byte("connector-key-sentinel-bytes-c5bff0")
	alias := secret
	values := map[string][]byte{"api_key": secret}

	DestroySecretBytes(values)
	DestroySecretBytes(values)
	DestroySecretBytes(nil)

	if len(values) != 0 {
		t.Fatalf("DestroySecretBytes() left %d entries", len(values))
	}
	for i, value := range alias {
		if value != 0 {
			t.Fatalf("DestroySecretBytes() left non-zero byte at index %d", i)
		}
	}
}

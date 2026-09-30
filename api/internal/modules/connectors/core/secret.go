package core

import (
	"runtime"
	"strings"
)

// DestroySecretValues replaces every map value before deleting all entries.
// Go strings are immutable, so callers that need an in-place memory wipe
// should decode into mutable byte slices and use DestroySecretBytes instead.
func DestroySecretValues(values map[string]string) {
	for name, value := range values {
		values[name] = strings.Repeat("\x00", len(value))
	}
	runtime.KeepAlive(values)
	for name := range values {
		delete(values, name)
	}
}

// DestroySecretBytes zeroes the backing memory of every value and then deletes
// every entry. It is nil-safe and idempotent.
func DestroySecretBytes(values map[string][]byte) {
	for _, value := range values {
		clear(value)
	}
	runtime.KeepAlive(values)
	for name := range values {
		delete(values, name)
	}
}

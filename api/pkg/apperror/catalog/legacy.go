package catalog

import (
	"fmt"
	"strings"
)

const maxLegacyKeyLength = 128

// LegacyKey identifies an old error code together with its owning namespace.
// Namespacing prevents unrelated protocols that reuse the same numeric value
// from being mapped accidentally. Example: "llm.gateway:40101".
type LegacyKey struct {
	value string
}

// ParseLegacyKey validates an extension or migration-provided legacy key.
func ParseLegacyKey(value string) (LegacyKey, error) {
	key := LegacyKey{value: value}
	if err := key.Validate(); err != nil {
		return LegacyKey{}, err
	}
	return key, nil
}

// MustLegacyKey is intended for static catalog declarations.
func MustLegacyKey(value string) LegacyKey {
	key, err := ParseLegacyKey(value)
	if err != nil {
		panic(err)
	}
	return key
}

func (k LegacyKey) String() string {
	return k.value
}

func (k LegacyKey) Validate() error {
	if k.value == "" {
		return fmt.Errorf("legacy error key is empty")
	}
	if len(k.value) > maxLegacyKeyLength {
		return fmt.Errorf("legacy error key exceeds %d bytes", maxLegacyKeyLength)
	}
	if strings.TrimSpace(k.value) != k.value {
		return fmt.Errorf("legacy error key %q contains surrounding whitespace", k.value)
	}
	namespace, value, found := strings.Cut(k.value, ":")
	if !found || namespace == "" || value == "" || strings.Contains(value, ":") {
		return fmt.Errorf("legacy error key %q must use namespace:value", k.value)
	}
	if err := validateLegacyNamespace(namespace); err != nil {
		return fmt.Errorf("invalid legacy error key %q: %w", k.value, err)
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return fmt.Errorf("legacy error key %q contains an unsupported value character", k.value)
	}
	return nil
}

func validateLegacyNamespace(value string) error {
	segments := strings.Split(value, ".")
	if len(segments) < 2 {
		return fmt.Errorf("namespace must contain at least two lowercase segments")
	}
	for _, segment := range segments {
		if segment == "" || segment[0] < 'a' || segment[0] > 'z' {
			return fmt.Errorf("namespace segment %q must start with a lowercase letter", segment)
		}
		for _, character := range segment[1:] {
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
				continue
			}
			return fmt.Errorf("namespace segment %q contains an unsupported character", segment)
		}
	}
	return nil
}

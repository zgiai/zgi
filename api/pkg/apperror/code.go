package apperror

import (
	"fmt"
	"strings"
)

const maxCodeLength = 128

// Code is the stable, language-neutral identity of an application error.
// Canonical codes use lowercase dot-separated segments, for example
// "llm.provider.timeout". Its representation is private so callers cannot
// bypass ParseCode or MustCode with an unchecked conversion.
type Code struct {
	value string
}

// ParseCode validates a code supplied by tooling or an extension.
func ParseCode(value string) (Code, error) {
	code := Code{value: value}
	if err := code.Validate(); err != nil {
		return Code{}, err
	}
	return code, nil
}

// MustCode validates value and panics when it is not canonical. It is intended
// for static catalog declarations, never for request input.
func MustCode(value string) Code {
	code, err := ParseCode(value)
	if err != nil {
		panic(err)
	}
	return code
}

// String returns the canonical string representation.
func (c Code) String() string {
	return c.value
}

// Valid reports whether c follows the canonical code grammar.
func (c Code) Valid() bool {
	return c.Validate() == nil
}

// Validate checks that a code is stable, lowercase, and dot-separated.
func (c Code) Validate() error {
	value := c.value
	if value == "" {
		return fmt.Errorf("application error code is empty")
	}
	if len(value) > maxCodeLength {
		return fmt.Errorf("application error code exceeds %d bytes", maxCodeLength)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("application error code %q contains surrounding whitespace", value)
	}

	segments := strings.Split(value, ".")
	if len(segments) < 2 {
		return fmt.Errorf("application error code %q must contain at least two segments", value)
	}
	for _, segment := range segments {
		if err := validateCodeSegment(segment); err != nil {
			return fmt.Errorf("invalid application error code %q: %w", value, err)
		}
	}
	return nil
}

func validateCodeSegment(segment string) error {
	if segment == "" {
		return fmt.Errorf("segment is empty")
	}
	if !isLowerASCII(segment[0]) {
		return fmt.Errorf("segment %q must start with a lowercase letter", segment)
	}
	previousUnderscore := false
	for index := 1; index < len(segment); index++ {
		character := segment[index]
		switch {
		case isLowerASCII(character), isASCIIDigit(character):
			previousUnderscore = false
		case character == '_':
			if previousUnderscore || index == len(segment)-1 {
				return fmt.Errorf("segment %q contains an invalid underscore", segment)
			}
			previousUnderscore = true
		default:
			return fmt.Errorf("segment %q contains an unsupported character", segment)
		}
	}
	return nil
}

func isLowerASCII(character byte) bool {
	return character >= 'a' && character <= 'z'
}

func isASCIIDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

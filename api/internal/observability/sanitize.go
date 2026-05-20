package observability

import (
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

const invalidUTF8Replacement = "\uFFFD"

type sanitizedError struct {
	message string
}

func (e sanitizedError) Error() string {
	return e.message
}

// SanitizeString returns a valid UTF-8 string for OTel export.
func SanitizeString(value string) string {
	return strings.ToValidUTF8(value, invalidUTF8Replacement)
}

// SanitizeError returns an error with a valid UTF-8 message for OTel events.
func SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	return sanitizedError{message: SanitizeString(err.Error())}
}

// SanitizeAttributes returns attributes with valid UTF-8 string values.
func SanitizeAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	if len(attrs) == 0 {
		return attrs
	}

	out := make([]attribute.KeyValue, len(attrs))
	copy(out, attrs)

	for i, attr := range out {
		out[i] = sanitizeAttribute(attr)
	}

	return out
}

func sanitizeAttribute(attr attribute.KeyValue) attribute.KeyValue {
	switch attr.Value.Type() {
	case attribute.STRING:
		return attr.Key.String(SanitizeString(attr.Value.AsString()))
	case attribute.STRINGSLICE:
		return attr.Key.StringSlice(sanitizeStringSlice(attr.Value.AsStringSlice()))
	default:
		return attr
	}
}

func sanitizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return values
	}

	out := make([]string, len(values))
	for i, value := range values {
		out[i] = SanitizeString(value)
	}
	return out
}

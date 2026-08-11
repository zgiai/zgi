package apperror

import "strings"

// Param is one immutable scalar value that may later be rendered by a catalog.
// Complex objects are intentionally excluded so callers cannot accidentally
// pass mutable request bodies or provider payloads through the error kernel.
type Param struct {
	name  string
	value any
}

// StringParam creates a string parameter.
func StringParam(name, value string) Param {
	return newParam(name, value)
}

// IntParam creates a signed integer parameter.
func IntParam(name string, value int64) Param {
	return newParam(name, value)
}

// UintParam creates an unsigned integer parameter.
func UintParam(name string, value uint64) Param {
	return newParam(name, value)
}

// FloatParam creates a floating-point parameter.
func FloatParam(name string, value float64) Param {
	return newParam(name, value)
}

// BoolParam creates a boolean parameter.
func BoolParam(name string, value bool) Param {
	return newParam(name, value)
}

func newParam(name string, value any) Param {
	return Param{name: strings.TrimSpace(name), value: value}
}

// Name returns the parameter name.
func (p Param) Name() string {
	return p.name
}

// Value returns the immutable scalar parameter value.
func (p Param) Value() any {
	return p.value
}

func copyParams(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

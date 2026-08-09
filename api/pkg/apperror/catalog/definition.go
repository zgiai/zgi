package catalog

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

// Category is a stable semantic classification. Protocol adapters translate
// it into their own error type while HTTPStatus preserves existing HTTP
// compatibility during migration.
type Category uint8

const (
	CategoryValidation Category = iota + 1
	CategoryAuthentication
	CategoryAuthorization
	CategoryNotFound
	CategoryConflict
	CategoryQuota
	CategoryRateLimit
	CategoryUpstream
	CategoryInternal
)

func (c Category) String() string {
	switch c {
	case CategoryValidation:
		return "validation"
	case CategoryAuthentication:
		return "authentication"
	case CategoryAuthorization:
		return "authorization"
	case CategoryNotFound:
		return "not_found"
	case CategoryConflict:
		return "conflict"
	case CategoryQuota:
		return "quota"
	case CategoryRateLimit:
		return "rate_limit"
	case CategoryUpstream:
		return "upstream"
	case CategoryInternal:
		return "internal"
	default:
		return "unknown"
	}
}

func (c Category) valid() bool {
	return c >= CategoryValidation && c <= CategoryInternal
}

// ParamType restricts public-message substitutions to known scalar types.
// Catalog rendering never calls arbitrary String methods.
type ParamType uint8

const (
	ParamString ParamType = iota + 1
	ParamInteger
	ParamUnsigned
	ParamFloat
	ParamBoolean
)

func (t ParamType) String() string {
	switch t {
	case ParamString:
		return "string"
	case ParamInteger:
		return "integer"
	case ParamUnsigned:
		return "unsigned"
	case ParamFloat:
		return "float"
	case ParamBoolean:
		return "boolean"
	default:
		return "unknown"
	}
}

func (t ParamType) valid() bool {
	return t >= ParamString && t <= ParamBoolean
}

// Parameter declares one explicitly public substitution. Values not declared
// here are ignored even if they exist in an AppError's diagnostic parameters.
type Parameter struct {
	Name string
	Type ParamType
}

// Definition is the reviewable source of truth for one product error.
// Messages use {parameter_name} placeholders declared in Parameters.
type Definition struct {
	Code        apperror.Code
	Category    Category
	HTTPStatus  int
	Retryable   bool
	Messages    map[Locale]string
	Parameters  []Parameter
	LegacyCodes []LegacyKey
}

// Presentation is safe public content plus stable machine-readable metadata.
type Presentation struct {
	Code       apperror.Code
	Message    string
	Locale     Locale
	Category   Category
	HTTPStatus int
	Retryable  bool
}

type compiledDefinition struct {
	definition Definition
	templates  map[Locale]compiledTemplate
	parameters map[string]ParamType
}

type compiledTemplate struct {
	raw   string
	parts []templatePart
}

type templatePart struct {
	literal   string
	parameter string
}

func compileDefinition(definition Definition) (compiledDefinition, error) {
	if err := definition.Code.Validate(); err != nil {
		return compiledDefinition{}, fmt.Errorf("invalid code: %w", err)
	}
	if !definition.Category.valid() {
		return compiledDefinition{}, fmt.Errorf("code %s has invalid category", definition.Code)
	}
	if definition.HTTPStatus < 400 || definition.HTTPStatus > 599 {
		return compiledDefinition{}, fmt.Errorf("code %s has invalid HTTP status %d", definition.Code, definition.HTTPStatus)
	}

	parameters := make(map[string]ParamType, len(definition.Parameters))
	for _, parameter := range definition.Parameters {
		if !validParameterName(parameter.Name) {
			return compiledDefinition{}, fmt.Errorf("code %s has invalid public parameter %q", definition.Code, parameter.Name)
		}
		if !parameter.Type.valid() {
			return compiledDefinition{}, fmt.Errorf("code %s parameter %q has invalid type", definition.Code, parameter.Name)
		}
		if _, exists := parameters[parameter.Name]; exists {
			return compiledDefinition{}, fmt.Errorf("code %s repeats public parameter %q", definition.Code, parameter.Name)
		}
		parameters[parameter.Name] = parameter.Type
	}

	for locale := range definition.Messages {
		if !isSupportedLocale(locale) {
			return compiledDefinition{}, fmt.Errorf("code %s contains unsupported message locale %q", definition.Code, locale)
		}
	}
	templates := make(map[Locale]compiledTemplate, len(SupportedLocales()))
	for _, locale := range SupportedLocales() {
		message, exists := definition.Messages[locale]
		if !exists || strings.TrimSpace(message) == "" {
			return compiledDefinition{}, fmt.Errorf("code %s is missing public message for %s", definition.Code, locale)
		}
		template, used, err := compileTemplate(message)
		if err != nil {
			return compiledDefinition{}, fmt.Errorf("code %s message for %s: %w", definition.Code, locale, err)
		}
		if err := validateTemplateParameters(parameters, used); err != nil {
			return compiledDefinition{}, fmt.Errorf("code %s message for %s: %w", definition.Code, locale, err)
		}
		templates[locale] = template
	}

	return compiledDefinition{
		definition: cloneDefinition(definition),
		templates:  templates,
		parameters: parameters,
	}, nil
}

func compileTemplate(message string) (compiledTemplate, map[string]struct{}, error) {
	if len(message) > 1024 {
		return compiledTemplate{}, nil, fmt.Errorf("public message exceeds 1024 bytes")
	}
	used := make(map[string]struct{})
	parts := make([]templatePart, 0, 4)
	remaining := message
	for {
		opening := strings.IndexByte(remaining, '{')
		closing := strings.IndexByte(remaining, '}')
		if opening < 0 {
			if closing >= 0 {
				return compiledTemplate{}, nil, fmt.Errorf("contains an unmatched closing brace")
			}
			if remaining != "" {
				parts = append(parts, templatePart{literal: remaining})
			}
			break
		}
		if closing >= 0 && closing < opening {
			return compiledTemplate{}, nil, fmt.Errorf("contains an unmatched closing brace")
		}
		end := strings.IndexByte(remaining[opening+1:], '}')
		if end < 0 {
			return compiledTemplate{}, nil, fmt.Errorf("contains an unmatched opening brace")
		}
		end += opening + 1
		name := remaining[opening+1 : end]
		if !validParameterName(name) {
			return compiledTemplate{}, nil, fmt.Errorf("contains invalid placeholder %q", name)
		}
		if opening > 0 {
			parts = append(parts, templatePart{literal: remaining[:opening]})
		}
		parts = append(parts, templatePart{parameter: name})
		used[name] = struct{}{}
		remaining = remaining[end+1:]
	}
	return compiledTemplate{raw: message, parts: parts}, used, nil
}

func validateTemplateParameters(declared map[string]ParamType, used map[string]struct{}) error {
	for name := range used {
		if _, exists := declared[name]; !exists {
			return fmt.Errorf("placeholder %q is not declared as public", name)
		}
	}
	for name := range declared {
		if _, exists := used[name]; !exists {
			return fmt.Errorf("public parameter %q is not used", name)
		}
	}
	return nil
}

func validParameterName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, character := range name[1:] {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}

func (t compiledTemplate) render(schema map[string]ParamType, values map[string]any) (string, error) {
	if len(t.parts) == 1 && t.parts[0].parameter == "" {
		return t.raw, nil
	}
	var result strings.Builder
	result.Grow(len(t.raw))
	for _, part := range t.parts {
		if part.parameter == "" {
			result.WriteString(part.literal)
			continue
		}
		value, exists := values[part.parameter]
		if !exists {
			return "", fmt.Errorf("public parameter %q is missing", part.parameter)
		}
		rendered, err := formatParameter(schema[part.parameter], value)
		if err != nil {
			return "", fmt.Errorf("public parameter %q: %w", part.parameter, err)
		}
		result.WriteString(rendered)
	}
	return result.String(), nil
}

func formatParameter(parameterType ParamType, value any) (string, error) {
	switch parameterType {
	case ParamString:
		if typed, ok := value.(string); ok {
			return typed, nil
		}
	case ParamInteger:
		if typed, ok := value.(int64); ok {
			return strconv.FormatInt(typed, 10), nil
		}
	case ParamUnsigned:
		if typed, ok := value.(uint64); ok {
			return strconv.FormatUint(typed, 10), nil
		}
	case ParamFloat:
		if typed, ok := value.(float64); ok {
			return strconv.FormatFloat(typed, 'f', -1, 64), nil
		}
	case ParamBoolean:
		if typed, ok := value.(bool); ok {
			return strconv.FormatBool(typed), nil
		}
	}
	return "", fmt.Errorf("must be %s", parameterType)
}

func cloneDefinition(definition Definition) Definition {
	copyOfDefinition := definition
	copyOfDefinition.Messages = make(map[Locale]string, len(definition.Messages))
	for locale, message := range definition.Messages {
		copyOfDefinition.Messages[locale] = message
	}
	copyOfDefinition.Parameters = append([]Parameter(nil), definition.Parameters...)
	copyOfDefinition.LegacyCodes = append([]LegacyKey(nil), definition.LegacyCodes...)
	return copyOfDefinition
}

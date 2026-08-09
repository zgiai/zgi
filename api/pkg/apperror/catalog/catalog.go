package catalog

import (
	"errors"
	"fmt"
	"sort"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

var (
	ErrCodeNotCataloged   = errors.New("application error code is not cataloged")
	ErrMessageUnavailable = errors.New("public application error message is unavailable")
)

// Catalog is immutable after construction and safe for concurrent reads.
type Catalog struct {
	fallbackLocale Locale
	fallbackCode   apperror.Code
	definitions    map[apperror.Code]compiledDefinition
	legacyCodes    map[LegacyKey]apperror.Code
}

// New validates and copies every definition. It fails startup on duplicate
// identities, incomplete translations, unsafe placeholders, or ambiguous
// legacy aliases instead of discovering catalog drift in a live request.
func New(fallbackLocale Locale, fallbackCode apperror.Code, definitions ...Definition) (*Catalog, error) {
	if !isSupportedLocale(fallbackLocale) {
		return nil, fmt.Errorf("unsupported fallback locale %q", fallbackLocale)
	}
	catalog := &Catalog{
		fallbackLocale: fallbackLocale,
		fallbackCode:   fallbackCode,
		definitions:    make(map[apperror.Code]compiledDefinition, len(definitions)),
		legacyCodes:    make(map[LegacyKey]apperror.Code),
	}
	for _, definition := range definitions {
		compiled, err := compileDefinition(definition)
		if err != nil {
			return nil, err
		}
		if _, exists := catalog.definitions[definition.Code]; exists {
			return nil, fmt.Errorf("duplicate application error code %s", definition.Code)
		}
		for _, legacyCode := range definition.LegacyCodes {
			if err := legacyCode.Validate(); err != nil {
				return nil, fmt.Errorf("code %s: %w", definition.Code, err)
			}
			if existing, exists := catalog.legacyCodes[legacyCode]; exists {
				return nil, fmt.Errorf("legacy error key %s maps to both %s and %s", legacyCode, existing, definition.Code)
			}
			catalog.legacyCodes[legacyCode] = definition.Code
		}
		catalog.definitions[definition.Code] = compiled
	}
	fallback, exists := catalog.definitions[fallbackCode]
	if !exists {
		return nil, fmt.Errorf("fallback code %s: %w", fallbackCode, ErrCodeNotCataloged)
	}
	if len(fallback.parameters) != 0 {
		return nil, fmt.Errorf("fallback code %s must not require public parameters", fallbackCode)
	}
	return catalog, nil
}

// Present renders one cataloged error. Unsupported locales use the catalog's
// fallback locale. Undeclared diagnostic parameters are never rendered.
func (c *Catalog) Present(code apperror.Code, locale Locale, params map[string]any) (Presentation, error) {
	definition, exists := c.definitions[code]
	if !exists {
		return Presentation{}, fmt.Errorf("%w: %s", ErrCodeNotCataloged, code)
	}
	locale = c.effectiveLocale(locale)
	message, err := definition.templates[locale].render(definition.parameters, params)
	if err != nil {
		return Presentation{}, fmt.Errorf("%w for %s: %v", ErrMessageUnavailable, code, err)
	}
	return presentationFrom(definition.definition, locale, message), nil
}

// Fallback returns the always-renderable generic public error. Adapters use it
// when an unknown code or bad parameter reaches a protocol boundary.
func (c *Catalog) Fallback(locale Locale) Presentation {
	definition := c.definitions[c.fallbackCode]
	locale = c.effectiveLocale(locale)
	message, _ := definition.templates[locale].render(definition.parameters, nil)
	return presentationFrom(definition.definition, locale, message)
}

// Definition returns a defensive copy suitable for docs and diagnostics.
func (c *Catalog) Definition(code apperror.Code) (Definition, bool) {
	definition, exists := c.definitions[code]
	if !exists {
		return Definition{}, false
	}
	return cloneDefinition(definition.definition), true
}

// Definitions returns defensive copies in code order for documentation,
// startup diagnostics, and completeness tooling. Request handling should use
// Present instead of scanning this list.
func (c *Catalog) Definitions() []Definition {
	definitions := make([]Definition, 0, len(c.definitions))
	for _, definition := range c.definitions {
		definitions = append(definitions, cloneDefinition(definition.definition))
	}
	sort.Slice(definitions, func(left, right int) bool {
		return definitions[left].Code.String() < definitions[right].Code.String()
	})
	return definitions
}

// CodeFromLegacy explicitly converts a namespaced old code. Unknown or
// intentionally ambiguous old values do not receive a guessed meaning.
func (c *Catalog) CodeFromLegacy(key LegacyKey) (apperror.Code, bool) {
	code, exists := c.legacyCodes[key]
	return code, exists
}

func (c *Catalog) effectiveLocale(locale Locale) Locale {
	if !isSupportedLocale(locale) {
		return c.fallbackLocale
	}
	return locale
}

func presentationFrom(definition Definition, locale Locale, message string) Presentation {
	return Presentation{
		Code:       definition.Code,
		Message:    message,
		Locale:     locale,
		Category:   definition.Category,
		HTTPStatus: definition.HTTPStatus,
		Retryable:  definition.Retryable,
	}
}

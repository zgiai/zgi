// Package transport projects transport-neutral application errors into safe
// public presentations. Protocol packages remain responsible for their wire
// shape, status overrides, headers, and legacy response codes.
package transport

import (
	"errors"

	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var ErrCatalogRequired = errors.New("application error catalog is required")

// Resolution explains whether a public presentation came from the requested
// application error or from the catalog's safe fallback. It is diagnostic
// metadata and must not replace the original error in logs or tracing.
type Resolution uint8

const (
	ResolutionMatched Resolution = iota + 1
	ResolutionUnknownError
	ResolutionMessageUnavailable
	ResolutionLegacyMismatch
)

func (r Resolution) String() string {
	switch r {
	case ResolutionMatched:
		return "matched"
	case ResolutionUnknownError:
		return "unknown_error"
	case ResolutionMessageUnavailable:
		return "message_unavailable"
	case ResolutionLegacyMismatch:
		return "legacy_mismatch"
	default:
		return "unknown"
	}
}

// Result is a safe, protocol-neutral projection. The original error remains
// with the caller so observability can preserve its cause and operation.
type Result struct {
	Presentation catalog.Presentation
	Resolution   Resolution
}

// LegacyMessage contains only fields that are safe to overlay on an existing
// legacy protocol response. The caller must preserve that protocol's existing
// HTTP status, error code/type, and response shape.
type LegacyMessage struct {
	AppCode    apperror.Code
	Message    string
	Locale     catalog.Locale
	Retryable  bool
	Resolution Resolution
}

// Projector is immutable after construction and safe for concurrent use.
type Projector struct {
	catalog *catalog.Catalog
}

func NewProjector(productCatalog *catalog.Catalog) (*Projector, error) {
	if productCatalog == nil {
		return nil, ErrCatalogRequired
	}
	return &Projector{catalog: productCatalog}, nil
}

// Project returns a safe public presentation for err. Unknown errors and
// rendering failures use the catalog fallback instead of exposing err.Error().
func (p *Projector) Project(err error, locale catalog.Locale) Result {
	appErr, ok := apperror.As(err)
	if !ok {
		return p.fallback(locale, ResolutionUnknownError)
	}
	return p.projectAppError(appErr, locale)
}

// ProjectLegacyMessage verifies that legacyKey belongs to err's canonical
// application error before returning a message overlay. This prevents numeric
// codes reused by different protocols from silently selecting the wrong text.
// Callers must keep their established HTTP status and wire code/type.
func (p *Projector) ProjectLegacyMessage(err error, locale catalog.Locale, legacyKey catalog.LegacyKey) LegacyMessage {
	appErr, ok := apperror.As(err)
	if !ok {
		return legacyMessageFrom(p.fallback(locale, ResolutionUnknownError))
	}

	mappedCode, mapped := p.catalog.CodeFromLegacy(legacyKey)
	if !mapped || mappedCode != appErr.Code() {
		return legacyMessageFrom(p.fallback(locale, ResolutionLegacyMismatch))
	}
	return legacyMessageFrom(p.projectAppError(appErr, locale))
}

func (p *Projector) projectAppError(appErr *apperror.Error, locale catalog.Locale) Result {
	presentation, presentErr := p.catalog.Present(appErr.Code(), locale, appErr.Params())
	if presentErr != nil {
		return p.fallback(locale, ResolutionMessageUnavailable)
	}
	return Result{Presentation: presentation, Resolution: ResolutionMatched}
}

func (p *Projector) fallback(locale catalog.Locale, resolution Resolution) Result {
	return Result{
		Presentation: p.catalog.Fallback(locale),
		Resolution:   resolution,
	}
}

func legacyMessageFrom(result Result) LegacyMessage {
	return LegacyMessage{
		AppCode:    result.Presentation.Code,
		Message:    result.Presentation.Message,
		Locale:     result.Presentation.Locale,
		Retryable:  result.Presentation.Retryable,
		Resolution: result.Resolution,
	}
}

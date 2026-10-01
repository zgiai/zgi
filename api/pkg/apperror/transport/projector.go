// Package transport projects transport-neutral application errors into safe
// public presentations. Protocol packages remain responsible for their wire
// shape, status overrides, headers, and legacy response codes.
package transport

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var ErrCatalogRequired = errors.New("application error catalog is required")

// HeaderApplicationErrorCode carries a cataloged, language-neutral application
// error identity alongside a legacy response body. Protocol adapters may set
// it only after a successful projection; legacy numeric codes remain unchanged.
// The frozen error catalog lists this exact name in
// envelope.legacy_fields_preserved, and web/src/utils/auth-errors.ts reads it
// to disambiguate invite-registration rejections that reuse a legacy code.
const HeaderApplicationErrorCode = "X-ZGI-App-Error-Code"

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
	// ErrorID is generated once per projection at this boundary and is safe to
	// expose to clients for support correlation.
	ErrorID string
}

// ProjectionLog is the diagnostic record emitted for one projection. It keeps
// the original error and its operation so operators can trace a public error_id
// back to the concrete cause without exposing that cause to the client.
type ProjectionLog struct {
	ErrorID string
	// RequestID is the transport request identity, when the caller has one. It
	// lets support go from a request_id in the logs to the error_id shown to the
	// client, and back, without a database column for either.
	RequestID  string
	Code       apperror.Code
	Operation  string
	Cause      error
	Resolution Resolution
}

// ProjectionLogger records a projection at the boundary. Bootstrap wires the
// process logger; tests may inject a recorder or leave it nil.
type ProjectionLogger func(ProjectionLog)

// Option customizes a Projector without changing its immutable public contract.
type Option func(*Projector)

// WithIDGenerator overrides error_id generation. It exists for deterministic
// tests; production uses random identifiers.
func WithIDGenerator(generator func() string) Option {
	return func(projector *Projector) {
		if generator != nil {
			projector.newID = generator
		}
	}
}

// WithProjectionLogger replaces the diagnostic sink for generated error_ids.
func WithProjectionLogger(log ProjectionLogger) Option {
	return func(projector *Projector) {
		if log != nil {
			projector.logger = log
		}
	}
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
	newID   func() string
	logger  ProjectionLogger
}

func NewProjector(productCatalog *catalog.Catalog, options ...Option) (*Projector, error) {
	if productCatalog == nil {
		return nil, ErrCatalogRequired
	}
	projector := &Projector{
		catalog: productCatalog,
		newID:   defaultErrorID,
		logger:  defaultProjectionLogger,
	}
	for _, option := range options {
		if option != nil {
			option(projector)
		}
	}
	return projector, nil
}

// Registered reports whether code is an application code in this catalog.
// An edge relaying another edge's envelope checks it before echoing that code
// as its own, so an arbitrary upstream string never becomes a contract code.
func (p *Projector) Registered(code string) bool {
	if p == nil || p.catalog == nil {
		return false
	}
	parsed, err := apperror.ParseCode(strings.TrimSpace(code))
	if err != nil {
		return false
	}
	_, ok := p.catalog.Definition(parsed)
	return ok
}

// Project returns a safe public presentation for err. Unknown errors and
// rendering failures use the catalog fallback instead of exposing err.Error().
// It always assigns a fresh error_id and emits one diagnostic record.
func (p *Projector) Project(err error, locale catalog.Locale) Result {
	return p.ProjectWithRequestID(err, locale, "")
}

// ProjectWithRequestID behaves like Project and additionally records the
// transport request id on the diagnostic record, so one log line carries both
// the public error_id and the request_id used to correlate the two edges.
func (p *Projector) ProjectWithRequestID(err error, locale catalog.Locale, requestID string) Result {
	appErr, ok := apperror.As(err)
	var result Result
	if !ok {
		result = p.fallback(locale, ResolutionUnknownError)
	} else {
		result = p.projectAppError(appErr, locale)
	}
	result.ErrorID = p.newID()
	p.logProjection(result, appErr, err, strings.TrimSpace(requestID))
	return result
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

func (p *Projector) logProjection(result Result, appErr *apperror.Error, err error, requestID string) {
	if p.logger == nil {
		return
	}
	record := ProjectionLog{
		ErrorID:    result.ErrorID,
		RequestID:  requestID,
		Code:       result.Presentation.Code,
		Cause:      err,
		Resolution: result.Resolution,
	}
	if appErr != nil {
		record.Operation = appErr.Operation()
	}
	p.logger(record)
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

func defaultErrorID() string {
	return uuid.NewString()
}

// defaultProjectionLogger emits structured diagnostics only when the process
// logger is already installed. It deliberately avoids lazy file initialization
// so unit tests and short-lived commands never create log files as a side effect.
func defaultProjectionLogger(record ProjectionLog) {
	if !logger.IsInitialized() {
		return
	}
	fields := map[string]interface{}{
		"error_id":   record.ErrorID,
		"code":       record.Code.String(),
		"resolution": record.Resolution.String(),
	}
	if record.RequestID != "" {
		fields["request_id"] = record.RequestID
	}
	if record.Operation != "" {
		fields["operation"] = record.Operation
	}
	if record.Cause != nil {
		fields["cause"] = record.Cause.Error()
	}
	logger.Error("application error projected", fields)
}

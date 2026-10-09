package service

import (
	"sync/atomic"

	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// applicationErrorCatalog holds the process-wide catalog composed at bootstrap.
// The chat-runtime stream payload builder is a pure function used from several
// packages, so the catalog is published here once, before the server serves
// traffic, instead of being threaded through every caller.
var applicationErrorCatalog atomic.Pointer[appcatalog.Catalog]

// SetApplicationErrorCatalog publishes the composed application error catalog to
// the chat-runtime package. Bootstrap calls it once; passing nil clears it.
func SetApplicationErrorCatalog(catalog *appcatalog.Catalog) {
	applicationErrorCatalog.Store(catalog)
}

// streamErrorRetryable resolves the catalog-owned retryable flag for a stable
// application error code. It reports false when the code is unknown or the
// catalog has not been published, so callers never invent a retry guidance.
func streamErrorRetryable(code string) (retryable bool, known bool) {
	catalog := applicationErrorCatalog.Load()
	if catalog == nil || code == "" {
		return false, false
	}
	parsed, err := apperror.ParseCode(code)
	if err != nil {
		return false, false
	}
	definition, ok := catalog.Definition(parsed)
	if !ok {
		return false, false
	}
	return definition.Retryable, true
}

// streamErrorCatalogMessage renders the catalog's public message for a stable
// application error code in the given locale. It reports false when the code is
// unknown or the catalog has not been published, so callers keep their own
// message.
func streamErrorCatalogMessage(code string, locale appcatalog.Locale) (string, bool) {
	catalog := applicationErrorCatalog.Load()
	if catalog == nil || code == "" {
		return "", false
	}
	parsed, err := apperror.ParseCode(code)
	if err != nil {
		return "", false
	}
	presentation, err := catalog.Present(parsed, locale, nil)
	if err != nil || presentation.Message == "" {
		return "", false
	}
	return presentation.Message, true
}

// CatalogDefinitions returns chat-runtime-owned product error definitions.
func CatalogDefinitions() []appcatalog.Definition {
	return nil
}

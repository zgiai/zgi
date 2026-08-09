// Package apperror defines ZGI's transport-neutral application error kernel.
//
// An Error carries a stable machine-readable Code, an optional underlying
// cause, diagnostic operation context, and immutable scalar parameters. It
// deliberately has no dependency on HTTP, Gin, localization, persistence, or
// observability providers. Those concerns belong to adapters built on top of
// this package.
//
// This package defines how an error code behaves; it is not the product error
// code catalog. Product codes, safe public messages, translations, and legacy
// numeric-code mappings belong to the catalog layer so they can be reviewed
// and completeness-checked in one place.
//
// Error text is diagnostic and must never be returned directly to an API
// caller. User-facing messages are rendered by the error catalog layer.
package apperror

// Package apperror defines ZGI's transport-neutral application error kernel.
//
// An Error carries a stable machine-readable Code, an optional underlying
// cause, diagnostic operation context, and immutable scalar parameters. It
// deliberately has no dependency on HTTP, Gin, localization, persistence, or
// observability providers. Those concerns belong to adapters built on top of
// this package.
//
// Error text is diagnostic and must never be returned directly to an API
// caller. User-facing messages are rendered by the error catalog layer.
package apperror

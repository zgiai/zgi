// Package catalog provides the immutable, reviewable product catalog for
// application errors.
//
// The package sits above pkg/apperror: apperror owns error identity and cause
// preservation, while catalog owns safe public messages, locale fallback,
// transport hints, and explicit legacy-code aliases. A Catalog is assembled
// during process startup and is safe for concurrent reads afterward.
//
// Catalog messages are public API content. They must explain what the user can
// do next and must never contain an internal cause, provider payload, prompt,
// credential, or other sensitive value.
package catalog

package fxapp

import (
	chatruntimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// provideApplicationErrorCatalog composes independently owned definitions
// once during bootstrap. The returned catalog is immutable and can be injected
// into protocol adapters without introducing a mutable global registry.
func provideApplicationErrorCatalog() (*appcatalog.Catalog, error) {
	definitions := appcatalog.DefaultDefinitions()
	definitions = append(definitions, chatruntimeservice.CatalogDefinitions()...)
	definitions = append(definitions, llmerrors.CatalogDefinitions()...)
	return appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
}

// requireApplicationErrorCatalog makes catalog completeness a startup gate
// even before the first protocol adapter consumes it.
func requireApplicationErrorCatalog(*appcatalog.Catalog) {}

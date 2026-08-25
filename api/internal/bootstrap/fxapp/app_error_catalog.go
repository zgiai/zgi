package fxapp

import (
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	musicmodule "github.com/zgiai/zgi/api/internal/modules/music"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// provideApplicationErrorCatalog composes independently owned definitions
// once during bootstrap. The returned catalog is immutable and can be injected
// into protocol adapters without introducing a mutable global registry.
func provideApplicationErrorCatalog() (*appcatalog.Catalog, error) {
	definitions := appcatalog.DefaultDefinitions()
	definitions = append(definitions, imageservice.CatalogDefinitions()...)
	definitions = append(definitions, llmerrors.CatalogDefinitions()...)
	definitions = append(definitions, musicmodule.CatalogDefinitions()...)
	return appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
}

// requireApplicationErrorCatalog makes catalog completeness a startup gate
// even before the first protocol adapter consumes it.
func requireApplicationErrorCatalog(*appcatalog.Catalog) {}

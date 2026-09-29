package fxapp

import (
	chatruntimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	agents "github.com/zgiai/zgi/api/internal/modules/app/agents"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorpreferences "github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	musicmodule "github.com/zgiai/zgi/api/internal/modules/music"
	authservice "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// provideApplicationErrorCatalog composes independently owned definitions
// once during bootstrap. The returned catalog is immutable and can be injected
// into protocol adapters without introducing a mutable global registry.
func provideApplicationErrorCatalog() (*appcatalog.Catalog, error) {
	definitions := appcatalog.DefaultDefinitions()
	definitions = append(definitions, chatruntimeservice.CatalogDefinitions()...)
	definitions = append(definitions, agents.CatalogDefinitions()...)
	definitions = append(definitions, connectorpreferences.CatalogDefinitions()...)
	definitions = append(definitions, connectorcore.CatalogDefinitions()...)
	definitions = append(definitions, digitalhuman.CatalogDefinitions()...)
	definitions = append(definitions, imageservice.CatalogDefinitions()...)
	definitions = append(definitions, llmerrors.CatalogDefinitions()...)
	definitions = append(definitions, musicmodule.CatalogDefinitions()...)
	definitions = append(definitions, authservice.CatalogDefinitions()...)
	return appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
}

// requireApplicationErrorCatalog makes catalog completeness a startup gate
// even before the first protocol adapter consumes it.
func requireApplicationErrorCatalog(*appcatalog.Catalog) {}

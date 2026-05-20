package service

import (
	"github.com/samber/do/v2"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/columns"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/indexes"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/query"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/tables"
)

// ProvideTablesService registers the tables service for injection.
func ProvideTablesService(i do.Injector) (TablesService, error) {
	repo := do.MustInvoke[tables.Repository](i)
	return NewTablesService(repo), nil
}

// ProvideSchemasService registers the schemas service for injection.
func ProvideSchemasService(i do.Injector) (SchemasService, error) {
	repo := do.MustInvoke[schemas.Repository](i)
	return NewSchemasService(repo), nil
}

// ProvideColumnsService registers the columns service for injection.
func ProvideColumnsService(i do.Injector) (ColumnsService, error) {
	repo := do.MustInvoke[columns.Repository](i)
	return NewColumnsService(repo), nil
}

// ProvideIndexesService registers the indexes service for injection.
func ProvideIndexesService(i do.Injector) (IndexesService, error) {
	repo := do.MustInvoke[indexes.Repository](i)
	return NewIndexesService(repo), nil
}

// ProvideQueryService registers the query service for injection.
func ProvideQueryService(i do.Injector) (QueryService, error) {
	repo := do.MustInvoke[query.Repository](i)
	return NewQueryService(repo), nil
}

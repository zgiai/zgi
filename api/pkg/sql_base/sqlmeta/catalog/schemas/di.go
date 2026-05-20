package schemas

import (
	"github.com/samber/do/v2"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/driver"
)

// ProvideRepository registers the schema repository with the injector.
func ProvideRepository(i do.Injector) (Repository, error) {
	pool := do.MustInvoke[*driver.Pool](i)
	return NewRepository(pool), nil
}

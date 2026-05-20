package indexes

import (
	"github.com/samber/do/v2"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
)

// ProvideRepository registers the indexes repository with the dependency injector.
func ProvideRepository(i do.Injector) (Repository, error) {
	pool := do.MustInvoke[*driver.Pool](i)
	return NewRepository(pool), nil
}

package time

import (
	"github.com/zgiai/ginext/internal/modules/tools/builtin"
)

func init() {
	// Register TimeProvider to builtin registry
	builtin.Register(NewTimeProvider())
}

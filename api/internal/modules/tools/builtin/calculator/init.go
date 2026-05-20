package calculator

import "github.com/zgiai/zgi/api/internal/modules/tools/builtin"

func init() {
	builtin.Register(NewProvider())
}

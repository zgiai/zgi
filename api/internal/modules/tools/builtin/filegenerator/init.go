package filegenerator

import "github.com/zgiai/ginext/internal/modules/tools/builtin"

func init() {
	builtin.Register(NewProvider())
}

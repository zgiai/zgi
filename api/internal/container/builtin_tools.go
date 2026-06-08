package container

import (
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"

	// Import builtin tools to trigger init() registration
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/calculator"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/chartgenerator"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/filegenerator"
	_ "github.com/zgiai/zgi/api/internal/modules/tools/builtin/time"
)

// getBuiltinToolProviders returns all registered builtin tool providers
// This function is called during ToolManager initialization
func getBuiltinToolProviders() []tools.ToolProvider {
	return builtin.GetAllProviders()
}

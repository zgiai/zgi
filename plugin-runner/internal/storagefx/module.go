package storagefx

import (
	"go.uber.org/fx"

	"plugin_runner/internal/config"
	"plugin_runner/internal/storage"
	localstore "plugin_runner/internal/storage/local"
)

// Module wires the storage implementation into Fx.
var Module = fx.Provide(
	func(cfg *config.Config) storage.Store {
		return localstore.NewStore(cfg.WorkspacePath, cfg.PackageCachePath)
	},
)

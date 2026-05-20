package main

import (
	"go.uber.org/fx"

	"plugin_runner/internal/cache"
	"plugin_runner/internal/config"
	"plugin_runner/internal/dataplane"
	"plugin_runner/internal/logger"
	"plugin_runner/internal/manager"
	"plugin_runner/internal/manager/signaturefx"
	"plugin_runner/internal/registry"
	"plugin_runner/internal/runtime/local"
	"plugin_runner/internal/server"
	"plugin_runner/internal/storagefx"
)

func main() {
	app := fx.New(
		config.Module,
		logger.Module,
		dataplane.Module,
		cache.Module,
		signaturefx.Module,
		registry.Module,
		local.Module,
		storagefx.Module,
		manager.Module,
		server.Module,
	)
	app.Run()
}

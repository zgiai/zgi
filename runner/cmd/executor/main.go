package main

import (
	"go.uber.org/fx"

	"github.com/zgiai/zgi/runner/internal/cache"
	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/dataplane"
	"github.com/zgiai/zgi/runner/internal/logger"
	"github.com/zgiai/zgi/runner/internal/manager"
	"github.com/zgiai/zgi/runner/internal/manager/signaturefx"
	"github.com/zgiai/zgi/runner/internal/registry"
	"github.com/zgiai/zgi/runner/internal/runtime/local"
	"github.com/zgiai/zgi/runner/internal/server"
	"github.com/zgiai/zgi/runner/internal/storagefx"
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

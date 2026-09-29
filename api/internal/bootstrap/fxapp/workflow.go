package fxapp

import (
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/runtime"
	"go.uber.org/fx"
)

var workflowModule = fx.Module("workflow",
	fx.Provide(
		provideWorkflowEngineFactory,
	),
)

func provideWorkflowRuntimeDependencies(serviceContainer *container.ServiceContainer) workflowruntime.Dependencies {
	return serviceContainer.GetWorkflowRuntimeDependencies()
}

func provideWorkflowEngineFactory(serviceContainer *container.ServiceContainer) *graph_engine.EngineFactory {
	return graph_engine.NewLazyEngineFactory(graph_engine.DefaultMaxConcurrency, func() graph_engine.NodeRunner {
		return workflowruntime.NewNodeRunner(provideWorkflowRuntimeDependencies(serviceContainer))
	})
}

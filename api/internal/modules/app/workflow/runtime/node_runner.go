package workflowruntime

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	automationdefinition "github.com/zgiai/ginext/internal/modules/automation/service/definition"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow"
	notificationsms "github.com/zgiai/ginext/internal/modules/notification/sms"
	promptservice "github.com/zgiai/ginext/internal/modules/prompts/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/pkg/logger"
)

// Dependencies are application-scoped services used by concrete workflow nodes.
type Dependencies struct {
	ContentExtractor            file.ContentExtractor
	LLMClient                   interface{}
	ToolEngine                  interface{}
	GraphFlowService            *graphflow.Service
	FileService                 interfaces.FileService
	PromptResolver              promptservice.PromptService
	AutomationDefinitionService automationdefinition.Service
	NotificationSMSService      notificationsms.Service
}

// NodeRunner creates and executes concrete workflow nodes outside graph_engine.
type NodeRunner struct {
	deps                  Dependencies
	subgraphEngineFactory subgraph.EngineFactory
}

func NewNodeRunner(deps Dependencies) *NodeRunner {
	runner := &NodeRunner{deps: deps}
	runner.subgraphEngineFactory = func(parallelism int) subgraph.Engine {
		engine := graph_engine.NewWorkflowEngine(parallelism)
		engine.SetNodeRunner(runner)
		return engine
	}
	return runner
}

func (r *NodeRunner) RunNode(ctx context.Context, req graph_engine.NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	logger.Info("Creating node instance for nodeID: %s, nodeType: %s, config: %+v", req.NodeID, req.NodeType, req.Config)

	nodeFactory, err := nodes.GetNodeFactory(req.NodeType, nodes.LatestVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get node factory for type %s: %w", req.NodeType, err)
	}

	nodeInstance, err := nodeFactory(
		req.NodeID,
		req.Config,
		req.GraphInitParams,
		req.Graph,
		req.RuntimeState,
		nil,
		r.optionalDependencies(req.NodeID, req.NodeType)...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create node instance for type %s: %w", req.NodeType, err)
	}

	logger.Info("Node instance created successfully for nodeID: %s", req.NodeID)
	return runNode(ctx, nodeInstance, eventChan)
}

func (r *NodeRunner) optionalDependencies(nodeID string, nodeType shared.NodeType) []interface{} {
	var optionalDeps []interface{}

	if nodeType == shared.Start {
		if r.deps.ContentExtractor != nil {
			optionalDeps = append(optionalDeps, r.deps.ContentExtractor)
			logger.Info("Passing ContentExtractor to Start node: %s", nodeID)
		} else {
			logger.Info("No ContentExtractor available for Start node: %s", nodeID)
		}
	}

	if nodeType == shared.LLM || nodeType == shared.KnowledgeRetrieval ||
		nodeType == shared.ParameterExtractor || nodeType == shared.SQLGenerator ||
		nodeType == shared.ImageGen || nodeType == shared.Iteration || nodeType == shared.Loop ||
		nodeType == shared.QuestionAnswer {
		if r.deps.LLMClient != nil {
			optionalDeps = append(optionalDeps, r.deps.LLMClient)
			logger.Info("Passing LLMClient to %s node: %s", nodeType, nodeID)
		} else {
			logger.Warn("No LLMClient available for %s node: %s", nodeType, nodeID)
		}
		if nodeType == shared.LLM && r.deps.PromptResolver != nil {
			optionalDeps = append(optionalDeps, r.deps.PromptResolver)
		}
	}

	if nodeType == shared.Iteration || nodeType == shared.Loop {
		if r.deps.ToolEngine != nil {
			optionalDeps = append(optionalDeps, r.deps.ToolEngine)
			logger.Info("Passing ToolEngine to %s node: %s", nodeType, nodeID)
		} else {
			logger.Warn("No ToolEngine available for %s node: %s", nodeType, nodeID)
		}
		if r.subgraphEngineFactory != nil {
			optionalDeps = append(optionalDeps, r.subgraphEngineFactory)
		}
	}

	if nodeType == shared.KnowledgeRetrieval {
		if r.deps.GraphFlowService != nil {
			optionalDeps = append(optionalDeps, r.deps.GraphFlowService)
			logger.Info("Passing GraphFlowService to KnowledgeRetrieval node: %s", nodeID)
		} else {
			logger.Warn("No GraphFlowService available for KnowledgeRetrieval node: %s", nodeID)
		}
	}

	if nodeType == shared.Tools {
		if r.deps.ToolEngine != nil {
			optionalDeps = append(optionalDeps, r.deps.ToolEngine)
			logger.Info("Passing ToolEngine to Tools node: %s", nodeID)
		} else {
			logger.Warn("No ToolEngine available for Tools node: %s", nodeID)
		}
	}

	if nodeType == shared.HTTPRequest || nodeType == shared.LLM {
		if r.deps.FileService != nil {
			optionalDeps = append(optionalDeps, r.deps.FileService)
			logger.Info("Passing FileService to %s node: %s", nodeType, nodeID)
		} else {
			logger.Warn("No FileService available for %s node: %s", nodeType, nodeID)
		}
	}

	if nodeType == shared.CreateScheduledTask {
		if r.deps.AutomationDefinitionService != nil {
			optionalDeps = append(optionalDeps, r.deps.AutomationDefinitionService)
			logger.Info("Passing AutomationDefinitionService to %s node: %s", nodeType, nodeID)
		} else {
			logger.Warn("No AutomationDefinitionService available for %s node: %s", nodeType, nodeID)
		}
	}

	if nodeType == shared.NotificationSMS {
		if r.deps.NotificationSMSService != nil {
			optionalDeps = append(optionalDeps, r.deps.NotificationSMSService)
			logger.Info("Passing NotificationSMSService to %s node: %s", nodeType, nodeID)
		} else {
			logger.Warn("No NotificationSMSService available for %s node: %s", nodeType, nodeID)
		}
	}

	return optionalDeps
}

func runNode(ctx context.Context, node shared.NodeInterface, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	internalEvents := make(chan *shared.NodeEventCh, 10)
	forwardDone := make(chan struct{})
	var result *shared.NodeRunResult

	go func() {
		defer close(forwardDone)
		for event := range internalEvents {
			result = resultFromEvent(event, result)
			select {
			case eventChan <- event:
			case <-ctx.Done():
				return
			}
		}
	}()

	err := node.Run(ctx, internalEvents)
	close(internalEvents)
	<-forwardDone

	return result, err
}

func resultFromEvent(event *shared.NodeEventCh, fallback *shared.NodeRunResult) *shared.NodeRunResult {
	if event == nil || event.Data == nil {
		return fallback
	}
	switch event.Type {
	case shared.EventTypeRunCompleted:
		if completed, ok := event.Data.(*shared.RunCompletedEvent); ok {
			return completed.RunResult
		}
	case shared.EventTypeRunFailed:
		if failed, ok := event.Data.(*shared.RunFailedEvent); ok {
			return failed.RunResult
		}
	}
	return fallback
}

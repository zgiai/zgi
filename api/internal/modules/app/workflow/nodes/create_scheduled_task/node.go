package createscheduledtask

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
)

type definitionService interface {
	CreateTask(ctx context.Context, req automationdto.CreateTaskRequest) (*automationdto.CreateTaskResult, error)
}

type Node struct {
	base.NodeStruct
	nodeData          NodeData
	definitionService definitionService
}

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nodeData, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	node := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.CreateScheduledTask,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		nodeData: nodeData,
	}

	for _, dep := range optionalDeps {
		if svc, ok := dep.(automationdefinition.Service); ok {
			node.definitionService = svc
			break
		}
		if svc, ok := dep.(definitionService); ok {
			node.definitionService = svc
			break
		}
	}

	if node.definitionService == nil {
		return nil, fmt.Errorf("automation definition service is required for create-scheduled-task node")
	}

	return node, nil
}

func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}

	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	req, err := n.buildCreateTaskRequest()
	if err != nil {
		return nil, err
	}

	result, err := n.definitionService.CreateTask(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create automation task: %w", err)
	}
	if result == nil || result.Task == nil {
		return nil, fmt.Errorf("create automation task returned empty result")
	}

	outputs := map[string]any{
		"task_id":       result.Task.ID,
		"status":        result.Task.Status,
		"schedule_type": result.Task.ScheduleType,
		"timezone":      result.Task.Timezone,
		"next_run_at":   result.Task.NextRunAt,
	}

	return &shared.NodeRunResult{
		Status: shared.SUCCEEDED,
		Inputs: map[string]any{
			"task_name":       req.Name,
			"schedule_type":   req.ScheduleType,
			"schedule_config": req.ScheduleConfig,
		},
		Outputs: outputs,
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
			shared.ToolInfo: map[string]any{
				"type":          "automation",
				"node_type":     shared.CreateScheduledTask,
				"created_task":  result.Task.ID,
				"schedule_type": result.Task.ScheduleType,
			},
		},
	}, nil
}

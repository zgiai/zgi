package announcement

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	announcementruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/announcement"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/database"
)

const (
	outputAnnouncementID              = "announcement_id"
	outputAnnouncementToken           = "announcement_token"
	outputAnnouncementURL             = "announcement_url"
	outputAnnouncementExpiresAt       = "announcement_expires_at"
	outputAnnouncementRenderedContent = "announcement_rendered_content"
)

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

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.Announcement,

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
		NodeData: nodeData,
	}, nil
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
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, fmt.Errorf("variable pool is not initialized")
	}

	config := n.NodeData.Announcement
	config.Title = n.NodeData.Title
	config.Timeout = n.NodeData.Timeout
	if err := announcementruntime.ValidateConfig(config); err != nil {
		return nil, err
	}

	rendered := n.GraphRuntimeState.VariablePool.ConvertTemplate(config.Content).Markdown()
	service := announcementruntime.NewService(database.GetDB())
	runtimeAnnouncement, err := service.CreateOrGetRuntimeAnnouncement(ctx, announcementruntime.CreateRuntimeAnnouncementParams{
		TenantID:      n.TenantID,
		AppID:         n.APPID,
		WorkflowRunID: n.GraphRuntimeState.VariablePool.SystemVariables.WorkflowRunID,
		NodeID:        n.NodeID,
		NodeTitle:     n.NodeData.Title,
		Config:        config,
		Rendered:      rendered,
	})
	if err != nil {
		return nil, err
	}

	payload := runtimeAnnouncement.Payload
	outputs := map[string]any{
		outputAnnouncementID:              payload.ID,
		outputAnnouncementToken:           payload.Token,
		outputAnnouncementURL:             payload.URL,
		outputAnnouncementExpiresAt:       payload.ExpirationAt,
		outputAnnouncementRenderedContent: payload.Content,
	}
	return &shared.NodeRunResult{
		Status:           shared.SUCCEEDED,
		Inputs:           map[string]any{"announcement": config},
		Outputs:          outputs,
		ProcessData:      outputs,
		EdgeSourceHandle: "source",
	}, nil
}

func parseNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	rawNodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeID, ok := rawNodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}
	rawData, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}
	payload, err := json.Marshal(rawData)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("marshal announcement node data: %w", err)
	}
	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("unmarshal announcement node data: %w", err)
	}
	return nodeData, nodeID, nil
}

package notification_sms

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	notificationsms "github.com/zgiai/ginext/internal/modules/notification/sms"
)

type Node struct {
	base.NodeStruct
	NodeData
	service notificationsms.Service
}

type NodeData struct {
	base.NodeData
	Phone             string `json:"phone"`
	Provider          string `json:"provider"`
	Template          string `json:"template"`
	NotificationTitle string `json:"notification_title"`
	LinkCode          string `json:"link_code"`
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
	nd, nodeID, err := parseNodeData(config)
	if err != nil {
		return nil, err
	}

	var service notificationsms.Service
	for _, dep := range optionalDeps {
		if candidate, ok := dep.(notificationsms.Service); ok {
			service = candidate
			break
		}
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.NotificationSMS,

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
		NodeData: nd,
		service:  service,
	}, nil
}

func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{Type: shared.EventTypeRunStarted, NodeID: n.NodeID, Timestamp: time.Now()}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		select {
		case eventChan <- &shared.NodeEventCh{Type: shared.EventTypeRunFailed, NodeID: n.NodeID, Error: err, Timestamp: time.Now()}:
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
	if n.service == nil {
		return nil, fmt.Errorf("notification sms service is required")
	}
	if !n.service.IsEnabled() {
		return nil, fmt.Errorf("notification sms is not enabled")
	}

	req := notificationsms.Request{
		Phone:             n.resolveText(n.Phone),
		Provider:          strings.TrimSpace(n.Provider),
		Template:          strings.TrimSpace(n.Template),
		NotificationTitle: n.resolveText(n.NotificationTitle),
		LinkCode:          n.resolveText(n.LinkCode),
		Source:            "workflow",
		SourceID:          n.WorkflowID,
	}
	result, err := n.service.Send(ctx, req)
	if err != nil {
		return nil, err
	}

	outputs := map[string]any{
		"accepted":   result.Accepted,
		"provider":   result.Provider,
		"message_id": result.MessageID,
		"raw_code":   result.RawCode,
		"phone":      notificationsms.MaskPhone(req.Phone),
		"template":   req.Template,
	}
	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{"phone": notificationsms.MaskPhone(req.Phone), "provider": req.Provider, "template": req.Template},
		Outputs: outputs,
	}, nil
}

func (n *Node) resolveText(value string) string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(segmentGroupToText(n.GraphRuntimeState.VariablePool.ConvertTemplate(value)))
}

func parseNodeData(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"].(string)
	if !ok || strings.TrimSpace(nodeID) == "" {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	payload, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("marshal node data: %w", err)
	}
	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("unmarshal node data: %w", err)
	}
	if strings.TrimSpace(nodeData.Template) == "" {
		nodeData.Template = notificationsms.TemplatePendingActionNotification
	}
	return nodeData, nodeID, nil
}

func segmentGroupToText(segmentGroup *entities.SegmentGroup) string {
	if segmentGroup == nil {
		return ""
	}
	var builder strings.Builder
	for _, segment := range segmentGroup.Value {
		if segment == nil {
			continue
		}
		value := segment.ToObject()
		if text, ok := value.(string); ok {
			builder.WriteString(text)
			continue
		}
		payload, err := json.Marshal(value)
		if err != nil {
			builder.WriteString(fmt.Sprintf("%v", value))
			continue
		}
		builder.Write(payload)
	}
	return builder.String()
}

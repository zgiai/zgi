package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/database"
)

const (
	outputActionID        = "approval_action_id"
	outputActionLabel     = "approval_action_label"
	outputRenderedContent = "approval_rendered_content"
	outputApprovalFormID  = "__approval_form_id"
	outputApprovalToken   = "__approval_token"
	outputApprovalForm    = "__approval_form"

	approvalEmailURLPlaceholder = "{{#url#}}"
	approvalEmailURLSentinel    = "@@ZGI_APPROVAL_URL@@"
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
			NodeType:   shared.Approval,

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

	config := n.NodeData.Approval
	config.Title = n.NodeData.Title
	config.SubmitMethods = n.NodeData.SubmitMethods
	config.Timeout = n.NodeData.Timeout
	if err := approvalruntime.ValidateConfig(config); err != nil {
		return nil, err
	}

	rendered := n.GraphRuntimeState.VariablePool.ConvertTemplate(config.Content).Markdown()
	config = renderApprovalEmailTemplates(n.GraphRuntimeState.VariablePool, config)
	defaultValues := n.resolveDefaultValues(config)

	service := approvalruntime.NewService(database.GetDB())
	runtimeForm, err := service.CreateOrGetRuntimeForm(ctx, approvalruntime.CreateRuntimeFormParams{
		TenantID:      n.TenantID,
		AppID:         n.APPID,
		WorkflowRunID: n.GraphRuntimeState.VariablePool.SystemVariables.WorkflowRunID,
		NodeID:        n.NodeID,
		NodeTitle:     n.NodeData.Title,
		Config:        config,
		Rendered:      rendered,
		DefaultValues: defaultValues,
	})
	if err != nil {
		return nil, err
	}

	form := runtimeForm.Form
	if form.Status == approvalruntime.FormStatusTimeout || form.Status == approvalruntime.FormStatusExpired || time.Now().After(form.ExpirationTime) {
		return &shared.NodeRunResult{
			Status:           shared.SUCCEEDED,
			Inputs:           map[string]any{"approval": config},
			Outputs:          map[string]any{outputActionID: approvalruntime.ActionExpired, outputActionLabel: "Expired", outputRenderedContent: form.RenderedContent},
			ProcessData:      map[string]any{"form_id": form.ID, "status": form.Status, "expires_at": form.ExpirationTime.Unix()},
			EdgeSourceHandle: approvalruntime.ActionExpired,
		}, nil
	}

	if form.Status != approvalruntime.FormStatusSubmitted {
		return &shared.NodeRunResult{
			Status:      shared.PAUSED,
			Inputs:      map[string]any{"approval": config},
			ProcessData: map[string]any{"form_id": form.ID},
			Outputs: map[string]any{
				outputApprovalFormID: form.ID,
				outputApprovalToken:  runtimeForm.Payload.Token,
				outputApprovalForm:   runtimeForm.Payload,
			},
		}, nil
	}

	submittedOutputs, err := submittedData(form.SubmittedData)
	if err != nil {
		return nil, err
	}
	actionID := ""
	if form.SelectedActionID != nil {
		actionID = *form.SelectedActionID
	}
	submittedOutputs[outputActionID] = actionID
	submittedOutputs[outputActionLabel] = actionLabel(config.Actions, actionID)
	submittedOutputs[outputRenderedContent] = renderWithOutputs(form.RenderedContent, submittedOutputs)

	return &shared.NodeRunResult{
		Status:           shared.SUCCEEDED,
		Inputs:           map[string]any{"approval": config},
		Outputs:          submittedOutputs,
		ProcessData:      map[string]any{"form_id": form.ID, "status": form.Status, "expires_at": form.ExpirationTime.Unix()},
		EdgeSourceHandle: actionID,
	}, nil
}

func renderApprovalEmailTemplates(variablePool *entities.VariablePool, config approvalruntime.NodeConfig) approvalruntime.NodeConfig {
	if variablePool == nil || !config.SubmitMethods.Email.Enabled {
		return config
	}
	config.SubmitMethods.Email.Subject = renderApprovalEmailTemplate(variablePool, config.SubmitMethods.Email.Subject)
	config.SubmitMethods.Email.Body = renderApprovalEmailTemplate(variablePool, config.SubmitMethods.Email.Body)
	return config
}

func renderApprovalEmailTemplate(variablePool *entities.VariablePool, template string) string {
	if variablePool == nil || template == "" {
		return template
	}
	protected := strings.ReplaceAll(template, approvalEmailURLPlaceholder, approvalEmailURLSentinel)
	rendered := variablePool.ConvertTemplate(protected).Text()
	return strings.ReplaceAll(rendered, approvalEmailURLSentinel, approvalEmailURLPlaceholder)
}

func actionLabel(actions []approvalruntime.Action, id string) string {
	for _, action := range actions {
		if action.ID == id {
			return action.Label
		}
	}
	return id
}

func (n *Node) resolveDefaultValues(config approvalruntime.NodeConfig) map[string]interface{} {
	values := make(map[string]interface{})
	for _, field := range config.Fields {
		if field.Default == nil {
			continue
		}
		switch field.Default.Type {
		case "constant":
			values[field.Key] = field.Default.Value
		case "variable":
			if variable := n.GraphRuntimeState.VariablePool.GetWithPath(field.Default.Selector); variable != nil {
				values[field.Key] = variable.ToObject()
			}
		}
	}
	return values
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
		return NodeData{}, "", fmt.Errorf("marshal approval node data: %w", err)
	}
	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("unmarshal approval node data: %w", err)
	}
	return nodeData, nodeID, nil
}

func submittedData(raw *string) (map[string]any, error) {
	if raw == nil || *raw == "" {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(*raw), &result); err != nil {
		return nil, fmt.Errorf("decode approval submitted data: %w", err)
	}
	return result, nil
}

func renderWithOutputs(content string, outputs map[string]any) string {
	rendered := content
	for key, value := range outputs {
		placeholder := "{{#$output." + key + "#}}"
		var replacement string
		switch v := value.(type) {
		case string:
			replacement = v
		default:
			data, err := json.Marshal(v)
			if err != nil {
				replacement = fmt.Sprintf("%v", v)
			} else {
				replacement = string(data)
			}
		}
		rendered = strings.ReplaceAll(rendered, placeholder, replacement)
	}
	return rendered
}

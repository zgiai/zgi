package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/loop_subgraph"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/condbranch"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// Node represents a loop node.
type Node struct {
	base.NodeStruct
	nodeData NodeData

	now           func() time.Time
	llmClient     interface{}
	toolEngine    interface{}
	engineFactory subgraph.EngineFactory
}

// New creates a loop node instance from workflow config.
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
			NodeType:   shared.Loop,

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
		now:      time.Now,
	}

	for _, dep := range optionalDeps {
		if client, ok := dep.(llmclient.LLMClient); ok {
			node.llmClient = client
		}
		if te, ok := dep.(*tools.ToolEngine); ok {
			node.toolEngine = te
		}
		if factory, ok := dep.(subgraph.EngineFactory); ok {
			node.engineFactory = factory
		}
	}

	return node, nil
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
		return NodeData{}, "", fmt.Errorf("marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("unmarshal node data: %w", err)
	}

	if nodeData.LogicalOperator == "" {
		nodeData.LogicalOperator = condbranch.LogicalOperatorAnd
	}
	nodeData.ParallelNums = graph_engine.NormalizeMaxConcurrency(nodeData.ParallelNums)
	if nodeData.Outputs == nil {
		nodeData.Outputs = make(map[string]any)
	}

	return nodeData, nodeID, nil
}

// Run executes the loop node and emits workflow events.
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: n.now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx, eventChan)
	if err != nil {
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: n.now(),
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
		Timestamp: n.now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, fmt.Errorf("graph runtime state not initialized")
	}
	if n.nodeData.StartNodeID == nil || len(*n.nodeData.StartNodeID) == 0 {
		return nil, fmt.Errorf("start_node_id is required")
	}

	outputs := cloneAnyMap(n.nodeData.Outputs)
	loopVars, err := n.initLoopVariables()
	if err != nil {
		return nil, err
	}
	for key, value := range loopVars {
		outputs[key] = value
	}

	inputs := map[string]any{
		"loop_count":       n.nodeData.LoopCount,
		"loop_variables":   cloneAnyMap(loopVars),
		"break_conditions": n.nodeData.BreakConditions,
	}

	startedAt := n.now()
	loopCount := n.nodeData.LoopCount
	if loopCount < 0 {
		loopCount = 0
	}

	n.emitLoopStarted(ctx, eventChan, startedAt, inputs, loopCount)

	shouldBreak, _, err := n.evaluateBreakConditions()
	if err != nil {
		n.emitLoopFailed(ctx, eventChan, startedAt, inputs, outputs, 0, nil, err)
		return &shared.NodeRunResult{
			Status:  shared.FAILED,
			Inputs:  inputs,
			Outputs: outputs,
			ErrMsg:  err.Error(),
		}, err
	}
	if shouldBreak {
		loopCount = 0
	}

	loopDurationMap := make(map[string]float64)
	loopVariableMap := make(map[string]map[string]any)
	steps := 0

	executor := loop_subgraph.New(loop_subgraph.Config{
		NodeID:        n.NodeID,
		StartNodeID:   n.nodeData.StartNodeID,
		GraphConfig:   n.GraphConfig,
		RuntimeState:  n.GraphRuntimeState,
		Parallelism:   n.nodeData.ParallelNums,
		EventChan:     eventChan,
		EngineFactory: n.engineFactory,
	})

	for index := 0; index < loopCount; index++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		iterStart := n.now()
		result, runErr := executor.Run(ctx, index)
		duration := n.now().Sub(iterStart).Seconds()
		loopDurationMap[strconv.Itoa(index)] = duration
		steps++
		// Aggregate tokens reported by subgraph runtime (LLM gateway computed), no estimation here.
		n.addTokens(result.Tokens)

		if runErr != nil {
			metadata := map[shared.WorkflowNodeExecutionMetadataKey]any{
				shared.LoopDurationMap: loopDurationMap,
				shared.LoopVariableMap: loopVariableMap,
				shared.TotalTokens:     n.totalTokens(),
			}
			n.emitLoopFailed(ctx, eventChan, startedAt, inputs, outputs, steps, metadata, runErr)
			return &shared.NodeRunResult{
				Status:   shared.FAILED,
				Inputs:   inputs,
				Outputs:  outputs,
				Metadata: metadata,
				ErrMsg:   runErr.Error(),
			}, runErr
		}

		if result.Outputs != nil {
			outputs = mergeOutputs(outputs, result.Outputs)
			if n.GraphRuntimeState != nil {
				n.GraphRuntimeState.UpdateOutputs(func(current map[string]any) map[string]any {
					return mergeOutputs(current, result.Outputs)
				})
			}
		}

		loopVars = n.collectLoopVariables()
		loopVariableMap[strconv.Itoa(index)] = cloneAnyMap(loopVars)
		for key, value := range loopVars {
			outputs[key] = value
		}
		outputs["loop_round"] = index + 1

		if result.ReachedLoopEnd {
			break
		}

		shouldBreak, _, err = n.evaluateBreakConditions()
		if err != nil {
			metadata := map[shared.WorkflowNodeExecutionMetadataKey]any{
				shared.LoopDurationMap: loopDurationMap,
				shared.LoopVariableMap: loopVariableMap,
				shared.TotalTokens:     n.totalTokens(),
			}
			n.emitLoopFailed(ctx, eventChan, startedAt, inputs, outputs, steps, metadata, err)
			return &shared.NodeRunResult{
				Status:   shared.FAILED,
				Inputs:   inputs,
				Outputs:  outputs,
				Metadata: metadata,
				ErrMsg:   err.Error(),
			}, err
		}
		if shouldBreak {
			break
		}
		if index+1 < loopCount {
			n.emitLoopNext(ctx, eventChan, index+1, outputs)
		}
	}

	metadata := map[shared.WorkflowNodeExecutionMetadataKey]any{
		shared.LoopDurationMap: loopDurationMap,
		shared.LoopVariableMap: loopVariableMap,
		shared.TotalTokens:     n.totalTokens(),
	}

	n.emitLoopSucceeded(ctx, eventChan, startedAt, inputs, outputs, steps, metadata)

	return &shared.NodeRunResult{
		Status:   shared.SUCCEEDED,
		Inputs:   inputs,
		Outputs:  outputs,
		Metadata: metadata,
	}, nil
}

func (n *Node) initLoopVariables() (map[string]any, error) {
	result := make(map[string]any, len(n.nodeData.LoopVariables))
	for _, spec := range n.nodeData.LoopVariables {
		label := strings.TrimSpace(spec.Label)
		if label == "" {
			return nil, fmt.Errorf("loop variable label is required")
		}
		valueType, err := normalizeLoopVarType(spec.VarType)
		if err != nil {
			return nil, err
		}

		var rawValue any
		switch spec.ValueType {
		case "", ValueTypeConstant:
			rawValue = spec.Value
		case ValueTypeVariable:
			selector, err := toStringSlice(spec.Value)
			if err != nil {
				return nil, fmt.Errorf("loop variable %s selector invalid: %w", label, err)
			}
			variable := n.GraphRuntimeState.VariablePool.GetWithPath(selector)
			if variable == nil {
				return nil, fmt.Errorf("loop variable %s selector not found", label)
			}
			rawValue = variable.ToObject()
		default:
			return nil, fmt.Errorf("unsupported loop variable value_type %s", spec.ValueType)
		}

		segment, _, err := entities.ConvertValue(valueType, rawValue, entities.ValueConversionStrict)
		if err != nil {
			return nil, fmt.Errorf("loop variable %s convert: %w", label, err)
		}

		selector := []string{n.NodeID, label}
		n.GraphRuntimeState.VariablePool.AddSegment(selector, segment)
		result[label] = segment.ToObject()
	}

	return result, nil
}

func (n *Node) collectLoopVariables() map[string]any {
	result := make(map[string]any, len(n.nodeData.LoopVariables))
	for _, spec := range n.nodeData.LoopVariables {
		label := strings.TrimSpace(spec.Label)
		if label == "" {
			continue
		}
		selector := []string{n.NodeID, label}
		variable := n.GraphRuntimeState.VariablePool.Get(selector)
		if variable == nil {
			result[label] = nil
			continue
		}
		result[label] = variable.ToObject()
	}
	return result
}

func (n *Node) evaluateBreakConditions() (bool, []map[string]any, error) {
	if len(n.nodeData.BreakConditions) == 0 {
		return false, nil, nil
	}
	return condbranch.EvaluateConditions(n.GraphRuntimeState.VariablePool, n.nodeData.BreakConditions, n.nodeData.LogicalOperator)
}

func normalizeLoopVarType(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "string":
		return "string", nil
	case "number":
		return "number", nil
	case "integer":
		return "number", nil
	case "float":
		return "number", nil
	case "object":
		return "object", nil
	case "boolean":
		return "boolean", nil
	case "array_string", "array[string]":
		return "array_string", nil
	case "array_number", "array[number]":
		return "array_number", nil
	case "array_object", "array[object]":
		return "array_object", nil
	case "array_boolean", "array[boolean]":
		return "array_boolean", nil
	default:
		return "", fmt.Errorf("unsupported var_type %s", raw)
	}
}

func toStringSlice(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return v, nil
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("selector must be array of strings")
			}
			result = append(result, str)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("selector must be array")
	}
}

func mergeOutputs(base map[string]any, incoming map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	for key, value := range incoming {
		if key == "answer" {
			current, ok := base[key]
			if ok {
				currentStr, okCurrent := current.(string)
				newStr, okNew := value.(string)
				if okCurrent && okNew {
					base[key] = currentStr + newStr
					continue
				}
			}
		}
		base[key] = value
	}
	return base
}

func (n *Node) addTokens(tokens int) {
	if tokens <= 0 {
		return
	}
	if n.GraphRuntimeState == nil {
		return
	}
	n.GraphRuntimeState.AddTotalTokens(tokens)
}

func (n *Node) totalTokens() int {
	if n.GraphRuntimeState == nil {
		return 0
	}
	return n.GraphRuntimeState.TotalTokenCount()
}

func (n *Node) emitLoopStarted(
	ctx context.Context,
	eventChan chan *shared.NodeEventCh,
	startAt time.Time,
	inputs map[string]any,
	length int,
) {
	if eventChan == nil {
		return
	}
	event := &LoopStartedEvent{
		StartAt:  startAt,
		Inputs:   cloneAnyMap(inputs),
		Metadata: map[string]any{"loop_length": length},
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeLoopStarted, event)
}

func (n *Node) emitLoopNext(ctx context.Context, eventChan chan *shared.NodeEventCh, index int, preLoopOutput map[string]any) {
	if eventChan == nil {
		return
	}
	event := &LoopNextEvent{
		Index:         index,
		PreLoopOutput: cloneAnyMap(preLoopOutput),
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeLoopNext, event)
}

func (n *Node) emitLoopSucceeded(
	ctx context.Context,
	eventChan chan *shared.NodeEventCh,
	startAt time.Time,
	inputs map[string]any,
	outputs map[string]any,
	steps int,
	metadata map[shared.WorkflowNodeExecutionMetadataKey]any,
) {
	if eventChan == nil {
		return
	}
	event := &LoopSucceededEvent{
		StartAt:  startAt,
		Inputs:   cloneAnyMap(inputs),
		Outputs:  cloneAnyMap(outputs),
		Steps:    steps,
		Metadata: convertMetadata(metadata),
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeLoopSucceeded, event)
}

func (n *Node) emitLoopFailed(
	ctx context.Context,
	eventChan chan *shared.NodeEventCh,
	startAt time.Time,
	inputs map[string]any,
	outputs map[string]any,
	steps int,
	metadata map[shared.WorkflowNodeExecutionMetadataKey]any,
	err error,
) {
	if eventChan == nil {
		return
	}
	event := &LoopFailedEvent{
		StartAt:  startAt,
		Inputs:   cloneAnyMap(inputs),
		Outputs:  cloneAnyMap(outputs),
		Steps:    steps,
		Metadata: convertMetadata(metadata),
		Error:    err.Error(),
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeLoopFailed, event)
}

func (n *Node) emitEvent(
	ctx context.Context,
	eventChan chan *shared.NodeEventCh,
	eventType shared.NodeEventType,
	data any,
) {
	select {
	case <-ctx.Done():
		return
	case eventChan <- &shared.NodeEventCh{
		Type:      eventType,
		NodeID:    n.NodeID,
		Data:      data,
		Timestamp: n.now(),
	}:
	}
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	clone := make(map[string]any, len(input))
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

func convertMetadata(metadata map[shared.WorkflowNodeExecutionMetadataKey]any) map[string]any {
	if metadata == nil {
		return nil
	}
	result := make(map[string]any, len(metadata))
	for key, value := range metadata {
		result[string(key)] = value
	}
	return result
}

// LoopStartedEvent represents a loop started event payload.
type LoopStartedEvent struct {
	StartAt  time.Time      `json:"start_at"`
	Inputs   map[string]any `json:"inputs"`
	Metadata map[string]any `json:"metadata"`
}

// LoopNextEvent represents a loop next event payload.
type LoopNextEvent struct {
	Index         int            `json:"index"`
	PreLoopOutput map[string]any `json:"pre_loop_output,omitempty"`
}

func (e *LoopNextEvent) GetIndex() int {
	if e == nil {
		return 0
	}
	return e.Index
}

func (e *LoopNextEvent) GetPreLoopOutput() map[string]any {
	if e == nil {
		return nil
	}
	return e.PreLoopOutput
}

// LoopSucceededEvent represents a loop succeeded event payload.
type LoopSucceededEvent struct {
	StartAt  time.Time      `json:"start_at"`
	Inputs   map[string]any `json:"inputs"`
	Outputs  map[string]any `json:"outputs"`
	Steps    int            `json:"steps"`
	Metadata map[string]any `json:"metadata"`
}

// LoopFailedEvent represents a loop failed event payload.
type LoopFailedEvent struct {
	StartAt  time.Time      `json:"start_at"`
	Inputs   map[string]any `json:"inputs"`
	Outputs  map[string]any `json:"outputs"`
	Steps    int            `json:"steps"`
	Metadata map[string]any `json:"metadata"`
	Error    string         `json:"error"`
}

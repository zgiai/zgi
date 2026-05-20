package varassign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/conversation"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/database"
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
			NodeType:   shared.VariableAssigner,

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
		NodeData:          nodeData,
		conversationSaver: newWorkflowConversationPersister(),
	}, nil
}

type conversationVariablePersister interface {
	Save(ctx context.Context, conversationID, appID uuid.UUID, variables map[string]any) error
}

type targetContext struct {
	rootSelector []string
	targetVar    entities.Variable
	currentValue any
	isMissing    bool
	writeBack    func(updatedValue any) error
}

type workflowConversationPersister struct {
	service conversation.WorkflowConversationVariableService
}

func newWorkflowConversationPersister() conversationVariablePersister {
	db := database.GetDB()
	repo := conversation.NewWorkflowConversationVariableRepository(db)
	service := conversation.NewWorkflowConversationVariableService(repo)
	return &workflowConversationPersister{service: service}
}

func (p *workflowConversationPersister) Save(ctx context.Context, conversationID, appID uuid.UUID, variables map[string]any) error {
	return p.service.SaveConversationVariables(ctx, conversationID, appID, variables)
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
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	return nodeData, nodeID, nil
}

func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Start event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}
	result, err := n.executeRun(ctx, eventChan)
	if err != nil {
		// Send failure event
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

func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	_ = eventChan

	inputs := map[string]any{"items": n.NodeData.Items}

	vp, err := n.variablePool()
	if err != nil {
		return nil, err
	}

	updateSelectors, err := n.applyOperations(ctx, vp)
	if err != nil {
		return nil, err
	}

	processData, updatedVariables, conversationUpdates := n.collectUpdateSummaries(vp, updateSelectors)

	if len(updatedVariables) > 0 {
		processData[updatedVariablesKey] = updatedVariables
	}

	if len(conversationUpdates) > 0 {
		if err := n.persistConversationVariables(ctx, conversationUpdates); err != nil {
			return nil, err
		}
	}

	return &shared.NodeRunResult{
		Status:      shared.SUCCEEDED,
		Inputs:      inputs,
		ProcessData: processData,
		Outputs:     map[string]any{},
	}, nil
}

func ensureOperationSupported(variableType shared.SegmentType, op Operation) error {
	switch op {
	case OperationOverWrite, OperationClear:
		return nil
	case OperationSet:
		if isSegmentTypeOneOf(variableType, shared.SegmentTypeObject, shared.SegmentTypeString, shared.SegmentTypeNumber, shared.SegmentTypeInteger, shared.SegmentTypeFloat, shared.SegmentTypeBoolean) {
			return nil
		}
	case OperationAdd, OperationSubtract, OperationMultiply, OperationDivide:
		if isSegmentTypeOneOf(variableType, shared.SegmentTypeNumber, shared.SegmentTypeInteger, shared.SegmentTypeFloat) {
			return nil
		}
	case OperationAppend, OperationExtend, OperationRemoveFirst, OperationRemoveLast:
		if isArrayType(variableType) {
			return nil
		}
	}
	return fmt.Errorf("operation %q is not supported for type %s", op, variableType)
}

func ensureInputTypeSupported(variableType shared.SegmentType, op Operation, inputType InputType) error {
	switch inputType {
	case InputTypeVariable:
		if op == OperationSet || op == OperationAdd || op == OperationSubtract || op == OperationMultiply || op == OperationDivide {
			return fmt.Errorf("input type %q is not supported for operation %q", inputType, op)
		}
	case InputTypeConstant:
		switch variableType {
		case shared.SegmentTypeString, shared.SegmentTypeObject, shared.SegmentTypeBoolean:
			if op == OperationOverWrite || op == OperationSet {
				return nil
			}
		case shared.SegmentTypeNumber, shared.SegmentTypeInteger, shared.SegmentTypeFloat:
			if op == OperationOverWrite || op == OperationSet || op == OperationAdd || op == OperationSubtract || op == OperationMultiply || op == OperationDivide {
				return nil
			}
		}

		return fmt.Errorf("input type %q is not supported for operation %q and type %s", inputType, op, variableType)
	default:
		return fmt.Errorf("unknown input type %q", inputType)
	}

	return nil
}

func ensureValueValid(variableType shared.SegmentType, op Operation, value any) error {
	if op == OperationClear || op == OperationRemoveFirst || op == OperationRemoveLast {
		return nil
	}

	switch variableType {
	case shared.SegmentTypeString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string value, got %T", value)
		}
	case shared.SegmentTypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean value, got %T", value)
		}
	case shared.SegmentTypeObject:
		if !isMapStringAny(value) {
			return fmt.Errorf("expected object value, got %T", value)
		}
	case shared.SegmentTypeNumber, shared.SegmentTypeInteger, shared.SegmentTypeFloat:
		num, ok := toFloat64(value)
		if !ok {
			return fmt.Errorf("expected numeric value, got %T", value)
		}
		if op == OperationDivide && num == 0 {
			return errors.New("division by zero")
		}
	case shared.SegmentTypeArrayAny:
		if op == OperationAppend {
			if !isAllowedArrayAnyElement(value) {
				return fmt.Errorf("value %v not supported for append on %s", value, variableType)
			}
		} else {
			if err := ensureArrayAnyValue(value); err != nil {
				return err
			}
		}
	case shared.SegmentTypeArrayString:
		if op == OperationAppend {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("expected string value for append, got %T", value)
			}
		} else {
			if _, err := toStringSlice(value); err != nil {
				return err
			}
		}
	case shared.SegmentTypeArrayNumber:
		if op == OperationAppend {
			if _, ok := toFloat64(value); !ok {
				return fmt.Errorf("expected numeric value for append, got %T", value)
			}
		} else {
			if _, err := ensureFloatSlice(value); err != nil {
				return err
			}
		}
	case shared.SegmentTypeArrayObject:
		if op == OperationAppend {
			if !isMapStringAny(value) {
				return fmt.Errorf("expected object value for append, got %T", value)
			}
		} else {
			if _, err := ensureSliceOfMap(value); err != nil {
				return err
			}
		}
	case shared.SegmentTypeArrayBoolean:
		if op == OperationAppend {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("expected bool value for append, got %T", value)
			}
		} else {
			if _, err := ensureSliceOfBool(value); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported variable type %s", variableType)
	}

	return nil
}

func resolveVariableInput(vp *entities.VariablePool, rawSelector any, op Operation) (any, bool, error) {
	if op == OperationClear || op == OperationRemoveFirst || op == OperationRemoveLast {
		return nil, false, nil
	}

	selector, err := toStringSlice(rawSelector)
	if err != nil {
		return nil, false, fmt.Errorf("selector must be list of strings: %w", err)
	}

	variable := vp.GetWithPath(selector)
	if variable == nil {
		return nil, false, fmt.Errorf("variable %v not found", selector)
	}

	if variable.GetType() == shared.SegmentTypeNone {
		return nil, true, nil
	}

	return variable.ToObject(), false, nil
}

func tryDecodeJSONObject(value any) (any, bool) {
	switch v := value.(type) {
	case string:
		var result map[string]any
		if err := json.Unmarshal([]byte(v), &result); err == nil {
			return result, true
		}
	case []byte:
		var result map[string]any
		if err := json.Unmarshal(v, &result); err == nil {
			return result, true
		}
	}
	return nil, false
}

func applyOperation(variableType shared.SegmentType, current any, op Operation, value any) (any, error) {
	switch op {
	case OperationOverWrite, OperationSet:
		coerced, err := coerceValueForType(variableType, value)
		if err != nil {
			return nil, err
		}
		return coerced, nil
	case OperationClear:
		return emptyValueForType(variableType), nil
	case OperationAdd:
		cur, ok := toFloat64(current)
		if !ok {
			return nil, fmt.Errorf("current value %v is not numeric", current)
		}
		increment, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("value %v is not numeric", value)
		}
		return castNumericResult(variableType, cur+increment), nil
	case OperationSubtract:
		cur, ok := toFloat64(current)
		if !ok {
			return nil, fmt.Errorf("current value %v is not numeric", current)
		}
		decrement, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("value %v is not numeric", value)
		}
		return castNumericResult(variableType, cur-decrement), nil
	case OperationMultiply:
		cur, ok := toFloat64(current)
		if !ok {
			return nil, fmt.Errorf("current value %v is not numeric", current)
		}
		multiplier, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("value %v is not numeric", value)
		}
		return castNumericResult(variableType, cur*multiplier), nil
	case OperationDivide:
		cur, ok := toFloat64(current)
		if !ok {
			return nil, fmt.Errorf("current value %v is not numeric", current)
		}
		divisor, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("value %v is not numeric", value)
		}
		if divisor == 0 {
			return nil, errors.New("division by zero")
		}
		return castNumericResult(variableType, cur/divisor), nil
	case OperationAppend:
		return appendValue(variableType, current, value)
	case OperationExtend:
		return extendValue(variableType, current, value)
	case OperationRemoveFirst:
		return removeFirst(variableType, current)
	case OperationRemoveLast:
		return removeLast(variableType, current)
	default:
		return nil, fmt.Errorf("unsupported operation %q", op)
	}
}

func appendValue(variableType shared.SegmentType, current any, value any) (any, error) {
	switch variableType {
	case shared.SegmentTypeArrayAny:
		curSlice, err := toInterfaceSlice(current)
		if err != nil {
			return nil, err
		}
		if !isAllowedArrayAnyElement(value) {
			return nil, fmt.Errorf("value %v not supported for append on %s", value, variableType)
		}
		return append(curSlice, value), nil
	case shared.SegmentTypeArrayString:
		curSlice, err := toStringSlice(current)
		if err != nil {
			return nil, err
		}
		val, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("value %v is not string", value)
		}
		return append(curSlice, val), nil
	case shared.SegmentTypeArrayNumber:
		curSlice, err := toFloatSlice(current)
		if err != nil {
			return nil, err
		}
		val, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("value %v is not numeric", value)
		}
		return append(curSlice, val), nil
	case shared.SegmentTypeArrayObject:
		curSlice, err := toMapSlice(current)
		if err != nil {
			return nil, err
		}
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("value %v is not object", value)
		}
		return append(curSlice, obj), nil
	case shared.SegmentTypeArrayBoolean:
		curSlice, err := toBoolSlice(current)
		if err != nil {
			return nil, err
		}
		val, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("value %v is not bool", value)
		}
		return append(curSlice, val), nil
	default:
		return nil, fmt.Errorf("append not supported for %s", variableType)
	}
}

func extendValue(variableType shared.SegmentType, current any, value any) (any, error) {
	switch variableType {
	case shared.SegmentTypeArrayAny:
		curSlice, err := toInterfaceSlice(current)
		if err != nil {
			return nil, err
		}
		items, err := ensureArrayAnySlice(value)
		if err != nil {
			return nil, err
		}
		return append(curSlice, items...), nil
	case shared.SegmentTypeArrayString:
		curSlice, err := toStringSlice(current)
		if err != nil {
			return nil, err
		}
		items, err := toStringSlice(value)
		if err != nil {
			return nil, err
		}
		return append(curSlice, items...), nil
	case shared.SegmentTypeArrayNumber:
		curSlice, err := toFloatSlice(current)
		if err != nil {
			return nil, err
		}
		items, err := ensureFloatSlice(value)
		if err != nil {
			return nil, err
		}
		return append(curSlice, items...), nil
	case shared.SegmentTypeArrayObject:
		curSlice, err := toMapSlice(current)
		if err != nil {
			return nil, err
		}
		items, err := ensureSliceOfMap(value)
		if err != nil {
			return nil, err
		}
		return append(curSlice, items...), nil
	case shared.SegmentTypeArrayBoolean:
		curSlice, err := toBoolSlice(current)
		if err != nil {
			return nil, err
		}
		items, err := ensureSliceOfBool(value)
		if err != nil {
			return nil, err
		}
		return append(curSlice, items...), nil
	default:
		return nil, fmt.Errorf("extend not supported for %s", variableType)
	}
}

func removeFirst(variableType shared.SegmentType, current any) (any, error) {
	switch variableType {
	case shared.SegmentTypeArrayAny:
		curSlice, err := toInterfaceSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[1:], nil
	case shared.SegmentTypeArrayString:
		curSlice, err := toStringSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[1:], nil
	case shared.SegmentTypeArrayNumber:
		curSlice, err := toFloatSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[1:], nil
	case shared.SegmentTypeArrayObject:
		curSlice, err := toMapSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[1:], nil
	case shared.SegmentTypeArrayBoolean:
		curSlice, err := toBoolSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[1:], nil
	default:
		return nil, fmt.Errorf("remove-first not supported for %s", variableType)
	}
}

func removeLast(variableType shared.SegmentType, current any) (any, error) {
	switch variableType {
	case shared.SegmentTypeArrayAny:
		curSlice, err := toInterfaceSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[:len(curSlice)-1], nil
	case shared.SegmentTypeArrayString:
		curSlice, err := toStringSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[:len(curSlice)-1], nil
	case shared.SegmentTypeArrayNumber:
		curSlice, err := toFloatSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[:len(curSlice)-1], nil
	case shared.SegmentTypeArrayObject:
		curSlice, err := toMapSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[:len(curSlice)-1], nil
	case shared.SegmentTypeArrayBoolean:
		curSlice, err := toBoolSlice(current)
		if err != nil {
			return nil, err
		}
		if len(curSlice) == 0 {
			return curSlice, nil
		}
		return curSlice[:len(curSlice)-1], nil
	default:
		return nil, fmt.Errorf("remove-last not supported for %s", variableType)
	}
}

func emptyValueForType(variableType shared.SegmentType) any {
	switch variableType {
	case shared.SegmentTypeString:
		return ""
	case shared.SegmentTypeInteger:
		return 0
	case shared.SegmentTypeNumber, shared.SegmentTypeFloat:
		return float64(0)
	case shared.SegmentTypeBoolean:
		return false
	case shared.SegmentTypeObject:
		return map[string]any{}
	case shared.SegmentTypeArrayAny:
		return []any{}
	case shared.SegmentTypeArrayString:
		return []string{}
	case shared.SegmentTypeArrayNumber:
		return []float64{}
	case shared.SegmentTypeArrayObject:
		return []map[string]any{}
	case shared.SegmentTypeArrayBoolean:
		return []bool{}
	default:
		return nil
	}
}

func isSegmentTypeOneOf(target shared.SegmentType, candidates ...shared.SegmentType) bool {
	for _, c := range candidates {
		if target == c {
			return true
		}
	}
	return false
}

func isArrayType(variableType shared.SegmentType) bool {
	return variableType == shared.SegmentTypeArrayAny ||
		variableType == shared.SegmentTypeArrayString ||
		variableType == shared.SegmentTypeArrayNumber ||
		variableType == shared.SegmentTypeArrayObject ||
		variableType == shared.SegmentTypeArrayBoolean
}

func toStringSlice(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...), nil
	case []any:
		result := make([]string, len(v))
		for i, elem := range v {
			str, ok := elem.(string)
			if !ok {
				return nil, fmt.Errorf("selector element %v is not string", elem)
			}
			result[i] = str
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected []string, got %T", value)
	}
}

func toInterfaceSlice(value any) ([]any, error) {
	switch v := value.(type) {
	case []any:
		return append([]any(nil), v...), nil
	default:
		return nil, fmt.Errorf("expected slice, got %T", value)
	}
}

func toFloatSlice(value any) ([]float64, error) {
	switch v := value.(type) {
	case []float64:
		return append([]float64(nil), v...), nil
	case []any:
		result := make([]float64, len(v))
		for i, elem := range v {
			num, ok := toFloat64(elem)
			if !ok {
				return nil, fmt.Errorf("value %v is not numeric", elem)
			}
			result[i] = num
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected numeric slice, got %T", value)
	}
}

func toMapSlice(value any) ([]map[string]any, error) {
	switch v := value.(type) {
	case []map[string]any:
		result := make([]map[string]any, len(v))
		for i, elem := range v {
			result[i] = cloneMap(elem)
		}
		return result, nil
	case []any:
		result := make([]map[string]any, len(v))
		for i, elem := range v {
			obj, ok := elem.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("value %v is not object", elem)
			}
			result[i] = cloneMap(obj)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected object slice, got %T", value)
	}
}

func toBoolSlice(value any) ([]bool, error) {
	switch v := value.(type) {
	case []bool:
		return append([]bool(nil), v...), nil
	case []any:
		result := make([]bool, len(v))
		for i, elem := range v {
			b, ok := elem.(bool)
			if !ok {
				return nil, fmt.Errorf("value %v is not bool", elem)
			}
			result[i] = b
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected bool slice, got %T", value)
	}
}

func ensureFloatSlice(value any) ([]float64, error) {
	switch v := value.(type) {
	case []float64:
		res := make([]float64, len(v))
		copy(res, v)
		return res, nil
	case []any:
		res := make([]float64, len(v))
		for i, elem := range v {
			num, ok := toFloat64(elem)
			if !ok {
				return nil, fmt.Errorf("element %v is not numeric", elem)
			}
			res[i] = num
		}
		return res, nil
	default:
		return nil, fmt.Errorf("expected numeric slice, got %T", value)
	}
}

func ensureSliceOfMap(value any) ([]map[string]any, error) {
	return toMapSlice(value)
}

func ensureSliceOfBool(value any) ([]bool, error) {
	return toBoolSlice(value)
}

func isAllowedArrayAnyElement(value any) bool {
	switch value.(type) {
	case string, float64, float32, int, int64, map[string]any:
		return true
	default:
		return false
	}
}

func ensureArrayAnyValue(value any) error {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if !isAllowedArrayAnyElement(item) {
				return fmt.Errorf("value %v not supported for array[any]", item)
			}
		}
	case []string, []float64, []float32, []int, []int64, []map[string]any:
	default:
		return fmt.Errorf("expected slice value, got %T", value)
	}
	return nil
}

func ensureArrayAnySlice(value any) ([]any, error) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if !isAllowedArrayAnyElement(item) {
				return nil, fmt.Errorf("value %v not supported for array[any]", item)
			}
		}
		return append([]any(nil), v...), nil
	case []string:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case []float64:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case []float32:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = float64(item)
		}
		return result, nil
	case []int:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case []int64:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case []map[string]any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = cloneMap(item)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected slice value, got %T", value)
	}
}

func isMapStringAny(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

func toFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		num, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return num, true
	default:
		return 0, false
	}
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for k, v := range input {
		result[k] = v
	}
	return result
}

func castNumericResult(variableType shared.SegmentType, result float64) any {
	switch variableType {
	case shared.SegmentTypeInteger:
		if isWholeNumber(result) {
			return int(math.Round(result))
		}
		return result
	case shared.SegmentTypeNumber, shared.SegmentTypeFloat:
		return result
	default:
		return result
	}
}

func convertToIntIfWhole(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		if isWholeNumber(v) {
			return int(math.Round(v)), true
		}
	case float32:
		if isWholeNumber(float64(v)) {
			return int(math.Round(float64(v))), true
		}
	case json.Number:
		num, err := v.Float64()
		if err == nil && isWholeNumber(num) {
			return int(math.Round(num)), true
		}
	}
	return 0, false
}

func isWholeNumber(v float64) bool {
	return math.Abs(v-math.Round(v)) < 1e-9
}

func coerceValueForType(variableType shared.SegmentType, value any) (any, error) {
	switch variableType {
	case shared.SegmentTypeString:
		str, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("expected string value, got %T", value)
		}
		return str, nil
	case shared.SegmentTypeBoolean:
		b, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("expected bool value, got %T", value)
		}
		return b, nil
	case shared.SegmentTypeObject:
		if obj, ok := value.(map[string]any); ok {
			return cloneMap(obj), nil
		}
		return nil, fmt.Errorf("expected object value, got %T", value)
	case shared.SegmentTypeInteger:
		if intVal, ok := convertToIntIfWhole(value); ok {
			return intVal, nil
		}
		num, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("expected numeric value, got %T", value)
		}
		return num, nil
	case shared.SegmentTypeNumber, shared.SegmentTypeFloat:
		num, ok := toFloat64(value)
		if !ok {
			return nil, fmt.Errorf("expected numeric value, got %T", value)
		}
		return num, nil
	case shared.SegmentTypeArrayAny:
		return toInterfaceSlice(value)
	case shared.SegmentTypeArrayString:
		return toStringSlice(value)
	case shared.SegmentTypeArrayNumber:
		return ensureFloatSlice(value)
	case shared.SegmentTypeArrayObject:
		return ensureSliceOfMap(value)
	case shared.SegmentTypeArrayBoolean:
		return ensureSliceOfBool(value)
	default:
		return value, nil
	}
}

// persistConversationVariables saves the updated conversation variables to the persistent storage.
func (n *Node) persistConversationVariables(ctx context.Context, updates map[string]any) error {
	sysVars := n.GraphRuntimeState.VariablePool.SystemVariables
	if sysVars == nil {
		return errors.New("system variables not initialized")
	}

	conversationIDStr := sysVars.ConversationID
	if conversationIDStr == "" {
		if n.InvokeFrom != string(entities.InvokeFromDebugger) {
			return errors.New("conversation_id not found")
		}
		return nil
	}

	appIDStr := sysVars.AppID
	if appIDStr == "" {
		return errors.New("app_id not found for conversation variables")
	}

	conversationID, err := uuid.Parse(conversationIDStr)
	if err != nil {
		return fmt.Errorf("invalid conversation_id %q: %w", conversationIDStr, err)
	}

	appID, err := uuid.Parse(appIDStr)
	if err != nil {
		return fmt.Errorf("invalid app_id %q: %w", appIDStr, err)
	}

	saver := n.conversationSaver
	if saver == nil {
		saver = newWorkflowConversationPersister()
		n.conversationSaver = saver
	}

	if err := saver.Save(ctx, conversationID, appID, updates); err != nil {
		return fmt.Errorf("failed to persist conversation variables: %w", err)
	}

	return nil
}
func (n *Node) variablePool() (*entities.VariablePool, error) {
	if n.GraphRuntimeState == nil {
		return nil, errors.New("graph runtime state not initialized")
	}

	if n.GraphRuntimeState.VariablePool == nil {
		return nil, errors.New("variable pool not initialized")
	}

	return n.GraphRuntimeState.VariablePool, nil
}

func (n *Node) applyOperations(ctx context.Context, vp *entities.VariablePool) (map[string][]string, error) {
	selectors := make(map[string][]string)

	for _, item := range n.NodeData.Items {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		updated, err := n.evaluateItem(vp, item)
		if err != nil {
			return nil, err
		}

		if updated {
			key := strings.Join(item.VariableSelector, ".")
			selectors[key] = append([]string(nil), item.VariableSelector...)
		}
	}

	return selectors, nil
}

func (n *Node) evaluateItem(vp *entities.VariablePool, item VariableOperationItem) (bool, error) {
	value := item.Value
	if item.InputType == InputTypeVariable {
		resolved, skip, err := resolveVariableInput(vp, item.Value, item.Operation)
		if err != nil {
			return false, err
		}
		if skip {
			return false, nil
		}
		value = resolved
	}

	target, err := resolveTargetContext(vp, item.VariableSelector)
	if err != nil {
		return false, err
	}

	if target.isMissing {
		if err := ensureOperationSupportedForMissingNestedTarget(item.Operation); err != nil {
			return false, err
		}
		if err := ensureInputTypeSupportedForMissingNestedTarget(item.Operation, item.InputType); err != nil {
			return false, err
		}
		if item.Operation == OperationSet {
			if decoded, ok := tryDecodeJSONObject(value); ok {
				value = decoded
			}
		}
		if err := target.writeBack(value); err != nil {
			return false, err
		}
		return true, nil
	}

	varType := target.targetVar.GetType()
	currentValue := target.currentValue

	if err := ensureOperationSupported(varType, item.Operation); err != nil {
		return false, err
	}

	if err := ensureInputTypeSupported(varType, item.Operation, item.InputType); err != nil {
		return false, err
	}

	if item.Operation == OperationSet && varType == shared.SegmentTypeObject {
		if decoded, ok := tryDecodeJSONObject(value); ok {
			value = decoded
		}
	}

	if err := ensureValueValid(varType, item.Operation, value); err != nil {
		return false, err
	}

	updatedValue, err := applyOperation(varType, currentValue, item.Operation, value)
	if err != nil {
		return false, err
	}

	if err := target.writeBack(updatedValue); err != nil {
		return false, err
	}
	return true, nil
}

func (n *Node) collectUpdateSummaries(vp *entities.VariablePool, selectors map[string][]string) (map[string]any, []map[string]any, map[string]any) {
	processData := make(map[string]any)
	updatedVariables := make([]map[string]any, 0, len(selectors))
	conversationUpdates := make(map[string]any)

	for _, selector := range selectors {
		if len(selector) < 2 {
			continue
		}

		rootSelector := selector[:2]
		rootVariable := vp.Get(rootSelector)
		if rootVariable == nil {
			continue
		}

		leafVariable := rootVariable
		if len(selector) > 2 {
			leafVariable = vp.GetWithPath(selector)
			if leafVariable == nil {
				continue
			}
		}

		rootValue := rootVariable.ToObject()
		leafValue := leafVariable.ToObject()
		processData[rootVariable.GetName()] = rootValue

		selectorCopy := append([]string(nil), selector...)

		updatedVariables = append(updatedVariables, map[string]any{
			"name":       rootVariable.GetName(),
			"selector":   selectorCopy,
			"value_type": string(leafVariable.GetType()),
			"new_value":  leafValue,
		})

		if rootSelector[0] == entities.ConversationVariableNodeId {
			conversationUpdates[rootVariable.GetName()] = rootValue
		}
	}

	return processData, updatedVariables, conversationUpdates
}

func resolveTargetContext(vp *entities.VariablePool, selector []string) (*targetContext, error) {
	if len(selector) < entities.SelectorsLength {
		return nil, fmt.Errorf("variable %v not found", selector)
	}

	if len(selector) == entities.SelectorsLength {
		targetVar := vp.Get(selector)
		if targetVar == nil {
			return nil, fmt.Errorf("variable %v not found", selector)
		}

		selectorCopy := append([]string(nil), selector...)
		return &targetContext{
			rootSelector: selectorCopy,
			targetVar:    targetVar,
			currentValue: targetVar.ToObject(),
			writeBack: func(updatedValue any) error {
				vp.Add(selectorCopy, updatedValue)
				return nil
			},
		}, nil
	}

	rootSelector := append([]string(nil), selector[:2]...)
	rootVar := vp.Get(rootSelector)
	if rootVar == nil {
		return nil, fmt.Errorf("variable %v not found", selector)
	}

	if rootVar.GetType() != shared.SegmentTypeObject {
		return nil, fmt.Errorf("variable %v not found", selector)
	}
	rootObject, ok := rootVar.ToObject().(map[string]any)
	if !ok {
		return nil, fmt.Errorf("nested path %q is not object", strings.Join(selector[2:], "."))
	}

	path := append([]string(nil), selector[2:]...)
	targetVar, err := resolveExistingNestedVariable(vp, rootObject, selector, path)
	if err != nil {
		return nil, err
	}
	return &targetContext{
		rootSelector: rootSelector,
		targetVar:    targetVar,
		currentValue: targetCurrentValue(targetVar),
		isMissing:    targetVar == nil,
		writeBack: func(updatedValue any) error {
			updatedRoot, err := setNestedObjectValue(rootObject, path, updatedValue)
			if err != nil {
				return err
			}
			vp.Add(rootSelector, updatedRoot)
			return nil
		},
	}, nil
}

func resolveExistingNestedVariable(vp *entities.VariablePool, root map[string]any, selector, path []string) (entities.Variable, error) {
	current := root
	for _, key := range path[:len(path)-1] {
		nextValue, exists := current[key]
		if !exists {
			return nil, nil
		}

		nextMap, ok := asNestedObjectMap(nextValue)
		if !ok {
			return nil, fmt.Errorf("nested path %q is not object", strings.Join(path, "."))
		}
		current = nextMap
	}

	if _, exists := current[path[len(path)-1]]; !exists {
		return nil, nil
	}

	targetVar := vp.GetWithPath(selector)
	if targetVar == nil {
		return nil, fmt.Errorf("nested path %q is not object", strings.Join(path, "."))
	}
	return targetVar, nil
}

func targetCurrentValue(targetVar entities.Variable) any {
	if targetVar == nil {
		return nil
	}
	return targetVar.ToObject()
}

func setNestedObjectValue(root map[string]any, path []string, updatedValue any) (map[string]any, error) {
	if len(path) == 0 {
		return nil, errors.New("nested path is required")
	}

	clonedRoot := cloneMap(root)
	currentClone := clonedRoot
	currentSource := root

	for _, key := range path[:len(path)-1] {
		if currentSource == nil {
			nextClone := make(map[string]any)
			currentClone[key] = nextClone
			currentClone = nextClone
			continue
		}

		nextValue, exists := currentSource[key]
		if !exists {
			nextClone := make(map[string]any)
			currentClone[key] = nextClone
			currentClone = nextClone
			currentSource = nil
			continue
		}

		nextMap, ok := asNestedObjectMap(nextValue)
		if !ok {
			return nil, fmt.Errorf("nested path %q is not object", strings.Join(path, "."))
		}

		nextClone := cloneMap(nextMap)
		currentClone[key] = nextClone
		currentClone = nextClone
		currentSource = nextMap
	}

	currentClone[path[len(path)-1]] = updatedValue
	return clonedRoot, nil
}

func ensureOperationSupportedForMissingNestedTarget(op Operation) error {
	switch op {
	case OperationOverWrite, OperationSet:
		return nil
	default:
		return fmt.Errorf("operation %q is not supported for missing nested variable", op)
	}
}

func ensureInputTypeSupportedForMissingNestedTarget(op Operation, inputType InputType) error {
	switch inputType {
	case InputTypeVariable:
		if op == OperationSet || op == OperationAdd || op == OperationSubtract || op == OperationMultiply || op == OperationDivide {
			return fmt.Errorf("input type %q is not supported for operation %q", inputType, op)
		}
	case InputTypeConstant:
		if op == OperationOverWrite || op == OperationSet {
			return nil
		}
		return fmt.Errorf("input type %q is not supported for operation %q", inputType, op)
	default:
		return fmt.Errorf("unknown input type %q", inputType)
	}

	return nil
}

func asNestedObjectMap(value any) (map[string]any, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	if inferValueType(value) != shared.SegmentTypeObject {
		return nil, false
	}
	return obj, true
}

func inferValueType(value any) shared.SegmentType {
	tmp := entities.NewVariablePool()
	selector := []string{"tmp", "value"}
	tmp.Add(selector, value)
	variable := tmp.Get(selector)
	if variable == nil {
		return shared.SegmentTypeNone
	}
	return variable.GetType()
}

package code

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	codeexec "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/code/codeExec"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

const (
	codeResultMaxDepth         = 5
	defaultCodeMaxStringLength = 80000
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
	// Parse node data from config
	nd, nodeID, err := parseCodeNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	// Validate outputs configuration
	if err := nd.ValidateOutputs(); err != nil {
		return nil, fmt.Errorf("invalid outputs configuration: %w", err)
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.Code,

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
	}, nil
}

// parseCodeNodeDataFromConfig parses node data and id from config
func parseCodeNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	// 1. Get node ID
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	// 2. Get node data
	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	// 3. Convert to JSON and back to parse structure into NodeData
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(jsonBytes, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	return nodeData, nodeIDStr, nil
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
	_ = eventChan // currently not used, but can be used for progress updates

	variables := make(map[string]any)
	variablePool := n.GraphRuntimeState.VariablePool

	for _, variableSelector := range n.NodeData.Variables {
		variableName := variableSelector.Variable
		variable := variablePool.GetWithPath(variableSelector.ValueSelector)

		if variable == nil {
			variables[variableName] = nil
			continue
		}

		// Check if it's an ArrayFileSegment type
		if variable.GetType() == shared.SegmentTypeArrayFile {
			// For ArrayFileSegment, we need to convert files to dict format
			variableValue := variable.ToObject()
			if fileArray, ok := variableValue.([]*entities.File); ok && fileArray != nil {
				fileDicts := make([]map[string]any, len(fileArray))
				for i, file := range fileArray {
					f := convertEntityFileToWorkflowFile(file)
					fileDicts[i] = f.ToDict()
				}
				variables[variableName] = fileDicts
			} else {
				variables[variableName] = nil
			}
		} else {
			variables[variableName] = variable.ToObject()
		}
	}

	lang := codeexec.Language(n.CodeLanguage)
	code := n.Code

	executor := codeexec.NewExecutor(
		codeexec.NewPythonTransformer(),
		codeexec.NewJavaScriptTransformer(),
	)
	executionResult, err := executor.ExecuteWorkflowCodeTemplate(ctx, lang, code, variables)
	if err != nil {
		return nil, fmt.Errorf("code execution failed: %w", err)
	}

	r, err := n.transformOutputs(executionResult)
	if err != nil {
		return nil, err
	}

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  variables,
		Outputs: r,
	}, nil
}

func (n *Node) transformOutputs(raw map[string]any) (map[string]any, error) {
	if raw == nil {
		raw = map[string]any{}
	}

	if len(n.Outputs) == 0 {
		return raw, nil
	}

	transformed := make(map[string]any, len(n.Outputs))
	validated := 0

	for name, schema := range n.Outputs {
		value, ok := raw[name]
		if !ok {
			return nil, fmt.Errorf("output %s is missing", name)
		}

		transformedValue, err := n.transformValue(value, schema, name, 1)
		if err != nil {
			return nil, err
		}

		transformed[name] = transformedValue
		validated++
	}

	if len(raw) != validated {
		return nil, fmt.Errorf("not all output parameters are validated")
	}

	return transformed, nil
}

func (n *Node) transformValue(value any, schema Output, path string, depth int) (any, error) {
	if depth > codeResultMaxDepth {
		return nil, fmt.Errorf("depth limit %d reached, object too deep", codeResultMaxDepth)
	}

	switch schema.Type {
	case shared.SegmentTypeObject:
		if value == nil {
			return nil, nil
		}

		obj, err := toStringAnyMap(value)
		if err != nil {
			return nil, fmt.Errorf("output %s is not an object: %w", path, err)
		}

		if len(schema.Children) == 0 {
			return obj, nil
		}

		childTransformed := make(map[string]any, len(schema.Children))
		childValidated := 0

		for key, childSchema := range schema.Children {
			childPath := joinPath(path, key)
			rawChildValue, exists := obj[key]
			if !exists {
				return nil, fmt.Errorf("output %s is missing", childPath)
			}

			if childSchema == nil {
				childTransformed[key] = rawChildValue
				childValidated++
				continue
			}

			transformedValue, err := n.transformValue(rawChildValue, *childSchema, childPath, depth+1)
			if err != nil {
				return nil, err
			}

			childTransformed[key] = transformedValue
			childValidated++
		}

		if len(obj) != childValidated {
			return nil, fmt.Errorf("not all output parameters are validated for %s", path)
		}

		return childTransformed, nil
	case shared.SegmentTypeNumber:
		return sanitizeNumber(value, path)
	case shared.SegmentTypeString:
		return sanitizeString(value, path)
	case shared.SegmentTypeBoolean:
		return sanitizeBoolean(value, path)
	case shared.SegmentTypeArrayNumber:
		return sanitizeNumberArray(value, path)
	case shared.SegmentTypeArrayString:
		return sanitizeStringArray(value, path)
	case shared.SegmentTypeArrayObject:
		return n.sanitizeObjectArray(value, schema, path, depth)
	case shared.SegmentTypeArrayBoolean:
		return sanitizeBooleanArray(value, path)
	default:
		return nil, fmt.Errorf("output type %s is not supported", schema.Type)
	}
}

func (n *Node) sanitizeObjectArray(value any, schema Output, path string, depth int) (any, error) {
	if value == nil {
		return nil, nil
	}

	items, err := toSlice(value)
	if err != nil {
		return nil, fmt.Errorf("output %s is not an array: %w", path, err)
	}

	result := make([]any, len(items))
	for i, item := range items {
		itemPath := joinPath(path, fmt.Sprintf("[%d]", i))
		if item == nil {
			result[i] = nil
			continue
		}

		childSchema := Output{
			Type:     shared.SegmentTypeObject,
			Children: schema.Children,
		}

		transformed, err := n.transformValue(item, childSchema, itemPath, depth+1)
		if err != nil {
			return nil, err
		}

		result[i] = transformed
	}

	return result, nil
}

func sanitizeNumber(value any, path string) (any, error) {
	if value == nil {
		return nil, nil
	}

	numberValue, floatValue, err := convertToNumber(value)
	if err != nil {
		return nil, fmt.Errorf("output %s is not a number, got %T", path, value)
	}

	if err := ensureNumberInRange(floatValue, path); err != nil {
		return nil, err
	}

	return numberValue, nil
}

func sanitizeNumberArray(value any, path string) (any, error) {
	if value == nil {
		return nil, nil
	}

	items, err := toSlice(value)
	if err != nil {
		return nil, fmt.Errorf("output %s is not an array: %w", path, err)
	}

	result := make([]any, len(items))
	for i, item := range items {
		if item == nil {
			result[i] = nil
			continue
		}

		itemPath := joinPath(path, fmt.Sprintf("[%d]", i))
		sanitized, err := sanitizeNumber(item, itemPath)
		if err != nil {
			return nil, err
		}
		result[i] = sanitized
	}

	return result, nil
}

func sanitizeString(value any, path string) (any, error) {
	if value == nil {
		return nil, nil
	}

	str, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("output %s must be a string, got %T", path, value)
	}

	maxStringLength := getMaxStringLength()
	if maxStringLength > 0 && len(str) > maxStringLength {
		return nil, fmt.Errorf(
			"the length of output variable `%s` must be less than %d characters",
			path,
			maxStringLength,
		)
	}

	if strings.Contains(str, "\x00") {
		str = strings.ReplaceAll(str, "\x00", "")
	}

	return str, nil
}

func sanitizeStringArray(value any, path string) (any, error) {
	if value == nil {
		return nil, nil
	}

	items, err := toSlice(value)
	if err != nil {
		return nil, fmt.Errorf("output %s is not an array: %w", path, err)
	}

	result := make([]any, len(items))
	for i, item := range items {
		if item == nil {
			result[i] = nil
			continue
		}

		itemPath := joinPath(path, fmt.Sprintf("[%d]", i))
		sanitized, err := sanitizeString(item, itemPath)
		if err != nil {
			return nil, err
		}

		result[i] = sanitized
	}

	return result, nil
}

func sanitizeBoolean(value any, path string) (any, error) {
	if value == nil {
		return nil, nil
	}

	booleanValue, ok := value.(bool)
	if !ok {
		return nil, fmt.Errorf("output %s is not a boolean, got %T", path, value)
	}

	return booleanValue, nil
}

func sanitizeBooleanArray(value any, path string) (any, error) {
	if value == nil {
		return nil, nil
	}

	items, err := toSlice(value)
	if err != nil {
		return nil, fmt.Errorf("output %s is not an array: %w", path, err)
	}

	result := make([]any, len(items))
	for i, item := range items {
		if item == nil {
			result[i] = nil
			continue
		}

		booleanValue, ok := item.(bool)
		if !ok {
			return nil, fmt.Errorf("output %s[%d] is not a boolean, got %T", path, i, item)
		}

		result[i] = booleanValue
	}

	return result, nil
}

func convertToNumber(value any) (any, float64, error) {
	switch v := value.(type) {
	case int:
		return v, float64(v), nil
	case int8:
		return int64(v), float64(v), nil
	case int16:
		return int64(v), float64(v), nil
	case int32:
		return int64(v), float64(v), nil
	case int64:
		return v, float64(v), nil
	case uint:
		if v > math.MaxInt64 {
			return nil, 0, fmt.Errorf("value %d exceeds maximum supported integer", v)
		}
		return int64(v), float64(v), nil
	case uint8:
		return int64(v), float64(v), nil
	case uint16:
		return int64(v), float64(v), nil
	case uint32:
		return int64(v), float64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return nil, 0, fmt.Errorf("value %d exceeds maximum supported integer", v)
		}
		return int64(v), float64(v), nil
	case float32:
		return float64(v), float64(v), nil
	case float64:
		return v, v, nil
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, float64(i), nil
		}
		f, err := v.Float64()
		if err != nil {
			return nil, 0, err
		}
		return f, f, nil
	case bool:
		if v {
			return 1, 1, nil
		}
		return 0, 0, nil
	default:
		return nil, 0, fmt.Errorf("unsupported numeric type %T", value)
	}
}

func ensureNumberInRange(value float64, path string) error {
	maxNumber, minNumber, _ := getCodeExecLimits()
	if value > float64(maxNumber) || value < float64(minNumber) {
		return fmt.Errorf(
			"output variable `%s` is out of range, it must be between %d and %d",
			path,
			minNumber,
			maxNumber,
		)
	}

	return nil
}

func getCodeExecLimits() (int64, int64, int) {
	if config.GlobalConfig != nil {
		limits := config.GlobalConfig.CodeExec
		maxNumber := limits.MaxNumber
		if maxNumber == 0 {
			maxNumber = math.MaxInt64
		}

		minNumber := limits.MinNumber
		if minNumber == 0 {
			minNumber = math.MinInt64
		}

		maxStringLength := limits.MaxStringLength
		if maxStringLength == 0 {
			maxStringLength = defaultCodeMaxStringLength
		}

		return maxNumber, minNumber, maxStringLength
	}

	return math.MaxInt64, math.MinInt64, defaultCodeMaxStringLength
}

func getMaxStringLength() int {
	_, _, maxStringLength := getCodeExecLimits()
	return maxStringLength
}

func joinPath(prefix, suffix string) string {
	if prefix == "" {
		return suffix
	}

	if strings.HasPrefix(suffix, "[") {
		return prefix + suffix
	}

	return prefix + "." + suffix
}

func toStringAnyMap(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}

	if typed, ok := value.(map[string]any); ok {
		return typed, nil
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Map {
		return nil, fmt.Errorf("expected map[string]any, got %T", value)
	}

	if val.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("expected map with string keys, got %T", value)
	}

	result := make(map[string]any, val.Len())
	for iter := val.MapRange(); iter.Next(); {
		result[iter.Key().String()] = iter.Value().Interface()
	}

	return result, nil
}

func toSlice(value any) ([]any, error) {
	if value == nil {
		return nil, nil
	}

	if typed, ok := value.([]any); ok {
		return typed, nil
	}

	val := reflect.ValueOf(value)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil, fmt.Errorf("expected slice, got %T", value)
	}

	length := val.Len()
	result := make([]any, length)
	for i := 0; i < length; i++ {
		result[i] = val.Index(i).Interface()
	}

	return result, nil
}

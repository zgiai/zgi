package jsonparser

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

const (
	jsonResultMaxDepth = 5
)

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

	// For EXCEPTION results (DefaultVal / FailBranch strategy): send RunCompleted so the
	// workflow continues (or routes to the fail branch), but also carry the error in the
	// event so the graph engine's onNodeFinished callback and the UI can mark the node
	// as erred without terminating the whole workflow.
	var nodeErr error
	if result != nil && result.Status == shared.EXCEPTION && result.ErrMsg != "" {
		nodeErr = fmt.Errorf("%s", result.ErrMsg)
	}

	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Error:     nodeErr,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	variablePool := n.GraphRuntimeState.VariablePool

	// Determine the input source: prefer input_selector, otherwise use the first variable
	var inputSelector []string
	if len(n.NodeData.InputSelector) > 0 {
		inputSelector = n.NodeData.InputSelector
	} else if len(n.NodeData.Variables) > 0 {
		inputSelector = n.NodeData.Variables[0].ValueSelector
	} else {
		errMsg := "input_selector or variables is required"
		errType := "ConfigError"
		if result := n.applyErrorStrategy(errMsg, errType, nil); result != nil {
			return result, nil
		}
		// No error strategy: return error to let Run() send RunFailed and terminate the workflow.
		return nil, fmt.Errorf("[%s] %s", errType, errMsg)
	}

	variable := variablePool.GetWithPath(inputSelector)
	if variable == nil {
		errMsg := fmt.Sprintf("variable not found: %v", inputSelector)
		errType := "VariableNotFoundError"
		if result := n.applyErrorStrategy(errMsg, errType, nil); result != nil {
			return result, nil
		}
		return nil, fmt.Errorf("[%s] %s", errType, errMsg)
	}

	variableValue := variable.ToObject()

	parsedData, err := n.parseJSON(variableValue)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse JSON: %v", err)
		errType := "JSONParseError"
		if result := n.applyErrorStrategy(errMsg, errType, nil); result != nil {
			return result, nil
		}
		return nil, fmt.Errorf("[%s] %s", errType, errMsg)
	}

	inputs := map[string]any{
		"input": variableValue,
	}

	// Build the output data: choose flat or wrapped mode based on is_flatten_output
	rawOutputs := make(map[string]any, len(n.Outputs))
	if n.NodeData.IsFlattenOutput {
		// Flat mode: the parsed result must be a JSON object, and each Output Key reads the first-level field with the same name
		parsedMap, ok := parsedData.(map[string]any)
		if !ok {
			errMsg := fmt.Sprintf("is_flatten_output requires parsed JSON to be an object, got %T", parsedData)
			errType := "OutputTransformError"
			if result := n.applyErrorStrategy(errMsg, errType, nil); result != nil {
				return result, nil
			}
			return nil, fmt.Errorf("[%s] %s", errType, errMsg)
		}
		for outputKey := range n.Outputs {
			if val, exists := parsedMap[outputKey]; exists {
				rawOutputs[outputKey] = val
			} else {
				rawOutputs[outputKey] = nil
			}
		}
	} else {
		// Wrapped mode (default): assign the entire parsed result to each Output Key (usually only one top-level variable)
		for outputKey := range n.Outputs {
			rawOutputs[outputKey] = parsedData
		}
	}

	transformedOutputs, err := n.transformOutputs(rawOutputs)
	if err != nil {
		errMsg := fmt.Sprintf("failed to transform outputs: %v", err)
		errType := "OutputTransformError"
		if result := n.applyErrorStrategy(errMsg, errType, nil); result != nil {
			return result, nil
		}
		return nil, fmt.Errorf("[%s] %s", errType, errMsg)
	}

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  inputs,
		Outputs: transformedOutputs,
	}, nil
}

func (n *Node) parseJSON(value any) (any, error) {
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, fmt.Errorf("empty JSON string: input must be a valid JSON object (e.g., {\"key\": \"value\"}) or array (e.g., [1,2,3])")
		}
		v = stripMarkdownCodeFence(v)
		if v == "" {
			return nil, fmt.Errorf("empty JSON string after stripping markdown code fence")
		}
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("invalid JSON format: %w. Input must be a valid JSON string (e.g., {\"key\": \"value\"} or [1,2,3])", err)
		}
		return parsed, nil

	case map[string]any:
		return v, nil

	case []any:
		return v, nil

	case []byte:
		if len(v) == 0 {
			return nil, fmt.Errorf("empty JSON data")
		}
		var parsed any
		if err := json.Unmarshal(v, &parsed); err != nil {
			return nil, fmt.Errorf("invalid JSON format: %w", err)
		}
		return parsed, nil

	default:
		return nil, fmt.Errorf("unsupported data type: %T", value)
	}
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
	if depth > jsonResultMaxDepth {
		return nil, fmt.Errorf("depth limit %d reached, object too deep", jsonResultMaxDepth)
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
	maxNumber := math.MaxInt64
	minNumber := math.MinInt64

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

func (n *Node) applyErrorStrategy(errMsg, errType string, existingOutputs map[string]any) *shared.NodeRunResult {
	// "" means no strategy configured (or "none" normalized at parse time) → terminate workflow.
	if n.NodeData.ErrorStrategy == "" {
		return nil
	}

	outputs := make(map[string]any)
	for k, v := range existingOutputs {
		outputs[k] = v
	}

	switch n.NodeData.ErrorStrategy {
	case shared.DefaultVal:
		// Default-value strategy: expose only the user-configured defaults.
		// Do NOT write error_message/error_type into outputs – the caller
		// (Run) already carries the error in the event's Error field and in
		// result.ErrMsg/ErrType/Metadata, which the UI reads from there.
		for _, dv := range n.NodeData.DefaultValue {
			if dv.Key != "" {
				outputs[dv.Key] = n.parseDefaultValue(dv)
			}
		}

	case shared.FailBranch:
		// Fail-branch strategy: the fail branch's downstream nodes may want to
		// inspect what went wrong, so include error details in outputs.
		outputs["error_message"] = errMsg
		outputs["error_type"] = errType
	}

	result := &shared.NodeRunResult{
		Status:  shared.EXCEPTION,
		Outputs: outputs,
		ErrMsg:  errMsg,
		ErrType: errType,
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
			shared.ErrStrategy: n.NodeData.ErrorStrategy,
		},
	}

	if n.NodeData.ErrorStrategy == shared.FailBranch {
		result.EdgeSourceHandle = string(shared.FailedBranch)
	}

	return result
}

func (n *Node) parseDefaultValue(dv shared.DefaultValue) any {
	switch dv.Type {
	case shared.TypeString:
		return dv.Value

	case shared.TypeNumber:
		var num float64
		if _, err := fmt.Sscanf(dv.Value, "%f", &num); err == nil {
			return num
		}
		return 0

	case shared.TypeObject, shared.TypeArrayObject, shared.TypeArrayString, shared.TypeArrayNumber:
		var parsed any
		if err := json.Unmarshal([]byte(dv.Value), &parsed); err == nil {
			return parsed
		}
		return dv.Value

	default:
		return dv.Value
	}
}

// stripMarkdownCodeFence removes optional markdown code fences from a JSON string.
// It handles patterns like:
//   - ```json\n...\n```
//   - ```\n...\n```
//
// After stripping, the result is trimmed of surrounding whitespace.
func stripMarkdownCodeFence(s string) string {
	// Must start with ``` to be a code fence
	if !strings.HasPrefix(s, "```") {
		return s
	}

	// Find the first newline to skip the opening fence line (e.g. "```json\n")
	firstNewline := strings.Index(s, "\n")
	if firstNewline == -1 {
		// No newline found, nothing valid to extract
		return s
	}

	// Strip the opening fence line
	body := s[firstNewline+1:]

	// Strip the closing ``` (must be the last non-empty content)
	if idx := strings.LastIndex(body, "```"); idx != -1 {
		body = body[:idx]
	}

	return strings.TrimSpace(body)
}

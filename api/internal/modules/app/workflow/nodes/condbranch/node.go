package condbranch

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
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

	if nodeData.LogicalOperator == "" {
		nodeData.LogicalOperator = LogicalOperatorAnd
	}
	for i := range nodeData.Cases {
		if nodeData.Cases[i].LogicalOperator == "" {
			nodeData.Cases[i].LogicalOperator = LogicalOperatorAnd
		}
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.IfElse,

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
	_ = ctx
	_ = eventChan

	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, fmt.Errorf("variable pool is not initialized")
	}

	inputs := map[string]any{
		"conditions": []map[string]any{},
	}
	processData := map[string]any{
		"condition_results": []map[string]any{},
	}

	selectedCaseID := "false"
	finalResult := false

	conditionResults := make([]map[string]any, 0, len(n.NodeData.Cases))
	var lastInputConditions []map[string]any

	for _, c := range n.NodeData.Cases {
		inputConditions, groupResults, caseResult, err := processConditions(n.GraphRuntimeState.VariablePool, c.Conditions, c.LogicalOperator)
		if err != nil {
			return nil, err
		}

		lastInputConditions = inputConditions

		caseMap, err := caseToMap(c)
		if err != nil {
			return nil, err
		}

		conditionResults = append(conditionResults, map[string]any{
			"group":        caseMap,
			"results":      groupResults,
			"final_result": caseResult,
		})

		if caseResult {
			selectedCaseID = c.CaseID
			finalResult = true
			break
		}
	}

	if lastInputConditions == nil {
		lastInputConditions = []map[string]any{}
	}
	inputs["conditions"] = lastInputConditions
	processData["condition_results"] = conditionResults

	edgeHandle := selectedCaseID
	if edgeHandle == "" {
		edgeHandle = "false"
	}

	outputs := map[string]any{
		"result": finalResult,
	}

	return &shared.NodeRunResult{
		Status:           shared.SUCCEEDED,
		Inputs:           inputs,
		ProcessData:      processData,
		Outputs:          outputs,
		EdgeSourceHandle: edgeHandle,
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
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	return nodeData, nodeID, nil
}

func processConditions(vp *entities.VariablePool, conditions []Condition, operator LogicalOperator) ([]map[string]any, []bool, bool, error) {
	inputConditions := make([]map[string]any, 0)
	groupResults := make([]bool, 0, len(conditions))

	logicalOp := strings.ToLower(string(operator))
	if logicalOp != "or" {
		logicalOp = "and"
	}

	for _, condition := range conditions {
		variable := vp.GetWithPath(condition.VariableSelector)
		// Fix: Treat missing variable as nil instead of error
		// if variable == nil {
		// 	return nil, groupResults, false, fmt.Errorf("variable %v not found", condition.VariableSelector)
		// }

		var (
			result bool
			err    error
		)

		if variable != nil && variable.GetType() == shared.SegmentTypeArrayFile && requiresSubConditions(condition.ComparisonOperator) {
			if condition.SubVariableCondition == nil {
				return nil, groupResults, false, fmt.Errorf("sub variable condition is required for operator %q", condition.ComparisonOperator)
			}

			files, ok := variable.GetValue().([]*entities.File)
			if !ok {
				return nil, groupResults, false, fmt.Errorf("variable %v is not an array of files", condition.VariableSelector)
			}

			result, err = processSubConditions(files, condition.SubVariableCondition)
			if err != nil {
				return nil, groupResults, false, err
			}

		} else if condition.ComparisonOperator == ComparisonOperatorExists || condition.ComparisonOperator == ComparisonOperatorNotExists {
			var val any
			if variable != nil {
				val = variable.GetValue()
			}
			result, err = evaluateCondition(condition.ComparisonOperator, val, nil)
			if err != nil {
				return nil, groupResults, false, err
			}

		} else {
			expected, err := prepareExpectedValue(vp, variable, condition.Value)
			if err != nil {
				return nil, groupResults, false, err
			}

			var actual any
			if variable != nil {
				actual = variable.GetValue()
			}

			result, err = evaluateCondition(condition.ComparisonOperator, actual, expected)
			if err != nil {
				return nil, groupResults, false, err
			}

			inputConditions = append(inputConditions, map[string]any{
				"actual_value":        actual,
				"expected_value":      expected,
				"comparison_operator": condition.ComparisonOperator,
				"variable_selector":   condition.VariableSelector,
			})
		}

		groupResults = append(groupResults, result)

		if (logicalOp == "and" && !result) || (logicalOp == "or" && result) {
			return inputConditions, groupResults, result, nil
		}
	}

	finalResult := false
	if logicalOp == "and" {
		if len(groupResults) == 0 {
			finalResult = true
		} else {
			finalResult = true
			for _, r := range groupResults {
				if !r {
					finalResult = false
					break
				}
			}
		}
	} else {
		for _, r := range groupResults {
			if r {
				finalResult = true
				break
			}
		}
	}

	return inputConditions, groupResults, finalResult, nil
}

func prepareExpectedValue(vp *entities.VariablePool, variable entities.Variable, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}

	if str, ok := raw.(string); ok {
		raw = vp.ConvertTemplate(str).Text()
	}

	// If variable is nil (missing), we can't determine type, so return raw value as is
	// or treat as string/default depending on raw value type?
	// The current logic usually infers or uses the raw value.
	if variable == nil {
		return raw, nil
	}

	switch variable.GetType() {
	case shared.SegmentTypeBoolean:
		return convertToBool(raw)
	case shared.SegmentTypeArrayBoolean:
		items, err := toInterfaceSlice(raw)
		if err != nil {
			return nil, err
		}
		result := make([]bool, len(items))
		for i, item := range items {
			b, err := convertToBool(item)
			if err != nil {
				return nil, err
			}
			result[i] = b
		}
		return result, nil
	default:
		return raw, nil
	}
}

func evaluateCondition(operator ComparisonOperator, value, expected any) (bool, error) {
	switch operator {
	case ComparisonOperatorContains:
		return assertContains(value, expected)
	case ComparisonOperatorNotContains:
		ok, err := assertContains(value, expected)
		return !ok, err
	case ComparisonOperatorStartWith:
		actual, err := toString(value)
		if err != nil {
			return false, err
		}
		exp, err := toString(expected)
		if err != nil {
			return false, err
		}
		return strings.HasPrefix(actual, exp), nil
	case ComparisonOperatorEndWith:
		actual, err := toString(value)
		if err != nil {
			return false, err
		}
		exp, err := toString(expected)
		if err != nil {
			return false, err
		}
		return strings.HasSuffix(actual, exp), nil
	case ComparisonOperatorIs, ComparisonOperatorEqual:
		return deepEqual(value, expected), nil
	case ComparisonOperatorIsNot, ComparisonOperatorNotEqual:
		return !deepEqual(value, expected), nil
	case ComparisonOperatorEmpty:
		return isEmpty(value), nil
	case ComparisonOperatorNotEmpty:
		return !isEmpty(value), nil
	case ComparisonOperatorGreaterThan:
		return compareNumber(value, expected, func(a, b float64) bool { return a > b })
	case ComparisonOperatorLessThan:
		return compareNumber(value, expected, func(a, b float64) bool { return a < b })
	case ComparisonOperatorGreaterEq:
		return compareNumber(value, expected, func(a, b float64) bool { return a >= b })
	case ComparisonOperatorLessEq:
		return compareNumber(value, expected, func(a, b float64) bool { return a <= b })
	case ComparisonOperatorNull:
		return value == nil, nil
	case ComparisonOperatorNotNull:
		return value != nil, nil
	case ComparisonOperatorIn:
		return assertIn(value, expected)
	case ComparisonOperatorNotIn:
		ok, err := assertIn(value, expected)
		return !ok, err
	case ComparisonOperatorAllOf:
		return assertAllOf(value, expected)
	case ComparisonOperatorExists:
		return value != nil, nil
	case ComparisonOperatorNotExists:
		return value == nil, nil
	default:
		return false, fmt.Errorf("unsupported comparison operator: %s", operator)
	}
}

func assertContains(value, expected any) (bool, error) {
	if value == nil {
		return false, nil
	}

	if str, ok := value.(string); ok {
		exp, err := toString(expected)
		if err != nil {
			return false, err
		}
		return strings.Contains(str, exp), nil
	}

	slice, err := toInterfaceSlice(value)
	if err != nil {
		return false, fmt.Errorf("invalid actual value for contains: %T", value)
	}

	for _, item := range slice {
		if deepEqual(item, expected) {
			return true, nil
		}
	}
	return false, nil
}

func assertIn(value, expected any) (bool, error) {
	if expected == nil {
		return false, fmt.Errorf("expected value for 'in' operator cannot be nil")
	}

	slice, err := toInterfaceSlice(expected)
	if err != nil {
		return false, fmt.Errorf("expected value for 'in' operator must be array, got %T", expected)
	}

	for _, item := range slice {
		if deepEqual(item, value) {
			return true, nil
		}
	}
	return false, nil
}

func assertAllOf(value, expected any) (bool, error) {
	if expected == nil {
		return false, fmt.Errorf("expected value for 'all of' operator cannot be nil")
	}

	sliceExpected, err := toInterfaceSlice(expected)
	if err != nil {
		return false, fmt.Errorf("expected value for 'all of' operator must be array, got %T", expected)
	}

	valueSlice, err := toInterfaceSlice(value)
	if err != nil {
		if strVal, strErr := toString(value); strErr == nil {
			valueSlice = make([]any, len(strVal))
			for i, r := range strVal {
				valueSlice[i] = string(r)
			}
		} else {
			return false, fmt.Errorf("value for 'all of' operator must be iterable, got %T", value)
		}
	}

	for _, expectedItem := range sliceExpected {
		if !containsValue(valueSlice, expectedItem) {
			return false, nil
		}
	}
	return true, nil
}

func compareNumber(value, expected any, cmp func(a, b float64) bool) (bool, error) {
	actualNum, err := toFloat(value)
	if err != nil {
		return false, err
	}
	expectedNum, err := toFloat(expected)
	if err != nil {
		return false, err
	}
	return cmp(actualNum, expectedNum), nil
}

func toFloat(value any) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case json.Number:
		return v.Float64()
	case string:
		if v == "" {
			return 0, fmt.Errorf("cannot convert empty string to number")
		}
		num, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string %q to number: %w", v, err)
		}
		return num, nil
	default:
		return 0, fmt.Errorf("value %T is not numeric", value)
	}
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []byte:
		return len(v) == 0
	case []string:
		return len(v) == 0
	case []bool:
		return len(v) == 0
	case []int:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
		return rv.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return rv.IsNil()
	}

	return false
}

func toInterfaceSlice(value any) ([]any, error) {
	if value == nil {
		return nil, fmt.Errorf("nil value cannot be treated as slice")
	}

	switch v := value.(type) {
	case []any:
		return v, nil
	case []string:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case []bool:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	case []int:
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
	case []json.Number:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	}

	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		length := rv.Len()
		result := make([]any, length)
		for i := 0; i < length; i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result, nil
	}

	return nil, fmt.Errorf("value %T is not a slice", value)
}

func toString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case json.Number:
		return v.String(), nil
	case nil:
		return "", fmt.Errorf("cannot convert nil to string")
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

func deepEqual(a, b any) bool {
	return reflect.DeepEqual(normalizeJSONNumber(a), normalizeJSONNumber(b))
}

func convertToBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return false, fmt.Errorf("invalid json number for bool: %w", err)
		}
		return f != 0, nil
	case string:
		var decoded any
		if err := json.Unmarshal([]byte(v), &decoded); err == nil {
			return convertToBool(decoded)
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			return num != 0, nil
		}
		return false, fmt.Errorf("unexpected string value for bool: %q", v)
	default:
		return false, fmt.Errorf("unsupported type for bool conversion: %T", value)
	}
}

func normalizeJSONNumber(value any) any {
	switch v := value.(type) {
	case []any:
		res := make([]any, len(v))
		for i, item := range v {
			res[i] = normalizeJSONNumber(item)
		}
		return res
	case map[string]any:
		res := make(map[string]any, len(v))
		for key, item := range v {
			res[key] = normalizeJSONNumber(item)
		}
		return res
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return v
	}
}

func containsValue(slice []any, target any) bool {
	for _, item := range slice {
		if deepEqual(item, target) {
			return true
		}
	}
	return false
}

func requiresSubConditions(op ComparisonOperator) bool {
	switch op {
	case ComparisonOperatorContains, ComparisonOperatorNotContains, ComparisonOperatorAllOf:
		return true
	default:
		return false
	}
}

func processSubConditions(files []*entities.File, sub *SubVariableCondition) (bool, error) {
	if sub == nil {
		return false, fmt.Errorf("sub variable condition cannot be nil")
	}

	groupResults := make([]bool, 0, len(sub.Conditions))
	for _, condition := range sub.Conditions {
		values := make([]any, 0, len(files))
		fileAttr := entities.FileAttribute(condition.Key)
		for _, file := range files {
			values = append(values, getFileAttribute(file, fileAttr))
		}

		expected := condition.Value
		if fileAttr == entities.FileAttributeExtension {
			expStr, err := toString(expected)
			if err != nil {
				return false, err
			}
			if expStr != "" && !strings.HasPrefix(expStr, ".") {
				expStr = "." + expStr
			}
			expected = expStr

			for i, value := range values {
				if valueStr, ok := value.(string); ok && valueStr != "" && !strings.HasPrefix(valueStr, ".") {
					values[i] = "." + valueStr
				}
			}
		}

		subResults := make([]bool, 0, len(values))
		for _, value := range values {
			result, err := evaluateCondition(condition.ComparisonOperator, value, expected)
			if err != nil {
				return false, err
			}
			subResults = append(subResults, result)
		}

		if strings.Contains(string(condition.ComparisonOperator), "not") {
			groupResults = append(groupResults, all(subResults))
		} else {
			groupResults = append(groupResults, anyBool(subResults))
		}
	}

	if strings.ToLower(string(sub.LogicalOperator)) == "or" {
		return anyBool(groupResults), nil
	}

	return all(groupResults), nil
}

func anyBool(items []bool) bool {
	for _, item := range items {
		if item {
			return true
		}
	}
	return false
}

func all(items []bool) bool {
	for _, item := range items {
		if !item {
			return false
		}
	}
	return true
}

func getFileAttribute(file *entities.File, attr entities.FileAttribute) any {
	if file == nil {
		return nil
	}

	switch attr {
	case entities.FileAttributeURL:
		return file.RemoteURL
	case entities.FileAttributeName:
		return file.Filename
	case entities.FileAttributeSize:
		return file.Size
	case entities.FileAttributeType:
		return file.Type
	case entities.FileAttributeExtension:
		return file.Extension
	case entities.FileAttributeMimeType:
		return file.MimeType
	case entities.FileAttributeTransferMethod:
		return file.TransferMethod
	default:
		return nil
	}
}

func caseToMap(c Case) (map[string]any, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

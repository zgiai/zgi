package iter

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	"github.com/zgiai/ginext/internal/modules/tools"
)

// New creates iteration node instance from workflow config.
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
			NodeType:   shared.Iteration,

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

	// Check optionalDeps for LLMClient
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

	payload, err := jsonMarshal(rawData)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := jsonUnmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	nodeData.ParallelNums = graph_engine.NormalizeMaxConcurrency(nodeData.ParallelNums)
	if nodeData.ErrorHandleMode == "" {
		nodeData.ErrorHandleMode = ErrorHandleModeTerminated
	}

	return nodeData, nodeID, nil
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

// Run executes iteration node and emits workflow events.
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Start event
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
	iteratorValues, iteratorRaw, err := n.fetchIteratorValues()
	if err != nil {
		return nil, err
	}

	inputs := n.buildInputSnapshot(iteratorRaw)

	if len(iteratorValues) == 0 {
		return n.handleEmptyIteration(inputs), nil
	}

	if n.nodeData.StartNodeID == nil || len(*n.nodeData.StartNodeID) == 0 {
		return nil, errStartNodeIDMissing
	}

	startedAt := n.now()
	n.emitIterationStarted(ctx, eventChan, startedAt, inputs, len(iteratorValues))

	iterRunMap := make(map[string]float64, len(iteratorValues))
	iterDurationList := make([]float64, len(iteratorValues))
	iterExecutionTrace := make([]map[string]any, len(iteratorValues))
	var outputs []any

	if n.nodeData.IsParallel {
		outputs = make([]any, len(iteratorValues))
		err = n.executeParallelIterations(
			ctx,
			iteratorValues,
			&outputs,
			iterRunMap,
			iterDurationList,
			iterExecutionTrace,
			eventChan,
		)
	} else {
		outputs = make([]any, 0, len(iteratorValues))
		err = n.executeSequentialIterations(
			ctx,
			iteratorValues,
			&outputs,
			iterRunMap,
			iterDurationList,
			iterExecutionTrace,
			eventChan,
		)
	}

	metadata := map[shared.WorkflowNodeExecutionMetadataKey]any{
		shared.IterationDurationMap:    iterRunMap,
		shared.IterationDurationList:   iterDurationList,
		shared.IterationExecutionTrace: iterExecutionTrace,
		shared.TotalTokens:             n.totalTokens(),
	}

	if err != nil {
		flattened := flattenOutputsIfNeeded(outputs)
		n.emitIterationFailed(
			ctx,
			eventChan,
			startedAt,
			inputs,
			map[string]any{"output": flattened},
			len(iteratorValues),
			metadata,
			err,
		)
		return &shared.NodeRunResult{
			Status:   shared.FAILED,
			Inputs:   inputs,
			Outputs:  map[string]any{"output": flattened},
			Metadata: metadata,
			ErrMsg:   err.Error(),
		}, err
	}

	finalOutputs := flattenOutputsIfNeeded(outputs)

	n.emitIterationSucceeded(
		ctx,
		eventChan,
		startedAt,
		inputs,
		map[string]any{"output": finalOutputs},
		len(iteratorValues),
		metadata,
	)

	return &shared.NodeRunResult{
		Status:   shared.SUCCEEDED,
		Inputs:   inputs,
		Outputs:  map[string]any{"output": finalOutputs},
		Metadata: metadata,
	}, nil
}

func (n *Node) fetchIteratorValues() ([]any, any, error) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, nil, errIteratorVariableNotFound
	}

	variable := n.GraphRuntimeState.VariablePool.GetWithPath(n.nodeData.IteratorSelector)
	if variable == nil {
		return nil, nil, fmt.Errorf("%w: %v", errIteratorVariableNotFound, n.nodeData.IteratorSelector)
	}

	rawValue := variable.ToObject()
	values, err := normalizeToAnySlice(rawValue)
	if err != nil {
		return nil, rawValue, errInvalidIteratorValue
	}

	return values, rawValue, nil
}

func (n *Node) buildInputSnapshot(iteratorRaw any) map[string]any {
	inputs := map[string]any{}
	if key := selectorDisplayKey(n.nodeData.IteratorSelector); key != "" {
		inputs[key] = iteratorRaw
	}
	if value, ok := n.resolveSelectorValue(n.nodeData.OutputSelector); ok {
		if key := selectorDisplayKey(n.nodeData.OutputSelector); key != "" {
			inputs[key] = value
		}
	}
	return inputs
}

func (n *Node) resolveSelectorValue(selector []string) (any, bool) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || len(selector) < 2 {
		return nil, false
	}
	variable := n.GraphRuntimeState.VariablePool.GetWithPath(selector)
	if variable == nil {
		return nil, false
	}
	return variable.ToObject(), true
}

func selectorDisplayKey(selector []string) string {
	if len(selector) == 0 {
		return ""
	}
	return strings.TrimSpace(selector[len(selector)-1])
}

func (n *Node) handleEmptyIteration(inputs map[string]any) *shared.NodeRunResult {
	return &shared.NodeRunResult{
		Status: shared.SUCCEEDED,
		Inputs: inputs,
		Outputs: map[string]any{
			"output": []any{},
		},
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
			shared.IterationDurationMap:    map[string]float64{},
			shared.IterationDurationList:   []float64{},
			shared.IterationExecutionTrace: []map[string]any{},
			shared.TotalTokens:             n.totalTokens(),
		},
	}
}

func (n *Node) executeSequentialIterations(
	ctx context.Context,
	items []any,
	outputs *[]any,
	iterRunMap map[string]float64,
	iterDurationList []float64,
	iterExecutionTrace []map[string]any,
	eventChan chan *shared.NodeEventCh,
) error {
	for index, item := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n.emitIterationNext(ctx, eventChan, index)

		iterStart := n.now()
		outcome, err := n.runSingleIteration(ctx, index, item, eventChan)
		iterFinish := n.now()
		duration := shared.DurationMilliseconds(iterFinish.Sub(iterStart))
		iterDurationList[index] = duration
		if len(outcome.ConversationSnapshot) > 0 {
			n.syncConversationVariables(outcome.ConversationSnapshot)
		}

		if err != nil {
			iterExecutionTrace[index] = buildIterationTrace(index, iterStart, iterFinish, "failed")
			switch n.nodeData.ErrorHandleMode {
			case ErrorHandleModeContinueOnError:
				n.addTokens(outcome.Tokens)
				*outputs = append(*outputs, nil)
				iterRunMap[strconv.Itoa(index)] = duration
				continue
			case ErrorHandleModeRemoveAbnormalOutput:
				n.addTokens(outcome.Tokens)
				iterRunMap[strconv.Itoa(index)] = duration
				continue
			default:
				return err
			}
		}

		*outputs = append(*outputs, outcome.Output)
		iterRunMap[strconv.Itoa(index)] = duration
		iterExecutionTrace[index] = buildIterationTrace(index, iterStart, iterFinish, "succeeded")
		n.addTokens(outcome.Tokens)
	}

	if n.nodeData.ErrorHandleMode == ErrorHandleModeRemoveAbnormalOutput {
		filtered := make([]any, 0, len(*outputs))
		for _, output := range *outputs {
			if output != nil {
				filtered = append(filtered, output)
			}
		}
		*outputs = filtered
	}

	return nil
}

func (n *Node) executeParallelIterations(
	ctx context.Context,
	items []any,
	outputs *[]any,
	iterRunMap map[string]float64,
	iterDurationList []float64,
	iterExecutionTrace []map[string]any,
	eventChan chan *shared.NodeEventCh,
) error {
	if len(items) == 0 {
		return nil
	}

	maxWorkers := n.nodeData.ParallelNums
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	if maxWorkers > len(items) {
		maxWorkers = len(items)
	}

	type iterationJob struct {
		index int
		item  any
	}

	type iterationResult struct {
		index      int
		output     any
		tokensUsed int
		duration   float64
		startedAt  time.Time
		finishedAt time.Time
		snapshot   map[string]any
		err        error
	}

	jobCh := make(chan iterationJob)
	resultCh := make(chan iterationResult, len(items))

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for job := range jobCh {
			select {
			case <-ctx.Done():
				cancelledAt := n.now()
				resultCh <- iterationResult{
					index:      job.index,
					startedAt:  cancelledAt,
					finishedAt: cancelledAt,
					err:        ctx.Err(),
				}
				continue
			default:
			}

			start := n.now()
			outcome, err := n.runSingleIteration(ctx, job.index, job.item, eventChan)
			finished := n.now()
			duration := shared.DurationMilliseconds(finished.Sub(start))

			resultCh <- iterationResult{
				index:      job.index,
				output:     outcome.Output,
				tokensUsed: outcome.Tokens,
				snapshot:   outcome.ConversationSnapshot,
				duration:   duration,
				startedAt:  start,
				finishedAt: finished,
				err:        err,
			}
		}
	}

	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	for index, item := range items {
		n.emitIterationNext(ctx, eventChan, index)
		jobCh <- iterationJob{index: index, item: item}
	}
	close(jobCh)

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var firstErr error

	for res := range resultCh {
		if res.err != nil {
			iterExecutionTrace[res.index] = buildIterationTrace(res.index, res.startedAt, res.finishedAt, "failed")
			iterDurationList[res.index] = res.duration
			if len(res.snapshot) > 0 {
				n.syncConversationVariables(res.snapshot)
			}
			switch n.nodeData.ErrorHandleMode {
			case ErrorHandleModeContinueOnError:
				n.addTokens(res.tokensUsed)
				(*outputs)[res.index] = nil
				iterRunMap[strconv.Itoa(res.index)] = res.duration
				continue
			case ErrorHandleModeRemoveAbnormalOutput:
				n.addTokens(res.tokensUsed)
				(*outputs)[res.index] = nil
				iterRunMap[strconv.Itoa(res.index)] = res.duration
				continue
			default:
				if firstErr == nil {
					firstErr = res.err
				}
				continue
			}
		}

		if len(res.snapshot) > 0 {
			n.syncConversationVariables(res.snapshot)
		}

		(*outputs)[res.index] = res.output
		iterRunMap[strconv.Itoa(res.index)] = res.duration
		iterDurationList[res.index] = res.duration
		iterExecutionTrace[res.index] = buildIterationTrace(res.index, res.startedAt, res.finishedAt, "succeeded")
		n.addTokens(res.tokensUsed)
	}

	if n.nodeData.ErrorHandleMode == ErrorHandleModeRemoveAbnormalOutput {
		filtered := make([]any, 0, len(*outputs))
		for _, output := range *outputs {
			if output != nil {
				filtered = append(filtered, output)
			}
		}
		*outputs = filtered
	}

	if firstErr != nil && n.nodeData.ErrorHandleMode == ErrorHandleModeTerminated {
		return firstErr
	}

	return nil
}

func (n *Node) emitIterationStarted(
	ctx context.Context,
	eventChan chan *shared.NodeEventCh,
	startAt time.Time,
	inputs map[string]any,
	length int,
) {
	if eventChan == nil {
		return
	}

	event := &IterationStartedEvent{
		StartAt:  startAt,
		Inputs:   cloneAnyMap(inputs),
		Metadata: map[string]any{"iteration_length": length},
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeIterationStarted, event)
}

func (n *Node) emitIterationNext(
	ctx context.Context,
	eventChan chan *shared.NodeEventCh,
	index int,
) {
	if eventChan == nil {
		return
	}
	event := &IterationNextEvent{Index: index}
	n.emitEvent(ctx, eventChan, shared.EventTypeIterationNext, event)
}

func (n *Node) emitIterationSucceeded(
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
	event := &IterationSucceededEvent{
		StartAt:  startAt,
		Inputs:   cloneAnyMap(inputs),
		Outputs:  cloneAnyMap(outputs),
		Steps:    steps,
		Metadata: convertMetadata(metadata),
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeIterationSucceeded, event)
}

func (n *Node) emitIterationFailed(
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
	event := &IterationFailedEvent{
		StartAt:  startAt,
		Inputs:   cloneAnyMap(inputs),
		Outputs:  cloneAnyMap(outputs),
		Steps:    steps,
		Metadata: convertMetadata(metadata),
		Error:    err.Error(),
	}
	n.emitEvent(ctx, eventChan, shared.EventTypeIterationFailed, event)
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

func buildIterationTrace(index int, startedAt, finishedAt time.Time, status string) map[string]any {
	return map[string]any{
		"index":          index,
		"status":         status,
		"started_at_ms":  startedAt.UnixMilli(),
		"finished_at_ms": finishedAt.UnixMilli(),
		"duration_ms":    shared.DurationMilliseconds(finishedAt.Sub(startedAt)),
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

func convertMetadata(
	metadata map[shared.WorkflowNodeExecutionMetadataKey]any,
) map[string]any {
	if metadata == nil {
		return nil
	}
	result := make(map[string]any, len(metadata))
	for key, value := range metadata {
		result[string(key)] = value
	}
	return result
}

func (n *Node) syncConversationVariables(snapshot map[string]any) {
	if len(snapshot) == 0 || n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return
	}

	parentPool := n.GraphRuntimeState.VariablePool
	existing := parentPool.VariableDictionary[entities.ConversationVariableNodeId]

	for key := range existing {
		if _, ok := snapshot[key]; !ok {
			parentPool.Remove([]string{entities.ConversationVariableNodeId, key})
		}
	}

	for key, value := range snapshot {
		parentPool.Add([]string{entities.ConversationVariableNodeId, key}, value)
	}
}

func extractConversationSnapshot(pool *entities.VariablePool) map[string]any {
	if pool == nil {
		return nil
	}
	conversations := pool.VariableDictionary[entities.ConversationVariableNodeId]
	if conversations == nil {
		return nil
	}

	snapshot := make(map[string]any, len(conversations))
	for name, variable := range conversations {
		if variable == nil {
			continue
		}
		snapshot[name] = variable.ToObject()
	}
	return snapshot
}

func (n *Node) runSingleIteration(ctx context.Context, index int, item any, eventChan chan *shared.NodeEventCh) (iterationRunOutcome, error) {
	if n.nodeData.StartNodeID == nil || len(*n.nodeData.StartNodeID) == 0 {
		return iterationRunOutcome{}, errStartNodeIDMissing
	}

	result, err := subgraph.New(subgraph.Config{
		NodeID:         n.NodeID,
		StartNodeID:    n.nodeData.StartNodeID,
		OutputSelector: n.nodeData.OutputSelector,
		GraphConfig:    n.GraphConfig,
		RuntimeState:   n.GraphRuntimeState,
		Parallelism:    n.nodeData.ParallelNums,
		EventChan:      eventChan,
		EngineFactory:  n.engineFactory,
	}).Run(ctx, index, item)
	if err != nil {
		return iterationRunOutcome{}, err
	}

	return iterationRunOutcome{
		Output:               result.Output,
		Tokens:               result.Tokens,
		ConversationSnapshot: result.ConversationSnapshot,
	}, nil
}

func normalizeToAnySlice(value any) ([]any, error) {
	if value == nil {
		return []any{}, nil
	}

	switch v := value.(type) {
	case []any:
		return v, nil
	case []string:
		res := make([]any, 0, len(v))
		for _, element := range v {
			res = append(res, element)
		}
		return res, nil
	case []int:
		res := make([]any, 0, len(v))
		for _, element := range v {
			res = append(res, element)
		}
		return res, nil
	case []float64:
		res := make([]any, 0, len(v))
		for _, element := range v {
			res = append(res, element)
		}
		return res, nil
	case []float32:
		res := make([]any, 0, len(v))
		for _, element := range v {
			res = append(res, float64(element))
		}
		return res, nil
	default:
		val := reflect.ValueOf(value)
		if val.Kind() == reflect.Slice {
			length := val.Len()
			res := make([]any, 0, length)
			for i := 0; i < length; i++ {
				res = append(res, val.Index(i).Interface())
			}
			return res, nil
		}

		return nil, errInvalidIteratorValue
	}
}

func flattenOutputsIfNeeded(outputs []any) []any {
	if len(outputs) == 0 {
		return outputs
	}

	hasNonNil := false
	allSlices := true

	for _, output := range outputs {
		if output == nil {
			continue
		}
		hasNonNil = true
		if reflect.TypeOf(output).Kind() != reflect.Slice {
			allSlices = false
			break
		}
	}

	if !hasNonNil || !allSlices {
		return outputs
	}

	flattened := make([]any, 0, len(outputs))
	for _, output := range outputs {
		if output == nil {
			continue
		}
		val := reflect.ValueOf(output)
		if val.Kind() != reflect.Slice {
			flattened = append(flattened, output)
			continue
		}
		for i := 0; i < val.Len(); i++ {
			flattened = append(flattened, val.Index(i).Interface())
		}
	}

	return flattened
}

// jsonMarshal/jsonUnmarshal wrappers allow test overrides without importing encoding/json everywhere.
var (
	jsonMarshal   = defaultJSONMarshal
	jsonUnmarshal = defaultJSONUnmarshal
)

func defaultJSONMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func defaultJSONUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// IterationStartedEvent represents the iteration start payload.
type IterationStartedEvent struct {
	StartAt  time.Time      `json:"start_at"`
	Inputs   map[string]any `json:"inputs"`
	Metadata map[string]any `json:"metadata"`
}

// IterationNextEvent represents the next iteration payload.
type IterationNextEvent struct {
	Index int `json:"index"`
}

// IterationSucceededEvent represents the iteration success payload.
type IterationSucceededEvent struct {
	StartAt  time.Time      `json:"start_at"`
	Inputs   map[string]any `json:"inputs"`
	Outputs  map[string]any `json:"outputs"`
	Steps    int            `json:"steps"`
	Metadata map[string]any `json:"metadata"`
}

// IterationFailedEvent represents the iteration failure payload.
type IterationFailedEvent struct {
	StartAt  time.Time      `json:"start_at"`
	Inputs   map[string]any `json:"inputs"`
	Outputs  map[string]any `json:"outputs"`
	Steps    int            `json:"steps"`
	Metadata map[string]any `json:"metadata"`
	Error    string         `json:"error"`
}

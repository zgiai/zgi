package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	graph_entities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflow_shared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	// Answer emitter lifecycle states.
	answerEmitterUnknown   = "unknown"
	answerEmitterEligible  = "eligible"
	answerEmitterActive    = "active"
	answerEmitterCompleted = "completed"
	answerEmitterSkipped   = "skipped"
	answerEmitterFailed    = "failed"

	// Template segment kinds.
	answerSegmentStatic   = "static"
	answerSegmentVariable = "variable"

	answerScopeTop       = "top"
	answerScopeIteration = "iteration"
)

// answerOutputCoordinator serializes conversation answer output according to
// template order, answer lifecycle, and topological dependencies between answer nodes.
type answerOutputCoordinator struct {
	emitMu sync.Mutex
	mu     sync.Mutex

	workflowRunID  string
	conversationID string
	resultChan     chan<- *WorkflowStreamEvent

	chunkSize int

	emitters      map[string]*answerEmitter
	emitterOrder  []string
	templates     map[string]string
	nodeOrder     map[string]int
	nodeTypes     map[string]string
	edgeMap       map[string]map[string][]string
	reachable     map[string]bool
	variables     map[string]*answerVariableState
	varsByStream  map[string][]*answerVariableState
	varsBySource  map[string][]*answerVariableState
	answerParents map[string]map[string]bool
	iterationNext map[string]int

	fullAnswer  strings.Builder
	messageSent bool
}

// answerEmitter tracks the release state for a single answer node.
type answerEmitter struct {
	key                 string
	nodeID              string
	order               int
	parentOrder         int
	scope               answerOutputScope
	lifecycle           string
	templateFingerprint string
	segments            []answerTemplateSegment
	currentIndex        int
	drained             bool
	batchBarrierPassed  bool
	batchBlockers       map[string]bool
}

type answerOutputScope struct {
	kind         string
	parentNodeID string
	index        int
}

// answerTemplateSegment represents either fixed text or one variable placeholder.
type answerTemplateSegment struct {
	kind        string
	text        string
	selector    []string
	selectorKey string
	stateKey    string
}

// answerVariableState tracks the stream/finalization state for one placeholder instance.
type answerVariableState struct {
	stateKey    string
	selector    []string
	selectorKey string
	sourceNode  string
	outputKey   string
	scope       answerOutputScope

	chunks           []string
	releasedText     string
	hasFinal         bool
	finalValue       string
	finalOnly        bool
	sourceSkipped    bool
	sourceFailed     bool
	finalizedSegment bool
}

// answerMessageChunk is the internal message payload emitted to SSE.
type answerMessageChunk struct {
	nodeID string
	text   string
}

// newAnswerOutputCoordinator builds the ordered-output coordinator for conversation workflows.
func newAnswerOutputCoordinator(
	runType string,
	workflowRunID string,
	systemInputs map[string]interface{},
	streamGraph *workflowStreamGraph,
	resultChan chan<- *WorkflowStreamEvent,
) *answerOutputCoordinator {
	if runType != "CONVERSATION_WORKFLOW" || streamGraph == nil || resultChan == nil {
		return nil
	}

	runtimeNodeMap := streamGraph.RuntimeNodeMap
	if runtimeNodeMap == nil {
		runtimeNodeMap = streamGraph.NodeMap
	}

	nodeOrder := workflowGraphNodeOrder(streamGraph.GraphData)
	nodeTypes := workflowGraphNodeTypes(runtimeNodeMap)
	coordinator := &answerOutputCoordinator{
		workflowRunID:  workflowRunID,
		conversationID: workflowConversationIDFromSystemInputs(systemInputs),
		resultChan:     resultChan,
		chunkSize:      answerOutputChunkSize(),
		emitters:       make(map[string]*answerEmitter),
		templates:      make(map[string]string),
		nodeOrder:      nodeOrder,
		nodeTypes:      nodeTypes,
		edgeMap:        streamGraph.EdgeMap,
		reachable:      make(map[string]bool),
		variables:      make(map[string]*answerVariableState),
		varsByStream:   make(map[string][]*answerVariableState),
		varsBySource:   make(map[string][]*answerVariableState),
		answerParents:  make(map[string]map[string]bool),
		iterationNext:  make(map[string]int),
	}

	allAnswerEmitters := make(map[string]*answerEmitter)
	for _, nodeID := range workflowGraphOrderedNodeIDs(nodeOrder) {
		node := runtimeNodeMap[nodeID]
		data, _ := node["data"].(map[string]interface{})
		if data == nil {
			continue
		}
		if nodeType, _ := data["type"].(string); nodeType != "answer" {
			continue
		}
		template, _ := data["answer"].(string)
		coordinator.templates[nodeID] = template
		allAnswerEmitters[nodeID] = &answerEmitter{nodeID: nodeID}
		if !workflowGraphNodeIsTopLevelAnswer(node) {
			continue
		}
		coordinator.ensureEmitterLocked(answerTopScope(), nodeID)
	}
	if len(coordinator.templates) == 0 {
		return nil
	}

	coordinator.answerParents = buildAnswerPredecessorMap(allAnswerEmitters, streamGraph.EdgeMap)
	coordinator.markReachableFromHandlesLocked(streamGraph.StartNodeID, []string{"source"})
	return coordinator
}

// answerOutputChunkSize returns the fallback chunk size used by final-only replay.
func answerOutputChunkSize() int {
	if config.GlobalConfig == nil || config.GlobalConfig.AnswerNodeStreaming.ChunkSize <= 0 {
		return 20
	}
	return config.GlobalConfig.AnswerNodeStreaming.ChunkSize
}

func answerTopScope() answerOutputScope {
	return answerOutputScope{kind: answerScopeTop, index: -1}
}

func answerScopeFromReadyBatch(scope graph_engine.ReadyBatchScope) answerOutputScope {
	if scope.Kind == graph_engine.ReadyBatchScopeIteration {
		return answerOutputScope{kind: answerScopeIteration, parentNodeID: scope.ParentNodeID, index: scope.Index}
	}
	return answerTopScope()
}

func answerScopeFromStream(scope *workflow_shared.RunStreamScope) answerOutputScope {
	if scope != nil && scope.Kind == graph_engine.ReadyBatchScopeIteration {
		return answerOutputScope{kind: answerScopeIteration, parentNodeID: scope.ParentNodeID, index: scope.Index}
	}
	return answerTopScope()
}

func answerScopeFromMetadata(metadata map[string]interface{}) (answerOutputScope, bool) {
	if metadata == nil {
		return answerTopScope(), false
	}
	iterationID, _ := metadata[string(workflow_shared.ITERATION_ID)].(string)
	if iterationID == "" {
		return answerTopScope(), false
	}
	index, ok := intFromAny(metadata[string(workflow_shared.IterationIndex)])
	if !ok {
		return answerTopScope(), false
	}
	return answerOutputScope{kind: answerScopeIteration, parentNodeID: iterationID, index: index}, true
}

func answerIterationContextFromMetadata(metadata map[string]interface{}) (map[string]any, bool) {
	if metadata == nil {
		return nil, false
	}
	outputs := make(map[string]any)
	if item, ok := metadata[string(workflow_shared.IterationItem)]; ok {
		outputs["item"] = item
	}
	if index, ok := metadata[string(workflow_shared.IterationIndex)]; ok {
		outputs["index"] = index
	}
	if len(outputs) == 0 {
		return nil, false
	}
	return outputs, true
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	default:
		return 0, false
	}
}

func answerScopeKey(scope answerOutputScope) string {
	if scope.kind == answerScopeIteration {
		return fmt.Sprintf("%s:%s:%d", answerScopeIteration, scope.parentNodeID, scope.index)
	}
	return answerScopeTop
}

func answerEmitterKey(scope answerOutputScope, nodeID string) string {
	return answerScopeKey(scope) + "|" + nodeID
}

func scopedSelectorKey(scope answerOutputScope, selector []string) string {
	return answerScopeKey(scope) + "|" + streamSelectorKey(selector)
}

func scopedSourceKey(scope answerOutputScope, nodeID string) string {
	return answerScopeKey(scope) + "|" + nodeID
}

func answerEmitterSameScope(left *answerEmitter, right *answerEmitter) bool {
	if left == nil || right == nil {
		return false
	}
	return answerScopeKey(left.scope) == answerScopeKey(right.scope)
}

func compareAnswerEmitters(left *answerEmitter, right *answerEmitter) int {
	if left.scope.kind != right.scope.kind {
		if left.scope.kind == answerScopeTop {
			return -1
		}
		if right.scope.kind == answerScopeTop {
			return 1
		}
	}
	if left.scope.parentNodeID != right.scope.parentNodeID {
		if left.parentOrder != right.parentOrder {
			return left.parentOrder - right.parentOrder
		}
		return strings.Compare(left.scope.parentNodeID, right.scope.parentNodeID)
	}
	if left.scope.index != right.scope.index {
		return left.scope.index - right.scope.index
	}
	if left.order != right.order {
		return left.order - right.order
	}
	return strings.Compare(left.nodeID, right.nodeID)
}

func sortAnswerEmitters(emitters []*answerEmitter) {
	sort.SliceStable(emitters, func(i, j int) bool {
		return compareAnswerEmitters(emitters[i], emitters[j]) < 0
	})
}

func (c *answerOutputCoordinator) ensureEmitterLocked(scope answerOutputScope, nodeID string) *answerEmitter {
	if scope.kind == "" {
		scope = answerTopScope()
	}
	key := answerEmitterKey(scope, nodeID)
	if emitter := c.emitters[key]; emitter != nil {
		return emitter
	}
	template, ok := c.templates[nodeID]
	if !ok {
		return nil
	}
	segments := c.parseTemplateSegments(scope, template)
	parentOrder := c.nodeOrder[nodeID]
	if scope.kind == answerScopeIteration {
		if order, ok := c.nodeOrder[scope.parentNodeID]; ok {
			parentOrder = order
		}
	}
	emitter := &answerEmitter{
		key:                 key,
		nodeID:              nodeID,
		order:               c.nodeOrder[nodeID],
		parentOrder:         parentOrder,
		scope:               scope,
		lifecycle:           answerEmitterUnknown,
		segments:            segments,
		templateFingerprint: answerTemplateSegmentsFingerprint(segments),
		batchBlockers:       make(map[string]bool),
	}
	c.emitters[key] = emitter
	c.emitterOrder = append(c.emitterOrder, key)
	return emitter
}

func (c *answerOutputCoordinator) Enabled() bool {
	return c != nil
}

func (c *answerOutputCoordinator) HasOutput() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.messageSent
}

func (c *answerOutputCoordinator) HasCompleteOutput() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.messageSent {
		return false
	}
	for _, emitter := range c.emitters {
		if emitter == nil {
			continue
		}
		switch emitter.lifecycle {
		case answerEmitterUnknown:
			continue
		case answerEmitterSkipped:
			continue
		case answerEmitterCompleted:
			if !emitter.drained {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (c *answerOutputCoordinator) FullAnswer() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fullAnswer.String()
}

// MarkAnswerActive allows an answer node to start releasing template segments.
func (c *answerOutputCoordinator) MarkAnswerActive(nodeID string) {
	c.MarkAnswerActiveScoped(answerTopScope(), nodeID)
}

func (c *answerOutputCoordinator) MarkAnswerActiveScoped(scope answerOutputScope, nodeID string) {
	if c == nil {
		return
	}
	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	emitter := c.ensureEmitterLocked(scope, nodeID)
	if emitter != nil && (emitter.lifecycle == answerEmitterUnknown || emitter.lifecycle == answerEmitterEligible) {
		emitter.lifecycle = answerEmitterActive
	}
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
}

// MarkSelectedHandleReachable marks downstream answers on a chosen branch as eligible.
func (c *answerOutputCoordinator) MarkSelectedHandleReachable(nodeID string, status string, edgeSourceHandle string) {
	if c == nil || !answerStatusCanSelectDownstream(status) {
		return
	}
	if edgeSourceHandle == "" {
		edgeSourceHandle = "source"
	}

	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	c.markReachableFromHandlesLocked(nodeID, []string{edgeSourceHandle})
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
}

// MarkNodeSkipped releases placeholders that are known to never produce output.
func (c *answerOutputCoordinator) MarkNodeSkipped(nodeID string) {
	c.MarkNodeSkippedScoped(answerTopScope(), nodeID)
}

func (c *answerOutputCoordinator) MarkNodeSkippedScoped(scope answerOutputScope, nodeID string) {
	if c == nil {
		return
	}
	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	c.markNodeSkippedLocked(scope, nodeID)
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
}

func (c *answerOutputCoordinator) MarkScopedSourceAvailable(scope answerOutputScope, sourceNodeID string, outputs map[string]any) {
	if c == nil || sourceNodeID == "" || len(outputs) == 0 {
		return
	}
	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	c.markSourceNodeFinishedLocked(scope, sourceNodeID, string(workflow_shared.SUCCEEDED), outputs, nil)
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
}

// MarkNodeFinished finalizes source variables and answer lifecycle after node completion.
func (c *answerOutputCoordinator) MarkNodeFinished(nodeID string, nodeType string, status string, outputs map[string]any, err error) {
	c.MarkNodeFinishedScoped(answerTopScope(), nodeID, nodeType, status, outputs, err)
}

func (c *answerOutputCoordinator) MarkNodeFinishedScoped(scope answerOutputScope, nodeID string, nodeType string, status string, outputs map[string]any, err error) {
	if c == nil {
		return
	}
	if status == string(workflow_shared.PAUSED) {
		return
	}
	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	if status == string(workflow_shared.SKIPPED) {
		c.markNodeSkippedLocked(scope, nodeID)
	} else {
		c.markSourceNodeFinishedLocked(scope, nodeID, status, outputs, err)
	}
	if nodeType == "answer" {
		c.markAnswerNodeFinishedLocked(scope, nodeID, status, err)
	}
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
}

// HandleStreamChunk intercepts watched source chunks and either emits them immediately
// or switches the placeholder to final-only output when an earlier segment blocks it.
func (c *answerOutputCoordinator) HandleStreamChunk(nodeID string, streamEvent *workflow_shared.RunStreamChunkEvent) bool {
	if c == nil || streamEvent == nil || len(streamEvent.FromVariableSelector) < 2 {
		return false
	}
	if c.nodeTypes[nodeID] == "answer" && streamEvent.FromVariableSelector[1] == "text" {
		return true
	}
	scope := answerScopeFromStream(streamEvent.Scope)
	streamKey := scopedSelectorKey(scope, streamEvent.FromVariableSelector)

	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	variables := c.varsByStream[streamKey]
	if len(variables) == 0 {
		c.mu.Unlock()
		return false
	}
	for _, variable := range variables {
		if variable.finalOnly || variable.hasFinal || variable.sourceSkipped || variable.sourceFailed {
			continue
		}
		if c.canEmitVariableChunkImmediatelyLocked(variable) {
			variable.chunks = append(variable.chunks, streamEvent.ChunkContent)
		} else {
			variable.finalOnly = true
		}
	}
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
	return true
}

func (c *answerOutputCoordinator) RegisterReadyBatch(scope answerOutputScope, nodeIDs []string) {
	if c == nil || len(nodeIDs) == 0 {
		return
	}
	c.emitMu.Lock()
	defer c.emitMu.Unlock()

	c.mu.Lock()
	answerEmitters := make([]*answerEmitter, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if c.nodeTypes[nodeID] != "answer" {
			continue
		}
		emitter := c.ensureEmitterLocked(scope, nodeID)
		if emitter != nil {
			answerEmitters = append(answerEmitters, emitter)
		}
	}
	sortAnswerEmitters(answerEmitters)
	for i, emitter := range answerEmitters {
		if emitter.batchBlockers == nil {
			emitter.batchBlockers = make(map[string]bool)
		}
		for j := 0; j < i; j++ {
			if !answerEmitterSameScope(emitter, answerEmitters[j]) {
				continue
			}
			emitter.batchBlockers[answerEmitters[j].key] = true
		}
	}
	messages := c.drainLocked()
	c.mu.Unlock()

	c.emitMessages(messages)
}

// parseTemplateSegments splits an answer template into static and variable segments.
func (c *answerOutputCoordinator) parseTemplateSegments(scope answerOutputScope, template string) []answerTemplateSegment {
	parts := graph_entities.VariablePattern.Split(template, -1)
	matches := graph_entities.VariablePattern.FindAllStringSubmatch(template, -1)
	segments := make([]answerTemplateSegment, 0, len(parts)+len(matches))
	for i, part := range parts {
		if part != "" {
			segments = append(segments, answerTemplateSegment{kind: answerSegmentStatic, text: part})
		}
		if i >= len(matches) || len(matches[i]) < 2 {
			continue
		}
		selector := strings.Split(matches[i][1], ".")
		selectorKey := answerSelectorKey(selector)
		variable := c.addVariableState(scope, selector)
		segments = append(segments, answerTemplateSegment{
			kind:        answerSegmentVariable,
			selector:    selector,
			selectorKey: selectorKey,
			stateKey:    variable.stateKey,
		})
	}
	return segments
}

// addVariableState registers one placeholder occurrence and its source selector.
func (c *answerOutputCoordinator) addVariableState(scope answerOutputScope, selector []string) *answerVariableState {
	selectorKey := answerSelectorKey(selector)
	stateKey := fmt.Sprintf("%s|%s#%d", answerScopeKey(scope), selectorKey, len(c.variables))
	variable := &answerVariableState{
		stateKey:    stateKey,
		selector:    append([]string(nil), selector...),
		selectorKey: selectorKey,
		sourceNode:  selector[0],
		outputKey:   selector[1],
		scope:       scope,
	}
	c.variables[stateKey] = variable
	c.varsByStream[scopedSelectorKey(scope, selector)] = append(c.varsByStream[scopedSelectorKey(scope, selector)], variable)
	c.varsBySource[scopedSourceKey(scope, variable.sourceNode)] = append(c.varsBySource[scopedSourceKey(scope, variable.sourceNode)], variable)
	return variable
}

// markNodeSkippedLocked converts all selectors sourced from nodeID to skipped/finalized state.
func (c *answerOutputCoordinator) markNodeSkippedLocked(scope answerOutputScope, nodeID string) {
	for _, variable := range c.varsBySource[scopedSourceKey(scope, nodeID)] {
		variable.sourceSkipped = true
		variable.hasFinal = true
		variable.finalValue = ""
		variable.chunks = nil
	}
	if emitter := c.ensureEmitterLocked(scope, nodeID); emitter != nil && emitter.lifecycle != answerEmitterCompleted {
		emitter.lifecycle = answerEmitterSkipped
		emitter.drained = true
		emitter.batchBarrierPassed = true
		c.advanceIterationBarrierLocked(emitter)
	}
}

// markSourceNodeFinishedLocked captures the final rendered output for placeholders sourced by nodeID.
func (c *answerOutputCoordinator) markSourceNodeFinishedLocked(scope answerOutputScope, nodeID string, status string, outputs map[string]any, err error) {
	failed := err != nil || (status != "" && status != string(workflow_shared.SUCCEEDED))
	for _, variable := range c.varsBySource[scopedSourceKey(scope, nodeID)] {
		if failed {
			variable.sourceFailed = true
			variable.chunks = nil
			continue
		}
		if variable.sourceSkipped {
			continue
		}
		wasFailed := variable.sourceFailed
		variable.sourceFailed = false
		if wasFailed {
			variable.finalizedSegment = false
			c.recoverFailedEmitterForVariableLocked(variable)
		}
		variable.hasFinal = true
		variable.finalValue = renderAnswerVariableOutput(variable.selector, outputs)
	}
}

func (c *answerOutputCoordinator) recoverFailedEmitterForVariableLocked(variable *answerVariableState) {
	if c == nil || variable == nil {
		return
	}
	for _, emitter := range c.emitters {
		if emitter == nil || emitter.lifecycle != answerEmitterFailed {
			continue
		}
		if emitter.currentIndex >= len(emitter.segments) {
			continue
		}
		segment := emitter.segments[emitter.currentIndex]
		if segment.kind != answerSegmentVariable || segment.stateKey != variable.stateKey {
			continue
		}
		emitter.lifecycle = answerEmitterEligible
		emitter.drained = false
		emitter.batchBarrierPassed = false
	}
}

// markAnswerNodeFinishedLocked updates the emitter lifecycle after the answer node itself finishes.
func (c *answerOutputCoordinator) markAnswerNodeFinishedLocked(scope answerOutputScope, nodeID string, status string, err error) {
	emitter := c.ensureEmitterLocked(scope, nodeID)
	if emitter == nil {
		return
	}
	if emitter.lifecycle == answerEmitterUnknown {
		emitter.lifecycle = answerEmitterActive
	}
	if err != nil || (status != "" && status != string(workflow_shared.SUCCEEDED)) {
		emitter.lifecycle = answerEmitterFailed
		emitter.batchBarrierPassed = true
		c.advanceIterationBarrierLocked(emitter)
		return
	}
	emitter.lifecycle = answerEmitterCompleted
	emitter.batchBarrierPassed = true
	if emitter.currentIndex >= len(emitter.segments) {
		emitter.drained = true
	}
	c.advanceIterationBarrierLocked(emitter)
}

// drainLocked releases as many answer chunks as the current coordinator state allows.
func (c *answerOutputCoordinator) drainLocked() []answerMessageChunk {
	var messages []answerMessageChunk
	for {
		emitter := c.nextEmitterLocked()
		if emitter == nil {
			return messages
		}
		if !c.canEmitterProgressLocked(emitter) {
			emitter.batchBarrierPassed = true
			c.advanceIterationBarrierLocked(emitter)
			continue
		}
		emitted, progressed := c.drainEmitterLocked(emitter)
		emitter.batchBarrierPassed = true
		c.advanceIterationBarrierLocked(emitter)
		messages = append(messages, emitted...)
		if !progressed {
			continue
		}
	}
}

// nextEmitterLocked picks the next answer emitter that is allowed to make progress.
func (c *answerOutputCoordinator) nextEmitterLocked() *answerEmitter {
	var selected *answerEmitter
	for _, emitter := range c.emitters {
		if !c.canEmitterOutputLocked(emitter) {
			continue
		}
		if !c.canEmitterProgressLocked(emitter) && emitter.batchBarrierPassed {
			continue
		}
		if selected == nil || compareAnswerEmitters(emitter, selected) < 0 {
			selected = emitter
		}
	}
	return selected
}

// canEmitterOutputLocked checks lifecycle and topological predecessor constraints.
func (c *answerOutputCoordinator) canEmitterOutputLocked(emitter *answerEmitter) bool {
	if emitter == nil || emitter.drained {
		return false
	}
	if emitter.lifecycle != answerEmitterEligible && emitter.lifecycle != answerEmitterActive && emitter.lifecycle != answerEmitterCompleted {
		return false
	}
	if emitter.lifecycle == answerEmitterEligible && !answerEmitterHasVariableSegment(emitter) {
		return false
	}
	if emitter.scope.kind == answerScopeIteration {
		nextIndex := c.iterationNext[emitter.scope.parentNodeID]
		if emitter.scope.index > nextIndex {
			return false
		}
	}
	for parentID := range c.answerParents[emitter.nodeID] {
		parent := c.emitters[answerEmitterKey(emitter.scope, parentID)]
		if parent == nil {
			continue
		}
		if parent.lifecycle == answerEmitterFailed {
			return false
		}
		if parent.lifecycle != answerEmitterCompleted && parent.lifecycle != answerEmitterSkipped {
			return false
		}
	}
	for blockerKey := range emitter.batchBlockers {
		blocker := c.emitters[blockerKey]
		if blocker != nil && !blocker.batchBarrierPassed {
			return false
		}
	}
	return true
}

func answerEmitterHasVariableSegment(emitter *answerEmitter) bool {
	if emitter == nil {
		return false
	}
	for _, segment := range emitter.segments {
		if segment.kind == answerSegmentVariable {
			return true
		}
	}
	return false
}

func answerStatusCanSelectDownstream(status string) bool {
	switch status {
	case "", string(workflow_shared.SUCCEEDED), string(workflow_shared.EXCEPTION):
		return true
	default:
		return false
	}
}

func (c *answerOutputCoordinator) markReachableFromHandlesLocked(nodeID string, handles []string) {
	if c == nil || nodeID == "" {
		return
	}
	queue := make([]string, 0)
	for _, handle := range handles {
		for _, target := range c.edgeMap[nodeID][handle] {
			queue = append(queue, target)
		}
	}
	if len(queue) == 0 && !c.reachable[nodeID] {
		queue = append(queue, nodeID)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == "" || c.reachable[current] {
			continue
		}
		c.reachable[current] = true
		if c.nodeTypes[current] == "answer" {
			c.markAnswerEligibleLocked(answerTopScope(), current)
		}
		for _, target := range c.edgeMap[current]["source"] {
			queue = append(queue, target)
		}
	}
}

func (c *answerOutputCoordinator) markAnswerEligibleLocked(scope answerOutputScope, nodeID string) {
	emitter := c.ensureEmitterLocked(scope, nodeID)
	if emitter == nil || emitter.lifecycle != answerEmitterUnknown {
		return
	}
	emitter.lifecycle = answerEmitterEligible
}

func (c *answerOutputCoordinator) advanceIterationBarrierLocked(emitter *answerEmitter) {
	if emitter == nil || emitter.scope.kind != answerScopeIteration || !emitter.batchBarrierPassed {
		return
	}
	if emitter.lifecycle != answerEmitterSkipped && emitter.lifecycle != answerEmitterFailed && !emitter.drained {
		return
	}
	parentID := emitter.scope.parentNodeID
	for {
		nextIndex := c.iterationNext[parentID]
		if !c.iterationIndexBarrierPassedLocked(parentID, nextIndex) {
			return
		}
		c.iterationNext[parentID] = nextIndex + 1
	}
}

func (c *answerOutputCoordinator) iterationIndexBarrierPassedLocked(parentID string, index int) bool {
	found := false
	for _, emitter := range c.emitters {
		if emitter.scope.kind != answerScopeIteration || emitter.scope.parentNodeID != parentID || emitter.scope.index != index {
			continue
		}
		found = true
		if !emitter.batchBarrierPassed {
			return false
		}
	}
	return found
}

// canEmitterProgressLocked checks whether the current segment can emit something or finalize.
func (c *answerOutputCoordinator) canEmitterProgressLocked(emitter *answerEmitter) bool {
	if emitter == nil {
		return false
	}
	if emitter.currentIndex >= len(emitter.segments) {
		return emitter.lifecycle == answerEmitterCompleted
	}
	segment := emitter.segments[emitter.currentIndex]
	switch segment.kind {
	case answerSegmentStatic:
		return true
	case answerSegmentVariable:
		variable := c.variables[segment.stateKey]
		return c.canVariableProgressLocked(variable)
	default:
		return true
	}
}

// canVariableProgressLocked reports whether one placeholder can advance the answer stream.
func (c *answerOutputCoordinator) canVariableProgressLocked(variable *answerVariableState) bool {
	if variable == nil {
		return true
	}
	if variable.sourceFailed || variable.sourceSkipped {
		return true
	}
	if len(variable.chunks) > 0 && !variable.finalOnly {
		return true
	}
	return variable.hasFinal
}

// canEmitVariableChunkImmediatelyLocked allows direct emission only when this variable is next in order.
func (c *answerOutputCoordinator) canEmitVariableChunkImmediatelyLocked(variable *answerVariableState) bool {
	if variable == nil {
		return false
	}
	var emitter *answerEmitter
	for _, candidate := range c.emitters {
		if !c.canEmitterOutputLocked(candidate) {
			continue
		}
		if candidate.currentIndex >= len(candidate.segments) {
			continue
		}
		if emitter == nil || compareAnswerEmitters(candidate, emitter) < 0 {
			emitter = candidate
		}
	}
	if emitter == nil || emitter.currentIndex >= len(emitter.segments) {
		return false
	}
	return c.emitterCanReachVariableChunkLocked(emitter, variable.stateKey)
}

func (c *answerOutputCoordinator) emitterCanReachVariableChunkLocked(emitter *answerEmitter, stateKey string) bool {
	for index := emitter.currentIndex; index < len(emitter.segments); index++ {
		segment := emitter.segments[index]
		switch segment.kind {
		case answerSegmentStatic:
			continue
		case answerSegmentVariable:
			return segment.stateKey == stateKey
		default:
			continue
		}
	}
	return false
}

// drainEmitterLocked advances one answer emitter until it blocks, finishes, or fails.
func (c *answerOutputCoordinator) drainEmitterLocked(emitter *answerEmitter) ([]answerMessageChunk, bool) {
	var messages []answerMessageChunk
	for emitter.currentIndex < len(emitter.segments) {
		segment := emitter.segments[emitter.currentIndex]
		switch segment.kind {
		case answerSegmentStatic:
			if segment.text != "" {
				messages = append(messages, c.recordMessageLocked(emitter.nodeID, segment.text))
			}
			emitter.currentIndex++
		case answerSegmentVariable:
			variable := c.variables[segment.stateKey]
			emitted, done, failed := c.releaseVariableLocked(emitter.nodeID, variable)
			messages = append(messages, emitted...)
			if failed {
				emitter.lifecycle = answerEmitterFailed
				return messages, len(messages) > 0
			}
			if !done {
				return messages, len(messages) > 0
			}
			emitter.currentIndex++
		default:
			emitter.currentIndex++
		}
	}
	if emitter.lifecycle == answerEmitterCompleted {
		emitter.drained = true
		return messages, true
	}
	return messages, len(messages) > 0
}

// releaseVariableLocked emits buffered chunks, final replay chunks, or final suffixes for one placeholder.
func (c *answerOutputCoordinator) releaseVariableLocked(nodeID string, variable *answerVariableState) ([]answerMessageChunk, bool, bool) {
	if variable == nil {
		return nil, true, false
	}
	if variable.finalizedSegment {
		return nil, true, false
	}
	if variable.sourceFailed {
		return nil, false, true
	}
	if variable.sourceSkipped {
		variable.finalizedSegment = true
		variable.chunks = nil
		return nil, true, false
	}

	var messages []answerMessageChunk
	if len(variable.chunks) > 0 && !variable.finalOnly {
		for _, chunk := range variable.chunks {
			if chunk == "" {
				continue
			}
			variable.releasedText += chunk
			messages = append(messages, c.recordMessageLocked(nodeID, chunk))
		}
		variable.chunks = nil
		if !variable.hasFinal {
			return messages, false, false
		}
	}

	if variable.finalOnly {
		if !variable.hasFinal {
			return nil, false, false
		}
		messages = append(messages, c.emitFinalOnlyLocked(nodeID, variable)...)
		variable.releasedText = variable.finalValue
		variable.finalizedSegment = true
		return messages, true, false
	}

	if !variable.hasFinal {
		return messages, false, false
	}

	switch {
	case variable.releasedText == "":
		if variable.finalValue != "" {
			messages = append(messages, c.recordMessageLocked(nodeID, variable.finalValue))
		}
	case strings.HasPrefix(variable.finalValue, variable.releasedText):
		suffix := strings.TrimPrefix(variable.finalValue, variable.releasedText)
		for _, chunk := range answerChunkText(suffix, c.chunkSize) {
			messages = append(messages, c.recordMessageLocked(nodeID, chunk))
		}
	default:
		logger.Warn("answer ordered output final value does not match streamed prefix",
			"selector", variable.selectorKey,
			"streamed_length", len(variable.releasedText),
			"final_length", len(variable.finalValue),
		)
	}
	variable.releasedText = variable.finalValue
	variable.finalizedSegment = true
	return messages, true, false
}

// recordMessageLocked appends released text to the final answer snapshot and marks that output was sent.
func (c *answerOutputCoordinator) recordMessageLocked(nodeID string, text string) answerMessageChunk {
	c.fullAnswer.WriteString(text)
	c.messageSent = true
	return answerMessageChunk{nodeID: nodeID, text: text}
}

// emitMessages sends ordered answer chunks as conversation `message` events.
func (c *answerOutputCoordinator) emitMessages(messages []answerMessageChunk) {
	emitted := false
	for _, message := range messages {
		if message.text == "" {
			continue
		}
		emitted = true
		c.resultChan <- &WorkflowStreamEvent{
			EventType: "message",
			Data: map[string]interface{}{
				"id":              c.workflowRunID,
				"message_id":      c.workflowRunID,
				"conversation_id": c.conversationID,
				"node_id":         message.nodeID,
				"answer":          message.text,
				"created_at":      time.Now().Unix(),
			},
		}
	}
	if emitted {
		c.resultChan <- &WorkflowStreamEvent{
			EventType: workflowEventAnswerSnapshotReady,
			Data: map[string]interface{}{
				"answer": c.FullAnswer(),
			},
		}
	}
}

// workflowGraphNodeTypes extracts node types for quick source-node lookups.
func workflowGraphNodeTypes(nodeMap map[string]map[string]interface{}) map[string]string {
	nodeTypes := make(map[string]string, len(nodeMap))
	for nodeID, node := range nodeMap {
		data, _ := node["data"].(map[string]interface{})
		if data == nil {
			continue
		}
		nodeType, _ := data["type"].(string)
		nodeTypes[nodeID] = nodeType
	}
	return nodeTypes
}

func workflowGraphNodeIsTopLevelAnswer(node map[string]interface{}) bool {
	if node == nil {
		return false
	}
	if parentID, _ := node["parentId"].(string); strings.TrimSpace(parentID) != "" {
		return false
	}
	data, _ := node["data"].(map[string]interface{})
	if data == nil {
		return true
	}
	if iterationID, _ := data["iteration_id"].(string); strings.TrimSpace(iterationID) != "" {
		return false
	}
	if loopID, _ := data["loop_id"].(string); strings.TrimSpace(loopID) != "" {
		return false
	}
	return true
}

// workflowGraphNodeOrder records the original graph.nodes array order.
func workflowGraphNodeOrder(graphData map[string]any) map[string]int {
	nodeOrder := make(map[string]int)
	nodes, _ := graphData["nodes"].([]interface{})
	for i, nodeInterface := range nodes {
		node, _ := nodeInterface.(map[string]interface{})
		if node == nil {
			continue
		}
		nodeID, _ := node["id"].(string)
		if nodeID == "" {
			continue
		}
		nodeOrder[nodeID] = i
	}
	return nodeOrder
}

// workflowGraphOrderedNodeIDs returns node IDs sorted by their graph.nodes order.
func workflowGraphOrderedNodeIDs(nodeOrder map[string]int) []string {
	nodeIDs := make([]string, 0, len(nodeOrder))
	for nodeID := range nodeOrder {
		nodeIDs = append(nodeIDs, nodeID)
	}
	for i := 0; i < len(nodeIDs); i++ {
		for j := i + 1; j < len(nodeIDs); j++ {
			if nodeOrder[nodeIDs[j]] < nodeOrder[nodeIDs[i]] {
				nodeIDs[i], nodeIDs[j] = nodeIDs[j], nodeIDs[i]
			}
		}
	}
	return nodeIDs
}

// buildAnswerPredecessorMap computes answer-to-answer topological dependencies through the graph.
func buildAnswerPredecessorMap(emitters map[string]*answerEmitter, edgeMap map[string]map[string][]string) map[string]map[string]bool {
	predecessors := make(map[string]map[string]bool, len(emitters))
	for answerID := range emitters {
		predecessors[answerID] = make(map[string]bool)
	}
	for answerID := range emitters {
		visited := make(map[string]bool)
		queue := []string{answerID}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if visited[current] {
				continue
			}
			visited[current] = true
			for _, targets := range edgeMap[current] {
				for _, target := range targets {
					if target == answerID {
						continue
					}
					if _, isAnswer := emitters[target]; isAnswer {
						predecessors[target][answerID] = true
					}
					queue = append(queue, target)
				}
			}
		}
	}
	return predecessors
}

// streamSelectorKey normalizes a selector to its streaming source key.
func streamSelectorKey(selector []string) string {
	if len(selector) < 2 {
		return strings.Join(selector, "|")
	}
	return selector[0] + "|" + selector[1]
}

// answerSelectorKey preserves the full selector path for one placeholder instance.
func answerSelectorKey(selector []string) string {
	return strings.Join(selector, "|")
}

// renderAnswerVariableOutput renders the final text form for one placeholder from node outputs.
func renderAnswerVariableOutput(selector []string, outputs map[string]any) string {
	if len(selector) < 2 || outputs == nil {
		return ""
	}
	baseValue, exists := outputs[selector[1]]
	if !exists {
		return ""
	}
	pool := graph_entities.NewVariablePool()
	pool.Add(selector[:2], baseValue)
	variable := pool.GetWithPath(selector)
	if variable == nil {
		return ""
	}
	return answerSegmentToText(variable)
}

// answerSegmentToText converts a resolved variable segment into answer text.
func answerSegmentToText(segment graph_entities.Segment) string {
	if segment == nil {
		return ""
	}
	switch segment.GetType() {
	case workflow_shared.SegmentTypeFile, workflow_shared.SegmentTypeArrayFile:
		return segment.Markdown()
	}
	obj := segment.ToObject()
	if str, ok := obj.(string); ok {
		return str
	}
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("%v", obj)
	}
	return string(jsonBytes)
}

// answerChunkText splits final-only replay text into stable message-sized chunks.
func answerChunkText(text string, chunkSize int) []string {
	if text == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 20
	}
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}
	chunks := make([]string, 0, (len(runes)/chunkSize)+1)
	for start := 0; start < len(runes); {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			for offset := 0; offset < 10 && end-offset > start; offset++ {
				char := runes[end-offset]
				if char == ' ' || char == '\n' || char == ',' || char == '.' || char == '!' || char == '?' {
					end = end - offset + 1
					break
				}
			}
		}
		chunks = append(chunks, string(runes[start:end]))
		start = end
	}
	return chunks
}

package end

import (
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
)

// StreamProcessor base stream processor interface
type StreamProcessor interface {
	Process(generator <-chan GraphEngineEvent) <-chan GraphEngineEvent
	Reset()
}

// EndStreamProcessor end stream processor
type EndStreamProcessor struct {
	graph                               *entities.Graph
	variablePool                        *entities.VariablePool
	endStreamParam                      *EndStreamParam
	routePosition                       map[string]int
	currentStreamChunkGeneratingNodeIDs map[string][]string
	hasOutput                           bool
	outputNodeIDs                       map[string]bool
	restNodeIDs                         map[string]bool
}

// NewEndStreamProcessor creates new end stream processor
func NewEndStreamProcessor(graph *entities.Graph, variablePool *entities.VariablePool, endStreamParam *EndStreamParam) *EndStreamProcessor {
	processor := &EndStreamProcessor{
		graph:                               graph,
		variablePool:                        variablePool,
		endStreamParam:                      endStreamParam,
		routePosition:                       make(map[string]int),
		currentStreamChunkGeneratingNodeIDs: make(map[string][]string),
		hasOutput:                           false,
		outputNodeIDs:                       make(map[string]bool),
		restNodeIDs:                         make(map[string]bool),
	}

	// Initialize route position
	if processor.endStreamParam != nil {
		for endNodeID := range processor.endStreamParam.EndStreamVariableSelectorMapping {
			processor.routePosition[endNodeID] = 0
		}
	}

	return processor
}

// Process processes event stream
func (p *EndStreamProcessor) Process(generator <-chan GraphEngineEvent) <-chan GraphEngineEvent {
	output := make(chan GraphEngineEvent, 100) // Buffered channel

	go func() {
		defer close(output)

		for event := range generator {
			switch e := event.(type) {
			case *NodeRunStartedEvent:
				if e.RouteNodeState.NodeID == p.graph.GetRootNodeID() && len(p.restNodeIDs) == 0 {
					p.Reset()
				}
				output <- event

			case *NodeRunStreamChunkEvent:
				if e.InIterationID != nil || e.InLoopID != nil {
					if p.hasOutput && !p.outputNodeIDs[e.NodeID] {
						e.ChunkContent = "\n" + e.ChunkContent
					}
					p.outputNodeIDs[e.NodeID] = true
					p.hasOutput = true
					output <- event
					continue
				}

				var streamOutEndNodeIDs []string
				if nodeIDs, exists := p.currentStreamChunkGeneratingNodeIDs[e.RouteNodeState.NodeID]; exists {
					streamOutEndNodeIDs = nodeIDs
				} else {
					streamOutEndNodeIDs = p.getStreamOutEndNodeIDs(e)
					p.currentStreamChunkGeneratingNodeIDs[e.RouteNodeState.NodeID] = streamOutEndNodeIDs
				}

				if len(streamOutEndNodeIDs) > 0 {
					if p.hasOutput && !p.outputNodeIDs[e.NodeID] {
						e.ChunkContent = "\n" + e.ChunkContent
					}
					p.outputNodeIDs[e.NodeID] = true
					p.hasOutput = true
					output <- event
				}

			case *NodeRunSucceededEvent:
				output <- event

				if nodeIDs, exists := p.currentStreamChunkGeneratingNodeIDs[e.RouteNodeState.NodeID]; exists {
					// Update route position after all stream events complete
					for _, endNodeID := range nodeIDs {
						p.routePosition[endNodeID]++
					}
					delete(p.currentStreamChunkGeneratingNodeIDs, e.RouteNodeState.NodeID)
				}

				// Remove unreachable nodes
				p.removeUnreachableNodes(e)

				// Generate stream output
				for additionalEvent := range p.generateStreamOutputsWhenNodeFinished(e) {
					output <- additionalEvent
				}

			default:
				output <- event
			}
		}
	}()

	return output
}

// Reset resets processor state
func (p *EndStreamProcessor) Reset() {
	p.routePosition = make(map[string]int)
	if p.endStreamParam != nil {
		for endNodeID := range p.endStreamParam.EndStreamVariableSelectorMapping {
			p.routePosition[endNodeID] = 0
		}
	}

	// Reset remaining node IDs
	p.restNodeIDs = make(map[string]bool)
	for _, nodeID := range p.graph.GetNodeIDs() {
		p.restNodeIDs[nodeID] = true
	}

	p.currentStreamChunkGeneratingNodeIDs = make(map[string][]string)
}

// generateStreamOutputsWhenNodeFinished generates stream outputs when node finishes
func (p *EndStreamProcessor) generateStreamOutputsWhenNodeFinished(event *NodeRunSucceededEvent) <-chan GraphEngineEvent {
	output := make(chan GraphEngineEvent, 10)

	go func() {
		defer close(output)

		if p.endStreamParam == nil {
			return
		}

		for endNodeID, position := range p.routePosition {
			// Check if all dependent end node IDs are not in remaining node IDs
			if event.RouteNodeState.NodeID != endNodeID {
				if p.restNodeIDs[endNodeID] {
					allDepsNotInRest := true
					if deps, exists := p.endStreamParam.EndDependencies[endNodeID]; exists {
						for _, depID := range deps {
							if p.restNodeIDs[depID] {
								allDepsNotInRest = false
								break
							}
						}
					}
					if !allDepsNotInRest {
						continue
					}
				} else {
					continue
				}
			}

			routePosition := p.routePosition[endNodeID]
			position = 0
			var valueSelectors [][]string

			if selectors, exists := p.endStreamParam.EndStreamVariableSelectorMapping[endNodeID]; exists {
				for _, currentValueSelectors := range selectors {
					if position >= routePosition {
						valueSelectors = append(valueSelectors, currentValueSelectors)
					}
					position++
				}
			}

			for _, valueSelector := range valueSelectors {
				if len(valueSelector) == 0 {
					continue
				}

				value := p.variablePool.GetWithPath(valueSelector)
				if value == nil {
					break
				}

				var text string
				if markdownVar, ok := value.(VariableMarkdown); ok {
					text = markdownVar.GetMarkdown()
				} else if segment, ok := value.(entities.Segment); ok {
					// Try to convert to text
					if obj := segment.ToObject(); obj != nil {
						if str, ok := obj.(string); ok {
							text = str
						}
					}
				}

				if text != "" {
					currentNodeID := valueSelector[0]
					if p.hasOutput && !p.outputNodeIDs[currentNodeID] {
						text = "\n" + text
					}

					p.outputNodeIDs[currentNodeID] = true
					p.hasOutput = true

					streamEvent := &NodeRunStreamChunkEvent{
						BaseNodeEvent: BaseNodeEvent{
							ID:                  event.ID,
							NodeID:              event.NodeID,
							NodeType:            event.NodeType,
							NodeData:            event.NodeData,
							RouteNodeState:      event.RouteNodeState,
							ParallelID:          event.ParallelID,
							ParallelStartNodeID: event.ParallelStartNodeID,
							NodeVersion:         event.NodeVersion,
						},
						ChunkContent:         text,
						FromVariableSelector: valueSelector,
					}

					output <- streamEvent
				}

				p.routePosition[endNodeID]++
			}
		}
	}()

	return output
}

// getStreamOutEndNodeIDs gets end node IDs for stream output
func (p *EndStreamProcessor) getStreamOutEndNodeIDs(event *NodeRunStreamChunkEvent) []string {
	if len(event.FromVariableSelector) == 0 {
		return []string{}
	}

	streamOutputValueSelector := event.FromVariableSelector
	if len(streamOutputValueSelector) == 0 {
		return []string{}
	}

	var streamOutEndNodeIDs []string

	if p.endStreamParam == nil {
		return streamOutEndNodeIDs
	}

	for endNodeID, routePosition := range p.routePosition {
		if !p.restNodeIDs[endNodeID] {
			continue
		}

		// Check if all dependent end node IDs are not in remaining node IDs
		if deps, exists := p.endStreamParam.EndDependencies[endNodeID]; exists {
			allDepsNotInRest := true
			for _, depID := range deps {
				if p.restNodeIDs[depID] {
					allDepsNotInRest = false
					break
				}
			}
			if !allDepsNotInRest {
				continue
			}
		}

		selectors, exists := p.endStreamParam.EndStreamVariableSelectorMapping[endNodeID]
		if !exists || routePosition >= len(selectors) {
			continue
		}

		position := 0
		var valueSelector []string
		for _, currentValueSelectors := range selectors {
			if position == routePosition {
				valueSelector = currentValueSelectors
				break
			}
			position++
		}

		if len(valueSelector) == 0 {
			continue
		}

		// Check if chunk node ID is before or equal to current node ID
		if !p.slicesEqual(valueSelector, streamOutputValueSelector) {
			continue
		}

		streamOutEndNodeIDs = append(streamOutEndNodeIDs, endNodeID)
	}

	return streamOutEndNodeIDs
}

// removeUnreachableNodes removes unreachable nodes
func (p *EndStreamProcessor) removeUnreachableNodes(event *NodeRunSucceededEvent) {
	// Remove current completed node from remaining nodes
	delete(p.restNodeIDs, event.RouteNodeState.NodeID)
}

// slicesEqual compares if two string slices are equal
func (p *EndStreamProcessor) slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

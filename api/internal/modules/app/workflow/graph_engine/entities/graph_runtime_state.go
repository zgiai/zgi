package entities

import (
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

type GraphRuntimeState struct {
	mu sync.RWMutex `json:"-"`

	VariablePool *VariablePool      `json:"variable_pool"`  // variable pool (required)
	StartAt      float64            `json:"start_at"`       // start time (required)
	TotalTokens  int                `json:"total_tokens"`   // total tokens
	LLMUsage     *LLMUsage          `json:"llm_usage"`      // llm usage info
	Outputs      map[string]any     `json:"outputs"`        // final output values
	NodeRunSteps int                `json:"node_run_steps"` // node run steps
	NodeRunState *RuntimeRouteState `json:"node_run_state"` // node run state

	// Legacy fields for backward compatibility
	ExecutionLog []any      `json:"execution_log,omitempty"`
	Status       string     `json:"status,omitempty"`
	StartTime    time.Time  `json:"start_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`
	Error        error      `json:"error,omitempty"`
}

// LLMUsage tracks token usage and pricing for an LLM call.
type LLMUsage struct {
	PromptTokens        int             `json:"prompt_tokens"`
	PromptUnitPrice     decimal.Decimal `json:"prompt_unit_price"`
	PromptPriceUnit     decimal.Decimal `json:"prompt_price_unit"`
	PromptPrice         decimal.Decimal `json:"prompt_price"`
	CompletionTokens    int             `json:"completion_tokens"`
	CompletionUnitPrice decimal.Decimal `json:"completion_unit_price"`
	CompletionPriceUnit decimal.Decimal `json:"completion_price_unit"`
	CompletionPrice     decimal.Decimal `json:"completion_price"`
	TotalTokens         int             `json:"total_tokens"`
	TotalPrice          decimal.Decimal `json:"total_price"`
	Currency            string          `json:"currency"`
	Latency             float64         `json:"latency"`
}

// EmptyUsage creates empty LLM usage.
func EmptyUsage() *LLMUsage {
	return &LLMUsage{
		PromptTokens:        0,
		PromptUnitPrice:     decimal.NewFromInt(0),
		PromptPriceUnit:     decimal.NewFromInt(0),
		PromptPrice:         decimal.NewFromInt(0),
		CompletionTokens:    0,
		CompletionUnitPrice: decimal.NewFromInt(0),
		CompletionPriceUnit: decimal.NewFromInt(0),
		CompletionPrice:     decimal.NewFromInt(0),
		TotalTokens:         0,
		TotalPrice:          decimal.NewFromInt(0),
		Currency:            "USD",
		Latency:             0.0,
	}
}

// RuntimeRouteState tracks graph routes and node states at runtime.
type RuntimeRouteState struct {
	Routes           map[string][]string        `json:"routes"`             // graph state routes (source_node_state_id: target_node_state_id)
	NodeStateMapping map[string]*RouteNodeState `json:"node_state_mapping"` // node state mapping (route_node_state_id: route_node_state)
}

// NewRuntimeRouteState creates new RuntimeRouteState
func NewRuntimeRouteState() *RuntimeRouteState {
	return &RuntimeRouteState{
		Routes:           make(map[string][]string),
		NodeStateMapping: make(map[string]*RouteNodeState),
	}
}

// CreateNodeState creates node state
func (rrs *RuntimeRouteState) CreateNodeState(nodeID string) *RouteNodeState {
	state := &RouteNodeState{
		ID:      generateUUID(), // You'll need to implement this
		NodeID:  nodeID,
		StartAt: time.Now(),
		Status:  shared.RouteNodeStatusRunning,
		Index:   len(rrs.NodeStateMapping) + 1,
	}
	rrs.NodeStateMapping[state.ID] = state
	return state
}

// AddRoute adds route to the graph state
func (rrs *RuntimeRouteState) AddRoute(sourceNodeStateID, targetNodeStateID string) {
	if rrs.Routes[sourceNodeStateID] == nil {
		rrs.Routes[sourceNodeStateID] = make([]string, 0)
	}
	rrs.Routes[sourceNodeStateID] = append(rrs.Routes[sourceNodeStateID], targetNodeStateID)
}

// GetRoutesWithNodeStateBySourceNodeStateID gets routes with node state by source node id
func (rrs *RuntimeRouteState) GetRoutesWithNodeStateBySourceNodeStateID(sourceNodeStateID string) []*RouteNodeState {
	var result []*RouteNodeState
	for _, targetStateID := range rrs.Routes[sourceNodeStateID] {
		if nodeState, exists := rrs.NodeStateMapping[targetStateID]; exists {
			result = append(result, nodeState)
		}
	}
	return result
}

// RouteNodeState tracks a node state within a runtime route.
type RouteNodeState struct {
	ID            string                 `json:"id"`                        // route node state id
	NodeID        string                 `json:"node_id"`                   // node id
	NodeRunResult *shared.NodeRunResult  `json:"node_run_result,omitempty"` // node run result
	Status        shared.RouteNodeStatus `json:"status"`                    // status
	StartAt       time.Time              `json:"start_at"`                  // start time
	FinishedAt    *time.Time             `json:"finished_at,omitempty"`     // finished time
	PausedAt      *time.Time             `json:"paused_at,omitempty"`       // paused time
	PausedBy      *string                `json:"paused_by,omitempty"`       // paused by
	FailedReason  *string                `json:"failed_reason,omitempty"`   // failed reason
	Index         int                    `json:"index"`                     // index
}

// SetFinished marks the node state as finished.
func (rns *RouteNodeState) SetFinished(runResult *shared.NodeRunResult) error {
	// Check if already finished
	if rns.Status == shared.RouteNodeStatusSuccess ||
		rns.Status == shared.RouteNodeStatusFailed ||
		rns.Status == shared.RouteNodeStatusException {
		return fmt.Errorf("route state %s already finished", rns.ID)
	}

	// Set status based on run result
	switch runResult.Status {
	case shared.SUCCEEDED:
		rns.Status = shared.RouteNodeStatusSuccess
	case shared.FAILED:
		rns.Status = shared.RouteNodeStatusFailed
		rns.FailedReason = &runResult.ErrMsg
	case shared.EXCEPTION:
		rns.Status = shared.RouteNodeStatusException
		rns.FailedReason = &runResult.ErrMsg
	case shared.PAUSED:
		now := time.Now()
		rns.Status = shared.RouteNodeStatusPaused
		rns.PausedAt = &now
		rns.NodeRunResult = runResult
		return nil
	default:
		return fmt.Errorf("invalid route status %v", runResult.Status)
	}

	rns.NodeRunResult = runResult
	now := time.Now()
	rns.FinishedAt = &now
	return nil
}

// generateUUID generates a UUID (you may want to use a proper UUID library)
func generateUUID() string {
	// This is a simple implementation, consider using github.com/google/uuid
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// NewGraphRuntimeState creates a GraphRuntimeState.
func NewGraphRuntimeState(variablePool *VariablePool) *GraphRuntimeState {
	return &GraphRuntimeState{
		VariablePool: variablePool,
		StartAt:      float64(time.Now().UnixNano()) / 1e9, // Convert to float64 seconds
		TotalTokens:  0,
		LLMUsage:     EmptyUsage(),
		Outputs:      make(map[string]any),
		NodeRunSteps: 0,
		NodeRunState: NewRuntimeRouteState(),
	}
}

// NewGraphRuntimeStateWithDefaults creates GraphRuntimeState with default empty variable pool
func NewGraphRuntimeStateWithDefaults() *GraphRuntimeState {
	return NewGraphRuntimeState(NewVariablePool())
}

func (s *GraphRuntimeState) AddTotalTokens(tokens int) {
	if s == nil || tokens <= 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalTokens += tokens
}

func (s *GraphRuntimeState) TotalTokenCount() int {
	if s == nil {
		return 0
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.TotalTokens
}

func (s *GraphRuntimeState) UpdateOutputs(update func(map[string]any) map[string]any) {
	if s == nil || update == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Outputs = update(s.Outputs)
}

func (s *GraphRuntimeState) OutputsSnapshot() map[string]any {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Outputs == nil {
		return nil
	}
	result := make(map[string]any, len(s.Outputs))
	for key, value := range s.Outputs {
		result[key] = value
	}
	return result
}

func (s *GraphRuntimeState) UpsertRouteNodeState(nodeID string, routeNodeState *RouteNodeState) {
	if s == nil || s.NodeRunState == nil || nodeID == "" || routeNodeState == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.NodeRunState.NodeStateMapping[nodeID] = routeNodeState
}

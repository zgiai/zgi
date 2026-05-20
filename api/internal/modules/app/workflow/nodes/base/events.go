package base

// NodeEvent is the interface for node events
type NodeEvent interface {
	GetEventType() string
}

// RunCompletedEvent represents a run completed event
type RunCompletedEvent struct {
	RunResult interface{} `json:"run_result"`
}

func (e *RunCompletedEvent) GetEventType() string {
	return "executed"
}

// NewNodeEvent creates a node event - supports two or three parameters for backward compatibility
func NewNodeEvent(eventType string, data interface{}, args ...interface{}) *NodeEvent {
	var event NodeEvent = &RunCompletedEvent{RunResult: data}
	return &event
}

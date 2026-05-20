package diagnosis

import (
	"time"
)

// ErrorType represents the categorized type of error
type ErrorType string

const (
	ErrorTypeTimeout          ErrorType = "TIMEOUT"
	ErrorTypeAuthError        ErrorType = "AUTH_ERROR"
	ErrorTypeModelError       ErrorType = "MODEL_ERROR"
	ErrorTypeVariableNull     ErrorType = "VARIABLE_NULL"
	ErrorTypeConditionNoMatch ErrorType = "CONDITION_NO_MATCH"
	ErrorTypeParseError       ErrorType = "PARSE_ERROR"
	ErrorTypeDBError          ErrorType = "DB_ERROR"
	ErrorTypeUnknown          ErrorType = "UNKNOWN_ERROR"
)

// UpstreamNodeContext holds the configuration and output for a specific upstream node
type UpstreamNodeContext struct {
	Config map[string]any // Original JSON/Map config
	Output map[string]any // Original JSON/Map output
}

// ErrorContext carries the context for diagnosing workflow errors
type ErrorContext struct {
	NodeLogID        string
	WorkflowID       string
	WorkflowRunID    string
	NodeID           string
	NodeType         string
	NodeName         string
	ErrorType        ErrorType
	ErrorMessage     string
	ErrorStack       string                         // Traceback
	NodeConfig       map[string]any                 // Current node configuration
	InputSnapshot    map[string]any                 // Sanitized inputs
	UpstreamContexts map[string]UpstreamNodeContext // Configs and outputs of all identified upstream dependencies
	Timestamp        time.Time
	UserID           string
	OrgID            string
	WorkspaceID      string
}

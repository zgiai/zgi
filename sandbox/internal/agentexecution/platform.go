package agentexecution

import (
	"context"
	"encoding/json"
)

const PlatformProtocolVersion = 1

// PlatformRequest crosses only the authenticated execution control plane.
// Its scope is derived from the owning Job, not supplied by a Skill.
type PlatformRequest struct {
	ID        string          `json:"id"`
	Action    string          `json:"action"`
	Arguments json.RawMessage `json:"arguments"`
}

// PlatformResponse contains model-facing data, never a raw upstream error.
// File bodies use the existing file data plane, not this bounded JSON protocol.
type PlatformResponse struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data,omitempty"`
	ErrorCode string          `json:"error_code,omitempty"`
}

// PlatformBackend is optional so legacy runtimes remain closed by default.
// Only the authenticated control plane can list or resolve these requests.
type PlatformBackend interface {
	PlatformRequests(context.Context, Allocation, string) ([]PlatformRequest, error)
	ResolvePlatformRequest(context.Context, Allocation, string, PlatformResponse) error
}

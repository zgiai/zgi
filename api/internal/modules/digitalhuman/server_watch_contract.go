package digitalhuman

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TaskTriggerServerEvent is the trigger of a task run that a server watch
// started (ADR 0031).
const TaskTriggerServerEvent = "server_event"

// ErrServerWatchConnection is a connection the caller does not own, or one
// the DigitalHuman has no active binding on.
var ErrServerWatchConnection = fmt.Errorf("%w: server watch connection is not usable", ErrInvalidRequest)

// ServerWatch is the public view of a task's server watch.
type ServerWatch struct {
	TaskID           uuid.UUID `json:"task_id"`
	ConnectionID     uuid.UUID `json:"connection_id"`
	Services         []string  `json:"services"`
	IntervalMinutes  int       `json:"interval_minutes"`
	FailureThreshold int       `json:"failure_threshold"`
	Enabled          bool      `json:"enabled"`
	// OwnedByViewer is false when someone else's connection is watched; only
	// that person may change the connection or services.
	OwnedByViewer   bool                         `json:"owned_by_viewer"`
	LastCheckedAt   *time.Time                   `json:"last_checked_at,omitempty"`
	LastOutcome     string                       `json:"last_outcome,omitempty"`
	LastTriggeredAt *time.Time                   `json:"last_triggered_at,omitempty"`
	State           map[string]ServerWatchTarget `json:"state"`
}

// ServerWatchTarget is the last observation of one service, or of the host
// under the key ServerWatchHostKey.
type ServerWatchTarget struct {
	Healthy  bool      `json:"healthy"`
	Observed string    `json:"observed,omitempty"`
	Failures int       `json:"failures"`
	Alerted  bool      `json:"alerted"`
	At       time.Time `json:"at"`
}

// ServerWatchHostKey keys the host's own reachability in the watch state.
const ServerWatchHostKey = "@host"

// PutServerWatchRequest creates or replaces a task's watch.
type PutServerWatchRequest struct {
	ConnectionID     uuid.UUID `json:"connection_id"`
	Services         []string  `json:"services"`
	IntervalMinutes  int       `json:"interval_minutes"`
	FailureThreshold int       `json:"failure_threshold"`
	Enabled          *bool     `json:"enabled"`
}

// TaskServerWatchService keeps task server watches. The handler has already
// checked that the caller may see the task, and may change it for writes.
type TaskServerWatchService interface {
	// GetServerWatch returns nil when the task has no watch.
	GetServerWatch(ctx context.Context, scope Scope, digitalHumanID, taskID uuid.UUID) (*ServerWatch, error)
	// PutServerWatch creates or replaces the watch; Enabled false turns it off.
	PutServerWatch(ctx context.Context, scope Scope, digitalHumanID, taskID uuid.UUID, req PutServerWatchRequest) (*ServerWatch, error)
}

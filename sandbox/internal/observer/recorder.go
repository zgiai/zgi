package observer

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

type Event struct {
	ID        string         `json:"id"`
	SandboxID string         `json:"sandbox_id,omitempty"`
	Type      string         `json:"type"`
	Message   string         `json:"message"`
	CreatedAt time.Time      `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Recorder struct {
	store Store
}

type Query struct {
	SandboxID      string
	Type           string
	TypePrefix     string
	OrganizationID string
	WorkspaceID    string
	AppID          string
	WorkflowRunID  string
	UserID         string
	RequestID      string
	Limit          int
	Before         time.Time
}

type Metrics struct {
	WindowEventLimit              int            `json:"window_event_limit"`
	EventCount                    int            `json:"event_count"`
	ExecutionSuccessCount         int            `json:"execution_success_count"`
	ExecutionFailureCount         int            `json:"execution_failure_count"`
	TimeoutCount                  int            `json:"timeout_count"`
	CancellationCount             int            `json:"cancellation_count"`
	OutputTruncationCount         int            `json:"output_truncation_count"`
	BackendErrorCount             int            `json:"backend_error_count"`
	ExecutionDurationCount        int            `json:"execution_duration_count"`
	ExecutionDurationTotalMS      int64          `json:"execution_duration_total_ms"`
	ExecutionDurationMaxMS        int64          `json:"execution_duration_max_ms"`
	ExecutionDurationAverageMS    float64        `json:"execution_duration_average_ms"`
	ExecutionCountByBackend       map[string]int `json:"execution_count_by_backend,omitempty"`
	ExecutionFailureCountByReason map[string]int `json:"execution_failure_count_by_reason,omitempty"`
}

type Store interface {
	AppendEvent(Event) error
	QueryEvents(Query) ([]Event, error)
}

func NewRecorder(limit int) *Recorder {
	return NewRecorderWithStore(newMemoryStore(limit))
}

func NewRecorderWithStore(store Store) *Recorder {
	return &Recorder{store: store}
}

type memoryStore struct {
	mu     sync.RWMutex
	events []Event
	limit  int
}

func newMemoryStore(limit int) Store {
	if limit <= 0 {
		limit = 500
	}
	return &memoryStore{
		events: make([]Event, 0, limit),
		limit:  limit,
	}
}

func (r *Recorder) Record(eventType string, sandboxID string, message string, metadata map[string]any) Event {
	event := Event{
		ID:        token(),
		SandboxID: sandboxID,
		Type:      eventType,
		Message:   message,
		CreatedAt: time.Now().UTC(),
		Metadata:  metadata,
	}

	_ = r.store.AppendEvent(event)
	return event
}

func (r *Recorder) List(sandboxID string) []Event {
	return r.Query(Query{SandboxID: sandboxID})
}

func (r *Recorder) Query(query Query) []Event {
	events, err := r.store.QueryEvents(query)
	if err != nil {
		return nil
	}
	return events
}

func (r *Recorder) Metrics(limit int) Metrics {
	if limit <= 0 {
		limit = 1000
	}
	events := r.Query(Query{Limit: limit})
	metrics := Metrics{
		WindowEventLimit:              limit,
		EventCount:                    len(events),
		ExecutionCountByBackend:       map[string]int{},
		ExecutionFailureCountByReason: map[string]int{},
	}

	for _, event := range events {
		switch event.Type {
		case "exec.code", "exec.command":
			metrics.ExecutionSuccessCount++
			if backend := metadataString(event.Metadata, "backend"); backend != "" {
				metrics.ExecutionCountByBackend[backend]++
			}
			if metadataBool(event.Metadata, "truncated") {
				metrics.OutputTruncationCount++
			}
			if exitCode, ok := metadataInt64(event.Metadata, "exit_code"); ok && exitCode == 124 {
				metrics.TimeoutCount++
			}
			if durationMS, ok := metadataInt64(event.Metadata, "duration_ms"); ok {
				metrics.ExecutionDurationCount++
				metrics.ExecutionDurationTotalMS += durationMS
				if durationMS > metrics.ExecutionDurationMaxMS {
					metrics.ExecutionDurationMaxMS = durationMS
				}
			}
		case "exec.code.failed", "exec.command.failed":
			metrics.ExecutionFailureCount++
			reason := metadataString(event.Metadata, "error_type")
			if reason == "" {
				reason = "unknown"
			}
			metrics.ExecutionFailureCountByReason[reason]++
			errorCode := metadataString(event.Metadata, "code")
			switch reason {
			case "execution_canceled":
				metrics.CancellationCount++
			case "execution_error":
				metrics.BackendErrorCount++
			}
			if reason == "execution_timeout" || errorCode == "execution_queue_timeout" {
				metrics.TimeoutCount++
			}
		}
	}
	if metrics.ExecutionDurationCount > 0 {
		metrics.ExecutionDurationAverageMS = float64(metrics.ExecutionDurationTotalMS) / float64(metrics.ExecutionDurationCount)
	}
	if len(metrics.ExecutionCountByBackend) == 0 {
		metrics.ExecutionCountByBackend = nil
	}
	if len(metrics.ExecutionFailureCountByReason) == 0 {
		metrics.ExecutionFailureCountByReason = nil
	}
	return metrics
}

func (s *memoryStore) AppendEvent(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append([]Event{event}, s.events...)
	if len(s.events) > s.limit {
		s.events = s.events[:s.limit]
	}
	return nil
}

func (s *memoryStore) QueryEvents(query Query) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		if query.SandboxID != "" && event.SandboxID != query.SandboxID {
			continue
		}
		if query.Type != "" && event.Type != query.Type {
			continue
		}
		if query.TypePrefix != "" && !strings.HasPrefix(event.Type, query.TypePrefix) {
			continue
		}
		if query.OrganizationID != "" && metadataString(event.Metadata, "organization_id") != query.OrganizationID {
			continue
		}
		if query.WorkspaceID != "" && metadataString(event.Metadata, "workspace_id") != query.WorkspaceID {
			continue
		}
		if query.AppID != "" && metadataString(event.Metadata, "app_id") != query.AppID {
			continue
		}
		if query.WorkflowRunID != "" && metadataString(event.Metadata, "workflow_run_id") != query.WorkflowRunID {
			continue
		}
		if query.UserID != "" && metadataString(event.Metadata, "user_id") != query.UserID {
			continue
		}
		if query.RequestID != "" && metadataString(event.Metadata, "request_id") != query.RequestID {
			continue
		}
		if !query.Before.IsZero() && !event.CreatedAt.Before(query.Before) {
			continue
		}
		events = append(events, event)
		if query.Limit > 0 && len(events) >= query.Limit {
			break
		}
	}
	return events, nil
}

func token() string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func metadataBool(metadata map[string]any, key string) bool {
	value, _ := metadata[key].(bool)
	return value
}

func metadataInt64(metadata map[string]any, key string) (int64, bool) {
	switch value := metadata[key].(type) {
	case int:
		return int64(value), true
	case int64:
		return value, true
	case float64:
		return int64(value), true
	default:
		return 0, false
	}
}

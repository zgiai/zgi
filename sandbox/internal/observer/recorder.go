package observer

import (
	"crypto/rand"
	"encoding/hex"
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
	SandboxID string
	Type      string
	Limit     int
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

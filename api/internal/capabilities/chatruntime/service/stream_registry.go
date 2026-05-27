package service

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

type streamRegistry struct {
	mu      sync.Mutex
	cancels map[uuid.UUID]context.CancelFunc
	stopped map[uuid.UUID]bool
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{
		cancels: make(map[uuid.UUID]context.CancelFunc),
		stopped: make(map[uuid.UUID]bool),
	}
}

func (r *streamRegistry) Begin(messageID uuid.UUID, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels[messageID] = cancel
}

func (r *streamRegistry) Finish(messageID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancels, messageID)
	delete(r.stopped, messageID)
}

func (r *streamRegistry) Stop(messageID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped[messageID] = true
	if cancel := r.cancels[messageID]; cancel != nil {
		cancel()
	}
}

func (r *streamRegistry) IsStopped(messageID uuid.UUID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped[messageID]
}

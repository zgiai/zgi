package service

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

type streamRegistry struct {
	mu      sync.Mutex
	entries map[uuid.UUID]streamRegistryEntry
}

type streamRegistryEntry struct {
	ownerRunID uuid.UUID
	cancel     context.CancelFunc
	stopped    bool
}

func newStreamRegistry() *streamRegistry {
	return &streamRegistry{
		entries: make(map[uuid.UUID]streamRegistryEntry),
	}
}

func (r *streamRegistry) Begin(messageID, runID uuid.UUID, cancel context.CancelFunc) {
	r.mu.Lock()
	previous, replaced := r.entries[messageID]
	r.entries[messageID] = streamRegistryEntry{
		ownerRunID: runID,
		cancel:     cancel,
	}
	r.mu.Unlock()

	if replaced && previous.ownerRunID != runID && previous.cancel != nil {
		previous.cancel()
	}
}

func (r *streamRegistry) Finish(messageID, runID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[messageID]
	if !ok || entry.ownerRunID != runID {
		return
	}
	delete(r.entries, messageID)
}

func (r *streamRegistry) Stop(messageID, runID uuid.UUID) {
	r.mu.Lock()
	entry, ok := r.entries[messageID]
	if !ok || entry.ownerRunID != runID {
		r.mu.Unlock()
		return
	}
	entry.stopped = true
	r.entries[messageID] = entry
	r.mu.Unlock()

	if entry.cancel != nil {
		entry.cancel()
	}
}

func (r *streamRegistry) StopCurrent(messageID uuid.UUID) {
	r.mu.Lock()
	entry, ok := r.entries[messageID]
	if !ok {
		r.mu.Unlock()
		return
	}
	entry.stopped = true
	r.entries[messageID] = entry
	r.mu.Unlock()

	if entry.cancel != nil {
		entry.cancel()
	}
}

func (r *streamRegistry) IsStopped(messageID, runID uuid.UUID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[messageID]
	return ok && entry.ownerRunID == runID && entry.stopped
}

func (r *streamRegistry) CancelFunc(messageID, runID uuid.UUID) context.CancelFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[messageID]
	if !ok || entry.ownerRunID != runID {
		return nil
	}
	return entry.cancel
}

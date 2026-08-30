package contextmgr

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore keeps oversized tool results in memory for tests and embedded
// callers.
type MemoryStore struct {
	mu      sync.Mutex
	results map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		results: map[string]string{},
	}
}

func (s *MemoryStore) Put(ctx context.Context, agentRunID string, contentHash string, content string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil {
		return "", fmt.Errorf("memory context store is required")
	}
	runPart, err := safePathPart(agentRunID)
	if err != nil {
		return "", err
	}
	hashPart, err := safePathPart(contentHash)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.results[runPart+":"+hashPart] = content
	s.mu.Unlock()
	return "agent-context://tool-results/" + runPart + "/" + hashPart, nil
}

func (s *MemoryStore) Get(ctx context.Context, agentRunID string, contentHash string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil {
		return "", fmt.Errorf("memory context store is required")
	}
	runPart, err := safePathPart(agentRunID)
	if err != nil {
		return "", err
	}
	hashPart, err := safePathPart(contentHash)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	content, ok := s.results[runPart+":"+hashPart]
	s.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("context artifact not found")
	}
	return content, nil
}

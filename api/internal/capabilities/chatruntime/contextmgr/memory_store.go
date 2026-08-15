package contextmgr

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore keeps tests and embedded callers from writing runtime
// checkpoints into the repository's storage directory.
type MemoryStore struct {
	mu      sync.Mutex
	states  map[string]AgentContextState
	results map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		states:  map[string]AgentContextState{},
		results: map[string]string{},
	}
}

func (s *MemoryStore) Save(ctx context.Context, state AgentContextState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("memory context store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.AgentRunID] = cloneState(state)
	return nil
}

func (s *MemoryStore) Load(ctx context.Context, agentRunID string) (*AgentContextState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("memory context store is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[agentRunID]
	if !ok {
		return nil, nil
	}
	cloned := cloneState(state)
	return &cloned, nil
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

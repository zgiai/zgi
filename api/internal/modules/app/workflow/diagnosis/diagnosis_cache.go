package diagnosis

import (
	"context"
	"sync"
	"time"
)

// DiagnosisResult encapsulates the outcome of an error diagnosis
type DiagnosisResult struct {
	ResultText      string
	ModelUsed       string
	Tokens          int
	LatencyMs       int
	IsDiagnosed     bool
	NodeYAML        string
	UpstreamYAML    string
	InputSnapshot   string
	UpstreamOutputs string
}

// diagnosisCacheKey is used for identical error patterns
type diagnosisCacheKey struct {
	NodeType  string
	ErrorType string
}

// diagnosisCacheEntry contains the result and tracking details
type diagnosisCacheEntry struct {
	Result    DiagnosisResult
	HitCount  int
	ExpiresAt time.Time
}

// Cache prevents storming the LLM with the exact identical error from the same node type
type Cache struct {
	mu      sync.RWMutex
	entries map[diagnosisCacheKey]*diagnosisCacheEntry
	ttl     time.Duration
}

// NewCache creates a new Diagnosis Cache
func NewCache(ctx context.Context, ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[diagnosisCacheKey]*diagnosisCacheEntry),
		ttl:     ttl,
	}

	// Start simple cleanup coroutine with context cancellation support
	go func() {
		ticker := time.NewTicker(c.ttl)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.cleanup()
			}
		}
	}()

	return c
}

// Record checks if we should reuse a cached result and updates counters.
// A typical threshold is 10 identical errors within 5 minutes.
func (c *Cache) Record(nodeType string, errType ErrorType) (*DiagnosisResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := diagnosisCacheKey{NodeType: nodeType, ErrorType: string(errType)}
	now := time.Now()

	entry, exists := c.entries[key]
	if !exists || now.After(entry.ExpiresAt) {
		// Does not exist or expired, create new tracker
		c.entries[key] = &diagnosisCacheEntry{
			HitCount:  1,
			ExpiresAt: now.Add(c.ttl),
		}
		return nil, false
	}

	entry.HitCount++

	// If it hits more than 10 times, we reuse to prevent LLM quota drain and timeouts
	if entry.HitCount > 10 && entry.Result.IsDiagnosed {
		return &entry.Result, true
	}

	return nil, false
}

// SaveResult caches a successful LLM diagnosis outcome
func (c *Cache) SaveResult(nodeType string, errType ErrorType, res DiagnosisResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := diagnosisCacheKey{NodeType: nodeType, ErrorType: string(errType)}
	if entry, exists := c.entries[key]; exists {
		entry.Result = res
	}
}

// cleanup removes expired entries
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for k, v := range c.entries {
		if now.After(v.ExpiresAt) {
			delete(c.entries, k)
		}
	}
}

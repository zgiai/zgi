package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	GraphRuntimeStateInitializing = "initializing"
	GraphRuntimeStateReady        = "ready"
	GraphRuntimeStateDegraded     = "degraded"
)

const (
	GraphRuntimeReasonNotConfigured = "graph_runtime_not_configured"
	GraphRuntimeReasonUnavailable   = "graph_runtime_unavailable"
)

var ErrGraphRuntimeUnavailable = errors.New("knowledge graph runtime is unavailable")

type graphRuntimeChecker interface {
	VerifyConnectivity(ctx context.Context) error
}

type GraphRuntimeCapability struct {
	State      string    `json:"state"`
	Available  bool      `json:"available"`
	ReasonCode *string   `json:"reason_code"`
	Message    string    `json:"message"`
	CheckedAt  time.Time `json:"checked_at"`
}

type GraphRuntimeHealthService struct {
	checker graphRuntimeChecker
	mu      sync.RWMutex
	latest  GraphRuntimeCapability
}

func NewGraphRuntimeHealthService(checker graphRuntimeChecker) *GraphRuntimeHealthService {
	return &GraphRuntimeHealthService{
		checker: checker,
		latest: GraphRuntimeCapability{
			State:     GraphRuntimeStateInitializing,
			Available: false,
			Message:   "Knowledge graph services are initializing.",
			CheckedAt: time.Now().UTC(),
		},
	}
}

func (s *GraphRuntimeHealthService) Check(ctx context.Context) GraphRuntimeCapability {
	now := time.Now().UTC()
	capability := GraphRuntimeCapability{CheckedAt: now}
	if s == nil || s.checker == nil {
		reason := GraphRuntimeReasonNotConfigured
		capability.State = GraphRuntimeStateDegraded
		capability.ReasonCode = &reason
		capability.Message = "Knowledge graph runtime is not configured."
	} else if err := s.checker.VerifyConnectivity(ctx); err != nil {
		reason := GraphRuntimeReasonUnavailable
		capability.State = GraphRuntimeStateDegraded
		capability.ReasonCode = &reason
		capability.Message = "Knowledge graph runtime is unavailable."
	} else {
		capability.State = GraphRuntimeStateReady
		capability.Available = true
		capability.Message = "Knowledge graph services are ready."
	}
	if s != nil {
		s.mu.Lock()
		s.latest = capability
		s.mu.Unlock()
	}
	return capability
}

func (s *GraphRuntimeHealthService) Peek() GraphRuntimeCapability {
	if s == nil {
		return GraphRuntimeCapability{
			State:     GraphRuntimeStateDegraded,
			Available: false,
			Message:   "Knowledge graph runtime is unavailable.",
			CheckedAt: time.Now().UTC(),
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}

func (s *GraphRuntimeHealthService) Capability(ctx context.Context) GraphRuntimeCapability {
	if s == nil {
		return NewGraphRuntimeHealthService(nil).Check(ctx)
	}
	s.mu.RLock()
	latest := s.latest
	s.mu.RUnlock()
	if latest.State == GraphRuntimeStateInitializing {
		return s.Check(ctx)
	}
	return latest
}

func (s *GraphRuntimeHealthService) RequireReady(ctx context.Context) error {
	if capability := s.Check(ctx); !capability.Available {
		return ErrGraphRuntimeUnavailable
	}
	return nil
}

package service

import (
	"context"
	"errors"
	"testing"
)

type graphRuntimeCheckerStub struct {
	err error
}

func (s *graphRuntimeCheckerStub) VerifyConnectivity(context.Context) error { return s.err }

func TestGraphRuntimeHealthTransitionsAndOperationGate(t *testing.T) {
	checker := &graphRuntimeCheckerStub{err: errors.New("bolt://user:secret@neo4j:7687 failed")}
	service := NewGraphRuntimeHealthService(checker)
	if initial := service.Peek(); initial.State != GraphRuntimeStateInitializing || initial.Available {
		t.Fatalf("initial capability = %#v", initial)
	}
	degraded := service.Check(context.Background())
	if degraded.State != GraphRuntimeStateDegraded || degraded.Available {
		t.Fatalf("degraded capability = %#v", degraded)
	}
	if degraded.Message == checker.err.Error() {
		t.Fatal("health response leaked raw connectivity error")
	}
	if err := service.RequireReady(context.Background()); !errors.Is(err, ErrGraphRuntimeUnavailable) {
		t.Fatalf("operation gate error = %v", err)
	}

	checker.err = nil
	ready := service.Check(context.Background())
	if ready.State != GraphRuntimeStateReady || !ready.Available {
		t.Fatalf("ready capability = %#v", ready)
	}
	if err := service.RequireReady(context.Background()); err != nil {
		t.Fatalf("ready runtime rejected: %v", err)
	}
}

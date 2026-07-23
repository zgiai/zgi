package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestStreamRegistryOldRunFinishPreservesNewOwner(t *testing.T) {
	registry := newStreamRegistry()
	messageID := uuid.New()
	oldRunID := uuid.New()
	newRunID := uuid.New()
	oldCtx, oldCancel := context.WithCancel(context.Background())
	newCtx, newCancel := context.WithCancel(context.Background())
	t.Cleanup(oldCancel)
	t.Cleanup(newCancel)

	registry.Begin(messageID, oldRunID, oldCancel)
	registry.Begin(messageID, newRunID, newCancel)
	registry.Finish(messageID, oldRunID)

	if registry.CancelFunc(messageID, newRunID) == nil {
		t.Fatal("old run Finish removed the new run cancel function")
	}
	if registry.IsStopped(messageID, newRunID) {
		t.Fatal("new run inherited stopped state from the old owner")
	}
	select {
	case <-oldCtx.Done():
	default:
		t.Fatal("replacing the registry owner did not cancel the displaced local run")
	}
	select {
	case <-newCtx.Done():
		t.Fatal("old run Finish canceled the new owner")
	default:
	}
}

func TestStreamRegistryStopCancelsOnlyMatchingOwner(t *testing.T) {
	registry := newStreamRegistry()
	messageID := uuid.New()
	currentRunID := uuid.New()
	currentCtx, currentCancel := context.WithCancel(context.Background())
	t.Cleanup(currentCancel)
	registry.Begin(messageID, currentRunID, currentCancel)

	registry.Stop(messageID, uuid.New())
	select {
	case <-currentCtx.Done():
		t.Fatal("non-owner Stop canceled the current run")
	default:
	}
	if registry.IsStopped(messageID, currentRunID) {
		t.Fatal("non-owner Stop changed the current run stopped state")
	}

	registry.Stop(messageID, currentRunID)
	select {
	case <-currentCtx.Done():
	default:
		t.Fatal("matching owner Stop did not cancel the current run")
	}
	if !registry.IsStopped(messageID, currentRunID) {
		t.Fatal("matching owner Stop did not record stopped state")
	}
}

func TestStreamRegistryStopCurrentCancelsReplacementOwner(t *testing.T) {
	registry := newStreamRegistry()
	messageID := uuid.New()
	oldRunID := uuid.New()
	newRunID := uuid.New()
	_, oldCancel := context.WithCancel(context.Background())
	newCtx, newCancel := context.WithCancel(context.Background())
	t.Cleanup(oldCancel)
	t.Cleanup(newCancel)
	registry.Begin(messageID, oldRunID, oldCancel)
	registry.Begin(messageID, newRunID, newCancel)

	registry.StopCurrent(messageID)

	select {
	case <-newCtx.Done():
	default:
		t.Fatal("StopCurrent did not cancel the replacement owner")
	}
	if !registry.IsStopped(messageID, newRunID) {
		t.Fatal("StopCurrent did not record stopped state for the replacement owner")
	}
}

func TestStreamRegistryConcurrentOldFinishCannotDeleteReplacement(t *testing.T) {
	registry := newStreamRegistry()
	const iterations = 100
	for iteration := range iterations {
		messageID := uuid.New()
		oldRunID := uuid.New()
		newRunID := uuid.New()
		_, oldCancel := context.WithCancel(context.Background())
		newCtx, newCancel := context.WithCancel(context.Background())
		registry.Begin(messageID, oldRunID, oldCancel)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			registry.Finish(messageID, oldRunID)
		}()
		go func() {
			defer wg.Done()
			<-start
			registry.Begin(messageID, newRunID, newCancel)
		}()
		close(start)
		wg.Wait()

		if registry.CancelFunc(messageID, newRunID) == nil {
			t.Fatalf("iteration %d: concurrent old Finish removed replacement owner", iteration)
		}
		select {
		case <-newCtx.Done():
			t.Fatalf("iteration %d: replacement owner was canceled", iteration)
		default:
		}
		registry.Finish(messageID, newRunID)
		oldCancel()
		newCancel()
	}
}

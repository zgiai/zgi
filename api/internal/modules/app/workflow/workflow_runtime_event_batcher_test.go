package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRuntimeEventBatcherCoalescesOutstandingEvents(t *testing.T) {
	var mu sync.Mutex
	batchSizes := make([]int, 0, 2)
	seen := make([]string, 0, 32)
	batcher := newRuntimeEventBatcher(func(_ context.Context, records []workflowRunEventRecord) error {
		mu.Lock()
		defer mu.Unlock()
		batchSizes = append(batchSizes, len(records))
		for _, record := range records {
			seen = append(seen, record.eventType)
		}
		return nil
	})
	t.Cleanup(batcher.close)

	done := make([]<-chan error, 0, 32)
	for index := 0; index < 32; index++ {
		result, err := batcher.enqueue(t.Context(), []workflowRunEventRecord{{
			eventType: fmt.Sprintf("node_finished_%02d", index),
			data:      map[string]interface{}{"index": index},
		}}, false)
		if err != nil {
			t.Fatalf("enqueue event %d: %v", index, err)
		}
		done = append(done, result)
	}
	for index, result := range done {
		if err := <-result; err != nil {
			t.Fatalf("flush event %d: %v", index, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 32 {
		t.Fatalf("flushed events = %d, want 32", len(seen))
	}
	if len(batchSizes) > 2 {
		t.Fatalf("batch count = %d (%v), want at most 2", len(batchSizes), batchSizes)
	}
	for index, eventType := range seen {
		want := fmt.Sprintf("node_finished_%02d", index)
		if eventType != want {
			t.Fatalf("event %d = %q, want %q", index, eventType, want)
		}
	}
}

func TestRuntimeEventBatcherFlushesBarrierSeparately(t *testing.T) {
	flushed := make(chan []string, 2)
	batcher := newRuntimeEventBatcher(func(_ context.Context, records []workflowRunEventRecord) error {
		events := make([]string, len(records))
		for index, record := range records {
			events[index] = record.eventType
		}
		flushed <- events
		return nil
	})
	t.Cleanup(batcher.close)

	firstDone, err := batcher.enqueue(t.Context(), []workflowRunEventRecord{{eventType: "node_finished"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	barrierDone, err := batcher.enqueue(t.Context(), []workflowRunEventRecord{{eventType: "workflow_paused"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-barrierDone; err != nil {
		t.Fatal(err)
	}

	for index, want := range []string{"node_finished", "workflow_paused"} {
		select {
		case events := <-flushed:
			if len(events) != 1 || events[0] != want {
				t.Fatalf("flush %d = %v, want [%s]", index, events, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for flush %d", index)
		}
	}
}

func TestRuntimeEventBatcherRejectsAfterClose(t *testing.T) {
	batcher := newRuntimeEventBatcher(func(context.Context, []workflowRunEventRecord) error { return nil })
	batcher.close()
	if _, err := batcher.enqueue(t.Context(), []workflowRunEventRecord{{eventType: "node_finished"}}, false); err == nil {
		t.Fatal("enqueue after close succeeded")
	}
}

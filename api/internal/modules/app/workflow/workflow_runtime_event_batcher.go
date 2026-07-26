package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

const (
	// Parallel node callbacks arrive in the same scheduler turn. A short window
	// still coalesces those outstanding writes while avoiding a fixed 25ms tax
	// on every event in the much more common serial execution path.
	workflowRuntimeEventBatchWindow = 5 * time.Millisecond
	workflowRuntimeEventBatchCount  = 16
	workflowRuntimeEventBatchBytes  = 256 * 1024
	workflowRuntimeEventQueueSize   = 64
)

var errWorkflowRuntimeEventBatcherClosed = errors.New("workflow runtime event batcher is closed")

type runtimeEventBatchRequest struct {
	ctx     context.Context
	records []workflowRunEventRecord
	barrier bool
	done    chan error
}

// RuntimeEventBatcher coalesces concurrently arriving events for one
// execution. Callers wait for their batch result, preserving durable-before-
// visible semantics and providing bounded database backpressure.
type RuntimeEventBatcher struct {
	mu      sync.RWMutex
	queue   chan runtimeEventBatchRequest
	stopped chan struct{}
	closed  bool
	flush   func(context.Context, []workflowRunEventRecord) error
}

func newRuntimeEventBatcher(flush func(context.Context, []workflowRunEventRecord) error) *RuntimeEventBatcher {
	batcher := &RuntimeEventBatcher{
		queue: make(chan runtimeEventBatchRequest, workflowRuntimeEventQueueSize), stopped: make(chan struct{}), flush: flush,
	}
	go batcher.run()
	return batcher
}

func (b *RuntimeEventBatcher) enqueue(ctx context.Context, records []workflowRunEventRecord, barrier bool) (<-chan error, error) {
	if b == nil || len(records) == 0 {
		done := make(chan error, 1)
		done <- nil
		return done, nil
	}
	request := runtimeEventBatchRequest{ctx: ctx, records: records, barrier: barrier, done: make(chan error, 1)}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return nil, errWorkflowRuntimeEventBatcherClosed
	}
	select {
	case b.queue <- request:
		return request.done, nil
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
}

func (b *RuntimeEventBatcher) close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.queue)
	}
	b.mu.Unlock()
	<-b.stopped
}

func (b *RuntimeEventBatcher) run() {
	defer close(b.stopped)
	var carry *runtimeEventBatchRequest
	for {
		var first runtimeEventBatchRequest
		var ok bool
		if carry != nil {
			first = *carry
			carry = nil
			ok = true
		} else {
			first, ok = <-b.queue
		}
		if !ok {
			return
		}
		requests := []runtimeEventBatchRequest{first}
		eventCount, byteCount := runtimeEventRequestSize(first)
		queueClosed := false
		if !first.barrier {
			timer := time.NewTimer(workflowRuntimeEventBatchWindow)
		collect:
			for eventCount < workflowRuntimeEventBatchCount && byteCount < workflowRuntimeEventBatchBytes {
				select {
				case next, open := <-b.queue:
					if !open {
						queueClosed = true
						break collect
					}
					nextEvents, nextBytes := runtimeEventRequestSize(next)
					if next.barrier || eventCount+nextEvents > workflowRuntimeEventBatchCount || byteCount+nextBytes > workflowRuntimeEventBatchBytes {
						carry = &next
						break collect
					}
					requests = append(requests, next)
					eventCount += nextEvents
					byteCount += nextBytes
				case <-timer.C:
					break collect
				}
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}

		err := b.flushRequests(requests)
		for _, request := range requests {
			request.done <- err
		}
		if queueClosed && carry == nil {
			return
		}
	}
}

func (b *RuntimeEventBatcher) flushRequests(requests []runtimeEventBatchRequest) error {
	if len(requests) == 0 || b.flush == nil {
		return nil
	}
	ctx := requests[0].ctx
	records := make([]workflowRunEventRecord, 0, workflowRuntimeEventBatchCount)
	for _, request := range requests {
		records = append(records, request.records...)
	}
	for len(records) > 0 {
		count := runtimeEventChunkSize(records)
		if err := b.flush(ctx, records[:count]); err != nil {
			return err
		}
		records = records[count:]
	}
	return nil
}

func runtimeEventRequestSize(request runtimeEventBatchRequest) (int, int) {
	bytes := 0
	for _, record := range request.records {
		raw, _ := json.Marshal(record.data)
		bytes += len(raw)
	}
	return len(request.records), bytes
}

func runtimeEventChunkSize(records []workflowRunEventRecord) int {
	count := 0
	bytes := 0
	for count < len(records) && count < workflowRuntimeEventBatchCount {
		raw, _ := json.Marshal(records[count].data)
		if count > 0 && bytes+len(raw) > workflowRuntimeEventBatchBytes {
			break
		}
		bytes += len(raw)
		count++
	}
	if count == 0 {
		return 1
	}
	return count
}

package audit

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu      sync.Mutex
	records []Record
}

func (s *fakeStore) Insert(ctx context.Context, records []Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	return nil
}

func (s *fakeStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func waitForCount(t *testing.T, store *fakeStore, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.count() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("record count = %d, want at least %d", store.count(), want)
}

func TestAsyncRecorderFlushesFullBatch(t *testing.T) {
	store := &fakeStore{}
	recorder := NewAsyncRecorder(store)
	defer recorder.Close(context.Background())

	for i := 0; i < defaultBatchSize; i++ {
		recorder.Record(context.Background(), Record{SQLStatement: "select 1"})
	}

	waitForCount(t, store, defaultBatchSize)
}

func TestAsyncRecorderFlushesOnInterval(t *testing.T) {
	store := &fakeStore{}
	recorder := NewAsyncRecorder(store)
	defer recorder.Close(context.Background())

	recorder.Record(context.Background(), Record{SQLStatement: "select 1"})

	waitForCount(t, store, 1)
}

func TestAsyncRecorderFallbacksSynchronouslyWhenQueueIsFull(t *testing.T) {
	store := &fakeStore{}
	recorder := &AsyncRecorder{
		store: store,
		ch:    make(chan Record, 1),
	}

	recorder.Record(context.Background(), Record{SQLStatement: "select 1"})
	recorder.Record(context.Background(), Record{SQLStatement: "select 2"})

	if got := store.count(); got != 1 {
		t.Fatalf("fallback write count = %d, want 1", got)
	}
}

func TestAsyncRecorderCloseFlushesRemainingRecords(t *testing.T) {
	store := &fakeStore{}
	recorder := NewAsyncRecorder(store)

	recorder.Record(context.Background(), Record{SQLStatement: "select 1"})

	if err := recorder.Close(context.Background()); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if got := store.count(); got != 1 {
		t.Fatalf("record count after close = %d, want 1", got)
	}
}

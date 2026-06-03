package audit

import (
	"context"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const (
	defaultQueueSize  = 1000
	defaultBatchSize  = 100
	defaultFlushEvery = 100 * time.Millisecond
)

type AsyncRecorder struct {
	store Store

	ch   chan Record
	done chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

func NewAsyncRecorder(store Store) *AsyncRecorder {
	recorder := &AsyncRecorder{
		store: store,
		ch:    make(chan Record, defaultQueueSize),
		done:  make(chan struct{}),
	}
	recorder.wg.Add(1)
	go recorder.run()
	return recorder
}

func (r *AsyncRecorder) Record(ctx context.Context, record Record) {
	if r == nil || r.store == nil {
		return
	}

	select {
	case r.ch <- record:
	default:
		if err := r.store.Insert(ctx, []Record{record}); err != nil {
			logger.WarnContext(ctx, "failed to write sql audit record synchronously", zap.Error(err))
		}
	}
}

func (r *AsyncRecorder) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}

	r.once.Do(func() {
		close(r.done)
	})

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *AsyncRecorder) run() {
	defer r.wg.Done()

	ticker := time.NewTicker(defaultFlushEvery)
	defer ticker.Stop()

	batch := make([]Record, 0, defaultBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := r.store.Insert(context.Background(), batch); err != nil {
			logger.WarnContext(context.Background(), "failed to write sql audit batch", zap.Error(err), zap.Int("count", len(batch)))
		}
		batch = batch[:0]
	}

	for {
		select {
		case record := <-r.ch:
			batch = append(batch, record)
			if len(batch) >= defaultBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-r.done:
			for {
				select {
				case record := <-r.ch:
					batch = append(batch, record)
				default:
					flush()
					return
				}
			}
		}
	}
}

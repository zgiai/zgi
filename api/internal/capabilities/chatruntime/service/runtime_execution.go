package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type runtimeExecution struct {
	Context        context.Context
	PersistContext context.Context
	runID          uuid.UUID
	finish         func()
}

func (e *runtimeExecution) Finish() {
	if e != nil && e.finish != nil {
		e.finish()
	}
}

func (s *service) beginRuntimeExecution(ctx context.Context, messageID uuid.UUID) (*runtimeExecution, error) {
	basePersistCtx := context.WithoutCancel(ctx)
	runID := uuid.New()
	persistCtx := repository.WithRuntimeRunID(basePersistCtx, runID)
	runCtx, cancel := context.WithCancel(persistCtx)
	now := time.Now()
	if s.repos != nil && s.repos.RuntimeLease != nil {
		if err := s.repos.RuntimeLease.Begin(basePersistCtx, messageID, runID, now); err != nil {
			cancel()
			return nil, err
		}
	}

	s.streams.Begin(messageID, runID, cancel)
	done := make(chan struct{})
	var finishOnce sync.Once
	finish := func() {
		finishOnce.Do(func() {
			close(done)
			cancel()
			s.streams.Finish(messageID, runID)
			if s.repos != nil && s.repos.RuntimeLease != nil {
				if err := s.repos.RuntimeLease.Release(persistCtx, messageID, runID); err != nil {
					logger.WarnContext(persistCtx, "failed to release chat runtime lease",
						"message_id", messageID.String(),
						"runtime_run_id", runID.String(),
						err,
					)
				}
			}
		})
	}

	if s.repos != nil && s.repos.RuntimeLease != nil {
		go s.renewRuntimeLease(runCtx, done, messageID, runID, now)
	}

	return &runtimeExecution{
		Context:        runCtx,
		PersistContext: persistCtx,
		runID:          runID,
		finish:         finish,
	}, nil
}

func (s *service) renewRuntimeLease(ctx context.Context, done <-chan struct{}, messageID, runID uuid.UUID, lastSuccess time.Time) {
	s.renewRuntimeLeaseAtInterval(ctx, done, messageID, runID, lastSuccess, runtimeLeaseHeartbeat)
}

func (s *service) renewRuntimeLeaseAtInterval(ctx context.Context, done <-chan struct{}, messageID, runID uuid.UUID, lastSuccess time.Time, interval time.Duration) {
	if interval <= 0 {
		interval = runtimeLeaseHeartbeat
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			active, err := s.repos.RuntimeLease.Renew(renewCtx, messageID, runID, now)
			cancel()
			if err == nil && active {
				lastSuccess = now
				continue
			}
			if err == nil && !active {
				logger.InfoContext(ctx, "chat runtime lease ownership ended",
					"message_id", messageID.String(),
					"runtime_run_id", runID.String(),
				)
				s.streams.Stop(messageID, runID)
				return
			}
			logger.WarnContext(ctx, "failed to renew chat runtime lease",
				"message_id", messageID.String(),
				"runtime_run_id", runID.String(),
				"last_success_at", lastSuccess,
				err,
			)
			if now.Sub(lastSuccess) >= runtimeLeaseFailureTTL {
				cancel := s.streams.CancelFunc(messageID, runID)
				if cancel != nil {
					cancel()
				}
				return
			}
		}
	}
}

package app

import (
	"context"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/observer"
)

func (s *Server) StartBackgroundWorkers(ctx context.Context) {
	if !s.dependencyBuildWorkerConfigured() {
		return
	}
	go s.runDependencyBuildWorker(ctx)
}

func (s *Server) dependencyBuildWorkerConfigured() bool {
	return s != nil &&
		s.config.DependencyBuildWorkerEnabled &&
		strings.TrimSpace(s.config.DependencyBuildCommand) != "" &&
		strings.TrimSpace(s.config.DependencyRootFSDir) != "" &&
		s.store != nil
}

func (s *Server) runDependencyBuildWorker(ctx context.Context) {
	interval := time.Duration(s.config.DependencyBuildWorkerIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 2 * time.Second
	}

	s.observer.Record("dependency_build.worker.started", "", "dependency build worker started", observer.MetadataWithContext(ctx, map[string]any{
		"interval_seconds": int(interval.Seconds()),
		"worker_id":        s.config.WorkerID,
	}))
	defer s.observer.Record("dependency_build.worker.stopped", "", "dependency build worker stopped", observer.MetadataWithContext(ctx, map[string]any{
		"worker_id": s.config.WorkerID,
	}))

	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			processed := s.runOneQueuedDependencyBuild(ctx)
			next := interval
			if processed {
				next = 100 * time.Millisecond
			}
			timer.Reset(next)
		}
	}
}

func (s *Server) runOneQueuedDependencyBuild(ctx context.Context) bool {
	record, err := s.store.ClaimNextDependencyBuildRequest()
	if err != nil {
		s.observer.Record("dependency_build.worker.error", "", "dependency build worker claim failed", observer.MetadataWithContext(ctx, map[string]any{
			"error":     err.Error(),
			"worker_id": s.config.WorkerID,
		}))
		return false
	}
	if record == nil {
		return false
	}

	metadata := dependencyBuildEventMetadata(record)
	metadata["worker_id"] = s.config.WorkerID
	s.observer.Record("dependency_build.building", "", "dependency build started", observer.MetadataWithContext(ctx, metadata))

	updated, err := s.runDependencyBuildCommand(ctx, record)
	if err != nil {
		failed, updateErr := s.store.UpdateDependencyBuildRequestStatus(record.Fingerprint, "failed", "", 0, err.Error())
		if updateErr == nil {
			record = failed
		}
		metadata = dependencyBuildEventMetadata(record)
		metadata["worker_id"] = s.config.WorkerID
		metadata["error"] = err.Error()
		s.observer.Record("dependency_build.failed", "", "dependency build failed", observer.MetadataWithContext(ctx, metadata))
		return true
	}

	metadata = dependencyBuildEventMetadata(updated)
	metadata["worker_id"] = s.config.WorkerID
	s.observer.Record("dependency_build.ready", "", "dependency build completed", observer.MetadataWithContext(ctx, metadata))
	return true
}

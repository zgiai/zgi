package service

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"log"
	"strings"
	"time"
)

const (
	defaultVideoRuntimeTaskPollEvery   = 15 * time.Second
	defaultVideoRuntimeTaskPollStaleIn = 10 * time.Second
	defaultVideoRuntimeTaskPollBatch   = 50
	defaultVideoRuntimeTaskMaxAge      = 24 * time.Hour
	defaultVideoRuntimeSubmitTimeout   = 10 * time.Minute
)

type TaskPoller struct {
	svc *service
}

func NewTaskPoller(db *gorm.DB, llmClient interface{}) *TaskPoller {
	videoClient, _ := llmClient.(LLMVideoClient)
	if db == nil || videoClient == nil {
		return nil
	}
	return &TaskPoller{svc: &service{llmClient: videoClient, tasks: newTaskRepository(db), artifactSaver: defaultVideoArtifactSaver{}}}
}

func (p *TaskPoller) Start(ctx context.Context) {
	if p == nil || p.svc == nil {
		log.Println("[video runtime task poller] skipped: missing dependencies")
		return
	}
	ticker := time.NewTicker(defaultVideoRuntimeTaskPollEvery)
	defer ticker.Stop()

	log.Printf("[video runtime task poller] started, sweep_every=%s batch=%d", defaultVideoRuntimeTaskPollEvery, defaultVideoRuntimeTaskPollBatch)
	p.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("[video runtime task poller] stopped")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *TaskPoller) poll(ctx context.Context) {
	if err := p.svc.pollVideoRuntimeTasks(ctx); err != nil {
		log.Printf("[video runtime task poller] sweep failed: %v", err)
	}
}

func (s *service) pollVideoRuntimeTasks(ctx context.Context) error {
	if s == nil || s.tasks == nil || s.llmClient == nil {
		return nil
	}
	now := time.Now().UTC()
	staleBefore := now.Add(-defaultVideoRuntimeTaskPollStaleIn)
	submitExpiredBefore := now.Add(-defaultVideoRuntimeSubmitTimeout)
	tasks, err := s.tasks.listActiveForPolling(ctx, staleBefore, submitExpiredBefore, defaultVideoRuntimeTaskPollBatch)
	if err != nil {
		return fmt.Errorf("list video runtime tasks: %w", err)
	}
	for i := range tasks {
		if err := s.pollVideoRuntimeTask(ctx, &tasks[i]); err != nil {
			log.Printf("[video runtime task poller] task_id=%s failed: %v", tasks[i].TaskID, err)
		}
	}
	return nil
}

func (s *service) pollVideoRuntimeTask(ctx context.Context, record *videoTaskRecord) error {
	if record == nil {
		return nil
	}
	now := time.Now().UTC()
	if time.Since(record.CreatedAt) > defaultVideoRuntimeTaskMaxAge {
		record.Status = "failed"
		record.ErrorMessage = "video task exceeded max polling age"
		record.UpdatedAt = now
		record.CompletedAt = &now
		return s.tasks.save(ctx, record)
	}
	if strings.TrimSpace(record.UpstreamTaskID) == "" {
		if time.Since(record.CreatedAt) > defaultVideoRuntimeSubmitTimeout {
			record.Status = "failed"
			record.ErrorMessage = "video task was not submitted to upstream before timeout"
			record.UpdatedAt = now
			record.CompletedAt = &now
			return s.tasks.save(ctx, record)
		}
		return nil
	}

	scope := Scope{
		OrganizationID: record.OrganizationID,
		AccountID:      record.AccountID,
		WorkspaceID:    record.WorkspaceID,
	}
	if err := s.refreshTask(ctx, scope, record); err != nil {
		return s.markVideoRuntimeTaskPollError(ctx, record, err)
	}
	return nil
}

func (s *service) markVideoRuntimeTaskPollError(ctx context.Context, record *videoTaskRecord, err error) error {
	return s.markVideoRuntimeTaskFailedFromError(ctx, record, err)
}

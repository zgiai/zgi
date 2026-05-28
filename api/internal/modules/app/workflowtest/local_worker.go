package workflowtest

import (
	"context"
	"sync"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	localWorkerPollInterval = 3 * time.Second
	localWorkerClaimLimit   = 1
)

type LocalWorker struct {
	service *Service
	client  llmclient.LLMClient

	mu      sync.Mutex
	cancel  map[string]context.CancelFunc
	running map[string]struct{}
}

func NewLocalWorker(service *Service, client llmclient.LLMClient) *LocalWorker {
	return &LocalWorker{
		service: service,
		client:  client,
		cancel:  map[string]context.CancelFunc{},
		running: map[string]struct{}{},
	}
}

func (w *LocalWorker) Start(ctx context.Context) {
	if w == nil || w.service == nil || w.client == nil {
		return
	}
	ticker := time.NewTicker(localWorkerPollInterval)
	defer ticker.Stop()

	w.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			w.cancelAll()
			return
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

func (w *LocalWorker) Cancel(taskID string) {
	w.mu.Lock()
	cancel := w.cancel[taskID]
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (w *LocalWorker) sweep(ctx context.Context) {
	if _, err := w.service.RecoverStaleRunningGenerationTasks(ctx, time.Now().Add(-staleAsyncTaskThreshold)); err != nil {
		logger.Warn("workflow test local worker recover stale generation tasks failed", map[string]interface{}{"error": err.Error()})
	}
	if _, err := w.service.RecoverStaleRunningScenarioRecognitionTasks(ctx, time.Now().Add(-staleAsyncTaskThreshold)); err != nil {
		logger.Warn("workflow test local worker recover stale scenario recognition tasks failed", map[string]interface{}{"error": err.Error()})
	}

	generationTasks, err := w.service.repo.ListQueuedGenerationTasks(ctx, localWorkerClaimLimit)
	if err != nil {
		logger.Warn("workflow test local worker list generation tasks failed", map[string]interface{}{"error": err.Error()})
	} else {
		for _, task := range generationTasks {
			w.runOnce(ctx, task.ID, func(taskCtx context.Context) error {
				return w.service.RunGenerationTask(taskCtx, task.ID, w.client)
			})
		}
	}

	scenarioTasks, err := w.service.repo.ListQueuedScenarioRecognitionTasks(ctx, localWorkerClaimLimit)
	if err != nil {
		logger.Warn("workflow test local worker list scenario recognition tasks failed", map[string]interface{}{"error": err.Error()})
	} else {
		for _, task := range scenarioTasks {
			w.runOnce(ctx, task.ID, func(taskCtx context.Context) error {
				taskRecord, err := w.service.repo.GetScenarioRecognitionTaskByID(taskCtx, task.ID)
				if err != nil {
					return err
				}
				recognizer := &LLMScenarioRecognizer{
					Client:      w.client,
					WorkspaceID: taskRecord.WorkspaceID,
					AccountID:   taskRecord.AccountID,
					AgentID:     taskRecord.AgentID,
				}
				return w.service.RunScenarioRecognitionTask(taskCtx, task.ID, recognizer)
			})
		}
	}
}

func (w *LocalWorker) runOnce(parent context.Context, taskID string, run func(context.Context) error) {
	w.mu.Lock()
	if _, ok := w.running[taskID]; ok {
		w.mu.Unlock()
		return
	}
	w.running[taskID] = struct{}{}
	taskCtx, cancel := context.WithCancel(parent)
	w.cancel[taskID] = cancel
	w.mu.Unlock()

	go func() {
		defer func() {
			cancel()
			w.mu.Lock()
			delete(w.running, taskID)
			delete(w.cancel, taskID)
			w.mu.Unlock()
		}()
		if err := run(taskCtx); err != nil {
			logger.Warn("workflow test local worker task failed", map[string]interface{}{"task_id": taskID, "error": err.Error()})
		}
	}()
}

func (w *LocalWorker) cancelAll() {
	w.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(w.cancel))
	for _, cancel := range w.cancel {
		cancels = append(cancels, cancel)
	}
	w.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

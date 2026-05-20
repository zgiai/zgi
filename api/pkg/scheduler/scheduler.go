package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// Scheduler is a generic scheduled task scheduler
type Scheduler struct {
	scheduler *asynq.Scheduler
	server    *asynq.Server
	mux       *asynq.ServeMux
	config    *config.Config
	tasks     []ScheduledTask
	handlers  map[string]TaskHandler
	mu        sync.RWMutex
	running   bool
}

// NewScheduler creates a new scheduler
func NewScheduler(cfg *config.Config) (*Scheduler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	redisOpt := asynq.RedisClientOpt{
		Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password: cfg.Redis.Password,
		DB:       cfg.TaskQueue.RedisDB,
	}

	// Set timezone to Asia/Shanghai
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}

	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{
		Location: loc,
		PostEnqueueFunc: func(info *asynq.TaskInfo, err error) {
			if err != nil {
				if errors.Is(err, asynq.ErrDuplicateTask) {
					logger.Debug("Scheduled task already exists (deduplicated)", map[string]interface{}{
						"task_type": func() string {
							if info != nil {
								return info.Type
							}
							return "unknown"
						}(),
					})
					return
				}
				// info might be nil when there's an error
				taskType := "unknown"
				if info != nil {
					taskType = info.Type
				}
				logger.Critical("failed to enqueue scheduled task",
					"task_type", taskType,
					err,
				)
			} else if info != nil {
				logger.Debug("Scheduled task enqueued", map[string]interface{}{
					"task_id":   info.ID,
					"task_type": info.Type,
					"queue":     info.Queue,
				})
			}
		},
	})

	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.TaskQueue.Concurrency,
		Queues: map[string]int{
			"scheduler": cfg.TaskQueue.Concurrency,
		},
	})

	return &Scheduler{
		scheduler: scheduler,
		server:    server,
		mux:       asynq.NewServeMux(),
		config:    cfg,
		tasks:     make([]ScheduledTask, 0),
		handlers:  make(map[string]TaskHandler),
	}, nil
}

// RegisterTask registers a scheduled task
func (s *Scheduler) RegisterTask(task ScheduledTask, handler TaskHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	taskType := s.getTaskTypeWithPrefix(task.TaskType())

	// Register handler
	s.handlers[taskType] = handler
	s.mux.HandleFunc(taskType, func(ctx context.Context, t *asynq.Task) error {
		taskCtx := logger.WithTaskContext(ctx, t)
		err := handler.Handle(taskCtx, t)
		if err != nil {
			logger.ErrorContext(taskCtx, "scheduled task handler returned error", err)
		}
		return err
	})

	// Register to scheduler
	var entryID string
	var err error

	options := append([]asynq.Option{}, task.Options()...)
	options = append(options, asynq.Queue("scheduler"))

	if task.CronSpec() != "" {
		// Use cron expression
		entryID, err = s.scheduler.Register(
			task.CronSpec(),
			asynq.NewTask(taskType, task.Payload()),
			options...,
		)
	} else if task.Interval() > 0 {
		// Use fixed interval (convert to cron expression)
		cronSpec := fmt.Sprintf("@every %s", task.Interval().String())
		entryID, err = s.scheduler.Register(
			cronSpec,
			asynq.NewTask(taskType, task.Payload()),
			options...,
		)
	} else {
		return fmt.Errorf("task must have either CronSpec or Interval")
	}

	if err != nil {
		return fmt.Errorf("failed to register task %s: %w", taskType, err)
	}

	s.tasks = append(s.tasks, task)

	logger.Info("Scheduled task registered", map[string]interface{}{
		"task_type": taskType,
		"entry_id":  entryID,
		"cron_spec": task.CronSpec(),
	})

	return nil
}

// RegisterTasks registers multiple tasks at once
func (s *Scheduler) RegisterTasks(registrations ...TaskRegistration) error {
	for _, reg := range registrations {
		if err := s.RegisterTask(reg.Task, reg.Handler); err != nil {
			return err
		}
	}
	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.mu.Unlock()

	// Start task processing server in background
	go func() {
		if err := s.server.Run(s.mux); err != nil {
			if !errors.Is(err, asynq.ErrServerClosed) {
				logger.Critical("scheduler server error", err)
			}
		}
	}()

	// Start scheduler
	if err := s.scheduler.Start(); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	logger.Info("Scheduler started successfully", map[string]interface{}{
		"task_count": len(s.tasks),
	})
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.scheduler.Shutdown()
	s.server.Shutdown()
	s.running = false

	logger.Info("Scheduler stopped")
	return nil
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetRegisteredTasks returns all registered tasks
func (s *Scheduler) GetRegisteredTasks() []ScheduledTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tasks
}

// getTaskTypeWithPrefix adds environment prefix to task type
func (s *Scheduler) getTaskTypeWithPrefix(taskType string) string {
	if s.config.TaskQueue.EnvPrefix != "" {
		return fmt.Sprintf("%s:scheduler:%s", s.config.TaskQueue.EnvPrefix, taskType)
	}
	return fmt.Sprintf("scheduler:%s", taskType)
}

// GetMux returns the ServeMux for external handler registration
func (s *Scheduler) GetMux() *asynq.ServeMux {
	return s.mux
}

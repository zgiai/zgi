package service

import (
	"context"
	"errors"
	"sort"

	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

var (
	ErrProcessingExecutorDisabled      = errors.New("processing request executor is disabled")
	ErrProcessingExecutorNotRegistered = errors.New("processing request executor is not registered")
	ErrProcessingExecutorDuplicate     = errors.New("processing request executor is already registered")
)

type ProcessingRequestExecutorInfo struct {
	Key          string   `json:"key"`
	TargetLevels []string `json:"target_levels"`
	Enabled      bool     `json:"enabled"`
	Description  string   `json:"description,omitempty"`
}

type RegisteredProcessingRequestExecutor interface {
	ProcessingRequestExecutor
	Info() ProcessingRequestExecutorInfo
}

type ProcessingExecutorRegistry struct {
	executors map[string]RegisteredProcessingRequestExecutor
}

func NewProcessingExecutorRegistry(executors ...RegisteredProcessingRequestExecutor) (*ProcessingExecutorRegistry, error) {
	registry := &ProcessingExecutorRegistry{
		executors: map[string]RegisteredProcessingRequestExecutor{},
	}
	for _, executor := range executors {
		if err := registry.Register(executor); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func NewDefaultProcessingExecutorRegistry() *ProcessingExecutorRegistry {
	registry, err := NewProcessingExecutorRegistry(
		NewDisabledProcessingRequestExecutor("data-library-parse-disabled", []string{model.DocumentProcessingLevelParse}, "Parse execution is not enabled yet"),
		NewDisabledProcessingRequestExecutor("data-library-split-disabled", []string{model.DocumentProcessingLevelSplit}, "Split execution is not enabled yet"),
		NewDisabledProcessingRequestExecutor("data-library-vector-disabled", []string{model.DocumentProcessingLevelVectorize}, "Vector execution is not enabled yet"),
		NewDisabledProcessingRequestExecutor("data-library-full-disabled", []string{model.DocumentProcessingLevelFull}, "Full processing execution is not enabled yet"),
	)
	if err != nil {
		return &ProcessingExecutorRegistry{executors: map[string]RegisteredProcessingRequestExecutor{}}
	}
	return registry
}

func (r *ProcessingExecutorRegistry) Register(executor RegisteredProcessingRequestExecutor) error {
	if executor == nil {
		return ErrProcessingExecutorRequired
	}
	info := executor.Info()
	if info.Key == "" || executor.Key() == "" || info.Key != executor.Key() {
		return ErrProcessingExecutorKeyRequired
	}
	for _, targetLevel := range info.TargetLevels {
		if !isSupportedProcessingTarget(targetLevel) {
			return ErrProcessingLevelInvalid
		}
	}
	if r.executors == nil {
		r.executors = map[string]RegisteredProcessingRequestExecutor{}
	}
	if _, exists := r.executors[info.Key]; exists {
		return ErrProcessingExecutorDuplicate
	}
	r.executors[info.Key] = executor
	return nil
}

func (r *ProcessingExecutorRegistry) Get(key string) (RegisteredProcessingRequestExecutor, bool) {
	if r == nil || key == "" {
		return nil, false
	}
	executor, ok := r.executors[key]
	return executor, ok
}

func (r *ProcessingExecutorRegistry) MustGet(key string) (RegisteredProcessingRequestExecutor, error) {
	executor, ok := r.Get(key)
	if !ok {
		return nil, ErrProcessingExecutorNotRegistered
	}
	return executor, nil
}

func (r *ProcessingExecutorRegistry) List() []ProcessingRequestExecutorInfo {
	if r == nil || len(r.executors) == 0 {
		return nil
	}
	items := make([]ProcessingRequestExecutorInfo, 0, len(r.executors))
	for _, executor := range r.executors {
		items = append(items, executor.Info())
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
	return items
}

type disabledProcessingRequestExecutor struct {
	info ProcessingRequestExecutorInfo
}

func NewDisabledProcessingRequestExecutor(key string, targetLevels []string, description string) RegisteredProcessingRequestExecutor {
	return &disabledProcessingRequestExecutor{
		info: ProcessingRequestExecutorInfo{
			Key:          key,
			TargetLevels: append([]string(nil), targetLevels...),
			Enabled:      false,
			Description:  description,
		},
	}
}

func (e *disabledProcessingRequestExecutor) Key() string {
	return e.info.Key
}

func (e *disabledProcessingRequestExecutor) Info() ProcessingRequestExecutorInfo {
	info := e.info
	info.TargetLevels = append([]string(nil), e.info.TargetLevels...)
	return info
}

func (e *disabledProcessingRequestExecutor) Enqueue(ctx context.Context, req ProcessingExecutionRequest) error {
	return ErrProcessingExecutorDisabled
}

var _ RegisteredProcessingRequestExecutor = (*disabledProcessingRequestExecutor)(nil)

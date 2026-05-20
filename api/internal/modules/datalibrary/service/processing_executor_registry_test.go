package service

import (
	"context"
	"errors"
	"testing"

	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
)

func TestProcessingExecutorRegistryRegistersAndListsExecutors(t *testing.T) {
	registry, err := NewProcessingExecutorRegistry(
		&registeredTestProcessingExecutor{
			info: ProcessingRequestExecutorInfo{
				Key:          "split-worker",
				TargetLevels: []string{model.DocumentProcessingLevelSplit},
				Enabled:      true,
				Description:  "split artifacts",
			},
		},
		&registeredTestProcessingExecutor{
			info: ProcessingRequestExecutorInfo{
				Key:          "parse-worker",
				TargetLevels: []string{model.DocumentProcessingLevelParse},
				Enabled:      true,
			},
		},
	)
	if err != nil {
		t.Fatalf("NewProcessingExecutorRegistry: %v", err)
	}

	executor, ok := registry.Get("parse-worker")
	if !ok || executor.Key() != "parse-worker" {
		t.Fatalf("executor=%+v ok=%v", executor, ok)
	}

	items := registry.List()
	if len(items) != 2 || items[0].Key != "parse-worker" || items[1].Key != "split-worker" {
		t.Fatalf("items=%+v", items)
	}
}

func TestProcessingExecutorRegistryRejectsInvalidExecutors(t *testing.T) {
	registry, err := NewProcessingExecutorRegistry()
	if err != nil {
		t.Fatalf("NewProcessingExecutorRegistry: %v", err)
	}

	if err := registry.Register(nil); !errors.Is(err, ErrProcessingExecutorRequired) {
		t.Fatalf("nil err=%v", err)
	}

	if err := registry.Register(&registeredTestProcessingExecutor{}); !errors.Is(err, ErrProcessingExecutorKeyRequired) {
		t.Fatalf("key err=%v", err)
	}

	executor := &registeredTestProcessingExecutor{
		info: ProcessingRequestExecutorInfo{
			Key:          "bad-worker",
			TargetLevels: []string{"archive"},
			Enabled:      true,
		},
	}
	if err := registry.Register(executor); !errors.Is(err, ErrProcessingLevelInvalid) {
		t.Fatalf("level err=%v", err)
	}

	executor = &registeredTestProcessingExecutor{
		info: ProcessingRequestExecutorInfo{
			Key:          "parse-worker",
			TargetLevels: []string{model.DocumentProcessingLevelParse},
			Enabled:      true,
		},
	}
	if err := registry.Register(executor); err != nil {
		t.Fatalf("register parse-worker: %v", err)
	}
	if err := registry.Register(executor); !errors.Is(err, ErrProcessingExecutorDuplicate) {
		t.Fatalf("duplicate err=%v", err)
	}
}

func TestDisabledProcessingRequestExecutorDoesNotExecute(t *testing.T) {
	executor := NewDisabledProcessingRequestExecutor(
		"data-library-parse-disabled",
		[]string{model.DocumentProcessingLevelParse},
		"disabled until approved",
	)
	info := executor.Info()
	if info.Enabled || info.Key != "data-library-parse-disabled" || len(info.TargetLevels) != 1 {
		t.Fatalf("info=%+v", info)
	}
	if err := executor.Enqueue(context.Background(), ProcessingExecutionRequest{}); !errors.Is(err, ErrProcessingExecutorDisabled) {
		t.Fatalf("err=%v", err)
	}
}

func TestDefaultProcessingExecutorRegistryIsDisabled(t *testing.T) {
	registry := NewDefaultProcessingExecutorRegistry()
	items := registry.List()
	if len(items) != 4 {
		t.Fatalf("items=%+v", items)
	}
	for _, item := range items {
		if item.Enabled {
			t.Fatalf("default executor should be disabled: %+v", item)
		}
	}
}

type registeredTestProcessingExecutor struct {
	info ProcessingRequestExecutorInfo
}

func (e *registeredTestProcessingExecutor) Key() string {
	return e.info.Key
}

func (e *registeredTestProcessingExecutor) Info() ProcessingRequestExecutorInfo {
	return e.info
}

func (e *registeredTestProcessingExecutor) Enqueue(context.Context, ProcessingExecutionRequest) error {
	return nil
}

var _ RegisteredProcessingRequestExecutor = (*registeredTestProcessingExecutor)(nil)

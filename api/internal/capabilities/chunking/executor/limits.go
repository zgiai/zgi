package executor

import "runtime"

const (
	defaultMaxWorkers       = 4
	defaultMaxWorkersCap    = 8
	defaultMaxPartitionSize = 64
)

// Limits controls per-document chunk execution. It is intentionally local to a
// single document; global backpressure belongs in the task queue/runtime layer.
type Limits struct {
	MaxWorkers       int
	MaxPartitionSize int
}

func DefaultLimits() Limits {
	workers := runtime.NumCPU()
	if workers <= 0 {
		workers = defaultMaxWorkers
	}
	if workers > defaultMaxWorkersCap {
		workers = defaultMaxWorkersCap
	}
	if workers < 1 {
		workers = 1
	}
	return Limits{
		MaxWorkers:       workers,
		MaxPartitionSize: defaultMaxPartitionSize,
	}
}

func normalizeLimits(limits Limits) Limits {
	def := DefaultLimits()
	if limits.MaxWorkers <= 0 {
		limits.MaxWorkers = def.MaxWorkers
	}
	if limits.MaxWorkers > defaultMaxWorkersCap {
		limits.MaxWorkers = defaultMaxWorkersCap
	}
	if limits.MaxPartitionSize <= 0 {
		limits.MaxPartitionSize = def.MaxPartitionSize
	}
	return limits
}

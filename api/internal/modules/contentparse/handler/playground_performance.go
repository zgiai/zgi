package handler

import (
	"fmt"
	"runtime"
	"time"

	chunkexecutor "github.com/zgiai/ginext/internal/capabilities/chunking/executor"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/contracts"
)

type playgroundPerformanceInput struct {
	FileSize               int64
	TotalDuration          time.Duration
	UploadReadDuration     time.Duration
	ProviderHealthDuration time.Duration
	RoutePlanDuration      time.Duration
	ParseDuration          time.Duration
	ChunkMapDuration       time.Duration
	ChunkPlanDuration      time.Duration
	ChunkExecuteDuration   time.Duration
	RoutePlan              *routing.RoutePlan
	Artifact               *contracts.ParseArtifact
	ChunkExecution         *chunkexecutor.Result
	ChunkExecuteErr        error
}

func buildPlaygroundPerformanceSummary(input playgroundPerformanceInput) playgroundPerformanceSummary {
	totalDurationMS := durationMS(input.TotalDuration)
	parseDurationMS := durationMS(input.ParseDuration)
	textLength := 0
	elementCount := 0
	if input.Artifact != nil {
		textLength = len([]rune(input.Artifact.Text))
		elementCount = len(input.Artifact.Elements)
	}

	summary := playgroundPerformanceSummary{
		Runtime:                  "go",
		TotalDurationMS:          totalDurationMS,
		UploadReadDurationMS:     durationMS(input.UploadReadDuration),
		ProviderHealthDurationMS: durationMS(input.ProviderHealthDuration),
		RoutePlanDurationMS:      durationMS(input.RoutePlanDuration),
		ParseDurationMS:          parseDurationMS,
		ChunkMapDurationMS:       durationMS(input.ChunkMapDuration),
		ChunkPlanDurationMS:      durationMS(input.ChunkPlanDuration),
		ChunkExecuteDurationMS:   durationMS(input.ChunkExecuteDuration),
		ProviderAttemptCount:     countRouteAttempts(input.RoutePlan),
		FallbackAttemptCount:     countFallbackRouteAttempts(input.RoutePlan),
		TextCharsPerSecond:       throughputPerSecond(textLength, input.ParseDuration),
		ElementsPerSecond:        throughputPerSecond(elementCount, input.ParseDuration),
		FileMBPerSecond:          fileMBPerSecond(input.FileSize, input.ParseDuration),
		MaxConcurrency:           runtime.GOMAXPROCS(0),
		StageDurations: map[string]int64{
			"upload_read":     durationMS(input.UploadReadDuration),
			"provider_health": durationMS(input.ProviderHealthDuration),
			"route_plan":      durationMS(input.RoutePlanDuration),
			"parse":           parseDurationMS,
			"chunk_map":       durationMS(input.ChunkMapDuration),
			"chunk_plan":      durationMS(input.ChunkPlanDuration),
			"chunk_execute":   durationMS(input.ChunkExecuteDuration),
		},
		Capabilities: []string{
			"provider_route_failover",
			"canonical_ir_mapping",
			"bounded_parallel_chunking",
			"stable_chunk_merge",
		},
		Metadata: map[string]any{
			"runtime_cpu_count": runtime.NumCPU(),
		},
	}

	if input.ChunkExecution != nil {
		summary.ChunkWorkerCount = input.ChunkExecution.Metrics.WorkerCount
		summary.ChunkPartitionCount = input.ChunkExecution.Metrics.PartitionCount
		summary.ChunkUnitCount = input.ChunkExecution.Metrics.UnitCount
		summary.ChunkFilteredUnitCount = input.ChunkExecution.Metrics.FilteredUnitCount
		summary.SourceElementFilteredCount = input.ChunkExecution.Metrics.SourceElementFilteredCount
		if hasBBoxAwareFilter(input.ChunkExecution.Metrics.SourceElementFilterReasons) {
			summary.Capabilities = append(summary.Capabilities, "bbox_aware_noise_filter")
		}
	}
	if input.ChunkExecuteErr != nil {
		summary.Warnings = append(summary.Warnings, fmt.Sprintf("chunk_execute: %v", input.ChunkExecuteErr))
	}
	if summary.ProviderAttemptCount > 1 {
		summary.Capabilities = append(summary.Capabilities, "fallback_attempted")
	}
	return summary
}

func hasBBoxAwareFilter(reasons map[string]int) bool {
	for _, reason := range []string{"tiny_ocr_fragment", "edge_ocr_fragment", "ocr_punctuation_noise"} {
		if reasons[reason] > 0 {
			return true
		}
	}
	return false
}

func durationMS(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return duration.Milliseconds()
}

func throughputPerSecond(count int, duration time.Duration) float64 {
	if count <= 0 || duration <= 0 {
		return 0
	}
	return roundFloat(float64(count)/duration.Seconds(), 2)
}

func fileMBPerSecond(bytes int64, duration time.Duration) float64 {
	if bytes <= 0 || duration <= 0 {
		return 0
	}
	mb := float64(bytes) / 1024 / 1024
	return roundFloat(mb/duration.Seconds(), 2)
}

func countRouteAttempts(plan *routing.RoutePlan) int {
	if plan == nil || len(plan.Metadata) == 0 {
		return 0
	}
	attempts, ok := plan.Metadata["execution_attempts"].([]map[string]any)
	if ok {
		return len(attempts)
	}
	rawAttempts, ok := plan.Metadata["execution_attempts"].([]any)
	if ok {
		return len(rawAttempts)
	}
	return 0
}

func countFallbackRouteAttempts(plan *routing.RoutePlan) int {
	count := countRouteAttempts(plan)
	if count <= 1 {
		return 0
	}
	return count - 1
}

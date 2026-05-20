package service

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
)

const (
	defaultShadowPipelineConcurrency        = 2
	defaultShadowPipelineChunkWorkers       = 2
	defaultShadowPipelineChunkPartitionSize = 64
	maxShadowPipelineConcurrency            = 32
	maxShadowPipelineChunkWorkers           = 8
	maxShadowPipelineChunkPartitionSize     = 512
)

// DatasetShadowInput is the narrow handoff from dataset indexing into the
// content-parse artifact pipeline. It carries the legacy baseline for
// comparison, but it must not be used to write dataset segments or vectors.
type DatasetShadowInput struct {
	DocumentID        string
	DatasetID         string
	OrganizationID    string
	FileID            string
	FileName          string
	Data              []byte
	SourceContentHash string
	WorkspaceID       string
	EngineHint        contracts.ParseEngine
	PrimaryOutput     *dto.ExtractOutput
	LegacyChunks      []dto.TransformedChunk
	InitialSummary    map[string]interface{}
	RecognitionSource string
	Source            string
}

type DatasetShadowSourceContext struct {
	SourceContentHash string
	WorkspaceID       string
}

func NewDatasetShadowSummary(input DatasetShadowInput, capturedAt int64) map[string]interface{} {
	summary := cloneStringAnyMap(input.InitialSummary)
	if summary == nil {
		summary = map[string]interface{}{}
	}
	summary["enabled"] = true
	if _, ok := summary["captured_at"]; !ok {
		if capturedAt <= 0 {
			capturedAt = time.Now().Unix()
		}
		summary["captured_at"] = capturedAt
	}
	return summary
}

func ApplyDatasetShadowSourceContext(summary map[string]interface{}, input DatasetShadowInput, fileHash string, fileWorkspaceID string) (map[string]interface{}, DatasetShadowSourceContext) {
	if summary == nil {
		summary = map[string]interface{}{}
	}
	sourceHash := strings.TrimSpace(input.SourceContentHash)
	if sourceHash == "" {
		sourceHash = strings.TrimSpace(fileHash)
	}
	if sourceHash != "" {
		if _, exists := summary["source_content_hash"]; !exists {
			summary["source_content_hash"] = sourceHash
		}
	}

	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(fileWorkspaceID)
	}
	return summary, DatasetShadowSourceContext{
		SourceContentHash: sourceHash,
		WorkspaceID:       workspaceID,
	}
}

func RoutePlanSummary(plan *routing.RoutePlan) map[string]interface{} {
	if plan == nil {
		return nil
	}
	out := map[string]interface{}{
		"mode":               string(plan.Mode),
		"requested_engine":   string(plan.RequestedEngine),
		"planned_providers":  PlannedProviderOrder(plan),
		"fallback_providers": FallbackProviderOrder(plan),
		"metadata":           cloneStringAnyMap(plan.Metadata),
	}
	if plan.Primary != nil {
		out["primary_provider"] = plan.Primary.ProviderKey
		out["primary_adapter"] = plan.Primary.AdapterName
		out["primary_engine"] = string(plan.Primary.EngineName)
		out["primary_reason"] = cloneStringAnyMap(plan.Primary.Reason)
	}
	return out
}

func PlannedProviderOrder(plan *routing.RoutePlan) []string {
	if plan == nil {
		return nil
	}
	values := make([]string, 0, 1+len(plan.FallbackCandidates))
	if plan.Primary != nil && strings.TrimSpace(plan.Primary.ProviderKey) != "" {
		values = append(values, plan.Primary.ProviderKey)
	}
	values = append(values, FallbackProviderOrder(plan)...)
	return values
}

func FallbackProviderOrder(plan *routing.RoutePlan) []string {
	if plan == nil {
		return nil
	}
	values := make([]string, 0, len(plan.FallbackCandidates))
	for _, item := range plan.FallbackCandidates {
		if strings.TrimSpace(item.ProviderKey) != "" {
			values = append(values, item.ProviderKey)
		}
	}
	return values
}

func NewDatasetShadowParseRequest(input DatasetShadowInput) contracts.ParseRequest {
	return contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		SourceRef:  input.FileID,
		FileName:   input.FileName,
		Intent:     contracts.ParseIntentDatasetIndex,
		Profile:    contracts.ParseProfileDatasetIndex,
		EngineHint: input.EngineHint,
		Metadata: map[string]any{
			"document_id":        input.DocumentID,
			"dataset_id":         input.DatasetID,
			"file_id":            input.FileID,
			"organization_id":    input.OrganizationID,
			"recognition_source": input.RecognitionSource,
			"source":             input.Source,
		},
	}
}

func ApplyDatasetShadowArtifactSummary(summary map[string]interface{}, artifact *contracts.ParseArtifact) map[string]interface{} {
	if summary == nil {
		summary = map[string]interface{}{}
	}
	if artifact == nil {
		return summary
	}
	summary["status"] = artifact.Status
	summary["quality_level"] = artifact.QualityLevel
	summary["engine_used"] = artifact.EngineUsed
	summary["fallback_used"] = artifact.FallbackUsed
	summary["text_length"] = len(artifact.Text)
	summary["markdown_length"] = len(artifact.Markdown)
	summary["element_count"] = len(artifact.Elements)
	summary["artifact_id"] = artifact.ArtifactID
	summary["diagnostics"] = ParseArtifactDiagnosticsSummary(artifact.Diagnostics)
	summary["metadata"] = artifact.Metadata
	return summary
}

func ApplyDatasetShadowFailure(summary map[string]interface{}, err error) map[string]interface{} {
	if summary == nil {
		summary = map[string]interface{}{}
	}
	summary["status"] = "failed"
	if err != nil {
		summary["error"] = err.Error()
	}
	return summary
}

func NewDatasetShadowResult(summary map[string]interface{}, parseRunID string, parseArtifactID string) *DatasetShadowResult {
	result := &DatasetShadowResult{
		Summary:         summary,
		ParseRunID:      strings.TrimSpace(parseRunID),
		ParseArtifactID: strings.TrimSpace(parseArtifactID),
	}
	if value, ok := summary["chunking_run_id"].(string); ok {
		result.ChunkingRunID = value
	}
	if value, ok := summary["chunk_artifact_set_id"].(string); ok {
		result.ChunkArtifactSetID = value
	}
	return result
}

func cloneStringAnyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

// DatasetShadowResult is the durable output of a shadow run. The summary is
// intentionally metadata-only; dataset segments and vector collections remain
// owned by the legacy dataset indexing path until a later cutover.
type DatasetShadowResult struct {
	Summary            map[string]interface{}
	ParseRunID         string
	ParseArtifactID    string
	ChunkingRunID      string
	ChunkArtifactSetID string
}

// ShadowPipelineOptions keeps execution tuning close to the content-parse
// pipeline contract. Dataset indexing should pass work into the pipeline, not
// own parser/chunker tuning once the implementation moves here.
type ShadowPipelineOptions struct {
	Concurrency        int
	ChunkWorkers       int
	ChunkPartitionSize int
}

func DefaultShadowPipelineOptions() ShadowPipelineOptions {
	return ShadowPipelineOptions{
		Concurrency:        defaultShadowPipelineConcurrency,
		ChunkWorkers:       defaultShadowPipelineChunkWorkers,
		ChunkPartitionSize: defaultShadowPipelineChunkPartitionSize,
	}
}

func ShadowPipelineOptionsFromEnv() ShadowPipelineOptions {
	opts := DefaultShadowPipelineOptions()
	opts.Concurrency = readBoundedIntEnv("CONTENT_PARSE_SHADOW_CONCURRENCY", opts.Concurrency, maxShadowPipelineConcurrency)
	opts.ChunkWorkers = readBoundedIntEnv("CONTENT_PARSE_CHUNK_SHADOW_WORKERS", opts.ChunkWorkers, maxShadowPipelineChunkWorkers)
	opts.ChunkPartitionSize = readBoundedIntEnv("CONTENT_PARSE_CHUNK_SHADOW_MAX_PARTITION_SIZE", opts.ChunkPartitionSize, maxShadowPipelineChunkPartitionSize)
	return opts
}

func readBoundedIntEnv(key string, fallback int, maxValue int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return fallback
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

// ShadowPipelineRunner owns parse/chunk artifact shadow execution. Dataset
// indexing should depend on this seam instead of knowing provider routing,
// parse-run persistence, chunk-run persistence, or artifact-set persistence.
type ShadowPipelineRunner interface {
	EnqueueDatasetIndexingShadow(ctx context.Context, input DatasetShadowInput) bool
	RunDatasetIndexingShadow(ctx context.Context, input DatasetShadowInput) (*DatasetShadowResult, error)
}

package handler

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	chunkexecutor "github.com/zgiai/ginext/internal/capabilities/chunking/executor"
	contentparsecap "github.com/zgiai/ginext/internal/capabilities/contentparse"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/contracts"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/pkg/response"
)

const playgroundMaxFileSize = 64 << 20

const playgroundParseSessionTTL = 30 * time.Minute
const playgroundParseSessionMaxEntries = 128
const playgroundParseSessionMaxBytes = 64 << 20
const playgroundParseSessionMaxItemBytes = 8 << 20

var errPlaygroundParseSessionUnavailable = errors.New("parsed result session is expired or unavailable")

type PlaygroundHandler struct {
	orchestrator *contentparsecap.Orchestrator
	planner      routing.Planner
	chunkMapper  contracts.ChunkSourceMapper
	chunkPlanner contracts.ChunkPlanner
	catalog      *contracts.ParseProviderCatalog
	runs         service.PlaygroundRunService
	sessions     *playgroundParseSessionCache
	catalogs     service.ProviderCatalogResolver
}

type playgroundParseResponse struct {
	ParseSessionID string                         `json:"parse_session_id,omitempty"`
	File           playgroundFileSummary          `json:"file"`
	RoutePlan      *routing.RoutePlan             `json:"route_plan,omitempty"`
	Artifact       *contracts.ParseArtifact       `json:"artifact,omitempty"`
	ChunkSource    *contracts.ChunkSourceDocument `json:"chunk_source,omitempty"`
	ChunkPlan      *contracts.ChunkPlan           `json:"chunk_plan,omitempty"`
	ChunkExecution *chunkexecutor.Result          `json:"chunk_execution,omitempty"`
	QualitySummary playgroundQualitySummary       `json:"quality_summary"`
	Performance    playgroundPerformanceSummary   `json:"performance_summary"`
}

type playgroundSaveResponse struct {
	Run         *model.PlaygroundRun    `json:"run"`
	ParseResult playgroundParseResponse `json:"parse_result"`
}

type playgroundShareResponse struct {
	Run      *model.PlaygroundRun `json:"run"`
	ShareURL string               `json:"share_url,omitempty"`
}

type playgroundRunListResponse struct {
	Items []*model.PlaygroundRun `json:"items"`
}

type playgroundCompareResponse struct {
	SourceContentHash string                 `json:"source_content_hash"`
	Items             []*model.PlaygroundRun `json:"items"`
}

type playgroundProviderSummaryResponse struct {
	Items []service.PlaygroundProviderSummary `json:"items"`
}

type playgroundPDFRenderResponse struct {
	Engine    string   `json:"engine"`
	PageCount int      `json:"page_count"`
	Pages     []string `json:"pages"`
}

type playgroundProvidersResponse struct {
	Source    string                     `json:"source"`
	Providers []playgroundProviderStatus `json:"providers"`
	OCR       []playgroundOCRStatus      `json:"ocr_engines"`
}

type playgroundProviderStatus struct {
	Key          string                `json:"key"`
	DisplayName  string                `json:"display_name"`
	Type         string                `json:"type"`
	AdapterName  string                `json:"adapter_name"`
	EngineName   contracts.ParseEngine `json:"engine_name,omitempty"`
	Enabled      bool                  `json:"enabled"`
	Configured   bool                  `json:"configured"`
	Available    bool                  `json:"available"`
	Selectable   bool                  `json:"selectable"`
	FallbackOnly bool                  `json:"fallback_only"`
	Priority     int                   `json:"priority,omitempty"`
	Status       string                `json:"status"`
	Reason       string                `json:"reason,omitempty"`
}

type playgroundOCRStatus struct {
	Key       string `json:"key"`
	Provider  string `json:"provider,omitempty"`
	Available bool   `json:"available"`
	Default   bool   `json:"default,omitempty"`
	Path      string `json:"path,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type playgroundFileSummary struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type playgroundQualitySummary struct {
	Status         contracts.ParseStatus       `json:"status"`
	QualityLevel   contracts.ParseQualityLevel `json:"quality_level"`
	EngineUsed     contracts.ParseEngine       `json:"engine_used,omitempty"`
	FallbackUsed   bool                        `json:"fallback_used"`
	DurationMS     int64                       `json:"duration_ms"`
	TextLength     int                         `json:"text_length"`
	MarkdownLength int                         `json:"markdown_length"`
	ElementCount   int                         `json:"element_count"`
	BBoxCount      int                         `json:"bbox_count"`
	ReliableBBox   int                         `json:"reliable_bbox_count"`
	UnreliableBBox int                         `json:"unreliable_bbox_count"`
	BBoxRatio      float64                     `json:"bbox_ratio"`
	ReliableRatio  float64                     `json:"reliable_bbox_ratio"`
	PageCount      int                         `json:"page_count"`
	AvgConfidence  float64                     `json:"avg_confidence,omitempty"`
	OCREngine      string                      `json:"ocr_engine,omitempty"`
	OCRStrategy    string                      `json:"ocr_strategy,omitempty"`
}

type playgroundPerformanceSummary struct {
	Runtime                    string           `json:"runtime"`
	TotalDurationMS            int64            `json:"total_duration_ms"`
	UploadReadDurationMS       int64            `json:"upload_read_duration_ms"`
	ProviderHealthDurationMS   int64            `json:"provider_health_duration_ms"`
	RoutePlanDurationMS        int64            `json:"route_plan_duration_ms"`
	ParseDurationMS            int64            `json:"parse_duration_ms"`
	ChunkMapDurationMS         int64            `json:"chunk_map_duration_ms"`
	ChunkPlanDurationMS        int64            `json:"chunk_plan_duration_ms"`
	ChunkExecuteDurationMS     int64            `json:"chunk_execute_duration_ms"`
	ProviderAttemptCount       int              `json:"provider_attempt_count"`
	FallbackAttemptCount       int              `json:"fallback_attempt_count"`
	ChunkWorkerCount           int              `json:"chunk_worker_count"`
	ChunkPartitionCount        int              `json:"chunk_partition_count"`
	ChunkUnitCount             int              `json:"chunk_unit_count"`
	ChunkFilteredUnitCount     int              `json:"chunk_filtered_unit_count"`
	SourceElementFilteredCount int              `json:"source_element_filtered_count"`
	TextCharsPerSecond         float64          `json:"text_chars_per_second"`
	ElementsPerSecond          float64          `json:"elements_per_second"`
	FileMBPerSecond            float64          `json:"file_mb_per_second"`
	MaxConcurrency             int              `json:"max_concurrency"`
	StageDurations             map[string]int64 `json:"stage_durations_ms,omitempty"`
	Capabilities               []string         `json:"capabilities,omitempty"`
	Warnings                   []string         `json:"warnings,omitempty"`
	Metadata                   map[string]any   `json:"metadata,omitempty"`
}

type playgroundExecution struct {
	Response             playgroundParseResponse
	RequestedProviderKey string
	AdapterName          string
	EffectiveRequest     contracts.ParseRequest
	SourceData           []byte
	SourceMimeType       string
	SourceFileExt        string
}

type playgroundCachedExecution struct {
	Response             playgroundParseResponse
	RequestedProviderKey string
	AdapterName          string
	EffectiveRequest     contracts.ParseRequest
	SourceMimeType       string
	SourceFileExt        string
	WorkspaceID          *uuid.UUID
	AccountID            *uuid.UUID
	ExpiresAt            time.Time
	LastAccessedAt       time.Time
	SizeBytes            int64
}

type playgroundParseSessionCache struct {
	mu           sync.Mutex
	ttl          time.Duration
	maxEntries   int
	maxBytes     int64
	maxItemBytes int64
	totalBytes   int64
	items        map[string]playgroundCachedExecution
}

type playgroundRequestError struct {
	code response.ErrorCode
	err  error
}

func (e *playgroundRequestError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *playgroundRequestError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

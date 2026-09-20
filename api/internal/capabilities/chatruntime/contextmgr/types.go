package contextmgr

import (
	"context"
	"errors"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

var ErrContextExhausted = errors.New("agent context exhausted")

const (
	RequestTypeMain            = "main"
	RequestTypeCompact         = "compact"
	RequestTypeReactiveCompact = "reactive_compact"
)

const (
	DecisionNone            = "none"
	DecisionToolProjection  = "tool_projection"
	DecisionMicrocompact    = "microcompact"
	DecisionSemanticCompact = "semantic_compact"
	DecisionReactiveCompact = "reactive_compact"
)

// Config is immutable for one logical Agent run. All token limits are derived
// from AgentContextWindow, never directly from the model's physical window.
type Config struct {
	AgentRunID              string
	ConfiguredAgentWindowK  int
	ModelContextWindow      int
	MaxInputTokens          int
	MaxOutputTokens         int
	DefaultMainOutputTokens int
	SummaryOutputTokens     int
	EmergencyBufferTokens   int
	HysteresisTokens        int
	TargetRatio             float64
	MaxToolResultTokens     int
	ToolResultPreviewRunes  int
	// TailMinMessages is the minimum number of protocol messages kept verbatim
	// during semantic compaction. Tool-call batches may make the actual tail
	// larger so an assistant tool call is never separated from its results.
	TailMinMessages int
}

type Budget struct {
	ModelContextWindow        int     `json:"model_context_window"`
	ConfiguredAgentWindowK    int     `json:"configured_agent_window_k"`
	AgentContextWindow        int     `json:"agent_context_window"`
	AgentContextWindowClamped bool    `json:"agent_context_window_clamped"`
	MainOutputReserve         int     `json:"main_output_reserve"`
	SummaryOutputReserve      int     `json:"summary_output_reserve"`
	PromptBudget              int     `json:"prompt_budget"`
	CompactInputLimit         int     `json:"compact_input_limit"`
	SoftLimit                 int     `json:"soft_limit"`
	HardLimit                 int     `json:"hard_limit"`
	TargetTokens              int     `json:"target_tokens"`
	TailMinTokens             int     `json:"tail_min_tokens"`
	TailMaxTokens             int     `json:"tail_max_tokens"`
	ContextPressure           float64 `json:"context_pressure"`
}

type Decision struct {
	AgentRunID                    string         `json:"agent_run_id"`
	APIRound                      int            `json:"api_round"`
	RequestType                   string         `json:"request_type"`
	Action                        string         `json:"action"`
	Budget                        Budget         `json:"budget"`
	FixedRequestTokens            int            `json:"fixed_request_tokens"`
	CompressibleTokens            int            `json:"compressible_tokens"`
	FinalPromptTokens             int            `json:"final_prompt_tokens"`
	BeforeTokens                  int            `json:"before_tokens"`
	AfterTokens                   int            `json:"after_tokens"`
	CompactedThroughRound         int            `json:"compacted_through_round,omitempty"`
	PreservedRounds               []int          `json:"preserved_rounds,omitempty"`
	PreservedMessageCount         int            `json:"preserved_message_count,omitempty"`
	ToolResultOriginalTokens      int            `json:"tool_result_original_tokens"`
	ToolResultProjectedTokens     int            `json:"tool_result_projected_tokens"`
	ToolProjectionCount           int            `json:"tool_projection_count"`
	LossyRecoveryDroppedRounds    int            `json:"lossy_recovery_dropped_rounds,omitempty"`
	LossyRecoveryDroppedBatches   int            `json:"lossy_recovery_dropped_batches,omitempty"`
	Estimator                     string         `json:"estimator"`
	EstimateScale                 float64        `json:"estimate_scale"`
	CompactionFailure             string         `json:"compaction_failure,omitempty"`
	ConsecutiveCompactionFailures int            `json:"consecutive_compaction_failures"`
	ComponentTokens               map[string]int `json:"component_tokens,omitempty"`
}

type ContextSummary struct {
	Content               string    `json:"content"`
	CompactedThroughRound int       `json:"compacted_through_round"`
	CreatedAt             time.Time `json:"created_at"`
}

// CheckpointFunc persists the turn transcript at an interruption point: a new
// API round is about to start, or a model response was just observed. The
// manager hands over its own snapshot, so a checkpoint never has to call back
// into the manager it was registered on.
type CheckpointFunc func(ctx context.Context, transcript []adapter.Message)

type ContentReplacement struct {
	ToolCallID     string `json:"tool_call_id"`
	ContentHash    string `json:"content_hash"`
	ArtifactRef    string `json:"artifact_ref,omitempty"`
	OriginalTokens int    `json:"original_tokens"`
	PreviewTokens  int    `json:"preview_tokens"`
	Replacement    string `json:"replacement"`
}

type ContextArtifactResult struct {
	ArtifactRef      string `json:"artifact_ref"`
	ContentHash      string `json:"content_hash"`
	SourceToolCallID string `json:"source_tool_call_id,omitempty"`
	Content          string `json:"content"`
	ReturnedTokens   int    `json:"returned_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

type CompactionTracking struct {
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastFailure         string    `json:"last_failure,omitempty"`
	LastCompactedAt     time.Time `json:"last_compacted_at,omitempty"`
}

type AgentContextState struct {
	SchemaVersion               int               `json:"schema_version"`
	AgentRunID                  string            `json:"agent_run_id"`
	NextRound                   int               `json:"next_round"`
	Provider                    string            `json:"provider,omitempty"`
	Model                       string            `json:"model,omitempty"`
	Tokenizer                   string            `json:"tokenizer,omitempty"`
	ModelContextWindowTokens    int               `json:"model_context_window_tokens"`
	ConfiguredAgentWindowTokens int               `json:"configured_agent_window_tokens"`
	EffectiveAgentWindowTokens  int               `json:"effective_agent_window_tokens"`
	Messages                    []adapter.Message `json:"messages"`
	// TurnTranscript contains only model-visible assistant/tool messages
	// produced by this logical Agent turn. Bootstrap history, system messages,
	// the current user query, reasoning, and presentation-only SSE events are
	// intentionally excluded so the transcript can be persisted in the owning
	// chat_runtime_messages row and replayed between turns.
	TurnTranscript      []adapter.Message             `json:"turn_transcript,omitempty"`
	Summary             *ContextSummary               `json:"summary,omitempty"`
	ContentReplacements map[string]ContentReplacement `json:"content_replacements,omitempty"`
	LastUsage           *adapter.Usage                `json:"last_usage,omitempty"`
	EstimatorScale      float64                       `json:"estimator_scale"`
	Compaction          CompactionTracking            `json:"compaction"`
	CreatedAt           time.Time                     `json:"created_at,omitempty"`
}

type CompactCall struct {
	AgentRunID string
	APIRound   int
	Type       string
	Decision   Decision
	// Transcript is the turn transcript as it stands right before this
	// compaction. It travels by value so a compactor can persist it without
	// reaching back into the manager that is calling it.
	Transcript []adapter.Message
	// SummarizedEntryID is the exact parent-chain boundary represented by the
	// summary. An empty value means the compact input contained no durable entry.
	SummarizedEntryID string
}

type Compactor interface {
	Compact(context.Context, *adapter.ChatRequest, CompactCall) (string, *adapter.Usage, error)
}

type ToolResultStore interface {
	Put(context.Context, string, string, string) (string, error)
	Get(context.Context, string, string) (string, error)
}

type RequestObserver func(requestType string, round int, request *adapter.ChatRequest, decision Decision)

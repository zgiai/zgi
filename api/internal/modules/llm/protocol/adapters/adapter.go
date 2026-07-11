package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
)

// ProviderAdapter defines the provider-level contract shared by all adapters.
type ProviderAdapter interface {
	// ValidateConfig validates configuration validity
	ValidateConfig(config *AdapterConfig) error

	// GetProviderInfo gets provider metadata
	GetProviderInfo() *ProviderInfo
}

// ChatCapable defines text/chat generation capability.
type ChatCapable interface {
	ChatCompletion(ctx context.Context, request *ChatRequest) (*ChatResponse, error)
	ChatCompletionStream(ctx context.Context, request *ChatRequest) (<-chan StreamResponse, error)
}

// ResponseCapable defines /responses style generation capability.
type ResponseCapable interface {
	CreateResponse(ctx context.Context, request *CreateResponseRequest) (*CreateResponseResponse, error)
}

// RawResponseCapable defines native OpenAI Responses API capability.
type RawResponseCapable interface {
	CreateResponseRaw(ctx context.Context, request *RawResponseRequest) (*RawResponse, error)
	CreateResponseStream(ctx context.Context, request *RawResponseRequest) (<-chan RawStreamEvent, error)
}

// AnthropicMessagesCapable defines native Anthropic Messages API capability.
type AnthropicMessagesCapable interface {
	CreateAnthropicMessage(ctx context.Context, request *AnthropicMessageRequest) (*RawResponse, error)
	CreateAnthropicMessageStream(ctx context.Context, request *AnthropicMessageRequest) (<-chan RawStreamEvent, error)
}

// EmbeddingCapable defines embedding generation capability.
type EmbeddingCapable interface {
	CreateEmbeddings(ctx context.Context, request *EmbeddingsRequest) (*EmbeddingsResponse, error)
}

// ImageCapable defines image generation capability.
type ImageCapable interface {
	CreateImage(ctx context.Context, request *ImageRequest) (*ImageResponse, error)
}

// RerankCapable defines rerank capability.
type RerankCapable interface {
	Rerank(ctx context.Context, request *RerankRequest) (*RerankResponse, error)
}

// ModelListingCapable defines upstream model listing capability.
type ModelListingCapable interface {
	ListModels(ctx context.Context, apiKey string) ([]Model, error)
}

// BalanceCapable defines balance query capability.
type BalanceCapable interface {
	GetBalance(ctx context.Context, apiKey string) (*Balance, error)
}

// LLMProviderAdapter keeps the existing composite contract for compatibility.
type LLMProviderAdapter interface {
	ProviderAdapter
	ChatCapable
	ResponseCapable
	EmbeddingCapable
	ImageCapable
	RerankCapable
	ModelListingCapable
	BalanceCapable
}

// ChatRequest platform standard chat request format
type ChatRequest struct {
	Provider         string             `json:"-"`
	Model            string             `json:"model"`
	Messages         []Message          `json:"messages"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"top_p,omitempty"`
	MaxTokens        *int               `json:"max_tokens,omitempty"`
	Stream           bool               `json:"stream,omitempty"`
	StreamOptions    *StreamOptions     `json:"stream_options,omitempty"`
	Stop             []string           `json:"stop,omitempty"`
	PresencePenalty  *float64           `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64           `json:"frequency_penalty,omitempty"`
	User             string             `json:"user,omitempty"`
	Functions        []Function         `json:"functions,omitempty"`
	FunctionCall     interface{}        `json:"function_call,omitempty"`
	Tools            []Tool             `json:"tools,omitempty"`
	ToolChoice       interface{}        `json:"tool_choice,omitempty"`
	ResponseFormat   *ResponseFormat    `json:"response_format,omitempty"`
	Seed             *int               `json:"seed,omitempty"`
	N                *int               `json:"n,omitempty"`
	LogitBias        map[string]float64 `json:"logit_bias,omitempty"`

	// AdditionalParameters model-specific parameters that are not part of the standard core.
	// These are mapped by adapters to provider-specific formats.
	AdditionalParameters map[string]interface{} `json:"-"`
}

// StreamOptions options for streaming requests
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// CreateResponseRequest response API request format
type CreateResponseRequest struct {
	Model           string            `json:"model"`
	Input           interface{}       `json:"input,omitempty"`    // OpenRouter: string or array of messages
	Messages        []Message         `json:"messages,omitempty"` // Alternative to Input
	Tools           []Tool            `json:"tools,omitempty"`
	ToolChoice      interface{}       `json:"tool_choice,omitempty"`
	ResponseFormat  *ResponseFormat   `json:"response_format,omitempty"`
	Temperature     *float64          `json:"temperature,omitempty"`
	TopP            *float64          `json:"top_p,omitempty"`
	MaxTokens       *int              `json:"max_tokens,omitempty"`
	MaxOutputTokens *int              `json:"max_output_tokens,omitempty"` // OpenRouter specific
	Stream          bool              `json:"stream,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Instructions    string            `json:"instructions,omitempty"` // System instructions
	Modalities      []string          `json:"modalities,omitempty"`   // e.g. ["text", "image"]

	// AdditionalParameters model-specific parameters
	AdditionalParameters map[string]interface{} `json:"-"`
}

// RawResponseRequest preserves the external OpenAI Responses wire contract.
type RawResponseRequest struct {
	Model  string          `json:"model"`
	Stream bool            `json:"stream,omitempty"`
	Body   json.RawMessage `json:"-"`
}

// AnthropicMessageRequest preserves the external Anthropic Messages wire contract.
type AnthropicMessageRequest struct {
	Model   string            `json:"model"`
	Stream  bool              `json:"stream,omitempty"`
	Body    json.RawMessage   `json:"-"`
	Headers map[string]string `json:"-"`
}

// RawResponse carries a provider-native JSON response plus parsed usage for billing.
type RawResponse struct {
	Body       json.RawMessage   `json:"-"`
	Usage      *Usage            `json:"-"`
	Settlement *SettlementResult `json:"-"`
}

// RawStreamEvent carries a provider-native SSE event plus parsed usage for billing.
type RawStreamEvent struct {
	Event      string            `json:"-"`
	Data       json.RawMessage   `json:"-"`
	Usage      *Usage            `json:"-"`
	Settlement *SettlementResult `json:"-"`
	Error      error             `json:"-"`
	Done       bool              `json:"-"`
}

// EmbeddingsRequest embeddings request format
type EmbeddingsRequest struct {
	Input          interface{} `json:"input"` // string or []string or []int or [][]int
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"` // float or base64
	Dimensions     int         `json:"dimensions,omitempty"`
	InputType      string      `json:"input_type,omitempty"`
	Truncate       string      `json:"truncate,omitempty"`
	MaxTokens      int         `json:"max_tokens,omitempty"`
	User           string      `json:"user,omitempty"`
}

// EmbeddingsResponse embeddings response format
type EmbeddingsResponse struct {
	Object     string            `json:"object"`
	Data       []Embedding       `json:"data"`
	Model      string            `json:"model"`
	Usage      Usage             `json:"usage"`
	Settlement *SettlementResult `json:"-"`
}

// Embedding embedding object
type Embedding struct {
	Object    string    `json:"object"` // "embedding"
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// RerankRequest rerank request format
type RerankRequest struct {
	Model           string      `json:"model"`
	Query           string      `json:"query"`                        // The search query
	Documents       interface{} `json:"documents"`                    // []string or []map[string]interface{} with text field
	TopN            *int        `json:"top_n,omitempty"`              // Return top N most relevant documents
	ScoreThreshold  *float64    `json:"score_threshold,omitempty"`    // Minimum relevance score threshold
	MaxTokensPerDoc *int        `json:"max_tokens_per_doc,omitempty"` // Maximum tokens per document (Cohere)
	Priority        *int        `json:"priority,omitempty"`           // Priority level (Cohere specific)
	ReturnDocuments *bool       `json:"return_documents,omitempty"`   // Whether to return document text
	MaxChunksPerDoc *int        `json:"max_chunks_per_doc,omitempty"` // Deprecated: use MaxTokensPerDoc
	RankFields      []string    `json:"rank_fields,omitempty"`        // Fields to rank on (for structured documents)
}

// RerankResponse rerank response format
type RerankResponse struct {
	ID         string            `json:"id,omitempty"`
	Object     string            `json:"object"`
	Model      string            `json:"model"`
	Results    []RerankResult    `json:"results"`
	Usage      *Usage            `json:"usage,omitempty"`
	Settlement *SettlementResult `json:"-"`
}

// RerankResult individual rerank result
type RerankResult struct {
	Index          int         `json:"index"`              // Original index in input documents
	RelevanceScore float64     `json:"relevance_score"`    // Relevance score (typically 0-1)
	Document       interface{} `json:"document,omitempty"` // Original document if return_documents=true
	Text           string      `json:"text,omitempty"`     // Document text
}

// CreateResponseResponse response API response format
type CreateResponseResponse struct {
	ID        string   `json:"id"`
	Object    string   `json:"object"`
	Created   int64    `json:"created"`
	CreatedAt int64    `json:"created_at"` // OpenRouter uses created_at
	Model     string   `json:"model"`
	Usage     *Usage   `json:"usage,omitempty"`
	Output    []Output `json:"output,omitempty"` // OpenRouter responses have 'output'
	Choices   []Choice `json:"choices,omitempty"`
	Status    string   `json:"status,omitempty"` // OpenRouter: completed, in_progress, etc.
}

// Output response output (OpenRouter format)
type Output struct {
	Type       string          `json:"type"`              // "message"
	ID         string          `json:"id,omitempty"`      // Message ID
	Status     string          `json:"status,omitempty"`  // "completed", "in_progress"
	Role       string          `json:"role,omitempty"`    // "assistant"
	Content    []OutputContent `json:"content,omitempty"` // Array of content parts
	Message    *Message        `json:"message,omitempty"` // Legacy support
	RawContent interface{}     `json:"-"`                 // For flexible parsing
}

// OutputContent represents a content part in the output
type OutputContent struct {
	Type        string        `json:"type"` // "output_text", "input_text", etc.
	Text        string        `json:"text,omitempty"`
	Annotations []interface{} `json:"annotations,omitempty"`
}

// Message message structure
type Message struct {
	Role             string        `json:"role,omitempty"`    // system, user, assistant, function, tool
	Content          interface{}   `json:"content,omitempty"` // string or []MessageContentPart for multimodal
	Name             string        `json:"name,omitempty"`
	FunctionCall     *FunctionCall `json:"function_call,omitempty"`
	ToolCalls        []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ReasoningContent string        `json:"reasoning_content,omitempty"` // DeepSeek thinking-mode round trip
}

// MessageContentPart represents a content part in multimodal messages (OpenAI format)
type MessageContentPart struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for type="text"
	ImageURL *ImageURL `json:"image_url,omitempty"` // for type="image_url"
}

// ImageURL represents image URL in multimodal messages
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// Function function definition
type Function struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

// FunctionCall function call
type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// Tool tool definition
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// ToolCall tool call
type ToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

// ResponseFormat response format
type ResponseFormat struct {
	Type   string         `json:"type"`             // text, json_object
	Schema map[string]any `json:"schema,omitempty"` // optional JSON schema for structured output
}

// ChatResponse platform standard chat response format
type ChatResponse struct {
	ID         string            `json:"id"`
	Object     string            `json:"object"`
	Created    int64             `json:"created"`
	Model      string            `json:"model"`
	Choices    []Choice          `json:"choices"`
	Usage      *Usage            `json:"usage,omitempty"`
	Settlement *SettlementResult `json:"-"`
}

// Choice response choice
type Choice struct {
	Index        int      `json:"index"`
	Message      Message  `json:"message"`
	FinishReason string   `json:"finish_reason,omitempty"` // stop, length, function_call, tool_calls, content_filter
	Logprobs     *Logprob `json:"logprobs,omitempty"`
}

// Logprob log probability
type Logprob struct {
	Content []TokenLogprob `json:"content,omitempty"`
}

// TokenLogprob token log probability
type TokenLogprob struct {
	Token   string  `json:"token"`
	Logprob float64 `json:"logprob"`
}

// Usage token usage statistics
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SettlementResult carries console-api settlement data for official traffic.
type SettlementResult struct {
	SettlementID     string `json:"settlement_id"`
	OfficialPoints   int64  `json:"official_points"`
	RemainingBalance int64  `json:"remaining_balance"`
	Status           string `json:"status"`
}

// SettlementError carries console-api settlement failure data for official streams.
type SettlementError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// StreamResponse streaming response
type StreamResponse struct {
	ID         string            `json:"id"`
	Object     string            `json:"object"`
	Created    int64             `json:"created"`
	Model      string            `json:"model"`
	Choices    []StreamChoice    `json:"choices"`
	Usage      *Usage            `json:"usage,omitempty"` // Token usage info (usually in last chunk)
	Settlement *SettlementResult `json:"-"`
	Error      error             `json:"-"`
	Done       bool              `json:"-"`
}

// StreamChoice streaming response choice
type StreamChoice struct {
	Index        int     `json:"index"`
	Delta        Message `json:"delta"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Model model information
type Model struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Type                string                 `json:"type,omitempty"`
	Description         string                 `json:"description,omitempty"`
	ContextLength       int                    `json:"context_length"`
	Capabilities        []string               `json:"capabilities,omitempty"`
	Pricing             *Pricing               `json:"pricing,omitempty"`
	Created             int64                  `json:"created,omitempty"`
	OwnedBy             string                 `json:"owned_by,omitempty"`
	Permission          []string               `json:"permission,omitempty"`
	TopProvider         *ProviderInfo          `json:"top_provider,omitempty"`
	Architecture        *ModelArchitecture     `json:"architecture,omitempty"`
	SupportedParameters []string               `json:"supported_parameters,omitempty"`
	DefaultParameters   map[string]interface{} `json:"default_parameters,omitempty"`
	IsModerated         bool                   `json:"is_moderated,omitempty"`
	Endpoints           []string               `json:"endpoints,omitempty"`    // Supported API endpoints (e.g., ["chat", "embed", "rerank"])
	IsFinetuned         bool                   `json:"is_finetuned,omitempty"` // Whether model is a fine-tuned version
}

// ModelArchitecture model architecture info
type ModelArchitecture struct {
	Modality         string   `json:"modality,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	Tokenizer        string   `json:"tokenizer,omitempty"`
	InstructType     string   `json:"instruct_type,omitempty"`
}

// Pricing pricing information
type Pricing struct {
	Prompt            decimal.Decimal `json:"prompt"`                       // Price per million tokens
	Completion        decimal.Decimal `json:"completion"`                   // Price per million tokens
	Image             decimal.Decimal `json:"image,omitempty"`              // Image price (if applicable)
	Request           decimal.Decimal `json:"request,omitempty"`            // Price per request (if applicable)
	ImageToken        decimal.Decimal `json:"image_token,omitempty"`        // Price per image token
	ImageOutput       decimal.Decimal `json:"image_output,omitempty"`       // Price per image output
	Audio             decimal.Decimal `json:"audio,omitempty"`              // Price per minute audio
	InputAudioCache   decimal.Decimal `json:"input_audio_cache,omitempty"`  // Price per audio cache
	WebSearch         decimal.Decimal `json:"web_search,omitempty"`         // Price per web search
	InternalReasoning decimal.Decimal `json:"internal_reasoning,omitempty"` // Price per reasoning token
	InputCacheRead    decimal.Decimal `json:"input_cache_read,omitempty"`   // Price per cache read
	InputCacheWrite   decimal.Decimal `json:"input_cache_write,omitempty"`  // Price per cache write
}

type BalanceScope string

const (
	BalanceScopeAccount  BalanceScope = "account_balance"
	BalanceScopeKeyLimit BalanceScope = "key_limit"
)

type BalanceItem struct {
	Currency  string          `json:"currency"`
	Remaining decimal.Decimal `json:"remaining"`
}

// Balance balance information
type Balance struct {
	Total       decimal.Decimal `json:"total"`                  // Total quota
	Used        decimal.Decimal `json:"used"`                   // Used amount
	Remaining   decimal.Decimal `json:"remaining"`              // Remaining quota
	Currency    string          `json:"currency"`               // Currency unit (USD, CNY, etc.)
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`   // Expiration time
	IsUnlimited bool            `json:"is_unlimited,omitempty"` // Whether unlimited quota
	Scope       BalanceScope    `json:"scope,omitempty"`
	Items       []BalanceItem   `json:"items,omitempty"`
	Spendable   *bool           `json:"spendable,omitempty"`
}

// ProviderInfo provider information
type ProviderInfo struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // openai, anthropic, custom
	DisplayName  string   `json:"display_name"`
	Description  string   `json:"description,omitempty"`
	BaseURL      string   `json:"base_url"`
	Capabilities []string `json:"capabilities,omitempty"` // chat, completion, embedding, image, etc.
	Version      string   `json:"version,omitempty"`
}

// AdapterConfig adapter configuration
type AdapterConfig struct {
	ProviderName string                 `json:"provider_name"` // Provider unique identifier (e.g., openai, deepseek, openrouter)
	ProviderID   string                 `json:"provider_id"`   // Provider UUID for database lookup
	APIKey       string                 `json:"api_key"`
	BaseURL      string                 `json:"base_url,omitempty"`
	Timeout      time.Duration          `json:"timeout,omitempty"`
	MaxRetries   int                    `json:"max_retries,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty"`
	CustomParams map[string]interface{} `json:"custom_params,omitempty"`
	Organization string                 `json:"organization,omitempty"` // OpenAI specific

	GuardOutboundURL    bool `json:"-"`
	GuardOutboundDNS    bool `json:"-"`
	AllowPrivateBaseURL bool `json:"-"`

	// ProviderConfig carries provider-specific parameters, such as cloud project or signing metadata.
	ProviderConfig map[string]interface{} `json:"provider_config,omitempty"`

	// AuthHook is called on every outgoing HTTP request before it is sent.
	// Use this to inject custom authentication (e.g., HMAC signing for internal APIs).
	// When set, the adapter should call this instead of (or in addition to) setting Authorization header.
	AuthHook func(req *http.Request) `json:"-"`
}

// ImageRequest represents image generation request
type ImageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              *int   `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	User           string `json:"user,omitempty"`

	// AdditionalParameters model-specific parameters
	AdditionalParameters map[string]interface{} `json:"-"`
}

// ImageResponse represents image generation response
type ImageResponse struct {
	Created    int64             `json:"created"`
	Data       []ImageItem       `json:"data"`
	Settlement *SettlementResult `json:"-"`
}

// ImageItem represents a single generated image
type ImageItem struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

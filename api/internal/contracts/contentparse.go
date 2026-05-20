package contracts

import "context"

// ParseIntent identifies the consumer's primary goal so implementations can
// choose different parsing plans without coupling business modules to engines.
type ParseIntent string

const (
	ParseIntentPreview      ParseIntent = "preview"
	ParseIntentDatasetIndex ParseIntent = "dataset_index"
	ParseIntentChatContext  ParseIntent = "chat_context"
)

// ParseSourceType describes how raw content reaches the parsing capability.
type ParseSourceType string

const (
	ParseSourceTypeBytes      ParseSourceType = "bytes"
	ParseSourceTypeUploadFile ParseSourceType = "upload_file"
	ParseSourceTypeURL        ParseSourceType = "url"
	ParseSourceTypeInlineText ParseSourceType = "inline_text"
)

// ParseEngine is an abstract engine preference exposed to zgi modules. Concrete
// adapters may map it to SDK-specific or remote implementations internally.
type ParseEngine string

const (
	ParseEngineLocal   ParseEngine = "local"
	ParseEngineMineru  ParseEngine = "mineru"
	ParseEngineReducto ParseEngine = "reducto"
	ParseEngineVLM     ParseEngine = "vlm"
)

// ParseProviderType is the provider family exposed to platform operators.
type ParseProviderType string

const (
	ParseProviderTypeBuiltin ParseProviderType = "builtin"
	ParseProviderTypeRemote  ParseProviderType = "remote"
)

// ParseProfile lets business flows describe the desired parsing plan without
// binding themselves to a specific vendor or backend.
type ParseProfile string

const (
	ParseProfileAuto         ParseProfile = "auto"
	ParseProfileHighQuality  ParseProfile = "high_quality"
	ParseProfileFast         ParseProfile = "fast"
	ParseProfileLocalFirst   ParseProfile = "local_first"
	ParseProfileDefault      ParseProfile = "default"
	ParseProfileFastPreview  ParseProfile = "fast_preview"
	ParseProfileLayoutFirst  ParseProfile = "layout_first"
	ParseProfileTextFirst    ParseProfile = "text_first"
	ParseProfileDatasetIndex ParseProfile = "dataset_index"
)

type ParseStatus string

const (
	ParseStatusSucceeded ParseStatus = "succeeded"
	ParseStatusDegraded  ParseStatus = "degraded"
	ParseStatusFailed    ParseStatus = "failed"
)

type ParseQualityLevel string

const (
	ParseQualityHigh     ParseQualityLevel = "high"
	ParseQualityStandard ParseQualityLevel = "standard"
	ParseQualityDegraded ParseQualityLevel = "degraded"
	ParseQualityFailed   ParseQualityLevel = "failed"
)

// ParseRequest is the stable boundary contract consumed by business modules.
// This foundation version is intentionally small and byte-oriented so it can be
// introduced without changing any existing upload or dataset flows.
type ParseRequest struct {
	SourceType ParseSourceType `json:"source_type"`
	SourceRef  string          `json:"source_ref,omitempty"`
	FileName   string          `json:"file_name,omitempty"`
	Data       []byte          `json:"-"`
	Text       string          `json:"text,omitempty"`

	Intent     ParseIntent  `json:"intent"`
	Profile    ParseProfile `json:"profile,omitempty"`
	EngineHint ParseEngine  `json:"engine_hint,omitempty"`
	Force      bool         `json:"force,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

type ParseBoundingBox struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

type ParsedElement struct {
	ID         string            `json:"id,omitempty"`
	Type       string            `json:"type"`
	Subtype    string            `json:"subtype,omitempty"`
	Page       int               `json:"page"`
	Content    string            `json:"content,omitempty"`
	BBox       *ParseBoundingBox `json:"bbox,omitempty"`
	Ordinal    int               `json:"ordinal"`
	Precision  string            `json:"precision,omitempty"`
	Confidence *float64          `json:"confidence,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// ParseArtifact is the normalized parsing output that downstream modules can
// later cache, index, preview, or transform further.
type ParseArtifact struct {
	ArtifactID string `json:"artifact_id,omitempty"`

	SourceType ParseSourceType `json:"source_type"`
	SourceRef  string          `json:"source_ref,omitempty"`
	FileName   string          `json:"file_name,omitempty"`

	Intent  ParseIntent  `json:"intent"`
	Profile ParseProfile `json:"profile,omitempty"`

	Status       ParseStatus       `json:"status"`
	QualityLevel ParseQualityLevel `json:"quality_level"`
	EngineUsed   ParseEngine       `json:"engine_used,omitempty"`
	FallbackUsed bool              `json:"fallback_used,omitempty"`

	Text        string          `json:"text,omitempty"`
	Markdown    string          `json:"markdown,omitempty"`
	Elements    []ParsedElement `json:"elements,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
	Diagnostics map[string]any  `json:"diagnostics,omitempty"`
}

type AdapterHealth struct {
	Name      string         `json:"name"`
	Available bool           `json:"available"`
	Details   map[string]any `json:"details,omitempty"`
}

// ParseProviderConfig is the stable configuration model that a future admin UI
// can use to manage third-party parsing providers without exposing implementation
// details to business modules.
type ParseProviderConfig struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"display_name,omitempty"`
	Type         ParseProviderType `json:"type"`
	Enabled      bool              `json:"enabled"`
	Priority     int               `json:"priority,omitempty"`
	FallbackOnly bool              `json:"fallback_only,omitempty"`
	Adapter      string            `json:"adapter"`
	Engine       ParseEngine       `json:"engine,omitempty"`
	BaseURL      string            `json:"base_url,omitempty"`
	APIKeyEnv    string            `json:"api_key_env,omitempty"`
	TimeoutSec   int               `json:"timeout_sec,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
}

type ParseProviderCatalog struct {
	Providers []ParseProviderConfig `json:"providers"`
}

type ParseHealth struct {
	Adapters []AdapterHealth `json:"adapters"`
}

// ContentParseService is the platform-level capability boundary. Modules should
// depend on this contract instead of binding themselves to hyperparse directly.
type ContentParseService interface {
	Parse(ctx context.Context, req ParseRequest) (*ParseArtifact, error)
	Health(ctx context.Context) (*ParseHealth, error)
}

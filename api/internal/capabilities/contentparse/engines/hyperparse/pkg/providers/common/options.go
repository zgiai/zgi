package common

import "strings"

// Engine identifies the requested parser provider.
type Engine string

const (
	EngineLocal   Engine = "local"
	EngineMineru  Engine = "mineru"
	EngineVLM     Engine = "vlm"
	EngineReducto Engine = "reducto"
)

// ParseOptions contains shared provider options.
type ParseOptions struct {
	Mode string

	// ProviderRuntime is a request-scoped configuration snapshot. It must never
	// be written into process-global environment or package state.
	ProviderRuntime ProviderRuntimeConfig

	// ForceLocalVLM only affects the local engine: force the VLM fallback-page
	// selector after native parsing. This is a slower high-accuracy path and
	// should not be enabled by ordinary retry/no-cache operations.
	ForceLocalVLM bool

	// ForceLocalSidebarRecovery only affects the local engine: widen candidate
	// pages for right-sidebar recovery. Local OCR is tried before sidebar VLM.
	ForceLocalSidebarRecovery bool

	// OCREngine only affects local OCR repair/fallback. Empty means environment default.
	// Supported values: tesseract / paddleocr.
	OCREngine string

	// ImageRetryAggressive only affects the local image path and enables a larger
	// OCR retry budget for quality-first flows such as dataset indexing.
	ImageRetryAggressive bool

	// EnableImageVLMFallback only affects the local image path and allows selective
	// VLM fallback when OCR quality is poor.
	EnableImageVLMFallback bool

	// OnProgress optionally reports long-running parse stages to the host UI.
	// It is mainly used by local image captioning and OCR fallback stages.
	OnProgress func(ParseProgress)
}

type ProviderRuntimeConfig struct {
	ProviderKey         string
	Enabled             *bool
	Mode                string
	BaseURL             string
	APIKey              string
	TimeoutSeconds      int
	PollIntervalSeconds int
	ModelVersion        string
}

// ParseProgress is a lightweight progress event emitted to the host service.
type ParseProgress struct {
	Stage   string         `json:"stage,omitempty"`
	Status  string         `json:"status,omitempty"`
	Message string         `json:"message,omitempty"`
	Current int            `json:"current,omitempty"`
	Total   int            `json:"total,omitempty"`
	Detail  map[string]any `json:"detail,omitempty"`
}

// ParseEngine resolves current provider names and legacy aliases.
func ParseEngine(v string) (Engine, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(EngineLocal), "sdk":
		return EngineLocal, true
	case string(EngineMineru):
		return EngineMineru, true
	case string(EngineVLM), "gemini":
		return EngineVLM, true
	case string(EngineReducto):
		return EngineReducto, true
	default:
		return "", false
	}
}

// ResolveEngine prefers the request engine, then the backend default, and finally local.
// MinerU is a heavier model-backed parser and should be selected explicitly.
func ResolveEngine(requestEngine, backendEnv string) Engine {
	if eng, ok := ParseEngine(requestEngine); ok {
		return eng
	}
	if eng, ok := ParseEngine(backendEnv); ok {
		return eng
	}
	return EngineLocal
}

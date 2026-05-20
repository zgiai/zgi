// Package hyperparse provides the embedded runtime facade for the ZGI content
// parse capability: unified parse entry points, PDF full_document inspection,
// DPT export, and JSON-based runtime configuration.
//
// Architecture notes:
//   - Core parsing remains under internal/*; this package composes and forwards
//     rather than duplicating implementations.
//   - Lightweight multi-format parsing uses internal/core/pipeline.
//   - Deep PDF structuring uses internal/orchestrators/pdf.
//   - PDF VLM fallback and image caption orchestration live in internal/inspectsvc.
//   - Config.ApplyEnviron maps vision/vlm/pdf/ocr settings to process env vars;
//     VLM_* remains the preferred model-provider surface with legacy aliases.
//
// Embedded service example:
//
//	cfg, _ := hyperparse.LoadConfigJSON("hyperparse.config.json")
//	cfg.ApplyEnviron()
//	// then call RunPDFInspect / NewClient and related entry points
package hyperparse

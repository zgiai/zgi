package hyperparsesdk

import (
	"context"
	"fmt"
	"os"
	"strings"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	extractlocal "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/local"
	extractmineru "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/mineru"
	extractreducto "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/reducto"
	extractvlm "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/vlm"
	"github.com/zgiai/zgi/api/internal/contracts"
)

const adapterName = "hyperparse_sdk"

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) Name() string {
	return adapterName
}

func (a *Adapter) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	engine := toHyperparseEngine(req.EngineHint)
	opts := parseOptionsForRequest(req)

	result, err := a.parseWithRequest(ctx, engine, req, opts)
	if err != nil {
		return nil, err
	}

	return mapDocumentResult(req, engine, result), nil
}

func parseOptionsForRequest(req contracts.ParseRequest) extractcommon.ParseOptions {
	opts := extractcommon.ParseOptions{Mode: "relaxed"}
	switch req.Profile {
	case contracts.ParseProfileHighQuality, contracts.ParseProfileLayoutFirst:
		opts.Mode = "strict"
		opts.ImageRetryAggressive = true
		opts.EnableImageVLMFallback = true
	case contracts.ParseProfileDatasetIndex:
		opts.Mode = "strict"
		opts.ImageRetryAggressive = true
	case contracts.ParseProfileFast, contracts.ParseProfileFastPreview:
		opts.Mode = "relaxed"
	case contracts.ParseProfileLocalFirst:
		opts.Mode = "strict"
		opts.ImageRetryAggressive = true
		opts.EnableImageVLMFallback = false
	}
	if ocrEngine := metadataString(req.Metadata, "ocr_engine"); ocrEngine != "" {
		opts.OCREngine = ocrEngine
	}
	return opts
}

func ParseOptionsForRequest(req contracts.ParseRequest) extractcommon.ParseOptions {
	return parseOptionsForRequest(req)
}

func (a *Adapter) Health(_ context.Context) (contracts.AdapterHealth, error) {
	return contracts.AdapterHealth{
		Name:      a.Name(),
		Available: true,
		Details: map[string]any{
			"embedded_sdk":          true,
			"mineru_api_configured": extractmineru.Configured(),
			"mineru_mode":           extractmineru.Mode(),
			"reducto_configured":    strings.TrimSpace(os.Getenv("REDUCTO_API_KEY")) != "",
		},
	}, nil
}

func (a *Adapter) parseWithRequest(ctx context.Context, engine extractcommon.Engine, req contracts.ParseRequest, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	if supportsInputRef(engine, req.SourceType, req.SourceRef) {
		return a.parseInputRef(ctx, engine, strings.TrimSpace(req.SourceRef), opts)
	}
	if req.SourceType != contracts.ParseSourceTypeBytes {
		return nil, fmt.Errorf("hyperparse sdk adapter currently requires byte input for source type %q", req.SourceType)
	}
	if req.FileName == "" {
		return nil, fmt.Errorf("hyperparse sdk adapter requires file_name")
	}
	return a.parseBytes(ctx, engine, req.FileName, req.Data, opts)
}

func (a *Adapter) parseBytes(ctx context.Context, engine extractcommon.Engine, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	switch engine {
	case extractcommon.EngineLocal:
		return extractlocal.New().ParseBytes(ctx, filename, data, opts)
	case extractcommon.EngineMineru:
		return extractmineru.New().ParseBytes(ctx, filename, data, opts)
	case extractcommon.EngineReducto:
		return extractreducto.New().ParseBytes(ctx, filename, data, opts)
	case extractcommon.EngineVLM:
		return extractvlm.New().ParseBytes(ctx, filename, data, opts)
	default:
		return nil, fmt.Errorf("unsupported hyperparse sdk engine %q", engine)
	}
}

func (a *Adapter) parseInputRef(ctx context.Context, engine extractcommon.Engine, input string, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	switch engine {
	case extractcommon.EngineReducto:
		return extractreducto.New().ParseInput(ctx, input, opts)
	default:
		return nil, fmt.Errorf("engine %q does not support non-byte parse inputs yet", engine)
	}
}

func supportsInputRef(engine extractcommon.Engine, sourceType contracts.ParseSourceType, sourceRef string) bool {
	if strings.TrimSpace(sourceRef) == "" {
		return false
	}
	if engine != extractcommon.EngineReducto {
		return false
	}
	switch sourceType {
	case contracts.ParseSourceTypeURL, contracts.ParseSourceTypeUploadFile:
		return true
	default:
		return strings.HasPrefix(sourceRef, "reducto://") || strings.HasPrefix(sourceRef, "jobid://") || strings.HasPrefix(sourceRef, "http://") || strings.HasPrefix(sourceRef, "https://")
	}
}

func toHyperparseEngine(engine contracts.ParseEngine) extractcommon.Engine {
	switch engine {
	case contracts.ParseEngineMineru:
		return extractcommon.EngineMineru
	case contracts.ParseEngineReducto:
		return extractcommon.EngineReducto
	case contracts.ParseEngineVLM:
		return extractcommon.EngineVLM
	default:
		return extractcommon.EngineLocal
	}
}

func mapDocumentResult(req contracts.ParseRequest, engine extractcommon.Engine, result *extractcommon.DocumentResult) *contracts.ParseArtifact {
	if result == nil {
		return &contracts.ParseArtifact{
			SourceType:   req.SourceType,
			SourceRef:    req.SourceRef,
			FileName:     req.FileName,
			Intent:       req.Intent,
			Profile:      req.Profile,
			Status:       contracts.ParseStatusFailed,
			QualityLevel: contracts.ParseQualityFailed,
			EngineUsed:   contracts.ParseEngine(engine),
		}
	}

	extractcommon.EnrichStructuredOutput(result)

	artifact := &contracts.ParseArtifact{
		ArtifactID:   result.DocID,
		SourceType:   req.SourceType,
		SourceRef:    req.SourceRef,
		FileName:     req.FileName,
		Intent:       req.Intent,
		Profile:      req.Profile,
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityStandard,
		EngineUsed:   contracts.ParseEngine(engine),
		FallbackUsed: false,
		Text:         strings.TrimSpace(result.Markdown),
		Markdown:     strings.TrimSpace(result.Markdown),
		Metadata:     map[string]any{},
		Diagnostics:  cloneMap(result.Diagnostics),
		Elements:     make([]contracts.ParsedElement, 0, len(result.ExtractOutput.Elements)),
	}

	if extraction, ok := result.ExtractOutput.Metadata["extraction"].(map[string]any); ok {
		if fallbackUsed, ok := extraction["fallback_used"].(bool); ok {
			artifact.FallbackUsed = fallbackUsed
			if fallbackUsed {
				artifact.Status = contracts.ParseStatusDegraded
				artifact.QualityLevel = contracts.ParseQualityDegraded
			}
		}
	}

	for key, value := range result.ExtractOutput.Metadata {
		artifact.Metadata[key] = value
	}
	if strings.TrimSpace(result.Source) != "" {
		artifact.Metadata["recognition_source"] = strings.TrimSpace(result.Source)
	}

	for _, element := range result.ExtractOutput.Elements {
		artifact.Elements = append(artifact.Elements, contracts.ParsedElement{
			ID:        element.ID,
			Type:      element.Type,
			Subtype:   element.Subtype,
			Page:      element.Page,
			Content:   element.Content,
			BBox:      mapBoundingBox(element.BBox),
			Ordinal:   element.Ordinal,
			Precision: element.Precision,
			Confidence: readConfidence(
				element.Metadata,
			),
			Metadata: cloneMap(element.Metadata),
		})
	}

	if artifact.Status == contracts.ParseStatusSucceeded && strings.TrimSpace(artifact.Markdown) == "" && len(artifact.Elements) == 0 {
		artifact.Status = contracts.ParseStatusDegraded
		artifact.QualityLevel = contracts.ParseQualityDegraded
		if artifact.Diagnostics == nil {
			artifact.Diagnostics = map[string]any{}
		}
		artifact.Diagnostics["empty_output"] = map[string]any{
			"reason":     "parser returned no text or structured elements",
			"page_count": result.PageCount,
		}
	}

	return artifact
}

func MapDocumentResult(req contracts.ParseRequest, engine extractcommon.Engine, result *extractcommon.DocumentResult) *contracts.ParseArtifact {
	return mapDocumentResult(req, engine, result)
}

func mapBoundingBox(box *extractcommon.BBox) *contracts.ParseBoundingBox {
	if box == nil {
		return nil
	}
	return &contracts.ParseBoundingBox{
		Left:   box.Left,
		Top:    box.Top,
		Right:  box.Right,
		Bottom: box.Bottom,
	}
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func readConfidence(metadata map[string]any) *float64 {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["confidence"]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case float64:
		return &value
	case float32:
		converted := float64(value)
		return &converted
	case int:
		converted := float64(value)
		return &converted
	case int32:
		converted := float64(value)
		return &converted
	case int64:
		converted := float64(value)
		return &converted
	default:
		return nil
	}
}

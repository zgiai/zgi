package hyperparseapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/contracts"
)

const adapterName = "hyperparse_api"

type Adapter struct {
	baseURL string
	apiKey  string
	timeout time.Duration
}

func NewAdapter() *Adapter {
	return &Adapter{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("CONTENT_PARSE_HYPERPARSE_API_BASE_URL")), "/"),
		apiKey:  firstNonEmptyEnv("CONTENT_PARSE_HYPERPARSE_API_KEY", "HYPERPARSE_API_KEY"),
		timeout: timeoutFromEnv("CONTENT_PARSE_HYPERPARSE_API_TIMEOUT_SECONDS", 180*time.Second),
	}
}

func (a *Adapter) Name() string {
	return adapterName
}

func (a *Adapter) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	if strings.TrimSpace(a.baseURL) == "" {
		return nil, fmt.Errorf("hyperparse api adapter base URL is not configured")
	}
	if req.SourceType != contracts.ParseSourceTypeBytes {
		return nil, fmt.Errorf("hyperparse api adapter currently requires byte input for source type %q", req.SourceType)
	}
	if req.FileName == "" {
		return nil, fmt.Errorf("hyperparse api adapter requires file_name")
	}

	reqCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", req.FileName)
	if err != nil {
		return nil, fmt.Errorf("hyperparse api create form file: %w", err)
	}
	if _, err := fileWriter.Write(req.Data); err != nil {
		return nil, fmt.Errorf("hyperparse api write file bytes: %w", err)
	}
	_ = writer.WriteField("engine", toRemoteEngine(req.EngineHint))
	_ = writer.WriteField("mode", modeForProfile(req.Profile))
	if req.Profile != "" {
		_ = writer.WriteField("profile", string(req.Profile))
	}
	if req.Force {
		_ = writer.WriteField("fresh", "1")
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("hyperparse api close multipart: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, a.baseURL+"/api/hyperparse/parse", body)
	if err != nil {
		return nil, fmt.Errorf("hyperparse api new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if strings.TrimSpace(a.apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("hyperparse api request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("hyperparse api read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hyperparse api HTTP %d: %s", resp.StatusCode, snippet(raw))
	}

	var parsed parseDocumentEnvelope
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("hyperparse api decode response: %w", err)
	}
	if !parsed.OK {
		return nil, fmt.Errorf("hyperparse api parse failed: %s", parsed.Error)
	}

	return parsed.Result.toArtifact(req), nil
}

func (a *Adapter) Health(_ context.Context) (contracts.AdapterHealth, error) {
	return contracts.AdapterHealth{
		Name:      a.Name(),
		Available: strings.TrimSpace(a.baseURL) != "",
		Details: map[string]any{
			"base_url":      a.baseURL,
			"api_key_set":   strings.TrimSpace(a.apiKey) != "",
			"timeout_sec":   int(a.timeout.Seconds()),
			"engine_source": "remote_api",
		},
	}, nil
}

type parseDocumentEnvelope struct {
	OK     bool               `json:"ok"`
	Error  string             `json:"error,omitempty"`
	Result remoteParsedResult `json:"result"`
}

type remoteParsedResult struct {
	DocID       string               `json:"doc_id"`
	FileName    string               `json:"file_name"`
	PageCount   int                  `json:"page_count"`
	Markdown    string               `json:"markdown"`
	Source      string               `json:"source,omitempty"`
	Chunks      []remoteParsedChunk  `json:"chunks"`
	Diagnostics map[string]any       `json:"diagnostics,omitempty"`
	ExtractOut  *remoteExtractOutput `json:"extract_output,omitempty"`
}

type remoteParsedChunk struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Type       string         `json:"type"`
	Subtype    string         `json:"subtype,omitempty"`
	Page       int            `json:"page"`
	Bbox       *remoteBBox    `json:"bbox,omitempty"`
	Text       string         `json:"text,omitempty"`
	Markdown   string         `json:"markdown,omitempty"`
	Ordinal    int            `json:"ordinal"`
	Confidence float64        `json:"confidence,omitempty"`
	Precision  string         `json:"precision,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type remoteExtractOutput struct {
	Elements []remoteExtractElement `json:"elements"`
	Markdown string                 `json:"markdown,omitempty"`
	Source   string                 `json:"source,omitempty"`
	Metadata map[string]any         `json:"metadata,omitempty"`
}

type remoteExtractElement struct {
	ID        string         `json:"id,omitempty"`
	Type      string         `json:"type"`
	Subtype   string         `json:"subtype,omitempty"`
	Page      int            `json:"page"`
	Content   string         `json:"content,omitempty"`
	Bbox      *remoteBBox    `json:"bbox,omitempty"`
	Ordinal   int            `json:"ordinal"`
	Precision string         `json:"precision,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type remoteBBox struct {
	Left   float64 `json:"left"`
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
}

func (r remoteParsedResult) toArtifact(req contracts.ParseRequest) *contracts.ParseArtifact {
	metadata := map[string]any{}
	if r.ExtractOut != nil {
		metadata = cloneMap(r.ExtractOut.Metadata)
	}
	artifact := &contracts.ParseArtifact{
		ArtifactID:   r.DocID,
		SourceType:   req.SourceType,
		SourceRef:    req.SourceRef,
		FileName:     coalesce(r.FileName, req.FileName),
		Intent:       req.Intent,
		Profile:      req.Profile,
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityStandard,
		EngineUsed:   req.EngineHint,
		Text:         strings.TrimSpace(r.Markdown),
		Markdown:     strings.TrimSpace(r.Markdown),
		Metadata:     metadata,
		Diagnostics:  cloneMap(r.Diagnostics),
	}
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]any{}
	}
	if artifact.Metadata["source"] == nil && strings.TrimSpace(r.Source) != "" {
		artifact.Metadata["source"] = r.Source
	}

	if r.ExtractOut != nil && len(r.ExtractOut.Elements) > 0 {
		artifact.Elements = make([]contracts.ParsedElement, 0, len(r.ExtractOut.Elements))
		for _, el := range r.ExtractOut.Elements {
			artifact.Elements = append(artifact.Elements, contracts.ParsedElement{
				ID:        el.ID,
				Type:      el.Type,
				Subtype:   el.Subtype,
				Page:      el.Page,
				Content:   el.Content,
				BBox:      toBBox(el.Bbox),
				Ordinal:   el.Ordinal,
				Precision: el.Precision,
				Confidence: readConfidence(
					el.Metadata,
				),
				Metadata: cloneMap(el.Metadata),
			})
		}
	} else {
		artifact.Elements = make([]contracts.ParsedElement, 0, len(r.Chunks))
		for _, ch := range r.Chunks {
			artifact.Elements = append(artifact.Elements, contracts.ParsedElement{
				ID:        ch.ID,
				Type:      ch.Type,
				Subtype:   ch.Subtype,
				Page:      ch.Page,
				Content:   coalesce(ch.Markdown, ch.Text),
				BBox:      toBBox(ch.Bbox),
				Ordinal:   ch.Ordinal,
				Precision: ch.Precision,
				Confidence: floatPointer(
					ch.Confidence,
				),
				Metadata: cloneMap(ch.Payload),
			})
		}
	}
	return artifact
}

func toBBox(box *remoteBBox) *contracts.ParseBoundingBox {
	if box == nil {
		return nil
	}
	return &contracts.ParseBoundingBox{
		Left: box.Left, Top: box.Top, Right: box.Right, Bottom: box.Bottom,
	}
}

func modeForProfile(profile contracts.ParseProfile) string {
	switch profile {
	case contracts.ParseProfileHighQuality,
		contracts.ParseProfileLayoutFirst,
		contracts.ParseProfileDatasetIndex,
		contracts.ParseProfileLocalFirst:
		return "strict"
	default:
		return "relaxed"
	}
}

func floatPointer(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
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

func timeoutFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	secs, err := strconv.Atoi(raw)
	if err != nil || secs <= 0 {
		return fallback
	}
	return time.Duration(secs) * time.Second
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func toRemoteEngine(engine contracts.ParseEngine) string {
	switch engine {
	case contracts.ParseEngineMineru:
		return "mineru"
	case contracts.ParseEngineReducto:
		return "reducto"
	case contracts.ParseEngineVLM:
		return "vlm"
	default:
		return "local"
	}
}

func snippet(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 800 {
		return text[:800] + "..."
	}
	return text
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

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

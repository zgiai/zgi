package reducto

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

const (
	defaultBaseURL = "https://platform.reducto.ai"
	defaultTimeout = 180 * time.Second
)

type Client struct{}

func New() *Client { return &Client{} }

func Configured() bool {
	return reductoEnabled() && strings.TrimSpace(os.Getenv("REDUCTO_API_KEY")) != ""
}

func (c *Client) ParseBytes(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	_ = opts
	if !Configured() {
		return nil, fmt.Errorf("REDUCTO_ENABLED/REDUCTO_API_KEY is not configured")
	}
	fileID, err := upload(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	resp, err := parse(ctx, fileID)
	if err != nil {
		return nil, err
	}
	doc := toDocumentResult(filename, resp)
	return extractcommon.EnrichStructuredOutput(doc), nil
}

func (c *Client) ParseInput(ctx context.Context, input string, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	_ = opts
	if !Configured() {
		return nil, fmt.Errorf("REDUCTO_ENABLED/REDUCTO_API_KEY is not configured")
	}
	resp, err := parse(ctx, strings.TrimSpace(input))
	if err != nil {
		return nil, err
	}
	doc := toDocumentResult(strings.TrimSpace(input), resp)
	return extractcommon.EnrichStructuredOutput(doc), nil
}

func reductoEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REDUCTO_ENABLED"))) {
	case "", "1", "true", "yes", "on", "enabled":
		return true
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return true
	}
}

func baseURL() string {
	if v := strings.TrimSpace(os.Getenv("REDUCTO_BASE_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

func timeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("REDUCTO_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultTimeout
}

func upload(ctx context.Context, filename string, data []byte) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return "", fmt.Errorf("reducto: create multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("reducto: write multipart: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("reducto: close multipart: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL()+"/upload", &body)
	if err != nil {
		return "", fmt.Errorf("reducto: new upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(os.Getenv("REDUCTO_API_KEY")))
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("reducto: upload: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reducto: read upload response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("reducto: upload HTTP %d: %s", resp.StatusCode, snippet(respBody))
	}
	fileID, err := reductoFileID(respBody)
	if err != nil {
		return "", err
	}
	return fileID, nil
}

func reductoFileID(raw []byte) (string, error) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && strings.TrimSpace(asString) != "" {
		return strings.TrimSpace(asString), nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("reducto: decode upload response: %w", err)
	}
	for _, key := range []string{"file_id", "fileID", "upload", "id"} {
		if v, ok := obj[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), nil
		}
	}
	return "", fmt.Errorf("reducto: upload response missing file_id")
}

type parseResponse struct {
	JobID      string         `json:"job_id"`
	Duration   float64        `json:"duration,omitempty"`
	StudioLink string         `json:"studio_link,omitempty"`
	PDFURL     string         `json:"pdf_url,omitempty"`
	Usage      map[string]any `json:"usage,omitempty"`
	Result     parseResult    `json:"result"`
}

type parseResult struct {
	Type   string         `json:"type,omitempty"`
	URL    string         `json:"url,omitempty"`
	Chunks []parseChunk   `json:"chunks,omitempty"`
	Raw    map[string]any `json:"-"`
}

type parseChunk struct {
	Type              string         `json:"type,omitempty"`
	Content           string         `json:"content,omitempty"`
	Embed             string         `json:"embed,omitempty"`
	Enriched          string         `json:"enriched,omitempty"`
	EnrichmentSuccess bool           `json:"enrichment_success,omitempty"`
	BBox              map[string]any `json:"bbox,omitempty"`
	Blocks            []parseBlock   `json:"blocks,omitempty"`
}

type parseBlock struct {
	Type               string         `json:"type,omitempty"`
	Content            string         `json:"content,omitempty"`
	BBox               map[string]any `json:"bbox,omitempty"`
	Confidence         any            `json:"confidence,omitempty"`
	GranularConfidence map[string]any `json:"granular_confidence,omitempty"`
	ImageURL           string         `json:"image_url,omitempty"`
}

func parse(ctx context.Context, input string) (*parseResponse, error) {
	reqBody := map[string]any{
		"input": input,
		"formatting": map[string]any{
			"table_output_format": "html",
		},
		"retrieval": map[string]any{
			"chunking": map[string]any{"chunk_mode": "variable"},
		},
		"settings": map[string]any{
			"return_images": []string{"figure", "table"},
		},
		"enhance": map[string]any{
			"summarize_figures": true,
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("reducto: marshal parse request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL()+"/parse", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("reducto: new parse request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(os.Getenv("REDUCTO_API_KEY")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reducto: parse: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reducto: read parse response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("reducto: parse HTTP %d: %s", resp.StatusCode, snippet(respBody))
	}
	var parsed parseResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("reducto: decode parse response: %w", err)
	}
	if strings.EqualFold(parsed.Result.Type, "url") && strings.TrimSpace(parsed.Result.URL) != "" {
		chunks, err := fetchURLChunks(ctx, parsed.Result.URL)
		if err != nil {
			return nil, err
		}
		parsed.Result.Chunks = chunks
	}
	return &parsed, nil
}

func fetchURLChunks(ctx context.Context, resultURL string) ([]parseChunk, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, resultURL, nil)
	if err != nil {
		return nil, fmt.Errorf("reducto: new result-url request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reducto: fetch url result: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("reducto: read url result: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("reducto: url result HTTP %d: %s", resp.StatusCode, snippet(respBody))
	}
	chunks, err := decodeChunksPayload(respBody)
	if err != nil {
		return nil, err
	}
	return chunks, nil
}

func decodeChunksPayload(raw []byte) ([]parseChunk, error) {
	var direct []parseChunk
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}
	var wrapped struct {
		Chunks []parseChunk `json:"chunks"`
		Result struct {
			Chunks []parseChunk `json:"chunks"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("reducto: decode url result: %w", err)
	}
	if len(wrapped.Chunks) > 0 {
		return wrapped.Chunks, nil
	}
	return wrapped.Result.Chunks, nil
}

func toDocumentResult(filename string, resp *parseResponse) *extractcommon.DocumentResult {
	if resp == nil {
		return &extractcommon.DocumentResult{DocID: newID(), FileName: filename, Source: "reducto:parse"}
	}
	chunks := make([]extractcommon.Chunk, 0)
	markdownParts := make([]string, 0)
	maxPage := -1
	ordinal := 0
	for chunkIdx, chunk := range resp.Result.Chunks {
		if strings.TrimSpace(chunk.Content) != "" {
			markdownParts = append(markdownParts, strings.TrimSpace(chunk.Content))
		}
		if len(chunk.Blocks) == 0 {
			ordinal++
			bbox, page := bboxFromMap(chunk.BBox)
			if page > maxPage {
				maxPage = page
			}
			text := strings.TrimSpace(chunk.Content)
			payload := map[string]any{
				"source":                "reducto",
				"reducto_chunk_ordinal": chunkIdx + 1,
			}
			if strings.TrimSpace(chunk.Embed) != "" {
				payload["reducto_embed"] = strings.TrimSpace(chunk.Embed)
			}
			if strings.TrimSpace(chunk.Enriched) != "" {
				payload["reducto_enriched"] = strings.TrimSpace(chunk.Enriched)
			}
			if chunk.EnrichmentSuccess {
				payload["reducto_enrichment_success"] = true
			}
			kind, subtype := normalizeType(chunk.Type)
			chunks = append(chunks, buildChunk(ordinal, chunkIdx, kind, subtype, text, bbox, page, confidenceScore(nil, nil), payload))
			continue
		}
		for _, block := range chunk.Blocks {
			ordinal++
			bbox, page := bboxFromMap(block.BBox)
			if page > maxPage {
				maxPage = page
			}
			text := strings.TrimSpace(block.Content)
			if text == "" && strings.EqualFold(block.Type, "figure") {
				text = "Figure"
			}
			payload := map[string]any{
				"source":                "reducto",
				"reducto_type":          block.Type,
				"reducto_chunk_ordinal": chunkIdx + 1,
			}
			if block.Confidence != nil {
				payload["reducto_confidence"] = block.Confidence
			}
			if block.GranularConfidence != nil {
				payload["reducto_granular_confidence"] = block.GranularConfidence
			}
			if strings.TrimSpace(block.ImageURL) != "" {
				payload["image_url"] = strings.TrimSpace(block.ImageURL)
			}
			kind, subtype := normalizeType(block.Type)
			chunks = append(chunks, buildChunk(ordinal, chunkIdx, kind, subtype, text, bbox, page, confidenceScore(block.Confidence, block.GranularConfidence), payload))
		}
	}
	pageCount := intFromMap(resp.Usage, "num_pages")
	if pageCount <= 0 {
		pageCount = maxPage + 1
	}
	if pageCount <= 0 && len(chunks) > 0 {
		pageCount = 1
	}
	pages := make([]extractcommon.Page, 0, pageCount)
	for i := 0; i < pageCount; i++ {
		pages = append(pages, extractcommon.Page{PageIndex: i, Width: 1, Height: 1})
	}
	docID := strings.TrimSpace(resp.JobID)
	if docID == "" {
		docID = newID()
	}
	return &extractcommon.DocumentResult{
		DocID:     docID,
		FileName:  filename,
		PageCount: pageCount,
		Pages:     pages,
		Chunks:    chunks,
		Markdown:  strings.Join(markdownParts, "\n\n"),
		Source:    "reducto:parse",
		Diagnostics: map[string]any{
			"reducto_job_id":      resp.JobID,
			"reducto_duration":    resp.Duration,
			"reducto_studio_link": resp.StudioLink,
			"reducto_pdf_url":     resp.PDFURL,
			"reducto_usage":       resp.Usage,
		},
	}
}

func buildChunk(ordinal int, chunkIdx int, typ, subtype string, text string, bbox *extractcommon.BBox, page int, confidence float64, payload map[string]any) extractcommon.Chunk {
	precision := "unreliable"
	if bbox != nil {
		precision = "reliable"
	}
	if payload == nil {
		payload = map[string]any{"source": "reducto", "reducto_chunk_ordinal": chunkIdx + 1}
	}
	payload["page_index"] = page + 1
	return extractcommon.Chunk{
		ID:         fmt.Sprintf("reducto-%03d", ordinal),
		Type:       typ,
		Subtype:    subtype,
		Page:       page,
		BBox:       bbox,
		Text:       text,
		Markdown:   text,
		Ordinal:    ordinal,
		Confidence: confidence,
		Precision:  precision,
		Payload:    payload,
	}
}

func normalizeType(raw string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "title", "header", "section header", "heading":
		if strings.EqualFold(strings.TrimSpace(raw), "Header") {
			return "header", "header"
		}
		if strings.EqualFold(strings.TrimSpace(raw), "Section Header") {
			return "heading", "section_header"
		}
		if strings.EqualFold(strings.TrimSpace(raw), "Title") {
			return "heading", "title"
		}
		return "heading", "heading"
	case "footer":
		return "footer", "footer"
	case "table":
		return "table", "table"
	case "figure", "image", "chart":
		return "figure", "figure"
	case "key value", "key_value":
		return "text", "key_value"
	case "list item", "list_item":
		return "list", "list_item"
	case "checkbox":
		return "text", "checkbox"
	default:
		return "text", strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), " ", "_"))
	}
}

func confidenceScore(confidence any, granular map[string]any) float64 {
	if granular != nil {
		if value, ok := asFloat(granular["parse_confidence"]); ok && value > 0 {
			return value
		}
	}
	if value, ok := confidence.(string); ok {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "high":
			return 0.9
		case "low":
			return 0.5
		}
	}
	if value, ok := asFloat(confidence); ok && value > 0 {
		return value
	}
	return 0.75
}

func bboxFromMap(m map[string]any) (*extractcommon.BBox, int) {
	page := pageFromBBox(m)
	if len(m) == 0 {
		return nil, page
	}
	left, lok := numberFromKeys(m, "left", "x0", "x_min", "xmin", "l")
	top, tok := numberFromKeys(m, "top", "y0", "y_min", "ymin", "t")
	right, rok := numberFromKeys(m, "right", "x1", "x_max", "xmax", "r")
	bottom, bok := numberFromKeys(m, "bottom", "y1", "y_max", "ymax", "b")
	if !rok {
		if width, ok := numberFromKeys(m, "width", "w"); ok {
			right = left + width
			rok = true
		}
	}
	if !bok {
		if height, ok := numberFromKeys(m, "height", "h"); ok {
			bottom = top + height
			bok = true
		}
	}
	if !(lok && tok && rok && bok) {
		return nil, page
	}
	left, top, right, bottom = normalizeCoord(left), normalizeCoord(top), normalizeCoord(right), normalizeCoord(bottom)
	if right <= left || bottom <= top {
		return nil, page
	}
	return &extractcommon.BBox{
		Left:   clamp01(left),
		Top:    clamp01(top),
		Right:  clamp01(right),
		Bottom: clamp01(bottom),
	}, page
}

func pageFromBBox(m map[string]any) int {
	v, ok := numberFromKeys(m, "page", "page_index", "pageIndex", "page_number", "pageNumber")
	if !ok {
		return 0
	}
	page := int(v)
	if page > 0 {
		return page - 1
	}
	return 0
}

func numberFromKeys(m map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			if n, ok := asFloat(v); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func normalizeCoord(v float64) float64 {
	if v > 1 && v <= 1000 {
		return v / 1000
	}
	return v
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func intFromMap(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	v, ok := asFloat(m[key])
	if !ok {
		return 0
	}
	return int(v)
}

func snippet(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) > 800 {
		return s[:800] + "..."
	}
	return s
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("reducto-%d", time.Now().UnixNano())
	}
	return "reducto-" + hex.EncodeToString(b[:])
}

package vlm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"

	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/inspectsvc"
	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

const (
	defaultTimeout   = 180 * time.Second
	defaultMaxTokens = 8192
	defaultPageBatch = 10
	defaultMaxConcur = 5
	bboxScale        = 1000.0
	queueTimeout     = 60 * time.Second
)

var (
	concurrencyOnce sync.Once
	concurrencySem  chan struct{}
)

type ChatCompletionRequest struct {
	Model          string
	UserContent    []map[string]any
	MaxTokens      int
	ResponseFormat string
}

type ChatCompletionResponse struct {
	Content      string
	Model        string
	FinishReason string
	PromptTokens int
}

type ChatCompletionFunc func(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)

type Client struct {
	chatCompletion    ChatCompletionFunc
	primaryModel      string
	fallbackModelName string
}

func New() *Client { return &Client{} }

func NewWithChatCompletion(primaryModel, fallbackModel string, completion ChatCompletionFunc) *Client {
	return &Client{
		chatCompletion:    completion,
		primaryModel:      strings.TrimSpace(primaryModel),
		fallbackModelName: strings.TrimSpace(fallbackModel),
	}
}

// ParseBytes parses common image formats into one coherent knowledge document.
func (c *Client) ParseBytes(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	_ = opts
	if !isImageFilename(filename) {
		return nil, fmt.Errorf("vlm: unsupported non-image input %q", filename)
	}
	doc, err := c.runImagePipeline(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	return extractcommon.EnrichStructuredOutput(doc), nil
}

func (c *Client) ParsePDFBytes(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	_ = opts
	doc, err := c.runPipeline(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	return extractcommon.EnrichStructuredOutput(doc), nil
}

func (c *Client) model() string {
	if c != nil && strings.TrimSpace(c.primaryModel) != "" {
		return strings.TrimSpace(c.primaryModel)
	}
	return model()
}

func (c *Client) fallbackModel() string {
	if c != nil && strings.TrimSpace(c.fallbackModelName) != "" {
		return strings.TrimSpace(c.fallbackModelName)
	}
	return fallbackModel()
}

func (c *Client) usesInjectedCompletion() bool {
	return c != nil && c.chatCompletion != nil
}

func apiKey() string {
	return firstEnv("VLM_API_KEY", "DASHSCOPE_API_KEY", "GEMINI_API_KEY")
}

func baseURL() string {
	if v := firstEnv("VLM_BASE_URL", "DASHSCOPE_BASE_URL", "GEMINI_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return ""
}

func requiredBaseURL() (string, error) {
	value := baseURL()
	if value == "" {
		return "", fmt.Errorf("missing VLM_BASE_URL (or DASHSCOPE_BASE_URL/GEMINI_BASE_URL)")
	}
	return value, nil
}

func model() string {
	return firstEnv("VLM_MODEL", "DASHSCOPE_VL_MODEL", "GEMINI_MODEL")
}

func fallbackModel() string {
	return firstEnv("VLM_MODEL_FAST", "DASHSCOPE_VL_MODEL_FAST", "GEMINI_FALLBACK_MODEL")
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := envconfig.String(key); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func timeout() time.Duration {
	if v := envconfig.String("GEMINI_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultTimeout
}

func maxTokens() int {
	if v := envconfig.String("GEMINI_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxTokens
}

func pageBatch() int {
	if v := envconfig.String("GEMINI_PAGE_BATCH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultPageBatch
}

func maxConcurrency() int {
	if v := envconfig.String("GEMINI_MAX_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultMaxConcur
}

func getSemaphore() chan struct{} {
	concurrencyOnce.Do(func() {
		concurrencySem = make(chan struct{}, maxConcurrency())
	})
	return concurrencySem
}

const pdfLayoutPrompt = `You are a PDF layout parser. Analyze this PDF and output all semantic blocks in reading order.
Preserve the source document language in extracted text.

Return a strict JSON object only, with this shape:
{
  "chunks": [
    {
      "type": "heading" | "text" | "figure" | "table" | "formula" | "marginalia",
      "subtype": "h1" | "h2" | "h3" | "",
      "page": <0-based page index>,
      "bbox": [ymin, xmin, ymax, xmax],
      "text": "<plain text>",
      "markdown": "<markdown / HTML / LaTeX>"
    }
  ]
}

Rules:
1. Normalize bbox coordinates to the 0..1000 range with top-left origin.
2. Use human reading order: top-to-bottom, with column-aware ordering.
3. Headers, footers, page numbers, and watermarks use type=marginalia.
4. Images and charts use type=figure; markdown may contain an alt-style description.
5. Tables use type=table and HTML <table> markdown.
6. Formulas use type=formula and LaTeX markdown ($...$ or $$...$$).
7. Headings should set subtype by level, such as h1/h2/h3.
8. Do not merge blocks of different types and do not drop visible content.
9. Output only the JSON object: no code fences and no explanation.`

const imageUnderstandingPrompt = `You are a high-accuracy image understanding, OCR, and knowledge-document reconstruction system.

Analyze the attached IMAGE as a whole. Convert all useful visible information into ONE coherent, faithful Markdown knowledge document for search and retrieval. Do not treat the image as a PDF page and do not emit dozens of isolated layout fragments.

Primary objectives, in priority order:
1. Faithfulness: never invent text, values, relationships, arrows, labels, or conclusions that are not visible.
2. Completeness: capture all legible text and all meaningful non-text information.
3. Structure: preserve layout relationships such as columns, rows, sections, labels, hierarchy, adjacency, arrows, and reading order.
4. Interpretation: explain the overall meaning and important relationships, while clearly separating interpretation from literal transcription.
5. Retrieval quality: produce natural, self-contained text with enough context for each fact to be found later.

Language and OCR rules:
- Preserve the source language exactly. Do not translate Chinese into English or English into Chinese.
- Preserve names, numbers, units, dates, floor numbers, codes, punctuation, and capitalization.
- When the image is multilingual, retain every visible language without replacing one with another.
- Mark genuinely unreadable text as [无法辨认] for primarily Chinese images or [unreadable] otherwise. Do not guess.
- Do not output image URLs, base64 data, image Markdown placeholders, or decorative glyphs that carry no meaning.

First identify the image type, then reconstruct it appropriately:
- Flowchart or process diagram: transcribe every visible node; identify start/end, arrows, branches, decision conditions, loops, and outcomes; then explain the complete flow step by step. Never infer an edge that is not visible.
- Table, directory, timetable, menu, or signboard: use Markdown tables or grouped lists. Keep each row/column association intact. For multi-column layouts, process each column independently before combining; never pair entries across unrelated rows or columns.
- Chart or dashboard: capture title, axes, units, legend, series, visible values, comparisons, and trends. Distinguish directly visible values from qualitative interpretation.
- UI screenshot or form: describe page purpose, sections, fields, controls, current values, statuses, warnings, and the visible interaction sequence.
- Architecture, network, map, or labeled diagram: list components, labels, containment, direction, and connections, followed by an overall explanation.
- General photograph or illustration: describe the scene, subjects, actions, spatial relationships, visible text, and the image's likely communicative purpose without speculating beyond evidence.

Markdown document requirements:
- Begin with a concise overall description of what the image is and communicates.
- Organize the detailed transcription according to the image's real structure, not arbitrary OCR boxes.
- Include a dedicated explanation for flows, relationships, or conclusions when the image contains them.
- Avoid tiny standalone fragments. A floor number, label, or value must remain attached to the item it describes.
- Avoid redundant repetition between transcription and interpretation unless repetition is needed for clarity.
- Use headings, lists, and Markdown tables when they improve fidelity and retrieval.

Return the complete knowledge document directly as Markdown. Do not wrap it in JSON or a code fence, and do not add commentary outside the document.`

func buildImageUnderstandingPrompt(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return imageUnderstandingPrompt
	}
	return imageUnderstandingPrompt + "\n\nSource image filename: " + filename + `.

Use the filename as auxiliary document context when it contains meaningful labels, identifiers, locations, dates, departments, building names, or other metadata. If the filename provides context that is not visible in the image, include it clearly as filename-derived context. Do not let the filename override visible image content. If filename context conflicts with the image, mention the conflict instead of silently resolving it.`
}

type contentPart struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	File     *contentFile     `json:"file,omitempty"`
	ImageURL *contentImageURL `json:"image_url,omitempty"`
}

type contentFile struct {
	Filename string `json:"filename"`
	FileData string `json:"file_data"`
}

type contentImageURL struct {
	URL string `json:"url"`
}

type message struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type respFormat struct {
	Type string `json:"type"`
}

type request struct {
	Model          string      `json:"model"`
	Messages       []message   `json:"messages"`
	MaxTokens      int         `json:"max_tokens,omitempty"`
	ResponseFormat *respFormat `json:"response_format,omitempty"`
}

type choice struct {
	FinishReason string `json:"finish_reason"`
	Message      struct {
		Content string `json:"content"`
	} `json:"message"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type parseResponse struct {
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
	Error   any      `json:"error,omitempty"`
}

type chunkItem struct {
	Type     string    `json:"type"`
	Subtype  string    `json:"subtype,omitempty"`
	Page     int       `json:"page"`
	Bbox     []float64 `json:"bbox,omitempty"`
	Text     string    `json:"text,omitempty"`
	Markdown string    `json:"markdown,omitempty"`
}

type chunksPayload struct {
	Chunks []chunkItem `json:"chunks"`
}

type batchResult struct {
	BatchIndex int
	PageOffset int
	Model      string
	Chunks     []chunkItem
}

func (c *Client) runPipeline(ctx context.Context, filename string, data []byte) (*extractcommon.DocumentResult, error) {
	if len(data) == 0 {
		return nil, errors.New("gemini: empty PDF bytes")
	}
	if !c.usesInjectedCompletion() && apiKey() == "" {
		return nil, errors.New("vlm: VLM_API_KEY/DASHSCOPE_API_KEY/GEMINI_API_KEY not set")
	}
	if c.model() == "" {
		return nil, errors.New("vlm: VLM_MODEL/DASHSCOPE_VL_MODEL/GEMINI_MODEL not set")
	}

	totalPages, splits, err := splitPDFByPages(data, pageBatch())
	if err != nil {
		return nil, fmt.Errorf("gemini: split PDF: %w", err)
	}
	if totalPages <= 0 {
		return nil, errors.New("gemini: PDF has 0 pages")
	}

	results := make([]*batchResult, len(splits))
	errs := make([]error, len(splits))
	var wg sync.WaitGroup
	for i, split := range splits {
		wg.Add(1)
		go func(idx int, sp splitPDF) {
			defer wg.Done()
			br, e := c.callParse(ctx, filename, sp.Data, idx, len(splits), sp.PageStart, sp.PageEnd)
			if e != nil {
				errs[idx] = e
				return
			}
			br.PageOffset = sp.PageStart
			br.BatchIndex = idx
			results[idx] = br
		}(i, split)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			return nil, fmt.Errorf("gemini: batch %d/%d failed: %w", i+1, len(splits), e)
		}
	}

	return toDocumentResult(filename, totalPages, results)
}

func (c *Client) runImagePipeline(ctx context.Context, filename string, data []byte) (*extractcommon.DocumentResult, error) {
	if len(data) == 0 {
		return nil, errors.New("gemini: empty image bytes")
	}
	if !c.usesInjectedCompletion() && apiKey() == "" {
		return nil, errors.New("vlm: VLM_API_KEY/DASHSCOPE_API_KEY/GEMINI_API_KEY not set")
	}
	if c.model() == "" {
		return nil, errors.New("vlm: VLM_MODEL/DASHSCOPE_VL_MODEL/GEMINI_MODEL not set")
	}
	br, err := c.callImageParse(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	return toDocumentResult(filename, 1, []*batchResult{br})
}

func (c *Client) callParse(ctx context.Context, filename string, pdfData []byte, batchIdx, batchTotal int, pageStart, pageEnd int) (*batchResult, error) {
	sem := getSemaphore()
	acquireCtx, cancel := context.WithTimeout(ctx, queueTimeout)
	defer cancel()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-acquireCtx.Done():
		return nil, fmt.Errorf("gemini: queue acquire timeout after %s (batch %d/%d)", queueTimeout, batchIdx+1, batchTotal)
	}

	pageDataURLs, renderEngine, err := inspectsvc.RenderPDFPagesToDataURLs(pdfData, pageEnd-pageStart+1)
	if err != nil {
		return nil, fmt.Errorf("gemini: render PDF pages for batch %d/%d: %w", batchIdx+1, batchTotal, err)
	}
	if len(pageDataURLs) == 0 {
		return nil, fmt.Errorf("gemini: render PDF pages for batch %d/%d returned no pages", batchIdx+1, batchTotal)
	}

	primary := c.model()
	fallback := c.fallbackModel()
	br, err := c.callOnce(ctx, filename, pageDataURLs, primary)
	if err == nil {
		br.Model = firstNonEmptyString(br.Model, primary)
		return br, nil
	}
	if isRetryable(err) && fallback != "" && fallback != primary {
		time.Sleep(2 * time.Second)
		br2, err2 := c.callOnce(ctx, filename, pageDataURLs, fallback)
		if err2 == nil {
			br2.Model = firstNonEmptyString(br2.Model, fallback)
			return br2, nil
		}
		return nil, fmt.Errorf("gemini: primary+fallback both failed after PDF render via %s: primary=%v fallback=%v", renderEngine, err, err2)
	}
	return nil, err
}

func (c *Client) callImageParse(ctx context.Context, filename string, imgData []byte) (*batchResult, error) {
	sem := getSemaphore()
	acquireCtx, cancel := context.WithTimeout(ctx, queueTimeout)
	defer cancel()
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-acquireCtx.Done():
		return nil, fmt.Errorf("gemini: queue acquire timeout after %s", queueTimeout)
	}

	primary := c.model()
	fallback := c.fallbackModel()
	br, err := c.callImageOnce(ctx, filename, imgData, primary)
	if err == nil {
		return br, nil
	}
	if isRetryable(err) && fallback != "" && fallback != primary {
		time.Sleep(2 * time.Second)
		br2, err2 := c.callImageOnce(ctx, filename, imgData, fallback)
		if err2 == nil {
			return br2, nil
		}
		return nil, fmt.Errorf("gemini: image primary+fallback both failed: primary=%v fallback=%v", err, err2)
	}
	return nil, err
}

type retryableErr struct{ inner error }

func (e *retryableErr) Error() string { return e.inner.Error() }
func (e *retryableErr) Unwrap() error { return e.inner }

func isRetryable(err error) bool {
	var r *retryableErr
	return errors.As(err, &r)
}

func (c *Client) callOnce(ctx context.Context, filename string, pageDataURLs []string, useModel string) (*batchResult, error) {
	userContent := buildPDFPageImageContent(filename, pageDataURLs)
	return c.callContentOnce(ctx, userContent, useModel, "gemini")
}

func buildPDFPageImageContent(filename string, pageDataURLs []string) []map[string]any {
	prompt := pdfLayoutPrompt
	if strings.TrimSpace(filename) != "" {
		prompt += "\n\nThe attached images are rendered pages from PDF file: " + strings.TrimSpace(filename) + "."
	}
	userContent := []map[string]any{{"type": "text", "text": prompt}}
	for _, dataURL := range pageDataURLs {
		dataURL = strings.TrimSpace(dataURL)
		if dataURL == "" {
			continue
		}
		userContent = append(userContent, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": dataURL,
			},
		})
	}
	return userContent
}

func (c *Client) callImageOnce(ctx context.Context, filename string, imgData []byte, useModel string) (*batchResult, error) {
	mime := imageMIME(filename)
	b64 := base64.StdEncoding.EncodeToString(imgData)
	dataURI := "data:" + mime + ";base64," + b64

	userContent := []map[string]any{
		{"type": "text", "text": buildImageUnderstandingPrompt(filename)},
		{
			"type": "image_url",
			"image_url": map[string]any{
				"url": dataURI,
			},
		},
	}
	content, modelUsed, finishReason, _, err := c.complete(ctx, userContent, useModel, "")
	if err != nil {
		return nil, err
	}
	if err := validateCompletionContent("gemini: image", content, finishReason); err != nil {
		return nil, err
	}
	if modelUsed == "" {
		modelUsed = useModel
	}
	return &batchResult{
		Model: modelUsed,
		Chunks: []chunkItem{
			{
				Type:     "text",
				Page:     0,
				Bbox:     []float64{0, 0, bboxScale, bboxScale},
				Text:     content,
				Markdown: content,
			},
		},
	}, nil
}

func (c *Client) callContentOnce(ctx context.Context, userContent []map[string]any, useModel string, label string) (*batchResult, error) {
	content, modelUsed, finishReason, promptTokens, err := c.complete(ctx, userContent, useModel, "json_object")
	if err != nil {
		return nil, err
	}
	if err := validateCompletionContent(label, content, finishReason); err != nil {
		return nil, err
	}

	chunks, cerr := parseChunks(content)
	if cerr != nil {
		snippet := content
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return nil, &retryableErr{inner: fmt.Errorf("%s: parse chunks JSON: %w (content=%s)", label, cerr, snippet)}
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("%s: model returned 0 chunks (prompt_tokens=%d)", label, promptTokens)
	}
	if modelUsed == "" {
		modelUsed = useModel
	}
	return &batchResult{Model: modelUsed, Chunks: chunks}, nil
}

func validateCompletionContent(label, content, finishReason string) error {
	if strings.EqualFold(finishReason, "length") {
		return fmt.Errorf("%s: finish_reason=length (max_tokens=%d exceeded)", label, maxTokens())
	}
	if strings.EqualFold(finishReason, "content_filter") || strings.EqualFold(finishReason, "safety") {
		return fmt.Errorf("%s: finish_reason=%s (content blocked by safety filter)", label, finishReason)
	}
	if content == "" {
		return &retryableErr{inner: fmt.Errorf("%s: empty message.content (finish_reason=%s)", label, finishReason)}
	}
	return nil
}

func (c *Client) complete(ctx context.Context, userContent []map[string]any, useModel, responseFormat string) (content string, modelUsed string, finishReason string, promptTokens int, err error) {
	if c != nil && c.chatCompletion != nil {
		resp, err := c.chatCompletion(ctx, ChatCompletionRequest{
			Model:          useModel,
			UserContent:    userContent,
			MaxTokens:      maxTokens(),
			ResponseFormat: responseFormat,
		})
		if err != nil {
			return "", useModel, "", 0, &retryableErr{inner: err}
		}
		if resp == nil {
			return "", useModel, "", 0, &retryableErr{inner: fmt.Errorf("vlm: empty chat completion response")}
		}
		return strings.TrimSpace(resp.Content), firstNonEmptyString(resp.Model, useModel), resp.FinishReason, resp.PromptTokens, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()

	base, err := requiredBaseURL()
	if err != nil {
		return "", useModel, "", 0, err
	}
	endpoint := base + "/chat/completions"
	reqBody := map[string]any{
		"model": useModel,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": userContent,
			},
		},
		"max_tokens": maxTokens(),
	}
	if responseFormat != "" {
		reqBody["response_format"] = map[string]any{"type": responseFormat}
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", useModel, "", 0, fmt.Errorf("gemini: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", useModel, "", 0, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey())

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", useModel, "", 0, &retryableErr{inner: fmt.Errorf("gemini: call %s: %w", endpoint, err)}
	}
	defer resp.Body.Close()

	respBytes, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return "", useModel, "", 0, &retryableErr{inner: fmt.Errorf("gemini: read response: %w", rerr)}
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(respBytes)
		if len(snippet) > 800 {
			snippet = snippet[:800] + "..."
		}
		httpErr := fmt.Errorf("gemini: HTTP %d from %s: %s", resp.StatusCode, endpoint, snippet)
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return "", useModel, "", 0, &retryableErr{inner: httpErr}
		}
		return "", useModel, "", 0, httpErr
	}

	var parsed parseResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		snippet := string(respBytes)
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
		}
		return "", useModel, "", 0, &retryableErr{inner: fmt.Errorf("gemini: decode response: %w (body=%s)", err, snippet)}
	}
	if len(parsed.Choices) == 0 {
		return "", useModel, "", 0, &retryableErr{inner: fmt.Errorf("gemini: empty choices (error=%v)", parsed.Error)}
	}
	choice := parsed.Choices[0]
	return strings.TrimSpace(choice.Message.Content), useModel, choice.FinishReason, parsed.Usage.PromptTokens, nil
}

var codeFenceRE = regexp.MustCompile("(?s)^```(?:json)?\\s*(.*?)\\s*```\\s*$")

func parseChunks(content string) ([]chunkItem, error) {
	trimmed := strings.TrimSpace(content)
	if m := codeFenceRE.FindStringSubmatch(trimmed); len(m) == 2 {
		trimmed = strings.TrimSpace(m[1])
	}

	var payload chunksPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil && len(payload.Chunks) > 0 {
		return payload.Chunks, nil
	}
	var arr []chunkItem
	if err := json.Unmarshal([]byte(trimmed), &arr); err == nil {
		return arr, nil
	}

	var dummy any
	if err := json.Unmarshal([]byte(trimmed), &dummy); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	return nil, fmt.Errorf("JSON does not match {\"chunks\":[]} or []")
}

func toDocumentResult(filename string, totalPages int, batches []*batchResult) (*extractcommon.DocumentResult, error) {
	sort.SliceStable(batches, func(i, j int) bool { return batches[i].BatchIndex < batches[j].BatchIndex })

	totalChunks := 0
	for _, b := range batches {
		if b != nil {
			totalChunks += len(b.Chunks)
		}
	}
	if totalChunks == 0 {
		return nil, errors.New("gemini: no chunks returned across all batches")
	}

	pages := make([]extractcommon.Page, totalPages)
	for i := 0; i < totalPages; i++ {
		pages[i] = extractcommon.Page{PageIndex: i}
	}

	out := make([]extractcommon.Chunk, 0, totalChunks)
	mdParts := make([]string, 0, totalChunks)
	modelUsed := ""
	for _, br := range batches {
		if br == nil {
			continue
		}
		if modelUsed == "" && br.Model != "" {
			modelUsed = br.Model
		}
		for _, ch := range br.Chunks {
			page := ch.Page + br.PageOffset
			if page < 0 {
				page = 0
			}
			if totalPages > 0 && page >= totalPages {
				page = totalPages - 1
			}
			ordinal := len(out) + 1
			dto := extractcommon.Chunk{
				ID:        fmt.Sprintf("gemini-%d", ordinal-1),
				Type:      normalizeType(ch.Type),
				Subtype:   strings.TrimSpace(ch.Subtype),
				Page:      page,
				Ordinal:   ordinal,
				Precision: "reliable",
				BBox:      bboxFromYXYX(ch.Bbox),
				Text:      strings.TrimSpace(ch.Text),
				Markdown:  strings.TrimSpace(ch.Markdown),
			}
			if dto.Text == "" {
				dto.Text = dto.Markdown
			}
			if dto.Markdown == "" {
				dto.Markdown = dto.Text
			}
			out = append(out, dto)
			if dto.Markdown != "" {
				mdParts = append(mdParts, dto.Markdown)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("gemini: all chunks dropped during normalization")
	}
	if modelUsed == "" {
		modelUsed = model()
	}

	return &extractcommon.DocumentResult{
		DocID:     newID(),
		FileName:  filename,
		PageCount: totalPages,
		Pages:     pages,
		Chunks:    out,
		Markdown:  strings.Join(mdParts, "\n\n"),
		Source:    "vlm:" + modelUsed,
	}, nil
}

var typeWhitelist = map[string]struct{}{
	"heading": {}, "text": {}, "figure": {}, "table": {}, "formula": {}, "marginalia": {},
}

func normalizeType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if _, ok := typeWhitelist[t]; ok {
		return t
	}
	return "text"
}

func bboxFromYXYX(b []float64) *extractcommon.BBox {
	if len(b) != 4 {
		return nil
	}
	ymin, xmin, ymax, xmax := b[0], b[1], b[2], b[3]
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > bboxScale {
			return bboxScale
		}
		return v
	}
	ymin, xmin, ymax, xmax = clamp(ymin), clamp(xmin), clamp(ymax), clamp(xmax)
	if ymin > ymax {
		ymin, ymax = ymax, ymin
	}
	if xmin > xmax {
		xmin, xmax = xmax, xmin
	}
	if ymax-ymin <= 0 || xmax-xmin <= 0 {
		return nil
	}
	return &extractcommon.BBox{Left: xmin / bboxScale, Top: ymin / bboxScale, Right: xmax / bboxScale, Bottom: ymax / bboxScale}
}

type splitPDF struct {
	Data      []byte
	PageStart int
	PageEnd   int
}

func splitPDFByPages(data []byte, batchSize int) (int, []splitPDF, error) {
	if batchSize <= 0 {
		batchSize = defaultPageBatch
	}
	conf := pdfmodel.NewDefaultConfiguration()
	conf.ValidationMode = pdfmodel.ValidationRelaxed

	totalPages, err := pdfapi.PageCount(bytes.NewReader(data), conf)
	if err != nil {
		return 0, nil, fmt.Errorf("page count: %w", err)
	}
	if totalPages <= 0 {
		return 0, nil, errors.New("PDF has 0 pages")
	}
	if totalPages <= batchSize {
		return totalPages, []splitPDF{{Data: data, PageStart: 0, PageEnd: totalPages - 1}}, nil
	}

	var out []splitPDF
	for start := 0; start < totalPages; start += batchSize {
		end := start + batchSize - 1
		if end >= totalPages {
			end = totalPages - 1
		}
		selector := fmt.Sprintf("%d-%d", start+1, end+1)
		buf := &bytes.Buffer{}
		confBatch := pdfmodel.NewDefaultConfiguration()
		confBatch.ValidationMode = pdfmodel.ValidationRelaxed
		if err := pdfapi.Trim(bytes.NewReader(data), buf, []string{selector}, confBatch); err != nil {
			return totalPages, nil, fmt.Errorf("trim pages %s: %w", selector, err)
		}
		out = append(out, splitPDF{Data: append([]byte(nil), buf.Bytes()...), PageStart: start, PageEnd: end})
	}
	return totalPages, out, nil
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}

func isImageFilename(filename string) bool {
	ext := strings.ToLower(strings.TrimSpace(filename))
	return strings.HasSuffix(ext, ".png") ||
		strings.HasSuffix(ext, ".jpg") ||
		strings.HasSuffix(ext, ".jpeg") ||
		strings.HasSuffix(ext, ".webp") ||
		strings.HasSuffix(ext, ".tif") ||
		strings.HasSuffix(ext, ".tiff")
}

func imageMIME(filename string) string {
	ext := strings.ToLower(strings.TrimSpace(filename))
	switch {
	case strings.HasSuffix(ext, ".png"):
		return "image/png"
	case strings.HasSuffix(ext, ".webp"):
		return "image/webp"
	case strings.HasSuffix(ext, ".tif"), strings.HasSuffix(ext, ".tiff"):
		return "image/tiff"
	default:
		return "image/jpeg"
	}
}

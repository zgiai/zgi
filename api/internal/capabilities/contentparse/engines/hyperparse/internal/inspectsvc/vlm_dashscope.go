package inspectsvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

var dashscopeHTTPClient = &http.Client{Timeout: 180 * time.Second}

// VLMFallbackChunkJSONInstruction is used when native rules are not enough and
// page images need a structured VLM fallback.
const VLMFallbackChunkJSONInstruction = `You are a document structure extraction assistant. This call is used as a fallback when native parsing rules do not extract enough content.
Extract content into chunks that are close to native chunk semantics: paragraphs, headings, lists, tables, images, formulas, and key-value data. Output exactly one valid JSON object, with no Markdown fences and no comments.

Top-level format: {"chunks":[ ... ]}

Each object in chunks must include:
- "type": one of paragraph | heading | table | list_item | image | formula | kv | annotation | other
- "page_index": integer, starting from 1 for the input page image
- "text": chunk text
  - table: prefer payload with row_count, column_count, and cells; otherwise use a GFM Markdown pipe table. The server normalizes tables to native-compatible HTML with ids.
  - formula: use "formula:<expression>|<description>", for example "formula:E=mc^2|mass-energy equivalence".
  - image: include an image description, preferably in "<<:figure: ...::>" style with visible text and visual description.
- "order": optional integer for reading order within the page
- "confidence": optional number from 0 to 1
- "source_trace": optional string, for example "vlm:page1"
- "payload": optional object, for example {"row_count":n,"column_count":m,"cells":[...]}
- "bbox": optional normalized page coordinates {"left","right","top","bottom"} in [0,1]

Important constraints:
1) Choose the type from visual evidence, especially for image, table, and formula chunks; do not label image regions as ordinary paragraphs.
2) If the same page contains text and images, keep them as separate chunks instead of merging them into one large paragraph.
3) Keep reading order stable, top-to-bottom and left-to-right.
4) Use type=image only for local images, illustrations, logos, photos, or chart regions. Do not mark a whole text page or large body text region as image.
5) Provide bbox for image chunks when possible. If bbox is unavailable, be strict that the chunk is really a local image region.
6) For handwritten notes, teacher edits, circled text, or margin annotations, output a separate type=annotation chunk and preserve the original text as much as possible.

The output must be complete parseable UTF-8 JSON.
7) The text field must be readable in the source document language or valid Unicode. Do not output mojibake, replacement characters, or meaningless symbol sequences. If content is unclear, prefer an empty string or "unrecognized" over hallucinated text.`

// VLMImageContentFromDataURLs converts page data URLs to image_url content parts.
func VLMImageContentFromDataURLs(pageDataURLs []string) []map[string]any {
	out := make([]map[string]any, 0, len(pageDataURLs))
	for _, dataURL := range pageDataURLs {
		out = append(out, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": dataURL,
			},
		})
	}
	return out
}

func dashscopeChatCompletion(userContent []map[string]any) (reply string, model string, err error) {
	return dashscopeChatCompletionContext(context.Background(), userContent)
}

func dashscopeChatCompletionWithModel(userContent []map[string]any, preferredModel string) (reply string, model string, err error) {
	return dashscopeChatCompletionWithModelContext(context.Background(), userContent, preferredModel)
}

func dashscopeChatCompletionContext(ctx context.Context, userContent []map[string]any) (reply string, model string, err error) {
	return dashscopeChatCompletionWithModelContext(ctx, userContent, "")
}

func dashscopeChatCompletionWithModelContext(ctx context.Context, userContent []map[string]any, preferredModel string) (reply string, model string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	apiKey := VLMAPIKey()
	if apiKey == "" {
		return "", "", fmt.Errorf("missing %s (or %s/%s)", EnvVLMAPIKey, EnvLegacyAPIKey, EnvGeminiAPIKey)
	}
	baseURL := VLMBaseURL()
	if baseURL == "" {
		return "", "", fmt.Errorf("missing %s (or %s/%s)", EnvVLMBaseURL, EnvLegacyBaseURL, EnvGeminiBaseURL)
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	model = dashscopeModelForRequest(preferredModel)
	if strings.TrimSpace(model) == "" {
		return "", "", fmt.Errorf("missing %s (or %s/%s)", EnvVLMModel, EnvLegacyModel, EnvGeminiModel)
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{
				"role":    "user",
				"content": userContent,
			},
		},
		"temperature": 0,
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", model, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := dashscopeHTTPClient.Do(req)
	if err != nil {
		return "", model, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", model, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", model, err
	}
	choices, _ := raw["choices"].([]any)
	if len(choices) == 0 {
		return "", model, fmt.Errorf("empty choices")
	}
	c0, _ := choices[0].(map[string]any)
	msg, _ := c0["message"].(map[string]any)
	if msg == nil {
		return "", model, fmt.Errorf("missing message")
	}
	switch content := msg["content"].(type) {
	case string:
		return content, model, nil
	case []any:
		var sb strings.Builder
		for _, it := range content {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			tp := fmt.Sprintf("%v", m["type"])
			if tp == "text" || tp == "output_text" || tp == "input_text" {
				sb.WriteString(fmt.Sprintf("%v\n", m["text"]))
			}
		}
		return strings.TrimSpace(sb.String()), model, nil
	default:
		return fmt.Sprintf("%v", msg["content"]), model, nil
	}
}

func dashscopeModelForRequest(preferredModel string) string {
	if model := strings.TrimSpace(preferredModel); model != "" {
		return model
	}
	model := VLMModel()
	if model != "" {
		return model
	}
	return ""
}

// CallDashscopeVLM extracts page-level OCR-style plain text for quality comparisons.
func CallDashscopeVLM(images []map[string]any) (string, string, error) {
	return CallDashscopeVLMContext(context.Background(), images)
}

func CallDashscopeVLMContext(ctx context.Context, images []map[string]any) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parts := []map[string]any{
		{
			"type": "text",
			"text": "Recognize the text in the image line by line and output plain text in the original reading order. Do not summarize.",
		},
	}
	parts = append(parts, images...)
	return dashscopeChatCompletionContext(ctx, parts)
}

// CallDashscopeVLMFallbackStructured returns structured chunk JSON when native parsing is insufficient.
func CallDashscopeVLMFallbackStructured(images []map[string]any) (model string, chunks []map[string]any, raw string, err error) {
	model, chunks, raw, _, err = CallDashscopeVLMFallbackStructuredProfiledContext(context.Background(), images)
	return model, chunks, raw, err
}

func CallDashscopeVLMFallbackStructuredProfiled(images []map[string]any) (model string, chunks []map[string]any, raw string, timing VLMCallTimingBreakdown, err error) {
	return CallDashscopeVLMFallbackStructuredProfiledContext(context.Background(), images)
}

func CallDashscopeVLMFallbackStructuredContext(ctx context.Context, images []map[string]any) (model string, chunks []map[string]any, raw string, err error) {
	model, chunks, raw, _, err = CallDashscopeVLMFallbackStructuredProfiledContext(ctx, images)
	return model, chunks, raw, err
}

func CallDashscopeVLMFallbackStructuredProfiledContext(ctx context.Context, images []map[string]any) (model string, chunks []map[string]any, raw string, timing VLMCallTimingBreakdown, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parts := []map[string]any{
		{"type": "text", "text": VLMFallbackChunkJSONInstruction},
	}
	parts = append(parts, images...)
	log.Printf("[ui.vlm.fallback] dashscope chat/completions start image_parts=%d", len(images))
	t0 := time.Now()
	raw, model, err = dashscopeChatCompletionContext(ctx, parts)
	elapsed := time.Since(t0).Milliseconds()
	timing.MainRequestMs = elapsed
	if err != nil {
		log.Printf("[ui.vlm.fallback] dashscope done elapsed_ms=%d err=%v", elapsed, err)
		return model, nil, "", timing, err
	}
	log.Printf("[ui.vlm.fallback] dashscope done elapsed_ms=%d model=%q reply_len=%d", elapsed, model, len(raw))
	chunks, perr := pdforchestrator.ParseVLMChunksJSON(raw)
	if perr != nil || len(chunks) == 0 {
		log.Printf("[ui.vlm.fallback] parse JSON chunks failed or empty: %v (parsed_len=%d) -> strict_retry", perr, len(chunks))
		retryModel, retryChunks, retryRaw, retryMs, retryErr := callDashscopeVLMFallbackStructuredStrictContext(ctx, images)
		timing.StrictRetryMs = retryMs
		timing.StrictRetryUsed = true
		if retryErr == nil && len(retryChunks) > 0 {
			if strings.TrimSpace(retryModel) != "" {
				model = retryModel
			}
			log.Printf("[ui.vlm.fallback] strict_retry success chunks=%d", len(retryChunks))
			return model, retryChunks, retryRaw, timing, nil
		}
		log.Printf("[ui.vlm.fallback] strict_retry failed: %v -> fallback_single", retryErr)
		fb := pdforchestrator.VLMChunksFallbackSingle(raw, len(images))
		timing.FallbackSingleUsed = true
		if fb == nil {
			log.Printf("[ui.vlm.fallback] fallback_single produced no chunks (empty reply?)")
		} else {
			log.Printf("[ui.vlm.fallback] fallback_single chunks=%d", len(fb))
		}
		return model, fb, raw, timing, nil
	}
	log.Printf("[ui.vlm.fallback] parse JSON chunks ok count=%d", len(chunks))
	return model, chunks, raw, timing, nil
}

func callDashscopeVLMFallbackStructuredStrict(images []map[string]any) (model string, chunks []map[string]any, raw string, elapsedMs int64, err error) {
	return callDashscopeVLMFallbackStructuredStrictContext(context.Background(), images)
}

func callDashscopeVLMFallbackStructuredStrictContext(ctx context.Context, images []map[string]any) (model string, chunks []map[string]any, raw string, elapsedMs int64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	strictInstruction := VLMFallbackChunkJSONInstruction + `

Strict constraints:
1) Output only one complete JSON object: {"chunks":[...]}
2) Do not output explanations, prefixes, suffixes, code fences, or comments.
3) If unsure, still return valid JSON; an empty array {"chunks":[]} is allowed.`
	parts := []map[string]any{
		{"type": "text", "text": strictInstruction},
	}
	parts = append(parts, images...)
	log.Printf("[ui.vlm.fallback] strict_retry start image_parts=%d", len(images))
	t0 := time.Now()
	raw, model, err = dashscopeChatCompletionContext(ctx, parts)
	elapsed := time.Since(t0).Milliseconds()
	elapsedMs = elapsed
	if err != nil {
		log.Printf("[ui.vlm.fallback] strict_retry done elapsed_ms=%d err=%v", elapsed, err)
		return model, nil, "", elapsedMs, err
	}
	log.Printf("[ui.vlm.fallback] strict_retry done elapsed_ms=%d model=%q reply_len=%d", elapsed, model, len(raw))
	chunks, perr := pdforchestrator.ParseVLMChunksJSON(raw)
	if perr != nil || len(chunks) == 0 {
		return model, nil, raw, elapsedMs, fmt.Errorf("strict parse failed: %v", perr)
	}
	return model, chunks, raw, elapsedMs, nil
}

// CallDashscopeVLMFallbackStructuredBatched runs structured fallback by batch and merges order.
func CallDashscopeVLMFallbackStructuredBatched(images []map[string]any, pageNumbers []int) (model string, chunks []map[string]any, raw string, err error) {
	model, chunks, raw, _, err = CallDashscopeVLMFallbackStructuredBatchedProfiledContext(context.Background(), images, pageNumbers)
	return model, chunks, raw, err
}

func CallDashscopeVLMFallbackStructuredBatchedProfiled(images []map[string]any, pageNumbers []int) (model string, chunks []map[string]any, raw string, timing VLMCallTimingBreakdown, err error) {
	return CallDashscopeVLMFallbackStructuredBatchedProfiledContext(context.Background(), images, pageNumbers)
}

func CallDashscopeVLMFallbackStructuredBatchedContext(ctx context.Context, images []map[string]any, pageNumbers []int) (model string, chunks []map[string]any, raw string, err error) {
	model, chunks, raw, _, err = CallDashscopeVLMFallbackStructuredBatchedProfiledContext(ctx, images, pageNumbers)
	return model, chunks, raw, err
}

func CallDashscopeVLMFallbackStructuredBatchedProfiledContext(ctx context.Context, images []map[string]any, pageNumbers []int) (model string, chunks []map[string]any, raw string, timing VLMCallTimingBreakdown, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(images) == 0 {
		return "", nil, "", timing, nil
	}
	batchSize := VLMFallbackBatchPages()
	if batchSize <= 0 || batchSize >= len(images) {
		model, chunks, raw, timing, err = CallDashscopeVLMFallbackStructuredProfiledContext(ctx, images)
		if err == nil {
			RemapVLMChunkPages(chunks, pageNumbers)
		}
		return model, chunks, raw, timing, err
	}
	concurrency := VLMFallbackConcurrency()
	type batchRes struct {
		idx    int
		model  string
		chunks []map[string]any
		raw    string
		timing VLMCallTimingBreakdown
		err    error
	}
	batches := make([][]map[string]any, 0, (len(images)+batchSize-1)/batchSize)
	for i := 0; i < len(images); i += batchSize {
		j := i + batchSize
		if j > len(images) {
			j = len(images)
		}
		batches = append(batches, images[i:j])
	}
	log.Printf("[ui.vlm.fallback] batched start pages=%d batch_pages=%d batch_count=%d conc=%d", len(images), batchSize, len(batches), concurrency)
	timing.BatchedCalls = len(batches)
	results := make([]batchRes, len(batches))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range batches {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = batchRes{idx: i, err: ctx.Err()}
				return
			}
			defer func() { <-sem }()
			m, cs, rw, tt, e := CallDashscopeVLMFallbackStructuredProfiledContext(ctx, batches[i])
			if e == nil && len(pageNumbers) == len(images) {
				start := i * batchSize
				end := start + len(batches[i])
				RemapVLMChunkPages(cs, pageNumbers[start:end])
			}
			results[i] = batchRes{idx: i, model: m, chunks: cs, raw: rw, timing: tt, err: e}
		}()
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].idx < results[j].idx })
	firstModel := ""
	merged := make([]map[string]any, 0, len(images)*2)
	rawParts := make([]string, 0, len(results))
	orderOffset := 0
	var errs []string
	for _, r := range results {
		timing.Merge(r.timing)
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("batch#%d: %v", r.idx, r.err))
			continue
		}
		if firstModel == "" && strings.TrimSpace(r.model) != "" {
			firstModel = r.model
		}
		if strings.TrimSpace(r.raw) != "" {
			rawParts = append(rawParts, r.raw)
		}
		for _, c := range r.chunks {
			if c == nil {
				continue
			}
			if ov, ok := c["order"]; ok {
				switch v := ov.(type) {
				case int:
					c["order"] = v + orderOffset
				case float64:
					c["order"] = int(v) + orderOffset
				}
			}
			merged = append(merged, c)
		}
		orderOffset += 10000
	}
	if len(merged) == 0 && len(errs) > 0 {
		return firstModel, nil, strings.Join(rawParts, "\n\n"), timing, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	if len(errs) > 0 {
		log.Printf("[ui.vlm.fallback] batched partial errors: %s", strings.Join(errs, "; "))
	}
	log.Printf("[ui.vlm.fallback] batched done merged_chunks=%d", len(merged))
	return firstModel, merged, strings.Join(rawParts, "\n\n"), timing, nil
}

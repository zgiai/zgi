package mineru

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"
)

const (
	defaultBaseURL         = "http://127.0.0.1:18091"
	defaultOfficialBaseURL = "https://mineru.net"
	defaultTimeout         = 800 * time.Second
	defaultPollInterval    = 3 * time.Second
	bboxScale              = 1000.0
)

var (
	mineruMarkdownImagePattern = regexp.MustCompile(`!\[([^\]\r\n]*)\]\(([^)\r\n]+)\)`)
	mineruHTMLImageSrcPattern  = regexp.MustCompile(`(?i)(<img\b[^>]*\bsrc\s*=\s*)(["']?)([^"'\s>]+)(["']?)`)
)

type Client struct{}

func New() *Client { return &Client{} }

// ParseBytes calls MinerU through either the sidecar(file_parse) mode or the official API.
func (c *Client) ParseBytes(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	_ = opts
	resp, err := callMineruParse(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	doc, err := mineruToDocumentResult(filename, resp)
	if err != nil {
		return nil, err
	}
	return extractcommon.EnrichStructuredOutput(doc), nil
}

// ParsePDFBytes is a compatibility entry point that reuses ParseBytes.
func (c *Client) ParsePDFBytes(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	return c.ParseBytes(ctx, filename, data, opts)
}

func Configured() bool {
	if mineruMode() == "official" {
		return envconfig.String("MINERU_OFFICIAL_TOKEN") != ""
	}
	return envconfig.String("MINERU_API_URL") != ""
}

func Mode() string {
	return mineruMode()
}

func ConfiguredBaseURL() string {
	if mineruMode() == "official" {
		return officialBaseURL()
	}
	return envconfig.String("MINERU_API_URL")
}

func APIKeyEnv() string {
	if mineruMode() == "official" {
		return "MINERU_OFFICIAL_TOKEN"
	}
	return "MINERU_API_TOKEN"
}

func TimeoutSeconds() int {
	if mineruMode() == "official" {
		if v := secondsFromEnv("MINERU_OFFICIAL_TIMEOUT_SECONDS"); v > 0 {
			return v
		}
	}
	return secondsFromEnv("MINERU_TIMEOUT_SECONDS")
}

func mineruMode() string {
	mode := strings.ToLower(envconfig.String("MINERU_MODE"))
	if mode == "" {
		return "sidecar"
	}
	return mode
}

func baseURL() string {
	if v := envconfig.String("MINERU_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultBaseURL
}

func timeout() time.Duration {
	if v := secondsFromEnv("MINERU_TIMEOUT_SECONDS"); v > 0 {
		return time.Duration(v) * time.Second
	}
	return defaultTimeout
}

func officialBaseURL() string {
	if v := envconfig.String("MINERU_OFFICIAL_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultOfficialBaseURL
}

func officialTimeout() time.Duration {
	if v := secondsFromEnv("MINERU_OFFICIAL_TIMEOUT_SECONDS"); v > 0 {
		return time.Duration(v) * time.Second
	}
	return timeout()
}

func officialPollInterval() time.Duration {
	if v := secondsFromEnv("MINERU_OFFICIAL_POLL_INTERVAL_SECONDS"); v > 0 {
		return time.Duration(v) * time.Second
	}
	return defaultPollInterval
}

func secondsFromEnv(key string) int {
	if v := envconfig.String(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

type parseResponse struct {
	TaskID  string                 `json:"task_id"`
	Status  string                 `json:"status"`
	Backend string                 `json:"backend"`
	Results map[string]fileResults `json:"results"`
	Error   any                    `json:"error,omitempty"`
}

type fileResults struct {
	MdContent   string            `json:"md_content,omitempty"`
	MiddleJSON  string            `json:"middle_json,omitempty"`
	ContentList string            `json:"content_list,omitempty"`
	Images      map[string]string `json:"images,omitempty"`
}

type contentItem struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	TextLevel int    `json:"text_level,omitempty"`
	BBox      []int  `json:"bbox,omitempty"`
	PageIdx   int    `json:"page_idx"`
	ImgPath   string `json:"img_path,omitempty"`

	TableBody     string `json:"table_body,omitempty"`
	TableCaption  any    `json:"table_caption,omitempty"`
	TableFootnote any    `json:"table_footnote,omitempty"`
	ImageCaption  any    `json:"image_caption,omitempty"`
	ImageFootnote any    `json:"image_footnote,omitempty"`
	ChartCaption  any    `json:"chart_caption,omitempty"`
	ChartFootnote any    `json:"chart_footnote,omitempty"`
}

type contentItemV2 struct {
	Type    string `json:"type"`
	SubType string `json:"sub_type,omitempty"`
	Content any    `json:"content,omitempty"`
	BBox    []int  `json:"bbox,omitempty"`
	Anchor  string `json:"anchor,omitempty"`
}

type middleJSON struct {
	PdfInfo []middlePage `json:"pdf_info"`
}

type middlePage struct {
	PageIdx       int                  `json:"page_idx"`
	PageSize      []int                `json:"page_size"`
	PreprocBlocks []middlePreprocBlock `json:"preproc_blocks,omitempty"`
}

type middlePreprocBlock struct {
	Score *float64 `json:"score,omitempty"`
	BBox  []int    `json:"bbox,omitempty"`
	Index int      `json:"index,omitempty"`
	Type  string   `json:"type,omitempty"`
}

func callMineruParse(ctx context.Context, filename string, data []byte) (*parseResponse, error) {
	if mineruMode() == "official" {
		return callOfficialMineruParse(ctx, filename, data)
	}

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)

	fw, err := mw.CreateFormFile("files", filename)
	if err != nil {
		return nil, fmt.Errorf("mineru: create form file: %w", err)
	}
	if _, err := fw.Write(data); err != nil {
		return nil, fmt.Errorf("mineru: write file bytes: %w", err)
	}

	for k, v := range map[string]string{
		"backend":             "pipeline",
		"parse_method":        "auto",
		"table_enable":        "true",
		"formula_enable":      "true",
		"return_md":           "true",
		"return_content_list": "true",
		"return_middle_json":  "true",
		"return_model_output": "false",
		"return_images":       "true",
		"response_format_zip": "false",
	} {
		if err := mw.WriteField(k, v); err != nil {
			return nil, fmt.Errorf("mineru: write field %s: %w", k, err)
		}
	}
	if err := mw.WriteField("lang_list", detectLang(filename)); err != nil {
		return nil, fmt.Errorf("mineru: write lang_list: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("mineru: close multipart: %w", err)
	}

	url := baseURL() + "/file_parse"
	reqCtx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("mineru: new request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mineru: call %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mineru: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(respBody)
		if len(snippet) > 800 {
			snippet = snippet[:800] + "..."
		}
		return nil, fmt.Errorf("mineru: HTTP %d from %s: %s", resp.StatusCode, url, snippet)
	}

	var parsed parseResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("mineru: decode response: %w", err)
	}
	if parsed.Status != "" && parsed.Status != "completed" {
		return nil, fmt.Errorf("mineru: task status=%q error=%v", parsed.Status, parsed.Error)
	}
	if len(parsed.Results) == 0 {
		return nil, fmt.Errorf("mineru: empty results")
	}
	return &parsed, nil
}

type officialBatchRequest struct {
	Files         []officialBatchFile `json:"files"`
	ModelVersion  string              `json:"model_version,omitempty"`
	EnableFormula bool                `json:"enable_formula"`
	EnableTable   bool                `json:"enable_table"`
	Language      string              `json:"language,omitempty"`
}

type officialBatchFile struct {
	Name       string `json:"name"`
	DataID     string `json:"data_id,omitempty"`
	IsOCR      bool   `json:"is_ocr"`
	PageRanges string `json:"page_ranges,omitempty"`
}

type officialResponse[T any] struct {
	Code    officialCode `json:"code"`
	Msg     string       `json:"msg"`
	TraceID string       `json:"trace_id"`
	Data    T            `json:"data"`
}

type officialCode string

func (c *officialCode) UnmarshalJSON(data []byte) error {
	var number int
	if err := json.Unmarshal(data, &number); err == nil {
		*c = officialCode(strconv.Itoa(number))
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	*c = officialCode(text)
	return nil
}

func (c officialCode) OK() bool {
	return c == "" || c == "0"
}

type officialFileURLData struct {
	BatchID  string   `json:"batch_id"`
	FileURLs []string `json:"file_urls"`
}

type officialBatchResultData struct {
	BatchID        string                  `json:"batch_id"`
	ExtractResults []officialExtractResult `json:"extract_result"`
}

type officialExtractResult struct {
	FileName   string         `json:"file_name"`
	State      string         `json:"state"`
	ErrMsg     string         `json:"err_msg"`
	FullZipURL string         `json:"full_zip_url"`
	DataID     string         `json:"data_id"`
	Progress   map[string]any `json:"extract_progress,omitempty"`
}

type officialZipArtifacts struct {
	Markdown        string
	MiddleJSON      string
	ContentList     string
	Images          map[string]string
	MarkdownPath    string
	MiddleJSONPath  string
	ContentListPath string
}

func callOfficialMineruParse(ctx context.Context, filename string, data []byte) (*parseResponse, error) {
	token := envconfig.String("MINERU_OFFICIAL_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("mineru official: MINERU_OFFICIAL_TOKEN is required")
	}

	reqCtx, cancel := context.WithTimeout(ctx, officialTimeout())
	defer cancel()

	client := &http.Client{}
	dataID := "zgi_" + newID()
	applyData, err := officialApplyUploadURL(reqCtx, client, token, filename, dataID)
	if err != nil {
		return nil, err
	}
	if applyData.BatchID == "" {
		return nil, fmt.Errorf("mineru official: empty batch_id")
	}
	if len(applyData.FileURLs) == 0 || strings.TrimSpace(applyData.FileURLs[0]) == "" {
		return nil, fmt.Errorf("mineru official: empty upload URL for batch %s", applyData.BatchID)
	}
	if err := officialUploadFile(reqCtx, client, applyData.FileURLs[0], data); err != nil {
		return nil, err
	}

	result, err := officialPollResult(reqCtx, client, token, applyData.BatchID, dataID)
	if err != nil {
		return nil, err
	}
	artifacts, err := officialDownloadAndReadZip(reqCtx, client, result.FullZipURL)
	if err != nil {
		return nil, err
	}
	if artifacts.ContentList == "" {
		artifacts.ContentList = syntheticContentListFromMarkdown(artifacts.Markdown)
	}
	if artifacts.ContentList == "" {
		return nil, fmt.Errorf("mineru official: result zip missing content_list.json and full.md")
	}

	return &parseResponse{
		TaskID:  coalesce(result.DataID, applyData.BatchID),
		Status:  "completed",
		Backend: "official:" + officialModelVersion(filename),
		Results: map[string]fileResults{
			coalesce(result.FileName, filename): {
				MdContent:   artifacts.Markdown,
				MiddleJSON:  artifacts.MiddleJSON,
				ContentList: artifacts.ContentList,
				Images:      artifacts.Images,
			},
		},
	}, nil
}

func officialApplyUploadURL(ctx context.Context, client *http.Client, token, filename, dataID string) (*officialFileURLData, error) {
	payload := officialBatchRequest{
		Files: []officialBatchFile{
			{
				Name:   filename,
				DataID: dataID,
				IsOCR:  true,
			},
		},
		ModelVersion:  officialModelVersion(filename),
		EnableFormula: true,
		EnableTable:   true,
		Language:      detectLang(filename),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("mineru official: encode upload-url request: %w", err)
	}

	var parsed officialResponse[officialFileURLData]
	if err := officialJSON(ctx, client, http.MethodPost, officialBaseURL()+"/api/v4/file-urls/batch", token, body, &parsed); err != nil {
		return nil, err
	}
	if !parsed.Code.OK() {
		return nil, fmt.Errorf("mineru official: apply upload URL failed code=%s msg=%s trace_id=%s", parsed.Code, parsed.Msg, parsed.TraceID)
	}
	return &parsed.Data, nil
}

func officialUploadFile(ctx context.Context, client *http.Client, uploadURL string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("mineru official: new upload request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mineru official: upload file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := readSnippet(resp.Body)
		return fmt.Errorf("mineru official: upload HTTP %d: %s", resp.StatusCode, snippet)
	}
	return nil
}

func officialPollResult(ctx context.Context, client *http.Client, token, batchID, dataID string) (*officialExtractResult, error) {
	ticker := time.NewTicker(officialPollInterval())
	defer ticker.Stop()

	for {
		result, err := officialGetBatchResult(ctx, client, token, batchID, dataID)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(result.State)) {
		case "done":
			if strings.TrimSpace(result.FullZipURL) == "" {
				return nil, fmt.Errorf("mineru official: done result missing full_zip_url")
			}
			return result, nil
		case "failed":
			return nil, fmt.Errorf("mineru official: parse failed: %s", result.ErrMsg)
		case "", "waiting-file", "pending", "running", "converting":
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("mineru official: wait batch %s: %w", batchID, ctx.Err())
			case <-ticker.C:
				continue
			}
		default:
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("mineru official: wait batch %s state=%q: %w", batchID, result.State, ctx.Err())
			case <-ticker.C:
				continue
			}
		}
	}
}

func officialGetBatchResult(ctx context.Context, client *http.Client, token, batchID, dataID string) (*officialExtractResult, error) {
	var parsed officialResponse[officialBatchResultData]
	if err := officialJSON(ctx, client, http.MethodGet, officialBaseURL()+"/api/v4/extract-results/batch/"+batchID, token, nil, &parsed); err != nil {
		return nil, err
	}
	if !parsed.Code.OK() {
		return nil, fmt.Errorf("mineru official: get batch result failed code=%s msg=%s trace_id=%s", parsed.Code, parsed.Msg, parsed.TraceID)
	}
	if len(parsed.Data.ExtractResults) == 0 {
		return &officialExtractResult{State: "pending"}, nil
	}
	if dataID != "" {
		for i := range parsed.Data.ExtractResults {
			if parsed.Data.ExtractResults[i].DataID == dataID {
				return &parsed.Data.ExtractResults[i], nil
			}
		}
	}
	return &parsed.Data.ExtractResults[0], nil
}

func officialJSON(ctx context.Context, client *http.Client, method, url, token string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("mineru official: new %s %s: %w", method, url, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "*/*")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("mineru official: call %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("mineru official: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(respBody)
		if len(snippet) > 800 {
			snippet = snippet[:800] + "..."
		}
		return fmt.Errorf("mineru official: HTTP %d from %s: %s", resp.StatusCode, url, snippet)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("mineru official: decode response: %w", err)
	}
	return nil
}

func officialDownloadAndReadZip(ctx context.Context, client *http.Client, zipURL string) (*officialZipArtifacts, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		return nil, fmt.Errorf("mineru official: new zip request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mineru official: download result zip: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		snippet, _ := readSnippet(resp.Body)
		return nil, fmt.Errorf("mineru official: zip HTTP %d: %s", resp.StatusCode, snippet)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mineru official: read result zip: %w", err)
	}
	return officialReadZipArtifacts(data)
}

func officialReadZipArtifacts(data []byte) (*officialZipArtifacts, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("mineru official: open result zip: %w", err)
	}

	artifacts := &officialZipArtifacts{}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			continue
		}
		base := strings.ToLower(filepath.Base(file.Name))
		switch {
		case isOfficialMarkdown(base):
			content, err := readZipFile(file)
			if err != nil {
				return nil, err
			}
			if artifacts.Markdown == "" || base == "full.md" {
				artifacts.Markdown = content
				artifacts.MarkdownPath = file.Name
			}
		case isOfficialContentList(base):
			content, err := readZipFile(file)
			if err != nil {
				return nil, err
			}
			if artifacts.ContentList == "" || strings.HasSuffix(base, "_content_list.json") {
				artifacts.ContentList = content
				artifacts.ContentListPath = file.Name
			}
		case isOfficialMiddleJSON(base):
			content, err := readZipFile(file)
			if err != nil {
				return nil, err
			}
			if artifacts.MiddleJSON == "" || strings.HasSuffix(base, "_middle.json") {
				artifacts.MiddleJSON = content
				artifacts.MiddleJSONPath = file.Name
			}
		case isOfficialImage(base):
			dataURI, err := readZipImageDataURI(file, base)
			if err != nil {
				return nil, err
			}
			if artifacts.Images == nil {
				artifacts.Images = map[string]string{}
			}
			for _, name := range officialImageAssetNames(file.Name) {
				if _, exists := artifacts.Images[name]; !exists {
					artifacts.Images[name] = dataURI
				}
			}
		}
	}
	return artifacts, nil
}

func readZipFile(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("mineru official: open zip entry %s: %w", file.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("mineru official: read zip entry %s: %w", file.Name, err)
	}
	return string(data), nil
}

func readZipImageDataURI(file *zip.File, base string) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("mineru official: open image zip entry %s: %w", file.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("mineru official: read image zip entry %s: %w", file.Name, err)
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(base)))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("mineru official: unsupported image content type %s for %s", contentType, file.Name)
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(data)), nil
}

func officialImageAssetNames(name string) []string {
	normalized := strings.Trim(strings.ReplaceAll(name, "\\", "/"), "/")
	if normalized == "" {
		return nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	add := func(value string) {
		value = strings.Trim(strings.ReplaceAll(value, "\\", "/"), "/")
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	add(normalized)
	if idx := strings.Index(strings.ToLower(normalized), "images/"); idx >= 0 {
		add(normalized[idx:])
	}
	add(filepath.Base(normalized))
	return out
}

func isOfficialMarkdown(base string) bool {
	return base == "full.md" || strings.HasSuffix(base, "_full.md") || (strings.HasSuffix(base, ".md") && !strings.Contains(base, "origin"))
}

func isOfficialContentList(base string) bool {
	if !strings.HasSuffix(base, ".json") || strings.Contains(base, "content_list_v2") {
		return false
	}
	return base == "content_list.json" || strings.HasSuffix(base, "_content_list.json")
}

func isOfficialMiddleJSON(base string) bool {
	return base == "middle.json" || base == "layout.json" || strings.HasSuffix(base, "_middle.json") || strings.HasSuffix(base, "_layout.json")
}

func isOfficialImage(base string) bool {
	switch strings.ToLower(filepath.Ext(base)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func officialModelVersion(filename string) string {
	if strings.EqualFold(filepath.Ext(filename), ".html") || strings.EqualFold(filepath.Ext(filename), ".htm") {
		return "MinerU-HTML"
	}
	if v := envconfig.String("MINERU_OFFICIAL_MODEL_VERSION"); v != "" {
		return v
	}
	return "vlm"
}

func readSnippet(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, 800))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func syntheticContentListFromMarkdown(markdown string) string {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return ""
	}
	items := make([]contentItem, 0)
	for _, block := range splitMarkdownBlocks(markdown) {
		text := strings.TrimSpace(block)
		if text == "" {
			continue
		}
		item := contentItem{
			Type:    "text",
			Text:    text,
			PageIdx: 0,
		}
		if strings.HasPrefix(text, "#") {
			level := 0
			for _, r := range text {
				if r != '#' {
					break
				}
				level++
			}
			if level > 0 && level <= 6 {
				item.TextLevel = level
				item.Text = strings.TrimSpace(strings.TrimLeft(text, "#"))
			}
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		items = append(items, contentItem{
			Type:    "text",
			Text:    markdown,
			PageIdx: 0,
		})
	}
	data, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(data)
}

func splitMarkdownBlocks(markdown string) []string {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	parts := strings.Split(normalized, "\n\n")
	if len(parts) > 1 {
		return parts
	}
	return strings.Split(normalized, "\n")
}

func detectLang(filename string) string {
	for _, r := range filename {
		if r > 127 {
			return "ch"
		}
	}
	return "en"
}

func mineruToDocumentResult(filename string, resp *parseResponse) (*extractcommon.DocumentResult, error) {
	var fr fileResults
	var fileBase string
	for k, v := range resp.Results {
		fileBase = k
		fr = v
		break
	}
	if fr.ContentList == "" {
		return nil, fmt.Errorf("mineru: missing content_list in results")
	}

	items, err := decodeContentItems(fr.ContentList)
	if err != nil {
		return nil, fmt.Errorf("mineru: decode content_list: %w", err)
	}

	var middle middleJSON
	if fr.MiddleJSON != "" {
		_ = json.Unmarshal([]byte(fr.MiddleJSON), &middle)
	}

	pages := buildPages(middle, items)
	chunks := buildChunks(items, middle, fr.Images)

	return &extractcommon.DocumentResult{
		DocID:     coalesce(resp.TaskID, newID()),
		FileName:  coalesce(filename, fileBase),
		PageCount: len(pages),
		Pages:     pages,
		Chunks:    chunks,
		Markdown:  fr.MdContent,
		Source:    "mineru:" + coalesce(resp.Backend, "pipeline"),
		Diagnostics: map[string]any{
			"mineru_structure": buildStructureDiagnostics(items, chunks),
		},
	}, nil
}

func decodeContentItems(raw string) ([]contentItem, error) {
	var items []contentItem
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		return items, nil
	}

	var pages [][]contentItemV2
	if err := json.Unmarshal([]byte(raw), &pages); err != nil {
		return nil, err
	}
	out := make([]contentItem, 0)
	for pageIdx, page := range pages {
		for _, item := range page {
			legacy := contentItem{
				Type:    mapV2Type(item.Type),
				Text:    textFromV2Content(item.Content),
				BBox:    item.BBox,
				PageIdx: pageIdx,
			}
			if item.Type == "title" {
				legacy.TextLevel = levelFromV2Content(item.Content)
			}
			if legacy.Type == "table" {
				legacy.TableBody = textFromV2Content(item.Content)
			}
			if legacy.Text == "" && item.Anchor != "" {
				legacy.Text = item.Anchor
			}
			out = append(out, legacy)
		}
	}
	return out, nil
}

func levelFromV2Content(value any) int {
	content, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	return intFromAny(content["level"])
}

func mapV2Type(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "title", "paragraph", "page_header", "page_footer", "page_number", "page_aside_text", "page_footnote", "list", "index", "code", "algorithm":
		return "text"
	case "equation_interline":
		return "equation"
	case "chart":
		return "image"
	default:
		return typ
	}
}

func textFromV2Content(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s := textFromV2Content(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case []string:
		return strings.TrimSpace(strings.Join(v, "\n"))
	case map[string]any:
		if content, ok := v["content"].(string); ok && strings.TrimSpace(content) != "" {
			return strings.TrimSpace(content)
		}
		for _, key := range []string{
			"title_content",
			"paragraph_content",
			"math_content",
			"table_content",
			"table_body",
			"code_content",
			"algorithm_content",
			"list_items",
			"page_header_content",
			"page_footer_content",
			"page_number_content",
			"page_aside_text_content",
			"page_footnote_content",
		} {
			if s := textFromV2Content(v[key]); s != "" {
				return s
			}
		}
		parts := make([]string, 0, len(v))
		for key, item := range v {
			if key == "type" || key == "level" || key == "url" {
				continue
			}
			if s := textFromV2Content(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func buildPages(middle middleJSON, items []contentItem) []extractcommon.Page {
	byIdx := make(map[int]extractcommon.Page)
	for _, p := range middle.PdfInfo {
		w, h := 0.0, 0.0
		if len(p.PageSize) >= 2 {
			w = float64(p.PageSize[0])
			h = float64(p.PageSize[1])
		}
		byIdx[p.PageIdx] = extractcommon.Page{PageIndex: p.PageIdx, Width: w, Height: h}
	}
	maxPage := -1
	for _, it := range items {
		if it.PageIdx > maxPage {
			maxPage = it.PageIdx
		}
	}
	if maxPage < 0 {
		maxPage = len(byIdx) - 1
	}
	out := make([]extractcommon.Page, 0, maxPage+1)
	for i := 0; i <= maxPage; i++ {
		if p, ok := byIdx[i]; ok {
			out = append(out, p)
		} else {
			out = append(out, extractcommon.Page{PageIndex: i})
		}
	}
	return out
}

func buildChunks(items []contentItem, middle middleJSON, imageAssets map[string]string) []extractcommon.Chunk {
	scores := buildMineruBlockScoreLookup(middle)
	out := make([]extractcommon.Chunk, 0, len(items))
	pageOrder := map[int]int{}
	for i, it := range items {
		t, sub := mapType(it)
		payload := buildPayload(i, it)
		ch := extractcommon.Chunk{
			ID:        fmt.Sprintf("mineru-%d", i),
			Type:      t,
			Subtype:   sub,
			Page:      it.PageIdx,
			Ordinal:   i + 1,
			Precision: "reliable",
			BBox:      toBBox(it.BBox),
			Text:      extractText(it),
			Payload:   payload,
		}
		itemPageOrder := pageOrder[it.PageIdx]
		pageOrder[it.PageIdx] = itemPageOrder + 1
		if score, ok := scores.scoreFor(i, itemPageOrder, it); ok {
			ch.Confidence = score
			payload["mineru_block_score"] = score
		}
		if it.TableBody != "" {
			tableBody := rewriteMineruContentImageReferences(it.TableBody, imageAssets)
			ch.Text = tableBody
			ch.Markdown = tableBody
			payload["table_body"] = tableBody
		}
		if shouldUseMineruImageAsset(it) {
			if dataURI, ok := lookupMineruContentImageDataURI(imageAssets, it.ImgPath); ok {
				payload["original_img_path"] = it.ImgPath
				payload["image_data_uri"] = dataURI
				payload["image_ref_type"] = "data_uri"
				payload["img_path"] = dataURI
				ch.Markdown = mineruImageMarkdownWithPath(it, dataURI)
			} else {
				ch.Markdown = mineruImageMarkdown(it)
			}
		}
		out = append(out, ch)
	}
	return out
}

func shouldUseMineruImageAsset(it contentItem) bool {
	if strings.TrimSpace(it.ImgPath) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(it.Type)) {
	case "image", "chart", "figure":
		return true
	default:
		return false
	}
}

func mineruImageMarkdown(it contentItem) string {
	return mineruImageMarkdownWithPath(it, strings.TrimSpace(it.ImgPath))
}

func mineruImageMarkdownWithPath(it contentItem, path string) string {
	alt := strings.TrimSpace(firstString(it.ImageCaption))
	if alt == "" {
		alt = strings.TrimSpace(firstString(it.ChartCaption))
	}
	if alt == "" {
		alt = "figure"
	}
	return fmt.Sprintf("![%s](%s)", alt, strings.TrimSpace(path))
}

func rewriteMineruContentImageReferences(content string, imageAssets map[string]string) string {
	if strings.TrimSpace(content) == "" || len(imageAssets) == 0 {
		return content
	}

	rewritten := mineruMarkdownImagePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := mineruMarkdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		if dataURI, ok := lookupMineruContentImageDataURI(imageAssets, parts[2]); ok {
			return fmt.Sprintf("![%s](%s)", parts[1], dataURI)
		}
		return match
	})

	return mineruHTMLImageSrcPattern.ReplaceAllStringFunc(rewritten, func(match string) string {
		parts := mineruHTMLImageSrcPattern.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		if dataURI, ok := lookupMineruContentImageDataURI(imageAssets, parts[3]); ok {
			quote := parts[2]
			if quote == "" {
				quote = `"`
			}
			return parts[1] + quote + dataURI + quote
		}
		return match
	})
}

func lookupMineruContentImageDataURI(images map[string]string, imagePath string) (string, bool) {
	if len(images) == 0 {
		return "", false
	}
	for _, candidate := range mineruImageAssetNameCandidates(imagePath) {
		if value, ok := images[candidate]; ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

func mineruImageAssetNameCandidates(imagePath string) []string {
	normalized := strings.Trim(strings.ReplaceAll(imagePath, "\\", "/"), "/")
	if normalized == "" {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	add := func(value string) {
		value = strings.Trim(strings.ReplaceAll(value, "\\", "/"), "/")
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(normalized)
	if idx := strings.Index(strings.ToLower(normalized), "images/"); idx >= 0 {
		add(normalized[idx+len("images/"):])
		add(normalized[idx:])
	}
	add(filepath.Base(normalized))
	return out
}

type mineruBlockScoreLookup struct {
	byIndex map[int]float64
	byBBox  map[string]float64
	byOrder map[string]float64
}

func buildMineruBlockScoreLookup(middle middleJSON) mineruBlockScoreLookup {
	lookup := mineruBlockScoreLookup{
		byIndex: map[int]float64{},
		byBBox:  map[string]float64{},
		byOrder: map[string]float64{},
	}
	pageOrder := map[int]int{}
	for _, page := range middle.PdfInfo {
		for _, block := range page.PreprocBlocks {
			score, ok := normalizeMineruScore(block.Score)
			if !ok {
				continue
			}
			if len(block.BBox) >= 4 {
				lookup.byBBox[mineruBBoxKey(page.PageIdx, block.BBox)] = score
			}
			if block.Index > 0 {
				lookup.byIndex[block.Index] = score
			}
			order := pageOrder[page.PageIdx]
			lookup.byOrder[mineruOrderKey(page.PageIdx, order)] = score
			pageOrder[page.PageIdx] = order + 1
		}
	}
	return lookup
}

func (l mineruBlockScoreLookup) scoreFor(itemIndex, pageOrder int, item contentItem) (float64, bool) {
	if len(l.byIndex) == 0 && len(l.byBBox) == 0 && len(l.byOrder) == 0 {
		return 0, false
	}
	if len(item.BBox) >= 4 {
		if score, ok := l.byBBox[mineruBBoxKey(item.PageIdx, item.BBox)]; ok {
			return score, true
		}
	}
	if score, ok := l.byIndex[itemIndex+1]; ok {
		return score, true
	}
	if score, ok := l.byOrder[mineruOrderKey(item.PageIdx, pageOrder)]; ok {
		return score, true
	}
	return 0, false
}

func normalizeMineruScore(score *float64) (float64, bool) {
	if score == nil {
		return 0, false
	}
	value := *score
	if value < 0 {
		return 0, false
	}
	if value > 1 {
		value = 1
	}
	return value, true
}

func mineruBBoxKey(pageIdx int, bbox []int) string {
	if len(bbox) < 4 {
		return fmt.Sprintf("%d:", pageIdx)
	}
	return fmt.Sprintf("%d:%d,%d,%d,%d", pageIdx, bbox[0], bbox[1], bbox[2], bbox[3])
}

func mineruOrderKey(pageIdx, order int) string {
	return fmt.Sprintf("%d:%d", pageIdx, order)
}

func buildPayload(index int, it contentItem) map[string]any {
	payload := map[string]any{
		"mineru_type":       it.Type,
		"mineru_index":      index,
		"reading_order":     index + 1,
		"page_idx":          it.PageIdx,
		"bbox_source":       "mineru",
		"structure_version": "mineru_content_list_v1",
		"has_mineru_bbox":   len(it.BBox) >= 4,
		"mineru_text_level": it.TextLevel,
	}
	if it.ImgPath != "" {
		payload["img_path"] = it.ImgPath
	}
	if strings.TrimSpace(it.TableBody) != "" {
		payload["table_body"] = it.TableBody
	}
	addAny(payload, "table_caption", it.TableCaption)
	addAny(payload, "table_footnote", it.TableFootnote)
	addAny(payload, "image_caption", it.ImageCaption)
	addAny(payload, "image_footnote", it.ImageFootnote)
	addAny(payload, "chart_caption", it.ChartCaption)
	addAny(payload, "chart_footnote", it.ChartFootnote)
	return payload
}

func addAny(payload map[string]any, key string, value any) {
	switch v := value.(type) {
	case nil:
		return
	case string:
		if strings.TrimSpace(v) != "" {
			payload[key] = v
		}
	case []any:
		if len(v) > 0 {
			payload[key] = v
		}
	case map[string]any:
		if len(v) > 0 {
			payload[key] = v
		}
	default:
		payload[key] = v
	}
}

func buildStructureDiagnostics(items []contentItem, chunks []extractcommon.Chunk) map[string]any {
	byMineruType := map[string]int{}
	byChunkType := map[string]int{}
	withBBox := 0
	withCaption := 0
	withFootnote := 0
	withTableBody := 0
	for i, it := range items {
		typ := coalesce(it.Type, "unknown")
		byMineruType[typ]++
		if len(it.BBox) >= 4 {
			withBBox++
		}
		if strings.TrimSpace(it.TableBody) != "" {
			withTableBody++
		}
		if hasAny(it.TableCaption) || hasAny(it.ImageCaption) || hasAny(it.ChartCaption) {
			withCaption++
		}
		if hasAny(it.TableFootnote) || hasAny(it.ImageFootnote) || hasAny(it.ChartFootnote) {
			withFootnote++
		}
		if i < len(chunks) {
			byChunkType[coalesce(chunks[i].Type, "unknown")]++
		}
	}
	coverage := 0.0
	if len(items) > 0 {
		coverage = float64(withBBox) / float64(len(items))
	}
	return map[string]any{
		"schema":              "mineru_content_list_v1",
		"content_items":       len(items),
		"chunks":              len(chunks),
		"mineru_type_counts":  byMineruType,
		"chunk_type_counts":   byChunkType,
		"bbox_items":          withBBox,
		"bbox_coverage":       coverage,
		"caption_items":       withCaption,
		"footnote_items":      withFootnote,
		"table_body_items":    withTableBody,
		"reading_order_field": "reading_order",
	}
}

func hasAny(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func mapType(it contentItem) (string, string) {
	if shouldUseMineruImageAsset(it) {
		return "figure", ""
	}

	switch it.Type {
	case "text":
		if it.TextLevel >= 1 {
			return "heading", fmt.Sprintf("h%d", it.TextLevel)
		}
		return "text", ""
	case "image":
		return "figure", ""
	case "table":
		return "table", ""
	case "equation":
		return "formula", ""
	case "header", "footer", "page_number", "page_footnote", "aside_text", "discarded":
		return "marginalia", it.Type
	default:
		return "text", it.Type
	}
}

func extractText(it contentItem) string {
	if s := strings.TrimSpace(it.Text); s != "" {
		return s
	}
	if it.Type == "table" && strings.TrimSpace(it.TableBody) != "" {
		return strings.TrimSpace(it.TableBody)
	}
	if shouldUseMineruImageAsset(it) {
		return "[figure]"
	}
	if it.Type == "image" || it.Type == "chart" {
		if caption := firstString(it.ImageCaption); caption != "" {
			return caption
		}
		if caption := firstString(it.ChartCaption); caption != "" {
			return caption
		}
	}
	if it.Type == "table" {
		return "[table]"
	}
	if it.Type == "equation" {
		return "[formula]"
	}
	return ""
}

func firstString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		for _, item := range v {
			if s := firstString(item); s != "" {
				return s
			}
		}
	case map[string]any:
		for _, item := range v {
			if s := firstString(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func toBBox(b []int) *extractcommon.BBox {
	if len(b) < 4 {
		return nil
	}
	x0 := float64(b[0]) / bboxScale
	y0 := float64(b[1]) / bboxScale
	x1 := float64(b[2]) / bboxScale
	y1 := float64(b[3]) / bboxScale
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	return &extractcommon.BBox{Left: clamp(x0), Top: clamp(y0), Right: clamp(x1), Bottom: clamp(y1)}
}

func coalesce(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", buf)
}

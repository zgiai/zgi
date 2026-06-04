package filegenerator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	defaultSystemOfficeProfile  = "skill-office"
	fileGeneratorHTMLInputPath  = "input.html"
	fileGeneratorPDFOutputPath  = "artifacts/input.pdf"
	fileGeneratorPPTXSpecPath   = "presentation.json"
	fileGeneratorPPTXScriptPath = "render-pptx.mjs"
	fileGeneratorPPTXOutputPath = "artifacts/output.pptx"
)

type fileGeneratorSandboxClient struct {
	endpoint          string
	apiKey            string
	dependencyProfile string
	client            *http.Client
	timeouts          fileGeneratorSandboxTimeouts
}

type fileGeneratorSandboxTimeouts struct {
	create   time.Duration
	upload   time.Duration
	command  time.Duration
	artifact time.Duration
	cleanup  time.Duration
}

type fileGeneratorSandboxCommandResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error"`
}

func (r fileGeneratorSandboxCommandResult) stderrText() string {
	if text := strings.TrimSpace(r.Stderr); text != "" {
		return text
	}
	return strings.TrimSpace(r.Error)
}

type fileGeneratorSandboxFileContent struct {
	Content      string `json:"content"`
	Encoding     string `json:"encoding"`
	Size         int64  `json:"size"`
	Path         string `json:"path"`
	ContentType  string `json:"content_type,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

type fileGeneratorSandboxDependencyCatalog struct {
	Profiles []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Enabled bool   `json:"enabled"`
		Version string `json:"version"`
	} `json:"profiles"`
}

func renderFileGeneratorHTMLPDF(ctx context.Context, runtime *tools.ToolRuntime, tenantID string, document string) ([]byte, error) {
	client, err := newFileGeneratorSandboxClient()
	if err != nil {
		return nil, err
	}
	organizationID := strings.TrimSpace(tenantID)
	if runtime != nil {
		organizationID = normalizeDefault(runtime.TenantID, organizationID)
	}
	sandboxID, err := client.createSandbox(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("create HTML PDF sandbox: %w", err)
	}
	defer func() {
		_ = client.deleteSandbox(context.Background(), sandboxID, organizationID)
	}()
	if err := client.uploadFile(ctx, sandboxID, organizationID, fileGeneratorHTMLInputPath, []byte(document)); err != nil {
		return nil, fmt.Errorf("upload HTML PDF source: %w", err)
	}
	command, err := client.runHTMLPDFCommand(ctx, sandboxID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("render HTML PDF in sandbox: %w", err)
	}
	if command.ExitCode != 0 {
		return nil, fmt.Errorf("render HTML PDF exited with code %d: %s", command.ExitCode, command.stderrText())
	}
	content, err := client.downloadFile(ctx, sandboxID, organizationID, fileGeneratorPDFOutputPath)
	if err != nil {
		return nil, fmt.Errorf("download rendered PDF: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(content.Encoding), "base64") {
		return nil, fmt.Errorf("download rendered PDF returned unsupported encoding: %s", content.Encoding)
	}
	data, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		return nil, fmt.Errorf("decode rendered PDF: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("render HTML PDF produced an empty file")
	}
	return data, nil
}

func renderFileGeneratorPPTX(ctx context.Context, runtime *tools.ToolRuntime, tenantID string, specJSON string) ([]byte, error) {
	client, err := newFileGeneratorSandboxClient()
	if err != nil {
		return nil, err
	}
	organizationID := strings.TrimSpace(tenantID)
	if runtime != nil {
		organizationID = normalizeDefault(runtime.TenantID, organizationID)
	}
	sandboxID, err := client.createSandbox(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("create PPTX sandbox: %w", err)
	}
	defer func() {
		_ = client.deleteSandbox(context.Background(), sandboxID, organizationID)
	}()
	if err := client.uploadFile(ctx, sandboxID, organizationID, fileGeneratorPPTXSpecPath, []byte(specJSON)); err != nil {
		return nil, fmt.Errorf("upload PPTX spec: %w", err)
	}
	if err := client.uploadFile(ctx, sandboxID, organizationID, fileGeneratorPPTXScriptPath, []byte(fileGeneratorPPTXRenderScript)); err != nil {
		return nil, fmt.Errorf("upload PPTX renderer: %w", err)
	}
	command, err := client.runPPTXCommand(ctx, sandboxID, organizationID)
	if err != nil {
		return nil, fmt.Errorf("render PPTX in sandbox: %w", err)
	}
	if command.ExitCode != 0 {
		return nil, fmt.Errorf("render PPTX exited with code %d: %s", command.ExitCode, command.stderrText())
	}
	content, err := client.downloadFile(ctx, sandboxID, organizationID, fileGeneratorPPTXOutputPath)
	if err != nil {
		return nil, fmt.Errorf("download rendered PPTX: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(content.Encoding), "base64") {
		return nil, fmt.Errorf("download rendered PPTX returned unsupported encoding: %s", content.Encoding)
	}
	data, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		return nil, fmt.Errorf("decode rendered PPTX: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("render PPTX produced an empty file")
	}
	return data, nil
}

func newFileGeneratorSandboxClient() (*fileGeneratorSandboxClient, error) {
	cfg := appconfig.GlobalConfig
	if cfg == nil {
		return nil, fmt.Errorf("system file generation sandbox renderer is not configured")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.CodeExec.Endpoint), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("system file generation sandbox renderer is not configured")
	}
	dependencyProfile := normalizeDefault(strings.TrimSpace(cfg.CodeExec.SystemOfficeProfile), defaultSystemOfficeProfile)
	connectTimeout := durationFromSeconds(cfg.CodeExec.ConnectTimeoutSeconds, 5*time.Second)
	return &fileGeneratorSandboxClient{
		endpoint:          endpoint,
		apiKey:            strings.TrimSpace(cfg.CodeExec.APIKey),
		dependencyProfile: dependencyProfile,
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   connectTimeout,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout: connectTimeout,
			},
		},
		timeouts: fileGeneratorSandboxTimeouts{
			create:   durationFromSeconds(cfg.CodeExec.CreateTimeoutSeconds, 10*time.Second),
			upload:   durationFromSeconds(cfg.CodeExec.UploadTimeoutSeconds, 30*time.Second),
			command:  htmlPDFRenderTimeout + durationFromSeconds(cfg.CodeExec.CommandTimeoutPaddingSeconds, 15*time.Second),
			artifact: durationFromSeconds(cfg.CodeExec.ArtifactTimeoutSeconds, 10*time.Second),
			cleanup:  durationFromSeconds(cfg.CodeExec.CleanupTimeoutSeconds, 5*time.Second),
		},
	}, nil
}

func (c *fileGeneratorSandboxClient) createSandbox(ctx context.Context, organizationID string) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	if err := c.ensureSystemOfficeProfileReady(ctx); err != nil {
		return "", err
	}
	payload := map[string]interface{}{
		"runtime_profile":    "session",
		"ttl_seconds":        300,
		"network_enabled":    false,
		"network_policy":     "deny-by-default",
		"dependency_profile": c.dependencyProfile,
	}
	if organizationID != "" {
		payload["organization_id"] = organizationID
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes", payload, &response, c.timeouts.create); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.ID) == "" {
		return "", fmt.Errorf("sandbox create response did not include sandbox id")
	}
	return response.ID, nil
}

func (c *fileGeneratorSandboxClient) ensureSystemOfficeProfileReady(ctx context.Context) error {
	if strings.TrimSpace(c.dependencyProfile) == "" {
		return fmt.Errorf("system file generation dependency profile is not configured")
	}
	var catalog fileGeneratorSandboxDependencyCatalog
	if err := c.doJSON(ctx, http.MethodGet, "/v1/sandbox/dependencies?language=nodejs", nil, &catalog, c.timeouts.create); err != nil {
		return fmt.Errorf("check system file generation dependency profile: %w", err)
	}
	for _, profile := range catalog.Profiles {
		if profile.Name != c.dependencyProfile {
			continue
		}
		if profile.Enabled && strings.EqualFold(profile.Status, "ready") {
			return nil
		}
		return fmt.Errorf("system file generation dependency profile %s is not ready: status=%s enabled=%t", c.dependencyProfile, profile.Status, profile.Enabled)
	}
	return fmt.Errorf("system file generation dependency profile %s is not available", c.dependencyProfile)
}

func (c *fileGeneratorSandboxClient) uploadFile(ctx context.Context, sandboxID string, organizationID string, path string, data []byte) error {
	payload := map[string]interface{}{
		"sandbox_id": sandboxID,
		"path":       path,
		"content":    base64.StdEncoding.EncodeToString(data),
		"encoding":   "base64",
	}
	if organizationID != "" {
		payload["organization_id"] = organizationID
	}
	return c.doJSON(ctx, http.MethodPost, "/v1/files/upload", payload, nil, c.timeouts.upload)
}

func (c *fileGeneratorSandboxClient) runHTMLPDFCommand(ctx context.Context, sandboxID string, organizationID string) (*fileGeneratorSandboxCommandResult, error) {
	var response fileGeneratorSandboxCommandResult
	payload := map[string]interface{}{
		"sandbox_id": sandboxID,
		"command":    "sh",
		"args": []string{
			"-lc",
			"profile_dir=\"$(pwd)/.libreoffice-profile\" && mkdir -p artifacts \"$profile_dir\" && libreoffice \"-env:UserInstallation=file://${profile_dir}\" --headless --convert-to pdf --outdir artifacts input.html",
		},
		"profile":         "skill-node",
		"timeout_seconds": int(htmlPDFRenderTimeout.Seconds()),
		"stdout_limit_kb": 1024,
		"stderr_limit_kb": 1024,
		"working_subpath": ".",
	}
	if organizationID != "" {
		payload["organization_id"] = organizationID
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/exec/command", payload, &response, c.timeouts.command); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *fileGeneratorSandboxClient) runPPTXCommand(ctx context.Context, sandboxID string, organizationID string) (*fileGeneratorSandboxCommandResult, error) {
	var response fileGeneratorSandboxCommandResult
	payload := map[string]interface{}{
		"sandbox_id": sandboxID,
		"command":    "node",
		"args": []string{
			fileGeneratorPPTXScriptPath,
		},
		"profile":         "skill-node",
		"timeout_seconds": int(htmlPDFRenderTimeout.Seconds()),
		"stdout_limit_kb": 1024,
		"stderr_limit_kb": 1024,
		"working_subpath": ".",
	}
	if organizationID != "" {
		payload["organization_id"] = organizationID
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/exec/command", payload, &response, c.timeouts.command); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *fileGeneratorSandboxClient) downloadFile(ctx context.Context, sandboxID string, organizationID string, path string) (*fileGeneratorSandboxFileContent, error) {
	var response fileGeneratorSandboxFileContent
	endpoint := "/v1/files/download?sandbox_id=" + url.QueryEscape(sandboxID) + "&path=" + url.QueryEscape(path) + "&encoding=base64"
	if organizationID != "" {
		endpoint += "&organization_id=" + url.QueryEscape(organizationID)
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint, nil, &response, c.timeouts.artifact); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *fileGeneratorSandboxClient) deleteSandbox(ctx context.Context, sandboxID string, organizationID string) error {
	if strings.TrimSpace(sandboxID) == "" {
		return nil
	}
	endpoint := "/v1/sandboxes/" + url.PathEscape(sandboxID)
	if organizationID != "" {
		endpoint += "?organization_id=" + url.QueryEscape(organizationID)
	}
	return c.doJSON(ctx, http.MethodDelete, endpoint, nil, nil, c.timeouts.cleanup)
}

func (c *fileGeneratorSandboxClient) doJSON(ctx context.Context, method string, path string, payload interface{}, out interface{}, timeout time.Duration) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("X-API-Key", c.apiKey)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("failed to parse sandbox response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = res.Status
		}
		return fmt.Errorf("sandbox request %s %s failed: %s", method, path, message)
	}
	if out != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, out); err != nil {
			return fmt.Errorf("failed to parse sandbox data: %w", err)
		}
	}
	return nil
}

func durationFromSeconds(seconds int, fallback time.Duration) time.Duration {
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

const fileGeneratorPPTXRenderScript = `
import { createRequire } from "node:module";
import fs from "node:fs";
const require = createRequire(import.meta.url);
const pptxgenModule = require("pptxgenjs");
const pptxgen = pptxgenModule.default || pptxgenModule;

const spec = JSON.parse(fs.readFileSync("presentation.json", "utf8"));
const pptx = new pptxgen();
pptx.layout = spec.layout === "4:3" ? "LAYOUT_4X3" : "LAYOUT_WIDE";
pptx.author = "ZGI";
pptx.subject = "Generated presentation";
pptx.company = "ZGI";
pptx.lang = spec.language || "en-US";

function compact(value) {
  const out = {};
  for (const [key, item] of Object.entries(value)) {
    if (item !== undefined && item !== null && item !== "") out[key] = item;
  }
  return out;
}

function numberOr(value, fallback) {
  return typeof value === "number" ? value : fallback;
}

function applyStyle(opts, style, defaults = {}) {
  const merged = { ...defaults, ...(style || {}) };
  if (merged.font_face) opts.fontFace = merged.font_face;
  if (merged.font_size) opts.fontSize = merged.font_size;
  if (merged.color) opts.color = merged.color;
  if (merged.bold !== undefined) opts.bold = merged.bold;
  if (merged.italic !== undefined) opts.italic = merged.italic;
  if (merged.underline !== undefined) opts.underline = merged.underline;
  if (merged.strike !== undefined) opts.strike = merged.strike;
  if (merged.align) opts.align = merged.align;
  if (merged.valign) opts.valign = merged.valign;
  if (merged.margin !== undefined) opts.margin = merged.margin;
  if (merged.break_line !== undefined) opts.breakLine = merged.break_line;
  if (merged.line_spacing !== undefined) {
    opts.lineSpacingMultiple = merged.line_spacing;
    opts.lineSpacing = merged.line_spacing;
  }
}

function box(element, fallback) {
  return {
    x: numberOr(element.x, fallback.x),
    y: numberOr(element.y, fallback.y),
    w: numberOr(element.w, fallback.w),
    h: numberOr(element.h, fallback.h),
  };
}

fs.mkdirSync("artifacts", { recursive: true });

for (const slideSpec of spec.slides) {
  const slide = pptx.addSlide();
  if (slideSpec.background_color) slide.background = { color: slideSpec.background_color };
  for (const element of slideSpec.elements) {
    if (element.type === "title" || element.type === "text") {
      const fallback = element.type === "title"
        ? { x: 0.6, y: 0.35, w: 12.1, h: 0.7 }
        : { x: 0.75, y: 1.25, w: 11.8, h: 1.0 };
      const opts = box(element, fallback);
      opts.margin = element.margin ?? 0.04;
      opts.breakLine = element.break_line ?? false;
      if (element.line_spacing !== undefined) {
        opts.lineSpacingMultiple = element.line_spacing;
        opts.lineSpacing = element.line_spacing;
      }
      applyStyle(opts, element.style, spec.default_style);
      if (element.type === "title" && !opts.fontSize) opts.fontSize = 30;
      if (element.type === "title" && opts.bold === undefined) opts.bold = true;
      slide.addText(element.text, compact(opts));
      continue;
    }
    if (element.type === "table") {
      const rows = [];
      if (element.headers && element.headers.length) {
        rows.push(element.headers.map((text) => ({
          text,
          options: compact({
            bold: true,
            color: element.header_color,
            fill: element.header_fill_color ? { color: element.header_fill_color } : undefined,
          }),
        })));
      }
      for (const row of element.rows || []) rows.push(row);
      const opts = box(element, { x: 0.75, y: 1.45, w: 11.8, h: 4.8 });
      opts.margin = element.margin ?? 0.05;
      opts.border = { type: "solid", color: element.border_color || "D1D5DB", pt: 1 };
      opts.fontSize = element.style?.font_size || spec.default_style?.font_size || 12;
      opts.color = element.style?.color || spec.default_style?.color || "111827";
      if (element.column_widths && element.column_widths.length) opts.colW = element.column_widths;
      if (element.row_fill_color) opts.fill = { color: element.row_fill_color };
      applyStyle(opts, element.style, spec.default_style);
      slide.addTable(rows, compact(opts));
      continue;
    }
    if (element.type === "shape") {
      const opts = box(element, { x: 0.75, y: 1.2, w: 2.0, h: 1.0 });
      if (element.fill_color) opts.fill = { color: element.fill_color };
      if (element.line_color) opts.line = { color: element.line_color };
      if (element.rotation !== undefined) opts.rotate = element.rotation;
      if (element.transparency !== undefined) opts.transparency = element.transparency;
      slide.addShape(pptx.ShapeType.rect, compact(opts));
    }
  }
}

await pptx.writeFile({ fileName: "artifacts/output.pptx" });
console.log(JSON.stringify({ success: true, path: "artifacts/output.pptx", slides: spec.slides.length }));
`

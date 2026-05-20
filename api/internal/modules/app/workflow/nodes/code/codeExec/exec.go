package codeexec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/observability"
)

// Language represents the language used by code node
type Language string

const (
	LanguagePython3    Language = "python3"
	LanguageJavascript Language = "javascript"
	LanguageJinja2     Language = "jinja2"
)

// ErrUnsupportedLanguage indicates language is not supported
var ErrUnsupportedLanguage = errors.New("unsupported language")

// ErrTransformerNotRegistered indicates transformer not registered for language
var ErrTransformerNotRegistered = errors.New("transformer not registered for language")

// CodeExecutionError describes errors that occur when executing remote code
type CodeExecutionError struct {
	Message string
}

// Error implements error interface
func (e *CodeExecutionError) Error() string {
	return e.Message
}

// TemplateTransformer defines code template transformer interface
type TemplateTransformer interface {
	Language() Language
	TransformCaller(code string, inputs map[string]any) (runner string, preload string, err error)
	TransformResponse(raw string) (map[string]any, error)
}

// Executor is responsible for transforming template code to executable content and calling remote sandbox
type Executor struct {
	httpClient        *http.Client
	transformers      map[Language]TemplateTransformer
	languageToRuntime map[Language]string
	enableNetwork     bool
}

// NewExecutor creates a minimal executor
func NewExecutor(transformers ...TemplateTransformer) *Executor {
	e := &Executor{
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: 2 * time.Minute,
		}),
		transformers: make(map[Language]TemplateTransformer),
		languageToRuntime: map[Language]string{
			LanguagePython3:    "python3",
			LanguageJavascript: "nodejs",
			LanguageJinja2:     "python3",
		},
		enableNetwork: true,
	}

	for _, transformer := range transformers {
		if transformer == nil {
			continue
		}
		e.transformers[transformer.Language()] = transformer
	}

	return e
}

// RegisterTransformer registers additional transformer to executor
func (e *Executor) RegisterTransformer(transformer TemplateTransformer) {
	if e == nil || transformer == nil {
		return
	}
	e.transformers[transformer.Language()] = transformer
}

type executionRequest struct {
	Language      string `json:"language"`
	Code          string `json:"code"`
	Preload       string `json:"preload"`
	EnableNetwork bool   `json:"enable_network"`
}

type executionResponseData struct {
	Stdout *string `json:"stdout"`
	Error  *string `json:"error"`
}

type executionResponse struct {
	Code    int                   `json:"code"`
	Message string                `json:"message"`
	Data    executionResponseData `json:"data"`
}

// ExecuteWorkflowCodeTemplate executes workflow code template
func (e *Executor) ExecuteWorkflowCodeTemplate(
	ctx context.Context,
	lang Language,
	code string,
	inputs map[string]any,
) (map[string]any, error) {
	if e == nil {
		return nil, errors.New("executor is nil")
	}

	transformer, exists := e.transformers[lang]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrTransformerNotRegistered, lang)
	}

	runner, preload, err := transformer.TransformCaller(code, inputs)
	if err != nil {
		return nil, fmt.Errorf("transform caller failed: %w", err)
	}

	raw, err := e.ExecuteCode(ctx, lang, preload, runner)
	if err != nil {
		return nil, err
	}

	result, err := transformer.TransformResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("transform response failed: %w", err)
	}
	return result, nil
}

// ExecuteCode calls remote sandbox to execute code
func (e *Executor) ExecuteCode(ctx context.Context, lang Language, preload, runner string) (string, error) {
	if e == nil {
		return "", errors.New("executor is nil")
	}

	cfg := config.GlobalConfig
	if cfg == nil {
		return "", &CodeExecutionError{Message: "global config is not initialized"}
	}

	endpoint := strings.TrimSuffix(cfg.CodeExec.Endpoint, "/")
	if endpoint == "" {
		return "", &CodeExecutionError{Message: "code execution endpoint is not configured"}
	}

	runtime, exists := e.languageToRuntime[lang]
	if !exists {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedLanguage, lang)
	}

	payload := executionRequest{
		Language:      runtime,
		Code:          runner,
		Preload:       preload,
		EnableNetwork: e.enableNetwork,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal execution payload: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/v1/sandbox/run", endpoint),
		bytes.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if apiKey := cfg.CodeExec.APIKey; apiKey != "" {
		request.Header.Set("X-Api-Key", apiKey)
	}

	resp, err := e.httpClient.Do(request)
	if err != nil {
		return "", &CodeExecutionError{Message: fmt.Sprintf("failed to execute code: %v", err)}
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		return "", &CodeExecutionError{Message: "code execution service is unavailable"}
	}
	if resp.StatusCode != http.StatusOK {
		return "", &CodeExecutionError{
			Message: fmt.Sprintf("failed to execute code, status code: %d, response: %s", resp.StatusCode, string(bodyBytes)),
		}
	}

	var response executionResponse
	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		return "", &CodeExecutionError{Message: fmt.Sprintf("failed to parse response: %v", err)}
	}
	if response.Code != 0 {
		return "", &CodeExecutionError{
			Message: fmt.Sprintf("got error code: %d, message: %s", response.Code, response.Message),
		}
	}

	if response.Data.Error != nil && *response.Data.Error != "" {
		return "", &CodeExecutionError{Message: *response.Data.Error}
	}

	if response.Data.Stdout == nil {
		return "", nil
	}

	return *response.Data.Stdout, nil
}

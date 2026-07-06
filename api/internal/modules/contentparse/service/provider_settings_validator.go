package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const parserValidationTimeout = 10 * time.Second

type ParserProviderValidator interface {
	Validate(ctx context.Context, req ParserProviderValidationRequest) error
}

type ParserProviderValidationRequest struct {
	ProviderKey   string
	BaseURL       string
	TimeoutSec    int
	APIKey        string
	Mode          string
	OfficialToken string
}

type defaultParserProviderValidator struct{}

func (defaultParserProviderValidator) Validate(ctx context.Context, req ParserProviderValidationRequest) error {
	switch strings.ToLower(strings.TrimSpace(req.ProviderKey)) {
	case ParserProviderReducto:
		return validateReductoProvider(ctx, req)
	case ParserProviderMineru:
		if strings.EqualFold(strings.TrimSpace(req.Mode), MineruModeOfficial) {
			return validateMineruOfficialProvider(ctx, req)
		}
		return validateMineruSidecarProvider(ctx, req)
	default:
		return ErrUnsupportedParserProvider
	}
}

func validateReductoProvider(ctx context.Context, req ParserProviderValidationRequest) error {
	if strings.TrimSpace(req.APIKey) == "" {
		return fmt.Errorf("Reducto API key is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultReductoBaseURL
	}
	checkURL := baseURL + "/jobs?limit=1&exclude_configs=true"
	reqCtx, cancel := validationContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return fmt.Errorf("Reducto validation request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.APIKey))
	httpReq.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("Reducto validation request failed: %w", err)
	}
	defer resp.Body.Close()
	body := readValidationSnippet(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Reducto token rejected: HTTP %d %s", resp.StatusCode, validationErrorMessage(body))
	}
	return fmt.Errorf("Reducto validation failed: HTTP %d %s", resp.StatusCode, validationErrorMessage(body))
}

func validateMineruOfficialProvider(ctx context.Context, req ParserProviderValidationRequest) error {
	if strings.TrimSpace(req.OfficialToken) == "" {
		return fmt.Errorf("MinerU official token is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultMineruOfficialBaseURL
	}
	checkURL := baseURL + "/api/v4/extract/task/" + uuid.NewString()
	reqCtx, cancel := validationContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, checkURL, nil)
	if err != nil {
		return fmt.Errorf("MinerU validation request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(req.OfficialToken))
	httpReq.Header.Set("Accept", "*/*")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("MinerU official validation request failed: %w", err)
	}
	defer resp.Body.Close()
	body := readValidationSnippet(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("MinerU official token rejected: HTTP %d %s", resp.StatusCode, validationErrorMessage(body))
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("MinerU official validation failed: HTTP %d %s", resp.StatusCode, validationErrorMessage(body))
	}
	return nil
}

func validateMineruSidecarProvider(ctx context.Context, req ParserProviderValidationRequest) error {
	baseURL := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("MinerU API URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("MinerU API URL is invalid")
	}
	reqCtx, cancel := validationContext(ctx)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL, nil)
	if err != nil {
		return fmt.Errorf("MinerU sidecar validation request: %w", err)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("MinerU sidecar validation request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("MinerU sidecar validation failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func validationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, parserValidationTimeout)
}

func readValidationSnippet(reader io.Reader) []byte {
	if reader == nil {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(reader, 4096))
	return body
}

func validationErrorMessage(body []byte) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	var parsed struct {
		Detail string `json:"detail"`
		Msg    string `json:"msg"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		switch {
		case strings.TrimSpace(parsed.Detail) != "":
			text = parsed.Detail
		case strings.TrimSpace(parsed.Msg) != "":
			text = parsed.Msg
		case strings.TrimSpace(parsed.Error.Message) != "":
			text = parsed.Error.Message
		}
	}
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	return text
}

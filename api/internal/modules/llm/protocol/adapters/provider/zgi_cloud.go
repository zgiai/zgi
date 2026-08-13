package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	zgiCloudAdapterName       = "zgi-cloud"
	zgiCloudTranscriptionPath = "/audio/transcriptions"
	zgiCloudSpeechPath        = "/audio/speech"
	errUnsupportedFmt         = "%w: zgi-cloud adapter does not support %s"
	zgiCloudSpeechContentType = "audio/mpeg"
	zgiCloudSpeechFormat      = "mp3"

	headerSettlementID     = "X-ZGI-Settlement-ID"
	headerOfficialPoints   = "X-ZGI-Official-Points"
	headerRemainingBalance = "X-ZGI-Remaining-Balance"
	headerSettlementStatus = "X-ZGI-Settlement-Status"
	headerZGIRequestID     = "X-ZGI-Request-ID"
	headerZGIModelName     = "X-ZGI-Model-Name"

	eventZGISettlement      = "zgi.settlement"
	eventZGISettlementError = "zgi.settlement_error"
)

// ZGICloudAdapter forwards official traffic from api back to console internal endpoints.
type ZGICloudAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewZGICloudAdapter creates an adapter for the official console transport.
func NewZGICloudAdapter(config *adapter.AdapterConfig) (*ZGICloudAdapter, error) {
	if err := validateZGICloudConfig(config); err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &ZGICloudAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, maxRetries),
		baseURL:    config.BaseURL,
	}, nil
}

func validateZGICloudConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.BaseURL == "" {
		return fmt.Errorf("%w: base url is required", adapter.ErrInvalidConfig)
	}
	if config.APIKey == "" && config.AuthHook == nil {
		return fmt.Errorf("%w: api key or auth hook is required", adapter.ErrInvalidConfig)
	}
	return nil
}

func (a *ZGICloudAdapter) Name() string {
	return zgiCloudAdapterName
}

func (a *ZGICloudAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	url := fmt.Sprintf("%s/chat/completions", a.baseURL)
	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", url, a.buildHeaders(), buildOpenAICompatibleChatPayload(request))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	var response adapter.ChatResponse
	if err := json.Unmarshal(httpResp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	response.Settlement = settlementFromHeaders(httpResp.Header)
	return &response, nil
}

func (a *ZGICloudAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	request.Stream = true
	url := fmt.Sprintf("%s/chat/completions", a.baseURL)
	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, a.buildHeaders(), buildOpenAICompatibleChatPayload(request))
	if err != nil {
		return nil, handleZGICloudStreamError(err)
	}

	respChan := make(chan adapter.StreamResponse, 10)
	dataChan := make(chan string, 10)
	errChan := make(chan error, 1)

	go adapter.ParseSSE(resp.Body, dataChan, errChan)

	go func() {
		defer close(respChan)
		defer func() {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		var lastUsage *adapter.Usage
		var settlement *adapter.SettlementResult

		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{
					Error:      ctx.Err(),
					Done:       true,
					Usage:      lastUsage,
					Settlement: settlement,
				}
				return
			case err := <-errChan:
				if err != nil {
					respChan <- adapter.StreamResponse{
						Error:      err,
						Done:       true,
						Usage:      lastUsage,
						Settlement: settlement,
					}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					respChan <- adapter.StreamResponse{
						Done:       true,
						Usage:      lastUsage,
						Settlement: settlement,
					}
					return
				}

				if parsedSettlement := settlementFromRawData(data); parsedSettlement != nil {
					settlement = parsedSettlement
					continue
				}
				if parsedSettlementErr := settlementErrorFromRawData(data); parsedSettlementErr != nil {
					respChan <- adapter.StreamResponse{
						Error:      settlementErrorToError(parsedSettlementErr),
						Done:       true,
						Usage:      lastUsage,
						Settlement: settlement,
					}
					return
				}

				var streamResp adapter.StreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					respChan <- adapter.StreamResponse{
						Error:      fmt.Errorf("failed to parse stream data: %w", err),
						Done:       true,
						Usage:      lastUsage,
						Settlement: settlement,
					}
					return
				}

				if streamResp.Usage != nil {
					lastUsage = streamResp.Usage
				}

				respChan <- streamResp
			}
		}
	}()

	return respChan, nil
}

func (a *ZGICloudAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	// TODO: Official Responses traffic must use the raw /v1/responses transport.
	// Migrate legacy typed callers separately if they still need official routing.
	return nil, fmt.Errorf(errUnsupportedFmt, adapter.ErrCapabilityUnsupported, "responses")
}

func (a *ZGICloudAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/responses", a.baseURL)
	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", url, a.buildHeaders(), body)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	return &adapter.RawResponse{
		Body:       httpResp.Body,
		Usage:      openAIUsageFromRaw(httpResp.Body),
		Settlement: settlementFromHeaders(httpResp.Header),
	}, nil
}

func (a *ZGICloudAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/responses", a.baseURL)
	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, a.buildHeaders(), body)
	if err != nil {
		return nil, handleZGICloudStreamError(err)
	}

	return streamRawHTTPEvents(ctx, resp.Body, func(raw json.RawMessage, _ *adapter.Usage) *adapter.Usage {
		return openAIUsageFromRaw(raw)
	}), nil
}

func (a *ZGICloudAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/anthropic/v1/messages", a.baseURL)
	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", url, a.buildAnthropicHeaders(request.Headers), body)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	return &adapter.RawResponse{
		Body:       httpResp.Body,
		Usage:      anthropicUsageFromRaw(httpResp.Body, nil),
		Settlement: settlementFromHeaders(httpResp.Header),
	}, nil
}

func (a *ZGICloudAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/anthropic/v1/messages", a.baseURL)
	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, a.buildAnthropicHeaders(request.Headers), body)
	if err != nil {
		return nil, handleZGICloudStreamError(err)
	}

	return streamRawHTTPEvents(ctx, resp.Body, anthropicUsageFromRaw), nil
}

func handleZGICloudStreamError(err error) error {
	var statusErr *adapter.HTTPStatusError
	if errors.As(err, &statusErr) {
		return handleOpenAICompatibleError(statusErr.StatusCode, statusErr.Body)
	}
	return fmt.Errorf("stream request failed: %w", err)
}

func (a *ZGICloudAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	url := fmt.Sprintf("%s/embeddings", a.baseURL)
	payload, err := buildOpenAICompatibleEmbeddingsPayload(request)
	if err != nil {
		return nil, err
	}

	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", url, a.buildHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	var response adapter.EmbeddingsResponse
	if err := json.Unmarshal(httpResp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	response.Settlement = settlementFromHeaders(httpResp.Header)
	return &response, nil
}

func (a *ZGICloudAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	url := fmt.Sprintf("%s/images/generations", a.baseURL)
	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if request.N != nil {
		payload["n"] = *request.N
	}
	if request.Size != "" {
		payload["size"] = request.Size
	}
	if request.Quality != "" {
		payload["quality"] = request.Quality
	}
	if request.Style != "" {
		payload["style"] = request.Style
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = request.ResponseFormat
	}
	if request.User != "" {
		payload["user"] = request.User
	}
	for k, v := range request.AdditionalParameters {
		payload[k] = v
	}

	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", url, a.buildHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	var response adapter.ImageResponse
	if err := json.Unmarshal(httpResp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	response.Settlement = settlementFromHeaders(httpResp.Header)
	return &response, nil
}

func (a *ZGICloudAdapter) CreateVideo(ctx context.Context, request *adapter.VideoRequest) (*adapter.VideoResponse, error) {
	endpoint := fmt.Sprintf("%s/videos/generations", a.baseURL)
	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", endpoint, a.buildHeaders(), buildOpenAICompatibleVideoPayload(request))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	response, err := decodeOpenAICompatibleVideoResponse(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	response.Settlement = settlementFromHeaders(httpResp.Header)
	return response, nil
}

func (a *ZGICloudAdapter) GetVideoTask(ctx context.Context, request *adapter.VideoTaskRequest) (*adapter.VideoResponse, error) {
	if request == nil || strings.TrimSpace(request.TaskID) == "" {
		return nil, fmt.Errorf("%w: task_id is required", adapter.ErrInvalidRequest)
	}
	query := url.Values{}
	if strings.TrimSpace(request.Model) != "" {
		query.Set("model", strings.TrimSpace(request.Model))
	}
	for k, v := range request.AdditionalParameters {
		if text, ok := v.(string); ok && strings.TrimSpace(text) != "" {
			query.Set(k, strings.TrimSpace(text))
		}
	}
	endpoint := fmt.Sprintf("%s/videos/generations/%s", a.baseURL, url.PathEscape(strings.TrimSpace(request.TaskID)))
	if encoded := query.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "GET", endpoint, a.buildHeaders(), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	response, err := decodeOpenAICompatibleVideoResponse(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	response.Settlement = settlementFromHeaders(httpResp.Header)
	return response, nil
}

func (a *ZGICloudAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	url := fmt.Sprintf("%s/rerank", a.baseURL)
	payload := map[string]any{
		"model":     request.Model,
		"query":     request.Query,
		"documents": request.Documents,
	}
	if request.TopN != nil {
		payload["top_n"] = *request.TopN
	}
	if request.MaxTokensPerDoc != nil {
		payload["max_tokens_per_doc"] = *request.MaxTokensPerDoc
	} else if request.MaxChunksPerDoc != nil {
		payload["max_tokens_per_doc"] = *request.MaxChunksPerDoc
	}
	if request.ScoreThreshold != nil {
		payload["score_threshold"] = *request.ScoreThreshold
	}
	if request.Priority != nil {
		payload["priority"] = *request.Priority
	}
	if request.ReturnDocuments != nil {
		payload["return_documents"] = *request.ReturnDocuments
	}
	if len(request.RankFields) > 0 {
		payload["rank_fields"] = request.RankFields
	}

	httpResp, err := a.httpClient.DoRequestDetailed(ctx, "POST", url, a.buildHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleOpenAICompatibleError(httpResp.StatusCode, httpResp.Body)
	}

	var response adapter.RerankResponse
	if err := json.Unmarshal(httpResp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	response.Settlement = settlementFromHeaders(httpResp.Header)
	return &response, nil
}

// Transcribe streams PCM audio to Console and returns the final editable transcript.
// The request is sent exactly once because its body cannot be replayed safely.
func (a *ZGICloudAdapter) Transcribe(ctx context.Context, request *adapter.TranscriptionRequest) (*adapter.TranscriptionResponse, error) {
	if request == nil || strings.TrimSpace(request.RequestID) == "" || strings.TrimSpace(request.Model) == "" || request.Audio == nil {
		return nil, fmt.Errorf("%w: request id, model, and audio are required", adapter.ErrInvalidRequest)
	}

	headers := a.buildHeaders()
	headers["Content-Type"] = "audio/pcm"
	headers[headerZGIRequestID] = request.RequestID
	headers[headerZGIModelName] = request.Model

	httpResp, err := a.httpClient.DoRawRequestDetailed(
		ctx,
		http.MethodPost,
		a.baseURL+zgiCloudTranscriptionPath,
		headers,
		request.Audio,
	)
	if err != nil {
		return nil, fmt.Errorf("transcription request failed: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, handleZGICloudTranscriptionError(httpResp.StatusCode, httpResp.Body)
	}

	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			RequestID string `json:"request_id"`
			Text      string `json:"text"`
		} `json:"data"`
	}
	if err := json.Unmarshal(httpResp.Body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse transcription response: %w", err)
	}
	if envelope.Code != 0 {
		return nil, adapter.NewAdapterError("TRANSCRIPTION_FAILED", envelope.Message, http.StatusBadGateway, adapter.ErrUpstreamError)
	}
	if envelope.Data.RequestID != request.RequestID {
		return nil, fmt.Errorf("%w: transcription response request id mismatch", adapter.ErrUpstreamError)
	}

	return &adapter.TranscriptionResponse{
		RequestID: envelope.Data.RequestID,
		Text:      envelope.Data.Text,
	}, nil
}

// GenerateSpeech streams one MP3 response from Console into dst without retrying.
func (a *ZGICloudAdapter) GenerateSpeech(ctx context.Context, request *adapter.SpeechRequest, dst io.Writer) error {
	if request == nil ||
		strings.TrimSpace(request.RequestID) == "" ||
		strings.TrimSpace(request.Model) == "" ||
		strings.TrimSpace(request.Input) == "" ||
		strings.TrimSpace(request.Voice) == "" ||
		request.ResponseFormat != zgiCloudSpeechFormat ||
		dst == nil {
		return fmt.Errorf("%w: request id, model, input, voice, mp3 format, and destination are required", adapter.ErrInvalidRequest)
	}

	headers := a.buildHeaders()
	headers["Accept"] = zgiCloudSpeechContentType
	headers[headerZGIRequestID] = request.RequestID
	headers[headerZGIModelName] = request.Model

	resp, err := a.httpClient.DoStreamRequest(
		ctx,
		http.MethodPost,
		a.baseURL+zgiCloudSpeechPath,
		headers,
		request,
	)
	if err != nil {
		return handleZGICloudSpeechError(err)
	}
	defer resp.Body.Close()

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType != zgiCloudSpeechContentType {
		return fmt.Errorf("%w: speech response content type is not audio/mpeg", adapter.ErrUpstreamError)
	}
	written, err := io.Copy(dst, resp.Body)
	if err != nil {
		return fmt.Errorf("speech response stream failed: %w", err)
	}
	if written == 0 {
		return fmt.Errorf("%w: speech provider returned empty audio", adapter.ErrUpstreamError)
	}
	return nil
}

func handleZGICloudSpeechError(err error) error {
	var statusErr *adapter.HTTPStatusError
	if errors.As(err, &statusErr) {
		return handleZGICloudTranscriptionError(statusErr.StatusCode, statusErr.Body)
	}
	return fmt.Errorf("speech request failed: %w", err)
}

func handleZGICloudTranscriptionError(statusCode int, body []byte) error {
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return adapter.HandleNonJSONError(statusCode, body)
	}

	message := strings.TrimSpace(envelope.Message)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	code := strconv.Itoa(envelope.Code)

	switch statusCode {
	case http.StatusBadRequest:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrInvalidRequest)
	case http.StatusPaymentRequired:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrInsufficientBalance)
	case http.StatusNotFound:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrModelNotFound)
	case 499:
		return adapter.NewAdapterError(code, message, statusCode, context.Canceled)
	case http.StatusServiceUnavailable:
		return adapter.NewAdapterError(adapter.ErrorCodePlatformChannelUnavailable, message, statusCode, adapter.ErrPlatformChannelUnavailable)
	case http.StatusGatewayTimeout:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrTimeout)
	default:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrUpstreamError)
	}
}

func (a *ZGICloudAdapter) ListModels(context.Context, string) ([]adapter.Model, error) {
	return nil, fmt.Errorf(errUnsupportedFmt, adapter.ErrCapabilityUnsupported, "model listing")
}

func (a *ZGICloudAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf(errUnsupportedFmt, adapter.ErrCapabilityUnsupported, "balance")
}

func (a *ZGICloudAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateZGICloudConfig(config)
}

func (a *ZGICloudAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         zgiCloudAdapterName,
		Type:         zgiCloudAdapterName,
		DisplayName:  "ZGI Cloud",
		Description:  "Official console transport adapter",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "image", "embedding", "rerank", "transcription", "speech_generation"},
		Version:      "v1",
	}
}

func (a *ZGICloudAdapter) buildHeaders() map[string]string {
	headers := make(map[string]string, len(a.config.Headers))
	for k, v := range a.config.Headers {
		headers[k] = v
	}
	return headers
}

func (a *ZGICloudAdapter) buildAnthropicHeaders(requestHeaders map[string]string) map[string]string {
	headers := a.buildHeaders()
	for k, v := range requestHeaders {
		headers[k] = v
	}
	return headers
}

func settlementFromHeaders(headers http.Header) *adapter.SettlementResult {
	if headers == nil {
		return nil
	}
	settlementID := strings.TrimSpace(headers.Get(headerSettlementID))
	status := strings.TrimSpace(headers.Get(headerSettlementStatus))
	pointsRaw := strings.TrimSpace(headers.Get(headerOfficialPoints))
	if settlementID == "" && status == "" && pointsRaw == "" {
		return nil
	}
	points, _ := strconv.ParseInt(pointsRaw, 10, 64)
	remaining, _ := strconv.ParseInt(strings.TrimSpace(headers.Get(headerRemainingBalance)), 10, 64)
	return &adapter.SettlementResult{
		SettlementID:     settlementID,
		OfficialPoints:   points,
		RemainingBalance: remaining,
		Status:           status,
	}
}

func settlementFromRawData(data string) *adapter.SettlementResult {
	raw := strings.TrimSpace(data)
	if raw == "" || !strings.Contains(raw, "settlement_id") || !strings.Contains(raw, "official_points") {
		return nil
	}
	var settlement adapter.SettlementResult
	if err := json.Unmarshal([]byte(raw), &settlement); err != nil {
		return nil
	}
	if settlement.SettlementID == "" && settlement.Status == "" {
		return nil
	}
	return &settlement
}

func settlementErrorFromRawData(data string) *adapter.SettlementError {
	raw := strings.TrimSpace(data)
	if raw == "" || !strings.Contains(raw, "message") || !strings.Contains(raw, "status") {
		return nil
	}
	var settlementErr adapter.SettlementError
	if err := json.Unmarshal([]byte(raw), &settlementErr); err != nil {
		return nil
	}
	if settlementErr.Message == "" && settlementErr.Code == "" {
		return nil
	}
	return &settlementErr
}

func settlementErrorToError(settlementErr *adapter.SettlementError) error {
	if settlementErr == nil {
		return fmt.Errorf("console proxy settlement failed")
	}
	message := strings.TrimSpace(settlementErr.Message)
	if message == "" {
		message = strings.TrimSpace(settlementErr.Code)
	}
	if message == "" {
		message = "unknown settlement error"
	}
	return fmt.Errorf("console proxy settlement failed: %s", message)
}

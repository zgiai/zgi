package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	doubaoSpeechProviderName      = "doubao-speech"
	doubaoAudioDefaultBaseURL     = "https://openspeech.bytedance.com"
	doubaoAudioBaseURLParam       = "audio_base_url"
	doubaoAudioAPIPrefix          = "/api/v3"
	doubaoSpeechPath              = "/api/v3/tts/unidirectional"
	doubaoSpeechFinishCode        = 20000000
	doubaoSpeechSampleRate        = 24000
	doubaoHeaderAPIKey            = "X-Api-Key"
	doubaoHeaderResourceID        = "X-Api-Resource-Id"
	doubaoHeaderRequireUsage      = "X-Control-Require-Usage-Tokens-Return"
	doubaoSpeechRequireAllUsage   = "*"
	doubaoSpeechResponseFormatMP3 = "mp3"
)

type doubaoSpeechPayload struct {
	Request doubaoSpeechRequest `json:"req_params"`
}

type doubaoSpeechRequest struct {
	Text        string                  `json:"text"`
	Speaker     string                  `json:"speaker"`
	AudioParams doubaoSpeechAudioParams `json:"audio_params"`
}

type doubaoSpeechAudioParams struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
}

type doubaoSpeechFrame struct {
	Code    *int   `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// GenerateSpeech uses Volcengine Speech V3's unidirectional chunked protocol.
// Source: https://docs.volcengine.com/docs/6561/2528925?lang=zh
func (a *DoubaoAdapter) GenerateSpeech(
	ctx context.Context,
	request *adapter.SpeechRequest,
	dst io.Writer,
) (*adapter.SettlementResult, error) {
	if request == nil ||
		strings.TrimSpace(request.Model) == "" ||
		strings.TrimSpace(request.Input) == "" ||
		strings.TrimSpace(request.Voice) == "" ||
		request.ResponseFormat != doubaoSpeechResponseFormatMP3 ||
		dst == nil {
		return nil, fmt.Errorf("%w: model, input, voice, mp3 format, and destination are required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(a.config.APIKey) == "" {
		return nil, fmt.Errorf("%w: doubao api key is required", adapter.ErrInvalidConfig)
	}

	response, err := a.httpClient.DoStreamRequest(
		ctx,
		http.MethodPost,
		resolveDoubaoAudioEndpoint(a.config, doubaoSpeechPath),
		doubaoAudioHeaders(a.config, request.Model),
		doubaoSpeechPayload{Request: doubaoSpeechRequest{
			Text:    request.Input,
			Speaker: request.Voice,
			AudioParams: doubaoSpeechAudioParams{
				Format:     request.ResponseFormat,
				SampleRate: doubaoSpeechSampleRate,
			},
		}},
	)
	if err != nil {
		return nil, normalizeDoubaoAudioHTTPError(err)
	}
	defer response.Body.Close()

	decoder := json.NewDecoder(response.Body)
	audioSeen := false
	finished := false
	for {
		var frame doubaoSpeechFrame
		if err := decoder.Decode(&frame); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("%w: decode doubao speech frame: %v", adapter.ErrUpstreamError, err)
		}
		if frame.Code == nil {
			return nil, fmt.Errorf("%w: doubao speech frame contained no code", adapter.ErrUpstreamError)
		}
		if *frame.Code != 0 && *frame.Code != doubaoSpeechFinishCode {
			return nil, adapter.NewAdapterError(
				fmt.Sprintf("%d", *frame.Code),
				strings.TrimSpace(frame.Message),
				response.StatusCode,
				adapter.ErrUpstreamError,
			)
		}
		if frame.Data != "" {
			chunk, err := base64.StdEncoding.DecodeString(frame.Data)
			if err != nil {
				return nil, fmt.Errorf("%w: decode doubao speech audio: %v", adapter.ErrUpstreamError, err)
			}
			if _, err := io.Copy(dst, bytes.NewReader(chunk)); err != nil {
				return nil, fmt.Errorf("write doubao speech audio: %w", err)
			}
			audioSeen = true
		}
		if *frame.Code == doubaoSpeechFinishCode {
			finished = true
			break
		}
	}

	if !finished {
		return nil, fmt.Errorf("%w: doubao speech stream ended before final frame", adapter.ErrUpstreamError)
	}
	if !audioSeen {
		return nil, fmt.Errorf("%w: doubao speech response contained no audio", adapter.ErrUpstreamError)
	}
	return nil, nil
}

func resolveDoubaoAudioEndpoint(config *adapter.AdapterConfig, path string) string {
	baseURL := doubaoAudioBaseURL(config)
	if strings.HasSuffix(baseURL, doubaoAudioAPIPrefix) {
		path = strings.TrimPrefix(path, doubaoAudioAPIPrefix)
	}
	return baseURL + path
}

func doubaoAudioBaseURL(config *adapter.AdapterConfig) string {
	if config != nil && config.CustomParams != nil {
		if configured, ok := config.CustomParams[doubaoAudioBaseURLParam].(string); ok && strings.TrimSpace(configured) != "" {
			return strings.TrimRight(strings.TrimSpace(configured), "/")
		}
	}
	if config != nil && config.ProviderName == doubaoSpeechProviderName && strings.TrimSpace(config.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	}
	return doubaoAudioDefaultBaseURL
}

func doubaoAudioHeaders(config *adapter.AdapterConfig, model string) map[string]string {
	headers := make(map[string]string, len(config.Headers)+3)
	for key, value := range config.Headers {
		headers[key] = value
	}
	headers[doubaoHeaderAPIKey] = strings.TrimSpace(config.APIKey)
	headers[doubaoHeaderResourceID] = strings.TrimSpace(model)
	headers[doubaoHeaderRequireUsage] = doubaoSpeechRequireAllUsage
	return headers
}

func normalizeDoubaoAudioHTTPError(err error) error {
	var statusErr *adapter.HTTPStatusError
	if !errors.As(err, &statusErr) {
		return err
	}
	baseErr := adapter.ErrUpstreamError
	switch statusErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		baseErr = adapter.ErrAuthFailed
	case http.StatusTooManyRequests:
		baseErr = adapter.ErrRateLimited
	}
	return adapter.NewAdapterError(
		"DOUBAO_AUDIO_HTTP_ERROR",
		fmt.Sprintf("doubao audio request failed with status %d", statusErr.StatusCode),
		statusErr.StatusCode,
		baseErr,
	)
}

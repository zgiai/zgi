package agents

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	agentSpeechJSONContentType   = "application/json"
	agentSpeechMPEGContentType   = "audio/mpeg"
	agentSpeechMaxRequestBytes   = 64 * 1024
	agentSpeechMaxInputRunes     = 5000
	agentSpeechGenerationTimeout = 65 * time.Second
)

var errAgentSpeechInputTooLarge = errors.New("agent speech input is too large")

type agentSpeechRequest struct {
	Input string `json:"input"`
}

func (h *AgentsHandler) GenerateAgentSpeech(c *gin.Context) {
	scope, _, _, ok := h.agentDraftRuntimeAccess(c)
	if !ok {
		return
	}
	h.generateSpeech(c, scope.OrganizationID.String())
}

func (h *AgentsHandler) GenerateWebAppAgentSpeech(c *gin.Context) {
	scope, _, _, _, ok := h.webAppAgentRuntimeAccess(c)
	if !ok {
		return
	}
	h.generateSpeech(c, scope.OrganizationID.String())
}

func (h *AgentsHandler) generateSpeech(c *gin.Context, organizationID string) {
	if h.speechService == nil {
		writeVoiceError(c, http.StatusInternalServerError, "SPEECH_NOT_CONFIGURED", "Speech generation is not configured")
		return
	}
	request, err := decodeAgentSpeechRequest(c.Request)
	if errors.Is(err, errAgentSpeechInputTooLarge) {
		writeVoiceError(c, http.StatusRequestEntityTooLarge, "SPEECH_INPUT_TOO_LARGE", "Speech input exceeds the 5,000-character limit")
		return
	}
	if err != nil {
		writeVoiceError(c, http.StatusBadRequest, "INVALID_SPEECH_REQUEST", "A non-empty speech input is required")
		return
	}

	c.Header("Content-Type", agentSpeechMPEGContentType)
	stream := &agentSpeechStreamWriter{dst: c.Writer}
	speechCtx, cancel := context.WithTimeout(c.Request.Context(), agentSpeechGenerationTimeout)
	defer cancel()
	err = h.speechService.Generate(speechCtx, organizationID, request.Input, stream)
	if err == nil && stream.wrote {
		return
	}
	if err == nil {
		err = errors.New("speech generation returned no audio")
	}
	if stream.wrote {
		_ = c.Error(err)
		c.Abort()
		return
	}

	c.Writer.Header().Del("Content-Type")
	handleSpeechError(c, err)
}

func decodeAgentSpeechRequest(request *http.Request) (agentSpeechRequest, error) {
	if request == nil || request.Body == nil {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != agentSpeechJSONContentType {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}

	body, err := io.ReadAll(io.LimitReader(request.Body, agentSpeechMaxRequestBytes+1))
	if err != nil {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}
	if len(body) > agentSpeechMaxRequestBytes {
		return agentSpeechRequest{}, errAgentSpeechInputTooLarge
	}
	if !utf8.Valid(body) {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var speechRequest agentSpeechRequest
	if err := decoder.Decode(&speechRequest); err != nil {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}
	speechRequest.Input = strings.TrimSpace(speechRequest.Input)
	if speechRequest.Input == "" || !utf8.ValidString(speechRequest.Input) {
		return agentSpeechRequest{}, adapter.ErrInvalidRequest
	}
	if utf8.RuneCountInString(speechRequest.Input) > agentSpeechMaxInputRunes {
		return agentSpeechRequest{}, errAgentSpeechInputTooLarge
	}
	return speechRequest, nil
}

type agentSpeechStreamWriter struct {
	dst   gin.ResponseWriter
	wrote bool
}

func (w *agentSpeechStreamWriter) Write(chunk []byte) (int, error) {
	written, err := w.dst.Write(chunk)
	if written > 0 {
		w.wrote = true
		w.dst.Flush()
	}
	return written, err
}

func handleSpeechError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		writeVoiceError(c, statusClientClosedRequest, "REQUEST_CANCELLED", "Speech generation was cancelled")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, adapter.ErrTimeout):
		writeVoiceError(c, http.StatusGatewayTimeout, "SPEECH_TIMEOUT", "Speech generation timed out")
	case errors.Is(err, adapter.ErrInvalidRequest):
		writeVoiceError(c, http.StatusBadRequest, "INVALID_SPEECH_REQUEST", "Invalid speech generation request")
	case errors.Is(err, gateway.ErrInsufficientBalance), errors.Is(err, adapter.ErrInsufficientBalance):
		writeVoiceError(c, http.StatusPaymentRequired, "INSUFFICIENT_BALANCE", "Insufficient balance for speech generation")
	case errors.Is(err, gateway.ErrInsufficientQuota):
		writeVoiceError(c, http.StatusTooManyRequests, "INSUFFICIENT_QUOTA", "Speech generation quota is exhausted")
	case errors.Is(err, ErrSpeechUnavailable), errors.Is(err, llmdefaultservice.ErrModelUnavailable), errors.Is(err, llmdefaultservice.ErrDefaultModelNotFound), errors.Is(err, gateway.ErrNoProviderAvailable), errors.Is(err, adapter.ErrPlatformChannelUnavailable), errors.Is(err, adapter.ErrQuotaExhausted):
		writeVoiceError(c, http.StatusServiceUnavailable, "SPEECH_UNAVAILABLE", "Speech generation is temporarily unavailable")
	default:
		writeVoiceError(c, http.StatusBadGateway, "SPEECH_GENERATION_FAILED", "Speech generation failed")
	}
}

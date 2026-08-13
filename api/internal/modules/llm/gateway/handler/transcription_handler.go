package handler

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

const (
	transcriptionPCMContentType = "audio/pcm"
	statusClientClosedRequest   = 499
)

type transcriptionService interface {
	Transcribe(context.Context, *apikeymodel.TenantAPIKey, *gateway.TranscriptionRequest) (*gateway.TranscriptionResponse, error)
}

// TranscriptionHandler accepts PCM audio and returns final editable text.
type TranscriptionHandler struct {
	service transcriptionService
}

// NewTranscriptionHandler creates the public transcription handler.
func NewTranscriptionHandler(service transcriptionService) *TranscriptionHandler {
	return &TranscriptionHandler{service: service}
}

// Transcribe handles POST /v1/audio/transcriptions?model=...
func (h *TranscriptionHandler) Transcribe(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	model := strings.TrimSpace(c.Query("model"))
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if model == "" || err != nil || mediaType != transcriptionPCMContentType || c.Request.Body == nil || c.Request.ContentLength == 0 {
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Model and non-empty audio/pcm body are required"))
		return
	}

	result, err := h.service.Transcribe(c.Request.Context(), apiKey, &gateway.TranscriptionRequest{
		Model: model,
		Audio: c.Request.Body,
	})
	if err != nil {
		writeTranscriptionProtocolError(c, err)
		return
	}
	if result == nil {
		writeOpenAIProtocolError(c, internalProtocolError())
		return
	}

	c.JSON(http.StatusOK, result)
}

func writeTranscriptionProtocolError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		writeOpenAIProtocolError(c, newProtocolError(
			statusClientClosedRequest,
			"server_error",
			"request_cancelled",
			"api_error",
			"Transcription request was cancelled",
		))
	case errors.Is(err, context.DeadlineExceeded):
		writeOpenAIProtocolError(c, newProtocolError(
			http.StatusGatewayTimeout,
			"server_error",
			"upstream_timeout",
			"timeout_error",
			"Transcription timed out",
		))
	default:
		writeOpenAIProtocolError(c, classifyProtocolError(err))
	}
}

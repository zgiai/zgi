package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

const (
	speechJSONContentType     = "application/json"
	speechMPEGContentType     = "audio/mpeg"
	speechResponseFormatMP3   = "mp3"
	speechMaxRequestBodyBytes = 64 << 10
)

type speechService interface {
	GenerateSpeech(context.Context, *apikeymodel.TenantAPIKey, *gateway.SpeechRequest, io.Writer) error
}

// SpeechHandler accepts complete text and streams generated MP3 audio.
type SpeechHandler struct {
	service speechService
}

// NewSpeechHandler creates the public speech generation handler.
func NewSpeechHandler(service speechService) *SpeechHandler {
	return &SpeechHandler{service: service}
}

// Generate handles POST /v1/audio/speech.
func (h *SpeechHandler) Generate(c *gin.Context) {
	apiKey, ok := apiKeyFromContext(c)
	if !ok {
		return
	}

	if c.Request != nil && c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, speechMaxRequestBodyBytes)
	}
	request, ok := decodeSpeechRequest(c.Request)
	if !ok {
		writeOpenAIProtocolError(c, invalidRequestProtocolError("Model, input, voice, and mp3 response format are required"))
		return
	}

	c.Header("Content-Type", speechMPEGContentType)
	stream := &speechStreamWriter{dst: c.Writer}
	err := h.service.GenerateSpeech(c.Request.Context(), apiKey, &request, stream)
	if err == nil {
		return
	}
	if stream.wrote {
		recordStreamServiceError(c, err)
		c.Abort()
		return
	}

	c.Writer.Header().Del("Content-Type")
	recordServiceError(c, err)
	writeSpeechProtocolError(c, err)
}

func decodeSpeechRequest(request *http.Request) (gateway.SpeechRequest, bool) {
	if request == nil || request.Body == nil {
		return gateway.SpeechRequest{}, false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != speechJSONContentType {
		return gateway.SpeechRequest{}, false
	}

	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var speechRequest gateway.SpeechRequest
	if err := decoder.Decode(&speechRequest); err != nil {
		return gateway.SpeechRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return gateway.SpeechRequest{}, false
	}
	if strings.TrimSpace(speechRequest.Model) == "" ||
		strings.TrimSpace(speechRequest.Input) == "" ||
		strings.TrimSpace(speechRequest.Voice) == "" ||
		speechRequest.ResponseFormat != speechResponseFormatMP3 {
		return gateway.SpeechRequest{}, false
	}
	return speechRequest, true
}

type speechStreamWriter struct {
	dst   gin.ResponseWriter
	wrote bool
}

func (w *speechStreamWriter) Write(chunk []byte) (int, error) {
	written, err := w.dst.Write(chunk)
	if written > 0 {
		w.wrote = true
		w.dst.Flush()
	}
	return written, err
}

func writeSpeechProtocolError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		writeOpenAIProtocolError(c, newProtocolError(
			statusClientClosedRequest,
			"server_error",
			"request_cancelled",
			"api_error",
			"Speech generation request was cancelled",
		))
	case errors.Is(err, context.DeadlineExceeded):
		writeOpenAIProtocolError(c, newProtocolError(
			http.StatusGatewayTimeout,
			"server_error",
			"upstream_timeout",
			"timeout_error",
			"Speech generation timed out",
		))
	default:
		writeOpenAIProtocolError(c, classifyProtocolError(err))
	}
}

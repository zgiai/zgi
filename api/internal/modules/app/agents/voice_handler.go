package agents

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	agentVoicePCMContentType        = "audio/pcm"
	agentVoicePCMSampleRate         = 16000
	agentVoicePCMBytesPerSample     = 2
	agentVoicePCMBytesPerSecond     = agentVoicePCMSampleRate * agentVoicePCMBytesPerSample
	maxAgentVoicePCMBytes           = 60 * agentVoicePCMBytesPerSecond
	agentVoiceTranscriptionOverhead = 20 * time.Second
	statusClientClosedRequest       = 499
)

func (h *AgentsHandler) TranscribeAgentVoice(c *gin.Context) {
	scope, _, _, ok := h.agentDraftRuntimeAccess(c)
	if !ok {
		return
	}
	h.transcribeVoice(c, scope.OrganizationID.String())
}

func (h *AgentsHandler) TranscribeWebAppAgentVoice(c *gin.Context) {
	scope, _, _, _, ok := h.webAppAgentRuntimeAccess(c)
	if !ok {
		return
	}
	h.transcribeVoice(c, scope.OrganizationID.String())
}

func (h *AgentsHandler) transcribeVoice(c *gin.Context, organizationID string) {
	if h.voiceService == nil {
		writeVoiceError(c, http.StatusInternalServerError, "VOICE_NOT_CONFIGURED", "Voice transcription is not configured")
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != agentVoicePCMContentType || c.Request.Body == nil {
		writeVoiceError(c, http.StatusBadRequest, "INVALID_AUDIO", "A non-empty audio/pcm body is required")
		return
	}
	audio, err := io.ReadAll(io.LimitReader(c.Request.Body, maxAgentVoicePCMBytes+1))
	if err != nil {
		writeVoiceError(c, http.StatusBadRequest, "INVALID_AUDIO", "Failed to read PCM audio")
		return
	}
	if len(audio) > maxAgentVoicePCMBytes {
		writeVoiceError(c, http.StatusRequestEntityTooLarge, "AUDIO_TOO_LARGE", "PCM audio exceeds the 60-second limit")
		return
	}
	if len(audio) == 0 {
		writeVoiceError(c, http.StatusBadRequest, "INVALID_AUDIO", "A non-empty audio/pcm body is required")
		return
	}
	if len(audio)%agentVoicePCMBytesPerSample != 0 {
		writeVoiceError(c, http.StatusBadRequest, "INVALID_AUDIO", "PCM audio must contain complete 16-bit samples")
		return
	}

	transcriptionCtx, cancel := context.WithTimeout(c.Request.Context(), agentVoiceTranscriptionTimeout(len(audio)))
	defer cancel()
	result, err := h.voiceService.Transcribe(transcriptionCtx, organizationID, bytes.NewReader(audio))
	if err != nil {
		handleVoiceError(c, err)
		return
	}
	response.Success(c, result)
}

func handleVoiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		writeVoiceError(c, statusClientClosedRequest, "REQUEST_CANCELLED", "Voice transcription was cancelled")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, adapter.ErrTimeout):
		writeVoiceError(c, http.StatusGatewayTimeout, "TRANSCRIPTION_TIMEOUT", "Voice transcription timed out")
	case errors.Is(err, ErrNoSpeechDetected):
		writeVoiceError(c, http.StatusUnprocessableEntity, "NO_SPEECH_DETECTED", "No speech was detected")
	case errors.Is(err, adapter.ErrInvalidRequest):
		writeVoiceError(c, http.StatusBadRequest, "INVALID_AUDIO", "Invalid PCM audio")
	case errors.Is(err, gateway.ErrInsufficientBalance), errors.Is(err, adapter.ErrInsufficientBalance):
		writeVoiceError(c, http.StatusPaymentRequired, "INSUFFICIENT_BALANCE", "Insufficient balance for voice transcription")
	case errors.Is(err, gateway.ErrInsufficientQuota):
		writeVoiceError(c, http.StatusTooManyRequests, "INSUFFICIENT_QUOTA", "Voice transcription quota is exhausted")
	case errors.Is(err, ErrVoiceUnavailable), errors.Is(err, llmdefaultservice.ErrModelUnavailable), errors.Is(err, llmdefaultservice.ErrDefaultModelNotFound), errors.Is(err, gateway.ErrNoProviderAvailable), errors.Is(err, adapter.ErrPlatformChannelUnavailable), errors.Is(err, adapter.ErrQuotaExhausted):
		writeVoiceError(c, http.StatusServiceUnavailable, "VOICE_UNAVAILABLE", "Voice transcription is temporarily unavailable")
	default:
		writeVoiceError(c, http.StatusBadGateway, "TRANSCRIPTION_FAILED", "Voice transcription failed")
	}
}

func agentVoiceTranscriptionTimeout(audioBytes int) time.Duration {
	audioDuration := time.Duration(audioBytes) * time.Second / time.Duration(agentVoicePCMBytesPerSecond)
	return audioDuration + agentVoiceTranscriptionOverhead
}

func writeVoiceError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"code": code, "message": message})
}

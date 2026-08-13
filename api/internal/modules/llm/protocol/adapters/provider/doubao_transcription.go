package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	doubaoHeaderRequestID              = "X-Api-Request-Id"
	doubaoTranscriptionChunkPeriod     = 200 * time.Millisecond
	doubaoTranscriptionHandshakeTimout = 10 * time.Second
	doubaoTranscriptionFinalTimeout    = 5 * time.Second
)

type doubaoTranscriptionSendResult struct {
	lastSequence int32
	err          error
}

type doubaoTranscriptionReceiveResult struct {
	frame doubaoTranscriptionFrame
	err   error
}

// Transcribe uses Volcengine Speech V3's streaming-input WebSocket protocol.
// Source: https://www.volcengine.com/docs/6561/1354869?lang=zh
func (a *DoubaoAdapter) Transcribe(ctx context.Context, request *adapter.TranscriptionRequest) (*adapter.TranscriptionResponse, error) {
	if request == nil || strings.TrimSpace(request.Model) == "" || request.Audio == nil {
		return nil, fmt.Errorf("%w: model and PCM audio are required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(a.config.APIKey) == "" {
		return nil, fmt.Errorf("%w: doubao api key is required", adapter.ErrInvalidConfig)
	}
	requestID := strings.TrimSpace(request.RequestID)
	if requestID == "" {
		requestID = uuid.NewString()
	}
	endpoint, err := resolveDoubaoAudioWebSocketEndpoint(a.config, doubaoTranscriptionPath)
	if err != nil {
		return nil, err
	}
	if err := adapter.ValidateOutboundBaseURL(ctx, doubaoAudioBaseURL(a.config), a.config); err != nil {
		return nil, err
	}

	headers := http.Header{}
	for key, value := range a.config.Headers {
		headers.Set(key, value)
	}
	headers.Set(doubaoHeaderAPIKey, strings.TrimSpace(a.config.APIKey))
	headers.Set(doubaoHeaderResourceID, strings.TrimSpace(request.Model))
	headers.Set(doubaoHeaderRequestID, requestID)
	dialer := &websocket.Dialer{
		HandshakeTimeout: doubaoTranscriptionHandshakeTimout,
		Proxy:            http.ProxyFromEnvironment,
		NetDialContext:   adapter.OutboundDialContext(a.config),
	}
	connection, response, err := dialer.DialContext(ctx, endpoint, headers)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, fmt.Errorf("%w: connect doubao streaming transcription: %v", adapter.ErrUpstreamError, err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() {
		_ = connection.Close()
	})
	defer stopCancellation()

	fullRequest, err := encodeDoubaoTranscriptionFullRequest()
	if err != nil {
		return nil, err
	}
	if err := writeDoubaoTranscriptionFrame(connection, fullRequest); err != nil {
		return nil, doubaoTranscriptionContextError(ctx, "send full transcription request", err)
	}
	if _, err := readDoubaoTranscriptionFrame(connection); err != nil {
		return nil, doubaoTranscriptionContextError(ctx, "read full transcription response", err)
	}

	final, err := streamDoubaoTranscriptionAudio(ctx, connection, request.Audio)
	if err != nil {
		return nil, err
	}
	return &adapter.TranscriptionResponse{
		RequestID: requestID,
		Text:      final.Payload.Result.Text,
	}, nil
}

func streamDoubaoTranscriptionAudio(ctx context.Context, connection *websocket.Conn, audio io.Reader) (doubaoTranscriptionFrame, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sent := make(chan doubaoTranscriptionSendResult, 1)
	received := make(chan doubaoTranscriptionReceiveResult, 1)
	go func() {
		sequence, err := sendDoubaoTranscriptionAudio(streamCtx, connection, audio)
		sent <- doubaoTranscriptionSendResult{lastSequence: sequence, err: err}
	}()
	go func() {
		frame, err := receiveDoubaoTranscriptionFinal(connection)
		received <- doubaoTranscriptionReceiveResult{frame: frame, err: err}
	}()

	var sendResult *doubaoTranscriptionSendResult
	var receiveResult *doubaoTranscriptionReceiveResult
	var finalTimer *time.Timer
	var finalTimeout <-chan time.Time
	defer func() {
		if finalTimer != nil {
			finalTimer.Stop()
		}
	}()
	for sendResult == nil || receiveResult == nil {
		select {
		case result := <-sent:
			sendResult = &result
			if result.err != nil {
				abortDoubaoTranscription(cancel, connection, audio)
				return doubaoTranscriptionFrame{}, result.err
			}
			finalTimer = time.NewTimer(doubaoTranscriptionFinalTimeout)
			finalTimeout = finalTimer.C
		case result := <-received:
			receiveResult = &result
			if result.err != nil {
				abortDoubaoTranscription(cancel, connection, audio)
				return doubaoTranscriptionFrame{}, doubaoTranscriptionContextError(ctx, "read transcription result", result.err)
			}
		case <-ctx.Done():
			abortDoubaoTranscription(cancel, connection, audio)
			return doubaoTranscriptionFrame{}, fmt.Errorf("stream transcription audio: %w", ctx.Err())
		case <-finalTimeout:
			abortDoubaoTranscription(cancel, connection, audio)
			return doubaoTranscriptionFrame{}, fmt.Errorf("%w: doubao transcription final result timed out", adapter.ErrTimeout)
		}
	}
	wantSequence := -sendResult.lastSequence
	if receiveResult.frame.Sequence != wantSequence {
		return doubaoTranscriptionFrame{}, fmt.Errorf(
			"%w: doubao final transcription sequence = %d, want %d",
			adapter.ErrUpstreamError,
			receiveResult.frame.Sequence,
			wantSequence,
		)
	}
	return receiveResult.frame, nil
}

func abortDoubaoTranscription(cancel context.CancelFunc, connection *websocket.Conn, audio io.Reader) {
	cancel()
	_ = connection.Close()
	if closer, ok := audio.(io.Closer); ok {
		_ = closer.Close()
	}
}

func sendDoubaoTranscriptionAudio(ctx context.Context, connection *websocket.Conn, audio io.Reader) (int32, error) {
	startedAt := time.Now()
	chunkSize := doubaoTranscriptionSampleRate * (doubaoTranscriptionBits / 8) * doubaoTranscriptionChannels * int(doubaoTranscriptionChunkPeriod/time.Millisecond) / 1000
	current, reachedEOF, err := readDoubaoTranscriptionAudioChunk(audio, chunkSize)
	if err != nil {
		return 0, doubaoTranscriptionContextError(ctx, "read transcription audio", err)
	}
	if len(current) == 0 {
		return 0, fmt.Errorf("%w: transcription audio is required", adapter.ErrInvalidRequest)
	}

	for index := 0; ; index++ {
		last := reachedEOF
		var next []byte
		if !last {
			next, reachedEOF, err = readDoubaoTranscriptionAudioChunk(audio, chunkSize)
			if err != nil {
				return 0, doubaoTranscriptionContextError(ctx, "read transcription audio", err)
			}
			last = len(next) == 0 && reachedEOF
		}
		if err := waitDoubaoTranscriptionDeadline(ctx, startedAt, index); err != nil {
			return 0, err
		}
		sequence := int32(index + 2)
		if last {
			sequence = -sequence
		}
		frame, err := encodeDoubaoTranscriptionAudio(sequence, current, last)
		if err != nil {
			return 0, err
		}
		if err := writeDoubaoTranscriptionFrame(connection, frame); err != nil {
			return 0, doubaoTranscriptionContextError(ctx, "send transcription audio", err)
		}
		if last {
			return sequence, nil
		}
		current = next
	}
}

func receiveDoubaoTranscriptionFinal(connection *websocket.Conn) (doubaoTranscriptionFrame, error) {
	for {
		frame, err := readDoubaoTranscriptionFrame(connection)
		if err != nil {
			return doubaoTranscriptionFrame{}, err
		}
		if frame.Last {
			return frame, nil
		}
	}
}

func readDoubaoTranscriptionAudioChunk(reader io.Reader, chunkSize int) ([]byte, bool, error) {
	chunk := make([]byte, chunkSize)
	read, err := io.ReadFull(reader, chunk)
	switch {
	case err == nil:
		return chunk, false, nil
	case err == io.EOF:
		return nil, true, nil
	case err == io.ErrUnexpectedEOF:
		return chunk[:read], true, nil
	default:
		return nil, false, err
	}
}

func waitDoubaoTranscriptionDeadline(ctx context.Context, startedAt time.Time, index int) error {
	delay := time.Until(startedAt.Add(time.Duration(index) * doubaoTranscriptionChunkPeriod))
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("pace transcription audio: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func doubaoTranscriptionContextError(ctx context.Context, action string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", action, ctxErr)
	}
	return fmt.Errorf("%w: %s: %w", adapter.ErrUpstreamError, action, err)
}

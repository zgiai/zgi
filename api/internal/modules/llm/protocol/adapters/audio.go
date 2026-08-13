package adapter

import (
	"context"
	"io"
)

// TranscriptionCapable defines speech-to-text capability.
type TranscriptionCapable interface {
	Transcribe(ctx context.Context, request *TranscriptionRequest) (*TranscriptionResponse, error)
}

// TranscriptionRequest carries a single PCM stream. The adapter consumes Audio
// synchronously and must not retry after dispatch because the stream is not replayable.
type TranscriptionRequest struct {
	RequestID string
	Model     string
	Audio     io.Reader
}

// TranscriptionResponse contains the final editable transcript.
type TranscriptionResponse struct {
	RequestID string `json:"request_id"`
	Text      string `json:"text"`
}

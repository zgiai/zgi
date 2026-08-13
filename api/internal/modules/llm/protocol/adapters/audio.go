package adapter

import (
	"context"
	"errors"
	"io"
)

const (
	// MaxMusicPromptRunes is the ZGI music request prompt ceiling.
	MaxMusicPromptRunes = 2000
	// MaxMusicLyricsRunes is the ZGI music request lyrics ceiling.
	MaxMusicLyricsRunes = 3500
	// MaxGeneratedMusicBytes is the ZGI music delivery ceiling.
	MaxGeneratedMusicBytes int64 = 64 << 20
)

var (
	ErrMusicStreamIncomplete       = errors.New("music stream did not complete")
	ErrMusicResponseTooLarge       = errors.New("music response exceeds size limit")
	ErrMusicCompensationNotFound   = errors.New("music billing record not found")
	ErrMusicCompensationNotReady   = errors.New("music billing record is not ready for compensation")
	ErrMusicCompensationNotCharged = errors.New("music request reached a terminal state without a charge")
)

// TranscriptionCapable defines speech-to-text capability.
type TranscriptionCapable interface {
	Transcribe(ctx context.Context, request *TranscriptionRequest) (*TranscriptionResponse, error)
}

// SpeechCapable defines text-to-speech capability.
type SpeechCapable interface {
	GenerateSpeech(ctx context.Context, request *SpeechRequest, dst io.Writer) error
}

// MusicCapable defines complete MP3 music generation. Implementations only
// return nil after the upstream stream has reached its explicit success marker.
type MusicCapable interface {
	GenerateMusic(ctx context.Context, request *MusicRequest, dst io.Writer) error
}

// MusicCompensationCapable resolves billing after the trusted caller cannot
// durably deliver a generated file. It distinguishes refunded and no-charge terminal states.
type MusicCompensationCapable interface {
	CompensateMusicDelivery(ctx context.Context, requestID string) error
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

// SpeechRequest carries one complete text input for an MP3 response stream.
type SpeechRequest struct {
	RequestID      string `json:"-"`
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
}

type MusicMode string

const (
	MusicModeVocal        MusicMode = "vocal"
	MusicModeAutoLyrics   MusicMode = "auto_lyrics"
	MusicModeInstrumental MusicMode = "instrumental"
)

// MusicRequest carries one complete music generation request.
type MusicRequest struct {
	RequestID      string    `json:"-"`
	Model          string    `json:"model"`
	Mode           MusicMode `json:"mode"`
	Prompt         string    `json:"prompt"`
	Lyrics         string    `json:"lyrics"`
	ResponseFormat string    `json:"response_format"`
}

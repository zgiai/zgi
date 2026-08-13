package music

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	messageGenerationOutcomeUnknown = "Music generation outcome is unknown; billing reconciliation is in progress"
	messageFileStorageFailed        = "Failed to store music file; refund is pending"
	messageResultDeliveryFailed     = "Failed to deliver music result; refund is pending"
	messageNoCharge                 = "The music task was not charged or the reserved credits were released"
	messageDeliveryRefunded         = "Failed to deliver music result; the charge has been refunded"
	messageTaskInterrupted          = "Music task execution was interrupted; billing reconciliation is in progress"
	messageLyricsGenerationFailed   = "Failed to generate lyrics; please try again"
)

// Generator is the model-generation capability consumed by the Music worker.
type Generator interface {
	GenerateMusic(context.Context, string, *adapter.MusicRequest, io.Writer) error
}

// LyricsGenerator creates complete lyrics before the separately billed music call.
type LyricsGenerator interface {
	GenerateLyrics(context.Context, string, *adapter.LyricsRequest) (*adapter.LyricsResult, error)
}

// DeliveryCompensator resolves billing when a generated track cannot be delivered.
type DeliveryCompensator interface {
	CompensateMusicDelivery(context.Context, string, string) error
}

type Worker struct {
	repo        Repository
	dispatcher  Dispatcher
	generator   Generator
	lyrics      LyricsGenerator
	compensator DeliveryCompensator
	assets      AssetStore
}

func NewWorker(repo Repository, dispatcher Dispatcher, generator Generator, lyrics LyricsGenerator, compensator DeliveryCompensator, assets AssetStore) *Worker {
	if repo == nil || dispatcher == nil || generator == nil || lyrics == nil || compensator == nil || assets == nil {
		panic("music worker requires repository, dispatcher, music and lyrics generators, delivery compensator, and asset store")
	}
	return &Worker{repo: repo, dispatcher: dispatcher, generator: generator, lyrics: lyrics, compensator: compensator, assets: assets}
}

func (w *Worker) Generate(ctx context.Context, id uuid.UUID) error {
	task, err := w.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	switch task.Status {
	case StatusSucceeded, StatusFailed:
		return nil
	case StatusCompensationPending:
		return w.dispatcher.EnqueueCompensation(ctx, id)
	case StatusGeneratingLyrics:
	case StatusGenerating:
		return w.recoverUnknownDelivery(ctx, task)
	case StatusQueued:
		next := StatusGenerating
		if task.Mode == adapter.MusicModeAutoLyrics {
			next = StatusGeneratingLyrics
		}
		if err := w.repo.Transition(ctx, id, StatusQueued, next, TaskUpdate{}); err != nil {
			return err
		}
	default:
		return ErrInvalidTransition
	}
	if task.Mode == adapter.MusicModeAutoLyrics {
		result, err := w.lyrics.GenerateLyrics(ctx, task.OrganizationID.String(), &adapter.LyricsRequest{
			RequestID: task.ID.String(),
			Model:     task.Model,
			Prompt:    task.Prompt,
		})
		if err != nil || result == nil {
			return w.repo.Transition(ctx, task.ID, StatusGeneratingLyrics, StatusFailed, TaskUpdate{
				ErrorCode:    ErrorCodeLyricsGenerationFailed,
				ErrorMessage: messageLyricsGenerationFailed,
			})
		}
		generated := GeneratedLyrics{
			Title:     result.Title,
			StyleTags: append([]string(nil), result.StyleTags...),
			Lyrics:    result.Lyrics,
		}
		if !validGeneratedLyrics(generated) {
			return w.repo.Transition(ctx, task.ID, StatusGeneratingLyrics, StatusFailed, TaskUpdate{
				ErrorCode:    ErrorCodeLyricsGenerationFailed,
				ErrorMessage: messageLyricsGenerationFailed,
			})
		}
		if err := w.repo.UpdateGeneratedLyrics(ctx, task.ID, generated); err != nil {
			return err
		}
		if err := w.repo.Transition(ctx, task.ID, StatusGeneratingLyrics, StatusGenerating, TaskUpdate{}); err != nil {
			return err
		}
		task.Title = generated.Title
		task.StyleTags = append(task.StyleTags[:0], generated.StyleTags...)
		task.Lyrics = generated.Lyrics
	}

	audio := newMusicBuffer(adapter.MaxGeneratedMusicBytes)
	mode := task.Mode
	if mode == adapter.MusicModeAutoLyrics {
		mode = adapter.MusicModeVocal
	}
	err = w.generator.GenerateMusic(ctx, task.OrganizationID.String(), &adapter.MusicRequest{
		RequestID:      task.ID.String(),
		Model:          task.Model,
		Mode:           mode,
		Prompt:         task.Prompt,
		Lyrics:         task.Lyrics,
		ResponseFormat: task.ResponseFormat,
	}, audio)
	if err != nil {
		return w.beginCompensation(ctx, task.ID, ErrorCodeDeliveryUnknown, messageGenerationOutcomeUnknown)
	}
	fileID, err := w.assets.Save(ctx, task, audio.Bytes())
	if err != nil {
		return w.beginCompensation(ctx, task.ID, ErrorCodeDeliveryFailed, messageFileStorageFailed)
	}
	parsedFileID, err := uuid.Parse(fileID)
	if err != nil {
		_ = w.assets.Delete(ctx, fileID)
		return w.beginCompensation(ctx, task.ID, ErrorCodeDeliveryFailed, messageFileStorageFailed)
	}
	if err := w.repo.Transition(ctx, task.ID, StatusGenerating, StatusSucceeded, TaskUpdate{
		FileID: &parsedFileID,
	}); err != nil {
		if current, getErr := w.repo.Get(ctx, task.ID); getErr == nil && current.Status == StatusSucceeded {
			return nil
		}
		if deleteErr := w.assets.Delete(ctx, fileID); deleteErr != nil {
			logger.ErrorContext(ctx, "failed to delete undelivered music file", deleteErr, "task_id", task.ID.String())
		}
		return w.beginCompensation(ctx, task.ID, ErrorCodeDeliveryFailed, messageResultDeliveryFailed)
	}
	return nil
}

func validGeneratedLyrics(generated GeneratedLyrics) bool {
	title := strings.TrimSpace(generated.Title)
	lyrics := strings.TrimSpace(generated.Lyrics)
	return title != "" && lyrics != "" && utf8.ValidString(generated.Title) && utf8.ValidString(generated.Lyrics) &&
		utf8.RuneCountInString(title) <= 255 && utf8.RuneCountInString(lyrics) <= adapter.MaxMusicLyricsRunes
}

type musicBuffer struct {
	buffer    bytes.Buffer
	remaining int64
}

func newMusicBuffer(limit int64) *musicBuffer {
	if limit <= 0 {
		panic("music buffer requires a positive limit")
	}
	return &musicBuffer{remaining: limit}
}

func (b *musicBuffer) Write(chunk []byte) (int, error) {
	if int64(len(chunk)) <= b.remaining {
		written, err := b.buffer.Write(chunk)
		b.remaining -= int64(written)
		return written, err
	}
	written, err := b.buffer.Write(chunk[:b.remaining])
	b.remaining -= int64(written)
	if err != nil {
		return written, err
	}
	return written, adapter.ErrMusicResponseTooLarge
}

func (b *musicBuffer) Bytes() []byte { return b.buffer.Bytes() }

func (b *musicBuffer) String() string { return b.buffer.String() }

func (w *Worker) Compensate(ctx context.Context, id uuid.UUID) error {
	task, err := w.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if task.Status == StatusFailed {
		return nil
	}
	if task.Status == StatusSucceeded {
		return nil
	}
	if task.Status != StatusCompensationPending {
		return ErrInvalidTransition
	}
	if err := w.compensator.CompensateMusicDelivery(ctx, task.OrganizationID.String(), task.ID.String()); errors.Is(err, adapter.ErrMusicCompensationNotFound) || errors.Is(err, adapter.ErrMusicCompensationNotCharged) {
		return w.repo.Transition(ctx, task.ID, StatusCompensationPending, StatusFailed, TaskUpdate{
			ErrorCode:    ErrorCodeGenerationFailed,
			ErrorMessage: messageNoCharge,
		})
	} else if err != nil {
		touchErr := w.repo.TouchStatus(ctx, task.ID, StatusCompensationPending)
		return errors.Join(fmt.Errorf("compensate music delivery: %w", err), touchErr)
	}
	return w.repo.Transition(ctx, task.ID, StatusCompensationPending, StatusFailed, TaskUpdate{
		ErrorCode:    ErrorCodeDeliveryFailedRefunded,
		ErrorMessage: messageDeliveryRefunded,
	})
}

func (w *Worker) recoverUnknownDelivery(ctx context.Context, task *Task) error {
	return w.beginCompensation(ctx, task.ID, ErrorCodeDeliveryUnknown, messageTaskInterrupted)
}

func (w *Worker) beginCompensation(ctx context.Context, id uuid.UUID, code, message string) error {
	if err := w.repo.Transition(ctx, id, StatusGenerating, StatusCompensationPending, TaskUpdate{
		ErrorCode:    code,
		ErrorMessage: message,
	}); err != nil {
		if !errors.Is(err, ErrInvalidTransition) {
			return err
		}
		current, getErr := w.repo.Get(ctx, id)
		if getErr != nil {
			return getErr
		}
		switch current.Status {
		case StatusCompensationPending:
		case StatusSucceeded, StatusFailed:
			return nil
		default:
			return ErrInvalidTransition
		}
	}
	return w.dispatcher.EnqueueCompensation(ctx, id)
}

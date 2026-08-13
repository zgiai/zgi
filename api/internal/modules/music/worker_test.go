package music

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestWorkerCompletesTaskAfterFullMusicIsStored(t *testing.T) {
	task := queuedTask()
	repo := newMemoryRepository(task)
	generator := &musicGeneratorStub{audio: []byte("complete-mp3")}
	compensator := &deliveryCompensatorStub{}
	assets := &assetStoreStub{fileID: uuid.NewString()}
	worker := NewWorker(repo, &dispatcherStub{}, generator, &lyricsGeneratorStub{}, compensator, assets)

	if err := worker.Generate(t.Context(), task.ID); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusSucceeded || got.FileID == nil || got.FileID.String() != assets.fileID {
		t.Fatalf("completed task = %#v", got)
	}
	if !bytes.Equal(assets.saved, generator.audio) {
		t.Fatalf("saved audio = %q, want %q", assets.saved, generator.audio)
	}
	if compensator.calls != 0 {
		t.Fatalf("compensation calls = %d, want 0", compensator.calls)
	}
	if got.DurationMS != 0 || len(got.WaveformPeaks) != 0 {
		t.Fatalf("server decoded playback metadata = %#v, want empty", got)
	}
}

func TestWorkerGeneratesAndPersistsLyricsBeforeMusic(t *testing.T) {
	task := queuedTask()
	task.Mode = adapter.MusicModeAutoLyrics
	repo := newMemoryRepository(task)
	lyrics := &lyricsGeneratorStub{result: &adapter.LyricsResult{
		Title:     "雨后的木吉他",
		StyleTags: []string{"Folk", "Warm"},
		Lyrics:    "[Verse]\n雨停在黄昏以后",
	}}
	generator := &musicGeneratorStub{audio: []byte("complete-mp3")}
	worker := NewWorker(repo, &dispatcherStub{}, generator, lyrics, &deliveryCompensatorStub{}, &assetStoreStub{fileID: uuid.NewString()})

	if err := worker.Generate(t.Context(), task.ID); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusSucceeded || got.Title != lyrics.result.Title || got.Lyrics != lyrics.result.Lyrics || !reflect.DeepEqual([]string(got.StyleTags), lyrics.result.StyleTags) {
		t.Fatalf("completed auto-lyrics task = %#v", got)
	}
	if lyrics.calls != 1 || lyrics.request == nil || lyrics.request.RequestID != task.ID.String() || lyrics.request.Model != task.Model || lyrics.request.Prompt != task.Prompt {
		t.Fatalf("lyrics request = %#v", lyrics)
	}
	if generator.request == nil || generator.request.Mode != adapter.MusicModeVocal || generator.request.Lyrics != lyrics.result.Lyrics {
		t.Fatalf("music request = %#v", generator.request)
	}
}

func TestWorkerFailsWithoutCompensationWhenLyricsGenerationFails(t *testing.T) {
	task := queuedTask()
	task.Mode = adapter.MusicModeAutoLyrics
	repo := newMemoryRepository(task)
	dispatcher := &dispatcherStub{}
	generator := &musicGeneratorStub{}
	lyrics := &lyricsGeneratorStub{err: errors.New("lyrics provider unavailable")}
	compensator := &deliveryCompensatorStub{}
	worker := NewWorker(repo, dispatcher, generator, lyrics, compensator, &assetStoreStub{})

	if err := worker.Generate(t.Context(), task.ID); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusFailed || got.ErrorCode != ErrorCodeLyricsGenerationFailed {
		t.Fatalf("failed auto-lyrics task = %#v", got)
	}
	if generator.calls != 0 || compensator.calls != 0 || len(dispatcher.compensatedIDs) != 0 {
		t.Fatalf("calls after lyrics failure = music %d, compensation %d/%d", generator.calls, compensator.calls, len(dispatcher.compensatedIDs))
	}
}

func TestGeneratedLyricsRejectRawValuesOutsideProductLimits(t *testing.T) {
	for name, generated := range map[string]GeneratedLyrics{
		"title": {
			Title:  strings.Repeat(" ", 255) + "title",
			Lyrics: "lyrics",
		},
		"lyrics": {
			Title:  "title",
			Lyrics: strings.Repeat(" ", adapter.MaxMusicLyricsRunes) + "lyrics",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if validGeneratedLyrics(generated) {
				t.Fatalf("validGeneratedLyrics(%s) = true, want false", name)
			}
		})
	}
}

func TestMusicBufferRejectsDataBeyondLimit(t *testing.T) {
	buffer := newMusicBuffer(4)
	written, err := io.Copy(buffer, io.LimitReader(strings.NewReader("12345"), 5))
	if !errors.Is(err, adapter.ErrMusicResponseTooLarge) {
		t.Fatalf("Copy() error = %v, want ErrMusicResponseTooLarge", err)
	}
	if written != 4 || buffer.String() != "1234" {
		t.Fatalf("Copy() = %d, buffer = %q; want 4 and %q", written, buffer.String(), "1234")
	}
}

func TestGeneratedMusicProductLimitIs64MiB(t *testing.T) {
	if adapter.MaxGeneratedMusicBytes != 64<<20 {
		t.Fatalf("MaxGeneratedMusicBytes = %d, want %d", adapter.MaxGeneratedMusicBytes, 64<<20)
	}
}

func TestWorkerQueuesCompensationWhenGenerationBillingOutcomeIsUnknown(t *testing.T) {
	task := queuedTask()
	repo := newMemoryRepository(task)
	dispatcher := &dispatcherStub{}
	generator := &musicGeneratorStub{err: errors.New("provider failed")}
	worker := NewWorker(repo, dispatcher, generator, &lyricsGeneratorStub{}, &deliveryCompensatorStub{}, &assetStoreStub{})

	if err := worker.Generate(t.Context(), task.ID); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusCompensationPending || got.ErrorCode != ErrorCodeDeliveryUnknown {
		t.Fatalf("pending task = %#v", got)
	}
	if got, want := got.ErrorMessage, "Music generation outcome is unknown; billing reconciliation is in progress"; got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
	if dispatcher.compensated != task.ID {
		t.Fatalf("compensation task = %s, want %s", dispatcher.compensated, task.ID)
	}
}

func TestWorkerQueuesCompensationAfterDurableStorageFailure(t *testing.T) {
	task := queuedTask()
	repo := newMemoryRepository(task)
	dispatcher := &dispatcherStub{}
	generator := &musicGeneratorStub{audio: []byte("complete-mp3")}
	compensator := &deliveryCompensatorStub{}
	worker := NewWorker(repo, dispatcher, generator, &lyricsGeneratorStub{}, compensator, &assetStoreStub{saveErr: errors.New("storage unavailable")})

	if err := worker.Generate(t.Context(), task.ID); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusCompensationPending || got.ErrorCode != ErrorCodeDeliveryFailed {
		t.Fatalf("task = %#v", got)
	}
	if got, want := got.ErrorMessage, "Failed to store music file; refund is pending"; got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
	if dispatcher.compensated != task.ID {
		t.Fatalf("compensation task = %s, want %s", dispatcher.compensated, task.ID)
	}
	if compensator.calls != 0 {
		t.Fatalf("generation worker compensated inline: calls = %d", compensator.calls)
	}
}

func TestWorkerNeverRegeneratesTaskLeftInGeneratingState(t *testing.T) {
	task := queuedTask()
	task.Status = StatusGenerating
	repo := newMemoryRepository(task)
	dispatcher := &dispatcherStub{}
	generator := &musicGeneratorStub{audio: []byte("must-not-run")}
	worker := NewWorker(repo, dispatcher, generator, &lyricsGeneratorStub{}, &deliveryCompensatorStub{}, &assetStoreStub{})

	if err := worker.Generate(t.Context(), task.ID); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if generator.calls != 0 {
		t.Fatalf("generate calls = %d, want 0", generator.calls)
	}
	if repo.tasks[task.ID].Status != StatusCompensationPending || dispatcher.compensated != task.ID {
		t.Fatalf("recovered task = %#v", repo.tasks[task.ID])
	}
}

func TestWorkerCompensationMarksRefundedDeliveryFailure(t *testing.T) {
	task := queuedTask()
	task.Status = StatusCompensationPending
	task.ErrorCode = ErrorCodeDeliveryFailed
	repo := newMemoryRepository(task)
	generator := &musicGeneratorStub{}
	compensator := &deliveryCompensatorStub{}
	worker := NewWorker(repo, &dispatcherStub{}, generator, &lyricsGeneratorStub{}, compensator, &assetStoreStub{})

	if err := worker.Compensate(t.Context(), task.ID); err != nil {
		t.Fatalf("Compensate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusFailed || got.ErrorCode != ErrorCodeDeliveryFailedRefunded {
		t.Fatalf("compensated task = %#v", got)
	}
	if got, want := got.ErrorMessage, "Failed to deliver music result; the charge has been refunded"; got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
	if generator.calls != 0 || compensator.calls != 1 {
		t.Fatalf("generate calls = %d, compensate calls = %d", generator.calls, compensator.calls)
	}
}

func TestWorkerCompensationTreatsMissingDeductionAsNoCharge(t *testing.T) {
	task := queuedTask()
	task.Status = StatusCompensationPending
	repo := newMemoryRepository(task)
	compensator := &deliveryCompensatorStub{err: adapter.ErrMusicCompensationNotFound}
	worker := NewWorker(repo, &dispatcherStub{}, &musicGeneratorStub{}, &lyricsGeneratorStub{}, compensator, &assetStoreStub{})

	if err := worker.Compensate(t.Context(), task.ID); err != nil {
		t.Fatalf("Compensate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusFailed || got.ErrorCode != ErrorCodeGenerationFailed {
		t.Fatalf("task = %#v", got)
	}
	if got, want := got.ErrorMessage, "The music task was not charged or the reserved credits were released"; got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
}

func TestWorkerCompensationTreatsTerminalRollbackAsNoCharge(t *testing.T) {
	task := queuedTask()
	task.Status = StatusCompensationPending
	repo := newMemoryRepository(task)
	compensator := &deliveryCompensatorStub{err: adapter.ErrMusicCompensationNotCharged}
	worker := NewWorker(repo, &dispatcherStub{}, &musicGeneratorStub{}, &lyricsGeneratorStub{}, compensator, &assetStoreStub{})

	if err := worker.Compensate(t.Context(), task.ID); err != nil {
		t.Fatalf("Compensate() error = %v", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusFailed || got.ErrorCode != ErrorCodeGenerationFailed {
		t.Fatalf("task = %#v", got)
	}
}

func TestWorkerCompensationRetryRotatesPendingTask(t *testing.T) {
	task := queuedTask()
	task.Status = StatusCompensationPending
	repo := newMemoryRepository(task)
	compensator := &deliveryCompensatorStub{err: adapter.ErrMusicCompensationNotReady}
	worker := NewWorker(repo, &dispatcherStub{}, &musicGeneratorStub{}, &lyricsGeneratorStub{}, compensator, &assetStoreStub{})

	err := worker.Compensate(t.Context(), task.ID)
	if !errors.Is(err, adapter.ErrMusicCompensationNotReady) {
		t.Fatalf("Compensate() error = %v, want ErrMusicCompensationNotReady", err)
	}
	got := repo.tasks[task.ID]
	if got.Status != StatusCompensationPending {
		t.Fatalf("task status = %s, want %s", got.Status, StatusCompensationPending)
	}
	if !got.UpdatedAt.After(task.UpdatedAt) {
		t.Fatalf("updated_at = %s, want after %s", got.UpdatedAt, task.UpdatedAt)
	}
}

func queuedTask() *Task {
	scope := testScope()
	return &Task{
		ID:             uuid.New(),
		RequestID:      uuid.New(),
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		AccountID:      scope.AccountID,
		Model:          "music-3.0",
		Mode:           "instrumental",
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
		Status:         StatusQueued,
	}
}

type musicGeneratorStub struct {
	audio   []byte
	err     error
	calls   int
	request *adapter.MusicRequest
}

func (s *musicGeneratorStub) GenerateMusic(_ context.Context, _ string, request *adapter.MusicRequest, dst io.Writer) error {
	s.calls++
	if request != nil {
		copy := *request
		s.request = &copy
	}
	if s.err != nil {
		return s.err
	}
	_, err := dst.Write(s.audio)
	return err
}

type lyricsGeneratorStub struct {
	result  *adapter.LyricsResult
	err     error
	calls   int
	request *adapter.LyricsRequest
}

func (s *lyricsGeneratorStub) GenerateLyrics(_ context.Context, _ string, request *adapter.LyricsRequest) (*adapter.LyricsResult, error) {
	s.calls++
	if request != nil {
		copy := *request
		s.request = &copy
	}
	return s.result, s.err
}

type deliveryCompensatorStub struct {
	err   error
	calls int
}

func (s *deliveryCompensatorStub) CompensateMusicDelivery(context.Context, string, string) error {
	s.calls++
	return s.err
}

type assetStoreStub struct {
	fileID                string
	saved                 []byte
	saveErr               error
	deletedStoredObjectID string
	deleteStoredObjectErr error
}

func (s *assetStoreStub) Save(_ context.Context, _ *Task, audio []byte) (string, error) {
	s.saved = append([]byte(nil), audio...)
	if s.saveErr != nil {
		return "", s.saveErr
	}
	return s.fileID, nil
}

func (s *assetStoreStub) Delete(context.Context, string) error { return nil }
func (s *assetStoreStub) DeleteStoredObject(_ context.Context, fileID string) error {
	s.deletedStoredObjectID = fileID
	return s.deleteStoredObjectErr
}
func (s *assetStoreStub) URL(context.Context, string) (string, error) {
	return "https://files.example/music.mp3", nil
}

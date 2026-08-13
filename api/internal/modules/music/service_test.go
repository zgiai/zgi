package music

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestServiceCreateQueuesValidatedMusicTask(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := &dispatcherStub{}
	model := "music-3.0"
	service := NewService(repo, dispatcher, &availableModelsStub{models: []*llmmodelsvc.AvailableModel{{
		Name: model,
		Endpoints: llmmodel.ModelEndpoints{
			MusicGeneration: true,
		},
	}}}, &assetStoreStub{})
	scope := testScope()

	requestID := uuid.New()
	task, err := service.Create(t.Context(), scope, CreateRequest{
		RequestID: requestID,
		Model:     model,
		Mode:      adapter.MusicModeInstrumental,
		Prompt:    "warm piano",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.ID == uuid.Nil || task.Status != StatusQueued || task.ResponseFormat != "mp3" {
		t.Fatalf("task = %#v", task)
	}
	if task.ID == requestID || task.RequestID != requestID {
		t.Fatalf("task identity = %s, request identity = %s", task.ID, task.RequestID)
	}
	if dispatcher.generated != task.ID {
		t.Fatalf("enqueued task = %s, want %s", dispatcher.generated, task.ID)
	}
	if got := repo.tasks[task.ID].OrganizationID; got != scope.OrganizationID {
		t.Fatalf("organization id = %s, want %s", got, scope.OrganizationID)
	}
}

func TestServiceCreateRejectsUnavailableMusicModelBeforeInsert(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo, &dispatcherStub{}, &availableModelsStub{}, &assetStoreStub{})
	_, err := service.Create(t.Context(), testScope(), CreateRequest{
		RequestID: uuid.New(),
		Model:     "missing",
		Mode:      adapter.MusicModeInstrumental,
		Prompt:    "warm piano",
	})
	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("Create() error = %v, want ErrModelUnavailable", err)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("created tasks = %d, want 0", len(repo.tasks))
	}
}

func TestServiceCreateMarksTaskFailedWhenQueueIsUnavailable(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := &dispatcherStub{generateErr: errors.New("redis unavailable")}
	service := NewService(repo, dispatcher, availableMusicModelStub(), &assetStoreStub{})
	_, err := service.Create(t.Context(), testScope(), CreateRequest{
		RequestID: uuid.New(),
		Model:     "music-3.0",
		Mode:      adapter.MusicModeInstrumental,
		Prompt:    "warm piano",
	})
	if err == nil {
		t.Fatal("Create() error = nil")
	}
	for _, task := range repo.tasks {
		if task.Status != StatusFailed || task.ErrorCode != ErrorCodeQueueUnavailable {
			t.Fatalf("task after enqueue failure = %#v", task)
		}
		if got, want := task.ErrorMessage, "Music task queue is unavailable; please try again"; got != want {
			t.Fatalf("error message = %q, want %q", got, want)
		}
	}
}

func TestServiceCreateIsIdempotentForSameRequest(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := &dispatcherStub{}
	service := NewService(repo, dispatcher, availableMusicModelStub(), &assetStoreStub{})
	scope := testScope()
	request := CreateRequest{
		RequestID: uuid.New(),
		Model:     "music-3.0",
		Mode:      adapter.MusicModeInstrumental,
		Prompt:    "warm piano",
	}

	first, err := service.Create(t.Context(), scope, request)
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	second, err := service.Create(t.Context(), scope, request)
	if err != nil {
		t.Fatalf("second Create() error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("task IDs = %s and %s, want identical", first.ID, second.ID)
	}
	if dispatcher.generateCalls != 1 {
		t.Fatalf("generation enqueue calls = %d, want 1", dispatcher.generateCalls)
	}
	if got := len(repo.tasks); got != 1 {
		t.Fatalf("created tasks = %d, want 1", got)
	}
}

func TestServiceCreateRejectsReusedRequestIDWithDifferentPayload(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := &dispatcherStub{}
	service := NewService(repo, dispatcher, availableMusicModelStub(), &assetStoreStub{})
	scope := testScope()
	request := CreateRequest{
		RequestID: uuid.New(),
		Model:     "music-3.0",
		Mode:      adapter.MusicModeInstrumental,
		Prompt:    "warm piano",
	}

	if _, err := service.Create(t.Context(), scope, request); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	request.Prompt = "cold piano"
	if _, err := service.Create(t.Context(), scope, request); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("second Create() error = %v, want ErrTaskConflict", err)
	}
	if dispatcher.generateCalls != 1 {
		t.Fatalf("generation enqueue calls = %d, want 1", dispatcher.generateCalls)
	}
}

func TestServiceCreateRequiresRequestID(t *testing.T) {
	service := NewService(newMemoryRepository(), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	_, err := service.Create(t.Context(), testScope(), CreateRequest{
		Model:  "music-3.0",
		Mode:   adapter.MusicModeInstrumental,
		Prompt: "warm piano",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Create() error = %v, want ErrInvalidRequest", err)
	}
}

func TestServiceCreateRequiresPromptForVocalMusic(t *testing.T) {
	repo := newMemoryRepository()
	dispatcher := &dispatcherStub{}
	service := NewService(repo, dispatcher, availableMusicModelStub(), &assetStoreStub{})
	_, err := service.Create(t.Context(), testScope(), CreateRequest{
		RequestID: uuid.New(),
		Model:     "music-3.0",
		Mode:      adapter.MusicModeVocal,
		Lyrics:    "[Verse] hello",
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Create() error = %v, want ErrInvalidRequest", err)
	}
	if len(repo.tasks) != 0 || dispatcher.generateCalls != 0 {
		t.Fatalf("invalid request side effects = %d tasks, %d enqueues; want 0, 0", len(repo.tasks), dispatcher.generateCalls)
	}
}

func TestServiceListsOwnedTasksWithNormalizedPagination(t *testing.T) {
	scope := testScope()
	newer := queuedTask()
	newer.OrganizationID = scope.OrganizationID
	newer.WorkspaceID = scope.WorkspaceID
	newer.AccountID = scope.AccountID
	newer.Prompt = "newer"
	newer.CreatedAt = newer.CreatedAt.Add(time.Hour)
	newer.UpdatedAt = newer.CreatedAt
	older := queuedTask()
	older.OrganizationID = scope.OrganizationID
	older.WorkspaceID = scope.WorkspaceID
	older.AccountID = scope.AccountID
	older.Prompt = "older"
	other := queuedTask()
	other.OrganizationID = scope.OrganizationID
	other.WorkspaceID = scope.WorkspaceID

	service := NewService(newMemoryRepository(older, newer, other), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	result, err := service.List(t.Context(), scope, ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if result.Page != 1 || result.PageSize != defaultMusicTaskPageSize || result.Total != 2 {
		t.Fatalf("List() result = %#v", result)
	}
	if len(result.Items) != 2 || result.Items[0].ID != newer.ID || result.Items[1].ID != older.ID {
		t.Fatalf("List() items = %#v, want newest owned tasks", result.Items)
	}
}

func TestNormalizeListRequestRejectsOffsetOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	_, err := normalizeListRequest(testScope(), ListRequest{Page: maxInt, PageSize: maxMusicTaskPageSize})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("normalizeListRequest() error = %v, want ErrInvalidRequest", err)
	}
}

func TestServiceCreateAcceptsMusicTextAtProductLimit(t *testing.T) {
	tests := []CreateRequest{
		{
			RequestID: uuid.New(),
			Model:     "music-3.0",
			Mode:      adapter.MusicModeInstrumental,
			Prompt:    strings.Repeat("界", adapter.MaxMusicPromptRunes),
		},
		{
			RequestID: uuid.New(),
			Model:     "music-3.0",
			Mode:      adapter.MusicModeVocal,
			Prompt:    "warm vocal",
			Lyrics:    strings.Repeat("界", adapter.MaxMusicLyricsRunes),
		},
	}

	for _, request := range tests {
		service := NewService(newMemoryRepository(), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
		if _, err := service.Create(t.Context(), testScope(), request); err != nil {
			t.Fatalf("Create() error = %v at product text limit", err)
		}
	}
}

func TestServiceCreateRejectsOversizedMusicTextBeforeInsert(t *testing.T) {
	tests := []CreateRequest{
		{
			RequestID: uuid.New(),
			Model:     "music-3.0",
			Mode:      adapter.MusicModeInstrumental,
			Prompt:    strings.Repeat("界", adapter.MaxMusicPromptRunes+1),
		},
		{
			RequestID: uuid.New(),
			Model:     "music-3.0",
			Mode:      adapter.MusicModeVocal,
			Lyrics:    strings.Repeat("界", adapter.MaxMusicLyricsRunes+1),
		},
	}

	for _, request := range tests {
		repo := newMemoryRepository()
		dispatcher := &dispatcherStub{}
		service := NewService(repo, dispatcher, availableMusicModelStub(), &assetStoreStub{})
		if _, err := service.Create(t.Context(), testScope(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("Create() error = %v, want ErrInvalidRequest", err)
		}
		if len(repo.tasks) != 0 || dispatcher.generateCalls != 0 {
			t.Fatalf("oversized request side effects = %d tasks, %d enqueues; want 0, 0", len(repo.tasks), dispatcher.generateCalls)
		}
	}
}

func TestNewServiceRejectsMissingAssetStore(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewService() did not panic for missing asset store")
		}
	}()
	NewService(newMemoryRepository(), &dispatcherStub{}, availableMusicModelStub(), nil)
}

type availableModelsStub struct {
	models []*llmmodelsvc.AvailableModel
	err    error
}

func (s *availableModelsStub) ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelsvc.AvailableModel, error) {
	return s.models, s.err
}

func availableMusicModelStub() *availableModelsStub {
	return &availableModelsStub{models: []*llmmodelsvc.AvailableModel{{
		Name: "music-3.0",
		Endpoints: llmmodel.ModelEndpoints{
			MusicGeneration: true,
		},
	}}}
}

func testScope() Scope {
	return Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), AccountID: uuid.New()}
}

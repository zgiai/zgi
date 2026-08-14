package music

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

const musicResponseFormat = "mp3"

const (
	defaultMusicTaskPageSize = 20
	maxMusicTaskPageSize     = 100
	maxMusicTaskSearchRunes  = 200
)

type AvailableModelLister interface {
	ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelsvc.AvailableModel, error)
}

type Dispatcher interface {
	EnqueueGeneration(context.Context, uuid.UUID) error
	EnqueueCompensation(context.Context, uuid.UUID) error
}

type AssetStore interface {
	Save(context.Context, *Task, []byte) (string, error)
	Delete(context.Context, string) error
	DeleteStoredObject(context.Context, string, Scope) error
	URL(context.Context, string) (string, error)
}

type Service struct {
	repo       Repository
	dispatcher Dispatcher
	models     AvailableModelLister
	assets     AssetStore
}

func NewService(repo Repository, dispatcher Dispatcher, models AvailableModelLister, assets AssetStore) *Service {
	if repo == nil || dispatcher == nil || models == nil || assets == nil {
		panic("music service requires repository, dispatcher, model catalog, and asset store")
	}
	return &Service{repo: repo, dispatcher: dispatcher, models: models, assets: assets}
}

func (s *Service) Create(ctx context.Context, scope Scope, request CreateRequest) (*Task, error) {
	request, err := normalizeCreateRequest(scope, request)
	if err != nil {
		return nil, err
	}
	if existing, err := s.repo.GetByRequest(ctx, scope, request.RequestID); err == nil {
		return matchExistingTask(existing, scope, request)
	} else if !errors.Is(err, ErrTaskNotFound) {
		return nil, fmt.Errorf("get existing music task: %w", err)
	}
	models, err := s.models.ListAvailable(ctx, scope.OrganizationID, "", string(llmmodel.UseCaseMusicGen))
	if err != nil {
		return nil, fmt.Errorf("list available music models: %w", err)
	}
	available := false
	for _, model := range models {
		if model != nil && model.Name == request.Model && model.Endpoints.MusicGeneration {
			available = true
			break
		}
	}
	if !available {
		return nil, ErrModelUnavailable
	}
	now := time.Now().UTC()
	task := &Task{
		ID:             uuid.New(),
		OrganizationID: scope.OrganizationID,
		WorkspaceID:    scope.WorkspaceID,
		AccountID:      scope.AccountID,
		RequestID:      request.RequestID,
		Model:          request.Model,
		Mode:           request.Mode,
		Prompt:         request.Prompt,
		Lyrics:         request.Lyrics,
		ResponseFormat: musicResponseFormat,
		Status:         StatusQueued,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.Create(ctx, task); err != nil {
		if existing, getErr := s.repo.GetByRequest(ctx, scope, request.RequestID); getErr == nil {
			return matchExistingTask(existing, scope, request)
		}
		return nil, fmt.Errorf("create music task: %w", err)
	}
	if err := s.dispatcher.EnqueueGeneration(ctx, task.ID); err != nil {
		transitionErr := s.repo.Transition(ctx, task.ID, StatusQueued, StatusFailed, TaskUpdate{
			ErrorCode:    ErrorCodeQueueUnavailable,
			ErrorMessage: "Music task queue is unavailable; please try again",
		})
		return nil, errors.Join(fmt.Errorf("enqueue music task: %w", err), transitionErr)
	}
	return task, nil
}

func (s *Service) Get(ctx context.Context, scope Scope, id uuid.UUID) (*TaskView, error) {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.AccountID == uuid.Nil || id == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	task, err := s.repo.GetScoped(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	view := taskView(task)
	if task.Status == StatusSucceeded && task.FileID != nil {
		url, err := s.assets.URL(ctx, task.FileID.String())
		if err != nil {
			return nil, fmt.Errorf("sign music file: %w", err)
		}
		view.URL = url
	}
	return view, nil
}

func (s *Service) List(ctx context.Context, scope Scope, request ListRequest) (*TaskList, error) {
	query, err := normalizeListRequest(scope, request)
	if err != nil {
		return nil, err
	}
	tasks, total, err := s.repo.ListScoped(ctx, scope, query)
	if err != nil {
		return nil, fmt.Errorf("list music tasks: %w", err)
	}
	items := make([]*TaskView, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, taskView(task))
	}
	return &TaskList{
		Items:    items,
		Total:    total,
		Page:     query.Page,
		PageSize: query.PageSize,
		HasMore:  int64(query.Page)*int64(query.PageSize) < total,
	}, nil
}

func (s *Service) Delete(ctx context.Context, scope Scope, id uuid.UUID) error {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.AccountID == uuid.Nil || id == uuid.Nil {
		return ErrInvalidRequest
	}
	task, err := s.repo.GetScoped(ctx, scope, id)
	if err != nil {
		return err
	}
	if !isDeletableStatus(task.Status) {
		return apperror.Wrap(
			ErrTaskNotDeletable,
			AppCodeTaskNotDeletable,
			apperror.WithOperation("music.task.delete"),
		)
	}
	if task.Status == StatusSucceeded && task.FileID == nil {
		return ErrTaskAssetMissing
	}
	if task.FileID != nil {
		if err := s.assets.DeleteStoredObject(ctx, task.FileID.String(), scope); err != nil {
			return fmt.Errorf("delete music storage object: %w", err)
		}
	}
	if err := s.repo.DeleteScopedTerminal(ctx, scope, id); err != nil {
		return fmt.Errorf("delete music task metadata: %w", err)
	}
	return nil
}

func normalizeCreateRequest(scope Scope, request CreateRequest) (CreateRequest, error) {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.AccountID == uuid.Nil || request.RequestID == uuid.Nil ||
		!utf8.ValidString(request.Prompt) || !utf8.ValidString(request.Lyrics) {
		return CreateRequest{}, ErrInvalidRequest
	}
	request.Model = strings.TrimSpace(request.Model)
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.Lyrics = strings.TrimSpace(request.Lyrics)
	if request.Model == "" || request.Prompt == "" || utf8.RuneCountInString(request.Prompt) > adapter.MaxMusicPromptRunes ||
		utf8.RuneCountInString(request.Lyrics) > adapter.MaxMusicLyricsRunes {
		return CreateRequest{}, ErrInvalidRequest
	}
	switch request.Mode {
	case adapter.MusicModeVocal:
		if request.Lyrics == "" {
			return CreateRequest{}, ErrInvalidRequest
		}
	case adapter.MusicModeAutoLyrics, adapter.MusicModeInstrumental:
		if request.Prompt == "" || request.Lyrics != "" {
			return CreateRequest{}, ErrInvalidRequest
		}
	default:
		return CreateRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func normalizeListRequest(scope Scope, request ListRequest) (ListQuery, error) {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.AccountID == uuid.Nil ||
		!utf8.ValidString(request.Search) {
		return ListQuery{}, ErrInvalidRequest
	}
	if request.Page == 0 {
		request.Page = 1
	}
	if request.PageSize == 0 {
		request.PageSize = defaultMusicTaskPageSize
	}
	request.Search = strings.TrimSpace(request.Search)
	if request.Page < 1 || request.PageSize < 1 || request.PageSize > maxMusicTaskPageSize ||
		utf8.RuneCountInString(request.Search) > maxMusicTaskSearchRunes {
		return ListQuery{}, ErrInvalidRequest
	}
	maxInt := int(^uint(0) >> 1)
	if request.Page > maxInt/request.PageSize {
		return ListQuery{}, ErrInvalidRequest
	}
	return ListQuery(request), nil
}

func matchExistingTask(task *Task, scope Scope, request CreateRequest) (*Task, error) {
	if task == nil || task.AccountID != scope.AccountID || task.RequestID != request.RequestID || task.Model != request.Model || task.Mode != request.Mode ||
		task.Prompt != request.Prompt || task.Lyrics != request.Lyrics || task.ResponseFormat != musicResponseFormat {
		return nil, ErrTaskConflict
	}
	return task, nil
}

func taskView(task *Task) *TaskView {
	return &TaskView{
		ID:             task.ID,
		Model:          task.Model,
		Mode:           task.Mode,
		Prompt:         task.Prompt,
		Lyrics:         task.Lyrics,
		Title:          task.Title,
		StyleTags:      append([]string(nil), task.StyleTags...),
		ResponseFormat: task.ResponseFormat,
		Status:         task.Status,
		FileID:         task.FileID,
		DurationMS:     task.DurationMS,
		WaveformPeaks:  append([]int16(nil), task.WaveformPeaks...),
		ErrorCode:      task.ErrorCode,
		ErrorMessage:   task.ErrorMessage,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
		StartedAt:      task.StartedAt,
		CompletedAt:    task.CompletedAt,
	}
}

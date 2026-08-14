package music

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRepositoryScopesReadsAndEnforcesStateTransitions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Task{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	task := queuedTask()
	if err := repo.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	byRequest, err := repo.GetByRequest(t.Context(), Scope{
		OrganizationID: task.OrganizationID,
		WorkspaceID:    task.WorkspaceID,
		AccountID:      task.AccountID,
	}, task.RequestID)
	if err != nil || byRequest.ID != task.ID {
		t.Fatalf("GetByRequest() = %#v, %v", byRequest, err)
	}
	duplicate := *task
	duplicate.ID = uuid.New()
	if err := repo.Create(t.Context(), &duplicate); err == nil {
		t.Fatal("Create() duplicate request id error = nil")
	}
	wrongWorkspaceID := uuid.New()
	wrongScope := Scope{OrganizationID: task.OrganizationID, WorkspaceID: &wrongWorkspaceID, AccountID: task.AccountID}
	if _, err := repo.GetScoped(t.Context(), wrongScope, task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("GetScoped() error = %v, want ErrTaskNotFound", err)
	}
	otherAccount := Scope{OrganizationID: task.OrganizationID, WorkspaceID: task.WorkspaceID, AccountID: uuid.New()}
	if _, err := repo.GetScoped(t.Context(), otherAccount, task.ID); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("GetScoped() for another account error = %v, want ErrTaskNotFound", err)
	}
	ownerScope := Scope{OrganizationID: task.OrganizationID, WorkspaceID: task.WorkspaceID, AccountID: task.AccountID}
	owned, err := repo.GetScoped(t.Context(), ownerScope, task.ID)
	if err != nil || owned.ID != task.ID {
		t.Fatalf("GetScoped() for owner = %#v, %v", owned, err)
	}
	if err := repo.Transition(t.Context(), task.ID, StatusQueued, StatusGenerating, TaskUpdate{}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Transition(t.Context(), task.ID, StatusQueued, StatusFailed, TaskUpdate{}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("stale transition error = %v, want ErrInvalidTransition", err)
	}
	if err := repo.Transition(t.Context(), task.ID, StatusGenerating, StatusCompensationPending, TaskUpdate{}); err != nil {
		t.Fatal(err)
	}
	oldAttempt := time.Now().Add(-time.Hour).UTC()
	if err := db.Model(&Task{}).Where("id = ?", task.ID).Update("updated_at", oldAttempt).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.TouchStatus(t.Context(), task.ID, StatusCompensationPending); err != nil {
		t.Fatal(err)
	}
	touched, err := repo.Get(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !touched.UpdatedAt.After(oldAttempt) {
		t.Fatalf("updated_at = %s, want after %s", touched.UpdatedAt, oldAttempt)
	}
}

func TestRepositoryPersonalScopeListsAndGetsOwnedTasksAcrossWorkspaces(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:music-personal-scope?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Task{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	personalScope := testScope()
	personalScope.WorkspaceID = nil
	personalTask := queuedTask()
	personalTask.OrganizationID = personalScope.OrganizationID
	personalTask.WorkspaceID = nil
	personalTask.AccountID = personalScope.AccountID
	workspaceTask := queuedTask()
	workspaceTask.OrganizationID = personalScope.OrganizationID
	workspaceTask.AccountID = personalScope.AccountID
	otherAccountTask := queuedTask()
	otherAccountTask.OrganizationID = personalScope.OrganizationID
	otherAccountTask.WorkspaceID = nil
	otherOrganizationTask := queuedTask()
	otherOrganizationTask.AccountID = personalScope.AccountID
	otherOrganizationTask.WorkspaceID = nil

	for _, task := range []*Task{personalTask, workspaceTask, otherAccountTask, otherOrganizationTask} {
		if err := repo.Create(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := repo.ListScoped(t.Context(), personalScope, ListQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListScoped() personal error = %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("ListScoped() personal total = %d, items = %d; want 2 owned tasks", total, len(items))
	}
	if _, err := repo.GetScoped(t.Context(), personalScope, workspaceTask.ID); err != nil {
		t.Fatalf("GetScoped() workspace task from personal scope error = %v", err)
	}
	byRequest, err := repo.GetByRequest(t.Context(), personalScope, personalTask.RequestID)
	if err != nil || byRequest.ID != personalTask.ID {
		t.Fatalf("GetByRequest() personal = %#v, %v", byRequest, err)
	}
}

func TestRepositoryPersistsGeneratedLyricsAndPlaybackMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:music-playback-metadata?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Task{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	task := queuedTask()
	task.Mode = "auto_lyrics"
	if err := repo.Create(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := repo.Transition(t.Context(), task.ID, StatusQueued, StatusGeneratingLyrics, TaskUpdate{}); err != nil {
		t.Fatal(err)
	}
	generated := GeneratedLyrics{
		Title:     "雨后的木吉他",
		StyleTags: []string{"Folk", "Warm"},
		Lyrics:    "[Verse]\n雨停在黄昏以后",
	}
	if err := repo.UpdateGeneratedLyrics(t.Context(), task.ID, generated); err != nil {
		t.Fatal(err)
	}
	if err := repo.Transition(t.Context(), task.ID, StatusGeneratingLyrics, StatusGenerating, TaskUpdate{}); err != nil {
		t.Fatal(err)
	}
	fileID := uuid.New()
	waveform := []int16{12, 48, 100, 32}
	if err := repo.Transition(t.Context(), task.ID, StatusGenerating, StatusSucceeded, TaskUpdate{
		FileID:        &fileID,
		DurationMS:    61_250,
		WaveformPeaks: waveform,
	}); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != generated.Title || got.Lyrics != generated.Lyrics || !reflect.DeepEqual([]string(got.StyleTags), generated.StyleTags) {
		t.Fatalf("generated lyrics metadata = %#v", got)
	}
	if got.DurationMS != 61_250 || !reflect.DeepEqual([]int16(got.WaveformPeaks), waveform) {
		t.Fatalf("playback metadata = %#v", got)
	}
}

func TestRepositoryListsOnlyOwnedTasksNewestFirst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Task{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	scope := testScope()
	now := time.Now().UTC()
	ownedOlder := queuedTask()
	ownedOlder.OrganizationID = scope.OrganizationID
	ownedOlder.WorkspaceID = scope.WorkspaceID
	ownedOlder.AccountID = scope.AccountID
	ownedOlder.Prompt = "Quiet piano"
	ownedOlder.CreatedAt = now.Add(-time.Hour)
	ownedOlder.UpdatedAt = ownedOlder.CreatedAt
	ownedNewer := queuedTask()
	ownedNewer.OrganizationID = scope.OrganizationID
	ownedNewer.WorkspaceID = scope.WorkspaceID
	ownedNewer.AccountID = scope.AccountID
	ownedNewer.Prompt = "Piano at sunrise"
	ownedNewer.CreatedAt = now
	ownedNewer.UpdatedAt = now
	otherAccount := queuedTask()
	otherAccount.OrganizationID = scope.OrganizationID
	otherAccount.WorkspaceID = scope.WorkspaceID
	otherAccount.Prompt = "Piano owned by another account"

	for _, task := range []*Task{ownedOlder, ownedNewer, otherAccount} {
		if err := repo.Create(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := repo.ListScoped(t.Context(), scope, ListQuery{
		Page:     1,
		PageSize: 10,
		Search:   "piano",
	})
	if err != nil {
		t.Fatalf("ListScoped() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("ListScoped() total = %d, want 2", total)
	}
	if len(items) != 2 || items[0].ID != ownedNewer.ID || items[1].ID != ownedOlder.ID {
		t.Fatalf("ListScoped() items = %#v, want newest owned tasks", items)
	}
}

func TestRepositoryTreatsTaskSearchWildcardsLiterally(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:music-search-literals?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Task{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	scope := testScope()
	literal := queuedTask()
	literal.OrganizationID = scope.OrganizationID
	literal.WorkspaceID = scope.WorkspaceID
	literal.AccountID = scope.AccountID
	literal.Prompt = "100% piano"
	wildcardMatch := queuedTask()
	wildcardMatch.OrganizationID = scope.OrganizationID
	wildcardMatch.WorkspaceID = scope.WorkspaceID
	wildcardMatch.AccountID = scope.AccountID
	wildcardMatch.Prompt = "1000 piano"
	for _, task := range []*Task{literal, wildcardMatch} {
		if err := repo.Create(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}

	items, total, err := repo.ListScoped(t.Context(), scope, ListQuery{Page: 1, PageSize: 10, Search: "100%"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != literal.ID {
		t.Fatalf("ListScoped() = total %d, items %#v; want only literal wildcard match", total, items)
	}
}

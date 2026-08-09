package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTaskRepositoryListPaginatesCountsAndSearchesAllRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:video-list-%s?mode=memory&cache=shared", uuid.NewString())), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	if err := db.Exec(`CREATE TABLE video_runtime_tasks (
		id text PRIMARY KEY,
		organization_id text NOT NULL,
		account_id text NOT NULL,
		workspace_id text,
		task_id text,
		provider text,
		model text,
		model_label text,
		prompt text,
		status text,
		created_at datetime
	)`).Error; err != nil {
		t.Fatalf("create table error = %v", err)
	}

	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}
	createdAt := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 25; index++ {
		prompt := fmt.Sprintf("forest scene %02d", index)
		if index == 17 {
			prompt = "literal 100%_match"
		}
		if err := db.Exec(
			`INSERT INTO video_runtime_tasks
			(id, organization_id, account_id, task_id, provider, model, model_label, prompt, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid.New(), scope.OrganizationID, scope.AccountID, fmt.Sprintf("video-%02d", index),
			"doubao", "seedance", "Seedance", prompt, "succeeded", createdAt.Add(-time.Duration(index)*time.Minute),
		).Error; err != nil {
			t.Fatalf("insert task %d error = %v", index, err)
		}
	}
	if err := db.Exec(
		`INSERT INTO video_runtime_tasks
		(id, organization_id, account_id, task_id, prompt, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New(), scope.OrganizationID, uuid.New(), "other-account", "forest scene", createdAt,
	).Error; err != nil {
		t.Fatalf("insert other account task error = %v", err)
	}

	repository := newTaskRepository(db)
	first, err := repository.list(context.Background(), scope, taskListParams{Limit: 10})
	if err != nil {
		t.Fatalf("first list error = %v", err)
	}
	if first.Total != 25 || len(first.Records) != 10 || !first.HasMore {
		t.Fatalf("first page = total %d, records %d, hasMore %v; want 25, 10, true", first.Total, len(first.Records), first.HasMore)
	}
	if first.Records[0].TaskID != "video-00" || first.Records[9].TaskID != "video-09" {
		t.Fatalf("first page bounds = %q..%q, want video-00..video-09", first.Records[0].TaskID, first.Records[9].TaskID)
	}

	last := first.Records[len(first.Records)-1]
	second, err := repository.list(context.Background(), scope, taskListParams{
		Limit:           10,
		BeforeCreatedAt: &last.CreatedAt,
		BeforeID:        &last.ID,
	})
	if err != nil {
		t.Fatalf("second list error = %v", err)
	}
	if second.Total != 25 || len(second.Records) != 10 || !second.HasMore {
		t.Fatalf("second page = total %d, records %d, hasMore %v; want 25, 10, true", second.Total, len(second.Records), second.HasMore)
	}
	if second.Records[0].TaskID != "video-10" {
		t.Fatalf("second page first task = %q, want video-10", second.Records[0].TaskID)
	}

	search, err := repository.list(context.Background(), scope, taskListParams{Limit: 20, Search: "%_match"})
	if err != nil {
		t.Fatalf("search list error = %v", err)
	}
	if search.Total != 1 || len(search.Records) != 1 || search.Records[0].TaskID != "video-17" {
		t.Fatalf("literal search = total %d records %#v, want only video-17", search.Total, search.Records)
	}
}

func TestVideoTaskCursorRoundTrip(t *testing.T) {
	want := videoTaskCursor{
		CreatedAt: time.Date(2026, time.August, 9, 12, 34, 56, 789, time.UTC),
		ID:        uuid.New(),
	}
	got, err := decodeVideoTaskCursor(encodeVideoTaskCursor(want))
	if err != nil {
		t.Fatalf("decodeVideoTaskCursor() error = %v", err)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || got.ID != want.ID {
		t.Fatalf("cursor round trip = %#v, want %#v", got, want)
	}
	if _, err := decodeVideoTaskCursor("not-a-cursor"); err == nil {
		t.Fatal("decodeVideoTaskCursor() error = nil, want invalid cursor error")
	}
}

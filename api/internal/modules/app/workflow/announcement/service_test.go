package announcement

import (
	"context"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGenerateToken(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	if len(token) != tokenLength {
		t.Fatalf("generateToken() length = %d, want %d", len(token), tokenLength)
	}
	for _, char := range token {
		if !isTokenAlphabetChar(char) {
			t.Fatalf("generateToken() contains unsupported character %q", char)
		}
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  NodeConfig
		wantErr bool
	}{
		{
			name: "accepts default timeout",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
			},
		},
		{
			name: "accepts one week",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 7,
					Unit:     "day",
				},
			},
		},
		{
			name: "requires content",
			config: NodeConfig{
				Title: "Release notice",
				Timeout: TimeoutConfig{
					Duration: 1,
					Unit:     "day",
				},
			},
			wantErr: true,
		},
		{
			name: "requires title",
			config: NodeConfig{
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 1,
					Unit:     "day",
				},
			},
			wantErr: true,
		},
		{
			name: "rejects over one week",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 8,
					Unit:     "day",
				},
			},
			wantErr: true,
		},
		{
			name: "rejects title over max length",
			config: NodeConfig{
				Title:   strings.Repeat("a", MaxTitleLength+1),
				Content: "Release window starts at 10:00.",
			},
			wantErr: true,
		},
		{
			name: "rejects unsupported unit",
			config: NodeConfig{
				Title:   "Release notice",
				Content: "Release window starts at 10:00.",
				Timeout: TimeoutConfig{
					Duration: 1,
					Unit:     "week",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateRuntimeAnnouncementWithTokenRetryLoadsExistingOnRunNodeConflict(t *testing.T) {
	db, mock, cleanup := newAnnouncementMockDB(t)
	defer cleanup()
	service := NewService(db)
	existing := testAnnouncement("existing", "tenant-1", "run-1", "node-1", "token-a", time.Now().Add(time.Hour))
	candidate := testAnnouncement("candidate", "tenant-1", "run-1", "node-1", "token-b", time.Now().Add(time.Hour))
	expectAnnouncementInsert(mock, fmt.Errorf(`ERROR: duplicate key value violates unique constraint "idx_announcements_run_node"`))
	expectRuntimeAnnouncementLoad(mock, existing)

	got, err := service.createRuntimeAnnouncementWithTokenRetry(context.Background(), candidate)
	if err != nil {
		t.Fatalf("createRuntimeAnnouncementWithTokenRetry() error = %v", err)
	}
	if got.ID != existing.ID || got.AccessToken != existing.AccessToken {
		t.Fatalf("got announcement ID/token = %s/%s, want existing %s/%s", got.ID, got.AccessToken, existing.ID, existing.AccessToken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCreateRuntimeAnnouncementWithTokenRetryRegeneratesOnTokenConflict(t *testing.T) {
	db, mock, cleanup := newAnnouncementMockDB(t)
	defer cleanup()
	service := NewService(db)
	originalTokenGenerator := newAnnouncementToken
	newAnnouncementToken = func() (string, error) {
		return "token-b", nil
	}
	t.Cleanup(func() {
		newAnnouncementToken = originalTokenGenerator
	})
	candidate := testAnnouncement("candidate", "tenant-1", "run-2", "node-2", "token-a", time.Now().Add(time.Hour))
	expectAnnouncementInsert(mock, fmt.Errorf(`ERROR: duplicate key value violates unique constraint "idx_announcements_access_token"`))
	expectAnnouncementInsert(mock, nil)

	got, err := service.createRuntimeAnnouncementWithTokenRetry(context.Background(), candidate)
	if err != nil {
		t.Fatalf("createRuntimeAnnouncementWithTokenRetry() error = %v", err)
	}
	if got.ID != candidate.ID || got.AccessToken != "token-b" {
		t.Fatalf("got announcement ID/token = %s/%s, want candidate/token-b", got.ID, got.AccessToken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCleanupExpiredAnnouncementsDeletesExpiredRows(t *testing.T) {
	db, mock, cleanup := newAnnouncementMockDB(t)
	defer cleanup()
	service := NewService(db)
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "announcements" WHERE expiration_time <= $1`)).
		WithArgs(now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	deleted, err := service.CleanupExpiredAnnouncements(context.Background(), now)
	if err != nil {
		t.Fatalf("CleanupExpiredAnnouncements() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func isTokenAlphabetChar(char rune) bool {
	for _, allowed := range tokenAlphabet {
		if char == allowed {
			return true
		}
	}
	return false
}

func newAnnouncementMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open test db: %v", err)
	}
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func testAnnouncement(id, tenantID, workflowRunID, nodeID, token string, expiration time.Time) *Announcement {
	return &Announcement{
		ID:              id,
		TenantID:        tenantID,
		AppID:           "app-1",
		WorkflowRunID:   workflowRunID,
		NodeID:          nodeID,
		NodeTitle:       "Notice",
		Content:         "content",
		RenderedContent: "rendered",
		AccessToken:     token,
		ExpirationTime:  expiration,
	}
}

func expectAnnouncementInsert(mock sqlmock.Sqlmock, resultErr error) {
	mock.ExpectBegin()
	query := regexp.QuoteMeta(`INSERT INTO "announcements"`)
	args := []driver.Value{
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
		sqlmock.AnyArg(),
	}
	if resultErr != nil {
		mock.ExpectExec(query).
			WithArgs(args...).
			WillReturnError(resultErr)
		mock.ExpectRollback()
		return
	}
	mock.ExpectExec(query).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func expectRuntimeAnnouncementLoad(mock sqlmock.Sqlmock, announcement *Announcement) {
	rows := sqlmock.NewRows([]string{
		"id",
		"tenant_id",
		"app_id",
		"workflow_run_id",
		"node_id",
		"node_title",
		"content",
		"rendered_content",
		"access_token",
		"expiration_time",
		"created_at",
		"updated_at",
	}).AddRow(
		announcement.ID,
		announcement.TenantID,
		announcement.AppID,
		announcement.WorkflowRunID,
		announcement.NodeID,
		announcement.NodeTitle,
		announcement.Content,
		announcement.RenderedContent,
		announcement.AccessToken,
		announcement.ExpirationTime,
		time.Now(),
		time.Now(),
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "announcements" WHERE tenant_id = $1 AND workflow_run_id = $2 AND node_id = $3 ORDER BY "announcements"."id" LIMIT $4`)).
		WithArgs(announcement.TenantID, announcement.WorkflowRunID, announcement.NodeID, 1).
		WillReturnRows(rows)
}

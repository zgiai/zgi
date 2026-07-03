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
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
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
	expectAnnouncementInsert(mock, candidate, fmt.Errorf(`ERROR: duplicate key value violates unique constraint "idx_announcements_access_token"`))
	retryCandidate := *candidate
	retryCandidate.AccessToken = "token-b"
	expectAnnouncementInsert(mock, &retryCandidate, nil)

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

func TestCreateRuntimeAnnouncementCreatesNewRowsForSameRunNode(t *testing.T) {
	db, mock, cleanup := newAnnouncementMockDB(t)
	defer cleanup()
	originalTokenGenerator := newAnnouncementToken
	tokens := []string{"token-a1", "token-b2"}
	newAnnouncementToken = func() (string, error) {
		if len(tokens) == 0 {
			t.Fatal("newAnnouncementToken called more times than expected")
		}
		token := tokens[0]
		tokens = tokens[1:]
		return token, nil
	}
	t.Cleanup(func() {
		newAnnouncementToken = originalTokenGenerator
	})

	shortLinks := &fakeShortLinkService{}
	service := NewServiceWithShortLinkService(db, shortLinks)
	params := CreateRuntimeAnnouncementParams{
		TenantID:      "tenant-1",
		AppID:         "app-1",
		WorkflowRunID: "run-duplicate",
		NodeID:        "node-duplicate",
		NodeTitle:     "Notice",
		Config: NodeConfig{
			Title:   "Notice",
			Content: "content",
		},
		Rendered: "rendered",
	}
	expectRuntimeAnnouncementCreate(mock, "token-a1")
	first, err := service.CreateRuntimeAnnouncement(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateRuntimeAnnouncement first error = %v", err)
	}
	expectRuntimeAnnouncementCreate(mock, "token-b2")
	second, err := service.CreateRuntimeAnnouncement(context.Background(), params)
	if err != nil {
		t.Fatalf("CreateRuntimeAnnouncement second error = %v", err)
	}
	if first.Announcement.ID == second.Announcement.ID {
		t.Fatalf("second announcement ID = %q, want a new row", second.Announcement.ID)
	}
	if first.Announcement.AccessToken == second.Announcement.AccessToken {
		t.Fatalf("second announcement token = %q, want a new token", second.Announcement.AccessToken)
	}
	if len(shortLinks.requests) != 2 {
		t.Fatalf("short link request count = %d, want 2", len(shortLinks.requests))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCreateRuntimeAnnouncementPayloadUsesShortURL(t *testing.T) {
	db, mock, cleanup := newAnnouncementMockDB(t)
	defer cleanup()
	originalTokenGenerator := newAnnouncementToken
	newAnnouncementToken = func() (string, error) {
		return "token-url", nil
	}
	t.Cleanup(func() {
		newAnnouncementToken = originalTokenGenerator
	})

	shortLinks := &fakeShortLinkService{shortToken: "short-url"}
	service := NewServiceWithShortLinkService(db, shortLinks)
	expectRuntimeAnnouncementCreate(mock, "token-url")
	announcement, err := service.CreateRuntimeAnnouncement(context.Background(), CreateRuntimeAnnouncementParams{
		TenantID:      "tenant-1",
		AppID:         "app-1",
		WorkflowRunID: "run-url-1",
		NodeID:        "node-url-1",
		NodeTitle:     "Notice",
		Config: NodeConfig{
			Title:   "Notice",
			Content: "content",
		},
		Rendered: "rendered",
	})
	if err != nil {
		t.Fatalf("CreateRuntimeAnnouncement() error = %v", err)
	}
	if !strings.HasPrefix(announcement.Payload.URL, "https://zgi.example.com/") {
		t.Fatalf("payload url = %q, want short URL", announcement.Payload.URL)
	}
	if announcement.Payload.AccessToken != announcement.Announcement.AccessToken {
		t.Fatalf("payload access_token = %q, want announcement token %q", announcement.Payload.AccessToken, announcement.Announcement.AccessToken)
	}
	if strings.Contains(announcement.Payload.URL, announcement.Announcement.AccessToken) {
		t.Fatalf("payload url = %q, should not expose announcement token", announcement.Payload.URL)
	}
	if len(shortLinks.requests) != 1 {
		t.Fatalf("short link request count = %d, want 1", len(shortLinks.requests))
	}
	shortLinkRequest := shortLinks.requests[0]
	if shortLinkRequest.ExpiresAt == nil || !shortLinkRequest.ExpiresAt.Equal(announcement.Announcement.ExpirationTime) {
		t.Fatalf("short link expires_at = %v, want announcement expiration %v", shortLinkRequest.ExpiresAt, announcement.Announcement.ExpirationTime)
	}
	if announcement.Payload.Token != "short-url" {
		t.Fatalf("payload token = %q, want short-url", announcement.Payload.Token)
	}
	if !strings.HasSuffix(announcement.Payload.URL, "/short-url") {
		t.Fatalf("payload url = %q, want to end with short-url", announcement.Payload.URL)
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

func expectAnnouncementInsert(mock sqlmock.Sqlmock, announcement *Announcement, resultErr error) {
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
	expectShortLinkCreate(mock, announcement.AccessToken, announcement.ExpirationTime)
	mock.ExpectCommit()
}

func expectRuntimeAnnouncementCreate(mock sqlmock.Sqlmock, targetToken string) {
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
	mock.ExpectExec(query).
		WithArgs(args...).
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectShortLinkCreate(mock, targetToken, sqlmock.AnyArg())
	mock.ExpectCommit()
}

func expectShortLinkCreate(mock sqlmock.Sqlmock, targetToken string, expiresAt driver.Value) {
	rows := sqlmock.NewRows([]string{
		"id",
		"short_token",
		"target_kind",
		"target_token",
		"target_path",
		"expires_at",
		"created_at",
		"updated_at",
	})
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "system_short_links" WHERE target_kind = $1 AND target_token = $2 ORDER BY "system_short_links"."id" LIMIT $3`)).
		WithArgs("workflow_announcement", targetToken, 1).
		WillReturnRows(rows)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "system_short_links"`)).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"workflow_announcement",
			targetToken,
			"/n/"+targetToken,
			expiresAt,
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

type fakeShortLinkService struct {
	shortToken string
	requests   []shortlinkcap.CreateOrGetRequest
}

func (s *fakeShortLinkService) CreateOrGet(_ context.Context, req shortlinkcap.CreateOrGetRequest) (*shortlinkcap.ShortLink, error) {
	s.requests = append(s.requests, req)
	shortToken := s.shortToken
	if shortToken == "" {
		shortToken = "short-" + req.TargetToken
	}
	return &shortlinkcap.ShortLink{
		ID:          "short-link-" + req.TargetToken,
		ShortToken:  shortToken,
		TargetKind:  req.TargetKind,
		TargetToken: req.TargetToken,
		TargetPath:  req.TargetPath,
		ExpiresAt:   req.ExpiresAt,
	}, nil
}

func (s *fakeShortLinkService) Resolve(context.Context, string) (*shortlinkcap.ShortLink, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeShortLinkService) SyncKnownTargetExpiresAt(context.Context, time.Time, int) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *fakeShortLinkService) CleanupExpired(context.Context, time.Time, int) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *fakeShortLinkService) BuildPublicURL(shortToken string) (string, error) {
	return "https://zgi.example.com/n/" + shortToken, nil
}

package shortlink

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGenerateTokenUsesReadableAlphabet(t *testing.T) {
	token, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken() error = %v", err)
	}
	if len(token) != tokenLength {
		t.Fatalf("token length = %d, want %d", len(token), tokenLength)
	}
	if !isValidToken(token) {
		t.Fatalf("token %q should be valid", token)
	}
}

func TestIsValidTokenNormalizesUppercase(t *testing.T) {
	if !isValidToken("ABC234EF") {
		t.Fatal("uppercase token should be accepted after lowercase normalization")
	}
	if isValidToken("abc10ief") {
		t.Fatal("ambiguous characters should be rejected")
	}
	if isValidToken("abc234e") {
		t.Fatal("short token should be rejected")
	}
}

func TestValidateTargetRejectsExternalRedirects(t *testing.T) {
	tests := []struct {
		name string
		kind string
		path string
	}{
		{name: "absolute http", kind: TargetKindApprovalForm, path: "http://evil.test/a/token"},
		{name: "absolute https", kind: TargetKindApprovalForm, path: "https://evil.test/a/token"},
		{name: "protocol relative", kind: TargetKindApprovalForm, path: "//evil.test/a/token"},
		{name: "console path", kind: TargetKindApprovalForm, path: "/console/login"},
		{name: "extra segment", kind: TargetKindApprovalForm, path: "/a/token/extra"},
		{name: "query", kind: TargetKindApprovalForm, path: "/a/token?next=https://evil.test"},
		{name: "unknown kind", kind: "file_preview", path: "/a/token"},
		{name: "empty path", kind: TargetKindApprovalForm, path: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateTarget(tt.kind, tt.path); err == nil {
				t.Fatal("expected target to be rejected")
			}
		})
	}
}

func TestCreateOrGetIsIdempotent(t *testing.T) {
	service := newSQLiteService(t)
	ctx := context.Background()
	expiresAt := testExpiresAt()
	req := CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "approval-token",
		TargetPath:  "/a/approval-token",
		ExpiresAt:   &expiresAt,
	}
	first, err := service.CreateOrGet(ctx, req)
	if err != nil {
		t.Fatalf("CreateOrGet first error = %v", err)
	}
	second, err := service.CreateOrGet(ctx, req)
	if err != nil {
		t.Fatalf("CreateOrGet second error = %v", err)
	}
	if first.ShortToken != second.ShortToken {
		t.Fatalf("short token = %q, want %q", second.ShortToken, first.ShortToken)
	}
}

func TestCreateOrGetRequiresExpiresAtForKnownTargets(t *testing.T) {
	service := newSQLiteService(t)
	_, err := service.CreateOrGet(context.Background(), CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "approval-token",
		TargetPath:  "/a/approval-token",
	})
	if err == nil || !strings.Contains(err.Error(), "expires_at is required") {
		t.Fatalf("expected expires_at requirement error, got %v", err)
	}
}

func TestCreateOrGetStoresAndRefreshesExpiresAt(t *testing.T) {
	service := newSQLiteService(t)
	ctx := context.Background()
	firstExpiresAt := time.Now().Add(time.Hour).Truncate(time.Second)
	secondExpiresAt := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	req := CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "approval-token",
		TargetPath:  "/a/approval-token",
		ExpiresAt:   &firstExpiresAt,
	}
	first, err := service.CreateOrGet(ctx, req)
	if err != nil {
		t.Fatalf("CreateOrGet first error = %v", err)
	}
	if first.ExpiresAt == nil || !first.ExpiresAt.Equal(firstExpiresAt) {
		t.Fatalf("first expires_at = %v, want %v", first.ExpiresAt, firstExpiresAt)
	}

	req.ExpiresAt = &secondExpiresAt
	second, err := service.CreateOrGet(ctx, req)
	if err != nil {
		t.Fatalf("CreateOrGet second error = %v", err)
	}
	if second.ShortToken != first.ShortToken {
		t.Fatalf("short token = %q, want %q", second.ShortToken, first.ShortToken)
	}
	if second.ExpiresAt == nil || !second.ExpiresAt.Equal(secondExpiresAt) {
		t.Fatalf("second expires_at = %v, want %v", second.ExpiresAt, secondExpiresAt)
	}
}

func TestCreateOrGetRequiresTargetTokenToMatchPath(t *testing.T) {
	service := newSQLiteService(t)
	expiresAt := testExpiresAt()
	_, err := service.CreateOrGet(context.Background(), CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "approval-token",
		TargetPath:  "/a/other-token",
		ExpiresAt:   &expiresAt,
	})
	if err == nil || !strings.Contains(err.Error(), "target_token must match target_path") {
		t.Fatalf("expected target mismatch error, got %v", err)
	}
}

func TestCreateOrGetRetriesTokenConflict(t *testing.T) {
	service := newSQLiteService(t)
	ctx := context.Background()
	restore := stubTokens(t, "abc234ef", "def345gh")
	defer restore()
	expiresAt := testExpiresAt()
	_, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "first",
		TargetPath:  "/a/first",
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet first error = %v", err)
	}
	link, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "second",
		TargetPath:  "/a/second",
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet second error = %v", err)
	}
	if link.ShortToken != "def345gh" {
		t.Fatalf("short token = %q, want retry token", link.ShortToken)
	}
}

func TestCreateOrGetFailsAfterTokenRetries(t *testing.T) {
	service := newSQLiteService(t)
	ctx := context.Background()
	restore := stubTokens(t, "abc234ef")
	defer restore()
	expiresAt := testExpiresAt()
	_, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "first",
		TargetPath:  "/a/first",
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet first error = %v", err)
	}
	_, err = service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "second",
		TargetPath:  "/a/second",
		ExpiresAt:   &expiresAt,
	})
	if err == nil || !strings.Contains(err.Error(), "after token retries") {
		t.Fatalf("expected retry exhaustion, got %v", err)
	}
}

func TestResolveLowercasesToken(t *testing.T) {
	service := newSQLiteService(t)
	ctx := context.Background()
	restore := stubTokens(t, "abc234ef")
	defer restore()
	expiresAt := time.Now().Add(time.Hour)
	created, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindWorkflowAnnouncement,
		TargetToken: "notice-token",
		TargetPath:  "/n/notice-token",
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet error = %v", err)
	}
	resolved, err := service.Resolve(ctx, strings.ToUpper(created.ShortToken))
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	if resolved.TargetPath != "/n/notice-token" {
		t.Fatalf("target path = %q", resolved.TargetPath)
	}
}

func TestResolveNotFound(t *testing.T) {
	service := newSQLiteService(t)
	_, err := service.Resolve(context.Background(), "abc234ef")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve error = %v, want ErrNotFound", err)
	}
}

func TestResolveReturnsNotFoundForExpiredShortLink(t *testing.T) {
	service := newSQLiteService(t)
	ctx := context.Background()
	expiresAt := time.Now().Add(-time.Minute)
	created, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "expired-token",
		TargetPath:  "/a/expired-token",
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet error = %v", err)
	}

	_, err = service.Resolve(ctx, created.ShortToken)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Resolve error = %v, want ErrExpired", err)
	}
}

func TestResolveBackfillsLegacyExpiredShortLink(t *testing.T) {
	db, service := newSQLiteDBAndService(t)
	createKnownTargetTables(t, db)
	ctx := context.Background()
	expiredAt := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := db.Exec(
		"INSERT INTO workflow_approval_forms (id, access_token, expiration_time) VALUES (?, ?, ?)",
		"form-expired", "form-token", expiredAt,
	).Error; err != nil {
		t.Fatalf("insert approval form: %v", err)
	}
	link := ShortLink{
		ID:          "00000000-0000-0000-0000-000000000101",
		ShortToken:  "abc234ef",
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "form-token",
		TargetPath:  "/a/form-token",
	}
	if err := db.Create(&link).Error; err != nil {
		t.Fatalf("create short link: %v", err)
	}

	_, err := service.Resolve(ctx, link.ShortToken)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Resolve error = %v, want ErrExpired", err)
	}
	var stored ShortLink
	if err := db.First(&stored, "id = ?", link.ID).Error; err != nil {
		t.Fatalf("load short link: %v", err)
	}
	if stored.ExpiresAt == nil || !stored.ExpiresAt.Equal(expiredAt) {
		t.Fatalf("expires_at = %v, want %v", stored.ExpiresAt, expiredAt)
	}
}

func TestSyncKnownTargetExpiresAtBackfillsKnownTargetsAndOrphans(t *testing.T) {
	db, service := newSQLiteDBAndService(t)
	createKnownTargetTables(t, db)
	ctx := context.Background()
	now := time.Date(2026, 6, 7, 10, 30, 0, 0, time.UTC)
	formExpiresAt := now.Add(time.Hour)
	legacyExpiresAt := now.Add(2 * time.Hour)
	announcementExpiresAt := now.Add(3 * time.Hour)
	if err := db.Exec(
		"INSERT INTO workflow_approval_forms (id, access_token, expiration_time) VALUES (?, ?, ?), (?, ?, ?)",
		"form-current", "form-token", formExpiresAt,
		"form-legacy", "legacy-form-token", legacyExpiresAt,
	).Error; err != nil {
		t.Fatalf("insert approval forms: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO workflow_approval_recipients (id, form_id, access_token) VALUES (?, ?, ?)",
		"recipient-legacy", "form-legacy", "legacy-recipient-token",
	).Error; err != nil {
		t.Fatalf("insert approval recipient: %v", err)
	}
	if err := db.Exec(
		"INSERT INTO announcements (id, access_token, expiration_time) VALUES (?, ?, ?)",
		"announcement-current", "announcement-token", announcementExpiresAt,
	).Error; err != nil {
		t.Fatalf("insert announcement: %v", err)
	}
	createStoredShortLink(t, db, "00000000-0000-0000-0000-000000000201", "abc234ef", TargetKindApprovalForm, "form-token", "/a/form-token")
	createStoredShortLink(t, db, "00000000-0000-0000-0000-000000000202", "bcd234ef", TargetKindApprovalForm, "legacy-recipient-token", "/a/legacy-recipient-token")
	createStoredShortLink(t, db, "00000000-0000-0000-0000-000000000203", "cde234ef", TargetKindWorkflowAnnouncement, "announcement-token", "/n/announcement-token")
	createStoredShortLink(t, db, "00000000-0000-0000-0000-000000000204", "def234ef", TargetKindApprovalForm, "missing-token", "/a/missing-token")

	synced, err := service.SyncKnownTargetExpiresAt(ctx, now, 10)
	if err != nil {
		t.Fatalf("SyncKnownTargetExpiresAt error = %v", err)
	}
	if synced != 4 {
		t.Fatalf("synced = %d, want 4", synced)
	}
	assertStoredExpiresAt(t, db, "00000000-0000-0000-0000-000000000201", formExpiresAt)
	assertStoredExpiresAt(t, db, "00000000-0000-0000-0000-000000000202", legacyExpiresAt)
	assertStoredExpiresAt(t, db, "00000000-0000-0000-0000-000000000203", announcementExpiresAt)
	assertStoredExpiresAt(t, db, "00000000-0000-0000-0000-000000000204", now)

	deleted, err := service.CleanupExpired(ctx, now, 10)
	if err != nil {
		t.Fatalf("CleanupExpired error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if err := db.First(&ShortLink{}, "id = ?", "00000000-0000-0000-0000-000000000204").Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("orphan lookup error = %v, want gorm.ErrRecordNotFound", err)
	}
}

func TestCleanupExpiredDeletesOnlyExpiredShortLinks(t *testing.T) {
	db, service := newSQLiteDBAndService(t)
	ctx := context.Background()
	expiredAt := time.Now().Add(-time.Hour)
	activeAt := time.Now().Add(time.Hour)
	expired, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "expired-token",
		TargetPath:  "/a/expired-token",
		ExpiresAt:   &expiredAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet expired error = %v", err)
	}
	active, err := service.CreateOrGet(ctx, CreateOrGetRequest{
		TargetKind:  TargetKindApprovalForm,
		TargetToken: "active-token",
		TargetPath:  "/a/active-token",
		ExpiresAt:   &activeAt,
	})
	if err != nil {
		t.Fatalf("CreateOrGet active error = %v", err)
	}
	permanent := &ShortLink{
		ID:          "00000000-0000-0000-0000-000000000301",
		ShortToken:  "per234ef",
		TargetKind:  "file_preview",
		TargetToken: "permanent-token",
		TargetPath:  "/files/permanent-token",
	}
	if err := db.Create(permanent).Error; err != nil {
		t.Fatalf("create permanent short link: %v", err)
	}

	deleted, err := service.CleanupExpired(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("CleanupExpired error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := service.Resolve(ctx, expired.ShortToken); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired Resolve error = %v, want ErrNotFound", err)
	}
	if _, err := service.Resolve(ctx, active.ShortToken); err != nil {
		t.Fatalf("active Resolve error = %v", err)
	}
	if _, err := service.Resolve(ctx, permanent.ShortToken); err != nil {
		t.Fatalf("permanent Resolve error = %v", err)
	}
}

func TestBuildPublicURL(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		Console: appconfig.ConsoleConfig{WebURL: "https://zgi.example.com/"},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
	service := newSQLiteService(t)
	got, err := service.BuildPublicURL("ABC234EF")
	if err != nil {
		t.Fatalf("BuildPublicURL error = %v", err)
	}
	if got != "https://zgi.example.com/abc234ef" {
		t.Fatalf("public URL = %q", got)
	}
}

func newSQLiteService(t *testing.T) Service {
	t.Helper()
	_, service := newSQLiteDBAndService(t)
	return service
}

func newSQLiteDBAndService(t *testing.T) (*gorm.DB, Service) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ShortLink{}); err != nil {
		t.Fatalf("migrate short links: %v", err)
	}
	return db, NewServiceWithDB(db)
}

func createKnownTargetTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	statements := []string{
		"CREATE TABLE workflow_approval_forms (id text PRIMARY KEY, access_token text, expiration_time datetime)",
		"CREATE TABLE workflow_approval_recipients (id text PRIMARY KEY, form_id text, access_token text)",
		"CREATE TABLE announcements (id text PRIMARY KEY, access_token text, expiration_time datetime)",
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create known target table: %v", err)
		}
	}
}

func createStoredShortLink(t *testing.T, db *gorm.DB, id, shortToken, targetKind, targetToken, targetPath string) {
	t.Helper()
	link := &ShortLink{
		ID:          id,
		ShortToken:  shortToken,
		TargetKind:  targetKind,
		TargetToken: targetToken,
		TargetPath:  targetPath,
	}
	if err := db.Create(link).Error; err != nil {
		t.Fatalf("create stored short link: %v", err)
	}
}

func assertStoredExpiresAt(t *testing.T, db *gorm.DB, id string, want time.Time) {
	t.Helper()
	var link ShortLink
	if err := db.First(&link, "id = ?", id).Error; err != nil {
		t.Fatalf("load short link %s: %v", id, err)
	}
	if link.ExpiresAt == nil || !link.ExpiresAt.Equal(want) {
		t.Fatalf("short link %s expires_at = %v, want %v", id, link.ExpiresAt, want)
	}
}

func testExpiresAt() time.Time {
	return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
}

func stubTokens(t *testing.T, tokens ...string) func() {
	t.Helper()
	original := newToken
	index := 0
	newToken = func() (string, error) {
		if len(tokens) == 0 {
			return "", fmt.Errorf("no stub tokens configured")
		}
		if index >= len(tokens) {
			return tokens[len(tokens)-1], nil
		}
		token := tokens[index]
		index++
		return token, nil
	}
	return func() {
		newToken = original
	}
}

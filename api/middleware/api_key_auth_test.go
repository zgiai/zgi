package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestValidateAPIKeyWithoutRetiredQuotaColumns(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_secret")
	keyID := uuid.New()
	agentID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"tenant_id",
		"key_hash",
		"key_prefix",
		"name",
		"status",
		"expires_at",
		"usage_count",
		"last_used_at",
		"created_at",
		"updated_at",
	}).AddRow(
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		keyHash,
		"zgi_test_sec",
		"test key",
		"active",
		nil,
		int64(7),
		nil,
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurface(mock, agentID, tenantID, true)

	info, err := validateAPIKey(db, rawKey)
	if err != nil {
		t.Fatalf("validateAPIKey returned error: %v", err)
	}

	if info.ID != keyID {
		t.Fatalf("api key id = %s, want %s", info.ID, keyID)
	}
	if info.AgentID != agentID {
		t.Fatalf("agent id = %s, want %s", info.AgentID, agentID)
	}
	if info.TenantID != tenantID {
		t.Fatalf("tenant id = %s, want %s", info.TenantID, tenantID)
	}
	if info.UsageCount != 7 {
		t.Fatalf("usage count = %d, want 7", info.UsageCount)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestValidateAPIKeyRejectsDisabledAgentAPISurface(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_disabled_api")
	keyID := uuid.New()
	agentID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"tenant_id",
		"key_hash",
		"key_prefix",
		"name",
		"status",
		"expires_at",
		"usage_count",
		"last_used_at",
		"created_at",
		"updated_at",
	}).AddRow(
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		keyHash,
		"zgi_test_dis",
		"disabled api key",
		"active",
		nil,
		int64(0),
		nil,
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurface(mock, agentID, tenantID, false)

	info, err := validateAPIKey(db, rawKey)
	if err == nil {
		t.Fatalf("validateAPIKey error = nil, want disabled API surface error")
	}
	if info != nil {
		t.Fatalf("validateAPIKey info = %#v, want nil", info)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestValidateAPIKeyRejectsPersistedDisabledAgentAPISurface(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_persisted_disabled_api")
	keyID := uuid.New()
	agentID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"tenant_id",
		"key_hash",
		"key_prefix",
		"name",
		"status",
		"expires_at",
		"usage_count",
		"last_used_at",
		"created_at",
		"updated_at",
	}).AddRow(
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		keyHash,
		"zgi_test_per",
		"persisted disabled api key",
		"active",
		nil,
		int64(0),
		nil,
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurfaceWithPersistedSurface(mock, agentID, tenantID, true, false)

	info, err := validateAPIKey(db, rawKey)
	if err == nil {
		t.Fatalf("validateAPIKey error = nil, want persisted disabled API surface error")
	}
	if info != nil {
		t.Fatalf("validateAPIKey info = %#v, want nil", info)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestValidateAPIKeyAllowsPersistedEnabledAgentAPISurface(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_persisted_enabled_api")
	keyID := uuid.New()
	agentID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"tenant_id",
		"key_hash",
		"key_prefix",
		"name",
		"status",
		"expires_at",
		"usage_count",
		"last_used_at",
		"created_at",
		"updated_at",
	}).AddRow(
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		keyHash,
		"zgi_test_per",
		"persisted enabled api key",
		"active",
		nil,
		int64(0),
		nil,
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurfaceWithPersistedSurface(mock, agentID, tenantID, false, true)

	info, err := validateAPIKey(db, rawKey)
	if err != nil {
		t.Fatalf("validateAPIKey error = %v, want persisted enabled API surface to allow bearer key", err)
	}
	if info == nil {
		t.Fatalf("validateAPIKey info = nil, want API key info")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestValidateAPIKeyRejectsPersistedNonPublicAgentAPISurfaceGrant(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_persisted_account_api")
	keyID := uuid.New()
	agentID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"tenant_id",
		"key_hash",
		"key_prefix",
		"name",
		"status",
		"expires_at",
		"usage_count",
		"last_used_at",
		"created_at",
		"updated_at",
	}).AddRow(
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		keyHash,
		"zgi_test_per",
		"persisted account api key",
		"active",
		nil,
		int64(0),
		nil,
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurfaceWithPersistedGrant(mock, agentID, tenantID, runtimeauth.PublishedRuntimeSubjectAccount, uuid.New())

	info, err := validateAPIKey(db, rawKey)
	if err == nil {
		t.Fatalf("validateAPIKey error = nil, want non-public API surface grant rejection")
	}
	if info != nil {
		t.Fatalf("validateAPIKey info = %#v, want nil", info)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestUpdateLastUsedWithoutRetiredQuotaColumns(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	updateSQL := `UPDATE "agent_api_keys" SET ("last_used_at"=\$1,"usage_count"=usage_count \+ 1,"updated_at"=\$2|"last_used_at"=\$1,"updated_at"=\$2,"usage_count"=usage_count \+ 1) WHERE id = \$3`
	mock.ExpectExec(updateSQL).
		WillReturnResult(sqlmock.NewResult(0, 1))

	updateLastUsed(db, uuid.New())

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKeyOnFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_secret")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnError(gorm.ErrRecordNotFound)

	output := captureProcessOutput(t, func() {
		router := gin.New()
		router.Use(APIKeyAuthMiddleware(db))
		router.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	assertOutputExcludes(t, output, rawKey, "Bearer "+rawKey, keyHash)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestAPIKeyAuthMiddlewareDoesNotLogSensitiveAPIKeyOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_secret_success")
	keyID := uuid.New()
	agentID := uuid.New()
	tenantID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id",
		"agent_id",
		"tenant_id",
		"key_hash",
		"key_prefix",
		"name",
		"status",
		"expires_at",
		"usage_count",
		"last_used_at",
		"created_at",
		"updated_at",
	}).AddRow(
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		keyHash,
		"zgi_test_sec",
		"secret success key",
		"active",
		nil,
		int64(7),
		nil,
		now,
		now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(keyHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurface(mock, agentID, tenantID, true)
	mock.ExpectExec(`UPDATE "agent_api_keys" SET ("last_used_at"=\$1,"usage_count"=usage_count \+ 1,"updated_at"=\$2|"last_used_at"=\$1,"updated_at"=\$2,"usage_count"=usage_count \+ 1) WHERE id = \$3`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	output := captureProcessOutput(t, func() {
		router := gin.New()
		router.Use(APIKeyAuthMiddleware(db))
		router.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+rawKey)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
		}
		waitForSQLExpectations(t, mock)
	})

	assertOutputExcludes(
		t,
		output,
		rawKey,
		"Bearer "+rawKey,
		keyHash,
		keyID.String(),
		agentID.String(),
		tenantID.String(),
		"secret success key",
	)
}

func openAgentAPIKeyRuntimeMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func hashTestAPIKey(rawKey string) (string, string) {
	hash := sha256.Sum256([]byte(rawKey))
	return rawKey, hex.EncodeToString(hash[:])
}

func expectAgentAPISurface(mock sqlmock.Sqlmock, agentID, tenantID uuid.UUID, enabled bool) {
	mock.ExpectQuery(`SELECT .* FROM "agents" WHERE id = \$1 AND tenant_id = \$2 AND deleted_at IS NULL LIMIT \$3`).
		WithArgs(agentID, tenantID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"tenant_id",
			"enable_api",
			"web_app_status",
			"deleted_at",
		}).AddRow(
			agentID.String(),
			tenantID.String(),
			enabled,
			runtimeauth.WebAppStatusInactive,
			nil,
		))
	expectEmptyPublishedRuntimeSurfaces(mock, agentID)
}

func expectAgentAPISurfaceWithPersistedSurface(mock sqlmock.Sqlmock, agentID, tenantID uuid.UUID, legacyEnabled bool, persistedEnabled bool) {
	mock.ExpectQuery(`SELECT .* FROM "agents" WHERE id = \$1 AND tenant_id = \$2 AND deleted_at IS NULL LIMIT \$3`).
		WithArgs(agentID, tenantID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"tenant_id",
			"enable_api",
			"web_app_status",
			"deleted_at",
		}).AddRow(
			agentID.String(),
			tenantID.String(),
			legacyEnabled,
			runtimeauth.WebAppStatusInactive,
			nil,
		))
	surfaceID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(sqlmock.NewRows(publishedRuntimeSurfaceColumns()).AddRow(
			surfaceID.String(),
			string(runtimeauth.PublishedRuntimeResourceAgent),
			agentID.String(),
			uuid.New().String(),
			tenantID.String(),
			string(runtimeauth.PublishedRuntimeSurfaceAPI),
			persistedEnabled,
			runtimeauth.PublishedRuntimeSourceGrant,
			now,
			now,
			nil,
		))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
}

func expectAgentAPISurfaceWithPersistedGrant(mock sqlmock.Sqlmock, agentID, tenantID uuid.UUID, subjectType runtimeauth.PublishedRuntimeSubjectType, subjectID uuid.UUID) {
	mock.ExpectQuery(`SELECT .* FROM "agents" WHERE id = \$1 AND tenant_id = \$2 AND deleted_at IS NULL LIMIT \$3`).
		WithArgs(agentID, tenantID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"tenant_id",
			"enable_api",
			"web_app_status",
			"deleted_at",
		}).AddRow(
			agentID.String(),
			tenantID.String(),
			true,
			runtimeauth.WebAppStatusInactive,
			nil,
		))
	surfaceID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(sqlmock.NewRows(publishedRuntimeSurfaceColumns()).AddRow(
			surfaceID.String(),
			string(runtimeauth.PublishedRuntimeResourceAgent),
			agentID.String(),
			uuid.New().String(),
			tenantID.String(),
			string(runtimeauth.PublishedRuntimeSurfaceAPI),
			true,
			runtimeauth.PublishedRuntimeSourceGrant,
			now,
			now,
			nil,
		))
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(
			uuid.NewString(),
			surfaceID.String(),
			string(subjectType),
			subjectID.String(),
			true,
			now,
			now,
			nil,
		))
}

func expectEmptyPublishedRuntimeSurfaces(mock sqlmock.Sqlmock, agentID uuid.UUID) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(sqlmock.NewRows(publishedRuntimeSurfaceColumns()))
}

func publishedRuntimeSurfaceColumns() []string {
	return []string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}
}

func captureProcessOutput(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	originalStderr := os.Stderr

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		t.Fatalf("create stderr pipe: %v", err)
	}

	outputs := make(chan string, 2)
	copyOutput := func(reader *os.File) {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		outputs <- buffer.String()
	}

	go copyOutput(stdoutReader)
	go copyOutput(stderrReader)

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	closed := false
	closeWriters := func() {
		if closed {
			return
		}
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
		closed = true
	}
	defer func() {
		closeWriters()
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	}()

	fn()

	closeWriters()
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	output := (<-outputs) + (<-outputs)
	_ = stdoutReader.Close()
	_ = stderrReader.Close()
	return output
}

func assertOutputExcludes(t *testing.T, output string, forbidden ...string) {
	t.Helper()

	for _, value := range forbidden {
		if strings.Contains(output, value) {
			t.Fatalf("output contains sensitive value %q: %s", value, output)
		}
	}
}

func waitForSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()

	var err error
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		err = mock.ExpectationsWereMet()
		if err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("sql expectations not met: %v", err)
}

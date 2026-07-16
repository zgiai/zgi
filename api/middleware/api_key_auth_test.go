package middleware

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
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
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, nil)

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

func TestValidateAPIKeyRejectsDisabledLegacyAgentAPISurface(t *testing.T) {
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
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, nil)

	_, err := validateAPIKey(db, rawKey)
	if err == nil {
		t.Fatalf("validateAPIKey error = nil, want disabled api surface rejection")
	}
	if !strings.Contains(err.Error(), "api surface is disabled") {
		t.Fatalf("validateAPIKey error = %v, want disabled api surface error", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestValidateAgentAPISurfaceRejectsOfflineAgentParentGate(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	agentID := uuid.New()
	tenantID := uuid.New()
	expectAgentAPISurfaceWithStatus(mock, agentID, tenantID, true, "inactive")

	err := validateAgentAPISurface(db, agentID, tenantID)
	if err == nil {
		t.Fatalf("validateAgentAPISurface error = nil, want offline agent rejection")
	}
	if !strings.Contains(err.Error(), "offline") {
		t.Fatalf("validateAgentAPISurface error = %v, want offline error", err)
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
	expectAgentAPISurface(mock, agentID, tenantID, true)
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, []agentAPIRuntimeSurfaceExpectation{{
		id:           uuid.New(),
		tenantID:     tenantID,
		surface:      "api",
		enabled:      false,
		source:       "grant",
		expectGrants: true,
	}})

	_, err := validateAPIKey(db, rawKey)
	if err == nil {
		t.Fatalf("validateAPIKey error = nil, want persisted disabled api surface rejection")
	}
	if !strings.Contains(err.Error(), "api surface is disabled") {
		t.Fatalf("validateAPIKey error = %v, want disabled api surface error", err)
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
	expectAgentAPISurface(mock, agentID, tenantID, false)
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, []agentAPIRuntimeSurfaceExpectation{{
		id:           uuid.New(),
		tenantID:     tenantID,
		surface:      "api",
		enabled:      true,
		source:       "grant",
		expectGrants: true,
	}})

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

func TestValidateAPIKeyIgnoresPersistedNonPublicAgentAPISurfaceGrant(t *testing.T) {
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
	expectAgentAPISurface(mock, agentID, tenantID, true)
	accountGrantID := uuid.New()
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, []agentAPIRuntimeSurfaceExpectation{{
		id:       uuid.New(),
		tenantID: tenantID,
		surface:  "api",
		enabled:  true,
		source:   "grant",
		grants: []agentAPIRuntimeGrantExpectation{{
			subjectType: "account",
			subjectID:   &accountGrantID,
			enabled:     true,
		}},
	}})

	info, err := validateAPIKey(db, rawKey)
	if err != nil {
		t.Fatalf("validateAPIKey error = %v, want persisted API grant to be ignored", err)
	}
	if info == nil {
		t.Fatalf("validateAPIKey info = nil, want API key info")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestUpdateLastUsedOnlyTouchesLastUsedAt(t *testing.T) {
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	updateSQL := `UPDATE "agent_api_keys" SET "last_used_at"=\$1,"updated_at"=\$2 WHERE id = \$3`
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
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, nil)
	mock.ExpectExec(`UPDATE "agent_api_keys" SET "last_used_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
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

func TestAPIKeyAuthMiddlewareAcceptsXAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawKey, keyHash := hashTestAPIKey("zgi_test_x_header")
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
		"zgi_test_x",
		"x header key",
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
	expectAgentAPISurface(mock, agentID, tenantID, true)
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, nil)
	mock.ExpectExec(`UPDATE "agent_api_keys" SET "last_used_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := gin.New()
	router.Use(APIKeyAuthMiddleware(db))
	router.GET("/protected", func(c *gin.Context) {
		if c.GetString("agent_id") != agentID.String() {
			t.Fatalf("agent_id = %q, want %q", c.GetString("agent_id"), agentID.String())
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	waitForSQLExpectations(t, mock)
}

func TestAPIKeyAuthMiddlewareBearerTakesPrecedenceOverXAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, mock, cleanup := openAgentAPIKeyRuntimeMockDB(t)
	defer cleanup()

	rawBearer, bearerHash := hashTestAPIKey("zgi_test_bearer")
	rawXKey, _ := hashTestAPIKey("zgi_test_x_ignored")
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
		bearerHash,
		"zgi_bearer",
		"bearer key",
		"active",
		nil,
		int64(0),
		nil,
		now,
		now,
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agent_api_keys" WHERE (key_hash = $1 AND status = 'active') AND (expires_at IS NULL OR expires_at > $2) ORDER BY "agent_api_keys"."id" LIMIT $3`)).
		WithArgs(bearerHash, sqlmock.AnyArg(), 1).
		WillReturnRows(rows)
	expectAgentAPISurface(mock, agentID, tenantID, true)
	expectAgentAPIRuntimeSurfaceLookup(mock, agentID, nil)
	mock.ExpectExec(`UPDATE "agent_api_keys" SET "last_used_at"=\$1,"updated_at"=\$2 WHERE id = \$3`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	router := gin.New()
	router.Use(APIKeyAuthMiddleware(db))
	router.GET("/protected", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawBearer)
	req.Header.Set("X-API-Key", rawXKey)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	waitForSQLExpectations(t, mock)
}

func TestAPIKeyUsageResponseWriterCapsCapturedBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &responseWriter{
		ResponseWriter: gin.ResponseWriter(&testGinResponseWriter{ResponseWriter: recorder}),
		body:           &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}

	chunk := strings.Repeat("x", apiKeyUsageMaxCapturedResponseBytes+1024)
	if _, err := writer.Write([]byte(chunk)); err != nil {
		t.Fatalf("write response: %v", err)
	}

	if writer.body.Len() != apiKeyUsageMaxCapturedResponseBytes {
		t.Fatalf("captured body len = %d, want %d", writer.body.Len(), apiKeyUsageMaxCapturedResponseBytes)
	}
	if recorder.Body.Len() != len(chunk) {
		t.Fatalf("forwarded body len = %d, want %d", recorder.Body.Len(), len(chunk))
	}
}

type testGinResponseWriter struct {
	http.ResponseWriter
}

func (w *testGinResponseWriter) Status() int                       { return http.StatusOK }
func (w *testGinResponseWriter) Size() int                         { return 0 }
func (w *testGinResponseWriter) Written() bool                     { return false }
func (w *testGinResponseWriter) WriteHeaderNow()                   {}
func (w *testGinResponseWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *testGinResponseWriter) Pusher() http.Pusher               { return nil }
func (w *testGinResponseWriter) Flush()                            {}
func (w *testGinResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, http.ErrNotSupported
}
func (w *testGinResponseWriter) CloseNotify() <-chan bool {
	ch := make(chan bool)
	return ch
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
	expectAgentAPISurfaceWithStatus(mock, agentID, tenantID, enabled, "active")
}

func expectAgentAPISurfaceWithStatus(mock sqlmock.Sqlmock, agentID, tenantID uuid.UUID, enabled bool, webAppStatus string) {
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
			webAppStatus,
			nil,
		))
}

type agentAPIRuntimeSurfaceExpectation struct {
	id             uuid.UUID
	organizationID uuid.UUID
	tenantID       uuid.UUID
	surface        string
	enabled        bool
	source         string
	expectGrants   bool
	grants         []agentAPIRuntimeGrantExpectation
}

type agentAPIRuntimeGrantExpectation struct {
	subjectType string
	subjectID   *uuid.UUID
	enabled     bool
}

func expectAgentAPIRuntimeSurfaceLookup(mock sqlmock.Sqlmock, agentID uuid.UUID, surfaces []agentAPIRuntimeSurfaceExpectation) {
	rows := sqlmock.NewRows([]string{
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
	})
	now := time.Now().UTC().Truncate(time.Second)
	for _, surface := range surfaces {
		organizationID := surface.organizationID
		if organizationID == uuid.Nil {
			organizationID = uuid.New()
		}
		workspaceID := surface.tenantID
		if workspaceID == uuid.Nil {
			workspaceID = uuid.New()
		}
		source := surface.source
		if source == "" {
			source = "grant"
		}
		rows.AddRow(
			surface.id.String(),
			"agent",
			agentID.String(),
			organizationID.String(),
			workspaceID.String(),
			surface.surface,
			surface.enabled,
			source,
			now,
			now,
			nil,
		)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs("agent", agentID).
		WillReturnRows(rows)
	if len(surfaces) == 0 {
		return
	}

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	for _, surface := range surfaces {
		for _, grant := range surface.grants {
			var subjectID interface{}
			if grant.subjectID != nil {
				subjectID = grant.subjectID.String()
			}
			grantRows.AddRow(
				uuid.New().String(),
				surface.id.String(),
				grant.subjectType,
				subjectID,
				grant.enabled,
				now,
				now,
				nil,
			)
		}
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(grantRows)
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

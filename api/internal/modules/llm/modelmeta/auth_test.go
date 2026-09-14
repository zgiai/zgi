package modelmeta

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/database"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openModelMetaAuthTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
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
		t.Fatalf("open gorm: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}
	return db, mock, cleanup
}

func performSuperAdminRequest(db *gorm.DB, accountID string) (*httptest.ResponseRecorder, bool) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(NewService(db))
	router := gin.New()
	called := false
	router.GET("/protected",
		func(c *gin.Context) {
			if accountID != "" {
				c.Set("account_id", accountID)
			}
		},
		handler.SuperAdminRequired(),
		func(c *gin.Context) {
			called = true
			response.Success(c, gin.H{"ok": true})
		},
	)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	router.ServeHTTP(recorder, req)
	return recorder, called
}

func expectSuperAdminQuery(mock sqlmock.Sqlmock, accountID string) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(`SELECT is_super_admin FROM "accounts" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(accountID, 1)
}

func TestSuperAdminRequired_AllowsSuperAdmin(t *testing.T) {
	db, mock, cleanup := openModelMetaAuthTestDB(t)
	defer cleanup()

	accountID := "11111111-1111-1111-1111-111111111111"
	expectSuperAdminQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"is_super_admin"}).AddRow(true))

	recorder, called := performSuperAdminRequest(db, accountID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !called {
		t.Fatalf("expected protected handler to be called")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSuperAdminRequired_DeniesRegularAccount(t *testing.T) {
	db, mock, cleanup := openModelMetaAuthTestDB(t)
	defer cleanup()

	accountID := "22222222-2222-2222-2222-222222222222"
	expectSuperAdminQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"is_super_admin"}).AddRow(false))

	recorder, called := performSuperAdminRequest(db, accountID)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if called {
		t.Fatalf("expected protected handler not to be called")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSuperAdminRequired_DeniesMissingAccount(t *testing.T) {
	db, mock, cleanup := openModelMetaAuthTestDB(t)
	defer cleanup()

	accountID := "11111111-1111-1111-1111-111111111111"
	expectSuperAdminQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"is_super_admin"}))

	recorder, called := performSuperAdminRequest(db, accountID)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if called {
		t.Fatalf("expected protected handler not to be called")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestSuperAdminRequired_RequiresAuthenticatedAccount(t *testing.T) {
	db, mock, cleanup := openModelMetaAuthTestDB(t)
	defer cleanup()

	recorder, called := performSuperAdminRequest(db, "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body: %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if called {
		t.Fatalf("expected protected handler not to be called")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestModelMetaJWTAndSuperAdminChainDoesNotRequireTenant(t *testing.T) {
	db, mock, cleanup := openModelMetaAuthTestDB(t)
	defer cleanup()
	setModelMetaAuthTestDB(t, db)

	accountID := uuid.NewString()
	expectLastActiveTouch(mock)
	token := newModelMetaAuthTestToken(t, accountID)
	expectJWTAccountQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(accountID, "active"))
	expectSuperAdminQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"is_super_admin"}).AddRow(true))

	recorder, called := performJWTAndSuperAdminRequest(db, "Bearer "+token)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !called {
		t.Fatalf("expected protected handler to be called")
	}
	require.Eventually(t, func() bool { return mock.ExpectationsWereMet() == nil }, time.Second, time.Millisecond)
}

func TestModelMetaJWTAndSuperAdminChainDeniesRegularAccount(t *testing.T) {
	db, mock, cleanup := openModelMetaAuthTestDB(t)
	defer cleanup()
	setModelMetaAuthTestDB(t, db)

	accountID := uuid.NewString()
	expectLastActiveTouch(mock)
	token := newModelMetaAuthTestToken(t, accountID)
	expectJWTAccountQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(accountID, "active"))
	expectSuperAdminQuery(mock, accountID).
		WillReturnRows(sqlmock.NewRows([]string{"is_super_admin"}).AddRow(false))

	recorder, called := performJWTAndSuperAdminRequest(db, "Bearer "+token)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if called {
		t.Fatalf("expected protected handler not to be called")
	}
	require.Eventually(t, func() bool { return mock.ExpectationsWereMet() == nil }, time.Second, time.Millisecond)
}

// JWT authentication schedules a last-active write. Wait for it before closing
// the mocked DB instead of letting a background goroutine outlive the fixture.
func expectLastActiveTouch(mock sqlmock.Sqlmock) {
	mock.MatchExpectationsInOrder(false)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE "accounts"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func setModelMetaAuthTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	previousDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previousDB)
	})
}

func newModelMetaAuthTestToken(t *testing.T, accountID string) string {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() { redisutil.SetClient(previousClient); _ = client.Close() })

	jwtpkg.Init(&config.Config{
		JWT: config.JWTConfig{
			Secret:    "modelmeta-auth-test-secret",
			JWTExpire: time.Hour,
			Issuer:    "test",
		},
	})

	token, err := jwtpkg.GenerateTokenFixed(accountID, "")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

func expectJWTAccountQuery(mock sqlmock.Sqlmock, accountID string) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(`SELECT .*"id".*"status".*FROM "accounts".*WHERE id = \$1.*deleted_at.*LIMIT \$2`).
		WithArgs(accountID, 1)
}

func performJWTAndSuperAdminRequest(db *gorm.DB, authHeader string) (*httptest.ResponseRecorder, bool) {
	gin.SetMode(gin.TestMode)

	handler := NewHandler(NewService(db))
	router := gin.New()
	called := false
	router.GET("/console/api/llm/modelmeta/status",
		middleware.JWT(),
		handler.SuperAdminRequired(),
		func(c *gin.Context) {
			called = true
			response.Success(c, gin.H{"ok": true})
		},
	)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/console/api/llm/modelmeta/status", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	router.ServeHTTP(recorder, req)
	return recorder, called
}

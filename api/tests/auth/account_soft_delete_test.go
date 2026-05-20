package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/system/service"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/ginext/internal/modules/user/auth/repository"
	auth_service "github.com/zgiai/ginext/internal/modules/user/auth/service"
	helper "github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/database"
	jwtpkg "github.com/zgiai/ginext/pkg/jwt"
)

func TestDeleteCurrentAccountSoftDeletesAndKeepsEmailOccupied(t *testing.T) {
	db := newAccountTestDB(t)
	accountService := newAccountServiceForTest(db)
	account := createAccountForTest(t, db, "delete-me@example.com", "secret123")

	if err := accountService.DeleteCurrentAccount(context.Background(), account.ID, "secret123"); err != nil {
		t.Fatalf("DeleteCurrentAccount() error = %v, want nil", err)
	}

	_, err := auth_repo.NewAccountRepository(db).GetAccount(context.Background(), account.ID)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("GetAccount() error = %v, want %v", err, gorm.ErrRecordNotFound)
	}

	var deleted auth_model.Account
	if err := db.Unscoped().Where("id = ?", account.ID).First(&deleted).Error; err != nil {
		t.Fatalf("load deleted account error = %v, want nil", err)
	}
	if deleted.Status != auth_model.AccountStatusClosed {
		t.Fatalf("deleted account status = %s, want %s", deleted.Status, auth_model.AccountStatusClosed)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatal("deleted account deleted_at is not set")
	}

	if !accountService.ExistsByEmail(context.Background(), account.Email) {
		t.Fatal("ExistsByEmail() = false, want true for soft-deleted account")
	}

	if _, err := accountService.Authenticate(context.Background(), account.Email, "secret123", ""); err == nil {
		t.Fatal("Authenticate() error = nil, want failure for soft-deleted account")
	}
}

func TestJWTRejectsSoftDeletedAccount(t *testing.T) {
	db := newAccountTestDB(t)
	accountService := newAccountServiceForTest(db)
	account := createAccountForTest(t, db, "jwt-delete@example.com", "secret123")

	setupJWTTestConfig(t)
	token, err := jwtpkg.GenerateTokenFixed(account.ID, account.Name)
	if err != nil {
		t.Fatalf("GenerateTokenFixed() error = %v, want nil", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", middleware.JWT(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	beforeDelete := httptest.NewRecorder()
	beforeReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	beforeReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(beforeDelete, beforeReq)
	if beforeDelete.Code != http.StatusNoContent {
		t.Fatalf("before delete status = %d, want %d", beforeDelete.Code, http.StatusNoContent)
	}

	if err := accountService.DeleteAccount(context.Background(), account.ID); err != nil {
		t.Fatalf("DeleteAccount() error = %v, want nil", err)
	}

	afterDelete := httptest.NewRecorder()
	afterReq := httptest.NewRequest(http.MethodGet, "/protected", nil)
	afterReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(afterDelete, afterReq)
	if afterDelete.Code != http.StatusNotFound {
		t.Fatalf("after delete status = %d, want %d", afterDelete.Code, http.StatusNotFound)
	}
	if !strings.Contains(afterDelete.Body.String(), "User not found") {
		t.Fatalf("after delete response = %s, want user not found", afterDelete.Body.String())
	}
}

func newAccountTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db error = %v, want nil", err)
	}

	if err := db.AutoMigrate(&auth_model.Account{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v, want nil", err)
	}

	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(oldDB)
	})

	return db
}

func newAccountServiceForTest(db *gorm.DB) *auth_service.AccountService {
	repo := auth_repo.NewAccountRepository(db)
	return auth_service.NewAccountService(
		repo,
		db,
		helper.NewTokenManager(),
		nil,
		nil,
		nil,
		nil,
		nil,
		service.NewSystemConfigService(),
		nil,
		nil,
	)
}

func createAccountForTest(t *testing.T, db *gorm.DB, email, password string) *auth_model.Account {
	t.Helper()

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(password)
	if err != nil {
		t.Fatalf("HashPasswordPBKDF2() error = %v, want nil", err)
	}

	now := time.Now().UTC()
	account := &auth_model.Account{
		ID:           uuid.NewString(),
		Name:         "soft-delete-user",
		Email:        email,
		Password:     &hashedPassword,
		PasswordSalt: &salt,
		Status:       auth_model.AccountStatusActive,
		LastActiveAt: &now,
	}

	if err := db.Create(account).Error; err != nil {
		t.Fatalf("create account error = %v, want nil", err)
	}

	return account
}

func setupJWTTestConfig(t *testing.T) {
	t.Helper()

	oldConfig := config.GlobalConfig
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:    "test-secret",
			JWTExpire: time.Hour,
			Issuer:    "zgi-test",
		},
	}
	config.GlobalConfig = cfg
	jwtpkg.Init(cfg)

	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
		if oldConfig != nil {
			jwtpkg.Init(oldConfig)
		}
	})
}

package service

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResetPasswordWithAutoRegisterDoesNotCreateMissingAccount(t *testing.T) {
	service, mock := newForgotPasswordTestAccountService(t)

	token, err := service.tokenMgr.GenerateToken(
		context.Background(),
		TokenTypeResetPassword,
		nil,
		ptrString("missing@example.com"),
		map[string]interface{}{"code": "123456"},
	)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}

	expectAccountByEmailMissing(mock, "missing@example.com")

	err = service.ResetPasswordWithAutoRegister(token, "Password1")
	if err == nil || !strings.Contains(err.Error(), "account not found") {
		t.Fatalf("ResetPasswordWithAutoRegister error = %v, want account not found", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func newForgotPasswordTestAccountService(t *testing.T) (*AccountService, sqlmock.Sqlmock) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	return NewAccountService(
		auth_repo.NewAccountRepository(db),
		db,
		helper.NewTokenManager(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	), mock
}

func ptrString(value string) *string {
	return &value
}

func expectAccountByEmailMissing(mock sqlmock.Sqlmock, email string) {
	query := regexp.QuoteMeta(`SELECT * FROM "accounts" WHERE LOWER(email) = $1 AND "accounts"."deleted_at" IS NULL ORDER BY "accounts"."id" LIMIT $2`)
	mock.ExpectQuery(query).
		WithArgs(email, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"name",
			"email",
			"status",
			"is_super_admin",
			"created_at",
			"updated_at",
		}))
}

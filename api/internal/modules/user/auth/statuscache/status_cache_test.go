package statuscache

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/pkg/database"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGetAccountStatusCachesInRedisAndInvalidates(t *testing.T) {
	db, mock, cleanupDB := openStatusCacheMockDB(t)
	defer cleanupDB()
	previousDB := database.GetDB()
	database.SetDB(db)
	defer database.SetDB(previousDB)

	redisServer := miniredis.RunT(t)
	redisClient := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(redisClient)
	defer redisutil.SetClient(previousRedis)

	accountID := "3a01dd4e-cc8e-42c4-a8c0-92a0f8249381"
	expectAccountStatusQuery(mock, accountID, auth_model.AccountStatusActive)

	status, err := GetAccountStatus(context.Background(), accountID)
	if err != nil {
		t.Fatalf("first GetAccountStatus error = %v", err)
	}
	if status != auth_model.AccountStatusActive {
		t.Fatalf("first status = %q, want %q", status, auth_model.AccountStatusActive)
	}

	status, err = GetAccountStatus(context.Background(), accountID)
	if err != nil {
		t.Fatalf("cached GetAccountStatus error = %v", err)
	}
	if status != auth_model.AccountStatusActive {
		t.Fatalf("cached status = %q, want %q", status, auth_model.AccountStatusActive)
	}

	InvalidateAccountStatus(context.Background(), accountID)
	expectAccountStatusQuery(mock, accountID, auth_model.AccountStatusFrozen)

	status, err = GetAccountStatus(context.Background(), accountID)
	if err != nil {
		t.Fatalf("after invalidate GetAccountStatus error = %v", err)
	}
	if status != auth_model.AccountStatusFrozen {
		t.Fatalf("after invalidate status = %q, want %q", status, auth_model.AccountStatusFrozen)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func openStatusCacheMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func expectAccountStatusQuery(mock sqlmock.Sqlmock, accountID string, status auth_model.AccountStatus) {
	query := regexp.QuoteMeta(`SELECT "id","status" FROM "accounts" WHERE id = $1 AND "accounts"."deleted_at" IS NULL LIMIT $2`)
	mock.ExpectQuery(query).
		WithArgs(accountID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(accountID, status))
}

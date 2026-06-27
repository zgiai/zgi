package service

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDashboardServiceTableExistsCacheIsConcurrentSafe(t *testing.T) {
	db, mock := openDashboardServiceMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`)).
		WithArgs("agents").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	svc := NewDashboardService(db).(*dashboardService)
	start := make(chan struct{})
	results := make(chan bool, 32)

	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- svc.tableExists(context.Background(), "agents")
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	for result := range results {
		require.True(t, result)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func openDashboardServiceMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	return db, mock
}

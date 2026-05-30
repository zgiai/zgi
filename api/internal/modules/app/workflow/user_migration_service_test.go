package workflow

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserMigrationService_MigrateChatRuntimeConversations(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	virtualID := uuid.New()
	authenticatedID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE chat_runtime_messages AS m SET status = \$1, error = NULL, updated_at = \$2 FROM chat_runtime_conversations AS c WHERE c\.id = m\.conversation_id AND c\.account_id = \$3 AND c\.source = \$4 AND c\.deleted_at IS NULL AND m\.deleted_at IS NULL AND m\.status IN \(\$5,\$6\)`).
		WithArgs("stopped", sqlmock.AnyArg(), virtualID, "webapp", "pending", "streaming").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "chat_runtime_conversations" SET "active_message_id"=\$1,"runtime_status"=\$2,"updated_at"=\$3 WHERE account_id = \$4 AND source = \$5 AND deleted_at IS NULL AND runtime_status = \$6`).
		WithArgs(nil, "idle", sqlmock.AnyArg(), virtualID, "webapp", "streaming").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "chat_runtime_conversations" SET .* WHERE account_id = .* AND source = .* AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	tx := db.Begin()
	rows, err := (&userMigrationService{}).migrateChatRuntimeConversations(
		context.Background(),
		tx,
		virtualID.String(),
		authenticatedID.String(),
	)
	require.NoError(t, err)
	require.NoError(t, tx.Commit().Error)
	require.Equal(t, int64(2), rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

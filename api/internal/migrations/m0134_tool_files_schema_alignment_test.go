package migrations

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	toolfile "github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM0134_tool_files_schema_alignment_RepairsLegacyToolFilesSchema(t *testing.T) {
	db := openLegacyToolFilesMigrationTestDB(t)

	require.NoError(t, M0134_tool_files_schema_alignment().Migrate(db))

	require.True(t, db.Migrator().HasColumn("tool_files", "mimetype"))
	require.True(t, db.Migrator().HasColumn("tool_files", "lifecycle"))
	require.True(t, db.Migrator().HasColumn("tool_files", "expires_at"))
	require.True(t, db.Migrator().HasColumn("tool_files", "created_at"))
	require.True(t, db.Migrator().HasColumn("tool_files", "updated_at"))
	require.True(t, db.Migrator().HasColumn("tool_files", "deleted_at"))
	require.True(t, db.Migrator().HasIndex("tool_files", "idx_tool_files_lifecycle_expires_at"))
	require.True(t, db.Migrator().HasIndex("tool_files", "idx_tool_files_deleted_at"))

	record := &toolfile.ToolFile{
		UserID:   "user-1",
		TenantID: "tenant-1",
		FileKey:  "tools/tenant-1/file.png",
		MimeType: "image/png",
		Name:     "file.png",
		Size:     5,
	}
	require.NoError(t, db.Create(record).Error)

	var count int64
	require.NoError(t, db.Model(&toolfile.ToolFile{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestM0134_tool_files_schema_alignment_RollbackReturnsIrreversibleError(t *testing.T) {
	db := openLegacyToolFilesMigrationTestDB(t)

	err := M0134_tool_files_schema_alignment().Rollback(db)
	require.Error(t, err)
	require.Contains(t, err.Error(), "irreversible")
}

func openLegacyToolFilesMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "tool_files_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE tool_files (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			conversation_id TEXT,
			file_key TEXT NOT NULL,
			mimetype TEXT NOT NULL,
			original_url TEXT,
			name TEXT NOT NULL,
			size INTEGER NOT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE INDEX tool_file_conversation_id_idx ON tool_files(conversation_id)
	`).Error)

	return db
}

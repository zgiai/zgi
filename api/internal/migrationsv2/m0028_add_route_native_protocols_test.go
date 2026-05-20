package migrationsv2

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestM0028_add_route_native_protocols_AddsNativeProtocols(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "route_native_protocols.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		)
	`).Error)

	require.NoError(t, M0028_add_route_native_protocols().Migrate(db))

	exists, err := hasExactColumnV2(db, routeNativeProtocolsTable, "native_protocols")
	require.NoError(t, err)
	require.True(t, exists)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_routes (id, name)
		VALUES ('route-1', 'private route')
	`).Error)

	var nativeProtocols string
	require.NoError(t, db.Raw(`
		SELECT native_protocols
		FROM llm_routes
		WHERE id = 'route-1'
	`).Scan(&nativeProtocols).Error)
	require.Equal(t, "{}", nativeProtocols)
}

func TestM0028_add_route_native_protocols_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "route_native_protocols_idempotent.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			native_protocols TEXT NOT NULL DEFAULT '{}'
		)
	`).Error)

	require.NoError(t, M0028_add_route_native_protocols().Migrate(db))
	require.NoError(t, M0028_add_route_native_protocols().Migrate(db))
}

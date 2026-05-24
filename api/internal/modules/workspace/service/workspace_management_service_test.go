package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetWorkspaceMembersPaginatedReturnsHasMobile(t *testing.T) {
	t.Parallel()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
CREATE TABLE accounts (
	id text primary key,
	name text not null,
	email text not null,
	avatar text,
	status text not null,
	last_login_at datetime,
	last_active_at datetime,
	created_at datetime not null,
	mobile_e164 text
)`).Error)
	require.NoError(t, db.Exec(`
CREATE TABLE workspace_members (
	workspace_id text not null,
	account_id text not null,
	role text not null,
	role_id text,
	created_at datetime not null
)`).Error)

	now := time.Now().UTC()
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, mobile_e164) VALUES (?, ?, ?, ?, ?, ?)`,
		"acc-with-mobile",
		"Mobile User",
		"mobile@example.com",
		"active",
		now,
		"+8613800138000",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, mobile_e164) VALUES (?, ?, ?, ?, ?, ?)`,
		"acc-without-mobile",
		"No Mobile User",
		"nomobile@example.com",
		"active",
		now,
		"",
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO workspace_members (workspace_id, account_id, role, created_at) VALUES (?, ?, ?, ?)`,
		"ws-1",
		"acc-with-mobile",
		"member",
		now,
	).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO workspace_members (workspace_id, account_id, role, created_at) VALUES (?, ?, ?, ?)`,
		"ws-1",
		"acc-without-mobile",
		"member",
		now.Add(time.Second),
	).Error)

	svc := &WorkspaceManagementServiceImpl{db: db}

	members, total, err := svc.GetWorkspaceMembersPaginated(
		context.Background(),
		"ws-1",
		1,
		20,
		"",
		"",
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)

	hasMobileByID := map[string]bool{}
	for _, member := range members {
		hasMobileByID[member.ID] = member.HasMobile
	}

	require.True(t, hasMobileByID["acc-with-mobile"])
	require.False(t, hasMobileByID["acc-without-mobile"])
}

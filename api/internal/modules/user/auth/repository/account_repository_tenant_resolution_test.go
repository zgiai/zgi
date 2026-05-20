package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	testTenantResolutionOrgID       = "fd97bd62-ca46-4901-b381-142d5c9a214b"
	testTenantResolutionWorkspaceID = "9ac6e6ac-c62e-4f17-a861-8f0ce3f4d4fd"
	testTenantResolutionAccountID   = "2f494361-67a0-4371-a0bf-8f357fbb140b"
)

func TestTenantResolution_GetEnterpriseGroupsByTenantID_ResolvesWorkspaceIDToOrganizationID(t *testing.T) {
	t.Parallel()

	repo, db := setupAccountRepositoryTenantResolutionTest(t)
	seedTenantResolutionBaseData(t, db)

	groupIDs, err := repo.GetEnterpriseGroupsByTenantID(context.Background(), testTenantResolutionWorkspaceID)
	require.NoError(t, err)
	require.Equal(t, []string{testTenantResolutionOrgID}, groupIDs)
}

func TestTenantResolution_GetEnterpriseGroupsByTenantID_ResolvesOrganizationIDToSelf(t *testing.T) {
	t.Parallel()

	repo, db := setupAccountRepositoryTenantResolutionTest(t)
	seedTenantResolutionBaseData(t, db)

	groupIDs, err := repo.GetEnterpriseGroupsByTenantID(context.Background(), testTenantResolutionOrgID)
	require.NoError(t, err)
	require.Equal(t, []string{testTenantResolutionOrgID}, groupIDs)
}

func TestTenantResolution_GetEnterpriseGroupsByTenantID_MissingIDReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	repo, _ := setupAccountRepositoryTenantResolutionTest(t)

	groupIDs, err := repo.GetEnterpriseGroupsByTenantID(context.Background(), "non-existent-id")
	require.NoError(t, err)
	require.Empty(t, groupIDs)
}

func TestTenantResolution_GetGroupRoleByTenantID_UsesOrganizationIDInput(t *testing.T) {
	t.Parallel()

	repo, db := setupAccountRepositoryTenantResolutionTest(t)
	seedTenantResolutionBaseData(t, db)

	require.NoError(t, db.Exec(
		"INSERT INTO members (organization_id, account_id, role) VALUES (?, ?, ?)",
		testTenantResolutionOrgID,
		testTenantResolutionAccountID,
		"admin",
	).Error)

	role, err := repo.GetGroupRoleByTenantID(context.Background(), testTenantResolutionAccountID, testTenantResolutionOrgID)
	require.NoError(t, err)
	require.Equal(t, "admin", role)
}

func setupAccountRepositoryTenantResolutionTest(t *testing.T) (AccountRepository, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skip("sqlite driver requires cgo in this environment")
		}
		t.Fatalf("failed to open sqlite db: %v", err)
	}

	statements := []string{
		"CREATE TABLE organizations (id TEXT PRIMARY KEY)",
		"CREATE TABLE workspaces (id TEXT PRIMARY KEY, organization_id TEXT)",
		"CREATE TABLE members (organization_id TEXT, account_id TEXT, role TEXT)",
	}
	for _, stmt := range statements {
		require.NoError(t, db.Exec(stmt).Error)
	}

	return NewAccountRepository(db), db
}

func seedTenantResolutionBaseData(t *testing.T, db *gorm.DB) {
	t.Helper()

	require.NoError(t, db.Exec("INSERT INTO organizations (id) VALUES (?)", testTenantResolutionOrgID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO workspaces (id, organization_id) VALUES (?, ?)",
		testTenantResolutionWorkspaceID,
		testTenantResolutionOrgID,
	).Error)
}

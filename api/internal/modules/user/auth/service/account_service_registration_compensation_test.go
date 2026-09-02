package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
)

func TestDeleteUnboundAccountPermanently(t *testing.T) {
	t.Run("deletes unbound account", func(t *testing.T) {
		service, db, account := newRegistrationCompensationTestService(t)

		deleted, err := service.DeleteUnboundAccountPermanently(t.Context(), account)

		require.NoError(t, err)
		require.True(t, deleted)
		var count int64
		require.NoError(t, db.Unscoped().Model(&auth_model.Account{}).Where("id = ?", account.ID).Count(&count).Error)
		require.Zero(t, count)
	})

	tests := []struct {
		name string
		bind func(t *testing.T, db *gorm.DB, accountID string)
	}{
		{
			name: "keeps organization member",
			bind: func(t *testing.T, db *gorm.DB, accountID string) {
				require.NoError(t, db.Exec(
					"INSERT INTO members (organization_id, account_id) VALUES (?, ?)",
					uuid.NewString(), accountID,
				).Error)
			},
		},
		{
			name: "keeps workspace member",
			bind: func(t *testing.T, db *gorm.DB, accountID string) {
				require.NoError(t, db.Exec(
					"INSERT INTO workspace_members (workspace_id, account_id) VALUES (?, ?)",
					uuid.NewString(), accountID,
				).Error)
			},
		},
		{
			name: "keeps account context",
			bind: func(t *testing.T, db *gorm.DB, accountID string) {
				require.NoError(t, db.Exec(
					"INSERT INTO account_contexts (account_id, current_organization_id) VALUES (?, ?)",
					accountID, uuid.NewString(),
				).Error)
			},
		},
		{
			name: "keeps organization join request",
			bind: func(t *testing.T, db *gorm.DB, accountID string) {
				require.NoError(t, db.Exec(
					"INSERT INTO organization_join_requests (id, account_id) VALUES (?, ?)",
					uuid.NewString(), accountID,
				).Error)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, db, account := newRegistrationCompensationTestService(t)
			tt.bind(t, db, account.ID)

			deleted, err := service.DeleteUnboundAccountPermanently(t.Context(), account)

			require.NoError(t, err)
			require.False(t, deleted)
			var count int64
			require.NoError(t, db.Unscoped().Model(&auth_model.Account{}).Where("id = ?", account.ID).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}

func newRegistrationCompensationTestService(t *testing.T) (*AccountService, *gorm.DB, *auth_model.Account) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&auth_model.Account{}))
	for _, statement := range []string{
		"CREATE TABLE members (organization_id TEXT NOT NULL, account_id TEXT NOT NULL)",
		"CREATE TABLE workspace_members (workspace_id TEXT NOT NULL, account_id TEXT NOT NULL)",
		"CREATE TABLE account_contexts (account_id TEXT NOT NULL, current_organization_id TEXT, current_workspace_id TEXT)",
		"CREATE TABLE organization_join_requests (id TEXT NOT NULL, account_id TEXT NOT NULL)",
	} {
		require.NoError(t, db.Exec(statement).Error)
	}

	account := &auth_model.Account{
		ID:     uuid.NewString(),
		Name:   "Invited User",
		Email:  uuid.NewString() + "@example.com",
		Status: auth_model.AccountStatusActive,
	}
	require.NoError(t, db.Create(account).Error)
	return &AccountService{accountRepo: auth_repo.NewAccountRepository(db)}, db, account
}

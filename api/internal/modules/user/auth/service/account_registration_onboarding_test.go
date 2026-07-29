package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	"gorm.io/gorm"
)

type onboardingRegistrationAccountRepository struct {
	auth_repo.AccountRepository
	created *auth_model.Account
}

func (r *onboardingRegistrationAccountRepository) ExecuteInTransaction(_ context.Context, fn func(tx *gorm.DB) error) error {
	return fn(&gorm.DB{})
}

func (r *onboardingRegistrationAccountRepository) WithTx(_ *gorm.DB) auth_repo.AccountRepository {
	return r
}

func (r *onboardingRegistrationAccountRepository) CreateAccount(_ context.Context, account *auth_model.Account) error {
	account.ID = "account-1"
	r.created = account
	return nil
}

func TestRegisterExCanCreateAccountWithoutOrganization(t *testing.T) {
	repo := &onboardingRegistrationAccountRepository{}
	service := &AccountService{accountRepo: repo}
	password := "secret123"
	language := "en-US"
	createWorkspace := false

	account, err := service.RegisterEx(
		t.Context(),
		"user@example.com",
		"User",
		&password,
		nil,
		nil,
		&language,
		nil,
		nil,
		&createWorkspace,
	)

	require.NoError(t, err)
	require.Same(t, repo.created, account)
	require.Equal(t, "account-1", account.ID)
	require.Equal(t, "user@example.com", account.Email)
}

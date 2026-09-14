package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	authrepo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type recordingRegistrationRoutes struct {
	steps *[]string
	err   error
}

func (r *recordingRegistrationRoutes) InitOfficialChannel(context.Context, uuid.UUID) error {
	*r.steps = append(*r.steps, "official-route")
	return r.err
}

type recordingRegistrationConsole struct {
	platformconsole.ConsoleProvider
	steps        *[]string
	registerErr  error
	grantErr     error
	registerReqs []*platformconsole.RegisterOrganizationRequest
}

type asyncOnlyRegistrationConsole struct {
	platformconsole.ConsoleProvider
}

func (*asyncOnlyRegistrationConsole) IsAvailable() bool { return true }
func (*asyncOnlyRegistrationConsole) GetMode() string   { return registrationRunModeCloud }

func (c *recordingRegistrationConsole) IsAvailable() bool { return true }
func (c *recordingRegistrationConsole) GetMode() string   { return registrationRunModeCloud }
func (c *recordingRegistrationConsole) RegisterOrganization(_ context.Context, req *platformconsole.RegisterOrganizationRequest) error {
	return c.RegisterOrganizationSync(context.Background(), req)
}
func (c *recordingRegistrationConsole) RegisterOrganizationSync(_ context.Context, req *platformconsole.RegisterOrganizationRequest) error {
	*c.steps = append(*c.steps, "register-organization")
	c.registerReqs = append(c.registerReqs, req)
	return c.registerErr
}
func (c *recordingRegistrationConsole) NotifyOfficialSignup(context.Context, *platformconsole.NotifyOfficialSignupRequest) (*platformconsole.NotifyOfficialSignupResponse, error) {
	*c.steps = append(*c.steps, "signup-grant")
	if c.grantErr != nil {
		return nil, c.grantErr
	}
	return &platformconsole.NotifyOfficialSignupResponse{}, nil
}

func TestRegistrationProvisioningOutboxPersistsOrderedSteps(t *testing.T) {
	db, entry, account, organization := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	routes := &recordingRegistrationRoutes{steps: &steps}
	console := &recordingRegistrationConsole{steps: &steps}
	processor := NewRegistrationProvisioningOutboxProcessor(db, routes, console)
	processor.now = func() time.Time { return entry.NextAttemptAt.Add(time.Second) }

	processed, err := processor.ProcessPending(t.Context(), 10)

	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Equal(t, []string{"official-route", "register-organization", "signup-grant"}, steps)
	require.Len(t, console.registerReqs, 1)
	require.Equal(t, account.Email, console.registerReqs[0].OwnerEmail)
	require.Equal(t, organization.Name, console.registerReqs[0].Name)
	require.Equal(t, organization.CreatedAt, console.registerReqs[0].CreatedAt)

	var persisted RegistrationProvisioningOutbox
	require.NoError(t, db.First(&persisted, "id = ?", entry.ID).Error)
	require.Equal(t, registrationProvisioningOutboxCompleted, persisted.Status)
	require.Equal(t, registrationProvisioningStepGrant, persisted.CompletedStep)
	require.NotNil(t, persisted.CompletedAt)
	require.Nil(t, persisted.LeaseOwner)
	require.Nil(t, persisted.LeaseUntil)
}

func TestRegistrationProvisioningOutboxRequiresSynchronousOrganizationRegistration(t *testing.T) {
	db, _, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	processor := NewRegistrationProvisioningOutboxProcessor(
		db,
		&recordingRegistrationRoutes{steps: &steps},
		&asyncOnlyRegistrationConsole{},
	)

	require.ErrorContains(t, processor.ValidateConfiguration(), "synchronous organization registration")
}

func TestRegistrationProvisioningOutboxRetriesFromPersistedStep(t *testing.T) {
	db, entry, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	routes := &recordingRegistrationRoutes{steps: &steps}
	console := &recordingRegistrationConsole{steps: &steps, registerErr: errors.New("console unavailable")}
	processor := NewRegistrationProvisioningOutboxProcessor(db, routes, console)
	now := entry.NextAttemptAt.Add(time.Second)
	processor.now = func() time.Time { return now }

	processed, err := processor.ProcessPending(t.Context(), 10)
	require.Equal(t, 1, processed)
	require.ErrorContains(t, err, "organization_registration:failed")
	require.NotContains(t, err.Error(), "console unavailable")

	var retrying RegistrationProvisioningOutbox
	require.NoError(t, db.First(&retrying, "id = ?", entry.ID).Error)
	require.Equal(t, registrationProvisioningOutboxPending, retrying.Status)
	require.Equal(t, registrationProvisioningStepRoute, retrying.CompletedStep)
	require.Equal(t, 1, retrying.AttemptCount)
	require.True(t, retrying.NextAttemptAt.After(now))
	require.Equal(t, "organization_registration:failed", retrying.LastError)
	require.NotContains(t, retrying.LastError, "console unavailable")

	console.registerErr = nil
	now = retrying.NextAttemptAt.Add(time.Second)
	processed, err = processor.ProcessPending(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Equal(t, []string{
		"official-route", "register-organization",
		"register-organization", "signup-grant",
	}, steps, "a persisted route step must not be replayed, and grant must follow successful registration")
}

func TestRegistrationProvisioningOutboxLeasePreventsConcurrentClaimAndAllowsRecovery(t *testing.T) {
	db, entry, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	first := NewRegistrationProvisioningOutboxProcessor(db, &recordingRegistrationRoutes{steps: &steps}, &recordingRegistrationConsole{steps: &steps})
	now := entry.NextAttemptAt.Add(time.Second)
	first.now = func() time.Time { return now }
	claimed, err := first.claimPending(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	second := NewRegistrationProvisioningOutboxProcessor(db, &recordingRegistrationRoutes{steps: &steps}, &recordingRegistrationConsole{steps: &steps})
	second.now = func() time.Time { return now.Add(first.leaseDuration / 2) }
	processed, err := second.ProcessPending(t.Context(), 1)
	require.NoError(t, err)
	require.Zero(t, processed, "a live lease must not be claimed by another instance")

	second.now = func() time.Time { return now.Add(first.leaseDuration + time.Second) }
	processed, err = second.ProcessPending(t.Context(), 1)
	require.NoError(t, err)
	require.Equal(t, 1, processed, "an expired lease must be recoverable after restart")
}

func TestRegistrationProvisioningOutboxExpiredReclaimFencesStaleSameProcessorWork(t *testing.T) {
	db, entry, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	processor := NewRegistrationProvisioningOutboxProcessor(
		db,
		&recordingRegistrationRoutes{steps: &steps},
		&recordingRegistrationConsole{steps: &steps},
	)
	firstClaimTime := entry.NextAttemptAt.Add(time.Second)
	processor.now = func() time.Time { return firstClaimTime }
	first, err := processor.claimPending(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, first, 1)

	processor.now = func() time.Time { return firstClaimTime.Add(processor.leaseDuration + time.Second) }
	second, err := processor.claimPending(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.NotEqual(t, registrationProvisioningLeaseOwner(&first[0]), registrationProvisioningLeaseOwner(&second[0]))

	require.ErrorContains(t,
		processor.persistStep(t.Context(), &first[0], registrationProvisioningStepRoute),
		"lease lost",
		"an expired claim must not checkpoint after the same processor has reclaimed the row",
	)
	require.NoError(t, processor.persistStep(t.Context(), &second[0], registrationProvisioningStepRoute))

	var persisted RegistrationProvisioningOutbox
	require.NoError(t, db.First(&persisted, "id = ?", entry.ID).Error)
	require.Equal(t, registrationProvisioningStepRoute, persisted.CompletedStep)
	require.Equal(t, registrationProvisioningLeaseOwner(&second[0]), registrationProvisioningLeaseOwner(&persisted))
}

func TestRegistrationProvisioningOutboxTaskPollsWithinProvisioningWindow(t *testing.T) {
	interval := (&RegistrationProvisioningOutboxTask{}).Interval()
	require.GreaterOrEqual(t, interval, 2*time.Second)
	require.LessOrEqual(t, interval, 5*time.Second)
}

func TestRegistrationProvisioningOutboxCanceledParentStillReleasesLease(t *testing.T) {
	db, entry, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	processor := NewRegistrationProvisioningOutboxProcessor(db, &recordingRegistrationRoutes{steps: &steps}, &recordingRegistrationConsole{steps: &steps})
	processor.now = func() time.Time { return entry.NextAttemptAt.Add(time.Second) }
	claimed, err := processor.claimPending(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	err = processor.processClaimed(canceledCtx, &claimed[0])
	require.ErrorContains(t, err, "organization_lookup:canceled")
	require.NotContains(t, err.Error(), context.Canceled.Error())

	var persisted RegistrationProvisioningOutbox
	require.NoError(t, db.First(&persisted, "id = ?", entry.ID).Error)
	require.Equal(t, registrationProvisioningOutboxPending, persisted.Status)
	require.Nil(t, persisted.LeaseOwner)
	require.Nil(t, persisted.LeaseUntil)
	require.Equal(t, "organization_lookup:canceled", persisted.LastError)
}

func TestRegistrationProvisioningOutboxCanceledParentStillPersistsCheckpoints(t *testing.T) {
	db, entry, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	processor := NewRegistrationProvisioningOutboxProcessor(db, &recordingRegistrationRoutes{steps: &steps}, &recordingRegistrationConsole{steps: &steps})
	processor.now = func() time.Time { return entry.NextAttemptAt.Add(time.Second) }
	claimed, err := processor.claimPending(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, processor.persistStep(canceledCtx, &claimed[0], registrationProvisioningStepOrganization))
	require.NoError(t, processor.complete(canceledCtx, &claimed[0]))

	var persisted RegistrationProvisioningOutbox
	require.NoError(t, db.First(&persisted, "id = ?", entry.ID).Error)
	require.Equal(t, registrationProvisioningOutboxCompleted, persisted.Status)
	require.Equal(t, registrationProvisioningStepGrant, persisted.CompletedStep)
}

func TestRegistrationProvisioningOutboxPersistsOnlySafeRemoteFailureClass(t *testing.T) {
	db, entry, _, _ := newRegistrationProvisioningOutboxFixture(t)
	steps := []string{}
	processor := NewRegistrationProvisioningOutboxProcessor(db, &recordingRegistrationRoutes{steps: &steps}, &recordingRegistrationConsole{steps: &steps})
	processor.now = func() time.Time { return entry.NextAttemptAt.Add(time.Second) }
	claimed, err := processor.claimPending(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	cause := fmt.Errorf("register response included owner@example.com: %w", &platformconsole.ConsoleAPIError{
		StatusCode: 502,
		Message:    "owner@example.com failed",
	})

	err = processor.retry(t.Context(), &claimed[0], cause, "organization_registration")
	require.Error(t, err)
	require.ErrorContains(t, err, "organization_registration:http_502")
	require.NotContains(t, err.Error(), "owner@example.com")

	var persisted RegistrationProvisioningOutbox
	require.NoError(t, db.First(&persisted, "id = ?", entry.ID).Error)
	require.Equal(t, "organization_registration:http_502", persisted.LastError)
	require.NotContains(t, persisted.LastError, "owner@example.com")
}

func TestRegisterExRollsBackCloudAccountWhenOutboxWriteFails(t *testing.T) {
	db := newAccountRegistrationOutboxTestDB(t, false)
	organizationID := uuid.NewString()
	service := &AccountService{
		accountRepo: authrepo.NewAccountRepository(db),
		registrationProvisioner: &staticRegistrationAccountProvisioner{result: &RegistrationProvisioningResult{
			OrganizationID: organizationID, CreatedOrganization: true, RequiresCloudOutbox: true,
		}},
	}
	required := true

	account, err := service.RegisterEx(t.Context(), "missing-outbox@example.com", "Missing Outbox", nil, nil, nil, nil, nil, nil, &required)

	require.Nil(t, account)
	require.ErrorContains(t, err, "registration_provisioning_outbox")
	var accountCount int64
	require.NoError(t, db.Model(&authmodel.Account{}).Count(&accountCount).Error)
	require.Zero(t, accountCount, "an outbox persistence failure must roll back the account")
}

func TestRegisterExRollsBackOutboxWithLaterTransactionFailure(t *testing.T) {
	db := newAccountRegistrationOutboxTestDB(t, true)
	organizationID := uuid.NewString()
	service := &AccountService{
		accountRepo: authrepo.NewAccountRepository(db),
		eventBus:    failingRegistrationEventBus{},
		registrationProvisioner: &staticRegistrationAccountProvisioner{result: &RegistrationProvisioningResult{
			OrganizationID: organizationID, CreatedOrganization: true, RequiresCloudOutbox: true,
			CreatedWorkspace: &workspacemodel.Workspace{ID: uuid.NewString()},
		}},
	}
	required := true

	account, err := service.RegisterEx(t.Context(), "rollback-outbox@example.com", "Rollback Outbox", nil, nil, nil, nil, nil, nil, &required)

	require.Nil(t, account)
	require.ErrorContains(t, err, "forced post-outbox failure")
	var accountCount, outboxCount int64
	require.NoError(t, db.Model(&authmodel.Account{}).Count(&accountCount).Error)
	require.NoError(t, db.Model(&RegistrationProvisioningOutbox{}).Count(&outboxCount).Error)
	require.Zero(t, accountCount)
	require.Zero(t, outboxCount)
}

func TestRegisterExDispatcherFailureDoesNotFailCommittedRegistration(t *testing.T) {
	db := newAccountRegistrationOutboxTestDB(t, true)
	organizationID := uuid.NewString()
	service := &AccountService{
		accountRepo: authrepo.NewAccountRepository(db),
		registrationProvisioner: &staticRegistrationAccountProvisioner{result: &RegistrationProvisioningResult{
			OrganizationID: organizationID, CreatedOrganization: true, RequiresCloudOutbox: true,
		}},
		registrationOutboxDispatcher: func(context.Context, string) error { return errors.New("immediate dispatch failed") },
	}
	required := true

	account, err := service.RegisterEx(t.Context(), "durable-outbox@example.com", "Durable Outbox", nil, nil, nil, nil, nil, nil, &required)

	require.NoError(t, err)
	require.NotNil(t, account)
	var outbox RegistrationProvisioningOutbox
	require.NoError(t, db.Where("account_id = ?", account.ID).First(&outbox).Error)
	require.Equal(t, registrationProvisioningOutboxPending, outbox.Status)
}

type failingRegistrationEventBus struct{}

func (failingRegistrationEventBus) Publish(context.Context, string, interface{}) error {
	return errors.New("forced post-outbox failure")
}

func newRegistrationProvisioningOutboxFixture(t *testing.T) (*gorm.DB, *RegistrationProvisioningOutbox, *authmodel.Account, *workspacemodel.Organization) {
	t.Helper()
	db := newAccountRegistrationOutboxTestDB(t, true)
	account := &authmodel.Account{ID: uuid.NewString(), Name: "Cloud Owner", Email: fmt.Sprintf("%s@example.com", uuid.NewString())}
	organization := &workspacemodel.Organization{
		ID: uuid.NewString(), Name: "Cloud Organization", Status: workspacemodel.OrganizationStatusActive, CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	require.NoError(t, db.Create(account).Error)
	require.NoError(t, db.Create(organization).Error)
	var entry *RegistrationProvisioningOutbox
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		entry, err = EnqueueRegistrationProvisioningOutbox(t.Context(), tx, account.ID, organization.ID)
		return err
	}))
	return db, entry, account, organization
}

func newAccountRegistrationOutboxTestDB(t *testing.T, migrateOutbox bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&authmodel.Account{}, &workspacemodel.Organization{}))
	if migrateOutbox {
		require.NoError(t, db.AutoMigrate(&RegistrationProvisioningOutbox{}))
	}
	return db
}

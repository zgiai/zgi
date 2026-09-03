package onboarding

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
)

type memoryFlows struct {
	flow    Flow
	device  DeviceRecord
	commits int
	values  map[string]string
}

func (m *memoryFlows) FindRequest(_ context.Context, org, owner uuid.UUID, hash string) (Flow, bool, error) {
	f := m.flow
	return f, f.ID != uuid.Nil && f.OrganizationID == org && f.OwnerAccountID == owner && f.IdempotencyKeyHash == hash, nil
}

func (m *memoryFlows) Admit(_ context.Context, f Flow) (Flow, error) {
	if m.flow.ID != uuid.Nil {
		if m.flow.RequestFingerprint != f.RequestFingerprint {
			return Flow{}, registrationError(core.ErrorCodeConflict)
		}
		return m.flow, nil
	}
	m.flow = f
	return f, nil
}
func (m *memoryFlows) Get(_ context.Context, org, owner, id uuid.UUID) (Flow, error) {
	f := m.flow
	if f.OrganizationID != org || f.OwnerAccountID != owner || f.ID != id {
		return Flow{}, registrationError(core.ErrorCodeNotFound)
	}
	return f, nil
}
func (m *memoryFlows) Cancel(ctx context.Context, org, owner, id uuid.UUID, rev int64) (Flow, error) {
	f, err := m.Get(ctx, org, owner, id)
	if err != nil {
		return Flow{}, err
	}
	if f.State.Active() {
		if f.Revision != rev {
			return Flow{}, registrationError(core.ErrorCodeConflict)
		}
		m.flow.State = Cancelled
		m.flow.Revision++
		m.device = DeviceRecord{}
	}
	return m.flow, nil
}
func (m *memoryFlows) Claim(_ context.Context, id uuid.UUID) (Flow, bool, error) {
	if !m.flow.State.Active() {
		return Flow{}, false, nil
	}
	until := time.Now().Add(time.Minute)
	m.flow.LeaseOwner = &id
	m.flow.LeaseUntil = &until
	m.flow.Fencing++
	return m.flow, true, nil
}
func (m *memoryFlows) Device(ctx context.Context, f Flow) (DeviceRecord, error) {
	if _, err := m.Get(ctx, f.OrganizationID, f.OwnerAccountID, f.ID); err != nil {
		return DeviceRecord{}, err
	}
	return m.device, nil
}
func (m *memoryFlows) AwaitAuthorization(_ context.Context, f Flow, d DeviceRecord, _ time.Duration, _ time.Duration) error {
	if !m.flow.State.Active() {
		return registrationError(core.ErrorCodeConflict)
	}
	m.device = d
	m.flow.State = Awaiting
	m.flow.LeaseOwner = nil
	m.flow.LeaseUntil = nil
	m.flow.Revision++
	return nil
}
func (m *memoryFlows) Advance(_ context.Context, _ Flow, region string, switched bool, interval int) error {
	m.flow.Region = region
	m.flow.SwitchedRegion = switched
	m.flow.PollIntervalSeconds = interval
	return nil
}
func (m *memoryFlows) Terminate(_ context.Context, f Flow, state State, code core.ErrorCode) error {
	if !m.flow.State.Active() || m.flow.Fencing != f.Fencing {
		return registrationError(core.ErrorCodeConflict)
	}
	m.flow.State = state
	m.flow.SafeErrorCode = &code
	m.device = DeviceRecord{}
	return nil
}
func (m *memoryFlows) Complete(_ context.Context, f Flow, values map[string]string) (uuid.UUID, error) {
	if !m.flow.State.Active() || m.flow.Fencing != f.Fencing {
		return uuid.Nil, registrationError(core.ErrorCodeConflict)
	}
	m.commits++
	m.values = map[string]string{}
	for k, v := range values {
		m.values[k] = v
	}
	id := uuid.New()
	m.flow.State = Ready
	m.flow.ResultConnectionID = &id
	m.device = DeviceRecord{}
	return id, nil
}

type authorityFixture struct {
	err   error
	calls int
}

func (a *authorityFixture) Check(context.Context, Flow) error { a.calls++; return a.err }

type connectionFixture struct{ value core.PersonalConnection }

func (c connectionFixture) GetConnection(_ context.Context, org, owner, id uuid.UUID) (core.PersonalConnection, error) {
	if c.value.ID != id || c.value.OrganizationID != org || c.value.OwnerAccountID != owner {
		return core.PersonalConnection{}, registrationError(core.ErrorCodeNotFound)
	}
	return c.value, nil
}

type registrarFixture struct {
	beginCalls, pollCalls int
	poll                  func() (RegistrationResult, error)
	beginErr              error
}

func (r *registrarFixture) Begin(context.Context, string, string) (RegistrationSession, error) {
	r.beginCalls++
	return RegistrationSession{DeviceCode: "DEVICE-SECRET", VerificationURL: "https://accounts.feishu.cn/app?user_code=ABCD", UserCode: "ABCD", Interval: 5 * time.Second, TTL: 600 * time.Second}, r.beginErr
}
func (r *registrarFixture) Poll(context.Context, string, string, bool) (RegistrationResult, error) {
	r.pollCalls++
	return r.poll()
}
func serviceFixture(t *testing.T) (*Service, *memoryFlows, *registrarFixture, *authorityFixture, StartCommand) {
	t.Helper()
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := catalog.New(d)
	if err != nil {
		t.Fatal(err)
	}
	store, err := credentials.NewStore("test", map[string]string{"test": strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := connections.NewCommandHasher(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	repo := &memoryFlows{}
	registrar := &registrarFixture{poll: func() (RegistrationResult, error) {
		return RegistrationResult{Status: "complete", AppID: "cli_fixture", AppSecret: "APP-SECRET"}, nil
	}}
	authority := &authorityFixture{}
	s, err := NewService(repo, definitions, connectionFixture{}, store, hasher, authority, registrar)
	if err != nil {
		t.Fatal(err)
	}
	command := StartCommand{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: d.ID, Region: "feishu", TemplateID: d.Setup.TemplateID, IdempotencyKey: uuid.NewString()}
	return s, repo, registrar, authority, command
}
func TestReadOnlyPollingNeverBeginsRegistrationAndCompletionIsSecretFree(t *testing.T) {
	s, repo, registrar, _, command := serviceFixture(t)
	ctx := context.Background()
	snapshot, err := s.Start(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err = s.Get(ctx, command.OrganizationID, command.OwnerAccountID, snapshot.ID); err != nil {
			t.Fatal(err)
		}
	}
	if registrar.beginCalls != 0 {
		t.Fatal("GET starts external app")
	}
	if _, err = s.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.Get(ctx, command.OrganizationID, command.OwnerAccountID, snapshot.ID)
	if err != nil || snapshot.VerificationURL == "" {
		t.Fatalf("QR unavailable: %v", err)
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "DEVICE-SECRET") || strings.Contains(repo.device.EncryptedDevice, "DEVICE-SECRET") {
		t.Fatal("device secret leaked")
	}
	if _, err = s.ProcessOne(ctx); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.Get(ctx, command.OrganizationID, command.OwnerAccountID, snapshot.ID)
	if err != nil || snapshot.State != Ready || repo.commits != 1 || repo.values["app_secret"] != "APP-SECRET" || repo.device.EncryptedDevice != "" {
		t.Fatalf("completion %v", err)
	}
	if _, err = s.ProcessOne(ctx); err != nil || repo.commits != 1 {
		t.Fatal("completed flow replayed")
	}
	encoded, _ = json.Marshal(snapshot)
	if strings.Contains(string(encoded), "APP-SECRET") {
		t.Fatal("app secret leaked")
	}
}

func TestRegistrationTemplateChangeNeverReplacesFrozenRequest(t *testing.T) {
	s, repo, registrar, authority, command := serviceFixture(t)
	ctx := context.Background()
	oldRequest := command
	oldRequest.TemplateID = "feishu-app-template-v1"
	if _, err := s.Start(ctx, oldRequest); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) || repo.flow.ID != uuid.Nil || registrar.beginCalls != 0 {
		t.Fatal("new requests cannot use the retired template")
	}
	first, err := s.Start(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	// An existing receipt is recoverable under its original request, never
	// rewritten to the new template using the same idempotency key.
	repo.flow.TemplateID = oldRequest.TemplateID
	if _, err := s.Start(ctx, command); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("idempotency key changed template")
	}
	got, err := s.Start(ctx, oldRequest)
	if err != nil || got.ID != first.ID || registrar.beginCalls != 0 {
		t.Fatal("old receipt recovery must not begin another application")
	}
	// Production authority rejects the changed definition before begin/poll.
	authority.err = core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	if _, err := s.ProcessOne(ctx); !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) || repo.flow.State != Failed || registrar.beginCalls != 0 || registrar.pollCalls != 0 || repo.commits != 0 {
		t.Fatal("retired flow reached registration or credential commit")
	}
}

func TestRegistrationReplayKeepsOriginalRegionAfterOfficialSwitch(t *testing.T) {
	s, repo, registrar, _, command := serviceFixture(t)
	first, err := s.Start(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	repo.flow.Region = "lark"
	repo.flow.SwitchedRegion = true
	second, err := s.Start(context.Background(), command)
	if err != nil || second.ID != first.ID || second.Region != "lark" || registrar.beginCalls != 0 {
		t.Fatalf("region replay %v", err)
	}
	command.Region = "lark"
	if _, err := s.Start(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("changed original request accepted")
	}
}
func TestCancellationWinsAgainstLateCredentials(t *testing.T) {
	s, repo, registrar, _, command := serviceFixture(t)
	ctx := context.Background()
	_, _ = s.Start(ctx, command)
	_, _ = s.ProcessOne(ctx)
	registrar.poll = func() (RegistrationResult, error) {
		_, err := s.Cancel(ctx, command.OrganizationID, command.OwnerAccountID, repo.flow.ID, repo.flow.Revision)
		if err != nil {
			t.Fatal(err)
		}
		return RegistrationResult{Status: "complete", AppID: "cli_fixture", AppSecret: "LATE-SECRET"}, nil
	}
	_, _ = s.ProcessOne(ctx)
	if repo.flow.State != Cancelled || repo.commits != 0 || repo.device.EncryptedDevice != "" {
		t.Fatal("late poll revived cancelled flow")
	}
}
func TestUnknownBeginIsNotRetriedAndRevokedOwnerCannotPoll(t *testing.T) {
	s, repo, registrar, authority, command := serviceFixture(t)
	ctx := context.Background()
	_, _ = s.Start(ctx, command)
	registrar.beginErr = registrationError(core.ErrorCodeOutcomeUnknown)
	_, _ = s.ProcessOne(ctx)
	_, _ = s.ProcessOne(ctx)
	if repo.flow.State != RecoveryRequired || registrar.beginCalls != 1 {
		t.Fatal("unknown begin replayed")
	}
	s, repo, registrar, authority, command = serviceFixture(t)
	_, _ = s.Start(ctx, command)
	_, _ = s.ProcessOne(ctx)
	authority.err = registrationError(core.ErrorCodeAccessDenied)
	_, _ = s.ProcessOne(ctx)
	if registrar.pollCalls != 0 || repo.commits != 0 || repo.device.EncryptedDevice != "" {
		t.Fatal("revoked owner reached provider")
	}
}
func TestCrossOwnerReadAndStaleUpdateAreRejectedBeforeRegistration(t *testing.T) {
	s, repo, registrar, _, command := serviceFixture(t)
	ctx := context.Background()
	snapshot, err := s.Start(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(ctx, command.OrganizationID, uuid.New(), snapshot.ID); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatal("cross-owner flow read")
	}
	identity := "feishu:cli_existing"
	connection := core.PersonalConnection{ID: uuid.New(), OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID, ConnectorID: "feishu", ExternalAccountID: &identity, Revision: 4, CredentialVersion: 2}
	s.connections = connectionFixture{value: connection}
	command.ConnectionID = connection.ID
	command.ExpectedRevision = 3
	if _, err = s.Start(ctx, command); !core.IsErrorCode(err, core.ErrorCodeConflict) || registrar.beginCalls != 0 {
		t.Fatal("stale update reached registration")
	}
	repo.flow = Flow{}
	command.ExpectedRevision = 4
	command.IdempotencyKey = uuid.NewString()
	if _, err = s.Start(ctx, command); err != nil {
		t.Fatal(err)
	}
	_, _ = s.ProcessOne(ctx)
	_, _ = s.ProcessOne(ctx)
	if repo.commits != 0 || repo.flow.State != Failed {
		t.Fatal("update accepted another app")
	}
}

package onboarding

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
)

type ConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
}
type StartCommand struct {
	Intent           string
	ProtocolVersion  int
	OrganizationID   uuid.UUID
	OwnerAccountID   uuid.UUID
	ConnectorID      string
	Region           string
	TemplateID       string
	ConnectionID     uuid.UUID
	ExpectedRevision int
	IdempotencyKey   string `json:"-"`
}
type Service struct {
	repository  Repository
	catalog     connections.DefinitionCatalog
	connections ConnectionReader
	secrets     *credentials.Store
	hasher      *connections.CommandHasher
	authority   Authority
	registrar   Registrar
	adapters    map[string]ProtocolAdapter
	cleanup     func(context.Context) error
	now         func() time.Time
}

func NewService(repo Repository, definitions connections.DefinitionCatalog, reader ConnectionReader, secrets *credentials.Store, hasher *connections.CommandHasher, authority Authority, registrar Registrar) (*Service, error) {
	if repo == nil || definitions == nil || reader == nil || secrets == nil || hasher == nil || authority == nil || registrar == nil {
		return nil, registrationError(core.ErrorCodeInternal)
	}
	return &Service{repository: repo, catalog: definitions, connections: reader, secrets: secrets, hasher: hasher, authority: authority, registrar: registrar, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Service) Start(ctx context.Context, command StartCommand) (Snapshot, error) {
	if command.ProtocolVersion != 0 || command.Intent == "register_bot" || command.Intent == "login_session" || command.Intent == "reauthorize" {
		return s.startMessaging(ctx, command)
	}
	if command.Intent != "" && command.Intent != "create_app" && command.Intent != "update_app" {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil {
		return Snapshot{}, registrationError(core.ErrorCodeAccessDenied)
	}
	keyHash, err := s.hasher.IdempotencyKeyHash(command.OrganizationID, command.OwnerAccountID, command.IdempotencyKey)
	if err != nil {
		return Snapshot{}, err
	}
	prior, found, err := s.repository.FindRequest(ctx, command.OrganizationID, command.OwnerAccountID, keyHash)
	if err != nil {
		return Snapshot{}, err
	}
	if found {
		matches := prior.TemplateID == command.TemplateID
		if prior.Intent == "create_app" {
			matches = matches && command.ConnectionID == uuid.Nil && prior.ConnectorID == command.ConnectorID && prior.RequestedRegion == command.Region && command.ExpectedRevision == 0
		} else {
			matches = matches && prior.ConnectionID != nil && *prior.ConnectionID == command.ConnectionID && prior.ExpectedConnectionRevision != nil && *prior.ExpectedConnectionRevision == command.ExpectedRevision
		}
		if !matches {
			return Snapshot{}, registrationError(core.ErrorCodeConflict)
		}
		return s.snapshot(ctx, prior)
	}
	intent := "create_app"
	var existing core.PersonalConnection
	if command.ConnectionID != uuid.Nil {
		var err error
		existing, err = s.connections.GetConnection(ctx, command.OrganizationID, command.OwnerAccountID, command.ConnectionID)
		if err != nil {
			return Snapshot{}, err
		}
		if command.ExpectedRevision < 1 || command.ExpectedRevision != existing.Revision || existing.ExternalAccountID == nil {
			return Snapshot{}, registrationError(core.ErrorCodeConflict)
		}
		parts := strings.SplitN(*existing.ExternalAccountID, ":", 2)
		if len(parts) != 2 || registrationOrigin(parts[0]) == "" || !appIdentifier.MatchString(parts[1]) {
			return Snapshot{}, registrationError(core.ErrorCodeConflict)
		}
		intent = "update_app"
		command.ConnectorID = existing.ConnectorID.String()
		command.Region = parts[0]
	}
	d, ok := s.catalog.Get(command.ConnectorID)
	if !ok || !d.IsAppCredentials() || d.Setup == nil || d.Setup.Handler != "feishu-app-registration-v1" || command.TemplateID != d.Setup.TemplateID || registrationOrigin(command.Region) == "" {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	digest, err := catalog.DefinitionContractDigest(d)
	if err != nil {
		return Snapshot{}, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	fingerprint, err := s.hasher.RequestFingerprint(connections.CommandFingerprintInput{ConnectorID: d.ID, ReconnectConnectionID: command.ConnectionID, Values: map[string]string{"intent": intent, "region": command.Region, "template_id": command.TemplateID, "template_digest": d.Setup.TemplateDigest, "definition_digest": digest, "expected_revision": strconv.Itoa(command.ExpectedRevision)}})
	if err != nil {
		return Snapshot{}, err
	}
	now := s.now()
	f := Flow{ID: uuid.New(), OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID, ConnectorID: d.ID, Intent: intent, Region: command.Region, TemplateID: command.TemplateID, TemplateDigest: d.Setup.TemplateDigest, AcceptedDefinitionDigest: digest, IdempotencyKeyHash: keyHash, RequestFingerprint: fingerprint, State: Starting, Generation: 1, Revision: 1, PollIntervalSeconds: 5, ExpiresAt: now.Add(10 * time.Minute), NextPollAt: now, CreatedAt: now, UpdatedAt: now}
	f.RequestedRegion = command.Region
	if intent == "update_app" {
		appID := strings.SplitN(*existing.ExternalAccountID, ":", 2)[1]
		f.ConnectionID = &existing.ID
		f.ExpectedConnectionRevision = &existing.Revision
		f.ExpectedCredentialVersion = &existing.CredentialVersion
		f.ExpectedAppID = &appID
	}
	if err = s.authority.Check(ctx, f); err != nil {
		return Snapshot{}, err
	}
	f, err = s.repository.Admit(ctx, f)
	if err != nil {
		return Snapshot{}, err
	}
	return s.snapshot(ctx, f)
}
func (s *Service) Get(ctx context.Context, org, owner, id uuid.UUID) (Snapshot, error) {
	f, err := s.repository.Get(ctx, org, owner, id)
	if err != nil {
		return Snapshot{}, err
	}
	return s.snapshot(ctx, f)
}
func (s *Service) Cancel(ctx context.Context, org, owner, id uuid.UUID, revision int64) (Snapshot, error) {
	f, err := s.repository.Cancel(ctx, org, owner, id, revision)
	if err != nil {
		return Snapshot{}, err
	}
	return s.snapshot(ctx, f)
}
func (s *Service) snapshot(ctx context.Context, f Flow) (Snapshot, error) {
	result := Snapshot{ID: f.ID, ConnectorID: f.ConnectorID, Intent: f.Intent, State: f.State, Region: f.Region, Revision: f.Revision, ExpiresAt: f.ExpiresAt, RetryAfter: max(2, f.PollIntervalSeconds), ErrorCode: f.SafeErrorCode, ResultConnectionID: f.ResultConnectionID, PendingSetupID: f.PendingSetupID}
	if f.ProtocolVersion != 0 {
		return s.messagingSnapshot(ctx, f, result)
	}
	result.QRExpiresAt = &f.ExpiresAt
	if f.State == Awaiting && f.ExpiresAt.After(s.now()) {
		if err := s.authority.Check(ctx, f); err != nil {
			return Snapshot{}, err
		}
		device, err := s.openDevice(ctx, f)
		if err != nil {
			return Snapshot{}, err
		}
		result.VerificationURL = device.VerificationURL
		result.UserCode = device.UserCode
	}
	return result, nil
}

type deviceSecret struct {
	DeviceCode      string `json:"device_code"`
	VerificationURL string `json:"verification_url"`
	UserCode        string `json:"user_code"`
}

func deviceSubject(f Flow) credentials.Subject {
	return credentials.Subject{Purpose: credentials.OnboardingDevice, OrganizationID: f.OrganizationID, ID: f.ID, ConnectorID: f.ConnectorID, Version: f.Generation}
}
func (s *Service) openDevice(ctx context.Context, f Flow) (deviceSecret, error) {
	record, err := s.repository.Device(ctx, f)
	if err != nil {
		return deviceSecret{}, err
	}
	if record.FlowID != f.ID || record.OrganizationID != f.OrganizationID || record.OwnerAccountID != f.OwnerAccountID || record.Generation != f.Generation {
		return deviceSecret{}, registrationError(core.ErrorCodeAccessDenied)
	}
	plain, err := s.secrets.Open(deviceSubject(f), record.EncryptedDevice)
	if err != nil {
		return deviceSecret{}, registrationError(core.ErrorCodeInternal)
	}
	defer clear(plain)
	var device deviceSecret
	if json.Unmarshal(plain, &device) != nil || device.DeviceCode == "" {
		return deviceSecret{}, registrationError(core.ErrorCodeInternal)
	}
	return device, nil
}

// ProcessOne advances at most one leased flow. GET never calls this method.
// A bounded worker lifecycle calls it independently of browser polling.
func (s *Service) ProcessOne(ctx context.Context) (bool, error) {
	f, found, err := s.repository.Claim(ctx, uuid.New())
	if err != nil || !found {
		return found, err
	}
	if f.ProtocolVersion != 0 {
		return true, s.processMessaging(ctx, f)
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	fail := func(cause error) (bool, error) {
		code := core.ErrorCodeOf(cause)
		state := Failed
		if code == core.ErrorCodeFlowExpired {
			state = Expired
		}
		if code == core.ErrorCodeOutcomeUnknown || code == core.ErrorCodeInternal || bounded.Err() != nil {
			state = RecoveryRequired
			code = core.ErrorCodeOutcomeUnknown
		}
		// Cleanup remains bounded but is not cancelled with a timed-out HTTP call.
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		_ = s.repository.Terminate(cleanup, f, state, code)
		return true, cause
	}
	if err = s.authority.Check(bounded, f); err != nil {
		return fail(err)
	}
	if f.State == Starting {
		appID := ""
		if f.ExpectedAppID != nil {
			appID = *f.ExpectedAppID
		}
		startedAt := s.now()
		if f.Intent == "resume_app" && !resumeQREnabled(f.Region) {
			return fail(registrationError(core.ErrorCodeConnectorUnavailable))
		}
		session, err := s.registrar.Begin(bounded, RegistrationRequest{Purpose: RegistrationPurpose(f.Intent), Region: f.Region, AppID: appID})
		if err != nil {
			return fail(err)
		}
		plain, err := json.Marshal(deviceSecret{DeviceCode: session.DeviceCode, VerificationURL: session.VerificationURL, UserCode: session.UserCode})
		if err != nil {
			return fail(registrationError(core.ErrorCodeInternal))
		}
		defer clear(plain)
		encrypted, err := s.secrets.Protect(deviceSubject(f), plain)
		if err != nil {
			return fail(registrationError(core.ErrorCodeInternal))
		}
		remaining := startedAt.Add(session.TTL).Sub(s.now())
		if remaining <= 0 {
			return fail(registrationError(core.ErrorCodeFlowExpired))
		}
		err = s.repository.AwaitAuthorization(bounded, f, DeviceRecord{FlowID: f.ID, OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, Generation: f.Generation, EncryptedDevice: encrypted}, session.Interval, remaining)
		if err != nil {
			return fail(err)
		}
		return true, nil
	}
	device, err := s.openDevice(bounded, f)
	if err != nil {
		return fail(err)
	}
	result, err := s.registrar.Poll(bounded, f.Region, device.DeviceCode, f.SwitchedRegion)
	device.DeviceCode = ""
	if err != nil {
		return fail(err)
	}
	switch result.Status {
	case "pending", "slow_down", "switch_region":
		region, switched, interval := f.Region, f.SwitchedRegion, f.PollIntervalSeconds
		if result.Status == "slow_down" {
			interval = min(300, interval+5)
		}
		if result.Status == "switch_region" {
			if f.Intent == "update_app" || f.Intent == "resume_app" || f.SwitchedRegion {
				return fail(registrationError(core.ErrorCodeOutcomeUnknown))
			}
			region, switched = "lark", true
		}
		if err = s.repository.Advance(bounded, f, region, switched, interval); err != nil {
			return fail(err)
		}
		return true, nil
	case "complete":
		values := map[string]string{"region": f.Region, "app_id": result.AppID, "app_secret": result.AppSecret}
		result.AppSecret = ""
		defer core.DestroySecretValues(values)
		if f.ExpectedAppID != nil && *f.ExpectedAppID != result.AppID {
			return fail(registrationError(core.ErrorCodeConflict))
		}
		if err = s.authority.Check(bounded, f); err != nil {
			return fail(err)
		}
		if f.Intent == "create_app" || f.Intent == "resume_app" {
			repo, ok := s.repository.(PendingRepository)
			if !ok {
				return fail(registrationError(core.ErrorCodeInternal))
			}
			observed, observeErr := repo.ObserveApplication(bounded, f, result.AppID)
			if observeErr != nil {
				return fail(observeErr)
			}
			f = observed
		}
		if _, err = s.repository.Complete(bounded, f, values); err != nil {
			if f.PendingSetupID != nil {
				return true, s.deferCandidate(ctx, f, err)
			}
			return fail(err)
		}
		return true, nil
	default:
		return fail(registrationError(core.ErrorCodeOutcomeUnknown))
	}
}

// Run has no unbounded queue and emits no provider payloads or QR URLs. The
// shared repository lease caps concurrent requests across all API instances.
func (s *Service) Run(ctx context.Context) {
	// A WeChat status request may long-poll for 35 seconds. Keep it from
	// starving other owners; database leases still enforce the global cap of 32.
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() { defer workers.Done(); s.runWorker(ctx) }()
	}
	defer workers.Wait()
	cleanupTicker := time.NewTicker(time.Minute)
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			if s.cleanup != nil {
				bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
				_ = s.cleanup(bounded)
				cancel()
			}
		}
	}
}
func (s *Service) runWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.ProcessOne(ctx)
		}
	}
}

func (s *Service) WithMessagingCleanup(cleanup func(context.Context) error) { s.cleanup = cleanup }

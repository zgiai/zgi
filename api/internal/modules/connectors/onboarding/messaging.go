package onboarding

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// ProtocolAdapter implementations and their config are supplied only by the
// composition root. A package manifest cannot install executable handlers.
type ProtocolAdapter interface {
	ConnectorID() string
	Advance(context.Context, Flow, ProtocolSecret) (ProtocolResult, error)
}
type ProtocolSecret struct {
	Candidate  map[string]string `json:"candidate,omitempty"`
	Nonce      string            `json:"nonce,omitempty"`
	DeviceCode string            `json:"device_code,omitempty"`
	QRPayload  string            `json:"qr_payload,omitempty"`
	UserCode   string            `json:"user_code,omitempty"`
	Origin     string            `json:"origin,omitempty"`
	VerifyCode string            `json:"verify_code,omitempty"`
}
type ProtocolResult struct {
	Step         string
	Secret       ProtocolSecret
	Interval     time.Duration
	TTL          time.Duration
	ExpirySource string
	Values       map[string]string `json:"-"`
}
type ProtocolRepository interface {
	SaveProtocolStep(context.Context, Flow, DeviceRecord, ProtocolResult) error
	SubmitChallenge(context.Context, Flow, DeviceRecord, int64, int64) (Flow, error)
}

func (s *Service) WithMessagingAdapters(adapters ...ProtocolAdapter) error {
	if _, ok := s.repository.(ProtocolRepository); !ok && len(adapters) > 0 {
		return registrationError(core.ErrorCodeInternal)
	}
	registered := map[string]ProtocolAdapter{}
	for _, adapter := range adapters {
		if adapter == nil {
			return registrationError(core.ErrorCodeInvalidInput)
		}
		id := adapter.ConnectorID()
		if _, ok := catalog.MessagingDefinition(id); !ok || registered[id] != nil {
			return registrationError(core.ErrorCodeInvalidInput)
		}
		registered[id] = adapter
	}
	s.adapters = registered
	return nil
}

func (s *Service) startMessaging(ctx context.Context, c StartCommand) (Snapshot, error) {
	if c.OrganizationID == uuid.Nil || c.OwnerAccountID == uuid.Nil {
		return Snapshot{}, registrationError(core.ErrorCodeAccessDenied)
	}
	if c.ProtocolVersion != 1 || c.Region != "" || c.TemplateID != "" {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	key, err := s.hasher.IdempotencyKeyHash(c.OrganizationID, c.OwnerAccountID, c.IdempotencyKey)
	if err != nil {
		return Snapshot{}, err
	}
	prior, found, err := s.repository.FindRequest(ctx, c.OrganizationID, c.OwnerAccountID, key)
	if err != nil {
		return Snapshot{}, err
	}
	if found {
		matches := prior.ProtocolVersion == c.ProtocolVersion && prior.Intent == c.Intent
		if c.ConnectionID == uuid.Nil {
			matches = matches && prior.ConnectionID == nil && prior.ConnectorID == c.ConnectorID && c.ExpectedRevision == 0
		} else {
			matches = matches && prior.ConnectionID != nil && *prior.ConnectionID == c.ConnectionID && prior.ExpectedConnectionRevision != nil && *prior.ExpectedConnectionRevision == c.ExpectedRevision
		}
		if !matches {
			return Snapshot{}, registrationError(core.ErrorCodeConflict)
		}
		return s.snapshot(ctx, prior)
	}
	var existing core.PersonalConnection
	if c.ConnectionID != uuid.Nil {
		if c.Intent != "reauthorize" {
			return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
		}
		existing, err = s.connections.GetConnection(ctx, c.OrganizationID, c.OwnerAccountID, c.ConnectionID)
		if err != nil {
			return Snapshot{}, err
		}
		if c.ExpectedRevision < 1 || existing.Revision != c.ExpectedRevision || existing.ExternalAccountID == nil {
			return Snapshot{}, registrationError(core.ErrorCodeConflict)
		}
		c.ConnectorID = existing.ConnectorID.String()
	} else if c.ExpectedRevision != 0 || c.Intent == "reauthorize" {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	d, ok := s.catalog.Get(c.ConnectorID)
	if !ok || !d.IsMessagingCredentials() || catalog.Validate(d) != nil || d.Setup.ProtocolVersion != c.ProtocolVersion {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	if c.ConnectionID == uuid.Nil && (c.ConnectorID == "weixin" && c.Intent != "login_session" || c.ConnectorID != "weixin" && c.Intent != "register_bot") {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	if s.adapters[c.ConnectorID] == nil {
		return Snapshot{}, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	digest, err := catalog.DefinitionContractDigest(d)
	if err != nil {
		return Snapshot{}, err
	}
	fingerprint, err := s.hasher.RequestFingerprint(connections.CommandFingerprintInput{ConnectorID: d.ID, ReconnectConnectionID: c.ConnectionID, Values: map[string]string{"intent": c.Intent, "protocol": "1", "definition": digest, "revision": strconv.Itoa(c.ExpectedRevision)}})
	if err != nil {
		return Snapshot{}, err
	}
	now := s.now()
	f := Flow{ID: uuid.New(), OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectorID: d.ID, Intent: c.Intent, ProtocolVersion: 1, Handler: d.Setup.Handler, Step: "begin", ExpirySource: "local_limit", AcceptedDefinitionDigest: digest, IdempotencyKeyHash: key, RequestFingerprint: fingerprint, State: Starting, Generation: 1, Revision: 1, PollIntervalSeconds: 3, ExpiresAt: now.Add(5 * time.Minute), NextPollAt: now, CreatedAt: now, UpdatedAt: now}
	if d.ID == "dingtalk" {
		f.Step = "init"
	}
	if c.ConnectionID != uuid.Nil {
		f.ConnectionID = &existing.ID
		f.ExpectedIdentity = existing.ExternalAccountID
		f.ExpectedConnectionRevision = &existing.Revision
		f.ExpectedCredentialVersion = &existing.CredentialVersion
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

func (s *Service) openProtocol(ctx context.Context, f Flow) (ProtocolSecret, error) {
	r, err := s.repository.Device(ctx, f)
	if err != nil {
		return ProtocolSecret{}, err
	}
	if r.FlowID != f.ID || r.OrganizationID != f.OrganizationID || r.OwnerAccountID != f.OwnerAccountID || r.Generation != f.Generation {
		return ProtocolSecret{}, registrationError(core.ErrorCodeAccessDenied)
	}
	plain, err := s.secrets.Open(deviceSubject(f), r.EncryptedDevice)
	if err != nil {
		return ProtocolSecret{}, registrationError(core.ErrorCodeInternal)
	}
	defer clear(plain)
	var secret ProtocolSecret
	if json.Unmarshal(plain, &secret) != nil {
		return ProtocolSecret{}, registrationError(core.ErrorCodeInternal)
	}
	return secret, nil
}
func (s *Service) sealProtocol(f Flow, secret ProtocolSecret) (DeviceRecord, error) {
	plain, err := json.Marshal(secret)
	if err != nil {
		return DeviceRecord{}, registrationError(core.ErrorCodeInternal)
	}
	defer clear(plain)
	encrypted, err := s.secrets.Protect(deviceSubject(f), plain)
	if err != nil {
		return DeviceRecord{}, registrationError(core.ErrorCodeInternal)
	}
	return DeviceRecord{FlowID: f.ID, OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, Generation: f.Generation, EncryptedDevice: encrypted, ExpiresAt: f.ExpiresAt}, nil
}
func (s *Service) messagingSnapshot(ctx context.Context, f Flow, out Snapshot) (Snapshot, error) {
	out.ProtocolVersion = f.ProtocolVersion
	out.Step = f.Step
	out.ExpirySource = f.ExpirySource
	if f.ProtocolVersion != 1 {
		return Snapshot{}, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	if f.State.Active() && f.ExpiresAt.After(s.now()) {
		if err := s.authority.Check(ctx, f); err != nil {
			return Snapshot{}, err
		}
		out.AllowedActions = []string{"cancel"}
		if f.State == Awaiting {
			secret, err := s.openProtocol(ctx, f)
			if err != nil {
				return Snapshot{}, err
			}
			out.QRPayload = secret.QRPayload
			out.UserCode = secret.UserCode
			if f.Step == "awaiting_code" {
				out.AllowedActions = append(out.AllowedActions, "submit_code")
				out.ChallengeRevision = f.ChallengeRevision
			}
		}
	}
	return out, nil
}

func (s *Service) processMessaging(ctx context.Context, f Flow) error {
	bounded, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	fail := func(err error) error {
		code := core.ErrorCodeOf(err)
		state := Failed
		if code == core.ErrorCodeFlowExpired {
			state = Expired
		}
		if code == core.ErrorCodeInternal || code == core.ErrorCodeOutcomeUnknown || bounded.Err() != nil {
			state = RecoveryRequired
			code = core.ErrorCodeOutcomeUnknown
		}
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		_ = s.repository.Terminate(cleanup, f, state, code)
		return err
	}
	d, ok := s.catalog.Get(f.ConnectorID)
	adapter := s.adapters[f.ConnectorID]
	if !ok || f.ProtocolVersion != 1 || d.Setup == nil || f.Handler != d.Setup.Handler || adapter == nil {
		return fail(registrationError(core.ErrorCodeConnectorUnavailable))
	}
	if err := s.authority.Check(bounded, f); err != nil {
		return fail(err)
	}
	secret := ProtocolSecret{}
	if f.State == Awaiting {
		var err error
		secret, err = s.openProtocol(bounded, f)
		if err != nil {
			return fail(err)
		}
	}
	var result ProtocolResult
	var err error
	if f.Step == "verify" {
		result.Values = secret.Candidate
	} else {
		result, err = adapter.Advance(bounded, f, secret)
	}
	if err != nil {
		return fail(err)
	}
	if len(result.Values) > 0 {
		defer core.DestroySecretValues(result.Values)
		if catalog.ValidateCredentialValues(d, result.Values) != nil {
			return fail(registrationError(core.ErrorCodeInvalidAppCredentials))
		}
		if f.ExpectedIdentity != nil && *f.ExpectedIdentity != catalog.CredentialIdentity(d, result.Values) {
			return fail(registrationError(core.ErrorCodeConflict))
		}
		if err = s.authority.Check(bounded, f); err != nil {
			return fail(err)
		}
		if f.Step != "verify" {
			repo, ok := s.repository.(ProtocolRepository)
			if !ok {
				return fail(registrationError(core.ErrorCodeInternal))
			}
			next := ProtocolResult{Step: "verify", Secret: ProtocolSecret{Candidate: result.Values}, Interval: time.Second}
			record, err := s.sealProtocol(f, next.Secret)
			if err != nil {
				return fail(err)
			}
			if err = repo.SaveProtocolStep(bounded, f, record, next); err != nil {
				return fail(err)
			}
			return nil
		}
		proof := core.WithRegistrationProof(bounded, core.RegistrationProof{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, FlowID: f.ID, ConnectorID: f.ConnectorID, Identity: catalog.CredentialIdentity(d, result.Values), Values: result.Values})
		_, err = s.repository.Complete(proof, f, result.Values)
		if err != nil {
			return fail(err)
		}
		return nil
	}
	if f.Step == "verify" {
		return fail(registrationError(core.ErrorCodeInvalidAppCredentials))
	}
	repo, ok := s.repository.(ProtocolRepository)
	if !ok {
		return fail(registrationError(core.ErrorCodeInternal))
	}
	record, err := s.sealProtocol(f, result.Secret)
	if err != nil {
		return fail(err)
	}
	if err = repo.SaveProtocolStep(bounded, f, record, result); err != nil {
		return fail(err)
	}
	return nil
}

type ActionCommand struct {
	OrganizationID, OwnerAccountID, FlowID uuid.UUID
	Action                                 string
	ExpectedRevision, ChallengeRevision    int64
	Values                                 map[string]string `json:"-"`
}

var challengeCode = regexp.MustCompile(`^[0-9]{1,12}$`)

func (s *Service) Action(ctx context.Context, c ActionCommand) (Snapshot, error) {
	defer core.DestroySecretValues(c.Values)
	f, err := s.repository.Get(ctx, c.OrganizationID, c.OwnerAccountID, c.FlowID)
	if err != nil {
		return Snapshot{}, err
	}
	if c.Action != "submit_code" || f.ConnectorID != "weixin" || f.ProtocolVersion != 1 || f.Step != "awaiting_code" || !f.State.Active() || !f.ExpiresAt.After(s.now()) || c.ExpectedRevision != f.Revision || c.ChallengeRevision != f.ChallengeRevision || f.ChallengeAttempts >= 3 || len(c.Values) != 1 || !challengeCode.MatchString(c.Values["code"]) {
		return Snapshot{}, registrationError(core.ErrorCodeConflict)
	}
	if err = s.authority.Check(ctx, f); err != nil {
		return Snapshot{}, err
	}
	secret, err := s.openProtocol(ctx, f)
	if err != nil {
		return Snapshot{}, err
	}
	secret.VerifyCode = c.Values["code"]
	record, err := s.sealProtocol(f, secret)
	if err != nil {
		return Snapshot{}, err
	}
	repo, ok := s.repository.(ProtocolRepository)
	if !ok {
		return Snapshot{}, registrationError(core.ErrorCodeInternal)
	}
	f, err = repo.SubmitChallenge(ctx, f, record, c.ExpectedRevision, c.ChallengeRevision)
	if err != nil {
		return Snapshot{}, err
	}
	return s.snapshot(ctx, f)
}

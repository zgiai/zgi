package onboarding

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"gorm.io/gorm"
)

type MessagingProbe func(context.Context, Flow, map[string]string) (connections.CandidateVerificationResult, error)
type DiagnosticObserver func(context.Context, string, core.ErrorCode, core.ProviderDiagnostic)

func (s *Service) WithDiagnosticObserver(observer DiagnosticObserver) {
	s.diagnosticObserver = observer
}
func (s *Service) observeDiagnostic(ctx context.Context, connector string, err error) *core.ProviderDiagnostic {
	if connector != "weixin" && connector != "wecom" {
		return nil
	}
	diagnostic := core.DiagnosticOf(err)
	if diagnostic != nil && s.diagnosticObserver != nil {
		s.diagnosticObserver(ctx, connector, core.ErrorCodeOf(err), *diagnostic)
	}
	return diagnostic
}

type MessagingCommit func(context.Context, *gorm.DB, Flow, map[string]string, connections.CandidateVerificationResult) (uuid.UUID, error)
type messagingReliabilityRepository interface {
	CheckpointMessaging(context.Context, Flow, DeviceRecord) error
	WaitMessaging(context.Context, Flow, core.ErrorCode, string, string) error
	RetryMessaging(context.Context, Flow, int64, string) (Flow, error)
	CompleteMessaging(context.Context, Flow, map[string]string, connections.CandidateVerificationResult) (uuid.UUID, error)
}

func (s *Service) WithMessagingProbe(probe MessagingProbe) { s.messagingProbe = probe }
func (r *GormRepository) WithMessagingCommit(commit MessagingCommit) *GormRepository {
	r.messagingCommit = commit
	return r
}

// This receipt exists only inside the authenticated encrypted device payload.
// Public snapshots cannot carry provider evidence, candidate keys or messages.
type messagingCheckpoint struct {
	Version                                              int
	OrganizationID, OwnerAccountID, FlowID, ConnectionID uuid.UUID
	Generation                                           int
	ProducerFence                                        int64
	CredentialDigest, DefinitionDigest                   string
	ExpiresAt                                            time.Time
	Verification                                         connections.CandidateVerificationResult
}

func (s *Service) processReliableMessaging(ctx context.Context, f Flow) error {
	repo, ok := s.repository.(messagingReliabilityRepository)
	if !ok || s.messagingProbe == nil {
		return registrationError(core.ErrorCodeConnectorUnavailable)
	}
	deadline := s.now().Add(50 * time.Second)
	if f.ConnectorID == "weixin" {
		deadline = s.now().Add(55 * time.Second)
	}
	if f.VerificationDeadline != nil && f.VerificationDeadline.Before(deadline) {
		deadline = *f.VerificationDeadline
	}
	if f.verificationExpiry().Before(deadline) {
		deadline = f.verificationExpiry()
	}
	bounded, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	finish := func(err error, checkpoint bool) error {
		code := core.ErrorCodeOf(err)
		cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		phase, trace := f.VerificationPhase, ""
		if diagnostic := s.observeDiagnostic(cleanup, f.ConnectorID, err); diagnostic != nil {
			phase = diagnostic.Phase
			trace = diagnostic.TraceID
		}
		if phase == "" {
			phase = "operation"
		}
		if bounded.Err() == nil && (code == core.ErrorCodeProviderUnavailable || code == core.ErrorCodeVerificationWaiting || checkpoint && code == core.ErrorCodeInternal) {
			_ = repo.WaitMessaging(cleanup, f, code, phase, trace)
		} else {
			state := Failed
			if code == core.ErrorCodeOutcomeUnknown || code == core.ErrorCodeVerificationUncertain || bounded.Err() != nil {
				state = RecoveryRequired
				code = core.ErrorCodeVerificationUncertain
			}
			_ = s.repository.Terminate(cleanup, f, state, code)
		}
		return err
	}
	if err := s.authority.Check(bounded, f); err != nil {
		return finish(err, false)
	}
	secret, err := s.openProtocol(bounded, f)
	if err != nil {
		return finish(err, false)
	}
	defer core.DestroySecretValues(secret.Candidate)
	d, ok := s.catalog.Get(f.ConnectorID)
	if !ok || f.CandidateConnectionID == nil || f.CandidateExpiresAt == nil || catalog.ValidateCredentialValues(d, secret.Candidate) != nil {
		return finish(registrationError(core.ErrorCodeInvalidAppCredentials), false)
	}
	identity := catalog.CredentialIdentity(d, secret.Candidate)
	if f.ExpectedIdentity != nil && *f.ExpectedIdentity != identity {
		return finish(registrationError(core.ErrorCodeConflict), false)
	}
	if f.Step == "commit" {
		cp := secret.Checkpoint
		if cp == nil || cp.Version != 1 || cp.OrganizationID != f.OrganizationID || cp.OwnerAccountID != f.OwnerAccountID || cp.FlowID != f.ID || cp.ConnectionID != *f.CandidateConnectionID || cp.Generation != f.Generation || cp.ProducerFence <= 0 || cp.ProducerFence >= f.Fencing || cp.CredentialDigest != messaging.CredentialDigest(secret.Candidate) || cp.DefinitionDigest != f.AcceptedDefinitionDigest || !cp.ExpiresAt.Equal(*f.CandidateExpiresAt) || cp.Verification.ExternalAccountID != identity {
			return finish(registrationError(core.ErrorCodeConflict), false)
		}
		_, err = repo.CompleteMessaging(bounded, f, secret.Candidate, cp.Verification)
		if err != nil {
			return finish(err, true)
		}
		return nil
	}
	proof := core.WithRegistrationProof(bounded, core.RegistrationProof{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, FlowID: f.ID, ConnectorID: f.ConnectorID, Identity: identity, Values: secret.Candidate})
	verified, err := s.messagingProbe(proof, f, secret.Candidate)
	if err != nil {
		return finish(err, false)
	}
	if verified.ExternalAccountID != identity {
		return finish(registrationError(core.ErrorCodeConflict), false)
	}
	secret.Checkpoint = &messagingCheckpoint{Version: 1, OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, FlowID: f.ID, ConnectionID: *f.CandidateConnectionID, Generation: f.Generation, ProducerFence: f.Fencing, CredentialDigest: messaging.CredentialDigest(secret.Candidate), DefinitionDigest: f.AcceptedDefinitionDigest, ExpiresAt: *f.CandidateExpiresAt, Verification: verified}
	record, err := s.sealProtocol(f, secret)
	if err != nil {
		return finish(err, true)
	}
	if err = repo.CheckpointMessaging(bounded, f, record); err != nil {
		return finish(err, true)
	}
	return nil
}

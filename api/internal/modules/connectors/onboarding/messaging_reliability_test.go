package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"strings"
	"testing"
	"time"
)

type reliabilityMemory struct {
	protocolMemory
	checkpoints int
	waits       int
}

func (m *reliabilityMemory) CheckpointMessaging(_ context.Context, f Flow, d DeviceRecord) error {
	m.device = d
	m.flow.Step = "commit"
	m.flow.State = Verifying
	m.flow.StepDispatched = false
	m.checkpoints++
	return nil
}
func (m *reliabilityMemory) WaitMessaging(_ context.Context, _ Flow, code core.ErrorCode, phase, trace string) error {
	m.flow.State = VerificationWaiting
	m.flow.SafeErrorCode = &code
	m.flow.VerificationPhase = phase
	m.flow.DiagnosticTraceID = trace
	m.waits++
	return nil
}
func (m *reliabilityMemory) RetryMessaging(_ context.Context, _ Flow, _ int64, _ string) (Flow, error) {
	m.flow.State = Verifying
	return m.flow, nil
}
func (m *reliabilityMemory) CompleteMessaging(_ context.Context, f Flow, _ map[string]string, _ connections.CandidateVerificationResult) (uuid.UUID, error) {
	m.flow.State = Ready
	m.commits++
	m.device = DeviceRecord{}
	return *f.CandidateConnectionID, nil
}

func messagingReliabilityFixture(t *testing.T) (*Service, *reliabilityMemory, map[string]string) {
	t.Helper()
	s, _, _, _, c := serviceFixture(t)
	d, _ := catalog.MessagingDefinition("weixin")
	s.catalog, _ = catalog.New(d)
	now := time.Now().UTC()
	expiry := now.Add(15 * time.Minute)
	id := uuid.New()
	worker := uuid.New()
	until := now.Add(time.Minute)
	repo := &reliabilityMemory{}
	repo.flow = Flow{ID: uuid.New(), OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectorID: "weixin", Intent: "login_session", ProtocolVersion: 1, Step: "verify", State: Verifying, Generation: 1, Fencing: 1, LeaseOwner: &worker, LeaseUntil: &until, CandidateConnectionID: &id, CandidateAcceptedAt: &now, CandidateExpiresAt: &expiry, ExpiresAt: now.Add(-time.Second)}
	s.repository = repo
	values := map[string]string{"account_id": "account-fixture", "token": "PRIVATE_TOKEN", "base_url": "https://ilinkai.weixin.qq.com"}
	record, err := s.sealProtocol(repo.flow, ProtocolSecret{Candidate: values})
	if err != nil {
		t.Fatal(err)
	}
	repo.device = record
	return s, repo, values
}
func TestMessagingCheckpointResumesWithoutSecondProviderRead(t *testing.T) {
	s, repo, _ := messagingReliabilityFixture(t)
	calls := 0
	s.WithMessagingProbe(func(_ context.Context, _ Flow, values map[string]string) (connections.CandidateVerificationResult, error) {
		calls++
		d, _ := catalog.MessagingDefinition("weixin")
		return connections.CandidateVerificationResult{ExternalAccountID: catalog.CredentialIdentity(d, values)}, nil
	})
	if err := s.processReliableMessaging(context.Background(), repo.flow); err != nil {
		t.Fatal(err)
	}
	if repo.checkpoints != 1 || repo.commits != 0 || calls != 1 || strings.Contains(repo.device.EncryptedDevice, "PRIVATE_TOKEN") {
		t.Fatal("checkpoint boundary")
	}
	repo.flow.Fencing++
	if err := s.processReliableMessaging(context.Background(), repo.flow); err != nil {
		t.Fatal(err)
	}
	if repo.commits != 1 || calls != 1 || repo.flow.State != Ready {
		t.Fatal("commit repeated provider read")
	}
}
func TestMessagingWaitingRetainsCandidateButUncertainPurges(t *testing.T) {
	for _, tc := range []struct {
		code  core.ErrorCode
		state State
	}{{core.ErrorCodeProviderUnavailable, VerificationWaiting}, {core.ErrorCodeVerificationUncertain, RecoveryRequired}} {
		t.Run(string(tc.code), func(t *testing.T) {
			s, repo, _ := messagingReliabilityFixture(t)
			cipher := repo.device.EncryptedDevice
			expiry := *repo.flow.CandidateExpiresAt
			s.WithMessagingProbe(func(context.Context, Flow, map[string]string) (connections.CandidateVerificationResult, error) {
				return connections.CandidateVerificationResult{}, core.NewError(tc.code, true, nil)
			})
			if err := s.processReliableMessaging(context.Background(), repo.flow); err == nil {
				t.Fatal("failed probe ready")
			}
			if repo.flow.State != tc.state || repo.commits != 0 {
				t.Fatal("failure state", repo.flow.State)
			}
			if tc.state == VerificationWaiting && (repo.device.EncryptedDevice != cipher || !repo.flow.CandidateExpiresAt.Equal(expiry)) {
				t.Fatal("candidate not retained exactly")
			}
			if tc.state == RecoveryRequired && repo.device.EncryptedDevice != "" {
				t.Fatal("uncertain candidate retained")
			}
		})
	}
}

func TestMessagingCancelledProbeDoesNotOfferConsumingRetry(t *testing.T) {
	s, repo, _ := messagingReliabilityFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.WithMessagingProbe(func(context.Context, Flow, map[string]string) (connections.CandidateVerificationResult, error) {
		cancel()
		return connections.CandidateVerificationResult{}, core.NewError(core.ErrorCodeProviderUnavailable, true, nil)
	})
	if err := s.processReliableMessaging(ctx, repo.flow); err == nil {
		t.Fatal("cancelled probe became ready")
	}
	if repo.flow.State != RecoveryRequired || repo.commits != 0 {
		t.Fatal("cancelled consumption offered retry", repo.flow.State)
	}
}
func TestMessagingDiagnosticObserverOnlyReceivesSafeCopy(t *testing.T) {
	s := &Service{}
	count := 0
	s.WithDiagnosticObserver(func(_ context.Context, p string, code core.ErrorCode, d core.ProviderDiagnostic) {
		count++
		raw, _ := json.Marshal(d)
		if p != "weixin" || code != core.ErrorCodeProviderUnavailable || strings.Contains(string(raw), "PRIVATE") {
			t.Fatal("unsafe observation")
		}
		d.Category = "PRIVATE"
	})
	d := &core.ProviderDiagnostic{TraceID: uuid.NewString(), Phase: "operation", Category: "deadline", ElapsedMS: 35000, RequestWritten: true}
	err := core.NewError(core.ErrorCodeProviderUnavailable, false, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, RequestID: "PRIVATE_TOKEN", Diagnostic: d})
	observed := s.observeDiagnostic(context.Background(), "weixin", err)
	if count != 1 || observed.Category != "deadline" {
		t.Fatal("observer corrupted stored diagnostic")
	}
	s.observeDiagnostic(context.Background(), "weixin", errors.New("PRIVATE_URL?token=PRIVATE_TOKEN"))
	s.observeDiagnostic(context.Background(), "weixin", core.NewError(core.ErrorCodeProviderUnavailable, false, &providers.Failure{Diagnostic: &core.ProviderDiagnostic{TraceID: uuid.NewString(), Phase: "PRIVATE", Category: "deadline"}}))
	if count != 1 {
		t.Fatal("raw error reached observer")
	}
}

type rejectingReceiptAuthority struct{ calls int }

func (a *rejectingReceiptAuthority) Check(context.Context, Flow) error {
	a.calls++
	return registrationError(core.ErrorCodeConflict)
}
func TestMessagingRetryReceiptsAfterReadyAndExpiry(t *testing.T) {
	for _, state := range []State{Ready, Expired} {
		t.Run(string(state), func(t *testing.T) {
			s, repo, _ := messagingReliabilityFixture(t)
			originalRevision := int64(7)
			key := uuid.NewString()
			hash, err := s.hasher.IdempotencyKeyHash(repo.flow.OrganizationID, repo.flow.OwnerAccountID, key)
			if err != nil {
				t.Fatal(err)
			}
			expiry := time.Now().Add(-time.Second)
			repo.flow.CandidateExpiresAt = &expiry
			repo.flow.State = state
			repo.flow.Revision = 11
			repo.flow.RetryKeyHash = &hash
			repo.flow.RetryExpectedRevision = &originalRevision
			if state == Ready {
				repo.flow.ResultConnectionID = repo.flow.CandidateConnectionID
			}
			authority := &rejectingReceiptAuthority{}
			s.authority = authority
			command := ActionCommand{OrganizationID: repo.flow.OrganizationID, OwnerAccountID: repo.flow.OwnerAccountID, FlowID: repo.flow.ID, Action: "retry_verification", ExpectedRevision: originalRevision, IdempotencyKey: key}
			got, err := s.Action(context.Background(), command)
			if err != nil || got.State != state || got.Revision != 11 || !got.CandidateExpiresAt.Equal(expiry) || authority.calls != 0 {
				t.Fatal("receipt performed admission or extended candidate", err)
			}
			command.ExpectedRevision++
			if _, err = s.Action(context.Background(), command); core.ErrorCodeOf(err) != core.ErrorCodeConflict {
				t.Fatal("same key accepted a different original revision")
			}
			command.ExpectedRevision = originalRevision
			command.OwnerAccountID = uuid.New()
			if _, err = s.Action(context.Background(), command); core.ErrorCodeOf(err) != core.ErrorCodeNotFound {
				t.Fatal("receipt crossed owner scope")
			}
			command.OwnerAccountID = repo.flow.OwnerAccountID
			command.IdempotencyKey = uuid.NewString()
			if _, err = s.Action(context.Background(), command); core.ErrorCodeOf(err) != core.ErrorCodeConflict {
				t.Fatal("new key bypassed expiry")
			}
		})
	}
}

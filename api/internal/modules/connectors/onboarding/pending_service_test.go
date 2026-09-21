package onboarding

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func (m *memoryFlows) GetPending(_ context.Context, org, owner, id uuid.UUID) (PendingSetup, *uuid.UUID, error) {
	if m.pending.ID != id || m.pending.OrganizationID != org || m.pending.OwnerAccountID != owner {
		return PendingSetup{}, nil, registrationError(core.ErrorCodeNotFound)
	}
	return m.pending, nil, nil
}
func (m *memoryFlows) ListPending(_ context.Context, org, owner uuid.UUID, _ string, _, _ int) ([]PendingSetup, int64, error) {
	if m.pending.ID != uuid.Nil && m.pending.OrganizationID == org && m.pending.OwnerAccountID == owner {
		return []PendingSetup{m.pending}, 1, nil
	}
	return nil, 0, nil
}
func (m *memoryFlows) CancelPending(ctx context.Context, org, owner, id uuid.UUID, rev int64) (PendingSetup, error) {
	p, _, err := m.GetPending(ctx, org, owner, id)
	if err != nil {
		return p, err
	}
	if p.Revision != rev {
		return p, registrationError(core.ErrorCodeConflict)
	}
	m.pending.State = PendingCancelled
	return m.pending, nil
}
func (m *memoryFlows) ObserveApplication(_ context.Context, f Flow, appID string) (Flow, error) {
	if m.flow.State != Awaiting || !m.flow.ExpiresAt.After(time.Now()) || m.flow.Fencing != f.Fencing {
		return Flow{}, registrationError(core.ErrorCodeConflict)
	}
	now := time.Now()
	deadline := now.Add(30 * time.Second)
	if f.PendingSetupID == nil {
		m.pending = PendingSetup{ID: uuid.New(), OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectorID: f.ConnectorID, Region: f.Region, AppID: appID, SourceFlowID: f.ID, State: VerificationRequired, Revision: 1, CreatedAt: now, ResumeExpiresAt: now.Add(30 * 24 * time.Hour)}
		m.pending.TemplateID = f.TemplateID
		m.pending.TemplateDigest = f.TemplateDigest
		m.pending.AcceptedDefinitionDigest = f.AcceptedDefinitionDigest
	} else if *f.PendingSetupID != m.pending.ID || !pendingMatches(m.pending, f) || appID != m.pending.AppID {
		return Flow{}, registrationError(core.ErrorCodeConflict)
	}
	m.flow.PendingSetupID = &m.pending.ID
	m.flow.State = Verifying
	m.flow.VerificationDeadline = &deadline
	m.device = DeviceRecord{}
	return m.flow, nil
}
func (m *memoryFlows) DeferApplication(_ context.Context, f Flow, code core.ErrorCode) error {
	if m.flow.State != Verifying {
		return registrationError(core.ErrorCodeConflict)
	}
	m.pending.State = VerificationRequired
	if code == core.ErrorCodeApplicationNotReady {
		m.pending.State = AwaitingApplication
	}
	m.pending.SafeReasonCode = &code
	now := time.Now()
	m.pending.LastVerificationAt = &now
	m.pending.Revision++
	m.flow.State = PendingCompletion
	m.flow.SafeErrorCode = &code
	m.flow.LeaseOwner = nil
	m.flow.LeaseUntil = nil
	return nil
}

func TestApplicationNotReadyRetainsOnlyIdentityAndStopsPolling(t *testing.T) {
	s, repo, reg, _, cmd := serviceFixture(t)
	repo.commitErr = registrationError(core.ErrorCodeApplicationNotReady)
	ctx := context.Background()
	first, err := s.Start(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.ProcessOne(ctx)
	_, err = s.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repo.pending.AppID != "cli_fixture" || repo.pending.State != AwaitingApplication || repo.flow.State != PendingCompletion || repo.flow.ResultConnectionID != nil || repo.commits != 0 || repo.device.EncryptedDevice != "" {
		t.Fatal("approval became failure or callable connection")
	}
	for range 3 {
		_, _ = s.ProcessOne(ctx)
	}
	if reg.beginCalls != 1 || reg.pollCalls != 1 {
		t.Fatal("pending approval polls provider")
	}
	flow, err := s.Get(ctx, cmd.OrganizationID, cmd.OwnerAccountID, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if flow.PendingSetupID == nil || flow.VerificationURL != "" || flow.UserCode != "" {
		t.Fatal("pending QR exposed")
	}
	p, err := s.GetPending(ctx, cmd.OrganizationID, cmd.OwnerAccountID, *flow.PendingSetupID)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "APP-SECRET") || strings.Contains(string(b), "DEVICE-SECRET") || strings.Contains(string(b), "credential") {
		t.Fatal("pending exposes credentials")
	}
	if p.ResumeExpiresAt.Sub(repo.pending.CreatedAt) != 30*24*time.Hour {
		t.Fatal("pending has QR expiry")
	}
	if _, err = s.GetPending(ctx, cmd.OrganizationID, uuid.New(), p.ID); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatal("cross-owner pending")
	}
}

func TestOnlyApplicationNotReadyIsClassifiedAsApprovalWait(t *testing.T) {
	for _, code := range []core.ErrorCode{core.ErrorCodeInvalidAppCredentials, core.ErrorCodeOutcomeUnknown, core.ErrorCodeAccessDenied} {
		s, repo, _, _, cmd := serviceFixture(t)
		repo.commitErr = registrationError(code)
		_, _ = s.Start(context.Background(), cmd)
		_, _ = s.ProcessOne(context.Background())
		_, _ = s.ProcessOne(context.Background())
		if repo.pending.State != VerificationRequired || repo.pending.SafeReasonCode == nil || *repo.pending.SafeReasonCode != code {
			t.Fatalf("wrong recovery reason %s", code)
		}
	}
}

func TestVerificationExpiryIsNotQRCodeExpiry(t *testing.T) {
	now := time.Now()
	qr := now.Add(-time.Second)
	verification := now.Add(20 * time.Second)
	f := Flow{ProtocolVersion: 0, State: Verifying, ExpiresAt: qr, VerificationDeadline: &verification}
	if !f.verificationExpiry().After(now) {
		t.Fatal("received credential expired with QR")
	}
	f.State = Awaiting
	if f.verificationExpiry().After(now) {
		t.Fatal("expired QR extended")
	}
	f.State = Verifying
	f.ProtocolVersion = 1
	if f.verificationExpiry().After(now) {
		t.Fatal("messaging protocol changed")
	}
}

func TestPendingManualIdentityIsIndependentOfCreate(t *testing.T) {
	s, repo, reg, _, cmd := serviceFixture(t)
	ctx := context.Background()
	repo.commitErr = registrationError(core.ErrorCodeApplicationNotReady)
	_, _ = s.Start(ctx, cmd)
	_, _ = s.ProcessOne(ctx)
	_, _ = s.ProcessOne(ctx)
	resume := ResumeCommand{OrganizationID: cmd.OrganizationID, OwnerAccountID: cmd.OwnerAccountID, PendingSetupID: repo.pending.ID, ExpectedRevision: repo.pending.Revision, IdempotencyKey: uuid.NewString()}
	for _, values := range []map[string]string{{"region": "feishu", "app_id": "cli_other", "app_secret": "SECRET"}, {"region": "lark", "app_id": "cli_fixture", "app_secret": "SECRET"}} {
		resume.Values = values
		if _, err := s.ResumePendingManual(ctx, resume); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) || len(values) != 0 {
			t.Fatal("resume accepted another app or retained secret", err)
		}
	}
	if reg.beginCalls != 1 || reg.pollCalls != 1 || repo.commits != 0 {
		t.Fatal("resume fell back to external creation")
	}
	// A new manually verified attempt retains the same parent and identity.
	repo.flow = Flow{}
	repo.commitErr = nil
	resume.Values = map[string]string{"region": "feishu", "app_id": "cli_fixture", "app_secret": "SECRET"}
	result, err := s.ResumePendingManual(ctx, resume)
	if err != nil || result.State != Ready || result.PendingSetupID == nil || *result.PendingSetupID != repo.pending.ID || len(resume.Values) != 0 || repo.commits != 1 || reg.beginCalls != 1 {
		t.Fatal("manual recovery failed", err)
	}
	resume.Values = map[string]string{"region": "feishu", "app_id": "cli_fixture", "app_secret": "SECRET"}
	if replay, err := s.ResumePendingManual(ctx, resume); err != nil || replay.ResultConnectionID == nil || *replay.ResultConnectionID != *result.ResultConnectionID || repo.commits != 1 {
		t.Fatal("idempotent retry duplicated connection", err)
	}
}

func TestPendingReadDoesNotRefreshVerificationOrRetainExpiredActions(t *testing.T) {
	s, repo, _, _, cmd := serviceFixture(t)
	now := time.Now().UTC()
	past := now.Add(-48 * time.Hour)
	repo.pending = PendingSetup{ID: uuid.New(), OrganizationID: cmd.OrganizationID, OwnerAccountID: cmd.OwnerAccountID, State: AwaitingApplication, LastVerificationAt: &past, ResumeExpiresAt: now.Add(-time.Second), Revision: 4}
	s.now = func() time.Time { return now }
	for range 3 {
		p, err := s.GetPending(context.Background(), cmd.OrganizationID, cmd.OwnerAccountID, repo.pending.ID)
		if err != nil || p.State != PendingExpired || len(p.AllowedActions) != 0 || !p.LastVerificationAt.Equal(past) || repo.pending.State != AwaitingApplication {
			t.Fatal("GET mutated approval state or expiry", err)
		}
	}
}

func TestPendingQRResumeEnabledForBothRegionsWithoutCreatingAnotherApp(t *testing.T) {
	for _, region := range []string{"feishu", "lark"} {
		for _, outcome := range []string{"ready", "awaiting_application", "different_app", "switch_region"} {
			t.Run(region+"/"+outcome, func(t *testing.T) {
				s, repo, reg, _, cmd := serviceFixture(t)
				ctx := context.Background()
				cmd.Region = region
				repo.commitErr = registrationError(core.ErrorCodeApplicationNotReady)
				if _, err := s.Start(ctx, cmd); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if _, err := s.ProcessOne(ctx); err != nil {
						t.Fatal(err)
					}
				}
				original := repo.pending
				public, err := s.GetPending(ctx, cmd.OrganizationID, cmd.OwnerAccountID, original.ID)
				if err != nil || !strings.Contains(strings.Join(public.AllowedActions, ","), "resume_qr") || public.ResumeQRUnavailableReason != "" {
					t.Fatal("QR resume unavailable", err)
				}
				// A completed source attempt no longer occupies the active slot.
				repo.flow = Flow{}
				resume := ResumeCommand{OrganizationID: cmd.OrganizationID, OwnerAccountID: cmd.OwnerAccountID, PendingSetupID: original.ID, ExpectedRevision: original.Revision, IdempotencyKey: uuid.NewString()}
				started, err := s.ResumePending(ctx, resume)
				if err != nil || started.State != Starting || repo.flow.Intent != "resume_app" || repo.flow.ExpectedAppID == nil || *repo.flow.ExpectedAppID != original.AppID || repo.flow.Region != region || repo.flow.ConnectionID != nil {
					t.Fatal("resume lost original app", err)
				}
				if again, err := s.ResumePending(ctx, resume); err != nil || again.ID != started.ID {
					t.Fatal("resume admission not idempotent", err)
				}
				if reg.beginCalls != 1 {
					t.Fatal("admission started provider registration")
				}
				if _, err := s.ProcessOne(ctx); err != nil {
					t.Fatal(err)
				}
				if reg.beginCalls != 2 || reg.requests[1] != (RegistrationRequest{Purpose: ResumeApplication, Region: region, AppID: original.AppID}) {
					t.Fatal("worker used create instead of same-app resume")
				}
				for range 2 {
					if _, err := s.Get(ctx, cmd.OrganizationID, cmd.OwnerAccountID, started.ID); err != nil {
						t.Fatal(err)
					}
				}
				if reg.beginCalls != 2 {
					t.Fatal("GET recreated registration")
				}
				if outcome == "ready" {
					repo.commitErr = nil
				}
				if outcome == "different_app" {
					reg.poll = func() (RegistrationResult, error) {
						return RegistrationResult{Status: "complete", AppID: "cli_other", AppSecret: "SECRET"}, nil
					}
				}
				if outcome == "switch_region" {
					reg.poll = func() (RegistrationResult, error) { return RegistrationResult{Status: "switch_region"}, nil }
				}
				_, pollErr := s.ProcessOne(ctx)
				switch outcome {
				case "different_app":
					if !core.IsErrorCode(pollErr, core.ErrorCodeConflict) {
						t.Fatal("different app not rejected", pollErr)
					}
				case "switch_region":
					if !core.IsErrorCode(pollErr, core.ErrorCodeOutcomeUnknown) {
						t.Fatal("region change not rejected", pollErr)
					}
				default:
					if pollErr != nil {
						t.Fatal(pollErr)
					}
				}
				if repo.pending.ID != original.ID || repo.pending.AppID != original.AppID || repo.pending.Region != region || !repo.pending.ResumeExpiresAt.Equal(original.ResumeExpiresAt) {
					t.Fatal("resume replaced pending identity or extended retention")
				}
				if outcome == "ready" {
					if repo.flow.State != Ready || repo.commits != 1 || repo.flow.ResultConnectionID == nil {
						t.Fatal("valid same app did not complete")
					}
				} else if repo.commits != 0 || repo.flow.ResultConnectionID != nil {
					t.Fatal("unverified app became a connection")
				}
				if outcome == "awaiting_application" && (repo.flow.State != PendingCompletion || repo.pending.State != AwaitingApplication) {
					t.Fatal("approval wait lost its pending record")
				}
				for range 2 {
					if _, err := s.ProcessOne(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if reg.beginCalls != 2 {
					t.Fatal("resume silently fell back to application creation")
				}
			})
		}
	}
}

func TestPendingQRResumeStillRejectsStaleTerminalAndUnownedRecords(t *testing.T) {
	for _, scenario := range []string{"stale", "expired", "cancelled", "owner", "org"} {
		t.Run(scenario, func(t *testing.T) {
			s, repo, reg, _, cmd := serviceFixture(t)
			ctx := context.Background()
			repo.commitErr = registrationError(core.ErrorCodeApplicationNotReady)
			_, _ = s.Start(ctx, cmd)
			_, _ = s.ProcessOne(ctx)
			_, _ = s.ProcessOne(ctx)
			resume := ResumeCommand{OrganizationID: cmd.OrganizationID, OwnerAccountID: cmd.OwnerAccountID, PendingSetupID: repo.pending.ID, ExpectedRevision: repo.pending.Revision, IdempotencyKey: uuid.NewString()}
			repo.flow = Flow{}
			switch scenario {
			case "stale":
				resume.ExpectedRevision++
			case "expired":
				repo.pending.ResumeExpiresAt = time.Now().Add(-time.Second)
			case "cancelled":
				repo.pending.State = PendingCancelled
			case "owner":
				resume.OwnerAccountID = uuid.New()
			case "org":
				resume.OrganizationID = uuid.New()
			}
			if _, err := s.ResumePending(ctx, resume); err == nil {
				t.Fatal("unsafe resume admitted")
			}
			if reg.beginCalls != 1 || repo.commits != 0 {
				t.Fatal("rejected resume reached provider or created a connection")
			}
		})
	}
}

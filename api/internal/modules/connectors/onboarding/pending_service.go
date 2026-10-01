package onboarding

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func (s *Service) pendingSnapshot(p PendingSetup, active *uuid.UUID) PendingSnapshot {
	state := p.State
	if state.Active() && !p.ResumeExpiresAt.After(s.now()) {
		state = PendingExpired
	}
	out := PendingSnapshot{Kind: "pending_setup", ID: p.ID, ConnectorID: p.ConnectorID, Region: p.Region, AppID: p.AppID, State: state, Revision: p.Revision, ResumeExpiresAt: p.ResumeExpiresAt, ReasonCode: p.SafeReasonCode, LastVerificationAt: p.LastVerificationAt, AllowedActions: []string{}, ResultConnectionID: p.ResultConnectionID}
	if state.Active() {
		out.ActiveFlowID = active
		out.AllowedActions = []string{"cancel"}
		if active == nil {
			out.AllowedActions = append(out.AllowedActions, "resume_manual")
			if resumeQREnabled(p.Region) {
				out.AllowedActions = append(out.AllowedActions, "resume_qr")
			}
		}
		if !resumeQREnabled(p.Region) {
			out.ResumeQRUnavailableReason = "same_app_resume_unavailable"
		}
	}
	return out
}
func (s *Service) pendingRepo() (PendingRepository, error) {
	r, ok := s.repository.(PendingRepository)
	if !ok {
		return nil, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	return r, nil
}
func (s *Service) GetPending(ctx context.Context, org, owner, id uuid.UUID) (PendingSnapshot, error) {
	r, err := s.pendingRepo()
	if err != nil {
		return PendingSnapshot{}, err
	}
	p, active, err := r.GetPending(ctx, org, owner, id)
	if err != nil {
		return PendingSnapshot{}, err
	}
	return s.pendingSnapshot(p, active), nil
}
func (s *Service) ListPending(ctx context.Context, org, owner uuid.UUID, connector string, limit, offset int) (PendingList, error) {
	if limit < 1 || limit > 200 || offset < 0 || (connector != "" && connector != "feishu") {
		return PendingList{}, registrationError(core.ErrorCodeInvalidInput)
	}
	r, err := s.pendingRepo()
	if err != nil {
		return PendingList{}, err
	}
	items, total, err := r.ListPending(ctx, org, owner, connector, limit, offset)
	if err != nil {
		return PendingList{}, err
	}
	result := PendingList{Items: []PendingSnapshot{}, TotalPending: total}
	for _, p := range items {
		result.Items = append(result.Items, s.pendingSnapshot(p, nil))
	}
	return result, nil
}
func (s *Service) CancelPending(ctx context.Context, org, owner, id uuid.UUID, revision int64) (PendingSnapshot, error) {
	if revision < 1 {
		return PendingSnapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	r, err := s.pendingRepo()
	if err != nil {
		return PendingSnapshot{}, err
	}
	p, err := r.CancelPending(ctx, org, owner, id, revision)
	if err != nil {
		return PendingSnapshot{}, err
	}
	return s.pendingSnapshot(p, nil), nil
}

type ResumeCommand struct {
	OrganizationID   uuid.UUID
	OwnerAccountID   uuid.UUID
	PendingSetupID   uuid.UUID
	ExpectedRevision int64
	IdempotencyKey   string            `json:"-"`
	Values           map[string]string `json:"-"`
}

func (s *Service) ResumePending(ctx context.Context, cmd ResumeCommand) (Snapshot, error) {
	return s.resumePending(ctx, cmd, false)
}
func (s *Service) ResumePendingManual(ctx context.Context, cmd ResumeCommand) (Snapshot, error) {
	defer core.DestroySecretValues(cmd.Values)
	return s.resumePending(ctx, cmd, true)
}
func (s *Service) resumePending(ctx context.Context, cmd ResumeCommand, manual bool) (Snapshot, error) {
	r, err := s.pendingRepo()
	if err != nil {
		return Snapshot{}, err
	}
	if cmd.ExpectedRevision < 1 {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	p, _, err := r.GetPending(ctx, cmd.OrganizationID, cmd.OwnerAccountID, cmd.PendingSetupID)
	if err != nil {
		return Snapshot{}, err
	}
	if manual && (len(cmd.Values) != 3 || cmd.Values["app_id"] != p.AppID || cmd.Values["region"] != p.Region || cmd.Values["app_secret"] == "" || len(cmd.Values["app_secret"]) > 16384) {
		return Snapshot{}, registrationError(core.ErrorCodeInvalidInput)
	}
	f := pendingFlow(p)
	keyHash, err := s.hasher.IdempotencyKeyHash(cmd.OrganizationID, cmd.OwnerAccountID, cmd.IdempotencyKey)
	if err != nil {
		return Snapshot{}, err
	}
	values := map[string]string{"pending_id": p.ID.String(), "revision": strconv.FormatInt(cmd.ExpectedRevision, 10), "region": p.Region, "app_id": p.AppID, "template_digest": p.TemplateDigest, "definition_digest": p.AcceptedDefinitionDigest, "manual": strconv.FormatBool(manual)}
	if manual {
		values["app_secret"] = cmd.Values["app_secret"]
	}
	defer core.DestroySecretValues(values)
	fingerprint, err := s.hasher.RequestFingerprint(connections.CommandFingerprintInput{ConnectorID: p.ConnectorID, Values: values})
	if err != nil {
		return Snapshot{}, err
	}
	prior, found, err := s.repository.FindRequest(ctx, cmd.OrganizationID, cmd.OwnerAccountID, keyHash)
	if err != nil {
		return Snapshot{}, err
	}
	if found {
		if prior.RequestFingerprint != fingerprint {
			return Snapshot{}, registrationError(core.ErrorCodeConflict)
		}
		return s.snapshot(ctx, prior)
	}
	if p.State == PendingCompleted {
		return Snapshot{ID: p.SourceFlowID, ConnectorID: p.ConnectorID, Intent: "resume_app", State: Ready, Region: p.Region, Revision: p.Revision, ExpiresAt: p.UpdatedAt, RetryAfter: 2, PendingSetupID: &p.ID, ResultConnectionID: p.ResultConnectionID}, nil
	}
	if !p.State.Active() || !p.ResumeExpiresAt.After(s.now()) || p.Revision != cmd.ExpectedRevision {
		return Snapshot{}, registrationError(core.ErrorCodeConflict)
	}
	if !manual && !resumeQREnabled(p.Region) {
		return Snapshot{}, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	if err = s.authority.Check(ctx, f); err != nil {
		return Snapshot{}, err
	}
	now := s.now()
	f.ID = uuid.New()
	f.ExpectedPendingRevision = &cmd.ExpectedRevision
	f.IdempotencyKeyHash = keyHash
	f.RequestFingerprint = fingerprint
	f.State = Starting
	f.Generation = 1
	f.Revision = 1
	f.PollIntervalSeconds = 5
	f.CreatedAt = now
	f.UpdatedAt = now
	f.ExpiresAt = now.Add(10 * time.Minute)
	f.NextPollAt = now
	if manual {
		worker := uuid.New()
		deadline := now.Add(30 * time.Second)
		until := now.Add(45 * time.Second)
		f.State = Verifying
		f.ManualVerification = true
		f.VerificationDeadline = &deadline
		f.LeaseOwner = &worker
		f.LeaseUntil = &until
		f.Fencing = 1
	}
	f, err = s.repository.Admit(ctx, f)
	if err != nil {
		return Snapshot{}, err
	}
	if !manual || f.State != Verifying {
		return s.snapshot(ctx, f)
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err = s.repository.Complete(bounded, f, cmd.Values); err != nil {
		if deferErr := s.deferCandidate(ctx, f, err); deferErr != nil {
			return Snapshot{}, deferErr
		}
	}
	return s.Get(ctx, cmd.OrganizationID, cmd.OwnerAccountID, f.ID)
}
func (s *Service) deferCandidate(ctx context.Context, f Flow, cause error) error {
	r, err := s.pendingRepo()
	if err != nil {
		return err
	}
	code := core.ErrorCodeOf(cause)
	if code == "" {
		code = core.ErrorCodeOutcomeUnknown
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return r.DeferApplication(cleanup, f, code)
}

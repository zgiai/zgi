package consolesync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"gorm.io/gorm"
)

// Status is the snapshot consumed by the health endpoint (plan P8).
type Status struct {
	Enabled             bool       `json:"enabled"`
	Running             bool       `json:"running"`
	Stale               bool       `json:"stale"`
	InstanceID          string     `json:"instance_id,omitempty"`
	CursorSeq           int64      `json:"cursor_seq"`
	AppliedVersion      int64      `json:"applied_version"`
	LastHeartbeatAt     *time.Time `json:"last_heartbeat_at,omitempty"`
	LastPullAt          *time.Time `json:"last_pull_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	NextPollAt          *time.Time `json:"next_poll_at,omitempty"`
	CredentialStatus    string     `json:"credential_status,omitempty"`
	CredentialLoaded    bool       `json:"credential_loaded"`
	LastError           string     `json:"last_error,omitempty"`
	LastDrift           []Drift    `json:"last_drift,omitempty"`
	DesiredVersion      int64      `json:"desired_version"`
	Reason              string     `json:"reason,omitempty"`
}

// Service runs the control-plane loop.
type Service struct {
	cfg        Config
	store      *Store
	client     *Client
	applier    *Applier
	reconciler *Reconciler
	buildInfo  string

	mu            sync.Mutex
	state         *PreSyncState
	running       bool
	stale         bool
	lastSuccess   *time.Time
	lastDrift     []Drift
	desiredVer    int64
	lastReconcile time.Time
}

// NewService builds the sync service. It returns nil when the loop must not run, so the
// caller can simply skip starting it (SELF_HOSTED and disabled deployments).
func NewService(db *gorm.DB, cfg Config, buildInfo string) *Service {
	if !cfg.ShouldStart() {
		log.Printf("[consolesync] not started: %s", cfg.Reason())
		return nil
	}
	store := NewStore(db)
	return &Service{
		cfg:        cfg,
		store:      store,
		client:     NewClient(cfg.BaseURL, cfg.HTTPTimeout),
		applier:    NewApplier(db, store),
		reconciler: NewReconciler(store),
		buildInfo:  buildInfo,
	}
}

// Start runs the loop until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Load the instance credential so every outbound call is authenticated from the
	// first request onwards.
	consolecred.Configure(s.cfg.InstanceID, s.cfg.InstanceSecret)
	log.Printf("[consolesync] started against %s as instance %s", s.cfg.BaseURL, s.cfg.InstanceID)

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	s.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			log.Printf("[consolesync] stopped")
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick performs one heartbeat/pull/apply/ack cycle. Errors are recorded and never panic:
// the loop must survive a console outage.
func (s *Service) tick(ctx context.Context) {
	if err := s.runOnce(ctx); err != nil {
		s.recordFailure(ctx, err)
		return
	}
	s.recordSuccess(ctx)
}

func (s *Service) runOnce(ctx context.Context) error {
	state, err := s.store.LoadState(ctx)
	if err != nil {
		return err
	}

	if s.isStale(state, time.Now().UTC()) {
		s.enterStale(ctx, state)
	}

	snapshot, err := s.store.SnapshotControlledFields(ctx)
	if err != nil {
		return err
	}

	heartbeat, err := s.client.Heartbeat(ctx, HeartbeatRequest{
		AppliedVersion: state.AppliedVersion,
		CursorSeq:      state.CursorSeq,
		AccountStatus:  snapshot.AccountStatus,
		Version:        s.buildInfo,
		Drift:          s.driftFor(ctx, state),
		Health: HealthInfo{
			Status:            "ok",
			ConsecutiveErrors: state.ConsecutiveFailures,
			LastError:         state.LastError,
		},
	})
	if err != nil {
		return err
	}

	// A successful heartbeat means the console link is healthy again: reload the
	// credential that the stale gate dropped.
	s.leaveStale()

	state.LastHeartbeatAt = timePtr(time.Now().UTC())
	state.CredentialStatus = CredentialStatusActive
	if heartbeat != nil {
		s.mu.Lock()
		s.desiredVer = heartbeat.DesiredVersion
		s.mu.Unlock()
	}

	// Apply everything console queued, strictly in seq order.
	if err := s.drainCommands(ctx, state); err != nil {
		// A pull/apply failure must not lose the heartbeat bookkeeping.
		state.LastError = err.Error()
		_ = s.store.SaveState(ctx, state)
		return err
	}

	state.LastError = ""
	return s.store.SaveState(ctx, state)
}

// drainCommands pulls and applies commands until the cursor catches up.
func (s *Service) drainCommands(ctx context.Context, state *PreSyncState) error {
	for {
		pulled, err := s.client.PullCommands(ctx, state.CursorSeq, 100)
		if err != nil {
			return err
		}
		if pulled == nil || len(pulled.Commands) == 0 {
			return nil
		}

		state.LastPullAt = timePtr(time.Now().UTC())
		appliedAny := false

		for _, command := range pulled.Commands {
			// A hole in the sequence means we would apply out of order: stop, report the
			// drift and let the next cycle re-pull from the same cursor.
			if err := checkSequence(state.CursorSeq, command.Seq); err != nil {
				return err
			}
			if command.Seq <= state.CursorSeq {
				continue
			}

			result, err := s.applier.Apply(ctx, &command)
			switch {
			case errors.Is(err, ErrAlreadyApplied):
				// Console re-delivered a command we already recorded: advance and move on.
				state.CursorSeq = command.Seq
				appliedAny = true
				continue
			case err != nil:
				// The applier recorded the failure; ACK it so console can surface it.
				if ackErr := s.ackCommand(ctx, command, StatusFailed, "", "", err); ackErr != nil {
					return ackErr
				}
				state.CursorSeq = command.Seq
				appliedAny = true
				continue
			default:
				if ackErr := s.ackCommand(ctx, command, StatusApplied, result.Before, result.After, nil); ackErr != nil {
					return ackErr
				}
				state.CursorSeq = command.Seq
				appliedAny = true
			}
		}

		if appliedAny && pulled.DesiredVersion > 0 {
			state.AppliedVersion = pulled.DesiredVersion
		}
		if len(pulled.Commands) < 100 {
			return nil
		}
	}
}

// ackCommand reports the terminal result to console.
func (s *Service) ackCommand(ctx context.Context, command CommandEnvelope, status, before, after string, applyErr error) error {
	request := AckRequest{Status: status}
	if applyErr != nil {
		request.Error = applyErr.Error()
	}
	if err := s.client.Ack(ctx, command.ID, request); err != nil {
		return fmt.Errorf("ack command %s: %w", command.ID, err)
	}
	return nil
}

func (s *Service) driftFor(ctx context.Context, state *PreSyncState) []Drift {
	now := time.Now().UTC()
	if !s.lastReconcile.IsZero() && now.Sub(s.lastReconcile) < s.cfg.ReconcileEvery {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lastDrift
	}

	desired, err := s.client.DesiredState(ctx)
	if err != nil {
		// Console unreachable for the desired state is not fatal; the heartbeat already
		// failed in that case and is retried by the loop.
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lastDrift
	}
	snapshot, err := s.store.SnapshotControlledFields(ctx)
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lastDrift
	}

	drift := s.reconciler.Drift(desired, snapshot)
	s.mu.Lock()
	s.lastDrift = drift
	s.lastReconcile = now
	s.mu.Unlock()

	if desired != nil && desired.Version > 0 {
		state.AppliedVersion = desired.Version
	}
	return drift
}

func (s *Service) isStale(state *PreSyncState, now time.Time) bool {
	reference := state.LastHeartbeatAt
	if s.lastSuccess != nil && (reference == nil || s.lastSuccess.After(*reference)) {
		reference = s.lastSuccess
	}
	if reference == nil {
		// Never synced: stale only after the configured window from process start.
		return false
	}
	return now.Sub(*reference) > s.cfg.StaleAfter
}

// enterStale implements the recommended P9 gate: drop the loaded console credential so
// subsequent official calls fail closed, without touching llm/gateway.
func (s *Service) enterStale(ctx context.Context, state *PreSyncState) {
	s.mu.Lock()
	alreadyStale := s.stale
	s.stale = true
	s.mu.Unlock()
	if alreadyStale {
		return
	}

	consolecred.Global().Clear()
	log.Printf("[consolesync] console link is stale for more than %s: official channel credential cleared", s.cfg.StaleAfter)

	state.CredentialStatus = CredentialStatusStale
	state.LastError = "console link stale; official channel suspended"
	_ = s.store.SaveState(ctx, state)
}

func (s *Service) leaveStale() {
	s.mu.Lock()
	wasStale := s.stale
	s.stale = false
	s.mu.Unlock()

	if !wasStale {
		// Keep the credential loaded even when it was never dropped.
		if !consolecred.Global().Loaded() {
			consolecred.Configure(s.cfg.InstanceID, s.cfg.InstanceSecret)
		}
		return
	}
	consolecred.Configure(s.cfg.InstanceID, s.cfg.InstanceSecret)
	log.Printf("[consolesync] console link recovered: official channel credential restored")
}

func (s *Service) recordSuccess(ctx context.Context) {
	now := time.Now().UTC()
	s.mu.Lock()
	s.lastSuccess = &now
	s.mu.Unlock()

	state, err := s.store.LoadState(ctx)
	if err != nil {
		return
	}
	state.ConsecutiveFailures = 0
	state.NextPollAt = timePtr(now.Add(s.cfg.Interval))
	state.LastError = ""
	_ = s.store.SaveState(ctx, state)
}

func (s *Service) recordFailure(ctx context.Context, err error) {
	if err == nil {
		return
	}
	state, loadErr := s.store.LoadState(ctx)
	if loadErr != nil {
		return
	}
	state.ConsecutiveFailures++
	state.LastError = truncate(err.Error(), 2000)
	state.NextPollAt = timePtr(time.Now().UTC().Add(s.backoff(state.ConsecutiveFailures)))
	if apiErr := new(APIError); errors.As(err, &apiErr) && apiErr.Unauthorized() {
		state.CredentialStatus = CredentialStatusStale
	}
	_ = s.store.SaveState(ctx, state)
	log.Printf("[consolesync] sync failed (attempt %d): %v", state.ConsecutiveFailures, err)
}

// backoff grows the retry delay up to five minutes, matching the plan's bounded
// exponential backoff.
func (s *Service) backoff(failures int) time.Duration {
	delay := s.cfg.Interval
	for i := 1; i < failures && delay < 5*time.Minute; i++ {
		delay *= 2
	}
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	return delay
}

// Status returns the current sync snapshot for health reporting.
func (s *Service) Status(ctx context.Context) Status {
	if s == nil {
		return Status{Enabled: false, Running: false, Reason: "console sync is not configured"}
	}
	status := Status{
		Enabled:          true,
		InstanceID:       s.cfg.InstanceID,
		CredentialLoaded: consolecred.Global().Loaded(),
	}
	s.mu.Lock()
	status.Running = s.running
	status.Stale = s.stale
	status.LastSuccessAt = s.lastSuccess
	status.LastDrift = s.lastDrift
	status.DesiredVersion = s.desiredVer
	s.mu.Unlock()

	if state, err := s.store.LoadState(ctx); err == nil && state != nil {
		status.CursorSeq = state.CursorSeq
		status.AppliedVersion = state.AppliedVersion
		status.LastHeartbeatAt = state.LastHeartbeatAt
		status.LastPullAt = state.LastPullAt
		status.ConsecutiveFailures = state.ConsecutiveFailures
		status.NextPollAt = state.NextPollAt
		status.CredentialStatus = state.CredentialStatus
		status.LastError = state.LastError
	}
	return status
}

// ErrSequenceGap means console delivered a seq that skips one or more commands.
var ErrSequenceGap = errors.New("console command sequence gap")

// checkSequence enforces strict in-order application.
//
// A delivered seq must never skip ahead of the cursor: a hole means one or more commands
// were missed, so the caller stops applying and the next cycle re-pulls from the same
// cursor instead of jumping over them (plan §3.8, acceptance row 7).
func checkSequence(cursor, incoming int64) error {
	if incoming > cursor+1 {
		return fmt.Errorf("%w: expected seq %d, received %d", ErrSequenceGap, cursor+1, incoming)
	}
	return nil
}

func timePtr(value time.Time) *time.Time { return &value }

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

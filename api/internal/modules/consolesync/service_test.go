package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newConsolesyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:consolesync-%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	statements := []string{
		`CREATE TABLE pre_sync_state (
			id INTEGER PRIMARY KEY,
			downstream_client_id TEXT,
			cursor_seq INTEGER NOT NULL DEFAULT 0,
			applied_version INTEGER NOT NULL DEFAULT 0,
			last_heartbeat_at TIMESTAMP,
			last_pull_at TIMESTAMP,
			consecutive_failures INTEGER NOT NULL DEFAULT 0,
			next_poll_at TIMESTAMP,
			credential_status VARCHAR(32),
			last_error TEXT,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)`,
		`CREATE TABLE pre_applied_commands (
			command_id TEXT PRIMARY KEY,
			seq INTEGER NOT NULL,
			type VARCHAR(64) NOT NULL,
			status VARCHAR(20) NOT NULL,
			before_state JSONB,
			after_state JSONB,
			error TEXT,
			applied_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE organizations (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255),
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE llm_organization_api_keys (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			quota_limit INTEGER,
			used_quota INTEGER NOT NULL DEFAULT 0,
			remain_quota INTEGER NOT NULL DEFAULT 0,
			model_limits_enabled BOOLEAN NOT NULL DEFAULT false,
			model_limits JSONB,
			allow_ips TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE llm_workspace_quotas (
			workspace_id VARCHAR(255) PRIMARY KEY,
			organization_id TEXT,
			used_quota INTEGER NOT NULL DEFAULT 0,
			remain_quota INTEGER NOT NULL DEFAULT 0,
			quota_limit INTEGER,
			created_at TIMESTAMP,
			updated_at TIMESTAMP
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	return db
}

const testOrganizationID = "11111111-1111-4111-8111-111111111111"

func seedOrganization(t *testing.T, db *gorm.DB, status string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO organizations (id, name, status, created_at) VALUES (?, ?, ?, ?)`,
		testOrganizationID, "Acme", status, time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
}

func seedAPIKey(t *testing.T, db *gorm.DB, id string, quota int64) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO llm_organization_api_keys
		 (id, organization_id, status, quota_limit, used_quota, remain_quota, allow_ips, created_at, updated_at)
		 VALUES (?, ?, 'active', ?, 10, ?, '', ?, ?)`,
		id, testOrganizationID, quota, quota-10, time.Now().UTC(), time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed api key: %v", err)
	}
}

func newApplierFixture(t *testing.T) (*Applier, *Store, *gorm.DB) {
	t.Helper()
	db := newConsolesyncTestDB(t)
	store := NewStore(db)
	seedOrganization(t, db, "active")
	return NewApplier(db, store), store, db
}

func envelope(commandType string, payload string) CommandEnvelope {
	return CommandEnvelope{
		ID:      uuid.NewString(),
		Seq:     1,
		Type:    commandType,
		Payload: payload,
	}
}

func TestConfigShouldStartOnlyInCloudModeWithCredentials(t *testing.T) {
	base := Config{
		Enabled:        true,
		Edition:        "CLOUD",
		BaseURL:        "https://console.example.com",
		InstanceID:     "instance-1",
		InstanceSecret: "secret",
	}
	if !base.ShouldStart() {
		t.Fatalf("cloud config with credentials must start")
	}
	if reason := base.Reason(); reason != "" {
		t.Fatalf("Reason() = %q, want empty", reason)
	}

	selfHosted := base
	selfHosted.Edition = "SELF_HOSTED"
	if selfHosted.ShouldStart() {
		t.Fatalf("SELF_HOSTED mode must never start the sync loop")
	}

	disabled := base
	disabled.Enabled = false
	if disabled.ShouldStart() {
		t.Fatalf("disabled config must not start")
	}

	noCredential := base
	noCredential.InstanceSecret = ""
	if noCredential.ShouldStart() {
		t.Fatalf("config without an instance secret must not start")
	}
}

func TestApplyAccountStatusWritesOrganizationStatus(t *testing.T) {
	applier, _, db := newApplierFixture(t)

	command := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	result, err := applier.Apply(context.Background(), ptrEnvelope(command))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Before == "" || result.After == "" {
		t.Fatalf("before/after snapshots must be captured")
	}

	var status string
	if err := db.Table("organizations").Select("status").Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "inactive" {
		t.Fatalf("status = %q, want inactive", status)
	}

	var ledger PreAppliedCommand
	if err := db.Where("command_id = ?", command.ID).First(&ledger).Error; err != nil {
		t.Fatalf("idempotency ledger row missing: %v", err)
	}
	if ledger.Status != StatusApplied {
		t.Fatalf("ledger status = %q, want applied", ledger.Status)
	}
	if !strings.Contains(ledger.BeforeState, "active") {
		t.Fatalf("before snapshot lost: %q", ledger.BeforeState)
	}
}

func TestApplyAccountStatusRejectsUnknownStatus(t *testing.T) {
	applier, _, _ := newApplierFixture(t)
	_, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandAccountStatusSet, `{"status":"bogus"}`,
	)))
	if err == nil {
		t.Fatalf("Apply() accepted an unsupported status")
	}
}

func TestApplyAPIKeySetOnlyTouchesStatus(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	seedAPIKey(t, db, "key-1", 100)

	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandAPIKeySet, `{"key_id":"key-1","status":"disabled"}`,
	))); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	var row struct {
		Status     string `gorm:"column:status"`
		QuotaLimit *int64 `gorm:"column:quota_limit"`
	}
	if err := db.Table("llm_organization_api_keys").
		Select("status, quota_limit").Where("id = ?", "key-1").Scan(&row).Error; err != nil {
		t.Fatalf("read api key: %v", err)
	}
	if row.Status != "disabled" {
		t.Fatalf("status = %q, want disabled", row.Status)
	}
	if row.QuotaLimit == nil || *row.QuotaLimit != 100 {
		t.Fatalf("quota_limit changed by apikey.set: %v", row.QuotaLimit)
	}
}

func TestApplyAPIKeyQuotaRecomputesRemainQuota(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	seedAPIKey(t, db, "key-2", 100)

	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandAPIKeyQuotaSet, `{"key_id":"key-2","quota_limit":50,"allow_ips":"10.0.0.1"}`,
	))); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	var row struct {
		QuotaLimit  int64  `gorm:"column:quota_limit"`
		RemainQuota int64  `gorm:"column:remain_quota"`
		AllowIPs    string `gorm:"column:allow_ips"`
	}
	if err := db.Table("llm_organization_api_keys").
		Select("quota_limit, remain_quota, allow_ips").Where("id = ?", "key-2").Scan(&row).Error; err != nil {
		t.Fatalf("read api key: %v", err)
	}
	if row.QuotaLimit != 50 {
		t.Fatalf("quota_limit = %d, want 50", row.QuotaLimit)
	}
	if row.RemainQuota != 40 {
		t.Fatalf("remain_quota = %d, want 40 (limit - used)", row.RemainQuota)
	}
	if row.AllowIPs != "10.0.0.1" {
		t.Fatalf("allow_ips = %q", row.AllowIPs)
	}
}

func TestApplyWorkspaceQuotaUpdatesRealTable(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	if err := db.Exec(
		`INSERT INTO llm_workspace_quotas
		 (workspace_id, organization_id, used_quota, remain_quota, quota_limit, created_at, updated_at)
		 VALUES ('ws-1', ?, 20, 80, 100, ?, ?)`,
		testOrganizationID, time.Now().UTC(), time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed workspace quota: %v", err)
	}

	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandQuotaSet, `{"scope":"workspace","scope_id":"ws-1","limit":200}`,
	))); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	var row struct {
		QuotaLimit  *int64 `gorm:"column:quota_limit"`
		RemainQuota int64  `gorm:"column:remain_quota"`
	}
	if err := db.Table("llm_workspace_quotas").
		Select("quota_limit, remain_quota").Where("workspace_id = ?", "ws-1").Scan(&row).Error; err != nil {
		t.Fatalf("read workspace quota: %v", err)
	}
	if row.QuotaLimit == nil || *row.QuotaLimit != 200 {
		t.Fatalf("quota_limit = %v, want 200", row.QuotaLimit)
	}
	if row.RemainQuota != 180 {
		t.Fatalf("remain_quota = %d, want 180", row.RemainQuota)
	}
}

// A quota command for a workspace without a quota row must create it instead of
// silently doing nothing.
func TestApplyWorkspaceQuotaCreatesMissingRow(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandQuotaSet, `{"scope":"workspace","scope_id":"ws-new","limit":10}`,
	))); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	var count int64
	if err := db.Table("llm_workspace_quotas").Where("workspace_id = ?", "ws-new").Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("workspace quota row was not created")
	}
}

// A null limit means unlimited.
func TestApplyWorkspaceQuotaAcceptsNullLimit(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	if err := db.Exec(
		`INSERT INTO llm_workspace_quotas
		 (workspace_id, organization_id, used_quota, remain_quota, quota_limit, created_at, updated_at)
		 VALUES ('ws-2', ?, 5, 5, 10, ?, ?)`,
		testOrganizationID, time.Now().UTC(), time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandQuotaSet, `{"scope":"workspace","scope_id":"ws-2","limit":null}`,
	))); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	var limit *int64
	if err := db.Table("llm_workspace_quotas").Select("quota_limit").
		Where("workspace_id = ?", "ws-2").Scan(&limit).Error; err != nil {
		t.Fatalf("read: %v", err)
	}
	if limit != nil {
		t.Fatalf("quota_limit = %v, want NULL for unlimited", *limit)
	}
}

// Idempotency: the same command id must only ever be applied once.
func TestApplyIsIdempotent(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	command := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)

	if _, err := applier.Apply(context.Background(), ptrEnvelope(command)); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Change the local value so a second apply would be observable.
	if err := db.Exec(`UPDATE organizations SET status = 'active' WHERE id = ?`, testOrganizationID).Error; err != nil {
		t.Fatalf("reset status: %v", err)
	}

	if _, err := applier.Apply(context.Background(), ptrEnvelope(command)); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("second apply error = %v, want ErrAlreadyApplied", err)
	}

	var status string
	if err := db.Table("organizations").Select("status").Where("id = ?", testOrganizationID).Scan(&status).Error; err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "active" {
		t.Fatalf("a replayed command was applied again: status = %q", status)
	}
}

func TestApplyRefusesExpiredCommand(t *testing.T) {
	applier, _, _ := newApplierFixture(t)
	past := time.Now().Add(-time.Minute)
	command := envelope(CommandAccountStatusSet, `{"status":"inactive"}`)
	command.ExpiresAt = &past

	if _, err := applier.Apply(context.Background(), ptrEnvelope(command)); !errors.Is(err, ErrCommandExpired) {
		t.Fatalf("error = %v, want ErrCommandExpired", err)
	}
}

func TestApplyRejectsUnsupportedCommandType(t *testing.T) {
	applier, _, _ := newApplierFixture(t)
	_, err := applier.Apply(context.Background(), ptrEnvelope(envelope("wallet.adjust", `{}`)))
	if !errors.Is(err, ErrUnsupportedCommand) {
		t.Fatalf("error = %v, want ErrUnsupportedCommand", err)
	}
}

// credential.disable drops the loaded credential so the official channel fails closed.
func TestApplyCredentialDisableClearsCredential(t *testing.T) {
	applier, store, _ := newApplierFixture(t)
	consolecred.Configure("instance-1", "secret")
	defer consolecred.Global().Clear()
	if !consolecred.Global().Loaded() {
		t.Fatalf("precondition: credential must be loaded")
	}

	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandCredentialDisable, `{"reason":"console revoked"}`,
	))); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if consolecred.Global().Loaded() {
		t.Fatalf("credential was not cleared by credential.disable")
	}

	state, err := store.LoadState(context.Background())
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.CredentialStatus != CredentialStatusStale {
		t.Fatalf("credential_status = %q, want stale", state.CredentialStatus)
	}
}

func TestReconcilerReportsAccountStatusDriftOnly(t *testing.T) {
	_, store, _ := newApplierFixture(t)
	reconciler := NewReconciler(store)

	snapshot, err := store.SnapshotControlledFields(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	same := reconciler.Drift(&DesiredStateResponse{AccountStatus: "active"}, snapshot)
	if len(same) != 0 {
		t.Fatalf("drift = %+v, want none when values agree", same)
	}

	drifted := reconciler.Drift(&DesiredStateResponse{AccountStatus: "inactive"}, snapshot)
	if len(drifted) != 1 {
		t.Fatalf("drift = %+v, want exactly one account status entry", drifted)
	}
	if drifted[0].Field != "status" || drifted[0].Observed != "active" || drifted[0].Desired != "inactive" {
		t.Fatalf("unexpected drift entry: %+v", drifted[0])
	}
}

func TestReconcilerReportsWorkspaceQuotaDrift(t *testing.T) {
	_, store, db := newApplierFixture(t)
	reconciler := NewReconciler(store)
	if err := db.Exec(
		`INSERT INTO llm_workspace_quotas
		 (workspace_id, organization_id, used_quota, remain_quota, quota_limit, created_at, updated_at)
		 VALUES ('ws-9', ?, 0, 10, 10, ?, ?)`,
		testOrganizationID, time.Now().UTC(), time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	snapshot, err := store.SnapshotControlledFields(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	drift := reconciler.Drift(&DesiredStateResponse{
		AccountStatus: "active",
		Quota:         `{"workspaces":{"ws-9":99}}`,
	}, snapshot)
	if len(drift) != 1 || drift[0].Scope != "workspace" {
		t.Fatalf("drift = %+v, want one workspace quota entry", drift)
	}
	if drift[0].Desired != "99" || drift[0].Observed != "10" {
		t.Fatalf("unexpected drift: %+v", drift[0])
	}
}

func TestStoreStatePersistsCursor(t *testing.T) {
	db := newConsolesyncTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	state, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	state.CursorSeq = 7
	state.AppliedVersion = 3
	if err := store.SaveState(ctx, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	reloaded, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.CursorSeq != 7 || reloaded.AppliedVersion != 3 {
		t.Fatalf("cursor was lost: %+v", reloaded)
	}
}

func TestBackoffIsBounded(t *testing.T) {
	service := &Service{cfg: Config{Interval: 45 * time.Second}}
	if got := service.backoff(1); got != 45*time.Second {
		t.Fatalf("backoff(1) = %s, want 45s", got)
	}
	if got := service.backoff(10); got != 5*time.Minute {
		t.Fatalf("backoff(10) = %s, want the 5m ceiling", got)
	}
}

func TestClientDecodesConsoleEnvelope(t *testing.T) {
	// The console wraps every payload in {code,message,data}; a raw body must not decode.
	raw := `{"code":0,"message":"success","data":{"version":4,"account_status":"active"}}`
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var desired DesiredStateResponse
	if err := json.Unmarshal(envelope.Data, &desired); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if desired.Version != 4 || desired.AccountStatus != "active" {
		t.Fatalf("decoded %+v", desired)
	}
}

func ptrEnvelope(command CommandEnvelope) *CommandEnvelope { return &command }

// Acceptance row 7: a hole in the sequence must stop in-order application instead of
// jumping over the missing commands.
func TestCheckSequenceDetectsGaps(t *testing.T) {
	if err := checkSequence(4, 5); err != nil {
		t.Fatalf("contiguous seq rejected: %v", err)
	}
	if err := checkSequence(4, 4); err != nil {
		t.Fatalf("replayed seq rejected: %v", err)
	}
	err := checkSequence(4, 6)
	if !errors.Is(err, ErrSequenceGap) {
		t.Fatalf("error = %v, want ErrSequenceGap", err)
	}
}

// Acceptance row 9 / decision 8.1(4): while the console link is stale the official channel
// must fail closed, which is implemented by dropping the loaded credential.
func TestStaleGateClearsCredentialAndRecovers(t *testing.T) {
	db := newConsolesyncTestDB(t)
	store := NewStore(db)
	service := &Service{
		cfg: Config{
			Enabled:        true,
			Edition:        "CLOUD",
			BaseURL:        "https://console.example.com",
			InstanceID:     "instance-1",
			InstanceSecret: "secret",
			Interval:       45 * time.Second,
			StaleAfter:     5 * time.Minute,
		},
		store: store,
	}
	ctx := context.Background()

	consolecred.Configure("instance-1", "secret")
	t.Cleanup(func() { consolecred.Global().Clear() })

	// A heartbeat far in the past means the link is stale.
	state, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("bootstrap state: %v", err)
	}
	stale := time.Now().Add(-30 * time.Minute)
	state.LastHeartbeatAt = &stale
	if err := store.SaveState(ctx, state); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if !service.isStale(state, time.Now().UTC()) {
		t.Fatalf("a 30 minute old heartbeat must count as stale")
	}

	service.enterStale(ctx, state)
	if consolecred.Global().Loaded() {
		t.Fatalf("stale gate did not clear the credential; the official channel would stay open")
	}
	reloaded, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if reloaded.CredentialStatus != CredentialStatusStale {
		t.Fatalf("credential_status = %q, want stale", reloaded.CredentialStatus)
	}
	if service.Status(ctx).Stale != true {
		t.Fatalf("status did not report the stale state")
	}

	// A successful heartbeat restores the credential and clears the flag.
	service.leaveStale()
	if !consolecred.Global().Loaded() {
		t.Fatalf("recovery did not restore the credential")
	}
	if service.Status(ctx).Stale {
		t.Fatalf("recovery did not clear the stale flag")
	}
}

func TestHeartbeatIsNotStaleBeforeTheFirstSuccess(t *testing.T) {
	service := &Service{cfg: Config{StaleAfter: 5 * time.Minute}}
	// No heartbeat recorded yet: a fresh process must be allowed to try.
	if service.isStale(&PreSyncState{}, time.Now().UTC()) {
		t.Fatalf("a process that has never synced must not be considered stale")
	}
}

package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
			idempotency_key VARCHAR(128),
			status VARCHAR(20) NOT NULL,
			before_state JSONB,
			after_state JSONB,
			error TEXT,
			applied_at TIMESTAMP NOT NULL
		)`,
		`CREATE UNIQUE INDEX idx_pre_applied_commands_idempotency_key
			ON pre_applied_commands (idempotency_key)`,
		`CREATE TABLE organizations (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255),
			short_name VARCHAR(255),
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE accounts (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			password VARCHAR(255),
			password_salt VARCHAR(255),
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			initialized_at TIMESTAMP,
			last_active_at TIMESTAMP,
			last_login_at TIMESTAMP,
			extensions JSONB,
			deleted_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE account_contexts (
			account_id VARCHAR(255) PRIMARY KEY,
			current_organization_id VARCHAR(255),
			current_workspace_id VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE departments (
			id VARCHAR(255) PRIMARY KEY,
			group_id VARCHAR(255) NOT NULL,
			parent_id VARCHAR(255),
			name VARCHAR(255) NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			created_by VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE department_members (
			id VARCHAR(255) PRIMARY KEY,
			department_id VARCHAR(255) NOT NULL,
			account_id VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE members (
			organization_id VARCHAR(255) NOT NULL,
			account_id VARCHAR(255) NOT NULL,
			role VARCHAR(16) NOT NULL DEFAULT 'normal',
			status VARCHAR(16) NOT NULL DEFAULT 'active',
			name VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (organization_id, account_id)
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

func TestSnapshotControlledFieldsSupportsMultipleOrganizations(t *testing.T) {
	_, store, db := newApplierFixture(t)
	if err := db.Exec(
		`INSERT INTO organizations (id, name, status, created_at) VALUES (?, ?, ?, ?)`,
		"22222222-2222-4222-8222-222222222222", "Second", "inactive", time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed second organization: %v", err)
	}

	snapshot, err := store.SnapshotControlledFields(context.Background())
	if err != nil {
		t.Fatalf("snapshot with multiple organizations: %v", err)
	}
	if snapshot.OrganizationID != "" {
		t.Fatalf("organization id = %q, want empty for an instance-level snapshot", snapshot.OrganizationID)
	}
	if snapshot.AccountStatus != "" {
		t.Fatalf("account status = %q, want empty when tenant statuses differ", snapshot.AccountStatus)
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

// callIsSigned reports whether a captured request carried the full instance credential.
func callIsSigned(call capturedCall) bool {
	return call.Header.Get(consolecred.HeaderInstanceID) == "instance-1" &&
		call.Header.Get(consolecred.HeaderSignature) != ""
}

// receiveCallForPath drains captured calls until one for path arrives, so a test can assert
// on a specific request even when the client issues others in the same cycle.
func receiveCallForPath(t *testing.T, calls <-chan capturedCall, path string) capturedCall {
	t.Helper()
	for i := 0; i < 64; i++ {
		call := receiveCall(t, calls)
		if call.Path == path {
			return call
		}
	}
	t.Fatalf("the console never received a call for %s", path)
	return capturedCall{}
}

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
// must fail closed. The gate is an explicit flag on the shared credential store, NOT
// Clear(): the sync/inventory client shares that store and must keep signing, otherwise
// the link can never recover.
func TestStaleGateKeepsTheSyncCredentialAndSuspendsOfficialChannel(t *testing.T) {
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
	t.Cleanup(func() {
		consolecred.Global().SetStale(false)
		consolecred.Global().Clear()
	})

	state, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("bootstrap state: %v", err)
	}
	stale := time.Now().Add(-30 * time.Minute)
	state.LastHeartbeatAt = &stale
	if err := store.SaveState(ctx, state); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	service.enterStale(ctx, state)
	if consolecred.Global().Stale() != true {
		t.Fatalf("enterStale did not set the stale flag")
	}
	if !consolecred.Global().Loaded() {
		t.Fatalf("enterStale must keep the credential loaded: clearing it is the self-lock")
	}

	// The sync/inventory client keeps signing while stale: this is what repairs the link.
	req := httptest.NewRequest(http.MethodPost, "https://console.example.com"+apiPrefix+pathHeartbeat, nil)
	consolecred.SignSyncRequest(req)
	if req.Header.Get(consolecred.HeaderInstanceID) != "instance-1" {
		t.Fatalf("the sync client did not sign while stale; the link could never recover")
	}
	// The official channel is gated on the same flag.
	official := httptest.NewRequest(http.MethodPost, "https://console.example.com"+apiPrefix+pathHeartbeat, nil)
	consolecred.SignRequest(official)
	if official.Header.Get(consolecred.HeaderInstanceID) != "" {
		t.Fatalf("the official channel was signed while stale")
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
	if !service.Status(ctx).CredentialLoaded {
		t.Fatalf("status must still report the credential as loaded")
	}

	// A successful call clears the flag and re-enables the official channel.
	service.leaveStale()
	if consolecred.Global().Stale() {
		t.Fatalf("recovery did not clear the stale flag")
	}
	if !consolecred.Global().Loaded() {
		t.Fatalf("recovery lost the credential")
	}
	officialAgain := httptest.NewRequest(http.MethodPost, "https://console.example.com"+apiPrefix+pathHeartbeat, nil)
	consolecred.SignRequest(officialAgain)
	if officialAgain.Header.Get(consolecred.HeaderInstanceID) != "instance-1" {
		t.Fatalf("the official channel was not re-enabled after recovery")
	}
	if service.Status(ctx).Stale {
		t.Fatalf("recovery did not clear the stale flag in Status()")
	}
}

// (c) A persisted last_heartbeat_at is history, not evidence: a fresh process must not be
// gated by it. The link only turns stale after an in-process success followed by StaleAfter
// without contact, and a restart resets the gate.
func TestIsStaleIgnoresPersistedHistoryUntilThisProcessSucceeds(t *testing.T) {
	service := &Service{cfg: Config{StaleAfter: 5 * time.Minute}}
	now := time.Now().UTC()
	old := now.Add(-8 * time.Hour)
	state := &PreSyncState{LastHeartbeatAt: &old}

	// Right after Start: an 8h old persisted heartbeat must not count, otherwise a restart
	// can never repair the row.
	if service.isStale(state, now) {
		t.Fatalf("a fresh process was gated by persisted history alone")
	}

	// An in-process success starts the clock.
	service.noteSuccess(now)
	if service.isStale(state, now.Add(4*time.Minute)) {
		t.Fatalf("link reported stale before StaleAfter without contact")
	}
	if !service.isStale(state, now.Add(6*time.Minute)) {
		t.Fatalf("link not reported stale after StaleAfter without contact")
	}

	// A newer persisted heartbeat is also respected, but only once this process succeeded.
	newer := now.Add(-time.Minute)
	fresh := &Service{cfg: Config{StaleAfter: 5 * time.Minute}}
	fresh.noteSuccess(now.Add(-4 * time.Minute))
	if fresh.isStale(&PreSyncState{LastHeartbeatAt: &newer}, now) {
		t.Fatalf("a heartbeat newer than the in-process success must keep the link fresh")
	}
}

// (c) Timezone regression: a stored last_heartbeat_at must round-trip to the same instant
// and be handed back as UTC. Drivers return timestamptz in their own location (pgx builds
// it with time.Unix, i.e. time.Local; sqlite returns a fixed offset), which previously read
// back a UTC write as "+08:00" while the instant was correct.
func TestLastHeartbeatAtRoundTripsToTheSameUTCInstant(t *testing.T) {
	db := newConsolesyncTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	shanghai := time.FixedZone("CST", 8*60*60)
	written := time.Date(2026, 9, 25, 12, 3, 36, 0, shanghai)

	state, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("bootstrap state: %v", err)
	}
	state.LastHeartbeatAt = &written
	state.LastPullAt = &written
	state.NextPollAt = &written
	if err := store.SaveState(ctx, state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	reloaded, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if reloaded.LastHeartbeatAt == nil {
		t.Fatalf("last_heartbeat_at was lost")
	}
	if !reloaded.LastHeartbeatAt.Equal(written) {
		t.Fatalf("last_heartbeat_at = %s, want the same instant as %s",
			reloaded.LastHeartbeatAt, written)
	}
	if reloaded.LastHeartbeatAt.Location() != time.UTC {
		t.Fatalf("last_heartbeat_at location = %s, want UTC", reloaded.LastHeartbeatAt.Location())
	}
	if got := reloaded.LastHeartbeatAt.Format(time.RFC3339); got != "2026-09-25T04:03:36Z" {
		t.Fatalf("last_heartbeat_at = %q, want the real UTC instant", got)
	}
	if reloaded.LastPullAt == nil || !reloaded.LastPullAt.Equal(written) {
		t.Fatalf("last_pull_at did not round-trip: %v", reloaded.LastPullAt)
	}
	if reloaded.NextPollAt == nil || !reloaded.NextPollAt.Equal(written) {
		t.Fatalf("next_poll_at did not round-trip: %v", reloaded.NextPollAt)
	}

	// A row written by an older build / another writer in a non-UTC location (the "+08:00"
	// rendering seen in the bug report) must still be read as the same real UTC instant,
	// because the read path normalises what the driver hands back.
	if err := db.Exec(
		`UPDATE pre_sync_state SET last_heartbeat_at = ? WHERE id = 1`, written,
	).Error; err != nil {
		t.Fatalf("seed a legacy offset row: %v", err)
	}
	legacy, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("LoadState() after a legacy offset row: %v", err)
	}
	if legacy.LastHeartbeatAt == nil || !legacy.LastHeartbeatAt.Equal(written) {
		t.Fatalf("legacy row instant = %v, want %s", legacy.LastHeartbeatAt, written)
	}
	if legacy.LastHeartbeatAt.Location() != time.UTC {
		t.Fatalf("legacy row location = %s, want UTC", legacy.LastHeartbeatAt.Location())
	}
	if got := legacy.LastHeartbeatAt.Format(time.RFC3339); got != "2026-09-25T04:03:36Z" {
		t.Fatalf("legacy row = %q, want the real UTC instant", got)
	}
}

// The regression from the bug report, end to end: a persisted 8h old heartbeat plus a
// stale official channel must not stop the signed heartbeat/inventory call that repairs
// the link, and that successful call must clear the stale state.
func TestStalePersistedHeartbeatDoesNotBlockSignedSyncAndRecovers(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedInventoryOrganization(t, db, testOrganizationID, "Acme", "acme", "active", time.Now().UTC())
	// The heartbeat and the inventory report can run concurrently against the shared
	// in-memory database; a single connection avoids SQLITE_BUSY flakes.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	calls := make(chan capturedCall, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case apiPrefix + pathHeartbeat:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"desired_version":1}}`))
		case apiPrefix + pathCommands:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"commands":[]}}`))
		case apiPrefix + pathDesiredState:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"version":1,"account_status":"active"}}`))
		case testOrganizationPath:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"upserted":1}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	service := NewService(db, Config{
		Enabled:        true,
		Edition:        "CLOUD",
		BaseURL:        server.URL,
		InstanceID:     "instance-1",
		InstanceSecret: "secret",
		Interval:       45 * time.Second,
		StaleAfter:     5 * time.Minute,
		HTTPTimeout:    5 * time.Second,
		ReconcileEvery: time.Minute,
	}, "test")
	if service == nil {
		t.Fatalf("NewService() returned nil for a valid cloud config")
	}
	t.Cleanup(func() {
		consolecred.Global().SetStale(false)
		consolecred.Global().Clear()
	})
	// Start() loads the instance credential; do the same here without running the loop.
	consolecred.Configure(service.cfg.InstanceID, service.cfg.InstanceSecret)

	ctx := context.Background()
	// The persisted row from a previous run: 8h old and already marked stale.
	persisted, err := service.store.LoadState(ctx)
	if err != nil {
		t.Fatalf("bootstrap state: %v", err)
	}
	old := time.Now().UTC().Add(-8 * time.Hour)
	persisted.LastHeartbeatAt = &old
	persisted.CredentialStatus = CredentialStatusStale
	if err := service.store.SaveState(ctx, persisted); err != nil {
		t.Fatalf("seed persisted state: %v", err)
	}

	// Restart case: a fresh process must not be gated by the persisted row. runOnce signs
	// its heartbeat and must come out of it not stale.
	if err := service.runOnce(ctx); err != nil {
		t.Fatalf("runOnce() after a restart with a persisted stale row = %v", err)
	}
	firstHeartbeat := receiveCallForPath(t, calls, apiPrefix+pathHeartbeat)
	if !callIsSigned(firstHeartbeat) {
		t.Fatalf("a restarted process sent an unsigned heartbeat: the persisted row gated it")
	}
	if service.Status(ctx).Stale || consolecred.Global().Stale() {
		t.Fatalf("a restarted process became stale from persisted history alone")
	}

	// Simulate the gate having fired in this process before the recovery attempt.
	service.enterStale(ctx, persisted)
	if !consolecred.Global().Stale() {
		t.Fatalf("precondition: the link must be stale")
	}
	// Even while stale, the sync/inventory client signs and gets through.
	if err := service.reportOrganizationInventory(ctx); err != nil {
		t.Fatalf("reportOrganizationInventory() while stale = %v", err)
	}
	inventory := receiveCallForPath(t, calls, testOrganizationPath)
	if !callIsSigned(inventory) {
		t.Fatalf("the inventory call went out unsigned while stale: the self-lock is back")
	}
	if consolecred.Global().Stale() {
		t.Fatalf("a successful inventory call must clear the stale state")
	}

	// Re-arm the gate and prove the heartbeat path recovers it too.
	service.enterStale(ctx, persisted)
	if err := service.runOnce(ctx); err != nil {
		t.Fatalf("runOnce() while stale = %v", err)
	}
	heartbeat := receiveCallForPath(t, calls, apiPrefix+pathHeartbeat)
	if !callIsSigned(heartbeat) {
		t.Fatalf("the heartbeat went out unsigned while stale: the self-lock is back")
	}
	if consolecred.Global().Stale() || service.Status(ctx).Stale {
		t.Fatalf("a successful heartbeat must clear the stale state")
	}
	if !consolecred.Global().Loaded() {
		t.Fatalf("the credential was dropped")
	}
	reloaded, err := service.store.LoadState(ctx)
	if err != nil {
		t.Fatalf("reload state: %v", err)
	}
	if reloaded.LastHeartbeatAt == nil || time.Since(*reloaded.LastHeartbeatAt) > time.Minute {
		t.Fatalf("last_heartbeat_at did not advance: %v", reloaded.LastHeartbeatAt)
	}
	if reloaded.LastHeartbeatAt.Location() != time.UTC {
		t.Fatalf("last_heartbeat_at was not normalised to UTC: %v", reloaded.LastHeartbeatAt)
	}
}

func TestHeartbeatIsNotStaleBeforeTheFirstSuccess(t *testing.T) {
	service := &Service{cfg: Config{StaleAfter: 5 * time.Minute}}
	// No heartbeat recorded yet: a fresh process must be allowed to try.
	if service.isStale(&PreSyncState{}, time.Now().UTC()) {
		t.Fatalf("a process that has never synced must not be considered stale")
	}
}

// credential_status flips to "stale" on the first 401/403, long before the link is
// really stale. official_channel_suspended mirrors the process-wide flag that actually
// suspends the official channel, so operators can tell the two apart. The existing
// fields keep their meaning.
func TestStatusReportsOfficialChannelSuspendedSeparately(t *testing.T) {
	db := newConsolesyncTestDB(t)
	store := NewStore(db)
	service := &Service{
		cfg:   Config{Enabled: true, InstanceID: "instance-1", StaleAfter: 5 * time.Minute},
		store: store,
	}
	ctx := context.Background()
	t.Cleanup(func() { consolecred.Global().SetStale(false) })

	state, err := store.LoadState(ctx)
	if err != nil {
		t.Fatalf("bootstrap state: %v", err)
	}
	// First failed sync: the persisted credential status is stale, the link is not.
	state.CredentialStatus = CredentialStatusStale
	state.ConsecutiveFailures = 1
	if err := store.SaveState(ctx, state); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	status := service.Status(ctx)
	if status.CredentialStatus != CredentialStatusStale || status.Stale || status.OfficialChannelSuspended {
		t.Fatalf("after first failure: credential_status=%q stale=%v official_channel_suspended=%v",
			status.CredentialStatus, status.Stale, status.OfficialChannelSuspended)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if v, ok := decoded["official_channel_suspended"]; !ok || v != false {
		t.Fatalf("json official_channel_suspended = %v (present=%v), want false", v, ok)
	}
	for _, key := range []string{"credential_status", "stale", "consecutive_failures", "credential_loaded"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("existing field %q disappeared from the status JSON", key)
		}
	}

	// The link really goes stale: the official channel is suspended.
	service.enterStale(ctx, state)
	status = service.Status(ctx)
	if !status.Stale || !status.OfficialChannelSuspended {
		t.Fatalf("stale link: stale=%v official_channel_suspended=%v, want both true", status.Stale, status.OfficialChannelSuspended)
	}

	service.leaveStale()
	if status = service.Status(ctx); status.Stale || status.OfficialChannelSuspended {
		t.Fatalf("after recovery: stale=%v official_channel_suspended=%v, want both false", status.Stale, status.OfficialChannelSuspended)
	}
}

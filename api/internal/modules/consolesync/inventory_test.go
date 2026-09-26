package consolesync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"gorm.io/gorm"
)

// capturedCall is one request observed by a fake console. Copies are passed over a channel
// so the handler goroutine and the test never share memory unsafely.
type capturedCall struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

func captureConsoleCall(r *http.Request) capturedCall {
	body, _ := io.ReadAll(r.Body)
	return capturedCall{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
		Body:   body,
	}
}

const testOrganizationPath = apiPrefix + pathOrganizations

// Payload construction: field mapping, RFC3339/UTC timestamps and the exact JSON keys the
// console contract expects.
func TestBuildOrganizationInventoryShapesTheConsolePayload(t *testing.T) {
	updatedAt := time.Date(2026, 9, 26, 2, 40, 46, 0, time.UTC)
	syncedAt := time.Date(2026, 9, 26, 3, 0, 0, 0, time.UTC)

	payload := BuildOrganizationInventory([]OrganizationRecord{{
		ID:          "11111111-1111-4111-8111-111111111111",
		Name:        "默认组织",
		ShortName:   "",
		Status:      "active",
		MemberCount: 3,
		UpdatedAt:   updatedAt,
	}}, syncedAt)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	want := `{"organizations":[{"id":"11111111-1111-4111-8111-111111111111","name":"默认组织",` +
		`"short_name":"","status":"active","member_count":3,"updated_at":"2026-09-26T02:40:46Z"}],` +
		`"synced_at":"2026-09-26T03:00:00Z"}`
	if string(raw) != want {
		t.Fatalf("payload = %s\nwant       %s", raw, want)
	}
}

// A non-UTC zone and a sub-second timestamp are both normalised to whole-second UTC
// RFC3339, which is what console stores.
func TestBuildOrganizationInventoryNormalisesTimeToUTC(t *testing.T) {
	shanghai := time.FixedZone("CST", 8*60*60)
	// The same instant, written on a Shanghai wall clock with sub-second precision.
	updatedAt := time.Date(2026, 9, 26, 10, 40, 46, 123456789, shanghai)

	payload := BuildOrganizationInventory([]OrganizationRecord{
		{ID: "org-1", Name: "Acme", ShortName: "acme", Status: "inactive", UpdatedAt: updatedAt},
		{ID: "org-2", Name: "Empty", Status: "active", MemberCount: 7},
	}, time.Date(2026, 9, 26, 3, 0, 0, 0, shanghai))

	if got := payload.Organizations[0].UpdatedAt; got != "2026-09-26T02:40:46Z" {
		t.Fatalf("updated_at = %q, want the UTC RFC3339 rendering", got)
	}
	if got := payload.SyncedAt; got != "2026-09-25T19:00:00Z" {
		t.Fatalf("synced_at = %q, want the UTC RFC3339 rendering", got)
	}
	if payload.Organizations[0].ShortName != "acme" || payload.Organizations[0].Status != "inactive" {
		t.Fatalf("unexpected first organization: %+v", payload.Organizations[0])
	}
	if payload.Organizations[1].MemberCount != 7 || payload.Organizations[1].ShortName != "" {
		t.Fatalf("unexpected second organization: %+v", payload.Organizations[1])
	}
}

// member_count must count only active members, and an organization without members must
// still be reported with 0.
func TestListOrganizationRecordsCountsActiveMembersOnly(t *testing.T) {
	db := newConsolesyncTestDB(t)
	store := NewStore(db)
	updatedAt := time.Date(2026, 9, 26, 2, 40, 46, 0, time.UTC)

	seedInventoryOrganization(t, db, "org-a", "Acme", "acme", "active", updatedAt)
	seedInventoryOrganization(t, db, "org-b", "Bravo", "", "inactive", updatedAt)
	seedMember(t, db, "org-a", "account-1", "active")
	seedMember(t, db, "org-a", "account-2", "active")
	seedMember(t, db, "org-a", "account-3", "inactive")
	seedMember(t, db, "org-b", "account-4", "active")
	seedMember(t, db, "org-b", "account-5", "inactive")

	records, err := store.ListOrganizationRecords(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizationRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want both organizations", records)
	}

	first := records[0]
	if first.ID != "org-a" || first.Name != "Acme" || first.ShortName != "acme" || first.Status != "active" {
		t.Fatalf("unexpected first record: %+v", first)
	}
	if first.MemberCount != 2 {
		t.Fatalf("org-a member_count = %d, want 2 active members", first.MemberCount)
	}
	if got := first.UpdatedAt.UTC().Format(time.RFC3339); got != "2026-09-26T02:40:46Z" {
		t.Fatalf("org-a updated_at = %q", got)
	}

	second := records[1]
	if second.ID != "org-b" || second.ShortName != "" {
		t.Fatalf("unexpected second record: %+v", second)
	}
	if second.MemberCount != 1 {
		t.Fatalf("org-b member_count = %d, want 1 active member", second.MemberCount)
	}

	// The same rows must survive the payload mapping.
	payload := BuildOrganizationInventory(records, updatedAt)
	if payload.Organizations[0].MemberCount != 2 || payload.Organizations[0].UpdatedAt != "2026-09-26T02:40:46Z" {
		t.Fatalf("payload lost the member count/timestamp: %+v", payload.Organizations[0])
	}
}

// An organization with no member rows is reported with member_count 0.
func TestListOrganizationRecordsReportsZeroMembers(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedInventoryOrganization(t, db, "org-solo", "Solo", "", "active", time.Now().UTC())

	records, err := NewStore(db).ListOrganizationRecords(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizationRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].MemberCount != 0 {
		t.Fatalf("records = %+v, want one organization with member_count 0", records)
	}
}

// The exact HTTP call: method, path, instance credential headers and body.
func TestReportOrganizationInventoryPostsSignedPayload(t *testing.T) {
	db := newConsolesyncTestDB(t)
	updatedAt := time.Date(2026, 9, 26, 2, 40, 46, 0, time.UTC)
	seedInventoryOrganization(t, db, testOrganizationID, "Acme", "acme", "active", updatedAt)
	seedMember(t, db, testOrganizationID, "account-1", "active")
	seedMember(t, db, testOrganizationID, "account-2", "active")
	seedMember(t, db, testOrganizationID, "account-3", "inactive")

	calls := make(chan capturedCall, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"upserted":1}}`))
	}))
	defer server.Close()

	service := newInventoryService(t, db, server.URL)
	if err := service.reportOrganizationInventory(context.Background()); err != nil {
		t.Fatalf("reportOrganizationInventory() error = %v", err)
	}

	call := receiveCall(t, calls)
	if call.Method != http.MethodPost {
		t.Fatalf("method = %q, want POST", call.Method)
	}
	if call.Path != testOrganizationPath {
		t.Fatalf("path = %q, want %q", call.Path, testOrganizationPath)
	}
	if got := call.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := call.Header.Get(consolecred.HeaderInstanceID); got != "instance-1" {
		t.Fatalf("%s = %q, want the resolved instance id", consolecred.HeaderInstanceID, got)
	}
	timestamp := call.Header.Get(consolecred.HeaderTimestamp)
	if timestamp == "" {
		t.Fatalf("%s is missing", consolecred.HeaderTimestamp)
	}
	wantSignature := consolecred.Sign("secret", timestamp, testOrganizationPath)
	if got := call.Header.Get(consolecred.HeaderSignature); got != wantSignature {
		t.Fatalf("signature = %q, want HMAC over timestamp|path (%q)", got, wantSignature)
	}

	var body OrganizationInventoryRequest
	if err := json.Unmarshal(call.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body.Organizations) != 1 {
		t.Fatalf("organizations = %+v, want exactly one", body.Organizations)
	}
	organization := body.Organizations[0]
	if organization.ID != testOrganizationID || organization.Name != "Acme" ||
		organization.ShortName != "acme" || organization.Status != "active" ||
		organization.MemberCount != 2 || organization.UpdatedAt != "2026-09-26T02:40:46Z" {
		t.Fatalf("unexpected organization payload: %+v", organization)
	}
	if _, err := time.Parse(time.RFC3339, body.SyncedAt); err != nil {
		t.Fatalf("synced_at %q is not RFC3339: %v", body.SyncedAt, err)
	}
}

// Non-2xx and envelope code != 0 are both errors.
func TestReportOrganizationInventoryTreatsRejectionAsError(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		message string
	}{
		{
			name:    "http 500",
			status:  http.StatusInternalServerError,
			body:    `{"code":500,"message":"inventory exploded"}`,
			message: "inventory exploded",
		},
		{
			name:    "envelope code",
			status:  http.StatusOK,
			body:    `{"code":40001,"message":"organization not bound"}`,
			message: "organization not bound",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			db := newConsolesyncTestDB(t)
			seedInventoryOrganization(t, db, testOrganizationID, "Acme", "", "active", time.Now().UTC())

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(testCase.status)
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			err := newInventoryService(t, db, server.URL).reportOrganizationInventory(context.Background())
			if err == nil {
				t.Fatalf("reportOrganizationInventory() accepted a rejected report")
			}
			if !strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("error = %v, want it to carry %q", err, testCase.message)
			}
		})
	}
}

// A console that rejects the inventory must not stop the sync loop, and Start must still
// shut down cleanly (which also proves the report goroutine is joined).
func TestOrganizationReportFailureDoesNotStopTheLoop(t *testing.T) {
	db := newConsolesyncTestDB(t)
	// The heartbeat and the report run concurrently; a single connection avoids SQLITE_BUSY
	// flakes on the shared in-memory database.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	seedInventoryOrganization(t, db, testOrganizationID, "Acme", "", "active", time.Now().UTC())

	var heartbeats, organizations int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case apiPrefix + pathHeartbeat:
			atomic.AddInt32(&heartbeats, 1)
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"desired_version":1}}`))
		case apiPrefix + pathCommands:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"commands":[]}}`))
		case apiPrefix + pathDesiredState:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"version":1,"account_status":"active"}}`))
		case testOrganizationPath:
			atomic.AddInt32(&organizations, 1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":500,"message":"inventory exploded"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	service := NewService(db, Config{
		Enabled:                    true,
		Edition:                    "CLOUD",
		BaseURL:                    server.URL,
		InstanceID:                 "instance-1",
		InstanceSecret:             "secret",
		Interval:                   20 * time.Millisecond,
		StaleAfter:                 5 * time.Minute,
		HTTPTimeout:                5 * time.Second,
		ReconcileEvery:             time.Minute,
		OrganizationReportInterval: 25 * time.Millisecond,
	}, "test")
	if service == nil {
		t.Fatalf("NewService() returned nil for a valid cloud config")
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		service.Start(ctx)
	}()

	waitFor(t, 5*time.Second, func() bool {
		return atomic.LoadInt32(&organizations) >= 1 && atomic.LoadInt32(&heartbeats) >= 2
	}, "a failing inventory report alongside continuing heartbeats")

	cancel()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatalf("Start() did not return after context cancellation")
	}
}

func TestConfigFromAppCarriesTheOrganizationReportInterval(t *testing.T) {
	if got := ConfigFromApp(nil).OrganizationReportInterval; got != DefaultOrganizationReportInterval {
		t.Fatalf("nil config interval = %s, want the %s default", got, DefaultOrganizationReportInterval)
	}

	// 0 is an explicit "disabled" and must survive resolution.
	disabled := configWithReportInterval(0)
	if got := ConfigFromApp(&disabled).OrganizationReportInterval; got != 0 {
		t.Fatalf("interval = %s, want 0 (disabled)", got)
	}

	custom := configWithReportInterval(90 * time.Second)
	if got := ConfigFromApp(&custom).OrganizationReportInterval; got != 90*time.Second {
		t.Fatalf("interval = %s, want 90s", got)
	}
}

// configWithReportInterval builds the minimal application config needed to resolve the
// organization report cadence.
func configWithReportInterval(interval time.Duration) config.Config {
	return config.Config{
		Console: config.ConsoleConfig{SyncOrganizationReportInterval: interval},
	}
}

func newInventoryService(t *testing.T, db *gorm.DB, baseURL string) *Service {
	t.Helper()
	consolecred.Configure("instance-1", "secret")
	t.Cleanup(func() { consolecred.Global().Clear() })

	return &Service{
		cfg: Config{
			Enabled:                    true,
			Edition:                    "CLOUD",
			BaseURL:                    baseURL,
			InstanceID:                 "instance-1",
			InstanceSecret:             "secret",
			HTTPTimeout:                5 * time.Second,
			OrganizationReportInterval: time.Minute,
		},
		store:  NewStore(db),
		client: NewClient(baseURL, 5*time.Second),
	}
}

func seedInventoryOrganization(t *testing.T, db *gorm.DB, id, name, shortName, status string, updatedAt time.Time) {
	t.Helper()
	var short any
	if shortName != "" {
		short = shortName
	}
	if err := db.Exec(
		`INSERT INTO organizations (id, name, short_name, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		id, name, short, status, updatedAt.UTC(), updatedAt.UTC(),
	).Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
}

func seedMember(t *testing.T, db *gorm.DB, organizationID, accountID, status string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO members (organization_id, account_id, role, status) VALUES (?, ?, 'normal', ?)`,
		organizationID, accountID, status,
	).Error; err != nil {
		t.Fatalf("seed member: %v", err)
	}
}

func receiveCall(t *testing.T, calls <-chan capturedCall) capturedCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatalf("the console never received the expected request")
		return capturedCall{}
	}
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

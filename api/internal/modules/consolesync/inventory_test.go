package consolesync

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
		`"short_name":"","status":"active","member_count":3,"updated_at":"2026-09-26T02:40:46Z",` +
		`"departments":[],"members":[]}],` +
		`"synced_at":"2026-09-26T03:00:00Z"}`
	if string(raw) != want {
		t.Fatalf("payload = %s\nwant       %s", raw, want)
	}
}

// The new inventory fields: department rows and member rows, including a null and an
// RFC3339 last_login_at and a null department_id, exactly as console consumes them.
func TestBuildOrganizationInventoryShapesMembersAndDepartments(t *testing.T) {
	lastLogin := time.Date(2026, 9, 26, 2, 40, 46, 0, time.UTC)
	departmentID := "33333333-3333-4333-8333-333333333333"

	payload := BuildOrganizationInventory([]OrganizationRecord{{
		ID:          "11111111-1111-4111-8111-111111111111",
		Name:        "默认组织",
		Status:      "active",
		MemberCount: 2,
		UpdatedAt:   lastLogin,
		Departments: []OrganizationDepartmentRecord{
			{ID: departmentID, Name: "研发部", MemberCount: 3},
		},
		Members: []OrganizationMemberRecord{
			{
				AccountID:    "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
				Name:         "张三",
				Email:        "zhang@example.com",
				Role:         "admin",
				Status:       "active",
				LastLoginAt:  &lastLogin,
				DepartmentID: &departmentID,
			},
			{
				AccountID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
				Name:      "李四",
				Email:     "li@example.com",
				Role:      "normal",
				Status:    "inactive",
			},
		},
	}}, lastLogin)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	want := `{"organizations":[{"id":"11111111-1111-4111-8111-111111111111","name":"默认组织",` +
		`"short_name":"","status":"active","member_count":2,"updated_at":"2026-09-26T02:40:46Z",` +
		`"departments":[{"id":"33333333-3333-4333-8333-333333333333","name":"研发部","member_count":3}],` +
		`"members":[{"account_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","name":"张三",` +
		`"email":"zhang@example.com","role":"admin","status":"active",` +
		`"last_login_at":"2026-09-26T02:40:46Z","department_id":"33333333-3333-4333-8333-333333333333"},` +
		`{"account_id":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","name":"李四","email":"li@example.com",` +
		`"role":"normal","status":"inactive","last_login_at":null,"department_id":null}]}],` +
		`"synced_at":"2026-09-26T02:40:46Z"}`
	if string(raw) != want {
		t.Fatalf("payload = %s\nwant       %s", raw, want)
	}
}

// A non-UTC last_login_at is normalised to the same whole-second UTC RFC3339 console stores.
func TestBuildOrganizationInventoryNormalisesMemberLoginTime(t *testing.T) {
	shanghai := time.FixedZone("CST", 8*60*60)
	lastLogin := time.Date(2026, 9, 26, 10, 40, 46, 123456789, shanghai)

	payload := BuildOrganizationInventory([]OrganizationRecord{{
		ID: "org-1",
		Members: []OrganizationMemberRecord{
			{AccountID: "account-1", LastLoginAt: &lastLogin},
		},
	}}, time.Now().UTC())

	got := payload.Organizations[0].Members[0].LastLoginAt
	if got == nil || *got != "2026-09-26T02:40:46Z" {
		t.Fatalf("last_login_at = %v, want the UTC RFC3339 rendering", got)
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
	if len(records[0].Members) != 0 || len(records[0].Departments) != 0 {
		t.Fatalf("records = %+v, want empty member and department lists", records[0])
	}
}

// The member list must be the one pre's own contacts page shows: `members` joined to live
// `accounts` (soft-deleted accounts excluded), the member display name falling back to the
// account name, and the department resolved only within the same organization.
func TestListOrganizationRecordsReportsMembersAndDepartments(t *testing.T) {
	db := newConsolesyncTestDB(t)
	lastLogin := time.Date(2026, 9, 26, 2, 40, 46, 0, time.UTC)

	seedInventoryOrganization(t, db, "org-a", "Acme", "acme", "active", lastLogin)
	seedInventoryOrganization(t, db, "org-b", "Bravo", "", "active", lastLogin)

	seedInventoryAccount(t, db, "acc-1", "Account One", "one@example.com", &lastLogin, false)
	seedInventoryAccount(t, db, "acc-2", "Account Two", "two@example.com", nil, false)
	seedInventoryAccount(t, db, "acc-deleted", "Gone", "gone@example.com", nil, true)
	seedInventoryAccount(t, db, "acc-other", "Other", "other@example.com", nil, false)

	seedMember(t, db, "org-a", "acc-1", "active")
	seedMember(t, db, "org-a", "acc-2", "inactive")
	seedMember(t, db, "org-a", "acc-deleted", "active")
	seedMember(t, db, "org-b", "acc-other", "active")
	seedMemberName(t, db, "org-a", "acc-1", "Member One")

	seedDepartment(t, db, "dept-1", "org-a", "研发部", "active", 1)
	seedDepartment(t, db, "dept-archived", "org-a", "归档部", "archived", 2)
	seedDepartment(t, db, "dept-b", "org-b", "销售部", "active", 1)
	// acc-2's only department membership belongs to org-b, so it must not surface on org-a.
	// The deleted account still counts towards the raw department count pre's tree shows.
	seedDepartmentMember(t, db, "dm-1", "dept-1", "acc-1")
	seedDepartmentMember(t, db, "dm-2", "dept-1", "acc-deleted")
	seedDepartmentMember(t, db, "dm-3", "dept-b", "acc-other")
	seedDepartmentMember(t, db, "dm-4", "dept-b", "acc-2")

	records, err := NewStore(db).ListOrganizationRecords(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizationRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %+v, want both organizations", records)
	}

	orgA := records[0]
	if orgA.ID != "org-a" || len(orgA.Members) != 2 {
		t.Fatalf("org-a members = %+v, want the two live members", orgA.Members)
	}
	members := map[string]OrganizationMemberRecord{}
	for _, member := range orgA.Members {
		members[member.AccountID] = member
	}

	first := members["acc-1"]
	if first.Name != "Member One" || first.Email != "one@example.com" || first.Role != "normal" || first.Status != "active" {
		t.Fatalf("acc-1 = %+v, want the member display name and the account email/role/status", first)
	}
	if first.LastLoginAt == nil || first.LastLoginAt.UTC().Format(time.RFC3339) != "2026-09-26T02:40:46Z" {
		t.Fatalf("acc-1 last_login_at = %v, want the stored instant", first.LastLoginAt)
	}
	if first.DepartmentID == nil || *first.DepartmentID != "dept-1" {
		t.Fatalf("acc-1 department_id = %v, want dept-1", first.DepartmentID)
	}

	second := members["acc-2"]
	if second.Name != "Account Two" {
		t.Fatalf("acc-2 name = %q, want the account name fallback", second.Name)
	}
	if second.Status != "inactive" {
		t.Fatalf("acc-2 status = %q, want inactive (pre's list shows inactive members too)", second.Status)
	}
	if second.LastLoginAt != nil {
		t.Fatalf("acc-2 last_login_at = %v, want nil", second.LastLoginAt)
	}
	if second.DepartmentID != nil {
		t.Fatalf("acc-2 department_id = %v, want nil: its only department belongs to org-b", second.DepartmentID)
	}

	// Departments: the active list of the organization, with the raw member count pre's own
	// department tree shows.
	if len(orgA.Departments) != 1 || orgA.Departments[0].ID != "dept-1" || orgA.Departments[0].Name != "研发部" {
		t.Fatalf("org-a departments = %+v, want only the active department", orgA.Departments)
	}
	if orgA.Departments[0].MemberCount != 2 {
		t.Fatalf("dept-1 member_count = %d, want 2", orgA.Departments[0].MemberCount)
	}

	orgB := records[1]
	if len(orgB.Departments) != 1 || orgB.Departments[0].ID != "dept-b" {
		t.Fatalf("org-b departments = %+v, want only dept-b", orgB.Departments)
	}
	if len(orgB.Members) != 1 || orgB.Members[0].AccountID != "acc-other" {
		t.Fatalf("org-b members = %+v, want only acc-other", orgB.Members)
	}
	if orgB.Members[0].DepartmentID == nil || *orgB.Members[0].DepartmentID != "dept-b" {
		t.Fatalf("org-b member department_id = %v, want dept-b", orgB.Members[0].DepartmentID)
	}

	// The same rows must survive the payload mapping.
	payload := BuildOrganizationInventory(records, lastLogin)
	if len(payload.Organizations[0].Members) != 2 || len(payload.Organizations[0].Departments) != 1 {
		t.Fatalf("payload lost members or departments: %+v", payload.Organizations[0])
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

// A burst of "report now" signals from the command path must coalesce: at most one report
// runs at a time, and an arbitrarily large burst produces one extra report, not one per
// trigger.
func TestConcurrentOrganizationReportTriggersCoalesceToASingleInFlightReport(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedInventoryOrganization(t, db, testOrganizationID, "Acme", "acme", "active", time.Now().UTC())
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	var inFlight, maxInFlight, total int32
	release := make(chan struct{})
	started := make(chan struct{})
	var firstOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == testOrganizationPath {
			current := atomic.AddInt32(&inFlight, 1)
			for {
				highest := atomic.LoadInt32(&maxInFlight)
				if current <= highest || atomic.CompareAndSwapInt32(&maxInFlight, highest, current) {
					break
				}
			}
			atomic.AddInt32(&total, 1)
			firstOnce.Do(func() { close(started) })
			// Hold the first report open so the triggers below all arrive while it runs.
			<-release
			atomic.AddInt32(&inFlight, -1)
		}
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"upserted":1}}`))
	}))
	defer server.Close()

	service := newInventoryService(t, db, server.URL)
	stop := startOrganizationReporterForTest(t, service)
	defer stop()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatalf("the initial report never reached the console")
	}

	// Concurrent triggers, all while the initial report is still in flight.
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.requestOrganizationReport()
		}()
	}
	wg.Wait()
	close(release)

	waitFor(t, 5*time.Second, func() bool {
		return atomic.LoadInt32(&total) >= 2 && atomic.LoadInt32(&inFlight) == 0
	}, "the coalesced trigger to be reported")

	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("max concurrent reports = %d, want 1: triggers must never run in parallel", got)
	}
	if got := atomic.LoadInt32(&total); got < 2 || got > 3 {
		t.Fatalf("reports = %d, want the initial report plus at most two coalesced triggers", got)
	}
}

// A triggered report is a real console contact, so it must refresh the same link-health
// bookkeeping as the periodic one: the in-process success marker and the stale gate.
func TestTriggeredOrganizationReportRefreshesLinkHealth(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedInventoryOrganization(t, db, testOrganizationID, "Acme", "acme", "active", time.Now().UTC())
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	calls := make(chan capturedCall, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"upserted":1}}`))
	}))
	defer server.Close()

	service := newInventoryService(t, db, server.URL)
	t.Cleanup(func() { consolecred.Global().SetStale(false) })
	stop := startOrganizationReporterForTest(t, service)
	defer stop()

	initial := receiveCallForPath(t, calls, testOrganizationPath)
	if !callIsSigned(initial) {
		t.Fatalf("the initial report was not signed")
	}
	waitFor(t, 5*time.Second, func() bool {
		return service.Status(context.Background()).LastSuccessAt != nil
	}, "the initial report to record an in-process success")

	// Pretend the link went stale: the next report must repair it exactly like a periodic
	// one would.
	service.mu.Lock()
	service.stale = true
	service.lastSuccess = nil
	service.mu.Unlock()
	consolecred.Global().SetStale(true)

	service.requestOrganizationReport()

	triggered := receiveCallForPath(t, calls, testOrganizationPath)
	if !callIsSigned(triggered) {
		t.Fatalf("the triggered report went out unsigned while stale: the link could never recover")
	}
	waitFor(t, 5*time.Second, func() bool {
		status := service.Status(context.Background())
		return !consolecred.Global().Stale() && !status.Stale && status.LastSuccessAt != nil
	}, "the triggered report to refresh the link health")
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
		store:     NewStore(db),
		client:    NewClient(baseURL, 5*time.Second),
		reportNow: make(chan struct{}, 1),
	}
}

// startOrganizationReporterForTest runs the inventory reporter loop in the background and
// returns a stop function that cancels it and waits for it to be joined, so a test can
// shut the goroutine down before its fake console is closed.
func startOrganizationReporterForTest(t *testing.T, service *Service) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.runOrganizationReporter(ctx)
	}()
	return func() {
		cancel()
		<-done
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

// seedMemberName sets the member-level display name pre's contacts page prefers over the
// account name.
func seedMemberName(t *testing.T, db *gorm.DB, organizationID, accountID, name string) {
	t.Helper()
	if err := db.Exec(
		`UPDATE members SET name = ? WHERE organization_id = ? AND account_id = ?`,
		name, organizationID, accountID,
	).Error; err != nil {
		t.Fatalf("seed member name: %v", err)
	}
}

func seedInventoryAccount(t *testing.T, db *gorm.DB, id, name, email string, lastLoginAt *time.Time, deleted bool) {
	t.Helper()
	var deletedAt any
	if deleted {
		deletedAt = time.Now().UTC()
	}
	if err := db.Exec(
		`INSERT INTO accounts (id, name, email, status, last_login_at, deleted_at, created_at, updated_at, last_active_at)
		 VALUES (?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
		id, name, email, lastLoginAt, deletedAt, time.Now().UTC(), time.Now().UTC(), time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
}

func seedDepartment(t *testing.T, db *gorm.DB, id, organizationID, name, status string, sortOrder int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO departments (id, group_id, name, status, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, organizationID, name, status, sortOrder, time.Now().UTC(), time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
}

func seedDepartmentMember(t *testing.T, db *gorm.DB, id, departmentID, accountID string) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO department_members (id, department_id, account_id, created_at) VALUES (?, ?, ?, ?)`,
		id, departmentID, accountID, time.Now().UTC(),
	).Error; err != nil {
		t.Fatalf("seed department member: %v", err)
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

package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"gorm.io/gorm"
)

// Payload for organization.member.create, matching the agreed console contract.
func memberCreatePayload(organizationID, email, name, role string) string {
	raw, _ := json.Marshal(map[string]string{
		"organization_id": organizationID,
		"email":           email,
		"name":            name,
		"role":            role,
	})
	return string(raw)
}

// memberCreatePayloadWithDepartment is the same payload with the optional department_id.
func memberCreatePayloadWithDepartment(organizationID, email, name, role, departmentID string) string {
	raw, _ := json.Marshal(map[string]string{
		"organization_id": organizationID,
		"email":           email,
		"name":            name,
		"role":            role,
		"department_id":   departmentID,
	})
	return string(raw)
}

func seedMemberAccount(t *testing.T, db *gorm.DB, id, email, name, status string) {
	t.Helper()
	now := time.Now().UTC()
	if err := db.Exec(
		`INSERT INTO accounts (id, name, email, status, created_at, updated_at, last_active_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, name, email, status, now, now, now,
	).Error; err != nil {
		t.Fatalf("seed account: %v", err)
	}
}

func decodeMemberAck(t *testing.T, raw string) OrganizationMemberCreateAck {
	t.Helper()
	var ack OrganizationMemberCreateAck
	if err := json.Unmarshal([]byte(raw), &ack); err != nil {
		t.Fatalf("decode member create ack %q: %v", raw, err)
	}
	return ack
}

// Happy path: an unknown email becomes a pending account and exactly one active
// membership row with the mapped pre role.
func TestApplyOrganizationMemberCreateCreatesAccountAndMembership(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()

	command := envelope(CommandOrganizationMemberCreate,
		memberCreatePayload(testOrganizationID, "New.User@Example.com", "张三", "admin"))

	result, err := applier.Apply(ctx, ptrEnvelope(command))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	ack := decodeMemberAck(t, result.After)
	if !ack.Created {
		t.Fatalf("created = false, want true for a new account")
	}
	if ack.OrganizationID != testOrganizationID {
		t.Fatalf("organization_id = %q, want %q", ack.OrganizationID, testOrganizationID)
	}
	if ack.Email != "new.user@example.com" {
		t.Fatalf("email = %q, want the normalised address", ack.Email)
	}
	if ack.AccountID == "" || ack.MemberID != ack.AccountID {
		t.Fatalf("member_id/account_id = %q/%q, want the same non-empty id", ack.MemberID, ack.AccountID)
	}

	var account struct {
		Name          string
		Status        string
		Password      *string
		PasswordSalt  *string
		InitializedAt *time.Time
	}
	if err := db.Table("accounts").
		Select("name, status, password, password_salt, initialized_at").
		Where("id = ?", ack.AccountID).Scan(&account).Error; err != nil {
		t.Fatalf("read account: %v", err)
	}
	if account.Status != memberAccountStatusPending {
		t.Fatalf("account status = %q, want pending (invited, not activated)", account.Status)
	}
	if account.Password != nil || account.PasswordSalt != nil {
		t.Fatalf("console must never supply a password: password=%v salt=%v", account.Password, account.PasswordSalt)
	}
	if account.InitializedAt != nil {
		t.Fatalf("a pending account must not be marked initialised: %v", account.InitializedAt)
	}
	if account.Name != "张三" {
		t.Fatalf("account name = %q, want 张三", account.Name)
	}

	var member struct {
		Role   string
		Status string
		Name   *string
	}
	if err := db.Table("members").
		Select("role, status, name").
		Where("organization_id = ? AND account_id = ?", testOrganizationID, ack.AccountID).
		Scan(&member).Error; err != nil {
		t.Fatalf("read membership: %v", err)
	}
	if member.Role != preOrganizationRoleAdmin {
		t.Fatalf("member role = %q, want admin", member.Role)
	}
	if member.Status != memberStatusActive {
		t.Fatalf("member status = %q, want active", member.Status)
	}
	if member.Name == nil || *member.Name != "张三" {
		t.Fatalf("member name = %v, want 张三", member.Name)
	}

	var ledger PreAppliedCommand
	if err := db.Where("command_id = ?", command.ID).First(&ledger).Error; err != nil {
		t.Fatalf("idempotency ledger row missing: %v", err)
	}
	if ledger.Status != StatusApplied {
		t.Fatalf("ledger status = %q, want applied", ledger.Status)
	}
}

// An account that already exists (in any case) is reused; only the membership is added and
// the ACK reports created=false. Console's "member" maps onto pre's "normal" role.
func TestApplyOrganizationMemberCreateReusesExistingAccount(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	seedMemberAccount(t, db, "acc-existing", "Existing@Example.com", "Existing", "active")

	result, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandOrganizationMemberCreate,
		memberCreatePayload(testOrganizationID, "existing@example.com", "Existing", "member"),
	)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	ack := decodeMemberAck(t, result.After)
	if ack.Created {
		t.Fatalf("created = true, want false when the account already existed")
	}
	if ack.AccountID != "acc-existing" {
		t.Fatalf("account_id = %q, want acc-existing", ack.AccountID)
	}

	var accounts int64
	if err := db.Table("accounts").Where("LOWER(email) = ?", "existing@example.com").Count(&accounts).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if accounts != 1 {
		t.Fatalf("account count = %d, want 1", accounts)
	}

	var member struct {
		Role   string
		Status string
	}
	if err := db.Table("members").Select("role, status").
		Where("organization_id = ? AND account_id = ?", testOrganizationID, "acc-existing").
		Scan(&member).Error; err != nil {
		t.Fatalf("read membership: %v", err)
	}
	if member.Role != preOrganizationRoleNormal {
		t.Fatalf("member role = %q, want normal (console's member)", member.Role)
	}
	if member.Status != memberStatusActive {
		t.Fatalf("member status = %q, want active", member.Status)
	}
}

// Replay: the same command id is refused by the ledger, and the same org+email delivered
// under a fresh command id is a no-op that reports the same ids.
func TestApplyOrganizationMemberCreateIsIdempotentOnReplay(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()
	payload := memberCreatePayload(testOrganizationID, "replay@example.com", "Replay", "member")

	first := envelope(CommandOrganizationMemberCreate, payload)
	firstResult, err := applier.Apply(ctx, ptrEnvelope(first))
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	firstAck := decodeMemberAck(t, firstResult.After)
	if !firstAck.Created {
		t.Fatalf("first apply must create the account")
	}

	if _, err := applier.Apply(ctx, ptrEnvelope(first)); !errors.Is(err, ErrAlreadyApplied) {
		t.Fatalf("second apply of the same command id = %v, want ErrAlreadyApplied", err)
	}

	// The same membership requested under a new command id (a console retry after a lost
	// ACK) must not duplicate anything and must return the same ids.
	second := envelope(CommandOrganizationMemberCreate, payload)
	secondResult, err := applier.Apply(ctx, ptrEnvelope(second))
	if err != nil {
		t.Fatalf("replay with a new command id: %v", err)
	}
	secondAck := decodeMemberAck(t, secondResult.After)
	if secondAck.Created {
		t.Fatalf("replay reported created=true")
	}
	if secondAck.AccountID != firstAck.AccountID || secondAck.MemberID != firstAck.MemberID {
		t.Fatalf("replay ids changed: %+v vs %+v", secondAck, firstAck)
	}

	var accounts, members int64
	if err := db.Table("accounts").Where("LOWER(email) = ?", "replay@example.com").Count(&accounts).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if err := db.Table("members").Where("account_id = ?", firstAck.AccountID).Count(&members).Error; err != nil {
		t.Fatalf("count members: %v", err)
	}
	if accounts != 1 || members != 1 {
		t.Fatalf("replay duplicated rows: accounts=%d members=%d, want 1 and 1", accounts, members)
	}
}

// A department_id that belongs to the organization also places the member in pre's
// `department_members` table, the one the contacts page reads.
func TestApplyOrganizationMemberCreatePlacesMemberInDepartment(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	const departmentID = "33333333-3333-4333-8333-333333333333"
	seedDepartment(t, db, departmentID, testOrganizationID, "研发部", "active", 1)

	result, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandOrganizationMemberCreate,
		memberCreatePayloadWithDepartment(testOrganizationID, "dept@example.com", "张三", "member", departmentID),
	)))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	ack := decodeMemberAck(t, result.After)
	if !ack.Created {
		t.Fatalf("created = false, want true for a new account")
	}

	var membership struct {
		ID           string `gorm:"column:id"`
		DepartmentID string `gorm:"column:department_id"`
		AccountID    string `gorm:"column:account_id"`
	}
	if err := db.Table("department_members").
		Select("id, department_id, account_id").
		Where("account_id = ?", ack.AccountID).
		Scan(&membership).Error; err != nil {
		t.Fatalf("read department membership: %v", err)
	}
	if membership.ID == "" {
		t.Fatalf("no department_members row was written for %s", ack.AccountID)
	}
	if membership.DepartmentID != departmentID || membership.AccountID != ack.AccountID {
		t.Fatalf("department membership = %+v, want department %s account %s", membership, departmentID, ack.AccountID)
	}
}

// A department that does not belong to the organization -- or does not exist at all -- is a
// clear failure, and neither the account, the membership nor a department row is written.
func TestApplyOrganizationMemberCreateRejectsUnknownDepartment(t *testing.T) {
	const foreignOrganization = "22222222-2222-4222-8222-222222222222"
	const foreignDepartment = "44444444-4444-4444-8444-444444444444"
	const missingDepartment = "55555555-5555-4555-8555-555555555555"

	cases := []struct {
		name         string
		departmentID string
		seedForeign  bool
	}{
		{"department of another organization", foreignDepartment, true},
		{"department that does not exist", missingDepartment, false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			applier, _, db := newApplierFixture(t)
			if testCase.seedForeign {
				seedDepartment(t, db, foreignDepartment, foreignOrganization, "外部部门", "active", 1)
			}

			_, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
				CommandOrganizationMemberCreate,
				memberCreatePayloadWithDepartment(testOrganizationID, "user@example.com", "张三", "member", testCase.departmentID),
			)))
			if err == nil {
				t.Fatalf("Apply() accepted %s", testCase.name)
			}
			if !strings.Contains(err.Error(), testCase.departmentID) || !strings.Contains(err.Error(), "does not exist") {
				t.Fatalf("error = %q, want a clear unknown-department message", err.Error())
			}

			var accounts, members, departmentMembers int64
			if err := db.Table("accounts").Count(&accounts).Error; err != nil {
				t.Fatalf("count accounts: %v", err)
			}
			if err := db.Table("members").Count(&members).Error; err != nil {
				t.Fatalf("count members: %v", err)
			}
			if err := db.Table("department_members").Count(&departmentMembers).Error; err != nil {
				t.Fatalf("count department members: %v", err)
			}
			if accounts != 0 || members != 0 || departmentMembers != 0 {
				t.Fatalf("a rejected department wrote rows: accounts=%d members=%d department_members=%d",
					accounts, members, departmentMembers)
			}
		})
	}
}

// The department assignment is idempotent: the same command delivered again under a fresh
// command id must not create a second department_members row.
func TestApplyOrganizationMemberCreateDepartmentAssignmentIsIdempotent(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	ctx := context.Background()
	const departmentID = "33333333-3333-4333-8333-333333333333"
	seedDepartment(t, db, departmentID, testOrganizationID, "研发部", "active", 1)
	payload := memberCreatePayloadWithDepartment(testOrganizationID, "replay-dept@example.com", "张三", "member", departmentID)

	first, err := applier.Apply(ctx, ptrEnvelope(envelope(CommandOrganizationMemberCreate, payload)))
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	firstAck := decodeMemberAck(t, first.After)

	second, err := applier.Apply(ctx, ptrEnvelope(envelope(CommandOrganizationMemberCreate, payload)))
	if err != nil {
		t.Fatalf("replay with a new command id: %v", err)
	}
	if ack := decodeMemberAck(t, second.After); ack.Created {
		t.Fatalf("replay reported created=true")
	}

	var departmentMembers int64
	if err := db.Table("department_members").
		Where("account_id = ? AND department_id = ?", firstAck.AccountID, departmentID).
		Count(&departmentMembers).Error; err != nil {
		t.Fatalf("count department members: %v", err)
	}
	if departmentMembers != 1 {
		t.Fatalf("department_members rows = %d, want exactly 1", departmentMembers)
	}
}

// A member who already belongs to another department of the organization is refused, like
// pre's own direct-add flow, instead of ending up in two departments the UI would render twice.
func TestApplyOrganizationMemberCreateRejectsSecondDepartment(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	const firstDepartment = "33333333-3333-4333-8333-333333333333"
	const secondDepartment = "66666666-6666-4666-8666-666666666666"
	seedDepartment(t, db, firstDepartment, testOrganizationID, "研发部", "active", 1)
	seedDepartment(t, db, secondDepartment, testOrganizationID, "市场部", "active", 2)
	seedMemberAccount(t, db, "acc-existing", "existing@example.com", "Existing", "active")
	seedDepartmentMember(t, db, "dm-existing", firstDepartment, "acc-existing")

	_, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandOrganizationMemberCreate,
		memberCreatePayloadWithDepartment(testOrganizationID, "existing@example.com", "Existing", "member", secondDepartment),
	)))
	if err == nil {
		t.Fatalf("Apply() moved a member into a second department")
	}
	if !strings.Contains(err.Error(), firstDepartment) {
		t.Fatalf("error = %q, want it to name the member's current department", err.Error())
	}

	var departmentMembers, requested int64
	if err := db.Table("department_members").Where("account_id = ?", "acc-existing").Count(&departmentMembers).Error; err != nil {
		t.Fatalf("count department members: %v", err)
	}
	if err := db.Table("department_members").
		Where("account_id = ? AND department_id = ?", "acc-existing", secondDepartment).
		Count(&requested).Error; err != nil {
		t.Fatalf("count requested department members: %v", err)
	}
	if departmentMembers != 1 || requested != 0 {
		t.Fatalf("department_members rows = %d (requested %d), want the original row only", departmentMembers, requested)
	}
}

// An organization that does not exist is a clear failure and writes nothing.
func TestApplyOrganizationMemberCreateRejectsUnknownOrganization(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	const unknownOrganization = "22222222-2222-4222-8222-222222222222"

	_, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
		CommandOrganizationMemberCreate,
		memberCreatePayload(unknownOrganization, "user@example.com", "张三", "member"),
	)))
	if err == nil {
		t.Fatalf("Apply() accepted an unknown organization")
	}
	if !strings.Contains(err.Error(), unknownOrganization) || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %q, want a clear unknown-organization message", err.Error())
	}

	var accounts, members int64
	if err := db.Table("accounts").Count(&accounts).Error; err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if err := db.Table("members").Count(&members).Error; err != nil {
		t.Fatalf("count members: %v", err)
	}
	if accounts != 0 || members != 0 {
		t.Fatalf("a failed command wrote rows: accounts=%d members=%d", accounts, members)
	}
}

// Malformed payloads are rejected before anything is written.
func TestApplyOrganizationMemberCreateValidatesPayload(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"invalid organization uuid", memberCreatePayload("not-a-uuid", "user@example.com", "张三", "member")},
		{"missing email", memberCreatePayload(testOrganizationID, "", "张三", "member")},
		{"invalid email", memberCreatePayload(testOrganizationID, "not-an-email", "张三", "member")},
		{"unsupported role", memberCreatePayload(testOrganizationID, "user@example.com", "张三", "owner")},
		{"missing role", memberCreatePayload(testOrganizationID, "user@example.com", "张三", "")},
		{"invalid department uuid", memberCreatePayloadWithDepartment(
			testOrganizationID, "user@example.com", "张三", "member", "not-a-uuid")},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			applier, _, db := newApplierFixture(t)
			_, err := applier.Apply(context.Background(), ptrEnvelope(envelope(
				CommandOrganizationMemberCreate, testCase.payload,
			)))
			if err == nil {
				t.Fatalf("Apply() accepted %s", testCase.name)
			}
			var accounts, members int64
			if err := db.Table("accounts").Count(&accounts).Error; err != nil {
				t.Fatalf("count accounts: %v", err)
			}
			if err := db.Table("members").Count(&members).Error; err != nil {
				t.Fatalf("count members: %v", err)
			}
			if accounts != 0 || members != 0 {
				t.Fatalf("a rejected payload wrote rows: accounts=%d members=%d", accounts, members)
			}
		})
	}
}

// End to end through the sync pipeline: the command arrives from console and the ACK that
// goes back carries the member payload in `after`, which is what console correlates.
func TestOrganizationMemberCreateAcksTheCreatedMember(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedOrganization(t, db, "active")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	command := envelope(CommandOrganizationMemberCreate,
		memberCreatePayload(testOrganizationID, "acked@example.com", "张三", "admin"))
	commandsData, err := json.Marshal(PullCommandsResponse{
		Commands:       []CommandEnvelope{command},
		DesiredVersion: 1,
	})
	if err != nil {
		t.Fatalf("marshal commands: %v", err)
	}

	calls := make(chan capturedCall, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case apiPrefix + pathHeartbeat:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"desired_version":1}}`))
		case apiPrefix + pathCommands:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":` + string(commandsData) + `}`))
		case apiPrefix + pathDesiredState:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"version":1,"account_status":"active"}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
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
	consolecred.Configure(service.cfg.InstanceID, service.cfg.InstanceSecret)

	if err := service.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() = %v", err)
	}

	ackCall := receiveCallForPath(t, calls, apiPrefix+pathCommands+"/"+command.ID+"/ack")
	var ack AckRequest
	if err := json.Unmarshal(ackCall.Body, &ack); err != nil {
		t.Fatalf("decode ack request %s: %v", ackCall.Body, err)
	}
	if ack.Status != StatusApplied {
		t.Fatalf("ack status = %q, want applied", ack.Status)
	}
	memberAck := decodeMemberAck(t, ack.After)
	if !memberAck.Created || memberAck.Email != "acked@example.com" {
		t.Fatalf("ack after payload = %+v, want the created member", memberAck)
	}
	if memberAck.MemberID == "" || memberAck.AccountID != memberAck.MemberID {
		t.Fatalf("ack ids = %q/%q, want a non-empty shared id", memberAck.MemberID, memberAck.AccountID)
	}
	if memberAck.OrganizationID != testOrganizationID {
		t.Fatalf("ack organization_id = %q", memberAck.OrganizationID)
	}
}

// A rejected command is acknowledged as a failure carrying the applier's error message.
func TestOrganizationMemberCreateAcksFailures(t *testing.T) {
	db := newConsolesyncTestDB(t)
	seedOrganization(t, db, "active")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB(): %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	const unknownOrganization = "22222222-2222-4222-8222-222222222222"
	command := envelope(CommandOrganizationMemberCreate,
		memberCreatePayload(unknownOrganization, "user@example.com", "张三", "member"))
	commandsData, err := json.Marshal(PullCommandsResponse{
		Commands:       []CommandEnvelope{command},
		DesiredVersion: 1,
	})
	if err != nil {
		t.Fatalf("marshal commands: %v", err)
	}

	calls := make(chan capturedCall, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case apiPrefix + pathHeartbeat:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"desired_version":1}}`))
		case apiPrefix + pathCommands:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":` + string(commandsData) + `}`))
		case apiPrefix + pathDesiredState:
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"version":1,"account_status":"active"}}`))
		default:
			_, _ = w.Write([]byte(`{"code":0,"message":"success"}`))
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
	consolecred.Configure(service.cfg.InstanceID, service.cfg.InstanceSecret)

	if err := service.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() = %v", err)
	}

	ackCall := receiveCallForPath(t, calls, apiPrefix+pathCommands+"/"+command.ID+"/ack")
	var ack AckRequest
	if err := json.Unmarshal(ackCall.Body, &ack); err != nil {
		t.Fatalf("decode ack request %s: %v", ackCall.Body, err)
	}
	if ack.Status != StatusFailed {
		t.Fatalf("ack status = %q, want failed", ack.Status)
	}
	if !strings.Contains(ack.Error, "does not exist") {
		t.Fatalf("ack error = %q, want the unknown-organization message", ack.Error)
	}
}

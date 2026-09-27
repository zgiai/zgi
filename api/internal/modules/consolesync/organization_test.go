package consolesync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/util"
)

func TestApplyOrganizationCreateCreatesOwnerWithPrePasswordHash(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	organizationID := "11111111-1111-1111-1111-111111111111"
	password := "Owner-Password-123!"
	payload, _ := json.Marshal(map[string]string{
		"organization_id": organizationID,
		"name":            "Console Created Organization",
		"admin_email":     "owner@example.com",
		"password":        password,
	})
	result, err := applier.Apply(context.Background(), ptrEnvelope(envelope(CommandOrganizationCreate, string(payload))))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	var member struct{ AccountID, Role string }
	if err := db.Table("members").Select("account_id, role").Where("organization_id = ?", organizationID).Take(&member).Error; err != nil {
		t.Fatalf("read owner membership: %v", err)
	}
	if member.Role != "owner" {
		t.Fatalf("role = %q, want owner", member.Role)
	}
	var account struct{ Password, PasswordSalt, Status string }
	if err := db.Table("accounts").Select("password, password_salt, status").Where("id = ?", member.AccountID).Take(&account).Error; err != nil {
		t.Fatalf("read owner account: %v", err)
	}
	ok, err := util.ComparePasswordPBKDF2(password, account.Password, account.PasswordSalt)
	if err != nil || !ok || account.Status != "active" {
		t.Fatalf("owner credentials invalid: ok=%v err=%v status=%s", ok, err, account.Status)
	}
	if strings.Contains(result.After, password) {
		t.Fatalf("organization create ACK leaked plaintext password: %s", result.After)
	}
}

func TestApplyOrganizationDepartmentCreateCreatesDepartmentAndParent(t *testing.T) {
	applier, _, db := newApplierFixture(t)
	organizationID := testOrganizationID

	parentPayload, _ := json.Marshal(map[string]string{
		"organization_id": organizationID,
		"name":            "产品部",
	})
	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(CommandOrganizationDepartmentCreate, string(parentPayload)))); err != nil {
		t.Fatalf("create parent department: %v", err)
	}
	var parent struct{ ID string }
	if err := db.Table("departments").Select("id").Where("group_id = ? AND name = ?", organizationID, "产品部").Take(&parent).Error; err != nil {
		t.Fatalf("read parent department: %v", err)
	}

	childPayload, _ := json.Marshal(map[string]string{
		"organization_id": organizationID,
		"name":            "后端组",
		"parent_id":       parent.ID,
	})
	if _, err := applier.Apply(context.Background(), ptrEnvelope(envelope(CommandOrganizationDepartmentCreate, string(childPayload)))); err != nil {
		t.Fatalf("create child department: %v", err)
	}
	var child struct{ ParentID *string }
	if err := db.Table("departments").Select("parent_id").Where("group_id = ? AND name = ?", organizationID, "后端组").Take(&child).Error; err != nil {
		t.Fatalf("read child department: %v", err)
	}
	if child.ParentID == nil || *child.ParentID != parent.ID {
		t.Fatalf("child parent_id = %v, want %s", child.ParentID, parent.ID)
	}
}

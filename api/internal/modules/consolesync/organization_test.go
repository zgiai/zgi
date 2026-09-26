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

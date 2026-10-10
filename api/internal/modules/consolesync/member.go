package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	helper "github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

// This file implements the console command `organization.member.create` (plan P2/pre).
//
// It is the control-plane counterpart of pre's own "direct add member" flow
// (workspace/service/organization_member_invite_service.go): the account is looked up by
// email and created as a pending invitation when it is unknown, then exactly one
// organization membership row is ensured.
//
// FINDING (verified against the workspace module): every organization membership read path
// in pre -- repository.GetAccountJoin, organizationService.IsOrganizationMember,
// getOrganizationMembersPaginated, ListUserOrganizationsForAccount,
// GetFirstJoinedOrganization, chatruntime's IsOrganizationMember -- reads `members`
// (workspace/model.OrganizationMember) joined to `accounts`.
//
// `tenant_account_joins` is NOT a table in this schema: it is a read-only compatibility
// VIEW over `workspace_members` (migrations/baseline/70_compat_views.go) and no Go code in
// this repository reads it. `workspace_members` is *workspace* membership, so an
// organization-only member (no workspace assigned) has no workspace row to write. This
// command therefore writes `accounts` (only when the email is new), `members`, and -- when
// the optional `department_id` is present -- one `department_members` row, the table pre's
// own contacts page reads its department membership from.
//
// All statements go through GORM's builder against the shared table names, so postgres,
// sqlite and mysql behave identically without dialect-specific SQL.
//
// Password is optional for backwards-compatible commands. New Console commands carry it
// only through the encrypted control-plane queue; pre hashes it locally and activates the
// account immediately. Legacy commands without a password retain the pending invitation
// representation and cannot authenticate until the invitation is accepted.

// Organization member roles as console sends them.
const (
	commandRoleAdmin  = "admin"
	commandRoleMember = "member"
)

// pre's own organization member roles (workspace/model.OrganizationRole). Console's
// "member" is pre's "normal": the strings differ, the concept does not.
const (
	preOrganizationRoleAdmin  = "admin"
	preOrganizationRoleNormal = "normal"
)

// Account/member states used by this command. They mirror auth/model.AccountStatusPending,
// auth/model.AccountStatusActive and workspace/model.OrganizationMemberStatusActive.
const (
	memberAccountStatusPending = "pending"
	memberStatusActive         = "active"
)

// OrganizationMemberCreateAck is the ACK data of a successful organization.member.create.
//
// `members` has no surrogate id -- its primary key is the (organization_id, account_id)
// pair -- and pre's own member API identifies a member by its account id
// (OrganizationMemberWithExtensionResponse.ID = account.ID). member_id therefore carries
// the account id, the "equivalent" the contract allows.
//
// Created reports whether the *account* was created. It is false when the account already
// existed and only the membership was added (or when the whole command was a replay).
type OrganizationMemberCreateAck struct {
	MemberID       string `json:"member_id"`
	AccountID      string `json:"account_id"`
	OrganizationID string `json:"organization_id"`
	Created        bool   `json:"created"`
	Email          string `json:"email"`
}

// applyOrganizationMemberCreate adds one organization member.
//
// It runs inside the applier's transaction, so the account, the membership, the optional
// department membership and the idempotency ledger row commit together.
func (a *Applier) applyOrganizationMemberCreate(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	organizationID := strings.TrimSpace(rawString(payload, "organization_id"))
	email := normalizeMemberEmail(rawString(payload, "email"))
	role := strings.ToLower(rawString(payload, "role"))
	name := strings.TrimSpace(rawString(payload, "name"))
	departmentID := strings.TrimSpace(rawString(payload, "department_id"))
	password := rawString(payload, "password")

	if err := validateOrganizationMemberCreate(organizationID, email, role, departmentID); err != nil {
		return "", "", err
	}
	memberRole, err := preOrganizationMemberRole(role)
	if err != nil {
		return "", "", err
	}

	// The command carries the target organization explicitly, so an unknown id is a hard
	// failure instead of a guess: console must learn that nothing was applied.
	var organization struct {
		ID string `gorm:"column:id"`
	}
	err = tx.WithContext(ctx).Table("organizations").
		Select("id").
		Where("id = ?", organizationID).
		Take(&organization).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", fmt.Errorf("organization.member.create: organization %s does not exist", organizationID)
	}
	if err != nil {
		return "", "", fmt.Errorf("organization.member.create: read organization: %w", err)
	}

	// The optional department must belong to that same organization. An unknown or foreign
	// id is a hard failure, checked before anything is written.
	if departmentID != "" {
		if err := requireOrganizationDepartment(ctx, tx, organizationID, departmentID); err != nil {
			return "", "", err
		}
	}

	account, accountCreated, err := findOrCreateMemberAccount(ctx, tx, email, name, password)
	if err != nil {
		return "", "", err
	}

	memberCreated, err := ensureOrganizationMember(ctx, tx, organizationID, account.ID, memberRole, name)
	if err != nil {
		return "", "", err
	}

	if departmentID != "" {
		if err := ensureDepartmentMember(ctx, tx, organizationID, departmentID, account.ID); err != nil {
			return "", "", err
		}
	}

	before := marshalMap(map[string]interface{}{
		"account_existed": !accountCreated,
		"member_existed":  !memberCreated,
	})
	ack := OrganizationMemberCreateAck{
		MemberID:       account.ID,
		AccountID:      account.ID,
		OrganizationID: organizationID,
		Created:        accountCreated,
		Email:          account.Email,
	}
	return before, marshalMap(ack), nil
}

// memberAccount is the slice of `accounts` this command needs.
type memberAccount struct {
	ID     string `gorm:"column:id"`
	Email  string `gorm:"column:email"`
	Name   string `gorm:"column:name"`
	Status string `gorm:"column:status"`
}

// findOrCreateMemberAccount returns the account for the email, creating a pending
// invitation account when no live account matches.
//
// The lookup is case-insensitive and skips soft-deleted rows, so the command can never
// resurrect or collide with a deleted account.
func findOrCreateMemberAccount(ctx context.Context, tx *gorm.DB, email, name, password string) (*memberAccount, bool, error) {
	account, err := findMemberAccountByEmail(ctx, tx, email)
	if err == nil {
		return account, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	now := time.Now().UTC()
	status := memberAccountStatusPending
	var hashedPassword interface{}
	var passwordSalt interface{}
	var initializedAt interface{}
	if strings.TrimSpace(password) != "" {
		hashed, salt, err := helper.HashPasswordPBKDF2(password)
		if err != nil {
			return nil, false, fmt.Errorf("organization.member.create: hash password: %w", err)
		}
		hashedPassword = hashed
		passwordSalt = salt
		initializedAt = now
		status = "active"
	}
	account = &memberAccount{
		ID:     uuid.NewString(),
		Email:  email,
		Name:   memberAccountName(email, name),
		Status: status,
	}
	if err := tx.WithContext(ctx).Table("accounts").Create(map[string]interface{}{
		"id":             account.ID,
		"name":           account.Name,
		"email":          account.Email,
		"password":       hashedPassword,
		"password_salt":  passwordSalt,
		"status":         account.Status,
		"initialized_at": initializedAt,
		"created_at":     now,
		"updated_at":     now,
		"last_active_at": now,
	}).Error; err != nil {
		return nil, false, fmt.Errorf("organization.member.create: create pending account: %w", err)
	}
	return account, true, nil
}

// findMemberAccountByEmail finds the live account for an email, case-insensitively.
//
// If several rows somehow share the email the oldest one wins, so the result is stable
// across replays instead of depending on physical row order.
func findMemberAccountByEmail(ctx context.Context, tx *gorm.DB, email string) (*memberAccount, error) {
	var account memberAccount
	err := tx.WithContext(ctx).Table("accounts").
		Select("id, email, name, status").
		Where("LOWER(email) = ?", email).
		Where("deleted_at IS NULL").
		Order("created_at ASC, id ASC").
		Take(&account).Error
	if err != nil {
		return nil, fmt.Errorf("organization.member.create: query account by email: %w", err)
	}
	return &account, nil
}

// ensureOrganizationMember creates the single (organization_id, account_id) membership row
// when it is missing and reports whether it was created.
//
// Replays and pre-existing memberships are a no-op: the existing row, and in particular an
// existing role, is left untouched. A create command must never demote an administrator or
// the organization owner.
func ensureOrganizationMember(ctx context.Context, tx *gorm.DB, organizationID, accountID, role, name string) (bool, error) {
	var existing struct {
		AccountID string `gorm:"column:account_id"`
	}
	err := tx.WithContext(ctx).Table("members").
		Select("account_id").
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Take(&existing).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("organization.member.create: check membership: %w", err)
	}

	now := time.Now().UTC()
	var memberName interface{}
	if name != "" {
		memberName = name
	}
	if err := tx.WithContext(ctx).Table("members").Create(map[string]interface{}{
		"organization_id": organizationID,
		"account_id":      accountID,
		"role":            role,
		"name":            memberName,
		"status":          memberStatusActive,
		"created_at":      now,
		"updated_at":      now,
	}).Error; err != nil {
		return false, fmt.Errorf("organization.member.create: create membership: %w", err)
	}
	return true, nil
}

// requireOrganizationDepartment refuses a department that is not part of the organization.
//
// The lookup mirrors pre's own direct-add flow (getDirectAddDepartmentTx) and the
// department repository: `departments.id` scoped by `group_id`, with no status filter, so a
// department pre's own UI can still display is accepted.
func requireOrganizationDepartment(ctx context.Context, tx *gorm.DB, organizationID, departmentID string) error {
	var department struct {
		ID string `gorm:"column:id"`
	}
	err := tx.WithContext(ctx).Table("departments").
		Select("id").
		Where("id = ? AND group_id = ?", departmentID, organizationID).
		Take(&department).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("organization.member.create: department %s does not exist in organization %s",
			departmentID, organizationID)
	}
	if err != nil {
		return fmt.Errorf("organization.member.create: read department: %w", err)
	}
	return nil
}

// ensureDepartmentMember writes the one `department_members` row pre's own UI reads
// (workspace/model.DepartmentMember, written by AddMemberToDepartment / ChangeMemberDepartment
// and by the direct-add flow).
//
// pre keeps a member in at most one department per organization, so an existing membership
// is never duplicated:
//   - the requested department is already the member's department: idempotent no-op, so a
//     console retry under a fresh command id still succeeds;
//   - the member sits in a different department: the command fails with a clear error and
//     writes nothing, mirroring pre's direct-add conflict instead of silently creating a
//     second membership the UI would render twice.
func ensureDepartmentMember(ctx context.Context, tx *gorm.DB, organizationID, departmentID, accountID string) error {
	var existing struct {
		DepartmentID string `gorm:"column:department_id"`
	}
	err := tx.WithContext(ctx).
		Table("department_members AS dm").
		Select("dm.department_id").
		Joins("JOIN departments AS d ON d.id = dm.department_id").
		Where("d.group_id = ? AND dm.account_id = ?", organizationID, accountID).
		Take(&existing).Error
	if err == nil {
		if existing.DepartmentID == departmentID {
			return nil
		}
		return fmt.Errorf("organization.member.create: member %s already belongs to department %s in organization %s",
			accountID, existing.DepartmentID, organizationID)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("organization.member.create: check department membership: %w", err)
	}

	now := time.Now().UTC()
	if err := tx.WithContext(ctx).Table("department_members").Create(map[string]interface{}{
		"id":            uuid.NewString(),
		"department_id": departmentID,
		"account_id":    accountID,
		"created_at":    now,
	}).Error; err != nil {
		return fmt.Errorf("organization.member.create: create department membership: %w", err)
	}
	return nil
}

// validateOrganizationMemberCreate rejects a malformed payload before anything is written.
func validateOrganizationMemberCreate(organizationID, email, role, departmentID string) error {
	if _, err := uuid.Parse(organizationID); err != nil {
		return fmt.Errorf("organization.member.create: organization_id %q is not a valid uuid", organizationID)
	}
	if email == "" {
		return errors.New("organization.member.create: email is required")
	}
	if !validMemberEmail(email) {
		return fmt.Errorf("organization.member.create: email %q is not a valid email address", email)
	}
	if role != commandRoleAdmin && role != commandRoleMember {
		return fmt.Errorf("organization.member.create: role %q is not supported (expected %s or %s)",
			role, commandRoleAdmin, commandRoleMember)
	}
	if departmentID != "" {
		if _, err := uuid.Parse(departmentID); err != nil {
			return fmt.Errorf("organization.member.create: department_id %q is not a valid uuid", departmentID)
		}
	}
	return nil
}

// preOrganizationMemberRole maps the console role onto pre's own enum.
func preOrganizationMemberRole(role string) (string, error) {
	switch role {
	case commandRoleAdmin:
		return preOrganizationRoleAdmin, nil
	case commandRoleMember:
		return preOrganizationRoleNormal, nil
	default:
		return "", fmt.Errorf("organization.member.create: unsupported role %q", role)
	}
}

// normalizeMemberEmail lower-cases and trims, matching normalizeAccountEmail in the
// workspace module: pre stores account emails normalized.
func normalizeMemberEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validMemberEmail accepts a bare RFC 5322 address. A display-name form such as
// `Zhang <user@example.com>` is rejected because the command carries the address itself.
func validMemberEmail(email string) bool {
	if email == "" || strings.ContainsAny(email, " \t\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return parsed.Address == email
}

// memberAccountName keeps `accounts.name` non-null: the payload name wins, otherwise the
// email local part is used.
func memberAccountName(email, name string) string {
	if name != "" {
		return name
	}
	if at := strings.Index(email, "@"); at > 0 {
		return email[:at]
	}
	return email
}

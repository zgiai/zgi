package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	helper "github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

// applyOrganizationCreate creates the pre organization and its core owner account.
// The console sends the plaintext password only inside this command. It is immediately
// converted with pre's existing PBKDF2 helper and never appears in the ACK/inventory.
func (a *Applier) applyOrganizationCreate(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	organizationID := strings.TrimSpace(rawString(payload, "organization_id"))
	name := strings.TrimSpace(rawString(payload, "name"))
	email := normalizeMemberEmail(rawString(payload, "admin_email"))
	password := rawString(payload, "password")
	if _, err := uuid.Parse(organizationID); err != nil {
		return "", "", fmt.Errorf("organization.create: organization_id must be a valid uuid: %w", err)
	}
	if name == "" || email == "" || password == "" {
		return "", "", errors.New("organization.create: name, admin_email and password are required")
	}
	if !validMemberEmail(email) {
		return "", "", fmt.Errorf("organization.create: invalid admin email %q", email)
	}

	var existingOrg struct{ ID, Name string }
	err := tx.WithContext(ctx).Table("organizations").Select("id, name").Where("id = ?", organizationID).Take(&existingOrg).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", fmt.Errorf("organization.create: read organization: %w", err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		now := time.Now().UTC()
		if err := tx.WithContext(ctx).Table("organizations").Create(map[string]interface{}{
			"id": organizationID, "name": name, "short_name": name, "status": "active",
			"created_at": now, "updated_at": now,
		}).Error; err != nil {
			return "", "", fmt.Errorf("organization.create: create organization: %w", err)
		}
	} else if existingOrg.Name != name {
		return "", "", fmt.Errorf("organization.create: organization %s already exists with a different name", organizationID)
	}

	var owner struct{ AccountID, Email string }
	ownerErr := tx.WithContext(ctx).Table("members AS m").
		Select("m.account_id, a.email").Joins("JOIN accounts AS a ON a.id = m.account_id").
		Where("m.organization_id = ? AND m.role = ? AND m.status = ?", organizationID, "owner", "active").Take(&owner).Error
	if ownerErr == nil {
		if normalizeMemberEmail(owner.Email) != email {
			return "", "", fmt.Errorf("organization.create: organization %s already has a different owner", organizationID)
		}
		return marshalMap(map[string]interface{}{"organization_id": organizationID, "account_id": owner.AccountID, "created": false}),
			marshalMap(map[string]interface{}{"organization_id": organizationID, "account_id": owner.AccountID, "email": email, "role": "owner", "created": false}), nil
	}
	if !errors.Is(ownerErr, gorm.ErrRecordNotFound) {
		return "", "", fmt.Errorf("organization.create: read owner: %w", ownerErr)
	}

	_, err = findMemberAccountByEmail(ctx, tx, email)
	if err == nil {
		return "", "", fmt.Errorf("organization.create: admin email %s already exists", email)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", err
	}
	hashed, salt, err := helper.HashPasswordPBKDF2(password)
	if err != nil {
		return "", "", fmt.Errorf("organization.create: hash owner password: %w", err)
	}
	now := time.Now().UTC()
	accountID := uuid.NewString()
	if err := tx.WithContext(ctx).Table("accounts").Create(map[string]interface{}{
		"id": accountID, "name": name, "email": email, "password": hashed, "password_salt": salt,
		"status": "active", "initialized_at": now, "created_at": now, "updated_at": now, "last_active_at": now,
	}).Error; err != nil {
		return "", "", fmt.Errorf("organization.create: create owner account: %w", err)
	}
	if err := tx.WithContext(ctx).Table("members").Create(map[string]interface{}{
		"organization_id": organizationID, "account_id": accountID, "role": "owner", "status": "active",
		"name": name, "created_at": now, "updated_at": now,
	}).Error; err != nil {
		return "", "", fmt.Errorf("organization.create: create owner membership: %w", err)
	}
	// Login uses account_contexts to select the organization after authentication.
	// Keep it scoped to this organization; no workspace is invented by the control
	// plane because workspace creation is outside this command's contract.
	if err := tx.WithContext(ctx).Table("account_contexts").Create(map[string]interface{}{
		"account_id": accountID, "current_organization_id": organizationID,
		"created_at": now, "updated_at": now,
	}).Error; err != nil {
		return "", "", fmt.Errorf("organization.create: create owner context: %w", err)
	}
	return marshalMap(map[string]interface{}{"organization_id": organizationID, "account_id": accountID, "created": true}),
		marshalMap(map[string]interface{}{"organization_id": organizationID, "account_id": accountID, "email": email, "role": "owner", "created": true}), nil
}

package consolesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// applyOrganizationDepartmentCreate mirrors pre's own department creation service,
// but runs as a control-plane command so Console remains the place where organization
// administration is initiated. The department is deliberately created as a normal
// active department and is included in the next inventory report.
func (a *Applier) applyOrganizationDepartmentCreate(ctx context.Context, tx *gorm.DB, payload map[string]json.RawMessage) (string, string, error) {
	organizationID := strings.TrimSpace(rawString(payload, "organization_id"))
	name := strings.TrimSpace(rawString(payload, "name"))
	parentID := strings.TrimSpace(rawString(payload, "parent_id"))
	if parsed, err := uuid.Parse(organizationID); err != nil || parsed == uuid.Nil {
		return "", "", errors.New("organization.department.create: organization_id must be a valid uuid")
	}
	if name == "" {
		return "", "", errors.New("organization.department.create: name is required")
	}
	if utf8.RuneCountInString(name) > 50 {
		return "", "", errors.New("organization.department.create: name must be at most 50 characters")
	}
	if parentID != "" {
		if parsed, err := uuid.Parse(parentID); err != nil || parsed == uuid.Nil {
			return "", "", errors.New("organization.department.create: parent_id must be a valid uuid")
		}
	}

	var organization struct {
		ID string `gorm:"column:id"`
	}
	if err := tx.WithContext(ctx).Table("organizations").Select("id").Where("id = ?", organizationID).Take(&organization).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", fmt.Errorf("organization.department.create: organization %s does not exist", organizationID)
		}
		return "", "", fmt.Errorf("organization.department.create: read organization: %w", err)
	}

	if parentID != "" {
		if err := requireOrganizationDepartment(ctx, tx, organizationID, parentID); err != nil {
			return "", "", fmt.Errorf("organization.department.create: parent: %w", err)
		}
	}

	query := tx.WithContext(ctx).Table("departments").Where("group_id = ? AND name = ?", organizationID, name)
	if parentID == "" {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", parentID)
	}
	var existing struct {
		ID string `gorm:"column:id"`
	}
	if err := query.Select("id").Take(&existing).Error; err == nil {
		return "", "", fmt.Errorf("organization.department.create: department name %q already exists in the same parent", name)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", fmt.Errorf("organization.department.create: check department name: %w", err)
	}

	now := time.Now().UTC()
	departmentID := uuid.NewString()
	row := map[string]interface{}{
		"id": departmentID, "group_id": organizationID, "name": name, "status": "active",
		"sort_order": 0, "created_by": "console", "created_at": now, "updated_at": now,
	}
	if parentID != "" {
		row["parent_id"] = parentID
	}
	if err := tx.WithContext(ctx).Table("departments").Create(row).Error; err != nil {
		return "", "", fmt.Errorf("organization.department.create: create department: %w", err)
	}

	before := marshalMap(map[string]interface{}{"created": false})
	after := marshalMap(map[string]interface{}{
		"department_id": departmentID, "organization_id": organizationID,
		"name": name, "parent_id": nullableString(parentID), "created": true,
	})
	return before, after, nil
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

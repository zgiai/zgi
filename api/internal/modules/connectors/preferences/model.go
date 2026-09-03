// Package preferences owns personal new-chat initialization preferences. They
// are not runtime authority; only a target binding authorizes Connector use.
package preferences

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
)

const MaxDefaults = 64

type Mode string

const (
	ModeSelected Mode = "selected"
	ModeNone     Mode = "none"
)

type Selection struct {
	Mode         Mode   `json:"mode"`
	ConnectionID string `json:"connection_id,omitempty"`
}

type ConnectionSummary struct {
	ID                  uuid.UUID      `json:"id"`
	Name                string         `json:"name"`
	ExternalDisplayName string         `json:"external_display_name"`
	State               core.Readiness `json:"state"`
	UsableOperationIDs  []string       `json:"usable_operation_ids"`
}

type Default struct {
	Selection
	Connection *ConnectionSummary `json:"connection,omitempty"`
	Available  *bool              `json:"available,omitempty"`
}

type Snapshot struct {
	Revision int64              `json:"revision"`
	Defaults map[string]Default `json:"defaults"`
}

type UpdateCommand struct {
	OrganizationID   uuid.UUID
	OwnerAccountID   uuid.UUID
	ConnectorID      string
	Selection        Selection
	ExpectedRevision int64
	IdempotencyKey   string
}

type preferenceRecord struct {
	OrganizationID uuid.UUID      `gorm:"column:organization_id;primaryKey"`
	OwnerAccountID uuid.UUID      `gorm:"column:owner_account_id;primaryKey"`
	Revision       int64          `gorm:"column:revision"`
	Defaults       datatypes.JSON `gorm:"column:defaults"`
	CreatedAt      time.Time      `gorm:"column:created_at"`
	UpdatedAt      time.Time      `gorm:"column:updated_at"`
}

func (preferenceRecord) TableName() string { return "connector_chat_preferences" }

type mutationRecord struct {
	OrganizationID uuid.UUID `gorm:"column:organization_id;primaryKey"`
	OwnerAccountID uuid.UUID `gorm:"column:owner_account_id;primaryKey"`
	KeyHash        string    `gorm:"column:key_hash;primaryKey"`
	Fingerprint    string    `gorm:"column:fingerprint"`
	ResultRevision int64     `gorm:"column:result_revision"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (mutationRecord) TableName() string { return "connector_chat_preference_mutations" }

func validConnector(id string) bool {
	normalized, err := core.NormalizeIdentifier(id)
	return err == nil && normalized.String() == id
}

func validSelection(selection Selection) bool {
	switch selection.Mode {
	case ModeNone:
		return selection.ConnectionID == ""
	case ModeSelected:
		id, err := uuid.Parse(selection.ConnectionID)
		return err == nil && id != uuid.Nil && id.String() == selection.ConnectionID
	default:
		return false
	}
}

func decodeDefaults(raw []byte) (map[string]Selection, error) {
	var defaults map[string]Selection
	if len(raw) == 0 || json.Unmarshal(raw, &defaults) != nil || defaults == nil || len(defaults) > MaxDefaults {
		return nil, storageError(fmt.Errorf("invalid stored preference map"))
	}
	for id, selection := range defaults {
		if !validConnector(id) || !validSelection(selection) {
			return nil, storageError(fmt.Errorf("invalid stored preference entry"))
		}
	}
	return defaults, nil
}

func validateCommand(command UpdateCommand) error {
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil || !validConnector(command.ConnectorID) ||
		!validSelection(command.Selection) || command.ExpectedRevision < 0 ||
		len(command.IdempotencyKey) < 16 || len(command.IdempotencyKey) > 200 || strings.TrimSpace(command.IdempotencyKey) != command.IdempotencyKey {
		return inputError()
	}
	for _, char := range command.IdempotencyKey {
		if char < 33 || char > 126 {
			return inputError()
		}
	}
	return nil
}

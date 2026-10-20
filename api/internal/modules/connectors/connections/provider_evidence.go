package connections

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type providerEvidenceRecord struct {
	ConnectionID      uuid.UUID `gorm:"primaryKey"`
	OrganizationID    uuid.UUID
	OwnerAccountID    uuid.UUID
	ConnectorID       string
	CredentialVersion int
	Evidence          datatypes.JSON
	VerifiedAt        time.Time
}

func (providerEvidenceRecord) TableName() string { return "connector_connection_provider_evidence" }
func writeProviderEvidence(ctx context.Context, tx *gorm.DB, c core.PersonalConnection) error {
	if c.AuthMethodID != "feishu.app_credentials" && !isMailConnection(c) && !isMessagingConnection(c) && !isSocialEvidenceConnection(c) {
		return nil
	}
	if c.ProviderEvidence == nil {
		return tx.WithContext(ctx).Where("connection_id = ? AND organization_id = ? AND owner_account_id = ?", c.ID, c.OrganizationID, c.OwnerAccountID).Delete(&providerEvidenceRecord{}).Error
	}
	value := c.ProviderEvidence
	if isSocialEvidenceConnection(c) {
		if !validSocialEvidence(c, value.Social) {
			return invalidInputError("social account evidence identity")
		}
	} else if isMailConnection(c) {
		if value.Mail == nil || c.ExternalAccountID == nil || value.Mail.AuthenticatedLogin != *c.ExternalAccountID || value.Mail.Provider != c.ConnectorID.String() || value.Mail.IdentityKind != "authenticated_login" || !value.Mail.CredentialVerified || len(value.Mail.Operations) > 4 {
			return invalidInputError("mail evidence identity")
		}
	} else if isMessagingConnection(c) {
		if value.Messaging == nil || c.ExternalAccountID == nil || value.Messaging.Identity != *c.ExternalAccountID || value.Messaging.Provider != c.ConnectorID.String() || !value.Messaging.CredentialVerified || len(value.Messaging.Operations) > 16 {
			return invalidInputError("messaging evidence identity")
		}
	} else if c.ExternalAccountID == nil || value.Region+":"+value.AppID != *c.ExternalAccountID || len(value.GrantedScopes) > 256 || value.EventsEnabled || value.UserAuthorizationEnabled {
		return invalidInputError("provider evidence identity")
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 32768 {
		return invalidInputError("provider evidence size")
	}
	record := providerEvidenceRecord{ConnectionID: c.ID, OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectorID: c.ConnectorID.String(), CredentialVersion: c.CredentialVersion, Evidence: encoded, VerifiedAt: c.LastVerifiedAt}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "connection_id"}}, DoUpdates: clause.AssignmentColumns([]string{"credential_version", "evidence", "verified_at"})}).Create(&record).Error
}
func loadProviderEvidence(ctx context.Context, db *gorm.DB, c core.PersonalConnection) (core.PersonalConnection, error) {
	if c.AuthMethodID != "feishu.app_credentials" && !isMailConnection(c) && !isMessagingConnection(c) && !isSocialEvidenceConnection(c) {
		return c, nil
	}
	var record providerEvidenceRecord
	err := db.WithContext(ctx).Where("connection_id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND credential_version = ?", c.ID, c.OrganizationID, c.OwnerAccountID, c.ConnectorID.String(), c.CredentialVersion).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c, nil
	}
	if err != nil {
		return core.PersonalConnection{}, internalError("read provider evidence")
	}
	var value core.ProviderEvidence
	if json.Unmarshal(record.Evidence, &value) != nil {
		return core.PersonalConnection{}, internalError("decode provider evidence")
	}
	if isSocialEvidenceConnection(c) && !validSocialEvidence(c, value.Social) {
		return core.PersonalConnection{}, internalError("invalid social account evidence")
	}
	c.ProviderEvidence = value.Clone()
	return c, nil
}

func isSocialEvidenceConnection(c core.PersonalConnection) bool {
	for _, d := range catalog.SocialMCPDefinitions() {
		if d.ID == "linkedin-social" && c.ConnectorID.String() == d.ID && c.ProviderID.String() == d.ProviderID && c.DriverID.String() == d.DriverID && c.AuthMethodID.String() == d.AuthMethodID {
			return true
		}
	}
	return false
}
func validSocialEvidence(c core.PersonalConnection, e *core.SocialEvidence) bool {
	if e == nil || e.Version != 1 || e.Provider != c.ConnectorID.String() || c.ExternalAccountID == nil || e.Identity != *c.ExternalAccountID || len(e.AccountIDs) > 50 || e.CheckedAt.IsZero() {
		return false
	}
	seen := map[string]bool{}
	for _, id := range e.AccountIDs {
		const prefix = "urn:li:organization:"
		if !strings.HasPrefix(id, prefix) || len(id) <= len(prefix) || len(id) > 255 || seen[id] {
			return false
		}
		for _, r := range id[len(prefix):] {
			if r < '0' || r > '9' {
				return false
			}
		}
		seen[id] = true
	}
	return true
}

func isMessagingConnection(c core.PersonalConnection) bool {
	d, ok := catalog.MessagingDefinition(c.ConnectorID.String())
	return ok && d.ProviderID == c.ProviderID.String() && d.DriverID == c.DriverID.String() && d.AuthMethodID == c.AuthMethodID.String()
}

// InTransactionRepository returns a shallow, request-scoped lifecycle service.
// Dependencies and verification remain unchanged; only persistence is enlisted
// in the caller's atomic Connector onboarding transaction.
func (s *Service) InTransactionRepository(repo Repository) (*Service, error) {
	if s == nil || repo == nil {
		return nil, internalError("scope setup repository")
	}
	result := *s
	result.repository = repo
	return &result, nil
}
func (s *MaintenanceService) InTransactionRepository(repo MaintenanceRepository) (*MaintenanceService, error) {
	if s == nil || repo == nil {
		return nil, internalError("scope maintenance repository")
	}
	result := *s
	result.repository = repo
	return &result, nil
}

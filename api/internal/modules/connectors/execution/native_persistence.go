package execution

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormInvocationStore struct{ db *gorm.DB }

func NewGormInvocationStore(db *gorm.DB) *GormInvocationStore { return &GormInvocationStore{db: db} }

type invocationRecord struct {
	ReceiptID          uuid.UUID `gorm:"column:receipt_id;primaryKey"`
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	BindingID          uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        string
	TargetType         string
	TargetID           string
	TargetScope        string
	TargetVersionID    string
	BindingInstanceID  string
	OperationID        string
	LogicalOperationID string
	CorrelationID      string
	InputDigest        string
	ContractDigest     string
	BindingRevision    int
	CredentialVersion  int
	State              string
	CreatedAt          time.Time
}

func (invocationRecord) TableName() string { return "connector_invocation_claims" }

func invocationRow(claim InvocationClaim) invocationRecord {
	i, s := claim.Identity, claim.Snapshot
	return invocationRecord{OrganizationID: i.OrganizationID, OwnerAccountID: i.OwnerAccountID, ActingPrincipalID: i.ActingPrincipalID,
		BindingID: i.BindingID, ConnectionID: i.ConnectionID, ConnectorID: i.ConnectorID.String(), TargetType: string(i.Target.Type), TargetID: i.Target.ID,
		TargetScope: string(i.Target.EffectiveScope()), TargetVersionID: i.Target.VersionID, BindingInstanceID: i.Target.BindingInstanceID,
		OperationID: i.OperationID.String(), LogicalOperationID: i.LogicalOperationID, CorrelationID: i.CorrelationID,
		InputDigest: s.InputDigest, ContractDigest: s.ContractDigest, BindingRevision: s.BindingRevision, CredentialVersion: s.CredentialVersion}
}

func (store *GormInvocationStore) Claim(ctx context.Context, claim InvocationClaim) (RouteClaimReceipt, error) {
	if store == nil || store.db == nil {
		return "", connectorError(core.ErrorCodeInternal, false)
	}
	if !claim.valid() {
		return "", connectorError(core.ErrorCodeInvalidInput, false)
	}
	row := invocationRow(claim)
	row.ReceiptID = uuid.New()
	row.CreatedAt = time.Now().UTC()
	row.State = "claimed"
	result := store.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "logical_operation_id"}}, DoNothing: true}).Create(&row)
	if result.Error != nil {
		return "", connectorError(core.ErrorCodeInternal, false)
	}
	if result.RowsAffected != 1 {
		return "", connectorError(core.ErrorCodeConflict, false)
	}
	return RouteClaimReceipt(row.ReceiptID.String()), nil
}

func (store *GormInvocationStore) Resume(ctx context.Context, claim InvocationClaim, receipt RouteClaimReceipt) error {
	if store == nil || store.db == nil {
		return connectorError(core.ErrorCodeInternal, false)
	}
	id, err := uuid.Parse(string(receipt))
	if err != nil || id == uuid.Nil || !claim.valid() {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	var row invocationRecord
	if err = store.db.WithContext(ctx).Where("receipt_id = ? AND organization_id = ? AND owner_account_id = ?", id, claim.Identity.OrganizationID, claim.Identity.OwnerAccountID).Take(&row).Error; err != nil {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	// A converted terminal/unknown record remains a permanent replay barrier.
	if row.State != "claimed" && row.State != "awaiting_confirmation" {
		return connectorError(core.ErrorCodeConflict, false)
	}
	row.ReceiptID = uuid.Nil
	row.State = ""
	row.CreatedAt = time.Time{}
	if row != invocationRow(claim) {
		return connectorError(core.ErrorCodeConflict, false)
	}
	return nil
}

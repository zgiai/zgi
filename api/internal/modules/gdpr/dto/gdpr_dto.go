package dto

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/gdpr/model"
)

// ============================================================================
// Data Export DTOs
// ============================================================================

// ExportDataRequest represents a request to export user data
type ExportDataRequest struct {
	AccountID uuid.UUID `json:"account_id" binding:"required"`
	Format    string    `json:"format"` // json or csv, default json
}

// ExportDataResponse represents exported user data
type ExportDataResponse struct {
	AccountID    uuid.UUID              `json:"account_id"`
	Email        string                 `json:"email"`
	Name         string                 `json:"name"`
	ExportedAt   time.Time              `json:"exported_at"`
	Format       string                 `json:"format"`
	Tenants      []TenantExport         `json:"tenants,omitempty"`
	Transactions []TransactionExport    `json:"transactions,omitempty"`
	APIKeys      []APIKeyExport         `json:"api_keys,omitempty"`
	Consents     []model.UserConsent    `json:"consents,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TenantExport represents tenant data for export
type TenantExport struct {
	TenantID   uuid.UUID `json:"tenant_id"`
	TenantName string    `json:"tenant_name"`
	Role       string    `json:"role"`
	JoinedAt   time.Time `json:"joined_at"`
}

// TransactionExport represents transaction data for export
type TransactionExport struct {
	TransactionType string    `json:"transaction_type"`
	Amount          string    `json:"amount"`
	Currency        string    `json:"currency"`
	Description     string    `json:"description,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// APIKeyExport represents API key data for export (masked)
type APIKeyExport struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ============================================================================
// Data Erasure DTOs
// ============================================================================

// EraseDataRequest represents a request to erase/anonymize user data
type EraseDataRequest struct {
	AccountID       uuid.UUID `json:"account_id" binding:"required"`
	Reason          string    `json:"reason"`
	ConfirmationKey string    `json:"confirmation_key" binding:"required"` // Must match account email
}

// EraseDataResponse represents the result of data erasure
type EraseDataResponse struct {
	AccountID       uuid.UUID `json:"account_id"`
	Status          string    `json:"status"` // completed, partial, failed
	AnonymizedItems int       `json:"anonymized_items"`
	DeletedItems    int       `json:"deleted_items"`
	RetainedItems   int       `json:"retained_items"` // Items kept for legal/financial reasons
	Message         string    `json:"message,omitempty"`
	CompletedAt     time.Time `json:"completed_at"`
}

// ============================================================================
// Consent DTOs
// ============================================================================

// UpdateConsentRequest represents a request to update user consent
type UpdateConsentRequest struct {
	ConsentType model.ConsentType `json:"consent_type" binding:"required"`
	IsGranted   bool              `json:"is_granted"`
}

// ConsentStatusResponse represents current consent status
type ConsentStatusResponse struct {
	AccountID uuid.UUID           `json:"account_id"`
	Consents  []model.UserConsent `json:"consents"`
	UpdatedAt time.Time           `json:"updated_at"`
}

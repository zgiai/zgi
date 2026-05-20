package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/gdpr/dto"
	"github.com/zgiai/zgi/api/internal/modules/gdpr/model"
	"github.com/zgiai/zgi/api/internal/modules/gdpr/repository"
	"gorm.io/gorm"
)

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrInvalidConfirmation = errors.New("invalid confirmation key")
	ErrConsentNotFound     = errors.New("consent not found")
)

// GDPRService defines the interface for GDPR operations
type GDPRService interface {
	// Data Export
	ExportUserData(ctx context.Context, actorID uuid.UUID, req *dto.ExportDataRequest) (*dto.ExportDataResponse, error)

	// Data Erasure
	EraseUserData(ctx context.Context, actorID uuid.UUID, req *dto.EraseDataRequest) (*dto.EraseDataResponse, error)

	// Consent Management
	GetConsentStatus(ctx context.Context, accountID uuid.UUID) (*dto.ConsentStatusResponse, error)
	UpdateConsent(ctx context.Context, accountID uuid.UUID, req *dto.UpdateConsentRequest, ipAddress, userAgent string) error

	// Retention Cleanup (for scheduled job)
	RunRetentionCleanup(ctx context.Context) error
}

type gdprService struct {
	db   *gorm.DB
	repo repository.GDPRRepository
}

// NewGDPRService creates a new GDPR service
func NewGDPRService(db *gorm.DB, repo repository.GDPRRepository) GDPRService {
	return &gdprService{
		db:   db,
		repo: repo,
	}
}

// ============================================================================
// Data Export
// ============================================================================

func (s *gdprService) ExportUserData(ctx context.Context, actorID uuid.UUID, req *dto.ExportDataRequest) (*dto.ExportDataResponse, error) {
	// Get account info
	var account struct {
		ID    uuid.UUID
		Email string
		Name  string
	}
	if err := s.db.WithContext(ctx).
		Table("accounts").
		Select("id, email, name").
		Where("id = ?", req.AccountID).
		First(&account).Error; err != nil {
		return nil, ErrAccountNotFound
	}

	format := req.Format
	if format == "" {
		format = "json"
	}

	response := &dto.ExportDataResponse{
		AccountID:  account.ID,
		Email:      account.Email,
		Name:       account.Name,
		ExportedAt: time.Now(),
		Format:     format,
	}

	// Get tenant memberships
	var tenants []dto.TenantExport
	s.db.WithContext(ctx).
		Table("workspace_members taj").
		Select("taj.workspace_id, t.name as tenant_name, taj.role, taj.created_at as joined_at").
		Joins("LEFT JOIN tenants t ON t.id = taj.workspace_id").
		Where("taj.account_id = ?", req.AccountID).
		Scan(&tenants)
	response.Tenants = tenants

	// Get consents
	consents, _ := s.repo.GetConsentsByAccount(ctx, req.AccountID)
	response.Consents = make([]model.UserConsent, len(consents))
	for i, c := range consents {
		response.Consents[i] = *c
	}

	// Log the export action
	s.logAuditAction(ctx, model.ActionTypeDataExport, &actorID, account.Email, req.AccountID, account.Email, nil, map[string]interface{}{
		"format":       format,
		"tenant_count": len(tenants),
	})

	return response, nil
}

// ============================================================================
// Data Erasure
// ============================================================================

func (s *gdprService) EraseUserData(ctx context.Context, actorID uuid.UUID, req *dto.EraseDataRequest) (*dto.EraseDataResponse, error) {
	// Get account info for verification
	var account struct {
		ID    uuid.UUID
		Email string
		Name  string
	}
	if err := s.db.WithContext(ctx).
		Table("accounts").
		Select("id, email, name").
		Where("id = ?", req.AccountID).
		First(&account).Error; err != nil {
		return nil, ErrAccountNotFound
	}

	// Verify confirmation key matches email
	if req.ConfirmationKey != account.Email {
		return nil, ErrInvalidConfirmation
	}

	response := &dto.EraseDataResponse{
		AccountID:   req.AccountID,
		Status:      "completed",
		CompletedAt: time.Now(),
	}

	// Generate anonymized identifier
	anonymizedID := generateAnonymizedID(req.AccountID)

	// Transaction for data anonymization
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Anonymize account data (keep record but remove PII)
		result := tx.Table("accounts").
			Where("id = ?", req.AccountID).
			Updates(map[string]interface{}{
				"name":       anonymizedID,
				"email":      fmt.Sprintf("%s@deleted.local", anonymizedID),
				"avatar":     "",
				"password":   "",
				"status":     "deleted",
				"updated_at": time.Now(),
			})
		if result.Error != nil {
			return result.Error
		}
		response.AnonymizedItems += int(result.RowsAffected)

		// 2. Delete consents (no need to retain)
		result = tx.Table("user_consents").
			Where("account_id = ?", req.AccountID).
			Delete(&model.UserConsent{})
		if result.Error != nil {
			return result.Error
		}
		response.DeletedItems += int(result.RowsAffected)

		return nil
	})

	if err != nil {
		response.Status = "failed"
		response.Message = err.Error()

		s.logAuditAction(ctx, model.ActionTypeDataErasure, &actorID, "", req.AccountID, account.Email, nil, map[string]interface{}{
			"status": "failed",
			"error":  err.Error(),
			"reason": req.Reason,
		})
		return response, err
	}

	response.Message = fmt.Sprintf("Data anonymized successfully. %d items anonymized, %d items deleted, %d items retained for compliance.", response.AnonymizedItems, response.DeletedItems, response.RetainedItems)

	// Log the erasure action
	s.logAuditAction(ctx, model.ActionTypeDataErasure, &actorID, "", req.AccountID, account.Email, nil, map[string]interface{}{
		"status":           "completed",
		"anonymized_items": response.AnonymizedItems,
		"deleted_items":    response.DeletedItems,
		"retained_items":   response.RetainedItems,
		"reason":           req.Reason,
		"anonymized_id":    anonymizedID,
	})

	return response, nil
}

// ============================================================================
// Consent Management
// ============================================================================

func (s *gdprService) GetConsentStatus(ctx context.Context, accountID uuid.UUID) (*dto.ConsentStatusResponse, error) {
	consents, err := s.repo.GetConsentsByAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}

	result := &dto.ConsentStatusResponse{
		AccountID: accountID,
		Consents:  make([]model.UserConsent, len(consents)),
		UpdatedAt: time.Now(),
	}

	for i, c := range consents {
		result.Consents[i] = *c
	}

	return result, nil
}

func (s *gdprService) UpdateConsent(ctx context.Context, accountID uuid.UUID, req *dto.UpdateConsentRequest, ipAddress, userAgent string) error {
	now := time.Now()
	consent := &model.UserConsent{
		AccountID:   accountID,
		ConsentType: req.ConsentType,
		IsGranted:   req.IsGranted,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		UpdatedAt:   now,
	}

	if req.IsGranted {
		consent.GrantedAt = &now
		consent.RevokedAt = nil
	} else {
		consent.RevokedAt = &now
	}

	if err := s.repo.UpsertConsent(ctx, consent); err != nil {
		return err
	}

	// Log consent change
	s.logAuditAction(ctx, model.ActionTypeConsentChange, &accountID, "", accountID, "", nil, map[string]interface{}{
		"consent_type": req.ConsentType,
		"is_granted":   req.IsGranted,
	})

	return nil
}

// ============================================================================
// Retention Cleanup (Scheduled Job)
// ============================================================================

func (s *gdprService) RunRetentionCleanup(ctx context.Context) error {
	policies, err := s.repo.GetAllRetentionPolicies(ctx)
	if err != nil {
		return err
	}

	for _, policy := range policies {
		if !policy.IsActive {
			continue
		}

		switch policy.DataType {
		case "audit_logs":
			s.cleanupAuditLogs(ctx, policy)
		}
	}

	return nil
}

func (s *gdprService) cleanupAuditLogs(ctx context.Context, policy *model.DataRetentionPolicy) {
	// Hard delete old audit logs (beyond retention period)
	if policy.HardDeleteAfterDays != nil {
		cutoff := time.Now().AddDate(0, 0, -*policy.HardDeleteAfterDays)
		s.db.WithContext(ctx).
			Table("gdpr_audit_logs").
			Where("created_at < ?", cutoff).
			Delete(nil)
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func (s *gdprService) logAuditAction(ctx context.Context, actionType model.ActionType, actorID *uuid.UUID, actorEmail string, subjectID uuid.UUID, subjectEmail string, tenantID *uuid.UUID, details map[string]interface{}) {
	log := &model.GDPRAuditLog{
		ActionType:   actionType,
		ActorID:      actorID,
		ActorEmail:   actorEmail,
		SubjectID:    subjectID,
		SubjectEmail: subjectEmail,
		TenantID:     tenantID,
		Details:      details,
		Status:       model.AuditStatusCompleted,
		CreatedAt:    time.Now(),
	}
	_ = s.repo.CreateAuditLog(ctx, log)
}

func generateAnonymizedID(id uuid.UUID) string {
	hash := sha256.Sum256([]byte(id.String()))
	return "DELETED_" + hex.EncodeToString(hash[:])[:8]
}

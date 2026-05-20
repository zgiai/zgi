package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	"time"

	"github.com/google/uuid"

	"gorm.io/gorm"
)

type InvitationService interface {
	GenerateInvitationCode(ctx context.Context, tenantID, inviterID string, maxUses int, expiresAt time.Time) (*model.InvitationCode, error)
	ValidateInvitationCode(ctx context.Context, code string) (*model.InvitationCode, error)
	UseInvitationCode(ctx context.Context, code, accountID string) error
	GetInvitationsByTenant(ctx context.Context, tenantID string) ([]*model.InvitationCode, error)
	DeleteInvitationCode(ctx context.Context, id string) error
}

type invitationService struct {
	invitationRepo repository.InvitationRepository
}

func NewInvitationService(invitationRepo repository.InvitationRepository) InvitationService {
	return &invitationService{
		invitationRepo: invitationRepo,
	}
}

// GenerateInvitationCode generates an invitation code
func (s *invitationService) GenerateInvitationCode(ctx context.Context, tenantID, inviterID string, maxUses int, expiresAt time.Time) (*model.InvitationCode, error) {
	// Generate random invitation code
	codeBytes := make([]byte, 16)
	_, err := rand.Read(codeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random code: %w", err)
	}
	code := hex.EncodeToString(codeBytes)

	// Create invitation code
	invitation := &model.InvitationCode{
		ID:             uuid.New().String(),
		Batch:          uuid.New().String(),
		Code:           code,
		Email:          "", // Email needs to be passed in here
		Status:         model.InvitationStatusPending,
		UsedByTenantID: &tenantID,
		CreatedAt:      time.Now(),
	}

	err = s.invitationRepo.Create(ctx, invitation)
	if err != nil {
		return nil, fmt.Errorf("failed to create invitation code: %w", err)
	}

	return invitation, nil
}

// ValidateInvitationCode validates an invitation code
func (s *invitationService) ValidateInvitationCode(ctx context.Context, code string) (*model.InvitationCode, error) {
	invitation, err := s.invitationRepo.GetByCode(ctx, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid invitation code")
		}
		return nil, fmt.Errorf("failed to get invitation code: %w", err)
	}

	// Check status
	if !invitation.IsPending() {
		return nil, errors.New("invitation code is not active")
	}

	// Check if expired
	if invitation.IsExpired() {
		return nil, errors.New("invitation code has expired")
	}

	return invitation, nil
}

// UseInvitationCode uses an invitation code
func (s *invitationService) UseInvitationCode(ctx context.Context, code, accountID string) error {
	// Validate invitation code
	invitation, err := s.ValidateInvitationCode(ctx, code)
	if err != nil {
		return err
	}

	// Update invitation code status
	now := time.Now()
	invitation.Status = model.InvitationStatusUsed
	invitation.UsedAt = &now
	invitation.UsedByAccountID = &accountID

	err = s.invitationRepo.Update(ctx, invitation)
	if err != nil {
		return fmt.Errorf("failed to update invitation code: %w", err)
	}

	return nil
}

// GetInvitationsByTenant gets the list of invitation codes for a tenant
func (s *invitationService) GetInvitationsByTenant(ctx context.Context, tenantID string) ([]*model.InvitationCode, error) {
	invitations, err := s.invitationRepo.GetByTenantID(ctx, tenantID, 100, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to get invitations: %w", err)
	}

	return invitations, nil
}

// DeleteInvitationCode deletes an invitation code
func (s *invitationService) DeleteInvitationCode(ctx context.Context, id string) error {
	// Check if invitation code exists
	_, err := s.invitationRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get invitation code: %w", err)
	}

	// Delete invitation code
	err = s.invitationRepo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete invitation code: %w", err)
	}

	return nil
}

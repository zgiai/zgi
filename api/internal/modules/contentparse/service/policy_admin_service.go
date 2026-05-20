package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"github.com/zgiai/ginext/internal/modules/contentparse/repository"
)

type PolicyAdminService interface {
	GetPolicyByID(ctx context.Context, id uuid.UUID) (*model.RoutePolicy, error)
	ListPoliciesByScope(ctx context.Context, scope string, workspaceID *uuid.UUID) ([]*model.RoutePolicy, error)
	CreatePolicy(ctx context.Context, item *model.RoutePolicy) error
	UpdatePolicy(ctx context.Context, item *model.RoutePolicy) error
	DeletePolicy(ctx context.Context, id uuid.UUID) error
	ListRulesByPolicyID(ctx context.Context, policyID uuid.UUID) ([]*model.RoutePolicyRule, error)
	CreateRule(ctx context.Context, item *model.RoutePolicyRule) error
	UpdateRule(ctx context.Context, item *model.RoutePolicyRule) error
	DeleteRule(ctx context.Context, id uuid.UUID) error
}

type policyAdminService struct {
	repo repository.RoutePolicyRepository
}

func NewPolicyAdminService(repo repository.RoutePolicyRepository) PolicyAdminService {
	return &policyAdminService{repo: repo}
}

func (s *policyAdminService) GetPolicyByID(ctx context.Context, id uuid.UUID) (*model.RoutePolicy, error) {
	return s.repo.GetPolicyByID(ctx, id)
}

func (s *policyAdminService) ListPoliciesByScope(ctx context.Context, scope string, workspaceID *uuid.UUID) ([]*model.RoutePolicy, error) {
	return s.repo.ListPoliciesByScope(ctx, scope, workspaceID)
}

func (s *policyAdminService) CreatePolicy(ctx context.Context, item *model.RoutePolicy) error {
	return s.repo.CreatePolicy(ctx, item)
}

func (s *policyAdminService) UpdatePolicy(ctx context.Context, item *model.RoutePolicy) error {
	return s.repo.UpdatePolicy(ctx, item)
}

func (s *policyAdminService) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeletePolicy(ctx, id)
}

func (s *policyAdminService) ListRulesByPolicyID(ctx context.Context, policyID uuid.UUID) ([]*model.RoutePolicyRule, error) {
	return s.repo.ListRulesByPolicyID(ctx, policyID)
}

func (s *policyAdminService) CreateRule(ctx context.Context, item *model.RoutePolicyRule) error {
	return s.repo.CreateRule(ctx, item)
}

func (s *policyAdminService) UpdateRule(ctx context.Context, item *model.RoutePolicyRule) error {
	return s.repo.UpdateRule(ctx, item)
}

func (s *policyAdminService) DeleteRule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteRule(ctx, id)
}

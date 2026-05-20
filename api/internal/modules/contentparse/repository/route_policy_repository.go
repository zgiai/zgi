package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"gorm.io/gorm"
)

type RoutePolicyRepository interface {
	CreatePolicy(ctx context.Context, item *model.RoutePolicy) error
	GetPolicyByID(ctx context.Context, id uuid.UUID) (*model.RoutePolicy, error)
	GetPolicyByScopeAndKey(ctx context.Context, scope string, workspaceID *uuid.UUID, policyKey string) (*model.RoutePolicy, error)
	ListPoliciesByScope(ctx context.Context, scope string, workspaceID *uuid.UUID) ([]*model.RoutePolicy, error)
	UpdatePolicy(ctx context.Context, item *model.RoutePolicy) error
	DeletePolicy(ctx context.Context, id uuid.UUID) error

	CreateRule(ctx context.Context, item *model.RoutePolicyRule) error
	ListRulesByPolicyID(ctx context.Context, policyID uuid.UUID) ([]*model.RoutePolicyRule, error)
	UpdateRule(ctx context.Context, item *model.RoutePolicyRule) error
	DeleteRule(ctx context.Context, id uuid.UUID) error
}

type routePolicyRepository struct {
	db *gorm.DB
}

func NewRoutePolicyRepository(db *gorm.DB) RoutePolicyRepository {
	return &routePolicyRepository{db: db}
}

func (r *routePolicyRepository) CreatePolicy(ctx context.Context, item *model.RoutePolicy) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *routePolicyRepository) GetPolicyByID(ctx context.Context, id uuid.UUID) (*model.RoutePolicy, error) {
	var item model.RoutePolicy
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *routePolicyRepository) GetPolicyByScopeAndKey(ctx context.Context, scope string, workspaceID *uuid.UUID, policyKey string) (*model.RoutePolicy, error) {
	var item model.RoutePolicy
	query := r.db.WithContext(ctx).Where("scope = ? AND policy_key = ?", scope, policyKey)
	if workspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	err := query.First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *routePolicyRepository) ListPoliciesByScope(ctx context.Context, scope string, workspaceID *uuid.UUID) ([]*model.RoutePolicy, error) {
	var items []*model.RoutePolicy
	query := r.db.WithContext(ctx).Where("scope = ?", scope)
	if workspaceID == nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	err := query.Order("created_at DESC").Find(&items).Error
	return items, err
}

func (r *routePolicyRepository) UpdatePolicy(ctx context.Context, item *model.RoutePolicy) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *routePolicyRepository) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.RoutePolicy{}, "id = ?", id).Error
}

func (r *routePolicyRepository) CreateRule(ctx context.Context, item *model.RoutePolicyRule) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *routePolicyRepository) ListRulesByPolicyID(ctx context.Context, policyID uuid.UUID) ([]*model.RoutePolicyRule, error) {
	var items []*model.RoutePolicyRule
	err := r.db.WithContext(ctx).Where("policy_id = ?", policyID).Order("sort_order ASC, created_at ASC").Find(&items).Error
	return items, err
}

func (r *routePolicyRepository) UpdateRule(ctx context.Context, item *model.RoutePolicyRule) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *routePolicyRepository) DeleteRule(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.RoutePolicyRule{}, "id = ?", id).Error
}

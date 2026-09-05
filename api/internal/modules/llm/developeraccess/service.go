package developeraccess

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	accessmodel "github.com/zgiai/zgi/api/internal/modules/llm/developeraccess/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspacerepo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

var (
	ErrForbidden      = errors.New("developer access forbidden")
	ErrNotFound       = errors.New("developer access resource not found")
	ErrInvalid        = errors.New("invalid developer access request")
	ErrAccessDisabled = apperror.New(llmerrors.AppCodeDeveloperAccessDisabled)
	ErrApprovalNeeded = apperror.New(llmerrors.AppCodeDeveloperApprovalNeeded)
	ErrConflict       = apperror.New(llmerrors.AppCodeDeveloperAccessConflict)
	ErrQuotaExceeded  = errors.New("developer access quota exceeded")
)

const (
	// time.Duration stores nanoseconds in an int64. Keep TTL conversion within
	// that representation so an oversized API value cannot wrap into the past.
	maxDeveloperAccessTTLSeconds int64 = 9_223_372_036
	maxDeveloperAccessPage             = 1_000_000
)

type PolicyInput struct {
	Mode              string   `json:"mode" binding:"required"`
	DefaultQuota      *int64   `json:"default_quota"`
	MaxQuota          *int64   `json:"max_quota"`
	MaxKeys           int      `json:"max_keys"`
	DefaultTTLSeconds *int64   `json:"default_ttl_seconds"`
	MaxTTLSeconds     *int64   `json:"max_ttl_seconds"`
	AllowedModels     []string `json:"allowed_models"`
}

type CreateRequestInput struct {
	Purpose             string   `json:"purpose" binding:"required,max=2000"`
	Environment         string   `json:"environment"`
	RequestedQuota      *int64   `json:"requested_quota"`
	RequestedModels     []string `json:"requested_models"`
	RequestedTTLSeconds *int64   `json:"requested_ttl_seconds"`
}

type RequestQuery struct {
	Status   string `form:"status"`
	Scope    string `form:"scope"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type RequestPage struct {
	Items        []accessmodel.AccessRequest `json:"items"`
	Total        int64                       `json:"total"`
	PendingTotal int64                       `json:"pending_total"`
	Page         int                         `json:"page"`
	PageSize     int                         `json:"page_size"`
}

type ReviewRequestInput struct {
	QuotaLimit    *int64     `json:"quota_limit"`
	MaxKeys       *int       `json:"max_keys"`
	AllowedModels []string   `json:"allowed_models"`
	ExpiresAt     *time.Time `json:"expires_at"`
	Reason        string     `json:"reason"`
}

type CreateKeyInput struct {
	Name        string     `json:"name" binding:"required,max=255"`
	Environment string     `json:"environment"`
	ExpiresAt   *time.Time `json:"expires_at"`
	ModelNames  []string   `json:"model_names"`
}

type UpdateKeyInput struct {
	Name string `json:"name" binding:"required,max=255"`
}

type RevokeKeyInput struct {
	Reason string `json:"reason" binding:"max=500"`
}

type RotateKeyInput struct {
	Name      string     `json:"name" binding:"omitempty,max=255"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type KeyView struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	KeyMasked      string     `json:"key_masked"`
	PrincipalID    string     `json:"principal_id"`
	PrincipalName  string     `json:"principal_name,omitempty"`
	PrincipalEmail string     `json:"principal_email,omitempty"`
	Environment    string     `json:"environment"`
	ModelNames     []string   `json:"model_names"`
	CreatedAt      time.Time  `json:"created_at"`
	AccessedAt     *time.Time `json:"accessed_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	CanActivate    bool       `json:"can_activate"`
	CanRotate      bool       `json:"can_rotate"`
}

type KeyQuery struct {
	Scope    string `form:"scope"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

type KeyPage struct {
	Items    []KeyView `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

type AuditQuery struct {
	PrincipalID string `form:"principal_id"`
	APIKeyID    string `form:"api_key_id"`
	ModelName   string `form:"model_name"`
	Status      string `form:"status"`
	StartTime   int64  `form:"start_time"`
	EndTime     int64  `form:"end_time"`
	Page        int    `form:"page"`
	PageSize    int    `form:"page_size"`
}

type AuditItem struct {
	AttemptID          string    `json:"attempt_id"`
	RequestID          string    `json:"request_id"`
	PrincipalID        string    `json:"principal_id"`
	PrincipalName      string    `json:"principal_name,omitempty"`
	PrincipalEmail     string    `json:"principal_email,omitempty"`
	APIKeyID           string    `json:"api_key_id"`
	APIKeyName         string    `json:"api_key_name,omitempty"`
	APIKeyMasked       string    `json:"api_key_masked,omitempty"`
	ModelName          string    `json:"model_name"`
	ProviderName       string    `json:"provider_name"`
	Status             string    `json:"status"`
	PromptTokens       int64     `json:"prompt_tokens"`
	CompletionTokens   int64     `json:"completion_tokens"`
	TotalTokens        int64     `json:"total_tokens"`
	TotalPoints        int64     `json:"total_points"`
	QuotaChargedPoints int64     `json:"quota_charged_points"`
	QuotaOveragePoints int64     `json:"quota_overage_points"`
	ResponseTimeMS     int64     `json:"response_time_ms"`
	ErrorCode          *string   `json:"error_code,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type AuditPage struct {
	Items    []AuditItem `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

type CreatedKey struct {
	KeyView
	Secret string `json:"secret"`
}

type MeView struct {
	WorkspaceID      string                             `json:"workspace_id"`
	OrganizationID   string                             `json:"organization_id"`
	PrincipalType    string                             `json:"principal_type"`
	PrincipalID      string                             `json:"principal_id"`
	Role             workspacemodel.WorkspaceMemberRole `json:"role"`
	CanManage        bool                               `json:"can_manage"`
	CanCreateKey     bool                               `json:"can_create_key"`
	CanRequestAccess bool                               `json:"can_request_access"`
	Mode             string                             `json:"mode"`
	Policy           *accessmodel.Policy                `json:"policy"`
	Grant            *accessmodel.Grant                 `json:"grant,omitempty"`
	PendingRequest   *accessmodel.AccessRequest         `json:"pending_request,omitempty"`
	ActiveKeyCount   int64                              `json:"active_key_count"`
}

type workspaceScope struct {
	Workspace *workspacemodel.Workspace
	Member    *workspacemodel.WorkspaceMember
	CanManage bool
}

type Service struct {
	db                  *gorm.DB
	keys                apikeyrepo.APIKeyRepository
	members             workspacerepo.WorkspaceMemberRepository
	organizationService interfaces.OrganizationService
	now                 func() time.Time
}

func NewService(db *gorm.DB, keys apikeyrepo.APIKeyRepository, organizationService interfaces.OrganizationService) *Service {
	return &Service{
		db: db, keys: keys,
		members:             workspacerepo.NewWorkspaceMemberRepository(db),
		organizationService: organizationService,
		now:                 time.Now,
	}
}

func (s *Service) scope(ctx context.Context, workspaceID, accountID string) (*workspaceScope, error) {
	workspaceID, accountID = strings.TrimSpace(workspaceID), strings.TrimSpace(accountID)
	if workspaceID == "" || accountID == "" {
		return nil, ErrForbidden
	}
	var workspace workspacemodel.Workspace
	if err := s.db.WithContext(ctx).Where("id = ?", workspaceID).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load workspace: %w", err)
	}
	if workspace.OrganizationID == nil || strings.TrimSpace(*workspace.OrganizationID) == "" {
		return nil, ErrForbidden
	}
	if !workspace.IsNormal() {
		return nil, ErrForbidden
	}
	var organization workspacemodel.Organization
	if err := s.db.WithContext(ctx).Where("id = ?", *workspace.OrganizationID).First(&organization).Error; err != nil {
		return nil, fmt.Errorf("load workspace organization: %w", err)
	}
	if !organization.IsActive() {
		return nil, ErrForbidden
	}
	member, err := s.members.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		return nil, fmt.Errorf("load workspace member: %w", err)
	}
	canManage := member != nil && member.Role.IsAdminRole()
	if !canManage && s.organizationService != nil {
		allowed, permissionErr := s.organizationService.CheckWorkspacePermission(ctx, *workspace.OrganizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionWorkspaceManage)
		if permissionErr != nil {
			return nil, fmt.Errorf("check workspace permission: %w", permissionErr)
		}
		canManage = allowed
	}
	if member == nil && !canManage {
		return nil, ErrForbidden
	}
	return &workspaceScope{Workspace: &workspace, Member: member, CanManage: canManage}, nil
}

func defaultPolicy(scope *workspaceScope) *accessmodel.Policy {
	return &accessmodel.Policy{
		OrganizationID: *scope.Workspace.OrganizationID,
		WorkspaceID:    scope.Workspace.ID,
		Mode:           accessmodel.AccessModeApprovalRequired,
		MaxKeys:        3,
		AllowedModels:  []string{},
		Version:        1,
	}
}

// applySecurePolicyDefaults keeps finite ceilings effective even when an
// older policy row (or a partial policy update) omitted the corresponding
// default. A finite maximum and an unlimited default are contradictory; the
// maximum is the least-surprising, fail-closed value for new grant periods.
func applySecurePolicyDefaults(policy *accessmodel.Policy) {
	if policy.DefaultQuota == nil && policy.MaxQuota != nil {
		value := *policy.MaxQuota
		policy.DefaultQuota = &value
	}
	if policy.DefaultTTLSeconds == nil && policy.MaxTTLSeconds != nil {
		value := *policy.MaxTTLSeconds
		policy.DefaultTTLSeconds = &value
	}
}

func applySecurePolicyInputDefaults(input PolicyInput) PolicyInput {
	if input.DefaultQuota == nil && input.MaxQuota != nil {
		value := *input.MaxQuota
		input.DefaultQuota = &value
	}
	if input.DefaultTTLSeconds == nil && input.MaxTTLSeconds != nil {
		value := *input.MaxTTLSeconds
		input.DefaultTTLSeconds = &value
	}
	return input
}

func (s *Service) policy(ctx context.Context, scope *workspaceScope) (*accessmodel.Policy, error) {
	var policy accessmodel.Policy
	err := s.db.WithContext(ctx).Where("workspace_id = ?", scope.Workspace.ID).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultPolicy(scope), nil
	}
	if err != nil {
		return nil, fmt.Errorf("load developer access policy: %w", err)
	}
	applySecurePolicyDefaults(&policy)
	return &policy, nil
}

func (s *Service) GetMe(ctx context.Context, workspaceID, accountID string) (*MeView, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	var grant accessmodel.Grant
	grantErr := s.db.WithContext(ctx).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", workspaceID, accessmodel.PrincipalTypeUser, accountID).First(&grant).Error
	if grantErr != nil && !errors.Is(grantErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load developer access grant: %w", grantErr)
	}
	var pending accessmodel.AccessRequest
	pendingErr := s.db.WithContext(ctx).Where("workspace_id = ? AND requester_account_id = ? AND status = ?", workspaceID, accountID, accessmodel.RequestStatusPending).First(&pending).Error
	if pendingErr != nil && !errors.Is(pendingErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load pending access request: %w", pendingErr)
	}
	var count int64
	if err := activePersonalKeysQuery(s.db.WithContext(ctx), s.now()).
		Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", workspaceID, accessmodel.PrincipalTypeUser, accountID).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("count active keys: %w", err)
	}
	view := &MeView{
		WorkspaceID: workspaceID, OrganizationID: *scope.Workspace.OrganizationID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: accountID,
		CanManage: scope.CanManage, Mode: policy.Mode, Policy: policy, ActiveKeyCount: count,
	}
	if scope.Member != nil {
		view.Role = scope.Member.Role
	}
	if grantErr == nil {
		view.Grant = &grant
	}
	if pendingErr == nil {
		view.PendingRequest = &pending
	}
	// Personal keys always belong to a workspace member. A broader organization
	// management entitlement may administer the workspace, but it must not mint
	// a user key that runtime membership validation would immediately reject.
	now := s.now()
	hasUsableGrant := view.Grant != nil && view.Grant.IsActive(now) && grantHasAvailableQuota(view.Grant)
	canStartGrant := view.Grant == nil && (scope.CanManage || policy.Mode == accessmodel.AccessModeSelfService)
	canRenewGrant := view.Grant != nil && grantCanStartNewPeriod(view.Grant, scope, policy, now)
	canFundNewPeriod := quotaHasAvailableBalance(policy.DefaultQuota)
	view.CanCreateKey = scope.Member != nil && policy.Mode != accessmodel.AccessModeDisabled &&
		(hasUsableGrant || (canFundNewPeriod && (canStartGrant || canRenewGrant)))
	view.CanRequestAccess = canRequestDeveloperAccess(scope, policy, view.CanCreateKey, view.PendingRequest != nil)
	return view, nil
}

func canRequestDeveloperAccess(scope *workspaceScope, policy *accessmodel.Policy, canCreateKey, hasPendingRequest bool) bool {
	return scope.Member != nil && !canCreateKey && !hasPendingRequest && policy.Mode == accessmodel.AccessModeApprovalRequired
}

func activePersonalKeysQuery(db *gorm.DB, now time.Time) *gorm.DB {
	return db.Model(&apikeymodel.TenantAPIKey{}).
		Where("status = ?", "active").
		Where("(expires_at IS NULL OR expires_at > ?)", now)
}

func (s *Service) GetPolicy(ctx context.Context, workspaceID, accountID string) (*accessmodel.Policy, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	if !scope.CanManage {
		return nil, ErrForbidden
	}
	return s.policy(ctx, scope)
}

func validatePolicy(input PolicyInput) error {
	if input.Mode != accessmodel.AccessModeSelfService && input.Mode != accessmodel.AccessModeApprovalRequired && input.Mode != accessmodel.AccessModeDisabled {
		return ErrInvalid
	}
	if input.MaxKeys < 1 || input.MaxKeys > 100 {
		return ErrInvalid
	}
	for _, quota := range []*int64{input.DefaultQuota, input.MaxQuota} {
		if quota != nil && *quota < 0 {
			return ErrInvalid
		}
	}
	if input.DefaultQuota != nil && input.MaxQuota != nil && *input.DefaultQuota > *input.MaxQuota {
		return ErrInvalid
	}
	for _, ttl := range []*int64{input.DefaultTTLSeconds, input.MaxTTLSeconds} {
		if !validTTLSeconds(ttl) {
			return ErrInvalid
		}
	}
	if input.DefaultTTLSeconds != nil && input.MaxTTLSeconds != nil && *input.DefaultTTLSeconds > *input.MaxTTLSeconds {
		return ErrInvalid
	}
	return nil
}

func (s *Service) PutPolicy(ctx context.Context, workspaceID, accountID string, input PolicyInput) (*accessmodel.Policy, error) {
	input = applySecurePolicyInputDefaults(input)
	if err := validatePolicy(input); err != nil {
		return nil, err
	}
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	if !scope.CanManage {
		return nil, ErrForbidden
	}
	return s.putPolicy(ctx, scope, accountID, input)
}

func (s *Service) putPolicy(ctx context.Context, scope *workspaceScope, accountID string, input PolicyInput) (*accessmodel.Policy, error) {
	input = applySecurePolicyInputDefaults(input)
	if err := validatePolicy(input); err != nil {
		return nil, err
	}
	var result accessmodel.Policy
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if !lockedScope.CanManage {
			return ErrForbidden
		}
		current, err := policyForUpdate(ctx, tx, lockedScope)
		if err != nil {
			return err
		}
		result = *current
		result.Mode, result.DefaultQuota, result.MaxQuota, result.MaxKeys = input.Mode, input.DefaultQuota, input.MaxQuota, input.MaxKeys
		result.DefaultTTLSeconds, result.MaxTTLSeconds = input.DefaultTTLSeconds, input.MaxTTLSeconds
		result.AllowedModels, result.UpdatedByAccountID = unique(input.AllowedModels), &accountID
		result.Version++
		if result.ID == "" {
			if err := tx.Create(&result).Error; err != nil {
				return fmt.Errorf("create developer access policy: %w", err)
			}
		} else if err := tx.Save(&result).Error; err != nil {
			return fmt.Errorf("update developer access policy: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) ListRequests(ctx context.Context, workspaceID, accountID string, input RequestQuery) (*RequestPage, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	input.Page, input.PageSize, err = normalizePagination(input.Page, input.PageSize)
	if err != nil {
		return nil, err
	}
	input.Scope = strings.TrimSpace(input.Scope)
	input.Status = strings.TrimSpace(input.Status)
	if input.Scope != "" && input.Scope != "all" && input.Scope != "mine" && input.Scope != "members" {
		return nil, ErrInvalid
	}
	if !scope.CanManage && input.Scope == "members" {
		return nil, ErrForbidden
	}
	if input.Status != "" && input.Status != accessmodel.RequestStatusPending && input.Status != accessmodel.RequestStatusApproved &&
		input.Status != accessmodel.RequestStatusRejected && input.Status != accessmodel.RequestStatusCancelled {
		return nil, ErrInvalid
	}

	query := s.db.WithContext(ctx).Model(&accessmodel.AccessRequest{}).Where(
		"workspace_id = ? AND organization_id = ?",
		workspaceID,
		*scope.Workspace.OrganizationID,
	)
	if !scope.CanManage || input.Scope == "mine" {
		query = query.Where("requester_account_id = ?", accountID)
	} else if input.Scope == "members" {
		query = query.Where("requester_account_id <> ?", accountID)
	}

	var pendingTotal int64
	if err := query.Session(&gorm.Session{}).Where("status = ?", accessmodel.RequestStatusPending).Count(&pendingTotal).Error; err != nil {
		return nil, fmt.Errorf("count pending access requests: %w", err)
	}
	if input.Status != "" {
		query = query.Where("status = ?", input.Status)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count access requests: %w", err)
	}
	var items []accessmodel.AccessRequest
	if err := query.
		Order(clause.Expr{SQL: "CASE WHEN status = ? THEN 0 ELSE 1 END", Vars: []interface{}{accessmodel.RequestStatusPending}}).
		Order("created_at DESC").
		Limit(input.PageSize).
		Offset((input.Page - 1) * input.PageSize).
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list access requests: %w", err)
	}
	if err := s.hydrateRequesters(ctx, items); err != nil {
		return nil, err
	}
	return &RequestPage{Items: items, Total: total, PendingTotal: pendingTotal, Page: input.Page, PageSize: input.PageSize}, nil
}

func (s *Service) hydrateRequesters(ctx context.Context, items []accessmodel.AccessRequest) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i := range items {
		if _, ok := seen[items[i].RequesterAccountID]; ok {
			continue
		}
		seen[items[i].RequesterAccountID] = struct{}{}
		ids = append(ids, items[i].RequesterAccountID)
	}
	type requesterIdentity struct {
		ID    string
		Name  string
		Email string
	}
	var identities []requesterIdentity
	if err := s.db.WithContext(ctx).Table("accounts").Select("id", "name", "email").Where("id IN ?", ids).Find(&identities).Error; err != nil {
		return fmt.Errorf("load access request identities: %w", err)
	}
	byID := make(map[string]requesterIdentity, len(identities))
	for _, identity := range identities {
		byID[identity.ID] = identity
	}
	for i := range items {
		identity := byID[items[i].RequesterAccountID]
		items[i].RequesterName = identity.Name
		items[i].RequesterEmail = identity.Email
	}
	return nil
}

func validateEnvironment(environment string) (string, error) {
	if environment == "" {
		return "development", nil
	}
	if environment != "development" && environment != "production" {
		return "", ErrInvalid
	}
	return environment, nil
}

func modelsWithin(requested, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(allowed))
	for _, item := range allowed {
		set[item] = struct{}{}
	}
	for _, item := range requested {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func (s *Service) lockScopeForUpdate(ctx context.Context, tx *gorm.DB, expected *workspaceScope, accountID string) (*workspaceScope, error) {
	var workspace workspacemodel.Workspace
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", expected.Workspace.ID).First(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock developer access workspace: %w", err)
	}
	if !workspace.IsNormal() || workspace.OrganizationID == nil || expected.Workspace.OrganizationID == nil ||
		*workspace.OrganizationID != *expected.Workspace.OrganizationID {
		return nil, ErrConflict
	}
	var organization workspacemodel.Organization
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", *workspace.OrganizationID).First(&organization).Error; err != nil {
		return nil, fmt.Errorf("revalidate developer access organization: %w", err)
	}
	if !organization.IsActive() {
		return nil, ErrForbidden
	}

	var organizationMember workspacemodel.OrganizationMember
	organizationMemberErr := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("organization_id = ? AND account_id = ?", organization.ID, accountID).
		First(&organizationMember).Error
	if organizationMemberErr != nil && !errors.Is(organizationMemberErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("revalidate developer access organization member: %w", organizationMemberErr)
	}

	var member workspacemodel.WorkspaceMember
	memberErr := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND account_id = ?", workspace.ID, accountID).First(&member).Error
	if memberErr != nil && !errors.Is(memberErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("revalidate developer access member: %w", memberErr)
	}
	canManage := organizationMemberErr == nil && organizationMember.Status == workspacemodel.OrganizationMemberStatusActive &&
		(organizationMember.Role == workspacemodel.OrganizationRoleOwner || organizationMember.Role == workspacemodel.OrganizationRoleAdmin)
	if !canManage && memberErr == nil {
		canManage = workspacemodel.WorkspaceMemberAllowsPermission(
			member.Role,
			member.RoleID,
			member.Permissions,
			member.PermissionSource,
			workspacemodel.WorkspacePermissionWorkspaceManage,
		)
	}
	if errors.Is(memberErr, gorm.ErrRecordNotFound) && !canManage {
		return nil, ErrForbidden
	}
	current := &workspaceScope{Workspace: &workspace, CanManage: canManage}
	if memberErr == nil {
		current.Member = &member
	}
	return current, nil
}

func policyForUpdate(ctx context.Context, tx *gorm.DB, scope *workspaceScope) (*accessmodel.Policy, error) {
	var policy accessmodel.Policy
	err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ?", scope.Workspace.ID).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultPolicy(scope), nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock developer access policy: %w", err)
	}
	if scope.Workspace.OrganizationID == nil || policy.OrganizationID != *scope.Workspace.OrganizationID {
		return nil, ErrConflict
	}
	applySecurePolicyDefaults(&policy)
	return &policy, nil
}

func (s *Service) CreateRequest(ctx context.Context, workspaceID, accountID string, input CreateRequestInput) (*accessmodel.AccessRequest, error) {
	input.Purpose = strings.TrimSpace(input.Purpose)
	if input.Purpose == "" || len(input.Purpose) > 2000 {
		return nil, ErrInvalid
	}
	environment, err := validateEnvironment(input.Environment)
	if err != nil {
		return nil, err
	}
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	if scope.Member == nil {
		return nil, ErrForbidden
	}
	return s.createRequest(ctx, scope, accountID, environment, input)
}

func (s *Service) createRequest(ctx context.Context, scope *workspaceScope, accountID, environment string, input CreateRequestInput) (*accessmodel.AccessRequest, error) {
	var request accessmodel.AccessRequest
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if lockedScope.Member == nil || lockedScope.Workspace.OrganizationID == nil {
			return ErrForbidden
		}
		policy, err := policyForUpdate(ctx, tx, lockedScope)
		if err != nil {
			return err
		}
		if policy.Mode == accessmodel.AccessModeDisabled {
			return ErrAccessDisabled
		}
		if policy.Mode == accessmodel.AccessModeSelfService {
			return ErrConflict
		}
		if input.RequestedQuota != nil && (*input.RequestedQuota < 0 || (policy.MaxQuota != nil && *input.RequestedQuota > *policy.MaxQuota)) {
			return ErrInvalid
		}
		if !validTTLSeconds(input.RequestedTTLSeconds) || (input.RequestedTTLSeconds != nil && policy.MaxTTLSeconds != nil && *input.RequestedTTLSeconds > *policy.MaxTTLSeconds) {
			return ErrInvalid
		}
		if !modelsWithin(input.RequestedModels, policy.AllowedModels) {
			return ErrInvalid
		}
		request = accessmodel.AccessRequest{
			OrganizationID: *lockedScope.Workspace.OrganizationID, WorkspaceID: lockedScope.Workspace.ID, RequesterAccountID: accountID,
			Purpose: input.Purpose, Environment: environment, RequestedQuota: input.RequestedQuota,
			RequestedModels: unique(input.RequestedModels), RequestedTTLSeconds: input.RequestedTTLSeconds, Status: accessmodel.RequestStatusPending,
		}
		return tx.Create(&request).Error
	}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create access request: %w", err)
	}
	return &request, nil
}

func (s *Service) CancelRequest(ctx context.Context, workspaceID, accountID, requestID string) (*accessmodel.AccessRequest, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	var request accessmodel.AccessRequest
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if lockedScope.Member == nil {
			return ErrForbidden
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ? AND requester_account_id = ?", requestID, workspaceID, accountID).First(&request).Error; err != nil {
			return err
		}
		if request.Status != accessmodel.RequestStatusPending {
			return ErrConflict
		}
		request.Status = accessmodel.RequestStatusCancelled
		return tx.Save(&request).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (s *Service) ReviewRequest(ctx context.Context, workspaceID, accountID, requestID string, approve bool, input ReviewRequestInput) (*accessmodel.AccessRequest, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	if !scope.CanManage {
		return nil, ErrForbidden
	}
	return s.reviewRequest(ctx, scope, accountID, requestID, approve, input)
}

func (s *Service) reviewRequest(ctx context.Context, scope *workspaceScope, accountID, requestID string, approve bool, input ReviewRequestInput) (*accessmodel.AccessRequest, error) {
	workspaceID := scope.Workspace.ID
	var request accessmodel.AccessRequest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if !lockedScope.CanManage {
			return ErrForbidden
		}
		policy, err := policyForUpdate(ctx, tx, lockedScope)
		if err != nil {
			return err
		}
		if policy.Mode == accessmodel.AccessModeDisabled {
			return ErrAccessDisabled
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ?", requestID, workspaceID).First(&request).Error; err != nil {
			return err
		}
		if request.Status != accessmodel.RequestStatusPending {
			return ErrConflict
		}
		if request.RequesterAccountID == accountID {
			return ErrForbidden
		}
		now := s.now()
		request.ReviewerAccountID, request.ReviewedAt = &accountID, &now
		reason := strings.TrimSpace(input.Reason)
		if reason != "" {
			request.ReviewReason = &reason
		}
		if !approve {
			request.Status = accessmodel.RequestStatusRejected
			return tx.Save(&request).Error
		}
		quota := input.QuotaLimit
		if quota == nil {
			quota = request.RequestedQuota
		}
		if quota == nil {
			quota = policy.DefaultQuota
		}
		if quota != nil && (*quota < 0 || (policy.MaxQuota != nil && *quota > *policy.MaxQuota)) {
			return ErrInvalid
		}
		maxKeys := policy.MaxKeys
		if input.MaxKeys != nil {
			maxKeys = *input.MaxKeys
		}
		if maxKeys < 1 || maxKeys > policy.MaxKeys {
			return ErrInvalid
		}
		models := input.AllowedModels
		if input.AllowedModels == nil {
			models = request.RequestedModels
		}
		if len(models) == 0 {
			models = policy.AllowedModels
		}
		if !modelsWithin(models, policy.AllowedModels) {
			return ErrInvalid
		}
		expiresAt := input.ExpiresAt
		if expiresAt == nil {
			requestedTTL := request.RequestedTTLSeconds
			if requestedTTL == nil {
				requestedTTL = policy.DefaultTTLSeconds
			}
			expiresAt, err = ttlExpiry(now, requestedTTL)
			if err != nil {
				return err
			}
		}
		if expiresAt != nil && !expiresAt.After(now) {
			return ErrInvalid
		}
		if expiresAt != nil && policy.MaxTTLSeconds != nil {
			maximumExpiry, expiryErr := ttlExpiry(now, policy.MaxTTLSeconds)
			if expiryErr != nil || expiresAt.After(*maximumExpiry) {
				return ErrInvalid
			}
		}
		if err := upsertGrant(tx, lockedScope, request.RequesterAccountID, accountID, "approved_request", quota, maxKeys, models, expiresAt, now); err != nil {
			return err
		}
		if err := revokePrincipalKeys(tx, workspaceID, request.RequesterAccountID, accountID, "grant_updated", now); err != nil {
			return err
		}
		request.Status = accessmodel.RequestStatusApproved
		return tx.Save(&request).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func upsertGrant(tx *gorm.DB, scope *workspaceScope, principalID, actorID, source string, quota *int64, maxKeys int, models []string, expiresAt *time.Time, now time.Time) error {
	var grant accessmodel.Grant
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", scope.Workspace.ID, accessmodel.PrincipalTypeUser, principalID).First(&grant).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		grant = accessmodel.Grant{OrganizationID: *scope.Workspace.OrganizationID, WorkspaceID: scope.Workspace.ID, PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID, Source: source, Status: accessmodel.GrantStatusActive, MaxKeys: maxKeys, AllowedModels: unique(models), ExpiresAt: expiresAt, CreatedByAccountID: &actorID, UpdatedByAccountID: &actorID, AuthorizationVersion: 1}
		applyGrantLimits(&grant, quota, true)
		created, err := createGrantOrReloadForUpdate(tx, &grant)
		if err != nil {
			return err
		}
		if created {
			return nil
		}
		// Approval is authoritative. If self-service creation won the insert
		// race, apply the reviewer's narrower limits to the locked winner.
	}
	startsNewPeriod := !grant.IsActive(now)
	grant.Source, grant.Status, grant.MaxKeys, grant.AllowedModels, grant.ExpiresAt = source, accessmodel.GrantStatusActive, maxKeys, unique(models), expiresAt
	grant.UpdatedByAccountID = &actorID
	grant.AuthorizationVersion++
	applyGrantLimits(&grant, quota, startsNewPeriod)
	return tx.Save(&grant).Error
}

func applyGrantLimits(grant *accessmodel.Grant, quota *int64, startsNewPeriod bool) {
	if startsNewPeriod {
		grant.UsedQuota = 0
	}
	grant.QuotaLimit = quota
	if quota == nil {
		grant.RemainQuota = 0
	} else if *quota > grant.UsedQuota {
		grant.RemainQuota = *quota - grant.UsedQuota
	} else {
		grant.RemainQuota = 0
	}
}

func grantHasAvailableQuota(grant *accessmodel.Grant) bool {
	return grant != nil && (grant.QuotaLimit == nil || grant.RemainQuota > 0)
}

func quotaHasAvailableBalance(quota *int64) bool {
	return quota == nil || *quota > 0
}

func grantCanStartNewPeriod(grant *accessmodel.Grant, scope *workspaceScope, policy *accessmodel.Policy, now time.Time) bool {
	if grant == nil {
		return false
	}
	// Revoked grants currently represent membership removal. A member who has
	// rejoined a self-service workspace, or a rejoined workspace manager, may
	// start a fresh authorization period; the version bump below keeps every
	// key from the old membership revoked.
	if grant.Status == accessmodel.GrantStatusRevoked {
		return policy.Mode == accessmodel.AccessModeSelfService || scope.CanManage
	}
	return grant.Status == accessmodel.GrantStatusActive &&
		grant.ExpiresAt != nil && !grant.ExpiresAt.After(now) &&
		(policy.Mode == accessmodel.AccessModeSelfService || scope.CanManage)
}

func createGrantOrReloadForUpdate(tx *gorm.DB, grant *accessmodel.Grant) (bool, error) {
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(grant)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil
	}
	// A concurrent first-key or approval request may have inserted the same
	// principal grant after our initial lookup. Lock and reuse that winning row
	// instead of exposing a duplicate-key failure to the caller.
	var winner accessmodel.Grant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", grant.WorkspaceID, grant.PrincipalType, grant.PrincipalID).
		First(&winner).Error; err != nil {
		return false, err
	}
	*grant = winner
	return false, nil
}

func createGrantIfAbsent(tx *gorm.DB, scope *workspaceScope, principalID, actorID, source string, quota *int64, maxKeys int, models []string, expiresAt *time.Time) error {
	grant := accessmodel.Grant{
		OrganizationID: *scope.Workspace.OrganizationID, WorkspaceID: scope.Workspace.ID,
		PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID,
		Source: source, Status: accessmodel.GrantStatusActive, MaxKeys: maxKeys,
		AllowedModels: unique(models), ExpiresAt: expiresAt,
		CreatedByAccountID: &actorID, UpdatedByAccountID: &actorID, AuthorizationVersion: 1,
	}
	applyGrantLimits(&grant, quota, true)
	_, err := createGrantOrReloadForUpdate(tx, &grant)
	return err
}

func revokePrincipalKeys(tx *gorm.DB, workspaceID, principalID, actorID, reason string, now time.Time) error {
	return tx.Model(&apikeymodel.TenantAPIKey{}).
		Where("workspace_id = ? AND principal_type = ? AND principal_id = ? AND status <> ?", workspaceID, accessmodel.PrincipalTypeUser, principalID, "revoked").
		Updates(map[string]interface{}{
			"status":                "revoked",
			"revoked_at":            now,
			"revoked_by_account_id": actorID,
			"revoked_reason":        reason,
			"updated_at":            now,
		}).Error
}

func (s *Service) ensureGrant(ctx context.Context, scope *workspaceScope, accountID string) (*accessmodel.Grant, error) {
	var grant accessmodel.Grant
	err := s.db.WithContext(ctx).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", scope.Workspace.ID, accessmodel.PrincipalTypeUser, accountID).First(&grant).Error
	if err == nil {
		if grant.IsActive(s.now()) {
			return &grant, nil
		}
		// Expiry ends one budget period. Renew an eligible self-service or
		// workspace-manager principal atomically, reset period usage, and bump
		// authorization so old keys remain stale until a replacement is created.
		err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
			if err != nil {
				return err
			}
			if lockedScope.Member == nil {
				return ErrForbidden
			}
			policy, err := policyForUpdate(ctx, tx, lockedScope)
			if err != nil {
				return err
			}
			var locked accessmodel.Grant
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", grant.ID).First(&locked).Error; err != nil {
				return err
			}
			now := s.now()
			if locked.IsActive(now) {
				grant = locked
				return nil
			}
			if policy.Mode == accessmodel.AccessModeDisabled {
				return ErrAccessDisabled
			}
			if !grantCanStartNewPeriod(&locked, lockedScope, policy, now) {
				return ErrApprovalNeeded
			}
			if !quotaHasAvailableBalance(policy.DefaultQuota) {
				return ErrQuotaExceeded
			}
			source := "self_service"
			if lockedScope.CanManage && policy.Mode != accessmodel.AccessModeSelfService {
				source = "workspace_admin"
			}
			locked.Source, locked.Status = source, accessmodel.GrantStatusActive
			applyGrantLimits(&locked, policy.DefaultQuota, true)
			locked.MaxKeys = policy.MaxKeys
			locked.AllowedModels = unique(policy.AllowedModels)
			expiresAt, err := ttlExpiry(now, policy.DefaultTTLSeconds)
			if err != nil {
				return err
			}
			locked.ExpiresAt = expiresAt
			locked.UpdatedByAccountID = &accountID
			locked.AuthorizationVersion++
			if err := tx.Save(&locked).Error; err != nil {
				return err
			}
			grant = locked
			return revokePrincipalKeys(tx, lockedScope.Workspace.ID, accountID, accountID, "grant_renewed", now)
		})
		if err != nil {
			return nil, fmt.Errorf("renew developer access grant: %w", err)
		}
		return &grant, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load developer access grant: %w", err)
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if lockedScope.Member == nil {
			return ErrForbidden
		}
		policy, err := policyForUpdate(ctx, tx, lockedScope)
		if err != nil {
			return err
		}
		if policy.Mode == accessmodel.AccessModeDisabled {
			return ErrAccessDisabled
		}
		if !lockedScope.CanManage && policy.Mode != accessmodel.AccessModeSelfService {
			return ErrApprovalNeeded
		}
		if !quotaHasAvailableBalance(policy.DefaultQuota) {
			return ErrQuotaExceeded
		}
		source := "self_service"
		if lockedScope.CanManage && policy.Mode != accessmodel.AccessModeSelfService {
			source = "workspace_admin"
		}
		now := s.now()
		expiresAt, err := ttlExpiry(now, policy.DefaultTTLSeconds)
		if err != nil {
			return err
		}
		return createGrantIfAbsent(tx, lockedScope, accountID, accountID, source, policy.DefaultQuota, policy.MaxKeys, policy.AllowedModels, expiresAt)
	})
	if err != nil {
		return nil, fmt.Errorf("create self-service grant: %w", err)
	}
	if err := s.db.WithContext(ctx).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", scope.Workspace.ID, accessmodel.PrincipalTypeUser, accountID).First(&grant).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

func validTTLSeconds(seconds *int64) bool {
	return seconds == nil || (*seconds > 0 && *seconds <= maxDeveloperAccessTTLSeconds)
}

func ttlExpiry(now time.Time, seconds *int64) (*time.Time, error) {
	if seconds == nil {
		return nil, nil
	}
	if !validTTLSeconds(seconds) {
		return nil, ErrInvalid
	}
	value := now.Add(time.Duration(*seconds) * time.Second)
	return &value, nil
}

func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "zgi_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Service) CreateKey(ctx context.Context, workspaceID, accountID string, input CreateKeyInput) (*CreatedKey, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 255 {
		return nil, ErrInvalid
	}
	environment, err := validateEnvironment(input.Environment)
	if err != nil {
		return nil, err
	}
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	if scope.Member == nil {
		return nil, ErrForbidden
	}
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	if policy.Mode == accessmodel.AccessModeDisabled {
		return nil, ErrAccessDisabled
	}
	grant, err := s.ensureGrant(ctx, scope, accountID)
	if err != nil {
		return nil, err
	}
	if !grantHasAvailableQuota(grant) {
		return nil, ErrQuotaExceeded
	}
	if !modelsWithin(input.ModelNames, grant.AllowedModels) {
		return nil, ErrInvalid
	}
	models := unique(input.ModelNames)
	if len(models) == 0 {
		models = grant.AllowedModels
	}
	now := s.now()
	expiresAt := input.ExpiresAt
	if expiresAt != nil && !expiresAt.After(now) {
		return nil, ErrInvalid
	}
	if grant.ExpiresAt != nil && (expiresAt == nil || expiresAt.After(*grant.ExpiresAt)) {
		expiresAt = grant.ExpiresAt
	}
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate API key: %w", err)
	}
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		return nil, fmt.Errorf("encode model scope: %w", err)
	}
	modelsString := string(modelsJSON)
	principalType := accessmodel.PrincipalTypeUser
	key := &apikeymodel.TenantAPIKey{
		OrganizationID: *scope.Workspace.OrganizationID, WorkspaceID: &workspaceID,
		PrincipalType: &principalType, PrincipalID: &accountID, AccessGrantID: &grant.ID, CreatedByID: &accountID,
		Key: "", KeyHash: util.HashAPIKey(secret), KeyPrefix: secret[:8], KeySuffix: secret[len(secret)-4:], SecretVersion: 2,
		Name: input.Name, Status: "active", Environment: environment, ExpiresAt: expiresAt, QuotaLimit: nil,
		ModelLimitsEnabled: len(models) > 0, ModelLimits: &modelsString, AllowIPs: "",
		AuthorizationVersion: grant.AuthorizationVersion,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if lockedScope.Member == nil {
			return ErrForbidden
		}
		lockedPolicy, err := policyForUpdate(ctx, tx, lockedScope)
		if err != nil {
			return err
		}
		if lockedPolicy.Mode == accessmodel.AccessModeDisabled {
			return ErrAccessDisabled
		}
		var lockedGrant accessmodel.Grant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", grant.ID).First(&lockedGrant).Error; err != nil {
			return err
		}
		if !lockedGrant.IsActive(s.now()) || lockedGrant.AuthorizationVersion != grant.AuthorizationVersion {
			return ErrApprovalNeeded
		}
		if !grantHasAvailableQuota(&lockedGrant) {
			return ErrQuotaExceeded
		}
		var active int64
		if err := activePersonalKeysQuery(tx, s.now()).
			Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", workspaceID, principalType, accountID).
			Count(&active).Error; err != nil {
			return err
		}
		if active >= int64(lockedGrant.MaxKeys) {
			return ErrConflict
		}
		// Personal keys are hash-only. Omit the legacy plaintext column so
		// PostgreSQL stores NULL; an empty string would collide with the
		// existing partial unique index after the first personal key.
		return tx.Omit("Key").Create(key).Error
	})
	if err != nil {
		return nil, err
	}
	view := keyToView(key)
	return &CreatedKey{KeyView: view, Secret: secret}, nil
}

func keyToView(key *apikeymodel.TenantAPIKey) KeyView {
	models := []string{}
	if key.ModelLimits != nil {
		_ = json.Unmarshal([]byte(*key.ModelLimits), &models)
	}
	principal := ""
	if key.PrincipalID != nil {
		principal = *key.PrincipalID
	}
	masked := "****"
	if key.KeyPrefix != "" || key.KeySuffix != "" {
		masked = key.KeyPrefix + "••••••••" + key.KeySuffix
	}
	return KeyView{ID: key.ID, Name: key.Name, Status: key.Status, KeyMasked: masked, PrincipalID: principal, Environment: key.Environment, ModelNames: models, CreatedAt: key.CreatedAt, AccessedAt: key.AccessedAt, ExpiresAt: key.ExpiresAt, RevokedAt: key.RevokedAt}
}

func normalizePagination(page, pageSize int) (int, int, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if page > maxDeveloperAccessPage || pageSize > 100 {
		return 0, 0, ErrInvalid
	}
	return page, pageSize, nil
}

func (s *Service) ListKeys(ctx context.Context, workspaceID, accountID string, input KeyQuery) (*KeyPage, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	input.Page, input.PageSize, err = normalizePagination(input.Page, input.PageSize)
	if err != nil {
		return nil, err
	}
	input.Scope = strings.TrimSpace(input.Scope)
	if input.Scope != "" && input.Scope != "all" && input.Scope != "mine" && input.Scope != "members" {
		return nil, ErrInvalid
	}
	if !scope.CanManage && input.Scope == "members" {
		return nil, ErrForbidden
	}
	query := s.db.WithContext(ctx).Where(
		"workspace_id = ? AND organization_id = ? AND principal_type = ?",
		workspaceID,
		*scope.Workspace.OrganizationID,
		accessmodel.PrincipalTypeUser,
	)
	if !scope.CanManage || input.Scope == "mine" {
		query = query.Where("principal_id = ?", accountID)
	} else if input.Scope == "members" {
		query = query.Where("principal_id <> ?", accountID)
	}
	var total int64
	if err := query.Model(&apikeymodel.TenantAPIKey{}).Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count personal API keys: %w", err)
	}
	var rows []apikeymodel.TenantAPIKey
	if err := query.
		Order(clause.Expr{SQL: "CASE WHEN status <> 'revoked' AND (expires_at IS NULL OR expires_at > ?) THEN 0 ELSE 1 END", Vars: []interface{}{s.now()}}).
		Order("created_at DESC").
		Limit(input.PageSize).
		Offset((input.Page - 1) * input.PageSize).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list personal API keys: %w", err)
	}
	views := make([]KeyView, 0, len(rows))
	for i := range rows {
		views = append(views, keyToView(&rows[i]))
	}
	if err := s.hydrateKeyPrincipals(ctx, views); err != nil {
		return nil, err
	}
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := s.hydrateKeyCapabilities(ctx, policy, accountID, rows, views); err != nil {
		return nil, err
	}
	return &KeyPage{Items: views, Total: total, Page: input.Page, PageSize: input.PageSize}, nil
}

func (s *Service) hydrateKeyCapabilities(ctx context.Context, policy *accessmodel.Policy, accountID string, keys []apikeymodel.TenantAPIKey, views []KeyView) error {
	if policy == nil || policy.Mode == accessmodel.AccessModeDisabled || len(keys) == 0 {
		return nil
	}
	now := s.now()
	grantIDs := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for i := range keys {
		key := &keys[i]
		if key.Status == "revoked" || key.AccessGrantID == nil || (key.ExpiresAt != nil && !key.ExpiresAt.After(now)) {
			continue
		}
		if _, ok := seen[*key.AccessGrantID]; ok {
			continue
		}
		seen[*key.AccessGrantID] = struct{}{}
		grantIDs = append(grantIDs, *key.AccessGrantID)
	}
	if len(grantIDs) == 0 {
		return nil
	}
	var grants []accessmodel.Grant
	if err := s.db.WithContext(ctx).Where("id IN ?", grantIDs).Find(&grants).Error; err != nil {
		return fmt.Errorf("load API key grants: %w", err)
	}
	grantsByID := make(map[string]accessmodel.Grant, len(grants))
	for i := range grants {
		grantsByID[grants[i].ID] = grants[i]
	}
	type grantKeyCount struct {
		GrantID string `gorm:"column:grant_id"`
		Count   int64  `gorm:"column:key_count"`
	}
	var counts []grantKeyCount
	if err := activePersonalKeysQuery(s.db.WithContext(ctx), now).
		Select("access_grant_id AS grant_id, COUNT(*) AS key_count").
		Where("access_grant_id IN ?", grantIDs).
		Group("access_grant_id").
		Scan(&counts).Error; err != nil {
		return fmt.Errorf("count active grant API keys: %w", err)
	}
	activeByGrant := make(map[string]int64, len(counts))
	for _, item := range counts {
		activeByGrant[item.GrantID] = item.Count
	}
	for i := range keys {
		key := &keys[i]
		if key.Status == "revoked" || key.AccessGrantID == nil || (key.ExpiresAt != nil && !key.ExpiresAt.After(now)) {
			continue
		}
		grant, ok := grantsByID[*key.AccessGrantID]
		if !ok || !grant.IsActive(now) || grant.AuthorizationVersion != key.AuthorizationVersion || !grantHasAvailableQuota(&grant) {
			continue
		}
		active := activeByGrant[grant.ID]
		if key.Status == "inactive" {
			views[i].CanActivate = active < int64(grant.MaxKeys)
		}
		if key.PrincipalID != nil && *key.PrincipalID == accountID {
			otherActive := active
			if key.Status == "active" {
				otherActive--
			}
			views[i].CanRotate = otherActive < int64(grant.MaxKeys)
		}
	}
	return nil
}

func (s *Service) hydrateKeyPrincipals(ctx context.Context, items []KeyView) error {
	ids := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.PrincipalID == "" {
			continue
		}
		if _, ok := seen[item.PrincipalID]; ok {
			continue
		}
		seen[item.PrincipalID] = struct{}{}
		ids = append(ids, item.PrincipalID)
	}
	if len(ids) == 0 {
		return nil
	}
	type identity struct{ ID, Name, Email string }
	var identities []identity
	if err := s.db.WithContext(ctx).Table("accounts").Select("id", "name", "email").Where("id IN ?", ids).Scan(&identities).Error; err != nil {
		return fmt.Errorf("load API key principals: %w", err)
	}
	byID := make(map[string]identity, len(identities))
	for _, item := range identities {
		byID[item.ID] = item
	}
	for i := range items {
		items[i].PrincipalName = byID[items[i].PrincipalID].Name
		items[i].PrincipalEmail = byID[items[i].PrincipalID].Email
	}
	return nil
}

func (s *Service) ListAudit(ctx context.Context, workspaceID, accountID string, input AuditQuery) (*AuditPage, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	input.Page, input.PageSize, err = normalizePagination(input.Page, input.PageSize)
	if err != nil {
		return nil, err
	}
	if input.StartTime < 0 || input.EndTime < 0 || (input.StartTime > 0 && input.EndTime > 0 && input.EndTime < input.StartTime) {
		return nil, ErrInvalid
	}
	if input.Status != "" && input.Status != "success" && input.Status != "failed" && input.Status != "partial" {
		return nil, ErrInvalid
	}

	query := s.db.WithContext(ctx).Table("llm_usage_bills").
		Where(
			"workspace_id = ? AND organization_id = ? AND principal_type = ? AND auth_method = ?",
			workspaceID,
			*scope.Workspace.OrganizationID,
			accessmodel.PrincipalTypeUser,
			"personal_api_key",
		)
	if !scope.CanManage {
		query = query.Where("principal_id = ?", accountID)
	} else if principalID := strings.TrimSpace(input.PrincipalID); principalID != "" {
		query = query.Where("principal_id = ?", principalID)
	}
	if keyID := strings.TrimSpace(input.APIKeyID); keyID != "" {
		query = query.Where("api_key_id = ?", keyID)
	}
	if modelName := strings.TrimSpace(input.ModelName); modelName != "" {
		query = query.Where("model_name = ?", modelName)
	}
	if input.Status != "" {
		query = query.Where("status = ?", input.Status)
	}
	if input.StartTime > 0 {
		query = query.Where("request_created_at >= ?", time.Unix(input.StartTime, 0).UTC())
	}
	if input.EndTime > 0 {
		query = query.Where("request_created_at < ?", time.Unix(input.EndTime, 0).UTC().Add(time.Second))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count developer access audit: %w", err)
	}
	items := make([]AuditItem, 0, input.PageSize)
	if err := query.Select(`attempt_id, request_id, principal_id, api_key_id, model_name, provider_name,
		status, prompt_tokens, completion_tokens, total_tokens, total_points, response_time_ms,
		error_code, request_created_at AS created_at`).
		Order("request_created_at DESC").
		Limit(input.PageSize).
		Offset((input.Page - 1) * input.PageSize).
		Scan(&items).Error; err != nil {
		return nil, fmt.Errorf("list developer access audit: %w", err)
	}

	identities := make([]KeyView, 0, len(items))
	for _, item := range items {
		identities = append(identities, KeyView{PrincipalID: item.PrincipalID})
	}
	if err := s.hydrateKeyPrincipals(ctx, identities); err != nil {
		return nil, err
	}
	identityByID := make(map[string]KeyView, len(identities))
	for _, item := range identities {
		identityByID[item.PrincipalID] = item
	}
	keyIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.APIKeyID != "" {
			keyIDs = append(keyIDs, item.APIKeyID)
		}
	}
	var keys []apikeymodel.TenantAPIKey
	if len(keyIDs) > 0 {
		if err := s.db.WithContext(ctx).Unscoped().Where("id IN ?", unique(keyIDs)).Find(&keys).Error; err != nil {
			return nil, fmt.Errorf("load audited API keys: %w", err)
		}
	}
	keysByID := make(map[string]KeyView, len(keys))
	for i := range keys {
		keysByID[keys[i].ID] = keyToView(&keys[i])
	}
	type subjectCharge struct {
		AttemptID      string `gorm:"column:attempt_id"`
		ReservedAmount int64  `gorm:"column:reserved_amount"`
		ActualAmount   int64  `gorm:"column:actual_amount"`
		RefundedAmount int64  `gorm:"column:refunded_amount"`
	}
	attemptIDs := make([]string, 0, len(items))
	for _, item := range items {
		if item.AttemptID != "" {
			attemptIDs = append(attemptIDs, item.AttemptID)
		}
	}
	chargesByAttempt := make(map[string]int64, len(attemptIDs))
	if len(attemptIDs) > 0 {
		var charges []subjectCharge
		if err := s.db.WithContext(ctx).Table("billing_attempt_entries").
			Select("attempt_id", "reserved_amount", "actual_amount", "refunded_amount").
			Where("attempt_id IN ? AND entry_type = ?", unique(attemptIDs), "subject").
			Scan(&charges).Error; err != nil {
			return nil, fmt.Errorf("load developer access quota charges: %w", err)
		}
		for _, charge := range charges {
			chargesByAttempt[charge.AttemptID] = netSubjectCharge(charge.ReservedAmount, charge.ActualAmount, charge.RefundedAmount)
		}
	}
	for i := range items {
		identity := identityByID[items[i].PrincipalID]
		key := keysByID[items[i].APIKeyID]
		items[i].PrincipalName, items[i].PrincipalEmail = identity.PrincipalName, identity.PrincipalEmail
		items[i].APIKeyName, items[i].APIKeyMasked = key.Name, key.KeyMasked
		items[i].QuotaChargedPoints = items[i].TotalPoints
		if charged, ok := chargesByAttempt[items[i].AttemptID]; ok {
			items[i].QuotaChargedPoints = charged
		}
		if overage := items[i].TotalPoints - items[i].QuotaChargedPoints; overage > 0 {
			items[i].QuotaOveragePoints = overage
		}
	}
	return &AuditPage{Items: items, Total: total, Page: input.Page, PageSize: input.PageSize}, nil
}

func netSubjectCharge(reserved, actual, refunded int64) int64 {
	unusedReservationRefund := reserved - actual
	if unusedReservationRefund < 0 {
		unusedReservationRefund = 0
	}
	compensatedRefund := refunded - unusedReservationRefund
	if compensatedRefund < 0 {
		compensatedRefund = 0
	}
	net := actual - compensatedRefund
	if net < 0 {
		return 0
	}
	return net
}

func personalKeyForUpdate(ctx context.Context, tx *gorm.DB, scope *workspaceScope, accountID, keyID string) (*apikeymodel.TenantAPIKey, error) {
	var key apikeymodel.TenantAPIKey
	query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"id = ? AND workspace_id = ? AND organization_id = ? AND principal_type = ?",
			keyID,
			scope.Workspace.ID,
			*scope.Workspace.OrganizationID,
			accessmodel.PrincipalTypeUser,
		)
	if !scope.CanManage {
		query = query.Where("principal_id = ?", accountID)
	}
	if err := query.First(&key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &key, nil
}

func (s *Service) UpdateKey(ctx context.Context, workspaceID, accountID, keyID string, input UpdateKeyInput) (*KeyView, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 255 {
		return nil, ErrInvalid
	}
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	var key apikeymodel.TenantAPIKey
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		current, err := personalKeyForUpdate(ctx, tx, lockedScope, accountID, keyID)
		if err != nil {
			return err
		}
		key = *current
		key.Name = input.Name
		// Update only the mutable field. Loading a NULL legacy plaintext key into
		// the string model yields ""; saving the whole row would write that value
		// back and can collide with the legacy partial unique index.
		return tx.WithContext(ctx).Model(&apikeymodel.TenantAPIKey{}).
			Where("id = ? AND organization_id = ?", key.ID, key.OrganizationID).
			Update("name", input.Name).Error
	}); err != nil {
		return nil, err
	}
	if invalidator, ok := s.keys.(interface {
		InvalidateKeyCache(context.Context, string)
	}); ok {
		invalidator.InvalidateKeyCache(ctx, key.KeyHash)
	}
	view := keyToView(&key)
	return &view, nil
}

func (s *Service) SetKeyStatus(ctx context.Context, workspaceID, accountID, keyID, status, reason string) (*KeyView, error) {
	if status != "active" && status != "inactive" && status != "revoked" {
		return nil, ErrInvalid
	}
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	return s.setKeyStatus(ctx, scope, accountID, keyID, status, reason)
}

func (s *Service) setKeyStatus(ctx context.Context, scope *workspaceScope, accountID, keyID, status, reason string) (*KeyView, error) {
	if status == "active" {
		policy, err := s.policy(ctx, scope)
		if err != nil {
			return nil, err
		}
		if policy.Mode == accessmodel.AccessModeDisabled {
			return nil, ErrAccessDisabled
		}
	}
	var candidate apikeymodel.TenantAPIKey
	if status == "active" {
		query := s.db.WithContext(ctx).Where(
			"id = ? AND workspace_id = ? AND organization_id = ? AND principal_type = ?",
			keyID,
			scope.Workspace.ID,
			*scope.Workspace.OrganizationID,
			accessmodel.PrincipalTypeUser,
		)
		if !scope.CanManage {
			query = query.Where("principal_id = ?", accountID)
		}
		if err := query.First(&candidate).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if candidate.AccessGrantID == nil {
			return nil, ErrApprovalNeeded
		}
	}
	var key apikeymodel.TenantAPIKey
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		var grant accessmodel.Grant
		// Approval updates lock the grant before revoking its keys. Activation
		// must use the same order to avoid a grant/key deadlock cycle.
		if status == "active" {
			policy, err := policyForUpdate(ctx, tx, lockedScope)
			if err != nil {
				return err
			}
			if policy.Mode == accessmodel.AccessModeDisabled {
				return ErrAccessDisabled
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", *candidate.AccessGrantID).First(&grant).Error; err != nil {
				return ErrApprovalNeeded
			}
		}
		current, err := personalKeyForUpdate(ctx, tx, lockedScope, accountID, keyID)
		if err != nil {
			return err
		}
		key = *current
		if key.Status == "revoked" {
			return ErrConflict
		}
		if status == "active" {
			if key.ExpiresAt != nil && !key.ExpiresAt.After(s.now()) {
				return ErrConflict
			}
			if key.AccessGrantID == nil || *key.AccessGrantID != grant.ID {
				return ErrApprovalNeeded
			}
			if !grant.IsActive(s.now()) || grant.AuthorizationVersion != key.AuthorizationVersion {
				return ErrApprovalNeeded
			}
			if !grantHasAvailableQuota(&grant) {
				return ErrQuotaExceeded
			}
			var active int64
			if err := activePersonalKeysQuery(tx, s.now()).
				Where("access_grant_id = ? AND id <> ?", grant.ID, key.ID).
				Count(&active).Error; err != nil {
				return err
			}
			if active >= int64(grant.MaxKeys) {
				return ErrConflict
			}
		}
		key.Status = status
		if status == "revoked" {
			now := s.now()
			reason = strings.TrimSpace(reason)
			key.RevokedAt, key.RevokedByID = &now, &accountID
			if reason != "" {
				key.RevokedReason = &reason
			}
		}
		return tx.Omit("Key").Save(&key).Error
	})
	if err != nil {
		return nil, err
	}
	if invalidator, ok := s.keys.(interface {
		InvalidateKeyCache(context.Context, string)
	}); ok {
		invalidator.InvalidateKeyCache(ctx, key.KeyHash)
	}
	view := keyToView(&key)
	return &view, nil
}

func (s *Service) RotateKey(ctx context.Context, workspaceID, accountID, keyID string, input RotateKeyInput) (*CreatedKey, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	if scope.Member == nil {
		return nil, ErrForbidden
	}
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	if policy.Mode == accessmodel.AccessModeDisabled {
		return nil, ErrAccessDisabled
	}
	var candidate apikeymodel.TenantAPIKey
	if err := s.db.WithContext(ctx).
		Where("id = ? AND workspace_id = ? AND organization_id = ? AND principal_type = ? AND principal_id = ?", keyID, workspaceID, *scope.Workspace.OrganizationID, accessmodel.PrincipalTypeUser, accountID).
		First(&candidate).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if candidate.Status == "revoked" || candidate.AccessGrantID == nil {
		return nil, ErrConflict
	}
	secret, err := generateSecret()
	if err != nil {
		return nil, fmt.Errorf("generate rotated API key: %w", err)
	}
	var replacement apikeymodel.TenantAPIKey
	var oldHash string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockedScope, err := s.lockScopeForUpdate(ctx, tx, scope, accountID)
		if err != nil {
			return err
		}
		if lockedScope.Member == nil {
			return ErrForbidden
		}
		lockedPolicy, err := policyForUpdate(ctx, tx, lockedScope)
		if err != nil {
			return err
		}
		if lockedPolicy.Mode == accessmodel.AccessModeDisabled {
			return ErrAccessDisabled
		}
		// ReviewRequest locks the grant and then revokes its keys. Rotation uses
		// the same grant-before-key order so the two paths cannot deadlock.
		var grant accessmodel.Grant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND organization_id = ?", *candidate.AccessGrantID, *lockedScope.Workspace.OrganizationID).
			First(&grant).Error; err != nil {
			return err
		}
		var current apikeymodel.TenantAPIKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND workspace_id = ? AND organization_id = ? AND principal_type = ? AND principal_id = ?", keyID, workspaceID, *lockedScope.Workspace.OrganizationID, accessmodel.PrincipalTypeUser, accountID).
			First(&current).Error; err != nil {
			return err
		}
		if current.Status == "revoked" || current.AccessGrantID == nil || *current.AccessGrantID != grant.ID {
			return ErrConflict
		}
		if !grant.IsActive(s.now()) || grant.AuthorizationVersion != current.AuthorizationVersion {
			return ErrApprovalNeeded
		}
		if !grantHasAvailableQuota(&grant) {
			return ErrQuotaExceeded
		}
		expiresAt := input.ExpiresAt
		if expiresAt == nil {
			expiresAt = current.ExpiresAt
		}
		if expiresAt != nil && !expiresAt.After(s.now()) {
			return ErrInvalid
		}
		if grant.ExpiresAt != nil && (expiresAt == nil || expiresAt.After(*grant.ExpiresAt)) {
			expiresAt = grant.ExpiresAt
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = current.Name
		}
		if len(name) > 255 {
			return ErrInvalid
		}
		var active int64
		if err := activePersonalKeysQuery(tx, s.now()).
			Where("access_grant_id = ? AND id <> ?", grant.ID, current.ID).
			Count(&active).Error; err != nil {
			return err
		}
		if active >= int64(grant.MaxKeys) {
			return ErrConflict
		}
		now := s.now()
		oldHash = current.KeyHash
		current.Status = "revoked"
		current.RevokedAt, current.RevokedByID = &now, &accountID
		reason := "rotated"
		current.RevokedReason = &reason
		if err := tx.Omit("Key").Save(&current).Error; err != nil {
			return err
		}
		principalType := accessmodel.PrincipalTypeUser
		replacement = apikeymodel.TenantAPIKey{
			OrganizationID: current.OrganizationID, WorkspaceID: &workspaceID,
			PrincipalType: &principalType, PrincipalID: &accountID, AccessGrantID: &grant.ID, CreatedByID: &accountID,
			Key: "", KeyHash: util.HashAPIKey(secret), KeyPrefix: secret[:8], KeySuffix: secret[len(secret)-4:], SecretVersion: 2,
			Name: name, Status: "active", Environment: current.Environment, ExpiresAt: expiresAt,
			ModelLimitsEnabled: current.ModelLimitsEnabled, ModelLimits: current.ModelLimits, AllowIPs: current.AllowIPs,
			AuthorizationVersion: grant.AuthorizationVersion, RotatedFromID: &current.ID,
		}
		return tx.Omit("Key").Create(&replacement).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if invalidator, ok := s.keys.(interface {
		InvalidateKeyCache(context.Context, string)
	}); ok {
		invalidator.InvalidateKeyCache(ctx, oldHash)
	}
	view := keyToView(&replacement)
	return &CreatedKey{KeyView: view, Secret: secret}, nil
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

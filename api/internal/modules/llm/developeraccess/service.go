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
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspacerepo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/internal/util"
)

var (
	ErrForbidden      = errors.New("developer access forbidden")
	ErrNotFound       = errors.New("developer access resource not found")
	ErrInvalid        = errors.New("invalid developer access request")
	ErrAccessDisabled = errors.New("developer access is disabled")
	ErrApprovalNeeded = errors.New("developer access approval is required")
	ErrConflict       = errors.New("developer access state conflict")
	ErrQuotaExceeded  = errors.New("developer access quota exceeded")
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

type KeyView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	KeyMasked   string     `json:"key_masked"`
	PrincipalID string     `json:"principal_id"`
	Environment string     `json:"environment"`
	ModelNames  []string   `json:"model_names"`
	CreatedAt   time.Time  `json:"created_at"`
	AccessedAt  *time.Time `json:"accessed_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
}

type CreatedKey struct {
	KeyView
	Secret string `json:"secret"`
}

type MeView struct {
	WorkspaceID    string                             `json:"workspace_id"`
	OrganizationID string                             `json:"organization_id"`
	PrincipalType  string                             `json:"principal_type"`
	PrincipalID    string                             `json:"principal_id"`
	Role           workspacemodel.WorkspaceMemberRole `json:"role"`
	CanManage      bool                               `json:"can_manage"`
	CanCreateKey   bool                               `json:"can_create_key"`
	Mode           string                             `json:"mode"`
	Policy         *accessmodel.Policy                `json:"policy"`
	Grant          *accessmodel.Grant                 `json:"grant,omitempty"`
	PendingRequest *accessmodel.AccessRequest         `json:"pending_request,omitempty"`
	ActiveKeyCount int64                              `json:"active_key_count"`
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

func (s *Service) policy(ctx context.Context, scope *workspaceScope) (*accessmodel.Policy, error) {
	var policy accessmodel.Policy
	err := s.db.WithContext(ctx).Where("workspace_id = ?", scope.Workspace.ID).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultPolicy(scope), nil
	}
	if err != nil {
		return nil, fmt.Errorf("load developer access policy: %w", err)
	}
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
	if err := s.db.WithContext(ctx).Model(&apikeymodel.TenantAPIKey{}).Where("workspace_id = ? AND principal_type = ? AND principal_id = ? AND status = ?", workspaceID, accessmodel.PrincipalTypeUser, accountID, "active").Count(&count).Error; err != nil {
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
	view.CanCreateKey = policy.Mode != accessmodel.AccessModeDisabled && (scope.CanManage || (view.Grant != nil && view.Grant.IsActive(s.now())) || policy.Mode == accessmodel.AccessModeSelfService)
	return view, nil
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
		if ttl != nil && *ttl <= 0 {
			return ErrInvalid
		}
	}
	if input.DefaultTTLSeconds != nil && input.MaxTTLSeconds != nil && *input.DefaultTTLSeconds > *input.MaxTTLSeconds {
		return ErrInvalid
	}
	return nil
}

func (s *Service) PutPolicy(ctx context.Context, workspaceID, accountID string, input PolicyInput) (*accessmodel.Policy, error) {
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
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	policy.Mode, policy.DefaultQuota, policy.MaxQuota, policy.MaxKeys = input.Mode, input.DefaultQuota, input.MaxQuota, input.MaxKeys
	policy.DefaultTTLSeconds, policy.MaxTTLSeconds = input.DefaultTTLSeconds, input.MaxTTLSeconds
	policy.AllowedModels, policy.UpdatedByAccountID = unique(input.AllowedModels), &accountID
	policy.Version++
	if policy.ID == "" {
		if err := s.db.WithContext(ctx).Create(policy).Error; err != nil {
			return nil, fmt.Errorf("create developer access policy: %w", err)
		}
	} else if err := s.db.WithContext(ctx).Save(policy).Error; err != nil {
		return nil, fmt.Errorf("update developer access policy: %w", err)
	}
	return policy, nil
}

func (s *Service) ListRequests(ctx context.Context, workspaceID, accountID, status string) ([]accessmodel.AccessRequest, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("workspace_id = ?", workspaceID)
	if !scope.CanManage {
		query = query.Where("requester_account_id = ?", accountID)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("status = ?", status)
	}
	var items []accessmodel.AccessRequest
	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list access requests: %w", err)
	}
	if err := s.hydrateRequesters(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
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
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	if policy.Mode == accessmodel.AccessModeDisabled {
		return nil, ErrAccessDisabled
	}
	if policy.Mode == accessmodel.AccessModeSelfService {
		return nil, ErrConflict
	}
	if input.RequestedQuota != nil && (*input.RequestedQuota < 0 || (policy.MaxQuota != nil && *input.RequestedQuota > *policy.MaxQuota)) {
		return nil, ErrInvalid
	}
	if input.RequestedTTLSeconds != nil && (*input.RequestedTTLSeconds <= 0 || (policy.MaxTTLSeconds != nil && *input.RequestedTTLSeconds > *policy.MaxTTLSeconds)) {
		return nil, ErrInvalid
	}
	if !modelsWithin(input.RequestedModels, policy.AllowedModels) {
		return nil, ErrInvalid
	}
	request := &accessmodel.AccessRequest{
		OrganizationID: *scope.Workspace.OrganizationID, WorkspaceID: workspaceID, RequesterAccountID: accountID,
		Purpose: input.Purpose, Environment: environment, RequestedQuota: input.RequestedQuota,
		RequestedModels: unique(input.RequestedModels), RequestedTTLSeconds: input.RequestedTTLSeconds, Status: accessmodel.RequestStatusPending,
	}
	if err := s.db.WithContext(ctx).Create(request).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrConflict
		}
		return nil, fmt.Errorf("create access request: %w", err)
	}
	return request, nil
}

func (s *Service) CancelRequest(ctx context.Context, workspaceID, accountID, requestID string) (*accessmodel.AccessRequest, error) {
	if _, err := s.scope(ctx, workspaceID, accountID); err != nil {
		return nil, err
	}
	var request accessmodel.AccessRequest
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	var request accessmodel.AccessRequest
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ?", requestID, workspaceID).First(&request).Error; err != nil {
			return err
		}
		if request.Status != accessmodel.RequestStatusPending {
			return ErrConflict
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
		if len(models) == 0 {
			models = request.RequestedModels
		}
		if len(models) == 0 {
			models = policy.AllowedModels
		}
		if !modelsWithin(models, policy.AllowedModels) {
			return ErrInvalid
		}
		expiresAt := input.ExpiresAt
		if expiresAt == nil && request.RequestedTTLSeconds != nil {
			value := now.Add(time.Duration(*request.RequestedTTLSeconds) * time.Second)
			expiresAt = &value
		}
		if expiresAt != nil && policy.MaxTTLSeconds != nil && expiresAt.After(now.Add(time.Duration(*policy.MaxTTLSeconds)*time.Second)) {
			return ErrInvalid
		}
		if err := upsertGrant(tx, scope, request.RequesterAccountID, accountID, "approved_request", quota, maxKeys, models, expiresAt); err != nil {
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

func upsertGrant(tx *gorm.DB, scope *workspaceScope, principalID, actorID, source string, quota *int64, maxKeys int, models []string, expiresAt *time.Time) error {
	var grant accessmodel.Grant
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", scope.Workspace.ID, accessmodel.PrincipalTypeUser, principalID).First(&grant).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		grant = accessmodel.Grant{OrganizationID: *scope.Workspace.OrganizationID, WorkspaceID: scope.Workspace.ID, PrincipalType: accessmodel.PrincipalTypeUser, PrincipalID: principalID, Source: source, Status: accessmodel.GrantStatusActive, MaxKeys: maxKeys, AllowedModels: unique(models), ExpiresAt: expiresAt, CreatedByAccountID: &actorID, UpdatedByAccountID: &actorID, AuthorizationVersion: 1}
	} else {
		grant.Source, grant.Status, grant.MaxKeys, grant.AllowedModels, grant.ExpiresAt = source, accessmodel.GrantStatusActive, maxKeys, unique(models), expiresAt
		grant.UpdatedByAccountID = &actorID
		grant.AuthorizationVersion++
	}
	grant.QuotaLimit = quota
	if quota == nil {
		grant.RemainQuota = 0
	} else if *quota > grant.UsedQuota {
		grant.RemainQuota = *quota - grant.UsedQuota
	} else {
		grant.RemainQuota = 0
	}
	if grant.ID == "" {
		return tx.Create(&grant).Error
	}
	return tx.Save(&grant).Error
}

func (s *Service) ensureGrant(ctx context.Context, scope *workspaceScope, accountID string, policy *accessmodel.Policy) (*accessmodel.Grant, error) {
	var grant accessmodel.Grant
	err := s.db.WithContext(ctx).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", scope.Workspace.ID, accessmodel.PrincipalTypeUser, accountID).First(&grant).Error
	if err == nil {
		if !grant.IsActive(s.now()) {
			return nil, ErrApprovalNeeded
		}
		return &grant, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load developer access grant: %w", err)
	}
	if !scope.CanManage && policy.Mode != accessmodel.AccessModeSelfService {
		return nil, ErrApprovalNeeded
	}
	source := "self_service"
	if scope.CanManage && policy.Mode != accessmodel.AccessModeSelfService {
		source = "workspace_admin"
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return upsertGrant(tx, scope, accountID, accountID, source, policy.DefaultQuota, policy.MaxKeys, policy.AllowedModels, ttlExpiry(s.now(), policy.DefaultTTLSeconds))
	})
	if err != nil {
		return nil, fmt.Errorf("create self-service grant: %w", err)
	}
	if err := s.db.WithContext(ctx).Where("workspace_id = ? AND principal_type = ? AND principal_id = ?", scope.Workspace.ID, accessmodel.PrincipalTypeUser, accountID).First(&grant).Error; err != nil {
		return nil, err
	}
	return &grant, nil
}

func ttlExpiry(now time.Time, seconds *int64) *time.Time {
	if seconds == nil {
		return nil
	}
	value := now.Add(time.Duration(*seconds) * time.Second)
	return &value
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
	policy, err := s.policy(ctx, scope)
	if err != nil {
		return nil, err
	}
	if policy.Mode == accessmodel.AccessModeDisabled {
		return nil, ErrAccessDisabled
	}
	grant, err := s.ensureGrant(ctx, scope, accountID, policy)
	if err != nil {
		return nil, err
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
		var lockedGrant accessmodel.Grant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", grant.ID).First(&lockedGrant).Error; err != nil {
			return err
		}
		if !lockedGrant.IsActive(s.now()) || lockedGrant.AuthorizationVersion != grant.AuthorizationVersion {
			return ErrApprovalNeeded
		}
		var active int64
		if err := tx.Model(&apikeymodel.TenantAPIKey{}).Where("workspace_id = ? AND principal_type = ? AND principal_id = ? AND status = ?", workspaceID, principalType, accountID, "active").Count(&active).Error; err != nil {
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

func (s *Service) ListKeys(ctx context.Context, workspaceID, accountID string) ([]KeyView, error) {
	scope, err := s.scope(ctx, workspaceID, accountID)
	if err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).Where("workspace_id = ? AND principal_type = ?", workspaceID, accessmodel.PrincipalTypeUser)
	if !scope.CanManage {
		query = query.Where("principal_id = ?", accountID)
	}
	var rows []apikeymodel.TenantAPIKey
	if err := query.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list personal API keys: %w", err)
	}
	views := make([]KeyView, 0, len(rows))
	for i := range rows {
		views = append(views, keyToView(&rows[i]))
	}
	return views, nil
}

func (s *Service) ownedKey(ctx context.Context, scope *workspaceScope, accountID, keyID string) (*apikeymodel.TenantAPIKey, error) {
	var key apikeymodel.TenantAPIKey
	query := s.db.WithContext(ctx).Where("id = ? AND workspace_id = ? AND principal_type = ?", keyID, scope.Workspace.ID, accessmodel.PrincipalTypeUser)
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
	key, err := s.ownedKey(ctx, scope, accountID, keyID)
	if err != nil {
		return nil, err
	}
	key.Name = input.Name
	if err := s.keys.Update(ctx, key); err != nil {
		return nil, err
	}
	view := keyToView(key)
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
	var key apikeymodel.TenantAPIKey
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ? AND principal_type = ?", keyID, scope.Workspace.ID, accessmodel.PrincipalTypeUser)
		if !scope.CanManage {
			query = query.Where("principal_id = ?", accountID)
		}
		if err := query.First(&key).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if key.Status == "revoked" {
			return ErrConflict
		}
		if status == "active" {
			if key.ExpiresAt != nil && !key.ExpiresAt.After(s.now()) {
				return ErrConflict
			}
			if key.AccessGrantID == nil {
				return ErrApprovalNeeded
			}
			var grant accessmodel.Grant
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", *key.AccessGrantID).First(&grant).Error; err != nil || !grant.IsActive(s.now()) || grant.AuthorizationVersion != key.AuthorizationVersion {
				return ErrApprovalNeeded
			}
			var active int64
			if err := tx.Model(&apikeymodel.TenantAPIKey{}).
				Where("access_grant_id = ? AND status = ? AND id <> ?", grant.ID, "active", key.ID).
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
		return tx.Save(&key).Error
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

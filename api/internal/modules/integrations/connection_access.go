package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ConnectionGrantPrincipalType string

const (
	ConnectionGrantPrincipalOrganization ConnectionGrantPrincipalType = "organization"
	ConnectionGrantPrincipalWorkspace    ConnectionGrantPrincipalType = "workspace"
	ConnectionGrantPrincipalAccount      ConnectionGrantPrincipalType = "account"
)

type ConnectionGrantAccessMode string

const (
	ConnectionGrantAccessRead  ConnectionGrantAccessMode = "read"
	ConnectionGrantAccessWrite ConnectionGrantAccessMode = "write"
)

type IntegrationConnectionGrant struct {
	ID                  uuid.UUID                    `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID      uuid.UUID                    `gorm:"type:uuid;not null;index" json:"organization_id"`
	ConnectionID        uuid.UUID                    `gorm:"type:uuid;not null;index" json:"connection_id"`
	PrincipalType       ConnectionGrantPrincipalType `gorm:"size:32;not null" json:"principal_type"`
	PrincipalID         *uuid.UUID                   `gorm:"type:uuid" json:"principal_id,omitempty"`
	AccessMode          ConnectionGrantAccessMode    `gorm:"size:16;not null" json:"access_mode"`
	AllowedActionIDs    []string                     `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"allowed_action_ids"`
	ResourceConstraints map[string]any               `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"resource_constraints"`
	Revision            int                          `gorm:"not null;default:1" json:"revision"`
	CreatedBy           *uuid.UUID                   `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy           *uuid.UUID                   `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt           time.Time                    `json:"created_at"`
	UpdatedAt           time.Time                    `json:"updated_at"`
}

func (IntegrationConnectionGrant) TableName() string { return "integration_connection_grants" }

func (grant *IntegrationConnectionGrant) BeforeCreate(_ *gorm.DB) error {
	if grant.ID == uuid.Nil {
		grant.ID = uuid.New()
	}
	if grant.Revision < 1 {
		grant.Revision = 1
	}
	grant.AllowedActionIDs = normalizeAccessActionIDs(grant.AllowedActionIDs)
	constraints, err := normalizeConnectionGrantConstraints(grant.ResourceConstraints)
	if err != nil {
		return err
	}
	grant.ResourceConstraints = constraints
	return validateConnectionGrant(grant)
}

type ConnectionAccessRequest struct {
	OrganizationID    uuid.UUID
	WorkspaceID       *uuid.UUID
	AccountID         uuid.UUID
	ConnectionID      uuid.UUID
	IntegrationID     string
	ActionID          string
	Effect            toolgovernance.Effect
	ResourceIDs       []string
	ResourcesRequired bool
}

type ConnectionAccessAuthorizer interface {
	AuthorizeConnectionUse(ctx context.Context, request ConnectionAccessRequest) error
}

// AgentConnectionAccessAuthorizer revalidates the current shared grant for an
// Agent invocation. Agent runtimes may execute without the account that
// configured them, so personal account credentials and account-only grants are
// never authorization sources here.
type AgentConnectionAccessAuthorizer interface {
	AuthorizeAgentConnectionUse(ctx context.Context, request ConnectionAccessRequest) error
}

type ConnectionPreferenceAccessChecker interface {
	AuthorizeConnectionPreference(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, connectionID uuid.UUID) error
}

// ConnectionVisibilityAccessChecker authorizes disclosure of connection
// metadata without requiring the connection to be currently healthy. It is
// intended only for already-selected connection diagnostics; execution and
// new selections must use the stricter authorizers above.
type ConnectionVisibilityAccessChecker interface {
	AuthorizeConnectionVisibility(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, connectionID uuid.UUID) error
}

type ConnectionGrantRepository interface {
	ListApplicable(ctx context.Context, organizationID, connectionID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]IntegrationConnectionGrant, error)
	List(ctx context.Context, organizationID, connectionID uuid.UUID) ([]IntegrationConnectionGrant, error)
	Save(ctx context.Context, grant *IntegrationConnectionGrant, expectedRevision int) error
	Delete(ctx context.Context, organizationID, connectionID, grantID uuid.UUID) error
}

type GormConnectionGrantRepository struct{ db *gorm.DB }

func NewGormConnectionGrantRepository(db *gorm.DB) *GormConnectionGrantRepository {
	return &GormConnectionGrantRepository{db: db}
}

func (repository *GormConnectionGrantRepository) ListApplicable(ctx context.Context, organizationID, connectionID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]IntegrationConnectionGrant, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("connection grant repository is unavailable")
	}
	query := repository.db.WithContext(ctx).
		Where("organization_id = ? AND connection_id = ?", organizationID, connectionID).
		Where("(principal_type = ? AND principal_id IS NULL) OR (principal_type = ? AND principal_id = ?)", ConnectionGrantPrincipalOrganization, ConnectionGrantPrincipalAccount, accountID)
	if workspaceID != nil && *workspaceID != uuid.Nil {
		query = repository.db.WithContext(ctx).
			Where("organization_id = ? AND connection_id = ?", organizationID, connectionID).
			Where("(principal_type = ? AND principal_id IS NULL) OR (principal_type = ? AND principal_id = ?) OR (principal_type = ? AND principal_id = ?)", ConnectionGrantPrincipalOrganization, ConnectionGrantPrincipalAccount, accountID, ConnectionGrantPrincipalWorkspace, *workspaceID)
	}
	var grants []IntegrationConnectionGrant
	if err := query.Order("principal_type ASC, id ASC").Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("list applicable connection grants: %w", err)
	}
	return grants, nil
}

func (repository *GormConnectionGrantRepository) List(ctx context.Context, organizationID, connectionID uuid.UUID) ([]IntegrationConnectionGrant, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("connection grant repository is unavailable")
	}
	var grants []IntegrationConnectionGrant
	if err := repository.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ?", organizationID, connectionID).
		Order("principal_type ASC, id ASC").Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("list connection grants: %w", err)
	}
	return grants, nil
}

func (repository *GormConnectionGrantRepository) Save(ctx context.Context, grant *IntegrationConnectionGrant, expectedRevision int) error {
	if repository == nil || repository.db == nil || grant == nil {
		return fmt.Errorf("connection grant repository is unavailable")
	}
	grant.AllowedActionIDs = normalizeAccessActionIDs(grant.AllowedActionIDs)
	constraints, err := normalizeConnectionGrantConstraints(grant.ResourceConstraints)
	if err != nil {
		return err
	}
	grant.ResourceConstraints = constraints
	if err := validateConnectionGrant(grant); err != nil {
		return err
	}
	if grant.ID == uuid.Nil {
		grant.ID = uuid.New()
	}
	if expectedRevision < 1 {
		return repository.db.WithContext(ctx).Create(grant).Error
	}
	var existing IntegrationConnectionGrant
	err = repository.db.WithContext(ctx).
		Select("resource_constraints", "revision").
		Where("organization_id = ? AND connection_id = ? AND id = ?", grant.OrganizationID, grant.ConnectionID, grant.ID).
		Take(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && existing.Revision != expectedRevision) {
		return ErrConnectionChanged
	}
	if err != nil {
		return fmt.Errorf("load connection grant constraints: %w", err)
	}
	if len(existing.ResourceConstraints) > 0 || len(grant.ResourceConstraints) > 0 {
		// Resource-aware grant editing is not exposed yet. Refuse updates at
		// the repository boundary as defense in depth, so another caller
		// cannot accidentally erase or replace a constraint and broaden access.
		return invalidInput("resource-constrained grants cannot be updated", nil)
	}
	actionsJSON, err := json.Marshal(grant.AllowedActionIDs)
	if err != nil {
		return invalidInput("connection grant actions are invalid", err)
	}
	constraintsJSON, err := json.Marshal(grant.ResourceConstraints)
	if err != nil {
		return invalidInput("connection grant resources are invalid", err)
	}
	result := repository.db.WithContext(ctx).Model(&IntegrationConnectionGrant{}).
		Where("organization_id = ? AND connection_id = ? AND id = ? AND revision = ?", grant.OrganizationID, grant.ConnectionID, grant.ID, expectedRevision).
		Updates(map[string]any{
			"principal_type": grant.PrincipalType, "principal_id": grant.PrincipalID,
			"access_mode": grant.AccessMode, "allowed_action_ids": datatypes.JSON(actionsJSON),
			"resource_constraints": datatypes.JSON(constraintsJSON), "updated_by": grant.UpdatedBy,
			"revision": gorm.Expr("revision + 1"), "updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		return fmt.Errorf("update connection grant: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionChanged
	}
	grant.Revision = expectedRevision + 1
	return nil
}

func (repository *GormConnectionGrantRepository) Delete(ctx context.Context, organizationID, connectionID, grantID uuid.UUID) error {
	result := repository.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND id = ?", organizationID, connectionID, grantID).Delete(&IntegrationConnectionGrant{})
	if result.Error != nil {
		return fmt.Errorf("delete connection grant: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionNotFound
	}
	return nil
}

type DefaultConnectionAccessService struct {
	connections ConnectionRepository
	grants      ConnectionGrantRepository
}

func NewConnectionAccessService(connections ConnectionRepository, grants ConnectionGrantRepository) *DefaultConnectionAccessService {
	return &DefaultConnectionAccessService{connections: connections, grants: grants}
}

func (service *DefaultConnectionAccessService) AuthorizeConnectionUse(ctx context.Context, request ConnectionAccessRequest) error {
	if service == nil || service.connections == nil || service.grants == nil {
		return NewError(ErrorCodeAccessDenied, "connection authorization is unavailable", nil)
	}
	if request.OrganizationID == uuid.Nil || request.ConnectionID == uuid.Nil || request.AccountID == uuid.Nil || strings.TrimSpace(request.ActionID) == "" {
		return invalidInput("connection authorization context is incomplete", nil)
	}
	connection, err := service.connections.GetByID(ctx, request.OrganizationID, request.ConnectionID)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			return NewError(ErrorCodeAccessDenied, "integration connection is not available", err)
		}
		return err
	}
	if request.IntegrationID != "" && !strings.EqualFold(connection.IntegrationID, request.IntegrationID) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not available", nil)
	}
	if !connectionAvailableForSelection(connection, time.Now().UTC()) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not active or requires authentication", nil)
	}
	if connection.CredentialSource == ConnectionCredentialSourceAccount {
		if connection.OwnerAccountID != nil && *connection.OwnerAccountID == request.AccountID {
			return nil
		}
		// Personal credentials are never shareable through organization,
		// workspace, or account grants. A grant must not turn another user's
		// OAuth token or API key into a shared connection.
		return NewError(ErrorCodeAccessDenied, "integration connection is not available", nil)
	}
	grants, err := service.grants.ListApplicable(ctx, request.OrganizationID, request.ConnectionID, request.AccountID, request.WorkspaceID)
	if err != nil {
		return NewError(ErrorCodeAccessDenied, "connection grants could not be resolved", err)
	}
	for _, grant := range grants {
		if connectionGrantAllows(grant, request) {
			return nil
		}
	}
	return NewError(ErrorCodeAccessDenied, "account is not authorized to use this integration connection action", nil)
}

func (service *DefaultConnectionAccessService) AuthorizeAgentConnectionUse(ctx context.Context, request ConnectionAccessRequest) error {
	if request.OrganizationID == uuid.Nil || request.ConnectionID == uuid.Nil || strings.TrimSpace(request.ActionID) == "" {
		return invalidInput("Agent connection authorization context is incomplete", nil)
	}
	connection, grants, err := service.agentConnectionAccess(ctx, request.OrganizationID, request.WorkspaceID, request.ConnectionID)
	if err != nil {
		return err
	}
	if request.IntegrationID != "" && !strings.EqualFold(connection.IntegrationID, request.IntegrationID) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not available", nil)
	}
	if !connectionAvailableForSelection(connection, time.Now().UTC()) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not active or requires authentication", nil)
	}
	for _, grant := range grants {
		if connectionGrantAllows(grant, request) {
			return nil
		}
	}
	return NewError(ErrorCodeAccessDenied, "Agent is not authorized to use this integration connection action", nil)
}

func (service *DefaultConnectionAccessService) AuthorizeAgentConnectionPreference(
	ctx context.Context,
	organizationID uuid.UUID,
	workspaceID *uuid.UUID,
	connectionID uuid.UUID,
) error {
	connection, _, err := service.agentConnectionAccess(ctx, organizationID, workspaceID, connectionID)
	if err != nil {
		return err
	}
	if !connectionAvailableForSelection(connection, time.Now().UTC()) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not active or requires authentication", nil)
	}
	return nil
}

// AuthorizeAgentConnectionActionPreference verifies that a healthy shared
// connection has at least one current organization/workspace grant permitting
// the requested action and effect. Resource constraints deliberately remain
// out of this coarse selection check: they narrow the resources available at
// invocation time and are revalidated by AuthorizeAgentConnectionUse.
func (service *DefaultConnectionAccessService) AuthorizeAgentConnectionActionPreference(
	ctx context.Context,
	organizationID uuid.UUID,
	workspaceID *uuid.UUID,
	connectionID uuid.UUID,
	integrationID string,
	actionID string,
	effect toolgovernance.Effect,
) error {
	connection, grants, err := service.agentConnectionAccess(ctx, organizationID, workspaceID, connectionID)
	if err != nil {
		return err
	}
	if integrationID != "" && !strings.EqualFold(connection.IntegrationID, integrationID) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not available", nil)
	}
	if !connectionAvailableForSelection(connection, time.Now().UTC()) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not active or requires authentication", nil)
	}
	for _, grant := range grants {
		if grantAllowsEffect(grant.AccessMode, effect) && containsAccessAction(grant.AllowedActionIDs, actionID) {
			return nil
		}
	}
	return NewError(ErrorCodeAccessDenied, "Agent is not authorized to select this integration connection action", nil)
}

func (service *DefaultConnectionAccessService) AuthorizeAgentConnectionVisibility(
	ctx context.Context,
	organizationID uuid.UUID,
	workspaceID *uuid.UUID,
	connectionID uuid.UUID,
) error {
	_, _, err := service.agentConnectionAccess(ctx, organizationID, workspaceID, connectionID)
	return err
}

func (service *DefaultConnectionAccessService) agentConnectionAccess(
	ctx context.Context,
	organizationID uuid.UUID,
	workspaceID *uuid.UUID,
	connectionID uuid.UUID,
) (*IntegrationConnection, []IntegrationConnectionGrant, error) {
	if service == nil || service.connections == nil || service.grants == nil {
		return nil, nil, NewError(ErrorCodeAccessDenied, "connection authorization is unavailable", nil)
	}
	if organizationID == uuid.Nil || connectionID == uuid.Nil {
		return nil, nil, invalidInput("Agent connection authorization context is incomplete", nil)
	}
	connection, err := service.connections.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return nil, nil, NewError(ErrorCodeAccessDenied, "integration connection is not available", err)
	}
	if connection.CredentialSource != ConnectionCredentialSourceOrganization {
		return nil, nil, NewError(ErrorCodeAccessDenied, "integration connection is not available to Agents", nil)
	}
	grants, err := service.grants.ListApplicable(ctx, organizationID, connectionID, uuid.Nil, workspaceID)
	if err != nil {
		return nil, nil, NewError(ErrorCodeAccessDenied, "connection grants could not be resolved", err)
	}
	shared := make([]IntegrationConnectionGrant, 0, len(grants))
	for _, grant := range grants {
		switch grant.PrincipalType {
		case ConnectionGrantPrincipalOrganization:
			if grant.PrincipalID == nil {
				shared = append(shared, grant)
			}
		case ConnectionGrantPrincipalWorkspace:
			if workspaceID != nil && *workspaceID != uuid.Nil && grant.PrincipalID != nil && *grant.PrincipalID == *workspaceID {
				shared = append(shared, grant)
			}
		}
	}
	if len(shared) == 0 {
		return nil, nil, NewError(ErrorCodeAccessDenied, "Agent has no shared grant for this integration connection", nil)
	}
	return connection, shared, nil
}

func (service *DefaultConnectionAccessService) AuthorizeConnectionPreference(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, connectionID uuid.UUID) error {
	connection, err := service.authorizeConnectionVisibility(ctx, organizationID, accountID, workspaceID, connectionID)
	if err != nil {
		return err
	}
	if !connectionAvailableForSelection(connection, time.Now().UTC()) {
		return NewError(ErrorCodeAccessDenied, "integration connection is not active or requires authentication", nil)
	}
	return nil
}

func (service *DefaultConnectionAccessService) AuthorizeConnectionVisibility(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, connectionID uuid.UUID) error {
	_, err := service.authorizeConnectionVisibility(ctx, organizationID, accountID, workspaceID, connectionID)
	return err
}

func (service *DefaultConnectionAccessService) authorizeConnectionVisibility(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, connectionID uuid.UUID) (*IntegrationConnection, error) {
	if service == nil || service.connections == nil || service.grants == nil {
		return nil, NewError(ErrorCodeAccessDenied, "connection authorization is unavailable", nil)
	}
	connection, err := service.connections.GetByID(ctx, organizationID, connectionID)
	if err != nil {
		return nil, NewError(ErrorCodeAccessDenied, "integration connection is not available", err)
	}
	if connection.CredentialSource == ConnectionCredentialSourceAccount {
		if connection.OwnerAccountID != nil && *connection.OwnerAccountID == accountID {
			return connection, nil
		}
		return nil, NewError(ErrorCodeAccessDenied, "integration connection is not available", nil)
	}
	if connection.CredentialSource != ConnectionCredentialSourceOrganization {
		return nil, NewError(ErrorCodeAccessDenied, "integration connection is not available", nil)
	}
	grants, err := service.grants.ListApplicable(ctx, organizationID, connectionID, accountID, workspaceID)
	if err != nil {
		return nil, NewError(ErrorCodeAccessDenied, "connection grants could not be resolved", err)
	}
	if len(grants) == 0 {
		return nil, NewError(ErrorCodeAccessDenied, "account is not authorized to select this integration connection", nil)
	}
	return connection, nil
}

func connectionGrantAllows(grant IntegrationConnectionGrant, request ConnectionAccessRequest) bool {
	if !grantAllowsEffect(grant.AccessMode, request.Effect) || !containsAccessAction(grant.AllowedActionIDs, request.ActionID) {
		return false
	}
	allowAll, _ := grant.ResourceConstraints["allow_all"].(bool)
	allowed := resourceConstraintIDs(grant.ResourceConstraints)
	if len(request.ResourceIDs) == 0 {
		// A constrained grant must never degrade into an unconstrained grant
		// merely because the caller has not implemented provider resource
		// extraction yet. Explicit allow_all is the only safe exception.
		if allowAll {
			return true
		}
		return !request.ResourcesRequired && len(grant.ResourceConstraints) == 0
	}
	if allowAll {
		return true
	}
	if len(allowed) == 0 {
		return false
	}
	for _, requested := range request.ResourceIDs {
		if _, exists := allowed[strings.TrimSpace(requested)]; !exists {
			return false
		}
	}
	return true
}

func grantAllowsEffect(mode ConnectionGrantAccessMode, effect toolgovernance.Effect) bool {
	if mode == ConnectionGrantAccessWrite {
		return true
	}
	return mode == ConnectionGrantAccessRead && toolgovernance.NormalizeEffect(effect) == toolgovernance.EffectRead
}

func containsAccessAction(values []string, actionID string) bool {
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	for _, value := range values {
		if value == actionID {
			return true
		}
	}
	return false
}

func resourceConstraintIDs(constraints map[string]any) map[string]struct{} {
	result := map[string]struct{}{}
	raw := constraints["resource_ids"]
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				result[value] = struct{}{}
			}
		}
	case []any:
		for _, item := range values {
			if value, ok := item.(string); ok {
				if value = strings.TrimSpace(value); value != "" {
					result[value] = struct{}{}
				}
			}
		}
	}
	return result
}

func validateConnectionGrant(grant *IntegrationConnectionGrant) error {
	if grant == nil || grant.OrganizationID == uuid.Nil || grant.ConnectionID == uuid.Nil {
		return invalidInput("connection grant identity is required", nil)
	}
	switch grant.PrincipalType {
	case ConnectionGrantPrincipalOrganization:
		if grant.PrincipalID != nil {
			return invalidInput("organization grants cannot specify a principal id", nil)
		}
	case ConnectionGrantPrincipalWorkspace, ConnectionGrantPrincipalAccount:
		if grant.PrincipalID == nil || *grant.PrincipalID == uuid.Nil {
			return invalidInput("connection grant principal id is required", nil)
		}
	default:
		return invalidInput("connection grant principal type is invalid", nil)
	}
	if grant.AccessMode != ConnectionGrantAccessRead && grant.AccessMode != ConnectionGrantAccessWrite {
		return invalidInput("connection grant access mode is invalid", nil)
	}
	if len(grant.AllowedActionIDs) == 0 || len(grant.AllowedActionIDs) > 128 {
		return invalidInput("connection grant must allow at least one action", nil)
	}
	for _, actionID := range grant.AllowedActionIDs {
		if actionID == "*" {
			return invalidInput("connection grants must name explicit provider actions", nil)
		}
		if !integrationIdentifierPattern.MatchString(actionID) {
			return invalidInput("connection grant action is invalid", nil)
		}
	}
	return nil
}

func normalizeAccessActionIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

type ConnectionScopeRequirement struct {
	AllOf []string
	AnyOf []string
}

func AuthorizeConnectionScopes(granted []string, requirement ConnectionScopeRequirement) error {
	grantedSet := make(map[string]struct{}, len(granted))
	for _, scope := range granted {
		if scope = strings.TrimSpace(scope); scope != "" {
			grantedSet[scope] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, scope := range normalizeScopeRequirement(requirement.AllOf) {
		if _, exists := grantedSet[scope]; !exists {
			missing = append(missing, scope)
		}
	}
	anyOf := normalizeScopeRequirement(requirement.AnyOf)
	if len(anyOf) > 0 {
		matched := false
		for _, scope := range anyOf {
			if _, exists := grantedSet[scope]; exists {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, anyOf...)
		}
	}
	if len(missing) > 0 {
		return NewError(ErrorCodeInsufficientScope, "integration connection does not grant the scopes required by this action", nil)
	}
	return nil
}

func normalizeScopeRequirement(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeConnectionGrantConstraints(value map[string]any) (map[string]any, error) {
	if len(value) == 0 {
		return map[string]any{}, nil
	}
	result := make(map[string]any, 2)
	for key, raw := range value {
		switch key {
		case "allow_all":
			allowed, ok := raw.(bool)
			if !ok {
				return nil, invalidInput("connection grant allow_all must be boolean", nil)
			}
			result[key] = allowed
		case "resource_ids":
			ids := resourceConstraintIDs(map[string]any{"resource_ids": raw})
			if len(ids) == 0 || len(ids) > 100 {
				return nil, invalidInput("connection grant resource ids are invalid", nil)
			}
			normalized := make([]string, 0, len(ids))
			for id := range ids {
				if len([]rune(id)) > 256 {
					return nil, invalidInput("connection grant resource id is too long", nil)
				}
				normalized = append(normalized, id)
			}
			sort.Strings(normalized)
			result[key] = normalized
		default:
			return nil, invalidInput("connection grant resource constraint is unsupported", nil)
		}
	}
	return result, nil
}

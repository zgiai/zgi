package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations/catalog"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	integrationpreferences "github.com/zgiai/zgi/api/internal/modules/integrations/preferences"
	wecomintegration "github.com/zgiai/zgi/api/internal/modules/integrations/providers/collaboration/wecom"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

const integrationErrorProjectorContextKey = "integration_application_error_projector"

type IntegrationRouteDeps struct {
	DB                      *gorm.DB
	Registry                *integrations.Registry
	Connections             integrations.ConnectionService
	ConnectionRepo          integrations.ConnectionRepository
	Grants                  integrations.ConnectionGrantRepository
	Access                  *integrations.DefaultConnectionAccessService
	Preferences             *integrationpreferences.DefaultAIChatIntegrationPreferenceService
	HealthEvents            integrations.ConnectionHealthEventRepository
	Policies                integrations.ActionPolicyService
	Executions              integrations.ExecutionQueryRepository
	AccountService          interfaces.AccountService
	OAuthFlows              *integrations.OAuthFlowService
	OAuthClients            *integrations.OAuthClientConfigService
	OAuthRecovery           integrations.OAuthRecoveryAdminRepository
	ProviderInstallFlows    *integrations.ProviderInstallFlowService
	WeComCallbacks          *wecomintegration.Adapter
	AgentBindings           *agentbindings.Repository
	WorkflowReferences      workflowConnectionReferenceService
	ApplicationErrorCatalog *appcatalog.Catalog
	OAuthCallbackURL        string
	OAuthResultURL          string
	WeComInstallCallbackURL string
	WeComResultURL          string
}

type workflowConnectionReferenceService interface {
	EnsureConnectionReferenceIndexReady(context.Context, uuid.UUID) error
	PreviewConnectionReferences(context.Context, uuid.UUID, uuid.UUID) ([]agentbindings.WorkflowConnectionReference, error)
	DetachConnectionReferences(context.Context, *gorm.DB, uuid.UUID, uuid.UUID, uuid.UUID) ([]agentbindings.WorkflowConnectionReference, error)
}

type integrationHandler struct {
	deps                   IntegrationRouteDeps
	workspaceScopeResolver *aichatWorkspaceScopeResolver
}

const detachIntegrationConnectionReferencesOperation = "detach_integration_connection_references"

type detachIntegrationConnectionReferencesRequest struct {
	ImpactToken string `json:"impact_token" binding:"required"`
}

type integrationConnectionDeleteImpactResponse struct {
	ConnectionID       uuid.UUID                        `json:"connection_id"`
	BoundAgentCount    int                              `json:"bound_agent_count"`
	BoundWorkflowCount int                              `json:"bound_workflow_count"`
	CanDelete          bool                             `json:"can_delete"`
	ImpactToken        string                           `json:"impact_token,omitempty"`
	ExpiresAt          int64                            `json:"expires_at,omitempty"`
	References         []integrationConnectionReference `json:"references"`
}

type integrationConnectionReference struct {
	AssetID             uuid.UUID                            `json:"asset_id"`
	AssetType           string                               `json:"asset_type"`
	Name                string                               `json:"name"`
	WorkspaceID         uuid.UUID                            `json:"workspace_id"`
	WorkspaceName       string                               `json:"workspace_name"`
	DraftReferenced     bool                                 `json:"draft_referenced"`
	PublishedReferenced bool                                 `json:"published_referenced"`
	Actions             []string                             `json:"actions"`
	Nodes               []integrationConnectionReferenceNode `json:"nodes"`
}

type integrationConnectionReferenceNode struct {
	NodeID              string `json:"node_id"`
	NodeName            string `json:"node_name"`
	ActionID            string `json:"action_id"`
	DraftReferenced     bool   `json:"draft_referenced"`
	PublishedReferenced bool   `json:"published_referenced"`
}

type createIntegrationConnectionRequest struct {
	IntegrationID    string                                  `json:"integration_id" binding:"required"`
	DriverID         string                                  `json:"driver_id" binding:"required"`
	Name             string                                  `json:"name" binding:"required"`
	CredentialSource integrations.ConnectionCredentialSource `json:"credential_source" binding:"required"`
	AuthType         integrations.ConnectionAuthType         `json:"auth_type" binding:"required"`
	AuthMethodID     string                                  `json:"auth_method_id"`
	Credentials      map[string]string                       `json:"credentials"`
	Config           map[string]interface{}                  `json:"config"`
	ExpiresAt        *time.Time                              `json:"expires_at"`
}

type updateIntegrationConnectionRequest struct {
	Revision       int                     `json:"revision" binding:"required,min=1"`
	Name           *string                 `json:"name"`
	Credentials    map[string]string       `json:"credentials"`
	Config         *map[string]interface{} `json:"config"`
	ExpiresAt      *time.Time              `json:"expires_at"`
	ClearExpiresAt bool                    `json:"clear_expires_at"`
	Disabled       *bool                   `json:"disabled"`
}

type replaceIntegrationPoliciesRequest struct {
	Revision string                           `json:"revision" binding:"required,len=64,hexadecimal"`
	Policies []integrations.ActionPolicyInput `json:"policies" binding:"required"`
}

type saveIntegrationConnectionGrantRequest struct {
	Revision         int                                       `json:"revision"`
	PrincipalType    integrations.ConnectionGrantPrincipalType `json:"principal_type" binding:"required"`
	PrincipalID      *uuid.UUID                                `json:"principal_id"`
	AccessMode       integrations.ConnectionGrantAccessMode    `json:"access_mode" binding:"required"`
	AllowedActionIDs []string                                  `json:"allowed_action_ids" binding:"required,min=1,max=128"`
	// Resource constraints remain unavailable until provider-owned resource
	// extraction is wired into the execution boundary. Reject, rather than
	// silently ignore, a client attempting to configure them.
	ResourceConstraints map[string]interface{} `json:"resource_constraints"`
}

type replaceAIChatIntegrationPreferencesRequest struct {
	Items []aichatIntegrationPreferenceRequest `json:"items" binding:"max=32"`
}

type aichatIntegrationPreferenceRequest struct {
	IntegrationID         string      `json:"integration_id" binding:"required"`
	SelectedConnectionIDs []uuid.UUID `json:"selected_connection_ids" binding:"required,min=1,max=20"`
	PreferredConnectionID *uuid.UUID  `json:"preferred_connection_id" binding:"required"`
}

type integrationCatalogResponse struct {
	Items []integrations.ProviderCatalogItem `json:"items"`
}

type integrationActionSearchResponse struct {
	Items []integrations.ActionSummary `json:"items"`
}

type integrationCapabilityAvailability string

const (
	integrationCapabilityAvailable         integrationCapabilityAvailability = "available"
	integrationCapabilityNeedsConnection   integrationCapabilityAvailability = "needs_connection"
	integrationCapabilityNeedsScope        integrationCapabilityAvailability = "needs_scope"
	integrationCapabilityNeedsPermission   integrationCapabilityAvailability = "needs_permission"
	integrationCapabilityDisabledByPolicy  integrationCapabilityAvailability = "disabled_by_policy"
	integrationCapabilityDataEgressBlocked integrationCapabilityAvailability = "data_egress_blocked"
)

type integrationCapabilitySummary struct {
	Total          int `json:"total"`
	Read           int `json:"read"`
	Write          int `json:"write"`
	Available      int `json:"available"`
	NeedsAttention int `json:"needs_attention"`
}

type integrationActionCapabilityResponse struct {
	integrations.ActionSummary
	Enabled                   bool                                   `json:"enabled"`
	ApprovalPolicy            integrations.IntegrationApprovalPolicy `json:"approval_policy"`
	DataEgressAllowed         bool                                   `json:"data_egress_allowed"`
	Availability              integrationCapabilityAvailability      `json:"availability"`
	CompatibleConnectionCount int                                    `json:"compatible_connection_count"`
}

type integrationProviderCapabilitiesResponse struct {
	IntegrationID string                                `json:"integration_id"`
	Summary       integrationCapabilitySummary          `json:"summary"`
	Actions       []integrationActionCapabilityResponse `json:"actions"`
}

type integrationGrantPrincipalState string

const (
	integrationGrantPrincipalStateActive  integrationGrantPrincipalState = "active"
	integrationGrantPrincipalStateMissing integrationGrantPrincipalState = "missing"
)

type integrationConnectionGrantResponse struct {
	ID                     uuid.UUID                                 `json:"id"`
	OrganizationID         uuid.UUID                                 `json:"organization_id"`
	ConnectionID           uuid.UUID                                 `json:"connection_id"`
	PrincipalType          integrations.ConnectionGrantPrincipalType `json:"principal_type"`
	PrincipalID            *uuid.UUID                                `json:"principal_id,omitempty"`
	PrincipalDisplayName   string                                    `json:"principal_display_name,omitempty"`
	PrincipalState         integrationGrantPrincipalState            `json:"principal_state"`
	HasResourceConstraints bool                                      `json:"has_resource_constraints"`
	Editable               bool                                      `json:"editable"`
	AccessMode             integrations.ConnectionGrantAccessMode    `json:"access_mode"`
	AllowedActionIDs       []string                                  `json:"allowed_action_ids"`
	Revision               int                                       `json:"revision"`
	CreatedBy              *uuid.UUID                                `json:"created_by,omitempty"`
	UpdatedBy              *uuid.UUID                                `json:"updated_by,omitempty"`
	CreatedAt              time.Time                                 `json:"created_at"`
	UpdatedAt              time.Time                                 `json:"updated_at"`
}

func connectionGrantResponse(
	grant integrations.IntegrationConnectionGrant,
	displayName string,
	state integrationGrantPrincipalState,
) integrationConnectionGrantResponse {
	hasResourceConstraints := len(grant.ResourceConstraints) > 0
	return integrationConnectionGrantResponse{
		ID: grant.ID, OrganizationID: grant.OrganizationID, ConnectionID: grant.ConnectionID,
		PrincipalType: grant.PrincipalType, PrincipalID: grant.PrincipalID,
		PrincipalDisplayName: displayName, PrincipalState: state,
		HasResourceConstraints: hasResourceConstraints, Editable: !hasResourceConstraints,
		AccessMode: grant.AccessMode, AllowedActionIDs: grant.AllowedActionIDs,
		Revision: grant.Revision, CreatedBy: grant.CreatedBy, UpdatedBy: grant.UpdatedBy,
		CreatedAt: grant.CreatedAt, UpdatedAt: grant.UpdatedAt,
	}
}

func RegisterIntegrationRoutes(router *gin.RouterGroup, deps IntegrationRouteDeps) {
	if deps.DB == nil || deps.Registry == nil || deps.Connections == nil || deps.ConnectionRepo == nil || deps.Grants == nil || deps.Access == nil || deps.Preferences == nil || deps.HealthEvents == nil || deps.Policies == nil || deps.Executions == nil || deps.AccountService == nil || deps.OAuthFlows == nil || deps.OAuthClients == nil || deps.OAuthRecovery == nil || deps.ProviderInstallFlows == nil || deps.WeComCallbacks == nil || deps.AgentBindings == nil || deps.WorkflowReferences == nil || deps.ApplicationErrorCatalog == nil || strings.TrimSpace(deps.OAuthCallbackURL) == "" || strings.TrimSpace(deps.OAuthResultURL) == "" || strings.TrimSpace(deps.WeComInstallCallbackURL) == "" || strings.TrimSpace(deps.WeComResultURL) == "" {
		panic("integration routes require complete dependencies")
	}
	errorProjector, err := apptransport.NewProjector(deps.ApplicationErrorCatalog)
	if err != nil {
		panic("integration routes require application error catalog")
	}
	workspaceScopeResolver := &aichatWorkspaceScopeResolver{
		workspaces: gormAIChatWorkspaceProvider{db: deps.DB},
		accounts:   deps.AccountService,
	}
	handler := &integrationHandler{deps: deps, workspaceScopeResolver: workspaceScopeResolver}
	errorProjection := integrationErrorProjectorMiddleware(errorProjector)
	// Provider callbacks cannot require the console JWT. OAuth callbacks use
	// single-use server state; encrypted WeCom callbacks authenticate with their
	// signature, AES key, and the receiver identifier expected by each route.
	router.GET("/integrations/oauth/callback", middleware.SetupRequired(), errorProjection, handler.oauthCallback)
	router.GET("/integrations/wecom/install/callback", middleware.SetupRequired(), errorProjection, handler.wecomInstallCallback)
	router.GET("/integrations/wecom/suite/events", middleware.SetupRequired(), errorProjection, handler.wecomCallbackVerification)
	router.POST("/integrations/wecom/suite/events", middleware.SetupRequired(), errorProjection, handler.wecomSuiteEncryptedCallback)
	router.GET("/integrations/wecom/data/events/:corp_id", middleware.SetupRequired(), errorProjection, handler.wecomDataCallbackVerification)
	router.POST("/integrations/wecom/data/events/:corp_id", middleware.SetupRequired(), errorProjection, handler.wecomDataEncryptedCallback)
	group := router.Group("/integrations")
	group.Use(middleware.SetupRequired())
	group.Use(integrationErrorProjectorMiddleware(errorProjector))
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))

	group.GET("/catalog", handler.catalog)
	group.GET("/providers", handler.catalog)
	group.GET("/providers/:integration_id", handler.providerDetail)
	group.GET("/providers/:integration_id/actions", handler.searchProviderActions)
	group.GET("/providers/:integration_id/actions/:action_id", handler.actionDetail)
	group.GET("/providers/:integration_id/capabilities", handler.providerCapabilities)
	group.GET("/available-connections", handler.listAvailableConnections)
	group.GET("/aichat/preferences", handler.listAIChatPreferences)
	group.PUT("/aichat/preferences", handler.replaceAIChatPreferences)
	group.GET("/my-connections", handler.listMyConnections)
	group.POST("/my-connections", handler.createMyConnection)
	group.PATCH("/my-connections/:id", handler.updateMyConnection)
	group.POST("/my-connections/:id/test", handler.testMyConnection)
	group.POST("/my-connections/:id/complete-setup", handler.completeMyConnectionSetup)
	group.DELETE("/my-connections/:id", handler.deleteMyConnection)
	group.POST("/oauth/flows", handler.startOAuthFlow)
	group.GET("/oauth/flows/:flow_id", handler.pollOAuthFlow)
	group.POST("/oauth/flows/:flow_id/cancel", handler.cancelOAuthFlow)
	group.POST("/provider-install/flows", handler.startProviderInstallFlow)
	group.GET("/provider-install/flows/:flow_id", handler.pollProviderInstallFlow)
	group.POST("/provider-install/flows/:flow_id/cancel", handler.cancelProviderInstallFlow)

	management := group.Group("")
	management.Use(middleware.EnterpriseAdminOrOwnerRequired())
	management.GET("/connections", handler.listConnections)
	management.GET("/connections/:id", handler.getConnection)
	management.POST("/connections", handler.createConnection)
	management.PATCH("/connections/:id", handler.updateConnection)
	management.POST("/connections/:id/test", handler.testConnection)
	management.POST("/connections/:id/complete-setup", handler.completeConnectionSetup)
	management.POST("/connections/:id/default", handler.setDefaultConnection)
	management.GET("/connections/:id/grants", handler.listConnectionGrants)
	management.POST("/connections/:id/grants", handler.createConnectionGrant)
	management.PUT("/connections/:id/grants/:grant_id", handler.updateConnectionGrant)
	management.DELETE("/connections/:id/grants/:grant_id", handler.deleteConnectionGrant)
	management.GET("/connections/:id/health-events", handler.listConnectionHealthEvents)
	management.GET("/connections/:id/delete-impact", handler.connectionDeleteImpact)
	management.POST("/connections/:id/detach-references", handler.detachConnectionReferences)
	management.DELETE("/connections/:id", handler.deleteConnection)
	management.GET("/executions", handler.listExecutions)
	management.GET("/oauth-recovery", handler.oauthRecoverySummary)
	management.POST("/oauth-recovery/:operation_ref/acknowledge", handler.acknowledgeOAuthRecovery)
	management.GET("/:integration_id/action-policies", handler.listActionPolicies)
	management.PUT("/:integration_id/action-policies", handler.replaceActionPolicies)
	management.GET("/:integration_id/oauth-client-configs/:auth_method_id", handler.getOAuthClientConfig)
	management.GET("/:integration_id/oauth-client-configs/:auth_method_id/impact", handler.getOAuthClientConfigImpact)
	management.PUT("/:integration_id/oauth-client-configs/:auth_method_id", handler.putOAuthClientConfig)
	management.DELETE("/:integration_id/oauth-client-configs/:auth_method_id", handler.deleteOAuthClientConfig)
}

func (handler *integrationHandler) catalog(c *gin.Context) {
	audience := strings.ToLower(strings.TrimSpace(c.Query("audience")))
	if audience == "" {
		audience = "account"
	}
	if audience != "account" && audience != "shared" && audience != "organization" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if audience == "organization" && !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	workspaceID, ok := handler.integrationWorkspaceID(c, organizationID, accountID)
	if !ok {
		return
	}
	connections, err := handler.deps.Connections.List(c.Request.Context(), organizationID, integrations.ConnectionListFilter{})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	visible := make([]integrations.ConnectionView, 0, len(connections))
	for _, connection := range connections {
		if connection.CredentialSource != integrations.ConnectionCredentialSourceOrganization &&
			connection.CredentialSource != integrations.ConnectionCredentialSourceAccount {
			continue
		}
		// Organization management and Agent configuration must never derive a
		// provider readiness badge from one administrator's personal token.
		// The account audience remains available to personal settings and
		// AIChat, where an owner may legitimately select their own connection.
		if (audience == "organization" || audience == "shared") && connection.CredentialSource == integrations.ConnectionCredentialSourceAccount {
			continue
		}
		if audience == "organization" {
			// Organization admins manage every shared connection in the active
			// organization, including newly-created connections whose grants are
			// not configured yet. The admin check above is the disclosure boundary.
			visible = append(visible, connection)
			continue
		}
		if err := handler.deps.Access.AuthorizeConnectionVisibility(c.Request.Context(), organizationID, accountID, workspaceID, connection.ID); err == nil {
			visible = append(visible, connection)
		}
	}
	items := catalog.WithConnectionHealth(handler.deps.Registry.Catalog(), visible, time.Now().UTC())
	for itemIndex := range items {
		handler.applyOAuthClientReadiness(c.Request.Context(), organizationID, items[itemIndex].ID, items[itemIndex].DriverID, items[itemIndex].Auth)
	}
	response.Success(c, integrationCatalogResponse{Items: items})
}

func (handler *integrationHandler) providerDetail(c *gin.Context) {
	definition, ok := handler.deps.Registry.ProviderDefinition(c.Param("integration_id"))
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	organizationID, ok := integrationOrganizationID(c)
	if !ok {
		return
	}
	handler.applyOAuthClientReadiness(c.Request.Context(), organizationID, definition.ID, definition.DriverID, definition.AuthMethods)
	response.Success(c, definition)
}

func (handler *integrationHandler) applyOAuthClientReadiness(
	ctx context.Context,
	organizationID uuid.UUID,
	integrationID string,
	driverID string,
	methods []integrations.AuthMethodDefinition,
) {
	if handler == nil || handler.deps.OAuthClients == nil || organizationID == uuid.Nil {
		return
	}
	for index := range methods {
		if methods[index].Type != integrations.AuthMethodTypeOAuth2 || methods[index].OAuth == nil {
			continue
		}
		methods[index].OAuth.ClientConfigured = handler.deps.OAuthClients.OAuthClientConfigured(ctx, integrations.OAuthClientResolveRequest{
			OrganizationID: organizationID,
			IntegrationID:  integrationID,
			DriverID:       driverID,
			AuthMethodID:   methods[index].ID,
		})
	}
}

func (handler *integrationHandler) searchProviderActions(c *gin.Context) {
	caller := tools.ToolInvokeFrom(strings.ToLower(strings.TrimSpace(c.Query("caller"))))
	if caller != "" && caller != tools.ToolInvokeFromAIChat && caller != tools.ToolInvokeFromAgent && caller != tools.ToolInvokeFromWorkflow && caller != tools.ToolInvokeFromAPI {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, ok := handler.deps.Registry.ProviderDefinition(c.Param("integration_id")); !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	items := handler.deps.Registry.SearchActionSummaries(integrations.ActionSearchRequest{
		Query: c.Query("query"), IntegrationID: c.Param("integration_id"), Caller: caller,
		Limit: parsePositiveInt(c.Query("limit"), 20),
	})
	response.Success(c, integrationActionSearchResponse{Items: items})
}

func (handler *integrationHandler) actionDetail(c *gin.Context) {
	action, ok := handler.deps.Registry.ActionDetail(c.Param("integration_id"), c.Param("action_id"))
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	response.Success(c, action)
}

func (handler *integrationHandler) providerCapabilities(c *gin.Context) {
	integrationID := strings.ToLower(strings.TrimSpace(c.Param("integration_id")))
	definition, ok := handler.deps.Registry.ProviderDefinition(integrationID)
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return
	}
	audience := strings.ToLower(strings.TrimSpace(c.Query("audience")))
	if audience == "" {
		audience = "account"
	}
	if audience != "account" && audience != "organization" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if audience == "organization" && !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	workspaceID, ok := handler.integrationWorkspaceID(c, organizationID, accountID)
	if !ok {
		return
	}
	connections, err := handler.deps.Connections.List(c.Request.Context(), organizationID, integrations.ConnectionListFilter{
		IntegrationID: integrationID,
		Page:          1,
		PageSize:      1000,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	visible := make([]integrations.ConnectionView, 0, len(connections))
	for _, connection := range connections {
		if connection.CredentialSource != integrations.ConnectionCredentialSourceOrganization &&
			connection.CredentialSource != integrations.ConnectionCredentialSourceAccount {
			continue
		}
		if audience == "organization" {
			if connection.CredentialSource == integrations.ConnectionCredentialSourceOrganization {
				visible = append(visible, connection)
			}
			continue
		}
		if err := handler.deps.Access.AuthorizeConnectionVisibility(
			c.Request.Context(),
			organizationID,
			accountID,
			workspaceID,
			connection.ID,
		); err == nil {
			visible = append(visible, connection)
		}
	}

	summaries := handler.deps.Registry.SearchActionSummaries(integrations.ActionSearchRequest{
		IntegrationID: integrationID,
		Limit:         200,
	})
	summaryByID := make(map[string]integrations.ActionSummary, len(summaries))
	for _, item := range summaries {
		summaryByID[item.ID] = item
	}
	result := integrationProviderCapabilitiesResponse{
		IntegrationID: integrationID,
		Actions:       make([]integrationActionCapabilityResponse, 0, len(definition.Actions)),
	}
	for _, action := range definition.Actions {
		actionSummary, exists := summaryByID[action.ID]
		if !exists {
			continue
		}
		decision, err := handler.deps.Policies.Resolve(
			c.Request.Context(),
			organizationID.String(),
			integrationID,
			action,
		)
		if err != nil {
			integrationRouteError(c, err)
			return
		}
		authorize := func(connection integrations.ConnectionView) error {
			if audience == "organization" {
				// The organization view represents shared use. Reuse the same
				// organization/workspace grant check used when an Agent selects an
				// action, rather than treating every administrator-visible
				// connection as executable.
				return handler.deps.Access.AuthorizeAgentConnectionActionPreference(
					c.Request.Context(),
					organizationID,
					workspaceID,
					connection.ID,
					integrationID,
					action.ID,
					action.Effect,
				)
			}
			return handler.deps.Access.AuthorizeConnectionUse(
				c.Request.Context(),
				integrations.ConnectionAccessRequest{
					OrganizationID: organizationID,
					WorkspaceID:    workspaceID,
					AccountID:      accountID,
					ConnectionID:   connection.ID,
					IntegrationID:  integrationID,
					ActionID:       action.ID,
					Effect:         action.Effect,
				},
			)
		}
		availability, compatibleCount, err := integrationActionCapabilityAvailability(
			definition,
			action,
			decision,
			visible,
			authorize,
		)
		if err != nil {
			integrationRouteError(c, err)
			return
		}
		result.Summary.Total++
		if action.Effect == toolgovernance.EffectRead || action.Effect == toolgovernance.EffectNone {
			result.Summary.Read++
		} else {
			result.Summary.Write++
		}
		if availability == integrationCapabilityAvailable {
			result.Summary.Available++
		} else {
			result.Summary.NeedsAttention++
		}
		result.Actions = append(result.Actions, integrationActionCapabilityResponse{
			ActionSummary:             actionSummary,
			Enabled:                   decision.Enabled,
			ApprovalPolicy:            decision.ApprovalPolicy,
			DataEgressAllowed:         decision.DataEgressAllowed,
			Availability:              availability,
			CompatibleConnectionCount: compatibleCount,
		})
	}
	response.Success(c, result)
}

func integrationActionCapabilityAvailability(
	definition integrations.ProviderDefinition,
	action integrations.ActionDefinition,
	decision integrations.ActionPolicyDecision,
	connections []integrations.ConnectionView,
	authorize func(integrations.ConnectionView) error,
) (integrationCapabilityAvailability, int, error) {
	if !decision.Enabled {
		return integrationCapabilityDisabledByPolicy, 0, nil
	}
	if action.DataEgress && !decision.DataEgressAllowed {
		return integrationCapabilityDataEgressBlocked, 0, nil
	}
	compatibleCount := 0
	hasScopeGap := false
	hasPermissionGap := false
	now := time.Now().UTC()
	for _, connection := range connections {
		if connection.Status != integrations.ConnectionStatusActive ||
			connection.HealthStatus == integrations.ConnectionHealthUnhealthy ||
			connection.AuthStatus == integrations.ConnectionAuthExpired ||
			connection.AuthStatus == integrations.ConnectionAuthReconnectRequired ||
			(connection.ExpiresAt != nil && !connection.ExpiresAt.After(now)) ||
			(connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(now)) ||
			(connection.RefreshTokenExpiresAt != nil && !connection.RefreshTokenExpiresAt.After(now)) ||
			!integrations.ActionSupportsAuthMethod(action, connection.AuthMethodID) {
			continue
		}
		if integrationConnectionActionEvidenceContains(connection.DeniedActionIDs, action.ID) {
			hasScopeGap = true
			continue
		}
		scopeRequirement := integrations.ActionScopeRequirement(action)
		if (len(scopeRequirement.AllOf) > 0 || len(scopeRequirement.AnyOf) > 0) &&
			integrations.AuthMethodScopeEvidence(definition, connection.AuthMethodID) != integrations.AuthScopeEvidenceConnectorDeclared &&
			(connection.AuthType == integrations.ConnectionAuthTypeOAuth2 || len(connection.GrantedScopes) > 0) {
			if err := integrations.AuthorizeConnectionScopes(
				connection.GrantedScopes,
				scopeRequirement,
			); err != nil {
				hasScopeGap = true
				continue
			}
		}
		if authorize != nil {
			if err := authorize(connection); err != nil {
				if integrations.ErrorCode(err) == integrations.ErrorCodeAccessDenied {
					hasPermissionGap = true
					continue
				}
				return "", 0, err
			}
		}
		compatibleCount++
	}
	if compatibleCount > 0 {
		return integrationCapabilityAvailable, compatibleCount, nil
	}
	if hasPermissionGap {
		return integrationCapabilityNeedsPermission, 0, nil
	}
	if hasScopeGap {
		return integrationCapabilityNeedsScope, 0, nil
	}
	return integrationCapabilityNeedsConnection, 0, nil
}

func integrationConnectionActionEvidenceContains(actionIDs []string, actionID string) bool {
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	for _, candidate := range actionIDs {
		if strings.ToLower(strings.TrimSpace(candidate)) == actionID {
			return true
		}
	}
	return false
}

func (handler *integrationHandler) listAvailableConnections(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	workspaceID, ok := handler.integrationWorkspaceID(c, organizationID, accountID)
	if !ok {
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := min(parsePositiveInt(c.Query("page_size"), 50), 100)
	connections, err := handler.deps.Connections.List(c.Request.Context(), organizationID, integrations.ConnectionListFilter{
		IntegrationID: c.Query("integration_id"), Statuses: []integrations.ConnectionStatus{integrations.ConnectionStatusActive},
		Page: 1, PageSize: 1000,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	available := make([]integrations.ConnectionView, 0, len(connections))
	for _, connection := range connections {
		if connection.CredentialSource != integrations.ConnectionCredentialSourceOrganization &&
			connection.CredentialSource != integrations.ConnectionCredentialSourceAccount {
			continue
		}
		if err := handler.deps.Access.AuthorizeConnectionPreference(c.Request.Context(), organizationID, accountID, workspaceID, connection.ID); err == nil {
			available = append(available, connection)
		}
	}
	start := min((page-1)*pageSize, len(available))
	end := min(start+pageSize, len(available))
	response.Success(c, integrations.ConnectionListPage{
		Items: available[start:end], Page: page, PageSize: pageSize, Total: int64(len(available)), HasMore: end < len(available),
	})
}

func (handler *integrationHandler) listAIChatPreferences(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	workspaceID, ok := handler.integrationWorkspaceID(c, organizationID, accountID)
	if !ok {
		return
	}
	items, err := handler.deps.Preferences.List(c.Request.Context(), organizationID, accountID, workspaceID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{"items": items})
}

func (handler *integrationHandler) replaceAIChatPreferences(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	workspaceID, ok := handler.integrationWorkspaceID(c, organizationID, accountID)
	if !ok {
		return
	}
	var request replaceAIChatIntegrationPreferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Items == nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	inputs := make([]integrationpreferences.AIChatIntegrationPreferenceInput, 0, len(request.Items))
	for _, item := range request.Items {
		inputs = append(inputs, integrationpreferences.AIChatIntegrationPreferenceInput{
			IntegrationID: item.IntegrationID, SelectedConnectionIDs: item.SelectedConnectionIDs,
			PreferredConnectionID: item.PreferredConnectionID,
		})
	}
	items, err := handler.deps.Preferences.Replace(c.Request.Context(), organizationID, accountID, workspaceID, inputs)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{"items": items})
}

func (handler *integrationHandler) listMyConnections(c *gin.Context) {
	organizationID, accountID, ok := integrationActor(c)
	if !ok {
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := min(parsePositiveInt(c.Query("page_size"), 50), 100)
	connections, err := handler.deps.Connections.List(c.Request.Context(), organizationID, integrations.ConnectionListFilter{
		IntegrationID:     c.Query("integration_id"),
		CredentialSources: []integrations.ConnectionCredentialSource{integrations.ConnectionCredentialSourceAccount},
		OwnerAccountID:    &accountID,
		Page:              1, PageSize: 1000,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	owned := make([]integrations.ConnectionView, 0, len(connections))
	for _, connection := range connections {
		if connection.CredentialSource == integrations.ConnectionCredentialSourceAccount && connection.OwnerAccountID != nil && *connection.OwnerAccountID == accountID {
			owned = append(owned, connection)
		}
	}
	start := min((page-1)*pageSize, len(owned))
	end := min(start+pageSize, len(owned))
	response.Success(c, integrations.ConnectionListPage{
		Items: owned[start:end], Page: page, PageSize: pageSize, Total: int64(len(owned)), HasMore: end < len(owned),
	})
}

func (handler *integrationHandler) createMyConnection(c *gin.Context) {
	organizationID, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request createIntegrationConnectionRequest
	defer func() { clearCredentialMap(request.Credentials) }()
	if err := c.ShouldBindJSON(&request); err != nil || request.CredentialSource != integrations.ConnectionCredentialSourceAccount {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	item, err := handler.deps.Connections.Create(c.Request.Context(), integrations.CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: request.IntegrationID, DriverID: request.DriverID,
		Name: request.Name, CredentialSource: integrations.ConnectionCredentialSourceAccount,
		AuthType: request.AuthType, AuthMethodID: request.AuthMethodID, OwnerAccountID: &actorID,
		Credentials: request.Credentials, Config: request.Config, ExpiresAt: request.ExpiresAt, ActorID: &actorID,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) updateMyConnection(c *gin.Context) {
	organizationID, connectionID, actorID, ok := handler.integrationOwnedConnection(c)
	if !ok {
		return
	}
	var request updateIntegrationConnectionRequest
	defer func() { clearCredentialMap(request.Credentials) }()
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	item, err := handler.deps.Connections.Update(c.Request.Context(), integrations.UpdateConnectionInput{
		OrganizationID: organizationID, ConnectionID: connectionID, ExpectedRevision: request.Revision,
		Name: request.Name, Credentials: request.Credentials, Config: request.Config,
		ExpiresAt: request.ExpiresAt, ClearExpiresAt: request.ClearExpiresAt, Disabled: request.Disabled, ActorID: &actorID,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) testMyConnection(c *gin.Context) {
	organizationID, connectionID, actorID, ok := handler.integrationOwnedConnection(c)
	if !ok {
		return
	}
	connection, profile, err := handler.deps.Connections.Test(c.Request.Context(), organizationID, connectionID, &actorID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	mayIncurCost := false
	if definition, found := handler.deps.Registry.ProviderDefinition(connection.IntegrationID); found {
		mayIncurCost = definition.HealthProbe.MayIncurCost
	}
	response.Success(c, map[string]interface{}{"connection": connection, "profile": profile, "may_incur_cost": mayIncurCost})
}

func (handler *integrationHandler) completeMyConnectionSetup(c *gin.Context) {
	organizationID, connectionID, actorID, ok := handler.integrationOwnedConnection(c)
	if !ok {
		return
	}
	connection, err := handler.deps.Connections.Get(c.Request.Context(), organizationID, connectionID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	if !handler.completeConnectionSetupRequest(c, organizationID, actorID, connection, true) {
		return
	}
	item, err := handler.deps.Connections.Get(c.Request.Context(), organizationID, connectionID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) deleteMyConnection(c *gin.Context) {
	organizationID, connectionID, actorID, ok := handler.integrationOwnedConnection(c)
	if !ok {
		return
	}
	bindingsRepo := handler.agentBindingRepository()
	if bindingsRepo == nil {
		integrationRouteError(c, fmt.Errorf("integration connection binding repository is unavailable"))
		return
	}
	bindings, err := bindingsRepo.ListImpact(c.Request.Context(), agentbindings.ResourceRef{
		OrganizationID: organizationID,
		BindingType:    agentbindings.BindingTypeIntegrationConnection,
		ResourceID:     connectionID.String(),
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	if len(bindings) > 0 {
		c.JSON(http.StatusConflict, response.Response{Code: integrations.ErrorCodeConnectionInUse, Message: "connection is still referenced by an Agent or Workflow"})
		return
	}
	if actorAware, supported := handler.deps.Connections.(integrations.ActorAwareConnectionService); supported {
		err = actorAware.DeleteAs(c.Request.Context(), organizationID, connectionID, &actorID)
	} else {
		err = handler.deps.Connections.Delete(c.Request.Context(), organizationID, connectionID)
	}
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{"deleted": true, "id": connectionID})
}

func (handler *integrationHandler) integrationOwnedConnection(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	organizationID, connectionID, ok := integrationRouteIDs(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	connection, err := handler.deps.ConnectionRepo.GetByID(c.Request.Context(), organizationID, connectionID)
	if err != nil || connection == nil || connection.CredentialSource != integrations.ConnectionCredentialSourceAccount || connection.OwnerAccountID == nil || *connection.OwnerAccountID != actorID {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeAccessDenied, "personal integration connection is not available", err))
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return organizationID, connectionID, actorID, true
}

func (handler *integrationHandler) listConnections(c *gin.Context) {
	organizationID, ok := integrationOrganizationID(c)
	if !ok {
		return
	}
	filter := integrations.ConnectionListFilter{
		IntegrationID: c.Query("integration_id"),
		DriverID:      c.Query("driver_id"),
		CredentialSources: []integrations.ConnectionCredentialSource{
			integrations.ConnectionCredentialSourceOrganization,
		},
		Page:     parsePositiveInt(c.Query("page"), 1),
		PageSize: min(parsePositiveInt(c.Query("page_size"), 20), 100),
	}
	if rawStatus := strings.ToLower(strings.TrimSpace(c.Query("status"))); rawStatus != "" {
		status := integrations.ConnectionStatus(rawStatus)
		switch status {
		case integrations.ConnectionStatusPending, integrations.ConnectionStatusActive, integrations.ConnectionStatusInvalid, integrations.ConnectionStatusDisabled:
			filter.Statuses = []integrations.ConnectionStatus{status}
		default:
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}
	page, err := handler.deps.Connections.ListPage(c.Request.Context(), organizationID, filter)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, page)
}

func (handler *integrationHandler) getConnection(c *gin.Context) {
	_, _, item, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) createConnection(c *gin.Context) {
	organizationID, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request createIntegrationConnectionRequest
	defer func() { clearCredentialMap(request.Credentials) }()
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if request.CredentialSource == integrations.ConnectionCredentialSourceAccount {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeAccessDenied, "personal connections must be managed through my-connections", nil))
		return
	}
	if request.CredentialSource != integrations.ConnectionCredentialSourceOrganization {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeInvalidInput, "credential source must be organization", nil))
		return
	}
	item, err := handler.deps.Connections.Create(c.Request.Context(), integrations.CreateConnectionInput{
		OrganizationID: organizationID, IntegrationID: request.IntegrationID, DriverID: request.DriverID,
		Name: request.Name, CredentialSource: request.CredentialSource, AuthType: request.AuthType,
		AuthMethodID: request.AuthMethodID,
		Credentials:  request.Credentials, Config: request.Config, ExpiresAt: request.ExpiresAt, ActorID: &actorID,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) updateConnection(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request updateIntegrationConnectionRequest
	defer func() { clearCredentialMap(request.Credentials) }()
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	item, err := handler.deps.Connections.Update(c.Request.Context(), integrations.UpdateConnectionInput{
		OrganizationID: organizationID, ConnectionID: connectionID, Name: request.Name,
		Credentials: request.Credentials, Config: request.Config, ExpiresAt: request.ExpiresAt,
		ClearExpiresAt: request.ClearExpiresAt, Disabled: request.Disabled, ActorID: &actorID,
		ExpectedRevision: request.Revision,
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) testConnection(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	connection, profile, err := handler.deps.Connections.Test(c.Request.Context(), organizationID, connectionID, &actorID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	mayIncurCost := false
	if definition, ok := handler.deps.Registry.ProviderDefinition(connection.IntegrationID); ok {
		mayIncurCost = definition.HealthProbe.MayIncurCost
	}
	response.Success(c, map[string]interface{}{"connection": connection, "profile": profile, "may_incur_cost": mayIncurCost})
}

func (handler *integrationHandler) completeConnectionSetup(c *gin.Context) {
	organizationID, _, connection, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	if !handler.completeConnectionSetupRequest(c, organizationID, actorID, connection, false) {
		return
	}
	item, err := handler.deps.Connections.Get(c.Request.Context(), organizationID, connection.ID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) completeConnectionSetupRequest(
	c *gin.Context,
	organizationID, actorID uuid.UUID,
	connection integrations.ConnectionView,
	personal bool,
) bool {
	definition, found := handler.deps.Registry.ProviderDefinition(connection.IntegrationID)
	if !found {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "integration provider is unavailable", nil))
		return false
	}
	actionsByID := make(map[string]integrations.ActionDefinition, len(definition.Actions))
	for _, action := range definition.Actions {
		actionsByID[action.ID] = action
	}
	usableActionIDs := make([]string, 0)
	if connection.PermissionSummary != nil {
		for _, capability := range connection.PermissionSummary.AdaptedCapabilities {
			if !capability.ScopeSatisfied {
				continue
			}
			action, exists := actionsByID[capability.ActionID]
			if !exists {
				continue
			}
			decision, resolveErr := handler.deps.Policies.Resolve(
				c.Request.Context(),
				organizationID.String(),
				connection.IntegrationID,
				action,
			)
			if resolveErr != nil {
				integrationRouteError(c, resolveErr)
				return false
			}
			if decision.Enabled && (!action.DataEgress || decision.DataEgressAllowed) {
				usableActionIDs = append(usableActionIDs, capability.ActionID)
			}
		}
	}
	if err := integrations.CompleteConnectionSetup(c.Request.Context(), handler.deps.ConnectionRepo, handler.deps.Grants, integrations.CompleteConnectionSetupInput{
		OrganizationID:  organizationID,
		ConnectionID:    connection.ID,
		ActorID:         actorID,
		Personal:        personal,
		UsableActionIDs: usableActionIDs,
	}); err != nil {
		integrationRouteError(c, err)
		return false
	}
	return true
}

func (handler *integrationHandler) setDefaultConnection(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var item integrations.ConnectionView
	var err error
	if actorAware, supported := handler.deps.Connections.(integrations.ActorAwareConnectionService); supported {
		item, err = actorAware.SetDefaultAs(c.Request.Context(), organizationID, connectionID, &actorID)
	} else {
		item, err = handler.deps.Connections.SetDefault(c.Request.Context(), organizationID, connectionID)
	}
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, item)
}

func (handler *integrationHandler) listConnectionGrants(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	items, err := handler.deps.Grants.List(c.Request.Context(), organizationID, connectionID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	resolved, err := handler.resolveConnectionGrantPrincipals(c, organizationID, items)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{"items": resolved})
}

func (handler *integrationHandler) resolveConnectionGrantPrincipals(c *gin.Context, organizationID uuid.UUID, grants []integrations.IntegrationConnectionGrant) ([]integrationConnectionGrantResponse, error) {
	if len(grants) == 0 {
		return []integrationConnectionGrantResponse{}, nil
	}
	if handler.deps.DB == nil {
		return nil, errors.New("integration grant principal resolver is unavailable")
	}

	type principalRow struct {
		ID   string
		Name string
	}

	workspaceIDs := make([]string, 0)
	accountIDs := make([]string, 0)
	seenWorkspaces := make(map[string]struct{})
	seenAccounts := make(map[string]struct{})
	needsOrganization := false
	for _, grant := range grants {
		switch grant.PrincipalType {
		case integrations.ConnectionGrantPrincipalOrganization:
			needsOrganization = true
		case integrations.ConnectionGrantPrincipalWorkspace:
			if grant.PrincipalID == nil || *grant.PrincipalID == uuid.Nil {
				continue
			}
			id := grant.PrincipalID.String()
			if _, exists := seenWorkspaces[id]; !exists {
				seenWorkspaces[id] = struct{}{}
				workspaceIDs = append(workspaceIDs, id)
			}
		case integrations.ConnectionGrantPrincipalAccount:
			if grant.PrincipalID == nil || *grant.PrincipalID == uuid.Nil {
				continue
			}
			id := grant.PrincipalID.String()
			if _, exists := seenAccounts[id]; !exists {
				seenAccounts[id] = struct{}{}
				accountIDs = append(accountIDs, id)
			}
		}
	}

	displayNames := make(map[string]string, len(workspaceIDs)+len(accountIDs)+1)
	active := make(map[string]struct{}, len(workspaceIDs)+len(accountIDs)+1)
	principalKey := func(principalType integrations.ConnectionGrantPrincipalType, principalID string) string {
		return string(principalType) + ":" + principalID
	}

	if needsOrganization {
		var rows []principalRow
		if err := handler.deps.DB.WithContext(c.Request.Context()).Table("organizations").
			Select("id, name").
			Where("id = ? AND status = ?", organizationID.String(), "active").
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("resolve organization grant principal: %w", err)
		}
		for _, row := range rows {
			key := principalKey(integrations.ConnectionGrantPrincipalOrganization, row.ID)
			active[key] = struct{}{}
			displayNames[key] = strings.TrimSpace(row.Name)
		}
	}

	if len(workspaceIDs) > 0 {
		var rows []principalRow
		if err := handler.deps.DB.WithContext(c.Request.Context()).Table("workspaces").
			Select("id, name").
			Where("organization_id = ? AND status = ? AND id IN ?", organizationID.String(), "normal", workspaceIDs).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("resolve workspace grant principals: %w", err)
		}
		for _, row := range rows {
			key := principalKey(integrations.ConnectionGrantPrincipalWorkspace, row.ID)
			active[key] = struct{}{}
			displayNames[key] = strings.TrimSpace(row.Name)
		}
	}

	if len(accountIDs) > 0 {
		type accountPrincipalRow struct {
			ID   string
			Name string
		}
		var rows []accountPrincipalRow
		if err := handler.deps.DB.WithContext(c.Request.Context()).Table("members AS member").
			Select("member.account_id AS id, COALESCE(NULLIF(TRIM(member.name), ''), NULLIF(TRIM(account.name), ''), '') AS name").
			Joins("LEFT JOIN accounts AS account ON account.id = member.account_id AND account.deleted_at IS NULL").
			Where("member.organization_id = ? AND member.status = ? AND member.account_id IN ?", organizationID.String(), "active", accountIDs).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("resolve account grant principals: %w", err)
		}
		for _, row := range rows {
			key := principalKey(integrations.ConnectionGrantPrincipalAccount, row.ID)
			active[key] = struct{}{}
			displayNames[key] = strings.TrimSpace(row.Name)
		}
	}

	resolved := make([]integrationConnectionGrantResponse, 0, len(grants))
	for _, grant := range grants {
		principalID := organizationID.String()
		if grant.PrincipalType != integrations.ConnectionGrantPrincipalOrganization {
			if grant.PrincipalID == nil || *grant.PrincipalID == uuid.Nil {
				resolved = append(resolved, connectionGrantResponse(
					grant, "", integrationGrantPrincipalStateMissing,
				))
				continue
			}
			principalID = grant.PrincipalID.String()
		}
		key := principalKey(grant.PrincipalType, principalID)
		state := integrationGrantPrincipalStateMissing
		if _, exists := active[key]; exists {
			state = integrationGrantPrincipalStateActive
		}
		resolved = append(resolved, connectionGrantResponse(grant, displayNames[key], state))
	}
	return resolved, nil
}

func (handler *integrationHandler) createConnectionGrant(c *gin.Context) {
	organizationID, connectionID, connection, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request saveIntegrationConnectionGrantRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Revision != 0 || len(request.ResourceConstraints) > 0 {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if err := handler.validateConnectionGrantRequest(c, organizationID, connection, request); err != nil {
		integrationRouteError(c, err)
		return
	}
	grant := &integrations.IntegrationConnectionGrant{
		OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: request.PrincipalType, PrincipalID: request.PrincipalID,
		AccessMode: request.AccessMode, AllowedActionIDs: request.AllowedActionIDs,
		ResourceConstraints: map[string]interface{}{}, CreatedBy: &actorID, UpdatedBy: &actorID,
	}
	if err := handler.deps.Grants.Save(c.Request.Context(), grant, 0); err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, grant)
}

func (handler *integrationHandler) updateConnectionGrant(c *gin.Context) {
	organizationID, connectionID, connection, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	grantID, err := uuid.Parse(strings.TrimSpace(c.Param("grant_id")))
	if err != nil || grantID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request saveIntegrationConnectionGrantRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Revision < 1 || len(request.ResourceConstraints) > 0 {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	existingGrants, err := handler.deps.Grants.List(c.Request.Context(), organizationID, connectionID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	var existingGrant *integrations.IntegrationConnectionGrant
	for index := range existingGrants {
		if existingGrants[index].ID == grantID {
			existingGrant = &existingGrants[index]
			break
		}
	}
	if existingGrant == nil {
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "integration connection grant was not found", nil))
		return
	}
	if len(existingGrant.ResourceConstraints) > 0 {
		// The current management API cannot faithfully edit provider-owned
		// resource constraints. Refuse the update instead of replacing them
		// with an empty map and silently broadening access.
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeInvalidInput, "resource-constrained grants cannot be updated by this API", nil))
		return
	}
	if err := handler.validateConnectionGrantRequest(c, organizationID, connection, request); err != nil {
		integrationRouteError(c, err)
		return
	}
	grant := &integrations.IntegrationConnectionGrant{
		ID: grantID, OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: request.PrincipalType, PrincipalID: request.PrincipalID,
		AccessMode: request.AccessMode, AllowedActionIDs: request.AllowedActionIDs,
		ResourceConstraints: map[string]interface{}{}, Revision: request.Revision, UpdatedBy: &actorID,
	}
	if err := handler.deps.Grants.Save(c.Request.Context(), grant, request.Revision); err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, grant)
}

func (handler *integrationHandler) deleteConnectionGrant(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	grantID, err := uuid.Parse(strings.TrimSpace(c.Param("grant_id")))
	if err != nil || grantID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if err := handler.deps.Grants.Delete(c.Request.Context(), organizationID, connectionID, grantID); err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{"deleted": true, "id": grantID})
}

func (handler *integrationHandler) listConnectionHealthEvents(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := min(parsePositiveInt(c.Query("page_size"), 20), 100)
	items, total, err := handler.deps.HealthEvents.List(c.Request.Context(), organizationID, connectionID, page, pageSize)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{
		"items": items, "page": page, "page_size": pageSize, "total": total,
		"has_more": int64(page*pageSize) < total,
	})
}

func (handler *integrationHandler) validateConnectionGrantRequest(c *gin.Context, organizationID uuid.UUID, connection integrations.ConnectionView, request saveIntegrationConnectionGrantRequest) error {
	if len(request.AllowedActionIDs) == 0 {
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "connection grant must allow at least one action", nil)
	}
	for _, actionID := range request.AllowedActionIDs {
		actionID = strings.ToLower(strings.TrimSpace(actionID))
		if actionID == "*" {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "connection grants must name explicit provider actions", nil)
		}
		action, found := handler.deps.Registry.ActionDetail(connection.IntegrationID, actionID)
		if !found {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "connection grant contains an unknown provider action", nil)
		}
		if !integrations.ActionSupportsAuthMethod(action, connection.AuthMethodID) {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "connection grant action is not available for this connection authentication method", nil)
		}
		if request.AccessMode == integrations.ConnectionGrantAccessRead && toolgovernance.NormalizeEffect(action.Effect) != toolgovernance.EffectRead {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "read-only connection grants cannot include non-read provider actions", nil)
		}
	}
	switch request.PrincipalType {
	case integrations.ConnectionGrantPrincipalOrganization:
		if request.PrincipalID != nil {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "organization grants cannot specify a principal id", nil)
		}
	case integrations.ConnectionGrantPrincipalWorkspace:
		if request.PrincipalID == nil || *request.PrincipalID == uuid.Nil {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "workspace grant principal is required", nil)
		}
		var count int64
		if err := handler.deps.DB.WithContext(c.Request.Context()).Table("workspaces").
			Where("id = ? AND organization_id = ? AND status = ?", request.PrincipalID.String(), organizationID, "normal").Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "workspace grant principal is not an active workspace in the organization", nil)
		}
	case integrations.ConnectionGrantPrincipalAccount:
		if request.PrincipalID == nil || *request.PrincipalID == uuid.Nil {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "account grant principal is required", nil)
		}
		var count int64
		if err := handler.deps.DB.WithContext(c.Request.Context()).Table("members").
			Where("organization_id = ? AND account_id = ? AND status = ?", organizationID, *request.PrincipalID, "active").Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return integrations.NewError(integrations.ErrorCodeInvalidInput, "account grant principal is not an active organization member", nil)
		}
	default:
		return integrations.NewError(integrations.ErrorCodeInvalidInput, "connection grant principal type is invalid", nil)
	}
	return nil
}

func (handler *integrationHandler) connectionDeleteImpact(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	if err := handler.ensureConnectionReferenceIndexReady(c.Request.Context(), organizationID); err != nil {
		integrationRouteError(c, err)
		return
	}
	impact, err := handler.buildConnectionDeleteImpact(
		c.Request.Context(),
		handler.deps.DB,
		handler.agentBindingRepository(),
		organizationID,
		connectionID,
		actorID,
		time.Now().UTC(),
	)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, impact)
}

func (handler *integrationHandler) detachConnectionReferences(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request detachIntegrationConnectionReferencesRequest
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.ImpactToken) == "" {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	if handler.deps.DB == nil || handler.agentBindingRepository() == nil || handler.deps.WorkflowReferences == nil {
		integrationRouteError(c, fmt.Errorf("integration connection reference management is unavailable"))
		return
	}
	if err := handler.ensureConnectionReferenceIndexReady(c.Request.Context(), organizationID); err != nil {
		integrationRouteError(c, err)
		return
	}

	resource := agentbindings.ResourceRef{
		OrganizationID: organizationID,
		BindingType:    agentbindings.BindingTypeIntegrationConnection,
		ResourceID:     connectionID.String(),
	}
	var detachedImpact integrationConnectionDeleteImpactResponse
	verificationTime := time.Now().UTC()
	err := handler.deps.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		bindings := handler.agentBindingRepository().WithTx(tx)
		if err := bindings.LockResources(c.Request.Context(), tx, []agentbindings.ResourceRef{resource}); err != nil {
			return err
		}
		if err := bindings.VerifyImpactToken(
			c.Request.Context(),
			resource,
			detachIntegrationConnectionReferencesOperation,
			actorID,
			strings.TrimSpace(request.ImpactToken),
			verificationTime,
		); err != nil {
			return err
		}
		if _, err := handler.deps.WorkflowReferences.DetachConnectionReferences(
			c.Request.Context(), tx, organizationID, connectionID, actorID,
		); err != nil {
			return err
		}
		if _, err := bindings.RevokeAndPruneDrafts(c.Request.Context(), tx, resource, actorID); err != nil {
			return err
		}
		impact, err := handler.buildConnectionDeleteImpact(
			c.Request.Context(), tx, bindings, organizationID, connectionID, actorID, verificationTime,
		)
		if err != nil {
			return err
		}
		detachedImpact = impact
		return nil
	})
	if err != nil {
		if errors.Is(err, agentbindings.ErrImpactTokenInvalid) || errors.Is(err, agentbindings.ErrImpactChanged) {
			latest, latestErr := handler.buildConnectionDeleteImpact(
				c.Request.Context(),
				handler.deps.DB,
				handler.agentBindingRepository(),
				organizationID,
				connectionID,
				actorID,
				time.Now().UTC(),
			)
			if latestErr != nil {
				integrationRouteError(c, latestErr)
				return
			}
			c.JSON(http.StatusConflict, response.Response{
				Code:    "integration_connection_reference_impact_changed",
				Message: "connection references changed or the confirmation expired; review the latest impact",
				Data:    latest,
			})
			return
		}
		integrationRouteError(c, err)
		return
	}
	response.Success(c, detachedImpact)
}

func (handler *integrationHandler) deleteConnection(c *gin.Context) {
	organizationID, connectionID, _, ok := handler.integrationManagedConnection(c)
	if !ok {
		return
	}
	_, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	if err := handler.ensureConnectionReferenceIndexReady(c.Request.Context(), organizationID); err != nil {
		integrationRouteError(c, err)
		return
	}
	bindingsRepo := handler.agentBindingRepository()
	if bindingsRepo == nil {
		integrationRouteError(c, fmt.Errorf("integration connection binding repository is unavailable"))
		return
	}
	bindings, err := bindingsRepo.ListImpact(c.Request.Context(), agentbindings.ResourceRef{
		OrganizationID: organizationID,
		BindingType:    agentbindings.BindingTypeIntegrationConnection,
		ResourceID:     connectionID.String(),
	})
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	if len(bindings) > 0 {
		c.JSON(http.StatusConflict, response.Response{Code: "integration_connection_in_use", Message: "connection is still referenced by an Agent or Workflow"})
		return
	}
	if actorAware, supported := handler.deps.Connections.(integrations.ActorAwareConnectionService); supported {
		err = actorAware.DeleteAs(c.Request.Context(), organizationID, connectionID, &actorID)
	} else {
		err = handler.deps.Connections.Delete(c.Request.Context(), organizationID, connectionID)
	}
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, map[string]interface{}{"deleted": true, "id": connectionID})
}

func (handler *integrationHandler) ensureConnectionReferenceIndexReady(ctx context.Context, organizationID uuid.UUID) error {
	if handler == nil || handler.deps.WorkflowReferences == nil {
		// RegisterIntegrationRoutes requires this dependency. Focused handler tests
		// may construct only the dependencies exercised by their scenario.
		return nil
	}
	return handler.deps.WorkflowReferences.EnsureConnectionReferenceIndexReady(ctx, organizationID)
}

func (handler *integrationHandler) agentBindingRepository() *agentbindings.Repository {
	if handler == nil {
		return nil
	}
	if handler.deps.AgentBindings != nil {
		return handler.deps.AgentBindings
	}
	if handler.deps.DB == nil {
		return nil
	}
	return agentbindings.NewRepository(handler.deps.DB)
}

type integrationConnectionReferenceBuilder struct {
	reference integrationConnectionReference
	actions   map[string]struct{}
	nodes     map[string]*integrationConnectionReferenceNode
}

func (handler *integrationHandler) buildConnectionDeleteImpact(
	ctx context.Context,
	db *gorm.DB,
	bindingsRepo *agentbindings.Repository,
	organizationID uuid.UUID,
	connectionID uuid.UUID,
	actorID uuid.UUID,
	now time.Time,
) (integrationConnectionDeleteImpactResponse, error) {
	result := integrationConnectionDeleteImpactResponse{
		ConnectionID: connectionID,
		CanDelete:    true,
		References:   []integrationConnectionReference{},
	}
	if db == nil || bindingsRepo == nil {
		return result, fmt.Errorf("integration connection binding repository is unavailable")
	}
	resource := agentbindings.ResourceRef{
		OrganizationID: organizationID,
		BindingType:    agentbindings.BindingTypeIntegrationConnection,
		ResourceID:     connectionID.String(),
	}
	preview, bindings, err := bindingsRepo.PreviewImpactSnapshot(
		ctx,
		resource,
		detachIntegrationConnectionReferencesOperation,
		actorID,
		now,
	)
	if err != nil {
		return result, err
	}
	if len(bindings) == 0 {
		return result, nil
	}
	if preview == nil {
		return result, agentbindings.ErrImpactChanged
	}

	assetIDs := make(map[uuid.UUID]struct{})
	workspaceIDs := make(map[uuid.UUID]struct{})
	for _, binding := range bindings {
		if binding.AgentID != uuid.Nil {
			assetIDs[binding.AgentID] = struct{}{}
		}
		if binding.WorkspaceID != uuid.Nil {
			workspaceIDs[binding.WorkspaceID] = struct{}{}
		}
	}

	assetNames := make(map[uuid.UUID]string, len(assetIDs))
	if len(assetIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(assetIDs))
		for id := range assetIDs {
			ids = append(ids, id)
		}
		var rows []struct {
			ID   uuid.UUID
			Name string
		}
		if err := db.WithContext(ctx).
			Table("agents AS asset").
			Select("asset.id, asset.name").
			Joins("JOIN workspaces AS workspace ON workspace.id = asset.tenant_id").
			Where("asset.id IN ? AND workspace.organization_id = ?", ids, organizationID).
			Scan(&rows).Error; err != nil {
			return result, fmt.Errorf("load integration connection reference assets: %w", err)
		}
		for _, row := range rows {
			assetNames[row.ID] = strings.TrimSpace(row.Name)
		}
	}

	workspaceNames := make(map[uuid.UUID]string, len(workspaceIDs))
	if len(workspaceIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(workspaceIDs))
		for id := range workspaceIDs {
			ids = append(ids, id)
		}
		var rows []struct {
			ID   uuid.UUID
			Name string
		}
		if err := db.WithContext(ctx).Table("workspaces").Select("id, name").Where("id IN ? AND organization_id = ?", ids, organizationID).Scan(&rows).Error; err != nil {
			return result, fmt.Errorf("load integration connection reference workspaces: %w", err)
		}
		for _, row := range rows {
			workspaceNames[row.ID] = strings.TrimSpace(row.Name)
		}
	}

	referenceBuilders := make(map[string]*integrationConnectionReferenceBuilder)
	agentAssets := make(map[uuid.UUID]struct{})
	workflowAssets := make(map[uuid.UUID]struct{})
	for _, binding := range bindings {
		consumerType := binding.ConsumerType
		if consumerType == "" {
			consumerType = agentbindings.ConsumerTypeAgent
		}
		if consumerType != agentbindings.ConsumerTypeAgent && consumerType != agentbindings.ConsumerTypeWorkflow {
			continue
		}
		assetID := binding.AgentID
		if assetID == uuid.Nil {
			continue
		}
		assetType := string(consumerType)
		key := strings.Join([]string{assetType, assetID.String(), binding.WorkspaceID.String()}, "|")
		builder, exists := referenceBuilders[key]
		if !exists {
			builder = &integrationConnectionReferenceBuilder{
				reference: integrationConnectionReference{
					AssetID:       assetID,
					AssetType:     assetType,
					Name:          assetNames[assetID],
					WorkspaceID:   binding.WorkspaceID,
					WorkspaceName: workspaceNames[binding.WorkspaceID],
					Actions:       []string{},
					Nodes:         []integrationConnectionReferenceNode{},
				},
				actions: make(map[string]struct{}),
				nodes:   make(map[string]*integrationConnectionReferenceNode),
			}
			referenceBuilders[key] = builder
		}
		switch binding.BindingScope {
		case agentbindings.ScopeDraft:
			builder.reference.DraftReferenced = true
		case agentbindings.ScopePublished:
			builder.reference.PublishedReferenced = true
		}
		actionIDs := agentbindings.IntegrationAllowedActionIDs(binding)
		for _, actionID := range actionIDs {
			builder.actions[actionID] = struct{}{}
		}
		if consumerType == agentbindings.ConsumerTypeAgent {
			agentAssets[assetID] = struct{}{}
			continue
		}
		workflowAssets[assetID] = struct{}{}
		nodeID := strings.TrimSpace(binding.BindingInstanceID)
		if nodeID == "" {
			continue
		}
		nodeActionIDs := actionIDs
		if len(nodeActionIDs) == 0 {
			nodeActionIDs = []string{""}
		}
		for _, actionID := range nodeActionIDs {
			nodeKey := nodeID + "\x00" + actionID
			node, exists := builder.nodes[nodeKey]
			if !exists {
				node = &integrationConnectionReferenceNode{
					NodeID:   nodeID,
					NodeName: strings.TrimSpace(binding.DisplayName),
					ActionID: actionID,
				}
				builder.nodes[nodeKey] = node
			}
			if node.NodeName == "" {
				node.NodeName = strings.TrimSpace(binding.DisplayName)
			}
			switch binding.BindingScope {
			case agentbindings.ScopeDraft:
				node.DraftReferenced = true
			case agentbindings.ScopePublished:
				node.PublishedReferenced = true
			}
		}
	}

	result.References = make([]integrationConnectionReference, 0, len(referenceBuilders))
	for _, builder := range referenceBuilders {
		for actionID := range builder.actions {
			builder.reference.Actions = append(builder.reference.Actions, actionID)
		}
		sort.Strings(builder.reference.Actions)
		for _, node := range builder.nodes {
			builder.reference.Nodes = append(builder.reference.Nodes, *node)
		}
		sort.Slice(builder.reference.Nodes, func(i, j int) bool {
			left := builder.reference.Nodes[i]
			right := builder.reference.Nodes[j]
			if left.NodeID != right.NodeID {
				return left.NodeID < right.NodeID
			}
			return left.ActionID < right.ActionID
		})
		result.References = append(result.References, builder.reference)
	}
	sort.Slice(result.References, func(i, j int) bool {
		left := result.References[i]
		right := result.References[j]
		leftKey := strings.Join([]string{strings.ToLower(left.WorkspaceName), left.WorkspaceID.String(), left.AssetType, strings.ToLower(left.Name), left.AssetID.String()}, "|")
		rightKey := strings.Join([]string{strings.ToLower(right.WorkspaceName), right.WorkspaceID.String(), right.AssetType, strings.ToLower(right.Name), right.AssetID.String()}, "|")
		return leftKey < rightKey
	})
	result.BoundAgentCount = len(agentAssets)
	result.BoundWorkflowCount = len(workflowAssets)
	result.CanDelete = false
	result.ImpactToken = preview.ImpactToken
	result.ExpiresAt = preview.ExpiresAt
	return result, nil
}

func (handler *integrationHandler) listExecutions(c *gin.Context) {
	organizationID, ok := integrationOrganizationID(c)
	if !ok {
		return
	}
	filter := integrations.ExecutionListFilter{
		OrganizationID: organizationID, IntegrationID: c.Query("integration_id"), ActionID: c.Query("action_id"),
		Status: c.Query("status"), Page: parsePositiveInt(c.Query("page"), 1), PageSize: parsePositiveInt(c.Query("page_size"), 20),
	}
	if raw := strings.TrimSpace(c.Query("connection_id")); raw != "" {
		connectionID, err := uuid.Parse(raw)
		if err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		filter.ConnectionID = &connectionID
	}
	page, err := handler.deps.Executions.List(c.Request.Context(), filter)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, page)
}

func (handler *integrationHandler) listActionPolicies(c *gin.Context) {
	organizationID, ok := integrationOrganizationID(c)
	if !ok {
		return
	}
	policySet, err := handler.deps.Policies.ListVersioned(c.Request.Context(), organizationID, c.Param("integration_id"))
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, policySet)
}

func (handler *integrationHandler) replaceActionPolicies(c *gin.Context) {
	organizationID, actorID, ok := integrationActor(c)
	if !ok {
		return
	}
	var request replaceIntegrationPoliciesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	policySet, err := handler.deps.Policies.ReplaceVersioned(c.Request.Context(), organizationID, c.Param("integration_id"), request.Revision, request.Policies, &actorID)
	if err != nil {
		integrationRouteError(c, err)
		return
	}
	response.Success(c, policySet)
}

func integrationOrganizationID(c *gin.Context) (uuid.UUID, bool) {
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil || organizationID == uuid.Nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return uuid.Nil, false
	}
	return organizationID, true
}

func integrationActor(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	organizationID, ok := integrationOrganizationID(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	actorID, err := uuid.Parse(strings.TrimSpace(middleware.GetAccountID(c)))
	if err != nil || actorID == uuid.Nil {
		response.Fail(c, response.ErrUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}
	return organizationID, actorID, true
}

func (handler *integrationHandler) integrationWorkspaceID(c *gin.Context, organizationID, accountID uuid.UUID) (*uuid.UUID, bool) {
	if handler == nil || handler.workspaceScopeResolver == nil {
		writeAIChatWorkspaceScopeError(c, errors.New("AIChat workspace scope resolver is unavailable"))
		return nil, false
	}
	workspaceID, err := handler.workspaceScopeResolver.Resolve(
		c.Request.Context(),
		organizationID,
		accountID,
		util.GetWorkspaceID(c),
	)
	if err != nil {
		writeAIChatWorkspaceScopeError(c, err)
		return nil, false
	}
	return workspaceID, true
}

// integrationManagedConnection is the single authorization boundary for
// organization management endpoints that address a concrete connection.
// Account-owned connections are deliberately indistinguishable from missing
// records here: their metadata and lifecycle are available only to their owner
// through the /my-connections endpoints.
func (handler *integrationHandler) integrationManagedConnection(c *gin.Context) (uuid.UUID, uuid.UUID, integrations.ConnectionView, bool) {
	organizationID, connectionID, ok := integrationRouteIDs(c)
	if !ok {
		return uuid.Nil, uuid.Nil, integrations.ConnectionView{}, false
	}
	connection, err := handler.deps.Connections.Get(c.Request.Context(), organizationID, connectionID)
	if err != nil {
		integrationRouteError(c, err)
		return uuid.Nil, uuid.Nil, integrations.ConnectionView{}, false
	}
	switch connection.CredentialSource {
	case integrations.ConnectionCredentialSourceOrganization:
		return organizationID, connectionID, connection, true
	default:
		integrationRouteError(c, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "integration connection was not found", nil))
		return uuid.Nil, uuid.Nil, integrations.ConnectionView{}, false
	}
}

func integrationRouteIDs(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	organizationID, ok := integrationOrganizationID(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	connectionID, err := uuid.Parse(strings.TrimSpace(c.Param("id")))
	if err != nil || connectionID == uuid.Nil {
		response.Fail(c, response.ErrInvalidParam)
		return uuid.Nil, uuid.Nil, false
	}
	return organizationID, connectionID, true
}

func integrationRouteError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, integrations.ErrConnectionChanged) {
		c.JSON(http.StatusConflict, response.Response{Code: integrations.ErrorCodeConnectionConflict, Message: "integration connection changed; reload and retry"})
		return
	}
	var integrationError *integrations.Error
	if !errors.As(err, &integrationError) {
		logger.ErrorContext(c.Request.Context(), "integration route operation failed", err)
		c.JSON(http.StatusServiceUnavailable, response.Response{
			Code:    integrations.ErrorCodeUpstream,
			Message: integrationProjectedErrorMessage(c, err),
		})
		return
	}
	code := integrations.ErrorCode(err)
	switch code {
	case integrations.ErrorCodeInvalidInput, integrations.ErrorCodeConnectionInvalid,
		integrations.ErrorCodeResponseInvalid,
		integrations.ErrorCodeSensitiveInput:
		c.JSON(http.StatusBadRequest, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeAuthInvalid:
		c.JSON(http.StatusBadRequest, response.Response{Code: code, Message: err.Error(), Data: integrationProviderDiagnostics(err)})
	case integrations.ErrorCodeConnectionNotFound:
		c.JSON(http.StatusNotFound, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeAccessDenied, integrations.ErrorCodeInsufficientScope:
		c.JSON(http.StatusForbidden, response.Response{Code: code, Message: err.Error(), Data: integrationProviderDiagnostics(err)})
	case integrations.ErrorCodeDisabled,
		integrations.ErrorCodeReconnectRequired, integrations.ErrorCodeConnectionExpired:
		c.JSON(http.StatusForbidden, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeBudgetExceeded:
		c.JSON(http.StatusPaymentRequired, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeQuotaExceeded, integrations.ErrorCodeRateLimited:
		c.JSON(http.StatusTooManyRequests, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeProviderRejected:
		c.JSON(http.StatusUnprocessableEntity, response.Response{Code: code, Message: err.Error(), Data: integrationProviderDiagnostics(err)})
	case integrations.ErrorCodeTimeout:
		c.JSON(http.StatusGatewayTimeout, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeUpstream, integrations.ErrorCodeAuditFailed:
		c.JSON(http.StatusServiceUnavailable, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeConnectionConflict, integrations.ErrorCodePolicyConflict:
		c.JSON(http.StatusConflict, response.Response{Code: code, Message: err.Error()})
	case integrations.ErrorCodeConnectionInUse:
		c.JSON(http.StatusConflict, response.Response{Code: integrations.ErrorCodeConnectionInUse, Message: err.Error()})
	default:
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.FailWithMessage(c, response.ErrNotFound, "integration connection was not found")
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
	}
}

func integrationErrorProjectorMiddleware(projector *apptransport.Projector) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(integrationErrorProjectorContextKey, projector)
		c.Next()
	}
}

func integrationProjectedErrorMessage(c *gin.Context, err error) string {
	projectorValue, exists := c.Get(integrationErrorProjectorContextKey)
	projector, valid := projectorValue.(*apptransport.Projector)
	if !exists || !valid || projector == nil {
		logger.ErrorContext(c.Request.Context(), "integration application error projector is unavailable")
		return "The service encountered a problem. Try again later. If it continues, contact an administrator."
	}
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	return projector.Project(err, locale).Presentation.Message
}

func integrationProviderDiagnostics(err error) interface{} {
	diagnostics := integrations.ProviderDiagnosticsFromError(err)
	data := map[string]interface{}{}
	if diagnostics.ErrorCode != "" {
		data["provider_error_code"] = diagnostics.ErrorCode
	}
	if diagnostics.RequestID != "" {
		data["provider_request_id"] = diagnostics.RequestID
	}
	if diagnostics.HTTPStatus != 0 {
		data["provider_http_status"] = diagnostics.HTTPStatus
	}
	if len(data) == 0 {
		return nil
	}
	return data
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func clearCredentialMap(credentials map[string]string) {
	for key := range credentials {
		credentials[key] = ""
		delete(credentials, key)
	}
}

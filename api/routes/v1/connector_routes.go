package v1

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorconnections "github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorserveropsssh "github.com/zgiai/zgi/api/internal/modules/connectors/providers/serveropsssh"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

const connectorRequestBodyMaxBytes = 512 * 1024

type connectorDefinitionCatalog interface {
	Get(string) (connectorcatalog.Definition, bool)
	List(connectorcatalog.Filter) []connectorcatalog.Definition
}

type connectorConnectionService interface {
	StartKeySetup(context.Context, connectorconnections.KeySetupCommand) (connectorconnections.KeySetupResult, error)
	GetKeySetup(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorconnections.KeySetupResult, error)
	CancelKeySetup(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.KeySetupAttempt, error)
	ListConnections(context.Context, uuid.UUID, uuid.UUID, connectorconnections.ConnectionListFilter) ([]connectorcore.PersonalConnection, error)
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error)
}

type connectorBindingService interface {
	ListTargetBindings(context.Context, uuid.UUID, uuid.UUID, connectorbindings.TargetIdentity) ([]connectorcore.TargetBinding, error)
	UpsertBinding(context.Context, connectorbindings.UpsertBindingCommand) (connectorbindings.UpsertBindingResult, error)
	UpdateBindingAuthorization(context.Context, connectorbindings.UpdateBindingAuthorizationCommand) (connectorcore.TargetBinding, error)
	UpdateHostDelegation(context.Context, connectorbindings.UpdateHostDelegationCommand) (connectorcore.TargetBinding, error)
	DetachBinding(context.Context, connectorbindings.DetachBindingCommand) (connectorcore.TargetBinding, error)
	GetRemovalImpact(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorbindings.RemovalImpact, error)
}

type connectorMaintenanceService interface {
	Rename(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, string) (connectorcore.PersonalConnection, error)
	Reconnect(context.Context, connectorconnections.ReconnectCommand) (connectorconnections.ReconnectResult, error)
	Diagnose(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error)
	GetRemovalImpact(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorconnections.ConnectionRemovalImpact, error)
	Remove(context.Context, connectorconnections.RemoveConnectionCommand) error
}

var (
	_ connectorDefinitionCatalog  = (*connectorcatalog.Catalog)(nil)
	_ connectorConnectionService  = (*connectorconnections.Service)(nil)
	_ connectorBindingService     = (*connectorbindings.Service)(nil)
	_ connectorMaintenanceService = (*connectorconnections.MaintenanceService)(nil)
)

// ConnectorRouteDeps keeps the new Personal Connector HTTP boundary independent
// from the legacy integrations registry, connection service, grants, and policy
// machinery. Routes are registered only when the caller supplies explicit
// Connector-native dependencies.
type ConnectorRouteDeps struct {
	Catalog                 connectorDefinitionCatalog
	Connections             connectorConnectionService
	Bindings                connectorBindingService
	References              connectorReferenceService
	Maintenance             connectorMaintenanceService
	Onboarding              connectorOnboardingService
	HostKeyProbe            connectorserveropsssh.HostKeyProbe
	AccountService          interfaces.AccountService
	Preferences             connectorChatPreferenceService
	ApplicationErrorCatalog *appcatalog.Catalog
}

type connectorHandler struct {
	catalog        connectorDefinitionCatalog
	connections    connectorConnectionService
	bindings       connectorBindingService
	references     connectorReferenceService
	maintenance    connectorMaintenanceService
	onboarding     connectorOnboardingService
	preferences    connectorChatPreferenceService
	hostKeyProbe   connectorserveropsssh.HostKeyProbe
	errorProjector *apptransport.Projector
}

func RegisterConnectorRoutes(router *gin.RouterGroup, deps ConnectorRouteDeps) {
	if (deps.Preferences != nil || deps.Onboarding != nil) && deps.ApplicationErrorCatalog == nil {
		panic("connector preference routes require an application error catalog")
	}
	if router == nil || deps.Catalog == nil || deps.Connections == nil || deps.Bindings == nil ||
		deps.Maintenance == nil || deps.AccountService == nil {
		panic("connector routes require catalog, connection, binding, maintenance, and account services")
	}

	handler := &connectorHandler{
		references: deps.References, catalog: deps.Catalog, connections: deps.Connections, bindings: deps.Bindings, maintenance: deps.Maintenance,
		preferences: deps.Preferences, onboarding: deps.Onboarding, hostKeyProbe: deps.HostKeyProbe,
	}
	if deps.ApplicationErrorCatalog != nil {
		handler.errorProjector, _ = apptransport.NewProjector(deps.ApplicationErrorCatalog)
	}
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	handler.register(group)
}

func (handler *connectorHandler) register(group *gin.RouterGroup) {
	group.POST("/connectors/:connectorId/credential-setups", handler.startCredentialSetup)
	group.POST("/connectors/:connectorId/app-credential-setups", handler.startAppCredentialSetup)
	group.POST("/connectors/:connectorId/host-key-tests", handler.probeConnectorHostKey)
	if handler.onboarding != nil {
		handler.registerOnboarding(group)
	}
	if handler.preferences != nil {
		group.GET("/connector-preferences/chat/me", handler.getChatPreferences)
		group.PUT("/connector-preferences/chat/me/:connectorId", handler.updateChatPreference)
	}
	group.GET("/connectors/catalog", handler.listCatalog)
	group.GET("/connectors/catalog/:connectorId", handler.getCatalogItem)
	group.POST("/connectors/:connectorId/key-setups", handler.startKeySetup)
	group.GET("/connectors/key-setups/:attemptId", handler.getKeySetup)
	group.DELETE("/connectors/key-setups/:attemptId", handler.cancelKeySetup)
	group.GET("/connector-connections", handler.listConnections)
	group.GET("/connector-connections/:connectionId", handler.getConnection)
	group.PATCH("/connector-connections/:connectionId", handler.renameConnection)
	group.PUT("/connector-connections/:connectionId/operation-policy", handler.updateOperationPolicy)
	group.DELETE("/connector-connections/:connectionId", handler.removeConnection)
	group.POST("/connector-connections/:connectionId/reconnect", handler.reconnectConnection)
	group.GET("/connector-connections/:connectionId/diagnostics", handler.diagnoseConnection)
	group.GET("/connector-connections/:connectionId/removal-impact", handler.getConnectionRemovalImpact)
	group.GET("/connector-unavailable-references", handler.listUnavailableReferences)
	group.POST("/connector-unavailable-references/:referenceId/resolve", handler.resolveUnavailableReference)
	group.GET("/connector-target-bindings", handler.listTargetBindings)
	group.POST("/connector-target-bindings", handler.createTargetBinding)
	group.PATCH("/connector-target-bindings/:bindingId/authorization", handler.updateTargetBindingAuthorization)
	group.PATCH("/connector-target-bindings/:bindingId/host-delegation", handler.updateHostDelegation)
	group.DELETE("/connector-target-bindings/:bindingId", handler.detachTargetBinding)
}

type connectorCatalogResponse struct {
	Items []connectorCatalogItemResponse `json:"items"`
}

type connectorCatalogItemResponse struct {
	SetupMethod            connectorcatalog.SetupMethod            `json:"setup_method"`
	SetupAvailability      *connectorcatalog.MessagingAvailability `json:"setup_availability,omitempty"`
	ID                     string                                  `json:"id"`
	PluginID               string                                  `json:"plugin_id"`
	Name                   string                                  `json:"name"`
	NameI18n               connectorcatalog.LocalizedText          `json:"name_i18n,omitempty"`
	Description            string                                  `json:"description"`
	DescriptionI18n        connectorcatalog.LocalizedText          `json:"description_i18n,omitempty"`
	Icon                   string                                  `json:"icon,omitempty"`
	Classification         connectorcatalog.Classification         `json:"classification"`
	SupportedSurfaces      []connectorcatalog.Surface              `json:"supported_surfaces"`
	Category               connectorcatalog.ConnectorCategory      `json:"category"`
	Tags                   []connectorcatalog.ConnectorTag         `json:"tags,omitempty"`
	KeyFields              []connectorKeyFieldResponse             `json:"key_fields"`
	ExtensionSchemaVersion string                                  `json:"extension_schema_version,omitempty"`
	CredentialFields       []connectorcatalog.CredentialField      `json:"credential_fields,omitempty"`
	Setup                  *connectorcatalog.SetupDescriptor       `json:"setup,omitempty"`
	PermissionTemplate     map[string]any                          `json:"permission_template,omitempty"`
	Capabilities           []connectorCapabilityResponse           `json:"capabilities"`
	ConnectionSummary      connectorConnectionSummaryResponse      `json:"connection_summary"`
}

type connectorKeyFieldResponse struct {
	Name            string                         `json:"name"`
	Label           string                         `json:"label"`
	LabelI18n       connectorcatalog.LocalizedText `json:"label_i18n,omitempty"`
	Placeholder     string                         `json:"placeholder,omitempty"`
	PlaceholderI18n connectorcatalog.LocalizedText `json:"placeholder_i18n,omitempty"`
	HelpURL         string                         `json:"help_url,omitempty"`
	Required        bool                           `json:"required"`
	Secret          bool                           `json:"secret"`
}

type connectorCapabilityResponse struct {
	CredentialSubject string                           `json:"credential_subject,omitempty"`
	RequiredScopes    [][]string                       `json:"required_scopes,omitempty"`
	Effect            connectorcatalog.OperationEffect `json:"effect,omitempty"`
	Availability      string                           `json:"availability,omitempty"`
	Permission        string                           `json:"permission,omitempty"`
	Resource          string                           `json:"resource,omitempty"`
	ID                string                           `json:"id"`
	Name              string                           `json:"name"`
	NameI18n          connectorcatalog.LocalizedText   `json:"name_i18n,omitempty"`
	Description       string                           `json:"description"`
	DescriptionI18n   connectorcatalog.LocalizedText   `json:"description_i18n,omitempty"`
	Usable            bool                             `json:"usable"`
	UnavailableReason string                           `json:"unavailable_reason,omitempty"`
}

type connectorConnectionSummaryResponse struct {
	Count               int                     `json:"count"`
	NeedsAttentionCount int                     `json:"needs_attention_count,omitempty"`
	PrimaryConnectionID *uuid.UUID              `json:"primary_connection_id,omitempty"`
	State               connectorcore.Readiness `json:"state,omitempty"`
	ExternalDisplayName string                  `json:"external_display_name,omitempty"`
}

type connectorPersonalConnectionListResponse struct {
	Items []connectorPersonalConnectionResponse `json:"items"`
}

type connectorPersonalConnectionResponse struct {
	AllowedOperationIDs  *[]string                       `json:"allowed_operation_ids,omitempty"`
	ID                   uuid.UUID                       `json:"id"`
	ConnectorID          string                          `json:"connector_id"`
	Name                 string                          `json:"name"`
	ExternalAccountID    string                          `json:"external_account_id,omitempty"`
	ExternalDisplayName  string                          `json:"external_display_name,omitempty"`
	State                connectorcore.Readiness         `json:"state"`
	DiagnosticCode       string                          `json:"diagnostic_code,omitempty"`
	UsableOperationIDs   []string                        `json:"usable_operation_ids,omitempty"`
	CredentialConfigured bool                            `json:"credential_configured"`
	CredentialVersion    int                             `json:"credential_version,omitempty"`
	ProviderEvidence     *connectorcore.ProviderEvidence `json:"provider_evidence,omitempty"`
	Revision             int                             `json:"revision"`
	LastVerifiedAt       *time.Time                      `json:"last_verified_at,omitempty"`
	CreatedAt            time.Time                       `json:"created_at"`
	UpdatedAt            time.Time                       `json:"updated_at"`
}

type connectorPersonalConnectionDetailResponse struct {
	connectorPersonalConnectionResponse
	Capabilities []connectorCapabilityResponse    `json:"capabilities"`
	Bindings     []connectorTargetBindingResponse `json:"bindings"`
}

type connectorTargetBindingResponse struct {
	ID                uuid.UUID                    `json:"id"`
	ConnectionID      uuid.UUID                    `json:"connection_id"`
	ConnectorID       string                       `json:"connector_id"`
	Target            connectorTargetResponse      `json:"target"`
	TargetName        string                       `json:"target_name,omitempty"`
	Status            connectorcore.BindingStatus  `json:"status"`
	Revision          int                          `json:"revision"`
	AuthorizationMode string                       `json:"authorization_mode"`
	HostAudiences     []connectorcore.HostAudience `json:"host_audiences,omitempty"`
}

type connectorTargetResponse struct {
	Scope             connectorcore.TargetScope `json:"scope"`
	VersionID         string                    `json:"version_id,omitempty"`
	Type              connectorcore.TargetType  `json:"type"`
	ID                string                    `json:"id"`
	BindingInstanceID string                    `json:"binding_instance_id,omitempty"`
}

type connectorTargetRequest struct {
	Scope             connectorcore.TargetScope `json:"scope"`
	VersionID         string                    `json:"version_id,omitempty"`
	Type              connectorcore.TargetType  `json:"type"`
	ID                string                    `json:"id"`
	BindingInstanceID string                    `json:"binding_instance_id,omitempty"`
}

type connectorCreateTargetBindingRequest struct {
	ConnectionID      uuid.UUID              `json:"connection_id"`
	Target            connectorTargetRequest `json:"target"`
	ExpectedRevision  int                    `json:"expected_revision,omitempty"`
	AuthorizationMode string                 `json:"authorization_mode,omitempty"`
}

type connectorDetachTargetBindingRequest struct {
	Revision int `json:"revision"`
}

type connectorUpdateTargetBindingAuthorizationRequest struct {
	Revision          int    `json:"revision"`
	AuthorizationMode string `json:"authorization_mode"`
}

type connectorUpdateHostDelegationRequest struct {
	Revision  int                          `json:"revision"`
	Audiences []connectorcore.HostAudience `json:"audiences"`
}

type connectorTargetBindingListResponse struct {
	Items []connectorTargetBindingResponse `json:"items"`
}

type connectorKeySetupRequest struct {
	MailUsage string                  `json:"mail_usage"`
	Values    map[string]string       `json:"values"`
	Target    *connectorTargetRequest `json:"target,omitempty"`
}

type connectorRenameConnectionRequest struct {
	Revision int    `json:"revision"`
	Name     string `json:"name"`
}

type connectorReconnectRequest struct {
	MailUsage               string            `json:"mail_usage"`
	Values                  map[string]string `json:"values"`
	ConfirmUnstableIdentity bool              `json:"confirm_unstable_identity,omitempty"`
}

type connectorConnectionRemovalImpactResponse struct {
	ConnectionID    uuid.UUID                        `json:"connection_id"`
	CanRemove       bool                             `json:"can_remove"`
	ImpactToken     string                           `json:"impact_token,omitempty"`
	ExpiresAt       time.Time                        `json:"expires_at,omitempty"`
	AffectedTargets []connectorTargetBindingResponse `json:"affected_targets"`
}

type connectorKeySetupResponse struct {
	AttemptID      uuid.UUID                            `json:"attempt_id"`
	State          connectorcore.SetupState             `json:"state"`
	Connection     *connectorPersonalConnectionResponse `json:"connection,omitempty"`
	Attachment     *connectorTargetBindingResponse      `json:"attachment,omitempty"`
	ErrorCode      connectorcore.ErrorCode              `json:"error_code,omitempty"`
	Retryable      *bool                                `json:"retryable,omitempty"`
	BindingWarning string                               `json:"binding_warning,omitempty"`
}

type connectorErrorResponse struct {
	Code      connectorcore.ErrorCode `json:"code"`
	Message   string                  `json:"message"`
	Retryable bool                    `json:"retryable"`
}

func (handler *connectorHandler) listCatalog(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	filter, status, err := connectorCatalogFilter(c)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connections, err := handler.connections.ListConnections(
		c.Request.Context(), organizationID, accountID, connectorconnections.ConnectionListFilter{},
	)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connectionsByConnector := groupConnectionsByConnector(connections)
	// The member search is applied here rather than in the catalog so a
	// Connector also matches on its own Connection names, which the definition
	// catalog deliberately does not own.
	definitionFilter := filter
	definitionFilter.Query = ""
	definitions := handler.catalog.List(definitionFilter)
	items := make([]connectorCatalogItemResponse, 0, len(definitions))
	for _, definition := range definitions {
		connectorConnections := connectionsByConnector[definition.ID]
		if !connectorCatalogQueryMatches(definition, filter.Query, connectorConnections) {
			continue
		}
		item := handler.projectCatalog(definition, connectorConnections)
		if catalogItemMatchesStatus(item, status) {
			items = append(items, item)
		}
	}
	response.Success(c, connectorCatalogResponse{Items: items})
}

func (handler *connectorHandler) getCatalogItem(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectorID, err := exactConnectorID(c.Param("connectorId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	definition, found := handler.catalog.Get(connectorID.String())
	if !found {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil))
		return
	}
	connections, err := handler.connections.ListConnections(
		c.Request.Context(), organizationID, accountID,
		connectorconnections.ConnectionListFilter{ConnectorID: connectorID},
	)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, handler.projectCatalog(definition, connections))
}

func (handler *connectorHandler) startKeySetup(c *gin.Context) {
	if d, ok := handler.catalog.Get(c.Param("connectorId")); ok && d.IsMailCredentials() {
		c.Header("Cache-Control", "no-store")
	}
	if d, ok := handler.catalog.Get(c.Param("connectorId")); ok && d.IsMessagingCredentials() {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
		if !handler.messagingMethod(d.ID, "manual") {
			handler.credentialError(c, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil))
			return
		}
	}
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectorID, err := exactConnectorID(c.Param("connectorId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorKeySetupRequest
	defer func() {
		connectorcore.DestroySecretValues(request.Values)
	}()
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	handler.startParsedKeySetup(c, request, organizationID, accountID, connectorID.String())
}

func (handler *connectorHandler) startParsedKeySetup(c *gin.Context, request connectorKeySetupRequest, organizationID, accountID uuid.UUID, connectorID string) {
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	target, err := request.targetRef()
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	result, err := handler.connections.StartKeySetup(c.Request.Context(), connectorconnections.KeySetupCommand{
		OrganizationID: organizationID,
		OwnerAccountID: accountID,
		ConnectorID:    connectorID,
		IdempotencyKey: idempotencyKey,
		Values:         request.Values,
		MailUsage:      request.MailUsage,
		Target:         target,
	})
	if err != nil {
		handler.mailCredentialError(c, err, result.MailVerification)
		return
	}
	status := http.StatusAccepted
	if result.Attempt.State == connectorcore.SetupStateReady {
		status = http.StatusCreated
	}
	connectorSuccess(c, status, handler.projectKeySetupResult(c.Request.Context(), organizationID, accountID, result))
}

func (handler *connectorHandler) getKeySetup(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	attemptID, err := parseConnectorUUID(c.Param("attemptId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	result, err := handler.connections.GetKeySetup(c.Request.Context(), organizationID, accountID, attemptID)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, handler.projectKeySetupResult(c.Request.Context(), organizationID, accountID, result))
}

func (handler *connectorHandler) cancelKeySetup(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	attemptID, err := parseConnectorUUID(c.Param("attemptId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	if _, err := handler.connections.CancelKeySetup(c.Request.Context(), organizationID, accountID, attemptID); err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (handler *connectorHandler) listConnections(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	filter, target, err := connectorConnectionFilter(c)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	if target != nil {
		// Target context is an authorization filter, not a source of ambient
		// authority. Every first-release Personal Connection is cross-surface
		// eligible, but candidates are returned only for a target the owner may
		// currently edit.
		if _, err := handler.bindings.ListTargetBindings(c.Request.Context(), organizationID, accountID, *target); err != nil {
			connectorRouteError(c, err)
			return
		}
	}
	items, err := handler.connections.ListConnections(c.Request.Context(), organizationID, accountID, filter)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	projected := make([]connectorPersonalConnectionResponse, 0, len(items))
	for _, item := range items {
		projected = append(projected, projectPersonalConnection(item))
	}
	response.Success(c, connectorPersonalConnectionListResponse{Items: projected})
}

func (handler *connectorHandler) getConnection(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectionID, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connection, err := handler.connections.GetConnection(c.Request.Context(), organizationID, accountID, connectionID)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	detail, err := handler.projectConnectionDetail(c.Request.Context(), organizationID, accountID, connection)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, detail)
}

func (handler *connectorHandler) renameConnection(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectionID, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorRenameConnectionRequest
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	connection, err := handler.maintenance.Rename(
		c.Request.Context(), organizationID, accountID, connectionID, request.Revision, request.Name,
	)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, projectPersonalConnection(connection))
}

func (handler *connectorHandler) reconnectConnection(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectionID, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorReconnectRequest
	defer func() {
		connectorcore.DestroySecretValues(request.Values)
	}()
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := handler.maintenance.Reconnect(c.Request.Context(), connectorconnections.ReconnectCommand{
		OrganizationID: organizationID, OwnerAccountID: accountID, ConnectionID: connectionID,
		IdempotencyKey: idempotencyKey, Values: request.Values,
		MailUsage:               request.MailUsage,
		ConfirmUnstableIdentity: request.ConfirmUnstableIdentity,
	})
	if err != nil {
		handler.mailCredentialError(c, err, result.MailVerification)
		return
	}
	status := http.StatusAccepted
	if result.Attempt.State == connectorcore.SetupStateReady {
		status = http.StatusCreated
		if result.ReplacedExisting {
			status = http.StatusOK
		}
	}
	connectorSuccess(c, status, projectKeySetupResult(result.KeySetupResult))
}

func (handler *connectorHandler) diagnoseConnection(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectionID, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connection, err := handler.maintenance.Diagnose(c.Request.Context(), organizationID, accountID, connectionID)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	detail, err := handler.projectConnectionDetail(c.Request.Context(), organizationID, accountID, connection)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, detail)
}

func (handler *connectorHandler) getConnectionRemovalImpact(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectionID, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	impact, err := handler.maintenance.GetRemovalImpact(c.Request.Context(), organizationID, accountID, connectionID)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	affectedTargets := make([]connectorTargetBindingResponse, 0, len(impact.AffectedBindings))
	for _, binding := range impact.AffectedBindings {
		affectedTargets = append(affectedTargets, projectTargetBinding(binding))
	}
	response.Success(c, connectorConnectionRemovalImpactResponse{
		ConnectionID: impact.ConnectionID, CanRemove: impact.CanRemove,
		ImpactToken: impact.ImpactToken, ExpiresAt: impact.ExpiresAt, AffectedTargets: affectedTargets,
	})
}

func (handler *connectorHandler) removeConnection(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	connectionID, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	forceValue := strings.TrimSpace(c.Query("force"))
	force := false
	switch forceValue {
	case "", "false":
	case "true":
		force = true
	default:
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	impactToken := strings.TrimSpace(c.GetHeader("X-Impact-Token"))
	if force && impactToken == "" {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	if err := handler.maintenance.Remove(c.Request.Context(), connectorconnections.RemoveConnectionCommand{
		OrganizationID: organizationID, OwnerAccountID: accountID, ConnectionID: connectionID,
		Force: force, ImpactToken: impactToken,
	}); err != nil {
		connectorRouteError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (handler *connectorHandler) projectConnectionDetail(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	connection connectorcore.PersonalConnection,
) (connectorPersonalConnectionDetailResponse, error) {
	impact, err := handler.bindings.GetRemovalImpact(ctx, organizationID, accountID, connection.ID)
	if err != nil {
		return connectorPersonalConnectionDetailResponse{}, err
	}
	projectedBindings := make([]connectorTargetBindingResponse, 0, len(impact.AffectedBindings))
	for _, binding := range impact.AffectedBindings {
		projectedBindings = append(projectedBindings, projectTargetBinding(binding))
	}
	detail := connectorPersonalConnectionDetailResponse{
		connectorPersonalConnectionResponse: projectPersonalConnection(connection),
		Capabilities:                        []connectorCapabilityResponse{},
		Bindings:                            projectedBindings,
	}
	if definition, found := handler.catalog.Get(connection.ConnectorID.String()); found {
		detail.Capabilities = handler.projectCatalog(definition, []connectorcore.PersonalConnection{connection}).Capabilities
	}
	return detail, nil
}

func (handler *connectorHandler) listTargetBindings(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	target, err := connectorTargetIdentityFromQuery(c)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	items, err := handler.bindings.ListTargetBindings(c.Request.Context(), organizationID, accountID, target)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	projected := make([]connectorTargetBindingResponse, 0, len(items))
	for _, item := range items {
		projected = append(projected, projectTargetBinding(item))
	}
	response.Success(c, connectorTargetBindingListResponse{Items: projected})
}

func (handler *connectorHandler) createTargetBinding(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	var request connectorCreateTargetBindingRequest
	if err := decodeConnectorJSON(c, &request); err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idempotencyKey) < 16 || len(idempotencyKey) > 200 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	target, err := request.Target.targetRef()
	if err != nil || request.ConnectionID == uuid.Nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	result, err := handler.bindings.UpsertBinding(c.Request.Context(), connectorbindings.UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: accountID,
		ConnectionID: request.ConnectionID, IdempotencyKey: idempotencyKey, Target: target,
		ExpectedRevision: request.ExpectedRevision, AuthorizationMode: request.AuthorizationMode,
	})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	connectorSuccess(c, http.StatusCreated, projectTargetBinding(result.Binding))
}

func (handler *connectorHandler) detachTargetBinding(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	bindingID, err := parseConnectorUUID(c.Param("bindingId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorDetachTargetBindingRequest
	if err := decodeConnectorJSON(c, &request); err != nil || request.Revision < 1 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	detached, err := handler.bindings.DetachBinding(c.Request.Context(), connectorbindings.DetachBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: accountID,
		BindingID: bindingID, ExpectedRevision: request.Revision,
	})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, projectTargetBinding(detached))
}

func (handler *connectorHandler) updateTargetBindingAuthorization(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	bindingID, err := parseConnectorUUID(c.Param("bindingId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorUpdateTargetBindingAuthorizationRequest
	if err := decodeConnectorJSON(c, &request); err != nil || request.Revision < 1 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	updated, err := handler.bindings.UpdateBindingAuthorization(c.Request.Context(), connectorbindings.UpdateBindingAuthorizationCommand{
		OrganizationID: organizationID, ActingPrincipalID: accountID,
		BindingID: bindingID, ExpectedRevision: request.Revision,
		AuthorizationMode: request.AuthorizationMode,
	})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, projectTargetBinding(updated))
}

func (handler *connectorHandler) updateHostDelegation(c *gin.Context) {
	organizationID, accountID, ok := connectorActor(c)
	if !ok {
		return
	}
	bindingID, err := parseConnectorUUID(c.Param("bindingId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorUpdateHostDelegationRequest
	if err := decodeConnectorJSON(c, &request); err != nil || request.Revision < 1 || request.Audiences == nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil))
		return
	}
	updated, err := handler.bindings.UpdateHostDelegation(c.Request.Context(), connectorbindings.UpdateHostDelegationCommand{
		OrganizationID: organizationID, ActingPrincipalID: accountID, BindingID: bindingID,
		ExpectedRevision: request.Revision, Audiences: request.Audiences,
	})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, projectTargetBinding(updated))
}

func connectorActor(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	organizationID, organizationErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	accountID, accountErr := uuid.Parse(strings.TrimSpace(middleware.GetAccountID(c)))
	if organizationErr != nil || accountErr != nil || organizationID == uuid.Nil || accountID == uuid.Nil {
		connectorAuthError(c)
		return uuid.Nil, uuid.Nil, false
	}
	return organizationID, accountID, true
}

func connectorCatalogFilter(c *gin.Context) (connectorcatalog.Filter, string, error) {
	classification := connectorcatalog.Classification(strings.TrimSpace(c.Query("classification")))
	if classification != "" && classification != connectorcatalog.ClassificationConnector && classification != connectorcatalog.ClassificationDataSource {
		return connectorcatalog.Filter{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	query := strings.TrimSpace(c.Query("query"))
	if utf8.RuneCountInString(query) > 200 {
		return connectorcatalog.Filter{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		status = "all"
	}
	if status != "all" && status != "connected" && status != "needs_attention" {
		return connectorcatalog.Filter{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	category := connectorcatalog.ConnectorCategory(strings.TrimSpace(c.Query("category")))
	if category != "" && !category.Valid() {
		return connectorcatalog.Filter{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	tag := connectorcatalog.ConnectorTag(strings.TrimSpace(c.Query("tag")))
	if tag != "" && !tag.Valid() {
		return connectorcatalog.Filter{}, "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return connectorcatalog.Filter{Classification: classification, Query: query, Category: category, Tag: tag}, status, nil
}

func connectorConnectionFilter(c *gin.Context) (connectorconnections.ConnectionListFilter, *connectorbindings.TargetIdentity, error) {
	var filter connectorconnections.ConnectionListFilter
	if rawConnectorID := strings.TrimSpace(c.Query("connector_id")); rawConnectorID != "" {
		connectorID, err := exactConnectorID(rawConnectorID)
		if err != nil {
			return filter, nil, err
		}
		filter.ConnectorID = connectorID
	}
	rawTargetType := strings.TrimSpace(c.Query("target_type"))
	rawTargetID := strings.TrimSpace(c.Query("target_id"))
	if (rawTargetType == "") != (rawTargetID == "") {
		return filter, nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	if rawTargetType == "" {
		return filter, nil, nil
	}
	target := &connectorbindings.TargetIdentity{Type: connectorcore.TargetType(rawTargetType), ID: rawTargetID}
	if err := target.Validate(); err != nil {
		return filter, nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return filter, target, nil
}

func connectorTargetIdentityFromQuery(c *gin.Context) (connectorbindings.TargetIdentity, error) {
	target := connectorbindings.TargetIdentity{
		Type:  connectorcore.TargetType(strings.TrimSpace(c.Query("target_type"))),
		ID:    strings.TrimSpace(c.Query("target_id")),
		Scope: connectorcore.TargetScope(c.Query("target_scope")), VersionID: c.Query("target_version_id"),
	}
	if err := target.Validate(); err != nil {
		return connectorbindings.TargetIdentity{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return target, nil
}

func exactConnectorID(raw string) (connectorcore.Identifier, error) {
	identifier, err := connectorcore.NormalizeIdentifier(raw)
	if err != nil || identifier.String() != raw {
		return "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return identifier, nil
}

func parseConnectorUUID(raw string) (uuid.UUID, error) {
	identifier, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || identifier == uuid.Nil {
		return uuid.Nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return identifier, nil
}

func decodeConnectorJSON(c *gin.Context, destination any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, connectorRequestBodyMaxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("connector request must contain one JSON object")
	}
	return nil
}

func (request connectorKeySetupRequest) targetRef() (*connectorcore.TargetRef, error) {
	if request.Target == nil {
		return nil, nil
	}
	target := &connectorcore.TargetRef{
		Type: request.Target.Type, Scope: request.Target.Scope, VersionID: request.Target.VersionID,
		ID:                strings.TrimSpace(request.Target.ID),
		BindingInstanceID: strings.TrimSpace(request.Target.BindingInstanceID),
	}
	if err := target.Validate(); err != nil {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return target, nil
}

func (request connectorTargetRequest) targetRef() (connectorcore.TargetRef, error) {
	target := connectorcore.TargetRef{
		Type: request.Type, Scope: request.Scope, VersionID: request.VersionID,
		ID:                strings.TrimSpace(request.ID),
		BindingInstanceID: strings.TrimSpace(request.BindingInstanceID),
	}
	if err := target.Validate(); err != nil {
		return connectorcore.TargetRef{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	return target, nil
}

// connectorCatalogQueryMatches reports whether a catalog entry satisfies a
// member search query, either through its own definition metadata or through
// the name of one of the member's saved Connections.
func connectorCatalogQueryMatches(
	definition connectorcatalog.Definition,
	query string,
	connections []connectorcore.PersonalConnection,
) bool {
	if connectorcatalog.DefinitionMatchesQuery(definition, query) {
		return true
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	for _, connection := range connections {
		if strings.Contains(strings.ToLower(connection.Name), needle) {
			return true
		}
		if connection.ExternalDisplayName != nil &&
			strings.Contains(strings.ToLower(*connection.ExternalDisplayName), needle) {
			return true
		}
	}
	return false
}

func groupConnectionsByConnector(items []connectorcore.PersonalConnection) map[string][]connectorcore.PersonalConnection {
	result := make(map[string][]connectorcore.PersonalConnection)
	for _, item := range items {
		result[item.ConnectorID.String()] = append(result[item.ConnectorID.String()], item)
	}
	return result
}

func projectCatalogItem(definition connectorcatalog.Definition, connections []connectorcore.PersonalConnection) connectorCatalogItemResponse {
	primary := primaryConnectorConnection(connections)
	keyFields := make([]connectorKeyFieldResponse, 0, len(definition.KeyFields))
	for _, field := range definition.KeyFields {
		keyFields = append(keyFields, connectorKeyFieldResponse{
			Name: field.Name, Label: field.Label, Placeholder: field.Placeholder,
			LabelI18n: field.LabelI18n.Clone(), PlaceholderI18n: field.PlaceholderI18n.Clone(),
			HelpURL: field.HelpURL, Required: field.Required, Secret: true,
		})
	}
	capabilities := projectCapabilities(definition, connections)
	item := connectorCatalogItemResponse{
		SetupMethod: definition.SetupMethod,
		ID:          definition.ID, PluginID: definition.PluginID, Name: definition.Name,
		NameI18n: definition.NameI18n.Clone(), DescriptionI18n: definition.DescriptionI18n.Clone(),
		Description: definition.Description, Icon: definition.Icon, Classification: definition.Classification,
		SupportedSurfaces: append([]connectorcatalog.Surface{}, definition.Surfaces...),
		KeyFields:         keyFields, Capabilities: capabilities,
		ExtensionSchemaVersion: definition.ExtensionSchemaVersion, CredentialFields: definition.CredentialFields, Setup: definition.Setup,
		ConnectionSummary: connectorConnectionSummaryResponse{Count: len(connections)},
	}
	presentation, _ := connectorcatalog.PresentationFor(definition.ID)
	item.Category, item.Tags = presentation.Category, presentation.Tags
	if definition.IsAppCredentials() {
		item.PermissionTemplate = connectorcatalog.FeishuPermissionTemplate()
	}
	for _, connection := range connections {
		if connection.State != connectorcore.ReadinessReady {
			item.ConnectionSummary.NeedsAttentionCount++
		}
	}
	if primary != nil {
		primaryID := primary.ID
		item.ConnectionSummary.PrimaryConnectionID = &primaryID
		item.ConnectionSummary.State = primary.State
		if primary.ExternalDisplayName != nil {
			item.ConnectionSummary.ExternalDisplayName = strings.TrimSpace(*primary.ExternalDisplayName)
		}
	}
	return item
}

func projectCapabilities(definition connectorcatalog.Definition, connections []connectorcore.PersonalConnection) []connectorCapabilityResponse {
	capabilities := make([]connectorCapabilityResponse, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		usable := (len(connections) == 0 && !definition.UsesCredentials()) || operationUsable(connections, operation.ID)
		capability := connectorCapabilityResponse{
			CredentialSubject: operation.CredentialSubject, RequiredScopes: operation.RequiredScopes, Effect: operation.Effect,
			ID: operation.ID, Name: operation.Name, Description: operation.Description, Usable: usable,
			NameI18n: operation.NameI18n.Clone(), DescriptionI18n: operation.DescriptionI18n.Clone(),
		}
		if !usable {
			capability.UnavailableReason = "connector_capability_unavailable"
			if definition.UsesCredentials() && len(connections) == 0 {
				capability.UnavailableReason = "connector_not_verified"
			}
		}
		if definition.IsMailCredentials() {
			capability.Availability = "unverified"
			capability.Permission = "not_scoped"
			capability.Resource = "evaluated_at_execution"
			for _, connection := range connections {
				if connection.State != connectorcore.ReadinessReady || connection.ProviderEvidence == nil || connection.ProviderEvidence.Mail == nil {
					continue
				}
				for _, e := range connection.ProviderEvidence.Mail.Operations {
					if e.ID == operation.ID && operationUsable([]connectorcore.PersonalConnection{connection}, operation.ID) {
						capability.Availability = e.Availability
					}
				}
			}
		}
		if definition.IsMessagingCredentials() {
			capability.Availability = "unverified"
			capability.Permission = "unknown"
			capability.Resource = "evaluated_at_execution"
			for _, connection := range connections {
				if connection.State != connectorcore.ReadinessReady || connection.ProviderEvidence == nil || connection.ProviderEvidence.Messaging == nil {
					continue
				}
				for _, e := range connection.ProviderEvidence.Messaging.Operations {
					if e.ID == operation.ID && e.Availability != "denied" {
						capability.Availability = e.Availability
					}
				}
			}
		}
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

func primaryConnectorConnection(items []connectorcore.PersonalConnection) *connectorcore.PersonalConnection {
	for _, state := range []connectorcore.Readiness{
		connectorcore.ReadinessReady,
		connectorcore.ReadinessReconnectRequired,
		connectorcore.ReadinessUnavailable,
	} {
		for index := range items {
			if items[index].State == state {
				return &items[index]
			}
		}
	}
	return nil
}

func operationUsable(items []connectorcore.PersonalConnection, operationID string) bool {
	for _, item := range items {
		if item.State != connectorcore.ReadinessReady {
			continue
		}
		for _, candidate := range item.UsableOperationIDs {
			if candidate.String() == operationID {
				return true
			}
		}
	}
	return false
}

func catalogItemMatchesStatus(item connectorCatalogItemResponse, status string) bool {
	switch status {
	case "connected":
		return item.ConnectionSummary.Count > 0 && item.ConnectionSummary.State == connectorcore.ReadinessReady
	case "needs_attention":
		return item.ConnectionSummary.NeedsAttentionCount > 0
	default:
		return true
	}
}

func projectPersonalConnection(connection connectorcore.PersonalConnection) connectorPersonalConnectionResponse {
	result := connectorPersonalConnectionResponse{
		ID: connection.ID, ConnectorID: connection.ConnectorID.String(), Name: connection.Name,
		State: connection.State, CredentialConfigured: true, CredentialVersion: connection.CredentialVersion, ProviderEvidence: connection.ProviderEvidence.Clone(),
		Revision: connection.Revision, CreatedAt: connection.CreatedAt, UpdatedAt: connection.UpdatedAt,
	}
	if connection.Constraint != nil && connection.Constraint.Validate() == nil {
		ids := make([]string, 0, len(connection.Constraint.AllowedOperationIDs))
		for _, id := range connection.Constraint.AllowedOperationIDs {
			ids = append(ids, id.String())
		}
		result.AllowedOperationIDs = &ids
	}
	if connection.ExternalAccountID != nil {
		result.ExternalAccountID = strings.TrimSpace(*connection.ExternalAccountID)
	}
	if connection.ExternalDisplayName != nil {
		result.ExternalDisplayName = strings.TrimSpace(*connection.ExternalDisplayName)
	}
	if connection.DiagnosticCode != nil && safeConnectorCode(*connection.DiagnosticCode) {
		result.DiagnosticCode = strings.TrimSpace(*connection.DiagnosticCode)
	}
	if len(connection.UsableOperationIDs) > 0 {
		result.UsableOperationIDs = make([]string, 0, len(connection.UsableOperationIDs))
		for _, operationID := range connection.UsableOperationIDs {
			if operationID.Valid() {
				result.UsableOperationIDs = append(result.UsableOperationIDs, operationID.String())
			}
		}
	}
	if !connection.LastVerifiedAt.IsZero() {
		lastVerifiedAt := connection.LastVerifiedAt.UTC()
		result.LastVerifiedAt = &lastVerifiedAt
	}
	return result
}

func projectTargetBinding(binding connectorcore.TargetBinding) connectorTargetBindingResponse {
	var audiences []connectorcore.HostAudience
	if binding.Constraint != nil && binding.Constraint.HostDelegation != nil {
		audiences = append(audiences, binding.Constraint.HostDelegation.Audiences...)
	}
	return connectorTargetBindingResponse{
		ID: binding.ID, ConnectionID: binding.ConnectionID, ConnectorID: binding.ConnectorID.String(),
		Target: connectorTargetResponse{
			Type: binding.Target.Type, ID: binding.Target.ID, BindingInstanceID: binding.Target.BindingInstanceID,
			Scope: binding.Target.EffectiveScope(), VersionID: binding.Target.VersionID,
		},
		Status:            binding.Status,
		Revision:          binding.Revision,
		AuthorizationMode: binding.Constraint.AuthorizationModeValue(),
		HostAudiences:     audiences,
	}
}

func projectKeySetupResult(result connectorconnections.KeySetupResult) connectorKeySetupResponse {
	projected := connectorKeySetupResponse{AttemptID: result.Attempt.ID, State: result.Attempt.State}
	if result.Connection != nil {
		connection := projectPersonalConnection(*result.Connection)
		projected.Connection = &connection
	}
	if result.Attempt.State == connectorcore.SetupStateFailed {
		projected.ErrorCode = result.Attempt.ErrorCode
		retryable := result.Retryable
		projected.Retryable = &retryable
	}
	return projected
}

// projectKeySetupResult completes contextual setup without turning attachment
// failure into Connection failure. Replays derive the same binding command from
// the durable attempt UUID, so polling can finish a transiently failed bind and
// always resolves the same logical Binding UUID.
func (handler *connectorHandler) projectKeySetupResult(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	result connectorconnections.KeySetupResult,
) connectorKeySetupResponse {
	projected := projectKeySetupResult(result)
	if result.Attempt.State != connectorcore.SetupStateReady || result.Connection == nil || result.Attempt.Target == nil {
		return projected
	}
	attachment, err := handler.bindings.UpsertBinding(ctx, connectorbindings.UpsertBindingCommand{
		OrganizationID: organizationID, ActingPrincipalID: accountID,
		ConnectionID:   result.Connection.ID,
		IdempotencyKey: "key-setup:" + result.Attempt.ID.String(),
		Target:         *result.Attempt.Target,
	})
	if err != nil {
		projected.BindingWarning = string(connectorcore.ErrorCodeOf(err))
		return projected
	}
	response := projectTargetBinding(attachment.Binding)
	projected.Attachment = &response
	return projected
}

func safeConnectorCode(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > connectorcore.IdentifierMaxLength {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			(index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return true
}

func connectorSuccess(c *gin.Context, status int, data any) {
	c.JSON(status, response.Response{Code: "0", Message: "success", Data: data})
}

func (handler *connectorHandler) mailCredentialError(c *gin.Context, err error, verification *connectorcore.MailVerification) {
	verification = verification.SafeCopy()
	projectable, ok := connectorcore.ApplicationError(err)
	if verification == nil {
		handler.credentialError(c, err)
		return
	}
	if !ok || handler.errorProjector == nil {
		status, presentation := connectorLegacyError(err)
		c.JSON(status, struct {
			connectorErrorResponse
			MailVerification *connectorcore.MailVerification `json:"mail_verification"`
		}{presentation, verification})
		return
	}
	projected := handler.errorProjector.ProjectWithRequestID(projectable, apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language")), c.Writer.Header().Get("X-Request-ID")).Presentation
	retryable := false
	var domainError *connectorcore.Error
	if errors.As(err, &domainError) {
		retryable = domainError.Retryable
	}
	c.Header(apptransport.HeaderApplicationErrorCode, projected.Code.String())
	c.JSON(projected.HTTPStatus, struct {
		Code             connectorcore.ErrorCode         `json:"code"`
		Message          string                          `json:"message"`
		Retryable        bool                            `json:"retryable"`
		MailVerification *connectorcore.MailVerification `json:"mail_verification"`
	}{connectorcore.ErrorCodeOf(err), projected.Message, retryable, verification})
}

func connectorAuthError(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, connectorErrorResponse{
		Code: connectorcore.ErrorCodeAccessDenied, Message: connectorPublicMessage(connectorcore.ErrorCodeAccessDenied),
	})
}

func connectorRouteError(c *gin.Context, err error) {
	status, presentation := connectorLegacyError(err)
	c.JSON(status, presentation)
}

func connectorLegacyError(err error) (int, connectorErrorResponse) {
	code := connectorcore.ErrorCodeOf(err)
	retryable := false
	var connectorError *connectorcore.Error
	if errors.As(err, &connectorError) && connectorError != nil {
		retryable = connectorError.Retryable
	}
	status := http.StatusInternalServerError
	switch code {
	case connectorcore.ErrorCodeInvalidInput:
		status = http.StatusBadRequest
	case connectorcore.ErrorCodeNotFound:
		status = http.StatusNotFound
	case connectorcore.ErrorCodeAccessDenied:
		status = http.StatusForbidden
	case connectorcore.ErrorCodeConflict, connectorcore.ErrorCodeConnectionNotReady, connectorcore.ErrorCodeTargetNotReady, connectorcore.ErrorCodeOutcomeUnknown:
		status = http.StatusConflict
	case connectorcore.ErrorCodeInvalidKey, connectorcore.ErrorCodeInsufficientKey:
		status = http.StatusUnprocessableEntity
	case connectorcore.ErrorCodeProviderUnavailable, connectorcore.ErrorCodeConnectorUnavailable:
		status = http.StatusServiceUnavailable
	}
	return status, connectorErrorResponse{Code: code, Message: connectorPublicMessage(code), Retryable: retryable}
}

func connectorPublicMessage(code connectorcore.ErrorCode) string {
	switch code {
	case connectorcore.ErrorCodeInvalidInput:
		return "Invalid Connector request"
	case connectorcore.ErrorCodeNotFound:
		return "Connector resource not found"
	case connectorcore.ErrorCodeAccessDenied:
		return "Connector access denied"
	case connectorcore.ErrorCodeConflict:
		return "Connector request conflicts with current state"
	case connectorcore.ErrorCodeInvalidKey:
		return "The Key is invalid"
	case connectorcore.ErrorCodeInsufficientKey:
		return "The Key has insufficient access"
	case connectorcore.ErrorCodeProviderUnavailable:
		return "The provider is temporarily unavailable"
	case connectorcore.ErrorCodeConnectorUnavailable:
		return "The Connector is unavailable"
	case connectorcore.ErrorCodeConnectionNotReady:
		return "The Connection is not ready"
	case connectorcore.ErrorCodeTargetNotReady:
		return "The target is not ready"
	case connectorcore.ErrorCodeOutcomeUnknown:
		return "The operation outcome is unknown"
	default:
		return "Connector request failed"
	}
}

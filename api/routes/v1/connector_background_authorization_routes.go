package v1

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/backgroundauth"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type connectorBackgroundGrantService interface {
	IssueGrant(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, []string, map[string]string, time.Time, int) (backgroundauth.Grant, error)
	ListGrants(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]backgroundauth.Grant, error)
	RevokeGrant(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) error
}

type connectorBackgroundGrantPermissionService interface {
	CheckWorkspacePermission(context.Context, string, string, string, workspacemodel.WorkspacePermissionCode) (bool, error)
}

type connectorBackgroundGrantIssueRequest struct {
	Operations     []string          `json:"operations"`
	Destinations   map[string]string `json:"destinations"`
	ExpiresAt      time.Time         `json:"expires_at"`
	MaxInvocations int               `json:"max_invocations"`
}

type connectorBackgroundGrantResponse struct {
	ID                 uuid.UUID         `json:"id"`
	EmployeeID         uuid.UUID         `json:"employee_id"`
	VersionID          uuid.UUID         `json:"version_id"`
	BindingID          uuid.UUID         `json:"binding_id"`
	ConnectionID       uuid.UUID         `json:"connection_id"`
	Operations         []string          `json:"operations"`
	Destinations       map[string]string `json:"destinations"`
	ExpiresAt          time.Time         `json:"expires_at"`
	MaxInvocations     int               `json:"max_invocations"`
	UsedInvocations    int               `json:"used_invocations"`
	RevokedAt          *time.Time        `json:"revoked_at,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	BindingRevision    int               `json:"binding_revision"`
	ConnectionRevision int               `json:"connection_revision"`
	CredentialVersion  int               `json:"credential_version"`
	ContractDigest     string            `json:"contract_digest"`
}

func (handler *connectorHandler) registerBackgroundAuthorization(group *gin.RouterGroup) {
	if handler == nil || group == nil || handler.backgroundGrants == nil || handler.backgroundGrantPermissions == nil {
		return
	}
	group.POST("/connector-target-bindings/:bindingId/background-grants", handler.issueBackgroundGrant)
	group.GET("/connector-background-grants", handler.listBackgroundGrants)
	group.DELETE("/connector-background-grants/:grantId", handler.revokeBackgroundGrant)
}

func (handler *connectorHandler) issueBackgroundGrant(c *gin.Context) {
	organizationID, workspaceID, ownerID, ok := handler.backgroundGrantScope(c)
	if !ok {
		return
	}
	bindingID, err := parseConnectorUUID(c.Param("bindingId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request connectorBackgroundGrantIssueRequest
	if err := decodeConnectorJSON(c, &request); err != nil || len(request.Operations) == 0 || request.ExpiresAt.IsZero() || request.MaxInvocations < 1 {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, err))
		return
	}
	grant, err := handler.backgroundGrants.IssueGrant(c.Request.Context(), organizationID, workspaceID, ownerID, bindingID,
		request.Operations, request.Destinations, request.ExpiresAt, request.MaxInvocations)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	projected, err := projectConnectorBackgroundGrant(grant)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, projected)
}

func (handler *connectorHandler) listBackgroundGrants(c *gin.Context) {
	organizationID, workspaceID, ownerID, ok := handler.backgroundGrantScope(c)
	if !ok {
		return
	}
	grants, err := handler.backgroundGrants.ListGrants(c.Request.Context(), organizationID, workspaceID, ownerID)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	items := make([]connectorBackgroundGrantResponse, 0, len(grants))
	for _, grant := range grants {
		projected, projectionErr := projectConnectorBackgroundGrant(grant)
		if projectionErr != nil {
			connectorRouteError(c, projectionErr)
			return
		}
		items = append(items, projected)
	}
	response.Success(c, struct {
		Items []connectorBackgroundGrantResponse `json:"items"`
	}{Items: items})
}

func (handler *connectorHandler) revokeBackgroundGrant(c *gin.Context) {
	organizationID, workspaceID, ownerID, ok := handler.backgroundGrantScope(c)
	if !ok {
		return
	}
	grantID, err := parseConnectorUUID(c.Param("grantId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	if err := handler.backgroundGrants.RevokeGrant(c.Request.Context(), organizationID, workspaceID, ownerID, grantID); err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, struct {
		ID uuid.UUID `json:"id"`
	}{ID: grantID})
}

func (handler *connectorHandler) backgroundGrantScope(c *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	organizationID, organizationErr := uuid.Parse(util.GetOrganizationID(c))
	workspaceID, workspaceErr := uuid.Parse(util.GetWorkspaceID(c))
	ownerID, ownerErr := uuid.Parse(middleware.GetAccountID(c))
	if organizationErr != nil || workspaceErr != nil || ownerErr != nil || organizationID == uuid.Nil || workspaceID == uuid.Nil || ownerID == uuid.Nil {
		connectorAuthError(c)
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	allowed, err := handler.backgroundGrantPermissions.CheckWorkspacePermission(c.Request.Context(), organizationID.String(), workspaceID.String(), ownerID.String(), workspacemodel.WorkspacePermissionDigitalHumanUpdate)
	if err != nil {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, err))
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	if !allowed {
		connectorRouteError(c, connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil))
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return organizationID, workspaceID, ownerID, true
}

func projectConnectorBackgroundGrant(grant backgroundauth.Grant) (connectorBackgroundGrantResponse, error) {
	var operations []string
	var destinations map[string]string
	if json.Unmarshal([]byte(grant.OperationsJSON), &operations) != nil || json.Unmarshal([]byte(grant.DestinationsJSON), &destinations) != nil || operations == nil || destinations == nil {
		return connectorBackgroundGrantResponse{}, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return connectorBackgroundGrantResponse{ID: grant.ID, EmployeeID: grant.EmployeeID, VersionID: grant.VersionID,
		BindingID: grant.BindingID, ConnectionID: grant.ConnectionID, Operations: operations, Destinations: destinations,
		ExpiresAt: grant.ExpiresAt, MaxInvocations: grant.MaxInvocations, UsedInvocations: grant.UsedInvocations,
		RevokedAt: grant.RevokedAt, CreatedAt: grant.CreatedAt, BindingRevision: grant.BindingRevision,
		ConnectionRevision: grant.ConnectionRevision, CredentialVersion: grant.CredentialVersion, ContractDigest: grant.ContractDigest}, nil
}

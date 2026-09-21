package v1

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/pkg/response"
)

type connectorReferenceService interface {
	List(context.Context, uuid.UUID, uuid.UUID, bindings.TargetIdentity) ([]core.UnavailableReference, error)
	ResolveChat(context.Context, uuid.UUID, uuid.UUID, bindings.TargetIdentity, bindings.ReferenceCommand) (*core.TargetBinding, error)
}

func (h *connectorHandler) listUnavailableReferences(c *gin.Context) {
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	target, err := connectorTargetIdentityFromQuery(c)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	if h.references == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInternal, false, nil))
		return
	}
	items, err := h.references.List(c.Request.Context(), org, owner, target)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items})
}
func (h *connectorHandler) resolveUnavailableReference(c *gin.Context) {
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	var request struct {
		Target connectorTargetRequest `json:"target"`
		bindings.ReferenceCommand
	}
	if decodeConnectorJSON(c, &request) != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	id, err := parseConnectorUUID(c.Param("referenceId"))
	if err != nil || request.ReferenceID != uuid.Nil && request.ReferenceID != id {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	request.ReferenceID = id
	if h.references == nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInternal, false, nil))
		return
	}
	target := bindings.TargetIdentity{Type: request.Target.Type, ID: request.Target.ID, Scope: request.Target.Scope, VersionID: request.Target.VersionID}
	result, err := h.references.ResolveChat(c.Request.Context(), org, owner, target, request.ReferenceCommand)
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	if result == nil {
		response.Success(c, gin.H{"status": "detached"})
		return
	}
	response.Success(c, gin.H{"status": "explicitly_replaced", "binding": projectTargetBinding(*result)})
}

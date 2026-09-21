package v1

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/pkg/response"
)

type connectorOperationPolicyService interface {
	UpdateOperationPolicy(context.Context, connections.OperationPolicyCommand) (connections.OperationPolicyResult, error)
}

func (handler *connectorHandler) updateOperationPolicy(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	org, owner, ok := connectorActor(c)
	if !ok {
		return
	}
	id, err := parseConnectorUUID(c.Param("connectionId"))
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	var request struct {
		ExpectedRevision int      `json:"expected_revision"`
		OperationIDs     []string `json:"operation_ids"`
	}
	raw, readErr := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, connectorRequestBodyMaxBytes))
	defer clear(raw)
	if readErr != nil || !providers.ValidJSONObject(raw) {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if decodeConnectorJSON(c, &request) != nil {
		connectorRouteError(c, core.NewError(core.ErrorCodeInvalidInput, false, nil))
		return
	}
	service, ok := handler.maintenance.(connectorOperationPolicyService)
	if !ok {
		connectorRouteError(c, core.NewError(core.ErrorCodeConnectorUnavailable, false, nil))
		return
	}
	result, err := service.UpdateOperationPolicy(c.Request.Context(), connections.OperationPolicyCommand{OrganizationID: org, OwnerAccountID: owner, ConnectionID: id, ExpectedRevision: request.ExpectedRevision, OperationIDs: request.OperationIDs, IdempotencyKey: c.GetHeader("Idempotency-Key")})
	if err != nil {
		connectorRouteError(c, err)
		return
	}
	response.Success(c, result)
}

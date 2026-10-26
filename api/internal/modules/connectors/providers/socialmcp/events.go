package socialmcp

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/socialevents"
)

// EventStore receives only the server-restored owner and connection scope. No
// platform credentials or actor selectors from tool inputs cross this boundary.
type EventStore interface {
	List(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, int) (socialevents.Page, error)
	Acknowledge(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64) error
}

func localEventOperation(operation string) bool {
	return operation == "facebook-social.event.list" || operation == "facebook-social.event.ack"
}

// InvokeWithScope is called by the native runtime after execution authorization.
// The ordinary KeyDriver path cannot invoke connection-scoped local operations.
func (c *Client) InvokeWithScope(ctx context.Context, operation, token string, input map[string]any, scope providers.ResourceScope) (providers.Response, error) {
	if !localEventOperation(operation) {
		return c.Invoke(ctx, operation, token, input)
	}
	if c.definition.ID != "facebook-social" || c.options.Events == nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	if scope.OrganizationID == uuid.Nil || scope.OwnerAccountID == uuid.Nil || scope.ConnectionID == uuid.Nil || scope.CredentialVersion < 1 {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeAccessDenied}
	}
	args, err := c.schemas.Input(operation, input)
	if err != nil {
		return providers.Response{}, providers.Invalid()
	}
	var params struct {
		AfterID   int64 `json:"after_id"`
		Limit     int   `json:"limit"`
		ThroughID int64 `json:"through_id"`
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil || json.Unmarshal(encodedArgs, &params) != nil {
		return providers.Response{}, providers.Invalid()
	}
	var data any
	if operation == "facebook-social.event.list" {
		data, err = c.options.Events.List(ctx, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, params.AfterID, params.Limit)
	} else {
		through := params.ThroughID
		err = c.options.Events.Acknowledge(ctx, scope.OrganizationID, scope.OwnerAccountID, scope.ConnectionID, through)
		data = map[string]any{"through_id": through, "acknowledged": true}
	}
	if err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOf(err)}
	}
	// Normalize typed queue DTOs into the native provider JSON object contract.
	raw, err := json.Marshal(data)
	if err != nil || len(raw) > 60<<10 {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	var normalized map[string]any
	if json.Unmarshal(raw, &normalized) != nil || normalized == nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	value := map[string]any{"provider": c.definition.ID, "transport": "local", "data": normalized}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > 60<<10 || c.schemas.Output(operation, value) != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	return providers.Response{Value: value}, nil
}

var _ providers.ScopedKeyDriver = (*Client)(nil)

package v1

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type operationPolicyRouteService struct {
	connectorRouteRecordingMaintenanceService
	command connections.OperationPolicyCommand
	count   int
	err     error
}

func (s *operationPolicyRouteService) UpdateOperationPolicy(_ context.Context, c connections.OperationPolicyCommand) (connections.OperationPolicyResult, error) {
	s.command = c
	s.count++
	return connections.OperationPolicyResult{ConnectionID: c.ConnectionID, Revision: c.ExpectedRevision + 1, OperationIDs: c.OperationIDs}, s.err
}
func TestOperationPolicyRouteScopeStrictBodyAndConflict(t *testing.T) {
	org, owner, id := uuid.New(), uuid.New(), uuid.New()
	s := &operationPolicyRouteService{}
	router := newConnectorRouteTestRouterWithMaintenance(nil, nil, s, org, owner, nil)
	path := "/console/api/connector-connections/" + id.String() + "/operation-policy"
	key := uuid.NewString()
	headers := map[string]string{"Idempotency-Key": key}
	response := connectorRouteRequest(t, router, "PUT", path, `{"expected_revision":3,"operation_ids":[]}`, headers)
	if response.Code != 200 || response.Header().Get("Cache-Control") != "no-store" || !strings.Contains(response.Body.String(), `"operation_ids":[]`) {
		t.Fatal("empty policy response", response.Code)
	}
	if s.command.OrganizationID != org || s.command.OwnerAccountID != owner || s.command.ConnectionID != id || s.command.IdempotencyKey != key || s.command.ExpectedRevision != 3 {
		t.Fatal("scope or revision lost")
	}
	before := s.count
	for _, body := range []string{`{"expected_revision":3,"operation_ids":[],"owner_account_id":"forged"}`, `{"expected_revision":3,"expected_revision":4,"operation_ids":[]}`, `{"expected_revision":3,"operation_ids":"all"}`} {
		response = connectorRouteRequest(t, router, "PUT", path, body, headers)
		if response.Code != 400 || s.count != before {
			t.Fatal("invalid body reached service", response.Code)
		}
	}
	s.err = core.NewError(core.ErrorCodeConflict, false, nil)
	response = connectorRouteRequest(t, router, "PUT", path, `{"expected_revision":3,"operation_ids":[]}`, headers)
	if response.Code != 409 {
		t.Fatal("revision conflict mapping", response.Code)
	}
	s.err = core.NewError(core.ErrorCodeNotFound, false, nil)
	response = connectorRouteRequest(t, router, "PUT", path, `{"expected_revision":3,"operation_ids":[]}`, headers)
	if response.Code != 404 {
		t.Fatal("owner miss mapping", response.Code)
	}
}
